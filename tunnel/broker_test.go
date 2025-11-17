package tunnel

import (
	"errors"
	"io"
	"testing"
	"time"
)

// mockStream implements Stream interface for testing
type mockStream struct {
	sendChan chan *DataPacket
	recvChan chan *DataPacket
	closed   bool
}

func newMockStream() *mockStream {
	return &mockStream{
		sendChan: make(chan *DataPacket, 10),
		recvChan: make(chan *DataPacket, 10),
	}
}

func (s *mockStream) Send(packet *DataPacket) error {
	if s.closed {
		return errors.New("stream closed")
	}
	select {
	case s.sendChan <- packet:
		return nil
	default:
		return errors.New("send buffer full")
	}
}

func (s *mockStream) Recv() (*DataPacket, error) {
	if s.closed {
		return nil, io.EOF
	}
	packet, ok := <-s.recvChan
	if !ok {
		return nil, io.EOF
	}
	return packet, nil
}

func (s *mockStream) close() {
	s.closed = true
	close(s.recvChan)
}

func TestNewBroker(t *testing.T) {
	config := &BrokerConfig{
		Logger:            &mockLogger{},
		HeartbeatInterval: 10 * time.Second,
		HeartbeatTimeout:  30 * time.Second,
	}

	broker := NewBroker(config)
	if broker == nil {
		t.Fatal("Expected broker to be created")
	}
	if broker.heartbeatInt != 10*time.Second {
		t.Errorf("Expected heartbeat interval 10s, got %v", broker.heartbeatInt)
	}
	if broker.heartbeatTO != 30*time.Second {
		t.Errorf("Expected heartbeat timeout 30s, got %v", broker.heartbeatTO)
	}

	broker.Close()
}

func TestBrokerDefaults(t *testing.T) {
	config := &BrokerConfig{}
	broker := NewBroker(config)

	if broker.logger == nil {
		t.Error("Expected default logger to be set")
	}
	if broker.heartbeatInt != 30*time.Second {
		t.Errorf("Expected default heartbeat interval 30s, got %v", broker.heartbeatInt)
	}
	if broker.heartbeatTO != 60*time.Second {
		t.Errorf("Expected default heartbeat timeout 60s, got %v", broker.heartbeatTO)
	}

	broker.Close()
}

func TestBrokerRegisterStream(t *testing.T) {
	config := &BrokerConfig{Logger: &mockLogger{}}
	broker := NewBroker(config)
	defer broker.Close()

	ihStream := newMockStream()
	sessionID := "session-123"

	err := broker.RegisterStream(sessionID, ihStream, true)
	if err != nil {
		t.Fatalf("Failed to register IH stream: %v", err)
	}

	// Verify session exists
	broker.sessionsMu.RLock()
	sess, exists := broker.sessions[sessionID]
	broker.sessionsMu.RUnlock()

	if !exists {
		t.Fatal("Expected session to be created")
	}
	if sess.ihStream == nil {
		t.Error("Expected IH stream to be set")
	}
}

func TestBrokerPairing(t *testing.T) {
	config := &BrokerConfig{Logger: &mockLogger{}}
	broker := NewBroker(config)
	defer broker.Close()

	sessionID := "session-pair"
	ihStream := newMockStream()
	ahStream := newMockStream()

	// Register IH
	broker.RegisterStream(sessionID, ihStream, true)

	// Register AH
	broker.RegisterStream(sessionID, ahStream, false)

	// Wait for forwarding to start
	time.Sleep(50 * time.Millisecond)

	// Verify both streams are registered
	broker.sessionsMu.RLock()
	sess := broker.sessions[sessionID]
	broker.sessionsMu.RUnlock()

	if sess.ihStream == nil || sess.ahStream == nil {
		t.Error("Expected both streams to be registered")
	}
}

