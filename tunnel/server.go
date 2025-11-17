package tunnel

import (
	"crypto/tls"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/houzhh15/sdp-common/logging"
)

// DataPlaneServer encapsulates data plane server logic with mTLS support
// It provides a high-level API for Controller to manage data plane connections
type DataPlaneServer struct {
	addr      string
	tlsConfig *tls.Config
	logger    logging.Logger
	listener  net.Listener
	stopChan  chan struct{}
	wg        sync.WaitGroup
	mu        sync.RWMutex

	// Handler for incoming connections
	handler ConnectionHandler

	// Configuration
	bufferSize     int
	connectTimeout time.Duration
	readTimeout    time.Duration
	writeTimeout   time.Duration
	maxConnections int
	activeConns    int
}

// ConnectionHandler handles incoming data plane connections
type ConnectionHandler func(conn net.Conn) error

// DataPlaneServerConfig configuration for data plane server
type DataPlaneServerConfig struct {
	Addr           string      // Listen address (e.g., ":9443")
	TLSConfig      *tls.Config // mTLS configuration
	Logger         logging.Logger
	BufferSize     int           // Buffer size for data transfer (default: 32KB)
	ConnectTimeout time.Duration // Connection timeout (default: 5s)
	ReadTimeout    time.Duration // Read timeout (default: 30s)
	WriteTimeout   time.Duration // Write timeout (default: 30s)
	MaxConnections int           // Max concurrent connections (default: 10000)
}

// NewDataPlaneServer creates a new data plane server with mTLS support
func NewDataPlaneServer(config *DataPlaneServerConfig) *DataPlaneServer {
	if config.BufferSize == 0 {
		config.BufferSize = 32 * 1024 // 32KB
	}
	if config.ConnectTimeout == 0 {
		config.ConnectTimeout = 5 * time.Second
	}
	if config.ReadTimeout == 0 {
		config.ReadTimeout = 30 * time.Second
	}
	if config.WriteTimeout == 0 {
		config.WriteTimeout = 30 * time.Second
	}
	if config.MaxConnections == 0 {
		config.MaxConnections = 10000
	}

	return &DataPlaneServer{
		addr:           config.Addr,
		tlsConfig:      config.TLSConfig,
		logger:         config.Logger,
		stopChan:       make(chan struct{}),
		bufferSize:     config.BufferSize,
		connectTimeout: config.ConnectTimeout,
		readTimeout:    config.ReadTimeout,
		writeTimeout:   config.WriteTimeout,
		maxConnections: config.MaxConnections,
	}
}

// SetHandler sets the connection handler
func (s *DataPlaneServer) SetHandler(handler ConnectionHandler) {
	s.handler = handler
}

// Start starts the data plane server with mTLS
func (s *DataPlaneServer) Start() error {
	if s.handler == nil {
		return fmt.Errorf("connection handler not set")
	}

	var ln net.Listener
	var err error

	if s.tlsConfig != nil {
		// Start with mTLS
		ln, err = tls.Listen("tcp", s.addr, s.tlsConfig)
		if err != nil {
			return fmt.Errorf("failed to start TLS listener on %s: %w", s.addr, err)
		}
		s.logger.Info("Data plane server started with mTLS", "addr", s.addr)
	} else {
		// Fallback to plain TCP (not recommended for production)
		ln, err = net.Listen("tcp", s.addr)
		if err != nil {
			return fmt.Errorf("failed to start TCP listener on %s: %w", s.addr, err)
		}
		s.logger.Warn("Data plane server started WITHOUT TLS", "addr", s.addr)
	}

	s.mu.Lock()
	s.listener = ln
	s.mu.Unlock()

	// Accept connections loop
	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-s.stopChan:
				s.logger.Info("Data plane server stopped")
				return nil
			default:
				s.logger.Error("Failed to accept connection", "error", err)
				continue
			}
		}

		// Check connection limit
		s.mu.Lock()
		if s.activeConns >= s.maxConnections {
			s.mu.Unlock()
			s.logger.Warn("Max connections reached, rejecting", "max", s.maxConnections)
			conn.Close()
			continue
		}
		s.activeConns++
		s.mu.Unlock()

		// Handle connection in background
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			defer func() {
				s.mu.Lock()
				s.activeConns--
				s.mu.Unlock()
			}()

			if err := s.handler(conn); err != nil {
				s.logger.Error("Connection error", "error", err)
			}
		}()
	}
}

// Stop stops the data plane server gracefully
func (s *DataPlaneServer) Stop() error {
	close(s.stopChan)

	s.mu.Lock()
	if s.listener != nil {
		s.listener.Close()
	}
	s.mu.Unlock()

	// Wait for all connections to finish
	s.wg.Wait()

	s.logger.Info("Data plane server stopped gracefully")
	return nil
}

// GetStats returns server statistics
func (s *DataPlaneServer) GetStats() ServerStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return ServerStats{
		ActiveConnections: s.activeConns,
		MaxConnections:    s.maxConnections,
	}
}

// ServerStats contains server statistics
type ServerStats struct {
	ActiveConnections int
	MaxConnections    int
}
