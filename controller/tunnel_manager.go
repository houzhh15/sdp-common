package controller

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/houzhh15/sdp-common/logging"
	"github.com/houzhh15/sdp-common/transport"
	"github.com/houzhh15/sdp-common/tunnel"
)

// InMemoryTunnelManager implements tunnel.Manager interface using in-memory storage
type InMemoryTunnelManager struct {
	tunnels  sync.Map // map[string]*tunnel.Tunnel
	services sync.Map // map[string]*tunnel.ServiceConfig
	logger   logging.Logger
}

// NewInMemoryTunnelManager creates a new in-memory tunnel manager
func NewInMemoryTunnelManager(logger logging.Logger) tunnel.Manager {
	return &InMemoryTunnelManager{
		logger: logger,
	}
}

// CreateTunnel creates a new tunnel
func (m *InMemoryTunnelManager) CreateTunnel(ctx context.Context, req *tunnel.CreateTunnelRequest) (*tunnel.Tunnel, error) {
	// Per SDP 2.0 规范：从 ServiceConfig 获取目标地址
	serviceConfig, err := m.GetServiceConfig(ctx, req.ServiceID)
	if err != nil {
		return nil, fmt.Errorf("service not found: %s (error: %w)", req.ServiceID, err)
	}

	// Generate a simple tunnel ID (without uuid dependency for now)
	tunnelID := fmt.Sprintf("tunnel-%d", time.Now().UnixNano())

	tun := &tunnel.Tunnel{
		ID:           tunnelID,
		SessionToken: req.SessionToken,
		ClientID:     req.ClientID,
		ServiceID:    req.ServiceID,
		Protocol:     req.Protocol,
		Status:       tunnel.TunnelStatusActive,
		CreatedAt:    time.Now(),
		LastActive:   time.Now(),
		Stats:        &tunnel.TunnelStats{},
		Metadata:     req.Metadata,
	}

	if tun.Metadata == nil {
		tun.Metadata = make(map[string]interface{})
	}

	// 将目标地址存储到 Metadata 中（用于 TCP Proxy 查询）
	tun.Metadata["target_host"] = serviceConfig.TargetHost
	tun.Metadata["target_port"] = serviceConfig.TargetPort

	m.tunnels.Store(tun.ID, tun)
	m.logger.Info("Tunnel created",
		"tunnel_id", tun.ID,
		"client_id", req.ClientID,
		"service_id", req.ServiceID,
		"target", fmt.Sprintf("%s:%d", serviceConfig.TargetHost, serviceConfig.TargetPort))

	return tun, nil
}

// GetTunnel retrieves a tunnel by ID
func (m *InMemoryTunnelManager) GetTunnel(ctx context.Context, tunnelID string) (*tunnel.Tunnel, error) {
	val, ok := m.tunnels.Load(tunnelID)
	if !ok {
		return nil, fmt.Errorf("tunnel not found: %s", tunnelID)
	}
	return val.(*tunnel.Tunnel), nil
}

// UpdateTunnel updates an existing tunnel
func (m *InMemoryTunnelManager) UpdateTunnel(ctx context.Context, tun *tunnel.Tunnel) error {
	_, ok := m.tunnels.Load(tun.ID)
	if !ok {
		return fmt.Errorf("tunnel not found: %s", tun.ID)
	}

	tun.LastActive = time.Now()
	m.tunnels.Store(tun.ID, tun)
	m.logger.Info("Tunnel updated", "tunnel_id", tun.ID, "status", tun.Status)

	return nil
}

// DeleteTunnel removes a tunnel
func (m *InMemoryTunnelManager) DeleteTunnel(ctx context.Context, tunnelID string) error {
	m.tunnels.Delete(tunnelID)
	m.logger.Info("Tunnel deleted", "tunnel_id", tunnelID)
	return nil
}

// ListTunnels returns all tunnels matching the filter
func (m *InMemoryTunnelManager) ListTunnels(ctx context.Context, filter *tunnel.TunnelFilter) ([]*tunnel.Tunnel, error) {
	var tunnels []*tunnel.Tunnel
	m.tunnels.Range(func(key, value interface{}) bool {
		tun := value.(*tunnel.Tunnel)
		// Apply filter if needed
		if filter != nil {
			if filter.ClientID != "" && tun.ClientID != filter.ClientID {
				return true
			}
			if filter.ServiceID != "" && tun.ServiceID != filter.ServiceID {
				return true
			}
			if filter.Status != "" && tun.Status != filter.Status {
				return true
			}
		}
		tunnels = append(tunnels, tun)
		return true
	})
	return tunnels, nil
}

