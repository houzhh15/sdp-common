package controller

import (
	"crypto/tls"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDataPlaneConfig_Validate 测试数据平面配置验证
func TestDataPlaneConfig_Validate(t *testing.T) {
	// 创建临时证书文件用于测试
	tmpDir := t.TempDir()
	certFile := filepath.Join(tmpDir, "test.crt")
	keyFile := filepath.Join(tmpDir, "test.key")
	caFile := filepath.Join(tmpDir, "ca.crt")

	require.NoError(t, os.WriteFile(certFile, []byte("test cert"), 0644))
	require.NoError(t, os.WriteFile(keyFile, []byte("test key"), 0644))
	require.NoError(t, os.WriteFile(caFile, []byte("test ca"), 0644))

	tests := []struct {
		name    string
		config  *DataPlaneConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "Valid configuration with all fields",
			config: &DataPlaneConfig{
				ListenAddr: ":9443",
				TLS: TLSConfig{
					CertFile:   certFile,
					KeyFile:    keyFile,
					CAFile:     caFile,
					ClientAuth: "RequireAndVerifyClientCert",
				},
				RelayConfig: RelayConfig{
					PairingTimeout: 30 * time.Second,
					BufferSize:     32768,
					ReadTimeout:    300 * time.Second,
					WriteTimeout:   300 * time.Second,
					MaxConnections: 10000,
				},
			},
			wantErr: false,
		},
		{
			name: "Valid configuration with defaults",
			config: &DataPlaneConfig{
				TLS: TLSConfig{
					CertFile: certFile,
					KeyFile:  keyFile,
					CAFile:   caFile,
				},
			},
			wantErr: false,
		},
		{
			name: "Missing cert file",
			config: &DataPlaneConfig{
				TLS: TLSConfig{
					CertFile: "/nonexistent/cert.crt",
					KeyFile:  keyFile,
					CAFile:   caFile,
				},
			},
			wantErr: true,
			errMsg:  "cert_file not found",
		},
		{
			name: "Missing key file",
			config: &DataPlaneConfig{
				TLS: TLSConfig{
					CertFile: certFile,
					KeyFile:  "/nonexistent/key.key",
					CAFile:   caFile,
				},
			},
			wantErr: true,
			errMsg:  "key_file not found",
		},
		{
			name: "Missing CA file",
			config: &DataPlaneConfig{
				TLS: TLSConfig{
					CertFile: certFile,
					KeyFile:  keyFile,
					CAFile:   "/nonexistent/ca.crt",
				},
			},
			wantErr: true,
			errMsg:  "ca_file not found",
		},
		{
			name: "Invalid client auth mode",
			config: &DataPlaneConfig{
				TLS: TLSConfig{
					CertFile:   certFile,
					KeyFile:    keyFile,
					CAFile:     caFile,
					ClientAuth: "InvalidMode",
				},
			},
			wantErr: true,
			errMsg:  "invalid client_auth mode",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestTLSConfig_Validate 测试 TLS 配置验证
func TestTLSConfig_Validate(t *testing.T) {
	tmpDir := t.TempDir()
	certFile := filepath.Join(tmpDir, "test.crt")
	keyFile := filepath.Join(tmpDir, "test.key")
	caFile := filepath.Join(tmpDir, "ca.crt")

	require.NoError(t, os.WriteFile(certFile, []byte("test cert"), 0644))
	require.NoError(t, os.WriteFile(keyFile, []byte("test key"), 0644))
	require.NoError(t, os.WriteFile(caFile, []byte("test ca"), 0644))

	tests := []struct {
		name    string
		config  TLSConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "Valid TLS config",
			config: TLSConfig{
				CertFile:   certFile,
				KeyFile:    keyFile,
				CAFile:     caFile,
				ClientAuth: "RequireAndVerifyClientCert",
			},
			wantErr: false,
		},
		{
			name: "Default client auth mode",
			config: TLSConfig{
				CertFile: certFile,
				KeyFile:  keyFile,
				CAFile:   caFile,
			},
			wantErr: false,
		},
		{
			name:    "Empty cert file",
			config:  TLSConfig{},
			wantErr: true,
			errMsg:  "cert_file is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
				// Verify default client auth is set
				if tt.config.ClientAuth == "" {
					assert.Equal(t, "RequireAndVerifyClientCert", tt.config.ClientAuth)
				}
			}
		})
	}
}

