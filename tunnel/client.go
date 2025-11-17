package tunnel

import (
	"crypto/tls"
	"fmt"
	"net"
	"time"
)

// DataPlaneClient encapsulates data plane connection logic
// It handles the tunnel ID handshake protocol and provides a clean API for IH/AH clients
type DataPlaneClient struct {
	serverAddr string
	tlsConfig  *tls.Config
	timeout    time.Duration
}

// DataPlaneClientConfig configuration for data plane client
type DataPlaneClientConfig struct {
	ServerAddr string        // Controller TCP Proxy address (e.g., "localhost:9443")
	TLSConfig  *tls.Config   // mTLS configuration
	Timeout    time.Duration // Connection timeout (default: 10s)
}

// NewDataPlaneClient creates a new data plane client
func NewDataPlaneClient(serverAddr string, tlsConfig *tls.Config) *DataPlaneClient {
	return NewDataPlaneClientWithConfig(&DataPlaneClientConfig{
		ServerAddr: serverAddr,
		TLSConfig:  tlsConfig,
		Timeout:    10 * time.Second,
	})
}

// NewDataPlaneClientWithConfig creates a client with custom configuration
func NewDataPlaneClientWithConfig(config *DataPlaneClientConfig) *DataPlaneClient {
	if config.Timeout == 0 {
		config.Timeout = 10 * time.Second
	}
	return &DataPlaneClient{
		serverAddr: config.ServerAddr,
		tlsConfig:  config.TLSConfig,
		timeout:    config.Timeout,
	}
}

// Connect establishes a data plane connection and sends tunnel ID
// This method encapsulates the handshake protocol details
//
// Usage:
//
//	client := tunnel.NewDataPlaneClient("localhost:9443", tlsConfig)
//	conn, err := client.Connect("tunnel-abc123...")
//	if err != nil {
//	    return err
//	}
//	defer conn.Close()
//	// Start data transfer
//	io.Copy(conn, localConn)
func (c *DataPlaneClient) Connect(tunnelID string) (net.Conn, error) {
	if tunnelID == "" {
		return nil, fmt.Errorf("tunnel ID cannot be empty")
	}

	// 1. Establish TLS connection
	dialer := &tls.Dialer{
		NetDialer: &net.Dialer{
			Timeout: c.timeout,
		},
		Config: c.tlsConfig,
	}

	conn, err := dialer.Dial("tcp", c.serverAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s: %w", c.serverAddr, err)
	}

	// 2. Send tunnel ID (protocol handshake)
	if err := c.sendTunnelID(conn, tunnelID); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to send tunnel ID: %w", err)
	}

	return conn, nil
}

// sendTunnelID sends the tunnel ID using the data plane protocol
// Protocol: Fixed 36-byte tunnel ID (UUID format, right-padded with null bytes)
func (c *DataPlaneClient) sendTunnelID(conn net.Conn, tunnelID string) error {
	// Set write deadline for handshake
	if err := conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return fmt.Errorf("set write deadline: %w", err)
	}
	defer conn.SetWriteDeadline(time.Time{})

	// Encode tunnel ID as fixed 36-byte buffer
	tunnelIDBytes := make([]byte, TunnelIDLength)
	copy(tunnelIDBytes, []byte(tunnelID))

	// Send tunnel ID
	n, err := conn.Write(tunnelIDBytes)
	if err != nil {
		return fmt.Errorf("write tunnel ID: %w", err)
	}
	if n != TunnelIDLength {
		return fmt.Errorf("incomplete write: wrote %d bytes, expected %d", n, TunnelIDLength)
	}

	return nil
}

// ConnectWithRetry establishes connection with retry logic
func (c *DataPlaneClient) ConnectWithRetry(tunnelID string, maxRetries int, retryDelay time.Duration) (net.Conn, error) {
	var lastErr error
	for i := 0; i < maxRetries; i++ {
		conn, err := c.Connect(tunnelID)
		if err == nil {
			return conn, nil
		}
		lastErr = err
		if i < maxRetries-1 {
			time.Sleep(retryDelay)
		}
	}
	return nil, fmt.Errorf("failed after %d retries: %w", maxRetries, lastErr)
}
