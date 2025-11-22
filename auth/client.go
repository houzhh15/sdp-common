// Package auth provides standardized SDP authentication clientpackage auth

package auth

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

// Client handles SDP authentication with Controller
// This is the standard implementation for IH and AH clients
type Client struct {
	httpClient      *http.Client
	controllerURL   string
	certFingerprint string

	mu           sync.RWMutex
	token        string
	expiresAt    time.Time
	refreshTimer *time.Timer
	stopChan     chan struct{}
}

// DeviceInfo contains device information for authentication
type DeviceInfo struct {
	DeviceID   string            `json:"device_id"`
	OS         string            `json:"os"`
	OSVersion  string            `json:"os_version"`
	Hostname   string            `json:"hostname,omitempty"`
	Compliance bool              `json:"compliance"`
	Attributes map[string]string `json:"attributes,omitempty"`
}

// HandshakeRequest is the request body for authentication
type HandshakeRequest struct {
	CertFingerprint string     `json:"cert_fingerprint"`
	DeviceInfo      DeviceInfo `json:"device_info"`
	Username        string     `json:"username,omitempty"`
	Password        string     `json:"password,omitempty"`
}

// HandshakeResponse is the response from authentication
type HandshakeResponse struct {
	Token     string                 `json:"token"`
	ExpiresAt time.Time              `json:"expires_at"`
	Message   string                 `json:"message,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// RefreshResponse is the response from token refresh
type RefreshResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Config contains configuration for auth client
type Config struct {
	ControllerURL   string        // Controller API base URL (e.g., https://controller:8443)
	TLSConfig       *tls.Config   // TLS configuration for mTLS
	CertFingerprint string        // Client certificate fingerprint
	Timeout         time.Duration // HTTP timeout (default: 30s)
	RetryAttempts   int           // Retry attempts for handshake (default: 3)
	RetryInterval   time.Duration // Interval between retries (default: 5s)
	RefreshBefore   time.Duration // Refresh token before expiry (default: 5min)
}

// NewClient creates a new authentication client
func NewClient(config *Config) *Client {
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}
	if config.RetryAttempts == 0 {
		config.RetryAttempts = 3
	}
	if config.RetryInterval == 0 {
		config.RetryInterval = 5 * time.Second
	}
	if config.RefreshBefore == 0 {
		config.RefreshBefore = 5 * time.Minute
	}

	return &Client{
		httpClient: &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: config.TLSConfig,
			},
			Timeout: config.Timeout,
		},
		controllerURL:   config.ControllerURL,
		certFingerprint: config.CertFingerprint,
		stopChan:        make(chan struct{}),
	}
}

// Handshake performs initial authentication with Controller
// Implements automatic retry with exponential backoff
func (c *Client) Handshake(ctx context.Context, deviceInfo DeviceInfo, username, password string) (*HandshakeResponse, error) {
	reqBody := HandshakeRequest{
		CertFingerprint: c.certFingerprint,
		DeviceInfo:      deviceInfo,
		Username:        username,
		Password:        password,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	// Retry logic with exponential backoff
	var lastErr error
	retryInterval := 5 * time.Second

	for attempt := 1; attempt <= 3; attempt++ {
		resp, err := c.doHandshake(ctx, bodyBytes)
		if err == nil {
			// Success - store token and start auto-refresh
			c.mu.Lock()
			c.token = resp.Token
			c.expiresAt = resp.ExpiresAt
			c.mu.Unlock()

			c.startAutoRefresh()
			return resp, nil
		}

		lastErr = err
		if attempt < 3 {
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("handshake cancelled: %w", ctx.Err())
			case <-time.After(retryInterval):
				retryInterval *= 2 // Exponential backoff
			}
		}
	}

	return nil, fmt.Errorf("handshake failed after 3 attempts: %w", lastErr)
}

// doHandshake performs a single handshake attempt
func (c *Client) doHandshake(ctx context.Context, bodyBytes []byte) (*HandshakeResponse, error) {
	url := c.controllerURL + "/api/v1/auth/handshake"

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

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
		return nil, fmt.Errorf("handshake failed (status %d): %s", resp.StatusCode, string(body))
	}

	var handshakeResp HandshakeResponse
	if err := json.Unmarshal(body, &handshakeResp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return &handshakeResp, nil
}

// Refresh refreshes the authentication token
func (c *Client) Refresh(ctx context.Context) (*RefreshResponse, error) {
	c.mu.RLock()
	oldToken := c.token
	c.mu.RUnlock()

	if oldToken == "" {
		return nil, fmt.Errorf("no token to refresh")
	}

	url := c.controllerURL + "/api/v1/auth/refresh"

	req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+oldToken)

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
		return nil, fmt.Errorf("refresh failed (status %d): %s", resp.StatusCode, string(body))
	}

	var refreshResp RefreshResponse
	if err := json.Unmarshal(body, &refreshResp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	// Update token
	c.mu.Lock()
	c.token = refreshResp.Token
	c.expiresAt = refreshResp.ExpiresAt
	c.mu.Unlock()

	return &refreshResp, nil
}

// Revoke revokes the current token
func (c *Client) Revoke(ctx context.Context) error {
	c.mu.RLock()
	token := c.token
	c.mu.RUnlock()

	if token == "" {
		return nil // Nothing to revoke
	}

	url := c.controllerURL + "/api/v1/auth/revoke"

	req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("revoke failed (status %d): %s", resp.StatusCode, string(body))
	}

	// Clear token
	c.mu.Lock()
	c.token = ""
	c.expiresAt = time.Time{}
	c.mu.Unlock()

	return nil
}

// GetToken returns the current token
func (c *Client) GetToken() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.token
}

// GetExpiresAt returns when the current token expires
func (c *Client) GetExpiresAt() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.expiresAt
}

// IsValid checks if the current token is still valid
func (c *Client) IsValid() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.token == "" {
		return false
	}
	return time.Now().Before(c.expiresAt)
}

// startAutoRefresh starts automatic token refresh
func (c *Client) startAutoRefresh() {
	c.mu.Lock()
	expiresAt := c.expiresAt

	// Calculate refresh time (5 minutes before expiry)
	refreshAt := expiresAt.Add(-5 * time.Minute)
	duration := time.Until(refreshAt)

	// If already expired or will expire very soon, refresh immediately
	if duration < 0 {
		duration = 0
	}

	// Stop existing timer if any
	if c.refreshTimer != nil {
		c.refreshTimer.Stop()
	}

	c.refreshTimer = time.AfterFunc(duration, func() {
		// Check if stopped
		select {
		case <-c.stopChan:
			return
		default:
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if _, err := c.Refresh(ctx); err != nil {
			// Retry after 1 minute
			c.scheduleRetryRefresh(1 * time.Minute)
		} else {
			// Schedule next refresh
			c.startAutoRefresh()
		}
	})
	c.mu.Unlock()
}

// scheduleRetryRefresh schedules a retry for token refresh with exponential backoff
func (c *Client) scheduleRetryRefresh(after time.Duration) {
	c.mu.Lock()
	if c.refreshTimer != nil {
		c.refreshTimer.Stop()
	}

	c.refreshTimer = time.AfterFunc(after, func() {
		// Check if stopped
		select {
		case <-c.stopChan:
			return
		default:
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if _, err := c.Refresh(ctx); err != nil {
			// Continue retrying with exponential backoff (max 5 minutes)
			nextRetry := after * 2
			if nextRetry > 5*time.Minute {
				nextRetry = 5 * time.Minute
			}
			c.scheduleRetryRefresh(nextRetry)
		} else {
			// Success - schedule normal refresh
			c.startAutoRefresh()
		}
	})
	c.mu.Unlock()
}

// Stop stops the auto-refresh timer and cleans up resources
func (c *Client) Stop() {
	c.mu.Lock()
	if c.refreshTimer != nil {
		c.refreshTimer.Stop()
	}
	c.mu.Unlock()

	close(c.stopChan)
}
