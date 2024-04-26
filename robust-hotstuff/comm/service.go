package comm

import (
	"context"
	pb "github.com/relab/hotstuff/internal/proto/robusthotstuffpb"
	"github.com/wisecoach/robust-hotstuff/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

type Dispatcher interface {
	DispatchConsensus(ctx context.Context, msg *proto.Message) error
}

type Service struct {
	pb.UnimplementedHotstuffServer
	Dispatcher Dispatcher
}

func (s *Service) Consensus(ctx context.Context, msg *proto.Message) (*emptypb.Empty, error) {
	err := s.Dispatcher.DispatchConsensus(ctx, msg)
	if err != nil {
		return &emptypb.Empty{}, err
	}
	return &emptypb.Empty{}, nil
}
