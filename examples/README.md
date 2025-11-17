# sdp-common 使用示例

本目录包含了 sdp-common 包的完整使用示例。

## � 接口覆盖率统计

以下是 sdp-common 包的接口演示覆盖率：

| 包名 | 核心接口数 | 已演示接口 | 覆盖率 | 说明 |
|------|-----------|-----------|--------|------|
| **cert** | 10 | 10 | 100% | Manager (6/6), Registry (3/3), Validator (1/1) |
| **session** | 4 | 4 | 100% | Manager (Create/Validate/Refresh/Revoke) |
| **policy** | 4 | 4 | 100% | Engine (GetPolicies/EvaluateAccess), Storage (2/2) |
| **tunnel** | 9 | 9 | 100% | Manager (5/5), Notifier (2/2), Subscriber (2/2) |
| **logging** | 4 | 4 | 100% | Logger (Info/Warn/Error/Debug) |
| **transport** | 6 | 6 | 100% | HTTPServer (3/3), TCPProxyServer (3/3) |
| **protocol** | 3 | 3 | 100% | Error (NewError/WrapError/WithDetails) |
| **config** | 3 | 2 | 67% | Loader (Load/Validate, Watch 未演示) |
| **总计** | **43** | **42** | **98%** | 已达成目标 (>85%) |

**未演示接口**:
- `config.Loader.Watch()` - 配置热重载（示例场景不需要）

---

## �🚀 快速开始 (端到端测试)

**最简单的方式** - 一键启动所有组件:

```bash
cd sdp-common
bash scripts/e2e-test.sh
```

然后在另一个终端测试:

```bash
curl http://localhost:8080
```

详细说明请参考 [端到端测试指南](E2E_TEST_GUIDE.md)。

---

## 手动启动步骤

### 1. 生成证书

所有示例都需要 TLS 证书，首次运行前请执行：

```bash
cd sdp-common
./scripts/generate-certs.sh
```

这将在 `certs/` 目录生成所需的证书文件：
- `ca-cert.pem` / `ca-key.pem` - CA 证书
- `controller-cert.pem` / `controller-key.pem` - Controller 证书
- `ih-client-cert.pem` / `ih-client-key.pem` - IH Client 证书
- `ah-agent-cert.pem` / `ah-agent-key.pem` - AH Agent 证书

### 2. 编译示例

每个示例都是独立的 Go 模块：

```bash
# 编译 Controller
cd examples/controller
go build -o controller-example

# 编译 IH Client
cd ../ih-client
go build -o ih-client-example

# 编译 AH Agent
cd ../ah-agent
go build -o ah-agent-example
```

### 3. 运行示例

**重要**: 必须从示例所在目录运行,以确保相对路径 `../../certs/` 正确:

```bash
# 运行 Controller (终端 1)
cd examples/controller
./controller-example

# 运行 IH Client (终端 2)
cd examples/ih-client
./ih-client-example

# 运行 AH Agent (终端 3)
cd examples/ah-agent
./ah-agent-example
```

### 4. 查看帮助

所有示例都支持命令行参数，使用 `-h` 查看：

```bash
./controller-example -h
./ih-client-example -h
./ah-agent-example -h
```

### 3. 自定义配置

#### 使用配置文件（推荐）

示例现在支持 YAML 配置文件，提供更灵活的配置管理：

```bash
# Controller - 使用配置文件
cd examples/controller
./controller-example -config ../../configs/controller.yaml

# IH Client - 使用配置文件
cd examples/ih-client
./ih-client-example -config ../../configs/ih-client.yaml

# AH Agent - 使用配置文件
cd examples/ah-agent
./ah-agent-example -config ../../configs/ah-agent.yaml
```

