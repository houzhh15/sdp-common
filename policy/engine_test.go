package policy

import (
	"context"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// mockLogger 模拟日志记录器
type mockLogger struct{}

func (l *mockLogger) Info(msg string, fields ...interface{})  {}
func (l *mockLogger) Warn(msg string, fields ...interface{})  {}
func (l *mockLogger) Error(msg string, fields ...interface{}) {}
func (l *mockLogger) Debug(msg string, fields ...interface{}) {}

// setupTestDB 创建测试数据库
func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Open test database failed: %v", err)
	}
	return db
}

// TestDBStorage 测试数据库存储
func TestDBStorage(t *testing.T) {
	db := setupTestDB(t)
	storage, err := NewDBStorage(db)
	if err != nil {
		t.Fatalf("NewDBStorage failed: %v", err)
	}

	ctx := context.Background()

	// 测试保存策略
	policy := &Policy{
		PolicyID:         "policy-001",
		ClientID:         "client-001",
		ServiceID:        "service-001",
		BandwidthLimit:   1000000,
		ConcurrencyLimit: 10,
		ExpiryTime:       time.Now().Add(24 * time.Hour),
		Conditions: []*Condition{
			{
				Type:     "device_os",
				Operator: "eq",
				Value:    "Linux",
			},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := storage.SavePolicy(ctx, policy); err != nil {
		t.Fatalf("SavePolicy failed: %v", err)
	}

	// 测试获取策略
	retrieved, err := storage.GetPolicy(ctx, "policy-001")
	if err != nil {
		t.Fatalf("GetPolicy failed: %v", err)
	}

	if retrieved.PolicyID != policy.PolicyID {
		t.Errorf("Expected PolicyID %s, got %s", policy.PolicyID, retrieved.PolicyID)
	}

	if len(retrieved.Conditions) != 1 {
		t.Errorf("Expected 1 condition, got %d", len(retrieved.Conditions))
	}

	// 测试查询策略
	filter := &PolicyFilter{
		ClientID: "client-001",
		Active:   true,
	}
	policies, err := storage.QueryPolicies(ctx, filter)
	if err != nil {
		t.Fatalf("QueryPolicies failed: %v", err)
	}

	if len(policies) != 1 {
		t.Errorf("Expected 1 policy, got %d", len(policies))
	}

	// 测试删除策略
	if err := storage.DeletePolicy(ctx, "policy-001"); err != nil {
		t.Fatalf("DeletePolicy failed: %v", err)
	}

	// 验证已删除
	_, err = storage.GetPolicy(ctx, "policy-001")
	if err == nil {
		t.Error("Expected error for deleted policy, got nil")
	}
}

// TestDefaultEvaluator 测试默认评估器
func TestDefaultEvaluator(t *testing.T) {
	evaluator := NewDefaultEvaluator()
	ctx := context.Background()

	// 测试过期策略
	t.Run("ExpiredPolicy", func(t *testing.T) {
		policy := &Policy{
			PolicyID:   "policy-002",
			ExpiryTime: time.Now().Add(-1 * time.Hour), // 1小时前过期
		}

		evalCtx := &EvalContext{
			Timestamp: time.Now(),
		}

		allowed, err := evaluator.Evaluate(ctx, policy, evalCtx)
		if err != nil {
			t.Fatalf("Evaluate failed: %v", err)
		}

		if allowed {
			t.Error("Expected expired policy to be denied")
		}
	})

	// 测试设备 OS 条件
	t.Run("DeviceOSCondition", func(t *testing.T) {
		policy := &Policy{
			PolicyID:   "policy-003",
			ExpiryTime: time.Now().Add(24 * time.Hour),
			Conditions: []*Condition{
				{
					Type:     "device_os",
					Operator: "eq",
					Value:    "Linux",
				},
			},
		}

		evalCtx := &EvalContext{
			Request: &AccessRequest{
				DeviceInfo: &DeviceInfo{
					OS: "Linux",
				},
			},
			Timestamp: time.Now(),
		}

		allowed, err := evaluator.Evaluate(ctx, policy, evalCtx)
		if err != nil {
			t.Fatalf("Evaluate failed: %v", err)
		}

		if !allowed {
			t.Error("Expected matching device OS to be allowed")
		}

		// 测试不匹配的 OS
		evalCtx.Request.DeviceInfo.OS = "Windows"
		allowed, err = evaluator.Evaluate(ctx, policy, evalCtx)
		if err != nil {
			t.Fatalf("Evaluate failed: %v", err)
		}

		if allowed {
			t.Error("Expected non-matching device OS to be denied")
		}
	})

	// 测试设备合规性条件
	t.Run("DeviceComplianceCondition", func(t *testing.T) {
		policy := &Policy{
			PolicyID:   "policy-004",
			ExpiryTime: time.Now().Add(24 * time.Hour),
			Conditions: []*Condition{
				{
					Type:     "device_compliance",
					Operator: "eq",
					Value:    true,
				},
			},
		}

		evalCtx := &EvalContext{
			Request: &AccessRequest{
				DeviceInfo: &DeviceInfo{
					Compliance: true,
				},
			},
			Timestamp: time.Now(),
		}

		allowed, err := evaluator.Evaluate(ctx, policy, evalCtx)
		if err != nil {
			t.Fatalf("Evaluate failed: %v", err)
		}

		if !allowed {
			t.Error("Expected compliant device to be allowed")
		}
	})

	// 测试时间范围条件
	t.Run("TimeRangeCondition", func(t *testing.T) {
		now := time.Now()
		startTime := now.Add(-1 * time.Hour)
		endTime := now.Add(1 * time.Hour)

		policy := &Policy{
			PolicyID:   "policy-005",
			ExpiryTime: time.Now().Add(24 * time.Hour),
			Conditions: []*Condition{
				{
					Type:     "time_range",
					Operator: "between",
					Value:    []interface{}{startTime.Format(time.RFC3339), endTime.Format(time.RFC3339)},
				},
			},
		}

		evalCtx := &EvalContext{
			Request:   &AccessRequest{},
			Timestamp: now,
		}

		allowed, err := evaluator.Evaluate(ctx, policy, evalCtx)
		if err != nil {
			t.Fatalf("Evaluate failed: %v", err)
		}

		if !allowed {
			t.Error("Expected time within range to be allowed")
		}
	})
}

// TestEngine 测试策略引擎
func TestEngine(t *testing.T) {
	db := setupTestDB(t)
	storage, err := NewDBStorage(db)
	if err != nil {
		t.Fatalf("NewDBStorage failed: %v", err)
	}

	engine, err := NewEngine(&Config{
		Storage:   storage,
		Evaluator: NewDefaultEvaluator(),
		Logger:    &mockLogger{},
	})
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	ctx := context.Background()

	// 测试保存策略
	policy := &Policy{
		PolicyID:         "policy-010",
		ClientID:         "client-010",
		ServiceID:        "service-010",
		BandwidthLimit:   1000000,
		ConcurrencyLimit: 10,
		ExpiryTime:       time.Now().Add(24 * time.Hour),
	}

	if err := engine.SavePolicy(ctx, policy); err != nil {
		t.Fatalf("SavePolicy failed: %v", err)
	}

	// 测试获取客户端策略
	policies, err := engine.GetPoliciesForClient(ctx, "client-010")
	if err != nil {
		t.Fatalf("GetPoliciesForClient failed: %v", err)
	}

	if len(policies) != 1 {
		t.Errorf("Expected 1 policy, got %d", len(policies))
	}

	// 测试评估访问
	req := &AccessRequest{
		ClientID:  "client-010",
		ServiceID: "service-010",
		SourceIP:  "192.168.1.50",
		Timestamp: time.Now(),
	}

	decision, err := engine.EvaluateAccess(ctx, req)
	if err != nil {
		t.Fatalf("EvaluateAccess failed: %v", err)
	}

	if !decision.Allowed {
		t.Errorf("Expected access to be allowed, got denied: %s", decision.Reason)
	}

	if decision.Policy == nil {
		t.Error("Expected policy in decision, got nil")
	}

	if decision.Constraints == nil {
		t.Error("Expected constraints in decision, got nil")
	}

	// 测试访问不存在的服务
	req.ServiceID = "non-existent-service"
	decision, err = engine.EvaluateAccess(ctx, req)
	if err != nil {
		t.Fatalf("EvaluateAccess failed: %v", err)
	}

	if decision.Allowed {
		t.Error("Expected access to be denied for non-existent service")
	}

	// 测试删除策略
	if err := engine.DeletePolicy(ctx, "policy-010"); err != nil {
		t.Fatalf("DeletePolicy failed: %v", err)
	}

	// 验证已删除
	policies, err = engine.GetPoliciesForClient(ctx, "client-010")
	if err != nil {
		t.Fatalf("GetPoliciesForClient failed: %v", err)
	}

	if len(policies) != 0 {
		t.Errorf("Expected 0 policies after deletion, got %d", len(policies))
	}
}

// TestLoadPolicies 测试批量加载策略
func TestLoadPolicies(t *testing.T) {
	db := setupTestDB(t)
	storage, err := NewDBStorage(db)
	if err != nil {
		t.Fatalf("NewDBStorage failed: %v", err)
	}

	engine, err := NewEngine(&Config{
		Storage: storage,
		Logger:  &mockLogger{},
	})
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	ctx := context.Background()

	// 准备批量策略
	policies := []*Policy{
		{
			PolicyID:         "policy-020",
			ClientID:         "client-020",
			ServiceID:        "service-020",
			BandwidthLimit:   1000000,
			ConcurrencyLimit: 5,
			ExpiryTime:       time.Now().Add(24 * time.Hour),
		},
		{
			PolicyID:         "policy-021",
			ClientID:         "client-020",
			ServiceID:        "service-021",
			BandwidthLimit:   2000000,
			ConcurrencyLimit: 10,
			ExpiryTime:       time.Now().Add(24 * time.Hour),
		},
	}

	// 批量加载
	if err := engine.LoadPolicies(ctx, policies); err != nil {
		t.Fatalf("LoadPolicies failed: %v", err)
	}

	// 验证加载结果
	clientPolicies, err := engine.GetPoliciesForClient(ctx, "client-020")
	if err != nil {
		t.Fatalf("GetPoliciesForClient failed: %v", err)
	}

	if len(clientPolicies) != 2 {
		t.Errorf("Expected 2 policies, got %d", len(clientPolicies))
	}
}
