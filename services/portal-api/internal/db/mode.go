package db

import (
	"os"
	"strings"
)

const (
	ModeLive = "live"
	ModeMock = "mock"
)

// Mode returns the portal-api operating mode.
// PORTAL_API_MODE wins when set to live|mock.
// Otherwise production defaults to live; all other APP_ENV values default to mock.
func Mode() string {
	raw := strings.ToLower(strings.TrimSpace(os.Getenv("PORTAL_API_MODE")))
	switch raw {
	case ModeLive, ModeMock:
		return raw
	}

	appEnv := strings.ToLower(strings.TrimSpace(os.Getenv("APP_ENV")))
	if appEnv == "production" || appEnv == "prod" {
		return ModeLive
	}
	return ModeMock
}

// IsLive reports whether the API must fail closed (no mock success paths).
func IsLive() bool {
	return Mode() == ModeLive
}
