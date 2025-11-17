package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the complete SDP configuration
type Config struct {
	Component ComponentConfig `yaml:"component" json:"component"`
	TLS       TLSConfig       `yaml:"tls" json:"tls"`
	Auth      AuthConfig      `yaml:"auth" json:"auth"`
	Policy    PolicyConfig    `yaml:"policy" json:"policy"`
	Logging   LoggingConfig   `yaml:"logging" json:"logging"`
	Transport TransportConfig `yaml:"transport" json:"transport"`
}

// ComponentConfig defines the component type and metadata
type ComponentConfig struct {
	Type    string `yaml:"type" json:"type"` // controller, ih, ah
	ID      string `yaml:"id" json:"id"`
	Name    string `yaml:"name" json:"name"`
	Version string `yaml:"version" json:"version"`
}

// TLSConfig defines TLS certificate configuration
type TLSConfig struct {
	CertFile   string `yaml:"cert_file" json:"cert_file"`
	KeyFile    string `yaml:"key_file" json:"key_file"`
	CAFile     string `yaml:"ca_file" json:"ca_file"`
	MinVersion string `yaml:"min_version" json:"min_version"` // TLS1.2, TLS1.3
}

// AuthConfig defines authentication configuration
type AuthConfig struct {
	TokenTTL         time.Duration `yaml:"token_ttl" json:"token_ttl"`
	DeviceValidation bool          `yaml:"device_validation" json:"device_validation"`
	MFARequired      bool          `yaml:"mfa_required" json:"mfa_required"`
}

// PolicyConfig defines policy engine configuration
type PolicyConfig struct {
	Engine   string `yaml:"engine" json:"engine"`     // embedded, external
	Endpoint string `yaml:"endpoint" json:"endpoint"` // for external engine
}

// LoggingConfig defines logging configuration
type LoggingConfig struct {
	Level     string `yaml:"level" json:"level"`           // debug, info, warn, error
	Format    string `yaml:"format" json:"format"`         // json, text
	Output    string `yaml:"output" json:"output"`         // stdout, file
	AuditFile string `yaml:"audit_file" json:"audit_file"` // audit log file path
}

// TransportConfig defines transport layer configuration
type TransportConfig struct {
	HTTPAddr     string        `yaml:"http_addr" json:"http_addr"`
	GRPCAddr     string        `yaml:"grpc_addr" json:"grpc_addr"`
	TCPProxyAddr string        `yaml:"tcp_proxy_addr" json:"tcp_proxy_addr"`
	SSEHeartbeat time.Duration `yaml:"sse_heartbeat" json:"sse_heartbeat"`
	EnableGRPC   bool          `yaml:"enable_grpc" json:"enable_grpc"`
	ReadTimeout  time.Duration `yaml:"read_timeout" json:"read_timeout"`
	WriteTimeout time.Duration `yaml:"write_timeout" json:"write_timeout"`
	IdleTimeout  time.Duration `yaml:"idle_timeout" json:"idle_timeout"`
}

// Loader provides configuration loading functionality
type Loader struct{}

// NewLoader creates a new configuration loader
func NewLoader() *Loader {
	return &Loader{}
}

