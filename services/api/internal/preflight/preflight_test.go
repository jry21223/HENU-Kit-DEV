package preflight

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"final-review-platform/services/api/pkg/config"
)

func TestRunPassesValidProductionConfig(t *testing.T) {
	cfg := validProductionConfig(t)
	checks := Run(cfg, Options{CheckFiles: true})
	if !Passed(checks) {
		t.Fatalf("expected all checks to pass, failed: %#v", FailedChecks(checks))
	}
}

func TestRunFlagsDangerousProductionConfig(t *testing.T) {
	cfg := validProductionConfig(t)
	cfg.Environment = "development"
	cfg.AutoMigrate = true
	cfg.DevFixedCode = "123456"
	cfg.DatabaseURL = "postgres://change-me@postgres:5432/final_review_v2"
	cfg.Redis.Addr = "change-me-redis:6379"
	cfg.CORSAllowedOrigins = []string{"*"}
	cfg.WeChatPay.Mode = "mock"
	cfg.WeChatPay.APIV3Key = "short"
	cfg.WeChatPay.NotifyURL = "http://example.com/api/v1/payments/wechat/notify"

	failed := failedNames(Run(cfg, Options{CheckFiles: false}))
	for _, expected := range []string{
		"environment",
		"http_config",
		"auto_migrate",
		"fixed_verification_code",
		"database_url",
		"redis_config",
		"wechat_no_placeholders",
		"wechat_notify_url",
		"wechat_notify_not_placeholder",
	} {
		if !failed[expected] {
			t.Fatalf("expected %s to fail, got %#v", expected, failed)
		}
	}
}

func TestRunRejectsIncompleteLiveWeChatConfig(t *testing.T) {
	cfg := validProductionConfig(t)
	cfg.WeChatPay.APIV3Key = ""
	cfg.WeChatPay.MerchantPrivateKey = ""
	cfg.WeChatPay.MerchantPrivateKeyPath = ""

	failed := failedNames(Run(cfg, Options{CheckFiles: false}))
	if !failed["wechat_config"] || !failed["wechat_api_v3_key"] {
		t.Fatalf("expected live WeChat config checks to fail, got %#v", failed)
	}
}

func TestRunRejectsExpiringWeChatPlatformCertificate(t *testing.T) {
	cfg := validProductionConfig(t)
	certsDir := t.TempDir()
	writeTestPlatformCertificate(t, certsDir, "wechatpay_expiring.pem", 24*time.Hour)
	cfg.WeChatPay.PlatformCertsDir = certsDir
	cfg.WeChatPay.PlatformCertMinValidDays = 7

	failed := failedNames(Run(cfg, Options{CheckFiles: true}))
	if !failed["wechat_platform_cert_validity"] {
		t.Fatalf("expected expiring WeChat platform certificate to fail, got %#v", failed)
	}
}

func TestRunAcceptsDERWechatPlatformCertificate(t *testing.T) {
	cfg := validProductionConfig(t)
	certsDir := t.TempDir()
	writeTestPlatformCertificateDER(t, certsDir, "wechatpay_platform.cer", 30*24*time.Hour)
	cfg.WeChatPay.PlatformCertsDir = certsDir

	checks := Run(cfg, Options{CheckFiles: true})
	if !Passed(checks) {
		t.Fatalf("expected DER platform certificate to pass, failed: %#v", FailedChecks(checks))
	}
}

func TestRunRejectsPublicKeyOnlyWechatPlatformCertDirectory(t *testing.T) {
	cfg := validProductionConfig(t)
	_, publicPEM := testRSAKeyPairPEM(t)
	certsDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(certsDir, "wechatpay_platform.pem"), []byte(publicPEM), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg.WeChatPay.PlatformCertsDir = certsDir

	failed := failedNames(Run(cfg, Options{CheckFiles: true}))
	if !failed["wechat_platform_cert_validity"] {
		t.Fatalf("expected public-key-only WeChat platform directory to fail, got %#v", failed)
	}
}

