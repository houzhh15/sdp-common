package transport

import (
	"testing"
	"time"
)

func TestNewGRPCServer(t *testing.T) {
	server := NewGRPCServer(nil)
	if server == nil {
		t.Fatal("NewGRPCServer returned nil")
	}
}

func TestGRPCServer_RegisterService(t *testing.T) {
	server := NewGRPCServer(nil).(*grpcServer)

	// 注册一个空服务（仅测试接口）
	server.RegisterService(nil, nil)

	if len(server.services) != 1 {
		t.Errorf("Expected 1 service, got %d", len(server.services))
	}
}

func TestGRPCServer_Start(t *testing.T) {
	t.Skip("Skipping gRPC server start test: requires real service implementation")

	server := NewGRPCServer(nil)

	// 启动服务器
	go func() {
		if err := server.Start(":50051"); err != nil {
			t.Logf("Server stopped: %v", err)
		}
	}()

	time.Sleep(100 * time.Millisecond)

	// 停止服务器
	if err := server.Stop(); err != nil {
		t.Errorf("Failed to stop server: %v", err)
	}
}

func TestGRPCServer_Stop(t *testing.T) {
	server := NewGRPCServer(nil)

	// 停止未启动的服务器（应该不报错）
	if err := server.Stop(); err != nil {
		t.Errorf("Stop failed: %v", err)
	}
}

func TestGRPCServer_GetGRPCServer(t *testing.T) {
	server := NewGRPCServer(nil).(*grpcServer)

	// 未启动时应返回 nil
	grpcSvr := server.GetGRPCServer()
	if grpcSvr != nil {
		t.Error("Expected nil gRPC server before Start")
	}
}
