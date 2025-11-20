package transport

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/houzhh15/sdp-common/logging"
)

// TunnelRelayServer Controller 数据平面中继服务器
//
// 使用场景：
//   - ✅ Controller 数据平面中继（IH → Controller → AH → Target）
//   - ✅ IH 和 AH 通过 Controller 配对建立隧道连接
//   - ✅ Controller 作为中继节点双向转发数据
//
// 核心功能：
//  1. 接收 IH 和 AH 的 mTLS 连接
//  2. 通过 TunnelID 配对 IH 和 AH 连接
//  3. 使用 io.Copy 零拷贝双向转发数据
//  4. 处理连接超时和清理
//
// 与 TCPProxyServer 的区别：
//
//	TCPProxyServer: Client → Proxy → Target（单向代理，直连目标）
//	TunnelRelayServer: IH → Controller → AH（双向中继，配对转发）
type TunnelRelayServer interface {
	// StartTLS 启动 mTLS 监听（强制要求 mTLS）
	StartTLS(addr string, tlsConfig *tls.Config) error

	// Stop 停止服务器
	Stop() error

	// GetStats 获取统计信息
	GetStats() *RelayStats
}

// PendingConnection 待配对连接
type PendingConnection struct {
	Conn       net.Conn
	TunnelID   string
	ClientType string // "ih" or "ah"
	ReceivedAt time.Time
}

// RelayStats 中继统计信息
type RelayStats struct {
	ActiveTunnels      int
	PendingConnections int
	PendingIH          int // Separate count for pending IH connections
	PendingAH          int // Separate count for pending AH connections
	TotalRelayed       uint64
	ErrorCount         int
}

// tunnelRelayServer 实现
type tunnelRelayServer struct {
	listener net.Listener
	logger   logging.Logger
	wg       sync.WaitGroup
	stopChan chan struct{}
	mu       sync.RWMutex

	// 配置参数
	pairingTimeout time.Duration // 配对超时（默认 30 秒）
	bufferSize     int           // 缓冲区大小（默认 32KB）
	readTimeout    time.Duration // 读超时（默认 30 秒）
	writeTimeout   time.Duration // 写超时（默认 30 秒）
	maxConnections int           // 最大连接数

	// 待配对连接（tunnelID -> PendingConnection）
	pendingIH sync.Map // map[string]*PendingConnection
	pendingAH sync.Map // map[string]*PendingConnection

	// 统计信息
	activeTunnels int
	totalRelayed  uint64
	errorCount    int
}

// TunnelRelayConfig 中继服务器配置
type TunnelRelayConfig struct {
	PairingTimeout time.Duration // 配对超时（默认 30 秒）
	BufferSize     int           // 缓冲区大小（默认 32KB）
	ReadTimeout    time.Duration // 读超时（默认 30 秒）
	WriteTimeout   time.Duration // 写超时（默认 30 秒）
	MaxConnections int           // 最大连接数（默认 10000）
}

// NewTunnelRelayServer 创建隧道中继服务器
func NewTunnelRelayServer(logger logging.Logger, config *TunnelRelayConfig) TunnelRelayServer {
	if config == nil {
		config = &TunnelRelayConfig{
			PairingTimeout: 30 * time.Second,
			BufferSize:     32 * 1024, // 32KB
			ReadTimeout:    30 * time.Second,
			WriteTimeout:   30 * time.Second,
			MaxConnections: 10000,
		}
	}

	if logger == nil {
		logger = &noopLogger{}
	}

	server := &tunnelRelayServer{
		logger:         logger,
		stopChan:       make(chan struct{}),
		pairingTimeout: config.PairingTimeout,
		bufferSize:     config.BufferSize,
		readTimeout:    config.ReadTimeout,
		writeTimeout:   config.WriteTimeout,
		maxConnections: config.MaxConnections,
	}

	// 启动超时清理 goroutine
	go server.cleanupExpiredConnections()

	return server
}

// StartTLS 启动 mTLS 监听
func (s *tunnelRelayServer) StartTLS(addr string, tlsConfig *tls.Config) error {
	if tlsConfig == nil {
		return fmt.Errorf("TLS config is required for tunnel relay")
	}

	// 强制要求客户端证书
	if tlsConfig.ClientAuth != tls.RequireAndVerifyClientCert {
		s.logger.Warn("TLS config does not require client cert, overriding to RequireAndVerifyClientCert")
		tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
	}

	ln, err := tls.Listen("tcp", addr, tlsConfig)
	if err != nil {
		return fmt.Errorf("failed to listen on %s with TLS: %w", addr, err)
	}

	s.mu.Lock()
	s.listener = ln
	s.mu.Unlock()

	s.logger.Info("Tunnel Relay Server started with mTLS", "addr", addr)

	return s.acceptLoop()
}

