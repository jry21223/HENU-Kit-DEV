package payment

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"final-review-platform/services/api/internal/platform/model"
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

func TestCreateLiveNativePaymentSignsRequestAndVerifiesResponse(t *testing.T) {
	merchantKey := mustTestPrivateKey(t)
	platformKey := mustTestPrivateKey(t)
	serial := "ABC123"
	certsDir := t.TempDir()
	writeTestCertificate(t, certsDir, serial, platformKey)

	var capturedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.RequestURI() != wechatNativePath {
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.RequestURI())
		}
		if request.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("unexpected content-type: %s", request.Header.Get("Content-Type"))
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		capturedBody = body
		authHeader := request.Header.Get("Authorization")
		timestamp := extractQuotedHeaderValue(t, authHeader, "timestamp")
		nonce := extractQuotedHeaderValue(t, authHeader, "nonce_str")
		signature := extractQuotedHeaderValue(t, authHeader, "signature")
		message := BuildWeChatRequestSignatureMessage(http.MethodPost, wechatNativePath, timestamp, nonce, string(body))
		if err := VerifyWeChatMessage(message, signature, &merchantKey.PublicKey); err != nil {
			t.Fatalf("expected merchant request signature to verify, got %v", err)
		}
		for _, expected := range []string{`"appid":"wx-test"`, `"mchid":"mch-test"`, `"out_trade_no":"FRLIVE001"`, `"total":1990`, `"notify_url":"https://example.com/notify"`} {
			if !strings.Contains(string(body), expected) {
				t.Fatalf("request body missing %q: %s", expected, string(body))
			}
		}

		responseBody := []byte(`{"code_url":"weixin://wxpay/live-test"}`)
		responseTimestamp := "1770000100"
		responseNonce := "response-nonce"
		responseMessage := BuildWeChatNotifySignatureMessage(responseTimestamp, responseNonce, responseBody)
		responseSignature, err := SignWeChatMessage(responseMessage, platformKey)
		if err != nil {
			t.Fatal(err)
		}
		writer.Header().Set("Wechatpay-Timestamp", responseTimestamp)
		writer.Header().Set("Wechatpay-Nonce", responseNonce)
		writer.Header().Set("Wechatpay-Signature", responseSignature)
		writer.Header().Set("Wechatpay-Serial", serial)
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write(responseBody)
	}))
	defer server.Close()

	pkcs1PEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(merchantKey)})
	result, err := createLiveNativePayment(
		t.Context(),
		config.WeChatPayConfig{
			Mode:                "live",
			APIBaseURL:          server.URL,
			AppID:               "wx-test",
			MchID:               "mch-test",
			APIV3Key:            "12345678901234567890123456789012",
			MerchantSerialNo:    "merchant-serial",
			MerchantPrivateKey:  string(pkcs1PEM),
			PlatformCertsDir:    certsDir,
			NotifyURL:           "https://example.com/notify",
			NativeExpireMinutes: 15,
		},
		model.Order{OutTradeNo: "FRLIVE001", AmountTotal: 1990, Currency: "CNY"},
		model.CoursePackage{Title: "离散数学期末复习包"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.CodeURL != "weixin://wxpay/live-test" {
		t.Fatalf("unexpected code URL: %s", result.CodeURL)
	}
	if result.ExpiresAt.Before(time.Now().UTC().Add(14 * time.Minute)) {
		t.Fatalf("unexpected expiresAt: %s", result.ExpiresAt)
	}
	if len(capturedBody) == 0 {
		t.Fatal("expected fake WeChat server to receive request body")
	}
}

func TestCreateLiveNativePaymentRejectsUnsignedResponse(t *testing.T) {
	merchantKey := mustTestPrivateKey(t)
	certsDir := t.TempDir()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write([]byte(`{"code_url":"weixin://wxpay/live-test"}`))
	}))
	defer server.Close()

	pkcs1PEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(merchantKey)})
	_, err := createLiveNativePayment(
		t.Context(),
		config.WeChatPayConfig{
			Mode:                "live",
			APIBaseURL:          server.URL,
			AppID:               "wx-test",
			MchID:               "mch-test",
			APIV3Key:            "12345678901234567890123456789012",
			MerchantSerialNo:    "merchant-serial",
			MerchantPrivateKey:  string(pkcs1PEM),
			PlatformCertsDir:    certsDir,
			NotifyURL:           "https://example.com/notify",
			NativeExpireMinutes: 15,
		},
		model.Order{OutTradeNo: "FRLIVE002", AmountTotal: 1990, Currency: "CNY"},
		model.CoursePackage{Title: "离散数学期末复习包"},
	)
	if !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("expected unsigned response rejection, got %v", err)
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

func writeTestCertificate(t *testing.T, dir string, serial string, key *rsa.PrivateKey) {
	t.Helper()
	serialNumber := new(big.Int)
	if _, ok := serialNumber.SetString(serial, 16); !ok {
		t.Fatalf("invalid test serial: %s", serial)
	}
	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject:      pkix.Name{CommonName: "wechatpay-test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	content := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	if err := os.WriteFile(filepath.Join(dir, "wechatpay_"+serial+".pem"), content, 0o600); err != nil {
		t.Fatal(err)
	}
}
