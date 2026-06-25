package preflight

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"final-review-platform/services/api/internal/auth"
	"final-review-platform/services/api/internal/payment"
	"final-review-platform/services/api/pkg/config"
)

type Options struct {
	CheckFiles bool
}

type Check struct {
	Name    string `json:"name"`
	Passed  bool   `json:"passed"`
	Message string `json:"message"`
}

func Run(cfg config.Config, opts Options) []Check {
	checks := []Check{}
	checks = append(checks, check("environment", strings.EqualFold(strings.TrimSpace(cfg.Environment), "production"), "APP_ENV must be production"))
	httpErr := config.ValidateHTTPConfig(cfg)
	checks = append(checks, check("http_config", httpErr == nil, errorMessage("HTTP config", httpErr)))
	checks = append(checks, check("auto_migrate", !cfg.AutoMigrate, "AUTO_MIGRATE must be false in production"))
	checks = append(checks, check("fixed_verification_code", strings.TrimSpace(cfg.DevFixedCode) == "", "DEV_FIXED_VERIFICATION_CODE must be empty in production"))
	checks = append(checks, check("database_url", validPostgresURL(cfg.DatabaseURL), "DATABASE_URL must be a non-placeholder postgres URL"))
	checks = append(checks, check("redis_config", validRedisConfig(cfg.Redis), "REDIS_ADDR must be set and REDIS_PASSWORD must not be a placeholder"))

	_, jwtErr := auth.NewTokenManager(cfg)
	checks = append(checks, check("jwt_keys", jwtErr == nil, errorMessage("JWT key config", jwtErr)))

	wechatErr := payment.ValidateWeChatNativeConfig(cfg.Environment, cfg.WeChatPay)
	checks = append(checks, check("wechat_config", wechatErr == nil, errorMessage("WeChat Pay config", wechatErr)))
	checks = append(checks, check("wechat_no_placeholders", !wechatHasPlaceholders(cfg.WeChatPay), "WeChat Pay live fields must not contain example.com or change-me placeholders"))
	checks = append(checks, check("wechat_api_v3_key", validAPIV3Key(cfg.WeChatPay), "WECHAT_PAY_API_V3_KEY must be exactly 32 characters in live mode"))
	checks = append(checks, check("wechat_notify_url", validHTTPSURL(cfg.WeChatPay.NotifyURL), "WECHAT_PAY_NOTIFY_URL must be an exact HTTPS URL"))
	checks = append(checks, check("wechat_notify_not_placeholder", !hasPlaceholder(cfg.WeChatPay.NotifyURL), "WECHAT_PAY_NOTIFY_URL must not use example.com or change-me placeholders"))

	checks = append(checks, check("upload_dir", strings.TrimSpace(cfg.LocalUploadDir) != "", "LOCAL_UPLOAD_DIR must be set"))
	if opts.CheckFiles {
		checks = append(checks, fileChecks(cfg)...)
	}
	return checks
}

func Passed(checks []Check) bool {
	for _, item := range checks {
		if !item.Passed {
			return false
		}
	}
	return true
}

func check(name string, passed bool, message string) Check {
	if passed {
		return Check{Name: name, Passed: true, Message: "ok"}
	}
	return Check{Name: name, Passed: false, Message: message}
}

func errorMessage(prefix string, err error) string {
	if err == nil {
		return "ok"
	}
	return fmt.Sprintf("%s failed: %v", prefix, err)
}

func validPostgresURL(raw string) bool {
	value := strings.TrimSpace(raw)
	if value == "" || hasPlaceholder(value) {
		return false
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return false
	}
	return parsed.Scheme == "postgres" || parsed.Scheme == "postgresql"
}

func validRedisConfig(cfg config.RedisConfig) bool {
	if strings.TrimSpace(cfg.Addr) == "" {
		return false
	}
	return !hasPlaceholder(cfg.Addr) && !hasPlaceholder(cfg.Password)
}

func validHTTPSURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	return parsed.Scheme == "https" && parsed.Host != "" && parsed.Path != ""
}

func hasPlaceholder(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	return strings.Contains(lower, "change-me") || strings.Contains(lower, "example.com")
}

func validAPIV3Key(cfg config.WeChatPayConfig) bool {
	if !strings.EqualFold(strings.TrimSpace(cfg.Mode), "live") {
		return true
	}
	return len(strings.TrimSpace(cfg.APIV3Key)) == 32 && !hasPlaceholder(cfg.APIV3Key)
}

func wechatHasPlaceholders(cfg config.WeChatPayConfig) bool {
	values := []string{
		cfg.APIBaseURL,
		cfg.AppID,
		cfg.MchID,
		cfg.APIV3Key,
		cfg.MerchantSerialNo,
		cfg.MerchantPrivateKey,
		cfg.MerchantPrivateKeyPath,
		cfg.PlatformCertsDir,
		cfg.NotifyURL,
	}
	for _, value := range values {
		if hasPlaceholder(value) {
			return true
		}
	}
	return false
}

