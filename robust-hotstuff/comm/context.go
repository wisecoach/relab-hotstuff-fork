package comm

import (
	"context"
	"github.com/pkg/errors"
	"github.com/relab/hotstuff/logging"
	"github.com/wisecoach/robust-hotstuff/proto"
	"google.golang.org/grpc"
	"sync"
	"sync/atomic"
	"time"
)

type RemoteContext struct {
	logger                           logging.Logger
	expiresAt                        time.Time
	minimumExpirationWarningInterval time.Duration
	certExpWarningThreshold          time.Duration
	SendBuffSize                     int
	shutdownSignal                   chan struct{}
	endpoint                         string
	GetStreamFunc                    func(context.Context) (ConsensusClientStream, error) // interface{}
	ProbeConn                        func(conn *grpc.ClientConn) error
	conn                             *grpc.ClientConn
	nextStreamID                     uint64
}

func (rc *RemoteContext) NewStream(timeout time.Duration) (*Stream, error) {
	// if err := rc.ProbeConn(rc.conn); err != nil {
	// 	return nil, err
	// }

	ctx, cancel := context.WithCancel(context.TODO())
	stream, err := rc.GetStreamFunc(ctx)
	if err != nil {
		cancel()
		return nil, errors.WithStack(err)
	}

	streamID := atomic.AddUint64(&rc.nextStreamID, 1)

	var canceled uint32

	abortChan := make(chan struct{})
	abortReason := &atomic.Value{}

	once := &sync.Once{}

	cancelWithReason := func(err error) {
		once.Do(func() {
			abortReason.Store(err.Error())
			cancel()
			atomic.StoreUint32(&canceled, 1)
			close(abortChan)
		})
	}

	var s = &Stream{
		logger:      rc.logger,
		abortReason: abortReason,
		abortChan:   abortChan,
		sendBuff: make(chan struct {
			msg *proto.Message
		}, rc.SendBuffSize),
		commShutdown:    rc.shutdownSignal,
		ID:              streamID,
		Endpoint:        rc.endpoint,
		Timeout:         timeout,
		ConsensusClient: stream,
		Cancel:          cancelWithReason,
		canceled:        &canceled,
	}
	err = stream.Auth()
	if err != nil {
		return nil, errors.Wrap(err, "failed to create new stream")
	}

	go func() {
		s.serviceStream()
	}()

	return s, nil
}

func (rc *RemoteContext) Abort() {
}
