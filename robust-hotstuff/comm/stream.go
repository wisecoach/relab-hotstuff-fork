package comm

import (
	"errors"
	"fmt"
	"github.com/wisecoach/robust-hotstuff/logging"
	"github.com/wisecoach/robust-hotstuff/proto"
	"go.uber.org/zap"
	"sync/atomic"
	"time"
)

type Stream struct {
	logger    logging.Logger
	abortChan <-chan struct{}
	sendBuff  chan struct {
		msg *proto.Message
	}
	commShutdown    chan struct{}
	abortReason     *atomic.Value
	ID              uint64
	NodeName        string
	Endpoint        string
	Timeout         time.Duration
	ConsensusClient ConsensusClientStream
	Cancel          func(error)
	canceled        *uint32
}

type StreamOperation func() (*proto.Message, error)

func (stream *Stream) Canceled() bool {
	return atomic.LoadUint32(stream.canceled) == uint32(1)
}

func (stream *Stream) Send(msg *proto.Message) error {
	if stream.Canceled() {
		return errors.New(stream.abortReason.Load().(string))
	}
	select {
	case <-stream.abortChan:
		return errors.New(fmt.Sprintf("stream %d aborted", stream.ID))
	case stream.sendBuff <- struct {
		msg *proto.Message
	}{msg: msg}:
		return nil
	case <-stream.commShutdown:
		return nil
	}
}

func (stream *Stream) Recv() (*proto.Message, error) {
	return nil, nil
}

func (stream *Stream) serviceStream() {
	defer func() {
		stream.Cancel(errors.New("aborted"))
	}()

	for {
		select {
		case msg := <-stream.sendBuff:
			stream.sendMessage(msg.msg)
		case <-stream.abortChan:
			return
		case <-stream.commShutdown:
			return
		}
	}
}

func (stream *Stream) sendMessage(msg *proto.Message) {
	err := stream.ConsensusClient.Send(msg)
	stream.logger.Debug(fmt.Sprintf("send message to %s, type=%s", stream.NodeName, msg.Type.String()))
	if err != nil {
		stream.logger.Error("failed to send message", zap.Error(err))
		return
	}
}
