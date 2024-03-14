package comm

import (
	"context"
	"github.com/pkg/errors"
	pb "github.com/relab/hotstuff/internal/proto/robusthotstuffpb"
	"github.com/wisecoach/robust-hotstuff/logging"
	"github.com/wisecoach/robust-hotstuff/proto"
	"github.com/wisecoach/robust-hotstuff/types"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"sync"
	"time"
)

type ConsensusClient struct {
}

func New(dialer SecureDialer, compareCert CertificateComparator, H Handler) *Comm {
	return &Comm{
		consensusLock:                    sync.Mutex{},
		StreamsByType:                    make(map[string]*Stream),
		rpcLock:                          sync.RWMutex{},
		Timeout:                          time.Hour,
		logger:                           nil,
		MinimumExpirationWarningInterval: time.Hour,
		CertExpWarningThreshold:          time.Hour,
		shutdownSignal:                   make(chan struct{}),
		shutdown:                         false,
		SendBufferSize:                   2000,
		Lock:                             sync.RWMutex{},
		H:                                H,
		Connections:                      NewConnectionStore(dialer),
		Members:                          &MemberMapping{id2stub: make(map[string]*Stub), SamePublicKey: compareCert},
		CompareCertificate:               compareCert,
	}
}

type Comm struct {
	consensusLock sync.Mutex
	StreamsByType map[string]*Stream
	rpcLock       sync.RWMutex
	Timeout       time.Duration
	logger        logging.Logger

	MinimumExpirationWarningInterval time.Duration
	CertExpWarningThreshold          time.Duration
	shutdownSignal                   chan struct{}
	shutdown                         bool
	SendBufferSize                   int
	Lock                             sync.RWMutex
	H                                Handler
	Connections                      *ConnectionStore
	Members                          *MemberMapping
	CompareCertificate               CertificateComparator
}

func (c *Comm) Shutdown() {
	// TODO implement me
	panic("implement me")
}

type requestContext struct {
	sender string
}

func (c *Comm) Send(id types.ID, msg *proto.Message) error {
	c.logger.Debugf("begin to send message, to = %s, type = %s", id, msg.Type)
	dest := string(id)

	stream, err := c.getOrCreateStream(dest)
	if err != nil {
		c.logger.Error("failed to send message", zap.Error(err))
		return err
	}

	// c.consensusLock.Lock()
	// defer c.consensusLock.Unlock()

	c.logger.Debugf("sending message, to = %s, type = %s", id, msg.Type)

	err = stream.Send(msg)
	if err != nil {
		c.logger.Error("failed to send message", zap.Error(err))
	}
	return err
}

func (c *Comm) Broadcast(msg *proto.Message) error {
	streamsToSend := make(map[string]*Stream)
	for id := range c.Members.id2stub {
		stream, err := c.getOrCreateStream(id)
		if err == nil {
			streamsToSend[id] = stream
		}
	}

	// c.consensusLock.Lock()
	// defer c.consensusLock.Unlock()
	c.logger.Debugf("broadcast message, type = %s, num = %d", msg.Type, len(streamsToSend))

	job := sync.WaitGroup{}
	job.Add(len(streamsToSend))
	for _, stream := range streamsToSend {
		go func(stream *Stream) {
			err := stream.Send(msg)
			if err != nil {
				c.logger.Error("failed to broadcast message", zap.Error(err))
			}
			job.Done()
		}(stream)
	}
	job.Wait()

	return nil
}

func (c *Comm) Configure(nodes map[string]*types.RemoteNode) {

	c.rpcLock.Lock()
	defer c.rpcLock.Unlock()

	c.logger.Debug("configuring comm", zap.Any("num_nodes", len(nodes)))

	inUsedSet := make(map[string]*types.RemoteNode)
	for id, stub := range c.Members.id2stub {
		inUsedSet[id] = stub.RemoteNode
	}

	for _, node := range nodes {
		c.updateStubInMapping(c.Members, node)
	}

	for id := range nodes {
		delete(inUsedSet, id)
	}

	for id, node := range inUsedSet {
		c.Members.ByID(id).Deactivate()
		c.Members.Remove(id)
		c.Connections.Disconnect(node.Identity)
	}
}

