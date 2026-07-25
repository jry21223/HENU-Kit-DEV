package db

import (
	"testing"
)

func TestModeDefaults(t *testing.T) {
	t.Setenv("PORTAL_API_MODE", "")
	t.Setenv("APP_ENV", "development")
	if got := Mode(); got != ModeMock {
		t.Fatalf("development default mode = %q, want %q", got, ModeMock)
	}

	t.Setenv("APP_ENV", "production")
	if got := Mode(); got != ModeLive {
		t.Fatalf("production default mode = %q, want %q", got, ModeLive)
	}

	t.Setenv("PORTAL_API_MODE", "mock")
	t.Setenv("APP_ENV", "production")
	if got := Mode(); got != ModeMock {
		t.Fatalf("explicit mock override = %q, want %q", got, ModeMock)
	}

	t.Setenv("PORTAL_API_MODE", "live")
	t.Setenv("APP_ENV", "development")
	if got := Mode(); got != ModeLive {
		t.Fatalf("explicit live override = %q, want %q", got, ModeLive)
	}
}

func TestConnectLiveMissingDSN(t *testing.T) {
	t.Setenv("PORTAL_API_MODE", "live")
	t.Setenv("APP_ENV", "production")
	t.Setenv("TEST_LIVE_DSN", "")

	conn, err := Connect("TEST_LIVE_DSN")
	if err == nil {
		t.Fatal("expected error when live mode DSN is empty")
	}
	if conn != nil {
		t.Fatal("expected nil connection on live mode empty DSN")
	}
}

func TestConnectMockMissingDSN(t *testing.T) {
	t.Setenv("PORTAL_API_MODE", "mock")
	t.Setenv("APP_ENV", "development")
	t.Setenv("TEST_MOCK_DSN", "")

	conn, err := Connect("TEST_MOCK_DSN")
	if err != nil {
		t.Fatalf("mock mode empty DSN should be nil error, got %v", err)
	}
	if conn != nil {
		t.Fatal("expected nil connection in mock mode without DSN")
	}
}
