// Package controller provides a complete SDP Controller SDK
//
// This package combines all sdp-common packages into a ready-to-use Controller
// that implements the SDP 2.0 specification with minimal configuration.
//
// Example usage:
//
//	controller, err := controller.New(&controller.Config{
//		CertFile:     "certs/controller-cert.pem",
//		KeyFile:      "certs/controller-key.pem",
//		CAFile:       "certs/ca-cert.pem",
//		HTTPAddr:     ":8443",
//		TCPProxyAddr: ":9443",
//		LogLevel:     "info",
//	})
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	// Optional: Pre-configure services
//	controller.AddService("web-service", "localhost", 8080)
//
//	// Start (blocks until interrupted)
//	controller.Start()
package controller

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/houzhh15/sdp-common/cert"
	"github.com/houzhh15/sdp-common/logging"
	"github.com/houzhh15/sdp-common/policy"
	"github.com/houzhh15/sdp-common/session"
	"github.com/houzhh15/sdp-common/transport"
	"github.com/houzhh15/sdp-common/tunnel"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// Controller represents a complete SDP Controller instance
type Controller struct {
	config *Config

	// Core SDK components
	certManager    *cert.Manager
	certRegistry   *cert.Registry
	sessionManager *session.Manager
	policyEngine   *policy.Engine
	tunnelManager  *InMemoryTunnelManager
	tunnelNotifier *tunnel.Notifier
	logger         logging.Logger

	// Transport servers
	httpServer  transport.HTTPServer
	relayServer transport.TunnelRelayServer // Controller data plane: IH ↔ Controller ↔ AH

	// Internal state
	db         *gorm.DB
	mux        *http.ServeMux
	ctx        context.Context
	cancelFunc context.CancelFunc
}

