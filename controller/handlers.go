package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/houzhh15/sdp-common/policy"
	"github.com/houzhh15/sdp-common/protocol"
	"github.com/houzhh15/sdp-common/session"
	"github.com/houzhh15/sdp-common/tunnel"
)

// registerHandlers registers all HTTP API handlers
// This implements the complete SDP 2.0 specification REST API
func (c *Controller) registerHandlers() {
	// Health check endpoint
	c.mux.HandleFunc("/health", c.handleHealth)

	// Session management endpoints
	c.mux.HandleFunc("/api/v1/handshake", c.handleHandshake)
	c.mux.HandleFunc("/api/v1/sessions/refresh", c.handleSessionRefresh)
	c.mux.HandleFunc("/api/v1/sessions/", c.handleSessionRevoke)

	// Policy endpoints
	c.mux.HandleFunc("/api/v1/policies", c.handlePolicies)

	// Service configuration endpoints (SDP 2.0 0x04)
	c.mux.HandleFunc("/api/v1/services", c.handleServicesList)
	c.mux.HandleFunc("/api/v1/services/", c.handleServicesGet)

	// Tunnel management endpoints
	c.mux.HandleFunc("/api/v1/tunnels", c.handleTunnels)
	c.mux.HandleFunc("/api/v1/tunnels/", c.handleTunnelDelete)

	// SSE subscription endpoints
	c.mux.HandleFunc("/v1/agent/tunnels/stream", c.handleTunnelEventsSSE)
}

// handleHealth handles health check requests
func (c *Controller) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

// handleHandshake handles client handshake requests
func (c *Controller) handleHandshake(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Extract client certificate
	if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
		respondError(w, "INVALID_CERT", "No client certificate", nil)
		return
	}

	clientCert := r.TLS.PeerCertificates[0]
	fingerprint := calculateFingerprint(clientCert)

	c.logger.Info("Handshake request received", "fingerprint", fingerprint)

	// Validate certificate
	if err := c.certRegistry.Validate(fingerprint); err != nil {
		// If not registered, register it
		clientID := fmt.Sprintf("client-%d", time.Now().Unix())
		if err := c.certRegistry.Register(clientID, fingerprint, clientCert); err != nil {
			c.logger.Error("Failed to register certificate", "error", err)
			respondError(w, "INVALID_CERT", "Certificate registration failed", nil)
			return
		}
	}

	clientID := extractClientID(clientCert)

	// Optional: Evaluate access to a demo service
	_, err := c.policyEngine.EvaluateAccess(ctx, &policy.AccessRequest{
		ClientID:  clientID,
		ServiceID: "demo-service-001",
		Timestamp: time.Now(),
	})
	if err != nil {
		c.logger.Warn("Policy evaluation warning", "client_id", clientID, "error", err)
	}

	// Create session
	sess, err := c.sessionManager.CreateSession(ctx, &session.CreateSessionRequest{
		ClientID:        clientID,
		CertFingerprint: fingerprint,
		Metadata:        map[string]interface{}{"source_ip": r.RemoteAddr},
	})
	if err != nil {
		c.logger.Error("Failed to create session", "error", err)
		respondError(w, "UNAUTHORIZED", "Session creation failed", nil)
		return
	}

	c.logger.Info("Session created", "client_id", sess.ClientID, "token", sess.Token[:16]+"...")

	// Return session token
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"type":          protocol.MsgTypeHandshakeResp,
		"status":        "success",
		"session_token": sess.Token,
		"expires_at":    sess.ExpiresAt.Format(time.RFC3339),
	})
}

// handleSessionRefresh handles session refresh requests
func (c *Controller) handleSessionRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()
	token := extractBearerToken(r)
	if token == "" {
		respondError(w, "ERROR", "Missing authorization token", nil)
		return
	}

	sess, err := c.sessionManager.RefreshSession(ctx, token)
	if err != nil {
		c.logger.Warn("Session refresh failed", "error", err)
		respondError(w, "ERROR", "Session refresh failed", nil)
		return
	}

	c.logger.Info("Session refreshed", "client_id", sess.ClientID)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":        "success",
		"session_token": sess.Token,
		"expires_at":    sess.ExpiresAt.Format(time.RFC3339),
	})
}

// handleSessionRevoke handles session revoke requests
func (c *Controller) handleSessionRevoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()
	token := strings.TrimPrefix(r.URL.Path, "/api/v1/sessions/")
	if token == "" {
		respondError(w, "ERROR", "Missing session token", nil)
		return
	}

	err := c.sessionManager.RevokeSession(ctx, token)
	if err != nil {
		c.logger.Warn("Session revoke failed", "error", err)
		respondError(w, "ERROR", "Session not found", nil)
		return
	}

	c.logger.Info("Session revoked", "token", token[:16]+"...")

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "success",
	})
}

// handlePolicies handles policy query requests
func (c *Controller) handlePolicies(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()
	token := extractBearerToken(r)
	if token == "" {
		respondError(w, "ERROR", "Missing authorization token", nil)
		return
	}

	sess, err := c.sessionManager.ValidateSession(ctx, token)
	if err != nil {
		c.logger.Warn("Session validation failed", "error", err)
		respondError(w, "ERROR", "Invalid or expired session", nil)
		return
	}

	policies, err := c.policyEngine.GetPoliciesForClient(ctx, sess.ClientID)
	if err != nil {
		c.logger.Error("Failed to get policies", "client_id", sess.ClientID, "error", err)
		respondError(w, "ERROR", "Failed to retrieve policies", nil)
		return
	}

	c.logger.Info("Policies retrieved", "client_id", sess.ClientID, "count", len(policies))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"type":     protocol.MsgTypePolicyResp,
		"status":   "success",
		"policies": policies,
	})
}

