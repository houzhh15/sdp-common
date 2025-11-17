# 服务配置管理 - 快速参考

> **SDP 2.0 规范 0x04 消息** - 混合方案（HTTP GET + SSE Push）

## 核心概念

### ServiceConfig - 集中管理的服务配置

```
Controller 管理 ServiceConfig
    ↓
    ├─ HTTP GET /api/v1/services (AH Agent 启动时获取)
    └─ SSE Push (运行时配置更新)
         ↓
    AH Agent 本地缓存
         ↓
    TCP Proxy 查询 Tunnel.Metadata 获取目标地址
```

---

## 当前实现

### 1. Controller 端 - 预置服务配置

```go
// examples/controller/main.go - seedExampleServices()
manager.CreateServiceConfig(ctx, &tunnel.ServiceConfig{
    ServiceID:   "demo-service-001",
    ServiceName: "Demo Web Service",
    TargetHost:  "localhost",
    TargetPort:  9999,
    Protocol:    "tcp",
    Status:      tunnel.ServiceStatusActive,
})
```

### 2. AH Agent 端 - HTTP GET + SSE 订阅

```go
// 启动时：HTTP GET 获取初始配置
services, err := fetchServiceConfigs(controllerURL, tlsConfig)

// 运行时：SSE 订阅配置更新
subscriber := tunnel.NewSubscriber(&tunnel.SubscriberConfig{
    ControllerURL: controllerURL,
    Callback: func(event *tunnel.TunnelEvent) error {
        if eventType == "service_updated" {
            updateLocalService(serviceID, targetHost, targetPort)
        }
        return nil
    },
})
```

### 3. IH Client 端 - 创建隧道

```go
// 创建隧道时仅需 ServiceID
tunnelResp := createTunnel(ctx, &tunnel.CreateTunnelRequest{
    ClientID:  "ih-001",
    ServiceID: "demo-service-001", // Controller 自动查询 ServiceConfig
    Protocol:  "tcp",
})
```

---

## 架构优势

| 特性 | 实现方式 |
|-----|---------|
| **配置管理** | Controller 集中管理 ServiceConfig |
| **初始加载** | HTTP GET /api/v1/services（< 100ms） |
| **实时更新** | SSE Push（< 50ms P99） |
| **控制/数据分离** | Tunnel 不包含 TargetHost/Port |
| **场景覆盖** | 100%（启动 + 运行时） |

---

## HTTP API

### 列出所有服务配置

```bash
GET /api/v1/services
Authorization: (需要客户端证书)

Response:
{
  "status": "success",
  "services": [
    {
      "service_id": "demo-service-001",
      "service_name": "Demo Web Service",
      "target_host": "localhost",
      "target_port": 9999,
      "protocol": "tcp",
      "status": "active"
    }
  ],
  "count": 1
}
```

### 获取单个服务配置

```bash
GET /api/v1/services/{service_id}

Response:
{
  "status": "success",
  "service": { /* ServiceConfig */ }
}
```

---

## 数据流

```
Controller 管理员
    ↓
CreateServiceConfig("demo-service-001", "localhost:9999")
    ↓
存储到 Manager.services
    ↓
AH Agent 启动 → HTTP GET /api/v1/services
              → 缓存到本地 serviceConfigs map
    ↓
IH Client 创建隧道 {service_id: "demo-service-001"}
    ↓
Controller.CreateTunnel()
    → 查询 ServiceConfig
    → 填充 Tunnel.Metadata["target_host/port"]
    → SSE Push 通知 AH Agent
    ↓
AH Agent 接收隧道事件
    → TCP Proxy 接受连接
    → 从 Tunnel.Metadata 获取目标地址
    → 连接后端服务
```

---

## 配置对应关系

| 位置 | 字段 | 说明 |
|-----|------|-----|
| Controller | `ServiceConfig.ServiceID` | 服务唯一标识 |
| Controller | `ServiceConfig.TargetHost/Port` | 后端服务地址 |
| Tunnel | `Tunnel.ServiceID` | 关联到 ServiceConfig |
| Tunnel | `Tunnel.Metadata["target_*"]` | 内部存储目标地址（仅 TCP Proxy 使用） |

**一致性**：通过 `ServiceID` 关联，`TargetHost/Port` 不通过控制平面传输。

---

## 快速测试

```bash
# 1. 启动 Controller（已预置 demo-service-001）
./bin/controller-example

# 2. 启动 AH Agent（自动从 Controller 获取服务配置）
./bin/ah-agent-example

# 3. 验证服务配置
curl -k --cert certs/ah-agent-cert.pem --key certs/ah-agent-key.pem \
  https://localhost:8443/api/v1/services
# → 返回 demo-service-001

# 4. IH Client 创建隧道
./bin/ih-client-example
curl http://localhost:8080
# → 数据转发：IH → Controller → AH → localhost:9999
```

---

## 相关文档

- [API 参考手册](../SDP_COMMON_API_REFERENCE.md#52-serviceconfig---服务配置管理)
- [服务注册流程](SERVICE_REGISTRATION_FLOW.md)
- [协议合规分析](SDP2.0_PROTOCOL_MAPPING.md)

