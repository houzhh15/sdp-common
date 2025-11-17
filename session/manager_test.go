package session

import(
	"context"
	"sync"
	"testing"
	"time"
)

// mockLogger 模拟日志记录器
type mockLogger struct{}

func (l *mockLogger) Info(msg string, fields map[string]interface{})  {}
func (l *mockLogger) Warn(msg string, fields map[string]interface{})  {}
func (l *mockLogger) Error(msg string, err error, fields map[string]interface{}) {}
func (l *mockLogger) Debug(msg string, fields map[string]interface{}) {}

// TestCreateSession 测试会话创建
func TestCreateSession(t *testing.T) {
	manager := NewManager(&Config{
		TokenTTL:        3600 * time.Second,
		CleanupInterval: 300 * time.Second,
	}, &mockLogger{})

	req := &CreateSessionRequest{
		ClientID:        "test-client-001",
		CertFingerprint: "sha256:abcd1234",
		DeviceInfo: &DeviceInfo{
			DeviceID:   "device-001",
			OS:         "Linux",
			OSVersion:  "5.10",
			Compliance: true,
		},
		Metadata: map[string]interface{}{
			"source_ip": "192.168.1.100",
		},
	}

	session, err := manager.CreateSession(context.Background(), req)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// 验证 Token 长度（64 字符十六进制）
	if len(session.Token) != 64 {
		t.Errorf("Expected token length 64, got %d", len(session.Token))
	}

	// 验证字段
	if session.ClientID != req.ClientID {
		t.Errorf("Expected ClientID %s, got %s", req.ClientID, session.ClientID)
	}

	if session.CertFingerprint != req.CertFingerprint {
		t.Errorf("Expected CertFingerprint %s, got %s", req.CertFingerprint, session.CertFingerprint)
	}

	// 验证设备信息
	if session.DeviceInfo == nil {
		t.Fatal("DeviceInfo is nil")
	}
	if session.DeviceInfo.OS != "Linux" {
		t.Errorf("Expected OS Linux, got %s", session.DeviceInfo.OS)
	}

	// 验证时间
	if session.CreatedAt.IsZero() {
		t.Error("CreatedAt is zero")
	}
	if session.ExpiresAt.Before(session.CreatedAt) {
		t.Error("ExpiresAt is before CreatedAt")
	}

	// 验证存储
	manager.mu.RLock()
	if _, ok := manager.sessions[session.Token]; !ok {
		t.Error("Session not stored in sessions map")
	}
	if tokens, ok := manager.clientSessions[req.ClientID]; !ok || len(tokens) != 1 {
		t.Error("Session not stored in clientSessions map")
	}
	manager.mu.RUnlock()
}

// TestCreateSessionMissingClientID 测试缺少 ClientID
func TestCreateSessionMissingClientID(t *testing.T) {
	manager := NewManager(&Config{
		TokenTTL:        3600 * time.Second,
		CleanupInterval: 300 * time.Second,
	}, &mockLogger{})

	req := &CreateSessionRequest{
		ClientID: "", // 缺少 ClientID
	}

	_, err := manager.CreateSession(context.Background(), req)
	if err == nil {
		t.Error("Expected error for missing ClientID, got nil")
	}
}

// TestValidateSession 测试会话验证
func TestValidateSession(t *testing.T) {
	manager := NewManager(&Config{
		TokenTTL:        3600 * time.Second,
		CleanupInterval: 300 * time.Second,
	}, &mockLogger{})

	// 创建会话
	req := &CreateSessionRequest{
		ClientID: "test-client-002",
	}
	session, err := manager.CreateSession(context.Background(), req)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// 验证有效会话
	validatedSession, err := manager.ValidateSession(context.Background(), session.Token)
	if err != nil {
		t.Errorf("ValidateSession failed: %v", err)
	}
	if validatedSession.ClientID != req.ClientID {
		t.Errorf("Expected ClientID %s, got %s", req.ClientID, validatedSession.ClientID)
	}

	// 验证 LastAccessAt 已更新
	if validatedSession.LastAccessAt.Before(session.LastAccessAt) {
		t.Error("LastAccessAt not updated")
	}

	// 验证不存在的 Token
	_, err = manager.ValidateSession(context.Background(), "invalid-token")
	if err == nil {
		t.Error("Expected error for invalid token, got nil")
	}
}

