# SDP-Common

[![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat\u0026logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![Coverage](https://img.shields.io/badge/Coverage-80%25+-success)](https://github.com/houzhh15/sdp-common)

通用的 Software-Defined Perimeter (SDP) 2.0 公共库，为 Controller、Initiating Host (IH) 和 Accepting Host (AH) 提供标准化的核心功能实现。

## 🎯 项目简介

`sdp-common` 是一个基于 **SDP 2.0 规范**的 Golang 公共库，提供了以下核心能力：

- ✅ **证书管理**: mTLS 证书加载、验证、指纹计算
- ✅ **会话管理**: Token 生成、验证、生命周期管理
- ✅ **策略引擎**: 可插拔的策略评估和存储
- ✅ **隧道管理**: 数据平面透明代理和控制平面通知
- ✅ **日志审计**: 结构化日志和审计事件记录
- ✅ **传输层抽象**: HTTP/gRPC/SSE/TCP 多协议支持
- ✅ **配置管理**: YAML/JSON 配置加载和验证

### 设计原则

1. **架构合理性优先**: 混合架构，默认使用 HTTP+SSE+TCP（易用性），可选 gRPC（高性能）
2. **性能与灵活性平衡**: Controller 数据平面使用 TunnelRelayServer（IH↔AH 配对中继），IH/AH 客户端使用 TCP Proxy（本地代理），控制平面支持 HTTP/gRPC 双协议
3. **接口标准化**: 统一 Controller、IH、AH 的接口定义
4. **模块化设计**: 各模块高内聚低耦合，支持独立使用

详见 `docs/architecture-decision-analysis.md`

## 🚀 快速开始

### 前置要求

- Go 1.21 或更高版本
- Git

### 安装

```bash
go get github.com/houzhh15/sdp-common@latest
```

### 最小化示例

#### Controller 端

```go
package main

import (
    "github.com/houzhh15/sdp-common/cert"
    "github.com/houzhh15/sdp-common/config"
    "github.com/houzhh15/sdp-common/session"
    "github.com/houzhh15/sdp-common/transport"
)

func main() {
    // 1. 加载配置
    loader := config.NewLoader()
    cfg, _ := loader.Load("config.yaml")
    
    // 2. 初始化证书管理
    certMgr, _ := cert.NewManager(\u0026cert.Config{
        CertFile: cfg.TLS.CertFile,
        KeyFile:  cfg.TLS.KeyFile,
        CAFile:   cfg.TLS.CAFile,
    })
    
    // 3. 初始化会话管理
    sessMgr := session.NewManager(\u0026session.Config{
        TokenTTL: cfg.Auth.TokenTTL,
    }, logger)
    
    // 4. 启动 HTTP 服务器
    httpServer := transport.NewHTTPServer(certMgr.GetTLSConfig())
    httpServer.Start(":8080", handler)
}
```

#### IH Client 端

```go
package main

import (
    "github.com/houzhh15/sdp-common/cert"
    "github.com/houzhh15/sdp-common/tunnel"
)

func main() {
    // 1. 加载证书
    certMgr, _ := cert.NewManager(\u0026cert.Config{
        CertFile: "client-cert.pem",
        KeyFile:  "client-key.pem",
        CAFile:   "ca-cert.pem",
    })
    
    // 2. 握手认证
    client := \u0026http.Client{
        Transport: \u0026http.Transport{
            TLSClientConfig: certMgr.GetTLSConfig(),
        },
    }
    
    // 3. 订阅隧道事件（SSE）
    subscriber := tunnel.NewSubscriber("https://controller:8080", client)
    go subscriber.Start()
    
    for event := range subscriber.Events() {
        handleTunnelEvent(event)
    }
}
```

完整示例请参考 [examples/](examples/) 目录。

## 📦 核心功能

### 1. cert - 证书管理

**核心接口**:
- `Manager`: 证书加载、指纹计算、TLS 配置生成
- `Registry`: 证书注册表、吊销检查
- `Validator`: 证书验证器

**使用示例**:
```go
certMgr, _ := cert.NewManager(\u0026cert.Config{
    CertFile: "cert.pem",
    KeyFile:  "key.pem",
    CAFile:   "ca.pem",
})

// 获取指纹
fingerprint := certMgr.GetFingerprint()

// 验证过期时间
if err := certMgr.ValidateExpiry(); err != nil {
    log.Fatalf("证书已过期: %v", err)
}

// 生成 TLS 配置
tlsConfig := certMgr.GetTLSConfig()
```

详见 [cert/README.md](cert/README.md)

### 2. session - 会话管理

**核心接口**:
- `Manager`: 会话创建、验证、刷新、撤销
- `Session`: 会话对象，包含 Token、过期时间、设备信息

**使用示例**:
```go
sessMgr := session.NewManager(\u0026session.Config{
    TokenTTL: 3600 * time.Second,
}, logger)

// 创建会话
sess, _ := sessMgr.CreateSession(ctx, \u0026session.CreateSessionRequest{
    ClientID:        "ih-001",
    CertFingerprint: fingerprint,
})

// 验证会话
validSess, _ := sessMgr.ValidateSession(ctx, sess.Token)
```

详见 [session/README.md](session/README.md)

### 3. policy - 策略引擎

**核心接口**:
- `Engine`: 策略引擎，评估访问请求
- `Storage`: 策略存储接口（支持数据库、内存等）
- `Evaluator`: 策略评估器，可插拔实现

**使用示例**:
```go
storage := policy.NewDBStorage(db)
evaluator := \u0026policy.DefaultEvaluator{}
engine := policy.NewEngine(storage, evaluator, logger)

// 评估访问
decision, _ := engine.EvaluateAccess(ctx, \u0026policy.AccessRequest{
    ClientID:  "ih-001",
    ServiceID: "web-app",
    SourceIP:  "192.168.1.100",
})

if decision.Allowed {
    // 授权通过
}
```

详见 [policy/README.md](policy/README.md)

### 4. tunnel - 隧道管理

**核心组件**:
- `TCPProxy`: 数据平面透明代理（默认，9443 端口）
- `Notifier`: SSE 实时推送管理器（控制平面通知）
- `Subscriber`: AH 端隧道订阅器（SSE 客户端）
- `Broker`: gRPC 双向流转发（可选）

**使用示例**:
```go
// Controller: 启动 TCP Proxy
proxy := tunnel.NewTCPProxy(tunnelStore, logger)
go proxy.Start(":9443")

// Controller: SSE 推送隧道事件
notifier := tunnel.NewNotifier(logger)
notifier.Notify(\u0026tunnel.TunnelEvent{
    Type:   "created",
    Tunnel: tunnel,
})

// AH Agent: 订阅隧道事件
subscriber := tunnel.NewSubscriber(controllerURL, tlsConfig)
go subscriber.Start()

for event := range subscriber.Events() {
    if event.Type == "created" {
        // 建立数据平面连接
        connectToTCPProxy(event.Tunnel)
    }
}
```

详见 [tunnel/README.md](tunnel/README.md)

### 5. logging - 日志审计

**核心接口**:
- `Logger`: 结构化日志接口（Info/Warn/Error/Debug）
- `AuditLogger`: 审计日志接口（LogAccess/LogConnection/LogSecurity）

**使用示例**:
```go
logger := logging.NewLogger(\u0026logging.Config{
    Level:  "info",
    Format: "json",
    Output: "stdout",
})

auditLogger := logging.NewFileAuditLogger("audit.log", logger)

// 记录访问事件
auditLogger.LogAccess(ctx, \u0026logging.AccessEvent{
    Timestamp: time.Now(),
    ClientID:  "ih-001",
    ServiceID: "web-app",
    Action:    "handshake",
    Result:    "success",
})
```

详见 [logging/README.md](logging/README.md)

### 6. transport - 传输层抽象

**核心接口**:
- `HTTPServer`: HTTP/REST API 服务器（控制平面）
- `SSEServer`: SSE 推送服务器（实时通知）
- `TunnelRelayServer`: Controller 数据平面中继服务器
- `TCPProxyServer`: IH/AH 客户端代理服务器
- `GRPCServer`: gRPC 服务器（可选）

**使用场景说明**:
- **TunnelRelayServer**: Controller 中继 IH↔AH 连接（双向配对转发）
- **TCPProxyServer**: IH/AH 客户端直接连接目标应用（单向代理）

**使用示例**:
```go
// Controller: TunnelRelayServer（数据平面中继）
relayServer := transport.NewTunnelRelayServer(logger, &transport.TunnelRelayConfig{
    PairingTimeout: 30 * time.Second,
    BufferSize:     32 * 1024,
    MaxConnections: 10000,
})
tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
go relayServer.StartTLS(":9443", tlsConfig)

// IH Client: TCPProxyServer（本地代理）
tcpProxy := transport.NewTCPProxyServer(tunnelStore, logger, nil)
go tcpProxy.StartTLS("127.0.0.1:8080", tlsConfig)

// SSE 服务器（实时通知）
sseServer := transport.NewSSEServer()
http.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
    agentID := r.URL.Query().Get("agent_id")
    sseServer.Subscribe(agentID, w)
})
```

详见 [transport/README.md](transport/README.md)

### 7. protocol - 协议定义

**核心内容**:
- 统一错误码（`ErrCodeSuccess`, `ErrCodeUnauthorized`, 等）
- 消息类型常量（`MsgTypeHandshakeReq`, 等）
- 错误封装（`protocol.Error`）

### 8. config - 配置管理

**核心接口**:
- `Loader`: 配置加载器，支持 YAML/JSON
- `Config`: 统一配置结构

**使用示例**:
```go
loader := config.NewLoader()
cfg, _ := loader.Load("config.yaml")

// 验证配置
if err := loader.Validate(cfg); err != nil {
    log.Fatalf("配置无效: %v", err)
}
```

详见 [config/README.md](config/README.md)

## 📊 性能指标

基于 Go 1.21 在 Intel Core i7 (4核8线程) / 16GB RAM 环境下的测试结果：

| 指标 | 数值 | 备注 |
|------|------|------|
| **并发连接** | 10,000+ | 单 Controller 实例 |
| **握手延迟** | \u003c 100ms | P99，包含证书验证 |
| **会话创建** | \u003c 5ms | P99 |
| **策略评估** | \u003c 10ms | P99，简单条件 |
| **隧道配对延迟** | \u003c 10ms | P99，TunnelRelayServer |
| **SSE 推送延迟** | \u003c 100ms | 事件到达时间 |
| **内存占用** | ~200MB | Controller + 1000 会话 |

**性能特点**:
- **TunnelRelayServer**: 零拷贝双向转发
- **配对超时**: 30秒可配置，自动清理过期连接
- **并发支持**: 10,000+ 并发隧道

详细性能测试报告参见 [test/benchmark_test.go](test/benchmark_test.go)

## 💡 使用示例

### 完整的 Controller 初始化

```go
package main

import (
    "context"
    "log"
    "net/http"
    
    "github.com/houzhh15/sdp-common/cert"
    "github.com/houzhh15/sdp-common/config"
    "github.com/houzhh15/sdp-common/logging"
    "github.com/houzhh15/sdp-common/policy"
    "github.com/houzhh15/sdp-common/session"
    "github.com/houzhh15/sdp-common/transport"
    "github.com/houzhh15/sdp-common/tunnel"
    "gorm.io/driver/sqlite"
    "gorm.io/gorm"
)

func main() {
    // 1. 加载配置
    loader := config.NewLoader()
    cfg, err := loader.Load("config.yaml")
    if err != nil {
        log.Fatalf("加载配置失败: %v", err)
    }
    
    // 2. 初始化日志
    logger := logging.NewLogger(\u0026logging.Config{
        Level:  cfg.Logging.Level,
        Format: cfg.Logging.Format,
        Output: cfg.Logging.Output,
    })
    
    auditLogger := logging.NewFileAuditLogger(cfg.Logging.AuditFile, logger)
    
    // 3. 初始化证书管理
    certMgr, err := cert.NewManager(\u0026cert.Config{
        CertFile: cfg.TLS.CertFile,
        KeyFile:  cfg.TLS.KeyFile,
        CAFile:   cfg.TLS.CAFile,
    })
    if err != nil {
        log.Fatalf("初始化证书管理失败: %v", err)
    }
    
    // 4. 初始化数据库
    db, err := gorm.Open(sqlite.Open("sdp.db"), \u0026gorm.Config{})
    if err != nil {
        log.Fatalf("数据库连接失败: %v", err)
    }
    
    // 5. 初始化证书注册表
    certRegistry, err := cert.NewRegistry(db, logger)
    if err != nil {
        log.Fatalf("初始化证书注册表失败: %v", err)
    }
    
    // 6. 初始化会话管理
    sessMgr := session.NewManager(\u0026session.Config{
        TokenTTL: cfg.Auth.TokenTTL,
    }, logger)
    
    // 7. 初始化策略引擎
    policyStorage := policy.NewDBStorage(db)
    policyEvaluator := \u0026policy.DefaultEvaluator{}
    policyEngine := policy.NewEngine(policyStorage, policyEvaluator, logger)
    
    // 8. 初始化隧道存储
    tunnelStore := tunnel.NewMemoryStore()
    
    // 9. 初始化 TunnelRelayServer（数据平面中继）
    relayServer := transport.NewTunnelRelayServer(logger, &transport.TunnelRelayConfig{
        PairingTimeout: 30 * time.Second,
        BufferSize:     32 * 1024,
        MaxConnections: 10000,
    })
    
    // 10. 初始化 SSE 服务器（实时通知）
    sseServer := transport.NewSSEServer()
    
    // 11. 初始化 HTTP 服务器
    httpServer := transport.NewHTTPServer(certMgr.GetTLSConfig())
    
    // 12. 注册 HTTP 路由
    http.HandleFunc("/health", healthHandler)
    http.HandleFunc("/api/v1/handshake", handshakeHandler(sessMgr, certRegistry, auditLogger))
    http.HandleFunc("/api/v1/tunnels", tunnelCreateHandler(tunnelStore, policyEngine, sseServer))
    http.HandleFunc("/api/v1/tunnels/stream", func(w http.ResponseWriter, r *http.Request) {
        agentID := r.URL.Query().Get("agent_id")
        sseServer.Subscribe(agentID, w)
    })
    
    // 13. 启动 TunnelRelayServer（数据平面）
    tlsConfig := certMgr.GetTLSConfig()
    tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
    go func() {
        if err := relayServer.StartTLS(":9443", tlsConfig); err != nil {
            logger.Error("TunnelRelayServer 启动失败", "error", err.Error())
        }
    }()
    
    // 14. 启动 HTTP 服务器
    logger.Info("Controller 启动", "http_addr", ":8080", "relay_addr", ":9443")
    
    if err := httpServer.Start(":8080", nil); err != nil {
        log.Fatalf("HTTP 服务器启动失败: %v", err)
    }
}
```

完整示例代码参见 [examples/controller/main.go](examples/controller/main.go)


### 开发要求

- Go 1.21+
- 单元测试覆盖率 ≥ 80%
- 通过 `golangci-lint` 静态检查
- 遵循 Go 代码规范

### 运行测试

```bash
# 运行所有单元测试
go test ./... -v -cover

# 运行集成测试
go test ./test/integration -v

# 运行性能基准测试
go test ./test -bench=. -benchmem
```

## 📄 许可证

本项目采用 [Apache License 2.0](LICENSE) 许可证。



