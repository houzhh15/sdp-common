// Package main demonstrates how to use the Controller SDK
//
// This example shows how to start a complete SDP Controller with just a few lines of code.
// The Controller SDK (github.com/houzhh15/sdp-common/controller) provides:
//   - Complete HTTP REST API implementation (SDP 2.0 specification)
//   - Session management, policy evaluation, tunnel management
//   - TCP Proxy for data plane
//   - SSE push notifications for AH Agents
//
// Usage:
//
//	go run main.go
//
// For production:
//   - Replace InMemoryTunnelManager with database implementation
//   - Customize policy evaluation logic
//   - Integrate monitoring and alerting
package main

import (
	"flag"
	"log"
	"time"

	"github.com/houzhh15/sdp-common/controller"
	"github.com/houzhh15/sdp-common/policy"
)

var (
	certFile  = flag.String("cert", "../../certs/controller-cert.pem", "Certificate file")
	keyFile   = flag.String("key", "../../certs/controller-key.pem", "Private key file")
	caFile    = flag.String("ca", "../../certs/ca-cert.pem", "CA certificate file")
	httpAddr  = flag.String("addr", ":8443", "HTTPS server address")
	proxyAddr = flag.String("proxy-addr", ":9443", "TCP proxy address")
	logLevel  = flag.String("log-level", "info", "Log level (debug, info, warn, error)")
)

func main() {
	flag.Parse()

	// Create Controller with SDK
	ctrl, err := controller.New(&controller.Config{
		CertFile:     *certFile,
		KeyFile:      *keyFile,
		CAFile:       *caFile,
		HTTPAddr:     *httpAddr,
		TCPProxyAddr: *proxyAddr,
		LogLevel:     *logLevel,
		DBPath:       "controller.db",
	})
	if err != nil {
		log.Fatalf("Failed to create controller: %v", err)
	}

	// Pre-configure a demo service
	if err := ctrl.AddService("demo-service-001", "localhost", 9999); err != nil {
		log.Printf("Warning: Failed to add demo service: %v", err)
	}

	// Add demo policies (for testing)
	// In production, policies should be managed via REST API or admin UI
	if err := ctrl.AddPolicy(&policy.Policy{
		PolicyID:  "policy-allow-ih-client",
		ClientID:  "ih-client", // Match the client ID from session
		ServiceID: "demo-service-001",
		// Note: TargetHost/TargetPort are now obtained from ServiceConfig, not Policy
		Conditions: []*policy.Condition{},
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}); err != nil {
		log.Printf("Warning: Failed to add demo policy: %v", err)
	}

	// Start the Controller (blocks until interrupted with Ctrl+C)
	if err := ctrl.Start(); err != nil {
		log.Fatalf("Controller error: %v", err)
	}
}