func TestBrokerForwarding(t *testing.T) {
	config := &BrokerConfig{Logger: &mockLogger{}}
	broker := NewBroker(config)
	defer broker.Close()

	sessionID := "session-forward"
	ihStream := newMockStream()
	ahStream := newMockStream()

	// Register both streams
	broker.RegisterStream(sessionID, ihStream, true)
	broker.RegisterStream(sessionID, ahStream, false)

	// Wait for forwarding to start
	time.Sleep(50 * time.Millisecond)

	// Send packet from IH
	packet := &DataPacket{
		TunnelID:  sessionID,
		Sequence:  1,
		Payload:   []byte("test data from IH"),
		Timestamp: time.Now(),
		Direction: "IH->AH",
	}
	ihStream.recvChan <- packet

	// Receive on AH side
	select {
	case received := <-ahStream.sendChan:
		if string(received.Payload) != string(packet.Payload) {
			t.Errorf("Expected payload '%s', got '%s'", packet.Payload, received.Payload)
		}
	case <-time.After(time.Second):
		t.Error("Timeout waiting for forwarded packet")
	}
}

func TestBrokerBidirectionalForwarding(t *testing.T) {
	config := &BrokerConfig{Logger: &mockLogger{}}
	broker := NewBroker(config)
	defer broker.Close()

	sessionID := "session-bidir"
	ihStream := newMockStream()
	ahStream := newMockStream()

	broker.RegisterStream(sessionID, ihStream, true)
	broker.RegisterStream(sessionID, ahStream, false)

	time.Sleep(50 * time.Millisecond)

	// Send from IH to AH
	packet1 := &DataPacket{
		Payload: []byte("IH->AH"),
	}
	ihStream.recvChan <- packet1

	// Send from AH to IH
	packet2 := &DataPacket{
		Payload: []byte("AH->IH"),
	}
	ahStream.recvChan <- packet2

	// Verify both directions
	select {
	case received := <-ahStream.sendChan:
		if string(received.Payload) != "IH->AH" {
			t.Error("IH->AH forwarding failed")
		}
	case <-time.After(time.Second):
		t.Error("Timeout on IH->AH")
	}

	select {
	case received := <-ihStream.sendChan:
		if string(received.Payload) != "AH->IH" {
			t.Error("AH->IH forwarding failed")
		}
	case <-time.After(time.Second):
		t.Error("Timeout on AH->IH")
	}
}

func TestBrokerStats(t *testing.T) {
	config := &BrokerConfig{Logger: &mockLogger{}}
	broker := NewBroker(config)
	defer broker.Close()

	sessionID := "session-stats"
	ihStream := newMockStream()
	ahStream := newMockStream()

	broker.RegisterStream(sessionID, ihStream, true)
	broker.RegisterStream(sessionID, ahStream, false)

	time.Sleep(50 * time.Millisecond)

	// Send packet
	packet := &DataPacket{
		Payload: []byte("test data"),
	}
	ihStream.recvChan <- packet

	// Wait for forwarding
	<-ahStream.sendChan
	time.Sleep(50 * time.Millisecond)

	// Get stats
	stats, err := broker.GetStats(sessionID)
	if err != nil {
		t.Fatalf("Failed to get stats: %v", err)
	}

	if stats.BytesReceived != int64(len(packet.Payload)) {
		t.Errorf("Expected %d bytes received, got %d", len(packet.Payload), stats.BytesReceived)
	}
	if stats.BytesSent != int64(len(packet.Payload)) {
		t.Errorf("Expected %d bytes sent, got %d", len(packet.Payload), stats.BytesSent)
	}
	if stats.PacketsRecv != 1 {
		t.Errorf("Expected 1 packet received, got %d", stats.PacketsRecv)
	}
	if stats.PacketsSent != 1 {
		t.Errorf("Expected 1 packet sent, got %d", stats.PacketsSent)
	}
}

