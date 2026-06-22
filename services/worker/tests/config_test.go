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
	if cfg.LLMMode == "" {
		t.Fatal("expected default llm mode")
	}
}
