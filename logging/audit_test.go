package logging

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestNewFileAuditLogger(t *testing.T) {
	tmpFile := "/tmp/test_audit.log"
	defer os.Remove(tmpFile)

	logger, err := NewLogger(&Config{
		Level:  "info",
		Format: "json",
		Output: "stdout",
	})
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	auditLogger, err := NewFileAuditLogger(tmpFile, logger)
	if err != nil {
		t.Fatalf("Failed to create audit logger: %v", err)
	}
	defer auditLogger.Close()

	if auditLogger.outputPath != tmpFile {
		t.Errorf("Expected outputPath %s, got %s", tmpFile, auditLogger.outputPath)
	}
}

func TestFileAuditLogger_LogAccess(t *testing.T) {
	tmpFile := "/tmp/test_audit_access.log"
	defer os.Remove(tmpFile)

	logger, _ := NewLogger(&Config{Level: "info", Format: "json", Output: "stdout"})
	auditLogger, err := NewFileAuditLogger(tmpFile, logger)
	if err != nil {
		t.Fatalf("Failed to create audit logger: %v", err)
	}
	defer auditLogger.Close()

	event := &AccessEvent{
		Timestamp: time.Now(),
		ClientID:  "client-001",
		ServiceID: "service-001",
		SourceIP:  "192.168.1.100",
		Action:    "handshake",
		Result:    "success",
		Reason:    "authenticated",
		Details:   map[string]interface{}{"method": "mTLS"},
	}

	err = auditLogger.LogAccess(context.Background(), event)
	if err != nil {
		t.Errorf("LogAccess() failed: %v", err)
	}

	// 验证文件写入
	stat, err := os.Stat(tmpFile)
	if err != nil {
		t.Errorf("Failed to stat log file: %v", err)
	}
	if stat.Size() == 0 {
		t.Error("Expected log file to have content")
	}
}

func TestFileAuditLogger_LogConnection(t *testing.T) {
	tmpFile := "/tmp/test_audit_conn.log"
	defer os.Remove(tmpFile)

	logger, _ := NewLogger(&Config{Level: "info", Format: "json", Output: "stdout"})
	auditLogger, err := NewFileAuditLogger(tmpFile, logger)
	if err != nil {
		t.Fatalf("Failed to create audit logger: %v", err)
	}
	defer auditLogger.Close()

	event := &ConnectionEvent{
		Timestamp:  time.Now(),
		TunnelID:   "tunnel-123",
		ClientID:   "client-001",
		ServiceID:  "service-001",
		IHEndpoint: "192.168.1.100:12345",
		AHEndpoint: "10.0.0.1:8080",
		Action:     "open",
		Duration:   5 * time.Second,
		BytesSent:  1024,
		BytesRecv:  2048,
	}

	err = auditLogger.LogConnection(context.Background(), event)
	if err != nil {
		t.Errorf("LogConnection() failed: %v", err)
	}

	// 验证内存缓存
	if len(auditLogger.logs) == 0 {
		t.Error("Expected logs in memory cache")
	}
}

