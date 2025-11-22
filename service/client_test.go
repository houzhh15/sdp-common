package service

import (
	"crypto/tls"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewClient(t *testing.T) {
	config := &Config{
		ControllerURL: "https://localhost:8443",
		TLSConfig:     &tls.Config{},
		AgentID:       "agent-123",
	}

	client := NewClient(config)
	assert.NotNil(t, client)
	assert.Equal(t, config.ControllerURL, client.controllerURL)
	assert.Equal(t, config.AgentID, client.agentID)
	assert.NotNil(t, client.httpClient)
	assert.NotNil(t, client.services)
	assert.NotNil(t, client.stopChan)
}

func TestConfigDefaults(t *testing.T) {
	config := &Config{
		ControllerURL: "https://localhost:8443",
		TLSConfig:     &tls.Config{},
		AgentID:       "agent-123",
	}

	client := NewClient(config)
	assert.Equal(t, 10*time.Second, client.httpClient.Timeout)
}

func TestGetServices(t *testing.T) {
	config := &Config{
		ControllerURL: "https://localhost:8443",
		TLSConfig:     &tls.Config{},
		AgentID:       "agent-123",
	}

	client := NewClient(config)

	// Initially empty
	services := client.GetServices()
	assert.Empty(t, services)

	// Add services
	client.mu.Lock()
	client.services["svc-1"] = &Service{
		ID:         "svc-1",
		Name:       "Service 1",
		TargetHost: "localhost",
		TargetPort: 8080,
	}
	client.services["svc-2"] = &Service{
		ID:         "svc-2",
		Name:       "Service 2",
		TargetHost: "localhost",
		TargetPort: 8081,
	}
	client.mu.Unlock()

	services = client.GetServices()
	assert.Len(t, services, 2)
}

func TestGetService(t *testing.T) {
	config := &Config{
		ControllerURL: "https://localhost:8443",
		TLSConfig:     &tls.Config{},
		AgentID:       "agent-123",
	}

	client := NewClient(config)

	// Add a service
	client.mu.Lock()
	client.services["svc-1"] = &Service{
		ID:         "svc-1",
		Name:       "Service 1",
		TargetHost: "localhost",
		TargetPort: 8080,
		Protocol:   "http",
	}
	client.mu.Unlock()

	// Get existing service
	svc, ok := client.GetService("svc-1")
	assert.True(t, ok)
	assert.NotNil(t, svc)
	assert.Equal(t, "svc-1", svc.ID)
	assert.Equal(t, "Service 1", svc.Name)
	assert.Equal(t, 8080, svc.TargetPort)

	// Get non-existing service
	svc, ok = client.GetService("svc-999")
	assert.False(t, ok)
	assert.Nil(t, svc)
}

func TestService(t *testing.T) {
	svc := Service{
		ID:         "svc-1",
		Name:       "Test Service",
		TargetHost: "localhost",
		TargetPort: 8080,
		Protocol:   "http",
		Status:     "active",
		Metadata:   map[string]string{"env": "test"},
	}

	assert.Equal(t, "svc-1", svc.ID)
	assert.Equal(t, "Test Service", svc.Name)
	assert.Equal(t, "localhost", svc.TargetHost)
	assert.Equal(t, 8080, svc.TargetPort)
	assert.Equal(t, "http", svc.Protocol)
	assert.Equal(t, "active", svc.Status)
	assert.Equal(t, "test", svc.Metadata["env"])
}

func TestRegisterRequest(t *testing.T) {
	req := RegisterRequest{
		AgentID: "agent-123",
		Services: []Service{
			{
				ID:         "svc-1",
				Name:       "Service 1",
				TargetHost: "localhost",
				TargetPort: 8080,
			},
		},
	}

	assert.Equal(t, "agent-123", req.AgentID)
	assert.Len(t, req.Services, 1)
	assert.Equal(t, "svc-1", req.Services[0].ID)
}

func TestHeartbeatRequest(t *testing.T) {
	req := HeartbeatRequest{
		AgentID:    "agent-123",
		ServiceIDs: []string{"svc-1", "svc-2"},
		Timestamp:  time.Now().Format(time.RFC3339),
	}

	assert.Equal(t, "agent-123", req.AgentID)
	assert.Len(t, req.ServiceIDs, 2)
	assert.NotEmpty(t, req.Timestamp)
}

func TestStop(t *testing.T) {
	config := &Config{
		ControllerURL: "https://localhost:8443",
		TLSConfig:     &tls.Config{},
		AgentID:       "agent-123",
	}

	client := NewClient(config)

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
