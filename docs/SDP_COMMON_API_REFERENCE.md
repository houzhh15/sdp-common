# SDP-Common Package 接口参考文档

> **版本**: v1.0  
> **任务**: task_1763209090 - 从原型中提取SDP公共包  
> **项目**: SASE-POC  
> **生成日期**: 2025-11-16

---

## 📚 目录

- [1. 概述](#1-概述)
- [2. cert - 证书管理包](#2-cert---证书管理包)
- [3. session - 会话管理包](#3-session---会话管理包)
- [4. policy - 策略引擎包](#4-policy---策略引擎包)
- [5. tunnel - 隧道管理包](#5-tunnel---隧道管理包)
  - [5.1 Manager - 隧道生命周期管理](#51-manager---隧道生命周期管理)
  - [5.2 ServiceConfig - 服务配置管理](#52-serviceconfig---服务配置管理)
  - [5.3 Notifier - 隧道事件通知](#53-notifier---隧道事件通知)
  - [5.4 Subscriber - SSE 客户端订阅](#54-subscriber---sse-客户端订阅)
  - [5.5 DataPlaneClient - 数据平面客户端](#55-dataplaneclient---数据平面客户端)
  - [5.6 TCPProxy - 数据平面透明代理](#56-tcpproxy---数据平面透明代理)
  - [5.7 Broker - gRPC 流转发](#57-broker---grpc-流转发)
  - [5.8 EventStore - 事件持久化存储接口](#58-eventstore---事件持久化存储接口)
- [6. logging - 日志审计包](#6-logging---日志审计包)
- [7. transport - 传输层包](#7-transport---传输层包)
- [8. protocol - 协议定义包](#8-protocol---协议定义包)
- [9. config - 配置管理包](#9-config---配置管理包)
- [10. 身份验证与存储机制](#10-身份验证与存储机制)
- [11. 快速参考表](#11-快速参考表)

---

## 1. 概述

`sdp-common` 是一个符合 SDP 2.0 规范的 Golang 公共库，提供证书管理、会话管理、策略评估、隧道管理、日志审计等核心功能。

### 1.1 设计原则

- **混合架构**: 控制平面多协议支持（默认 HTTP+SSE），数据平面固定 TCP Proxy
- **模块化**: 高内聚低耦合，各包可独立使用
- **性能优先**: 数据平面零拷贝，实时通知 < 100ms 延迟
- **易于集成**: 统一接口，丰富的使用示例

### 1.2 架构图

```
┌─────────────────────────────────────────────────────────┐
│                   上层组件                               │
│  Controller / IH Client / AH Agent                       │
└────────────────┬────────────────────────────────────────┘
                 │
┌────────────────▼────────────────────────────────────────┐
│              sdp-common 公共库                           │
│  ┌──────────────────────────────────────────────────┐  │
│  │  核心功能包                                        │  │
│  │  • cert (证书管理)                                 │  │
│  │  • session (会话管理)                              │  │
│  │  • policy (策略引擎)                               │  │
│  │  • tunnel (隧道管理)                               │  │
│  │  • logging (日志审计)                              │  │
│  └──────────────────────────────────────────────────┘  │
│  ┌──────────────────────────────────────────────────┐  │
│  │  传输层 (混合架构)                                  │  │
│  │  • HTTP REST + SSE (默认)                         │  │
│  │  • TCP Proxy (数据平面，固定)                      │  │
│  │  • gRPC (可选)                                     │  │
│  └──────────────────────────────────────────────────┘  │
│  ┌──────────────────────────────────────────────────┐  │
│  │  基础设施                                          │  │
│  │  • protocol (协议定义)                             │  │
│  │  • config (配置管理)                               │  │
│  └──────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────┘
```

---

## 2. cert - 证书管理包

### 2.1 Manager - 证书管理器

**功能**: 加载和管理 TLS 证书，计算证书指纹，验证有效期

**接口定义**:

```go
type Manager struct {
    certFile string
    keyFile  string
    caFile   string
    cert     *tls.Certificate
    x509Cert *x509.Certificate
    caCertPool *x509.CertPool
}

// 配置结构
type Config struct {
    CertFile string
    KeyFile  string
    CAFile   string
}
```

**核心方法**:

| 方法 | 签名 | 功能描述 |
|------|------|----------|
| `NewManager` | `NewManager(config *Config) (*Manager, error)` | 创建证书管理器，加载证书文件 |
| `GetFingerprint` | `GetFingerprint() string` | 计算证书 SHA256 指纹 |
| `ValidateExpiry` | `ValidateExpiry() error` | 验证证书是否过期 |
| `DaysUntilExpiry` | `DaysUntilExpiry() int` | 获取证书剩余有效天数 |
| `GetX509Certificate` | `GetX509Certificate() *x509.Certificate` | 获取 X.509 证书对象 |
| `GetCertInfo` | `GetCertInfo() *CertInfo` | 获取证书完整信息（主题、颁发者、有效期等） |
| `GetTLSConfig` | `GetTLSConfig() *tls.Config` | 生成 TLS 配置（用于服务器/客户端） |
| `GetCertificate` | `GetCertificate() *tls.Certificate` | 获取 TLS 证书对象 |

**数据结构**:

```go
// CertInfo 证书信息
type CertInfo struct {
    Subject      string    // 主题
    Issuer       string    // 颁发者
    NotBefore    time.Time // 生效时间
    NotAfter     time.Time // 过期时间
    Fingerprint  string    // SHA256 指纹
    Status       CertStatus // 证书状态
    SerialNumber string    // 序列号
}

type CertStatus string
const (
    StatusActive  CertStatus = "active"
    StatusExpired CertStatus = "expired"
    StatusRevoked CertStatus = "revoked"
)
```

**使用示例**:

```go
// 加载证书
manager, err := cert.NewManager(&cert.Config{
    CertFile: "client-cert.pem",
    KeyFile:  "client-key.pem",
    CAFile:   "ca-cert.pem",
})
if err != nil {
    log.Fatal(err)
}

// 获取指纹
fingerprint := manager.GetFingerprint()
log.Printf("证书指纹: %s", fingerprint)

// 验证有效期
if err := manager.ValidateExpiry(); err != nil {
    log.Fatal("证书已过期:", err)
}

// 获取剩余天数
days := manager.DaysUntilExpiry()
if days < 30 {
    log.Printf("警告: 证书将在 %d 天后过期", days)
}

// 获取 X.509 证书对象
x509Cert := manager.GetX509Certificate()
fmt.Printf("证书主题: %s\n", x509Cert.Subject.String())

// 获取证书完整信息
certInfo := manager.GetCertInfo()
fmt.Printf("证书信息: %+v\n", certInfo)
fmt.Printf("状态: %s, 序列号: %s\n", certInfo.Status, certInfo.SerialNumber)

// 获取 TLS 配置
tlsConfig := manager.GetTLSConfig()
```

---

### 2.2 Registry - 证书注册表

**功能**: 证书注册、查询、吊销管理（需要数据库支持）

**接口定义**:

```go
type Registry struct {
    db       *gorm.DB
    logger   Logger
    mu       sync.RWMutex
    crlPath  string
}

// 证书信息
type CertInfo struct {
    Fingerprint string
    ClientID    string
    NotBefore   time.Time
    NotAfter    time.Time
    Subject     string
    Issuer      string
    Status      CertStatus  // active, revoked, expired
}

type CertStatus string
const (
    StatusActive  CertStatus = "active"
    StatusRevoked CertStatus = "revoked"
    StatusExpired CertStatus = "expired"
)
```

**核心方法**:

| 方法 | 签名 | 功能描述 |
|------|------|----------|
| `NewRegistry` | `NewRegistry(db *gorm.DB, logger Logger) (*Registry, error)` | 创建证书注册表 |
| `Register` | `Register(clientID, fingerprint string, cert *x509.Certificate) error` | 注册新证书 |
| `GetCertInfo` | `GetCertInfo(fingerprint string) (*CertInfo, error)` | 查询证书信息 |
| `Revoke` | `Revoke(fingerprint, reason string) error` | 吊销证书 |
| `Validate` | `Validate(fingerprint string) error` | 验证证书状态（是否吊销/过期） |
| `List` | `List(page, pageSize int, status CertStatus) ([]*CertInfo, int64, error)` | 分页查询证书列表 |
| `CleanExpired` | `CleanExpired() (int64, error)` | 清理过期证书，返回清理数量 |

**使用示例**:

```go
// 创建注册表
registry, err := cert.NewRegistry(db, logger)

// 注册证书
err = registry.Register("client-001", fingerprint, x509Cert)

// 验证证书
if err := registry.Validate(fingerprint); err != nil {
    log.Printf("证书验证失败: %v", err)
}

// 查询证书信息
info, err := registry.GetCertInfo(fingerprint)
fmt.Printf("证书状态: %s, 有效期至: %s\n", info.Status, info.NotAfter)

// 分页查询证书列表
certs, total, err := registry.List(1, 20, cert.StatusActive)
fmt.Printf("找到 %d 个活跃证书，当前页显示 %d 个\n", total, len(certs))

// 清理过期证书
count, err := registry.CleanExpired()
fmt.Printf("清理了 %d 个过期证书\n", count)

// 吊销证书
err = registry.Revoke(fingerprint, "密钥泄露")
```

---

### 2.3 Validator - 证书验证器

**功能**: 证书链验证、OCSP 吊销检查（可选）

**接口定义**:

```go
type Validator struct {
    caCertPool *x509.CertPool
    checkOCSP  bool
}
```

**核心方法**:

| 方法 | 签名 | 功能描述 |
|------|------|----------|
| `NewValidator` | `NewValidator(caCertPool *x509.CertPool, enableOCSP bool) *Validator` | 创建验证器 |
| `ValidateCert` | `ValidateCert(cert *x509.Certificate) error` | 验证证书链 |
**核心方法**:

| 方法 | 签名 | 功能描述 |
|------|------|----------|
| `NewValidator` | `NewValidator(config *ValidatorConfig) *Validator` | 创建证书验证器 |
| `ValidateCert` | `ValidateCert(cert *x509.Certificate) error` | 验证证书链 |
| `ValidateCertChain` | `ValidateCertChain(certChain []*x509.Certificate) error` | 验证完整证书链 |
| `CheckRevocation` | `CheckRevocation(cert *x509.Certificate) error` | OCSP 吊销检查 |

**配置结构**:

```go
type ValidatorConfig struct {
    CACertPool *x509.CertPool
    EnableOCSP bool
}
```

**使用示例**:

```go
// 创建验证器
validator := cert.NewValidator(&cert.ValidatorConfig{
    CACertPool: caCertPool,
    EnableOCSP: true,
})

// 验证单个证书
if err := validator.ValidateCert(clientCert); err != nil {
    log.Printf("证书链验证失败: %v", err)
}

// 验证完整证书链
certChain := []*x509.Certificate{leafCert, intermediateCert}
if err := validator.ValidateCertChain(certChain); err != nil {
    log.Printf("证书链验证失败: %v", err)
}

// OCSP 吊销检查
if err := validator.CheckRevocation(clientCert); err != nil {
    log.Printf("证书已被吊销: %v", err)
}
```

---

## 3. session - 会话管理包

### 3.1 Manager - 会话管理器

**功能**: Token 生成、会话创建、验证、刷新、撤销，自动过期清理

**接口定义**:

```go
type Manager struct {
    sessions        map[string]*Session  // token -> session
    clientSessions  map[string][]string  // clientID -> tokens
    mu              sync.RWMutex
    tokenTTL        time.Duration
    cleanupInterval time.Duration
    logger          Logger
    stopChan        chan struct{}
}

// 配置
type Config struct {
    TokenTTL        time.Duration  // Token 有效期，默认 3600s
    CleanupInterval time.Duration  // 清理间隔，默认 300s
}
```

**核心方法**:

| 方法 | 签名 | 功能描述 |
|------|------|----------|
| `NewManager` | `NewManager(config *Config, logger Logger) *Manager` | 创建会话管理器 |
| `CreateSession` | `CreateSession(ctx context.Context, req *CreateSessionRequest) (*Session, error)` | 创建新会话 |
| `ValidateSession` | `ValidateSession(ctx context.Context, token string) (*Session, error)` | 验证 Token 有效性 |
| `RefreshSession` | `RefreshSession(ctx context.Context, token string) (*Session, error)` | 刷新会话（延长过期时间） |
| `RevokeSession` | `RevokeSession(ctx context.Context, token string) error` | 撤销会话 |
| `GetActiveSessions` | `GetActiveSessions(ctx context.Context) ([]*Session, error)` | 获取所有活跃会话 |

**数据结构**:

```go
// Session - 会话对象
type Session struct {
    Token           string
    ClientID        string
    CertFingerprint string
    DeviceInfo      *DeviceInfo           // 设备信息（可选）
    CreatedAt       time.Time
    ExpiresAt       time.Time
    LastAccessAt    time.Time
    Metadata        map[string]interface{}
}

// CreateSessionRequest - 创建会话请求
type CreateSessionRequest struct {
    ClientID        string
    CertFingerprint string
    DeviceInfo      *DeviceInfo
    Metadata        map[string]interface{}
}

// DeviceInfo - 设备信息
type DeviceInfo struct {
    DeviceID    string
    OS          string  // linux, windows, darwin
    OSVersion   string
    Compliance  bool    // 合规状态
}
```

**使用示例**:

```go
// 创建会话管理器
manager := session.NewManager(&session.Config{
    TokenTTL:        3600 * time.Second,  // 1小时
    CleanupInterval: 300 * time.Second,   // 5分钟清理一次
}, logger)

// 创建会话
session, err := manager.CreateSession(ctx, &session.CreateSessionRequest{
    ClientID:        "ih-001",
    CertFingerprint: fingerprint,
    DeviceInfo: &session.DeviceInfo{
        DeviceID:   "device-123",
        OS:         "linux",
        OSVersion:  "5.15.0",
        Compliance: true,
    },
})

log.Printf("会话创建成功, Token: %s", session.Token)

// 验证会话
session, err := manager.ValidateSession(ctx, token)
if err != nil {
    log.Printf("会话无效: %v", err)
    return
}

// 刷新会话（延长过期时间）
session, err = manager.RefreshSession(ctx, token)

// 撤销会话
err = manager.RevokeSession(ctx, token)
```

---

## 4. policy - 策略引擎包

### 4.1 Engine - 策略引擎

**功能**: 策略查询、访问决策评估、策略加载

**接口定义**:

```go
type Engine struct {
    storage   Storage    // 存储接口
    evaluator Evaluator  // 评估接口
    logger    Logger
}

// 配置
type Config struct {
    Storage   Storage
    Evaluator Evaluator
    Logger    Logger
}
```

**核心方法**:

| 方法 | 签名 | 功能描述 |
|------|------|----------|
| `NewEngine` | `NewEngine(config *Config) (*Engine, error)` | 创建策略引擎 |
| `GetPoliciesForClient` | `GetPoliciesForClient(ctx context.Context, clientID string) ([]*Policy, error)` | 获取客户端策略列表 |
| `EvaluateAccess` | `EvaluateAccess(ctx context.Context, req *AccessRequest) (*AccessDecision, error)` | 评估访问请求 |
| `LoadPolicies` | `LoadPolicies(ctx context.Context, policies []*Policy) error` | 批量加载策略 |

**数据结构**:

```go
// Policy - 策略对象
type Policy struct {
    PolicyID         string
    ClientID         string
    ServiceID        string    // 通过 ServiceID 关联到 ServiceConfig，从中获取 TargetHost/Port
    BandwidthLimit   int64       // kbps
    ConcurrencyLimit int
    ExpiryTime       time.Time
    Conditions       []*Condition
}

// Condition - 策略条件
type Condition struct {
    Type     string      // device_os, geo_location, time_range
    Operator string      // eq, in, between
    Value    interface{}
}

// AccessRequest - 访问请求
type AccessRequest struct {
    ClientID   string
    ServiceID  string
    DeviceInfo *DeviceInfo
    SourceIP   string
    Timestamp  time.Time
}

// AccessDecision - 访问决策
type AccessDecision struct {
    Allowed     bool
    Reason      string
    Policy      *Policy
    Constraints *AccessConstraints
}

type AccessConstraints struct {
    MaxBandwidth   int64
    MaxConcurrency int
    ExpiresAt      time.Time
}
```

**使用示例**:

```go
// 创建策略引擎
storage := policy.NewDBStorage(db)
evaluator := &policy.DefaultEvaluator{}
engine, err := policy.NewEngine(&policy.Config{
    Storage:   storage,
    Evaluator: evaluator,
    Logger:    logger,
})

// 获取客户端策略
policies, err := engine.GetPoliciesForClient(ctx, "ih-001")
for _, p := range policies {
    fmt.Printf("服务: %s (策略ID: %s)\n", 
        p.ServiceID, p.PolicyID)
    // 注意：TargetHost/Port 应从 ServiceConfig 获取，而非 Policy
}

// 评估访问请求
decision, err := engine.EvaluateAccess(ctx, &policy.AccessRequest{
    ClientID:   "ih-001",
    ServiceID:  "postgres-db",
    DeviceInfo: deviceInfo,
    SourceIP:   "192.168.1.100",
    Timestamp:  time.Now(),
})

if decision.Allowed {
    fmt.Println("访问允许")
    fmt.Printf("带宽限制: %d kbps\n", decision.Constraints.MaxBandwidth)
} else {
    fmt.Printf("访问拒绝: %s\n", decision.Reason)
}
```

---

### 4.2 Storage - 策略存储接口

**功能**: 抽象策略存储层，支持多种后端（数据库、文件等）

**接口定义**:

```go
type Storage interface {
    SavePolicy(ctx context.Context, policy *Policy) error
    GetPolicy(ctx context.Context, policyID string) (*Policy, error)
    DeletePolicy(ctx context.Context, policyID string) error
    QueryPolicies(ctx context.Context, filter *PolicyFilter) ([]*Policy, error)
}

// DBStorage - 数据库实现
type DBStorage struct {
    db *gorm.DB
}

// PolicyFilter - 查询过滤器
type PolicyFilter struct {
    ClientID  string
    ServiceID string
    Active    bool  // 仅查询未过期策略
}
```

**实现示例**:

```go
// 创建数据库存储
storage := policy.NewDBStorage(db)

// 保存策略
err := storage.SavePolicy(ctx, &policy.Policy{
    PolicyID:         "policy-001",
    ClientID:         "ih-001",
    ServiceID:        "postgres-db",
    BandwidthLimit:   10485760, // 10 MB/s
    ConcurrencyLimit: 5,
})

// 查询策略
policies, err := storage.QueryPolicies(ctx, &policy.PolicyFilter{
    ClientID: "ih-001",
    Active:   true,
})
```

---

### 4.3 Evaluator - 策略评估器接口

**功能**: 插拔式策略评估逻辑，支持自定义评估规则

**接口定义**:

```go
type Evaluator interface {
    Evaluate(ctx context.Context, policy *Policy, evalCtx *EvalContext) (bool, error)
}

// EvalContext - 评估上下文
type EvalContext struct {
    DeviceInfo *DeviceInfo
    SourceIP   string
    Timestamp  time.Time
}

// DefaultEvaluator - 默认评估器
type DefaultEvaluator struct{}
```

**实现示例**:

```go
func (e *DefaultEvaluator) Evaluate(ctx context.Context, policy *Policy, evalCtx *EvalContext) (bool, error) {
    // 1. 检查过期时间
    if policy.ExpiryTime.Before(evalCtx.Timestamp) {
        return false, nil
    }
    
    // 2. 评估条件
    for _, cond := range policy.Conditions {
        if !e.evaluateCondition(cond, evalCtx) {
            return false, nil
        }
    }
    
    return true, nil
}
```

---

## 5. tunnel - 隧道管理包

### 5.1 Manager - 隧道生命周期管理

> **⚠️ 重要说明**: `Manager` 是一个**接口定义**，不是具体实现。`tunnel.NewManager()` 构造函数不存在。您需要自行实现此接口或使用以下参考实现。

**功能**: 隧道创建、查询、关闭，统一管理隧道状态

**接口定义**:

```go
type Manager interface {
    CreateTunnel(ctx context.Context, req *TunnelRequest) (*Tunnel, error)
    GetTunnel(ctx context.Context, tunnelID string) (*Tunnel, error)
    UpdateTunnel(ctx context.Context, tunnel *Tunnel) error
    DeleteTunnel(ctx context.Context, tunnelID string) error
    ListTunnels(ctx context.Context, filter TunnelFilter) ([]*Tunnel, error)
    GetStats(ctx context.Context, tunnelID string) (*TunnelStats, error)
}

// TunnelRequest - 隧道创建请求
type TunnelRequest struct {
    SessionToken string
    ClientID     string
    ServiceID    string
    LocalPort    int
}
```

**参考实现 - 内存版本**:

```go
// InMemoryTunnelManager 简单内存实现
type InMemoryTunnelManager struct {
    tunnels sync.Map // map[string]*Tunnel
    logger  logging.Logger
}

func NewInMemoryTunnelManager(logger logging.Logger) Manager {
    return &InMemoryTunnelManager{
        logger: logger,
    }
}

func (m *InMemoryTunnelManager) CreateTunnel(ctx context.Context, req *TunnelRequest) (*Tunnel, error) {
    tunnel := &Tunnel{
        ID:           uuid.New().String(),
        SessionToken: req.SessionToken,
        ClientID:     req.ClientID,
        ServiceID:    req.ServiceID,
        Status:       TunnelStatusActive,
        CreatedAt:    time.Now(),
        Stats:        &TunnelStats{},
    }
    
    m.tunnels.Store(tunnel.ID, tunnel)
    m.logger.Info("Tunnel created", "tunnel_id", tunnel.ID, "client_id", req.ClientID)
    
    return tunnel, nil
}

func (m *InMemoryTunnelManager) GetTunnel(ctx context.Context, tunnelID string) (*Tunnel, error) {
    if val, ok := m.tunnels.Load(tunnelID); ok {
        return val.(*Tunnel), nil
    }
    return nil, fmt.Errorf("tunnel not found: %s", tunnelID)
}

func (m *InMemoryTunnelManager) UpdateTunnel(ctx context.Context, tunnel *Tunnel) error {
    if _, ok := m.tunnels.Load(tunnel.ID); !ok {
        return fmt.Errorf("tunnel not found: %s", tunnel.ID)
    }
    m.tunnels.Store(tunnel.ID, tunnel)
    return nil
}

func (m *InMemoryTunnelManager) DeleteTunnel(ctx context.Context, tunnelID string) error {
    m.tunnels.Delete(tunnelID)
    m.logger.Info("Tunnel deleted", "tunnel_id", tunnelID)
    return nil
}

func (m *InMemoryTunnelManager) ListTunnels(ctx context.Context, filter TunnelFilter) ([]*Tunnel, error) {
    var tunnels []*Tunnel
    m.tunnels.Range(func(key, value interface{}) bool {
        tunnel := value.(*Tunnel)
        // 应用过滤条件...
        tunnels = append(tunnels, tunnel)
        return true
    })
    return tunnels, nil
}

func (m *InMemoryTunnelManager) GetStats(ctx context.Context, tunnelID string) (*TunnelStats, error) {
    tunnel, err := m.GetTunnel(ctx, tunnelID)
    if err != nil {
        return nil, err
    }
    return tunnel.Stats, nil
}
```

**生产环境实现建议**:

对于生产环境，建议使用数据库实现以支持持久化和高可用：

```go
// DBTunnelManager 数据库实现
type DBTunnelManager struct {
    db     *gorm.DB
    logger logging.Logger
}

func NewDBTunnelManager(db *gorm.DB, logger logging.Logger) Manager {
    return &DBTunnelManager{
        db:     db,
        logger: logger,
    }
}

// 实现 Manager 接口的所有方法...
```

**数据结构**:

```go
// Tunnel - 隧道对象
type Tunnel struct {
    ID           string
    SessionToken string
    ClientID     string
    ServiceID    string
    IHEndpoint   string
    AHEndpoint   string
    TargetHost   string
    TargetPort   int
    Protocol     string       // tcp, udp
    Status       TunnelStatus
    CreatedAt    time.Time
    LastActive   time.Time
    ExpiresAt    time.Time
    Stats        *TunnelStats
    Metadata     map[string]interface{}
}

type TunnelStatus string
const (
    TunnelStatusPending TunnelStatus = "pending"
    TunnelStatusActive  TunnelStatus = "active"
    TunnelStatusClosed  TunnelStatus = "closed"
    TunnelStatusError   TunnelStatus = "error"
)

// TunnelStats - 隧道统计
type TunnelStats struct {
    BytesSent     int64
    BytesReceived int64
    PacketsSent   int64
    PacketsRecv   int64
    ErrorCount    int64
    AvgLatency    time.Duration
}
```

**使用示例**:

```go
import (
    "context"
    "fmt"
    "github.com/houzhh15/sdp-common/tunnel"
    "github.com/houzhh15/sdp-common/logging"
)

// 1. 创建隧道管理器（使用内存实现）
logger := logging.NewLogger(&logging.Config{Level: "info"})
manager := NewInMemoryTunnelManager(logger)

// 或使用数据库实现
// manager := NewDBTunnelManager(db, logger)

// 2. 创建隧道
ctx := context.Background()
tun, err := manager.CreateTunnel(ctx, &tunnel.TunnelRequest{
    SessionToken: sessionToken,
    ClientID:     "ih-001",
    ServiceID:    "postgres-db",
    LocalPort:    15432,
})
if err != nil {
    log.Fatal(err)
}

fmt.Printf("隧道ID: %s, 状态: %s\n", tun.ID, tun.Status)

// 3. 查询隧道
tun, err = manager.GetTunnel(ctx, tun.ID)
if err != nil {
    log.Fatal(err)
}

// 4. 更新隧道状态
tun.Status = tunnel.TunnelStatusActive
tun.LastActive = time.Now()
err = manager.UpdateTunnel(ctx, tun)

// 5. 获取隧道统计
stats, err := manager.GetStats(ctx, tun.ID)
fmt.Printf("流量统计: 发送 %d 字节, 接收 %d 字节\n", 
    stats.BytesSent, stats.BytesReceived)
tun, err := manager.GetTunnel(ctx, tunnelID)

// 关闭隧道
err = manager.DeleteTunnel(ctx, tunnelID)
```

**关于 Tunnel 结构的重要变更** (2025-11-17):

```go
// ⚠️ Tunnel 结构已简化，移除 TargetHost/Port 字段
type Tunnel struct {
    ID           string
    SessionToken string
    ClientID     string
    ServiceID    string // ✅ 新增：通过 ServiceID 关联 ServiceConfig
    IHEndpoint   string
    AHEndpoint   string
    Protocol     string
    Status       TunnelStatus
    CreatedAt    time.Time
    LastActive   time.Time
    ExpiresAt    time.Time
    Stats        *TunnelStats
    Metadata     map[string]interface{} // 内部存储 target_host/target_port
}

// 迁移指南：创建隧道时无需提供 TargetHost/Port
// 1. 先配置 ServiceConfig
manager.CreateServiceConfig(ctx, &tunnel.ServiceConfig{
    ServiceID:  "service-001",
    TargetHost: "localhost",
    TargetPort: 8080,
})

// 2. 创建隧道（自动查询 ServiceConfig 并填充 Metadata）
tun, _ := manager.CreateTunnel(ctx, &tunnel.CreateTunnelRequest{
    ClientID:  "ih-001",
    ServiceID: "service-001", // ✅ 仅需 ServiceID
    Protocol:  "tcp",
})
```

---

### 5.2 ServiceConfig - 服务配置管理

> **✨ 新增功能** (2025-11-17): 支持 SDP 2.0 规范 0x04 消息（混合方案：HTTP GET + SSE Push）

**功能**: 管理 AH Agent 需要代理的后端服务配置，实现控制/数据平面分离

**接口定义**:

```go
type Manager interface {
    // ... 原有隧道管理方法 ...

    // ===== 服务配置管理（SDP 2.0 规范 0x04 消息支持）=====
    CreateServiceConfig(ctx context.Context, config *ServiceConfig) error
    GetServiceConfig(ctx context.Context, serviceID string) (*ServiceConfig, error)
    ListServiceConfigs(ctx context.Context, agentID string) ([]*ServiceConfig, error)
    UpdateServiceConfig(ctx context.Context, config *ServiceConfig) error
    DeleteServiceConfig(ctx context.Context, serviceID string) error
}
```

**数据结构**:

```go
// ServiceConfig 服务配置
type ServiceConfig struct {
    ServiceID   string                 `json:"service_id"`   // 服务标识
    ServiceName string                 `json:"service_name"` // 服务名称（可读）
    TargetHost  string                 `json:"target_host"`  // 目标主机地址
    TargetPort  int                    `json:"target_port"`  // 目标端口
    Protocol    string                 `json:"protocol"`     // 协议类型（tcp/udp）
    Description string                 `json:"description"`  // 服务描述
    Status      ServiceStatus          `json:"status"`       // 服务状态
    CreatedAt   time.Time              `json:"created_at"`
    UpdatedAt   time.Time              `json:"updated_at"`
    Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

type ServiceStatus string
const (
    ServiceStatusActive   ServiceStatus = "active"   // 活跃
    ServiceStatusInactive ServiceStatus = "inactive" // 停用
    ServiceStatusDeleted  ServiceStatus = "deleted"  // 已删除
)

// ServiceEvent 服务配置事件（用于 SSE 推送）
type ServiceEvent struct {
    Type      ServiceEventType       `json:"type"`
    Service   *ServiceConfig         `json:"service"`
    Timestamp time.Time              `json:"timestamp"`
    Details   map[string]interface{} `json:"details,omitempty"`
}

type ServiceEventType string
const (
    ServiceEventCreated ServiceEventType = "service_created"
    ServiceEventUpdated ServiceEventType = "service_updated"
    ServiceEventDeleted ServiceEventType = "service_deleted"
)
```

**使用示例 - Controller 端**:

```go
import (
    "context"
    "github.com/houzhh15/sdp-common/tunnel"
)

// 1. 创建服务配置
manager := NewInMemoryTunnelManager(logger)

serviceConfig := &tunnel.ServiceConfig{
    ServiceID:   "postgres-prod",
    ServiceName: "Production PostgreSQL",
    TargetHost:  "db.internal.company.com",
    TargetPort:  5432,
    Protocol:    "tcp",
    Description: "Production database",
    Status:      tunnel.ServiceStatusActive,
}

ctx := context.Background()
if err := manager.CreateServiceConfig(ctx, serviceConfig); err != nil {
    log.Fatal(err)
}

// 2. 查询单个服务配置
config, err := manager.GetServiceConfig(ctx, "postgres-prod")
if err != nil {
    log.Fatal(err)
}
fmt.Printf("服务: %s → %s:%d\n", config.ServiceName, config.TargetHost, config.TargetPort)

// 3. 列出所有服务配置
configs, err := manager.ListServiceConfigs(ctx, "")
for _, cfg := range configs {
    fmt.Printf("- %s: %s:%d (%s)\n", cfg.ServiceID, cfg.TargetHost, cfg.TargetPort, cfg.Status)
}

// 4. 更新服务配置（触发 SSE Push）
serviceConfig.TargetPort = 5433
if err := manager.UpdateServiceConfig(ctx, serviceConfig); err != nil {
    log.Fatal(err)
}

// 推送更新事件给 AH Agents
event := &tunnel.ServiceEvent{
    Type:      tunnel.ServiceEventUpdated,
    Service:   serviceConfig,
    Timestamp: time.Now(),
}
notifier.NotifyService(event) // 通过 SSE 推送

// 5. 删除服务配置
if err := manager.DeleteServiceConfig(ctx, "postgres-prod"); err != nil {
    log.Fatal(err)
}
```

**使用示例 - AH Agent 端（混合方案）**:

```go
import (
    "context"
    "encoding/json"
    "net/http"
    "time"
    "github.com/houzhh15/sdp-common/tunnel"
)

// 步骤 1: HTTP GET 获取初始服务配置
func fetchServiceConfigs(controllerURL string, tlsConfig *tls.Config) ([]*tunnel.ServiceConfig, error) {
    client := &http.Client{
        Transport: &http.Transport{TLSClientConfig: tlsConfig},
        Timeout:   10 * time.Second,
    }

    url := fmt.Sprintf("%s/api/v1/services", controllerURL)
    resp, err := client.Get(url)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()

    var result struct {
        Status   string                    `json:"status"`
        Services []*tunnel.ServiceConfig   `json:"services"`
        Count    int                       `json:"count"`
    }

    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return nil, err
    }

    return result.Services, nil
}

// 步骤 2: SSE 订阅服务配置变更
subscriber := tunnel.NewSubscriber(&tunnel.SubscriberConfig{
    ControllerURL: "https://controller:8443",
    TLSConfig:     tlsConfig,
    Callback: func(event *tunnel.TunnelEvent) error {
        // 处理服务配置事件
        if event.Type == "service_updated" {
            // 从 event.Details 提取 ServiceConfig
            // 更新本地服务配置
        }
        return nil
    },
    Logger: logger,
})

subscriber.Start(context.Background())
```

**HTTP API 端点（Controller 端实现参考）**:

```go
// GET /api/v1/services - 列出所有服务配置
mux.HandleFunc("/api/v1/services", func(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
        return
    }

    ctx := r.Context()
    configs, err := tunnelManager.ListServiceConfigs(ctx, "")
    if err != nil {
        respondError(w, protocol.ErrCodeServiceUnavail, "Failed to retrieve services", nil)
        return
    }

    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(map[string]interface{}{
        "status":   "success",
        "services": configs,
        "count":    len(configs),
    })
})

// GET /api/v1/services/{id} - 获取单个服务配置
mux.HandleFunc("/api/v1/services/", func(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
        return
    }

    ctx := r.Context()
    serviceID := strings.TrimPrefix(r.URL.Path, "/api/v1/services/")
    
    config, err := tunnelManager.GetServiceConfig(ctx, serviceID)
    if err != nil {
        respondError(w, protocol.ErrCodeNotFound, "Service not found", nil)
        return
    }

    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(map[string]interface{}{
        "status":  "success",
        "service": config,
    })
})
```

**架构优势**:

1. **符合 SDP 2.0 规范**: TargetHost/Port 不再通过控制平面传输（Tunnel 结构）
2. **混合方案**: HTTP GET（初始加载）+ SSE Push（实时更新），100% 场景覆盖
3. **性能优化**: 初始加载 < 100ms，实时推送 < 50ms（P99）
4. **易于维护**: 服务配置集中管理，动态更新无需重启


---

### 5.3 DataPlaneClient - 数据平面客户端 SDK

> **✨ 新增功能** (2025-11-17): 封装数据平面连接协议，简化 IH Client 和 AH Agent 实现

**功能**: 统一的数据平面连接 SDK，自动处理 Tunnel ID 握手协议

**核心特性**:
- **协议封装**: 隐藏 36 字节固定长度 Tunnel ID 握手协议细节
- **自动重试**: 内置连接重试机制（可配置）
- **错误处理**: 统一的错误类型和超时控制
- **TLS 支持**: 原生 mTLS 集成

**接口定义**:

```go
// DataPlaneClient 数据平面客户端
type DataPlaneClient struct {
    serverAddr string
    tlsConfig  *tls.Config
    timeout    time.Duration
}

// Config 配置选项
type DataPlaneClientConfig struct {
    ServerAddr string        // Controller TCP Proxy 地址 (例: "localhost:9443")
    TLSConfig  *tls.Config   // mTLS 配置
    Timeout    time.Duration // 连接超时（默认 10s）
}
```

**核心方法**:

| 方法 | 签名 | 功能描述 |
|------|------|----------|
| **NewDataPlaneClient** | `(serverAddr string, tlsConfig *tls.Config) *DataPlaneClient` | 创建客户端实例 |
| **Connect** | `(tunnelID string) (net.Conn, error)` | 建立连接并发送 Tunnel ID |
| **ConnectWithRetry** | `(tunnelID string, maxRetries int, retryDelay time.Duration) (net.Conn, error)` | 带重试的连接 |

**使用示例 - IH Client**:

```go
import (
    "github.com/houzhh15/sdp-common/tunnel"
    "crypto/tls"
    "log"
)

// 1. 准备 mTLS 配置
tlsConfig := &tls.Config{
    Certificates: []tls.Certificate{clientCert},
    RootCAs:      caCertPool,
}

// 2. 创建数据平面客户端
client := tunnel.NewDataPlaneClient("localhost:9443", tlsConfig)

// 3. 建立数据平面连接（自动发送 Tunnel ID）
proxyConn, err := client.Connect("550e8400-e29b-41d4-a716-446655440000")
if err != nil {
    log.Fatal("连接失败:", err)
}
defer proxyConn.Close()

// 4. 现在可以直接进行数据转发
// proxyConn 已经完成 Tunnel ID 握手，可以直接读写数据
io.Copy(localConn, proxyConn)
```

**使用示例 - AH Agent**:

```go
import (
    "github.com/houzhh15/sdp-common/tunnel"
    "crypto/tls"
)

// 1. 创建客户端
client := tunnel.NewDataPlaneClient("localhost:9443", tlsConfig)

// 2. 带重试连接（生产环境推荐）
proxyConn, err := client.ConnectWithRetry(
    tunnelID,
    3,                    // 最大重试 3 次
    2 * time.Second,      // 每次重试间隔 2s
)
if err != nil {
    log.Printf("连接失败（已重试3次）: %v", err)
    return
}
defer proxyConn.Close()

// 3. 连接到后端服务
targetConn, _ := net.Dial("tcp", "localhost:8080")
defer targetConn.Close()

// 4. 双向数据转发
go io.Copy(proxyConn, targetConn)
io.Copy(targetConn, proxyConn)
```

**完整示例 - 自定义配置**:

```go
// 高级配置
client := tunnel.NewDataPlaneClient("controller.example.com:9443", &tls.Config{
    Certificates:       []tls.Certificate{clientCert},
    RootCAs:            caCertPool,
    ServerName:         "controller.example.com",
    InsecureSkipVerify: false,
    MinVersion:         tls.VersionTLS13,
})

// 设置连接超时（默认 10s）
client.SetTimeout(5 * time.Second)

// 连接并处理错误
proxyConn, err := client.Connect(tunnelID)
if err != nil {
    switch {
    case errors.Is(err, context.DeadlineExceeded):
        log.Println("连接超时")
    case errors.Is(err, net.ErrClosed):
        log.Println("连接已关闭")
    default:
        log.Printf("连接错误: %v", err)
    }
    return
}
```

**协议细节（SDK 内部实现）**:

```go
// sendTunnelID 内部方法 - 发送 Tunnel ID 握手
func (c *DataPlaneClient) sendTunnelID(conn net.Conn, tunnelID string) error {
    // 1. 验证 Tunnel ID 格式（UUID）
    if len(tunnelID) == 0 || len(tunnelID) > TunnelIDLength {
        return fmt.Errorf("invalid tunnel ID length: %d", len(tunnelID))
    }

    // 2. 固定 36 字节格式（右填充 null）
    buf := make([]byte, TunnelIDLength)
    copy(buf, []byte(tunnelID))

    // 3. 发送握手数据
    if _, err := conn.Write(buf); err != nil {
        return fmt.Errorf("failed to send tunnel ID: %w", err)
    }

    return nil
}
```

**错误处理**:

```go
// 常见错误类型
var (
    ErrInvalidTunnelID = errors.New("invalid tunnel ID")
    ErrConnectionFailed = errors.New("connection failed")
    ErrHandshakeFailed = errors.New("tunnel ID handshake failed")
)

// 错误判断示例
proxyConn, err := client.Connect(tunnelID)
if err != nil {
    if errors.Is(err, tunnel.ErrInvalidTunnelID) {
        log.Println("Tunnel ID 格式错误")
    } else if errors.Is(err, tunnel.ErrHandshakeFailed) {
        log.Println("握手失败，可能 Controller 不接受此 Tunnel ID")
    }
    return
}
```

**性能优化建议**:

```go
// 1. 连接池（高并发场景）
type ConnectionPool struct {
    clients []*tunnel.DataPlaneClient
    mu      sync.Mutex
}

// 2. 连接复用（长连接场景）
client := tunnel.NewDataPlaneClient(serverAddr, tlsConfig)
for tunnelID := range tunnelQueue {
    conn, _ := client.Connect(tunnelID)
    go handleConnection(conn) // 每个 Tunnel ID 独立 goroutine
}

// 3. 超时控制（避免长时间阻塞）
client.SetTimeout(3 * time.Second)
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

// 在 goroutine 中连接
connCh := make(chan net.Conn)
go func() {
    conn, err := client.Connect(tunnelID)
    if err == nil {
        connCh <- conn
    }
}()

select {
case conn := <-connCh:
    // 连接成功
case <-ctx.Done():
    log.Println("总超时（5s）")
}
```

**与服务端配合**:

```go
// Controller 端（transport.TCPProxyServer）会自动处理 Tunnel ID 握手
// 客户端使用 DataPlaneClient 后，协议完全兼容：

// 服务端读取 Tunnel ID（transport/tcp_proxy_server.go）
buf := make([]byte, 36)
io.ReadFull(conn, buf)
tunnelID := string(bytes.TrimRight(buf, "\x00"))

// 客户端发送 Tunnel ID（tunnel/client.go）
client.Connect(tunnelID) // SDK 自动发送 36 字节握手
```

**最佳实践**:

1. **统一使用 SDK**: IH Client 和 AH Agent 都应使用 `DataPlaneClient`，避免手动实现协议
2. **配置重试**: 生产环境推荐使用 `ConnectWithRetry` 提高可靠性
3. **超时控制**: 根据网络环境设置合理的 `Timeout`（默认 10s）
4. **错误日志**: 连接失败时记录详细错误信息，便于排查问题
5. **资源清理**: 使用 `defer conn.Close()` 确保连接关闭

**完整协议规范**: 参见 `docs/DATA_PLANE_PROTOCOL.md`

---

### 5.4 Notifier - SSE 实时推送管理器

**功能**: 管理 SSE 客户端连接，实时推送隧道事件（创建、更新、删除）

**接口定义**:

```go
type Notifier interface {
    Subscribe(agentID string, w http.ResponseWriter) error
    Unsubscribe(agentID string)
    Notify(event *TunnelEvent) error
    NotifyOne(agentID string, event *TunnelEvent) error
    GetClients() []string
    
    // ===== 服务配置推送（双通道支持）=====
    NotifyService(event *ServiceEvent) error              // 广播服务配置事件
    NotifyServiceOne(agentID string, event *ServiceEvent) error // 单播服务配置事件
}

// TunnelEvent - 隧道事件
type TunnelEvent struct {
    Type      EventType              // created, updated, deleted
    Tunnel    *Tunnel                // 隧道对象（包含 ID、ServiceID 等基本信息）
    Timestamp time.Time              // 事件时间戳
    Details   map[string]interface{} // 事件详情（例如：controller_addr - Controller 数据平面地址）
}

type EventType string
const (
    EventTypeCreated EventType = "created"
    EventTypeUpdated EventType = "updated"
    EventTypeDeleted EventType = "deleted"
)

// ServiceEvent - 服务配置事件（已在 5.2 ServiceConfig 部分定义）
```

**重要说明 - Controller 数据平面地址传递**:

> **✨ 架构设计** (2025-11-19): Controller 通过 `event.Details["controller_addr"]` 传递数据平面地址

当 Controller 创建隧道时，会在 SSE 事件的 `Details` 字段中包含 `controller_addr`，指示 IH Client 和 AH Agent 连接到 Controller 的数据平面中继服务器（TunnelRelayServer）。

**字段优先级**（AH Agent 端获取 Controller 地址）:
1. **最高优先级**: `event.Details["controller_addr"]` - Controller 推送的动态地址
2. **次优先级**: `event.Tunnel.Metadata["ah_endpoint"]` - 隧道元数据中的端点
3. **兜底方案**: `event.Tunnel.AHEndpoint` - 隧道对象的 AH 端点字段

**推荐做法**:
- Controller 端：在 `handleTunnelCreate` 中设置 `event.Details["controller_addr"]`
- AH Agent 端：优先从 `event.Details` 获取，支持多级 fallback

**使用示例 - 隧道事件推送**:

```go
// 创建 Notifier
notifier := tunnel.NewNotifier(logger, 30*time.Second)

// HTTP 处理器中订阅
http.HandleFunc("/api/v1/tunnels/stream", func(w http.ResponseWriter, r *http.Request) {
    agentID := r.URL.Query().Get("agent_id")
    
    // 阻塞式订阅，保持连接
    if err := notifier.Subscribe(agentID, w); err != nil {
        log.Printf("订阅失败: %v", err)
    }
    
    defer notifier.Unsubscribe(agentID)
})

// 发送隧道事件（广播）
err := notifier.Notify(&tunnel.TunnelEvent{
    Type:   tunnel.EventTypeCreated,
    Tunnel: newTunnel,
    Details: map[string]interface{}{
        "controller_addr": "localhost:9443", // Controller 数据平面地址（IH 和 AH 连接地址）
    },
})

// 发送给特定客户端（单播）
err := notifier.NotifyOne("ah-agent-001", event)
```

**使用示例 - 服务配置事件推送**:

> **✨ 新增功能** (2025-11-17): 双通道 SSE 支持（隧道 + 服务配置）

```go
// 推送服务配置创建事件（广播）
serviceEvent := &tunnel.ServiceEvent{
    Type: tunnel.ServiceEventCreated,
    Service: &tunnel.ServiceConfig{
        ServiceID:   "postgres-prod",
        ServiceName: "Production PostgreSQL",
        TargetHost:  "db.internal.com",
        TargetPort:  5432,
        Status:      tunnel.ServiceStatusActive,
    },
    Timestamp: time.Now(),
}
err := notifier.NotifyService(serviceEvent)

// 推送服务配置更新事件（单播给特定 AH Agent）
updateEvent := &tunnel.ServiceEvent{
    Type:      tunnel.ServiceEventUpdated,
    Service:   updatedConfig,
    Timestamp: time.Now(),
    Details: map[string]interface{}{
        "changed_fields": []string{"target_port", "status"},
    },
}
err := notifier.NotifyServiceOne("ah-agent-001", updateEvent)

// AH Agent 端接收（参考 5.4 Subscriber）
// SSE 客户端会在两个独立通道上接收事件：
// - TunnelChannel:  接收隧道创建/更新/删除事件
// - ServiceChannel: 接收服务配置变更事件
```

**双通道架构说明**:

```
Controller (Notifier)                     AH Agent (Subscriber)
┌────────────────────┐                    ┌──────────────────────┐
│  Notify()          │───────────────────>│  TunnelChannel       │
│  NotifyOne()       │                    │  (隧道事件)          │
└────────────────────┘                    └──────────────────────┘
┌────────────────────┐                    ┌──────────────────────┐
│  NotifyService()   │───────────────────>│  ServiceChannel      │
│  NotifyServiceOne()│                    │  (服务配置事件)      │
└────────────────────┘                    └──────────────────────┘
```

---

### 5.5 Subscriber - AH 端隧道订阅器

**功能**: AH Agent 端订阅隧道事件，自动重连和断线恢复

**接口定义**:

```go
type Subscriber interface {
    Start(ctx context.Context) error
    Stop() error
    Events() <-chan *TunnelEvent  // 隧道事件通道
    IsConnected() bool
}

// SubscriberConfig - 订阅器配置
type SubscriberConfig struct {
    ControllerURL string
    AgentID       string
    TLSConfig     *tls.Config
    Callback      func(*TunnelEvent) error  // 隧道事件回调
    Logger        Logger
}
```

**使用示例 - 隧道事件订阅**:

```go
// 创建订阅器
subscriber := tunnel.NewSubscriber(&tunnel.SubscriberConfig{
    ControllerURL: "https://controller:8443",
    AgentID:       "ah-agent-001",
    TLSConfig:     tlsConfig,
    Callback:      handleTunnelEvent,
    Logger:        logger,
})

// 启动订阅
ctx := context.Background()
go subscriber.Start(ctx)

// 监听事件
for event := range subscriber.Events() {
    switch event.Type {
    case tunnel.EventTypeCreated:
        log.Printf("新隧道创建: %s", event.Tunnel.ID)
        
        // 从 event.Details 获取 Controller 数据平面地址
        var controllerAddr string
        if event.Details != nil {
            if addr, ok := event.Details["controller_addr"].(string); ok {
                controllerAddr = addr
            }
        }
        
        // Fallback: 从 Tunnel.Metadata 或 Tunnel.AHEndpoint 获取
        if controllerAddr == "" && event.Tunnel.Metadata != nil {
            if endpoint, ok := event.Tunnel.Metadata["ah_endpoint"].(string); ok {
                controllerAddr = endpoint
            }
        }
        if controllerAddr == "" {
            controllerAddr = event.Tunnel.AHEndpoint
        }
        
        // 建立到 Controller 数据平面的连接
        if controllerAddr != "" {
            handleNewTunnel(event.Tunnel, controllerAddr)
        }
        
    case tunnel.EventTypeDeleted:
        log.Printf("隧道关闭: %s", event.Tunnel.ID)
        // 清理本地资源
        cleanupTunnel(event.Tunnel.ID)
    }
}

// 停止订阅
subscriber.Stop()
```

**使用示例 - 服务配置事件接收（双通道）**:

> **✨ 新增功能** (2025-11-17): SSE 订阅现在支持接收服务配置事件

```go
// AH Agent 端使用示例（参考 examples/ah-agent/main.go）

// 1. HTTP GET 初始加载服务配置
func fetchServiceConfigs(controllerURL string, tlsConfig *tls.Config) ([]*tunnel.ServiceConfig, error) {
    client := &http.Client{
        Transport: &http.Transport{TLSClientConfig: tlsConfig},
        Timeout:   10 * time.Second,
    }

    url := fmt.Sprintf("%s/api/v1/services", controllerURL)
    resp, err := client.Get(url)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()

    var result struct {
        Status   string                    `json:"status"`
        Services []*tunnel.ServiceConfig   `json:"services"`
        Count    int                       `json:"count"`
    }

    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return nil, err
    }

    return result.Services, nil
}

// 2. SSE 订阅实时更新（隧道 + 服务配置双通道）
subscriber := tunnel.NewSubscriber(&tunnel.SubscriberConfig{
    ControllerURL: "https://controller:8443",
    AgentID:       "ah-agent-001",
    TLSConfig:     tlsConfig,
    Callback: func(event *tunnel.TunnelEvent) error {
        // 处理服务配置事件（通过 event.Metadata 传递）
        if eventType, ok := event.Metadata["event_type"].(string); ok {
            switch eventType {
            case "service_created", "service_updated":
                // 从 event.Metadata["service"] 提取 ServiceConfig
                if serviceData, ok := event.Metadata["service"].(map[string]interface{}); ok {
                    serviceID := serviceData["service_id"].(string)
                    targetHost := serviceData["target_host"].(string)
                    targetPort := int(serviceData["target_port"].(float64))
                    
                    // 更新本地服务配置
                    updateLocalService(serviceID, targetHost, targetPort)
                    logger.Info("服务配置已更新", "service_id", serviceID)
                }
            case "service_deleted":
                serviceID := event.Metadata["service_id"].(string)
                removeLocalService(serviceID)
                logger.Info("服务配置已删除", "service_id", serviceID)
            }
        }
        
        // 处理隧道事件
        switch event.Type {
        case tunnel.EventTypeCreated:
            handleNewTunnel(event.Tunnel)
        case tunnel.EventTypeDeleted:
            cleanupTunnel(event.Tunnel.ID)
        }
        
        return nil
    },
    Logger: logger,
})

go subscriber.Start(context.Background())
```

**完整混合方案示例（HTTP GET + SSE）**:

```go
// AH Agent 启动流程
func main() {
    // 步骤 1: HTTP GET 初始化服务配置
    services, err := fetchServiceConfigs(controllerURL, tlsConfig)
    if err != nil {
        log.Fatalf("Failed to fetch services: %v", err)
    }
    
    // 存储到本地映射
    serviceConfigs := make(map[string]*tunnel.ServiceConfig)
    for _, svc := range services {
        serviceConfigs[svc.ServiceID] = svc
        logger.Info("Loaded service", "id", svc.ServiceID, "target", 
            fmt.Sprintf("%s:%d", svc.TargetHost, svc.TargetPort))
    }
    
    // 步骤 2: SSE 订阅实时更新
    subscriber := tunnel.NewSubscriber(&tunnel.SubscriberConfig{
        ControllerURL: controllerURL,
        AgentID:       agentID,
        TLSConfig:     tlsConfig,
        Callback: func(event *tunnel.TunnelEvent) error {
            // 处理服务配置变更
            if eventType, ok := event.Metadata["event_type"].(string); ok {
                if strings.HasPrefix(eventType, "service_") {
                    handleServiceEvent(event, serviceConfigs)
                    return nil
                }
            }
            
            // 处理隧道事件
            handleTunnelEvent(event, serviceConfigs)
            return nil
        },
        Logger: logger,
    })
    
    go subscriber.Start(context.Background())
    
    // 步骤 3: 启动 TCP Proxy 服务器
    proxyServer := tunnel.NewTCPProxy(ahEndpoint, tlsConfig, logger)
    proxyServer.Start(context.Background())
}

func handleServiceEvent(event *tunnel.TunnelEvent, configs map[string]*tunnel.ServiceConfig) {
    eventType := event.Metadata["event_type"].(string)
    
    switch eventType {
    case "service_created", "service_updated":
        serviceData := event.Metadata["service"].(map[string]interface{})
        config := &tunnel.ServiceConfig{
            ServiceID:  serviceData["service_id"].(string),
            TargetHost: serviceData["target_host"].(string),
            TargetPort: int(serviceData["target_port"].(float64)),
            Protocol:   serviceData["protocol"].(string),
            Status:     tunnel.ServiceStatus(serviceData["status"].(string)),
        }
        configs[config.ServiceID] = config
        logger.Info("Service config updated", "id", config.ServiceID)
        
    case "service_deleted":
        serviceID := event.Metadata["service_id"].(string)
        delete(configs, serviceID)
        logger.Info("Service config deleted", "id", serviceID)
    }
}
```

---

### 5.6 TCPProxy - 数据平面透明代理

**功能**: 处理 IH-AH 数据平面连接配对和双向数据转发，零拷贝优化

**接口定义**:

```go
type TCPProxy struct {
    tunnels    map[string]*TunnelConnection
    tunnelsMu  sync.RWMutex
    pendingIH  map[string]*TunnelConnection
    pendingAH  map[string]*TunnelConnection
    pendingMu  sync.RWMutex
    logger     logging.Logger
    bufferSize int
    timeout    time.Duration
}

// TunnelConnection 隧道连接对
type TunnelConnection struct {
    TunnelID   string
    IHConn     net.Conn
    AHConn     net.Conn
    CreatedAt  time.Time
    LastActive time.Time
}
```

**核心方法**:

| 方法 | 签名 | 功能描述 |
|------|------|----------|
| `NewTCPProxy` | `NewTCPProxy(logger logging.Logger, bufferSize int, timeout time.Duration) *TCPProxy` | 创建 TCP 代理，bufferSize=0 使用默认值 |
| `HandleIHConnection` | `HandleIHConnection(conn net.Conn)` | 处理 IH 端连接（读取 tunnel ID 并配对） |
| `HandleAHConnection` | `HandleAHConnection(conn net.Conn)` | 处理 AH 端连接（读取 tunnel ID 并配对） |
| `GetActiveTunnels` | `GetActiveTunnels() []*TunnelConnection` | 获取所有活跃隧道 |
| `CloseTunnel` | `CloseTunnel(tunnelID string) error` | 关闭指定隧道 |

**使用示例**:

```go
// 创建 TCP Proxy
proxy := tunnel.NewTCPProxy(
    logger,
    32*1024,           // bufferSize: 32KB 缓冲区
    30*time.Second,    // timeout: 30秒超时
)

// 在 IH 端处理连接
go func() {
    ln, _ := net.Listen("tcp", ":9443")
    for {
        conn, _ := ln.Accept()
        go proxy.HandleIHConnection(conn)
    }
}()

// 在 AH 端处理连接
go func() {
    ln, _ := net.Listen("tcp", ":9444")
    for {
        conn, _ := ln.Accept()
        go proxy.HandleAHConnection(conn)
    }
}()
```

---

### 5.7 Broker - gRPC 流转发（可选）

**功能**: gRPC 双向流数据转发，心跳监测（高性能场景使用）

**接口定义**:

```go
type Broker interface {
    RegisterEndpoint(tunnelID string, stream TunnelStream, isIH bool) error
    ForwardData(tunnelID string) error
    CloseTunnel(tunnelID string) error
}

type TunnelStream interface {
    Send(*DataPacket) error
    Recv() (*DataPacket, error)
}

// DataPacket - 数据包
type DataPacket struct {
    TunnelID  string
    Sequence  uint64
    Payload   []byte
    Timestamp time.Time
}
```

**使用示例**:

```go
// 创建 Broker
broker := tunnel.NewBroker(&tunnel.BrokerConfig{
    Logger:            logger,
    HeartbeatInterval: 30 * time.Second,
    HeartbeatTimeout:  60 * time.Second,
})

// 注册 IH 端点
err := broker.RegisterEndpoint(tunnelID, ihStream, true)

// 注册 AH 端点
err := broker.RegisterEndpoint(tunnelID, ahStream, false)

// 数据转发（自动进行）
// Broker 自动将 IH 和 AH 之间的数据双向转发

// 关闭隧道
err = broker.CloseTunnel(tunnelID)
```

---

### 5.8 EventStore - 事件持久化存储接口

> **✨ 新增功能** (2025-11-22): 支持 SSE 事件持久化和 Last-Event-ID 重连恢复机制

**功能**: 事件存储接口，用于实现 SSE 断线重连时的事件恢复，支持多种存储实现（Redis Stream、Kafka、PostgreSQL、Memory 等）

**设计理念**:
- **协议无关**: Event 结构不包含 SSE 特定格式，支持 WebSocket/gRPC 等多种传输协议
- **存储无关**: 接口不绑定特定存储系统，可灵活切换
- **零事件丢失**: 通过 Last-Event-ID 机制确保 SSE 重连后能恢复错过的事件

**接口定义**:

```go
type EventStore interface {
    // Publish 发布事件到指定订阅者
    // subscriberID: 订阅者唯一标识（如 agentID）
    // event: 要发布的事件
    // 返回: 事件ID（用于 Last-Event-ID），错误
    Publish(ctx context.Context, subscriberID string, event *Event) (eventID string, err error)

    // Subscribe 订阅事件流（从指定 ID 之后开始）
    // subscriberID: 订阅者唯一标识
    // lastEventID: 上次收到的事件 ID，为空表示从最新事件开始
    // 返回: 事件通道（实时事件流），错误
    Subscribe(ctx context.Context, subscriberID, lastEventID string) (<-chan *Event, error)

    // GetEventsAfter 获取指定 ID 之后的历史事件（用于重连恢复）
    // subscriberID: 订阅者唯一标识
    // lastEventID: 上次收到的事件 ID
    // limit: 最大返回数量（0 表示使用默认值）
    // 返回: 历史事件列表，错误
    GetEventsAfter(ctx context.Context, subscriberID, lastEventID string, limit int) ([]*Event, error)

    // Ack 确认事件已处理（可选，用于消费者组模式）
    Ack(ctx context.Context, subscriberID, eventID string) error

    // Close 关闭存储连接
    Close() error
}
```

**数据结构**:

```go
// Event 通用事件结构（协议无关）
type Event struct {
    // ID 事件唯一标识（由存储系统生成，如 Redis Stream ID: "1637856000000-0"）
    ID string `json:"id"`

    // Type 事件类型（tunnel.created, service.updated, policy.changed 等）
    Type string `json:"type"`

    // Data 事件数据（JSON 格式）
    Data json.RawMessage `json:"data"`

    // Timestamp 事件时间戳（Unix 毫秒）
    Timestamp int64 `json:"timestamp"`

    // Metadata 可选的元数据
    Metadata map[string]string `json:"metadata,omitempty"`
}

// 标准事件类型常量
const (
    EventTypeTunnelCreated  = "tunnel.created"
    EventTypeTunnelClosed   = "tunnel.closed"
    EventTypeServiceUpdated = "service.updated"
    EventTypePolicyChanged  = "policy.changed"
    EventTypeAgentStatus    = "agent.status"
)

// TunnelEventData 隧道事件数据结构
type TunnelEventData struct {
    Action    string      `json:"action"` // created, closed, updated
    Tunnel    *TunnelInfo `json:"tunnel"`
    Timestamp time.Time   `json:"timestamp"`
}

// TunnelInfo 隧道信息（简化版，避免循环依赖）
type TunnelInfo struct {
    ID        string `json:"id"`
    ClientID  string `json:"client_id"`
    ServiceID string `json:"service_id"`
    Status    string `json:"status"`
}

// ServiceEventData 服务事件数据结构
type ServiceEventData struct {
    Action    string                 `json:"action"` // updated, removed
    ServiceID string                 `json:"service_id"`
    Config    map[string]interface{} `json:"config"`
    Timestamp time.Time              `json:"timestamp"`
}
```

**辅助函数**:

```go
// NewEvent 创建新事件（辅助函数）
func NewEvent(eventType string, data interface{}) (*Event, error) {
    jsonData, err := json.Marshal(data)
    if err != nil {
        return nil, err
    }

    return &Event{
        Type:      eventType,
        Data:      jsonData,
        Timestamp: time.Now().UnixMilli(),
        Metadata:  make(map[string]string),
    }, nil
}

// ParseData 解析事件数据到目标结构
func (e *Event) ParseData(v interface{}) error {
    return json.Unmarshal(e.Data, v)
}
```

**实现建议**:

EventStore 是一个接口，需要在项目中实现具体的存储方案：

1. **Redis Stream 实现** (推荐用于生产环境):
```go
// internal/event/redis_store.go
type RedisEventStore struct {
    rdb          *redis.Client
    streamPrefix string        // "events:"
    maxLen       int64         // 1000
    ttl          time.Duration // 24h
}

func (s *RedisEventStore) Publish(ctx context.Context, subscriberID string, event *Event) (string, error) {
    streamKey := s.streamPrefix + subscriberID
    eventJSON, _ := json.Marshal(event)
    
    // 使用 XADD 命令，自动生成 ID（时间戳-序列号格式）
    result, err := s.rdb.XAdd(ctx, &redis.XAddArgs{
        Stream: streamKey,
        MaxLen: s.maxLen,
        Approx: true,
        Values: map[string]interface{}{"event": string(eventJSON)},
    }).Result()
    
    return result, err
}
```

2. **Memory 实现** (用于测试和开发):
```go
// internal/event/memory_store.go
type MemoryEventStore struct {
    streams   map[string][]*Event // subscriberID -> events
    mu        sync.RWMutex
    maxEvents int // 1000
}

func (s *MemoryEventStore) Publish(ctx context.Context, subscriberID string, event *Event) (string, error) {
    s.mu.Lock()
    defer s.mu.Unlock()
    
    eventID := fmt.Sprintf("%d-%d", time.Now().UnixMilli(), rand.Int63())
    event.ID = eventID
    
    if _, exists := s.streams[subscriberID]; !exists {
        s.streams[subscriberID] = make([]*Event, 0, s.maxEvents)
    }
    
    s.streams[subscriberID] = append(s.streams[subscriberID], event)
    
    // 限制最大长度
    if len(s.streams[subscriberID]) > s.maxEvents {
        s.streams[subscriberID] = s.streams[subscriberID][1:]
    }
    
    return eventID, nil
}
```

**使用示例 - Controller 端 (发布事件)**:

```go
import (
    "context"
    "github.com/houzhh15/sdp-common/tunnel"
    "github.com/houzhh15/sdp-common/logging"
)

// 1. 创建 EventStore 实现（假设在 internal/event 包中）
eventStore := event.NewRedisEventStore(&event.RedisEventStoreConfig{
    RedisClient:  redisClient,
    Logger:       logger,
    StreamPrefix: "events:",
    MaxLen:       1000,
    TTL:          24 * time.Hour,
})

// 2. 创建事件管理器（可选的便捷层）
type EventManager struct {
    store  tunnel.EventStore
    logger logging.Logger
}

func (m *EventManager) PublishTunnelCreated(ctx context.Context, agentID, tunnelID, serviceID string) (string, error) {
    event, err := tunnel.NewEvent(tunnel.EventTypeTunnelCreated, &tunnel.TunnelEventData{
        Action: "created",
        Tunnel: &tunnel.TunnelInfo{
            ID:        tunnelID,
            ServiceID: serviceID,
            Status:    "active",
        },
    })
    if err != nil {
        return "", err
    }
    
    return m.store.Publish(ctx, agentID, event)
}

// 3. 在隧道创建业务逻辑中发布事件
eventID, err := eventManager.PublishTunnelCreated(ctx, "ah-agent-001", "tunnel-123", "postgres-db")
if err != nil {
    logger.Error("Failed to publish event", "error", err)
    // 可选：降级到旧的内存 SSE 方式
}

logger.Info("Event published", "event_id", eventID, "agent_id", "ah-agent-001")
```

**使用示例 - Controller 端 (SSE Handler 集成)**:

```go
// SSE Handler 支持 Last-Event-ID 重连恢复
func (h *Handler) SSEEventsHandler(c *gin.Context) {
    agentID := c.Query("agent_id")
    lastEventID := c.Request.Header.Get("Last-Event-ID")
    
    // 设置 SSE headers
    c.Header("Content-Type", "text/event-stream")
    c.Header("Cache-Control", "no-cache")
    c.Header("Connection", "keep-alive")
    
    flusher := c.Writer.(http.Flusher)
    
    // 1. 推送历史事件（如果有 Last-Event-ID）
    if lastEventID != "" {
        missedEvents, err := h.eventStore.GetEventsAfter(
            c.Request.Context(),
            agentID,
            lastEventID,
            100, // 最多 100 条
        )
        
        if err == nil {
            h.logger.Info("Pushing missed events",
                "agent_id", agentID,
                "last_event_id", lastEventID,
                "count", len(missedEvents))
            
            for _, event := range missedEvents {
                fmt.Fprintf(c.Writer, "id: %s\n", event.ID)
                fmt.Fprintf(c.Writer, "event: %s\n", event.Type)
                fmt.Fprintf(c.Writer, "data: %s\n\n", string(event.Data))
                flusher.Flush()
            }
        }
    }
    
    // 2. 订阅实时事件流
    eventCh, err := h.eventStore.Subscribe(
        c.Request.Context(),
        agentID,
        lastEventID,
    )
    if err != nil {
        c.String(500, "Subscribe failed: %v", err)
        return
    }
    
    // 3. 推送实时事件
    for event := range eventCh {
        fmt.Fprintf(c.Writer, "id: %s\n", event.ID)
        fmt.Fprintf(c.Writer, "event: %s\n", event.Type)
        fmt.Fprintf(c.Writer, "data: %s\n\n", string(event.Data))
        flusher.Flush()
    }
}
```

**使用示例 - AH Agent 端 (客户端追踪事件 ID)**:

```go
// AH Agent 的 Agent 结构体
type Agent struct {
    lastEventID   string
    eventIDMutex  sync.RWMutex
    eventCache    *lru.Cache // 事件去重缓存
    // ... 其他字段
}

// SSE 客户端重连时发送 Last-Event-ID
func (a *Agent) connectSSE() error {
    url := fmt.Sprintf("%s/api/v1/events?agent_id=%s", a.controllerURL, a.agentID)
    req, _ := http.NewRequest("GET", url, nil)
    
    // 读取最后的事件 ID
    a.eventIDMutex.RLock()
    if a.lastEventID != "" {
        req.Header.Set("Last-Event-ID", a.lastEventID)
        a.logger.Info("Reconnecting with last event ID", "last_event_id", a.lastEventID)
    }
    a.eventIDMutex.RUnlock()
    
    resp, err := a.httpClient.Do(req)
    if err != nil {
        return err
    }
    
    // 处理 SSE 事件流
    go a.handleSSEEvents(resp.Body)
    return nil
}

// 处理接收到的事件
func (a *Agent) handleSSEEvent(event *tunnel.Event) {
    // 事件去重检查
    if a.eventCache.Contains(event.ID) {
        a.logger.Debug("Duplicate event, skipping", "event_id", event.ID)
        return
    }
    a.eventCache.Add(event.ID, true)
    
    // 保存事件 ID
    a.eventIDMutex.Lock()
    a.lastEventID = event.ID
    a.eventIDMutex.Unlock()
    
    // 处理事件
    switch event.Type {
    case tunnel.EventTypeTunnelCreated:
        var tunnelData tunnel.TunnelEventData
        if err := event.ParseData(&tunnelData); err == nil {
            a.createTunnel(tunnelData.Tunnel)
        }
    case tunnel.EventTypeServiceUpdated:
        var serviceData tunnel.ServiceEventData
        if err := event.ParseData(&serviceData); err == nil {
            a.updateService(serviceData.ServiceID, serviceData.Config)
        }
    }
}
```

**性能考虑**:

| 存储实现 | 写入延迟 | 查询延迟 | 内存占用 | 适用场景 |
|---------|---------|---------|---------|---------|
| Redis Stream | < 5ms | < 10ms | 低（自动删除旧事件） | 生产环境 |
| Kafka | < 10ms | < 20ms | 中 | 高吞吐量场景 |
| PostgreSQL | < 50ms | < 100ms | 高 | 需要复杂查询 |
| Memory | < 1ms | < 1ms | 高（无持久化） | 开发/测试 |

**架构演进**:

EventStore 接口设计支持未来的协议演进：

```
Phase 1: SSE + EventStore（当前）
         SSE Handler 使用 EventStore 实现持久化

Phase 2: WebSocket + EventStore
         WebSocket Handler 复用相同的 EventStore

Phase 3: gRPC Stream + EventStore
         gRPC Service 复用相同的 EventStore
```

**相关文档**:
- [SSE 标准 (RFC 6455)](https://html.spec.whatwg.org/multipage/server-sent-events.html)
- [Redis Stream 文档](https://redis.io/docs/data-types/streams/)
- 项目文档: `docs/EVENT_MANAGEMENT_ARCHITECTURE.md`

---

## 6. logging - 日志审计包

### 6.1 Logger - 日志记录器接口

**功能**: 结构化日志记录，支持多种输出格式

**接口定义**:

```go
type Logger interface {
    Info(msg string, fields ...interface{})
    Warn(msg string, fields ...interface{})
    Error(msg string, fields ...interface{})
    Debug(msg string, fields ...interface{})
}

// Config - 日志配置
type Config struct {
    Level  string  // debug, info, warn, error
    Format string  // json, text
    Output string  // stdout, file
}
```

**使用示例**:

```go
// 创建日志记录器
logger, err := logging.NewLogger(&logging.Config{
    Level:  "info",
    Format: "json",
    Output: "stdout",
})

// 记录日志
logger.Info("服务启动", "version", "1.0.0", "port", 8443)
logger.Warn("证书即将过期", "days_remaining", 15)
logger.Error("连接失败", "error", err, "host", "192.168.1.100")
logger.Debug("调试信息", "data", debugData)
```

---

### 6.2 AuditLogger - 审计日志接口

**功能**: 记录访问、连接、安全事件，支持审计日志查询

**接口定义**:

```go
type AuditLogger interface {
    LogAccess(ctx context.Context, event *AccessEvent) error
    LogConnection(ctx context.Context, event *ConnectionEvent) error
    LogSecurity(ctx context.Context, event *SecurityEvent) error
    Query(ctx context.Context, filter *AuditFilter) ([]*AuditLog, error)
}
```

**数据结构**:

```go
// AccessEvent - 访问事件
type AccessEvent struct {
    Timestamp  time.Time
    ClientID   string
    ServiceID  string
    SourceIP   string
    Action     string  // handshake, policy_query, tunnel_create
    Result     string  // success, denied
    Reason     string
}

// ConnectionEvent - 连接事件
type ConnectionEvent struct {
    Timestamp  time.Time
    TunnelID   string
    ClientID   string
    ServiceID  string
    IHEndpoint string
    AHEndpoint string
    Action     string  // open, close, error
}

// SecurityEvent - 安全事件
type SecurityEvent struct {
    Timestamp time.Time
    ClientID  string
    EventType string  // cert_invalid, session_expired, device_noncompliant
    Severity  string  // low, medium, high, critical
    Details   map[string]interface{}
}
```

**使用示例**:

```go
// 创建审计日志
auditLogger := logging.NewFileAuditLogger("audit.log", logger)

// 记录访问事件
err := auditLogger.LogAccess(ctx, &logging.AccessEvent{
    Timestamp: time.Now(),
    ClientID:  "ih-001",
    ServiceID: "postgres-db",
    SourceIP:  "192.168.1.100",
    Action:    "tunnel_create",
    Result:    "success",
})

// 记录连接事件
err := auditLogger.LogConnection(ctx, &logging.ConnectionEvent{
    Timestamp:  time.Now(),
    TunnelID:   "tunnel-123",
    ClientID:   "ih-001",
    ServiceID:  "postgres-db",
    IHEndpoint: "192.168.1.100:15432",
    AHEndpoint: "192.168.1.200:5432",
    Action:     "open",
})

// 记录安全事件
err := auditLogger.LogSecurity(ctx, &logging.SecurityEvent{
    Timestamp: time.Now(),
    ClientID:  "ih-002",
    EventType: logging.EventTypeCertInvalid,
    Severity:  "high",
    Details: map[string]interface{}{
        "cert_fingerprint": "sha256:1234...",
        "reason":           "certificate expired",
    },
})

// 查询审计日志
logs, err := auditLogger.Query(ctx, &logging.AuditFilter{
    StartTime: time.Now().Add(-24 * time.Hour),
    EndTime:   time.Now(),
    ClientID:  "ih-001",
    EventType: "tunnel_create",
    Limit:     100,
})
```

---

## 7. transport - 传输层包

### 7.1 HTTPServer - HTTP REST 服务器

**功能**: HTTP REST API 服务器（控制平面默认）

**接口定义**:

```go
type HTTPServer interface {
    Start(addr string, handler http.Handler) error
    Stop() error
    RegisterMiddleware(mw func(http.Handler) http.Handler)
}
```

**使用示例**:

```go
// 创建 HTTP 服务器
server := transport.NewHTTPServer(tlsConfig)

// 注册中间件
server.RegisterMiddleware(loggingMiddleware)
server.RegisterMiddleware(authMiddleware)

// 创建路由处理器
mux := http.NewServeMux()
mux.HandleFunc("/api/v1/handshake", handshakeHandler)
mux.HandleFunc("/api/v1/policies", policiesHandler)

// 启动服务器
go server.Start(":8443", mux)

// 停止服务器
server.Stop()
```

---

### 7.2 SSE 推送功能

> **📌 架构说明**: SSE (Server-Sent Events) 功能已**集成在 `tunnel.Notifier` 中**，`transport` 包中**不提供独立的 `SSEServer` 接口**。这是为了简化架构，将 SSE 推送与隧道通知紧密结合。

**推荐使用方式**: 直接使用 `tunnel.Notifier`

**功能**: Server-Sent Events 长连接管理（实时隧道通知）

**使用示例**:

```go
import (
    "github.com/houzhh15/sdp-common/tunnel"
    "github.com/houzhh15/sdp-common/logging"
    "net/http"
    "time"
)

// 1. 创建 Notifier（内置 SSE 支持）
logger := logging.NewLogger(&logging.Config{Level: "info"})
notifier := tunnel.NewNotifier(logger, 30*time.Second)

// 2. 在 HTTP Handler 中订阅 SSE
http.HandleFunc("/api/v1/events", func(w http.ResponseWriter, r *http.Request) {
    agentID := r.Header.Get("X-Agent-ID")
    
    // Subscribe 方法会自动设置 SSE 响应头并保持长连接
    if err := notifier.Subscribe(agentID, w); err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
})

// 3. 推送事件给特定客户端
event := &tunnel.TunnelEvent{
    Type:      "tunnel_created",
    TunnelID:  "tunnel-123",
    Timestamp: time.Now(),
    Data: map[string]interface{}{
        "target_host": "192.168.1.10",
        "target_port": 5432,
    },
}

if err := notifier.Notify("agent-001", event); err != nil {
    log.Printf("推送失败: %v", err)
}

// 4. 广播事件给所有客户端
if err := notifier.Broadcast(event); err != nil {
    log.Printf("广播失败: %v", err)
}
```

**与独立 SSEServer 的对比**:

| 特性 | tunnel.Notifier (推荐) | 独立 SSEServer (已弃用) |
|------|----------------------|----------------------|
| 接口位置 | `tunnel` 包 | `transport` 包 |
| 事件类型 | 隧道专用事件 | 通用事件 |
| 架构复杂度 | ✅ 简单 | ❌ 复杂 |
| 性能 | ✅ 高效 | ⚠️ 一般 |
| 维护成本 | ✅ 低 | ❌ 高 |

**技术细节**:

`tunnel.Notifier` 内部实现了完整的 SSE 协议：
- 自动设置 `Content-Type: text/event-stream`
- 支持心跳保持连接（默认30秒）
- 自动处理客户端断开
- 支持单播和广播

**迁移指南** (如果您之前使用了 transport.SSEServer):

```go
// 旧代码
sseServer := transport.NewSSEServer(logger)
sseServer.Subscribe(clientID, w)
sseServer.Broadcast(event)

// 新代码
notifier := tunnel.NewNotifier(logger, 30*time.Second)
notifier.Subscribe(agentID, w)
notifier.Broadcast(tunnelEvent)
```

---

### 7.3 TCPProxyServer - TCP 单向代理服务器

> ⚠️ **使用场景限制**: 此服务器仅适用于 IH/AH 客户端直接连接目标应用的场景（Client → Proxy → Target）  
> **不适用于**: Controller 数据平面中继（应使用 `TunnelRelayServer`）

**功能**: TCP 单向透明代理，从 TunnelStore 查询目标地址并转发

**适用场景**:
- ✅ IH Client 本地代理转发到内网目标
- ✅ AH Agent 接收隧道数据后转发到目标应用
- ❌ Controller 数据平面（错误：会导致 IH → Controller → Target 的错误流向）

**接口定义**:

```go
type TCPProxyServer interface {
    // Start 启动 TCP 代理监听（不推荐：无 TLS）
    // Deprecated: Use StartTLS for production
    Start(addr string) error
    
    // StartTLS 启动 mTLS TCP 代理监听（推荐）
    StartTLS(addr string, tlsConfig *tls.Config) error
    
    // Stop 停止代理服务器
    Stop() error
    
    // HandleConnection 处理单个客户端连接
    HandleConnection(conn net.Conn) error
}
```

**使用示例（IH/AH 客户端场景）**:

```go
// 创建隧道存储适配器
tunnelStore := &MyTunnelStore{} // 实现 transport.TunnelStore 接口

// 创建 TCP 代理服务器
proxyServer := transport.NewTCPProxyServer(tunnelStore, logger, &transport.TCPProxyConfig{
    BufferSize:     32 * 1024,        // 32KB 缓冲区
    ConnectTimeout: 5 * time.Second,  // 5秒连接超时
    ReadTimeout:    30 * time.Second, // 30秒读超时
    WriteTimeout:   30 * time.Second, // 30秒写超时
    MaxConnections: 10000,            // 最大10000连接
})

// 启动代理（带 mTLS）
tlsConfig := certManager.GetTLSConfig()
go proxyServer.StartTLS(":9443", tlsConfig)

// 停止代理
proxyServer.Stop()
```

**错误使用示例（Controller 不应使用）**:

```go
// ❌ 错误：Controller 使用 TCPProxyServer
// 这会导致 IH → Controller → Target 的错误流向（跳过了 AH）
controller.dataPlane = transport.NewTCPProxyServer(...) // 不要这样做！

// ✅ 正确：Controller 应使用 TunnelRelayServer
controller.relayServer = transport.NewTunnelRelayServer(...) // 正确方式
```

---

### 7.4 TunnelRelayServer - Controller 数据平面中继服务器

> ✅ **Controller 专用**: 此服务器专为 Controller 数据平面设计，实现 IH ↔ Controller ↔ AH 的双向中继

**功能**: 配对 IH 和 AH 连接，实现零拷贝双向数据转发

**核心特性**:
- 通过 TunnelID 配对 IH 和 AH 连接
- 使用 io.Copy 零拷贝双向转发
- 配对超时自动清理（默认 30 秒）
- mTLS 强制认证
- 支持 10,000+ 并发隧道

**接口定义**:

```go
type TunnelRelayServer interface {
    // StartTLS 启动 mTLS 监听（强制要求 mTLS）
    StartTLS(addr string, tlsConfig *tls.Config) error
    
    // Stop 停止服务器
    Stop() error
    
    // GetStats 获取统计信息
    GetStats() *RelayStats
}

// RelayStats 中继统计信息
type RelayStats struct {
    ActiveTunnels      int    // 活跃隧道数
    PendingConnections int    // 待配对连接数
    TotalRelayed       uint64 // 总转发字节数
    ErrorCount         int    // 错误计数
}
```

**使用示例（Controller 数据平面）**:

```go
// 创建 TunnelRelayServer
relayServer := transport.NewTunnelRelayServer(logger, &transport.TunnelRelayConfig{
    PairingTimeout: 30 * time.Second,  // 配对超时
    BufferSize:     32 * 1024,         // 32KB 缓冲区
    ReadTimeout:    300 * time.Second, // 5分钟读超时
    WriteTimeout:   300 * time.Second, // 5分钟写超时
    MaxConnections: 10000,             // 最大并发连接
})

// 启动中继服务器（强制 mTLS）
tlsConfig := certManager.GetTLSConfig()
tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert // 强制客户端证书

go func() {
    if err := relayServer.StartTLS(":9443", tlsConfig); err != nil {
        log.Fatalf("Relay server error: %v", err)
    }
}()

// 查询统计信息
stats := relayServer.GetStats()
log.Printf("Active tunnels: %d, Pending: %d, Total relayed: %d bytes",
    stats.ActiveTunnels, stats.PendingConnections, stats.TotalRelayed)

// 停止服务器
relayServer.Stop()
```

**数据流程说明**:

```
1. IH Client → Controller:9443 (发送 TunnelID "550e8400-...")
2. AH Agent → Controller:9443 (发送相同 TunnelID "550e8400-...")
3. Controller 配对两个连接
4. Controller 双向转发：
   - IH 数据 → AH (io.Copy)
   - AH 数据 → IH (io.Copy)
```

**与 TCPProxyServer 的对比**:

| 特性 | TCPProxyServer | TunnelRelayServer |
|------|---------------|-------------------|
| **使用场景** | IH/AH 客户端 → 目标应用 | Controller 数据平面中继 |
| **数据流向** | Client → Proxy → Target（单向） | IH ↔ Controller ↔ AH（双向） |
| **连接配对** | 无需配对 | 通过 TunnelID 配对 |
| **目标地址** | 从 TunnelStore 查询 | 不查询（直接转发） |
| **适用组件** | IH Client, AH Agent | Controller |

---

### 7.5 GRPCServer - gRPC 服务器（可选）

### 7.4 GRPCServer - gRPC 服务器（可选）

**功能**: gRPC 服务器（控制平面可选）

**接口定义**:

```go
type GRPCServer interface {
    Start(addr string) error
    Stop() error
    RegisterService(desc *grpc.ServiceDesc, impl interface{})
}
```

**使用示例**:

```go
// 创建 gRPC 服务器
grpcServer := transport.NewGRPCServer(tlsConfig)

// 注册 gRPC 服务
grpcServer.RegisterService(&pb.ControlPlane_ServiceDesc, controlPlaneImpl)

// 启动服务器
go grpcServer.Start(":8443")

// 停止服务器
grpcServer.Stop()
```

---

## 8. protocol - 协议定义包

### 8.1 错误码定义

**功能**: 统一错误码和错误消息格式

**错误码常量**:

```go
const (
    // 成功
    ErrCodeSuccess = 0
    
    // 认证错误 (401xx)
    ErrCodeUnauthorized    = 40100  // 未授权
    ErrCodeInvalidCert     = 40101  // 证书无效
    ErrCodeSessionExpired  = 40102  // 会话过期
    
    // 授权错误 (403xx)
    ErrCodeNoPolicy        = 40301  // 无授权策略
    
    // 资源错误 (404xx)
    ErrCodeServiceNotFound = 40401  // 服务不存在
    
    // 限流错误 (409xx)
    ErrCodeConcurrencyLimit = 40901 // 并发限制
    
    // 服务错误 (503xx)
    ErrCodeServiceUnavail  = 50301  // 服务不可用
)
```

**Error 结构**:

```go
type Error struct {
    Code    int                    // 错误码
    Message string                 // 错误消息
    Details map[string]interface{} // 详细信息
}

func (e *Error) Error() string {
    return fmt.Sprintf("[%d] %s", e.Code, e.Message)
}
```

**使用示例**:

```go
// 创建错误
err := protocol.NewError(protocol.ErrCodeInvalidCert, "证书已过期")

// 包装错误
err := protocol.WrapError(protocol.ErrCodeServiceUnavail, originalErr)

// 添加详细信息
err.WithDetails("cert_fingerprint", fingerprint)
err.WithDetails("expires_at", cert.NotAfter)

// 错误处理
if protocolErr, ok := err.(*protocol.Error); ok {
    switch protocolErr.Code {
    case protocol.ErrCodeSessionExpired:
        // 重新认证
        reauth()
    case protocol.ErrCodeNoPolicy:
        // 请求授权
        requestAccess()
    default:
        // 通用错误处理
        log.Printf("错误: %v", protocolErr)
    }
}
```

---

### 8.2 消息类型定义

**功能**: 统一消息类型常量

```go
const (
    MsgTypeHandshakeReq  = "handshake_request"
    MsgTypeHandshakeResp = "handshake_response"
    MsgTypePolicyReq     = "policy_request"
    MsgTypePolicyResp    = "policy_response"
    MsgTypeTunnelReq     = "tunnel_request"
    MsgTypeTunnelResp    = "tunnel_response"
    MsgTypeHeartbeat     = "heartbeat"
)
```

---

## 9. config - 配置管理包

### 9.1 Config - 配置结构

**功能**: 统一配置结构，支持 YAML/JSON 加载

**配置结构**:

```go
type Config struct {
    Component ComponentConfig `yaml:"component"`
    TLS       TLSConfig       `yaml:"tls"`
    Auth      AuthConfig      `yaml:"auth"`
    Policy    PolicyConfig    `yaml:"policy"`
    Logging   LoggingConfig   `yaml:"logging"`
    Transport TransportConfig `yaml:"transport"`
}

type ComponentConfig struct {
    Type    string `yaml:"type"`     // controller, ih, ah
    ID      string `yaml:"id"`
    Name    string `yaml:"name"`
    Version string `yaml:"version"`
}

type TLSConfig struct {
    CertFile string `yaml:"cert_file"`
    KeyFile  string `yaml:"key_file"`
    CAFile   string `yaml:"ca_file"`
}

type AuthConfig struct {
    TokenTTL         time.Duration `yaml:"token_ttl"`
    DeviceValidation bool          `yaml:"device_validation"`
    MFARequired      bool          `yaml:"mfa_required"`
}

type PolicyConfig struct {
    Engine   string `yaml:"engine"`    // embedded, external
    Endpoint string `yaml:"endpoint"`  // 外部策略引擎地址
}

type LoggingConfig struct {
    Level     string `yaml:"level"`       // debug, info, warn, error
    Format    string `yaml:"format"`      // json, text
    Output    string `yaml:"output"`      // stdout, file
    AuditFile string `yaml:"audit_file"`
}

type TransportConfig struct {
    HTTPAddr     string        `yaml:"http_addr"`
    GRPCAddr     string        `yaml:"grpc_addr"`
    TCPProxyAddr string        `yaml:"tcp_proxy_addr"`
    SSEHeartbeat time.Duration `yaml:"sse_heartbeat"`
    EnableGRPC   bool          `yaml:"enable_grpc"`
}
```

---

### 9.2 Loader - 配置加载器

**功能**: 加载和验证配置文件

**接口定义**:

```go
type Loader struct{}

func NewLoader() *Loader
func (l *Loader) Load(path string) (*Config, error)
func (l *Loader) Validate(config *Config) error
func (l *Loader) Watch(callback func(*Config)) error  // 热重载
```

**YAML 配置示例**:

```yaml
component:
  type: controller
  id: ctrl-001
  name: SDP Controller
  version: 2.0.0

tls:
  cert_file: /etc/sdp/certs/controller-cert.pem
  key_file: /etc/sdp/certs/controller-key.pem
  ca_file: /etc/sdp/certs/ca-cert.pem

auth:
  token_ttl: 3600s
  device_validation: true
  mfa_required: false

policy:
  engine: embedded
  endpoint: ""

logging:
  level: info
  format: json
  output: stdout
  audit_file: /var/log/sdp/audit.log

transport:
  http_addr: ":8443"
  grpc_addr: ":8444"
  tcp_proxy_addr: ":9443"
  sse_heartbeat: 30s
  enable_grpc: false
```

**使用示例**:

```go
// 加载配置
loader := config.NewLoader()
cfg, err := loader.Load("config.yaml")
if err != nil {
    log.Fatal("加载配置失败:", err)
}

// 验证配置
if err := loader.Validate(cfg); err != nil {
    log.Fatal("配置验证失败:", err)
}

// 访问配置
fmt.Printf("组件类型: %s\n", cfg.Component.Type)
fmt.Printf("日志级别: %s\n", cfg.Logging.Level)
fmt.Printf("HTTP 地址: %s\n", cfg.Transport.HTTPAddr)

// 监听配置变化（热重载）
err = loader.Watch(func(newCfg *Config) {
    fmt.Println("配置已更新")
    // 应用新配置
    applyConfig(newCfg)
})
```

---

## 10. 身份验证与存储机制

### 10.1 身份验证流程

#### ClientID 提取机制

Controller 从客户端证书的 **Subject CommonName (CN)** 提取 ClientID：

```go
// examples/controller/main.go - extractClientID()
func extractClientID(cert *x509.Certificate) string {
    // 优先使用证书的 CommonName
    if cert.Subject.CommonName != "" {
        return cert.Subject.CommonName
    }
    // 回退方案：使用序列号生成 ID
    return fmt.Sprintf("client-%s", cert.SerialNumber.String())
}
```

**实际案例**：

```bash
# IH Client 证书
$ openssl x509 -in ih-client-cert.pem -noout -subject
subject=CN=ih-client, O=IH-Client

# Controller 提取的 ClientID
extractClientID(cert) → "ih-client"
```

#### 完整握手流程

```
1. IH Client 发起 mTLS 连接
   ↓
2. Controller 验证证书链
   ↓
3. 提取 ClientID = cert.Subject.CommonName
   ↓
4. 创建 Session
   sess := CreateSession(ClientID: "ih-client", Fingerprint: "sha256:...")
   ↓
5. 返回 Session Token
   Response: {"session_token": "abc123...", "expires_at": "2025-11-17T18:00:00Z"}
   ↓
6. IH Client 使用 Token 查询策略
   GET /api/v1/policies
   Authorization: Bearer abc123...
   ↓
7. Controller 验证 Token → 获取 Session → 提取 ClientID
   ValidateSession(token) → Session{ClientID: "ih-client"}
   ↓
8. 查询策略
   GetPoliciesForClient(ClientID: "ih-client")
   → 返回该客户端的授权策略列表
```

#### 关键点

| 阶段 | 关键组件 | 说明 |
|-----|---------|------|
| **证书验证** | `cert.Validator` | 验证证书链、有效期、吊销状态 |
| **ClientID 提取** | `extractClientID()` | 从 `cert.Subject.CommonName` 提取 |
| **Session 创建** | `session.Manager` | 生成 Token，关联 ClientID 和 Fingerprint |
| **策略查询** | `policy.Engine` | 根据 ClientID 查询授权策略 |

---

### 10.2 存储机制与持久化

#### DBStorage - 数据库存储（持久化）

```go
// policy/storage.go
type DBStorage struct {
    db *gorm.DB  // GORM 数据库连接
}

func NewDBStorage(db *gorm.DB) (*DBStorage, error) {
    // 自动迁移表结构
    if err := db.AutoMigrate(&policyDBModel{}); err != nil {
        return nil, err
    }
    return &DBStorage{db: db}, nil
}

// SavePolicy - 保存或更新策略
func (s *DBStorage) SavePolicy(ctx context.Context, policy *Policy) error {
    model := s.toDBModel(policy)
    // GORM Save() 语义：
    // - 如果记录存在（根据 primary key），则 UPDATE
    // - 如果记录不存在，则 INSERT
    result := s.db.WithContext(ctx).Save(model)
    return result.Error
}
```

**重要特性**：

1. **持久化**：数据写入 SQLite/MySQL/PostgreSQL，重启后不丢失
2. **唯一约束**：`policy_id` 设置为 `uniqueIndex`，防止重复插入
3. **Update 机制**：`Save()` 需要 primary key (`ID`)，否则会 INSERT 导致冲突

#### 策略更新冲突问题

**问题场景**：

```go
// Controller 启动时预置策略
seedExamplePolicies() {
    policy := &Policy{
        PolicyID: "policy-001",
        ClientID: "ih-client",  // 新值
        ServiceID: "demo-service-001",
    }
    
    // 检查是否存在
    existing, _ := storage.GetPolicy(ctx, "policy-001")
    if existing != nil {
        // 发现旧策略 ClientID = "ih-001"
        // 但直接 SavePolicy() 会失败：
        // UNIQUE constraint failed: policies.policy_id
    }
}
```

**解决方案**：先删除再创建

```go
if existing != nil && existing.ClientID != policy.ClientID {
    logger.Info("Updating policy with new ClientID",
        "old", existing.ClientID, "new", policy.ClientID)
    
    // 删除旧策略
    if err := storage.DeletePolicy(ctx, policy.PolicyID); err != nil {
        return err
    }
}

// 保存新策略
if err := storage.SavePolicy(ctx, policy); err != nil {
    return err
}
```

#### InMemoryStorage - 内存存储（非持久化）

```go
// 假设实现（sdp-common 未提供，需自行实现）
type InMemoryStorage struct {
    policies map[string]*Policy
    mu       sync.RWMutex
}

func (s *InMemoryStorage) SavePolicy(ctx context.Context, policy *Policy) error {
    s.mu.Lock()
    defer s.mu.Unlock()
    // 覆盖式更新，无 UNIQUE 约束
    s.policies[policy.PolicyID] = policy
    return nil
}
```

**特性对比**：

| 特性 | DBStorage (持久化) | InMemoryStorage (内存) |
|-----|-------------------|----------------------|
| **数据持久化** | ✅ 重启后保留 | ❌ 重启后丢失 |
| **更新冲突** | ⚠️ 需要显式 Delete+Save | ✅ 覆盖式更新 |
| **性能** | ~1ms (磁盘 I/O) | ~0.01ms (内存) |
| **适用场景** | 生产环境 | 开发/测试 |

---

### 10.3 常见问题排查

#### 问题 1：策略查询返回空列表

**症状**：
```json
{"level":"INFO","message":"Policies retrieved","fields":{"count":0}}
```

**根本原因**：
- 预置策略的 `ClientID` 与证书的 `CommonName` 不匹配

**排查步骤**：

```bash
# 1. 检查证书 CommonName
$ openssl x509 -in ih-client-cert.pem -noout -subject
subject=CN=ih-client, O=IH-Client

# 2. 检查 Controller 日志中的 Session 创建
[INFO] Session created map[client_id:ih-client token:abc123...]

# 3. 检查预置策略的 ClientID
// examples/controller/main.go - seedExamplePolicies()
ClientID: "ih-001",  ❌ 不匹配！应该是 "ih-client"
```

**解决方案**：

```go
// 修改预置策略的 ClientID
examplePolicy := &policy.Policy{
    PolicyID: "policy-001",
    ClientID: "ih-client",  // ✅ 匹配证书 CN
    ServiceID: "demo-service-001",
}
```

#### 问题 2：策略更新失败（UNIQUE constraint failed）

**症状**：
```
UNIQUE constraint failed: policies.policy_id
```

**原因**：
- DBStorage 使用 `Save()`，但缺少 primary key
- 尝试 INSERT 导致唯一索引冲突

**解决方案**：
```go
// 先删除旧策略
if err := storage.DeletePolicy(ctx, policyID); err != nil {
    return err
}
// 再保存新策略
if err := storage.SavePolicy(ctx, newPolicy); err != nil {
    return err
}
```

#### 问题 3：Controller 重启后旧策略仍存在

**原因**：
- DBStorage 持久化到 SQLite 文件（默认 `sdp.db`）
- 重启后自动加载旧数据

**解决方案**：

```bash
# 方案 1：删除数据库文件
$ rm sdp.db

# 方案 2：使用策略更新逻辑（推荐）
# 在 seedExamplePolicies() 中添加检查和更新逻辑
```

---

## 11. 快速参考表

### 11.1 核心接口速查

| 包 | 接口/类型 | 核心方法 | 用途 |
|----|-----------|----------|------|
| **cert** | `Manager` | `NewManager()`, `GetFingerprint()`, `ValidateExpiry()`, `GetTLSConfig()` | 证书加载与管理 |
| | `Registry` | `Register()`, `GetCertInfo()`, `Revoke()`, `Validate()` | 证书注册表 |
| | `Validator` | `ValidateCert()`, `CheckRevocation()` | 证书验证 |
| **session** | `Manager` | `CreateSession()`, `ValidateSession()`, `RefreshSession()`, `RevokeSession()` | 会话生命周期管理 |
| **policy** | `Engine` | `GetPoliciesForClient()`, `EvaluateAccess()`, `LoadPolicies()` | 策略引擎 |
| | `Storage` | `SavePolicy()`, `GetPolicy()`, `QueryPolicies()` | 策略存储 |
| | `Evaluator` | `Evaluate()` | 策略评估 |
| **tunnel** | `Manager` | `CreateTunnel()`, `GetTunnel()`, `DeleteTunnel()` | 隧道管理 |
| | `Notifier` | `Subscribe()`, `Notify()`, `NotifyOne()` | SSE 实时推送 |
| | `Subscriber` | `Start()`, `Stop()`, `Events()` | SSE 订阅客户端 |
| | `TCPProxy` | `HandleIHConnection()`, `HandleAHConnection()`, `GetActiveTunnels()` | TCP 透明代理 |
| | `Broker` | `RegisterEndpoint()`, `ForwardData()` | gRPC 流转发 |
| | `EventStore` | `Publish()`, `Subscribe()`, `GetEventsAfter()`, `Ack()`, `Close()` | 事件持久化存储 |
| | `Event` | `NewEvent()`, `ParseData()` | 通用事件结构 |
| **logging** | `Logger` | `Info()`, `Warn()`, `Error()`, `Debug()` | 日志记录 |
| | `AuditLogger` | `LogAccess()`, `LogConnection()`, `LogSecurity()` | 审计日志 |
| **transport** | `HTTPServer` | `Start()`, `Stop()`, `RegisterMiddleware()` | HTTP 服务器 |
| | `SSEServer` | `Subscribe()`, `Broadcast()` | SSE 推送服务器 |
| | `TCPProxyServer` | `Start()`, `HandleConnection()` | TCP 代理服务器 |
| | `GRPCServer` | `Start()`, `RegisterService()` | gRPC 服务器 |
| **protocol** | `Error` | `NewError()`, `WrapError()`, `WithDetails()` | 统一错误处理 |
| **config** | `Loader` | `Load()`, `Validate()`, `Watch()` | 配置加载 |

---

### 11.2 典型使用流程

#### Controller 初始化流程

```go
// 1. 加载配置
cfg, _ := config.NewLoader().Load("config.yaml")

// 2. 初始化日志
logger, _ := logging.NewLogger(&cfg.Logging)

// 3. 初始化证书
certMgr, _ := cert.NewManager(&cfg.TLS)
certRegistry, _ := cert.NewRegistry(db, logger)

// 4. 初始化会话管理
sessionMgr := session.NewManager(&session.Config{
    TokenTTL:        cfg.Auth.TokenTTL,
    CleanupInterval: 300 * time.Second,
}, logger)

// 5. 初始化策略引擎
storage := policy.NewDBStorage(db)
evaluator := &policy.DefaultEvaluator{}
policyEngine, _ := policy.NewEngine(&policy.Config{
    Storage:   storage,
    Evaluator: evaluator,
    Logger:    logger,
})

// 6. 初始化隧道管理
// 注意：tunnel.Manager 是接口，需要使用具体实现
// tunnelMgr := NewInMemoryTunnelManager(logger)  // 内存版本
// 或 tunnelMgr := NewDBTunnelManager(db, logger)  // 数据库版本

tunnelNotifier := tunnel.NewNotifier(logger, 30*time.Second)

// 7. 启动服务
httpServer := transport.NewHTTPServer(tlsConfig)
go httpServer.Start(":8443", mux)

tcpProxy := tunnel.NewTCPProxy(logger, 32*1024, 30*time.Second)
// TCP Proxy 需要在 IH 和 AH 两端分别启动
```

#### IH Client 初始化流程

```go
// 1. 加载证书
certMgr, _ := cert.NewManager(&cert.Config{
    CertFile: "ih-cert.pem",
    KeyFile:  "ih-key.pem",
    CAFile:   "ca-cert.pem",
})

// 2. 握手
fingerprint := certMgr.GetFingerprint()
resp := callHandshake(fingerprint)
sessionToken := resp.Token

// 3. 查询策略
policies := getPolicies(sessionToken)

// 4. 创建隧道
tunnel := createTunnel(sessionToken, "postgres-db")

// 5. 连接 TCP Proxy
conn, _ := tls.Dial("tcp", "controller:9443", certMgr.GetTLSConfig())
conn.Write([]byte(tunnel.ID))

// 6. 数据传输
io.Copy(conn, localConn)
```

#### AH Agent 初始化流程

```go
// 1. 加载证书
certMgr, _ := cert.NewManager(&cert.Config{
    CertFile: "ah-cert.pem",
    KeyFile:  "ah-key.pem",
    CAFile:   "ca-cert.pem",
})

// 2. 订阅隧道事件
subscriber := tunnel.NewSubscriber(&tunnel.SubscriberConfig{
    ControllerURL: "https://controller:8443",
    AgentID:       "ah-agent-001",
    TLSConfig:     certMgr.GetTLSConfig(),
    Callback:      handleTunnelEvent,
    Logger:        logger,
})

go subscriber.Start(ctx)

// 3. 处理隧道事件
for event := range subscriber.Events() {
    if event.Type == tunnel.EventTypeCreated {
        // 连接到目标服务
        targetConn, _ := net.Dial("tcp", event.Tunnel.TargetHost)
        
        // 连接到 Controller TCP Proxy
        proxyConn, _ := tls.Dial("tcp", "controller:9443", tlsConfig)
        proxyConn.Write([]byte(event.Tunnel.ID))
        
        // 双向转发
        go io.Copy(targetConn, proxyConn)
        go io.Copy(proxyConn, targetConn)
    }
}
```

---

### 11.3 性能指标

| 指标 | 目标值 | 实现方式 |
|------|--------|----------|
| 并发连接数 | ≥ 1000 | TCP Proxy + goroutine 池 |
| 握手延迟 | < 100ms | 证书缓存 |
| 实时通知延迟 | < 100ms | SSE 推送 |
| 数据平面延迟 | < 10ms (P99) | TCP Proxy 零拷贝 |
| 数据平面吞吐 | ≥ 900 Mbps | io.Copy 优化 |
| 内存占用 | < 500MB (1000连接) | 连接池复用 |

---

### 11.4 安全要求

- ✅ **mTLS 强制**: 所有组件间通信必须使用 mTLS
- ✅ **TLS 版本**: 最低 TLS 1.2，推荐 TLS 1.3
- ✅ **证书验证**: 验证有效期、吊销状态
- ✅ **Token 安全**: Session Token 加密存储，定期轮换
- ✅ **日志脱敏**: 敏感信息（密钥、Token）不写入日志

---

## 附录

### A. 依赖库

| 依赖 | 版本 | 用途 |
|------|------|------|
| `gorm.io/gorm` | latest | 数据库 ORM |
| `google.golang.org/grpc` | v1.50+ | gRPC 通信 |
| `google.golang.org/protobuf` | v1.28+ | Protobuf 序列化 |
| `gopkg.in/yaml.v3` | v3.0+ | YAML 配置解析 |

### B. 参考文档

- [SDP Specification v2.0](https://cloudsecurityalliance.org/)
- [NIST SP 800-207 - Zero Trust Architecture](https://csrc.nist.gov/publications/detail/sp/800-207/final)
- [RFC 8446 - TLS 1.3](https://www.rfc-editor.org/rfc/rfc8446)
- [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments)

### C. 术语表

| 术语 | 定义 |
|------|------|
| IH | Initiating Host，发起主机 |
| AH | Accepting Host，接受主机 |
| PDP | Policy Decision Point，策略决策点 |
| PEP | Policy Enforcement Point，策略执行点 |
| mTLS | Mutual TLS，双向 TLS 认证 |
| OCSP | Online Certificate Status Protocol，在线证书状态协议 |
| SSE | Server-Sent Events，服务器推送事件 |
| Last-Event-ID | SSE 标准头部字段，用于断线重连时恢复事件流 |
| Event Store | 事件存储，持久化事件用于重连恢复和审计 |
| Redis Stream | Redis 5.0+ 新增的流式数据结构，用于消息队列和事件存储 |

---

**文档版本**: v1.1  
**最后更新**: 2025-11-22  
**更新内容**: 新增 EventStore 事件持久化存储接口（5.8 节）  
**维护者**: SDP 开发团队