配置文件示例位于 `examples/configs/` 目录：
- `controller.yaml` - Controller 配置（组件信息、TLS 证书、认证、策略、日志、传输层）
- `ih-client.yaml` - IH Client 配置（组件信息、TLS 证书、Controller 地址、本地代理、日志、隧道）
- `ah-agent.yaml` - AH Agent 配置（组件信息、TLS 证书、日志、传输层）

**配置优先级**: 配置文件 > 命令行参数

#### 使用命令行参数

示例使用命令行参数而非配置文件，便于快速测试：

```bash
# Controller - 自定义端口和证书
./controller-example -addr :9443 -cert /path/to/cert.pem -key /path/to/key.pem

# IH Client - 连接到自定义 Controller
./ih-client-example -controller https://192.168.1.100:8443

# AH Agent - 使用自定义日志级别
./ah-agent-example -log-level debug
```

## 目录结构

```
examples/
├── controller/           # Controller示例
│   ├── main.go          # 使用命令行参数配置
│   ├── go.mod           # 独立模块配置
│   ├── go.sum
│   └── controller-example (编译后)
├── ih-client/           # IH Client示例
│   ├── main.go
│   ├── go.mod
│   ├── go.sum
│   └── ih-client-example (编译后)
└── ah-agent/            # AH Agent示例
    ├── main.go
    ├── go.mod
    ├── go.sum
    └── ah-agent-example (编译后)
```

## 示例说明

### 1. Controller 示例 (`controller/`)

演示如何使用 sdp-common 包初始化一个完整的 SDP Controller：

- ✅ 证书管理（cert.Manager）
- ✅ 证书验证和有效期检查
- ✅ 证书注册表（cert.Registry）
- ✅ 隧道通知（tunnel.Notifier）- SSE 推送给 AH Agent
- ✅ **TCP Proxy 数据平面** - 接收 IH Client 连接
- ✅ HTTPS 控制平面 API 服务器
- ✅ 审计日志（logging.Logger）

**提供的服务：**
- **HTTPS API (8443):**
  - `GET /health` - 健康检查
  - `POST /api/v1/handshake` - 客户端证书握手，返回 session token
  - `POST /api/v1/sessions/refresh` - 刷新会话 token
  - `DELETE /api/v1/sessions/{token}` - 撤销会话
  - `GET /api/v1/policies?client_id={id}` - 查询客户端授权策略列表
  - `POST /api/v1/tunnels` - 创建新隧道
  - `GET /api/v1/tunnels/{id}` - 查询隧道信息
  - `DELETE /api/v1/tunnels/{id}` - 关闭隧道
  - `GET /v1/agent/tunnels/stream` - SSE 隧道事件流(供 AH Agent 订阅)
- **TCP Proxy (9443):**
  - 接收 IH Client TLS 连接
  - 读取 Tunnel ID
  - 转发到对应的 AH Agent

**命令行参数：**
```bash
./controller-example -h
  -addr string
        HTTPS server address (default ":8443")
  -proxy-addr string
        TCP proxy address for IH Client connections (default ":9443")
  -ca string
        CA certificate file (default "../../certs/ca-cert.pem")
  -cert string
        Certificate file (default "../../certs/controller-cert.pem")
  -key string
        Private key file (default "../../certs/controller-key.pem")
  -log-level string
        Log level (default "info")
```

**运行：**
```bash
cd examples/controller
./controller-example

# 使用自定义配置
./controller-example -addr :9443 -proxy-addr :9444 -log-level debug
```

**预期输出：**
```
[2025-11-16T14:33:09+08:00] INFO: Controller starting map[version:1.0.0-example]
Certificate loaded, fingerprint: sha256:ba017db2...

✅ Controller started successfully!
   HTTPS Server: https://localhost:8443
   TCP Proxy:    localhost:9443 (for IH Client)
   Health Check: https://localhost:8443/health
   Press Ctrl+C to stop

[2025-11-16T14:33:09+08:00] INFO: Starting HTTPS server map[addr::8443]
[2025-11-16T14:33:09+08:00] INFO: TCP Proxy listening map[addr::9443]
```