// GetStats returns statistics for a tunnel
func (m *InMemoryTunnelManager) GetStats(ctx context.Context, tunnelID string) (*tunnel.TunnelStats, error) {
	tun, err := m.GetTunnel(ctx, tunnelID)
	if err != nil {
		return nil, err
	}
	return tun.Stats, nil
}

// ===== 服务配置管理方法（SDP 2.0 规范 0x04 消息支持）=====

// CreateServiceConfig 创建服务配置
func (m *InMemoryTunnelManager) CreateServiceConfig(ctx context.Context, config *tunnel.ServiceConfig) error {
	if config.ServiceID == "" {
		return fmt.Errorf("service_id is required")
	}

	// Set timestamps
	config.CreatedAt = time.Now()
	config.UpdatedAt = time.Now()

	if config.Status == "" {
		config.Status = tunnel.ServiceStatusActive
	}

	m.services.Store(config.ServiceID, config)
	m.logger.Info("Service config created",
		"service_id", config.ServiceID,
		"target", fmt.Sprintf("%s:%d", config.TargetHost, config.TargetPort))

	return nil
}

// GetServiceConfig 获取单个服务配置（HTTP GET）
func (m *InMemoryTunnelManager) GetServiceConfig(ctx context.Context, serviceID string) (*tunnel.ServiceConfig, error) {
	val, ok := m.services.Load(serviceID)
	if !ok {
		return nil, fmt.Errorf("service not found: %s", serviceID)
	}
	return val.(*tunnel.ServiceConfig), nil
}

// ListServiceConfigs 列出所有服务配置（HTTP GET）
func (m *InMemoryTunnelManager) ListServiceConfigs(ctx context.Context, agentID string) ([]*tunnel.ServiceConfig, error) {
	var configs []*tunnel.ServiceConfig
	m.services.Range(func(key, value interface{}) bool {
		config := value.(*tunnel.ServiceConfig)
		// 可以根据 agentID 过滤（如果需要）
		configs = append(configs, config)
		return true
	})
	return configs, nil
}

// UpdateServiceConfig 更新服务配置（触发 SSE Push）
func (m *InMemoryTunnelManager) UpdateServiceConfig(ctx context.Context, config *tunnel.ServiceConfig) error {
	_, ok := m.services.Load(config.ServiceID)
	if !ok {
		return fmt.Errorf("service not found: %s", config.ServiceID)
	}

	config.UpdatedAt = time.Now()
	m.services.Store(config.ServiceID, config)
	m.logger.Info("Service config updated",
		"service_id", config.ServiceID,
		"target", fmt.Sprintf("%s:%d", config.TargetHost, config.TargetPort))

	return nil
}

// DeleteServiceConfig 删除服务配置（触发 SSE Push）
func (m *InMemoryTunnelManager) DeleteServiceConfig(ctx context.Context, serviceID string) error {
	m.services.Delete(serviceID)
	m.logger.Info("Service config deleted", "service_id", serviceID)
	return nil
}

// TunnelStoreAdapter adapts tunnel.Manager to transport.TunnelStore interface
type TunnelStoreAdapter struct {
	manager tunnel.Manager
}

// NewTunnelStoreAdapter creates a new adapter
func NewTunnelStoreAdapter(manager tunnel.Manager) transport.TunnelStore {
	return &TunnelStoreAdapter{
		manager: manager,
	}
}

// Get retrieves tunnel information for TCP proxy (implements transport.TunnelStore)
func (a *TunnelStoreAdapter) Get(tunnelID string) (*transport.TunnelInfo, error) {
	ctx := context.Background()
	tun, err := a.manager.GetTunnel(ctx, tunnelID)
	if err != nil {
		return nil, err
	}

	// 从 Metadata 中获取目标地址
	targetHost, _ := tun.Metadata["target_host"].(string)
	targetPort, _ := tun.Metadata["target_port"].(int)

	if targetHost == "" || targetPort == 0 {
		return nil, fmt.Errorf("target address not found in tunnel metadata")
	}

	return &transport.TunnelInfo{
		TunnelID:   tun.ID,
		TargetHost: targetHost,
		TargetPort: targetPort,
		CreatedAt:  tun.CreatedAt,
		LastActive: tun.LastActive,
	}, nil
}

// Update updates the last active time for a tunnel (implements transport.TunnelStore)
func (a *TunnelStoreAdapter) Update(tunnelID string, lastActive time.Time) error {
	ctx := context.Background()
	tun, err := a.manager.GetTunnel(ctx, tunnelID)
	if err != nil {
		return err
	}

	tun.LastActive = lastActive
	return a.manager.UpdateTunnel(ctx, tun)
}
