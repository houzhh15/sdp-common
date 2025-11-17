// Package main demonstrates a complete IH (Initiating Host) Client with local proxy service
// This allows users to connect to a local port and access remote services through SDP tunnel
package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/houzhh15/sdp-common/cert"
	"github.com/houzhh15/sdp-common/logging"
	"github.com/houzhh15/sdp-common/policy"
	"github.com/houzhh15/sdp-common/tunnel"
)

var (
	certFile   = flag.String("cert", "../../certs/ih-client-cert.pem", "Certificate file path")
	keyFile    = flag.String("key", "../../certs/ih-client-key.pem", "Private key file path")
	caFile     = flag.String("ca", "../../certs/ca-cert.pem", "CA certificate file path")
	controller = flag.String("controller", "https://localhost:8443", "Controller URL")
	localAddr  = flag.String("local", "localhost:8080", "Local proxy listen address")
	proxyAddr  = flag.String("proxy", "localhost:9443", "Controller TCP proxy address")
	tunnelID   = flag.String("tunnel-id", "tunnel-12345678", "Tunnel ID for this connection")
	logLevel   = flag.String("log-level", "info", "Log level (debug, info, warn, error)")
)

// IHProxy represents the IH Client with local proxy capability
type IHProxy struct {
	localAddr string
	proxyAddr string
	tunnelID  string
	tlsConfig *tls.Config
	logger    logging.Logger
	listener  net.Listener
	mu        sync.Mutex
	active    map[string]net.Conn
	connCount int
	shutdown  chan struct{}
	wg        sync.WaitGroup

	// step-08: 新增字段用于完整流程
	sessionToken  string           // 会话Token
	controllerURL string           // Controller API地址
	httpClient    *http.Client     // HTTP客户端
	policies      []*policy.Policy // 缓存的策略列表
	tunnelCreated bool             // 隧道是否已创建
}

func main() {
	flag.Parse()

	// 1. Initialize logger
	logger, err := logging.NewLogger(&logging.Config{
		Level:  *logLevel,
		Format: "json",
		Output: "stdout",
	})
	if err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}

	logger.Info("IH Client Proxy starting", "version", "1.0.0-proxy")

	// 2. Initialize certificate manager
	certManager, err := cert.NewManager(&cert.Config{
		CertFile: *certFile,
		KeyFile:  *keyFile,
		CAFile:   *caFile,
	})
	if err != nil {
		log.Fatalf("Failed to initialize cert manager: %v", err)
	}

	fingerprint := certManager.GetFingerprint()
	logger.Info("Certificate loaded", "fingerprint", fingerprint)

	// Validate certificate expiry
	if err := certManager.ValidateExpiry(); err != nil {
		log.Fatalf("Certificate validation failed: %v", err)
	}

	daysLeft := certManager.DaysUntilExpiry()
	if daysLeft < 30 {
		logger.Warn("Certificate expiring soon", "days_remaining", daysLeft)
	}

	// 3. Create IH Proxy
	proxy := &IHProxy{
		localAddr:     *localAddr,
		proxyAddr:     *proxyAddr,
		tunnelID:      *tunnelID,
		tlsConfig:     certManager.GetTLSConfig(),
		logger:        logger,
		active:        make(map[string]net.Conn),
		shutdown:      make(chan struct{}),
		controllerURL: *controller,
		httpClient: &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: certManager.GetTLSConfig(),
			},
			Timeout: 30 * time.Second,
		},
	}

	// 4. step-08: 执行握手获取session token
	if err := proxy.handshake(fingerprint); err != nil {
		log.Fatalf("Handshake failed: %v", err)
	}

	// 5. step-08: 查询策略
	if err := proxy.queryPolicies(); err != nil {
		logger.Warn("Failed to query policies", "error", err.Error())
		// 继续运行，不中断服务
	}

	// step-08: 预先创建隧道 (before starting local proxy)
	serviceID := ""
	if len(proxy.policies) > 0 {
		serviceID = proxy.policies[0].ServiceID
	}

	newTunnelID, err := proxy.createTunnel(serviceID)
	if err != nil {
		logger.Warn("Failed to create tunnel during startup, will use command-line tunnel-id", "error", err.Error())
	} else {
		proxy.tunnelID = newTunnelID
		proxy.tunnelCreated = true
		logger.Info("Tunnel pre-created", "tunnel_id", newTunnelID)
	}

	// 6. Start local proxy server
	if err := proxy.Start(); err != nil {
		log.Fatalf("Failed to start proxy: %v", err)
	}

	// 5. Display startup information
	fmt.Printf("\n✅ IH Client Proxy started successfully!\n\n")
	fmt.Printf("📍 Configuration:\n")
	fmt.Printf("   Local Address:  %s  (用户连接这里)\n", *localAddr)
	fmt.Printf("   Proxy Address:  %s  (连接到 Controller)\n", *proxyAddr)
	fmt.Printf("   Tunnel ID:      %s\n", proxy.tunnelID)
	fmt.Printf("   Controller:     %s\n", *controller)
	fmt.Printf("   Client ID:      %s\n", fingerprint[:16]+"...")
	fmt.Printf("\n💡 使用方法:\n")
	fmt.Printf("   curl http://%s\n", *localAddr)
	fmt.Printf("   或在浏览器访问: http://%s\n", *localAddr)
	fmt.Printf("\n📊 监控:\n")
	fmt.Printf("   查看日志以监控连接状态\n")
	fmt.Printf("\n   Press Ctrl+C to stop\n\n")

	logger.Info("Proxy ready for connections",
		"local", *localAddr,
		"proxy", *proxyAddr,
		"tunnel_id", proxy.tunnelID)

	// 6. Monitor connection stats
	go proxy.monitorStats()

	// 7. Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	// 8. Graceful shutdown
	logger.Info("Shutting down gracefully...")
	proxy.Stop()
	logger.Info("IH Client Proxy stopped")
}