### 2. IH Client 示例 (`ih-client/`)

演示完整的 IH（Initiating Host）客户端,**提供本地代理服务**:

- ✅ 证书加载和验证（cert.Manager）
- ✅ TLS 配置（用于 mTLS）
- ✅ **本地 TCP 代理服务器** (监听本地端口)
- ✅ 连接到 Controller TCP Proxy
- ✅ 双向数据转发 (用户 ↔ 远程服务)
- ✅ 连接管理和监控
- ✅ 优雅关闭

**工作原理:**
```
用户应用(curl/浏览器)
    ↓ 连接
[本地代理: localhost:8080]  ← IH Client 监听这里
    ↓ 加密隧道
Controller TCP Proxy
    ↓
AH Agent
    ↓
目标服务(内网)
```

**命令行参数：**
```bash
./ih-client-example -h
  -ca string
        CA certificate file path (default "../../certs/ca-cert.pem")
  -cert string
        Certificate file path (default "../../certs/ih-client-cert.pem")
  -controller string
        Controller URL (default "https://localhost:8443")
  -key string
        Private key file path (default "../../certs/ih-client-key.pem")
  -local string
        Local proxy listen address (default "localhost:8080")
  -log-level string
        Log level (debug, info, warn, error) (default "info")
  -proxy string
        Controller TCP proxy address (default "localhost:9443")
  -tunnel-id string
        Tunnel ID for this connection (default "tunnel-12345678")
```

**运行：**
```bash
cd examples/ih-client
./ih-client-example

# 自定义配置
./ih-client-example -local localhost:8888 -proxy controller:9443

# 连接后测试
curl http://localhost:8080
# 或在浏览器访问: http://localhost:8080
```

**预期输出：**
```
{"level":"INFO","message":"IH Client Proxy starting","fields":{"version":"1.0.0-proxy"}}
{"level":"INFO","message":"Certificate loaded","fields":{"fingerprint":"sha256:a07c07c8..."}}
{"level":"INFO","message":"Local proxy listening","fields":{"addr":"localhost:8080"}}

✅ IH Client Proxy started successfully!

📍 Configuration:
   Local Address:  localhost:8080  (用户连接这里)
   Proxy Address:  localhost:9443  (连接到 Controller)
   Tunnel ID:      tunnel-12345678
   Controller:     https://localhost:8443

💡 使用方法:
   curl http://localhost:8080
   或在浏览器访问: http://localhost:8080
```

**用户连接时的日志:**
```json
{"level":"INFO","message":"New connection","fields":{"id":"conn-1","from":"127.0.0.1:52102"}}
{"level":"INFO","message":"Connecting to proxy","fields":{"id":"conn-1","addr":"localhost:9443"}}
{"level":"INFO","message":"Proxy connection established","fields":{"id":"conn-1"}}
```
```bash
./ih-client-example -h
  -ca string
        CA certificate file (default "../../certs/ca-cert.pem")
  -cert string
        Certificate file (default "../../certs/ih-client-cert.pem")
  -controller string
        Controller URL (default "https://localhost:8443")
  -key string
        Private key file (default "../../certs/ih-client-key.pem")
  -log-level string
        Log level (default "info")
```

**运行：**
```bash
cd examples/ih-client
./ih-client-example

# 连接到自定义 Controller
./ih-client-example -controller https://192.168.1.100:8443
```

**预期输出：**
```
[2025-11-16T13:37:24+08:00] INFO: IH Client starting
Certificate loaded, fingerprint: sha256:a07c07c8...

📋 Certificate Information:
   Subject: CN=ih-client,O=IH-Client
   Valid Until: 2026-11-16 (364 days remaining)

✅ IH Client started successfully!
   Controller: https://localhost:8443
   Client ID: sha256:a07c07c8...
   Tunnel ID: tunnel-12345678
```