func TestLoadEnvFile(t *testing.T) {
	t.Setenv("PREFLIGHT_TEST_APP_ENV", "")
	t.Setenv("PREFLIGHT_TEST_ORIGINS", "")
	t.Setenv("PREFLIGHT_TEST_EMPTY", "not-empty")

	path := filepath.Join(t.TempDir(), ".env")
	content := "\ufeff# comment\nexport PREFLIGHT_TEST_APP_ENV=production\nPREFLIGHT_TEST_ORIGINS=\"https://review.henu.local,https://admin.henu.local\"\nPREFLIGHT_TEST_EMPTY=''\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := LoadEnvFile(path); err != nil {
		t.Fatalf("LoadEnvFile failed: %v", err)
	}
	if got := os.Getenv("PREFLIGHT_TEST_APP_ENV"); got != "production" {
		t.Fatalf("expected APP_ENV production, got %q", got)
	}
	if got := os.Getenv("PREFLIGHT_TEST_ORIGINS"); got != "https://review.henu.local,https://admin.henu.local" {
		t.Fatalf("unexpected origins: %q", got)
	}
	if got := os.Getenv("PREFLIGHT_TEST_EMPTY"); got != "" {
		t.Fatalf("expected quoted empty value, got %q", got)
	}
}

func TestLoadEnvFileRejectsInvalidLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("APP_ENV production\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := LoadEnvFile(path); !errors.Is(err, ErrInvalidEnvFileLine) {
		t.Fatalf("expected ErrInvalidEnvFileLine, got %v", err)
	}
}

func validProductionConfig(t *testing.T) config.Config {
	t.Helper()
	privatePEM, publicPEM := testRSAKeyPairPEM(t)
	certsDir := t.TempDir()
	writeTestPlatformCertificate(t, certsDir, "wechatpay_platform.pem", 30*24*time.Hour)
	uploadDir := t.TempDir()
	return config.Config{
		Environment:        "production",
		DatabaseURL:        "postgres://final_review:strong-password@postgres:5432/final_review_v2?sslmode=disable",
		Redis:              config.RedisConfig{Addr: "redis:6379", Password: "strong-redis-password"},
		CORSAllowedOrigins: []string{"https://review.henu.local", "https://admin.henu.local"},
		AutoMigrate:        false,
		DevFixedCode:       "",
		LocalUploadDir:     uploadDir,
		JWT: config.JWTConfig{
			Issuer:           "final-review-platform",
			AccessTTLMinutes: 15,
			RefreshTTLHours:  168,
			PrivateKeyPEM:    privatePEM,
			PublicKeyPEM:     publicPEM,
		},
		WeChatPay: config.WeChatPayConfig{
			Mode:                "live",
			APIBaseURL:          "https://api.mch.weixin.qq.com",
			AppID:               "wx1234567890abcdef",
			MchID:               "1900000001",
			APIV3Key:            "12345678901234567890123456789012",
			MerchantSerialNo:    "ABC123456789",
			MerchantPrivateKey:  privatePEM,
			PlatformCertsDir:    certsDir,
			NotifyURL:           "https://review.henu.local/api/v1/payments/wechat/notify",
			NativeExpireMinutes: 15,
		},
	}
}

func testRSAKeyPairPEM(t *testing.T) (string, string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	privatePEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	publicBytes, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	publicPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicBytes})
	return string(privatePEM), string(publicPEM)
}

func writeTestPlatformCertificate(t *testing.T, dir string, name string, validFor time.Duration) {
	t.Helper()
	der := testPlatformCertificateDER(t, validFor)
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	if err := os.WriteFile(filepath.Join(dir, name), certPEM, 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeTestPlatformCertificateDER(t *testing.T, dir string, name string, validFor time.Duration) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), testPlatformCertificateDER(t, validFor), 0o600); err != nil {
		t.Fatal(err)
	}
}

func testPlatformCertificateDER(t *testing.T, validFor time.Duration) []byte {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	template := &x509.Certificate{
		SerialNumber: big.NewInt(now.UnixNano()),
		Subject:      pkix.Name{CommonName: "test-wechat-platform"},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(validFor),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return der
}

func failedNames(checks []Check) map[string]bool {
	failed := map[string]bool{}
	for _, item := range checks {
		if !item.Passed {
			failed[item.Name] = true
		}
	}
	return failed
}
