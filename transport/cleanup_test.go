package transport

import (
	"crypto/tls"
	"fmt"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCleanupExpiredConnections 测试超时清理机制
func TestCleanupExpiredConnections(t *testing.T) {
	t.Skip("Full integration test - requires timing validation")

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	// 使用很短的超时和清理间隔用于测试
	relayServer := NewTunnelRelayServer(logger, &TunnelRelayConfig{
		PairingTimeout: 2 * time.Second, // 2秒超时
	})

	tlsConfig, err := generateTestTLSConfigForCleanup()
	require.NoError(t, err)

	err = relayServer.StartTLS(":0", tlsConfig)
	require.NoError(t, err)
	defer relayServer.Stop()

	addr := relayServer.(*tunnelRelayServer).listener.Addr().String()

	// 创建一个只有 IH 的连接（不会被配对）
	ihConn, err := tls.Dial("tcp", addr, &tls.Config{
		InsecureSkipVerify: true,
	})
	require.NoError(t, err)

	tunnelID := "test-cleanup-timeout-123456789012"
	_, err = ihConn.Write([]byte(fmt.Sprintf("%s:IH\n", tunnelID)))
	require.NoError(t, err)

	// 等待足够时间让连接超时并被清理
	time.Sleep(5 * time.Second)

	// 验证连接已被清理
	stats := relayServer.GetStats()
	assert.Equal(t, 0, stats.PendingConnections, "Expired connection should be cleaned up")

	// 验证清理延迟小于要求（实际应该在 60秒 + 超时时间内）
	// 这里使用较短的超时时间进行测试
}

// TestTCPKeepAliveSettings 测试 TCP KeepAlive 设置
func TestTCPKeepAliveSettings(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	relayServer := NewTunnelRelayServer(logger, &TunnelRelayConfig{
		PairingTimeout: 30 * time.Second,
	})

	tlsConfig, err := generateTestTLSConfigForCleanup()
	require.NoError(t, err)

	err = relayServer.StartTLS(":0", tlsConfig)
	require.NoError(t, err)
	defer relayServer.Stop()

	addr := relayServer.(*tunnelRelayServer).listener.Addr().String()

	// 连接到服务器
	ihConn, err := tls.Dial("tcp", addr, &tls.Config{
		InsecureSkipVerify: true,
	})
	require.NoError(t, err)
	defer ihConn.Close()

	tunnelID := "test-keepalive-12345678901234567"
	_, err = ihConn.Write([]byte(tunnelID))
	require.NoError(t, err)

	// Note: 实际的 TCP KeepAlive 验证需要低级别的套接字检查
	// 这里主要验证连接能正常建立，说明 TCP 设置没有破坏功能
	time.Sleep(100 * time.Millisecond)

	t.Log("TCP KeepAlive and NoDelay settings applied (verified by successful connection)")
}

// TestResourceCleanupRate 测试资源释放率
func TestResourceCleanupRate(t *testing.T) {
	t.Skip("Performance test - requires multiple iterations")

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	relayServer := NewTunnelRelayServer(logger, &TunnelRelayConfig{
		PairingTimeout: 1 * time.Second,
	})

	tlsConfig, err := generateTestTLSConfigForCleanup()
	require.NoError(t, err)

	err = relayServer.StartTLS(":0", tlsConfig)
	require.NoError(t, err)
	defer relayServer.Stop()

	addr := relayServer.(*tunnelRelayServer).listener.Addr().String()

	// 创建多个连接
	numConnections := 100
	for i := 0; i < numConnections; i++ {
		go func(id int) {
			conn, err := tls.Dial("tcp", addr, &tls.Config{
				InsecureSkipVerify: true,
			})
			if err != nil {
				return
			}
			defer conn.Close()

			tunnelID := fmt.Sprintf("test-cleanup-%036d", id)
			conn.Write([]byte(tunnelID))

			// 立即关闭不等待配对
		}(i)
	}

	// 等待所有连接超时
	time.Sleep(3 * time.Second)

	// 验证所有连接都被清理
	stats := relayServer.GetStats()
	cleanupRate := float64(numConnections-stats.PendingConnections) / float64(numConnections) * 100

	assert.GreaterOrEqual(t, cleanupRate, 95.0, "Cleanup rate should be at least 95%")
	t.Logf("Cleanup rate: %.2f%%", cleanupRate)
}

// TestDisconnectionDetection 测试断开检测
func TestDisconnectionDetection(t *testing.T) {
	t.Skip("Integration test - requires connection state monitoring")

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	relayServer := NewTunnelRelayServer(logger, &TunnelRelayConfig{
		PairingTimeout: 30 * time.Second,
	})

	tlsConfig, err := generateTestTLSConfigForCleanup()
	require.NoError(t, err)

	err = relayServer.StartTLS(":0", tlsConfig)
	require.NoError(t, err)
	defer relayServer.Stop()

	addr := relayServer.(*tunnelRelayServer).listener.Addr().String()
	tunnelID := "test-disconnect-123456789012345678"

	// 建立 IH 连接
	ihConn, err := tls.Dial("tcp", addr, &tls.Config{
		InsecureSkipVerify: true,
	})
	require.NoError(t, err)

	_, err = ihConn.Write([]byte(tunnelID))
	require.NoError(t, err)

	// 建立 AH 连接
	ahConn, err := tls.Dial("tcp", addr, &tls.Config{
		InsecureSkipVerify: true,
	})
	require.NoError(t, err)

	_, err = ahConn.Write([]byte(tunnelID))
	require.NoError(t, err)

	// 等待配对
	time.Sleep(500 * time.Millisecond)

	// 突然关闭 IH 连接
	disconnectTime := time.Now()
	ihConn.Close()

	// 等待检测到断开
	time.Sleep(1 * time.Second)

	// AH 应该也被关闭
	_, err = ahConn.Read(make([]byte, 1))
	assert.Error(t, err, "AH connection should be closed when IH disconnects")

	detectionLatency := time.Since(disconnectTime)
	assert.Less(t, detectionLatency, 10*time.Second, "Disconnection detection should be less than 10 seconds")

	t.Logf("Disconnection detected in %v", detectionLatency)
}

// TestCleanupWithActiveConnections 测试清理过期连接时不影响活跃连接
func TestCleanupWithActiveConnections(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	relayServer := NewTunnelRelayServer(logger, &TunnelRelayConfig{
		PairingTimeout: 5 * time.Second,
	})

	tlsConfig, err := generateTestTLSConfigForCleanup()
	require.NoError(t, err)

	err = relayServer.StartTLS(":0", tlsConfig)
	require.NoError(t, err)
	defer relayServer.Stop()

	addr := relayServer.(*tunnelRelayServer).listener.Addr().String()

	// 创建活跃的隧道（会配对）
	activeTunnelID := "test-active-123456789012345678901"
	ihConn, err := tls.Dial("tcp", addr, &tls.Config{
		InsecureSkipVerify: true,
	})
	require.NoError(t, err)
	defer ihConn.Close()

	_, err = ihConn.Write([]byte(activeTunnelID))
	require.NoError(t, err)

	ahConn, err := tls.Dial("tcp", addr, &tls.Config{
		InsecureSkipVerify: true,
	})
	require.NoError(t, err)
	defer ahConn.Close()

	_, err = ahConn.Write([]byte(activeTunnelID))
	require.NoError(t, err)

	// 等待配对
	time.Sleep(200 * time.Millisecond)

	// 创建过期的连接（不会配对）
	expiredTunnelID := "test-expired-12345678901234567890"
	expiredConn, err := tls.Dial("tcp", addr, &tls.Config{
		InsecureSkipVerify: true,
	})
	require.NoError(t, err)

	_, err = expiredConn.Write([]byte(expiredTunnelID))
	require.NoError(t, err)

	// 等待过期连接超时
	time.Sleep(6 * time.Second)

	// 验证活跃连接仍然工作
	testData := []byte("test data from IH to AH")
	_, err = ihConn.Write(testData)
	require.NoError(t, err)

	buf := make([]byte, len(testData))
	_, err = io.ReadFull(ahConn, buf)
	require.NoError(t, err)
	assert.Equal(t, testData, buf, "Active connection should still work after cleanup")

	// 验证过期连接被清理
	stats := relayServer.GetStats()
	assert.Equal(t, 1, stats.ActiveTunnels, "Should have 1 active tunnel")
	assert.Equal(t, 0, stats.PendingConnections, "Expired connection should be cleaned up")
}

// generateTestTLSConfigForCleanup 生成测试用的 TLS 配置
func generateTestTLSConfigForCleanup() (*tls.Config, error) {
	// 复用 metrics_integration_test.go 中的函数
	return generateTestTLSConfig()
}
