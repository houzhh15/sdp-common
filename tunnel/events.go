package tunnel

import (
	"context"
	"encoding/json"
	"time"
)

// EventStore 事件存储接口（协议无关）
// 实现可以基于：Redis Stream, Kafka, PostgreSQL, Memory, etc.
type EventStore interface {
	// Publish 发布事件到指定订阅者
	// 返回事件ID（用于 Last-Event-ID）
	Publish(ctx context.Context, subscriberID string, event *Event) (eventID string, err error)

	// Subscribe 订阅事件流（从指定 ID 之后开始）
	// lastEventID 为空表示从最新事件开始
	Subscribe(ctx context.Context, subscriberID, lastEventID string) (<-chan *Event, error)

	// GetEventsAfter 获取指定 ID 之后的历史事件（用于重连恢复）
	GetEventsAfter(ctx context.Context, subscriberID, lastEventID string, limit int) ([]*Event, error)

	// Ack 确认事件已处理（可选，用于消费者组模式）
	Ack(ctx context.Context, subscriberID, eventID string) error

	// Close 关闭存储连接
	Close() error
}

// Event 通用事件结构（协议无关）
type Event struct {
	// ID 事件唯一标识（由存储系统生成，如 Redis Stream ID）
	ID string `json:"id"`

	// Type 事件类型（tunnel, service, policy, etc.）
	Type string `json:"type"`

	// Data 事件数据（JSON 格式）
	Data json.RawMessage `json:"data"`

	// Timestamp 事件时间戳（Unix 毫秒）
	Timestamp int64 `json:"timestamp"`

	// Metadata 可选的元数据
	Metadata map[string]string `json:"metadata,omitempty"`
}

// EventType 标准事件类型
const (
	EventTypeTunnelCreated  = "tunnel.created"
	EventTypeTunnelClosed   = "tunnel.closed"
	EventTypeServiceUpdated = "service.updated"
	EventTypePolicyChanged  = "policy.changed"
	EventTypeAgentStatus    = "agent.status"
)

// TunnelEventData 隧道事件数据结构
type TunnelEventData struct {
	Action    string      `json:"action"` // created, closed, updated
	Tunnel    *TunnelInfo `json:"tunnel"`
	Timestamp time.Time   `json:"timestamp"`
}

// TunnelInfo 隧道信息（简化版，避免循环依赖）
type TunnelInfo struct {
	ID        string `json:"id"`
	ClientID  string `json:"client_id"`
	ServiceID string `json:"service_id"`
	Status    string `json:"status"`
}

// ServiceEventData 服务事件数据结构
type ServiceEventData struct {
	Action    string                 `json:"action"` // updated, removed
	ServiceID string                 `json:"service_id"`
	Config    map[string]interface{} `json:"config"`
	Timestamp time.Time              `json:"timestamp"`
}

// NewEvent 创建新事件（辅助函数）
func NewEvent(eventType string, data interface{}) (*Event, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	return &Event{
		Type:      eventType,
		Data:      jsonData,
		Timestamp: time.Now().UnixMilli(),
		Metadata:  make(map[string]string),
	}, nil
}

// ParseData 解析事件数据到目标结构
func (e *Event) ParseData(v interface{}) error {
	return json.Unmarshal(e.Data, v)
}