// acceptLoop 接受连接循环
func (s *tunnelRelayServer) acceptLoop() error {
	for {
		s.mu.RLock()
		ln := s.listener
		s.mu.RUnlock()

		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-s.stopChan:
				s.logger.Info("Tunnel Relay Server stopped")
				return nil
			default:
				s.logger.Error("Failed to accept connection", "error", err.Error())
				continue
			}
		}

		// 检查连接数限制
		s.mu.RLock()
		activeCount := s.activeTunnels
		s.mu.RUnlock()

		if activeCount >= s.maxConnections {
			s.logger.Warn("Max connections reached, rejecting", "max", s.maxConnections)
			conn.Close()
			continue
		}

		// 异步处理连接
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			if err := s.handleConnection(conn); err != nil {
				s.logger.Error("Connection handling error", "error", err.Error())
				s.mu.Lock()
				s.errorCount++
				s.mu.Unlock()
			}
		}()
	}
}

// handleConnection 处理单个连接
func (s *tunnelRelayServer) handleConnection(conn net.Conn) error {
	defer conn.Close()

	// 设置 TCP KeepAlive 和 TCP_NODELAY
	if tcpConn, ok := conn.(*tls.Conn); ok {
		if netConn := tcpConn.NetConn(); netConn != nil {
			if tcp, ok := netConn.(*net.TCPConn); ok {
				// 启用 TCP KeepAlive，30秒间隔
				if err := tcp.SetKeepAlive(true); err != nil {
					s.logger.Warn("Failed to set TCP KeepAlive", "error", err)
				}
				if err := tcp.SetKeepAlivePeriod(30 * time.Second); err != nil {
					s.logger.Warn("Failed to set TCP KeepAlive period", "error", err)
				}
				// 启用 TCP_NODELAY 禁用 Nagle 算法
				if err := tcp.SetNoDelay(true); err != nil {
					s.logger.Warn("Failed to set TCP NoDelay", "error", err)
				}
			}
		}
	}

	// 设置读超时
	if s.readTimeout > 0 {
		conn.SetReadDeadline(time.Now().Add(s.readTimeout))
	}

	// 1. 读取 TunnelID（36 字节 UUID）
	buf := make([]byte, 36)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return fmt.Errorf("failed to read tunnel ID: %w", err)
	}
	tunnelID := string(buf)

	// 清除读超时
	if s.readTimeout > 0 {
		conn.SetReadDeadline(time.Time{})
	}

	// 2. 提取客户端 ID 判断是 IH 还是 AH
	tlsConn, ok := conn.(*tls.Conn)
	if !ok {
		return fmt.Errorf("not a TLS connection")
	}

	state := tlsConn.ConnectionState()
	if len(state.PeerCertificates) == 0 {
		return fmt.Errorf("no client certificate provided")
	}

	clientCN := state.PeerCertificates[0].Subject.CommonName
	clientType := s.determineClientType(clientCN)

	s.logger.Info("Connection received",
		"tunnel_id", tunnelID,
		"client_cn", clientCN,
		"client_type", clientType)

	// 3. 尝试配对
	if clientType == "ih" {
		return s.handleIHConnection(conn, tunnelID, clientCN)
	} else if clientType == "ah" {
		return s.handleAHConnection(conn, tunnelID, clientCN)
	} else {
		return fmt.Errorf("unknown client type: %s", clientCN)
	}
}

// determineClientType 根据证书 CN 判断客户端类型
func (s *tunnelRelayServer) determineClientType(cn string) string {
	// 简单判断：CN 包含 "ih" 或 "ah"
	// 实际场景可能需要更复杂的逻辑（如查询数据库）
	if len(cn) > 2 {
		prefix := cn[:2]
		if prefix == "ih" {
			return "ih"
		} else if prefix == "ah" {
			return "ah"
		}
	}
	return "unknown"
}

