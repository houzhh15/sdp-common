package controller

import (
	"crypto/tls"
	"fmt"
	"os"
	"time"
)

// Config Controller configuration
type Config struct {
	// TLS configuration
	CertFile string
	KeyFile  string
	CAFile   string

	// Server addresses
	HTTPAddr     string // HTTPS server address (e.g., ":8443")
	TCPProxyAddr string // TCP proxy address (e.g., ":9443")

	// Logging
	LogLevel string // debug, info, warn, error

	// Database
	DBPath string // SQLite database path (default: "controller.db")

	// Data plane configuration (ZTNA-03)
	DataPlane *DataPlaneConfig
}

// DataPlaneConfig 数据平面中继服务器配置
type DataPlaneConfig struct {
	// ListenAddr 监听地址 (默认 ":9443")
	ListenAddr string `yaml:"listen_addr"`

	// TLS TLS配置
	TLS TLSConfig `yaml:"tls"`

	// RelayConfig 中继配置
	RelayConfig RelayConfig `yaml:"relay_config"`
}

// TLSConfig TLS 配置
type TLSConfig struct {
	// CertFile 服务器证书文件路径
	CertFile string `yaml:"cert_file"`

	// KeyFile 服务器私钥文件路径
	KeyFile string `yaml:"key_file"`

	// CAFile CA 证书文件路径
	CAFile string `yaml:"ca_file"`

	// ClientAuth 客户端认证模式
	// 可选值: NoClientCert, RequestClientCert, RequireAnyClientCert,
	//        VerifyClientCertIfGiven, RequireAndVerifyClientCert
	ClientAuth string `yaml:"client_auth"`
}

// RelayConfig 中继配置
type RelayConfig struct {
	// PairingTimeout 配对超时时间 (默认 30秒)
	PairingTimeout time.Duration `yaml:"pairing_timeout"`

	// BufferSize 数据缓冲区大小 (默认 32KB)
	BufferSize int `yaml:"buffer_size"`

	// ReadTimeout TCP 读超时 (默认 300秒)
	ReadTimeout time.Duration `yaml:"read_timeout"`

	// WriteTimeout TCP 写超时 (默认 300秒)
	WriteTimeout time.Duration `yaml:"write_timeout"`

	// MaxConnections 最大并发连接数 (默认 10000)
	MaxConnections int `yaml:"max_connections"`
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.CertFile == "" {
		return fmt.Errorf("cert_file is required")
	}
	if c.KeyFile == "" {
		return fmt.Errorf("key_file is required")
	}
	if c.CAFile == "" {
		return fmt.Errorf("ca_file is required")
	}
	if c.HTTPAddr == "" {
		return fmt.Errorf("http_addr is required")
	}
	if c.TCPProxyAddr == "" {
		return fmt.Errorf("tcp_proxy_addr is required")
	}
	if c.LogLevel == "" {
		c.LogLevel = "info"
	}

	// Validate data plane configuration
	if c.DataPlane != nil {
		if err := c.DataPlane.Validate(); err != nil {
			return fmt.Errorf("data_plane config error: %w", err)
		}
	}

	return nil
}

// Validate 验证数据平面配置
func (d *DataPlaneConfig) Validate() error {
	// 验证监听地址
	if d.ListenAddr == "" {
		d.ListenAddr = ":9443" // 默认端口
	}

	// 验证 TLS 配置
	if err := d.TLS.Validate(); err != nil {
		return fmt.Errorf("tls config error: %w", err)
	}

	// 验证中继配置
	if err := d.RelayConfig.Validate(); err != nil {
		return fmt.Errorf("relay_config error: %w", err)
	}

	return nil
}

