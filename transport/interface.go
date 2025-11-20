package transport

import (
	"crypto/tls"
	"net"
	"net/http"

	"google.golang.org/grpc"
)

// HTTPServer HTTP/REST API 服务器（控制平面默认）
type HTTPServer interface {
	// Start 启动 HTTP 服务器
	Start(addr string, handler http.Handler) error
	// Stop 停止服务器
	Stop() error
	// RegisterMiddleware 注册中间件
	RegisterMiddleware(mw func(http.Handler) http.Handler)
}

// SSEServer SSE 推送服务器（实时通知默认）
type SSEServer interface {
	// Start 启动 SSE 服务器
	Start() error
	// Stop 停止服务器
	Stop() error
	// Subscribe 客户端订阅（阻塞式，保持连接）
	Subscribe(clientID string, w http.ResponseWriter) error
	// Broadcast 广播事件到所有客户端
	Broadcast(event *Event) error
}

// TCPProxyServer TCP 代理服务器
// 使用场景：IH/AH 客户端直接连接目标应用（Client → Proxy → Target）
// 不适用于 Controller 数据平面中继（应使用 TunnelRelayServer）
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

// GRPCServer gRPC 服务器（控制平面可选）
type GRPCServer interface {
	// Start 启动 gRPC 服务器
	Start(addr string) error
	// Stop 停止服务器
	Stop() error
	// RegisterService 注册 gRPC 服务
	RegisterService(desc *grpc.ServiceDesc, impl interface{})
}