// TestValidateSessionExpired 测试过期会话验证
func TestValidateSessionExpired(t *testing.T) {
	manager := NewManager(&Config{
		TokenTTL:        1 * time.Second, // 1 秒过期
		CleanupInterval: 300 * time.Second,
	}, &mockLogger{})

	// 创建会话
	req := &CreateSessionRequest{
		ClientID: "test-client-003",
	}
	session, err := manager.CreateSession(context.Background(), req)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// 等待过期
	time.Sleep(2 * time.Second)

	// 验证过期会话
	_, err = manager.ValidateSession(context.Background(), session.Token)
	if err == nil {
		t.Error("Expected error for expired session, got nil")
	}
}

// TestRefreshSession 测试会话刷新
func TestRefreshSession(t *testing.T) {
	manager := NewManager(&Config{
		TokenTTL:        3600 * time.Second,
		CleanupInterval: 300 * time.Second,
	}, &mockLogger{})

	// 创建会话
	req := &CreateSessionRequest{
		ClientID: "test-client-004",
	}
	session, err := manager.CreateSession(context.Background(), req)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	originalExpiresAt := session.ExpiresAt

	// 等待 1 秒
	time.Sleep(1 * time.Second)

	// 刷新会话
	refreshedSession, err := manager.RefreshSession(context.Background(), session.Token)
	if err != nil {
		t.Errorf("RefreshSession failed: %v", err)
	}

	// 验证过期时间延长
	if !refreshedSession.ExpiresAt.After(originalExpiresAt) {
		t.Error("ExpiresAt not extended after refresh")
	}

	// 验证不存在的 Token
	_, err = manager.RefreshSession(context.Background(), "invalid-token")
	if err == nil {
		t.Error("Expected error for invalid token, got nil")
	}
}

// TestRevokeSession 测试会话撤销
func TestRevokeSession(t *testing.T) {
	manager := NewManager(&Config{
		TokenTTL:        3600 * time.Second,
		CleanupInterval: 300 * time.Second,
	}, &mockLogger{})

	// 创建会话
	req := &CreateSessionRequest{
		ClientID: "test-client-005",
	}
	session, err := manager.CreateSession(context.Background(), req)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// 撤销会话
	err = manager.RevokeSession(context.Background(), session.Token)
	if err != nil {
		t.Errorf("RevokeSession failed: %v", err)
	}

	// 验证已撤销
	manager.mu.RLock()
	if _, ok := manager.sessions[session.Token]; ok {
		t.Error("Session still exists after revoke")
	}
	if tokens, ok := manager.clientSessions[req.ClientID]; ok && len(tokens) > 0 {
		t.Error("ClientSessions not cleaned up after revoke")
	}
	manager.mu.RUnlock()

	// 验证不存在的 Token
	err = manager.RevokeSession(context.Background(), "invalid-token")
	if err == nil {
		t.Error("Expected error for invalid token, got nil")
	}
}

// TestGetActiveSessions 测试获取活跃会话
func TestGetActiveSessions(t *testing.T) {
	manager := NewManager(&Config{
		TokenTTL:        3600 * time.Second,
		CleanupInterval: 300 * time.Second,
	}, &mockLogger{})

	// 创建 3 个会话
	for i := 1; i <= 3; i++ {
		req := &CreateSessionRequest{
			ClientID: "test-client-006",
		}
		_, err := manager.CreateSession(context.Background(), req)
		if err != nil {
			t.Fatalf("CreateSession %d failed: %v", i, err)
		}
	}

	// 获取活跃会话
	sessions, err := manager.GetActiveSessions(context.Background())
	if err != nil {
		t.Fatalf("GetActiveSessions failed: %v", err)
	}

	if len(sessions) != 3 {
		t.Errorf("Expected 3 active sessions, got %d", len(sessions))
	}
}

