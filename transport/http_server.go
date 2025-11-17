package transport

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"
)

// httpServer HTTP/REST API 服务器实现
// 支持 mTLS、中间件链、优雅关闭
type httpServer struct {
	server      *http.Server
	tlsConfig   *tls.Config
	middlewares []func(http.Handler) http.Handler
	mu          sync.RWMutex
}

// NewHTTPServer 创建 HTTP 服务器
// tlsConfig 为 nil 则使用普通 HTTP（不推荐生产环境）
func NewHTTPServer(tlsConfig *tls.Config) HTTPServer {
	return &httpServer{
		tlsConfig:   tlsConfig,
		middlewares: make([]func(http.Handler) http.Handler, 0),
	}
}

// RegisterMiddleware 注册中间件（后进先出顺序执行）
func (s *httpServer) RegisterMiddleware(mw func(http.Handler) http.Handler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.middlewares = append(s.middlewares, mw)
}

// Start 启动 HTTP 服务器
func (s *httpServer) Start(addr string, handler http.Handler) error {
	s.mu.Lock()

	// 应用中间件链（反向顺序）
	finalHandler := handler
	for i := len(s.middlewares) - 1; i >= 0; i-- {
		finalHandler = s.middlewares[i](finalHandler)
	}

	// 创建 HTTP Server
	s.server = &http.Server{
		Addr:         addr,
		Handler:      finalHandler,
		TLSConfig:    s.tlsConfig,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	s.mu.Unlock()

	// 启动服务器
	var err error
	if s.tlsConfig != nil {
		// HTTPS with mTLS
		err = s.server.ListenAndServeTLS("", "") // 证书已在 tlsConfig 中配置
	} else {
		// HTTP (不推荐)
		err = s.server.ListenAndServe()
	}

	// ErrServerClosed 不是错误（正常关闭）
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

// Stop 优雅关闭服务器（等待现有连接完成）
func (s *httpServer) Stop() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.server == nil {
		return nil
	}

	// 5 秒超时
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return s.server.Shutdown(ctx)
}

// StopImmediately 立即关闭服务器（强制断开所有连接）
func (s *httpServer) StopImmediately() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.server == nil {
		return nil
	}

	return s.server.Close()
}

// GetListener 获取底层 Listener（用于测试）
func (s *httpServer) GetListener() (net.Listener, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.server == nil {
		return nil, fmt.Errorf("server not started")
	}

	// 注意：http.Server 不直接暴露 Listener，需要在 Start 前手动创建
	return nil, fmt.Errorf("not implemented: use custom listener")
}
