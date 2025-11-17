package cert

import "time"

// CertStatus 证书状态
type CertStatus string

const (
	StatusActive  CertStatus = "active"  // 活跃状态
	StatusRevoked CertStatus = "revoked" // 已吊销
	StatusExpired CertStatus = "expired" // 已过期
)

// CertInfo 证书信息
type CertInfo struct {
	Fingerprint string     `json:"fingerprint"` // 证书指纹（SHA256）
	ClientID    string     `json:"client_id"`   // 客户端标识（可选）
	Subject     string     `json:"subject"`     // 证书主题
	Issuer      string     `json:"issuer"`      // 签发者
	NotBefore   time.Time  `json:"not_before"`  // 有效期开始时间
	NotAfter    time.Time  `json:"not_after"`   // 有效期结束时间
	Status      CertStatus `json:"status"`      // 证书状态
}
