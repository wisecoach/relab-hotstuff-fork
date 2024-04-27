package comm

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"net"
	"time"
)

const (
	DefaultMaxRecvMsgSize = 50000 * 1024 * 1024
	DefaultMaxSendMsgSize = 50000 * 1024 * 1024
)

type DynamicClientCredentials struct {
	TLSConfig *tls.Config
}

func (dtc *DynamicClientCredentials) latestConfig() *tls.Config {
	return dtc.TLSConfig.Clone()
}

func (dtc *DynamicClientCredentials) ClientHandshake(ctx context.Context, authority string, rawConn net.Conn) (net.Conn, credentials.AuthInfo, error) {
	creds := credentials.NewTLS(dtc.latestConfig())
	conn, auth, err := creds.ClientHandshake(ctx, authority, rawConn)
	return conn, auth, err
}

func (dtc *DynamicClientCredentials) ServerHandshake(rawConn net.Conn) (net.Conn, credentials.AuthInfo, error) {
	return nil, nil, errors.New("core/comm: server handshakes are not implemented with clientCreds")
}

func (dtc *DynamicClientCredentials) Info() credentials.ProtocolInfo {
	return credentials.NewTLS(dtc.latestConfig()).Info()
}

func (dtc *DynamicClientCredentials) Clone() credentials.TransportCredentials {
	return credentials.NewTLS(dtc.latestConfig())
}

func (dtc *DynamicClientCredentials) OverrideServerName(name string) error {
	dtc.TLSConfig.ServerName = name
	return nil
}

// KeepaliveOptions is used to set the gRPC keepalive settings for both
// clients and servers
type KeepaliveOptions struct {
	// ClientInterval is the duration after which if the client does not see
	// any activity from the server it pings the server to see if it is alive
	ClientInterval time.Duration
	// ClientTimeout is the duration the client waits for a response
	// from the server after sending a ping before closing the connection
	ClientTimeout time.Duration
	// ServerInterval is the duration after which if the server does not see
	// any activity from the client it pings the client to see if it is alive
	ServerInterval time.Duration
	// ServerTimeout is the duration the server waits for a response
	// from the client after sending a ping before closing the connection
	ServerTimeout time.Duration
	// ServerMinInterval is the minimum permitted time between client pings.
	// If clients send pings more frequently, the server will disconnect them
	ServerMinInterval time.Duration
}

// SecureOptions defines the TLS security parameters for a GRPCServer or
// GRPCClient instance.
type SecureOptions struct {
	// VerifyCertificate, if not nil, is called after normal
	// certificate verification by either a TLS client or server.
	// If it returns a non-nil error, the handshake is aborted and that error results.
	VerifyCertificate func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error
	// PEM-encoded X509 public key to be used for TLS communication
	Certificate []byte
	// PEM-encoded private key to be used for TLS communication
	Key []byte
	// Set of PEM-encoded X509 certificate authorities used by clients to
	// verify server certificates
	ServerRootCAs [][]byte
	// Set of PEM-encoded X509 certificate authorities used by servers to
	// verify client certificates
	ClientRootCAs [][]byte
	// Whether or not to use TLS for communication
	UseTLS bool
	// Whether or not TLS client must present certificates for authentication
	RequireClientCert bool
	// CipherSuites is a list of supported cipher suites for TLS
	CipherSuites []uint16
	// TimeShift makes TLS handshakes time sampling shift to the past by a given duration
	TimeShift time.Duration
	// ServerNameOverride is used to verify the hostname on the returned certificates. It
	// is also included in the client's handshake to support virtual hosting
	// unless it is an IP address.
	ServerNameOverride string
}

func (so SecureOptions) TLSConfig() (*tls.Config, error) {
	// if TLS is not enabled, return
	if !so.UseTLS {
		return nil, nil
	}

	tlsConfig := &tls.Config{
		MinVersion:            tls.VersionTLS12,
		ServerName:            so.ServerNameOverride,
		VerifyPeerCertificate: so.VerifyCertificate,
	}
	if len(so.ServerRootCAs) > 0 {
		tlsConfig.RootCAs = x509.NewCertPool()
		for _, certBytes := range so.ServerRootCAs {
			if !tlsConfig.RootCAs.AppendCertsFromPEM(certBytes) {
				return nil, errors.New("error adding root certificate")
			}
		}
	}

	if so.RequireClientCert {
		cert, err := so.ClientCertificate()
		if err != nil {
			return nil, errors.WithMessage(err, "failed to load client certificate")
		}
		tlsConfig.Certificates = append(tlsConfig.Certificates, cert)
	}

	if so.TimeShift > 0 {
		tlsConfig.Time = func() time.Time {
			return time.Now().Add((-1) * so.TimeShift)
		}
	}

	return tlsConfig, nil
}

func (so SecureOptions) ClientCertificate() (tls.Certificate, error) {
	if so.Key == nil || so.Certificate == nil {
		return tls.Certificate{}, errors.New("both Key and Certificate are required when using mutual TLS")
	}
	cert, err := tls.X509KeyPair(so.Certificate, so.Key)
	if err != nil {
		return tls.Certificate{}, errors.WithMessage(err, "failed to create key pair")
	}
	return cert, nil
}

// ClientConfig defines the parameters for configuring a GRPCClient instance
type ClientConfig struct {
	// SecOpts defines the security parameters
	SecOpts SecureOptions
	// KaOpts defines the keepalive parameters
	KaOpts KeepaliveOptions
	// DialTimeout controls how long the client can block when attempting to
	// establish a connection to a server
	DialTimeout time.Duration
	// AsyncConnect makes connection creation non blocking
	AsyncConnect bool
	// Maximum message size the client can receive
	MaxRecvMsgSize int
	// Maximum message size the client can send
	MaxSendMsgSize int
}

func (cc ClientConfig) DialOptions() ([]grpc.DialOption, error) {
	var dialOpts []grpc.DialOption
	dialOpts = append(dialOpts, grpc.WithKeepaliveParams(keepalive.ClientParameters{
		Time:                cc.KaOpts.ClientInterval,
		Timeout:             cc.KaOpts.ClientTimeout,
		PermitWithoutStream: true,
	}))

	// Unless asynchronous connect is set, make connection establishment blocking.
	if !cc.AsyncConnect {
		dialOpts = append(dialOpts,
			grpc.WithBlock(),
			grpc.FailOnNonTempDialError(true),
		)
	}
	// set send/recv message size to package defaults
	maxRecvMsgSize := DefaultMaxRecvMsgSize
	if cc.MaxRecvMsgSize != 0 {
		maxRecvMsgSize = cc.MaxRecvMsgSize
	}
	maxSendMsgSize := DefaultMaxSendMsgSize
	if cc.MaxSendMsgSize != 0 {
		maxSendMsgSize = cc.MaxSendMsgSize
	}
	dialOpts = append(dialOpts, grpc.WithDefaultCallOptions(
		grpc.MaxCallRecvMsgSize(maxRecvMsgSize),
		grpc.MaxCallSendMsgSize(maxSendMsgSize),
	))

	tlsConfig, err := cc.SecOpts.TLSConfig()
	if err != nil {
		return nil, err
	}
	if tlsConfig != nil {
		transportCreds := &DynamicClientCredentials{TLSConfig: tlsConfig}
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(transportCreds))
	} else {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	return dialOpts, nil
}

func (cc ClientConfig) Dial(address string) (*grpc.ClientConn, error) {
	dialOpts, err := cc.DialOptions()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), cc.DialTimeout)
	defer cancel()

	conn, err := grpc.DialContext(ctx, address, dialOpts...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create new connection")
	}
	return conn, nil
}
