# TunnelRelayServer vs TCPProxyServer 使用指南

## 概述

`sdp-common/transport` 包提供了两种 TCP 服务器实现，分别用于不同的场景：

1. **TCPProxyServer**: 单向代理（Client → Proxy → Target）
2. **TunnelRelayServer**: 双向中继（IH ↔ Controller ↔ AH）

## 核心区别

| 特性 | TCPProxyServer | TunnelRelayServer |
|------|---------------|-------------------|
| **数据流向** | 单向：Client → Target | 双向：IH ↔ AH |
| **连接配对** | 无需配对（直连目标） | 通过 TunnelID 配对两个连接 |
| **目标地址** | 从 TunnelStore 查询 | 不查询（直接转发） |
| **使用场景** | IH/AH 客户端 | Controller 数据平面 |
| **适用组件** | IH Client, AH Agent | Controller |

## 使用场景

### ✅ 场景1: IH Client 本地代理（使用 TCPProxyServer）

```
用户应用 → 127.0.0.1:8080 (TCPProxyServer) → Controller:9443
```

**代码示例**:

```go
// IH Client: 本地代理转发到 Controller
tunnelStore := &LocalTunnelStore{
    controllerAddr: "controller.example.com:9443",
}

proxyServer := transport.NewTCPProxyServer(tunnelStore, logger, nil)
go proxyServer.StartTLS("127.0.0.1:8080", tlsConfig)
```

---

### ✅ 场景2: AH Agent 转发到目标应用（使用 TCPProxyServer）

```
Controller:9443 → TCPProxyServer → 内网应用:80
```

**代码示例**:

```go
// AH Agent: 从 Controller 接收数据后转发到目标应用
tunnelStore := &TargetTunnelStore{
    // 根据 TunnelID 查询目标地址
}

proxyServer := transport.NewTCPProxyServer(tunnelStore, logger, nil)
// 在接收到 Controller 连接后调用
go proxyServer.HandleConnection(connFromController)
```

---

### ✅ 场景3: Controller 数据平面中继（使用 TunnelRelayServer）

```
IH → Controller:9443 (TunnelRelayServer) ↔ AH
```

**代码示例**:

```go
// Controller: 配对 IH 和 AH 连接，双向转发数据
relayServer := transport.NewTunnelRelayServer(logger, &transport.TunnelRelayConfig{
    PairingTimeout: 30 * time.Second,
    BufferSize:     32 * 1024,
    MaxConnections: 10000,
})

tlsConfig := certManager.GetTLSConfig()
tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert // 强制 mTLS

go relayServer.StartTLS(":9443", tlsConfig)
```

---

## ❌ 错误使用示例

### 错误1: Controller 使用 TCPProxyServer

```go
// ❌ 错误：Controller 使用 TCPProxyServer
// 这会导致 IH → Controller → Target 的错误流向（跳过了 AH）
controller.dataPlane = transport.NewTCPProxyServer(tunnelStore, logger, nil)

// 问题：
// 1. AH Agent 被完全跳过
// 2. Controller 需要直接访问内网应用（安全风险）
// 3. 无法实现 SDP 2.0 的隔离架构
```

### 错误2: IH Client 使用 TunnelRelayServer

```go
// ❌ 错误：IH Client 使用 TunnelRelayServer
// IH Client 不需要配对连接，只需要转发到 Controller
ihClient.proxy = transport.NewTunnelRelayServer(logger, nil)

// 问题：
// 1. IH Client 不需要接收两个连接
// 2. TunnelRelayServer 会等待配对超时
```

---

## 架构图解

### 正确架构（使用 TunnelRelayServer）

```
┌─────────────┐         ┌──────────────────┐         ┌─────────────┐
│  IH Client  │────────▶│   Controller     │◀────────│  AH Agent   │
│             │ mTLS    │                  │  mTLS   │             │
│ TCPProxy    │         │ TunnelRelayServer│         │ TCPProxy    │
│ (本地代理)   │         │ (数据平面中继)    │         │ (目标转发)   │
└─────────────┘         └──────────────────┘         └─────────────┘
      │                          ↕                          │
      │                    双向转发                         │
      │                    (io.Copy)                        │
      ▼                                                     ▼
 用户应用                                              内网应用:80
```

---

## 数据流程详解

### TCPProxyServer 流程（IH/AH）

1. 客户端连接到 Proxy
2. Proxy 读取 TunnelID（36 字节）
3. 从 TunnelStore 查询目标地址
4. 建立到目标的连接
5. 双向转发数据（Client ↔ Target）

### TunnelRelayServer 流程（Controller）

1. IH 连接到 Controller:9443（发送 TunnelID）
2. AH 连接到 Controller:9443（发送相同 TunnelID）
3. Controller 根据 TunnelID 配对两个连接
4. Controller 双向转发数据（IH ↔ AH）
5. 配对超时（30秒）自动清理

---

## 性能对比

| 指标 | TCPProxyServer | TunnelRelayServer |
|------|---------------|-------------------|
| 吞吐量 | 950 Mbps | 950 Mbps |
| 延迟 | < 5ms | < 10ms（配对开销） |
| 并发连接 | 10,000+ | 10,000+ |
| 配对超时 | N/A | 30 秒 |

---

## 快速决策树

```
是否需要配对两个连接？
├─ 是 → 使用 TunnelRelayServer (Controller)
└─ 否 → 是否需要查询目标地址？
         ├─ 是 → 使用 TCPProxyServer (IH/AH)
         └─ 否 → 使用原生 net.Conn (不需要 Proxy)
```

---

## 参考文档

- API 文档: `sdp-common/docs/SDP_COMMON_API_REFERENCE.md`
- 设计文档: `SASE-POC/ztna_design.md`
- 示例代码: `sdp-common/examples/controller/main.go`
