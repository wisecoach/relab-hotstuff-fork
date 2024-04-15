package adapters

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"github.com/relab/hotstuff"
	hecdsa "github.com/relab/hotstuff/crypto/ecdsa"

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
	signPb := hpb.MultiSignatureToProto(sign.(hecdsa.MultiSignature))
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
	ecdsaSig := &hpb.ECDSAMultiSignature{}

	decode, _ := pem.Decode(pk)
	publicKey, err := x509.ParsePKIXPublicKey(decode.Bytes)
	if err != nil {
		return false
	}

	if err := proto.Unmarshal(signature, ecdsaSig); err != nil {
		return false
	}
	sig := hpb.MultiSignatureFromProto(ecdsaSig)

	n := sig.Participants().Len()
	if n == 0 {
		return false
	}

	results := make(chan bool, n)
	hash := sha256.Sum256(data)

	for _, sig := range sig {
		go func(sig *hecdsa.Signature, hash hotstuff.Hash) {
			results <- c.verifySingle(sig, hash, publicKey.(*ecdsa.PublicKey))
		}(sig, hash)
	}

	valid := true
	for range sig {
		if !<-results {
			valid = false
		}
	}

	return valid
}

func (c *CryptoSupportAdapter) verifySingle(sig *hecdsa.Signature, hash hotstuff.Hash, pk *ecdsa.PublicKey) bool {
	return ecdsa.Verify(pk, hash[:], sig.R(), sig.S())
}
