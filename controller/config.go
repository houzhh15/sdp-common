package controller

import "fmt"

// Config Controller configuration
type Config struct {
	// TLS configuration
	CertFile string
	KeyFile  string
	CAFile   string

	// Server addresses
	HTTPAddr     string // HTTPS server address (e.g., ":8443")
	TCPProxyAddr string // TCP proxy address (e.g., ":9443")

	// Logging
	LogLevel string // debug, info, warn, error

	// Database
	DBPath string // SQLite database path (default: "controller.db")
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.CertFile == "" {
		return fmt.Errorf("cert_file is required")
	}
	if c.KeyFile == "" {
		return fmt.Errorf("key_file is required")
	}
	if c.CAFile == "" {
		return fmt.Errorf("ca_file is required")
	}
	if c.HTTPAddr == "" {
		return fmt.Errorf("http_addr is required")
	}
	if c.TCPProxyAddr == "" {
		return fmt.Errorf("tcp_proxy_addr is required")
	}
	if c.LogLevel == "" {
		c.LogLevel = "info"
	}
	return nil
}
