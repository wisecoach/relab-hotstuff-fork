// Package gorums implements a backend for HotStuff using the gorums framework.
package gorums

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/hotstuff/config"
	"github.com/relab/hotstuff/consensus"
	"github.com/relab/hotstuff/internal/proto/hotstuffpb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type gorumsReplica struct {
	node          *hotstuffpb.Node
	id            consensus.ID
	pubKey        consensus.PublicKey
	voteCancel    context.CancelFunc
	newviewCancel context.CancelFunc
}

// ID returns the replica's ID.
func (r *gorumsReplica) ID() consensus.ID {
	return r.id
}

// PublicKey returns the replica's public key.
func (r *gorumsReplica) PublicKey() consensus.PublicKey {
	return r.pubKey
}

// Vote sends the partial certificate to the other replica.
func (r *gorumsReplica) Vote(cert consensus.PartialCert) {
	if r.node == nil {
		return
	}
	var ctx context.Context
	r.voteCancel()
	ctx, r.voteCancel = context.WithCancel(context.Background())
	pCert := hotstuffpb.PartialCertToProto(cert)
	r.node.Vote(ctx, pCert, gorums.WithNoSendWaiting())
}

// NewView sends the quorum certificate to the other replica.
func (r *gorumsReplica) NewView(msg consensus.SyncInfo) {
	if r.node == nil {
		return
	}
	var ctx context.Context
	r.newviewCancel()
	ctx, r.newviewCancel = context.WithCancel(context.Background())
	r.node.NewView(ctx, hotstuffpb.SyncInfoToProto(msg), gorums.WithNoSendWaiting())
}

// Config holds information about the current configuration of replicas that participate in the protocol,
// and some information about the local replica. It also provides methods to send messages to the other replicas.
type Config struct {
	mod *consensus.Modules

	replicaCfg    config.ReplicaConfig
	mgr           *hotstuffpb.Manager
	cfg           *hotstuffpb.Configuration
	privKey       consensus.PrivateKey
	replicas      map[consensus.ID]consensus.Replica
	proposeCancel context.CancelFunc
	timeoutCancel context.CancelFunc
}

// InitModule gives the module a reference to the HotStuff object.
func (cfg *Config) InitModule(hs *consensus.Modules, _ *consensus.OptionsBuilder) {
	cfg.mod = hs
}

// NewConfig creates a new configuration.
func NewConfig(replicaCfg config.ReplicaConfig) *Config {
	cfg := &Config{
		replicaCfg:    replicaCfg,
		privKey:       replicaCfg.PrivateKey,
		replicas:      make(map[consensus.ID]consensus.Replica),
		proposeCancel: func() {},
		timeoutCancel: func() {},
	}

	for id, r := range replicaCfg.Replicas {
		cfg.replicas[id] = &gorumsReplica{
			id:            r.ID,
			pubKey:        r.PubKey,
			voteCancel:    func() {},
			newviewCancel: func() {},
		}
	}

	return cfg
}

// Connect opens connections to the replicas in the configuration.
func (cfg *Config) Connect(connectTimeout time.Duration) error {
	idMapping := make(map[string]uint32, len(cfg.replicaCfg.Replicas)-1)
	for _, replica := range cfg.replicaCfg.Replicas {
		if replica.ID != cfg.replicaCfg.ID {
			idMapping[replica.Address] = uint32(replica.ID)
		}
	}

	// embed own ID to allow other replicas to identify messages from this replica
	md := metadata.New(map[string]string{
		"id": fmt.Sprintf("%d", cfg.replicaCfg.ID),
	})

	mgrOpts := []gorums.ManagerOption{
		gorums.WithDialTimeout(connectTimeout),
		gorums.WithMetadata(md),
	}
	grpcOpts := []grpc.DialOption{
		grpc.WithBlock(),
		grpc.WithReturnConnectionError(),
	}

	if cfg.replicaCfg.Creds != nil {
		grpcOpts = append(grpcOpts, grpc.WithTransportCredentials(cfg.replicaCfg.Creds))
	} else {
		grpcOpts = append(grpcOpts, grpc.WithInsecure())
	}

	mgrOpts = append(mgrOpts, gorums.WithGrpcDialOptions(grpcOpts...))

	var err error
	cfg.mgr = hotstuffpb.NewManager(mgrOpts...)

	cfg.cfg, err = cfg.mgr.NewConfiguration(qspec{}, gorums.WithNodeMap(idMapping))
	if err != nil {
		return fmt.Errorf("failed to create configuration: %w", err)
	}

	for _, node := range cfg.cfg.Nodes() {
		id := consensus.ID(node.ID())
		replica := cfg.replicas[id].(*gorumsReplica)
		replica.node = node
	}

	return nil
}