func fileChecks(cfg config.Config) []Check {
	checks := []Check{}
	checks = append(checks, checkPathFile("jwt_private_key_path", cfg.JWT.PrivateKeyPEM, cfg.JWT.PrivateKeyPath, "JWT_PRIVATE_KEY_PATH must point to a readable file when JWT_PRIVATE_KEY is not set"))
	checks = append(checks, checkPathFile("jwt_public_key_path", cfg.JWT.PublicKeyPEM, cfg.JWT.PublicKeyPath, "JWT_PUBLIC_KEY_PATH must point to a readable file when JWT_PUBLIC_KEY is not set"))
	checks = append(checks, checkPathFile("wechat_private_key_path", cfg.WeChatPay.MerchantPrivateKey, cfg.WeChatPay.MerchantPrivateKeyPath, "WECHAT_PAY_MERCHANT_PRIVATE_KEY_PATH must point to a readable file when WECHAT_PAY_MERCHANT_PRIVATE_KEY is not set"))
	checks = append(checks, checkDirHasFiles("wechat_platform_certs_dir", cfg.WeChatPay.PlatformCertsDir, "WECHAT_PAY_PLATFORM_CERTS_DIR must contain at least one certificate file"))
	checks = append(checks, checkWechatPlatformCertValidity(cfg.WeChatPay.PlatformCertsDir, cfg.WeChatPay.PlatformCertMinValidDays))
	checks = append(checks, checkDir("local_upload_dir", cfg.LocalUploadDir, "LOCAL_UPLOAD_DIR must be an existing directory"))
	return checks
}

func checkPathFile(name string, inlineSecret string, path string, message string) Check {
	if strings.TrimSpace(inlineSecret) != "" {
		return Check{Name: name, Passed: true, Message: "ok"}
	}
	cleaned := strings.TrimSpace(path)
	if cleaned == "" {
		return Check{Name: name, Passed: false, Message: message}
	}
	info, err := os.Stat(cleaned)
	if err != nil || info.IsDir() {
		return Check{Name: name, Passed: false, Message: message}
	}
	return Check{Name: name, Passed: true, Message: "ok"}
}

func checkDir(name string, path string, message string) Check {
	info, err := os.Stat(strings.TrimSpace(path))
	if err != nil || !info.IsDir() {
		return Check{Name: name, Passed: false, Message: message}
	}
	return Check{Name: name, Passed: true, Message: "ok"}
}

func checkDirHasFiles(name string, path string, message string) Check {
	cleaned := strings.TrimSpace(path)
	info, err := os.Stat(cleaned)
	if err != nil || !info.IsDir() {
		return Check{Name: name, Passed: false, Message: message}
	}
	entries, err := os.ReadDir(cleaned)
	if err != nil {
		return Check{Name: name, Passed: false, Message: message}
	}
	for _, entry := range entries {
		if entry.Type().IsRegular() && !strings.HasPrefix(entry.Name(), ".") {
			return Check{Name: name, Passed: true, Message: "ok"}
		}
	}
	return Check{Name: name, Passed: false, Message: message}
}

func checkWechatPlatformCertValidity(path string, minValidDays int) Check {
	if minValidDays <= 0 {
		minValidDays = 7
	}
	certs, err := readWechatPlatformCertificates(path)
	if err != nil {
		return Check{Name: "wechat_platform_cert_validity", Passed: false, Message: err.Error()}
	}
	if len(certs) == 0 {
		return Check{
			Name:    "wechat_platform_cert_validity",
			Passed:  false,
			Message: "WECHAT_PAY_PLATFORM_CERTS_DIR must contain at least one parseable WeChat platform certificate, not just a public key",
		}
	}
	now := time.Now().UTC()
	minValidUntil := now.Add(time.Duration(minValidDays) * 24 * time.Hour)
	for _, cert := range certs {
		if (cert.NotBefore.IsZero() || !now.Before(cert.NotBefore)) && cert.NotAfter.After(minValidUntil) {
			return Check{Name: "wechat_platform_cert_validity", Passed: true, Message: "ok"}
		}
	}
	return Check{
		Name:    "wechat_platform_cert_validity",
		Passed:  false,
		Message: fmt.Sprintf("WECHAT_PAY_PLATFORM_CERTS_DIR must contain a platform certificate valid for at least %d more day(s)", minValidDays),
	}
}

func readWechatPlatformCertificates(path string) ([]*x509.Certificate, error) {
	cleaned := strings.TrimSpace(path)
	info, err := os.Stat(cleaned)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("WECHAT_PAY_PLATFORM_CERTS_DIR must be an existing directory")
	}
	entries, err := os.ReadDir(cleaned)
	if err != nil {
		return nil, fmt.Errorf("WECHAT_PAY_PLATFORM_CERTS_DIR must be readable")
	}
	certs := []*x509.Certificate{}
	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext != ".pem" && ext != ".crt" && ext != ".cer" {
			continue
		}
		content, err := os.ReadFile(filepath.Join(cleaned, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("WECHAT_PAY_PLATFORM_CERTS_DIR contains unreadable certificate file")
		}
		certs = append(certs, parseCertificates(content)...)
	}
	return certs, nil
}

func parseCertificates(content []byte) []*x509.Certificate {
	certs := []*x509.Certificate{}
	rest := content
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			continue
		}
		certs = append(certs, cert)
	}
	if len(certs) == 0 {
		cert, err := x509.ParseCertificate(content)
		if err != nil {
			return certs
		}
		certs = append(certs, cert)
	}
	return certs
}

func FailedChecks(checks []Check) []Check {
	failed := []Check{}
	for _, item := range checks {
		if !item.Passed {
			failed = append(failed, item)
		}
	}
	return failed
}
