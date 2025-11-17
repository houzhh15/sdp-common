package logging

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// AuditLogger 审计日志记录器接口
type AuditLogger interface {
	LogAccess(ctx context.Context, event *AccessEvent) error
	LogConnection(ctx context.Context, event *ConnectionEvent) error
	LogSecurity(ctx context.Context, event *SecurityEvent) error
	Query(ctx context.Context, filter *AuditFilter) ([]*AuditLog, error)
}

// FileAuditLogger 基于文件的审计日志记录器
// 从 controller/internal/logging/audit.go 提取并重构
type FileAuditLogger struct {
	outputPath string
	logger     Logger
	file       *os.File
	mu         sync.Mutex
	logs       []*AuditLog // 内存缓存，用于 Query（生产环境应使用数据库）
}

// NewFileAuditLogger 创建新的文件审计日志记录器
func NewFileAuditLogger(outputPath string, logger Logger) (*FileAuditLogger, error) {
	f, err := os.OpenFile(outputPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("open audit log file: %w", err)
	}

	return &FileAuditLogger{
		outputPath: outputPath,
		logger:     logger,
		file:       f,
		logs:       make([]*AuditLog, 0),
	}, nil
}

// LogAccess 记录访问事件
func (a *FileAuditLogger) LogAccess(ctx context.Context, event *AccessEvent) error {
	if event == nil {
		return fmt.Errorf("access event cannot be nil")
	}

	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	auditLog := &AuditLog{
		ID:        fmt.Sprintf("access_%d", time.Now().UnixNano()),
		Timestamp: event.Timestamp,
		EventType: "access",
		Data:      event,
		Indexed: map[string]interface{}{
			"client_id":  event.ClientID,
			"service_id": event.ServiceID,
			"action":     event.Action,
			"result":     event.Result,
		},
	}

	return a.writeLog(auditLog)
}

// LogConnection 记录连接事件
func (a *FileAuditLogger) LogConnection(ctx context.Context, event *ConnectionEvent) error {
	if event == nil {
		return fmt.Errorf("connection event cannot be nil")
	}

	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	auditLog := &AuditLog{
		ID:        fmt.Sprintf("conn_%d", time.Now().UnixNano()),
		Timestamp: event.Timestamp,
		EventType: "connection",
		Data:      event,
		Indexed: map[string]interface{}{
			"tunnel_id":  event.TunnelID,
			"client_id":  event.ClientID,
			"service_id": event.ServiceID,
			"action":     event.Action,
		},
	}

	return a.writeLog(auditLog)
}

// LogSecurity 记录安全事件
func (a *FileAuditLogger) LogSecurity(ctx context.Context, event *SecurityEvent) error {
	if event == nil {
		return fmt.Errorf("security event cannot be nil")
	}

	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	auditLog := &AuditLog{
		ID:        fmt.Sprintf("sec_%d", time.Now().UnixNano()),
		Timestamp: event.Timestamp,
		EventType: "security",
		Data:      event,
		Indexed: map[string]interface{}{
			"client_id":  event.ClientID,
			"event_type": event.EventType,
			"severity":   event.Severity,
		},
	}

	// 安全事件需要同时记录到结构化日志
	a.logger.Warn("Security Event",
		"event_type", event.EventType,
		"severity", event.Severity,
		"client_id", event.ClientID,
		"message", event.Message,
	)

	return a.writeLog(auditLog)
}

// Query 查询审计日志
// 注意：此实现使用内存缓存，仅适用于开发/测试环境
// 生产环境应使用数据库或专业日志系统
func (a *FileAuditLogger) Query(ctx context.Context, filter *AuditFilter) ([]*AuditLog, error) {
	if filter == nil {
		filter = &AuditFilter{}
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	var results []*AuditLog
	for _, log := range a.logs {
		if a.matchFilter(log, filter) {
			results = append(results, log)
		}
	}

	// 应用 Limit 和 Offset
	start := filter.Offset
	if start > len(results) {
		start = len(results)
	}

	end := len(results)
	if filter.Limit > 0 && start+filter.Limit < end {
		end = start + filter.Limit
	}

	return results[start:end], nil
}

// matchFilter 检查日志是否匹配过滤条件
func (a *FileAuditLogger) matchFilter(log *AuditLog, filter *AuditFilter) bool {
	// 时间范围过滤
	if !filter.StartTime.IsZero() && log.Timestamp.Before(filter.StartTime) {
		return false
	}
	if !filter.EndTime.IsZero() && log.Timestamp.After(filter.EndTime) {
		return false
	}

	// 索引字段过滤
	if filter.ClientID != "" {
		if v, ok := log.Indexed["client_id"].(string); !ok || v != filter.ClientID {
			return false
		}
	}
	if filter.ServiceID != "" {
		if v, ok := log.Indexed["service_id"].(string); !ok || v != filter.ServiceID {
			return false
		}
	}
	if filter.Action != "" {
		if v, ok := log.Indexed["action"].(string); !ok || v != filter.Action {
			return false
		}
	}
	if filter.Result != "" {
		if v, ok := log.Indexed["result"].(string); !ok || v != filter.Result {
			return false
		}
	}
	if filter.EventType != "" {
		if v, ok := log.Indexed["event_type"].(SecurityEventType); !ok || v != filter.EventType {
			return false
		}
	}
	if filter.Severity != "" {
		if v, ok := log.Indexed["severity"].(Severity); !ok || v != filter.Severity {
			return false
		}
	}

	return true
}

// writeLog 写入审计日志到文件
func (a *FileAuditLogger) writeLog(log *AuditLog) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	// 序列化为 JSON
	data, err := json.Marshal(log)
	if err != nil {
		return fmt.Errorf("marshal audit log: %w", err)
	}

	// 写入文件
	if _, err := a.file.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write audit log: %w", err)
	}

	// 添加到内存缓存（生产环境应移除）
	a.logs = append(a.logs, log)

	return nil
}

// Close 关闭审计日志记录器
func (a *FileAuditLogger) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.file != nil {
		return a.file.Close()
	}
	return nil
}
