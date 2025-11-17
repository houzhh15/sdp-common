package transport

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/houzhh15/sdp-common/logging"
)

// SSEClient SSE 客户端连接
type SSEClient struct {
	ID       string
	Writer   http.ResponseWriter
	Flusher  http.Flusher
	Channel  chan *Event
	Done     chan struct{}
	LastPing time.Time
}

// sseServer SSE 推送服务器实现
// 基于 tunnel.Notifier 抽象，约 60% 复用率
type sseServer struct {
	clients   sync.Map // map[string]*SSEClient
	logger    logging.Logger
	heartbeat time.Duration
	stopChan  chan struct{}
	wg        sync.WaitGroup
}

// NewSSEServer 创建 SSE 服务器
func NewSSEServer(logger logging.Logger, heartbeat time.Duration) SSEServer {
	if heartbeat == 0 {
		heartbeat = 30 * time.Second
	}

	if logger == nil {
		logger = &noopLogger{}
	}

	return &sseServer{
		logger:    logger,
		heartbeat: heartbeat,
		stopChan:  make(chan struct{}),
	}
}

// Start 启动 SSE 服务器（当前为空，订阅时启动）
func (s *sseServer) Start() error {
	// SSE 服务器通过 HTTP Handler 集成，无需单独启动
	return nil
}

// Stop 停止 SSE 服务器
func (s *sseServer) Stop() error {
	close(s.stopChan)

	// 关闭所有客户端连接
	s.clients.Range(func(key, value interface{}) bool {
		client := value.(*SSEClient)
		select {
		case <-client.Done:
			// Already closed
		default:
			close(client.Done)
		}
		return true
	})

	s.wg.Wait()
	return nil
}

// Subscribe 处理客户端订阅（阻塞式，保持连接）
func (s *sseServer) Subscribe(clientID string, w http.ResponseWriter) error {
	// 设置 SSE 响应头
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("X-Accel-Buffering", "no") // 禁用 nginx 缓冲

	// 确保支持流式响应
	flusher, ok := w.(http.Flusher)
	if !ok {
		return fmt.Errorf("streaming not supported")
	}

	// 创建客户端
	client := &SSEClient{
		ID:       clientID,
		Writer:   w,
		Flusher:  flusher,
		Channel:  make(chan *Event, 10), // 缓冲 10 个事件
		Done:     make(chan struct{}),
		LastPing: time.Now(),
	}

	// 存储客户端
	s.clients.Store(clientID, client)
	defer func() {
		s.clients.Delete(clientID)
		select {
		case <-client.Done:
			// Already closed
		default:
			close(client.Done)
		}
	}()

	s.logger.Info("SSE client connected", "client_id", clientID)

	// 发送初始连接消息
	fmt.Fprintf(w, "event: connected\ndata: {\"client_id\":\"%s\",\"timestamp\":%d}\n\n",
		clientID, time.Now().Unix())
	flusher.Flush()

	// 心跳 ticker
	ticker := time.NewTicker(s.heartbeat)
	defer ticker.Stop()

	s.wg.Add(1)
	defer s.wg.Done()

	// 事件循环
	for {
		select {
		case <-ticker.C:
			// 发送心跳（SSE 注释格式）
			fmt.Fprintf(w, ": ping\n\n")
			flusher.Flush()
			client.LastPing = time.Now()

		case event := <-client.Channel:
			// 发送事件
			if err := s.sendEvent(w, flusher, event); err != nil {
				s.logger.Error("Failed to send event", "client_id", clientID, "error", err.Error())
				return err
			}

		case <-client.Done:
			s.logger.Info("SSE client disconnected", "client_id", clientID)
			return nil

		case <-s.stopChan:
			s.logger.Info("SSE server stopping", "client_id", clientID)
			return nil
		}
	}
}

// Broadcast 广播事件到所有客户端
func (s *sseServer) Broadcast(event *Event) error {
	s.clients.Range(func(key, value interface{}) bool {
		client := value.(*SSEClient)

		// 非阻塞发送
		select {
		case client.Channel <- event:
			// 发送成功
		default:
			s.logger.Warn("Client channel full, event dropped", "client_id", client.ID)
		}

		return true
	})

	return nil
}

// NotifyOne 单播事件到特定客户端
func (s *sseServer) NotifyOne(clientID string, event *Event) error {
	value, ok := s.clients.Load(clientID)
	if !ok {
		return fmt.Errorf("client not found: %s", clientID)
	}

	client := value.(*SSEClient)
	select {
	case client.Channel <- event:
		return nil
	default:
		return fmt.Errorf("client channel full: %s", clientID)
	}
}

// GetClients 获取所有订阅客户端 ID
func (s *sseServer) GetClients() []string {
	var clients []string
	s.clients.Range(func(key, value interface{}) bool {
		clients = append(clients, key.(string))
		return true
	})
	return clients
}

// sendEvent 发送 SSE 格式事件
// 格式：event: <type>\ndata: <json>\n\n
func (s *sseServer) sendEvent(w http.ResponseWriter, flusher http.Flusher, event *Event) error {
	data, err := json.Marshal(event.Data)
	if err != nil {
		return fmt.Errorf("failed to marshal event data: %w", err)
	}

	message := fmt.Sprintf("event: %s\ndata: %s\n\n", event.Type, data)

	if _, err := fmt.Fprint(w, message); err != nil {
		return err
	}

	flusher.Flush()
	return nil
}

// noopLogger 空日志实现
type noopLogger struct{}

func (l *noopLogger) Info(msg string, fields ...interface{})  {}
func (l *noopLogger) Warn(msg string, fields ...interface{})  {}
func (l *noopLogger) Error(msg string, fields ...interface{}) {}
func (l *noopLogger) Debug(msg string, fields ...interface{}) {}