### 3. AH Agent 示例 (`ah-agent/`)

演示如何使用 sdp-common 包初始化一个 AH（Accepting Host）代理：

- ✅ 证书管理（cert.Manager）
- ✅ SSE 事件订阅（tunnel.Subscriber）
- ✅ **多服务注册**（Per SDP 2.0 规范）
- ✅ 隧道生命周期管理
- ✅ 基于 ServiceID 的路由
- ✅ 处理隧道创建/删除事件
- ✅ 建立到目标服务的连接
- ✅ 数据双向转发（Proxy ↔ Target Service）

**命令行参数：**
```bash
./ah-agent-example -h
  -agent-id string
        Agent ID (default "ah-agent-001")
  -ca string
        CA certificate file path (default "../../certs/ca-cert.pem")
  -cert string
        Certificate file path (default "../../certs/ah-agent-cert.pem")
  -controller string
        Controller URL (default "https://localhost:8443")
  -key string
        Private key file path (default "../../certs/ah-agent-key.pem")
  -log-level string
        Log level (debug, info, warn, error) (default "info")
```

> **重要变更** (2025-11-17): AH Agent 不再使用 `-services` 参数。
> 服务配置通过 Controller HTTP GET /api/v1/services 获取（混合方案）。

**运行示例：**

```bash
cd examples/ah-agent
./ah-agent-example

# 连接到远程 Controller
./ah-agent-example -controller https://controller.example.com:8443
```

**服务配置管理：**
```go
// Controller 端预置服务配置
manager.CreateServiceConfig(ctx, &tunnel.ServiceConfig{
    ServiceID:  "demo-service-001",
    TargetHost: "localhost",
    TargetPort: 9999,
    Protocol:   "tcp",
})

// AH Agent 启动时自动获取：
// 1. HTTP GET /api/v1/services（初始加载）
// 2. SSE 订阅配置更新（运行时）
```

**预期输出：**
```
{"timestamp":"2025-11-17T14:02:36+08:00","level":"INFO","message":"AH Agent 启动"}
{"timestamp":"2025-11-17T14:02:36+08:00","level":"INFO","message":"注册服务",
 "fields":{"service_id":"web-service","target":"localhost:8080"}}
{"timestamp":"2025-11-17T14:02:36+08:00","level":"INFO","message":"注册服务",
 "fields":{"service_id":"postgres-db","target":"localhost:5432"}}
{"timestamp":"2025-11-17T14:02:36+08:00","level":"INFO","message":"证书加载成功",
 "fields":{"fingerprint":"sha256:a3c8ef24...","days_until_expiry":364}}

✅ AH Agent started successfully!
   Controller: https://localhost:8443
   Agent ID: ah-agent-001
   Registered Services: 2
     - web-service → localhost:8080
     - postgres-db → localhost:5432
   Press Ctrl+C to stop
```

**使用场景说明（SDP 2.0 多服务架构）：**

AH Agent 是内网服务的代理，**一个 Agent 可以代理多个后端服务**，根据 ServiceID 动态路由：

1. **启动目标服务**：
```bash
# 启动 Web 服务
python -m http.server 8080 &

# 启动 PostgreSQL（假设已安装）
# postgres -D /var/lib/postgresql/data &

# 启动 Redis（假设已安装）
# redis-server --port 6379 &
```

2. **启动 AH Agent（自动从 Controller 获取服务配置）**：
```bash
./ah-agent-example
```

3. **服务配置管理（Controller 端预置）**：
```go
// examples/controller/main.go
// 使用 Controller SDK 的 AddService 方法添加服务配置
ctrl.AddService("demo-service-001", "localhost", 9999)

// 内部创建 ServiceConfig:
// ServiceConfig{
//     ServiceID:  "demo-service-001",
//     TargetHost: "localhost",
//     TargetPort: 9999,
//     Protocol:   "tcp",
// }
```

