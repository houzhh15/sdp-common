package tunnel

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/houzhh15/sdp-common/logging"
)

// SubscriberCallback defines callback function for tunnel notifications
type SubscriberCallback func(*TunnelEvent) error

// Subscriber manages SSE subscription for tunnel notifications (AH side)
type Subscriber struct {
	controllerURL string
	agentID       string
	client        *http.Client
	callback      SubscriberCallback
	logger        logging.Logger
	stopChan      chan struct{}
	wg            sync.WaitGroup
	mu            sync.RWMutex
	connected     bool
}

// SubscriberConfig holds Subscriber configuration
type SubscriberConfig struct {
	ControllerURL string
	AgentID       string
	TLSConfig     *tls.Config
	Callback      SubscriberCallback
	Logger        logging.Logger
}

// NewSubscriber creates a new tunnel subscriber
func NewSubscriber(config *SubscriberConfig) *Subscriber {
	if config.Logger == nil {
		config.Logger = &noopLogger{}
	}

	return &Subscriber{
		controllerURL: config.ControllerURL,
		agentID:       config.AgentID,
		client: &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: config.TLSConfig,
			},
			Timeout: 0, // No timeout for SSE long connections
		},
		callback: config.Callback,
		logger:   config.Logger,
		stopChan: make(chan struct{}),
	}
}

// Start begins subscribing to tunnel notifications
func (s *Subscriber) Start(ctx context.Context) error {
	s.wg.Add(1)
	go s.subscribeLoop(ctx)
	return nil
}

// Stop stops the subscriber
func (s *Subscriber) Stop() error {
	close(s.stopChan)
	s.wg.Wait()
	return nil
}

// IsConnected returns whether the subscriber is connected
func (s *Subscriber) IsConnected() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.connected
}

// subscribeLoop maintains SSE connection with exponential backoff retry
func (s *Subscriber) subscribeLoop(ctx context.Context) {
	defer s.wg.Done()

	backoff := time.Second
	maxBackoff := 60 * time.Second

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopChan:
			return
		default:
		}

		s.logger.Info("Connecting to SSE stream", "agent_id", s.agentID)

		err := s.connectAndListen(ctx)
		if err != nil {
			s.logger.Error("SSE connection failed", "error", err.Error(), "retry_in", backoff.String())

			// Mark as disconnected
			s.mu.Lock()
			s.connected = false
			s.mu.Unlock()

			// Exponential backoff
			select {
			case <-time.After(backoff):
				backoff *= 2
				if backoff > maxBackoff {
					backoff = maxBackoff
				}
			case <-ctx.Done():
				return
			case <-s.stopChan:
				return
			}
			continue
		}

		// Connection successful, reset backoff
		backoff = time.Second
	}
}

// connectAndListen establishes SSE connection and listens for events
func (s *Subscriber) connectAndListen(ctx context.Context) error {
	// Build SSE URL
	url := fmt.Sprintf("%s/v1/agent/tunnels/stream?agent_id=%s",
		strings.TrimSuffix(s.controllerURL, "/"),
		s.agentID)

	// Create request
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	// Set SSE headers
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Connection", "keep-alive")

	// Send request
	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	s.logger.Info("SSE connected", "agent_id", s.agentID)

	// Mark as connected
	s.mu.Lock()
	s.connected = true
	s.mu.Unlock()

	// Read SSE event stream
	return s.readEventStream(ctx, resp.Body)
}

// readEventStream reads and processes SSE events
func (s *Subscriber) readEventStream(ctx context.Context, body io.ReadCloser) error {
	reader := bufio.NewReader(body)
	var eventType string
	var eventData string

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.stopChan:
			return nil
		default:
		}

		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				return fmt.Errorf("connection closed")
			}
			return fmt.Errorf("read line: %w", err)
		}

		line = strings.TrimSpace(line)

		// Empty line indicates end of event
		if line == "" {
			if eventType != "" && eventData != "" {
				s.logger.Debug("SSE event received", "event_type", eventType, "data_len", len(eventData))
				if err := s.handleEvent(eventType, eventData); err != nil {
					s.logger.Error("Failed to handle event", "type", eventType, "error", err.Error())
				}
				eventType = ""
				eventData = ""
			}
			continue
		}

		// Parse SSE field
		if strings.HasPrefix(line, "event:") {
			eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		} else if strings.HasPrefix(line, "data:") {
			eventData = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		}
	}
}

// handleEvent processes different event types
func (s *Subscriber) handleEvent(eventType, data string) error {
	switch eventType {
	case "connected":
		s.logger.Info("SSE connection established", "message", data)
		return nil

	case "tunnel":
		// Parse tunnel event
		var event TunnelEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			return fmt.Errorf("parse tunnel event: %w", err)
		}

		s.logger.Info("Received tunnel event",
			"tunnel_id", event.Tunnel.ID,
			"type", event.Type,
			"service_id", event.Tunnel.ServiceID)

		// Invoke callback
		if s.callback != nil {
			return s.callback(&event)
		}
		return nil

	case "heartbeat":
		// Heartbeat to keep connection alive
		return nil

	default:
		s.logger.Warn("Unknown event type", "type", eventType)
		return nil
	}
}

// noopLogger is a no-op logger implementation
type noopLogger struct{}

func (l *noopLogger) Info(msg string, args ...interface{})  {}
func (l *noopLogger) Warn(msg string, args ...interface{})  {}
func (l *noopLogger) Error(msg string, args ...interface{}) {}
func (l *noopLogger) Debug(msg string, args ...interface{}) {}
