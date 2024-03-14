package adapters

import (
	"encoding/json"
	"errors"
	"github.com/relab/hotstuff"
	hpb "github.com/relab/hotstuff/internal/proto/hotstuffpb"
	"github.com/relab/hotstuff/modules"
	rhpb "github.com/wisecoach/robust-hotstuff/proto"
	"github.com/wisecoach/robust-hotstuff/types"
	"google.golang.org/protobuf/proto"
)

func NewBlockSupportAdapter(builder *modules.Builder) modules.Module {
	bsa := &blockSupportAdapter{}
	builder.Add(bsa)
	return bsa
}

type blockSupportAdapter struct {
}

func (b *blockSupportAdapter) InitModule(mods *modules.Core) {

}

func (b *blockSupportAdapter) SerializeBlock(block types.Block) ([]byte, error) {
	bk, ok := block.(*hotstuff.Block)
	if !ok {
		return nil, errors.New("block is not a hotstuff block")
	}
	pbBlock := hpb.BlockToProto(bk)
	marshal, err := proto.Marshal(pbBlock)
	if err != nil {
		return nil, err
	}
	return marshal, nil
}

func (b *blockSupportAdapter) DeserializeBlock(data []byte) (types.Block, error) {
	pbBlock := &hpb.Block{}
	if err := proto.Unmarshal(data, pbBlock); err != nil {
		return nil, err
	}
	bk := hpb.BlockFromProto(pbBlock)
	block := types.Block(bk)
	return block, nil
}

func (b *blockSupportAdapter) ResolveInfo(block types.Block) *rhpb.BlockInfo {
	bk, ok := block.(*hotstuff.Block)
	if !ok {
		return nil
	}
	return bk.Info
}

func (b *blockSupportAdapter) ResolveCerts(block types.Block) *rhpb.BlockCerts {
	bk, ok := block.(*hotstuff.Block)
	if !ok {
		return nil
	}
	return bk.Certs
}

func (b *blockSupportAdapter) Verify(block types.Block) error {
	return nil
}

func (b *blockSupportAdapter) IsConfigBlock(block types.Block) bool {
	bk, ok := block.(*hotstuff.Block)
	if !ok {
		return false
	}
	cmd := string(bk.Command())
	var config types.NewEpoch
	err := json.Unmarshal([]byte(cmd), &config)
	if err != nil {
		return false
	}
	return true
}

func (b *blockSupportAdapter) ResolveNewEpoch(block types.Block) (*types.NewEpoch, error) {
	bk, ok := block.(*hotstuff.Block)
	if !ok {
		return nil, errors.New("block is not a hotstuff block")
	}
	cmd := string(bk.Command())
	var config types.NewEpoch
	err := json.Unmarshal([]byte(cmd), &config)
	if err != nil {
		return nil, err
	}
	return &config, nil
}
