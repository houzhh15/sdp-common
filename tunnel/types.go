package tunnel

import (
	"time"
)

// Protocol constants
const (
	// TunnelIDLength is the fixed length of tunnel ID in data plane protocol
	// Per sdp-common data plane protocol v1.0: Fixed 36-byte UUID format
	TunnelIDLength = 36
)

// Tunnel 表示一个隧道连接
// Per SDP 2.0: Combines control plane metadata with data plane endpoints
type Tunnel struct {
	ID         string `json:"id"`
	ClientID   string `json:"client_id"`   // Per SDP 2.0: IH identifier
	ServiceID  string `json:"service_id"`  // Per SDP 2.0: Service identifier
	IHEndpoint string `json:"ih_endpoint"` // Initiating Host endpoint
	AHEndpoint string `json:"ah_endpoint"` // Accepting Host endpoint (TCP Proxy)

	// ⚠️ 架构决策说明：
	// SessionToken 不传给 AH（AH 不需要 IH 的 session）
	// - IH 通过 HTTP Handshake 获取 session_token
	// - AH 通过 mTLS 认证，无需 session 机制
	SessionToken string `json:"session_token,omitempty"` // 仅用于内部管理，不通过控制平面传输

	Protocol   string                 `json:"protocol"` // "tcp", "udp"
	Status     TunnelStatus           `json:"status"`
	CreatedAt  time.Time              `json:"created_at"`
	LastActive time.Time              `json:"last_active"`
	ExpiresAt  time.Time              `json:"expires_at,omitempty"`
	Stats      *TunnelStats           `json:"stats,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// ServiceConfig 服务配置（SDP 2.0 规范 0x04 消息）
// Per SDP 2.0 Spec 3.2.1.d: AH Service Message
// Controller 通过此消息告知 AH Agent 需要代理的服务配置
type ServiceConfig struct {
	ServiceID   string                 `json:"service_id"`   // 服务标识
	ServiceName string                 `json:"service_name"` // 服务名称（可读）
	TargetHost  string                 `json:"target_host"`  // 目标主机地址
	TargetPort  int                    `json:"target_port"`  // 目标端口
	Protocol    string                 `json:"protocol"`     // 协议类型（tcp/udp）
	Description string                 `json:"description"`  // 服务描述
	Status      ServiceStatus          `json:"status"`       // 服务状态
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"` // 额外元数据
}

// ServiceStatus 服务状态
type ServiceStatus string

const (
	ServiceStatusActive   ServiceStatus = "active"   // 活跃
	ServiceStatusInactive ServiceStatus = "inactive" // 停用
	ServiceStatusDeleted  ServiceStatus = "deleted"  // 已删除
)

// ServiceEvent 服务配置事件（用于 SSE 推送）
// Per SDP 2.0 Spec: 混合方案中的实时推送机制
type ServiceEvent struct {
	Type      ServiceEventType       `json:"type"`
	Service   *ServiceConfig         `json:"service"`
	Timestamp time.Time              `json:"timestamp"`
	Details   map[string]interface{} `json:"details,omitempty"`
}

// ServiceEventType 服务事件类型
type ServiceEventType string

const (
	ServiceEventCreated ServiceEventType = "service_created"
	ServiceEventUpdated ServiceEventType = "service_updated"
	ServiceEventDeleted ServiceEventType = "service_deleted"
)

// TunnelStatus 隧道状态
type TunnelStatus string

const (
	TunnelStatusPending TunnelStatus = "pending" // 等待连接
	TunnelStatusActive  TunnelStatus = "active"  // 活跃
	TunnelStatusClosed  TunnelStatus = "closed"  // 已关闭
	TunnelStatusError   TunnelStatus = "error"   // 错误
)

// TunnelStats 隧道统计信息
type TunnelStats struct {
	BytesSent     int64         `json:"bytes_sent"`
	BytesReceived int64         `json:"bytes_received"`
	PacketsSent   int64         `json:"packets_sent"`
	PacketsRecv   int64         `json:"packets_recv"`
	ErrorCount    int64         `json:"error_count"`
	AvgLatency    time.Duration `json:"avg_latency"`
	LastError     string        `json:"last_error,omitempty"`
}

// TunnelEvent 隧道事件
// Per SDP 2.0 Spec 3.2.1.g: IH认证信息 (0x05 IH Authenticators)
// 用于通知 AH 哪个 IH 被授权访问哪个服务
//
// ⚠️ 架构决策：sdp-common 使用 mTLS 替代 SPA 机制
// - 不使用 SPA (Single Packet Authorization)
// - 依赖 mTLS 双向认证提供安全保障
// - 因此不需要 hmac_seed 和 hotp_seed 字段
type TunnelEvent struct {
	Type      EventType              `json:"type"`
	Tunnel    *Tunnel                `json:"tunnel"`
	Timestamp time.Time              `json:"timestamp"`
	Details   map[string]interface{} `json:"details,omitempty"`
}

// EventType 事件类型
type EventType string

const (
	EventTypeCreated EventType = "created"
	EventTypeUpdated EventType = "updated"
	EventTypeDeleted EventType = "deleted"
	EventTypeError   EventType = "error"
)

// DataPacket 数据包
type DataPacket struct {
	TunnelID  string    `json:"tunnel_id"`
	Sequence  uint64    `json:"sequence"`
	Payload   []byte    `json:"payload"`
	Timestamp time.Time `json:"timestamp"`
	Direction string    `json:"direction"` // "ih_to_ah" or "ah_to_ih"
}

// TunnelStore 隧道存储接口
// 用于TCPProxy查询隧道信息
type TunnelStore interface {
	Get(tunnelID string) (*Tunnel, error)
	Set(tunnel *Tunnel) error
	Delete(tunnelID string) error
	List() ([]*Tunnel, error)
}

// Config 隧道配置
type Config struct {
	// TCP Proxy 配置
	TCPProxyAddr   string        `json:"tcp_proxy_addr"`  // 默认 ":9443"
	ConnectTimeout time.Duration `json:"connect_timeout"` // 连接超时
	IdleTimeout    time.Duration `json:"idle_timeout"`    // 空闲超时
	BufferSize     int           `json:"buffer_size"`     // 缓冲区大小

	// SSE 配置
	SSEHeartbeat time.Duration `json:"sse_heartbeat"` // SSE 心跳间隔

	// Tunnel 配置
	MaxConcurrent int           `json:"max_concurrent"` // 最大并发隧道数
	DefaultTTL    time.Duration `json:"default_ttl"`    // 默认生存时间
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		TCPProxyAddr:   ":9443",
		ConnectTimeout: 5 * time.Second,
		IdleTimeout:    300 * time.Second,
		BufferSize:     32 * 1024, // 32KB
		SSEHeartbeat:   30 * time.Second,
		MaxConcurrent:  10000,
		DefaultTTL:     3600 * time.Second, // 1小时
	}
}
