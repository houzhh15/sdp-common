package tunnel

import (
	"fmt"
	"io"
	"net"
	"testing"
	"time"
)

func TestTCPProxyCreation(t *testing.T) {
	logger := &mockLogger{}
	proxy := NewTCPProxy(logger, 0, 0)

	if proxy == nil {
		t.Fatal("Expected proxy to be created")
	}
	if proxy.bufferSize != DefaultConfig().BufferSize {
		t.Errorf("Expected buffer size %d, got %d", DefaultConfig().BufferSize, proxy.bufferSize)
	}
	if proxy.timeout != 30*time.Second {
		t.Errorf("Expected timeout 30s, got %v", proxy.timeout)
	}
}

func TestTCPProxyReadTunnelID(t *testing.T) {
	logger := &mockLogger{}
	proxy := NewTCPProxy(logger, 0, 0)

	// Create pipe for testing
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	// Write tunnel ID in background
	go func() {
		tunnelID := "test-tunnel-123"
		length := uint16(len(tunnelID))
		client.Write([]byte{byte(length >> 8), byte(length)})
		client.Write([]byte(tunnelID))
	}()

	// Read tunnel ID
	tunnelID, err := proxy.readTunnelID(server)
	if err != nil {
		t.Fatalf("Failed to read tunnel ID: %v", err)
	}

	if tunnelID != "test-tunnel-123" {
		t.Errorf("Expected tunnel ID 'test-tunnel-123', got '%s'", tunnelID)
	}
}

func TestTCPProxyReadTunnelIDInvalidLength(t *testing.T) {
	logger := &mockLogger{}
	proxy := NewTCPProxy(logger, 0, 0)

	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	// Write invalid length
	go func() {
		client.Write([]byte{0xFF, 0xFF}) // length > 256
	}()

	_, err := proxy.readTunnelID(server)
	if err == nil {
		t.Error("Expected error for invalid length")
	}
}

func TestTCPProxyPairing(t *testing.T) {
	logger := &mockLogger{}
	proxy := NewTCPProxy(logger, 0, 5*time.Second)

	// Create mock connections
	ihServer, ihClient := net.Pipe()
	ahServer, ahClient := net.Pipe()

	tunnelID := "tunnel-pair-test"

	// Send tunnel IDs
	go func() {
		sendTunnelID(ihClient, tunnelID)
	}()
	go func() {
		sendTunnelID(ahClient, tunnelID)
	}()

	// Handle connections
	done := make(chan struct{}, 2)
	go func() {
		proxy.HandleIHConnection(ihServer)
		done <- struct{}{}
	}()
	go func() {
		proxy.HandleAHConnection(ahServer)
		done <- struct{}{}
	}()

	// Wait for pairing
	time.Sleep(100 * time.Millisecond)

	// Verify tunnel exists
	proxy.tunnelsMu.RLock()
	tunnel, exists := proxy.tunnels[tunnelID]
	proxy.tunnelsMu.RUnlock()

	if !exists {
		t.Error("Expected tunnel to be established")
	}
	if tunnel.TunnelID != tunnelID {
		t.Errorf("Expected tunnel ID %s, got %s", tunnelID, tunnel.TunnelID)
	}

	// Close connections
	ihServer.Close()
	ahServer.Close()

	// Wait for handlers to finish
	<-done
	<-done
}

func TestTCPProxyForwarding(t *testing.T) {
	logger := &mockLogger{}
	proxy := NewTCPProxy(logger, 0, 0)

	// Create connections
	ihServer, ihClient := net.Pipe()
	ahServer, ahClient := net.Pipe()

	tunnelID := "tunnel-forward-test"

	// Send tunnel IDs
	go func() {
		sendTunnelID(ihClient, tunnelID)
	}()
	go func() {
		sendTunnelID(ahClient, tunnelID)
	}()

	// Handle connections
	go proxy.HandleIHConnection(ihServer)
	go proxy.HandleAHConnection(ahServer)

	// Wait for pairing
	time.Sleep(100 * time.Millisecond)

	// Send data from IH to AH
	testData := []byte("Hello from IH")
	go func() {
		ihClient.Write(testData)
		ihClient.Close()
	}()

	// Read data on AH side
	received := make([]byte, len(testData))
	n, err := io.ReadFull(ahClient, received)
	if err != nil {
		t.Fatalf("Failed to read from AH: %v", err)
	}

	if n != len(testData) {
		t.Errorf("Expected %d bytes, got %d", len(testData), n)
	}
	if string(received) != string(testData) {
		t.Errorf("Expected '%s', got '%s'", testData, received)
	}

	ahClient.Close()
}

