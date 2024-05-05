package adapters

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/modules"
	reputation2 "github.com/relab/hotstuff/pro-hotstuff/reputation"
	"github.com/wisecoach/pro-hotstuff/api"
	"github.com/wisecoach/pro-hotstuff/consensus"
	"github.com/wisecoach/pro-hotstuff/proto"
	"github.com/wisecoach/pro-hotstuff/reputation"
	"github.com/wisecoach/pro-hotstuff/storage"
	"github.com/wisecoach/pro-hotstuff/threat"
	"github.com/wisecoach/pro-hotstuff/types"
	"time"
)

func NewConsensusAdapter(builder *modules.Builder, byzantineStrategy string, leaderRotation string) modules.Module {
	ca := &ConsensusAdapter{byzantineStrategy: byzantineStrategy, leaderRotation: leaderRotation}
	builder.Add(ca)
	return ca
}

type ConsensusAdapter struct {
	comm              api.Comm
	blockSupport      api.BlockSupport
	cryptoSupport     api.CryptoSupport
	chainSupport      api.ChainSupport
	logger            logging.Logger
	byzantineStrategy string
	leaderRotation    string
	acceptor          modules.Acceptor

	api.Consensus
}

func (c *ConsensusAdapter) InitModule(mods *modules.Core) {
	mods.Get(&c.comm, &c.blockSupport, &c.cryptoSupport, &c.chainSupport, &c.logger, &c.acceptor)
	config := &consensus.Config{
		ViewControllerConfig: &consensus.ViewControllerConfig{
			HelpSync: false,
		},
		SyncConfig: &types.SyncConfig{
			Timeout: time.Second * 3,
		},
		SafetyConfig: &types.SafetyConfig{
			NeedVerify: true,
		},
	}
	internalStorage := storage.NewStorage()

	var manager api.ReputationManager
	if c.leaderRotation == "reputation" {
		manager = reputation.New(c.logger, c.cryptoSupport, threat.NewThreatDetector())
	} else {
		manager = reputation2.NewReputationManager()
	}

	c.Consensus = consensus.New(config, c.logger, internalStorage, c.comm, c.blockSupport, c.cryptoSupport, c.chainSupport, manager)
}

func (c *ConsensusAdapter) Start() {
	if c.byzantineStrategy == "silence" {
		return
	}
	if c.byzantineStrategy == "delay" {
		return
	}
	c.Consensus.Start()
}

func (c *ConsensusAdapter) HandleMessage(sender types.ID, msg *proto.Message) {
	if msg.Type == proto.MessageType_PROPOSAL {
		block, err := c.blockSupport.DeserializeBlock(msg.GetProposal().Proposal.Block)
		if err != nil {
			return
		}
		b := block.(*hotstuff.Block)
		c.acceptor.Proposed(b.Command())
	}

	if c.byzantineStrategy == "silence" {
		c.logger.Debugf("stimulate silence attack")
		c.handleMessageWithSilenceStrategy(sender, msg)
		return
	}
	if c.byzantineStrategy == "delay" {
		c.logger.Debugf("stimulate delay attack")
		c.handleMessageWithDelayStrategy(sender, msg)
		return
	}

	c.Consensus.HandleMessage(sender, msg)
}

func (c *ConsensusAdapter) handleMessageWithSilenceStrategy(sender types.ID, msg *proto.Message) {
	// do nothing
}

func (c *ConsensusAdapter) handleMessageWithDelayStrategy(sender types.ID, msg *proto.Message) {
	time.Sleep(time.Second * 3)
	c.Consensus.HandleMessage(sender, msg)
}
