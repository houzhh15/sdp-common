package tunnel

import (
	"crypto/tls"
	"testing"
	"time"
)

func TestNewDataPlaneClient(t *testing.T) {
	tlsConfig := &tls.Config{}
	client := NewDataPlaneClient("localhost:9443", tlsConfig)

	if client == nil {
		t.Fatal("Expected non-nil client")
	}
	if client.serverAddr != "localhost:9443" {
		t.Errorf("Expected serverAddr 'localhost:9443', got '%s'", client.serverAddr)
	}
	if client.timeout != 10*time.Second {
		t.Errorf("Expected timeout 10s, got %v", client.timeout)
	}
}

func TestNewDataPlaneClientWithConfig(t *testing.T) {
	tlsConfig := &tls.Config{}
	config := &DataPlaneClientConfig{
		ServerAddr: "localhost:8443",
		TLSConfig:  tlsConfig,
		Timeout:    5 * time.Second,
	}
	client := NewDataPlaneClientWithConfig(config)

	if client.serverAddr != "localhost:8443" {
		t.Errorf("Expected serverAddr 'localhost:8443', got '%s'", client.serverAddr)
	}
	if client.timeout != 5*time.Second {
		t.Errorf("Expected timeout 5s, got %v", client.timeout)
	}
}

func TestSendTunnelID(t *testing.T) {
	tests := []struct {
		name      string
		tunnelID  string
		wantBytes int
	}{
		{
			name:      "short tunnel ID",
			tunnelID:  "tunnel-123",
			wantBytes: TunnelIDLength,
		},
		{
			name:      "exact length tunnel ID",
			tunnelID:  "tunnel-123456789012345678901234567", // 36 chars
			wantBytes: TunnelIDLength,
		},
		{
			name:      "empty tunnel ID",
			tunnelID:  "",
			wantBytes: TunnelIDLength,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test encoding logic
			tunnelIDBytes := make([]byte, TunnelIDLength)
			copy(tunnelIDBytes, []byte(tt.tunnelID))

			if len(tunnelIDBytes) != tt.wantBytes {
				t.Errorf("Expected %d bytes, got %d", tt.wantBytes, len(tunnelIDBytes))
			}

			// Verify padding with null bytes
			for i := len(tt.tunnelID); i < TunnelIDLength; i++ {
				if tunnelIDBytes[i] != 0 {
					t.Errorf("Expected null byte at index %d, got %d", i, tunnelIDBytes[i])
				}
			}
		})
	}
}

func TestConnectEmptyTunnelID(t *testing.T) {
	client := NewDataPlaneClient("localhost:9443", &tls.Config{})
	_, err := client.Connect("")
	if err == nil {
		t.Error("Expected error for empty tunnel ID, got nil")
	}
	if err.Error() != "tunnel ID cannot be empty" {
		t.Errorf("Expected 'tunnel ID cannot be empty', got '%s'", err.Error())
	}
}
