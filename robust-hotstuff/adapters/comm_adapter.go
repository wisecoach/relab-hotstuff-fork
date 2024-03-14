package adapters

import (
	"bytes"
	"crypto/tls"
	"github.com/relab/hotstuff/internal/proto/orchestrationpb"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/robust-hotstuff/comm"
	"github.com/wisecoach/robust-hotstuff/api"
	"github.com/wisecoach/robust-hotstuff/logging"
	"github.com/wisecoach/robust-hotstuff/proto"
	"github.com/wisecoach/robust-hotstuff/types"
	"time"
)

type MessageHandler struct {
	Consensus api.Consensus
}

func (h *MessageHandler) OnConsensus(sender string, req *proto.Message) error {
	h.Consensus.HandleMessage(types.ID(sender), req)
	return nil
}

type CommAdapter struct {
	comm.Comm
	H      *MessageHandler
	Server *comm.Service
	Ops    *orchestrationpb.ReplicaOpts
}

func NewCommAdapter(builder *modules.Builder, ops *orchestrationpb.ReplicaOpts) modules.Module {
	config := comm.ClientConfig{
		SecOpts: comm.SecureOptions{},
		KaOpts: comm.KeepaliveOptions{
			ClientInterval:    time.Duration(1) * time.Minute,  // 1 min
			ClientTimeout:     time.Duration(20) * time.Second, // 20 sec - gRPC default
			ServerInterval:    time.Duration(2) * time.Hour,    // 2 hours - gRPC default
			ServerTimeout:     time.Duration(20) * time.Second, // 20 sec - gRPC default
			ServerMinInterval: time.Duration(1) * time.Minute,  // match ClientInterval
		},
		DialTimeout:    time.Hour,
		AsyncConnect:   true,
		MaxRecvMsgSize: comm.DefaultMaxRecvMsgSize,
		MaxSendMsgSize: comm.DefaultMaxRecvMsgSize,
	}
	secOptions := comm.SecureOptions{
		VerifyCertificate: nil,
		Certificate:       ops.Certificate,
		Key:               ops.PrivateKey,
		ServerRootCAs:     [][]byte{ops.CertificateAuthority},
		ClientRootCAs:     [][]byte{ops.CertificateAuthority},
		UseTLS:            ops.UseTLS,
		RequireClientCert: false,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
		},
		TimeShift:          0,
		ServerNameOverride: "",
	}
	config.SecOpts = secOptions

	compareCert := comm.CachePublicKeyComparisons(func(a, b []byte) bool {
		return bytes.Compare(a, b) == 0
	})

	h := &MessageHandler{}

	commImpl := comm.New(config, compareCert, h)
	commAdapter := &CommAdapter{
		Comm: *commImpl,
		H:    h,
		Server: &comm.Service{
			Dispatcher: commImpl,
		},
		Ops: ops,
	}
	builder.Add(commAdapter)
	return commAdapter
}

func (ca *CommAdapter) InitModule(mods *modules.Core) {
	mods.Get(&ca.H.Consensus)
	var logger logging.Logger
	mods.Get(&logger)
	ca.Comm.SetLogger(logger)
}