// TestTLSConfig_GetClientAuthType 测试获取客户端认证类型
func TestTLSConfig_GetClientAuthType(t *testing.T) {
	tests := []struct {
		name       string
		clientAuth string
		want       tls.ClientAuthType
	}{
		{
			name:       "NoClientCert",
			clientAuth: "NoClientCert",
			want:       tls.NoClientCert,
		},
		{
			name:       "RequestClientCert",
			clientAuth: "RequestClientCert",
			want:       tls.RequestClientCert,
		},
		{
			name:       "RequireAnyClientCert",
			clientAuth: "RequireAnyClientCert",
			want:       tls.RequireAnyClientCert,
		},
		{
			name:       "VerifyClientCertIfGiven",
			clientAuth: "VerifyClientCertIfGiven",
			want:       tls.VerifyClientCertIfGiven,
		},
		{
			name:       "RequireAndVerifyClientCert",
			clientAuth: "RequireAndVerifyClientCert",
			want:       tls.RequireAndVerifyClientCert,
		},
		{
			name:       "Invalid mode defaults to RequireAndVerifyClientCert",
			clientAuth: "InvalidMode",
			want:       tls.RequireAndVerifyClientCert,
		},
		{
			name:       "Empty defaults to RequireAndVerifyClientCert",
			clientAuth: "",
			want:       tls.RequireAndVerifyClientCert,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := TLSConfig{ClientAuth: tt.clientAuth}
			got := config.GetClientAuthType()
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestRelayConfig_Validate 测试中继配置验证
func TestRelayConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  RelayConfig
		wantErr bool
		errMsg  string
		check   func(*testing.T, *RelayConfig)
	}{
		{
			name: "Valid config with all fields",
			config: RelayConfig{
				PairingTimeout: 30 * time.Second,
				BufferSize:     32768,
				ReadTimeout:    300 * time.Second,
				WriteTimeout:   300 * time.Second,
				MaxConnections: 10000,
			},
			wantErr: false,
		},
		{
			name:    "Empty config uses defaults",
			config:  RelayConfig{},
			wantErr: false,
			check: func(t *testing.T, r *RelayConfig) {
				assert.Equal(t, 30*time.Second, r.PairingTimeout)
				assert.Equal(t, 32768, r.BufferSize)
				assert.Equal(t, 300*time.Second, r.ReadTimeout)
				assert.Equal(t, 300*time.Second, r.WriteTimeout)
				assert.Equal(t, 10000, r.MaxConnections)
			},
		},
		{
			name: "Negative pairing timeout",
			config: RelayConfig{
				PairingTimeout: -1 * time.Second,
			},
			wantErr: true,
			errMsg:  "pairing_timeout must be positive",
		},
		{
			name: "Negative read timeout",
			config: RelayConfig{
				ReadTimeout: -1 * time.Second,
			},
			wantErr: true,
			errMsg:  "read_timeout must be positive",
		},
		{
			name: "Negative write timeout",
			config: RelayConfig{
				WriteTimeout: -1 * time.Second,
			},
			wantErr: true,
			errMsg:  "write_timeout must be positive",
		},
		{
			name: "Buffer size too small",
			config: RelayConfig{
				BufferSize: 1024, // Less than 4096
			},
			wantErr: true,
			errMsg:  "buffer_size too small",
		},
		{
			name: "Negative buffer size",
			config: RelayConfig{
				BufferSize: -1,
			},
			wantErr: true,
			errMsg:  "buffer_size must be positive",
		},
		{
			name: "Negative max connections",
			config: RelayConfig{
				MaxConnections: -1,
			},
			wantErr: true,
			errMsg:  "max_connections must be positive",
		},
		{
			name: "Zero max connections",
			config: RelayConfig{
				MaxConnections: 0,
			},
			wantErr: false, // 0 will use default
			check: func(t *testing.T, r *RelayConfig) {
				assert.Equal(t, 10000, r.MaxConnections)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
				if tt.check != nil {
					tt.check(t, &tt.config)
				}
			}
		})
	}
}

// TestConfig_Validate_WithDataPlane 测试完整配置验证
func TestConfig_Validate_WithDataPlane(t *testing.T) {
	tmpDir := t.TempDir()
	certFile := filepath.Join(tmpDir, "test.crt")
	keyFile := filepath.Join(tmpDir, "test.key")
	caFile := filepath.Join(tmpDir, "ca.crt")

	require.NoError(t, os.WriteFile(certFile, []byte("test cert"), 0644))
	require.NoError(t, os.WriteFile(keyFile, []byte("test key"), 0644))
	require.NoError(t, os.WriteFile(caFile, []byte("test ca"), 0644))

	tests := []struct {
		name    string
		config  Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "Valid config with data plane",
			config: Config{
				CertFile:     certFile,
				KeyFile:      keyFile,
				CAFile:       caFile,
				HTTPAddr:     ":8443",
				TCPProxyAddr: ":9443",
				DataPlane: &DataPlaneConfig{
					ListenAddr: ":9443",
					TLS: TLSConfig{
						CertFile: certFile,
						KeyFile:  keyFile,
						CAFile:   caFile,
					},
					RelayConfig: RelayConfig{
						PairingTimeout: 30 * time.Second,
						BufferSize:     32768,
						MaxConnections: 10000,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Invalid data plane config",
			config: Config{
				CertFile:     certFile,
				KeyFile:      keyFile,
				CAFile:       caFile,
				HTTPAddr:     ":8443",
				TCPProxyAddr: ":9443",
				DataPlane: &DataPlaneConfig{
					TLS: TLSConfig{
						CertFile: "/nonexistent/cert.crt",
						KeyFile:  keyFile,
						CAFile:   caFile,
					},
				},
			},
			wantErr: true,
			errMsg:  "data_plane config error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
