package tunnel

import "context"

// Manager 隧道管理器接口
// 定义隧道生命周期管理的标准接口
type Manager interface {
	// CreateTunnel 创建新隧道
	CreateTunnel(ctx context.Context, req *CreateTunnelRequest) (*Tunnel, error)

	// GetTunnel 获取隧道信息
	GetTunnel(ctx context.Context, tunnelID string) (*Tunnel, error)

	// UpdateTunnel 更新隧道信息
	UpdateTunnel(ctx context.Context, tunnel *Tunnel) error

	// DeleteTunnel 删除隧道
	DeleteTunnel(ctx context.Context, tunnelID string) error

	// ListTunnels 列出隧道
	ListTunnels(ctx context.Context, filter *TunnelFilter) ([]*Tunnel, error)

	// GetStats 获取统计信息
	GetStats(ctx context.Context, tunnelID string) (*TunnelStats, error)

	// ===== 服务配置管理（SDP 2.0 规范 0x04 消息支持）=====
	// 混合方案：HTTP GET（初始加载）+ SSE Push（实时更新）

	// CreateServiceConfig 创建服务配置
	CreateServiceConfig(ctx context.Context, config *ServiceConfig) error

	// GetServiceConfig 获取单个服务配置（HTTP GET）
	GetServiceConfig(ctx context.Context, serviceID string) (*ServiceConfig, error)

	// ListServiceConfigs 列出所有服务配置（HTTP GET）
	ListServiceConfigs(ctx context.Context, agentID string) ([]*ServiceConfig, error)

	// UpdateServiceConfig 更新服务配置（触发 SSE Push）
	UpdateServiceConfig(ctx context.Context, config *ServiceConfig) error

	// DeleteServiceConfig 删除服务配置（触发 SSE Push）
	DeleteServiceConfig(ctx context.Context, serviceID string) error
}

// CreateTunnelRequest 创建隧道请求
// Per SDP 2.0 规范：TargetHost/Port 应从 ServiceConfig 获取，不再通过控制平面传输
type CreateTunnelRequest struct {
	SessionToken string                 `json:"session_token"`
	ClientID     string                 `json:"client_id"`
	ServiceID    string                 `json:"service_id"` // 通过 ServiceID 查询 ServiceConfig 获取目标地址
	Protocol     string                 `json:"protocol"`   // "tcp", "udp"
	TTL          int64                  `json:"ttl"`        // seconds
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// TunnelFilter 隧道过滤器
type TunnelFilter struct {
	ClientID  string       `json:"client_id,omitempty"`
	ServiceID string       `json:"service_id,omitempty"`
	Status    TunnelStatus `json:"status,omitempty"`
	Limit     int          `json:"limit,omitempty"`
	Offset    int          `json:"offset,omitempty"`
}
