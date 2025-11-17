package cert

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"os"
	"time"
)

// Config 证书管理器配置
type Config struct {
	CertFile string // 证书文件路径
	KeyFile  string // 私钥文件路径
	CAFile   string // CA证书文件路径
}

// Manager 证书管理器（无状态）
// 从 ih-client/internal/cert/manager.go 提取并扩展
type Manager struct {
	certFile   string
	keyFile    string
	caFile     string
	cert       *tls.Certificate
	x509Cert   *x509.Certificate
	caCertPool *x509.CertPool
}

// NewManager 创建证书管理器
func NewManager(config *Config) (*Manager, error) {
	if config.CertFile == "" || config.KeyFile == "" {
		return nil, fmt.Errorf("cert_file and key_file are required")
	}

	// 加载证书和私钥
	cert, err := tls.LoadX509KeyPair(config.CertFile, config.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("load certificate: %w", err)
	}

	// 解析X.509证书
	x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return nil, fmt.Errorf("parse certificate: %w", err)
	}

	// 加载CA证书池
	var caCertPool *x509.CertPool
	if config.CAFile != "" {
		caCertPool = x509.NewCertPool()
		caData, err := os.ReadFile(config.CAFile)
		if err != nil {
			return nil, fmt.Errorf("read CA file: %w", err)
		}
		if !caCertPool.AppendCertsFromPEM(caData) {
			return nil, fmt.Errorf("append CA certs failed")
		}
	}

	return &Manager{
		certFile:   config.CertFile,
		keyFile:    config.KeyFile,
		caFile:     config.CAFile,
		cert:       &cert,
		x509Cert:   x509Cert,
		caCertPool: caCertPool,
	}, nil
}

// GetFingerprint 获取证书指纹（SHA256）
// 复用 ih-client/internal/cert/manager.go 的实现
func (m *Manager) GetFingerprint() string {
	hash := sha256.Sum256(m.x509Cert.Raw)
	return "sha256:" + hex.EncodeToString(hash[:])
}

// ValidateExpiry 验证证书有效期
// 复用 ih-client/internal/cert/manager.go 的实现
func (m *Manager) ValidateExpiry() error {
	now := time.Now()
	if now.Before(m.x509Cert.NotBefore) {
		return fmt.Errorf("certificate not yet valid (valid from %s)", m.x509Cert.NotBefore)
	}
	if now.After(m.x509Cert.NotAfter) {
		return fmt.Errorf("certificate expired (expired at %s)", m.x509Cert.NotAfter)
	}
	return nil
}

// GetTLSConfig 生成TLS配置（新增方法）
func (m *Manager) GetTLSConfig() *tls.Config {
	config := &tls.Config{
		Certificates: []tls.Certificate{*m.cert},
		MinVersion:   tls.VersionTLS12,
	}

	if m.caCertPool != nil {
		config.RootCAs = m.caCertPool
		config.ClientCAs = m.caCertPool
		config.ClientAuth = tls.RequireAndVerifyClientCert
	}

	return config
}

// GetCertificate 获取TLS证书
func (m *Manager) GetCertificate() *tls.Certificate {
	return m.cert
}

// GetX509Certificate 获取X.509证书
func (m *Manager) GetX509Certificate() *x509.Certificate {
	return m.x509Cert
}

// DaysUntilExpiry 获取证书到期天数
func (m *Manager) DaysUntilExpiry() int {
	duration := time.Until(m.x509Cert.NotAfter)
	return int(duration.Hours() / 24)
}

// GetCertInfo 获取证书信息
func (m *Manager) GetCertInfo() *CertInfo {
	return &CertInfo{
		Fingerprint: m.GetFingerprint(),
		Subject:     m.x509Cert.Subject.String(),
		Issuer:      m.x509Cert.Issuer.String(),
		NotBefore:   m.x509Cert.NotBefore,
		NotAfter:    m.x509Cert.NotAfter,
		Status:      m.getCertStatus(),
	}
}

// getCertStatus 获取证书状态
func (m *Manager) getCertStatus() CertStatus {
	now := time.Now()
	if now.Before(m.x509Cert.NotBefore) || now.After(m.x509Cert.NotAfter) {
		return StatusExpired
	}
	return StatusActive
}
