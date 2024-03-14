package adapters

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/modules"
	"github.com/wisecoach/robust-hotstuff/proto"
	"github.com/wisecoach/robust-hotstuff/types"
	"strconv"
)

func NewChainSupportAdapter(builder *modules.Builder) modules.Module {
	c := &ChainSupportAdapter{}
	builder.Add(c)
	return c
}

type ChainSupportAdapter struct {
	Blockchain modules.BlockChain
	Executor   modules.Executor
	logger     logging.Logger
}

func (c *ChainSupportAdapter) InitModule(mods *modules.Core) {
	mods.Get(&c.Blockchain, &c.Executor, &c.logger)
}

func (c *ChainSupportAdapter) Height() uint64 {
	return c.Blockchain.Height()
}

func (c *ChainSupportAdapter) Block(height uint64) types.Block {
	bk := c.Blockchain.Block(height)
	block := types.Block(bk)
	return block
}

func (c *ChainSupportAdapter) CommitBlock(block types.Block) error {
	bk, ok := block.(*hotstuff.Block)
	if !ok {
		return nil
	}
	c.Blockchain.Store(bk)
	c.Executor.Exec(bk.Command())
	return nil
}

func (c *ChainSupportAdapter) GenerateBlock(info *proto.BlockInfo, certs *proto.BlockCerts) (types.Block, error) {
	if info.View%100 == 0 {
		c.logger.Info("GenerateBlock: ", info.View)
	}
	// <-time.After(100 * time.Millisecond)
	id, err := strconv.Atoi(info.Proposer)
	if err != nil {
		return nil, err
	}
	bk := hotstuff.NewBlock(hotstuff.Hash(info.ParentHash), hotstuff.QuorumCert{}, hotstuff.Command("ssss"), hotstuff.View(info.View), hotstuff.ID(id))
	bk.Info = info
	bk.Certs = certs
	blockHash := [32]byte(bk.Hash())
	bk.Info.BlockHash = blockHash[:]
	block := types.Block(bk)
	return block, nil
}
