package payment

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"strings"
	"testing"

	"final-review-platform/services/api/pkg/config"
)

func TestWeChatRequestSignatureAndAuthorizationHeader(t *testing.T) {
	privateKey := mustTestPrivateKey(t)
	body := `{"appid":"wx-test","mchid":"mch-test","amount":{"total":1990}}`
	header, err := BuildSignedWeChatAuthorizationHeader(
		"post",
		"/v3/pay/transactions/native",
		body,
		1770000000,
		"nonce-test",
		"mch-test",
		"serial-test",
		privateKey,
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		`WECHATPAY2-SHA256-RSA2048`,
		`mchid="mch-test"`,
		`nonce_str="nonce-test"`,
		`timestamp="1770000000"`,
		`serial_no="serial-test"`,
		`signature="`,
	} {
		if !strings.Contains(header, expected) {
			t.Fatalf("authorization header missing %q: %s", expected, header)
		}
	}
	signature := extractQuotedHeaderValue(t, header, "signature")
	message := BuildWeChatRequestSignatureMessage("POST", "/v3/pay/transactions/native", "1770000000", "nonce-test", body)
	if err := VerifyWeChatMessage(message, signature, &privateKey.PublicKey); err != nil {
		t.Fatalf("expected generated authorization signature to verify, got %v", err)
	}
}

func TestWeChatNotifySignatureVerification(t *testing.T) {
	privateKey := mustTestPrivateKey(t)
	body := []byte(`{"resource":{"ciphertext":"abc"}}`)
	message := BuildWeChatNotifySignatureMessage("1770000001", "notify-nonce", body)
	signature, err := SignWeChatMessage(message, privateKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyWeChatMessage(message, signature, &privateKey.PublicKey); err != nil {
		t.Fatalf("expected notify signature to verify, got %v", err)
	}
	tamperedMessage := BuildWeChatNotifySignatureMessage("1770000001", "notify-nonce", []byte(`{"resource":{"ciphertext":"tampered"}}`))
	if err := VerifyWeChatMessage(tamperedMessage, signature, &privateKey.PublicKey); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("expected tampered notify body to fail verification, got %v", err)
	}
	if err := VerifyWeChatMessage(message, "not-base64", &privateKey.PublicKey); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("expected malformed signature to fail verification, got %v", err)
	}
}

func TestDecryptWeChatResource(t *testing.T) {
	apiV3Key := "12345678901234567890123456789012"
	nonce := "nonce-123456"
	associatedData := "transaction"
	plaintext := []byte(`{"out_trade_no":"FR001","trade_state":"SUCCESS","amount":{"total":1990}}`)
	block, err := aes.NewCipher([]byte(apiV3Key))
	if err != nil {
		t.Fatal(err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	ciphertext := aead.Seal(nil, []byte(nonce), plaintext, []byte(associatedData))

	decrypted, err := DecryptWeChatResource(apiV3Key, nonce, associatedData, base64.StdEncoding.EncodeToString(ciphertext))
	if err != nil {
		t.Fatal(err)
	}
	if string(decrypted) != string(plaintext) {
		t.Fatalf("unexpected plaintext: %s", string(decrypted))
	}
	if _, err := DecryptWeChatResource("short-key", nonce, associatedData, base64.StdEncoding.EncodeToString(ciphertext)); !errors.Is(err, ErrInvalidAPIV3Key) {
		t.Fatalf("expected invalid API v3 key rejection, got %v", err)
	}
	if _, err := DecryptWeChatResource(apiV3Key, nonce, "wrong-aad", base64.StdEncoding.EncodeToString(ciphertext)); !errors.Is(err, ErrInvalidCiphertext) {
		t.Fatalf("expected wrong associated data rejection, got %v", err)
	}
	if _, err := DecryptWeChatResource(apiV3Key, nonce, associatedData, "not-base64"); !errors.Is(err, ErrInvalidCiphertext) {
		t.Fatalf("expected invalid ciphertext rejection, got %v", err)
	}
}

func TestLoadAndParseMerchantPrivateKey(t *testing.T) {
	privateKey := mustTestPrivateKey(t)
	pkcs1PEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)})
	parsed, err := ParseMerchantPrivateKeyPEM(pkcs1PEM)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.N.Cmp(privateKey.N) != 0 {
		t.Fatal("parsed PKCS1 key does not match generated key")
	}

	pkcs8Bytes, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	pkcs8PEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8Bytes})
	parsedPKCS8, err := LoadMerchantPrivateKey(config.WeChatPayConfig{MerchantPrivateKey: string(pkcs8PEM)})
	if err != nil {
		t.Fatal(err)
	}
	if parsedPKCS8.N.Cmp(privateKey.N) != 0 {
		t.Fatal("loaded PKCS8 key does not match generated key")
	}

	if _, err := LoadMerchantPrivateKey(config.WeChatPayConfig{}); !errors.Is(err, ErrMerchantPrivateKeyMissing) {
		t.Fatalf("expected missing private key rejection, got %v", err)
	}
	if _, err := ParseMerchantPrivateKeyPEM([]byte("broken")); !errors.Is(err, ErrInvalidMerchantPrivateKey) {
		t.Fatalf("expected invalid private key rejection, got %v", err)
	}
}

func TestParsePlatformPublicKeyPEM(t *testing.T) {
	privateKey := mustTestPrivateKey(t)
	publicBytes, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	publicPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicBytes})
	parsed, err := ParsePlatformPublicKeyPEM(publicPEM)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.N.Cmp(privateKey.N) != 0 {
		t.Fatal("parsed platform public key does not match generated key")
	}
	if _, err := ParsePlatformPublicKeyPEM([]byte("broken")); !errors.Is(err, ErrInvalidPlatformPublicKey) {
		t.Fatalf("expected invalid platform public key rejection, got %v", err)
	}
}

func mustTestPrivateKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func extractQuotedHeaderValue(t *testing.T, header string, key string) string {
	t.Helper()
	prefix := key + `="`
	start := strings.Index(header, prefix)
	if start < 0 {
		t.Fatalf("missing %s in header: %s", key, header)
	}
	valueStart := start + len(prefix)
	valueEnd := strings.Index(header[valueStart:], `"`)
	if valueEnd < 0 {
		t.Fatalf("unterminated %s in header: %s", key, header)
	}
	return header[valueStart : valueStart+valueEnd]
}