// New creates a new Controller instance with the given configuration
func New(cfg *Config) (*Controller, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	// Initialize logger
	logger, err := logging.NewLogger(&logging.Config{
		Level:  cfg.LogLevel,
		Format: "json",
		Output: "stdout",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize logger: %w", err)
	}

	// Initialize certificate manager
	certManager, err := cert.NewManager(&cert.Config{
		CertFile: cfg.CertFile,
		KeyFile:  cfg.KeyFile,
		CAFile:   cfg.CAFile,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize cert manager: %w", err)
	}

	// Validate certificate
	if err := certManager.ValidateExpiry(); err != nil {
		return nil, fmt.Errorf("certificate validation failed: %w", err)
	}

	tlsConfig := certManager.GetTLSConfig()

	// Initialize database
	dbPath := cfg.DBPath
	if dbPath == "" {
		dbPath = "controller.db"
	}
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Initialize certificate registry
	certRegistry, err := cert.NewRegistry(db, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize cert registry: %w", err)
	}

	// Initialize session manager
	sessionManager := session.NewManager(&session.Config{
		TokenTTL:        3600 * time.Second,
		CleanupInterval: 300 * time.Second,
	}, logger)

	// Initialize policy engine
	policyStorage, err := policy.NewDBStorage(db)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize policy storage: %w", err)
	}

	policyEngine, err := policy.NewEngine(&policy.Config{
		Storage:   policyStorage,
		Evaluator: &policy.DefaultEvaluator{},
		Logger:    logger,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize policy engine: %w", err)
	}

	// Initialize tunnel manager
	tunnelManager := NewInMemoryTunnelManager(logger)

	// Initialize tunnel notifier
	tunnelNotifier := tunnel.NewNotifier(logger, 30*time.Second)

	// Initialize HTTP server
	httpServer := transport.NewHTTPServer(tlsConfig)

	// Initialize Tunnel Relay Server for Controller data plane (IH ↔ Controller ↔ AH)
	// NOTE: Controller should use TunnelRelayServer, NOT TCPProxyServer
	// TCPProxyServer is for IH/AH clients connecting directly to targets
	var relayConfig *transport.TunnelRelayConfig
	if cfg.DataPlane != nil {
		// Use configuration from config file
		relayConfig = &transport.TunnelRelayConfig{
			PairingTimeout: cfg.DataPlane.RelayConfig.PairingTimeout,
			BufferSize:     cfg.DataPlane.RelayConfig.BufferSize,
			ReadTimeout:    cfg.DataPlane.RelayConfig.ReadTimeout,
			WriteTimeout:   cfg.DataPlane.RelayConfig.WriteTimeout,
			MaxConnections: cfg.DataPlane.RelayConfig.MaxConnections,
		}
	} else {
		// Use default configuration if not specified
		relayConfig = &transport.TunnelRelayConfig{
			PairingTimeout: 30 * time.Second,
			BufferSize:     32 * 1024,
			ReadTimeout:    300 * time.Second,
			WriteTimeout:   300 * time.Second,
			MaxConnections: 10000,
		}
	}
	relayServer := transport.NewTunnelRelayServer(logger, relayConfig)

	ctx, cancel := context.WithCancel(context.Background())

	c := &Controller{
		config:         cfg,
		certManager:    certManager,
		certRegistry:   certRegistry,
		sessionManager: sessionManager,
		policyEngine:   policyEngine,
		tunnelManager:  tunnelManager.(*InMemoryTunnelManager),
		tunnelNotifier: tunnelNotifier,
		logger:         logger,
		httpServer:     httpServer,
		relayServer:    relayServer,
		db:             db,
		mux:            http.NewServeMux(),
		ctx:            ctx,
		cancelFunc:     cancel,
	}

	// Register HTTP handlers
	c.registerHandlers()

	// Register middleware
	c.registerMiddleware()

	return c, nil
}

// Start starts the Controller and blocks until interrupted
func (c *Controller) Start() error {
	c.logger.Info("Controller starting", "version", "1.0.0")

	// Start data plane server in background with mTLS
	go c.startDataPlane()

	// Start HTTP server in background
	go c.startHTTPServer()

	fmt.Printf("\n✅ Controller started successfully!\n")
	fmt.Printf("   HTTPS Server: https://localhost%s\n", c.config.HTTPAddr)
	fmt.Printf("   TCP Proxy:    localhost%s\n", c.config.TCPProxyAddr)
	fmt.Printf("   Health Check: https://localhost%s/health\n", c.config.HTTPAddr)
	fmt.Printf("   Press Ctrl+C to stop\n\n")

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	c.logger.Info("Shutting down Controller...")
	return c.Stop()
}

// Stop gracefully stops the Controller
func (c *Controller) Stop() error {
	c.cancelFunc()

	if err := c.httpServer.Stop(); err != nil {
		c.logger.Error("Failed to stop HTTP server", "error", err)
	}

	if err := c.relayServer.Stop(); err != nil {
		c.logger.Error("Failed to stop relay server", "error", err)
	}

	c.logger.Info("Controller stopped")
	return nil
}

// AddService adds a pre-configured service to the tunnel manager
func (c *Controller) AddService(serviceID, targetHost string, targetPort int) error {
	return c.tunnelManager.CreateServiceConfig(c.ctx, &tunnel.ServiceConfig{
		ServiceID:   serviceID,
		ServiceName: serviceID,
		TargetHost:  targetHost,
		TargetPort:  targetPort,
		Protocol:    "tcp",
		Status:      tunnel.ServiceStatusActive,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	})
}

// AddPolicy adds a policy to the policy engine
func (c *Controller) AddPolicy(pol *policy.Policy) error {
	return c.policyEngine.SavePolicy(c.ctx, pol)
}

// startDataPlane starts the tunnel relay server with mTLS
func (c *Controller) startDataPlane() {
	// Determine listen address
	listenAddr := c.config.TCPProxyAddr
	if c.config.DataPlane != nil && c.config.DataPlane.ListenAddr != "" {
		listenAddr = c.config.DataPlane.ListenAddr
	}

	c.logger.Info("Starting tunnel relay server (data plane) with mTLS", "addr", listenAddr)

	// Get TLS config - use DataPlane config if available, otherwise fallback to default
	var tlsConfig *tls.Config
	if c.config.DataPlane != nil {
		// Load certificates from DataPlane config
		dataPlaneManager, err := cert.NewManager(&cert.Config{
			CertFile: c.config.DataPlane.TLS.CertFile,
			KeyFile:  c.config.DataPlane.TLS.KeyFile,
			CAFile:   c.config.DataPlane.TLS.CAFile,
		})
		if err != nil {
			c.logger.Error("Failed to load data plane certificates", "error", err)
			return
		}

		tlsConfig = dataPlaneManager.GetTLSConfig()
		// Override client auth mode with DataPlane config
		tlsConfig.ClientAuth = c.config.DataPlane.TLS.GetClientAuthType()
	} else {
		// Fallback to default cert manager
		tlsConfig = c.certManager.GetTLSConfig()
	}

	if err := c.relayServer.StartTLS(listenAddr, tlsConfig); err != nil {
		c.logger.Error("Tunnel relay server error", "error", err)
	}
}

// startHTTPServer starts the HTTP server
func (c *Controller) startHTTPServer() {
	c.logger.Info("Starting HTTPS server", "addr", c.config.HTTPAddr)
	if err := c.httpServer.Start(c.config.HTTPAddr, c.mux); err != nil {
		log.Fatalf("Failed to start HTTP server: %v", err)
	}
}

// registerMiddleware registers HTTP middleware
func (c *Controller) registerMiddleware() {
	c.httpServer.RegisterMiddleware(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			c.logger.Info("HTTP request", "method", r.Method, "path", r.URL.Path)
			next.ServeHTTP(w, r)
			c.logger.Debug("HTTP response", "method", r.Method, "path", r.URL.Path, "duration", time.Since(start))
		})
	})
}

// extractClientID extracts client ID from certificate
func extractClientID(cert *x509.Certificate) string {
	return cert.Subject.CommonName
}

// extractBearerToken extracts Bearer token from Authorization header
func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	return ""
}

// respondError sends a JSON error response
func respondError(w http.ResponseWriter, code string, message string, details interface{}) {
	respondErrorWithStatus(w, code, message, details, http.StatusBadRequest)
}

func respondErrorWithStatus(w http.ResponseWriter, code string, message string, details interface{}, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"type":      "error",
		"status":    "error",
		"code":      code,
		"message":   message,
		"details":   details,
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

// Helper function to calculate certificate fingerprint
func calculateFingerprint(cert *x509.Certificate) string {
	hash := sha256.Sum256(cert.Raw)
	return "sha256:" + hex.EncodeToString(hash[:])
}
