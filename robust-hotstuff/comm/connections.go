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
	Lookup(id string) (*grpc.ClientConn, bool)
	Put(id string, conn *grpc.ClientConn)
	Remove(id string)
	Size() int
}

type ConnByCertMap map[string]*grpc.ClientConn

// Lookup looks up a certificate and returns the connection that was mapped
// to the certificate, and whether it was found or not
func (cbc ConnByCertMap) Lookup(id string) (*grpc.ClientConn, bool) {
	conn, ok := cbc[id]
	return conn, ok
}

// Put associates the given connection to the certificate
func (cbc ConnByCertMap) Put(id string, conn *grpc.ClientConn) {
	cbc[id] = conn
}

// Remove removes the connection that is associated to the given certificate
func (cbc ConnByCertMap) Remove(id string) {
	delete(cbc, id)
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

func (c *ConnectionStore) Connection(id string, endpoint string) (*grpc.ClientConn, error) {
	c.lock.RLock()
	conn, alreadyConnected := c.Connections.Lookup(id)
	c.lock.RUnlock()

	if alreadyConnected {
		return conn, nil
	}

	// Else, we need to connect to the remote endpoint
	return c.connect(id, endpoint)
}

func (c *ConnectionStore) connect(id string, endpoint string) (*grpc.ClientConn, error) {
	c.lock.Lock()
	defer c.lock.Unlock()
	// Check again to see if some other goroutine has already connected while
	// we were waiting on the lock
	conn, alreadyConnected := c.Connections.Lookup(id)
	if alreadyConnected {
		return conn, nil
	}

	conn, err := c.dialer.Dial(endpoint)
	if err != nil {
		return nil, err
	}

	c.Connections.Put(id, conn)
	return conn, nil
}

// Disconnect closes the gRPC connection that is mapped to the given certificate
func (c *ConnectionStore) Disconnect(id string) {
	c.lock.Lock()
	defer c.lock.Unlock()

	conn, connected := c.Connections.Lookup(id)
	if !connected {
		return
	}
	conn.Close()
	c.Connections.Remove(id)
}
