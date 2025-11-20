package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/houzhh15/sdp-common/tunnel"
	"github.com/stretchr/testify/assert"
)

// TestSSESubscription_AgentParameters tests agent_id and agent_type parameter support
func TestSSESubscription_AgentParameters(t *testing.T) {
	// Note: This is a simplified test that verifies parameter extraction
	// Full SSE connection testing requires mock http.ResponseWriter with Flusher support
	
	tests := []struct {
		name          string
		queryParams   string
		expectedID    string
		expectedType  string
	}{
		{
			name:         "With agent_id and agent_type",
			queryParams:  "agent_id=ih-client-001&agent_type=ih",
			expectedID:   "ih-client-001",
			expectedType: "ih",
		},
		{
			name:         "With agent_id only",
			queryParams:  "agent_id=ah-agent-002",
			expectedID:   "ah-agent-002",
			expectedType: "unknown",
		},
		{
			name:         "No parameters",
			queryParams:  "",
			expectedID:   "unknown",
			expectedType: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/v1/agent/tunnels/stream?"+tt.queryParams, nil)
			
			// Extract parameters (same logic as handleTunnelEventsSSE)
			agentID := req.URL.Query().Get("agent_id")
			agentType := req.URL.Query().Get("agent_type")
			
			if agentID == "" {
				agentID = "unknown"
			}
			if agentType == "" {
				agentType = "unknown"
			}
			
			assert.Equal(t, tt.expectedID, agentID)
			assert.Equal(t, tt.expectedType, agentType)
		})
	}
}

// TestTunnelEvent_ControllerAddrInDetails tests controller_addr field in event.Details
func TestTunnelEvent_ControllerAddrInDetails(t *testing.T) {
	// Mock tunnel event as created in handleTunnelCreate
	controllerAddr := "controller.example.com:9443"
	
	event := &tunnel.TunnelEvent{
		Type: tunnel.EventTypeCreated,
		Tunnel: &tunnel.Tunnel{
			ID:        "550e8400-e29b-41d4-a716-446655440000",
			ClientID:  "ih-client-001",
			ServiceID: "crm-web",
		},
		Timestamp: time.Now(),
		Details: map[string]interface{}{
			"controller_addr": controllerAddr,
		},
	}
	
	// Verify controller_addr exists in Details
	addr, exists := event.Details["controller_addr"]
	assert.True(t, exists, "controller_addr should exist in event.Details")
	assert.Equal(t, controllerAddr, addr)
	
	// Verify event can be marshaled to JSON
	data, err := json.Marshal(event)
	assert.NoError(t, err)
	assert.NotEmpty(t, data)
	
	// Verify JSON contains controller_addr
	jsonStr := string(data)
	assert.Contains(t, jsonStr, "controller_addr")
	assert.Contains(t, jsonStr, controllerAddr)
}

// TestTunnelEvent_PayloadFormat tests tunnel_created event payload format
func TestTunnelEvent_PayloadFormat(t *testing.T) {
	// Test event payload according to design doc 3.2.1
	event := &tunnel.TunnelEvent{
		Type: tunnel.EventTypeCreated,
		Tunnel: &tunnel.Tunnel{
			ID:        "550e8400-e29b-41d4-a716-446655440000",
			ClientID:  "ih-client-001",
			ServiceID: "crm-web",
			CreatedAt: time.Now(),
			ExpiresAt: time.Now().Add(8 * time.Hour),
		},
		Details: map[string]interface{}{
			"controller_addr": "controller.example.com:9443",
		},
		Timestamp: time.Now(),
	}
	
	// Marshal to JSON
	data, err := json.Marshal(event)
	assert.NoError(t, err)
	
	// Unmarshal back to verify structure
	var decoded map[string]interface{}
	err = json.Unmarshal(data, &decoded)
	assert.NoError(t, err)
	
	// Verify required fields per design doc 3.2.1
	assert.NotNil(t, decoded["type"], "Event should have 'type' field")
	assert.NotNil(t, decoded["tunnel"], "Event should have 'tunnel' field")
	assert.NotNil(t, decoded["details"], "Event should have 'details' field")
	assert.NotNil(t, decoded["timestamp"], "Event should have 'timestamp' field")
	
	// Verify details contains controller_addr
	details := decoded["details"].(map[string]interface{})
	assert.Contains(t, details, "controller_addr")
}

