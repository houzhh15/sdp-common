package tunnel

import (
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"

	"github.com/houzhh15/sdp-common/logging"
)

// TunnelConnection represents a paired IH-AH tunnel connection
type TunnelConnection struct {
	TunnelID   string
	IHConn     net.Conn
	AHConn     net.Conn
	CreatedAt  time.Time
	LastActive time.Time
	mu         sync.RWMutex
}

// TCPProxy handles TCP data plane tunneling
type TCPProxy struct {
	tunnels    map[string]*TunnelConnection
	tunnelsMu  sync.RWMutex
	pendingIH  map[string]*TunnelConnection
	pendingAH  map[string]*TunnelConnection
	pendingMu  sync.RWMutex
	logger     logging.Logger
	bufferSize int
	timeout    time.Duration
}

// NewTCPProxy creates a new TCP proxy
func NewTCPProxy(logger logging.Logger, bufferSize int, timeout time.Duration) *TCPProxy {
	if bufferSize <= 0 {
		bufferSize = DefaultConfig().BufferSize
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	proxy := &TCPProxy{
		tunnels:    make(map[string]*TunnelConnection),
		pendingIH:  make(map[string]*TunnelConnection),
		pendingAH:  make(map[string]*TunnelConnection),
		logger:     logger,
		bufferSize: bufferSize,
		timeout:    timeout,
	}

	// Start cleanup goroutine for pending connections
	go proxy.cleanupPendingConnections()

	return proxy
}

// HandleIHConnection handles Initiating Host connection
func (p *TCPProxy) HandleIHConnection(conn net.Conn) {
	defer conn.Close()

	// Read tunnel ID from handshake
	tunnelID, err := p.readTunnelID(conn)
	if err != nil {
		p.logger.Error("Failed to read tunnel ID from IH", "error", err.Error())
		return
	}

	p.logger.Info("IH connected", "tunnel_id", tunnelID)

	// Check if AH already waiting
	p.pendingMu.Lock()
	if ahConn, exists := p.pendingAH[tunnelID]; exists {
		delete(p.pendingAH, tunnelID)
		p.pendingMu.Unlock()

		// Pair immediately
		p.establishTunnel(tunnelID, conn, ahConn.AHConn)
		return
	}

	// Store IH connection as pending
	tunnel := &TunnelConnection{
		TunnelID:   tunnelID,
		IHConn:     conn,
		CreatedAt:  time.Now(),
		LastActive: time.Now(),
	}
	p.pendingIH[tunnelID] = tunnel
	p.pendingMu.Unlock()

	p.logger.Info("IH waiting for AH", "tunnel_id", tunnelID)
}

// HandleAHConnection handles Accepting Host connection
func (p *TCPProxy) HandleAHConnection(conn net.Conn) {
	defer conn.Close()

	// Read tunnel ID from handshake
	tunnelID, err := p.readTunnelID(conn)
	if err != nil {
		p.logger.Error("Failed to read tunnel ID from AH", "error", err.Error())
		return
	}

	p.logger.Info("AH connected", "tunnel_id", tunnelID)

	// Check if IH already waiting
	p.pendingMu.Lock()
	if ihConn, exists := p.pendingIH[tunnelID]; exists {
		delete(p.pendingIH, tunnelID)
		p.pendingMu.Unlock()

		// Pair immediately
		p.establishTunnel(tunnelID, ihConn.IHConn, conn)
		return
	}

	// Store AH connection as pending
	tunnel := &TunnelConnection{
		TunnelID:   tunnelID,
		AHConn:     conn,
		CreatedAt:  time.Now(),
		LastActive: time.Now(),
	}
	p.pendingAH[tunnelID] = tunnel
	p.pendingMu.Unlock()

	p.logger.Info("AH waiting for IH", "tunnel_id", tunnelID)
}

// establishTunnel pairs IH and AH connections and starts forwarding
func (p *TCPProxy) establishTunnel(tunnelID string, ihConn, ahConn net.Conn) {
	tunnel := &TunnelConnection{
		TunnelID:   tunnelID,
		IHConn:     ihConn,
		AHConn:     ahConn,
		CreatedAt:  time.Now(),
		LastActive: time.Now(),
	}

	p.tunnelsMu.Lock()
	p.tunnels[tunnelID] = tunnel
	p.tunnelsMu.Unlock()

	p.logger.Info("Tunnel established", "tunnel_id", tunnelID)

	// Start bidirectional forwarding
	p.forwardBidirectional(tunnel)
}

// readTunnelID reads tunnel ID from connection handshake
// Protocol: Fixed 36-byte Tunnel ID (UUID format, right-padded with null bytes)
func (p *TCPProxy) readTunnelID(conn net.Conn) (string, error) {
	// Set read timeout for handshake
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	defer conn.SetReadDeadline(time.Time{})

	// Read fixed 36-byte tunnel ID (unified with transport.TCPProxyServer)
	buf := make([]byte, TunnelIDLength)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return "", fmt.Errorf("read tunnel ID: %w", err)
	}

	// Trim null padding
	tunnelID := string(buf)
	if idx := len(tunnelID); idx > 0 {
		for i := 0; i < len(tunnelID); i++ {
			if tunnelID[i] == 0 {
				tunnelID = tunnelID[:i]
				break
			}
		}
	}

	if tunnelID == "" {
		return "", fmt.Errorf("empty tunnel ID")
	}

	return tunnelID, nil
}

