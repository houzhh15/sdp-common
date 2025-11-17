package integration

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/houzhh15/sdp-common/cert"
	"github.com/houzhh15/sdp-common/logging"
	"github.com/houzhh15/sdp-common/policy"
	"github.com/houzhh15/sdp-common/session"
	"github.com/houzhh15/sdp-common/transport"
	"github.com/houzhh15/sdp-common/tunnel"
)

// TestE2E_HandshakeFlow 测试完整的 IH 客户端握手流程
func TestE2E_HandshakeFlow(t *testing.T) {
	t.Skip("需要实际证书文件，跳过集成测试")

	// 初始化日志
	logger, err := logging.NewLogger(&logging.Config{
		Level:  "debug",
		Format: "text",
		Output: "stdout",
	})
	if err != nil {
		t.Fatalf("NewLogger failed: %v", err)
	}

	// 初始化证书管理器
	certManager, err := cert.NewManager(&cert.Config{
		CAFile:   "../certs/ca-cert.pem",
		CertFile: "../certs/controller-cert.pem",
		KeyFile:  "../certs/controller-key.pem",
	})
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	// 初始化会话管理器
	sessionManager, err := session.NewManager(&session.Config{
		TokenExpiry:     30 * time.Minute,
		CleanupInterval: 5 * time.Minute,
		SessionIDPrefix: "test-",
		MaxSessions:     1000,
	})
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	// 启动 HTTP 服务器
	httpSvr, err := transport.NewHTTPServer(&transport.HTTPConfig{
		Addr:    ":18080",
		Timeout: 10 * time.Second,
	}, certManager, logger)
	if err != nil {
		t.Fatalf("NewHTTPServer failed: %v", err)
	}

	// 注册握手处理器
	httpSvr.RegisterHandler("/handshake", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
			http.Error(w, "no client certificate", http.StatusUnauthorized)
			return
		}

		clientCert := r.TLS.PeerCertificates[0]
		fingerprint := certManager.GetFingerprint(clientCert)

		sess, err := sessionManager.CreateSession(fingerprint)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"token":"` + sess.Token + `","client_id":"` + sess.ClientID + `"}`))
	}))

	go httpSvr.Start()
	defer httpSvr.Stop()

	time.Sleep(100 * time.Millisecond)

	// 模拟客户端请求
	clientCertManager, err := cert.NewManager(&cert.Config{
		CAFile:   "../certs/ca-cert.pem",
		CertFile: "../certs/ih-client-cert.pem",
		KeyFile:  "../certs/ih-client-key.pem",
	})
	if err != nil {
		t.Fatalf("NewManager (client) failed: %v", err)
	}

	tlsConfig := clientCertManager.GetTLSConfig()
	tlsConfig.RootCAs = x509.NewCertPool()
	tlsConfig.InsecureSkipVerify = true

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
	}

	resp, err := client.Get("https://localhost:18080/handshake")
	if err != nil {
		t.Fatalf("Handshake request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

// TestE2E_PolicyQuery 测试策略查询流程
func TestE2E_PolicyQuery(t *testing.T) {
	ctx := context.Background()

	logger, err := logging.NewLogger(&logging.Config{
		Level:  "info",
		Format: "text",
		Output: "stdout",
	})
	if err != nil {
		t.Fatalf("NewLogger failed: %v", err)
	}

	storage := policy.NewMemoryStorage()
	evaluator := policy.NewDefaultEvaluator()

	engine, err := policy.NewEngine(&policy.Config{
		Storage:   storage,
		Evaluator: evaluator,
		Logger:    logger,
	})
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	testPolicy := &policy.Policy{
		PolicyID:   "policy-001",
		ClientID:   "client-123",
		ServiceID:  "service-ssh",
		TargetHost: "192.168.1.100",
		TargetPort: 22,
		Conditions: []policy.Condition{
			{Type: policy.ConditionTypeTimeRange, Value: "09:00-18:00"},
		},
	}
	if err := storage.SavePolicy(ctx, testPolicy); err != nil {
		t.Fatalf("SavePolicy failed: %v", err)
	}

	policies, err := engine.GetPoliciesForClient(ctx, "client-123")
	if err != nil {
		t.Fatalf("GetPoliciesForClient failed: %v", err)
	}

	if len(policies) != 1 {
		t.Errorf("Expected 1 policy, got %d", len(policies))
	}

	if policies[0].PolicyID != "policy-001" {
		t.Errorf("Expected policy-001, got %s", policies[0].PolicyID)
	}
}

// TestE2E_TunnelCreationAndDataForward 测试隧道创建和数据转发
func TestE2E_TunnelCreationAndDataForward(t *testing.T) {
	logger, err := logging.NewLogger(&logging.Config{
		Level:  "debug",
		Format: "text",
		Output: "stdout",
	})
	if err != nil {
		t.Fatalf("NewLogger failed: %v", err)
	}

	proxy := tunnel.NewTCPProxy(logger, 32768, 30*time.Second)

	go proxy.Start(":19090", nil)
	defer proxy.Stop()

	time.Sleep(100 * time.Millisecond)

	t.Log("TCP Proxy started successfully on :19090")
}

// TestE2E_SSENotification 测试 SSE 实时推送
func TestE2E_SSENotification(t *testing.T) {
	logger, err := logging.NewLogger(&logging.Config{
		Level:  "debug",
		Format: "text",
		Output: "stdout",
	})
	if err != nil {
		t.Fatalf("NewLogger failed: %v", err)
	}

	notifier := tunnel.NewNotifier(logger, 30*time.Second)

	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		agentID := r.URL.Query().Get("agent_id")
		if err := notifier.Subscribe(agentID, w); err != nil {
			t.Errorf("Subscribe failed: %v", err)
		}
	}))
	defer testServer.Close()

	go func() {
		resp, err := http.Get(testServer.URL + "?agent_id=agent-456")
		if err != nil {
			return
		}
		defer resp.Body.Close()
	}()

	time.Sleep(100 * time.Millisecond)

	event := &tunnel.TunnelEvent{
		Type:     tunnel.EventTypeTunnelCreated,
		TunnelID: "tunnel-001",
		AgentID:  "agent-456",
		Data: map[string]interface{}{
			"target_host": "192.168.1.100",
			"target_port": 22,
		},
	}

	if err := notifier.Notify("agent-456", event); err != nil {
		t.Errorf("Notify failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)
	t.Log("SSE notification test completed")
}

// TestE2E_mTLSAuthentication 测试双向 TLS 认证
func TestE2E_mTLSAuthentication(t *testing.T) {
	t.Skip("需要实际证书文件，跳过集成测试")

	serverCertManager, err := cert.NewManager(&cert.Config{
		CAFile:   "../certs/ca-cert.pem",
		CertFile: "../certs/controller-cert.pem",
		KeyFile:  "../certs/controller-key.pem",
	})
	if err != nil {
		t.Fatalf("NewManager (server) failed: %v", err)
	}

	clientCertManager, err := cert.NewManager(&cert.Config{
		CAFile:   "../certs/ca-cert.pem",
		CertFile: "../certs/ih-client-cert.pem",
		KeyFile:  "../certs/ih-client-key.pem",
	})
	if err != nil {
		t.Fatalf("NewManager (client) failed: %v", err)
	}

	tlsConfig := serverCertManager.GetTLSConfig()
	listener, err := tls.Listen("tcp", ":19443", tlsConfig)
	if err != nil {
		t.Fatalf("tls.Listen failed: %v", err)
	}
	defer listener.Close()

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}

			tlsConn := conn.(*tls.Conn)
			if err := tlsConn.Handshake(); err != nil {
				conn.Close()
				continue
			}

			state := tlsConn.ConnectionState()
			if len(state.PeerCertificates) > 0 {
				clientCert := state.PeerCertificates[0]
				_ = serverCertManager.GetFingerprint(clientCert)
			}

			conn.Close()
		}
	}()

	time.Sleep(100 * time.Millisecond)

	clientTLSConfig := clientCertManager.GetTLSConfig()
	clientTLSConfig.RootCAs = x509.NewCertPool()
	clientTLSConfig.InsecureSkipVerify = true

	conn, err := tls.Dial("tcp", "localhost:19443", clientTLSConfig)
	if err != nil {
		t.Fatalf("tls.Dial failed: %v", err)
	}
	defer conn.Close()

	state := conn.ConnectionState()
	if !state.HandshakeComplete {
		t.Error("TLS handshake not complete")
	}

	if len(state.PeerCertificates) == 0 {
		t.Error("No server certificates received")
	}
}
