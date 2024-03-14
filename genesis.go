package hotstuff

import (
	"encoding/json"
	"github.com/relab/hotstuff/internal/proto/orchestrationpb"
	"github.com/wisecoach/robust-hotstuff/proto"
	"github.com/wisecoach/robust-hotstuff/types"
	"strconv"
	"time"
)

var genesisBlock = NewBlock(Hash{}, QuorumCert{}, "", 0, 0)

// GetGenesis returns a pointer to the genesis block, the starting point for the hotstuff blockchain.
func GetGenesis() *Block {
	return genesisBlock
}

func InitGenesis(replicaInfos []*orchestrationpb.ReplicaInfo) {
	replicas := make(map[string][]byte)
	idQueue := make([]string, 0)
	nodes := make(map[string]*types.RemoteNode)
	for _, info := range replicaInfos {
		id := strconv.Itoa(int(info.ID))
		replicas[id] = info.PublicKey
		idQueue = append(idQueue, id)
		nodes[id] = &types.RemoteNode{
			NodeAddress: types.NodeAddress{
				ID:       id,
				Endpoint: info.Address,
			},
			NodeCerts: types.NodeCerts{
				ServerTLSCert: nil,
				ClientTLSCert: nil,
				ServerRootCA:  nil,
				Identity:      info.PublicKey,
			},
		}
	}
	config := &types.OnChainConfig{
		ChainId:  "test",
		Replicas: replicas,
		IdQueue:  idQueue,
		ReputationConfig: &types.OnChainReputationConfig{
			LRMax:        100,
			LRDefault:    50,
			LRThreshold:  10,
			SRMax:        100,
			SRDefault:    50,
			SRThreshold:  10,
			Theta:        0.5,
			LRGrowth:     0.1,
			LRDecay:      0.1,
			LRCompensate: 0.1,
			SRGrowth:     0.1,
			SRDecay:      0.1,
		},
		SyncConfig: &types.OnChainSyncConfig{BaseTimeout: time.Second * 3},
		CommConfig: &types.OnChainCommConfig{
			Nodes: nodes,
		},
	}
	newEpoch := &types.NewEpoch{
		Epoch:      1,
		View:       1,
		Config:     config,
		Signatures: nil,
	}
	bytes, err := json.Marshal(newEpoch)
	if err != nil {
		return
	}
	cmd := Command(string(bytes))
	block := NewBlock([32]byte{}, QuorumCert{}, cmd, View(1), ID(1))
	hash := [32]byte(block.Hash())
	block.Info = &proto.BlockInfo{
		Epoch:      1,
		View:       1,
		Height:     0,
		BlockHash:  hash[:],
		ParentHash: hash[:],
		Timestamp:  nil,
		Proposer:   "1",
	}
	block.Certs = &proto.BlockCerts{
		PrepareQc: &proto.QuorumCert{
			Type:       proto.PhaseType_PREPARE,
			Epoch:      1,
			View:       1,
			BlockInfo:  block.Info,
			Signatures: nil,
		},
		PrecommitQc: &proto.QuorumCert{
			Type:       proto.PhaseType_PRECOMMIT,
			Epoch:      1,
			View:       1,
			BlockInfo:  block.Info,
			Signatures: nil,
		},
		CommitQc: &proto.QuorumCert{
			Type:       proto.PhaseType_COMMIT,
			Epoch:      1,
			View:       1,
			BlockInfo:  block.Info,
			Signatures: nil,
		},
		NextViewCert: &proto.NextViewCert{
			NextViews: make(map[string]*proto.NextView),
			Epoch:     0,
			View:      1,
		},
	}
	genesisBlock = block
}
