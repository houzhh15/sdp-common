package cert

import (
	"testing"
	"time"
)

func TestManager_GetFingerprint(t *testing.T) {
	// 创建测试证书管理器
	mgr, err := NewManager(&Config{
		CertFile: "../certs/controller-cert.pem",
		KeyFile:  "../certs/controller-key.pem",
		CAFile:   "../certs/ca-cert.pem",
	})
	if err != nil {
		t.Skip("证书文件不存在，跳过测试")
		return
	}

	fingerprint := mgr.GetFingerprint()
	if fingerprint == "" {
		t.Error("GetFingerprint返回空字符串")
	}

	if len(fingerprint) < 71 { // "sha256:" + 64位十六进制
		t.Errorf("指纹长度不正确: %d", len(fingerprint))
	}
}

func TestManager_ValidateExpiry(t *testing.T) {
	mgr, err := NewManager(&Config{
		CertFile: "../certs/controller-cert.pem",
		KeyFile:  "../certs/controller-key.pem",
		CAFile:   "../certs/ca-cert.pem",
	})
	if err != nil {
		t.Skip("证书文件不存在，跳过测试")
		return
	}

	err = mgr.ValidateExpiry()
	if err != nil {
		t.Logf("证书验证失败（可能已过期）: %v", err)
	}
}

func TestManager_GetTLSConfig(t *testing.T) {
	mgr, err := NewManager(&Config{
		CertFile: "../certs/controller-cert.pem",
		KeyFile:  "../certs/controller-key.pem",
		CAFile:   "../certs/ca-cert.pem",
	})
	if err != nil {
		t.Skip("证书文件不存在，跳过测试")
		return
	}

	tlsConfig := mgr.GetTLSConfig()
	if tlsConfig == nil {
		t.Error("GetTLSConfig返回nil")
	}

	if len(tlsConfig.Certificates) == 0 {
		t.Error("TLS配置中没有证书")
	}

	if tlsConfig.MinVersion != 0x0303 { // TLS 1.2
		t.Errorf("MinVersion错误: %x", tlsConfig.MinVersion)
	}
}

func TestManager_DaysUntilExpiry(t *testing.T) {
	mgr, err := NewManager(&Config{
		CertFile: "../certs/controller-cert.pem",
		KeyFile:  "../certs/controller-key.pem",
	})
	if err != nil {
		t.Skip("证书文件不存在，跳过测试")
		return
	}

	days := mgr.DaysUntilExpiry()
	t.Logf("证书距离过期还有 %d 天", days)

	// 检查是否接近过期（小于30天）
	if days < 30 && days > 0 {
		t.Logf("警告：证书即将过期（%d天）", days)
	}
}

func TestManager_GetCertInfo(t *testing.T) {
	mgr, err := NewManager(&Config{
		CertFile: "../certs/controller-cert.pem",
		KeyFile:  "../certs/controller-key.pem",
		CAFile:   "../certs/ca-cert.pem",
	})
	if err != nil {
		t.Skip("证书文件不存在，跳过测试")
		return
	}

	info := mgr.GetCertInfo()
	if info == nil {
		t.Fatal("GetCertInfo返回nil")
	}

	if info.Fingerprint == "" {
		t.Error("Fingerprint为空")
	}

	if info.Subject == "" {
		t.Error("Subject为空")
	}

	if info.NotBefore.After(time.Now()) || info.NotAfter.Before(time.Now()) {
		if info.Status != StatusExpired {
			t.Errorf("证书已过期但状态为: %s", info.Status)
		}
	}

	t.Logf("证书信息: Subject=%s, Issuer=%s, Status=%s",
		info.Subject, info.Issuer, info.Status)
}

func TestNewManager_WithoutCA(t *testing.T) {
	mgr, err := NewManager(&Config{
		CertFile: "../certs/controller-cert.pem",
		KeyFile:  "../certs/controller-key.pem",
		// 不提供CA文件
	})
	if err != nil {
		t.Skip("证书文件不存在，跳过测试")
		return
	}

	if mgr == nil {
		t.Fatal("NewManager返回nil")
	}

	tlsConfig := mgr.GetTLSConfig()
	if tlsConfig.RootCAs != nil {
		t.Error("没有CA文件时RootCAs应该为nil")
	}
}

func TestNewManager_InvalidPaths(t *testing.T) {
	tests := []struct {
		name   string
		config *Config
	}{
		{
			name: "缺少CertFile",
			config: &Config{
				KeyFile: "key.pem",
			},
		},
		{
			name: "缺少KeyFile",
			config: &Config{
				CertFile: "cert.pem",
			},
		},
		{
			name: "证书文件不存在",
			config: &Config{
				CertFile: "/nonexistent/cert.pem",
				KeyFile:  "/nonexistent/key.pem",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewManager(tt.config)
			if err == nil {
				t.Error("期望返回错误，但没有错误")
			}
		})
	}
}