func TestFileAuditLogger_LogSecurity(t *testing.T) {
	tmpFile := "/tmp/test_audit_security.log"
	defer os.Remove(tmpFile)

	logger, _ := NewLogger(&Config{Level: "info", Format: "json", Output: "stdout"})
	auditLogger, err := NewFileAuditLogger(tmpFile, logger)
	if err != nil {
		t.Fatalf("Failed to create audit logger: %v", err)
	}
	defer auditLogger.Close()

	tests := []struct {
		name  string
		event *SecurityEvent
	}{
		{
			name: "cert invalid",
			event: &SecurityEvent{
				Timestamp: time.Now(),
				ClientID:  "client-001",
				EventType: EventCertInvalid,
				Severity:  SeverityHigh,
				Message:   "Certificate validation failed",
				Details:   map[string]interface{}{"reason": "expired"},
			},
		},
		{
			name: "session expired",
			event: &SecurityEvent{
				Timestamp: time.Now(),
				ClientID:  "client-002",
				EventType: EventSessionExpired,
				Severity:  SeverityMedium,
				Message:   "Session has expired",
			},
		},
		{
			name: "device noncompliant",
			event: &SecurityEvent{
				Timestamp: time.Now(),
				ClientID:  "client-003",
				EventType: EventDeviceNoncompliant,
				Severity:  SeverityCritical,
				Message:   "Device does not meet security policy",
				Details:   map[string]interface{}{"missing": "antivirus"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := auditLogger.LogSecurity(context.Background(), tt.event)
			if err != nil {
				t.Errorf("LogSecurity() failed: %v", err)
			}
		})
	}

	if len(auditLogger.logs) != len(tests) {
		t.Errorf("Expected %d logs, got %d", len(tests), len(auditLogger.logs))
	}
}

func TestFileAuditLogger_Query(t *testing.T) {
	tmpFile := "/tmp/test_audit_query.log"
	defer os.Remove(tmpFile)

	logger, _ := NewLogger(&Config{Level: "info", Format: "json", Output: "stdout"})
	auditLogger, err := NewFileAuditLogger(tmpFile, logger)
	if err != nil {
		t.Fatalf("Failed to create audit logger: %v", err)
	}
	defer auditLogger.Close()

	// 创建测试数据
	events := []*AccessEvent{
		{
			ClientID:  "client-001",
			ServiceID: "service-001",
			Action:    "handshake",
			Result:    "success",
		},
		{
			ClientID:  "client-001",
			ServiceID: "service-002",
			Action:    "policy_query",
			Result:    "success",
		},
		{
			ClientID:  "client-002",
			ServiceID: "service-001",
			Action:    "handshake",
			Result:    "denied",
		},
	}

	for _, event := range events {
		auditLogger.LogAccess(context.Background(), event)
	}

	// 测试查询
	tests := []struct {
		name      string
		filter    *AuditFilter
		wantCount int
	}{
		{
			name:      "query all",
			filter:    &AuditFilter{},
			wantCount: 3,
		},
		{
			name: "query by client",
			filter: &AuditFilter{
				ClientID: "client-001",
			},
			wantCount: 2,
		},
		{
			name: "query by service",
			filter: &AuditFilter{
				ServiceID: "service-001",
			},
			wantCount: 2,
		},
		{
			name: "query by action",
			filter: &AuditFilter{
				Action: "handshake",
			},
			wantCount: 2,
		},
		{
			name: "query by result",
			filter: &AuditFilter{
				Result: "denied",
			},
			wantCount: 1,
		},
		{
			name: "query with limit",
			filter: &AuditFilter{
				Limit: 2,
			},
			wantCount: 2,
		},
		{
			name: "query with offset",
			filter: &AuditFilter{
				Offset: 2,
			},
			wantCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := auditLogger.Query(context.Background(), tt.filter)
			if err != nil {
				t.Errorf("Query() failed: %v", err)
			}
			if len(results) != tt.wantCount {
				t.Errorf("Expected %d results, got %d", tt.wantCount, len(results))
			}
		})
	}
}

func TestFileAuditLogger_NilEvents(t *testing.T) {
	tmpFile := "/tmp/test_audit_nil.log"
	defer os.Remove(tmpFile)

	logger, _ := NewLogger(&Config{Level: "info", Format: "json", Output: "stdout"})
	auditLogger, err := NewFileAuditLogger(tmpFile, logger)
	if err != nil {
		t.Fatalf("Failed to create audit logger: %v", err)
	}
	defer auditLogger.Close()

	ctx := context.Background()

	if err := auditLogger.LogAccess(ctx, nil); err == nil {
		t.Error("Expected error for nil AccessEvent")
	}

	if err := auditLogger.LogConnection(ctx, nil); err == nil {
		t.Error("Expected error for nil ConnectionEvent")
	}

	if err := auditLogger.LogSecurity(ctx, nil); err == nil {
		t.Error("Expected error for nil SecurityEvent")
	}
}

func TestFileAuditLogger_TimestampAutoFill(t *testing.T) {
	tmpFile := "/tmp/test_audit_timestamp.log"
	defer os.Remove(tmpFile)

	logger, _ := NewLogger(&Config{Level: "info", Format: "json", Output: "stdout"})
	auditLogger, err := NewFileAuditLogger(tmpFile, logger)
	if err != nil {
		t.Fatalf("Failed to create audit logger: %v", err)
	}
	defer auditLogger.Close()

	event := &AccessEvent{
		ClientID:  "client-001",
		ServiceID: "service-001",
		Action:    "test",
		Result:    "success",
		// Timestamp 未设置
	}

	err = auditLogger.LogAccess(context.Background(), event)
	if err != nil {
		t.Errorf("LogAccess() failed: %v", err)
	}

	// 验证 Timestamp 被自动填充
	if len(auditLogger.logs) == 0 {
		t.Fatal("No logs found")
	}

	log := auditLogger.logs[0]
	if log.Timestamp.IsZero() {
		t.Error("Expected timestamp to be auto-filled")
	}
}

func TestSecurityEventTypes(t *testing.T) {
	// 测试所有安全事件类型常量
	eventTypes := []SecurityEventType{
		EventCertInvalid,
		EventCertExpired,
		EventCertRevoked,
		EventSessionExpired,
		EventSessionRevoked,
		EventDeviceNoncompliant,
		EventUnauthorizedAccess,
		EventPolicyViolation,
		EventAnomalousActivity,
		EventBruteForceAttempt,
	}

	for _, et := range eventTypes {
		if et == "" {
			t.Errorf("Event type should not be empty")
		}
	}
}

func TestSeverityLevels(t *testing.T) {
	// 测试所有严重程度常量
	severities := []Severity{
		SeverityLow,
		SeverityMedium,
		SeverityHigh,
		SeverityCritical,
	}

	for _, s := range severities {
		if s == "" {
			t.Errorf("Severity should not be empty")
		}
	}
}
