// Package replica provides the required code for starting and running a replica and handling client requests.
package replica

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"github.com/relab/gorums"
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/backend"
	"github.com/relab/hotstuff/eventloop"
	pb "github.com/relab/hotstuff/internal/proto/robusthotstuffpb"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/pro-hotstuff/adapters"
	"github.com/wisecoach/pro-hotstuff/api"
	"github.com/wisecoach/pro-hotstuff/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/protobuf/types/known/emptypb"
	"net"
	"time"
)

// cmdID is a unique identifier for a command
type cmdID struct {
	clientID    uint32
	sequenceNum uint64
}

// Config configures a replica.
type Config struct {
	// The id of the replica.
	ID hotstuff.ID
	// The private key of the replica.
	PrivateKey hotstuff.PrivateKey
	// Controls whether TLS is used.
	TLS bool
	// The TLS certificate.
	Certificate *tls.Certificate
	// The root certificates trusted by the replica.
	RootCAs *x509.CertPool
	// The number of client commands that should be batched together in a block.
	BatchSize uint32
	// Options for the client server.
	ClientServerOptions []gorums.ServerOption
	// Options for the replica server.
	ReplicaServerOptions []gorums.ServerOption
	// Options for the replica manager.
	ManagerOptions []gorums.ManagerOption
	// Location information of all replicas
	LocationInfo map[hotstuff.ID]string
	// rate of replica join in new epoch
	NewEpochRate float64
	// time to start new epoch
	NewEpochDuration time.Duration
}

// Replica is a participant in the consensus protocol.
type Replica struct {
	clientSrv *clientSrv
	cfg       *backend.Config
	hsSrv     *backend.Server
	hs        *modules.Core
	Consensus api.Consensus
	Conf      Config

	execHandlers map[cmdID]func(*emptypb.Empty, error)
	cancel       context.CancelFunc
	done         chan struct{}
}

// New returns a new replica.
func New(conf Config, builder modules.Builder) (replica *Replica) {
	clientSrvOpts := conf.ClientServerOptions

	if conf.TLS {
		clientSrvOpts = append(clientSrvOpts, gorums.WithGRPCServerOptions(
			grpc.Creds(credentials.NewServerTLSFromCert(conf.Certificate)),
		))
	}

	clientSrv := newClientServer(conf, clientSrvOpts)

	srv := &Replica{
		Conf:         conf,
		clientSrv:    clientSrv,
		execHandlers: make(map[cmdID]func(*emptypb.Empty, error)),
		cancel:       func() {},
		done:         make(chan struct{}),
	}

	replicaSrvOpts := conf.ReplicaServerOptions
	if conf.TLS {
		replicaSrvOpts = append(replicaSrvOpts, gorums.WithGRPCServerOptions(
			grpc.Creds(credentials.NewTLS(&tls.Config{
				Certificates: []tls.Certificate{*conf.Certificate},
				ClientCAs:    conf.RootCAs,
				ClientAuth:   tls.RequireAndVerifyClientCert,
			})),
		))
	}

	srv.hsSrv = backend.NewServer(
		backend.WithLatencyInfo(conf.ID, conf.LocationInfo),
		backend.WithGorumsServerOptions(replicaSrvOpts...),
	)

	var creds credentials.TransportCredentials
	managerOpts := conf.ManagerOptions
	if conf.TLS {
		creds = credentials.NewTLS(&tls.Config{
			RootCAs:      conf.RootCAs,
			Certificates: []tls.Certificate{*conf.Certificate},
		})
	}
	srv.cfg = backend.NewConfig(creds, managerOpts...)

	builder.Add(
		srv.cfg,   // configuration
		srv.hsSrv, // event handling
		clientSrv,

		modules.ExtendedExecutor(srv.clientSrv),
		modules.ExtendedForkHandler(srv.clientSrv),
		srv.clientSrv.cmdCache,
	)
	srv.hs = builder.Build()
	srv.hs.Get(&srv.Consensus)

	return srv
}

// Modules returns the Modules object of this replica.
func (srv *Replica) Modules() *modules.Core {
	return srv.hs
}

// StartServers starts the client and replica servers.
func (srv *Replica) StartServers(replicaListen, clientListen net.Listener) {
	var comm *adapters.CommAdapter
	srv.hs.Get(&comm)
	grpcServer := grpc.NewServer()
	pb.RegisterHotstuffServer(grpcServer, comm.Server)
	go func() {
		err := grpcServer.Serve(replicaListen)
		if err != nil {
			return
		}
	}()

	srv.clientSrv.StartOnListener(clientListen)
}

func (srv *Replica) StartClientServer(clientListen net.Listener) {
	srv.clientSrv.StartOnListener(clientListen)
}

// Connect connects to the other replicas.
func (srv *Replica) Connect(replicas []backend.ReplicaInfo) error {
	return srv.cfg.Connect(replicas)
}

// Start runs the replica in a goroutine.
func (srv *Replica) Start() {
	var ctx context.Context
	ctx, srv.cancel = context.WithCancel(context.Background())
	go func() {
		srv.Run(ctx)
		close(srv.done)
	}()
}

// Stop stops the replica and closes connections.
func (srv *Replica) Stop() {
	srv.cancel()
	<-srv.done
	srv.Close()
}

// Run runs the replica until the context is cancelled.
func (srv *Replica) Run(ctx context.Context) {
	var (
		// synchronizer modules.Synchronizer
		eventLoop        *eventloop.EventLoop
		consensusAdapter *adapters.ConsensusAdapter
	)
	srv.hs.Get(&eventLoop, &consensusAdapter)

	// synchronizer.Start(ctx)
	consensusAdapter.Consensus.Start()

	bk := types.Block(hotstuff.GetGenesis())

	<-time.After(1 * time.Second)

	srv.Consensus.ProcessBlock(bk)

	eventLoop.Run(ctx)
}

// Close closes the connections and stops the servers used by the replica.
func (srv *Replica) Close() {
	srv.clientSrv.Stop()
	srv.cfg.Close()
	srv.hsSrv.Stop()
}

// GetHash returns the hash of all executed commands.
func (srv *Replica) GetHash() (b []byte) {
	return srv.clientSrv.hash.Sum(b)
}

// GetCmdCount returns the count of all executed commands.
func (srv *Replica) GetCmdCount() (c uint32) {
	return srv.clientSrv.cmdCount
}
