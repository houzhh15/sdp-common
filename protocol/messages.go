package protocol

// 消息类型常量
const (
	MsgTypeHandshakeReq  = "handshake_request"
	MsgTypeHandshakeResp = "handshake_response"
	MsgTypePolicyReq     = "policy_request"
	MsgTypePolicyResp    = "policy_response"
	MsgTypeTunnelReq     = "tunnel_request"
	MsgTypeTunnelResp    = "tunnel_response"
	MsgTypeHeartbeat     = "heartbeat"
)
