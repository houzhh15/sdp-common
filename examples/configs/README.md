# SDP 配置示例

本目录包含不同场景的 SDP 配置模板，演示混合架构的灵活性。

## 配置文件说明

### 1. default.yaml - 默认配置 (推荐)

**适用场景**: 所有通用场景，平衡性能与兼容性

**架构特点**:
- **Control Plane**: HTTP REST API (端口 8080)
- **Real-time Notification**: SSE (Server-Sent Events, 30s心跳)
- **Data Plane**: TCP Proxy (端口 9443)

**性能指标**:
- 握手延迟: <100ms
- 并发连接: 1000+
- TCP吞吐: ~950 Mbps

**推荐理由**:
- ✅ 最佳兼容性 (所有防火墙/代理支持)
- ✅ 运维友好 (标准HTTP调试工具)
- ✅ 可靠性高 (HTTP重试机制成熟)

---

### 2. high-performance.yaml - 高性能配置

**适用场景**: 内网高吞吐场景，追求极致性能

**架构特点**:
- **Control Plane**: gRPC (端口 50051)
- **Real-time Notification**: gRPC Streams (双向流)
- **Data Plane**: TCP Proxy (端口 9443)

**性能指标**:
- 握手延迟: <50ms (性能提升 50%)
- 并发连接: 2000+
- TCP吞吐: ~950 Mbps (数据平面不变)

**使用注意**:
- ⚠️ 需要客户端支持 gRPC
- ⚠️ 部分防火墙可能阻止 gRPC
- ⚠️ 调试工具较少

---

### 3. development.yaml - 开发环境配置

**适用场景**: 本地开发调试

**特殊配置**:
- 使用非特权端口 (18080, 19443)
- 日志级别 debug
- 日志格式 text (可读性好)
- Token有效期 24小时
- 关闭设备验证
- 更短的超时时间 (快速失败)

**使用方式**:
```bash
# 启动开发环境
cd sdp-common/examples/controller
go run main.go -config ../configs/development.yaml
```

---

### 4. production.yaml - 生产环境配置

**适用场景**: 生产部署

**安全加固**:
- 强制 TLS 1.3
- Token有效期 30分钟
- 启用设备验证
- 启用 MFA (多因素认证)
- 日志输出到文件
- 较长的超时时间 (稳定性优先)

**部署要求**:
- 必须使用有效的生产证书
- 配置监控和告警
- 定期轮换密钥

---

## 配置切换方式

### 方法1: 命令行指定

```bash
# 使用默认配置启动
./controller -config examples/configs/default.yaml

# 使用高性能配置
./controller -config examples/configs/high-performance.yaml
```

### 方法2: 环境变量覆盖

```bash
export SDP_CONFIG_PATH=examples/configs/production.yaml
./controller
```

### 方法3: 运行时切换 (需支持热重载)

```bash
# 发送 SIGHUP 信号重新加载配置
kill -HUP $(pidof controller)
```

---

## 性能对比

| 配置类型 | Control Plane | 握手延迟 | 并发连接 | 兼容性 | 推荐场景 |
|---------|--------------|---------|---------|-------|---------|
| default | HTTP+SSE | <100ms | 1000+ | ⭐⭐⭐⭐⭐ | 通用 |
| high-perf | gRPC | <50ms | 2000+ | ⭐⭐⭐ | 内网高吞吐 |
| dev | HTTP+SSE | <100ms | 100+ | ⭐⭐⭐⭐⭐ | 本地开发 |
| prod | HTTP+SSE | <100ms | 1000+ | ⭐⭐⭐⭐⭐ | 生产部署 |

---

## 常见问题

### Q1: 如何选择 HTTP 还是 gRPC?

**选择 HTTP+SSE (default.yaml)**:
- ✅ 需要穿透防火墙/代理
- ✅ 团队熟悉 REST API
- ✅ 需要使用标准调试工具 (curl, Postman)
- ✅ 兼容性是首要考虑

**选择 gRPC (high-performance.yaml)**:
- ✅ 内网环境，无防火墙限制
- ✅ 追求极致性能
- ✅ 团队熟悉 gRPC
- ✅ 客户端支持 gRPC

### Q2: 数据平面为什么固定使用 TCP Proxy?

**理由**:
- TCP是协议无关的 (支持所有应用层协议)
- 性能最优 (零开销转发)
- 架构简单 (无需协议转换)
- 已满足性能要求 (950 Mbps)

### Q3: 可以混合使用吗?

**可以!** SDP 2.0 支持渐进式迁移:

```yaml
# Controller 使用 HTTP
component:
  type: controller

transport:
  enable_grpc: false
  http_addr: :8080
```

```yaml
# IH Client 使用 gRPC (连接同一个 Controller)
component:
  type: ih

transport:
  enable_grpc: true
  grpc_addr: controller:50051
```

Controller 会根据客户端请求协议自动切换。

---

## SDP 2.0 规范 0x04 服务配置管理

> **✨ 重要更新** (2025-11-17): AH Agent 服务配置采用混合方案

### 变更说明

**旧方式（已废弃）**：在 `ah-agent.yaml` 中静态定义服务列表
```yaml
# ❌ 不再支持
services:
  - service_id: web-service
    target_host: localhost
    target_port: 8080
```

**新方式（推荐）**：Controller 集中管理 + HTTP GET + SSE Push

1. **Controller 端配置**（`examples/controller/main.go`）：
```go
// 预置服务配置
manager.CreateServiceConfig(ctx, &tunnel.ServiceConfig{
    ServiceID:   "demo-service-001",
    ServiceName: "Demo Web Service",
    TargetHost:  "localhost",
    TargetPort:  9999,
    Protocol:    "tcp",
    Status:      tunnel.ServiceStatusActive,
})
```

2. **AH Agent 启动**：
```bash
# 无需 -services 参数，自动从 Controller 获取
./ah-agent-example
```

3. **运行时流程**：
   - **启动时**：HTTP GET `/api/v1/services` 获取初始配置
   - **运行时**：SSE 订阅 `/api/v1/tunnels/stream` 接收实时更新

### 架构优势

| 特性 | 旧方式（静态配置） | 新方式（混合方案） |
|-----|------------------|-------------------|
| 配置管理 | 分散在每个 AH Agent | Controller 集中管理 |
| 配置更新 | 需重启 AH Agent | 无需重启，SSE 实时推送 |
| 一致性保证 | 手动同步 | 自动同步 |
| 符合规范 | ❌ 违反控制/数据平面分离 | ✅ 符合 SDP 2.0 规范 0x04 |
| 场景覆盖 | 仅静态场景 | 100%（启动 + 运行时） |

### 详细文档

- [0x04 实施总结](../../docs/0x04_HYBRID_APPROACH_SUMMARY.md)
- [API 参考手册](../../docs/SDP_COMMON_API_REFERENCE.md#52-serviceconfig---服务配置管理)

---

## 验证配置

使用 config 包的 Loader 验证配置文件:

```go
import "github.com/houzhh15/sdp-common/config"

loader := config.NewLoader()
cfg, err := loader.Load("examples/configs/default.yaml")
if err != nil {
    log.Fatalf("Invalid config: %v", err)
}

// 配置已通过验证
fmt.Printf("Component: %s\n", cfg.Component.Type)
```

---

## 相关文档

- [架构决策分析](../../ARCHITECTURE.md)
- [0x04 混合方案实施报告](../../docs/0x04_IMPLEMENTATION_REPORT.md)
- [性能测试报告](../../test/benchmark_test.go)