// handleIHConnection 处理 IH 连接
func (s *tunnelRelayServer) handleIHConnection(conn net.Conn, tunnelID, clientCN string) error {
	// 检查是否已有 AH 在等待
	if value, ok := s.pendingAH.LoadAndDelete(tunnelID); ok {
		ahConn := value.(*PendingConnection)

		// Record pairing duration
		pairingDuration := time.Since(ahConn.ReceivedAt).Seconds()
		recordPairingDuration(pairingDuration)

		// Update tunnel metrics
		s.mu.Lock()
		s.activeTunnels++
		s.mu.Unlock()
		tunnelTotal.WithLabelValues("active").Inc()

		s.logger.Info("Pairing completed (AH was waiting)",
			"tunnel_id", tunnelID,
			"ih_client", clientCN,
			"ah_client", ahConn.TunnelID,
			"pairing_duration", pairingDuration)

		// 立即开始转发
		return s.relayData(conn, ahConn.Conn, tunnelID, clientCN)
	}

	// AH 未到达，将 IH 加入等待队列
	pending := &PendingConnection{
		Conn:       conn,
		TunnelID:   tunnelID,
		ClientType: "ih",
		ReceivedAt: time.Now(),
	}
	s.pendingIH.Store(tunnelID, pending)

	s.logger.Info("IH waiting for AH", "tunnel_id", tunnelID, "client_cn", clientCN)

	// 等待配对或超时
	ctx, cancel := context.WithTimeout(context.Background(), s.pairingTimeout)
	defer cancel()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.pendingIH.Delete(tunnelID)
			return fmt.Errorf("pairing timeout for tunnel %s", tunnelID)

		case <-ticker.C:
			// 检查 AH 是否已到达
			if value, ok := s.pendingAH.LoadAndDelete(tunnelID); ok {
				s.pendingIH.Delete(tunnelID)
				ahConn := value.(*PendingConnection)

				// Record pairing duration (IH arrived first, AH arrived later)
				pairingDuration := time.Since(pending.ReceivedAt).Seconds()
				recordPairingDuration(pairingDuration)

				// Update tunnel metrics
				s.mu.Lock()
				s.activeTunnels++
				s.mu.Unlock()
				tunnelTotal.WithLabelValues("active").Inc()

				s.logger.Info("Pairing completed (AH arrived)",
					"tunnel_id", tunnelID,
					"ih_client", clientCN,
					"pairing_duration", pairingDuration)
				return s.relayData(conn, ahConn.Conn, tunnelID, clientCN)
			}
		}
	}
}

// handleAHConnection 处理 AH 连接
func (s *tunnelRelayServer) handleAHConnection(conn net.Conn, tunnelID, clientCN string) error {
	// 检查是否已有 IH 在等待
	if value, ok := s.pendingIH.LoadAndDelete(tunnelID); ok {
		ihConn := value.(*PendingConnection)

		// Record pairing duration (IH arrived first, AH arrived later)
		pairingDuration := time.Since(ihConn.ReceivedAt).Seconds()
		recordPairingDuration(pairingDuration)

		// Update tunnel metrics
		s.mu.Lock()
		s.activeTunnels++
		s.mu.Unlock()
		tunnelTotal.WithLabelValues("active").Inc()

		s.logger.Info("Pairing completed (IH was waiting)",
			"tunnel_id", tunnelID,
			"ah_client", clientCN,
			"ih_client", ihConn.TunnelID,
			"pairing_duration", pairingDuration)

		// 立即开始转发
		return s.relayData(conn, ihConn.Conn, tunnelID, clientCN)
	}

	// IH 未到达，将 AH 加入等待队列
	pending := &PendingConnection{
		Conn:       conn,
		TunnelID:   tunnelID,
		ClientType: "ah",
		ReceivedAt: time.Now(),
	}
	s.pendingAH.Store(tunnelID, pending)

	s.logger.Info("AH waiting for IH", "tunnel_id", tunnelID, "client_cn", clientCN)

	// 等待配对或超时
	ctx, cancel := context.WithTimeout(context.Background(), s.pairingTimeout)
	defer cancel()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.pendingAH.Delete(tunnelID)
			return fmt.Errorf("pairing timeout for tunnel %s", tunnelID)

		case <-ticker.C:
			// 检查 IH 是否已到达
			if value, ok := s.pendingIH.LoadAndDelete(tunnelID); ok {
				s.pendingAH.Delete(tunnelID)
				ihConn := value.(*PendingConnection)
				s.logger.Info("Pairing completed (IH arrived)",
					"tunnel_id", tunnelID,
					"ah_client", clientCN)
				return s.relayData(ihConn.Conn, conn, tunnelID, clientCN)
			}
		}
	}
}

