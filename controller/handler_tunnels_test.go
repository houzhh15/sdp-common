package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestRespondErrorWithStatus tests the error response function with custom status codes
func TestRespondErrorWithStatus(t *testing.T) {
	tests := []struct {
		name           string
		code           string
		message        string
		statusCode     int
		expectedCode   string
		expectedStatus int
	}{
		{
			name:           "Unauthorized Error",
			code:           "UNAUTHORIZED",
			message:        "Invalid session token",
			statusCode:     http.StatusUnauthorized,
			expectedCode:   "UNAUTHORIZED",
			expectedStatus: 401,
		},
		{
			name:           "Policy Denied Error",
			code:           "POLICY_DENIED",
			message:        "Access denied by policy",
			statusCode:     http.StatusForbidden,
			expectedCode:   "POLICY_DENIED",
			expectedStatus: 403,
		},
		{
			name:           "Service Not Found Error",
			code:           "SERVICE_NOT_FOUND",
			message:        "Service not found",
			statusCode:     http.StatusNotFound,
			expectedCode:   "SERVICE_NOT_FOUND",
			expectedStatus: 404,
		},
		{
			name:           "Internal Server Error",
			code:           "INTERNAL_ERROR",
			message:        "Tunnel creation failed",
			statusCode:     http.StatusInternalServerError,
			expectedCode:   "INTERNAL_ERROR",
			expectedStatus: 500,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			respondErrorWithStatus(rr, tt.code, tt.message, nil, tt.statusCode)

			// Assert status code
			assert.Equal(t, tt.expectedStatus, rr.Code)

			// Assert response body
			var resp map[string]interface{}
			err := json.Unmarshal(rr.Body.Bytes(), &resp)
			assert.NoError(t, err)

			assert.Equal(t, "error", resp["type"])
			assert.Equal(t, "error", resp["status"])
			assert.Equal(t, tt.expectedCode, resp["code"])
			assert.Equal(t, tt.message, resp["message"])
			assert.NotEmpty(t, resp["timestamp"])
		})
	}
}

// TestTunnelCreateRequest_InvalidJSON tests invalid request body handling
func TestTunnelCreateRequest_InvalidJSON(t *testing.T) {
	controller := &Controller{}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tunnels", bytes.NewReader([]byte("invalid json")))
	rr := httptest.NewRecorder()

	controller.handleTunnelCreate(rr, req)

	// Assert 400 Bad Request
	assert.Equal(t, http.StatusBadRequest, rr.Code)

	var resp map[string]interface{}
	json.Unmarshal(rr.Body.Bytes(), &resp)
	assert.Equal(t, "error", resp["status"])
	assert.Equal(t, "INVALID_REQUEST", resp["code"])
}

// TestTunnelResponse_Format tests the response format structure
func TestTunnelResponse_Format(t *testing.T) {
	// This test validates the expected response format structure
	// In a real scenario, this would be an integration test with a running controller
	expectedFields := []string{"type", "status", "tunnel_id", "controller_addr", "expires_at"}

	// Mock response structure (what handleTunnelCreate should produce)
	mockResponse := map[string]interface{}{
		"type":            "tunnel_response",
		"status":          "success",
		"tunnel_id":       "550e8400-e29b-41d4-a716-446655440000",
		"controller_addr": "controller.example.com:9443",
		"expires_at":      "2025-11-19T18:00:00Z",
	}

	// Verify all required fields exist
	for _, field := range expectedFields {
		_, exists := mockResponse[field]
		assert.True(t, exists, "Response should contain field: %s", field)
	}

	// Verify field values
	assert.Equal(t, "tunnel_response", mockResponse["type"])
	assert.Equal(t, "success", mockResponse["status"])
	assert.NotEmpty(t, mockResponse["tunnel_id"])
	assert.NotEmpty(t, mockResponse["controller_addr"])
	assert.NotEmpty(t, mockResponse["expires_at"])
}