func TestBrokerCloseSession(t *testing.T) {
	config := &BrokerConfig{Logger: &mockLogger{}}
	broker := NewBroker(config)
	defer broker.Close()

	sessionID := "session-close"
	ihStream := newMockStream()
	ahStream := newMockStream()

	broker.RegisterStream(sessionID, ihStream, true)
	broker.RegisterStream(sessionID, ahStream, false)

	time.Sleep(50 * time.Millisecond)

	// Close session
	err := broker.CloseSession(sessionID)
	if err != nil {
		t.Errorf("CloseSession failed: %v", err)
	}

	// Verify session removed
	broker.sessionsMu.RLock()
	_, exists := broker.sessions[sessionID]
	broker.sessionsMu.RUnlock()

	if exists {
		t.Error("Expected session to be removed")
	}

	// Try to close again
	err = broker.CloseSession(sessionID)
	if err == nil {
		t.Error("Expected error when closing non-existent session")
	}
}

func TestBrokerHeartbeatTimeout(t *testing.T) {
	config := &BrokerConfig{
		Logger:            &mockLogger{},
		HeartbeatInterval: 50 * time.Millisecond,
		HeartbeatTimeout:  100 * time.Millisecond,
	}
	broker := NewBroker(config)
	defer broker.Close()

	sessionID := "session-timeout"
	ihStream := newMockStream()
	ahStream := newMockStream()

	broker.RegisterStream(sessionID, ihStream, true)
	broker.RegisterStream(sessionID, ahStream, false)

	// Don't send any packets (no heartbeat update)
	time.Sleep(200 * time.Millisecond)

	// Verify session was closed due to timeout
	broker.sessionsMu.RLock()
	_, exists := broker.sessions[sessionID]
	broker.sessionsMu.RUnlock()

	if exists {
		t.Error("Expected session to be closed due to heartbeat timeout")
	}
}

func TestBrokerStreamError(t *testing.T) {
	config := &BrokerConfig{Logger: &mockLogger{}}
	broker := NewBroker(config)
	defer broker.Close()

	sessionID := "session-error"
	ihStream := newMockStream()
	ahStream := newMockStream()

	broker.RegisterStream(sessionID, ihStream, true)
	broker.RegisterStream(sessionID, ahStream, false)

	time.Sleep(50 * time.Millisecond)

	// Close IH stream to simulate error
	ihStream.close()

	// Wait for error handling
	time.Sleep(100 * time.Millisecond)

	// Verify session was closed
	broker.sessionsMu.RLock()
	_, exists := broker.sessions[sessionID]
	broker.sessionsMu.RUnlock()

	if exists {
		t.Error("Expected session to be closed after stream error")
	}
}

func TestBrokerClose(t *testing.T) {
	config := &BrokerConfig{Logger: &mockLogger{}}
	broker := NewBroker(config)

	sessionID := "session-broker-close"
	ihStream := newMockStream()
	ahStream := newMockStream()

	broker.RegisterStream(sessionID, ihStream, true)
	broker.RegisterStream(sessionID, ahStream, false)

	time.Sleep(50 * time.Millisecond)

	// Close broker
	err := broker.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}

	// Verify all sessions closed
	broker.sessionsMu.RLock()
	numSessions := len(broker.sessions)
	broker.sessionsMu.RUnlock()

	if numSessions != 0 {
		t.Errorf("Expected 0 sessions after close, got %d", numSessions)
	}
}

func TestBrokerMultipleSessions(t *testing.T) {
	config := &BrokerConfig{Logger: &mockLogger{}}
	broker := NewBroker(config)
	defer broker.Close()

	// Create 3 sessions
	for i := 0; i < 3; i++ {
		sessionID := "session-" + string(rune('A'+i))
		ihStream := newMockStream()
		ahStream := newMockStream()

		broker.RegisterStream(sessionID, ihStream, true)
		broker.RegisterStream(sessionID, ahStream, false)
	}

	time.Sleep(50 * time.Millisecond)

	// Verify all sessions exist
	broker.sessionsMu.RLock()
	numSessions := len(broker.sessions)
	broker.sessionsMu.RUnlock()

	if numSessions != 3 {
		t.Errorf("Expected 3 sessions, got %d", numSessions)
	}
}