func (c *Comm) updateStubInMapping(mapping *MemberMapping, node *types.RemoteNode) {
	stub := mapping.ByID(node.ID)
	if stub == nil {
		stub = &Stub{}
	}

	// Overwrite the stub Node data with the new data
	stub.RemoteNode = node

	// Put the stub into the mapping
	mapping.Put(node.ID, stub)

	// Check if the stub needs activation.
	if stub.Active() {
		return
	}

	// Activate the stub
	stub.Activate(c.createRemoteContext(stub))
}

// getOrCreateStream obtains a Submit stream for the given destination node
func (c *Comm) getOrCreateStream(destination string) (*Stream, error) {
	stream := c.getStream(destination)
	if stream != nil {
		return stream, nil
	}
	stub, err := c.Remote(destination)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	stream, err = stub.NewStream(c.Timeout)
	if err != nil {
		return nil, err
	}
	c.mapStream(destination, stream)
	return stream, nil
}

func (c *Comm) getStream(destination string) *Stream {
	c.rpcLock.RLock()
	defer c.rpcLock.RUnlock()
	return c.StreamsByType[destination]
}

func (c *Comm) mapStream(destination string, stream *Stream) {
	c.rpcLock.Lock()
	defer c.rpcLock.Unlock()
	c.StreamsByType[destination] = stream
	c.cleanCanceledStreams()
}

func (c *Comm) cleanCanceledStreams() {
	for destination, stream := range c.StreamsByType {
		if !stream.Canceled() {
			continue
		}
		delete(c.StreamsByType, destination)
	}
}

func (c *Comm) DispatchConsensus(ctx context.Context, request *proto.Message) error {
	reqCtx, err := c.requestContext(ctx, request)
	if err != nil {
		return err
	}
	return c.H.OnConsensus(reqCtx.sender, request)
}

func (c *Comm) requestContext(ctx context.Context, msg *proto.Message) (*requestContext, error) {
	return &requestContext{
		sender: msg.Sender,
	}, nil
}

func (c *Comm) Remote(id string) (*RemoteContext, error) {
	c.Lock.RLock()
	defer c.Lock.RUnlock()

	if c.shutdown {
		return nil, errors.New("communication has been shut down")
	}

	mapping := c.Members
	stub := mapping.ByID(id)

	if stub == nil {
		return nil, errors.Errorf("node %d doesn't exist", id)
	}

	if stub.Active() {
		return stub.RemoteContext, nil
	}

	err := stub.Activate(c.createRemoteContext(stub))
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return stub.RemoteContext, nil
}

func (c *Comm) createRemoteContext(stub *Stub) func() (*RemoteContext, error) {
	return func() (*RemoteContext, error) {
		conn, err := c.Connections.Connection(stub.Endpoint, stub.Identity)
		if err != nil {
			return nil, err
		}

		probeConnection := func(conn *grpc.ClientConn) error {
			connState := conn.GetState()
			if connState == connectivity.Connecting {
				return errors.Errorf("connection to %d(%s) is in state %s", stub.ID, stub.Endpoint, connState)
			}
			return nil
		}

		clusterClient := pb.NewHotstuffClient(conn)
		getStream := func(ctx context.Context) (ConsensusClientStream, error) {
			stream, err := clusterClient.Consensus(ctx)
			if err != nil {
				return nil, err
			}
			consensusClientStream := &CommClientStream{
				Client: stream,
			}
			return consensusClientStream, nil
		}

		rc := &RemoteContext{
			logger:                           c.logger,
			minimumExpirationWarningInterval: c.MinimumExpirationWarningInterval,
			certExpWarningThreshold:          c.CertExpWarningThreshold,
			SendBuffSize:                     c.SendBufferSize,
			shutdownSignal:                   c.shutdownSignal,
			endpoint:                         stub.Endpoint,
			ProbeConn:                        probeConnection,
			conn:                             conn,
			GetStreamFunc:                    getStream,
		}
		return rc, nil
	}
}

func (c *Comm) SetLogger(logger logging.Logger) {
	c.logger = logger
}

type CommClientStream struct {
	Client pb.Hotstuff_ConsensusClient
}

func (cs *CommClientStream) Send(msg *proto.Message) error {
	return cs.Client.Send(msg)
}

func (cs *CommClientStream) Recv() (*proto.Message, error) {
	return cs.Client.Recv()
}

func (cs *CommClientStream) Auth() error {
	return nil
}

func (cs *CommClientStream) Context() context.Context {
	return cs.Client.Context()
}
