package comm

import (
	"context"
	"github.com/wisecoach/robust-hotstuff/proto"
)

type Communicator interface {
	Remote(id string) (*RemoteContext, error)
	Shutdown()
}

type Handler interface {
	OnConsensus(sender string, req *proto.Message) error
}

type ConsensusClientStream interface {
	Send(msg *proto.Message) error
	Recv() (*proto.Message, error)
	Auth() error
	Context() context.Context
}