4. **隧道创建流程**：
   - IH Client 创建隧道，指定 `ServiceID`
   - Controller 查询 `ServiceConfig` 获取目标地址（从 ServiceConfig 表）
   - SSE 推送隧道事件给 AH Agent（包含 TargetHost:Port）
   - AH Agent 根据隧道事件中的目标地址建立连接
   - TCP Proxy 完成双向数据转发

4. **工作流程**：
```
IH Client 请求访问 "postgres-db"
    ↓
Controller 查询策略，允许访问
    ↓
Controller 创建隧道（ServiceID="postgres-db"）
    ↓ SSE 推送
AH Agent 收到事件，查找 serviceID → localhost:5432
    ↓
建立双向连接: TCP Proxy ↔ AH Agent ↔ PostgreSQL
    ↓
数据透明转发
```

**优势**：
- ✅ **一个进程管理多个服务**：无需为每个后端服务启动独立 Agent
- ✅ **符合 SDP 2.0 规范**：ServiceID 是策略评估的核心
- ✅ **灵活扩展**：新增服务只需修改配置，无需代码变更
- ✅ **资源高效**：共享 SSE 订阅连接和证书管理

- ✅ **灵活扩展**：新增服务只需修改配置，无需代码变更
- ✅ **符合微服务架构理念**

---

## 服务注册与发现机制

### 当前实现：ServiceConfig + Policy 分离架构

**本示例采用 SDP 2.0 标准架构**，适合演示和生产部署：

#### 1. 架构设计（关注点分离）

**ServiceConfig（服务配置）** 和 **Policy（授权策略）** 完全分离：

```go
// ServiceConfig - 管理服务部署信息（由 Controller 管理）
type ServiceConfig struct {
    ServiceID  string   // 服务唯一标识
    TargetHost string   // 目标主机地址
    TargetPort int      // 目标端口
    Protocol   string   // 协议类型
}

// Policy - 管理访问授权（由 Controller 管理）
type Policy struct {
    PolicyID   string   // 策略唯一标识
    ClientID   string   // 哪个 IH Client 可以访问
    ServiceID  string   // 可以访问哪个服务（关联 ServiceConfig）
    // 注意：不包含 TargetHost/TargetPort（从 ServiceConfig 获取）
}
```

**设计优势**：
- ✅ **单一职责**：Policy 只管授权，ServiceConfig 只管部署
- ✅ **灵活迁移**：服务迁移到新地址，只需更新 ServiceConfig，不影响授权策略
- ✅ **避免冗余**：一个服务配置，多个 Policy 引用，无数据重复
- ✅ **符合 SDP 2.0**：ServiceID 是授权评估的核心标识

#### 2. Controller 端预配置

```go
// Controller 启动时预配置（examples/controller/main.go）

// 1. 添加服务配置
ctrl.AddService("demo-service-001", "localhost", 9999)

// 2. 添加授权策略（不包含 TargetHost/TargetPort）
ctrl.AddPolicy(&policy.Policy{
    PolicyID:  "policy-allow-ih-client",
    ClientID:  "ih-client",              // 哪个 IH 可以访问
    ServiceID: "demo-service-001",       // 可以访问哪个服务
    // TargetHost/TargetPort 从 ServiceConfig 查询
})
```

#### 3. AH Agent 工作模式

**AH Agent 不需要预配置服务列表**，通过 SSE 实时接收隧道事件：

```go
// AH Agent 订阅隧道事件
subscriber := tunnel.NewSubscriber(controllerURL, tlsConfig)
go subscriber.Start()

for event := range subscriber.Events() {
    // 事件中包含目标地址（来自 ServiceConfig）
    tunnel := event.Tunnel
    // tunnel.TargetHost = "localhost"
    // tunnel.TargetPort = 9999
    
    // 建立到目标服务的连接
    conn, _ := net.Dial("tcp", fmt.Sprintf("%s:%d", 
        tunnel.TargetHost, tunnel.TargetPort))
    
    // 与 TCP Proxy 交换数据
    handleTunnelConnection(conn, tunnel)
}
```

