package comm

import (
	"context"
	"encoding/json"
	"github.com/pkg/errors"
	pb "github.com/relab/hotstuff/internal/proto/robusthotstuffpb"
	"github.com/wisecoach/pro-hotstuff/logging"
	"github.com/wisecoach/pro-hotstuff/proto"
	"github.com/wisecoach/pro-hotstuff/types"
	"go.uber.org/zap"
	"sync"
	"time"
)

type Handler interface {
	OnConsensus(sender string, req *proto.Message) error
}

type MessageToSend struct {
	Destination string
	Message     *proto.Message
}

func New(dialer SecureDialer, compareCert CertificateComparator, H Handler) *Comm {
	comm := &Comm{
		consensusLock:                    sync.Mutex{},
		id2Node:                          make(map[string]*types.RemoteNode),
		id2Client:                        make(map[string]pb.HotstuffClient),
		rpcLock:                          sync.RWMutex{},
		Timeout:                          time.Hour,
		logger:                           nil,
		MinimumExpirationWarningInterval: time.Hour,
		CertExpWarningThreshold:          time.Hour,
		shutdownSignal:                   make(chan struct{}),
		shutdown:                         false,
		SendBufferSize:                   1024 * 16,
		msgQueue:                         make(chan *MessageToSend, 1024*16),
		Lock:                             sync.RWMutex{},
		H:                                H,
		Connections:                      NewConnectionStore(dialer),
		CompareCertificate:               compareCert,
	}
	go comm.run()
	return comm
}

type Comm struct {
	consensusLock sync.Mutex
	id2Node       map[string]*types.RemoteNode
	id2Client     map[string]pb.HotstuffClient
	rpcLock       sync.RWMutex
	Timeout       time.Duration
	logger        logging.Logger

	msgQueue                         chan *MessageToSend
	MinimumExpirationWarningInterval time.Duration
	CertExpWarningThreshold          time.Duration
	shutdownSignal                   chan struct{}
	shutdown                         bool
	SendBufferSize                   int
	Lock                             sync.RWMutex
	H                                Handler
	Connections                      *ConnectionStore
	CompareCertificate               CertificateComparator
}

func (c *Comm) Shutdown() {
	// TODO implement me
	panic("implement me")
}

type requestContext struct {
	sender string
}

func (c *Comm) run() {
	for {
		select {
		case <-c.shutdownSignal:
			return
		case msg := <-c.msgQueue:
			go c.send(msg)
		}
	}
}

func (c *Comm) send(msg *MessageToSend) {
	maxRetries := 5
	for i := 0; i < maxRetries; i++ {
		client, err := c.getAndCreateClient(msg.Destination, i > 0)
		if err != nil {
			c.logger.Errorf("failed to get client, %s", err.Error())
			continue
		}
		timeout, cancel := context.WithTimeout(context.Background(), c.Timeout)
		_, err = client.Consensus(timeout, msg.Message)
		if err != nil {
			cancel()
			bytes, _ := json.Marshal(msg.Message)
			c.logger.Errorf("failed to get client, %s, msg_type=%s, msg_size=%d, retry=%d",
				err.Error(), msg.Message.Type, len(bytes), i+1)
			continue
		}
		cancel()
		if i > 1 {
			c.logger.Infof("successfully sent message after %d retries", i)
		}
		break
	}

}

func (c *Comm) Send(id types.ID, msg *proto.Message) error {
	dest := string(id)
	c.msgQueue <- &MessageToSend{
		Destination: dest,
		Message:     msg,
	}
	return nil
}

func (c *Comm) Broadcast(msg *proto.Message) error {
	for id := range c.id2Node {
		c.msgQueue <- &MessageToSend{
			Destination: id,
			Message:     msg,
		}
	}
	return nil
}

func (c *Comm) Multicast(targets []types.ID, msg *proto.Message) error {
	for _, id := range targets {
		c.msgQueue <- &MessageToSend{
			Destination: string(id),
			Message:     msg,
		}
	}

	return nil

}

func (c *Comm) Configure(nodes map[string]*types.RemoteNode) {

	c.rpcLock.Lock()
	defer c.rpcLock.Unlock()

	c.logger.Debugf("configuring comm, num_nodes = %d", len(nodes))

	inUsedSet := c.id2Node
	c.id2Node = nodes

	for _, node := range nodes {
		_, err := c.Connections.Connection(node.ID, node.Endpoint)
		if err != nil {
			c.logger.Error("failed to connect to node", zap.Error(err))
			continue
		}
	}

	for id := range nodes {
		delete(inUsedSet, id)
	}

	for _, node := range inUsedSet {
		c.Connections.Disconnect(node.ID)
	}
}

func (c *Comm) getAndCreateClient(destination string, needCreate bool) (pb.HotstuffClient, error) {
	if !needCreate {
		c.rpcLock.RLock()
		client, existed := c.id2Client[destination]
		c.rpcLock.RUnlock()

		if existed {
			return client, nil
		}
	}
	node, existed := c.id2Node[destination]
	if !existed {
		return nil, errors.Errorf("node %s doesn't exist in config", destination)
	}
	conn, err := c.Connections.Connection(destination, node.Endpoint)
	if err != nil {
		return nil, err
	}
	c.rpcLock.Lock()
	c.id2Client[destination] = pb.NewHotstuffClient(conn)
	client := c.id2Client[destination]
	c.rpcLock.Unlock()
	return client, nil
}

func (c *Comm) DispatchConsensus(ctx context.Context, request *proto.Message) error {
	if request == nil {
		return errors.New("nil request")
	}
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

func (c *Comm) SetLogger(logger logging.Logger) {
	c.logger = logger
}
