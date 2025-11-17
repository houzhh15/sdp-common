package cert

import (
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"golang.org/x/crypto/ocsp"
)

// Validator 证书验证器
type Validator struct {
	caCertPool *x509.CertPool
	checkOCSP  bool
	httpClient *http.Client
}

// ValidatorConfig 验证器配置
type ValidatorConfig struct {
	CACertPool *x509.CertPool // CA证书池
	CheckOCSP  bool           // 是否检查OCSP
	Timeout    time.Duration  // HTTP超时时间
}

// NewValidator 创建证书验证器
func NewValidator(config *ValidatorConfig) *Validator {
	if config == nil {
		config = &ValidatorConfig{
			CheckOCSP: false,
			Timeout:   10 * time.Second,
		}
	}

	return &Validator{
		caCertPool: config.CACertPool,
		checkOCSP:  config.CheckOCSP,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
	}
}

// ValidateCert 验证证书
func (v *Validator) ValidateCert(cert *x509.Certificate) error {
	if cert == nil {
		return errors.New("certificate is nil")
	}

	// 1. 检查证书有效期
	now := time.Now()
	if now.Before(cert.NotBefore) {
		return fmt.Errorf("certificate not yet valid (NotBefore: %s)", cert.NotBefore)
	}
	if now.After(cert.NotAfter) {
		return fmt.Errorf("certificate has expired (NotAfter: %s)", cert.NotAfter)
	}

	// 2. 检查证书链（如果提供了CA池）
	if v.caCertPool != nil {
		opts := x509.VerifyOptions{
			Roots:       v.caCertPool,
			CurrentTime: now,
		}

		if _, err := cert.Verify(opts); err != nil {
			return fmt.Errorf("certificate verification failed: %w", err)
		}
	}

	// 3. 检查OCSP（如果启用）
	if v.checkOCSP {
		if err := v.CheckRevocation(cert); err != nil {
			return fmt.Errorf("OCSP check failed: %w", err)
		}
	}

	return nil
}

// CheckRevocation 检查证书吊销状态（OCSP）
func (v *Validator) CheckRevocation(cert *x509.Certificate) error {
	if cert == nil {
		return errors.New("certificate is nil")
	}

	// 检查证书是否包含OCSP服务器地址
	if len(cert.OCSPServer) == 0 {
		return errors.New("certificate does not contain OCSP server URLs")
	}

	// 需要签发者证书
	if cert.Issuer.String() == cert.Subject.String() {
		// 自签名证书，跳过OCSP检查
		return nil
	}

	// 构建OCSP请求
	ocspRequest, err := ocsp.CreateRequest(cert, nil, nil)
	if err != nil {
		return fmt.Errorf("failed to create OCSP request: %w", err)
	}

	// 向OCSP服务器发送请求
	for _, server := range cert.OCSPServer {
		resp, err := v.sendOCSPRequest(server, ocspRequest)
		if err != nil {
			continue // 尝试下一个服务器
		}

		// 解析OCSP响应
		ocspResp, err := ocsp.ParseResponse(resp, nil)
		if err != nil {
			continue
		}

		// 检查吊销状态
		switch ocspResp.Status {
		case ocsp.Good:
			return nil
		case ocsp.Revoked:
			return fmt.Errorf("certificate has been revoked (reason: %d)", ocspResp.RevocationReason)
		case ocsp.Unknown:
			return errors.New("certificate status unknown")
		}
	}

	return errors.New("failed to check OCSP from all servers")
}

// sendOCSPRequest 发送OCSP请求
func (v *Validator) sendOCSPRequest(server string, request []byte) ([]byte, error) {
	httpResp, err := v.httpClient.Post(server, "application/ocsp-request", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to send OCSP request: %w", err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OCSP server returned status: %d", httpResp.StatusCode)
	}

	return io.ReadAll(httpResp.Body)
}

// ValidateCertChain 验证证书链
func (v *Validator) ValidateCertChain(certChain []*x509.Certificate) error {
	if len(certChain) == 0 {
		return errors.New("certificate chain is empty")
	}

	// 验证每个证书
	for i, cert := range certChain {
		if err := v.ValidateCert(cert); err != nil {
			return fmt.Errorf("certificate at position %d is invalid: %w", i, err)
		}
	}

	// 验证证书链完整性
	for i := 0; i < len(certChain)-1; i++ {
		cert := certChain[i]
		issuer := certChain[i+1]

		if cert.Issuer.String() != issuer.Subject.String() {
			return fmt.Errorf("certificate chain broken at position %d", i)
		}
	}

	return nil
}

// LoadCRLFromFile 从文件加载CRL（证书吊销列表）
func LoadCRLFromFile(path string) (*x509.RevocationList, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read CRL file: %w", err)
	}

	return ParseCRL(data)
}

// ParseCRL 解析CRL数据
func ParseCRL(data []byte) (*x509.RevocationList, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		// 尝试DER格式
		return x509.ParseRevocationList(data)
	}

	return x509.ParseRevocationList(block.Bytes)
}

// CheckCRL 检查证书是否在CRL中
func CheckCRL(cert *x509.Certificate, crl *x509.RevocationList) error {
	if cert == nil || crl == nil {
		return errors.New("certificate or CRL is nil")
	}

	// 检查CRL是否过期
	if time.Now().After(crl.NextUpdate) {
		return errors.New("CRL has expired")
	}

	// 检查证书是否在吊销列表中
	for _, revokedCert := range crl.RevokedCertificateEntries {
		if revokedCert.SerialNumber.Cmp(cert.SerialNumber) == 0 {
			return fmt.Errorf("certificate has been revoked at %s", revokedCert.RevocationTime)
		}
	}

	return nil
}
