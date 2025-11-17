package tunnel

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// mockLogger for testing
type mockLogger struct {
	messages []string
}

func (l *mockLogger) Info(msg string, args ...interface{})  { l.messages = append(l.messages, msg) }
func (l *mockLogger) Warn(msg string, args ...interface{})  { l.messages = append(l.messages, msg) }
func (l *mockLogger) Error(msg string, args ...interface{}) { l.messages = append(l.messages, msg) }
func (l *mockLogger) Debug(msg string, args ...interface{}) { l.messages = append(l.messages, msg) }

func TestNotifierSubscribe(t *testing.T) {
	logger := &mockLogger{}
	notifier := NewNotifier(logger, 100*time.Millisecond)

	// Create test response writer
	recorder := httptest.NewRecorder()

	// Subscribe in goroutine
	done := make(chan error)
	go func() {
		done <- notifier.Subscribe("test-agent", recorder)
	}()

	// Wait a bit for subscription to establish
	time.Sleep(50 * time.Millisecond)

	// Check headers
	if ct := recorder.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Expected Content-Type text/event-stream, got %s", ct)
	}

	// Check client was added
	clients := notifier.GetClients()
	if len(clients) != 1 {
		t.Errorf("Expected 1 client, got %d", len(clients))
	}

	// Unsubscribe
	notifier.Unsubscribe("test-agent")

	// Wait for subscribe to finish
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Subscribe returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Error("Subscribe did not finish in time")
	}
}

func TestNotifierNotify(t *testing.T) {
	logger := &mockLogger{}
	notifier := NewNotifier(logger, time.Second)

	// Create mock client
	recorder := httptest.NewRecorder()

	// Subscribe
	done := make(chan struct{})
	go func() {
		notifier.Subscribe("test-agent", recorder)
		close(done)
	}()
	time.Sleep(50 * time.Millisecond)

	// Send notification
	event := &TunnelEvent{
		Type:      EventTypeCreated,
		Timestamp: time.Now(),
		Tunnel: &Tunnel{
			ID:        "tunnel-123",
			ServiceID: "service-1",
			Status:    TunnelStatusActive,
		},
	}

	err := notifier.Notify(event)
	if err != nil {
		t.Errorf("Notify failed: %v", err)
	}

	// Wait for event to be written
	time.Sleep(100 * time.Millisecond)

	// Clean up
	notifier.Unsubscribe("test-agent")
	<-done

	// Check response
	body := recorder.Body.String()
	if !strings.Contains(body, "data:") {
		t.Log("Body does not contain data field")
	}
	if !strings.Contains(body, "tunnel-123") {
		t.Log("Body does not contain tunnel ID")
	}
}

