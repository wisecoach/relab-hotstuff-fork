package main

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/backend"
	"github.com/relab/hotstuff/crypto/keygen"
	"github.com/relab/hotstuff/internal/orchestration"
	"github.com/relab/hotstuff/internal/proto/orchestrationpb"
	"github.com/relab/hotstuff/internal/protostream"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/metrics"
	"github.com/relab/hotstuff/replica"
	"google.golang.org/protobuf/types/known/durationpb"
	"net"
	"strconv"
	"time"
)

func main() {

	logging.SetLogLevel("debug")

	startReplicas(3)

	<-time.After(5 * time.Hour)
}

func startReplicas(num int) {

	_, workerPipe := net.Pipe()
	worker := orchestration.NewWorker(
		protostream.NewWriter(workerPipe),
		protostream.NewReader(workerPipe),
		metrics.NopLogger(),
		nil,
		time.Second,
	)

	ops := make([]*orchestrationpb.ReplicaOpts, 0)
	conf := make([]*orchestrationpb.ReplicaInfo, 0)
	replicas := make([]*replica.Replica, 0)

	for i := 0; i < num; i++ {
		id := i
		caKey, ca, _ := keygen.GenerateCA()
		validFor := []string{"localhost", "127.0.0.1", "127.0.1.1"}
		crypto := "ecdsa"
		keyChain, _ := keygen.GenerateKeyChain(hotstuff.ID(id), validFor, crypto, ca, caKey)

		opts := &orchestrationpb.ReplicaOpts{
			ID:                   uint32(i),
			PrivateKey:           keyChain.PrivateKey,
			PublicKey:            keyChain.PublicKey,
			UseTLS:               false,
			Certificate:          keyChain.Certificate,
			CertificateKey:       keyChain.CertificateKey,
			CertificateAuthority: keygen.CertToPEM(ca),

			Crypto:            crypto,
			Consensus:         "chainedhotstuff",
			LeaderRotation:    "reputation",
			BatchSize:         1,
			ConnectTimeout:    durationpb.New(5 * time.Second),
			InitialTimeout:    durationpb.New(100 * time.Millisecond),
			MaxTimeout:        durationpb.New(0),
			TimeoutSamples:    1000,
			TimeoutMultiplier: 1.2,
			ByzantineStrategy: "",
			SharedSeed:        0,
			Modules:           nil,
			LocationInfo:      nil,
		}
		ops = append(ops, opts)
		conf = append(conf, &orchestrationpb.ReplicaInfo{
			ID:          uint32(i),
			Address:     "127.0.0.1",
			PublicKey:   keyChain.PublicKey,
			ReplicaPort: uint32(7000 + i),
			ClientPort:  uint32(8000 + i),
		})
	}

	hotstuff.InitGenesis(conf)

	for i, op := range ops {
		r, _ := worker.CreateReplica(op)
		replicaListen, _ := net.Listen("tcp", ":"+strconv.Itoa(7000+i))
		clientListen, _ := net.Listen("tcp", ":"+strconv.Itoa(8000+i))
		r.StartServers(replicaListen, clientListen)
		replicas = append(replicas, r)
	}

	replicaInfos := make([]backend.ReplicaInfo, 0, len(conf))
	for _, r := range conf {
		pubKey, _ := keygen.ParsePublicKey(r.GetPublicKey())
		addr := net.JoinHostPort(r.GetAddress(), strconv.Itoa(int(r.GetReplicaPort())))
		replicaInfos = append(replicaInfos, backend.ReplicaInfo{
			ID:      hotstuff.ID(r.GetID()),
			Address: addr,
			PubKey:  pubKey,
		})
	}

	for _, r := range replicas {
		r.Start()
	}

}
