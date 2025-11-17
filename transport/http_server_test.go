package transport

import (
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewHTTPServer(t *testing.T) {
	server := NewHTTPServer(nil)
	if server == nil {
		t.Fatal("NewHTTPServer returned nil")
	}
}

func TestHTTPServer_RegisterMiddleware(t *testing.T) {
	server := NewHTTPServer(nil).(*httpServer)

	// 注册中间件
	middleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r)
		})
	}

	server.RegisterMiddleware(middleware)

	if len(server.middlewares) != 1 {
		t.Errorf("Expected 1 middleware, got %d", len(server.middlewares))
	}
}

func TestHTTPServer_Start_Plain(t *testing.T) {
	server := NewHTTPServer(nil)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Hello"))
	})

	// 启动服务器（异步）
	go func() {
		if err := server.Start(":18080", handler); err != nil {
			t.Logf("Server stopped: %v", err)
		}
	}()

	// 等待服务器启动
	time.Sleep(100 * time.Millisecond)

	// 发送请求
	resp, err := http.Get("http://localhost:18080/")
	if err != nil {
		t.Fatalf("Failed to send request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "Hello" {
		t.Errorf("Expected 'Hello', got '%s'", body)
	}

	// 停止服务器
	if err := server.Stop(); err != nil {
		t.Errorf("Failed to stop server: %v", err)
	}
}

func TestHTTPServer_Middleware(t *testing.T) {
	server := NewHTTPServer(nil)

	// 注册两个中间件
	var order []string

	middleware1 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "mw1-before")
			next.ServeHTTP(w, r)
			order = append(order, "mw1-after")
		})
	}

	middleware2 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "mw2-before")
			next.ServeHTTP(w, r)
			order = append(order, "mw2-after")
		})
	}

	server.RegisterMiddleware(middleware1)
	server.RegisterMiddleware(middleware2)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		order = append(order, "handler")
		w.Write([]byte("OK"))
	})

	// 使用 httptest
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()

	// 手动应用中间件链
	s := server.(*httpServer)
	var finalHandler http.Handler = handler
	for i := len(s.middlewares) - 1; i >= 0; i-- {
		finalHandler = s.middlewares[i](finalHandler)
	}

	finalHandler.ServeHTTP(rec, req)

	// 验证执行顺序（正确的顺序：后注册的先执行）
	expectedOrder := []string{
		"mw1-before", "mw2-before", "handler", "mw2-after", "mw1-after",
	}

	if len(order) != len(expectedOrder) {
		t.Fatalf("Expected %d calls, got %d", len(expectedOrder), len(order))
	}

	for i, expected := range expectedOrder {
		if order[i] != expected {
			t.Errorf("Call %d: expected %s, got %s", i, expected, order[i])
		}
	}
}

func TestHTTPServer_Stop(t *testing.T) {
	server := NewHTTPServer(nil)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.Write([]byte("Done"))
	})

	// 启动服务器
	go func() {
		server.Start(":18081", handler)
	}()

	time.Sleep(100 * time.Millisecond)

	// 停止服务器（优雅关闭）
	stopChan := make(chan error)
	go func() {
		stopChan <- server.Stop()
	}()

	select {
	case err := <-stopChan:
		if err != nil {
			t.Errorf("Failed to stop server: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Error("Server stop timeout")
	}
}

func TestHTTPServer_TLS(t *testing.T) {
	t.Skip("Skipping TLS test: requires valid certificates")

	// 这里需要真实的证书文件
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	server := NewHTTPServer(tlsConfig)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Secure"))
	})

	go func() {
		server.Start(":18443", handler)
	}()

	time.Sleep(100 * time.Millisecond)

	// 这里需要 HTTPS 客户端
	// ...

	server.Stop()
}
