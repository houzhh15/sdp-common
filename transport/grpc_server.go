package transport

import (
	"crypto/tls"
	"fmt"
	"net"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// grpcServer gRPC 服务器实现
// 约 75% 复用率
type grpcServer struct {
	server    *grpc.Server
	tlsConfig *tls.Config
	address   string
	listener  net.Listener
	mu        sync.RWMutex
	services  []serviceDesc
}

type serviceDesc struct {
	desc *grpc.ServiceDesc
	impl interface{}
}

// NewGRPCServer 创建 gRPC 服务器
func NewGRPCServer(tlsConfig *tls.Config) GRPCServer {
	return &grpcServer{
		tlsConfig: tlsConfig,
		services:  make([]serviceDesc, 0),
	}
}

// RegisterService 注册 gRPC 服务（启动前调用）
func (s *grpcServer) RegisterService(desc *grpc.ServiceDesc, impl interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.services = append(s.services, serviceDesc{
		desc: desc,
		impl: impl,
	})
}

// Start 启动 gRPC 服务器
func (s *grpcServer) Start(addr string) error {
	s.mu.Lock()

	// 创建监听器
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		s.mu.Unlock()
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}
	s.listener = lis
	s.address = addr

	// 配置 gRPC 服务器选项
	opts := []grpc.ServerOption{}

	// 添加 TLS 凭证（如果配置）
	if s.tlsConfig != nil {
		creds := credentials.NewTLS(s.tlsConfig)
		opts = append(opts, grpc.Creds(creds))
	}

	// 创建 gRPC 服务器
	s.server = grpc.NewServer(opts...)

	// 注册所有服务
	for _, svc := range s.services {
		s.server.RegisterService(svc.desc, svc.impl)
	}

	s.mu.Unlock()

	// 启动服务器（阻塞）
	if err := s.server.Serve(lis); err != nil {
		return fmt.Errorf("gRPC server failed: %w", err)
	}

	return nil
}

// Stop 停止 gRPC 服务器（优雅关闭）
func (s *grpcServer) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.server == nil {
		return nil
	}

	// 优雅停止（等待现有 RPC 完成）
	s.server.GracefulStop()

	return nil
}

// StopImmediately 立即停止 gRPC 服务器（强制断开）
func (s *grpcServer) StopImmediately() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.server == nil {
		return nil
	}

	s.server.Stop()

	return nil
}

// GetGRPCServer 获取底层 gRPC 服务器（用于高级操作）
func (s *grpcServer) GetGRPCServer() *grpc.Server {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.server
}
