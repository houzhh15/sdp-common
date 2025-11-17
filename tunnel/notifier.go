package tunnel

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/houzhh15/sdp-common/logging"
)

// SSEClient SSE客户端连接
type SSEClient struct {
	ID             string
	Writer         http.ResponseWriter
	Flusher        http.Flusher
	TunnelChannel  chan *TunnelEvent  // 隧道事件通道
	ServiceChannel chan *ServiceEvent // 服务配置事件通道
	Done           chan struct{}
	LastPing       time.Time
}

// Notifier SSE实时推送管理器
// 从 controller/internal/api/tunnel_notifier.go 提取并重构
// 支持混合方案：隧道事件（0x05）和服务配置事件（0x04）
type Notifier struct {
	clients   sync.Map // map[string]*SSEClient
	logger    logging.Logger
	heartbeat time.Duration
}

// NewNotifier 创建新的推送管理器
func NewNotifier(logger logging.Logger, heartbeat time.Duration) *Notifier {
	if heartbeat == 0 {
		heartbeat = 30 * time.Second
	}

	return &Notifier{
		logger:    logger,
		heartbeat: heartbeat,
	}
}

// Subscribe 处理客户端订阅
func (n *Notifier) Subscribe(agentID string, w http.ResponseWriter) error {
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
		ID:             agentID,
		Writer:         w,
		Flusher:        flusher,
		TunnelChannel:  make(chan *TunnelEvent, 10),  // 缓冲 10 个隧道事件
		ServiceChannel: make(chan *ServiceEvent, 10), // 缓冲 10 个服务事件
		Done:           make(chan struct{}),
		LastPing:       time.Now(),
	}

	// 存储客户端
	n.clients.Store(agentID, client)
	defer func() {
		n.clients.Delete(agentID)
		select {
		case <-client.Done:
			// Already closed
		default:
			close(client.Done)
		}
	}()

	n.logger.Info("SSE client connected", "agent_id", agentID)

	// 发送初始连接消息
	fmt.Fprintf(w, "event: connected\ndata: {\"agent_id\":\"%s\",\"timestamp\":%d}\n\n", agentID, time.Now().Unix())
	flusher.Flush()

	// 心跳 ticker
	ticker := time.NewTicker(n.heartbeat)
	defer ticker.Stop()

	// 事件循环
	for {
		select {
		case <-ticker.C:
			// 发送心跳
			fmt.Fprintf(w, ": ping\n\n")
			flusher.Flush()
			client.LastPing = time.Now()

		case event := <-client.TunnelChannel:
			// 发送隧道事件
			// Note: SSE event type must be "tunnel" for Subscriber compatibility
			if err := n.sendTunnelEvent(w, flusher, event); err != nil {
				n.logger.Error("Failed to send tunnel event", "agent_id", agentID, "error", err)
				return err
			}

		case event := <-client.ServiceChannel:
			// 发送服务配置事件
			if err := n.sendServiceEvent(w, flusher, event); err != nil {
				n.logger.Error("Failed to send service event", "agent_id", agentID, "error", err)
				return err
			}

		case <-client.Done:
			n.logger.Info("SSE client disconnected", "agent_id", agentID)
			return nil
		}
	}
}

// sendTunnelEvent 发送隧道事件到客户端
func (n *Notifier) sendTunnelEvent(w http.ResponseWriter, flusher http.Flusher, event *TunnelEvent) error {
	// 序列化完整的 TunnelEvent（包含 Type 和 Tunnel）
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal tunnel event: %w", err)
	}

	n.logger.Debug("Sending SSE tunnel event", "data_length", len(data), "event_type", event.Type)

	// SSE 格式：event: tunnel\ndata: <TunnelEvent JSON>\n\n
	fmt.Fprintf(w, "event: tunnel\ndata: %s\n\n", data)
	flusher.Flush()

	n.logger.Debug("SSE tunnel event sent", "event_type", event.Type)
	return nil
}

