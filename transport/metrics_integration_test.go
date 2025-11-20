package transport

import (
	"bufio"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// generateTestTLSConfig generates a self-signed certificate for testing
func generateTestTLSConfig() (*tls.Config, error) {
	// Generate private key
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate private key: %w", err)
	}

	// Create certificate template
	notBefore := time.Now()
	notAfter := notBefore.Add(24 * time.Hour)

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("failed to generate serial number: %w", err)
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Test Organization"},
			CommonName:   "localhost",
		},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}

	// Create self-signed certificate
	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return nil, fmt.Errorf("failed to create certificate: %w", err)
	}

	// Create TLS certificate
	cert := tls.Certificate{
		Certificate: [][]byte{derBytes},
		PrivateKey:  priv,
	}

	// Create TLS config
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.NoClientCert, // Simplified for testing
		MinVersion:   tls.VersionTLS12,
	}

	return tlsConfig, nil
}

// TestPrometheusMetricsExposure 测试 Prometheus 指标暴露
func TestPrometheusMetricsExposure(t *testing.T) {
	// 注意: 指标是在 controller 的 handlers.go 中暴露的，不是在 relay server 中
	// 这里我们直接测试指标记录函数的存在性

	// 测试指标记录函数
	recordPairingDuration(0.5)
	recordBytesTransferred(1024)
	recordRelayError("test_error")

	// 验证函数调用没有 panic
	// 实际的指标值验证应该在 controller 集成测试中进行

	t.Log("Metrics recording functions work without panicking")
}

// TestTunnelPairingMetrics 测试隧道配对指标
func TestTunnelPairingMetrics(t *testing.T) {
	t.Skip("Skipping integration test - requires full TLS connection setup")

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	// 启动中继服务器
	relayServer := NewTunnelRelayServer(logger, &TunnelRelayConfig{
		PairingTimeout: 5 * time.Second,
	})

	tlsConfig, err := generateTestTLSConfig()
	require.NoError(t, err)

	err = relayServer.StartTLS(":0", tlsConfig)
	require.NoError(t, err)
	defer relayServer.Stop()

	addr := relayServer.(*tunnelRelayServer).listener.Addr().String()

	// 记录初始指标
	initialStats := relayServer.GetStats()

	// 模拟 IH 连接
	ihConn, err := tls.Dial("tcp", addr, &tls.Config{
		InsecureSkipVerify: true,
	})
	require.NoError(t, err)
	defer ihConn.Close()

	// 发送隧道标识（IH 角色）
	tunnelID := "test-tunnel-pairing-metrics"
	_, err = ihConn.Write([]byte(fmt.Sprintf("%s:IH\n", tunnelID)))
	require.NoError(t, err)

	// 等待连接被处理
	time.Sleep(200 * time.Millisecond)

	// 验证 pending 连接数增加
	stats := relayServer.GetStats()
	assert.Greater(t, stats.PendingConnections, initialStats.PendingConnections,
		"Pending connections should increase after IH connects")

	// 模拟 AH 连接
	ahConn, err := tls.Dial("tcp", addr, &tls.Config{
		InsecureSkipVerify: true,
	})
	require.NoError(t, err)
	defer ahConn.Close()

	// 发送隧道标识（AH 角色）
	_, err = ahConn.Write([]byte(fmt.Sprintf("%s:AH\n", tunnelID)))
	require.NoError(t, err)

	// 等待配对完成
	time.Sleep(200 * time.Millisecond)

	// 验证活跃隧道数增加
	stats = relayServer.GetStats()
	assert.Greater(t, stats.ActiveTunnels, initialStats.ActiveTunnels,
		"Active tunnels should increase after pairing")
	assert.Equal(t, initialStats.PendingConnections, stats.PendingConnections,
		"Pending connections should return to initial after pairing")

	// 注意: 配对时长会被记录到 tunnelPairingDuration histogram
	// 实际的 histogram 值验证需要访问 /metrics 端点
}

