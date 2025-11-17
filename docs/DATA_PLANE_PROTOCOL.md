# SDP Data Plane Protocol

> **Version**: 1.0  
> **Last Updated**: 2025-11-17  
> **Status**: Stable

---

## 📚 概述

本文档定义 **sdp-common 数据平面协议**，用于 IH Client、AH Agent 与 Controller 之间的数据传输连接。

### 协议分层

| 层次 | 协议 | 职责 | 文档 |
|-----|------|------|------|
| **控制平面** | HTTP REST API + SSE | 认证、授权、隧道管理、事件推送 | [SDP_COMMON_API_REFERENCE.md](SDP_COMMON_API_REFERENCE.md) |
| **数据平面** | **本协议** | 隧道标识、数据转发 | **本文档** |
| **传输层** | mTLS over TCP | 加密、认证 | TLS 1.2+ |

**关键说明**：
- ✅ SDP 2.0 规范定义控制平面协议（0x00-0x06 消息）
- ⚠️ SDP 2.0 规范**未定义**数据平面握手协议
- ✅ sdp-common 自定义数据平面握手协议（本文档）

---

## 🔌 连接建立流程

### 完整流程

```
1. IH Client 通过控制平面创建隧道
   ↓ HTTP POST /api/v1/tunnels {service_id: "demo-service-001"}
   ← 返回 {tunnel_id: "tunnel-abc123..."}

2. IH Client 建立数据平面连接
   ↓ mTLS Dial tcp://controller:9443
   ↓ 发送 Tunnel ID (36 bytes)
   
3. Controller 接收 IH 连接
   ↓ 读取 Tunnel ID
   ↓ 查询隧道信息
   ↓ 等待 AH 连接 或 直接连接后端

4. AH Agent 建立数据平面连接
   ↓ mTLS Dial tcp://controller:9443
   ↓ 发送 Tunnel ID (36 bytes)

5. Controller 配对连接
   ↓ 匹配 IH 和 AH 连接
   ↓ 开始双向数据转发
```

---

## 📦 协议格式

### 握手阶段

**格式**：固定 36 字节 Tunnel ID（UUID 格式，右侧填充 null 字节）

```
+-----------------------------------+
| Tunnel ID (36 bytes, UTF-8)       |
| 右侧填充 0x00 (null bytes)         |
+-----------------------------------+
| ... 后续数据流（透明转发） ...      |
```

**约束**：
- 长度：固定 36 字节
- 编码：UTF-8 字符串
- 填充：不足 36 字节时，右侧填充 `\x00`
- 示例：`"tunnel-12345678"` → `"tunnel-12345678\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00"` (36 bytes)

### 数据传输阶段

**格式**：透明 TCP 流（无额外协议头）

```
握手完成后，直接传输应用层数据，无需额外封装：

IH → Controller → AH → Backend Service
   ← Controller ← AH ← Backend Service
```

---

## 💻 客户端实现

### 使用 SDK（推荐）

```go
package main

import (
    "github.com/houzhh15/sdp-common/tunnel"
    "crypto/tls"
)

func main() {
    // 1. 创建数据平面客户端
    client := tunnel.NewDataPlaneClient(
        "localhost:9443",  // Controller TCP Proxy 地址
        tlsConfig,         // mTLS 配置
    )

    // 2. 建立连接（SDK 自动处理 Tunnel ID 发送）
    conn, err := client.Connect("tunnel-abc123...")
    if err != nil {
        log.Fatal(err)
    }
    defer conn.Close()

    // 3. 开始数据传输
    io.Copy(conn, localConn)  // 上行
    io.Copy(localConn, conn)  // 下行
}
```

### 手动实现（不推荐）

如果不使用 SDK，需要手动发送 Tunnel ID：

```go
// 1. 建立 TLS 连接
conn, err := tls.Dial("tcp", "localhost:9443", tlsConfig)
if err != nil {
    return err
}

// 2. 发送 Tunnel ID（固定 36 字节）
tunnelID := "tunnel-abc123..."
tunnelIDBytes := make([]byte, 36)
copy(tunnelIDBytes, []byte(tunnelID))
if _, err := conn.Write(tunnelIDBytes); err != nil {
    return err
}

// 3. 开始数据传输
// ... io.Copy ...
```

**⚠️ 警告**：手动实现容易出错，强烈建议使用 SDK！

---

## 🖥️ 服务端实现

### Controller 端接收

```go
package transport

import (
    "io"
    "net"
    "strings"
    "github.com/houzhh15/sdp-common/tunnel"
)

func (s *tcpProxyServer) HandleConnection(clientConn net.Conn) error {
    defer clientConn.Close()

    // 1. 读取 Tunnel ID（固定 36 字节）
    buf := make([]byte, tunnel.TunnelIDLength)  // 36 bytes
    if _, err := io.ReadFull(clientConn, buf); err != nil {
        return fmt.Errorf("failed to read tunnel ID: %w", err)
    }

    // 2. 去除填充的 null 字节
    tunnelID := strings.TrimRight(string(buf), "\x00")

    // 3. 查询隧道信息
    tunnelInfo, err := s.tunnelStore.Get(tunnelID)
    if err != nil {
        return fmt.Errorf("tunnel not found: %s", tunnelID)
    }

    // 4. 连接到目标服务
    targetConn, err := net.Dial("tcp", 
        fmt.Sprintf("%s:%d", tunnelInfo.TargetHost, tunnelInfo.TargetPort))
    if err != nil {
        return err
    }
    defer targetConn.Close()

    // 5. 双向数据转发
    go io.Copy(targetConn, clientConn)  // IH → Target
    io.Copy(clientConn, targetConn)     // Target → IH

    return nil
}
```