// TestGetSessionsByClient 测试获取客户端会话
func TestGetSessionsByClient(t *testing.T) {
	manager := NewManager(&Config{
		TokenTTL:        3600 * time.Second,
		CleanupInterval: 300 * time.Second,
	}, &mockLogger{})

	// 创建 2 个会话给 client-A
	for i := 1; i <= 2; i++ {
		req := &CreateSessionRequest{
			ClientID: "client-A",
		}
		_, err := manager.CreateSession(context.Background(), req)
		if err != nil {
			t.Fatalf("CreateSession %d failed: %v", i, err)
		}
	}

	// 创建 1 个会话给 client-B
	req := &CreateSessionRequest{
		ClientID: "client-B",
	}
	_, err := manager.CreateSession(context.Background(), req)
	if err != nil {
		t.Fatalf("CreateSession for client-B failed: %v", err)
	}

	// 获取 client-A 的会话
	sessions, err := manager.GetSessionsByClient(context.Background(), "client-A")
	if err != nil {
		t.Fatalf("GetSessionsByClient failed: %v", err)
	}

	if len(sessions) != 2 {
		t.Errorf("Expected 2 sessions for client-A, got %d", len(sessions))
	}

	// 获取 client-B 的会话
	sessions, err = manager.GetSessionsByClient(context.Background(), "client-B")
	if err != nil {
		t.Fatalf("GetSessionsByClient failed: %v", err)
	}

	if len(sessions) != 1 {
		t.Errorf("Expected 1 session for client-B, got %d", len(sessions))
	}

	// 获取不存在的客户端
	sessions, err = manager.GetSessionsByClient(context.Background(), "client-C")
	if err != nil {
		t.Fatalf("GetSessionsByClient failed: %v", err)
	}

	if sessions != nil {
		t.Errorf("Expected nil for non-existent client, got %d sessions", len(sessions))
	}
}

// TestCleanupExpired 测试过期清理
func TestCleanupExpired(t *testing.T) {
	manager := NewManager(&Config{
		TokenTTL:        1 * time.Second, // 1 秒过期
		CleanupInterval: 300 * time.Second,
	}, &mockLogger{})

	// 创建 2 个会话
	for i := 1; i <= 2; i++ {
		req := &CreateSessionRequest{
			ClientID: "test-client-007",
		}
		_, err := manager.CreateSession(context.Background(), req)
		if err != nil {
			t.Fatalf("CreateSession %d failed: %v", i, err)
		}
	}

	// 验证会话数
	stats := manager.GetStats()
	if stats["total"].(int) != 2 {
		t.Errorf("Expected 2 sessions, got %d", stats["total"])
	}

	// 等待过期
	time.Sleep(2 * time.Second)

	// 手动触发清理
	manager.cleanExpired()

	// 验证已清理
	stats = manager.GetStats()
	if stats["total"].(int) != 0 {
		t.Errorf("Expected 0 sessions after cleanup, got %d", stats["total"])
	}
}

// TestConcurrency 测试并发安全
func TestConcurrency(t *testing.T) {
	manager := NewManager(&Config{
		TokenTTL:        3600 * time.Second,
		CleanupInterval: 300 * time.Second,
	}, &mockLogger{})

	const numGoroutines = 10
	const numIterations = 10

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// 并发创建会话
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numIterations; j++ {
				req := &CreateSessionRequest{
					ClientID: "concurrent-client",
				}
				session, err := manager.CreateSession(context.Background(), req)
				if err != nil {
					t.Errorf("CreateSession failed: %v", err)
					return
				}

				// 验证会话
				_, err = manager.ValidateSession(context.Background(), session.Token)
				if err != nil {
					t.Errorf("ValidateSession failed: %v", err)
				}
			}
		}(i)
	}

	wg.Wait()

	// 验证会话数
	stats := manager.GetStats()
	expectedTotal := numGoroutines * numIterations
	if stats["total"].(int) != expectedTotal {
		t.Errorf("Expected %d sessions, got %d", expectedTotal, stats["total"])
	}
}

// TestGetStats 测试统计信息
func TestGetStats(t *testing.T) {
	manager := NewManager(&Config{
		TokenTTL:        1 * time.Second,
		CleanupInterval: 300 * time.Second,
	}, &mockLogger{})

	// 创建 3 个会话
	for i := 1; i <= 3; i++ {
		req := &CreateSessionRequest{
			ClientID: "test-client-008",
		}
		_, err := manager.CreateSession(context.Background(), req)
		if err != nil {
			t.Fatalf("CreateSession %d failed: %v", i, err)
		}
	}

	// 获取统计信息
	stats := manager.GetStats()

	if stats["total"].(int) != 3 {
		t.Errorf("Expected total 3, got %d", stats["total"])
	}
	if stats["active"].(int) != 3 {
		t.Errorf("Expected active 3, got %d", stats["active"])
	}

	// 等待过期
	time.Sleep(2 * time.Second)

	// 再次获取统计信息
	stats = manager.GetStats()

	if stats["total"].(int) != 3 {
		t.Errorf("Expected total 3, got %d", stats["total"])
	}
	if stats["active"].(int) != 0 {
		t.Errorf("Expected active 0, got %d", stats["active"])
	}
	if stats["expired"].(int) != 3 {
		t.Errorf("Expected expired 3, got %d", stats["expired"])
	}
}
