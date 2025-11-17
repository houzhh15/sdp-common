# Security Policy

## Supported Versions

以下版本当前正在接收安全更新：

| Version | Supported          |
| ------- | ------------------ |
| 1.0.x   | :white_check_mark: |
| < 1.0   | :x:                |

## Reporting a Vulnerability

我们非常重视安全问题。如果你发现了安全漏洞，请**不要**在公开的 GitHub Issues 中报告。

### 报告流程

1. **发送邮件**到 [houzhh15@example.com]，主题为 "Security Vulnerability Report"
   
2. **包含以下信息**：
   - 漏洞描述
   - 受影响的版本
   - 复现步骤
   - 潜在影响
   - 可能的修复方案（如果有）
   - 你的联系方式

3. **等待确认**：我们会在 **48 小时内**确认收到你的报告

4. **协作修复**：我们会与你保持沟通，共同确定修复方案

5. **漏洞披露**：在修复发布后，我们会在 CHANGELOG 中公开致谢

### 时间线

- **48 小时**：确认收到报告
- **7 天**：提供初步评估和修复计划
- **30 天**：发布修复补丁（根据严重程度可能加快）

### 安全更新通知

- 通过 GitHub Releases 发布
- 在 CHANGELOG.md 中标注 `[SECURITY]`
- 通过 GitHub Security Advisory 通知

## Security Best Practices

使用 sdp-common 时，请遵循以下安全最佳实践：

### 1. 证书管理

```yaml
# 推荐配置
tls:
  cert_file: "/secure/path/cert.pem"
  key_file: "/secure/path/key.pem"  # 权限: 0600
  ca_file: "/secure/path/ca.pem"
  min_version: "1.2"  # TLS 1.2+
```

- 使用强加密的私钥（RSA 2048+ 或 ECDSA P-256+）
- 定期轮换证书（建议每年）
- 限制私钥文件权限（chmod 600）
- 使用专用 CA 签发证书

### 2. 会话管理

```yaml
# 推荐配置
session:
  token_ttl: 3600s  # 1小时
  cleanup_interval: 300s  # 5分钟
```

- 设置合理的 Token 过期时间
- 启用会话自动清理
- 实现会话撤销机制
- 记录会话审计日志

### 3. 网络安全

```yaml
# 推荐配置
transport:
  http_addr: "127.0.0.1:8080"  # 仅监听本地
  tcp_proxy_addr: "0.0.0.0:9443"  # 启用 mTLS
  enable_mtls: true
```

- 控制平面仅暴露给可信网络
- 数据平面启用 mTLS
- 使用防火墙限制访问
- 启用速率限制

### 4. 日志审计

```yaml
# 推荐配置
logging:
  level: "info"  # 生产环境不要用 debug
  audit_file: "/secure/logs/audit.log"
  sensitive_fields:  # 脱敏敏感字段
    - "password"
    - "token"
    - "private_key"
```

- 记录所有安全事件
- 脱敏敏感信息
- 定期审查审计日志
- 设置日志告警

### 5. 依赖管理

```bash
# 定期更新依赖
go get -u ./...
go mod tidy

# 检查漏洞
go list -json -m all | nancy sleuth
```

- 定期更新依赖包
- 使用 `go mod verify` 验证依赖完整性
- 监控安全公告

## Known Security Considerations

### 1. mTLS 证书验证

sdp-common 默认启用严格的 mTLS 证书验证。如果需要在测试环境中禁用（**不推荐生产环境**）：

```go
// ⚠️ 仅用于测试！
tlsConfig := &tls.Config{
    InsecureSkipVerify: true,  // 危险！
}
```

### 2. TCP Proxy 缓冲区大小

默认缓冲区大小为 32KB。过大的缓冲区可能导致内存耗尽：

```yaml
transport:
  tcp_proxy:
    buffer_size: 32768  # 根据实际调整
    max_connections: 1000  # 限制并发连接
```

### 3. SSE 连接管理

长连接可能被恶意客户端滥用。建议：

```go
notifier := tunnel.NewNotifier(logger, 30*time.Second)  // 心跳检测
// 实现连接数限制和超时机制
```

## Security Audit

本项目遵循以下安全实践：

- ✅ 静态代码分析（golangci-lint）
- ✅ 依赖漏洞扫描（go mod 和第三方工具）
- ✅ 单元测试覆盖率 ≥ 80%
- ✅ 集成测试（包括安全场景）
- ✅ 代码审查（所有 PR 需要审查）

## Acknowledgments

我们感谢以下研究人员对安全性的贡献：

- （待添加）

如果你报告了安全漏洞并希望被公开致谢，请在报告中说明。

---

**最后更新**: 2025-11-16  
**联系方式**: houzhh15@example.com
