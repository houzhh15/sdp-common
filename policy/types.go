package policy

import (
	"time"
)

// Policy 策略（扩展原 PolicyEntry）
// 注意：TargetHost/TargetPort 已移除，应从 ServiceConfig 获取服务部署信息
type Policy struct {
	PolicyID         string                 `json:"policy_id" gorm:"uniqueIndex"`
	ClientID         string                 `json:"client_id" gorm:"index"`
	ServiceID        string                 `json:"service_id" gorm:"index"` // 通过 ServiceID 关联到 ServiceConfig
	BandwidthLimit   int64                  `json:"bandwidth_limit"`         // bytes/s
	ConcurrencyLimit int                    `json:"concurrency_limit"`       // 最大并发连接数
	ExpiryTime       time.Time              `json:"expiry_time"`
	Conditions       []*Condition           `json:"conditions,omitempty"` // 新增：策略条件
	Metadata         map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt        time.Time              `json:"created_at"`
	UpdatedAt        time.Time              `json:"updated_at"`
}

// Condition 策略条件（新增）
type Condition struct {
	Type     string      `json:"type"`     // "device_os", "geo_location", "time_range"
	Operator string      `json:"operator"` // "eq", "in", "between", "ne", "not_in"
	Value    interface{} `json:"value"`    // 条件值（可以是字符串、数组、时间等）
}

// PolicyFilter 策略查询过滤器
type PolicyFilter struct {
	ClientID  string
	ServiceID string
	Active    bool // 是否仅查询有效（未过期）策略
}

// AccessRequest 访问请求（新增）
type AccessRequest struct {
	ClientID   string                 `json:"client_id"`
	ServiceID  string                 `json:"service_id"`
	DeviceInfo *DeviceInfo            `json:"device_info,omitempty"`
	SourceIP   string                 `json:"source_ip"`
	Timestamp  time.Time              `json:"timestamp"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// DeviceInfo 设备信息
type DeviceInfo struct {
	DeviceID   string `json:"device_id"`
	OS         string `json:"os"`
	OSVersion  string `json:"os_version"`
	Compliance bool   `json:"compliance"`
}

// AccessDecision 访问决策（新增）
type AccessDecision struct {
	Allowed     bool               `json:"allowed"`
	Reason      string             `json:"reason"`
	Policy      *Policy            `json:"policy,omitempty"`
	Constraints *AccessConstraints `json:"constraints,omitempty"`
}

// AccessConstraints 访问约束（新增）
type AccessConstraints struct {
	BandwidthLimit   int64     `json:"bandwidth_limit"`
	ConcurrencyLimit int       `json:"concurrency_limit"`
	ExpiresAt        time.Time `json:"expires_at"`
}

// EvalContext 评估上下文（新增）
type EvalContext struct {
	Request   *AccessRequest
	Timestamp time.Time
}
