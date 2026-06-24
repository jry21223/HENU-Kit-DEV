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
	})
	if err != nil {
		t.Fatalf("expected valid production CORS origins, got %v", err)
	}
}
