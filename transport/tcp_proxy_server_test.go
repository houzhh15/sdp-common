package transport

import (
	"fmt"
	"io"
	"net"
	"testing"
	"time"
)

// mockTunnelStore 模拟隧道存储
type mockTunnelStore struct {
	tunnels map[string]*TunnelInfo
}

func newMockTunnelStore() *mockTunnelStore {
	return &mockTunnelStore{
		tunnels: make(map[string]*TunnelInfo),
	}
}

func (m *mockTunnelStore) Get(tunnelID string) (*TunnelInfo, error) {
	tunnel, ok := m.tunnels[tunnelID]
	if !ok {
		return nil, fmt.Errorf("tunnel not found: %s", tunnelID)
	}
	return tunnel, nil
}

func (m *mockTunnelStore) Update(tunnelID string, lastActive time.Time) error {
	tunnel, ok := m.tunnels[tunnelID]
	if !ok {
		return fmt.Errorf("tunnel not found: %s", tunnelID)
	}
	tunnel.LastActive = lastActive
	return nil
}

func (m *mockTunnelStore) Add(tunnel *TunnelInfo) {
	m.tunnels[tunnel.TunnelID] = tunnel
}

func TestNewTCPProxyServer(t *testing.T) {
	store := newMockTunnelStore()
	server := NewTCPProxyServer(store, nil, nil)

	if server == nil {
		t.Fatal("NewTCPProxyServer returned nil")
	}
}

func TestTCPProxyServer_HandleConnection(t *testing.T) {
	store := newMockTunnelStore()

	// 添加测试隧道（正好 36 字节）
	tunnelID := "12345678-1234-1234-1234-123456789012"
	if len(tunnelID) != 36 {
		t.Fatalf("Tunnel ID must be exactly 36 bytes, got %d", len(tunnelID))
	}

	store.Add(&TunnelInfo{
		TunnelID:   tunnelID,
		TargetHost: "127.0.0.1",
		TargetPort: 18082,
		CreatedAt:  time.Now(),
		LastActive: time.Now(),
	})

	// 启动目标服务器
	targetServer, err := net.Listen("tcp", ":18082")
	if err != nil {
		t.Fatalf("Failed to start target server: %v", err)
	}
	defer targetServer.Close()

	// 目标服务器逻辑（回显）
	go func() {
		for {
			conn, err := targetServer.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				io.Copy(c, c) // Echo
			}(conn)
		}
	}()

	// 创建 TCP Proxy
	server := NewTCPProxyServer(store, nil, nil)

	// 模拟客户端连接
	client, proxyConn := net.Pipe()
	defer client.Close()

	// 异步处理连接
	done := make(chan error)
	go func() {
		done <- server.HandleConnection(proxyConn)
	}()

	// 客户端发送隧道 ID（必须是 36 字节）
	if _, err := client.Write([]byte(tunnelID)); err != nil {
		t.Fatalf("Failed to write tunnel ID: %v", err)
	}

	// 等待连接建立
	time.Sleep(200 * time.Millisecond)

	// 发送测试数据
	testData := []byte("Hello Proxy")
	if _, err := client.Write(testData); err != nil {
		t.Fatalf("Failed to write data: %v", err)
	}

	// 读取回显
	buf := make([]byte, len(testData))
	if _, err := io.ReadFull(client, buf); err != nil {
		t.Fatalf("Failed to read echo: %v", err)
	}

	if string(buf) != string(testData) {
		t.Errorf("Expected %s, got %s", testData, buf)
	}

	// 关闭客户端连接
	client.Close()

	// 等待 HandleConnection 完成
	select {
	case err := <-done:
		if err != nil {
			t.Logf("HandleConnection returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("HandleConnection timeout")
	}
}

func TestTCPProxyServer_Start(t *testing.T) {
	store := newMockTunnelStore()
	server := NewTCPProxyServer(store, nil, nil)

	// 启动服务器
	go func() {
		if err := server.Start(":19443"); err != nil {
			t.Logf("Server stopped: %v", err)
		}
	}()

	// 等待启动
	time.Sleep(100 * time.Millisecond)

	// 尝试连接
	conn, err := net.Dial("tcp", "127.0.0.1:19443")
	if err != nil {
		t.Fatalf("Failed to connect to proxy: %v", err)
	}
	conn.Close()

	// 停止服务器
	if err := server.Stop(); err != nil {
		t.Errorf("Failed to stop server: %v", err)
	}
}

func TestTCPProxyServer_MaxConnections(t *testing.T) {
	store := newMockTunnelStore()
	config := &TCPProxyConfig{
		BufferSize:     1024,
		ConnectTimeout: 1 * time.Second,
		ReadTimeout:    2 * time.Second,
		WriteTimeout:   2 * time.Second,
		MaxConnections: 2, // 限制 2 个连接
	}

	server := NewTCPProxyServer(store, nil, config)

	// 启动服务器
	go server.Start(":19444")
	time.Sleep(200 * time.Millisecond)

	// 创建 2 个连接（阻塞在读取隧道 ID）
	conn1, err := net.Dial("tcp", "127.0.0.1:19444")
	if err != nil {
		t.Fatalf("Failed to create conn1: %v", err)
	}
	defer conn1.Close()

	conn2, err := net.Dial("tcp", "127.0.0.1:19444")
	if err != nil {
		t.Fatalf("Failed to create conn2: %v", err)
	}
	defer conn2.Close()

	// 等待连接被 accept
	time.Sleep(200 * time.Millisecond)

	// 第 3 个连接应该被立即关闭（因为超过限制）
	conn3, err := net.DialTimeout("tcp", "127.0.0.1:19444", 1*time.Second)
	if err != nil {
		// 连接可能被拒绝，这是期望的
		t.Logf("3rd connection rejected as expected: %v", err)
	} else {
		// 连接成功，但应该被立即关闭
		defer conn3.Close()

		// 尝试读取数据，应该立即失败（连接已关闭）
		buf := make([]byte, 1)
		conn3.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		_, err := conn3.Read(buf)
		if err == nil {
			t.Error("Expected connection to be closed, but read succeeded")
		}
	}

	server.Stop()
}

func TestTCPProxyServer_GetStats(t *testing.T) {
	store := newMockTunnelStore()
	server := NewTCPProxyServer(store, nil, nil).(*tcpProxyServer)

	stats := server.GetStats()
	if stats == nil {
		t.Fatal("GetStats returned nil")
	}

	if stats.ActiveConnections != 0 {
		t.Errorf("Expected 0 active connections, got %d", stats.ActiveConnections)
	}
}