func TestTCPProxyTimeout(t *testing.T) {
	logger := &mockLogger{}
	proxy := NewTCPProxy(logger, 0, 200*time.Millisecond)

	// Create IH connection only (no AH)
	ihServer, ihClient := net.Pipe()
	defer ihClient.Close()

	tunnelID := "tunnel-timeout-test"

	// Send tunnel ID
	go func() {
		sendTunnelID(ihClient, tunnelID)
	}()

	// Handle IH connection
	done := make(chan struct{})
	go func() {
		proxy.HandleIHConnection(ihServer)
		done <- struct{}{}
	}()

	// Wait for timeout
	time.Sleep(50 * time.Millisecond)

	// Verify pending IH exists
	proxy.pendingMu.RLock()
	_, exists := proxy.pendingIH[tunnelID]
	proxy.pendingMu.RUnlock()

	if !exists {
		t.Error("Expected pending IH connection")
	}

	// Wait for cleanup
	time.Sleep(300 * time.Millisecond)

	// Verify pending IH was cleaned up
	proxy.pendingMu.RLock()
	_, exists = proxy.pendingIH[tunnelID]
	proxy.pendingMu.RUnlock()

	if exists {
		t.Error("Expected pending IH to be cleaned up")
	}

	<-done
}

func TestTCPProxyGetStats(t *testing.T) {
	logger := &mockLogger{}
	proxy := NewTCPProxy(logger, 0, 0)

	// Add a tunnel manually
	ihServer, _ := net.Pipe()
	ahServer, _ := net.Pipe()

	tunnel := &TunnelConnection{
		TunnelID:  "stats-test",
		IHConn:    ihServer,
		AHConn:    ahServer,
		CreatedAt: time.Now(),
	}

	proxy.tunnelsMu.Lock()
	proxy.tunnels["stats-test"] = tunnel
	proxy.tunnelsMu.Unlock()

	// Get stats
	stats, err := proxy.GetStats("stats-test")
	if err != nil {
		t.Fatalf("Failed to get stats: %v", err)
	}
	if stats == nil {
		t.Error("Expected non-nil stats")
	}

	// Test non-existent tunnel
	_, err = proxy.GetStats("nonexistent")
	if err == nil {
		t.Error("Expected error for non-existent tunnel")
	}
}

func TestTCPProxyClose(t *testing.T) {
	logger := &mockLogger{}
	proxy := NewTCPProxy(logger, 0, 0)

	// Add mock tunnels
	ihServer, ihClient := net.Pipe()
	ahServer, ahClient := net.Pipe()

	tunnel := &TunnelConnection{
		TunnelID: "close-test",
		IHConn:   ihServer,
		AHConn:   ahServer,
	}

	proxy.tunnelsMu.Lock()
	proxy.tunnels["close-test"] = tunnel
	proxy.tunnelsMu.Unlock()

	// Close proxy
	err := proxy.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}

	// Verify connections are closed
	_, err1 := ihClient.Read(make([]byte, 1))
	_, err2 := ahClient.Read(make([]byte, 1))

	if err1 == nil {
		t.Error("Expected IH connection to be closed")
	}
	if err2 == nil {
		t.Error("Expected AH connection to be closed")
	}
}

// Helper function to send tunnel ID
func sendTunnelID(conn net.Conn, tunnelID string) error {
	length := uint16(len(tunnelID))
	if _, err := conn.Write([]byte{byte(length >> 8), byte(length)}); err != nil {
		return fmt.Errorf("write length: %w", err)
	}
	if _, err := conn.Write([]byte(tunnelID)); err != nil {
		return fmt.Errorf("write tunnel ID: %w", err)
	}
	return nil
}
