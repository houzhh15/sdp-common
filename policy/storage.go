package policy

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// Storage 策略存储接口（抽象存储层）
type Storage interface {
	SavePolicy(ctx context.Context, policy *Policy) error
	GetPolicy(ctx context.Context, policyID string) (*Policy, error)
	DeletePolicy(ctx context.Context, policyID string) error
	QueryPolicies(ctx context.Context, filter *PolicyFilter) ([]*Policy, error)
}

// policyDBModel 数据库模型（用于 GORM）
type policyDBModel struct {
	ID               uint   `gorm:"primarykey"`
	PolicyID         string `gorm:"uniqueIndex"`
	ClientID         string `gorm:"index"`
	ServiceID        string `gorm:"index"`
	BandwidthLimit   int64
	ConcurrencyLimit int
	ExpiryTime       time.Time
	ConditionsJSON   string `gorm:"type:text"` // JSON 序列化的条件列表
	MetadataJSON     string `gorm:"type:text"` // JSON 序列化的元数据
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func (policyDBModel) TableName() string {
	return "policies"
}

// DBStorage 数据库存储实现（复用 Engine 的 GORM 操作）
type DBStorage struct {
	db *gorm.DB
}

// NewDBStorage 创建数据库存储
func NewDBStorage(db *gorm.DB) (*DBStorage, error) {
	// 自动迁移
	if err := db.AutoMigrate(&policyDBModel{}); err != nil {
		return nil, fmt.Errorf("auto migrate policy table: %w", err)
	}

	return &DBStorage{db: db}, nil
}

// SavePolicy 保存策略（复用 PolicyService.CreatePolicy 逻辑）
func (s *DBStorage) SavePolicy(ctx context.Context, policy *Policy) error {
	// 转换为数据库模型
	model, err := s.toDBModel(policy)
	if err != nil {
		return fmt.Errorf("convert to db model: %w", err)
	}

	// 如果已存在则更新，否则创建
	result := s.db.WithContext(ctx).Save(model)
	if result.Error != nil {
		return fmt.Errorf("save policy: %w", result.Error)
	}

	return nil
}

// GetPolicy 获取策略（复用 PolicyService.GetPolicy 逻辑）
func (s *DBStorage) GetPolicy(ctx context.Context, policyID string) (*Policy, error) {
	var model policyDBModel
	result := s.db.WithContext(ctx).Where("policy_id = ?", policyID).First(&model)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("policy not found: %s", policyID)
		}
		return nil, fmt.Errorf("get policy: %w", result.Error)
	}

	// 转换为领域模型
	policy, err := s.fromDBModel(&model)
	if err != nil {
		return nil, fmt.Errorf("convert from db model: %w", err)
	}

	return policy, nil
}

// DeletePolicy 删除策略（复用 PolicyService.DeletePolicy 逻辑）
func (s *DBStorage) DeletePolicy(ctx context.Context, policyID string) error {
	result := s.db.WithContext(ctx).Where("policy_id = ?", policyID).Delete(&policyDBModel{})
	if result.Error != nil {
		return fmt.Errorf("delete policy: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("policy not found: %s", policyID)
	}

	return nil
}

// QueryPolicies 查询策略（复用 Engine.GetPolicies 和 PolicyService.ListPolicies 逻辑）
func (s *DBStorage) QueryPolicies(ctx context.Context, filter *PolicyFilter) ([]*Policy, error) {
	query := s.db.WithContext(ctx).Model(&policyDBModel{})

	if filter != nil {
		if filter.ClientID != "" {
			query = query.Where("client_id = ?", filter.ClientID)
		}
		if filter.ServiceID != "" {
			query = query.Where("service_id = ?", filter.ServiceID)
		}
		if filter.Active {
			// 仅查询未过期策略（复用 Engine.GetPolicies 的过滤逻辑）
			query = query.Where("expiry_time > ? OR expiry_time = ?", time.Now(), time.Time{})
		}
	}

	var models []policyDBModel
	if err := query.Find(&models).Error; err != nil {
		return nil, fmt.Errorf("query policies: %w", err)
	}

	// 转换为领域模型
	policies := make([]*Policy, 0, len(models))
	for i := range models {
		policy, err := s.fromDBModel(&models[i])
		if err != nil {
			return nil, fmt.Errorf("convert policy %s: %w", models[i].PolicyID, err)
		}
		policies = append(policies, policy)
	}

	return policies, nil
}

// toDBModel 转换为数据库模型
func (s *DBStorage) toDBModel(policy *Policy) (*policyDBModel, error) {
	model := &policyDBModel{
		PolicyID:         policy.PolicyID,
		ClientID:         policy.ClientID,
		ServiceID:        policy.ServiceID,
		BandwidthLimit:   policy.BandwidthLimit,
		ConcurrencyLimit: policy.ConcurrencyLimit,
		ExpiryTime:       policy.ExpiryTime,
		CreatedAt:        policy.CreatedAt,
		UpdatedAt:        policy.UpdatedAt,
	}

	// 序列化 Conditions
	if len(policy.Conditions) > 0 {
		conditionsJSON, err := json.Marshal(policy.Conditions)
		if err != nil {
			return nil, fmt.Errorf("marshal conditions: %w", err)
		}
		model.ConditionsJSON = string(conditionsJSON)
	}

	// 序列化 Metadata
	if len(policy.Metadata) > 0 {
		metadataJSON, err := json.Marshal(policy.Metadata)
		if err != nil {
			return nil, fmt.Errorf("marshal metadata: %w", err)
		}
		model.MetadataJSON = string(metadataJSON)
	}

	return model, nil
}

// fromDBModel 从数据库模型转换
func (s *DBStorage) fromDBModel(model *policyDBModel) (*Policy, error) {
	policy := &Policy{
		PolicyID:         model.PolicyID,
		ClientID:         model.ClientID,
		ServiceID:        model.ServiceID,
		BandwidthLimit:   model.BandwidthLimit,
		ConcurrencyLimit: model.ConcurrencyLimit,
		ExpiryTime:       model.ExpiryTime,
		CreatedAt:        model.CreatedAt,
		UpdatedAt:        model.UpdatedAt,
	}

	// 反序列化 Conditions
	if model.ConditionsJSON != "" {
		var conditions []*Condition
		if err := json.Unmarshal([]byte(model.ConditionsJSON), &conditions); err != nil {
			return nil, fmt.Errorf("unmarshal conditions: %w", err)
		}
		policy.Conditions = conditions
	}

	// 反序列化 Metadata
	if model.MetadataJSON != "" {
		var metadata map[string]interface{}
		if err := json.Unmarshal([]byte(model.MetadataJSON), &metadata); err != nil {
			return nil, fmt.Errorf("unmarshal metadata: %w", err)
		}
		policy.Metadata = metadata
	}

	return policy, nil
}
