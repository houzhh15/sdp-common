# SDP Common - Authentication Client

标准化的 SDP 认证客户端，用于 IH Client 和 AH Agent 与 Controller 进行身份验证。

## 特性

- ✅ **标准化协议**: 实现 SDP 标准的认证流程（Handshake/Refresh/Revoke）
- ✅ **自动刷新**: Token 过期前自动刷新，无需手动管理
- ✅ **重试机制**: 内置指数退避重试逻辑
- ✅ **线程安全**: 所有方法都是并发安全的
- ✅ **TLS 支持**: 完整的 mTLS 支持

## 安装

```bash
go get github.com/houzhh15/sdp-common/auth
```

## 快速开始

### 基本用法

```go
package main

import (
    "context"
    "crypto/tls"
    "log"
    
    "github.com/houzhh15/sdp-common/auth"
    "github.com/houzhh15/sdp-common/cert"
)

func main() {
    // 1. 加载证书
    certManager, err := cert.NewManager(&cert.Config{
        CertFile: "client.crt",
        KeyFile:  "client.key",
        CAFile:   "ca.crt",
    })
    if err != nil {
        log.Fatal(err)
    }
    
    // 2. 创建认证客户端
    authClient := auth.NewClient(&auth.Config{
        ControllerURL:   "https://controller:8443",
        TLSConfig:       certManager.GetTLSConfig(),
        CertFingerprint: certManager.GetFingerprint(),
    })
    defer authClient.Stop()
    
    // 3. 执行握手认证
    deviceInfo := auth.DeviceInfo{
        DeviceID:   "device-001",
        OS:         "linux",
        OSVersion:  "5.10",
        Hostname:   "client-host",
        Compliance: true,
    }
    
    ctx := context.Background()
    resp, err := authClient.Handshake(ctx, deviceInfo, "username", "password")
    if err != nil {
        log.Fatal(err)
    }
    
    log.Printf("Authenticated! Token expires at: %v", resp.ExpiresAt)
    
    // 4. 获取 Token（自动刷新）
    token := authClient.GetToken()
    log.Printf("Current token: %s", token)
}
```

### 高级配置

```go
authClient := auth.NewClient(&auth.Config{
    ControllerURL:   "https://controller:8443",
    TLSConfig:       tlsConfig,
    CertFingerprint: fingerprint,
    Timeout:         30 * time.Second,  // HTTP 超时
    RetryAttempts:   3,                 // 重试次数
    RetryInterval:   5 * time.Second,   // 重试间隔
    RefreshBefore:   5 * time.Minute,   // Token 过期前 5 分钟刷新
})
```

### Token 管理

```go
// 检查 Token 是否有效
if authClient.IsValid() {
    log.Println("Token is valid")
}

// 手动刷新 Token
resp, err := authClient.Refresh(ctx)
if err != nil {
    log.Fatal(err)
}

// 撤销 Token（登出）
err = authClient.Revoke(ctx)
if err != nil {
    log.Fatal(err)
}
```

## API 参考

### Config

| 字段 | 类型 | 必填 | 默认值 | 说明 |
|------|------|------|--------|------|
| ControllerURL | string | 是 | - | Controller API 地址 |
| TLSConfig | *tls.Config | 是 | - | TLS 配置（mTLS） |
| CertFingerprint | string | 是 | - | 客户端证书指纹 |
| Timeout | time.Duration | 否 | 30s | HTTP 请求超时 |
| RetryAttempts | int | 否 | 3 | 握手重试次数 |
| RetryInterval | time.Duration | 否 | 5s | 重试间隔 |
| RefreshBefore | time.Duration | 否 | 5min | 提前刷新时间 |

### Methods

#### Handshake

```go
func (c *Client) Handshake(ctx context.Context, deviceInfo DeviceInfo, username, password string) (*HandshakeResponse, error)
```

执行初始认证握手，获取 Token。

#### Refresh

```go
func (c *Client) Refresh(ctx context.Context) (*RefreshResponse, error)
```

刷新当前 Token。

#### Revoke

```go
func (c *Client) Revoke(ctx context.Context) error
```

撤销当前 Token。

#### GetToken

```go
func (c *Client) GetToken() string
```

获取当前 Token。

#### IsValid

```go
func (c *Client) IsValid() bool
```

检查当前 Token 是否有效。

#### Stop

```go
func (c *Client) Stop()
```

停止自动刷新并清理资源。

## 设计原则

1. **协议标准化**: 严格遵循 SDP 协议规范
2. **自动化管理**: Token 自动刷新，减少手动干预
3. **容错性**: 内置重试和错误恢复机制
4. **安全性**: 完整的 mTLS 支持
5. **易用性**: 简单的 API，开箱即用

## 测试

```bash
go test ./auth -v
```

## 许可证

Apache License 2.0