// ID returns the id of this replica.
func (cfg *Config) ID() consensus.ID {
	return cfg.replicaCfg.ID
}

// PrivateKey returns the id of this replica.
func (cfg *Config) PrivateKey() consensus.PrivateKey {
	return cfg.privKey
}

// Replicas returns all of the replicas in the configuration.
func (cfg *Config) Replicas() map[consensus.ID]consensus.Replica {
	return cfg.replicas
}

// Replica returns a replica if it is present in the configuration.
func (cfg *Config) Replica(id consensus.ID) (replica consensus.Replica, ok bool) {
	replica, ok = cfg.replicas[id]
	return
}

// Len returns the number of replicas in the configuration.
func (cfg *Config) Len() int {
	return len(cfg.replicas)
}

// QuorumSize returns the size of a quorum
func (cfg *Config) QuorumSize() int {
	return consensus.QuorumSize(cfg.Len())
}

// Propose sends the block to all replicas in the configuration
func (cfg *Config) Propose(proposal consensus.ProposeMsg) {
	if cfg.cfg == nil {
		return
	}
	var ctx context.Context
	cfg.proposeCancel()
	ctx, cfg.proposeCancel = context.WithCancel(context.Background())
	p := hotstuffpb.ProposalToProto(proposal)
	cfg.cfg.Propose(ctx, p, gorums.WithNoSendWaiting())
}

// Timeout sends the timeout message to all replicas.
func (cfg *Config) Timeout(msg consensus.TimeoutMsg) {
	if cfg.cfg == nil {
		return
	}
	var ctx context.Context
	cfg.timeoutCancel()
	ctx, cfg.timeoutCancel = context.WithCancel(context.Background())
	cfg.cfg.Timeout(ctx, hotstuffpb.TimeoutMsgToProto(msg), gorums.WithNoSendWaiting())
}

// Fetch requests a block from all the replicas in the configuration
func (cfg *Config) Fetch(ctx context.Context, hash consensus.Hash) (*consensus.Block, bool) {
	protoBlock, err := cfg.cfg.Fetch(ctx, &hotstuffpb.BlockHash{Hash: hash[:]})
	if err != nil && !errors.Is(err, context.Canceled) {
		cfg.mod.Logger().Infof("Failed to fetch block: %v", err)
		return nil, false
	}
	return hotstuffpb.BlockFromProto(protoBlock), true
}

// Close closes all connections made by this configuration.
func (cfg *Config) Close() {
	cfg.mgr.Close()
}

var _ consensus.Configuration = (*Config)(nil)

type qspec struct{}

// FetchQF is the quorum function for the Fetch quorum call method.
// It simply returns true if one of the replies matches the requested block.
func (q qspec) FetchQF(in *hotstuffpb.BlockHash, replies map[uint32]*hotstuffpb.Block) (*hotstuffpb.Block, bool) {
	var h consensus.Hash
	copy(h[:], in.GetHash())
	for _, b := range replies {
		block := hotstuffpb.BlockFromProto(b)
		if h == block.Hash() {
			return b, true
		}
	}
	return nil, false
}
