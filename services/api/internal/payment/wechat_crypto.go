package payment

import (
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"final-review-platform/services/api/pkg/config"
)

const wechatAuthorizationScheme = "WECHATPAY2-SHA256-RSA2048"

var (
	ErrInvalidMerchantPrivateKey = errors.New("wechat_invalid_merchant_private_key")
	ErrMerchantPrivateKeyMissing = errors.New("wechat_merchant_private_key_missing")
	ErrInvalidPlatformPublicKey  = errors.New("wechat_invalid_platform_public_key")
	ErrInvalidAPIV3Key           = errors.New("wechat_invalid_api_v3_key")
	ErrInvalidCiphertext         = errors.New("wechat_invalid_ciphertext")
	ErrInvalidSignature          = errors.New("wechat_invalid_signature")
)

func BuildWeChatRequestSignatureMessage(method string, canonicalURL string, timestamp string, nonce string, body string) string {
	return strings.ToUpper(strings.TrimSpace(method)) + "\n" +
		strings.TrimSpace(canonicalURL) + "\n" +
		strings.TrimSpace(timestamp) + "\n" +
		strings.TrimSpace(nonce) + "\n" +
		body + "\n"
}

func BuildWeChatNotifySignatureMessage(timestamp string, nonce string, body []byte) string {
	return strings.TrimSpace(timestamp) + "\n" + strings.TrimSpace(nonce) + "\n" + string(body) + "\n"
}

func SignWeChatMessage(message string, privateKey *rsa.PrivateKey) (string, error) {
	if privateKey == nil {
		return "", ErrInvalidMerchantPrivateKey
	}
	hashed := sha256.Sum256([]byte(message))
	signature, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, hashed[:])
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(signature), nil
}

func VerifyWeChatMessage(message string, signatureBase64 string, publicKey *rsa.PublicKey) error {
	if publicKey == nil {
		return ErrInvalidPlatformPublicKey
	}
	signature, err := base64.StdEncoding.DecodeString(strings.TrimSpace(signatureBase64))
	if err != nil {
		return ErrInvalidSignature
	}
	hashed := sha256.Sum256([]byte(message))
	if err := rsa.VerifyPKCS1v15(publicKey, crypto.SHA256, hashed[:], signature); err != nil {
		return ErrInvalidSignature
	}
	return nil
}

func BuildWeChatAuthorizationHeader(mchID string, serialNo string, nonce string, timestamp string, signature string) string {
	return fmt.Sprintf(
		`%s mchid="%s",nonce_str="%s",signature="%s",timestamp="%s",serial_no="%s"`,
		wechatAuthorizationScheme,
		escapeAuthorizationValue(mchID),
		escapeAuthorizationValue(nonce),
		escapeAuthorizationValue(signature),
		escapeAuthorizationValue(timestamp),
		escapeAuthorizationValue(serialNo),
	)
}

func BuildSignedWeChatAuthorizationHeader(method string, canonicalURL string, body string, timestamp int64, nonce string, mchID string, serialNo string, privateKey *rsa.PrivateKey) (string, error) {
	timestampText := strconv.FormatInt(timestamp, 10)
	message := BuildWeChatRequestSignatureMessage(method, canonicalURL, timestampText, nonce, body)
	signature, err := SignWeChatMessage(message, privateKey)
	if err != nil {
		return "", err
	}
	return BuildWeChatAuthorizationHeader(mchID, serialNo, nonce, timestampText, signature), nil
}

func DecryptWeChatResource(apiV3Key string, nonce string, associatedData string, ciphertextBase64 string) ([]byte, error) {
	key := []byte(strings.TrimSpace(apiV3Key))
	if len(key) != 32 {
		return nil, ErrInvalidAPIV3Key
	}
	ciphertext, err := base64.StdEncoding.DecodeString(strings.TrimSpace(ciphertextBase64))
	if err != nil {
		return nil, ErrInvalidCiphertext
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	plaintext, err := aead.Open(nil, []byte(nonce), ciphertext, []byte(associatedData))
	if err != nil {
		return nil, ErrInvalidCiphertext
	}
	return plaintext, nil
}

func LoadMerchantPrivateKey(cfg config.WeChatPayConfig) (*rsa.PrivateKey, error) {
	inline := strings.TrimSpace(cfg.MerchantPrivateKey)
	if inline != "" {
		return ParseMerchantPrivateKeyPEM([]byte(inline))
	}
	path := strings.TrimSpace(cfg.MerchantPrivateKeyPath)
	if path == "" {
		return nil, ErrMerchantPrivateKeyMissing
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseMerchantPrivateKeyPEM(content)
}

func ParseMerchantPrivateKeyPEM(content []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(content)
	if block == nil {
		return nil, ErrInvalidMerchantPrivateKey
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, ErrInvalidMerchantPrivateKey
	}
	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return nil, ErrInvalidMerchantPrivateKey
	}
	return rsaKey, nil
}

func ParsePlatformPublicKeyPEM(content []byte) (*rsa.PublicKey, error) {
	block, _ := pem.Decode(content)
	if block == nil {
		return nil, ErrInvalidPlatformPublicKey
	}
	if key, err := x509.ParsePKIXPublicKey(block.Bytes); err == nil {
		if rsaKey, ok := key.(*rsa.PublicKey); ok {
			return rsaKey, nil
		}
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err == nil {
		if rsaKey, ok := cert.PublicKey.(*rsa.PublicKey); ok {
			return rsaKey, nil
		}
	}
	return nil, ErrInvalidPlatformPublicKey
}

func escapeAuthorizationValue(value string) string {
	return strings.ReplaceAll(strings.TrimSpace(value), `"`, `\"`)
}
