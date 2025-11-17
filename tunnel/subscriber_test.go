package tunnel

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewSubscriber(t *testing.T) {
	config := &SubscriberConfig{
		ControllerURL: "https://controller:8443",
		AgentID:       "test-agent",
		TLSConfig:     &tls.Config{},
		Callback:      func(e *TunnelEvent) error { return nil },
	}

	sub := NewSubscriber(config)
	if sub == nil {
		t.Fatal("Expected subscriber to be created")
	}
	if sub.agentID != "test-agent" {
		t.Errorf("Expected agent ID test-agent, got %s", sub.agentID)
	}
	if sub.callback == nil {
		t.Error("Expected callback to be set")
	}
}

func TestSubscriberWithoutLogger(t *testing.T) {
	config := &SubscriberConfig{
		ControllerURL: "https://controller:8443",
		AgentID:       "test-agent",
		Callback:      func(e *TunnelEvent) error { return nil },
	}

	sub := NewSubscriber(config)
	if sub.logger == nil {
		t.Error("Expected default logger to be set")
	}
}

func TestSubscriberConnection(t *testing.T) {
	// Create mock SSE server
	eventsSent := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check headers
		if r.Header.Get("Accept") != "text/event-stream" {
			t.Error("Expected Accept: text/event-stream header")
		}

		// Send SSE headers
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Error("Expected response writer to support flushing")
			return
		}

		// Send connected event
		w.Write([]byte("event:connected\n"))
		w.Write([]byte("data:connected\n\n"))
		flusher.Flush()
		eventsSent++

		// Send tunnel event
		tunnelData := `{"type":"created","tunnel":{"id":"test-123","service_id":"svc-1","status":"active"},"timestamp":"2024-01-01T00:00:00Z"}`
		w.Write([]byte("event:tunnel\n"))
		w.Write([]byte("data:" + tunnelData + "\n\n"))
		flusher.Flush()
		eventsSent++

		// Send heartbeat
		w.Write([]byte("event:heartbeat\n"))
		w.Write([]byte("data:ping\n\n"))
		flusher.Flush()
		eventsSent++

		// Keep connection open for a bit
		time.Sleep(200 * time.Millisecond)
	}))
	defer server.Close()

	// Create subscriber
	eventsReceived := 0
	config := &SubscriberConfig{
		ControllerURL: server.URL,
		AgentID:       "test-agent",
		Callback: func(e *TunnelEvent) error {
			eventsReceived++
			if e.Tunnel.ID != "test-123" {
				t.Errorf("Expected tunnel ID test-123, got %s", e.Tunnel.ID)
			}
			return nil
		},
		Logger: &mockLogger{},
	}

	sub := NewSubscriber(config)

	// Start subscriber
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := sub.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start subscriber: %v", err)
	}

	// Wait for events
	time.Sleep(300 * time.Millisecond)

	// Stop subscriber
	cancel()
	sub.Stop()

	// Verify events were received
	if eventsReceived != 1 {
		t.Errorf("Expected 1 tunnel event, got %d", eventsReceived)
	}
	if eventsSent != 3 {
		t.Errorf("Expected 3 events sent, got %d", eventsSent)
	}
}

func TestSubscriberReconnect(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			// Fail first 2 attempts
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}

		// Succeed on 3rd attempt
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("event:connected\n"))
		w.Write([]byte("data:ok\n\n"))
		w.(http.Flusher).Flush()

		time.Sleep(100 * time.Millisecond)
	}))
	defer server.Close()

	config := &SubscriberConfig{
		ControllerURL: server.URL,
		AgentID:       "test-agent",
		Callback:      func(e *TunnelEvent) error { return nil },
		Logger:        &mockLogger{},
	}

	sub := NewSubscriber(config)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sub.Start(ctx)
	defer sub.Stop()

	// Wait for reconnection
	time.Sleep(4 * time.Second)

	// Verify multiple attempts were made
	if attempts < 3 {
		t.Errorf("Expected at least 3 connection attempts, got %d", attempts)
	}

	// Verify eventually connected
	if !sub.IsConnected() {
		t.Error("Expected subscriber to be connected")
	}
}

