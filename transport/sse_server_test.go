package transport

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewSSEServer(t *testing.T) {
	server := NewSSEServer(nil, 0)
	if server == nil {
		t.Fatal("NewSSEServer returned nil")
	}
}

func TestSSEServer_Subscribe(t *testing.T) {
	server := NewSSEServer(nil, 10*time.Second)

	// 使用 httptest.ResponseRecorder
	rec := httptest.NewRecorder()

	// 订阅（异步，因为是阻塞式）
	done := make(chan error)
	go func() {
		done <- server.Subscribe("client-1", rec)
	}()

	// 等待初始连接消息
	time.Sleep(100 * time.Millisecond)

	// 验证响应头
	headers := rec.Header()
	if headers.Get("Content-Type") != "text/event-stream" {
		t.Errorf("Expected Content-Type: text/event-stream, got %s", headers.Get("Content-Type"))
	}

	// 停止订阅
	server.Stop()

	select {
	case err := <-done:
		if err != nil {
			t.Logf("Subscribe returned: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Error("Subscribe timeout")
	}
}

func TestSSEServer_Broadcast(t *testing.T) {
	server := NewSSEServer(nil, 10*time.Second)

	// 创建 2 个客户端订阅
	rec1 := httptest.NewRecorder()
	rec2 := httptest.NewRecorder()

	go server.Subscribe("client-1", rec1)
	go server.Subscribe("client-2", rec2)

	// 等待连接建立
	time.Sleep(100 * time.Millisecond)

	// 广播事件
	event := &Event{
		Type: "test",
		Data: map[string]interface{}{
			"message": "hello",
		},
		Timestamp: time.Now(),
	}

	if err := server.Broadcast(event); err != nil {
		t.Fatalf("Failed to broadcast: %v", err)
	}

	// 验证客户端数量
	clients := server.(*sseServer).GetClients()
	if len(clients) != 2 {
		t.Errorf("Expected 2 clients, got %d", len(clients))
	}

	// 停止服务器
	server.Stop()
}

func TestSSEServer_NotifyOne(t *testing.T) {
	server := NewSSEServer(nil, 10*time.Second).(*sseServer)

	rec := httptest.NewRecorder()

	go server.Subscribe("client-1", rec)
	time.Sleep(100 * time.Millisecond)

	// 单播事件
	event := &Event{
		Type:      "notification",
		Data:      map[string]string{"status": "success"},
		Timestamp: time.Now(),
	}

	if err := server.NotifyOne("client-1", event); err != nil {
		t.Fatalf("Failed to notify one: %v", err)
	}

	// 尝试发送到不存在的客户端
	if err := server.NotifyOne("client-999", event); err == nil {
		t.Error("Expected error for non-existent client")
	}

	server.Stop()
}

func TestSSEServer_Heartbeat(t *testing.T) {
	// 短心跳间隔用于测试
	server := NewSSEServer(nil, 500*time.Millisecond)

	rec := httptest.NewRecorder()

	go server.Subscribe("client-1", rec)

	// 等待至少一次心跳
	time.Sleep(1 * time.Second)

	// 验证响应中包含心跳（": ping\n\n"）
	body := rec.Body.String()
	if len(body) == 0 {
		t.Error("Expected heartbeat in response body")
	}

	t.Logf("Response body (first 200 chars): %s", body[:min(200, len(body))])

	server.Stop()
}

func TestSSEServer_Stop(t *testing.T) {
	server := NewSSEServer(nil, 10*time.Second)

	rec := httptest.NewRecorder()
	done := make(chan error)

	go func() {
		done <- server.Subscribe("client-1", rec)
	}()

	time.Sleep(100 * time.Millisecond)

	// 停止服务器
	if err := server.Stop(); err != nil {
		t.Errorf("Failed to stop server: %v", err)
	}

	// 等待订阅退出
	select {
	case <-done:
		// OK
	case <-time.After(5 * time.Second):
		t.Error("Subscribe didn't exit after stop")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
