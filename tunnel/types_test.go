package tunnel

import (
	"testing"
	"time"
)

func TestTunnelTypes(t *testing.T) {
	tunnel := &Tunnel{
		ID:         "test-tunnel-001",
		ClientID:   "client-001",
		ServiceID:  "service-001",
		IHEndpoint: "192.168.1.100:12345",
		AHEndpoint: "10.0.0.1:8080",
		Protocol:   "tcp",
		Status:     TunnelStatusActive,
		CreatedAt:  time.Now(),
		LastActive: time.Now(),
		Stats:      &TunnelStats{},
		Metadata: map[string]interface{}{
			"target_host": "backend.example.com",
			"target_port": 8080,
		},
	}

	if tunnel.ID != "test-tunnel-001" {
		t.Errorf("Expected tunnel ID test-tunnel-001, got %s", tunnel.ID)
	}

	if tunnel.Status != TunnelStatusActive {
		t.Errorf("Expected status active, got %s", tunnel.Status)
	}
}

func TestTunnelStatus(t *testing.T) {
	statuses := []TunnelStatus{
		TunnelStatusPending,
		TunnelStatusActive,
		TunnelStatusClosed,
		TunnelStatusError,
	}

	for _, status := range statuses {
		if status == "" {
			t.Error("Status should not be empty")
		}
	}
}

func TestEventTypes(t *testing.T) {
	eventTypes := []EventType{
		EventTypeCreated,
		EventTypeUpdated,
		EventTypeDeleted,
		EventTypeError,
	}

	for _, et := range eventTypes {
		if et == "" {
			t.Error("Event type should not be empty")
		}
	}
}

func TestTunnelEvent(t *testing.T) {
	tunnel := &Tunnel{
		ID:        "test-001",
		ClientID:  "client-001",
		ServiceID: "service-001",
		Status:    TunnelStatusActive,
	}

	event := &TunnelEvent{
		Type:      EventTypeCreated,
		Tunnel:    tunnel,
		Timestamp: time.Now(),
	}

	if event.Type != EventTypeCreated {
		t.Errorf("Expected event type created, got %s", event.Type)
	}

	if event.Tunnel.ID != "test-001" {
		t.Errorf("Expected tunnel ID test-001, got %s", event.Tunnel.ID)
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.TCPProxyAddr != ":9443" {
		t.Errorf("Expected TCP proxy addr :9443, got %s", cfg.TCPProxyAddr)
	}

	if cfg.BufferSize != 32*1024 {
		t.Errorf("Expected buffer size 32KB, got %d", cfg.BufferSize)
	}

	if cfg.SSEHeartbeat != 30*time.Second {
		t.Errorf("Expected SSE heartbeat 30s, got %v", cfg.SSEHeartbeat)
	}
}

func TestTunnelStats(t *testing.T) {
	stats := &TunnelStats{
		BytesSent:     1024,
		BytesReceived: 2048,
		PacketsSent:   10,
		PacketsRecv:   20,
		ErrorCount:    1,
		AvgLatency:    5 * time.Millisecond,
	}

	if stats.BytesSent != 1024 {
		t.Errorf("Expected bytes sent 1024, got %d", stats.BytesSent)
	}

	if stats.AvgLatency != 5*time.Millisecond {
		t.Errorf("Expected avg latency 5ms, got %v", stats.AvgLatency)
	}
}

func TestDataPacket(t *testing.T) {
	packet := &DataPacket{
		TunnelID:  "tunnel-001",
		Sequence:  1,
		Payload:   []byte("test data"),
		Timestamp: time.Now(),
		Direction: "ih_to_ah",
	}

	if packet.TunnelID != "tunnel-001" {
		t.Errorf("Expected tunnel ID tunnel-001, got %s", packet.TunnelID)
	}

	if string(packet.Payload) != "test data" {
		t.Errorf("Expected payload 'test data', got %s", string(packet.Payload))
	}
}
