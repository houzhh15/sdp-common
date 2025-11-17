package session

import (
	"crypto/rand"
	"encoding/hex"
)

// generateToken 生成 64 字符十六进制 Token（完整复用 session.go）
func generateToken() (string, error) {
	b := make([]byte, 32) // 32 字节 = 64 字符 hex
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