// Start initializes and starts the local proxy server
func (p *IHProxy) Start() error {
	ln, err := net.Listen("tcp", p.localAddr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", p.localAddr, err)
	}

	p.listener = ln
	p.logger.Info("Local proxy listening", "addr", p.localAddr)

	p.wg.Add(1)
	go p.acceptLoop()

	return nil
}

// Stop gracefully shuts down the proxy
func (p *IHProxy) Stop() {
	close(p.shutdown)

	// Close listener
	if p.listener != nil {
		p.listener.Close()
	}

	// Close all active connections
	p.mu.Lock()
	for id, conn := range p.active {
		p.logger.Info("Closing connection", "id", id)
		conn.Close()
	}
	p.mu.Unlock()

	// Wait for all goroutines
	p.wg.Wait()
}

// acceptLoop accepts incoming connections from local users
func (p *IHProxy) acceptLoop() {
	defer p.wg.Done()

	for {
		select {
		case <-p.shutdown:
			return
		default:
		}

		// Set accept deadline to check shutdown periodically
		if tcpListener, ok := p.listener.(*net.TCPListener); ok {
			tcpListener.SetDeadline(time.Now().Add(1 * time.Second))
		}

		conn, err := p.listener.Accept()
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue // Timeout, check shutdown and retry
			}

			select {
			case <-p.shutdown:
				return
			default:
				p.logger.Error("Accept error", "error", err)
				continue
			}
		}

		p.wg.Add(1)
		go p.handleConnection(conn)
	}
}

// handleConnection processes a single user connection
func (p *IHProxy) handleConnection(localConn net.Conn) {
	defer p.wg.Done()

	// Generate connection ID
	p.mu.Lock()
	p.connCount++
	connID := fmt.Sprintf("conn-%d", p.connCount)
	p.mu.Unlock()

	p.logger.Info("New connection", "id", connID, "from", localConn.RemoteAddr())

	// Register connection
	p.mu.Lock()
	p.active[connID] = localConn
	p.mu.Unlock()

	defer func() {
		localConn.Close()
		p.mu.Lock()
		delete(p.active, connID)
		p.mu.Unlock()
		p.logger.Info("Connection closed", "id", connID)
	}()

	// Connect to Controller TCP Proxy with timeout
	p.logger.Info("Connecting to proxy", "id", connID, "addr", p.proxyAddr)

	// Use DataPlaneClient SDK to establish connection (encapsulates protocol details)
	dataPlaneClient := tunnel.NewDataPlaneClient(p.proxyAddr, p.tlsConfig)
	proxyConn, err := dataPlaneClient.Connect(p.tunnelID)
	if err != nil {
		p.logger.Error("Failed to connect to proxy", "id", connID, "error", err)
		return
	}
	defer proxyConn.Close()

	p.logger.Info("Proxy connection established", "id", connID)

	// Bidirectional data forwarding
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errChan := make(chan error, 2)

	// Local -> Proxy (upstream)
	go func() {
		n, err := io.Copy(proxyConn, localConn)
		p.logger.Debug("Upstream transfer completed", "id", connID, "bytes", n)
		errChan <- err
	}()

	// Proxy -> Local (downstream)
	go func() {
		n, err := io.Copy(localConn, proxyConn)
		p.logger.Debug("Downstream transfer completed", "id", connID, "bytes", n)
		errChan <- err
	}()

	// Wait for either direction to complete or context cancel
	select {
	case err := <-errChan:
		if err != nil && err != io.EOF {
			p.logger.Error("Data transfer error", "id", connID, "error", err)
		}
	case <-ctx.Done():
		p.logger.Info("Connection cancelled", "id", connID)
	case <-p.shutdown:
		p.logger.Info("Connection shutdown", "id", connID)
	}
}

