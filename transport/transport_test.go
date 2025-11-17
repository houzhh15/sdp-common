package transport

import (
	"crypto/tls"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewEvent(t *testing.T) {
	data := map[string]string{"key": "value"}
	event := NewEvent("test_event", data)

	if event.Type != "test_event" {
		t.Errorf("Type = %q, want %q", event.Type, "test_event")
	}
	if event.Data == nil {
		t.Error("Data should not be nil")
	}
	if event.Timestamp.IsZero() {
		t.Error("Timestamp should not be zero")
	}
	if time.Since(event.Timestamp) > time.Second {
		t.Error("Timestamp should be recent")
	}
}

func TestLoadTLSConfig_Success(t *testing.T) {
	// 跳过测试如果没有测试证书
	certDir := "../../../certs"
	if _, err := os.Stat(certDir); os.IsNotExist(err) {
		t.Skip("Test certificates not found, skipping TLS config test")
	}

	cfg := &TLSConfig{
		CertFile:   filepath.Join(certDir, "controller-cert.pem"),
		KeyFile:    filepath.Join(certDir, "controller-key.pem"),
		CAFile:     filepath.Join(certDir, "ca-cert.pem"),
		MinVersion: tls.VersionTLS12,
	}

	tlsConfig, err := LoadTLSConfig(cfg)
	if err != nil {
		t.Fatalf("LoadTLSConfig() error = %v", err)
	}

	if tlsConfig == nil {
		t.Fatal("tlsConfig should not be nil")
	}

	if tlsConfig.MinVersion != tls.VersionTLS12 {
		t.Errorf("MinVersion = %d, want %d", tlsConfig.MinVersion, tls.VersionTLS12)
	}

	if tlsConfig.ClientAuth != tls.RequireAndVerifyClientCert {
		t.Error("ClientAuth should be RequireAndVerifyClientCert (mTLS)")
	}

	if len(tlsConfig.Certificates) != 1 {
		t.Error("Should have exactly one certificate")
	}
}

func TestLoadTLSConfig_InvalidCertFile(t *testing.T) {
	cfg := &TLSConfig{
		CertFile:   "nonexistent-cert.pem",
		KeyFile:    "nonexistent-key.pem",
		CAFile:     "nonexistent-ca.pem",
		MinVersion: tls.VersionTLS12,
	}

	_, err := LoadTLSConfig(cfg)
	if err == nil {
		t.Error("LoadTLSConfig() should return error for nonexistent files")
	}
}

func TestLoadTLSConfig_DefaultMinVersion(t *testing.T) {
	// 跳过测试如果没有测试证书
	certDir := "../../../certs"
	if _, err := os.Stat(certDir); os.IsNotExist(err) {
		t.Skip("Test certificates not found, skipping")
	}

	cfg := &TLSConfig{
		CertFile: filepath.Join(certDir, "controller-cert.pem"),
		KeyFile:  filepath.Join(certDir, "controller-key.pem"),
		CAFile:   filepath.Join(certDir, "ca-cert.pem"),
		// MinVersion 未设置（0）
	}

	tlsConfig, err := LoadTLSConfig(cfg)
	if err != nil {
		t.Fatalf("LoadTLSConfig() error = %v", err)
	}

	if tlsConfig.MinVersion != tls.VersionTLS12 {
		t.Errorf("Default MinVersion = %d, want %d (TLS 1.2)", tlsConfig.MinVersion, tls.VersionTLS12)
	}
}