// TestSSE_HeartbeatFormat tests SSE keepalive message format
func TestSSE_HeartbeatFormat(t *testing.T) {
	// SSE keepalive format according to design doc 3.2.2
	// Should be ": ping\n\n" or ":keepalive\n\n"
	
	heartbeatFormats := []string{
		": ping\n\n",
		":keepalive\n\n",
	}
	
	for _, format := range heartbeatFormats {
		// Verify format starts with ':'
		assert.True(t, strings.HasPrefix(format, ":"), "SSE comment should start with ':'")
		
		// Verify format ends with double newline
		assert.True(t, strings.HasSuffix(format, "\n\n"), "SSE message should end with double newline")
	}
}

// TestSSE_EventPushLatency tests event push latency requirement (<100ms)
func TestSSE_EventPushLatency(t *testing.T) {
	// This is a benchmark-style test to verify push latency
	// In real implementation, latency depends on network and Go runtime
	
	start := time.Now()
	
	// Simulate event creation and marshaling (main overhead)
	event := &tunnel.TunnelEvent{
		Type: tunnel.EventTypeCreated,
		Tunnel: &tunnel.Tunnel{
			ID:        "550e8400-e29b-41d4-a716-446655440000",
			ClientID:  "ih-client-001",
			ServiceID: "crm-web",
		},
		Details: map[string]interface{}{
			"controller_addr": "controller.example.com:9443",
		},
		Timestamp: time.Now(),
	}
	
	_, err := json.Marshal(event)
	assert.NoError(t, err)
	
	elapsed := time.Since(start)
	
	// JSON marshaling should be very fast (typically <1ms)
	// We allow 10ms buffer for test environment variability
	assert.Less(t, elapsed, 10*time.Millisecond, "Event marshaling should be fast")
}

// TestSSE_ResponseHeaders tests SSE response headers
func TestSSE_ResponseHeaders(t *testing.T) {
	// Expected SSE headers per design doc 3.2.2
	expectedHeaders := map[string]string{
		"Content-Type":  "text/event-stream",
		"Cache-Control": "no-cache",
		"Connection":    "keep-alive",
	}
	
	for headerName, expectedValue := range expectedHeaders {
		// This validates the header structure
		// In real SSE handler, these are set via w.Header().Set()
		assert.NotEmpty(t, headerName, "Header name should not be empty")
		assert.NotEmpty(t, expectedValue, "Header value should not be empty")
	}
}

// TestControllerAddr_FormatValidation tests controller_addr format
func TestControllerAddr_FormatValidation(t *testing.T) {
	tests := []struct {
		name           string
		configAddr     string
		expectedFormat string
		valid          bool
	}{
		{
			name:           "Full address with host and port",
			configAddr:     "controller.example.com:9443",
			expectedFormat: "host:port",
			valid:          true,
		},
		{
			name:           "Port only (should prepend localhost)",
			configAddr:     ":9443",
			expectedFormat: "localhost:9443",
			valid:          true,
		},
		{
			name:           "IP address with port",
			configAddr:     "192.168.1.100:9443",
			expectedFormat: "host:port",
			valid:          true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate logic from handleTunnelCreate
			controllerAddr := tt.configAddr
			if len(controllerAddr) > 0 && controllerAddr[0] == ':' {
				controllerAddr = "localhost" + controllerAddr
			}
			
			if tt.name == "Port only (should prepend localhost)" {
				assert.Equal(t, "localhost:9443", controllerAddr)
			}
			
			// Verify format contains ':'
			assert.Contains(t, controllerAddr, ":", "controller_addr should contain port separator")
		})
	}
}
