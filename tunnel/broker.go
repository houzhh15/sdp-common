package tunnel

import (
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/houzhh15/sdp-common/logging"
)

// Stream represents a bidirectional gRPC stream (interface for proto abstraction)
type Stream interface {
	Send(*DataPacket) error
	Recv() (*DataPacket, error)
}

// Broker manages gRPC bidirectional stream forwarding (optional)
type Broker struct {
	sessions     map[string]*session
	sessionsMu   sync.RWMutex
	logger       logging.Logger
	heartbeatInt time.Duration
	heartbeatTO  time.Duration
	stopChan     chan struct{}
	wg           sync.WaitGroup
}

// session represents a tunnel session with IH and AH streams
type session struct {
	sessionID     string
	ihStream      Stream
	ahStream      Stream
	stats         *TunnelStats
	lastHeartbeat time.Time
	stopChan      chan struct{}
	mu            sync.RWMutex
}

// BrokerConfig holds Broker configuration
type BrokerConfig struct {
	Logger            logging.Logger
	HeartbeatInterval time.Duration
	HeartbeatTimeout  time.Duration
}

// NewBroker creates a new tunnel broker
func NewBroker(config *BrokerConfig) *Broker {
	if config.Logger == nil {
		config.Logger = &noopLogger{}
	}
	if config.HeartbeatInterval <= 0 {
		config.HeartbeatInterval = 30 * time.Second
	}
	if config.HeartbeatTimeout <= 0 {
		config.HeartbeatTimeout = 60 * time.Second
	}

	broker := &Broker{
		sessions:     make(map[string]*session),
		logger:       config.Logger,
		heartbeatInt: config.HeartbeatInterval,
		heartbeatTO:  config.HeartbeatTimeout,
		stopChan:     make(chan struct{}),
	}

	broker.wg.Add(1)
	go broker.heartbeatMonitor()

	return broker
}

// RegisterStream registers IH or AH stream for a session
func (b *Broker) RegisterStream(sessionID string, stream Stream, isIH bool) error {
	b.sessionsMu.Lock()
	defer b.sessionsMu.Unlock()

	sess, exists := b.sessions[sessionID]
	if !exists {
		sess = &session{
			sessionID: sessionID,
			stats: &TunnelStats{
				BytesSent:     0,
				BytesReceived: 0,
				PacketsSent:   0,
				PacketsRecv:   0,
			},
			lastHeartbeat: time.Now(),
			stopChan:      make(chan struct{}),
		}
		b.sessions[sessionID] = sess
	}

	sess.mu.Lock()
	defer sess.mu.Unlock()

	if isIH {
		sess.ihStream = stream
		b.logger.Info("IH stream registered", "session_id", sessionID)
	} else {
		sess.ahStream = stream
		b.logger.Info("AH stream registered", "session_id", sessionID)
	}

	sess.lastHeartbeat = time.Now()

	// If both streams are ready, start forwarding
	if sess.ihStream != nil && sess.ahStream != nil {
		b.wg.Add(2)
		go b.forwardIHtoAH(sess)
		go b.forwardAHtoIH(sess)
		b.logger.Info("Tunnel established", "session_id", sessionID)
	}

	return nil
}

// forwardIHtoAH forwards data from IH to AH
func (b *Broker) forwardIHtoAH(sess *session) {
	defer b.wg.Done()
	defer b.handleForwardError(sess, "IH->AH")

	recvChan := make(chan *DataPacket, 1)
	errChan := make(chan error, 1)

	// Start receive goroutine
	go func() {
		for {
			packet, err := sess.ihStream.Recv()
			if err != nil {
				errChan <- err
				return
			}
			recvChan <- packet
		}
	}()

	for {
		select {
		case <-sess.stopChan:
			return
		case <-b.stopChan:
			return
		case err := <-errChan:
			if err == io.EOF {
				b.logger.Info("IH stream closed", "session_id", sess.sessionID)
			} else {
				b.logger.Error("IH recv error", "session_id", sess.sessionID, "error", err.Error())
			}
			return
		case packet := <-recvChan:
			// Update stats
			sess.mu.Lock()
			sess.lastHeartbeat = time.Now()
			sess.stats.BytesReceived += int64(len(packet.Payload))
			sess.stats.PacketsRecv++
			sess.mu.Unlock()

			// Send to AH
			if err := sess.ahStream.Send(packet); err != nil {
				b.logger.Error("AH send error", "session_id", sess.sessionID, "error", err.Error())
				return
			}

			// Update stats
			sess.mu.Lock()
			sess.stats.BytesSent += int64(len(packet.Payload))
			sess.stats.PacketsSent++
			sess.mu.Unlock()
		}
	}
}