// relayData 双向转发数据（零拷贝）
func (s *tunnelRelayServer) relayData(ihConn, ahConn net.Conn, tunnelID, clientInfo string) error {
	defer ihConn.Close()
	defer ahConn.Close()

	s.mu.Lock()
	s.activeTunnels++
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.activeTunnels--
		s.mu.Unlock()
	}()

	s.logger.Info("Starting data relay", "tunnel_id", tunnelID, "client", clientInfo)

	errChan := make(chan error, 2)
	var bytesIHToAH, bytesAHToIH uint64

	// IH → AH
	go func() {
		n, err := io.Copy(ahConn, ihConn)
		bytesIHToAH = uint64(n)
		s.logger.Debug("IH→AH relay finished",
			"tunnel_id", tunnelID,
			"bytes", n,
			"error", err)
		errChan <- err
	}()

	// AH → IH
	go func() {
		n, err := io.Copy(ihConn, ahConn)
		bytesAHToIH = uint64(n)
		s.logger.Debug("AH→IH relay finished",
			"tunnel_id", tunnelID,
			"bytes", n,
			"error", err)
		errChan <- err
	}()

	// 等待任一方向完成
	err := <-errChan

	totalBytes := bytesIHToAH + bytesAHToIH

	s.mu.Lock()
	s.totalRelayed += totalBytes
	s.mu.Unlock()

	// Record bytes transferred in Prometheus
	recordBytesTransferred(totalBytes)

	// Record error if present
	if err != nil {
		s.mu.Lock()
		s.errorCount++
		s.mu.Unlock()

		// Determine error reason
		reason := "unknown"
		if err == io.EOF {
			reason = "connection_closed"
		} else if strings.Contains(err.Error(), "read") {
			reason = "read_error"
		} else if strings.Contains(err.Error(), "write") {
			reason = "write_error"
		}
		recordRelayError(reason)
	}

	s.logger.Info("Data relay completed",
		"tunnel_id", tunnelID,
		"ih_to_ah_bytes", bytesIHToAH,
		"ah_to_ih_bytes", bytesAHToIH,
		"error", err)

	return err
}

// cleanupExpiredConnections 清理过期的待配对连接
func (s *tunnelRelayServer) cleanupExpiredConnections() {
	ticker := time.NewTicker(60 * time.Second) // 每60秒扫描一次过期连接
	defer ticker.Stop()

	for {
		select {
		case <-s.stopChan:
			return
		case <-ticker.C:
			now := time.Now()

			// 清理过期的 IH 连接
			s.pendingIH.Range(func(key, value interface{}) bool {
				pending := value.(*PendingConnection)
				if now.Sub(pending.ReceivedAt) > s.pairingTimeout {
					s.logger.Warn("Cleaning up expired IH connection",
						"tunnel_id", pending.TunnelID,
						"age_seconds", int(now.Sub(pending.ReceivedAt).Seconds()))
					pending.Conn.Close()
					s.pendingIH.Delete(key)

					// Record timeout error in metrics
					recordRelayError("pairing_timeout")
				}
				return true
			})

			// 清理过期的 AH 连接
			s.pendingAH.Range(func(key, value interface{}) bool {
				pending := value.(*PendingConnection)
				if now.Sub(pending.ReceivedAt) > s.pairingTimeout {
					s.logger.Warn("Cleaning up expired AH connection",
						"tunnel_id", pending.TunnelID,
						"age_seconds", int(now.Sub(pending.ReceivedAt).Seconds()))
					pending.Conn.Close()
					s.pendingAH.Delete(key)

					// Record timeout error in metrics
					recordRelayError("pairing_timeout")
				}
				return true
			})
		}
	}
}

// Stop 停止服务器
func (s *tunnelRelayServer) Stop() error {
	// 使用 select 防止重复关闭
	select {
	case <-s.stopChan:
		// Already stopped
		return nil
	default:
		close(s.stopChan)
	}

	s.mu.Lock()
	if s.listener != nil {
		s.listener.Close()
	}
	s.mu.Unlock()

	// 关闭所有待配对连接
	s.pendingIH.Range(func(key, value interface{}) bool {
		pending := value.(*PendingConnection)
		pending.Conn.Close()
		return true
	})

	s.pendingAH.Range(func(key, value interface{}) bool {
		pending := value.(*PendingConnection)
		pending.Conn.Close()
		return true
	})

	// 等待所有连接完成
	s.wg.Wait()

	s.logger.Info("Tunnel Relay Server stopped gracefully")
	return nil
}

// GetStats 获取统计信息
func (s *tunnelRelayServer) GetStats() *RelayStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	pendingIHCount := 0
	s.pendingIH.Range(func(key, value interface{}) bool {
		pendingIHCount++
		return true
	})

	pendingAHCount := 0
	s.pendingAH.Range(func(key, value interface{}) bool {
		pendingAHCount++
		return true
	})

	return &RelayStats{
		ActiveTunnels:      s.activeTunnels,
		PendingConnections: pendingIHCount + pendingAHCount,
		PendingIH:          pendingIHCount,
		PendingAH:          pendingAHCount,
		TotalRelayed:       s.totalRelayed,
		ErrorCount:         s.errorCount,
	}
}
