package adapters

import (
	"context"
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/modules"
	"github.com/wisecoach/robust-hotstuff/proto"
	"github.com/wisecoach/robust-hotstuff/types"
	"strconv"
	"time"
)

func NewChainSupportAdapter(builder *modules.Builder, newEpochRate float64, newEpochDuration time.Duration, sent bool) modules.Module {
	c := &ChainSupportAdapter{newEpochRate: newEpochRate, newEpochDuration: newEpochDuration, sentNewEpoch: sent}
	builder.Add(c)
	return c
}

type ChainSupportAdapter struct {
	Blockchain       modules.BlockChain
	Executor         modules.Executor
	cmdQueue         modules.CommandQueue
	acceptor         modules.Acceptor
	logger           logging.Logger
	newEpochRate     float64
	newEpochDuration time.Duration
	startTime        time.Time
	sentNewEpoch     bool
}

func (c *ChainSupportAdapter) InitModule(mods *modules.Core) {
	mods.Get(&c.Blockchain, &c.Executor, &c.logger, &c.cmdQueue, &c.acceptor)
	c.startTime = time.Now()
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

func (c *ChainSupportAdapter) GenerateBlock(txs []types.Transaction, info *proto.BlockInfo, certs *proto.BlockCerts) (types.Block, error) {
	c.logger.Debugf("GenerateBlock: view = %d, height = %d", info.View, info.Height)
	if info.View%100 == 0 {
		c.logger.Info("GenerateBlock: ", info.View)
	}

	cmd := txs[0].(hotstuff.Command)

	id, err := strconv.Atoi(info.Proposer)
	if err != nil {
		return nil, err
	}
	c.logger.Debugf("GenerateBlock: height = %d", info.Height)
	bk := hotstuff.NewBlock(hotstuff.Hash(info.ParentHash), hotstuff.QuorumCert{}, cmd, hotstuff.View(info.View), hotstuff.ID(id))
	bk.Info = info
	bk.Certs = certs
	blockHash := [32]byte(bk.Hash())
	bk.Info.BlockHash = blockHash[:]
	block := types.Block(bk)
	return block, nil
}

func (c *ChainSupportAdapter) OrderedTransactions() ([]types.Transaction, error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var cmd hotstuff.Command
	if c.newEpochRate > 0 && !c.sentNewEpoch && c.startTime.Add(c.newEpochDuration).Before(time.Now()) {
		c.logger.Info("generate new epoch command")
		cmd = hotstuff.GetNewEpochCmd()
		c.sentNewEpoch = true
	} else {
		cmd, _ = c.cmdQueue.Get(ctx)
	}

	// <-time.After(1 * time.Millisecond)
	// cmd := hotstuff.Command("test")
	return []types.Transaction{cmd}, nil
}