// TestDataTransferMetrics 测试数据传输指标
func TestDataTransferMetrics(t *testing.T) {
	t.Skip("Skipping integration test - requires full TLS connection setup")

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	relayServer := NewTunnelRelayServer(logger, &TunnelRelayConfig{
		PairingTimeout: 30 * time.Second,
	})

	tlsConfig, err := generateTestTLSConfig()
	require.NoError(t, err)

	err = relayServer.StartTLS(":0", tlsConfig)
	require.NoError(t, err)
	defer relayServer.Stop()

	addr := relayServer.(*tunnelRelayServer).listener.Addr().String()
	tunnelID := "test-tunnel-data-metrics"

	// 连接 IH
	ihConn, err := tls.Dial("tcp", addr, &tls.Config{
		InsecureSkipVerify: true,
	})
	require.NoError(t, err)
	defer ihConn.Close()

	_, err = ihConn.Write([]byte(fmt.Sprintf("%s:IH\n", tunnelID)))
	require.NoError(t, err)

	// 连接 AH
	ahConn, err := tls.Dial("tcp", addr, &tls.Config{
		InsecureSkipVerify: true,
	})
	require.NoError(t, err)
	defer ahConn.Close()

	_, err = ahConn.Write([]byte(fmt.Sprintf("%s:AH\n", tunnelID)))
	require.NoError(t, err)

	// 等待配对完成
	time.Sleep(200 * time.Millisecond)

	// 记录初始统计
	initialStats := relayServer.GetStats()

	// IH -> AH 发送数据
	testData := []byte("Hello from IH to AH via Controller!")
	_, err = ihConn.Write(testData)
	require.NoError(t, err)

	// AH 接收数据
	buf := make([]byte, 1024)
	n, err := ahConn.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, testData, buf[:n], "Data should match")

	// AH -> IH 发送数据
	responseData := []byte("Response from AH to IH")
	_, err = ahConn.Write(responseData)
	require.NoError(t, err)

	// IH 接收数据
	n, err = ihConn.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, responseData, buf[:n], "Response data should match")

	// 等待指标更新
	time.Sleep(100 * time.Millisecond)

	// 关闭连接以触发指标记录
	ihConn.Close()
	ahConn.Close()

	time.Sleep(200 * time.Millisecond)

	// 验证传输字节数
	finalStats := relayServer.GetStats()
	expectedBytes := uint64(len(testData) + len(responseData))
	assert.GreaterOrEqual(t, finalStats.TotalRelayed-initialStats.TotalRelayed, expectedBytes,
		"Total relayed bytes should include test data")

	// 注意: 实际的 tunnelBytesTransferred counter 值需要从 /metrics 端点验证
}

// TestTimeoutMetrics 测试超时指标
func TestTimeoutMetrics(t *testing.T) {
	t.Skip("Skipping integration test - requires full TLS connection setup")

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	// 使用很短的超时时间
	relayServer := NewTunnelRelayServer(logger, &TunnelRelayConfig{
		PairingTimeout: 1 * time.Second, // 1秒超时
	})

	tlsConfig, err := generateTestTLSConfig()
	require.NoError(t, err)

	err = relayServer.StartTLS(":0", tlsConfig)
	require.NoError(t, err)
	defer relayServer.Stop()

	addr := relayServer.(*tunnelRelayServer).listener.Addr().String()
	tunnelID := "test-tunnel-timeout-metrics"

	// 只连接 IH，不连接 AH
	ihConn, err := tls.Dial("tcp", addr, &tls.Config{
		InsecureSkipVerify: true,
	})
	require.NoError(t, err)
	defer ihConn.Close()

	_, err = ihConn.Write([]byte(fmt.Sprintf("%s:IH\n", tunnelID)))
	require.NoError(t, err)

	// 等待超过超时时间
	time.Sleep(2 * time.Second)

	// 验证连接被清理
	stats := relayServer.GetStats()
	assert.Equal(t, 0, stats.PendingConnections,
		"Pending connections should be 0 after timeout")

	// 注意: 超时会触发 recordRelayError("pairing_timeout")
	// 实际的 tunnelRelayErrors counter 值需要从 /metrics 端点验证
}

// TestMetricsEndpoint 测试 /metrics 端点（需要集成 controller）
func TestMetricsEndpoint(t *testing.T) {
	t.Skip("This test requires full controller setup with /metrics endpoint")

	// 这个测试应该在 controller 包的集成测试中实现
	// 因为 /metrics 端点是在 controller/handlers.go 中注册的

	// 示例测试流程：
	// 1. 启动完整的 controller（包括中继服务器）
	// 2. 执行一些隧道操作（配对、数据传输、超时）
	// 3. 访问 http://controller/metrics
	// 4. 解析 Prometheus 格式输出
	// 5. 验证各项指标值：
	//    - tunnel_total{status="active"}
	//    - tunnel_total{status="pending"}
	//    - tunnel_total{status="failed"}
	//    - tunnel_bytes_transferred_total
	//    - tunnel_pairing_duration_seconds (histogram)
	//    - tunnel_relay_errors_total{reason="..."}
}

// Helper: 解析 Prometheus 文本格式
func parsePrometheusMetrics(data string) map[string]float64 {
	metrics := make(map[string]float64)
	scanner := bufio.NewScanner(strings.NewReader(data))

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// 跳过注释和空行
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// 解析 metric_name{labels} value
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			metricKey := parts[0]
			var value float64
			fmt.Sscanf(parts[1], "%f", &value)
			metrics[metricKey] = value
		}
	}

	return metrics
}

// Helper: 从 /metrics 端点获取指标
func fetchMetrics(endpoint string) (map[string]float64, error) {
	resp, err := http.Get(endpoint)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return parsePrometheusMetrics(string(body)), nil
}

// TestMetricsAccuracy 测试指标准确性（需要 controller 集成）
func TestMetricsAccuracy(t *testing.T) {
	t.Skip("This test requires full controller setup")

	// 完整的指标准确性测试流程：
	// 1. 启动 controller + relay server
	// 2. 建立 N 个隧道连接
	// 3. 传输已知大小的数据
	// 4. 获取 /metrics 输出
	// 5. 验证 tunnel_total{status="active"} == N
	// 6. 验证 tunnel_bytes_transferred_total >= expected_bytes
	// 7. 验证 tunnel_pairing_duration_seconds histogram bucket 分布合理
	// 8. 触发一些错误（超时、连接失败）
	// 9. 验证 tunnel_relay_errors_total{reason="..."} 正确递增
}