// Load reads and parses configuration from file
func (l *Loader) Load(path string) (*Config, error) {
	// Read file
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Determine format by extension
	ext := filepath.Ext(path)

	var config Config
	switch ext {
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(data, &config); err != nil {
			return nil, fmt.Errorf("failed to parse YAML: %w", err)
		}
	case ".json":
		if err := json.Unmarshal(data, &config); err != nil {
			return nil, fmt.Errorf("failed to parse JSON: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported config format: %s", ext)
	}

	// Validate configuration
	if err := l.Validate(&config); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	// Set defaults
	l.setDefaults(&config)

	return &config, nil
}

// Validate checks configuration validity
func (l *Loader) Validate(config *Config) error {
	// Validate component type
	switch config.Component.Type {
	case "controller", "ih", "ah":
		// valid
	default:
		return fmt.Errorf("invalid component type: %s (must be controller/ih/ah)", config.Component.Type)
	}

	// Validate required fields
	if config.Component.ID == "" {
		return fmt.Errorf("component.id is required")
	}

	// Validate TLS files exist
	if config.TLS.CertFile != "" {
		if _, err := os.Stat(config.TLS.CertFile); err != nil {
			return fmt.Errorf("cert_file not found: %s", config.TLS.CertFile)
		}
	}
	if config.TLS.KeyFile != "" {
		if _, err := os.Stat(config.TLS.KeyFile); err != nil {
			return fmt.Errorf("key_file not found: %s", config.TLS.KeyFile)
		}
	}
	if config.TLS.CAFile != "" {
		if _, err := os.Stat(config.TLS.CAFile); err != nil {
			return fmt.Errorf("ca_file not found: %s", config.TLS.CAFile)
		}
	}

	// Validate logging level
	switch config.Logging.Level {
	case "debug", "info", "warn", "error", "":
		// valid
	default:
		return fmt.Errorf("invalid logging level: %s", config.Logging.Level)
	}

	// Validate logging format
	switch config.Logging.Format {
	case "json", "text", "":
		// valid
	default:
		return fmt.Errorf("invalid logging format: %s", config.Logging.Format)
	}

	// Validate policy engine
	switch config.Policy.Engine {
	case "embedded", "external", "":
		// valid
	default:
		return fmt.Errorf("invalid policy engine: %s", config.Policy.Engine)
	}

	// If external policy engine, endpoint is required
	if config.Policy.Engine == "external" && config.Policy.Endpoint == "" {
		return fmt.Errorf("policy.endpoint is required when engine=external")
	}

	return nil
}

// setDefaults sets default values for optional fields
func (l *Loader) setDefaults(config *Config) {
	// Component defaults
	if config.Component.Version == "" {
		config.Component.Version = "v1.0.0"
	}

	// Auth defaults
	if config.Auth.TokenTTL == 0 {
		config.Auth.TokenTTL = 3600 * time.Second // 1 hour
	}

	// Logging defaults
	if config.Logging.Level == "" {
		config.Logging.Level = "info"
	}
	if config.Logging.Format == "" {
		config.Logging.Format = "json"
	}
	if config.Logging.Output == "" {
		config.Logging.Output = "stdout"
	}

	// Policy defaults
	if config.Policy.Engine == "" {
		config.Policy.Engine = "embedded"
	}

	// Transport defaults
	if config.Transport.HTTPAddr == "" {
		config.Transport.HTTPAddr = ":8080"
	}
	if config.Transport.GRPCAddr == "" {
		config.Transport.GRPCAddr = ":8081"
	}
	if config.Transport.TCPProxyAddr == "" {
		config.Transport.TCPProxyAddr = ":9443"
	}
	if config.Transport.SSEHeartbeat == 0 {
		config.Transport.SSEHeartbeat = 30 * time.Second
	}
	if config.Transport.ReadTimeout == 0 {
		config.Transport.ReadTimeout = 15 * time.Second
	}
	if config.Transport.WriteTimeout == 0 {
		config.Transport.WriteTimeout = 15 * time.Second
	}
	if config.Transport.IdleTimeout == 0 {
		config.Transport.IdleTimeout = 60 * time.Second
	}

	// TLS defaults
	if config.TLS.MinVersion == "" {
		config.TLS.MinVersion = "TLS1.2"
	}
}

// Watch monitors configuration file for changes (placeholder)
// This method can be extended with fsnotify or similar library
func (l *Loader) Watch(path string, callback func(*Config)) error {
	// Placeholder for future implementation
	// Could use github.com/fsnotify/fsnotify for file watching
	return fmt.Errorf("watch not implemented yet")
}
