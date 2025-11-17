package protocol

import (
	"errors"
	"testing"
)

func TestError_Error(t *testing.T) {
	err := NewError(ErrCodeInvalidCert, "certificate expired")
	expected := "[40101] certificate expired"
	if err.Error() != expected {
		t.Errorf("Error() = %q, want %q", err.Error(), expected)
	}
}

func TestNewError(t *testing.T) {
	err := NewError(ErrCodeSessionExpired, "session timeout")
	if err.Code != ErrCodeSessionExpired {
		t.Errorf("Code = %d, want %d", err.Code, ErrCodeSessionExpired)
	}
	if err.Message != "session timeout" {
		t.Errorf("Message = %q, want %q", err.Message, "session timeout")
	}
	if err.Details == nil {
		t.Error("Details should not be nil")
	}
}

func TestWrapError(t *testing.T) {
	originalErr := errors.New("connection refused")
	err := WrapError(ErrCodeServiceUnavail, originalErr)
	
	if err.Code != ErrCodeServiceUnavail {
		t.Errorf("Code = %d, want %d", err.Code, ErrCodeServiceUnavail)
	}
	if err.Message != originalErr.Error() {
		t.Errorf("Message = %q, want %q", err.Message, originalErr.Error())
	}
}

func TestError_WithDetails(t *testing.T) {
	err := NewError(ErrCodeUnauthorized, "access denied").
		WithDetails("client_id", "ih-001").
		WithDetails("reason", "invalid_token")
	
	if err.Details["client_id"] != "ih-001" {
		t.Error("client_id detail not set correctly")
	}
	if err.Details["reason"] != "invalid_token" {
		t.Error("reason detail not set correctly")
	}
}

func TestErrorCodes(t *testing.T) {
	tests := []struct {
		name string
		code int
	}{
		{"Success", ErrCodeSuccess},
		{"Unauthorized", ErrCodeUnauthorized},
		{"InvalidCert", ErrCodeInvalidCert},
		{"SessionExpired", ErrCodeSessionExpired},
		{"NoPolicy", ErrCodeNoPolicy},
		{"ServiceNotFound", ErrCodeServiceNotFound},
		{"ConcurrencyLimit", ErrCodeConcurrencyLimit},
		{"ServiceUnavail", ErrCodeServiceUnavail},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.code < 0 || (tt.code != 0 && (tt.code < 40000 || tt.code > 60000)) {
				t.Errorf("Invalid error code range: %d", tt.code)
			}
		})
	}
}
