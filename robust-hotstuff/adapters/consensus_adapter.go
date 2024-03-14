package adapters

import (
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/modules"
	"github.com/wisecoach/robust-hotstuff/api"
	"github.com/wisecoach/robust-hotstuff/consensus"
	"github.com/wisecoach/robust-hotstuff/storage"
	"github.com/wisecoach/robust-hotstuff/types"
	"time"
)

func NewConsensusAdapter(builder *modules.Builder) modules.Module {
	ca := &ConsensusAdapter{}
	builder.Add(ca)
	return ca
}

type ConsensusAdapter struct {
	comm          api.Comm
	blockSupport  api.BlockSupport
	cryptoSupport api.CryptoSupport
	chainSupport  api.ChainSupport
	logger        logging.Logger

	api.Consensus
}

func (c *ConsensusAdapter) InitModule(mods *modules.Core) {
	mods.Get(&c.comm, &c.blockSupport, &c.cryptoSupport, &c.chainSupport, &c.logger)
	config := &consensus.Config{
		ViewControllerConfig: &consensus.ViewControllerConfig{
			HelpSync: false,
		},
		SyncConfig: &types.SyncConfig{
			Timeout: time.Second * 3,
		},
		SafetyConfig: &types.SafetyConfig{
			NeedVerify: false,
		},
	}
	internalStorage := storage.NewStorage()
	c.Consensus = consensus.New(config, c.logger, internalStorage, c.comm, c.blockSupport, c.cryptoSupport, c.chainSupport)
}
