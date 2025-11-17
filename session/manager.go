package session

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/houzhh15/sdp-common/logging"
)

// DeviceInfo 设备信息（新增）
type DeviceInfo struct {
	DeviceID   string `json:"device_id"`
	OS         string `json:"os"`
	OSVersion  string `json:"os_version"`
	Compliance bool   `json:"compliance"`
}

// Session 会话对象（扩展原有定义）
type Session struct {
	Token           string                 `json:"token"`
	ClientID        string                 `json:"client_id"`
	CertFingerprint string                 `json:"cert_fingerprint"`
	DeviceInfo      *DeviceInfo            `json:"device_info,omitempty"` // 新增
	CreatedAt       time.Time              `json:"created_at"`
	ExpiresAt       time.Time              `json:"expires_at"`
	LastAccessAt    time.Time              `json:"last_access_at"` // 新增
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
}

// CreateSessionRequest 创建会话请求
type CreateSessionRequest struct {
	ClientID        string
	CertFingerprint string
	DeviceInfo      *DeviceInfo
	Metadata        map[string]interface{}
}

// Manager 会话管理器（合并 session.Manager 和 session.Registry）
type Manager struct {
	sessions        map[string]*Session // token -> session
	clientSessions  map[string][]string // clientID -> tokens (新增：支持同一客户端多会话)
	mu              sync.RWMutex
	tokenTTL        time.Duration
	cleanupInterval time.Duration
	logger          logging.Logger
	stopChan        chan struct{}
}

// Config 管理器配置
type Config struct {
	TokenTTL        time.Duration // Token 有效期，默认 3600s
	CleanupInterval time.Duration // 清理间隔，默认 300s (5分钟)
}

// NewManager 创建会话管理器（复用 session.go 逻辑）
func NewManager(cfg *Config, logger logging.Logger) *Manager {
	if cfg.TokenTTL == 0 {
		cfg.TokenTTL = 3600 * time.Second // 默认 1 小时
	}
	if cfg.CleanupInterval == 0 {
		cfg.CleanupInterval = 300 * time.Second // 默认 5 分钟
	}

	return &Manager{
		sessions:        make(map[string]*Session),
		clientSessions:  make(map[string][]string),
		tokenTTL:        cfg.TokenTTL,
		cleanupInterval: cfg.CleanupInterval,
		logger:          logger,
		stopChan:        make(chan struct{}),
	}
}

// CreateSession 创建会话（复用 session.go，增加 DeviceInfo 和 Metadata）
func (m *Manager) CreateSession(ctx context.Context, req *CreateSessionRequest) (*Session, error) {
	if req.ClientID == "" {
		return nil, fmt.Errorf("client_id is required")
	}

	// 生成 Token（复用 session.go 的 generateToken）
	token, err := generateToken()
	if err != nil {
		return nil, fmt.Errorf("generate token failed: %w", err)
	}

	now := time.Now()
	session := &Session{
		Token:           token,
		ClientID:        req.ClientID,
		CertFingerprint: req.CertFingerprint,
		DeviceInfo:      req.DeviceInfo,
		CreatedAt:       now,
		ExpiresAt:       now.Add(m.tokenTTL),
		LastAccessAt:    now,
		Metadata:        req.Metadata,
	}

	m.mu.Lock()
	m.sessions[token] = session
	m.clientSessions[req.ClientID] = append(m.clientSessions[req.ClientID], token)
	m.mu.Unlock()

	m.logger.Debug("Session created",
		"token", token,
		"client_id", req.ClientID,
		"expires_at", session.ExpiresAt.Format(time.RFC3339),
	)

	return session, nil
}

// ValidateSession 验证会话（复用 session.go，更新 LastAccessAt）
func (m *Manager) ValidateSession(ctx context.Context, token string) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.sessions[token]
	if !ok {
		return nil, fmt.Errorf("session not found")
	}

	// 检查过期
	if time.Now().After(session.ExpiresAt) {
		return nil, fmt.Errorf("session expired")
	}

	// 更新最后访问时间（新增）
	session.LastAccessAt = time.Now()

	return session, nil
}

// RefreshSession 刷新会话（新增方法）
func (m *Manager) RefreshSession(ctx context.Context, token string) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.sessions[token]
	if !ok {
		return nil, fmt.Errorf("session not found")
	}

	// 检查过期
	if time.Now().After(session.ExpiresAt) {
		return nil, fmt.Errorf("session expired")
	}

	// 延长过期时间
	session.ExpiresAt = time.Now().Add(m.tokenTTL)
	session.LastAccessAt = time.Now()

	m.logger.Debug("Session refreshed",
		"token", token,
		"client_id", session.ClientID,
		"expires_at", session.ExpiresAt.Format(time.RFC3339),
	)

	return session, nil
}