// Validate 验证 TLS 配置
func (t *TLSConfig) Validate() error {
	// 验证证书文件存在性
	if t.CertFile == "" {
		return fmt.Errorf("cert_file is required")
	}
	if _, err := os.Stat(t.CertFile); os.IsNotExist(err) {
		return fmt.Errorf("cert_file not found: %s", t.CertFile)
	}

	// 验证密钥文件存在性
	if t.KeyFile == "" {
		return fmt.Errorf("key_file is required")
	}
	if _, err := os.Stat(t.KeyFile); os.IsNotExist(err) {
		return fmt.Errorf("key_file not found: %s", t.KeyFile)
	}

	// 验证 CA 文件存在性
	if t.CAFile == "" {
		return fmt.Errorf("ca_file is required")
	}
	if _, err := os.Stat(t.CAFile); os.IsNotExist(err) {
		return fmt.Errorf("ca_file not found: %s", t.CAFile)
	}

	// 验证客户端认证模式
	validAuthModes := map[string]tls.ClientAuthType{
		"NoClientCert":               tls.NoClientCert,
		"RequestClientCert":          tls.RequestClientCert,
		"RequireAnyClientCert":       tls.RequireAnyClientCert,
		"VerifyClientCertIfGiven":    tls.VerifyClientCertIfGiven,
		"RequireAndVerifyClientCert": tls.RequireAndVerifyClientCert,
	}

	if t.ClientAuth == "" {
		t.ClientAuth = "RequireAndVerifyClientCert" // 默认最严格模式
	}

	if _, ok := validAuthModes[t.ClientAuth]; !ok {
		return fmt.Errorf("invalid client_auth mode: %s (valid: NoClientCert, RequestClientCert, RequireAnyClientCert, VerifyClientCertIfGiven, RequireAndVerifyClientCert)", t.ClientAuth)
	}

	return nil
}

// GetClientAuthType 返回 tls.ClientAuthType
func (t *TLSConfig) GetClientAuthType() tls.ClientAuthType {
	authModes := map[string]tls.ClientAuthType{
		"NoClientCert":               tls.NoClientCert,
		"RequestClientCert":          tls.RequestClientCert,
		"RequireAnyClientCert":       tls.RequireAnyClientCert,
		"VerifyClientCertIfGiven":    tls.VerifyClientCertIfGiven,
		"RequireAndVerifyClientCert": tls.RequireAndVerifyClientCert,
	}
	if authType, ok := authModes[t.ClientAuth]; ok {
		return authType
	}
	return tls.RequireAndVerifyClientCert // 默认
}

// Validate 验证中继配置
func (r *RelayConfig) Validate() error {
	// 设置默认值
	if r.PairingTimeout == 0 {
		r.PairingTimeout = 30 * time.Second
	}
	if r.BufferSize == 0 {
		r.BufferSize = 32768 // 32KB
	}
	if r.ReadTimeout == 0 {
		r.ReadTimeout = 300 * time.Second // 5分钟
	}
	if r.WriteTimeout == 0 {
		r.WriteTimeout = 300 * time.Second // 5分钟
	}
	if r.MaxConnections == 0 {
		r.MaxConnections = 10000
	}

	// 验证超时时间为正数
	if r.PairingTimeout < 0 {
		return fmt.Errorf("pairing_timeout must be positive, got: %v", r.PairingTimeout)
	}
	if r.ReadTimeout < 0 {
		return fmt.Errorf("read_timeout must be positive, got: %v", r.ReadTimeout)
	}
	if r.WriteTimeout < 0 {
		return fmt.Errorf("write_timeout must be positive, got: %v", r.WriteTimeout)
	}

	// 验证缓冲区大小
	if r.BufferSize <= 0 {
		return fmt.Errorf("buffer_size must be positive, got: %d", r.BufferSize)
	}
	if r.BufferSize < 4096 {
		return fmt.Errorf("buffer_size too small (minimum 4096), got: %d", r.BufferSize)
	}

	// 验证最大连接数
	if r.MaxConnections <= 0 {
		return fmt.Errorf("max_connections must be positive, got: %d", r.MaxConnections)
	}

	return nil
}
