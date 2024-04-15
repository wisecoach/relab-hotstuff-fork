package comm

import (
	"errors"
	"fmt"
	"github.com/wisecoach/robust-hotstuff/logging"
	"github.com/wisecoach/robust-hotstuff/proto"
	pb "google.golang.org/protobuf/proto"
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
	return stream.ConsensusClient.Recv()
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
	maxRetries := 5
	var err error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		err = stream.ConsensusClient.Send(msg)
		if err != nil {
			bytes, _ := pb.Marshal(msg)
			stream.logger.Errorf("failed to send message %s, err = %s, size=%d, attempt=%d", msg.Type, err.Error(), len(bytes), attempt)
		}
		<-time.After(time.Millisecond * 100)
		if err == nil {
			// 发送成功，退出循环
			break
		}
	}
	// stream.logger.Debug(fmt.Sprintf("send message to %s, type=%s", stream.NodeName, msg.Type.String()))
	if err != nil {
		bytes, _ := pb.Marshal(msg)
		stream.logger.Errorf("failed to send message %s, err = %s, size=%d", msg.Type, err.Error(), len(bytes))
		// if msg.Type == proto.MessageType_COMPENSATE {
		// 	compensate := msg.GetCompensate().Compensation
		// 	stream.logger.Errorf("compensate message, num_next_view=%d", len(compensate.NextViews))
		// }
		// jsonBytes, _ := json.Marshal(msg)
		// stream.logger.Errorf("failed to send message, msg = %s", string(jsonBytes))
		return
	}
}
