package reputation

import (
	"github.com/wisecoach/pro-hotstuff/api"
	"github.com/wisecoach/pro-hotstuff/proto"
	"github.com/wisecoach/pro-hotstuff/types"
	"math"
	"sync"
)

func NewReputationManager() api.ReputationManager {
	return &NoOpReputationManager{
		epoch:         0,
		view:          0,
		onChainConfig: nil,
		reputationMap: make(map[types.ID]*types.Reputation),
		idQueue:       make([]string, 0),
		validators:    make([]types.ID, 0),
		rwLock:        sync.RWMutex{},
	}
}

type NoOpReputationManager struct {
	epoch         uint64
	view          uint64
	onChainConfig *types.OnChainReputationConfig

	reputationMap map[types.ID]*types.Reputation
	idQueue       []string
	validators    []types.ID

	rwLock sync.RWMutex
}

func (rm *NoOpReputationManager) SaveThreatProof(threatProof *proto.ThreatProof) {
}

func (rm *NoOpReputationManager) LoadThreatProofs() []*proto.ThreatProof {
	return make([]*proto.ThreatProof, 0)
}

func (rm *NoOpReputationManager) GetLeader(view uint64) (types.ID, error) {
	rm.rwLock.RLock()
	defer rm.rwLock.RUnlock()

	validators := rm.validators
	index := int(view) % len(validators)
	return validators[index], nil
}

func (rm *NoOpReputationManager) SaveCompensation(id string, compensation *proto.Compensation) {
}

func (rm *NoOpReputationManager) LoadCompensations(view uint64) map[string]*proto.Compensation {
	return make(map[string]*proto.Compensation)
}

func (rm *NoOpReputationManager) ConfirmCompensations(view uint64) {
}

func (rm *NoOpReputationManager) CancelCompensations(view uint64) {
}

func (rm *NoOpReputationManager) TryNextView(nextView *proto.NextView) {
}

func (rm *NoOpReputationManager) LoadNextViews(view uint64) map[uint64]*proto.NextView {
	return make(map[uint64]*proto.NextView)
}

func (rm *NoOpReputationManager) CancelNextView(view uint64) {
}

func (rm *NoOpReputationManager) ConfirmNextView(view uint64) {
}

func (rm *NoOpReputationManager) NextViewsToConfirm() map[uint64]*proto.NextView {
	return make(map[uint64]*proto.NextView)
}

func (rm *NoOpReputationManager) SelectValidators(num int, view uint64) []types.ID {
	rm.rwLock.RLock()
	defer rm.rwLock.RUnlock()

	if num > len(rm.validators) {
		return rm.validators
	}

	return rm.validators[:num]
}

func (rm *NoOpReputationManager) SelectValidatorsToCompensate(view uint64) []types.ID {
	rm.rwLock.RLock()
	defer rm.rwLock.RUnlock()

	return rm.validators
}

func (rm *NoOpReputationManager) View() uint64 {
	rm.rwLock.RLock()
	defer rm.rwLock.RUnlock()

	return rm.view
}

func (rm *NoOpReputationManager) ReputationMap() map[types.ID]*types.Reputation {
	rm.rwLock.RLock()
	defer rm.rwLock.RUnlock()

	return rm.reputationMap
}

func (rm *NoOpReputationManager) Reputation(id types.ID) *types.Reputation {
	rm.rwLock.RLock()
	defer rm.rwLock.RUnlock()

	return rm.reputationMap[id]
}

func (rm *NoOpReputationManager) Validators(view uint64) []types.ID {
	rm.rwLock.RLock()
	defer rm.rwLock.RUnlock()

	return rm.validators
}

func (rm *NoOpReputationManager) IsValidator(view uint64, id types.ID) bool {
	return true
}

func (rm *NoOpReputationManager) Quorum(view uint64) int {
	return int(math.Ceil(float64(2*len(rm.validators)+1) / 3))
}

func (rm *NoOpReputationManager) RefreshReputation(cert *proto.QuorumCert) {
	rm.rwLock.Lock()
	defer rm.rwLock.Unlock()

	var view uint64
	for _, nextView := range cert.NextViews {
		view = nextView.View
		break
	}

	rm.view = view
}

func (rm *NoOpReputationManager) Configure(epoch uint64, view uint64, config *types.OnChainConfig) error {
	rm.rwLock.Lock()
	defer rm.rwLock.Unlock()

	rm.epoch = epoch
	rm.view = view
	rm.onChainConfig = config.ReputationConfig

	rm.reputationMap = make(map[types.ID]*types.Reputation)
	rm.idQueue = config.IdQueue
	rm.validators = make([]types.ID, 0)

	for _, id := range rm.idQueue {
		rm.reputationMap[types.ID(id)] = &types.Reputation{
			LR: rm.onChainConfig.LRDefault,
			SR: rm.onChainConfig.SRDefault,
		}
		rm.validators = append(rm.validators, types.ID(id))
	}

	return nil
}