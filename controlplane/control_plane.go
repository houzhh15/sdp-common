package controlplane

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/houzhh15/sdp-common/cert"
	"github.com/houzhh15/sdp-common/logging"
	"github.com/houzhh15/sdp-common/session"
	"github.com/houzhh15/sdp-common/tunnel"
)

// AuthHandler 认证处理器接口（由 internal/auth 实现）
type AuthHandler interface {
	HandleHandshake(c *gin.Context)
	HandleRefresh(c *gin.Context)
	HandleRevoke(c *gin.Context)
}

// TunnelHandler 隧道处理器接口（由 internal/tunnel 实现）
type TunnelHandler interface {
	HandleTunnelRequest(c *gin.Context)
	HandleSSESubscribe(c *gin.Context)
}

// ServiceHandler 服务处理器接口（由 internal/service 实现）
type ServiceHandler interface {
	GetConfigsHandler(c *gin.Context)
	CreateConfigHandler(c *gin.Context)
	ServiceHeartbeatHandler(c *gin.Context)
	ReportRequestFailureHandler(c *gin.Context)
	SSEEventsHandler(c *gin.Context)
}

// ControlPlaneConfig 控制平面配置
type ControlPlaneConfig struct {
	Addr         string
	TLSConfig    *tls.Config
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
}

// ControlPlaneServer 控制平面服务器（8443 端口）
// 提供 SDP 标准的控制平面框架，业务逻辑由外部 Handler 实现
type ControlPlaneServer struct {
	config         *ControlPlaneConfig
	certManager    *cert.Manager
	sessionManager *session.Manager
	notifier       *tunnel.Notifier
	logger         logging.Logger

	// 业务处理器（由 internal 包实现）
	authHandler    AuthHandler
	tunnelHandler  TunnelHandler
	serviceHandler ServiceHandler

	// HTTP 服务器
	server *http.Server
	router *gin.Engine
}

// NewControlPlaneServer 创建控制平面服务器
func NewControlPlaneServer(
	config *ControlPlaneConfig,
	certManager *cert.Manager,
	sessionManager *session.Manager,
	notifier *tunnel.Notifier,
	logger logging.Logger,
) *ControlPlaneServer {
	if logger == nil {
		logger = &noopLogger{}
	}

	// 创建 Gin 路由器
	router := gin.New()
	router.Use(gin.Recovery())

	return &ControlPlaneServer{
		config:         config,
		certManager:    certManager,
		sessionManager: sessionManager,
		notifier:       notifier,
		logger:         logger,
		router:         router,
	}
}

// RegisterAuthHandler 注册认证处理器
func (s *ControlPlaneServer) RegisterAuthHandler(handler AuthHandler) {
	s.authHandler = handler
}

// RegisterTunnelHandler 注册隧道处理器
func (s *ControlPlaneServer) RegisterTunnelHandler(handler TunnelHandler) {
	s.tunnelHandler = handler
}

// RegisterServiceHandler 注册服务处理器
func (s *ControlPlaneServer) RegisterServiceHandler(handler ServiceHandler) {
	s.serviceHandler = handler
}

// setupRoutes 设置路由（SDP 标准 API）
func (s *ControlPlaneServer) setupRoutes() error {
	// 请求日志中间件
	s.router.Use(func(c *gin.Context) {
		start := time.Now()
		c.Next()
		s.logger.Info("Control plane request",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"duration_ms", time.Since(start).Milliseconds())
	})

	// 健康检查端点
	s.router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "plane": "control"})
	})

	// SDP 标准 API v1
	v1 := s.router.Group("/api/v1")

	// 认证端点（必须）
	if s.authHandler == nil {
		return fmt.Errorf("auth handler is required")
	}
	auth := v1.Group("/auth")
	{
		auth.POST("/handshake", s.authHandler.HandleHandshake)
		auth.POST("/refresh", s.authHandler.HandleRefresh)
		auth.POST("/revoke", s.authHandler.HandleRevoke)
	}
	s.logger.Info("Auth routes registered")

	// 隧道端点（必须）
	if s.tunnelHandler == nil {
		return fmt.Errorf("tunnel handler is required")
	}
	tunnel := v1.Group("/tunnel")
	{
		tunnel.POST("/request", s.tunnelHandler.HandleTunnelRequest)
	}
	v1.GET("/events/subscribe", s.tunnelHandler.HandleSSESubscribe)
	s.logger.Info("Tunnel routes registered")

	// 服务端点（可选，用于 AH Agent）
	if s.serviceHandler != nil {
		services := v1.Group("/services")
		{
			services.GET("", s.serviceHandler.GetConfigsHandler)
			services.POST("/register", s.serviceHandler.CreateConfigHandler)
			services.POST("/heartbeat", s.serviceHandler.ServiceHeartbeatHandler)
			services.POST("/:id/failure", s.serviceHandler.ReportRequestFailureHandler)
		}
		v1.GET("/events", s.serviceHandler.SSEEventsHandler)
		s.logger.Info("Service routes registered")
	}

	return nil
}

// Start 启动控制平面服务器
func (s *ControlPlaneServer) Start() error {
	// 设置路由
	if err := s.setupRoutes(); err != nil {
		return fmt.Errorf("setup routes: %w", err)
	}

	// 创建 HTTP 服务器
	s.server = &http.Server{
		Addr:         s.config.Addr,
		Handler:      s.router,
		TLSConfig:    s.config.TLSConfig,
		ReadTimeout:  s.config.ReadTimeout,
		WriteTimeout: s.config.WriteTimeout,
		IdleTimeout:  s.config.IdleTimeout,
	}

	s.logger.Info("Starting control plane server (mTLS)", "addr", s.config.Addr)

	// 启动服务器（阻塞）
	if err := s.server.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("control plane server failed: %w", err)
	}

	return nil
}

// StartAsync 异步启动控制平面服务器
func (s *ControlPlaneServer) StartAsync() error {
	// 设置路由
	if err := s.setupRoutes(); err != nil {
		return fmt.Errorf("setup routes: %w", err)
	}

	// 创建 HTTP 服务器
	s.server = &http.Server{
		Addr:         s.config.Addr,
		Handler:      s.router,
		TLSConfig:    s.config.TLSConfig,
		ReadTimeout:  s.config.ReadTimeout,
		WriteTimeout: s.config.WriteTimeout,
		IdleTimeout:  s.config.IdleTimeout,
	}

	s.logger.Info("Starting control plane server (mTLS)", "addr", s.config.Addr)

	// 异步启动
	go func() {
		if err := s.server.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
			s.logger.Error("Control plane server failed", "error", err)
		}
	}()

	return nil
}

// Stop 停止控制平面服务器
func (s *ControlPlaneServer) Stop(ctx context.Context) error {
	if s.server == nil {
		return nil
	}

	s.logger.Info("Stopping control plane server...")

	if err := s.server.Shutdown(ctx); err != nil {
		return fmt.Errorf("shutdown control plane server: %w", err)
	}

	s.logger.Info("Control plane server stopped")
	return nil
}

// GetRouter 获取底层路由器（用于高级定制）
func (s *ControlPlaneServer) GetRouter() *gin.Engine {
	return s.router
}

// noopLogger 空日志实现
type noopLogger struct{}

func (n *noopLogger) Debug(msg string, keysAndValues ...interface{}) {}
func (n *noopLogger) Info(msg string, keysAndValues ...interface{})  {}
func (n *noopLogger) Warn(msg string, keysAndValues ...interface{})  {}
func (n *noopLogger) Error(msg string, keysAndValues ...interface{}) {}