// forwardBidirectional performs zero-copy bidirectional forwarding
func (p *TCPProxy) forwardBidirectional(tunnel *TunnelConnection) {
	defer func() {
		tunnel.IHConn.Close()
		tunnel.AHConn.Close()
		p.tunnelsMu.Lock()
		delete(p.tunnels, tunnel.TunnelID)
		p.tunnelsMu.Unlock()
		p.logger.Info("Tunnel closed", "tunnel_id", tunnel.TunnelID)
	}()

	errChan := make(chan error, 2)

	// IH -> AH
	go func() {
		n, err := io.Copy(tunnel.AHConn, tunnel.IHConn)
		log.Printf("Tunnel %s: IH->AH copied %d bytes", tunnel.TunnelID, n)
		errChan <- err
	}()

	// AH -> IH
	go func() {
		n, err := io.Copy(tunnel.IHConn, tunnel.AHConn)
		log.Printf("Tunnel %s: AH->IH copied %d bytes", tunnel.TunnelID, n)
		errChan <- err
	}()

	// Wait for either direction to complete
	err := <-errChan
	if err != nil && err != io.EOF {
		p.logger.Error("Forwarding error", "tunnel_id", tunnel.TunnelID, "error", err.Error())
	}
}

// cleanupPendingConnections removes timed-out pending connections
func (p *TCPProxy) cleanupPendingConnections() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now()

		p.pendingMu.Lock()
		for tunnelID, tunnel := range p.pendingIH {
			if now.Sub(tunnel.CreatedAt) > p.timeout {
				p.logger.Warn("IH connection timeout", "tunnel_id", tunnelID)
				tunnel.IHConn.Close()
				delete(p.pendingIH, tunnelID)
			}
		}
		for tunnelID, tunnel := range p.pendingAH {
			if now.Sub(tunnel.CreatedAt) > p.timeout {
				p.logger.Warn("AH connection timeout", "tunnel_id", tunnelID)
				tunnel.AHConn.Close()
				delete(p.pendingAH, tunnelID)
			}
		}
		p.pendingMu.Unlock()
	}
}

// Start starts the TCP proxy server
func (p *TCPProxy) Start(addr string, tlsConfig *tls.Config) error {
	listener, err := tls.Listen("tcp", addr, tlsConfig)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}
	defer listener.Close()

	p.logger.Info("TCP proxy started", "addr", addr)

	for {
		conn, err := listener.Accept()
		if err != nil {
			p.logger.Error("Accept error", "error", err.Error())
			continue
		}

		// Handle connection (distinguish IH/AH by handshake)
		go p.handleConnection(conn)
	}
}

// handleConnection handles a new connection
func (p *TCPProxy) handleConnection(conn net.Conn) {
	// In practice, IH and AH would connect to different ports
	// or send different handshake markers
	// For now, we assume all connections follow the same protocol
	p.HandleIHConnection(conn)
}

// GetStats returns statistics for a tunnel
func (p *TCPProxy) GetStats(tunnelID string) (*TunnelStats, error) {
	p.tunnelsMu.RLock()
	defer p.tunnelsMu.RUnlock()

	tunnel, exists := p.tunnels[tunnelID]
	if !exists {
		return nil, fmt.Errorf("tunnel not found: %s", tunnelID)
	}

	tunnel.mu.RLock()
	defer tunnel.mu.RUnlock()

	// Return basic stats (bytes/packets tracking would need to be added)
	return &TunnelStats{}, nil
}

// Close stops the proxy
func (p *TCPProxy) Close() error {
	p.tunnelsMu.Lock()
	defer p.tunnelsMu.Unlock()

	for _, tunnel := range p.tunnels {
		tunnel.IHConn.Close()
		tunnel.AHConn.Close()
	}

	p.logger.Info("TCP proxy stopped")
	return nil
}
