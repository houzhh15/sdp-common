package transport

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
)

// TLSConfig TLS 配置
type TLSConfig struct {
	CertFile   string `yaml:"cert_file" json:"cert_file"`
	KeyFile    string `yaml:"key_file" json:"key_file"`
	CAFile     string `yaml:"ca_file" json:"ca_file"`
	MinVersion uint16 `yaml:"min_version" json:"min_version"` // tls.VersionTLS12
}

// LoadTLSConfig 加载 TLS 配置并创建 tls.Config
// 自动启用 mTLS 双向认证（RequireAndVerifyClientCert）
func LoadTLSConfig(cfg *TLSConfig) (*tls.Config, error) {
	// 1. 加载服务端证书和私钥
	cert, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load cert/key: %w", err)
	}

	// 2. 加载 CA 证书（用于验证客户端证书）
	caCert, err := os.ReadFile(cfg.CAFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read CA cert: %w", err)
	}

	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("failed to parse CA cert")
	}

	// 3. 创建 TLS 配置
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientCAs:    caCertPool,
		ClientAuth:   tls.RequireAndVerifyClientCert, // 强制 mTLS
		MinVersion:   cfg.MinVersion,
	}

	// 默认最低版本 TLS 1.2
	if tlsConfig.MinVersion == 0 {
		tlsConfig.MinVersion = tls.VersionTLS12
	}

	return tlsConfig, nil
}
