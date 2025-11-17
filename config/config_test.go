package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoader_Load_YAML(t *testing.T) {
	// Create temporary YAML config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test.yaml")

	yamlContent := `component:
  type: controller
  id: ctrl-001
  name: Test Controller
  version: v1.0.0

tls:
  cert_file: test_cert.pem
  key_file: test_key.pem
  ca_file: test_ca.pem

auth:
  token_ttl: 3600s
  device_validation: true
  mfa_required: false

policy:
  engine: embedded

logging:
  level: info
  format: json
  output: stdout

transport:
  http_addr: ":8080"
  grpc_addr: ":8081"
  tcp_proxy_addr: ":9443"
`

	// Create dummy cert files
	os.WriteFile(filepath.Join(tmpDir, "test_cert.pem"), []byte("cert"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "test_key.pem"), []byte("key"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "test_ca.pem"), []byte("ca"), 0644)

	// Update paths to use temp directory
	yamlContent = `component:
  type: controller
  id: ctrl-001
  name: Test Controller
  version: v1.0.0

tls:
  cert_file: ` + filepath.Join(tmpDir, "test_cert.pem") + `
  key_file: ` + filepath.Join(tmpDir, "test_key.pem") + `
  ca_file: ` + filepath.Join(tmpDir, "test_ca.pem") + `

auth:
  token_ttl: 3600s
  device_validation: true
  mfa_required: false

policy:
  engine: embedded

logging:
  level: info
  format: json
  output: stdout

transport:
  http_addr: ":8080"
  grpc_addr: ":8081"
  tcp_proxy_addr: ":9443"
`

	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to create test config: %v", err)
	}

	loader := NewLoader()
	config, err := loader.Load(configPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Verify loaded config
	if config.Component.Type != "controller" {
		t.Errorf("Expected component.type=controller, got %s", config.Component.Type)
	}
	if config.Component.ID != "ctrl-001" {
		t.Errorf("Expected component.id=ctrl-001, got %s", config.Component.ID)
	}
	if config.Auth.TokenTTL != 3600*time.Second {
		t.Errorf("Expected token_ttl=3600s, got %v", config.Auth.TokenTTL)
	}
	if config.Logging.Level != "info" {
		t.Errorf("Expected logging.level=info, got %s", config.Logging.Level)
	}
}

func TestLoader_Load_JSON(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test.json")

	// Create dummy cert files
	os.WriteFile(filepath.Join(tmpDir, "test_cert.pem"), []byte("cert"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "test_key.pem"), []byte("key"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "test_ca.pem"), []byte("ca"), 0644)

	jsonContent := `{
  "component": {
    "type": "ih",
    "id": "ih-001",
    "name": "Test IH Client"
  },
  "tls": {
    "cert_file": "` + filepath.Join(tmpDir, "test_cert.pem") + `",
    "key_file": "` + filepath.Join(tmpDir, "test_key.pem") + `",
    "ca_file": "` + filepath.Join(tmpDir, "test_ca.pem") + `"
  },
  "auth": {
    "token_ttl": 1800000000000
  },
  "logging": {
    "level": "debug"
  }
}`

	if err := os.WriteFile(configPath, []byte(jsonContent), 0644); err != nil {
		t.Fatalf("Failed to create test config: %v", err)
	}

	loader := NewLoader()
	config, err := loader.Load(configPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if config.Component.Type != "ih" {
		t.Errorf("Expected component.type=ih, got %s", config.Component.Type)
	}
}

func TestLoader_Validate(t *testing.T) {
	loader := NewLoader()

	tests := []struct {
		name    string
		config  *Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid controller config",
			config: &Config{
				Component: ComponentConfig{
					Type: "controller",
					ID:   "ctrl-001",
				},
			},
			wantErr: false,
		},
		{
			name: "invalid component type",
			config: &Config{
				Component: ComponentConfig{
					Type: "invalid",
					ID:   "test-001",
				},
			},
			wantErr: true,
			errMsg:  "invalid component type",
		},
		{
			name: "missing component id",
			config: &Config{
				Component: ComponentConfig{
					Type: "controller",
				},
			},
			wantErr: true,
			errMsg:  "component.id is required",
		},
		{
			name: "invalid logging level",
			config: &Config{
				Component: ComponentConfig{
					Type: "controller",
					ID:   "ctrl-001",
				},
				Logging: LoggingConfig{
					Level: "invalid",
				},
			},
			wantErr: true,
			errMsg:  "invalid logging level",
		},
		{
			name: "external policy without endpoint",
			config: &Config{
				Component: ComponentConfig{
					Type: "controller",
					ID:   "ctrl-001",
				},
				Policy: PolicyConfig{
					Engine: "external",
				},
			},
			wantErr: true,
			errMsg:  "policy.endpoint is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := loader.Validate(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.errMsg != "" {
				if !contains(err.Error(), tt.errMsg) {
					t.Errorf("Expected error containing '%s', got '%s'", tt.errMsg, err.Error())
				}
			}
		})
	}
}

func TestLoader_SetDefaults(t *testing.T) {
	loader := NewLoader()
	config := &Config{
		Component: ComponentConfig{
			Type: "controller",
			ID:   "ctrl-001",
		},
	}

	loader.setDefaults(config)

	// Check defaults
	if config.Component.Version != "v1.0.0" {
		t.Errorf("Expected default version v1.0.0, got %s", config.Component.Version)
	}
	if config.Auth.TokenTTL != 3600*time.Second {
		t.Errorf("Expected default token_ttl 3600s, got %v", config.Auth.TokenTTL)
	}
	if config.Logging.Level != "info" {
		t.Errorf("Expected default logging level info, got %s", config.Logging.Level)
	}
	if config.Logging.Format != "json" {
		t.Errorf("Expected default logging format json, got %s", config.Logging.Format)
	}
	if config.Policy.Engine != "embedded" {
		t.Errorf("Expected default policy engine embedded, got %s", config.Policy.Engine)
	}
	if config.Transport.HTTPAddr != ":8080" {
		t.Errorf("Expected default http_addr :8080, got %s", config.Transport.HTTPAddr)
	}
}

func TestLoader_UnsupportedFormat(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test.txt")

	if err := os.WriteFile(configPath, []byte("invalid"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	loader := NewLoader()
	_, err := loader.Load(configPath)
	if err == nil {
		t.Error("Expected error for unsupported format")
	}
	if !contains(err.Error(), "unsupported config format") {
		t.Errorf("Expected 'unsupported config format' error, got: %v", err)
	}
}

func TestLoader_FileNotFound(t *testing.T) {
	loader := NewLoader()
	_, err := loader.Load("/nonexistent/config.yaml")
	if err == nil {
		t.Error("Expected error for nonexistent file")
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) &&
		(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
			containsSubstring(s, substr)))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
