// Package service provides standardized SDP service registration client
package service

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// Client handles service registration with Controller
// This is the standard implementation for AH agents
type Client struct {
	httpClient    *http.Client
	controllerURL string
	agentID       string

	mu       sync.RWMutex
	services map[string]*Service
	stopChan chan struct{}
}

// Service represents a service configuration
type Service struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	TargetHost string            `json:"target_host"`
	TargetPort int               `json:"target_port"`
	Protocol   string            `json:"protocol"`
	Status     string            `json:"status,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

// RegisterRequest is the request body for service registration
type RegisterRequest struct {
	AgentID  string    `json:"agent_id"`
	Services []Service `json:"services"`
}

// RegisterResponse is the response from service registration
type RegisterResponse struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

// FetchResponse is the response from fetching services
type FetchResponse struct {
	Status   string    `json:"status"`
	Services []Service `json:"services"`
	Count    int       `json:"count"`
}

// HeartbeatRequest is the request body for service heartbeat
type HeartbeatRequest struct {
	AgentID    string   `json:"agent_id"`
	ServiceIDs []string `json:"service_ids"`
	Timestamp  string   `json:"timestamp"`
}

// HeartbeatResponse is the response from service heartbeat
type HeartbeatResponse struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

// Config contains configuration for service client
type Config struct {
	ControllerURL string        // Controller API base URL
	TLSConfig     *tls.Config   // TLS configuration for mTLS
	AgentID       string        // Agent identifier
	Timeout       time.Duration // HTTP timeout (default: 10s)
}

// NewClient creates a new service registration client
func NewClient(config *Config) *Client {
	if config.Timeout == 0 {
		config.Timeout = 10 * time.Second
	}

	return &Client{
		httpClient: &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: config.TLSConfig,
			},
			Timeout: config.Timeout,
		},
		controllerURL: config.ControllerURL,
		agentID:       config.AgentID,
		services:      make(map[string]*Service),
		stopChan:      make(chan struct{}),
	}
}

// Register registers one or more services with Controller
func (c *Client) Register(ctx context.Context, services []Service) error {
	reqBody := RegisterRequest{
		AgentID:  c.agentID,
		Services: services,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	url := c.controllerURL + "/api/v1/services/register"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("register failed (status %d): %s", resp.StatusCode, string(body))
	}

	var registerResp RegisterResponse
	if err := json.Unmarshal(body, &registerResp); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}

	// Cache registered services
	c.mu.Lock()
	for _, svc := range services {
		c.services[svc.ID] = &svc
	}
	c.mu.Unlock()

	return nil
}

// Fetch fetches the list of services from Controller
func (c *Client) Fetch(ctx context.Context) ([]Service, error) {
	url := c.controllerURL + "/api/v1/services"

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch failed (status %d): %s", resp.StatusCode, string(body))
	}

	var fetchResp FetchResponse
	if err := json.Unmarshal(body, &fetchResp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	// Update cache
	c.mu.Lock()
	c.services = make(map[string]*Service)
	for i := range fetchResp.Services {
		svc := &fetchResp.Services[i]
		c.services[svc.ID] = svc
	}
	c.mu.Unlock()

	return fetchResp.Services, nil
}

// Unregister unregisters a service from Controller
func (c *Client) Unregister(ctx context.Context, serviceID string) error {
	url := fmt.Sprintf("%s/api/v1/services/%s", c.controllerURL, serviceID)

	req, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unregister failed (status %d): %s", resp.StatusCode, string(body))
	}

	// Remove from cache
	c.mu.Lock()
	delete(c.services, serviceID)
	c.mu.Unlock()

	return nil
}

// Heartbeat sends heartbeat for active services
func (c *Client) Heartbeat(ctx context.Context, serviceIDs []string) error {
	reqBody := HeartbeatRequest{
		AgentID:    c.agentID,
		ServiceIDs: serviceIDs,
		Timestamp:  time.Now().Format(time.RFC3339),
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	url := c.controllerURL + "/api/v1/services/heartbeat"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("heartbeat failed (status %d): %s", resp.StatusCode, string(body))
	}

	return nil
}

// ReportFailure reports a service request failure to Controller
func (c *Client) ReportFailure(ctx context.Context, serviceID, reason string) error {
	reqBody := map[string]interface{}{
		"service_id": serviceID,
		"reason":     reason,
		"timestamp":  time.Now().Format(time.RFC3339),
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/api/v1/services/%s/failure", c.controllerURL, serviceID)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("report failure failed (status %d): %s", resp.StatusCode, string(body))
	}

	return nil
}

// GetServices returns a copy of cached services
func (c *Client) GetServices() []Service {
	c.mu.RLock()
	defer c.mu.RUnlock()

	services := make([]Service, 0, len(c.services))
	for _, svc := range c.services {
		services = append(services, *svc)
	}
	return services
}

// GetService returns a specific service by ID
func (c *Client) GetService(serviceID string) (*Service, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	svc, ok := c.services[serviceID]
	if !ok {
		return nil, false
	}

	// Return a copy
	copy := *svc
	return &copy, true
}

// Stop stops the client and cleans up resources
func (c *Client) Stop() {
	close(c.stopChan)
}
