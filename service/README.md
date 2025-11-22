# SDP Common - Service Registration Client

标准化的 SDP 服务注册客户端，用于 AH Agent 向 Controller 注册和管理服务。

## 特性

- ✅ **标准化协议**: 实现 SDP 标准的服务注册流程
- ✅ **完整功能**: 注册、查询、注销、心跳、故障报告
- ✅ **本地缓存**: 自动缓存服务列表，减少网络请求
- ✅ **线程安全**: 所有方法都是并发安全的
- ✅ **TLS 支持**: 完整的 mTLS 支持

## 安装

```bash
go get github.com/houzhh15/sdp-common/service
```

## 快速开始

### 基本用法

```go
package main

import (
    "context"
    "log"
    
    "github.com/houzhh15/sdp-common/service"
    "github.com/houzhh15/sdp-common/cert"
)

func main() {
    // 1. 加载证书
    certManager, err := cert.NewManager(&cert.Config{
        CertFile: "agent.crt",
        KeyFile:  "agent.key",
        CAFile:   "ca.crt",
    })
    if err != nil {
        log.Fatal(err)
    }
    
    // 2. 创建服务注册客户端
    serviceClient := service.NewClient(&service.Config{
        ControllerURL: "https://controller:8443",
        TLSConfig:     certManager.GetTLSConfig(),
        AgentID:       "agent-001",
    })
    defer serviceClient.Stop()
    
    // 3. 注册服务
    services := []service.Service{
        {
            ID:         "web-app",
            Name:       "Web Application",
            TargetHost: "localhost",
            TargetPort: 8080,
            Protocol:   "http",
        },
        {
            ID:         "api-service",
            Name:       "API Service",
            TargetHost: "localhost",
            TargetPort: 9000,
            Protocol:   "http",
        },
    }
    
    ctx := context.Background()
    err = serviceClient.Register(ctx, services)
    if err != nil {
        log.Fatal(err)
    }
    
    log.Println("Services registered successfully!")
}
```

### 服务查询

```go
// 获取所有服务
services, err := serviceClient.Fetch(ctx)
if err != nil {
    log.Fatal(err)
}

for _, svc := range services {
    log.Printf("Service: %s (%s:%d)", svc.Name, svc.TargetHost, svc.TargetPort)
}

// 获取特定服务
svc, ok := serviceClient.GetService("web-app")
if ok {
    log.Printf("Found service: %s", svc.Name)
}
```

### 心跳发送

```go
// 定期发送心跳
ticker := time.NewTicker(30 * time.Second)
defer ticker.Stop()

for range ticker.C {
    serviceIDs := []string{"web-app", "api-service"}
    err := serviceClient.Heartbeat(ctx, serviceIDs)
    if err != nil {
        log.Printf("Heartbeat failed: %v", err)
    }
}
```

### 服务注销

```go
// 注销特定服务
err = serviceClient.Unregister(ctx, "web-app")
if err != nil {
    log.Fatal(err)
}
```

### 故障报告

```go
// 报告服务故障
err = serviceClient.ReportFailure(ctx, "api-service", "connection timeout")
if err != nil {
    log.Printf("Failed to report failure: %v", err)
}
```

## API 参考

### Config

| 字段 | 类型 | 必填 | 默认值 | 说明 |
|------|------|------|--------|------|
| ControllerURL | string | 是 | - | Controller API 地址 |
| TLSConfig | *tls.Config | 是 | - | TLS 配置（mTLS） |
| AgentID | string | 是 | - | Agent 标识符 |
| Timeout | time.Duration | 否 | 10s | HTTP 请求超时 |

### Service 结构

```go
type Service struct {
    ID         string            // 服务唯一标识
    Name       string            // 服务名称
    TargetHost string            // 目标主机地址
    TargetPort int               // 目标端口
    Protocol   string            // 协议（http/https/tcp）
    Status     string            // 状态（可选）
    Metadata   map[string]string // 元数据（可选）
}
```

### Methods

#### Register

```go
func (c *Client) Register(ctx context.Context, services []Service) error
```

向 Controller 注册一个或多个服务。

#### Fetch

```go
func (c *Client) Fetch(ctx context.Context) ([]Service, error)
```

从 Controller 获取服务列表。

#### Unregister

```go
func (c *Client) Unregister(ctx context.Context, serviceID string) error
```

从 Controller 注销指定服务。

#### Heartbeat

```go
func (c *Client) Heartbeat(ctx context.Context, serviceIDs []string) error
```

为指定服务发送心跳。

#### ReportFailure

```go
func (c *Client) ReportFailure(ctx context.Context, serviceID, reason string) error
```

报告服务请求失败。

#### GetServices

```go
func (c *Client) GetServices() []Service
```

获取本地缓存的服务列表。

#### GetService

```go
func (c *Client) GetService(serviceID string) (*Service, bool)
```

获取本地缓存的特定服务。

#### Stop

```go
func (c *Client) Stop()
```

停止客户端并清理资源。

## 完整示例

```go
package main

import (
    "context"
    "log"
    "os"
    "os/signal"
    "syscall"
    "time"
    
    "github.com/houzhh15/sdp-common/service"
    "github.com/houzhh15/sdp-common/cert"
)

func main() {
    // 加载证书
    certManager, _ := cert.NewManager(&cert.Config{
        CertFile: "agent.crt",
        KeyFile:  "agent.key",
        CAFile:   "ca.crt",
    })
    
    // 创建客户端
    client := service.NewClient(&service.Config{
        ControllerURL: "https://controller:8443",
        TLSConfig:     certManager.GetTLSConfig(),
        AgentID:       "agent-001",
    })
    defer client.Stop()
    
    ctx := context.Background()
    
    // 注册服务
    services := []service.Service{
        {
            ID:         "web-app",
            Name:       "Web Application",
            TargetHost: "localhost",
            TargetPort: 8080,
            Protocol:   "http",
            Metadata:   map[string]string{"env": "production"},
        },
    }
    
    if err := client.Register(ctx, services); err != nil {
        log.Fatal(err)
    }
    
    // 启动心跳
    go func() {
        ticker := time.NewTicker(30 * time.Second)
        defer ticker.Stop()
        
        for range ticker.C {
            serviceIDs := []string{"web-app"}
            if err := client.Heartbeat(ctx, serviceIDs); err != nil {
                log.Printf("Heartbeat failed: %v", err)
            }
        }
    }()
    
    // 等待退出信号
    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
    <-quit
    
    // 注销服务
    client.Unregister(ctx, "web-app")
}
```

## 设计原则

1. **协议标准化**: 严格遵循 SDP 协议规范
2. **本地缓存**: 减少网络请求，提高性能
3. **容错性**: 优雅处理网络错误
4. **安全性**: 完整的 mTLS 支持
5. **易用性**: 简单的 API，开箱即用

## 测试

```bash
go test ./service -v
```

## 许可证

Apache License 2.0