// sendServiceEvent 发送服务配置事件到客户端
func (n *Notifier) sendServiceEvent(w http.ResponseWriter, flusher http.Flusher, event *ServiceEvent) error {
	data, err := json.Marshal(event.Service)
	if err != nil {
		return fmt.Errorf("marshal service event: %w", err)
	}

	// SSE 格式：event: <type>\ndata: <json>\n\n
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Type, data)
	flusher.Flush()

	return nil
}

// Notify 广播隧道事件给所有订阅客户端
func (n *Notifier) Notify(event *TunnelEvent) error {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	count := 0
	n.clients.Range(func(key, value interface{}) bool {
		client := value.(*SSEClient)

		select {
		case client.TunnelChannel <- event:
			count++
			n.logger.Debug("Tunnel event sent to client",
				"agent_id", client.ID,
				"event_type", event.Type,
				"tunnel_id", event.Tunnel.ID,
			)
		case <-client.Done:
			// 客户端已断开
		default:
			// 通道已满，丢弃事件
			n.logger.Warn("SSE client tunnel channel full, dropping event",
				"agent_id", client.ID,
				"event_type", event.Type,
			)
		}

		return true
	})

	n.logger.Info("Tunnel event broadcasted",
		"event_type", event.Type,
		"tunnel_id", event.Tunnel.ID,
		"clients", count,
	)

	return nil
}

// NotifyService 广播服务配置事件给所有订阅客户端
func (n *Notifier) NotifyService(event *ServiceEvent) error {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	count := 0
	n.clients.Range(func(key, value interface{}) bool {
		client := value.(*SSEClient)

		select {
		case client.ServiceChannel <- event:
			count++
			n.logger.Debug("Service event sent to client",
				"agent_id", client.ID,
				"event_type", event.Type,
				"service_id", event.Service.ServiceID,
			)
		case <-client.Done:
			// 客户端已断开
		default:
			// 通道已满，丢弃事件
			n.logger.Warn("SSE client service channel full, dropping event",
				"agent_id", client.ID,
				"event_type", event.Type,
			)
		}

		return true
	})

	n.logger.Info("Service event broadcasted",
		"event_type", event.Type,
		"service_id", event.Service.ServiceID,
		"clients", count,
	)

	return nil
}

// NotifyOne 发送隧道事件给特定客户端
func (n *Notifier) NotifyOne(agentID string, event *TunnelEvent) error {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	value, ok := n.clients.Load(agentID)
	if !ok {
		return fmt.Errorf("client not found: %s", agentID)
	}

	client := value.(*SSEClient)

	select {
	case client.TunnelChannel <- event:
		n.logger.Debug("Tunnel event sent to client",
			"agent_id", agentID,
			"event_type", event.Type,
			"tunnel_id", event.Tunnel.ID,
		)
		return nil
	case <-client.Done:
		return fmt.Errorf("client disconnected: %s", agentID)
	default:
		return fmt.Errorf("client tunnel channel full: %s", agentID)
	}
}

// NotifyServiceOne 发送服务配置事件给特定客户端
func (n *Notifier) NotifyServiceOne(agentID string, event *ServiceEvent) error {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	value, ok := n.clients.Load(agentID)
	if !ok {
		return fmt.Errorf("client not found: %s", agentID)
	}

	client := value.(*SSEClient)

	select {
	case client.ServiceChannel <- event:
		n.logger.Debug("Service event sent to client",
			"agent_id", agentID,
			"event_type", event.Type,
			"service_id", event.Service.ServiceID,
		)
		return nil
	case <-client.Done:
		return fmt.Errorf("client disconnected: %s", agentID)
	default:
		return fmt.Errorf("client service channel full: %s", agentID)
	}
}

// GetClients 获取所有连接的客户端ID
func (n *Notifier) GetClients() []string {
	var clients []string

	n.clients.Range(func(key, value interface{}) bool {
		clients = append(clients, key.(string))
		return true
	})

	return clients
}

// Unsubscribe 取消订阅
func (n *Notifier) Unsubscribe(agentID string) {
	if value, ok := n.clients.LoadAndDelete(agentID); ok {
		client := value.(*SSEClient)
		close(client.Done)
		n.logger.Info("SSE client unsubscribed", "agent_id", agentID)
	}
}