// handleServicesList handles service configuration list requests
func (c *Controller) handleServicesList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()
	configs, err := c.tunnelManager.ListServiceConfigs(ctx, "")
	if err != nil {
		c.logger.Error("Failed to list service configs", "error", err)
		respondError(w, "ERROR", "Failed to retrieve service configs", nil)
		return
	}

	c.logger.Info("Service configs listed", "count", len(configs))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":   "success",
		"services": configs,
		"count":    len(configs),
	})
}

// handleServicesGet handles single service configuration get requests
func (c *Controller) handleServicesGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()
	serviceID := strings.TrimPrefix(r.URL.Path, "/api/v1/services/")
	if serviceID == "" {
		respondError(w, "ERROR", "Service ID is required", nil)
		return
	}

	config, err := c.tunnelManager.GetServiceConfig(ctx, serviceID)
	if err != nil {
		c.logger.Warn("Service config not found", "service_id", serviceID, "error", err)
		respondError(w, "ERROR", fmt.Sprintf("Service not found: %s", serviceID), nil)
		return
	}

	c.logger.Info("Service config retrieved", "service_id", serviceID)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "success",
		"service": config,
	})
}

// handleTunnels handles tunnel creation and listing
func (c *Controller) handleTunnels(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	switch r.Method {
	case http.MethodPost:
		c.handleTunnelCreate(w, r)
	case http.MethodGet:
		token := extractBearerToken(r)
		if token == "" {
			respondError(w, "ERROR", "Missing authorization token", nil)
			return
		}

		sess, err := c.sessionManager.ValidateSession(ctx, token)
		if err != nil {
			respondError(w, "ERROR", "Invalid or expired session", nil)
			return
		}

		tunnels, err := c.tunnelManager.ListTunnels(ctx, &tunnel.TunnelFilter{ClientID: sess.ClientID})
		if err != nil {
			respondError(w, "ERROR", "Failed to retrieve tunnels", nil)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"type":    "tunnel_list",
			"status":  "success",
			"tunnels": tunnels,
		})
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleTunnelCreate handles tunnel creation requests
func (c *Controller) handleTunnelCreate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req struct {
		SessionToken string `json:"session_token"`
		ServiceID    string `json:"service_id"`
		Protocol     string `json:"protocol"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, "ERROR", "Invalid request body", nil)
		return
	}

	sess, err := c.sessionManager.ValidateSession(ctx, req.SessionToken)
	if err != nil {
		respondError(w, "ERROR", "Invalid or expired session", nil)
		return
	}

	// Evaluate policy
	decision, err := c.policyEngine.EvaluateAccess(ctx, &policy.AccessRequest{
		ClientID:  sess.ClientID,
		ServiceID: req.ServiceID,
		Timestamp: time.Now(),
	})
	if err != nil || !decision.Allowed {
		c.logger.Warn("Access denied", "client_id", sess.ClientID, "service_id", req.ServiceID)
		respondError(w, "POLICY_DENIED", "Access denied by policy", nil)
		return
	}

	// Create tunnel
	tun, err := c.tunnelManager.CreateTunnel(ctx, &tunnel.CreateTunnelRequest{
		SessionToken: req.SessionToken,
		ClientID:     sess.ClientID,
		ServiceID:    req.ServiceID,
		Protocol:     req.Protocol,
	})
	if err != nil {
		c.logger.Error("Failed to create tunnel", "error", err)
		respondError(w, "ERROR", "Tunnel creation failed", nil)
		return
	}

	c.logger.Info("Tunnel created", "tunnel_id", tun.ID, "client_id", sess.ClientID)

	// Notify AH agents
	event := &tunnel.TunnelEvent{
		Type:      tunnel.EventTypeCreated,
		Tunnel:    tun,
		Timestamp: time.Now(),
	}
	c.tunnelNotifier.Notify(event)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"type":      protocol.MsgTypeTunnelResp,
		"status":    "success",
		"tunnel_id": tun.ID,
		"tunnel":    tun,
	})
}

// handleTunnelDelete handles tunnel deletion requests
func (c *Controller) handleTunnelDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()
	tunnelID := strings.TrimPrefix(r.URL.Path, "/api/v1/tunnels/")
	if tunnelID == "" {
		respondError(w, "ERROR", "Missing tunnel ID", nil)
		return
	}

	token := extractBearerToken(r)
	if token == "" {
		respondError(w, "ERROR", "Missing authorization token", nil)
		return
	}

	_, err := c.sessionManager.ValidateSession(ctx, token)
	if err != nil {
		respondError(w, "ERROR", "Invalid or expired session", nil)
		return
	}

	if err := c.tunnelManager.DeleteTunnel(ctx, tunnelID); err != nil {
		c.logger.Error("Failed to delete tunnel", "tunnel_id", tunnelID, "error", err)
		respondError(w, "ERROR", "Tunnel deletion failed", nil)
		return
	}

	c.logger.Info("Tunnel deleted", "tunnel_id", tunnelID)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"type":   "tunnel_delete",
		"status": "success",
	})
}

// handleTunnelEventsSSE handles SSE subscription for tunnel events
func (c *Controller) handleTunnelEventsSSE(w http.ResponseWriter, r *http.Request) {
	agentID := r.URL.Query().Get("agent_id")
	if agentID == "" {
		agentID = "unknown"
	}

	c.logger.Info("SSE connection request", "agent_id", agentID, "client", r.RemoteAddr)

	if err := c.tunnelNotifier.Subscribe(agentID, w); err != nil {
		c.logger.Error("Failed to subscribe", "error", err)
		http.Error(w, "Subscription failed", http.StatusInternalServerError)
		return
	}

	defer c.tunnelNotifier.Unsubscribe(agentID)
}
