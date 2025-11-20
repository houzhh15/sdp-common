package transport

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/houzhh15/sdp-common/logging"
)

// TunnelStore 隧道信息存储接口
type TunnelStore interface {
	// Get 根据隧道 ID 获取隧道信息
	Get(tunnelID string) (*TunnelInfo, error)
	// Update 更新隧道活跃时间
	Update(tunnelID string, lastActive time.Time) error
}

// TunnelInfo 隧道信息
type TunnelInfo struct {
	TunnelID   string
	TargetHost string
	TargetPort int
	CreatedAt  time.Time
	LastActive time.Time
}

// tcpProxyServer TCP 代理服务器实现
//
// 使用场景说明：
//   - ✅ 适用于 IH/AH 客户端直接连接目标应用的场景（Client → TCPProxy → Target）
//   - ✅ IH Client 本地代理转发到内网目标
//   - ✅ AH Agent 接收隧道数据后转发到目标应用
//   - ❌ 不适用于 Controller 数据平面中继（应使用 TunnelRelayServer）
//
// 错误使用示例：
//
//	Controller 使用 TCPProxyServer 会导致 IH → Controller → Target 的错误流向
//	正确的 Controller 数据流应该是：IH → Controller → AH → Target（使用 TunnelRelayServer）
//
// 正确使用示例：
//  1. IH Client: 本地应用 → 127.0.0.1:8080(TCPProxy) → Controller:9443
//  2. AH Agent: Controller:9443 → TCPProxy → 内网应用:80
//
// 复用率：约 65%（基于 tunnel.TCPProxy 抽象）
type tcpProxyServer struct {
	listener    net.Listener
	tunnelStore TunnelStore
	logger      logging.Logger
	wg          sync.WaitGroup
	stopChan    chan struct{}
	mu          sync.RWMutex

	// 配置参数
	bufferSize     int
	connectTimeout time.Duration
	readTimeout    time.Duration
	writeTimeout   time.Duration
	maxConnections int
	activeConns    int
}

// TCPProxyConfig TCP Proxy 配置
type TCPProxyConfig struct {
	BufferSize     int           // 缓冲区大小（默认 32KB）
	ConnectTimeout time.Duration // 连接超时（默认 5s）
	ReadTimeout    time.Duration // 读超时（默认 30s）
	WriteTimeout   time.Duration // 写超时（默认 30s）
	MaxConnections int           // 最大连接数（默认 10000）
}

// NewTCPProxyServer 创建 TCP 代理服务器
func NewTCPProxyServer(tunnelStore TunnelStore, logger logging.Logger, config *TCPProxyConfig) TCPProxyServer {
	if config == nil {
		config = &TCPProxyConfig{
			BufferSize:     32 * 1024, // 32KB
			ConnectTimeout: 5 * time.Second,
			ReadTimeout:    30 * time.Second,
			WriteTimeout:   30 * time.Second,
			MaxConnections: 10000,
		}
	}

	if logger == nil {
		logger = &noopLogger{}
	}

	return &tcpProxyServer{
		tunnelStore:    tunnelStore,
		logger:         logger,
		stopChan:       make(chan struct{}),
		bufferSize:     config.BufferSize,
		connectTimeout: config.ConnectTimeout,
		readTimeout:    config.ReadTimeout,
		writeTimeout:   config.WriteTimeout,
		maxConnections: config.MaxConnections,
	}
}

// Start 启动 TCP 代理监听（不推荐：无 TLS 加密）
// Deprecated: Use StartTLS for production deployments
func (s *tcpProxyServer) Start(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	s.mu.Lock()
	s.listener = ln
	s.mu.Unlock()

	s.logger.Warn("TCP Proxy started WITHOUT TLS (insecure)", "addr", addr)

	return s.acceptLoop()
}

// StartTLS 启动 mTLS TCP 代理监听（推荐用于生产环境）
func (s *tcpProxyServer) StartTLS(addr string, tlsConfig *tls.Config) error {
	if tlsConfig == nil {
		return fmt.Errorf("TLS config is required for StartTLS")
	}

	ln, err := tls.Listen("tcp", addr, tlsConfig)
	if err != nil {
		return fmt.Errorf("failed to listen on %s with TLS: %w", addr, err)
	}

	s.mu.Lock()
	s.listener = ln
	s.mu.Unlock()

	s.logger.Info("TCP Proxy started with mTLS", "addr", addr)

	return s.acceptLoop()
}

