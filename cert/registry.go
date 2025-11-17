package cert

import (
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/houzhh15/sdp-common/logging"
	"gorm.io/gorm"
)

// Registry 证书注册表（数据库支持）
type Registry struct {
	db      *gorm.DB
	logger  logging.Logger
	mu      sync.RWMutex
	crlPath string // CRL文件路径（可选）
}

// CertRecord 数据库证书记录
type CertRecord struct {
	ID           uint      `gorm:"primaryKey"`
	Fingerprint  string    `gorm:"uniqueIndex;not null"`
	ClientID     string    `gorm:"index"`
	Subject      string    `gorm:"not null"`
	Issuer       string    `gorm:"not null"`
	NotBefore    time.Time `gorm:"not null"`
	NotAfter     time.Time `gorm:"not null"`
	Status       string    `gorm:"default:'active'"`
	RevokedAt    *time.Time
	RevokeReason string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// TableName 指定表名
func (CertRecord) TableName() string {
	return "cert_records"
}

// NewRegistry 创建证书注册表
func NewRegistry(db *gorm.DB, logger logging.Logger) (*Registry, error) {
	if db == nil {
		return nil, errors.New("database is required")
	}

	registry := &Registry{
		db:     db,
		logger: logger,
	}

	// 自动迁移表结构
	if err := db.AutoMigrate(&CertRecord{}); err != nil {
		return nil, fmt.Errorf("failed to migrate cert_records table: %w", err)
	}

	return registry, nil
}

// Register 注册证书
func (r *Registry) Register(clientID, fingerprint string, cert *x509.Certificate) error {
	if fingerprint == "" {
		return errors.New("fingerprint is required")
	}
	if cert == nil {
		return errors.New("certificate is required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	record := CertRecord{
		Fingerprint: fingerprint,
		ClientID:    clientID,
		Subject:     cert.Subject.String(),
		Issuer:      cert.Issuer.String(),
		NotBefore:   cert.NotBefore,
		NotAfter:    cert.NotAfter,
		Status:      string(StatusActive),
	}

	result := r.db.Create(&record)
	if result.Error != nil {
		if r.logger != nil {
			r.logger.Error("Failed to register certificate", "fingerprint", fingerprint, "error", result.Error)
		}
		return fmt.Errorf("failed to register certificate: %w", result.Error)
	}

	if r.logger != nil {
		r.logger.Info("Certificate registered", "fingerprint", fingerprint, "client_id", clientID)
	}

	return nil
}

// GetCertInfo 获取证书信息
func (r *Registry) GetCertInfo(fingerprint string) (*CertInfo, error) {
	if fingerprint == "" {
		return nil, errors.New("fingerprint is required")
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	var record CertRecord
	result := r.db.Where("fingerprint = ?", fingerprint).First(&record)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("certificate not found: %s", fingerprint)
		}
		return nil, fmt.Errorf("failed to query certificate: %w", result.Error)
	}

	return &CertInfo{
		Fingerprint: record.Fingerprint,
		ClientID:    record.ClientID,
		Subject:     record.Subject,
		Issuer:      record.Issuer,
		NotBefore:   record.NotBefore,
		NotAfter:    record.NotAfter,
		Status:      CertStatus(record.Status),
	}, nil
}

// Revoke 吊销证书
func (r *Registry) Revoke(fingerprint, reason string) error {
	if fingerprint == "" {
		return errors.New("fingerprint is required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	result := r.db.Model(&CertRecord{}).
		Where("fingerprint = ?", fingerprint).
		Updates(map[string]interface{}{
			"status":        string(StatusRevoked),
			"revoked_at":    &now,
			"revoke_reason": reason,
		})

	if result.Error != nil {
		if r.logger != nil {
			r.logger.Error("Failed to revoke certificate", "fingerprint", fingerprint, "error", result.Error)
		}
		return fmt.Errorf("failed to revoke certificate: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("certificate not found: %s", fingerprint)
	}

	if r.logger != nil {
		r.logger.Info("Certificate revoked", "fingerprint", fingerprint, "reason", reason)
	}

	return nil
}

// Validate 验证证书状态
func (r *Registry) Validate(fingerprint string) error {
	info, err := r.GetCertInfo(fingerprint)
	if err != nil {
		return err
	}

	// 检查状态
	if info.Status == StatusRevoked {
		return fmt.Errorf("certificate has been revoked: %s", fingerprint)
	}

	// 检查过期
	now := time.Now()
	if now.Before(info.NotBefore) {
		return fmt.Errorf("certificate not yet valid: %s", fingerprint)
	}
	if now.After(info.NotAfter) {
		return fmt.Errorf("certificate has expired: %s", fingerprint)
	}

	return nil
}

// List 列出所有证书（分页）
func (r *Registry) List(page, pageSize int, status CertStatus) ([]*CertInfo, int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var total int64
	query := r.db.Model(&CertRecord{})

	// 状态过滤
	if status != "" {
		query = query.Where("status = ?", string(status))
	}

	// 查询总数
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count certificates: %w", err)
	}

	// 分页查询
	var records []CertRecord
	offset := (page - 1) * pageSize
	result := query.Offset(offset).Limit(pageSize).Find(&records)
	if result.Error != nil {
		return nil, 0, fmt.Errorf("failed to list certificates: %w", result.Error)
	}

	// 转换为CertInfo
	infos := make([]*CertInfo, len(records))
	for i, record := range records {
		infos[i] = &CertInfo{
			Fingerprint: record.Fingerprint,
			ClientID:    record.ClientID,
			Subject:     record.Subject,
			Issuer:      record.Issuer,
			NotBefore:   record.NotBefore,
			NotAfter:    record.NotAfter,
			Status:      CertStatus(record.Status),
		}
	}

	return infos, total, nil
}

// CleanExpired 清理过期证书
func (r *Registry) CleanExpired() (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	result := r.db.Model(&CertRecord{}).
		Where("not_after < ? AND status = ?", time.Now(), string(StatusActive)).
		Update("status", string(StatusExpired))

	if result.Error != nil {
		if r.logger != nil {
			r.logger.Error("Failed to clean expired certificates", "error", result.Error)
		}
		return 0, fmt.Errorf("failed to clean expired certificates: %w", result.Error)
	}

	if r.logger != nil {
		r.logger.Info("Cleaned expired certificates", "count", result.RowsAffected)
	}

	return result.RowsAffected, nil
}

// SetCRLPath 设置CRL文件路径
func (r *Registry) SetCRLPath(path string) error {
	if path != "" {
		if _, err := os.Stat(path); err != nil {
			return fmt.Errorf("CRL file not found: %s", path)
		}
	}

	r.mu.Lock()
	r.crlPath = path
	r.mu.Unlock()

	if r.logger != nil {
		r.logger.Info("CRL path updated", "path", path)
	}

	return nil
}

// GetCRLPath 获取CRL文件路径
func (r *Registry) GetCRLPath() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.crlPath
}
