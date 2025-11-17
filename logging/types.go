package logging

import "time"

// AccessEvent 访问事件
// 用于记录客户端对服务的访问请求
type AccessEvent struct {
	Timestamp time.Time              `json:"timestamp"`
	ClientID  string                 `json:"client_id"`
	ServiceID string                 `json:"service_id"`
	SourceIP  string                 `json:"source_ip"`
	Action    string                 `json:"action"` // "handshake", "policy_query", "tunnel_create"
	Result    string                 `json:"result"` // "success", "denied"
	Reason    string                 `json:"reason,omitempty"`
	Details   map[string]interface{} `json:"details,omitempty"`
}

// ConnectionEvent 连接事件
// 用于记录隧道连接的建立、关闭等操作
type ConnectionEvent struct {
	Timestamp  time.Time              `json:"timestamp"`
	TunnelID   string                 `json:"tunnel_id"`
	ClientID   string                 `json:"client_id"`
	ServiceID  string                 `json:"service_id"`
	IHEndpoint string                 `json:"ih_endpoint"` // Initiating Host endpoint
	AHEndpoint string                 `json:"ah_endpoint"` // Accepting Host endpoint
	Action     string                 `json:"action"`      // "open", "close", "error"
	Duration   time.Duration          `json:"duration,omitempty"`
	BytesSent  int64                  `json:"bytes_sent,omitempty"`
	BytesRecv  int64                  `json:"bytes_recv,omitempty"`
	Details    map[string]interface{} `json:"details,omitempty"`
}

// SecurityEvent 安全事件
// 用于记录安全相关的异常和告警
type SecurityEvent struct {
	Timestamp time.Time              `json:"timestamp"`
	ClientID  string                 `json:"client_id"`
	EventType SecurityEventType      `json:"event_type"`
	Severity  Severity               `json:"severity"` // "low", "medium", "high", "critical"
	Message   string                 `json:"message"`
	Details   map[string]interface{} `json:"details,omitempty"`
}

// SecurityEventType 安全事件类型
type SecurityEventType string

const (
	EventCertInvalid        SecurityEventType = "cert_invalid"
	EventCertExpired        SecurityEventType = "cert_expired"
	EventCertRevoked        SecurityEventType = "cert_revoked"
	EventSessionExpired     SecurityEventType = "session_expired"
	EventSessionRevoked     SecurityEventType = "session_revoked"
	EventDeviceNoncompliant SecurityEventType = "device_noncompliant"
	EventUnauthorizedAccess SecurityEventType = "unauthorized_access"
	EventPolicyViolation    SecurityEventType = "policy_violation"
	EventAnomalousActivity  SecurityEventType = "anomalous_activity"
	EventBruteForceAttempt  SecurityEventType = "brute_force_attempt"
)

// Severity 严重程度
type Severity string

const (
	SeverityLow      Severity = "low"
	SeverityMedium   Severity = "medium"
	SeverityHigh     Severity = "high"
	SeverityCritical Severity = "critical"
)

// AuditFilter 审计日志查询过滤器
type AuditFilter struct {
	ClientID  string            `json:"client_id,omitempty"`
	ServiceID string            `json:"service_id,omitempty"`
	Action    string            `json:"action,omitempty"`
	Result    string            `json:"result,omitempty"`
	EventType SecurityEventType `json:"event_type,omitempty"`
	Severity  Severity          `json:"severity,omitempty"`
	StartTime time.Time         `json:"start_time,omitempty"`
	EndTime   time.Time         `json:"end_time,omitempty"`
	Limit     int               `json:"limit,omitempty"`
	Offset    int               `json:"offset,omitempty"`
}

// AuditLog 审计日志记录
// 通用审计日志结构，可以包含任意类型的事件
type AuditLog struct {
	ID        string                 `json:"id"`
	Timestamp time.Time              `json:"timestamp"`
	EventType string                 `json:"event_type"` // "access", "connection", "security"
	Data      interface{}            `json:"data"`
	Indexed   map[string]interface{} `json:"indexed,omitempty"` // 用于快速查询的索引字段
}
