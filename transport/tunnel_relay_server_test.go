package transport

import (
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"io"
	"log/slog"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
) // mockConn implements net.Conn for testing
type mockConn struct {
	readBuf  []byte
	writeBuf []byte
	readPos  int
	closed   bool
	mu       sync.Mutex
}

func newMockConn(initialData []byte) *mockConn {
	return &mockConn{
		readBuf:  initialData,
		writeBuf: make([]byte, 0),
	}
}

func (m *mockConn) Read(b []byte) (n int, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return 0, io.EOF
	}
	if m.readPos >= len(m.readBuf) {
		time.Sleep(10 * time.Millisecond)
		return 0, io.EOF
	}
	n = copy(b, m.readBuf[m.readPos:])
	m.readPos += n
	return n, nil
}

func (m *mockConn) Write(b []byte) (n int, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return 0, io.ErrClosedPipe
	}
	m.writeBuf = append(m.writeBuf, b...)
	return len(b), nil
}

func (m *mockConn) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

func (m *mockConn) LocalAddr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 9443}
}

func (m *mockConn) RemoteAddr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345}
}

func (m *mockConn) SetDeadline(t time.Time) error      { return nil }
func (m *mockConn) SetReadDeadline(t time.Time) error  { return nil }
func (m *mockConn) SetWriteDeadline(t time.Time) error { return nil }

// mockTLSConn simulates TLS connection with mTLS certificate
type mockTLSConn struct {
	*mockConn
	clientCN string
}

func newMockTLSConn(initialData []byte, clientCN string) *mockTLSConn {
	return &mockTLSConn{
		mockConn: newMockConn(initialData),
		clientCN: clientCN,
	}
}

func (m *mockTLSConn) ConnectionState() tls.ConnectionState {
	return tls.ConnectionState{
		HandshakeComplete: true,
		PeerCertificates: []*x509.Certificate{
			{Subject: pkix.Name{CommonName: m.clientCN}},
		},
	}
}

// TestDetermineClientType tests client type detection
func TestDetermineClientType(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	server := &tunnelRelayServer{logger: logger}

	tests := []struct {
		name         string
		clientCN     string
		expectedType string
		expectError  bool
	}{
		{"Valid IH client", "ih-client-001", "ih", false},
		{"Valid AH client", "ah-agent-001", "ah", false},
		{"Invalid prefix", "unknown-001", "unknown", true},
		{"Empty CN", "", "unknown", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientType := server.determineClientType(tt.clientCN)
			if tt.expectError {
				assert.Equal(t, "unknown", clientType)
			} else {
				assert.Equal(t, tt.expectedType, clientType)
			}
		})
	}
}

// TestHandleConnection_TunnelIDValidation tests TunnelID validation
func TestHandleConnection_TunnelIDValidation(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	server := &tunnelRelayServer{
		logger:         logger,
		pairingTimeout: 5 * time.Second,
		bufferSize:     32 * 1024,
		pendingIH:      sync.Map{},
		pendingAH:      sync.Map{},
	}

	tests := []struct {
		name        string
		tunnelID    []byte
		expectError bool
	}{
		{"Valid 36-byte TunnelID", []byte("12345678-1234-1234-1234-123456789012"), false},
		{"Invalid short TunnelID", []byte("short-id"), true},
		{"Empty TunnelID", []byte{}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn := newMockTLSConn(tt.tunnelID, "ih-client-test")
			server.handleConnection(conn)
			if tt.expectError {
				assert.True(t, conn.closed, "Connection should be closed for invalid TunnelID")
			}
		})
	}
}

// TestPairing_Success_IHFirst tests successful pairing with IH arriving first
func TestPairing_Success_IHFirst(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	server := &tunnelRelayServer{
		logger:         logger,
		pairingTimeout: 5 * time.Second,
		bufferSize:     32 * 1024,
		pendingIH:      sync.Map{},
		pendingAH:      sync.Map{},
		wg:             sync.WaitGroup{},
		stopChan:       make(chan struct{}),
	}

	tunnelID := "test-tunnel-001-pairing-ih-first--"
	require.Equal(t, 36, len(tunnelID))

	ihData := []byte("hello from IH")
	ihConn := newMockTLSConn(append([]byte(tunnelID), ihData...), "ih-client-001")

	ahData := []byte("hello from AH")
	ahConn := newMockTLSConn(append([]byte(tunnelID), ahData...), "ah-agent-001")

	server.wg.Add(1)
	go func() {
		defer server.wg.Done()
		server.handleConnection(ihConn)
	}()

	time.Sleep(100 * time.Millisecond)

	_, ihExists := server.pendingIH.Load(tunnelID)
	assert.True(t, ihExists, "IH connection should be in pendingIH")

	server.wg.Add(1)
	go func() {
		defer server.wg.Done()
		server.handleConnection(ahConn)
	}()

	time.Sleep(200 * time.Millisecond)

	_, ihStillExists := server.pendingIH.Load(tunnelID)
	_, ahStillExists := server.pendingAH.Load(tunnelID)
	assert.False(t, ihStillExists, "IH should be removed after pairing")
	assert.False(t, ahStillExists, "AH should not exist after pairing")

	close(server.stopChan)
	server.wg.Wait()
}

