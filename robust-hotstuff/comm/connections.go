package comm

import (
	"crypto/x509"
	"google.golang.org/grpc"
	"sync"
)

type RemoteVerifier func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error

// SecureDialer connects to a remote address
type SecureDialer interface {
	Dial(address string) (*grpc.ClientConn, error)
}

// ConnectionMapper maps certificates to connections
type ConnectionMapper interface {
	Lookup(key []byte) (*grpc.ClientConn, bool)
	Put(key []byte, conn *grpc.ClientConn)
	Remove(key []byte)
	Size() int
}

type ConnByCertMap map[string]*grpc.ClientConn

// Lookup looks up a certificate and returns the connection that was mapped
// to the certificate, and whether it was found or not
func (cbc ConnByCertMap) Lookup(cert []byte) (*grpc.ClientConn, bool) {
	conn, ok := cbc[string(cert)]
	return conn, ok
}

// Put associates the given connection to the certificate
func (cbc ConnByCertMap) Put(cert []byte, conn *grpc.ClientConn) {
	cbc[string(cert)] = conn
}

// Remove removes the connection that is associated to the given certificate
func (cbc ConnByCertMap) Remove(cert []byte) {
	delete(cbc, string(cert))
}

// Size returns the size of the connections by certificate mapping
func (cbc ConnByCertMap) Size() int {
	return len(cbc)
}

func NewConnectionStore(dialer SecureDialer) *ConnectionStore {
	connMapping := &ConnectionStore{
		Connections: make(ConnByCertMap),
		dialer:      dialer,
	}
	return connMapping
}

// ConnectionStore stores connections to remote nodes
type ConnectionStore struct {
	lock        sync.RWMutex
	Connections ConnectionMapper
	dialer      SecureDialer
}

func (c *ConnectionStore) Connection(endpoint string, expectedServerCert []byte) (*grpc.ClientConn, error) {
	c.lock.RLock()
	conn, alreadyConnected := c.Connections.Lookup(expectedServerCert)
	c.lock.RUnlock()

	if alreadyConnected {
		return conn, nil
	}

	// Else, we need to connect to the remote endpoint
	return c.connect(endpoint, expectedServerCert)
}

func (c *ConnectionStore) connect(endpoint string, expectedServerCert []byte) (*grpc.ClientConn, error) {
	c.lock.Lock()
	defer c.lock.Unlock()
	// Check again to see if some other goroutine has already connected while
	// we were waiting on the lock
	conn, alreadyConnected := c.Connections.Lookup(expectedServerCert)
	if alreadyConnected {
		return conn, nil
	}

	conn, err := c.dialer.Dial(endpoint)
	if err != nil {
		return nil, err
	}

	c.Connections.Put(expectedServerCert, conn)
	return conn, nil
}

// Disconnect closes the gRPC connection that is mapped to the given certificate
func (c *ConnectionStore) Disconnect(expectedServerCert []byte) {
	c.lock.Lock()
	defer c.lock.Unlock()

	conn, connected := c.Connections.Lookup(expectedServerCert)
	if !connected {
		return
	}
	conn.Close()
	c.Connections.Remove(expectedServerCert)
}
