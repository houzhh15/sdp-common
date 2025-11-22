package auth

import (
	"context"
	"crypto/tls"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewClient(t *testing.T) {
	config := &Config{
		ControllerURL:   "https://localhost:8443",
		TLSConfig:       &tls.Config{},
		CertFingerprint: "test-fingerprint",
	}

	client := NewClient(config)
	assert.NotNil(t, client)
	assert.Equal(t, config.ControllerURL, client.controllerURL)
	assert.Equal(t, config.CertFingerprint, client.certFingerprint)
	assert.NotNil(t, client.httpClient)
	assert.NotNil(t, client.stopChan)
}

func TestConfigDefaults(t *testing.T) {
	config := &Config{
		ControllerURL:   "https://localhost:8443",
		TLSConfig:       &tls.Config{},
		CertFingerprint: "test-fingerprint",
	}

	client := NewClient(config)

	assert.Equal(t, 30*time.Second, client.httpClient.Timeout)
}

func TestGetToken(t *testing.T) {
	config := &Config{
		ControllerURL:   "https://localhost:8443",
		TLSConfig:       &tls.Config{},
		CertFingerprint: "test-fingerprint",
	}

	client := NewClient(config)

	// Initially empty
	assert.Empty(t, client.GetToken())

	// Set token
	client.mu.Lock()
	client.token = "test-token"
	client.mu.Unlock()

	assert.Equal(t, "test-token", client.GetToken())
}

func TestIsValid(t *testing.T) {
	config := &Config{
		ControllerURL:   "https://localhost:8443",
		TLSConfig:       &tls.Config{},
		CertFingerprint: "test-fingerprint",
	}

	client := NewClient(config)

	// No token - invalid
	assert.False(t, client.IsValid())

	// Token with future expiry - valid
	client.mu.Lock()
	client.token = "test-token"
	client.expiresAt = time.Now().Add(10 * time.Minute)
	client.mu.Unlock()

	assert.True(t, client.IsValid())

	// Token with past expiry - invalid
	client.mu.Lock()
	client.expiresAt = time.Now().Add(-1 * time.Minute)
	client.mu.Unlock()

	assert.False(t, client.IsValid())
}

func TestStop(t *testing.T) {
	config := &Config{
		ControllerURL:   "https://localhost:8443",
		TLSConfig:       &tls.Config{},
		CertFingerprint: "test-fingerprint",
	}

	client := NewClient(config)

	// Start a timer
	client.mu.Lock()
	client.refreshTimer = time.AfterFunc(1*time.Hour, func() {})
	client.mu.Unlock()

	// Stop should not panic
	client.Stop()

	// Verify stopChan is closed
	select {
	case <-client.stopChan:
		// Expected
	default:
		t.Error("stopChan should be closed")
	}
}

func TestHandshakeRequest(t *testing.T) {
	deviceInfo := DeviceInfo{
		DeviceID:   "device-123",
		OS:         "linux",
		OSVersion:  "5.10",
		Hostname:   "test-host",
		Compliance: true,
		Attributes: map[string]string{"key": "value"},
	}

	req := HandshakeRequest{
		CertFingerprint: "fingerprint",
		DeviceInfo:      deviceInfo,
		Username:        "user",
		Password:        "pass",
	}

	assert.Equal(t, "fingerprint", req.CertFingerprint)
	assert.Equal(t, "device-123", req.DeviceInfo.DeviceID)
	assert.Equal(t, "user", req.Username)
}

func TestRevoke_NoToken(t *testing.T) {
	config := &Config{
		ControllerURL:   "https://localhost:8443",
		TLSConfig:       &tls.Config{InsecureSkipVerify: true},
		CertFingerprint: "test-fingerprint",
	}

	client := NewClient(config)
	ctx := context.Background()

	// Revoking with no token should not error
	err := client.Revoke(ctx)
	assert.NoError(t, err)
}
