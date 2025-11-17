package transport

import "time"

// Event 通用事件结构
type Event struct {
	Type      string      `json:"type"`
	Data      interface{} `json:"data"`
	Timestamp time.Time   `json:"timestamp"`
}

// NewEvent 创建新事件
func NewEvent(eventType string, data interface{}) *Event {
	return &Event{
		Type:      eventType,
		Data:      data,
		Timestamp: time.Now(),
	}
}
