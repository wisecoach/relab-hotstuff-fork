package adapters

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"github.com/relab/hotstuff"
	hecdsa "github.com/relab/hotstuff/crypto/ecdsa"
	"github.com/wisecoach/pro-hotstuff/logging"
	"sync"

	hpb "github.com/relab/hotstuff/internal/proto/hotstuffpb"
	"github.com/relab/hotstuff/modules"
	"github.com/wisecoach/pro-hotstuff/types"
	"google.golang.org/protobuf/proto"
	"strconv"
)

func NewCryptoSupportAdapter(builder *modules.Builder) modules.Module {
	csa := &CryptoSupportAdapter{
		bytes2PK: make(map[string]*ecdsa.PublicKey),
		rwLock:   sync.RWMutex{},
	}
	builder.Add(csa)
	return csa
}

type CryptoSupportAdapter struct {
	options *modules.Options
	crypto  modules.Crypto
	logger  logging.Logger

	bytes2PK map[string]*ecdsa.PublicKey
	rwLock   sync.RWMutex
}

func (c *CryptoSupportAdapter) InitModule(mods *modules.Core) {
	mods.Get(&c.options, &c.crypto, &c.logger)
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

func (c *CryptoSupportAdapter) Verify(data []byte, signature []byte, pk []byte) (bool, error) {
	ecdsaSig := &hpb.ECDSAMultiSignature{}

	c.rwLock.RLock()
	publicKey := c.bytes2PK[string(pk)]
	c.rwLock.RUnlock()
	if publicKey == nil {
		decode, _ := pem.Decode(pk)
		if decode == nil {
			c.logger.Errorf("failed to decode public key: %s", string(pk))
			return false, errors.New("failed to decode public key")
		}
		PK, err := x509.ParsePKIXPublicKey(decode.Bytes)
		if err != nil {
			return false, err
		}
		publicKey = PK.(*ecdsa.PublicKey)
		c.rwLock.Lock()
		c.bytes2PK[string(pk)] = publicKey
		c.rwLock.Unlock()
	}

	if err := proto.Unmarshal(signature, ecdsaSig); err != nil {
		return false, err
	}
	sig := hpb.MultiSignatureFromProto(ecdsaSig)

	n := sig.Participants().Len()
	if n == 0 {
		return false, errors.New("no participants in signature")
	}

	results := make(chan bool, n)
	hash := sha256.Sum256(data)

	for _, sig := range sig {
		go func(sig *hecdsa.Signature, hash hotstuff.Hash) {
			results <- c.verifySingle(sig, hash, publicKey)
		}(sig, hash)
	}

	valid := true
	for range sig {
		if !<-results {
			valid = false
		}
	}

	return valid, nil
}

func (c *CryptoSupportAdapter) verifySingle(sig *hecdsa.Signature, hash hotstuff.Hash, pk *ecdsa.PublicKey) bool {
	return ecdsa.Verify(pk, hash[:], sig.R(), sig.S())
}