// RevokeSession 撤销会话（新增方法）
func (m *Manager) RevokeSession(ctx context.Context, token string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.sessions[token]
	if !ok {
		return fmt.Errorf("session not found")
	}

	// 从 sessions 映射中移除
	delete(m.sessions, token)

	// 从 clientSessions 映射中移除
	if tokens, exists := m.clientSessions[session.ClientID]; exists {
		newTokens := make([]string, 0, len(tokens))
		for _, t := range tokens {
			if t != token {
				newTokens = append(newTokens, t)
			}
		}
		if len(newTokens) > 0 {
			m.clientSessions[session.ClientID] = newTokens
		} else {
			delete(m.clientSessions, session.ClientID)
		}
	}

	m.logger.Info("Session revoked",
		"token", token,
		"client_id", session.ClientID,
	)

	return nil
}

// GetActiveSessions 获取所有活跃会话（新增方法）
func (m *Manager) GetActiveSessions(ctx context.Context) ([]*Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	now := time.Now()
	sessions := make([]*Session, 0, len(m.sessions))

	for _, session := range m.sessions {
		if now.Before(session.ExpiresAt) {
			sessions = append(sessions, session)
		}
	}

	return sessions, nil
}

// GetSessionsByClient 获取指定客户端的所有会话（新增方法）
func (m *Manager) GetSessionsByClient(ctx context.Context, clientID string) ([]*Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tokens, ok := m.clientSessions[clientID]
	if !ok {
		return nil, nil
	}

	sessions := make([]*Session, 0, len(tokens))
	now := time.Now()

	for _, token := range tokens {
		if session, exists := m.sessions[token]; exists {
			if now.Before(session.ExpiresAt) {
				sessions = append(sessions, session)
			}
		}
	}

	return sessions, nil
}

// StartCleanup 启动定期清理（复用 session.go 和 registry.go 逻辑）
func (m *Manager) StartCleanup(ctx context.Context) {
	ticker := time.NewTicker(m.cleanupInterval)
	defer ticker.Stop()

	m.logger.Info("Session cleanup started",
		"interval", m.cleanupInterval.String(),
	)

	for {
		select {
		case <-ticker.C:
			m.cleanExpired()
		case <-ctx.Done():
			m.logger.Info("Session cleanup stopped (context done)")
			return
		case <-m.stopChan:
			m.logger.Info("Session cleanup stopped (manual)")
			return
		}
	}
}

// StopCleanup 停止清理（新增）
func (m *Manager) StopCleanup() {
	close(m.stopChan)
}

// cleanExpired 清理过期会话（合并 session.go 和 registry.go 清理逻辑）
func (m *Manager) cleanExpired() {
	now := time.Now()
	expiredTokens := make([]string, 0)

	m.mu.RLock()
	for token, session := range m.sessions {
		if now.After(session.ExpiresAt) {
			expiredTokens = append(expiredTokens, token)
		}
	}
	m.mu.RUnlock()

	if len(expiredTokens) == 0 {
		return
	}

	// 移除过期会话
	m.mu.Lock()
	for _, token := range expiredTokens {
		if session, ok := m.sessions[token]; ok {
			delete(m.sessions, token)

			// 从 clientSessions 中移除
			if tokens, exists := m.clientSessions[session.ClientID]; exists {
				newTokens := make([]string, 0, len(tokens))
				for _, t := range tokens {
					if t != token {
						newTokens = append(newTokens, t)
					}
				}
				if len(newTokens) > 0 {
					m.clientSessions[session.ClientID] = newTokens
				} else {
					delete(m.clientSessions, session.ClientID)
				}
			}
		}
	}
	m.mu.Unlock()

	m.logger.Info("Cleaned up expired sessions",
		"count", len(expiredTokens),
	)
}

// GetStats 获取统计信息（复用 registry.go 逻辑）
func (m *Manager) GetStats() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	activeCount := 0
	expiredCount := 0
	now := time.Now()

	for _, session := range m.sessions {
		if now.Before(session.ExpiresAt) {
			activeCount++
		} else {
			expiredCount++
		}
	}

	return map[string]interface{}{
		"total":   len(m.sessions),
		"active":  activeCount,
		"expired": expiredCount,
		"clients": len(m.clientSessions),
	}
}
