package protocol

import "fmt"

// 错误码常量
const (
	// 成功
	ErrCodeSuccess = 0

	// 认证错误 (401xx)
	ErrCodeUnauthorized   = 40100 // 未授权
	ErrCodeInvalidCert    = 40101 // 证书无效
	ErrCodeSessionExpired = 40102 // 会话过期

	// 授权错误 (403xx)
	ErrCodeNoPolicy = 40301 // 无授权策略

	// 资源错误 (404xx)
	ErrCodeNotFound        = 40400 // 资源不存在
	ErrCodeServiceNotFound = 40401 // 服务不存在

	// 请求错误 (400xx)
	ErrCodeInvalidRequest = 40000 // 无效请求

	// 限流错误 (409xx)
	ErrCodeConcurrencyLimit = 40901 // 并发限制

	// 服务错误 (503xx)
	ErrCodeServiceUnavail = 50301 // 服务不可用
)

// Error SDP 协议错误
type Error struct {
	Code    int                    `json:"code"`
	Message string                 `json:"message"`
	Details map[string]interface{} `json:"details,omitempty"`
}

// Error 实现 error 接口
func (e *Error) Error() string {
	return fmt.Sprintf("[%d] %s", e.Code, e.Message)
}

// NewError 创建新错误
func NewError(code int, message string) *Error {
	return &Error{
		Code:    code,
		Message: message,
		Details: make(map[string]interface{}),
	}
}

// WrapError 包装已有错误
func WrapError(code int, err error) *Error {
	return &Error{
		Code:    code,
		Message: err.Error(),
		Details: make(map[string]interface{}),
	}
}

// WithDetails 添加详细信息
func (e *Error) WithDetails(key string, value interface{}) *Error {
	e.Details[key] = value
	return e
}
