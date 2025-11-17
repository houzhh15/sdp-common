# 测试证书目录

此目录包含用于开发和测试的 mTLS 证书。

## ⚠️ 安全警告

**这些证书仅用于开发和测试环境，请勿在生产环境中使用！**

生产环境应该：
- 使用自己的 CA 签发证书
- 定期轮换证书（建议每年）
- 使用强加密算法（RSA 2048+ 或 ECDSA P-256+）
- 妥善保管私钥文件（权限设置为 0600）

## 证书文件说明

### CA 证书（Certificate Authority）
- `ca-cert.pem` - CA 根证书（公钥）
- `ca-key.pem` - CA 私钥 ⚠️ 敏感文件
- `ca-cert.srl` - CA 序列号文件

### Controller 证书
- `controller-cert.pem` - Controller 服务器证书
- `controller-key.pem` - Controller 私钥 ⚠️ 敏感文件

### IH Client 证书
- `ih-client-cert.pem` - IH Client 客户端证书
- `ih-client-key.pem` - IH Client 私钥 ⚠️ 敏感文件

### AH Agent 证书
- `ah-agent-cert.pem` - AH Agent 客户端证书
- `ah-agent-key.pem` - AH Agent 私钥 ⚠️ 敏感文件

## 生成新证书

使用提供的脚本生成新的测试证书：

```bash
cd /path/to/sdp-common
./scripts/generate-certs.sh
```

或者使用 Makefile：

```bash
make cert-gen
```

## 证书配置示例

### Controller 配置

```yaml
tls:
  cert_file: "certs/controller-cert.pem"
  key_file: "certs/controller-key.pem"
  ca_file: "certs/ca-cert.pem"
  min_version: "1.2"
```

### IH Client 配置

```yaml
tls:
  cert_file: "certs/ih-client-cert.pem"
  key_file: "certs/ih-client-key.pem"
  ca_file: "certs/ca-cert.pem"
```

### AH Agent 配置

```yaml
tls:
  cert_file: "certs/ah-agent-cert.pem"
  key_file: "certs/ah-agent-key.pem"
  ca_file: "certs/ca-cert.pem"
```

## 验证证书

### 查看证书信息

```bash
# 查看证书详情
openssl x509 -in certs/controller-cert.pem -text -noout

# 查看证书有效期
openssl x509 -in certs/controller-cert.pem -noout -dates

# 查看证书指纹
openssl x509 -in certs/controller-cert.pem -noout -fingerprint -sha256
```

### 验证证书链

```bash
# 验证服务器证书是否由 CA 签发
openssl verify -CAfile certs/ca-cert.pem certs/controller-cert.pem

# 验证客户端证书
openssl verify -CAfile certs/ca-cert.pem certs/ih-client-cert.pem
```

## 证书有效期

生成的测试证书默认有效期：
- CA 证书：10 年
- 服务器/客户端证书：1 年

可以在 `scripts/generate-certs.sh` 中修改有效期。

## 故障排查

### 证书过期

如果遇到 "certificate has expired" 错误：

```bash
# 1. 删除旧证书
rm certs/*.pem certs/*.srl

# 2. 重新生成证书
./scripts/generate-certs.sh
```

### 证书不匹配

如果遇到 "certificate and private key do not match" 错误：

```bash
# 验证证书和私钥是否匹配
openssl x509 -noout -modulus -in certs/controller-cert.pem | openssl md5
openssl rsa -noout -modulus -in certs/controller-key.pem | openssl md5
# 两个输出应该相同
```

## 注意事项

1. **私钥安全**：
   - 私钥文件应设置为只有所有者可读：`chmod 600 *.key`
   - 不要将私钥提交到版本控制系统
   - `.gitignore` 已配置忽略 `*.key` 和 `*.pem` 文件

2. **证书更新**：
   - 在生产环境中，应该提前 30 天更新证书
   - 使用自动化工具（如 cert-manager）管理证书生命周期

3. **测试环境**：
   - 测试环境可以使用自签名证书
   - 确保 CA 证书在所有组件中一致

## 相关文档

- [TLS 配置指南](../docs/TLS_CONFIGURATION.md)（如果存在）
- [安全最佳实践](../SECURITY.md)
- [证书管理模块文档](../cert/README.md)（如果存在）

---

**最后更新**: 2025-11-16
