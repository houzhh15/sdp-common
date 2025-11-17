package policy

import (
	"context"
	"fmt"
	"time"

	"github.com/houzhh15/sdp-common/logging"
)

// Engine 策略引擎（扩展原 Engine，分离关注点）
type Engine struct {
	storage   Storage   // 存储接口
	evaluator Evaluator // 评估接口
	logger    logging.Logger
}

// Config 引擎配置
type Config struct {
	Storage   Storage
	Evaluator Evaluator
	Logger    logging.Logger
}

// NewEngine 创建策略引擎（重构原 NewEngine，支持依赖注入）
func NewEngine(cfg *Config) (*Engine, error) {
	if cfg.Storage == nil {
		return nil, fmt.Errorf("storage is required")
	}
	if cfg.Evaluator == nil {
		cfg.Evaluator = NewDefaultEvaluator()
	}

	return &Engine{
		storage:   cfg.Storage,
		evaluator: cfg.Evaluator,
		logger:    cfg.Logger,
	}, nil
}

// GetPoliciesForClient 获取客户端的策略列表（复用 Engine.GetPolicies 逻辑）
func (e *Engine) GetPoliciesForClient(ctx context.Context, clientID string) ([]*Policy, error) {
	filter := &PolicyFilter{
		ClientID: clientID,
		Active:   true, // 仅返回有效策略
	}

	policies, err := e.storage.QueryPolicies(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("query policies: %w", err)
	}

	e.logDebug("Get policies for client", map[string]interface{}{
		"client_id": clientID,
		"count":     len(policies),
	})

	return policies, nil
}

// EvaluateAccess 评估访问请求（重构 Engine.CheckAccess，集成 Evaluator）
func (e *Engine) EvaluateAccess(ctx context.Context, req *AccessRequest) (*AccessDecision, error) {
	// 1. 查询客户端的策略
	policies, err := e.GetPoliciesForClient(ctx, req.ClientID)
	if err != nil {
		return nil, fmt.Errorf("get policies: %w", err)
	}

	if len(policies) == 0 {
		return &AccessDecision{
			Allowed: false,
			Reason:  "no policy found for client",
		}, nil
	}

	// 2. 构造评估上下文
	evalCtx := &EvalContext{
		Request:   req,
		Timestamp: req.Timestamp,
	}
	if evalCtx.Timestamp.IsZero() {
		evalCtx.Timestamp = time.Now()
	}

	// 3. 遍历策略，找到第一个匹配的
	for _, policy := range policies {
		// 检查 ServiceID 匹配
		if policy.ServiceID != req.ServiceID {
			continue
		}

		// 评估策略
		allowed, err := e.evaluator.Evaluate(ctx, policy, evalCtx)
		if err != nil {
			e.logError("Evaluate policy failed", err, map[string]interface{}{
				"policy_id": policy.PolicyID,
				"client_id": req.ClientID,
			})
			continue
		}

		if allowed {
			// 策略匹配，允许访问
			decision := &AccessDecision{
				Allowed: true,
				Reason:  "policy matched",
				Policy:  policy,
				Constraints: &AccessConstraints{
					BandwidthLimit:   policy.BandwidthLimit,
					ConcurrencyLimit: policy.ConcurrencyLimit,
					ExpiresAt:        policy.ExpiryTime,
				},
			}

			e.logInfo("Access granted", map[string]interface{}{
				"client_id":  req.ClientID,
				"service_id": req.ServiceID,
				"policy_id":  policy.PolicyID,
			})

			return decision, nil
		}
	}

	// 没有匹配的策略
	e.logInfo("Access denied", map[string]interface{}{
		"client_id":  req.ClientID,
		"service_id": req.ServiceID,
		"reason":     "no matching policy",
	})

	return &AccessDecision{
		Allowed: false,
		Reason:  "no matching policy",
	}, nil
}

// LoadPolicies 批量加载策略（新增）
func (e *Engine) LoadPolicies(ctx context.Context, policies []*Policy) error {
	for _, policy := range policies {
		if err := e.storage.SavePolicy(ctx, policy); err != nil {
			return fmt.Errorf("save policy %s: %w", policy.PolicyID, err)
		}
	}

	e.logInfo("Policies loaded", map[string]interface{}{
		"count": len(policies),
	})

	return nil
}

// SavePolicy 保存策略
func (e *Engine) SavePolicy(ctx context.Context, policy *Policy) error {
	// 设置时间戳
	if policy.CreatedAt.IsZero() {
		policy.CreatedAt = time.Now()
	}
	policy.UpdatedAt = time.Now()

	if err := e.storage.SavePolicy(ctx, policy); err != nil {
		return fmt.Errorf("save policy: %w", err)
	}

	e.logInfo("Policy saved", map[string]interface{}{
		"policy_id": policy.PolicyID,
		"client_id": policy.ClientID,
	})

	return nil
}

// GetPolicy 获取策略
func (e *Engine) GetPolicy(ctx context.Context, policyID string) (*Policy, error) {
	policy, err := e.storage.GetPolicy(ctx, policyID)
	if err != nil {
		return nil, fmt.Errorf("get policy: %w", err)
	}

	return policy, nil
}

// DeletePolicy 删除策略
func (e *Engine) DeletePolicy(ctx context.Context, policyID string) error {
	if err := e.storage.DeletePolicy(ctx, policyID); err != nil {
		return fmt.Errorf("delete policy: %w", err)
	}

	e.logInfo("Policy deleted", map[string]interface{}{
		"policy_id": policyID,
	})

	return nil
}

// 日志辅助方法
func (e *Engine) logInfo(msg string, fields ...interface{}) {
	if e.logger != nil {
		e.logger.Info(msg, fields...)
	}
}

func (e *Engine) logDebug(msg string, fields ...interface{}) {
	if e.logger != nil {
		e.logger.Debug(msg, fields...)
	}
}

func (e *Engine) logError(msg string, fields ...interface{}) {
	if e.logger != nil {
		e.logger.Error(msg, fields...)
	}
}

func (e *Engine) logWarn(msg string, fields ...interface{}) {
	if e.logger != nil {
		e.logger.Warn(msg, fields...)
	}
}
