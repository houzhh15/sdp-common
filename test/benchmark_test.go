package benchmark

import (
	"context"
	"crypto/x509"
	"testing"
	"time"

	"github.com/houzhh15/sdp-common/cert"
	"github.com/houzhh15/sdp-common/logging"
	"github.com/houzhh15/sdp-common/policy"
	"github.com/houzhh15/sdp-common/session"
)

// BenchmarkCertManager_GetFingerprint 测试证书指纹计算性能
func BenchmarkCertManager_GetFingerprint(b *testing.B) {
	certManager, err := cert.NewManager(&cert.Config{
		CAFile:   "../certs/ca-cert.pem",
		CertFile: "../certs/controller-cert.pem",
		KeyFile:  "../certs/controller-key.pem",
	})
	if err != nil {
		b.Skip("证书文件不存在，跳过基准测试")
		return
	}

	clientCert := &x509.Certificate{
		SerialNumber: []byte{1, 2, 3, 4, 5},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = certManager.GetFingerprint(clientCert)
	}
}

// BenchmarkSessionManager_CreateSession 测试会话创建性能
func BenchmarkSessionManager_CreateSession(b *testing.B) {
	logger, _ := logging.NewLogger(&logging.Config{
		Level:  "error",
		Format: "text",
		Output: "stdout",
	})

	sessionManager := session.NewManager(&session.Config{
		Expiry:          30 * time.Minute,
		CleanupInterval: 5 * time.Minute,
	}, logger)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = sessionManager.CreateSession("test-fingerprint")
	}
}

// BenchmarkSessionManager_ValidateSession 测试会话验证性能
func BenchmarkSessionManager_ValidateSession(b *testing.B) {
	logger, _ := logging.NewLogger(&logging.Config{
		Level:  "error",
		Format: "text",
		Output: "stdout",
	})

	sessionManager := session.NewManager(&session.Config{
		Expiry:          30 * time.Minute,
		CleanupInterval: 5 * time.Minute,
	}, logger)

	sess, _ := sessionManager.CreateSession("test-fingerprint")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = sessionManager.ValidateSession(sess.Token)
	}
}

// BenchmarkPolicyEngine_EvaluateAccess 测试策略评估性能
func BenchmarkPolicyEngine_EvaluateAccess(b *testing.B) {
	ctx := context.Background()

	logger, _ := logging.NewLogger(&logging.Config{
		Level:  "error",
		Format: "text",
		Output: "stdout",
	})

	storage := policy.NewMemStorage()
	evaluator := policy.NewDefaultEvaluator()

	engine, err := policy.NewEngine(&policy.Config{
		Storage:   storage,
		Evaluator: evaluator,
		Logger:    logger,
	})
	if err != nil {
		b.Fatalf("NewEngine failed: %v", err)
	}

	testPolicy := &policy.Policy{
		PolicyID:   "policy-001",
		ClientID:   "client-123",
		ServiceID:  "service-ssh",
		TargetHost: "192.168.1.100",
		TargetPort: 22,
		Conditions: []*policy.Condition{
			{Type: "time_range", Value: "09:00-18:00"},
		},
	}
	storage.SavePolicy(ctx, testPolicy)

	request := &policy.AccessRequest{
		ClientID:  "client-123",
		ServiceID: "service-ssh",
		Timestamp: time.Now(),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = engine.EvaluateAccess(ctx, request)
	}
}

// BenchmarkConfigLoader_Load 测试配置加载性能
func BenchmarkConfigLoader_Load(b *testing.B) {
	b.Skip("需要实际配置文件")
}

// BenchmarkLogger_Info 测试日志记录性能
func BenchmarkLogger_Info(b *testing.B) {
	logger, _ := logging.NewLogger(&logging.Config{
		Level:  "info",
		Format: "json",
		Output: "stdout",
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.Info("test message", "key", "value")
	}
}

// BenchmarkSessionManager_ConcurrentCreate 测试并发会话创建性能
func BenchmarkSessionManager_ConcurrentCreate(b *testing.B) {
	logger, _ := logging.NewLogger(&logging.Config{
		Level:  "error",
		Format: "text",
		Output: "stdout",
	})

	sessionManager := session.NewManager(&session.Config{
		Expiry:          30 * time.Minute,
		CleanupInterval: 5 * time.Minute,
	}, logger)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			i++
			_, _ = sessionManager.CreateSession("test-fingerprint")
		}
	})
}

// BenchmarkPolicyEngine_ConcurrentEvaluate 测试并发策略评估性能
func BenchmarkPolicyEngine_ConcurrentEvaluate(b *testing.B) {
	ctx := context.Background()

	logger, _ := logging.NewLogger(&logging.Config{
		Level:  "error",
		Format: "text",
		Output: "stdout",
	})

	storage := policy.NewMemStorage()
	evaluator := policy.NewDefaultEvaluator()

	engine, err := policy.NewEngine(&policy.Config{
		Storage:   storage,
		Evaluator: evaluator,
		Logger:    logger,
	})
	if err != nil {
		b.Fatalf("NewEngine failed: %v", err)
	}

	testPolicy := &policy.Policy{
		PolicyID:   "policy-001",
		ClientID:   "client-123",
		ServiceID:  "service-ssh",
		TargetHost: "192.168.1.100",
		TargetPort: 22,
		Conditions: []*policy.Condition{
			{Type: "time_range", Value: "09:00-18:00"},
		},
	}
	storage.SavePolicy(ctx, testPolicy)

	request := &policy.AccessRequest{
		ClientID:  "client-123",
		ServiceID: "service-ssh",
		Timestamp: time.Now(),
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = engine.EvaluateAccess(ctx, request)
		}
	})
}