// monitorStats periodically logs connection statistics
func (p *IHProxy) monitorStats() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			p.mu.Lock()
			activeCount := len(p.active)
			totalCount := p.connCount
			p.mu.Unlock()

			if activeCount > 0 || totalCount > 0 {
				p.logger.Info("Connection stats",
					"active", activeCount,
					"total", totalCount)
			}

		case <-p.shutdown:
			return
		}
	}
}

// ==== step-08: 新增方法 ====

// handshake 执行证书握手，获取session token
func (p *IHProxy) handshake(fingerprint string) error {
	p.logger.Info("Starting handshake", "controller", p.controllerURL)

	// 构造握手请求
	reqBody := map[string]interface{}{
		"type":        "handshake_request",
		"fingerprint": fingerprint,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	// 发送POST请求
	req, err := http.NewRequest("POST", p.controllerURL+"/api/v1/handshake", bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("handshake failed: status=%d, body=%s", resp.StatusCode, string(body))
	}

	// 解析响应
	var handshakeResp struct {
		Type         string `json:"type"`
		Status       string `json:"status"`
		SessionToken string `json:"session_token"`
		ExpiresAt    string `json:"expires_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&handshakeResp); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	// 存储session token
	p.sessionToken = handshakeResp.SessionToken
	p.logger.Info("Handshake successful",
		"token", p.sessionToken[:16]+"...",
		"expires_at", handshakeResp.ExpiresAt)

	return nil
}

// queryPolicies 查询客户端授权策略
func (p *IHProxy) queryPolicies() error {
	p.logger.Info("Querying policies", "controller", p.controllerURL)

	// 构造请求 (假设clientID从fingerprint派生，实际应从握手响应获取)
	req, err := http.NewRequest("GET", p.controllerURL+"/api/v1/policies", nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.sessionToken)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("query policies failed: status=%d, body=%s", resp.StatusCode, string(body))
	}

	// 解析策略列表
	var policyResp struct {
		Type     string           `json:"type"`
		Status   string           `json:"status"`
		Policies []*policy.Policy `json:"policies"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&policyResp); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	p.policies = policyResp.Policies
	p.logger.Info("Policies retrieved", "count", len(p.policies))

	// 打印策略详情
	for i, pol := range p.policies {
		p.logger.Info("Policy details",
			"index", i,
			"policy_id", pol.PolicyID,
			"service_id", pol.ServiceID)
		// Note: TargetHost/Port 现在从 ServiceConfig 获取，而非 Policy
	}

	return nil
}

// createTunnel 创建隧道
func (p *IHProxy) createTunnel(serviceID string) (string, error) {
	p.logger.Info("Creating tunnel", "service_id", serviceID)

	// 构造隧道创建请求（session_token 必须在 body 中）
	reqBody := map[string]interface{}{
		"session_token": p.sessionToken,
		"service_id":    serviceID,
		"local_port":    8080,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", p.controllerURL+"/api/v1/tunnels", bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("create tunnel failed: status=%d, body=%s", resp.StatusCode, string(body))
	}

	// 解析响应
	var tunnelResp struct {
		Type      string `json:"type"`
		Status    string `json:"status"`
		TunnelID  string `json:"tunnel_id"`
		ExpiresAt string `json:"expires_at,omitempty"`
		// Note: TargetHost/Port 不在 Tunnel 响应中，应从 ServiceConfig 获取
	}
	if err := json.NewDecoder(resp.Body).Decode(&tunnelResp); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}

	p.logger.Info("Tunnel created",
		"tunnel_id", tunnelResp.TunnelID,
		"service_id", serviceID,
		"expires_at", tunnelResp.ExpiresAt)

	return tunnelResp.TunnelID, nil
}
