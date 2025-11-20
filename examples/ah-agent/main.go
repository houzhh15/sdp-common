package main

import (
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
	"syscall"
	"time"

	"github.com/houzhh15/sdp-common/cert"
	"github.com/houzhh15/sdp-common/logging"
	"github.com/houzhh15/sdp-common/tunnel"
)

// AH Agent 示例 - 接受主机代理 (SDP 2.0 规范 0x04 混合方案)
// 功能：
// 1. 从 Controller HTTP GET /api/v1/services 获取初始服务配置（0x04 消息）
// 2. 订阅 Controller 的服务配置变更事件（SSE service_updated）
// 3. 订阅 Controller 的隧道事件（SSE created/deleted）
// 4. 根据 ServiceID 路由到不同的目标服务
// 5. 通过 TCP Proxy 透明转发数据

func main() {
	// 解析命令行参数
	certFile := flag.String("cert", "../../certs/ah-agent-cert.pem", "Certificate file path")
	keyFile := flag.String("key", "../../certs/ah-agent-key.pem", "Private key file path")
	caFile := flag.String("ca", "../../certs/ca-cert.pem", "CA certificate file path")
	controller := flag.String("controller", "https://localhost:8443", "Controller URL")
	agentID := flag.String("agent-id", "ah-agent-001", "Agent ID")
	logLevel := flag.String("log-level", "info", "Log level (debug, info, warn, error)")
	flag.Parse()

	logger, err := logging.NewLogger(&logging.Config{
		Level:  *logLevel,
		Format: "json",
		Output: "stdout",
	})
	if err != nil {
		log.Fatalf("初始化日志失败: %v", err)
	}

	logger.Info("AH Agent 启动 (SDP 2.0 规范 0x04 混合方案)", "version", "1.0.0-example", "agent_id", *agentID)

	// 使用sdp-common的cert包加载证书
	certManager, err := cert.NewManager(&cert.Config{
		CertFile: *certFile,
		KeyFile:  *keyFile,
		CAFile:   *caFile,
	})
	if err != nil {
		logger.Error("加载证书失败", "error", err)
		os.Exit(1)
	}

	// 验证证书有效期
	if err := certManager.ValidateExpiry(); err != nil {
		logger.Error("证书验证失败", "error", err)
		os.Exit(1)
	}

	fingerprint := certManager.GetFingerprint()
	daysUntilExpiry := certManager.DaysUntilExpiry()
	logger.Info("证书加载成功",
		"fingerprint", fingerprint,
		"days_until_expiry", daysUntilExpiry)

	// 获取TLS配置
	tlsConfig := certManager.GetTLSConfig()

	agent := &AHAgent{
		agentID:       *agentID,
		controllerURL: *controller,
		services:      make(map[string]*tunnel.ServiceConfig),
		logger:        logger,
		tlsConfig:     tlsConfig,
		activeTunnels: make(map[string]*activeTunnel),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 混合方案步骤 1: HTTP GET 获取初始服务配置（0x04 消息）
	if err := agent.fetchServiceConfigs(ctx); err != nil {
		logger.Error("获取服务配置失败", "error", err)
		os.Exit(1)
	}

	// 启动订阅器（SSE 实时更新）
	subscriber := tunnel.NewSubscriber(&tunnel.SubscriberConfig{
		ControllerURL: *controller,
		AgentID:       *agentID,
		TLSConfig:     tlsConfig,
		Callback:      agent.handleEvent,
		Logger:        logger,
	})

	if err := subscriber.Start(ctx); err != nil {
		logger.Error("启动订阅器失败", "error", err)
		os.Exit(1)
	}
	defer subscriber.Stop()

	fmt.Printf("\n✅ AH Agent started successfully!\n")
	fmt.Printf("   Controller: %s\n", *controller)
	fmt.Printf("   Agent ID: %s\n", *agentID)
	fmt.Printf("   Registered Services: %d\n", len(agent.services))
	for serviceID, svc := range agent.services {
		fmt.Printf("     - %s → %s:%d\n", serviceID, svc.TargetHost, svc.TargetPort)
	}
	fmt.Printf("   Press Ctrl+C to stop\n\n")

	logger.Info("已连接到Controller", "url", *controller)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	logger.Info("收到退出信号，正在清理...")
	cancel()
	agent.cleanup()
	logger.Info("AH Agent 已停止")
}

type AHAgent struct {
	agentID       string
	controllerURL string
	services      map[string]*tunnel.ServiceConfig // serviceID -> 服务配置
	logger        logging.Logger
	tlsConfig     *tls.Config
	activeTunnels map[string]*activeTunnel
}

type activeTunnel struct {
	tunnelID   string
	serviceID  string // 用于标识服务
	targetHost string
	targetPort int
	proxyConn  net.Conn
	targetConn net.Conn
	cancel     context.CancelFunc
}

// fetchServiceConfigs HTTP GET 获取初始服务配置（混合方案步骤 1）
func (a *AHAgent) fetchServiceConfigs(ctx context.Context) error {
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: a.tlsConfig,
		},
		Timeout: 10 * time.Second,
	}

	url := fmt.Sprintf("%s/api/v1/services", a.controllerURL)
	a.logger.Info("正在获取服务配置", "url", url)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("创建请求失败: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP状态码异常: %d", resp.StatusCode)
	}

	var result struct {
		Status   string                  `json:"status"`
		Services []*tunnel.ServiceConfig `json:"services"`
		Count    int                     `json:"count"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("解析响应失败: %w", err)
	}

	// 保存服务配置
	for _, svc := range result.Services {
		a.services[svc.ServiceID] = svc
		a.logger.Info("加载服务配置",
			"service_id", svc.ServiceID,
			"target", fmt.Sprintf("%s:%d", svc.TargetHost, svc.TargetPort))
	}

	a.logger.Info("服务配置加载完成", "count", len(a.services))
	return nil
}

// handleEvent 处理所有事件（隧道事件和服务配置事件）
func (a *AHAgent) handleEvent(event *tunnel.TunnelEvent) error {
	// 根据事件类型分发
	switch event.Type {
	case tunnel.EventTypeCreated:
		a.handleTunnelCreated(event)
	case tunnel.EventTypeDeleted:
		a.handleTunnelDeleted(event)
	// 服务配置事件处理（通过 Metadata 区分）
	case "service_created", "service_updated":
		a.handleServiceEvent(event)
	default:
		a.logger.Warn("Unknown event type", "type", event.Type)
	}
	return nil
}

// handleServiceEvent 处理服务配置变更事件（混合方案步骤 2：SSE Push）
func (a *AHAgent) handleServiceEvent(event *tunnel.TunnelEvent) {
	// 从 event.Details 中获取服务配置
	if event.Details == nil {
		a.logger.Error("服务配置事件数据为空")
		return
	}

	serviceData, ok := event.Details["service"]
	if !ok {
		a.logger.Error("服务配置数据缺失")
		return
	}

	// 将 map 转换为 ServiceConfig
	serviceJSON, err := json.Marshal(serviceData)
	if err != nil {
		a.logger.Error("序列化服务配置失败", "error", err)
		return
	}

	var svc tunnel.ServiceConfig
	if err := json.Unmarshal(serviceJSON, &svc); err != nil {
		a.logger.Error("反序列化服务配置失败", "error", err)
		return
	}

	// 更新本地服务配置
	a.services[svc.ServiceID] = &svc
	a.logger.Info("服务配置已更新",
		"service_id", svc.ServiceID,
		"target", fmt.Sprintf("%s:%d", svc.TargetHost, svc.TargetPort),
		"event_type", event.Type)
}

func (a *AHAgent) handleTunnelCreated(event *tunnel.TunnelEvent) {
	if event.Tunnel == nil {
		a.logger.Error("隧道事件数据为空")
		return
	}

	tun := event.Tunnel

	// Per SDP 2.0: 根据 ServiceID 查找对应的目标服务
	serviceID := tun.ServiceID
	service, ok := a.services[serviceID]
	if !ok {
		a.logger.Error("未注册的服务",
			"service_id", serviceID,
			"tunnel_id", tun.ID,
			"registered_services", len(a.services))
		return
	}

	// Get Controller data plane address from event details (highest priority)
	var proxyAddr string
	if event.Details != nil {
		if addr, ok := event.Details["controller_addr"].(string); ok {
			proxyAddr = addr
		}
	}

	// Fallback 1: Get TCP Proxy address from tunnel metadata
	if proxyAddr == "" && tun.Metadata != nil {
		if endpoint, ok := tun.Metadata["ah_endpoint"].(string); ok {
			proxyAddr = endpoint
		}
	}

	// Fallback 2: AHEndpoint if metadata not available
	if proxyAddr == "" {
		proxyAddr = tun.AHEndpoint
	}

	if proxyAddr == "" {
		a.logger.Error("TCP Proxy 地址未提供", "tunnel_id", tun.ID)
		return
	}

	a.logger.Info("收到隧道创建通知",
		"tunnel_id", tun.ID,
		"service_id", serviceID,
		"tcp_proxy", proxyAddr,
		"target", fmt.Sprintf("%s:%d", service.TargetHost, service.TargetPort))

	// Per SDP 2.0 Architecture: AH connects to target service (step 1)
	targetAddr := fmt.Sprintf("%s:%d", service.TargetHost, service.TargetPort)
	targetConn, err := net.Dial("tcp", targetAddr)
	if err != nil {
		a.logger.Error("连接目标服务失败", "error", err, "target", targetAddr)
		return
	}

	// Per SDP 2.0 Architecture: AH connects to Controller TCP Proxy with mTLS (step 2)
	// Use DataPlaneClient SDK to establish connection (encapsulates protocol details)
	dataPlaneClient := tunnel.NewDataPlaneClient(proxyAddr, a.tlsConfig)
	proxyConn, err := dataPlaneClient.Connect(tun.ID)
	if err != nil {
		a.logger.Error("连接TCP Proxy失败", "error", err, "addr", proxyAddr)
		targetConn.Close()
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	activeTun := &activeTunnel{
		tunnelID:   tun.ID,
		serviceID:  serviceID,
		targetHost: service.TargetHost,
		targetPort: service.TargetPort,
		proxyConn:  proxyConn,
		targetConn: targetConn,
		cancel:     cancel,
	}
	a.activeTunnels[tun.ID] = activeTun

	// Per SDP 2.0 Architecture: Start bidirectional forwarding (step 3)
	go a.forwardData(ctx, activeTun)

	a.logger.Info("隧道已建立 (SDP 2.0 compliant)", "tunnel_id", tun.ID, "service_id", serviceID, "target", targetAddr, "proxy", proxyAddr)
}

func (a *AHAgent) forwardData(ctx context.Context, tun *activeTunnel) {
	defer func() {
		tun.cancel()
		tun.proxyConn.Close()
		tun.targetConn.Close()
		delete(a.activeTunnels, tun.tunnelID)
		a.logger.Info("隧道已关闭", "tunnel_id", tun.tunnelID)
	}()

	errChan := make(chan error, 2)

	go func() {
		_, err := io.Copy(tun.targetConn, tun.proxyConn)
		errChan <- err
	}()

	go func() {
		_, err := io.Copy(tun.proxyConn, tun.targetConn)
		errChan <- err
	}()

	select {
	case err := <-errChan:
		if err != nil && err != io.EOF {
			a.logger.Error("数据转发错误", "error", err, "tunnel_id", tun.tunnelID)
		}
	case <-ctx.Done():
		a.logger.Info("隧道被取消", "tunnel_id", tun.tunnelID)
	}
}

func (a *AHAgent) handleTunnelDeleted(event *tunnel.TunnelEvent) {
	if event.Tunnel == nil {
		a.logger.Error("隧道事件数据为空")
		return
	}

	tunnelID := event.Tunnel.ID
	a.logger.Info("收到隧道删除通知", "tunnel_id", tunnelID)

	if tun, ok := a.activeTunnels[tunnelID]; ok {
		tun.cancel()
	}
}

func (a *AHAgent) cleanup() {
	for _, tun := range a.activeTunnels {
		tun.cancel()
	}
}