// acceptLoop 接受连接循环（复用代码）
func (s *tcpProxyServer) acceptLoop() error {
	for {
		s.mu.RLock()
		ln := s.listener
		s.mu.RUnlock()

		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-s.stopChan:
				s.logger.Info("TCP Proxy stopped")
				return nil
			default:
				s.logger.Error("Failed to accept connection", "error", err.Error())
				continue
			}
		}

		// 检查连接数限制
		s.mu.Lock()
		if s.activeConns >= s.maxConnections {
			s.mu.Unlock()
			s.logger.Warn("Max connections reached, rejecting", "max", s.maxConnections)
			conn.Close()
			continue
		}
		s.activeConns++
		s.mu.Unlock()

		// 异步处理连接
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			defer func() {
				s.mu.Lock()
				s.activeConns--
				s.mu.Unlock()
			}()

			if err := s.HandleConnection(conn); err != nil {
				s.logger.Error("Connection error", "error", err.Error())
			}
		}()
	}
}

// Stop 停止代理服务器
func (s *tcpProxyServer) Stop() error {
	close(s.stopChan)

	s.mu.Lock()
	if s.listener != nil {
		s.listener.Close()
	}
	s.mu.Unlock()

	// 等待所有连接完成
	s.wg.Wait()

	s.logger.Info("TCP Proxy stopped gracefully")
	return nil
}

// HandleConnection 处理单个客户端连接
func (s *tcpProxyServer) HandleConnection(clientConn net.Conn) error {
	defer clientConn.Close()

	// 设置读超时
	if s.readTimeout > 0 {
		clientConn.SetReadDeadline(time.Now().Add(s.readTimeout))
	}

	// 1. 读取隧道 ID（前 36 字节 UUID）
	buf := make([]byte, 36)
	if _, err := io.ReadFull(clientConn, buf); err != nil {
		return fmt.Errorf("failed to read tunnel ID: %w", err)
	}
	// Trim null bytes (for padded tunnel IDs)
	tunnelID := strings.TrimRight(string(buf), "\x00")

	// 清除读超时
	if s.readTimeout > 0 {
		clientConn.SetReadDeadline(time.Time{})
	}

	s.logger.Info("Client connected", "tunnel_id", tunnelID)

	// 2. 查询隧道信息
	tunnel, err := s.tunnelStore.Get(tunnelID)
	if err != nil {
		return fmt.Errorf("tunnel not found: %s, error: %w", tunnelID, err)
	}

	// 3. 连接到目标服务
	targetAddr := fmt.Sprintf("%s:%d", tunnel.TargetHost, tunnel.TargetPort)

	dialer := &net.Dialer{
		Timeout: s.connectTimeout,
	}
	targetConn, err := dialer.Dial("tcp", targetAddr)
	if err != nil {
		return fmt.Errorf("failed to connect to target %s: %w", targetAddr, err)
	}
	defer targetConn.Close()

	s.logger.Info("Connected to target", "tunnel_id", tunnelID, "target", targetAddr)

	// 更新隧道活跃时间
	if err := s.tunnelStore.Update(tunnelID, time.Now()); err != nil {
		s.logger.Warn("Failed to update tunnel", "tunnel_id", tunnelID, "error", err.Error())
	}

	// 4. 双向数据转发
	errChan := make(chan error, 2)

	// IH -> Target
	go s.pipe(targetConn, clientConn, "IH->Target", errChan)

	// Target -> IH
	go s.pipe(clientConn, targetConn, "Target->IH", errChan)

	// 等待任一方向完成或错误
	err = <-errChan

	s.logger.Info("Connection closed", "tunnel_id", tunnelID, "error", err)
	return err
}

// pipe 数据管道（使用 io.Copy 零拷贝优化）
func (s *tcpProxyServer) pipe(dst, src net.Conn, direction string, errChan chan error) {
	// 设置读写超时
	if s.readTimeout > 0 {
		src.SetReadDeadline(time.Now().Add(s.readTimeout))
	}
	if s.writeTimeout > 0 {
		dst.SetWriteDeadline(time.Now().Add(s.writeTimeout))
	}

	// 使用 io.Copy 进行零拷贝优化
	// 在 Linux 上会自动使用 splice() 系统调用
	n, err := io.Copy(dst, src)

	s.logger.Debug("Pipe closed", "direction", direction, "bytes", n, "error", err)

	errChan <- err
}

// GetStats 获取代理统计信息
func (s *tcpProxyServer) GetStats() *ProxyStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return &ProxyStats{
		ActiveConnections: s.activeConns,
		MaxConnections:    s.maxConnections,
	}
}

// ProxyStats 代理统计信息
type ProxyStats struct {
	ActiveConnections int
	MaxConnections    int
	TotalBytes        uint64
	ErrorCount        int
}
