package config

import (
	"errors"
	"testing"
)

func TestValidateHTTPConfigRejectsWildcardWithCredentials(t *testing.T) {
	err := ValidateHTTPConfig(Config{
		Environment:        "development",
		CORSAllowedOrigins: []string{"*"},
	})
	if !errors.Is(err, ErrCORSWildcardWithCredentials) {
		t.Fatalf("expected wildcard CORS rejection, got %v", err)
	}
}

func TestValidateHTTPConfigRejectsUnsafeProductionCORS(t *testing.T) {
	tests := []struct {
		name    string
		origins []string
		want    error
	}{
		{name: "empty", origins: nil, want: ErrProductionCORSRequired},
		{name: "blank", origins: []string{" "}, want: ErrProductionCORSRequired},
		{name: "http", origins: []string{"http://review.example.com"}, want: ErrProductionCORSHTTPSRequired},
		{name: "with path", origins: []string{"https://review.example.com/app"}, want: ErrInvalidCORSOrigin},
		{name: "with query", origins: []string{"https://review.example.com?x=1"}, want: ErrInvalidCORSOrigin},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateHTTPConfig(Config{
				Environment:        "production",
				CORSAllowedOrigins: tt.origins,
			})
			if !errors.Is(err, tt.want) {
				t.Fatalf("expected %v, got %v", tt.want, err)
			}
		})
	}
}

func TestValidateHTTPConfigAcceptsExactProductionHTTPSOrigins(t *testing.T) {
	err := ValidateHTTPConfig(Config{
		Environment:        "production",
		CORSAllowedOrigins: []string{"https://review.example.com", "https://admin.review.example.com"},
		InternalHMACKeys:   map[string]string{"notice:active": "secret"},
	})
	if err != nil {
		t.Fatalf("expected valid production CORS origins, got %v", err)
	}
}

func TestValidateHTTPConfigRejectsUnsafeProductionRuntimeSettings(t *testing.T) {
	base := Config{Environment: "production", CORSAllowedOrigins: []string{"https://admin.example.com"}, InternalHMACKeys: map[string]string{"notice:active": "secret"}}
	base.AutoMigrate = true
	if err := ValidateHTTPConfig(base); !errors.Is(err, ErrProductionAutoMigrate) {
		t.Fatalf("expected production AutoMigrate rejection, got %v", err)
	}
	base.AutoMigrate = false
	base.InternalHMACKeys = nil
	if err := ValidateHTTPConfig(base); !errors.Is(err, ErrProductionHMACKeysRequired) {
		t.Fatalf("expected production HMAC key rejection, got %v", err)
	}
}

func TestLoadAllowsEmptyProductionFixedVerificationCode(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("DEV_FIXED_VERIFICATION_CODE", "")

	cfg := Load()
	if cfg.DevFixedCode != "" {
		t.Fatalf("expected empty fixed code in production, got %q", cfg.DevFixedCode)
	}
}

func TestLoadDefaultsDevelopmentFixedVerificationCode(t *testing.T) {
	t.Setenv("APP_ENV", "development")

	cfg := Load()
	if cfg.DevFixedCode != "123456" {
		t.Fatalf("expected development fixed code fallback, got %q", cfg.DevFixedCode)
	}
}
