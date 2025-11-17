package policy

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"
)

// Evaluator 策略评估器接口（新增）
type Evaluator interface {
	Evaluate(ctx context.Context, policy *Policy, evalCtx *EvalContext) (bool, error)
}

// DefaultEvaluator 默认评估器（重构 Engine.CheckAccess 逻辑）
type DefaultEvaluator struct{}

// NewDefaultEvaluator 创建默认评估器
func NewDefaultEvaluator() *DefaultEvaluator {
	return &DefaultEvaluator{}
}

// Evaluate 评估策略（复用 Engine.GetPolicies 的过期检查 + 新增条件评估）
func (e *DefaultEvaluator) Evaluate(ctx context.Context, policy *Policy, evalCtx *EvalContext) (bool, error) {
	// 1. 检查过期时间（复用现有逻辑）
	if !policy.ExpiryTime.IsZero() && evalCtx.Timestamp.After(policy.ExpiryTime) {
		return false, nil
	}

	// 2. 评估条件列表（新增）
	if len(policy.Conditions) > 0 {
		for _, cond := range policy.Conditions {
			ok, err := e.evaluateCondition(cond, evalCtx)
			if err != nil {
				return false, fmt.Errorf("evaluate condition %s: %w", cond.Type, err)
			}
			if !ok {
				return false, nil
			}
		}
	}

	return true, nil
}

// evaluateCondition 评估单个条件
func (e *DefaultEvaluator) evaluateCondition(cond *Condition, evalCtx *EvalContext) (bool, error) {
	switch cond.Type {
	case "device_os":
		return e.evaluateDeviceOS(cond, evalCtx)
	case "geo_location":
		return e.evaluateGeoLocation(cond, evalCtx)
	case "time_range":
		return e.evaluateTimeRange(cond, evalCtx)
	case "device_compliance":
		return e.evaluateDeviceCompliance(cond, evalCtx)
	default:
		return false, fmt.Errorf("unsupported condition type: %s", cond.Type)
	}
}

// evaluateDeviceOS 评估设备操作系统
func (e *DefaultEvaluator) evaluateDeviceOS(cond *Condition, evalCtx *EvalContext) (bool, error) {
	if evalCtx.Request == nil || evalCtx.Request.DeviceInfo == nil {
		return false, nil
	}

	deviceOS := evalCtx.Request.DeviceInfo.OS

	switch cond.Operator {
	case "eq":
		expectedOS, ok := cond.Value.(string)
		if !ok {
			return false, fmt.Errorf("invalid value type for eq operator")
		}
		return strings.EqualFold(deviceOS, expectedOS), nil

	case "in":
		allowedOSList, ok := cond.Value.([]interface{})
		if !ok {
			return false, fmt.Errorf("invalid value type for in operator")
		}
		for _, os := range allowedOSList {
			if osStr, ok := os.(string); ok && strings.EqualFold(deviceOS, osStr) {
				return true, nil
			}
		}
		return false, nil

	case "ne":
		expectedOS, ok := cond.Value.(string)
		if !ok {
			return false, fmt.Errorf("invalid value type for ne operator")
		}
		return !strings.EqualFold(deviceOS, expectedOS), nil

	default:
		return false, fmt.Errorf("unsupported operator for device_os: %s", cond.Operator)
	}
}

// evaluateGeoLocation 评估地理位置（示例实现）
func (e *DefaultEvaluator) evaluateGeoLocation(cond *Condition, evalCtx *EvalContext) (bool, error) {
	// 实际实现需要集成 GeoIP 库
	// 这里仅做示例
	if evalCtx.Request == nil || evalCtx.Request.SourceIP == "" {
		return false, nil
	}

	switch cond.Operator {
	case "in":
		allowedCountries, ok := cond.Value.([]interface{})
		if !ok {
			return false, fmt.Errorf("invalid value type for in operator")
		}
		// TODO: 实现 IP 地理位置查询
		// country := geoip.Lookup(evalCtx.Request.SourceIP)
		// return contains(allowedCountries, country), nil

		// 简化实现：假设总是返回 true
		return len(allowedCountries) > 0, nil

	default:
		return false, fmt.Errorf("unsupported operator for geo_location: %s", cond.Operator)
	}
}

// evaluateTimeRange 评估时间范围
func (e *DefaultEvaluator) evaluateTimeRange(cond *Condition, evalCtx *EvalContext) (bool, error) {
	switch cond.Operator {
	case "between":
		// Value 应该是 [startTime, endTime] 数组
		timeRange, ok := cond.Value.([]interface{})
		if !ok || len(timeRange) != 2 {
			return false, fmt.Errorf("invalid value type for between operator")
		}

		startTime, err := parseTime(timeRange[0])
		if err != nil {
			return false, fmt.Errorf("parse start time: %w", err)
		}

		endTime, err := parseTime(timeRange[1])
		if err != nil {
			return false, fmt.Errorf("parse end time: %w", err)
		}

		currentTime := evalCtx.Timestamp
		return currentTime.After(startTime) && currentTime.Before(endTime), nil

	default:
		return false, fmt.Errorf("unsupported operator for time_range: %s", cond.Operator)
	}
}

// evaluateDeviceCompliance 评估设备合规性
func (e *DefaultEvaluator) evaluateDeviceCompliance(cond *Condition, evalCtx *EvalContext) (bool, error) {
	if evalCtx.Request == nil || evalCtx.Request.DeviceInfo == nil {
		return false, nil
	}

	switch cond.Operator {
	case "eq":
		expectedCompliance, ok := cond.Value.(bool)
		if !ok {
			return false, fmt.Errorf("invalid value type for eq operator")
		}
		return evalCtx.Request.DeviceInfo.Compliance == expectedCompliance, nil

	default:
		return false, fmt.Errorf("unsupported operator for device_compliance: %s", cond.Operator)
	}
}

// parseTime 解析时间值
func parseTime(val interface{}) (time.Time, error) {
	switch v := val.(type) {
	case string:
		return time.Parse(time.RFC3339, v)
	case time.Time:
		return v, nil
	case int64:
		return time.Unix(v, 0), nil
	case float64:
		return time.Unix(int64(v), 0), nil
	default:
		return time.Time{}, fmt.Errorf("unsupported time type: %s", reflect.TypeOf(val).String())
	}
}