**工作流程**：
1. AH Agent 启动，订阅 Controller 的 SSE 推送
2. 当有新隧道创建时，SSE 推送包含：
   - TunnelID
   - ServiceID
   - **TargetHost / TargetPort**（从 ServiceConfig 查询）
3. AH Agent 根据事件中的目标地址建立连接
4. 无需本地维护服务配置列表

#### 4. IH Client 服务发现

IH Client 通过查询策略 API 发现可访问的服务：

```bash
# 1. 认证获取 token
curl -X POST https://controller:8443/api/v1/handshake \
  --cert ih-client-cert.pem --key ih-client-key.pem \
  -d '{"client_id": "ih-001", "fingerprint": "sha256:..."}'
# 返回: {"session_token": "abc123..."}

# 2. 查询可访问的服务（服务发现）
curl -X GET https://controller:8443/api/v1/policies \
  -H "Authorization: Bearer abc123..."
# 返回: {"policies": [
#   {
#     "service_id": "demo-service",
#     "target_host": "localhost",
#     "target_port": 8080
#   }
# ]}

# 3. 选择服务并创建隧道
curl -X POST https://controller:8443/api/v1/tunnels \
  -H "Authorization: Bearer abc123..." \
  -d '{"service_id": "demo-service"}'
```

#### 4. 完整流程图

```
┌─────────────┐         ┌─────────────┐         ┌─────────────┐
│  管理员配置  │         │  Controller │         │  AH Agent   │
└──────┬──────┘         └──────┬──────┘         └──────┬──────┘
       │                       │                       │
       │ 1. 配置策略            │                       │
       │ {ServiceID:"demo"}    │                       │
       └──────────────────────►│                       │
                               │                       │
                               │  2. 启动并加载配置     │
                               │     service_id:"demo" │
                               │◄──────────────────────┤
                               │  3. SSE 订阅隧道事件   │
                               │◄──────────────────────┤
       ┌─────────────┐         │                       │
       │  IH Client  │         │                       │
       └──────┬──────┘         │                       │
              │ 4. 认证        │                       │
              ├───────────────►│                       │
              │ 5. 查询策略    │                       │
              │   (服务发现)   │                       │
              ├───────────────►│                       │
              │◄───────────────┤                       │
              │ 返回: "demo"   │                       │
              │                │                       │
              │ 6. 创建隧道    │                       │
              │ service:"demo" │                       │
              ├───────────────►│                       │
              │                │ 7. SSE 推送           │
              │                │   TunnelCreated       │
              │                │   service:"demo"      │
              │                ├──────────────────────►│
              │                │                       │
              │                │                8. 查找本地配置
              │                │                   "demo"→localhost:8080
              │                │                       │
              │                │                9. 连接目标服务
              │                │◄──────────────────────┤
              │◄───────────────┤                       │
              │ {tunnel_id,    │                       │
              │  proxy:9443}   │                       │
              │                │                       │
              │ 10. 数据传输   │                       │
              ├───────────────►│◄─────────────────────►│
                     TCP Proxy (9443)        目标服务 (8080)
```

### 生产环境推荐：动态服务注册

详细的动态服务注册实现方案，请参考：**[SERVICE_REGISTRATION_FLOW.md](../SERVICE_REGISTRATION_FLOW.md)**

**主要改进**：
- ✅ AH Agent 启动时主动向 Controller 注册服务
- ✅ Controller 维护服务注册表（Service Registry）
- ✅ 支持服务健康检查和心跳
- ✅ IH Client 查询实时可用的服务列表
- ✅ 策略只管授权，服务注册表管可用性

---

## 完整示例：端到端测试

## 编译故障排除

### 问题：缺少 go.mod 或 go.sum

