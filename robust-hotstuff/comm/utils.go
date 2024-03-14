package comm

import (
	"context"
	"crypto/x509"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"math/rand"
	"sync"
	"time"
)

type CertificateComparator func([]byte, []byte) bool

type MemberMapping struct {
	id2stub       map[string]*Stub
	SamePublicKey CertificateComparator
}

// ByID retrieves the Stub with the given ID from the MemberMapping
func (mp *MemberMapping) ByID(ID string) *Stub {
	return mp.id2stub[ID]
}

// LookupByClientCert retrieves a Stub with the given client certificate
func (mp *MemberMapping) LookupByClientCert(cert []byte) *Stub {
	for _, stub := range mp.id2stub {
		if mp.SamePublicKey(stub.ClientTLSCert, cert) {
			return stub
		}
	}
	return nil
}

func (mp *MemberMapping) Put(ID string, stub *Stub) {
	mp.id2stub[ID] = stub
}

func (mp *MemberMapping) Remove(ID string) {
	delete(mp.id2stub, ID)
}

func ExtractRemoteAddress(ctx context.Context) string {
	var remoteAddress string
	p, ok := peer.FromContext(ctx)
	if !ok {
		return ""
	}
	if address := p.Addr; address != nil {
		remoteAddress = address.String()
	}
	return remoteAddress
}

// ExtractCertificateFromContext returns the TLS certificate (if applicable)
// from the given context of a gRPC stream
func ExtractCertificateFromContext(ctx context.Context) *x509.Certificate {
	pr, extracted := peer.FromContext(ctx)
	if !extracted {
		return nil
	}

	authInfo := pr.AuthInfo
	if authInfo == nil {
		return nil
	}

	tlsInfo, isTLSConn := authInfo.(credentials.TLSInfo)
	if !isTLSConn {
		return nil
	}
	certs := tlsInfo.State.PeerCertificates
	if len(certs) == 0 {
		return nil
	}
	return certs[0]
}

// ExtractRawCertificateFromContext returns the raw TLS certificate (if applicable)
// from the given context of a gRPC stream
func ExtractRawCertificateFromContext(ctx context.Context) []byte {
	cert := ExtractCertificateFromContext(ctx)
	if cert == nil {
		return nil
	}
	return cert.Raw
}

type arguments struct {
	a, b string
}

type ComparisonMemoizer struct {
	// Configuration
	F          func(a, b []byte) bool
	MaxEntries uint16
	// Internal state
	cache map[arguments]bool
	lock  sync.RWMutex
	once  sync.Once
	rand  *rand.Rand
}

func (cm *ComparisonMemoizer) shrinkIfNeeded() {
	for {
		currentSize := uint16(len(cm.cache))
		if currentSize < cm.MaxEntries {
			return
		}
		cm.shrink()
	}
}

func (cm *ComparisonMemoizer) shrink() {
	// Shrink the cache by 25% by removing every fourth element (on average)
	for key := range cm.cache {
		if cm.rand.Int()%4 != 0 {
			continue
		}
		delete(cm.cache, key)
	}
}

func (cm *ComparisonMemoizer) setup() {
	cm.lock.Lock()
	defer cm.lock.Unlock()
	cm.rand = rand.New(rand.NewSource(time.Now().UnixNano()))
	cm.cache = make(map[arguments]bool)
}

func (cm *ComparisonMemoizer) Compare(a, b []byte) bool {
	cm.once.Do(cm.setup)
	key := arguments{
		a: string(a),
		b: string(b),
	}

	cm.lock.RLock()
	result, exists := cm.cache[key]
	cm.lock.RUnlock()

	if exists {
		return result
	}

	result = cm.F(a, b)

	cm.lock.Lock()
	defer cm.lock.Unlock()

	cm.shrinkIfNeeded()
	cm.cache[key] = result

	return result
}

func CachePublicKeyComparisons(f CertificateComparator) CertificateComparator {
	m := &ComparisonMemoizer{
		MaxEntries: 4096,
		F:          f,
	}
	return m.Compare
}
