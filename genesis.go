package hotstuff

import (
	"encoding/json"
	"github.com/relab/hotstuff/internal/proto/orchestrationpb"
	"github.com/wisecoach/robust-hotstuff/proto"
	"github.com/wisecoach/robust-hotstuff/types"
	"sort"
	"strconv"
	"time"
)

var genesisBlock = NewBlock(Hash{}, QuorumCert{}, "", 0, 0)
var firstEpoch *types.NewEpoch
var newEpochCmd Command

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
				Endpoint: info.Address + ":" + strconv.Itoa(int(info.ReplicaPort)),
			},
			NodeCerts: types.NodeCerts{
				ServerTLSCert: nil,
				ClientTLSCert: nil,
				ServerRootCA:  nil,
				Identity:      info.PublicKey,
			},
		}
	}
	sort.Strings(idQueue)
	config := &types.OnChainConfig{
		ChainId:  "test",
		Replicas: replicas,
		IdQueue:  idQueue,
		ReputationConfig: &types.OnChainReputationConfig{
			LRInit:              0.5,
			LRDefault:           0.2,
			LRThreshold:         0.3,
			SRInit:              0.5,
			SRDefault:           0.5,
			SRThreshold:         0.3,
			LRLearnRate:         0.1,
			LRGrowth:            0.005,
			LRDecay:             0.8,
			LRCompensate:        0.01,
			SRGrowth:            0.01,
			SRDecay:             0.9,
			CompensateThreshold: 100,
			AWeight:             0.33,
			SWeight:             0.33,
			PWeight:             0.34,
			Da:                  0.8,
			Wcd:                 0.5,
			Dcd:                 0.8,
			Dlq:                 0.8,
			Ptimeout:            0.5,
		},
		SyncConfig: &types.OnChainSyncConfig{BaseTimeout: time.Millisecond * 5000},
		CommConfig: &types.OnChainCommConfig{
			Nodes: nodes,
		},
	}
	firstEpoch = &types.NewEpoch{
		Epoch:      1,
		View:       1,
		Config:     config,
		Signatures: nil,
	}
	bytes, err := json.Marshal(firstEpoch)
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

func GetNewEpochCmd() Command {
	return newEpochCmd
}

func InitNewEpoch(newReplicaInfos []*orchestrationpb.ReplicaInfo) {
	newEpoch := firstEpoch
	newEpoch.Epoch += 1
	config := newEpoch.Config
	for _, info := range newReplicaInfos {
		id := strconv.Itoa(int(info.ID))
		config.Replicas[id] = info.PublicKey
		config.IdQueue = append(config.IdQueue, id)
		config.CommConfig.Nodes[id] = &types.RemoteNode{
			NodeAddress: types.NodeAddress{
				ID:       id,
				Endpoint: info.Address + ":" + strconv.Itoa(int(info.ReplicaPort)),
			},
			NodeCerts: types.NodeCerts{
				ServerTLSCert: nil,
				ClientTLSCert: nil,
				ServerRootCA:  nil,
				Identity:      info.PublicKey,
			},
		}
	}
	sort.Strings(config.IdQueue)
	bytes, err := json.Marshal(newEpoch)
	if err != nil {
		return
	}
	newEpochCmd = Command(bytes)
}