如果遇到编译错误 "no required module provides package"，说明缺少依赖文件。

**解决方法：**
```bash
cd examples/[示例目录]
go mod tidy  # 重新生成依赖
go build     # 重新编译
```

### 问题：找不到 sdp-common 包

示例使用 `replace` 指令引用本地 sdp-common 包：

```go
// go.mod
replace github.com/houzhh15/sdp-common => ../..
```

确保你在正确的目录结构下运行。

### 问题：证书文件不存在

运行示例前需要生成证书：

```bash
cd sdp-common
./scripts/generate-certs.sh
```

## 配置文件说明

### http-sse-tcp.yaml - 混合架构配置

**适用场景：** 标准部署，控制平面使用 HTTP+SSE，数据平面使用 TCP

**特点：**
- 控制平面：HTTP API + SSE 事件推送
- 数据平面：TCP Proxy
- 支持大规模客户端连接
- 配置简单，易于部署

### grpc-unified.yaml - gRPC 统一架构

**适用场景：** 高性能需求，控制平面和数据平面统一使用 gRPC

**特点：**
- 统一使用 gRPC 协议
- 高吞吐量、低延迟
- 支持双向流
- 需要 TLS 1.3 和 mTLS
- 适合内部高性能场景

### high-performance.yaml - 高性能调优

**适用场景：** 大规模部署，需要最大化吞吐量

**特点：**
- 支持 2000+ 并发隧道
- 64KB 数据缓冲区
- 连接池优化
- 缓存预加载
- 详细的性能监控
- Prometheus + pprof 集成

### development.yaml - 开发环境配置

**适用场景：** 本地开发和调试

**特点：**
- 详细的 debug 日志
- SQLite 数据库（无需安装 PostgreSQL）
- 热重载支持
- Swagger API 文档
- CORS 支持（前端开发）
- 宽松的策略（默认允许）
- 启用 pprof 性能分析

## 证书准备

运行示例前，请确保已生成证书文件：

```bash
cd ../..
./scripts/generate-certs.sh
```

这将生成以下证书：
- `certs/ca-cert.pem` / `certs/ca-key.pem` - CA 证书
- `certs/controller-cert.pem` / `certs/controller-key.pem` - Controller 证书
- `certs/ih-client-cert.pem` / `certs/ih-client-key.pem` - IH Client 证书
- `certs/ah-agent-cert.pem` / `certs/ah-agent-key.pem` - AH Agent 证书

## cert 包使用示例

### 基本使用 - Manager

```go
import "github.com/houzhh15/sdp-common/cert"

// 加载证书
certMgr, err := cert.NewManager(&cert.Config{
    CertFile: "certs/controller-cert.pem",
    KeyFile:  "certs/controller-key.pem",
    CAFile:   "certs/ca-cert.pem",
})
if err != nil {
    log.Fatal(err)
}

// 获取指纹
fingerprint := certMgr.GetFingerprint()
fmt.Println("Fingerprint:", fingerprint)

// 验证有效期
if err := certMgr.ValidateExpiry(); err != nil {
    log.Fatal("Certificate expired:", err)
}

// 检查距离过期天数
days := certMgr.DaysUntilExpiry()
if days < 30 {
    log.Printf("Warning: Certificate expires in %d days", days)
}

// 获取TLS配置（用于服务器）
tlsConfig := certMgr.GetTLSConfig()
server := &http.Server{
    Addr:      ":8443",
    TLSConfig: tlsConfig,
}
```

### 高级使用 - Registry