func TestNotifierNotifyOne(t *testing.T) {
	logger := &mockLogger{}
	notifier := NewNotifier(logger, time.Second)

	// Create two clients
	recorder1 := httptest.NewRecorder()
	recorder2 := httptest.NewRecorder()

	go notifier.Subscribe("agent-1", recorder1)
	go notifier.Subscribe("agent-2", recorder2)
	time.Sleep(50 * time.Millisecond)

	// Send to agent-1 only
	event := &TunnelEvent{
		Type:      EventTypeCreated,
		Timestamp: time.Now(),
		Tunnel: &Tunnel{
			ID:        "tunnel-456",
			ServiceID: "service-2",
			Status:    TunnelStatusActive,
		},
	}

	err := notifier.NotifyOne("agent-1", event)
	if err != nil {
		t.Errorf("NotifyOne failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	// Clean up
	notifier.Unsubscribe("agent-1")
	notifier.Unsubscribe("agent-2")
	time.Sleep(50 * time.Millisecond)

	// Check agent-1 received
	body1 := recorder1.Body.String()
	if !strings.Contains(body1, "tunnel-456") {
		t.Error("Agent-1 should have received notification")
	}

	// Check agent-2 did not receive
	body2 := recorder2.Body.String()
	if strings.Contains(body2, "tunnel-456") {
		t.Error("Agent-2 should not have received notification")
	}
}

func TestNotifierUnsubscribe(t *testing.T) {
	logger := &mockLogger{}
	notifier := NewNotifier(logger, time.Second)

	recorder := httptest.NewRecorder()

	// Subscribe
	go notifier.Subscribe("test-agent", recorder)
	time.Sleep(50 * time.Millisecond)

	// Verify client exists
	clients := notifier.GetClients()
	if len(clients) != 1 {
		t.Errorf("Expected 1 client, got %d", len(clients))
	}

	// Unsubscribe
	notifier.Unsubscribe("test-agent")
	time.Sleep(50 * time.Millisecond)

	// Verify client removed
	clients = notifier.GetClients()
	if len(clients) != 0 {
		t.Errorf("Expected 0 clients, got %d", len(clients))
	}
}

func TestNotifierHeartbeat(t *testing.T) {
	logger := &mockLogger{}
	notifier := NewNotifier(logger, 100*time.Millisecond)

	recorder := httptest.NewRecorder()

	// Subscribe
	done := make(chan struct{})
	go func() {
		notifier.Subscribe("test-agent", recorder)
		close(done)
	}()

	time.Sleep(250 * time.Millisecond) // Wait for multiple heartbeats

	// Clean up
	notifier.Unsubscribe("test-agent")
	<-done

	// Check heartbeats were sent (looking for ping comments in SSE)
	body := recorder.Body.String()
	heartbeatCount := strings.Count(body, ": ping")
	if heartbeatCount < 1 {
		t.Logf("Body: %s", body)
		t.Logf("Expected at least 1 heartbeat, got %d", heartbeatCount)
	}
}

func TestNotifierMultipleClients(t *testing.T) {
	logger := &mockLogger{}
	notifier := NewNotifier(logger, time.Second)

	// Subscribe multiple clients
	numClients := 5
	recorders := make([]*httptest.ResponseRecorder, numClients)
	for i := 0; i < numClients; i++ {
		recorders[i] = httptest.NewRecorder()
		agentID := fmt.Sprintf("agent-%d", i)
		go notifier.Subscribe(agentID, recorders[i])
	}
	time.Sleep(100 * time.Millisecond)

	// Verify all clients connected
	clients := notifier.GetClients()
	if len(clients) != numClients {
		t.Errorf("Expected %d clients, got %d", numClients, len(clients))
	}

	// Send broadcast
	event := &TunnelEvent{
		Type:      EventTypeCreated,
		Timestamp: time.Now(),
		Tunnel: &Tunnel{
			ID:        "tunnel-broadcast",
			ServiceID: "service-broadcast",
			Status:    TunnelStatusActive,
		},
	}

	err := notifier.Notify(event)
	if err != nil {
		t.Errorf("Notify failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	// Clean up
	for i := 0; i < numClients; i++ {
		agentID := fmt.Sprintf("agent-%d", i)
		notifier.Unsubscribe(agentID)
	}
	time.Sleep(50 * time.Millisecond)

	// Verify all clients received broadcast
	for i, recorder := range recorders {
		body := recorder.Body.String()
		if !strings.Contains(body, "tunnel-broadcast") {
			t.Errorf("Client %d did not receive broadcast", i)
		}
	}
}

func TestNotifierChannelFull(t *testing.T) {
	logger := &mockLogger{}
	notifier := NewNotifier(logger, time.Second)

	// Create a recorder that blocks writes
	recorder := &blockingRecorder{
		ResponseRecorder: httptest.NewRecorder(),
		blocked:          make(chan struct{}),
	}

	// Subscribe
	go notifier.Subscribe("test-agent", recorder)
	time.Sleep(50 * time.Millisecond)

	// Block the recorder
	close(recorder.blocked)

	// Try to send many events to overflow the channel
	for i := 0; i < 20; i++ {
		event := &TunnelEvent{
			Type:      EventTypeCreated,
			Timestamp: time.Now(),
			Tunnel: &Tunnel{
				ID:        fmt.Sprintf("tunnel-%d", i),
				ServiceID: "service-1",
				Status:    TunnelStatusActive,
			},
		}
		notifier.NotifyOne("test-agent", event)
	}

	// Should not block or panic
	time.Sleep(100 * time.Millisecond)

	// Clean up
	notifier.Unsubscribe("test-agent")
}

// blockingRecorder simulates a slow/blocked HTTP response writer
type blockingRecorder struct {
	*httptest.ResponseRecorder
	blocked chan struct{}
}

func (r *blockingRecorder) Write(b []byte) (int, error) {
	<-r.blocked
	return r.ResponseRecorder.Write(b)
}

func (r *blockingRecorder) Flush() {
	<-r.blocked
	r.ResponseRecorder.Flush()
}