// forwardAHtoIH forwards data from AH to IH
func (b *Broker) forwardAHtoIH(sess *session) {
	defer b.wg.Done()
	defer b.handleForwardError(sess, "AH->IH")

	recvChan := make(chan *DataPacket, 1)
	errChan := make(chan error, 1)

	// Start receive goroutine
	go func() {
		for {
			packet, err := sess.ahStream.Recv()
			if err != nil {
				errChan <- err
				return
			}
			recvChan <- packet
		}
	}()

	for {
		select {
		case <-sess.stopChan:
			return
		case <-b.stopChan:
			return
		case err := <-errChan:
			if err == io.EOF {
				b.logger.Info("AH stream closed", "session_id", sess.sessionID)
			} else {
				b.logger.Error("AH recv error", "session_id", sess.sessionID, "error", err.Error())
			}
			return
		case packet := <-recvChan:
			// Update stats
			sess.mu.Lock()
			sess.lastHeartbeat = time.Now()
			sess.stats.BytesReceived += int64(len(packet.Payload))
			sess.stats.PacketsRecv++
			sess.mu.Unlock()

			// Send to IH
			if err := sess.ihStream.Send(packet); err != nil {
				b.logger.Error("IH send error", "session_id", sess.sessionID, "error", err.Error())
				return
			}

			// Update stats
			sess.mu.Lock()
			sess.stats.BytesSent += int64(len(packet.Payload))
			sess.stats.PacketsSent++
			sess.mu.Unlock()
		}
	}
}

// handleForwardError handles forwarding errors
func (b *Broker) handleForwardError(sess *session, direction string) {
	b.logger.Warn("Forward terminated", "session_id", sess.sessionID, "direction", direction)
	b.CloseSession(sess.sessionID)
}

// heartbeatMonitor monitors session heartbeats
func (b *Broker) heartbeatMonitor() {
	defer b.wg.Done()

	ticker := time.NewTicker(b.heartbeatInt)
	defer ticker.Stop()

	for {
		select {
		case <-b.stopChan:
			return
		case <-ticker.C:
			b.checkHeartbeats()
		}
	}
}

// checkHeartbeats checks for timed-out sessions
func (b *Broker) checkHeartbeats() {
	now := time.Now()
	sessionsToClose := []string{}

	b.sessionsMu.RLock()
	for sessionID, sess := range b.sessions {
		sess.mu.RLock()
		if now.Sub(sess.lastHeartbeat) > b.heartbeatTO {
			sessionsToClose = append(sessionsToClose, sessionID)
		}
		sess.mu.RUnlock()
	}
	b.sessionsMu.RUnlock()

	for _, sessionID := range sessionsToClose {
		b.logger.Warn("Heartbeat timeout", "session_id", sessionID)
		b.CloseSession(sessionID)
	}
}

// CloseSession closes a tunnel session
func (b *Broker) CloseSession(sessionID string) error {
	b.sessionsMu.Lock()
	defer b.sessionsMu.Unlock()

	sess, exists := b.sessions[sessionID]
	if !exists {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	sess.mu.Lock()
	select {
	case <-sess.stopChan:
		// Already closed
	default:
		close(sess.stopChan)
	}
	sess.mu.Unlock()

	delete(b.sessions, sessionID)
	b.logger.Info("Session closed", "session_id", sessionID)

	return nil
}

// GetStats returns statistics for a session
func (b *Broker) GetStats(sessionID string) (*TunnelStats, error) {
	b.sessionsMu.RLock()
	defer b.sessionsMu.RUnlock()

	sess, exists := b.sessions[sessionID]
	if !exists {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	sess.mu.RLock()
	defer sess.mu.RUnlock()

	// Return a copy of stats
	stats := *sess.stats
	return &stats, nil
}

// Close stops the broker
func (b *Broker) Close() error {
	close(b.stopChan)

	// Close all sessions
	b.sessionsMu.Lock()
	for sessionID := range b.sessions {
		sess := b.sessions[sessionID]
		sess.mu.Lock()
		select {
		case <-sess.stopChan:
		default:
			close(sess.stopChan)
		}
		sess.mu.Unlock()
	}
	b.sessionsMu.Unlock()

	b.wg.Wait()
	b.logger.Info("Broker stopped")
	return nil
}
