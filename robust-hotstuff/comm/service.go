package comm

import (
	"context"
	"fmt"
	pb "github.com/relab/hotstuff/internal/proto/robusthotstuffpb"
	"github.com/wisecoach/robust-hotstuff/proto"
	"io"
)

type Dispatcher interface {
	DispatchConsensus(ctx context.Context, msg *proto.Message) error
}

type Service struct {
	pb.UnimplementedHotstuffServer
	Dispatcher Dispatcher
}

type ConsensusStream interface {
	pb.Hotstuff_ConsensusServer
}

func (s *Service) Consensus(stream pb.Hotstuff_ConsensusServer) error {
	for {
		err := s.handleMessage(stream)
		if err == io.EOF {
			fmt.Println(err.Error())
			return nil
		}
		if err != nil {
			fmt.Println(err.Error())
			return err
		}
	}
}

func (s *Service) handleMessage(stream ConsensusStream) error {
	msg, err := stream.Recv()
	if err == io.EOF {
		return err
	}

	return s.Dispatcher.DispatchConsensus(stream.Context(), msg)
}
