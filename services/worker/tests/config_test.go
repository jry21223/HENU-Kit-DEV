package tests

import (
	"testing"

	"final-review-platform/services/worker/pkg/config"
)

func TestDefaultConfig(t *testing.T) {
	cfg := config.Load()
	if cfg.RedisAddr == "" {
		t.Fatal("expected default redis address")
	}
	if cfg.DatabaseURL == "" {
		t.Fatal("expected default database url")
	}
	if cfg.LLMMode == "" {
		t.Fatal("expected default llm mode")
	}
	if cfg.TaskStream == "" {
		t.Fatal("expected default task stream")
	}
}