```go
import (
    "github.com/houzhh15/sdp-common/cert"
    "gorm.io/driver/sqlite"
    "gorm.io/gorm"
)

// 创建数据库连接
db, err := gorm.Open(sqlite.Open("certs.db"), &gorm.Config{})
if err != nil {
    log.Fatal(err)
}

// 创建证书注册表
registry, err := cert.NewRegistry(db, logger)
if err != nil {
    log.Fatal(err)
}

// 注册客户端证书
err = registry.Register("client-001", fingerprint, x509Cert)
if err != nil {
    log.Fatal(err)
}

// 查询证书信息
certInfo, err := registry.GetCertInfo(fingerprint)
if err != nil {
    log.Fatal(err)
}
fmt.Printf("Client: %s, Status: %s\n", certInfo.ClientID, certInfo.Status)

// 验证证书状态
if err := registry.Validate(fingerprint); err != nil {
    log.Fatal("Certificate invalid:", err)
}

// 吊销证书
err = registry.Revoke(fingerprint, "compromised")
if err != nil {
    log.Fatal(err)
}

// 清理过期证书
count, err := registry.CleanExpired()
fmt.Printf("Cleaned %d expired certificates\n", count)
```

### 证书验证 - Validator

```go
import "github.com/houzhh15/sdp-common/cert"

// 创建验证器
validator := cert.NewValidator(&cert.ValidatorConfig{
    CACertPool: caCertPool,
    CheckOCSP:  true,  // 启用OCSP检查
    Timeout:    10 * time.Second,
})

// 验证证书
if err := validator.ValidateCert(x509Cert); err != nil {
    log.Fatal("Certificate validation failed:", err)
}

// 检查吊销状态（OCSP）
if err := validator.CheckRevocation(x509Cert); err != nil {
    log.Fatal("Certificate revoked:", err)
}

// 验证证书链
certChain := []*x509.Certificate{leafCert, intermediateCert, rootCert}
if err := validator.ValidateCertChain(certChain); err != nil {
    log.Fatal("Certificate chain invalid:", err)
}

// 加载CRL文件
crl, err := cert.LoadCRLFromFile("ca.crl")
if err != nil {
    log.Fatal(err)
}

// 检查证书是否在CRL中
if err := cert.CheckCRL(x509Cert, crl); err != nil {
    log.Fatal("Certificate revoked:", err)
}
```

## 常见问题

### Q1: 证书过期怎么办？

使用 `cert.Manager` 可以自动检测证书过期：

```go
daysLeft := certMgr.DaysUntilExpiry()
if daysLeft < 30 {
    // 发送告警，提醒更新证书
}
```

### Q2: 如何在运行时更新证书？

```go
// 重新加载证书
newCertMgr, err := cert.NewManager(&cert.Config{
    CertFile: "new-cert.pem",
    KeyFile:  "new-key.pem",
})

// 更新服务器TLS配置
server.TLSConfig = newCertMgr.GetTLSConfig()
```

### Q3: 如何实现证书吊销？

使用 `cert.Registry` 吊销证书：

```go
err := registry.Revoke(fingerprint, "key compromised")
if err != nil {
    log.Fatal(err)
}

// 后续验证会失败
if err := registry.Validate(fingerprint); err != nil {
    fmt.Println("Certificate revoked:", err)
}
```

### Q4: 支持哪些传输模式？

- **http-sse-tcp**: 控制平面 HTTP+SSE，数据平面 TCP（推荐）
- **grpc-unified**: 统一使用 gRPC（高性能）

### Q5: 如何启用 OCSP 检查？

```go
validator := cert.NewValidator(&cert.ValidatorConfig{
    CheckOCSP: true,  // 启用OCSP
    Timeout:   10 * time.Second,
})

if err := validator.CheckRevocation(x509Cert); err != nil {
    // 证书已被吊销或OCSP检查失败
}
```

## 相关文档

- [sdp-common 设计文档](../../docs/sdp2.0-common-package-design.md)
- [配置指南](../../docs/configuration-guide.md)
- [证书设置](../../docs/certs-setup.md)
- [架构决策分析](../../docs/architecture-decision-analysis.md)

## 贡献

如果您发现示例有问题或需要改进，请提交 Issue 或 Pull Request。