func TestSubscriberStop(t *testing.T) {
	// Create long-running server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("event:connected\n"))
		w.Write([]byte("data:ok\n\n"))
		w.(http.Flusher).Flush()

		// Keep connection open
		<-r.Context().Done()
	}))
	defer server.Close()

	config := &SubscriberConfig{
		ControllerURL: server.URL,
		AgentID:       "test-agent",
		Callback:      func(e *TunnelEvent) error { return nil },
		Logger:        &mockLogger{},
	}

	sub := NewSubscriber(config)

	ctx := context.Background()
	sub.Start(ctx)

	// Wait for connection
	time.Sleep(100 * time.Millisecond)

	// Stop subscriber
	err := sub.Stop()
	if err != nil {
		t.Errorf("Stop failed: %v", err)
	}

	// Verify stopped
	time.Sleep(100 * time.Millisecond)
}

func TestSubscriberEventParsing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)

		// Send malformed event
		w.Write([]byte("event:tunnel\n"))
		w.Write([]byte("data:{invalid json}\n\n"))
		flusher.Flush()

		// Send valid event
		validData := `{"type":"created","tunnel":{"id":"valid-123","service_id":"svc-1","status":"active"},"timestamp":"2024-01-01T00:00:00Z"}`
		w.Write([]byte("event:tunnel\n"))
		w.Write([]byte("data:" + validData + "\n\n"))
		flusher.Flush()

		time.Sleep(200 * time.Millisecond)
	}))
	defer server.Close()

	validEventsReceived := 0
	config := &SubscriberConfig{
		ControllerURL: server.URL,
		AgentID:       "test-agent",
		Callback: func(e *TunnelEvent) error {
			if e.Tunnel.ID == "valid-123" {
				validEventsReceived++
			}
			return nil
		},
		Logger: &mockLogger{},
	}

	sub := NewSubscriber(config)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sub.Start(ctx)
	time.Sleep(300 * time.Millisecond)
	cancel()
	sub.Stop()

	// Verify only valid event was processed
	if validEventsReceived != 1 {
		t.Errorf("Expected 1 valid event, got %d", validEventsReceived)
	}
}

func TestSubscriberContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("event:connected\n"))
		w.Write([]byte("data:ok\n\n"))
		w.(http.Flusher).Flush()

		// Block until context cancelled
		<-r.Context().Done()
	}))
	defer server.Close()

	config := &SubscriberConfig{
		ControllerURL: server.URL,
		AgentID:       "test-agent",
		Callback:      func(e *TunnelEvent) error { return nil },
		Logger:        &mockLogger{},
	}

	sub := NewSubscriber(config)

	ctx, cancel := context.WithCancel(context.Background())
	sub.Start(ctx)

	// Wait for connection
	time.Sleep(100 * time.Millisecond)

	// Cancel context
	cancel()

	// Wait for graceful shutdown
	time.Sleep(100 * time.Millisecond)

	sub.Stop()
}

func TestSubscriberUnknownEventType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)

		// Send unknown event type
		w.Write([]byte("event:unknown\n"))
		w.Write([]byte("data:some data\n\n"))
		flusher.Flush()

		time.Sleep(100 * time.Millisecond)
	}))
	defer server.Close()

	logger := &mockLogger{}
	config := &SubscriberConfig{
		ControllerURL: server.URL,
		AgentID:       "test-agent",
		Callback:      func(e *TunnelEvent) error { return nil },
		Logger:        logger,
	}

	sub := NewSubscriber(config)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sub.Start(ctx)
	time.Sleep(200 * time.Millisecond)
	cancel()
	sub.Stop()

	// Verify warning was logged
	foundWarning := false
	for _, msg := range logger.messages {
		if strings.Contains(msg, "Unknown event type") {
			foundWarning = true
			break
		}
	}
	if !foundWarning {
		t.Error("Expected warning for unknown event type")
	}
}