// TestPairing_Success_AHFirst tests successful pairing with AH arriving first
func TestPairing_Success_AHFirst(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	server := &tunnelRelayServer{
		logger:         logger,
		pairingTimeout: 5 * time.Second,
		bufferSize:     32 * 1024,
		pendingIH:      sync.Map{},
		pendingAH:      sync.Map{},
		wg:             sync.WaitGroup{},
		stopChan:       make(chan struct{}),
	}

	tunnelID := "test-tunnel-002-pairing-ah-first--"
	require.Equal(t, 36, len(tunnelID))

	ahData := []byte("hello from AH")
	ahConn := newMockTLSConn(append([]byte(tunnelID), ahData...), "ah-agent-002")

	ihData := []byte("hello from IH")
	ihConn := newMockTLSConn(append([]byte(tunnelID), ihData...), "ih-client-002")

	server.wg.Add(1)
	go func() {
		defer server.wg.Done()
		server.handleConnection(ahConn)
	}()

	time.Sleep(100 * time.Millisecond)

	_, ahExists := server.pendingAH.Load(tunnelID)
	assert.True(t, ahExists, "AH connection should be in pendingAH")

	server.wg.Add(1)
	go func() {
		defer server.wg.Done()
		server.handleConnection(ihConn)
	}()

	time.Sleep(200 * time.Millisecond)

	_, ihStillExists := server.pendingIH.Load(tunnelID)
	_, ahStillExists := server.pendingAH.Load(tunnelID)
	assert.False(t, ihStillExists, "IH should not exist after pairing")
	assert.False(t, ahStillExists, "AH should be removed after pairing")

	close(server.stopChan)
	server.wg.Wait()
}

// TestPairing_Timeout tests pairing timeout scenario
func TestPairing_Timeout(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	shortTimeout := 2 * time.Second
	server := &tunnelRelayServer{
		logger:         logger,
		pairingTimeout: shortTimeout,
		bufferSize:     32 * 1024,
		pendingIH:      sync.Map{},
		pendingAH:      sync.Map{},
		wg:             sync.WaitGroup{},
		stopChan:       make(chan struct{}),
	}

	go server.cleanupExpiredConnections()

	tunnelID := "test-tunnel-003-timeout-no-match--"
	require.Equal(t, 36, len(tunnelID))

	ihData := []byte("hello from IH")
	ihConn := newMockTLSConn(append([]byte(tunnelID), ihData...), "ih-client-003")

	server.wg.Add(1)
	go func() {
		defer server.wg.Done()
		server.handleConnection(ihConn)
	}()

	time.Sleep(100 * time.Millisecond)
	_, ihExists := server.pendingIH.Load(tunnelID)
	assert.True(t, ihExists, "IH should be in pendingIH initially")

	time.Sleep(shortTimeout + 1*time.Second)

	_, ihStillExists := server.pendingIH.Load(tunnelID)
	assert.False(t, ihStillExists, "Expired IH should be cleaned up")
	assert.True(t, ihConn.closed, "Expired IH should be closed")

	close(server.stopChan)
	server.wg.Wait()
}

// TestGetStats tests statistics retrieval
func TestGetStats(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	server := &tunnelRelayServer{
		logger:         logger,
		pairingTimeout: 5 * time.Second,
		bufferSize:     32 * 1024,
		pendingIH:      sync.Map{},
		pendingAH:      sync.Map{},
		activeTunnels:  5,
		totalRelayed:   1024000,
		errorCount:     3,
	}

	server.pendingIH.Store("tunnel-001", &PendingConnection{
		TunnelID:   "tunnel-001",
		ReceivedAt: time.Now(),
	})
	server.pendingAH.Store("tunnel-002", &PendingConnection{
		TunnelID:   "tunnel-002",
		ReceivedAt: time.Now(),
	})

	stats := server.GetStats()

	assert.Equal(t, 5, stats.ActiveTunnels)
	assert.Equal(t, 2, stats.PendingConnections)
	assert.Equal(t, 1, stats.PendingIH)
	assert.Equal(t, 1, stats.PendingAH)
	assert.Equal(t, uint64(1024000), stats.TotalRelayed)
	assert.Equal(t, 3, stats.ErrorCount)
}

// TestStop_GracefulShutdown tests graceful server shutdown
func TestStop_GracefulShutdown(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	server := &tunnelRelayServer{
		logger:         logger,
		pairingTimeout: 5 * time.Second,
		bufferSize:     32 * 1024,
		pendingIH:      sync.Map{},
		pendingAH:      sync.Map{},
		wg:             sync.WaitGroup{},
		stopChan:       make(chan struct{}),
	}

	ihConn := newMockConn([]byte{})
	ahConn := newMockConn([]byte{})

	server.pendingIH.Store("tunnel-001", &PendingConnection{
		Conn:       ihConn,
		TunnelID:   "tunnel-001",
		ReceivedAt: time.Now(),
	})
	server.pendingAH.Store("tunnel-002", &PendingConnection{
		Conn:       ahConn,
		TunnelID:   "tunnel-002",
		ReceivedAt: time.Now(),
	})

	err := server.Stop()
	assert.NoError(t, err)

	assert.True(t, ihConn.closed)
	assert.True(t, ahConn.closed)

	select {
	case <-server.stopChan:
	default:
		t.Error("stopChan should be closed after Stop")
	}

	err = server.Stop()
	assert.NoError(t, err, "Second Stop should not error")
}
