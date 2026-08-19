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

// Mode is read from the environment on every call, so operator input arrives
// with whatever case and padding a shell or Compose file introduced.
func TestModeNormalizesOperatorInput(t *testing.T) {
	for _, tc := range []struct {
		name string
		mode string
		want string
	}{
		{"uppercase", "LIVE", ModeLive},
		{"padded", "  live  ", ModeLive},
		{"mixed case mock", "MoCk", ModeMock},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("PORTAL_API_MODE", tc.mode)
			t.Setenv("APP_ENV", "development")
			if got := Mode(); got != tc.want {
				t.Errorf("Mode() with PORTAL_API_MODE=%q = %q, want %q", tc.mode, got, tc.want)
			}
		})
	}
}

// An unrecognized PORTAL_API_MODE must not quietly select mock in production:
// it falls through to APP_ENV, which still resolves to live there.
func TestUnrecognizedModeFallsThroughToAppEnv(t *testing.T) {
	for _, mode := range []string{"liv", "real", "1", "true"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("PORTAL_API_MODE", mode)
			t.Setenv("APP_ENV", "production")
			if got := Mode(); got != ModeLive {
				t.Errorf("Mode() with PORTAL_API_MODE=%q in production = %q, want %q", mode, got, ModeLive)
			}
		})
	}
}

func TestProdAppEnvAliasIsLive(t *testing.T) {
	t.Setenv("PORTAL_API_MODE", "")
	for _, appEnv := range []string{"production", "prod", "PRODUCTION", " Prod "} {
		t.Run(appEnv, func(t *testing.T) {
			t.Setenv("APP_ENV", appEnv)
			if got := Mode(); got != ModeLive {
				t.Errorf("Mode() with APP_ENV=%q = %q, want %q", appEnv, got, ModeLive)
			}
			if !IsLive() {
				t.Errorf("IsLive() with APP_ENV=%q = false, want true", appEnv)
			}
		})
	}
}

// The whole point of live mode is that a missing or unreachable database is an
// error rather than a silent fall-through to fixtures. Connect must never
// return (nil, nil) there — that pair is what tells a handler to serve mocks.
func TestConnectNeverReturnsSilentNilInLiveMode(t *testing.T) {
	t.Setenv("PORTAL_API_MODE", "live")
	t.Setenv("APP_ENV", "production")

	for _, tc := range []struct {
		name string
		dsn  string
	}{
		{"empty", ""},
		{"whitespace only", "   "},
		{"unreachable host", "postgres://portal@127.0.0.1:1/portal?sslmode=disable&connect_timeout=1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("TEST_LIVE_DSN", tc.dsn)
			conn, err := Connect("TEST_LIVE_DSN")
			if conn != nil {
				conn.Close()
				t.Fatal("Connect() returned a connection, want nil")
			}
			if err == nil {
				t.Fatal("Connect() in live mode returned (nil, nil): handlers would serve mock data")
			}
		})
	}
}

// A DSN made only of whitespace is the same as an unset one, so mock mode still
// gets its fixture signal rather than an error.
func TestConnectTreatsWhitespaceDSNAsUnsetInMockMode(t *testing.T) {
	t.Setenv("PORTAL_API_MODE", "mock")
	t.Setenv("APP_ENV", "development")
	t.Setenv("TEST_MOCK_DSN", "   ")

	conn, err := Connect("TEST_MOCK_DSN")
	if err != nil {
		t.Fatalf("Connect() = %v, want nil error in mock mode", err)
	}
	if conn != nil {
		conn.Close()
		t.Fatal("Connect() returned a connection, want nil")
	}
}
