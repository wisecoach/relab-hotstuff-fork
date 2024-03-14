package adapters

import (
	"crypto/sha256"
	"github.com/relab/hotstuff/crypto/ecdsa"
	hpb "github.com/relab/hotstuff/internal/proto/hotstuffpb"
	"github.com/relab/hotstuff/modules"
	"github.com/wisecoach/robust-hotstuff/types"
	"google.golang.org/protobuf/proto"
	"strconv"
)

func NewCryptoSupportAdapter(builder *modules.Builder) modules.Module {
	csa := &CryptoSupportAdapter{}
	builder.Add(csa)
	return csa
}

type CryptoSupportAdapter struct {
	options *modules.Options
	crypto  modules.Crypto
}

func (c *CryptoSupportAdapter) InitModule(mods *modules.Core) {
	mods.Get(&c.options, &c.crypto)
}

func (c *CryptoSupportAdapter) ID() types.ID {
	id := uint32(c.options.ID())
	itoa := strconv.Itoa(int(id))
	return types.ID(itoa)
}

func (c *CryptoSupportAdapter) Sign(data []byte) ([]byte, error) {
	sign, err := c.crypto.Sign(data)
	if err != nil {
		return nil, err
	}
	signPb := hpb.MultiSignatureToProto(sign.(ecdsa.MultiSignature))
	bytes, err := proto.Marshal(signPb)
	if err != nil {
		return nil, err
	}
	return bytes, nil
}

func (c *CryptoSupportAdapter) Hash(data []byte) []byte {
	bytes := sha256.Sum256(data)
	return bytes[:]
}

func (c *CryptoSupportAdapter) Verify(data []byte, signature []byte, pk []byte) bool {
	var ecdsaSig *hpb.ECDSAMultiSignature
	err := proto.Unmarshal(signature, ecdsaSig)
	if err != nil {
		return false
	}
	sig := hpb.MultiSignatureFromProto(ecdsaSig)
	return c.crypto.Verify(sig, data)
}
