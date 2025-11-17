# Controller 示例说明

## ⚠️ 架构说明：SDK vs 示例代码

### 当前状态

本示例包含 **887 行代码**，其中包括：

1. **SDK 已提供的能力**（约 200 行）:
   - `cert.Manager` - 证书管理
   - `session.Manager` - 会话管理  
   - `policy.Engine` - 策略评估
   - `tunnel.Manager` 接口定义
   - `transport.HTTPServer` - HTTP 服务器
   - `transport.TCPProxyServer` - TCP 代理服务器
   - `tunnel.Notifier` - SSE 实时推送

2. **示例代码实现的逻辑**（约 687 行）:
   - HTTP REST API 处理器（400+ 行）
     - `/api/v1/handshake` - 握手端点
     - `/api/v1/sessions/*` - 会话管理
     - `/api/v1/policies` - 策略查询
     - `/api/v1/services` - 服务配置
     - `/api/v1/tunnels` - 隧道管理
     - `/v1/agent/tunnels/stream` - SSE 推送
   - `InMemoryTunnelManager` 实现（247 行，在 `tunnel_manager.go`）
   - 证书注册逻辑（50 行）
   - 策略初始化（80 行）
   - 服务配置预置（30 行）

### 设计哲学

**为什么不把 HTTP 处理器放到 SDK？**

1. **灵活性**: 不同的 Controller 实现可能需要：
   - 不同的认证机制（OAuth, SAML, mTLS等）
   - 不同的存储后端（内存、Redis、PostgreSQL）
   - 不同的 API 响应格式
   - 自定义的业务逻辑

2. **SDP 规范实现**: 示例代码展示了 **完整的 SDP 2.0 规范实现**，包括：
   - 0x01 握手协议
   - 0x02 策略查询
   - 0x03 隧道管理
   - 0x04 服务配置（混合方案）
   - 0x05 隧道事件

3. **教育目的**: 开发者可以通过示例代码：
   - 理解如何组合使用 sdp-common 的各个包
   - 学习 HTTP REST API 的最佳实践
   - 根据需求修改和扩展

### SDK 边界清晰化建议

如果您认为边界不够清晰，可以考虑：

#### 方案 A: 创建高层 `controller` 包（推荐，但工作量大）

```go
// 理想状态：开发者只需 20 行代码
package main

import "github.com/houzhh15/sdp-common/controller"

func main() {
    cfg := controller.DefaultConfig()
    cfg.TLS.CertFile = "certs/controller-cert.pem"
    cfg.TLS.KeyFile = "certs/controller-key.pem"
    cfg.TLS.CAFile = "certs/ca-cert.pem"
    
    ctrl, _ := controller.New(cfg)
    ctrl.SeedExampleData() // 添加示例策略和服务
    ctrl.Start() // 阻塞式启动，自动注册所有标准 API
}
```

**优点**:
- 极简的开发者体验
- 统一的标准实现
- 减少重复代码

**缺点**:
- 需要创建新的 `controller/` 包（约 1000 行代码）
- 灵活性降低（难以定制）
- 增加维护成本

#### 方案 B: 提供 Handler 辅助函数（折中方案）

在 `protocol/` 或新建 `handlers/` 包中提供标准处理器：

```go
// handlers/handshake.go
func HandleHandshake(certRegistry cert.Registry, sessionMgr session.Manager, logger logging.Logger) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        // 标准握手逻辑（100 行）
    }
}

// 使用方式
mux.HandleFunc("/api/v1/handshake", handlers.HandleHandshake(certRegistry, sessionMgr, logger))
```

**优点**:
- 提供可复用的处理器
- 保持灵活性（可以不用）
- 减少示例代码行数

**缺点**:
- 仍需要开发者组装
- 增加包复杂度

#### 方案 C: 当前方案（最灵活）

**优点**:
- 最大灵活性
- 清晰的示例代码
- 易于理解和修改

**缺点**:
- 示例代码较长（887 行）
- 需要开发者理解全部逻辑
- 容易产生重复代码

### 当前采用方案

考虑到 sdp-common 的定位是 **公共库而非框架**，当前采用 **方案 C**，通过以下方式改进：

1. ✅ **清晰的注释**: 标注哪些是 SDK 能力，哪些是示例实现
2. ✅ **模块化代码**: 将 `tunnel_manager.go` 提取为独立文件
3. ✅ **文档说明**: 本 README 解释设计决策
4. 🔄 **提取辅助函数**: 将重复的响应处理提取为函数（如 `respondError`, `respondSuccess`）

### 如何使用本示例

1. **快速上手**: 直接运行示例，理解完整流程
2. **定制开发**: 复制示例代码，根据需求修改
3. **生产环境**: 
   - 替换 `InMemoryTunnelManager` 为数据库实现
   - 添加认证中间件
   - 集成监控和日志

### 相关文件

- `main.go` (887 行) - 主程序和 HTTP 处理器
- `tunnel_manager.go` (247 行) - 内存隧道管理器实现
- `../../docs/SDP_COMMON_API_REFERENCE.md` - 完整 API 文档

### 未来改进

如果社区反馈强烈，可以考虑：

1. 创建 `controller` 高层包（方案 A）
2. 提供 `handlers` 辅助包（方案 B）
3. 提供更多存储后端实现（Redis, PostgreSQL）

---

**反馈建议**: 如果您认为当前边界不够清晰，请提出 Issue 或 PR！