---

## 🔍 协议示例

### 示例 1: Tunnel ID 编码

```go
// Tunnel ID: "tunnel-12345678" (16 字符)
tunnelID := "tunnel-12345678"

// 编码为 36 字节
buf := make([]byte, 36)
copy(buf, []byte(tunnelID))
// buf = "tunnel-12345678\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00"

// 十六进制表示
// 74 75 6e 6e 65 6c 2d 31 32 33 34 35 36 37 38  (tunnel-12345678)
// 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00  (null padding)
// 00 00 00 00 00                                 (null padding)
```

### 示例 2: 完整握手

**客户端发送**：
```
hex: 74 75 6e 6e 65 6c 2d 61 62 63 31 32 33 34 35 36
     37 38 39 30 00 00 00 00 00 00 00 00 00 00 00 00
     00 00 00 00

ascii: "tunnel-abc1234567890\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00"
```

**服务端接收**：
```go
buf := make([]byte, 36)
io.ReadFull(conn, buf)
// buf = [116 117 110 110 101 108 45 97 98 99 49 50 51 52 53 54 55 56 57 48 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0]

tunnelID := strings.TrimRight(string(buf), "\x00")
// tunnelID = "tunnel-abc1234567890"
```

---

## ⚠️ 错误处理

### 常见错误

| 错误 | 原因 | 解决方案 |
|-----|------|---------|
| `failed to read tunnel ID: EOF` | 客户端未发送数据或连接断开 | 检查客户端实现，确保发送 36 字节 |
| `tunnel not found: xxx` | Tunnel ID 不存在或已过期 | 通过控制平面重新创建隧道 |
| `empty tunnel ID` | 发送了 36 个 null 字节 | 检查客户端编码逻辑 |
| `invalid tunnel ID length` | （旧协议）长度前缀错误 | 确保使用固定 36 字节格式 |

### 超时设置

```go
// 握手阶段超时（5 秒）
conn.SetReadDeadline(time.Now().Add(5 * time.Second))
io.ReadFull(conn, tunnelIDBytes)
conn.SetReadDeadline(time.Time{})  // 清除超时

// 数据传输阶段超时（可选，根据业务需求）
conn.SetReadDeadline(time.Now().Add(30 * time.Second))
```

---

## 🔒 安全考虑

### TLS 要求

- ✅ **必须**使用 mTLS（双向认证）
- ✅ **必须**验证证书链
- ✅ **推荐**使用 TLS 1.2 或更高版本
- ✅ **推荐**使用强加密套件（ECDHE-RSA-AES256-GCM-SHA384 等）

### Tunnel ID 安全

- ✅ Tunnel ID 应使用随机 UUID（不可预测）
- ✅ Tunnel ID 应在控制平面创建时生成
- ⚠️ Tunnel ID 不应包含敏感信息
- ⚠️ Tunnel ID 应设置过期时间（通过控制平面管理）

---

## 📊 性能优化

### 零拷贝转发

```go
// 使用 io.Copy 实现零拷贝
io.Copy(dst, src)  // 内部使用 splice/sendfile 系统调用
```

### 缓冲区优化

```go
// 自定义缓冲区大小（默认 32KB）
buf := make([]byte, 64*1024)  // 64KB buffer
io.CopyBuffer(dst, src, buf)
```

### 连接池复用

```go
// 复用 TLS 连接（避免频繁握手）
// 注意：每个连接只能用于一个隧道
```

---

## 🔄 版本兼容性

### v1.0（当前版本）

- 协议格式：固定 36 字节 Tunnel ID
- 发布日期：2025-11-17
- 状态：✅ Stable

### 未来版本考虑

- v1.1: 支持协议协商（版本号）
- v1.2: 支持多路复用（单连接多隧道）
- v2.0: 支持 QUIC 传输层

**兼容性承诺**：
- v1.x 版本保持向后兼容
- 新版本通过协议版本号协商
- 旧客户端继续使用 v1.0

---

## 📚 相关文档

- [SDP Common API Reference](SDP_COMMON_API_REFERENCE.md) - 控制平面 API 文档
- [SDP 2.0 Protocol Mapping](service/SDP2.0_PROTOCOL_MAPPING.md) - 协议映射说明
- [Service Discovery Quick Ref](service/SERVICE_DISCOVERY_QUICK_REF.md) - 服务配置管理

---

## 🤝 贡献指南

如需修改数据平面协议：

1. 提交 RFC（Request for Comments）
2. 讨论兼容性影响
3. 更新本文档
4. 更新 SDK 实现
5. 更新示例代码
6. 发布新版本

---

## 📝 更新日志

### 2025-11-17 - v1.0
- ✅ 统一协议格式（固定 36 字节）
- ✅ 添加协议文档
- ✅ 提供 DataPlaneClient SDK
- ✅ 重构 IH Client 示例

---

**文档版本**: 1.0  
**维护者**: SDP Common Team  
**更新频率**: 随协议变更更新
