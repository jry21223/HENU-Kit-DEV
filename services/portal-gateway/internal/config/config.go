package config

import (
	"fmt"
	"net/http"
	"os"
	"strings"
)

type ServiceAuth struct {
	ClientID     string
	ClientSecret string
	KeyID        string
}

type Config struct {
	ListenAddr string

	PlatformCoreURL       string
	PlatformCorePublicURL string
	PlatformClientID      string
	PlatformSecret        string
	PlatformKeyID         string
	PortalRedirectURI     string
	SessionKey            []byte

	RedisURL string

	PortalAPIURL        string
	LibraryDownloadURL  string
	LibraryDownloadAuth ServiceAuth

	PracticeURL string
	// QuizCraftCatalogEnabled controls only the catalog read client's
	// existence, not the route: /api/v1/practice/catalog is registered
	// unconditionally (ADR-0036) and fails closed with an honest 404 while the
	// client is nil. It stays default-off and independent of account-bound
	// stats, and keeps its own read credential pair (PracticeAuth) distinct
	// from the V2 read pair (QuizCraftCoreAuth).
	QuizCraftCatalogEnabled bool
	NoticeURL               string
	AccountPortfolioURL     string

	// QuizCraftV2ReadsEnabled is the single V2 read gate for the Core-derived
	// rankings, personal stats, favorites, and feedback-status clients. The
	// routes themselves are registered unconditionally (ADR-0036) and fail
	// closed while the client is nil. It is deliberately false unless the
	// deployment supplies an explicit "1"; #166 owns enabling this V2 read
	// path together with the browser bake flags.
	QuizCraftV2ReadsEnabled bool
	QuizCraftCoreURL        string
	QuizCraftCoreAuth       ServiceAuth

	PracticeAuth ServiceAuth
	// PracticeCommandAuth is deliberately distinct from PracticeAuth. The
	// latter is reserved for read-only product contracts; the former is the
	// narrowly scoped, default-off Portal -> QuizCraft command capability.
	PracticeCommandAuth  ServiceAuth
	NoticeAuth           ServiceAuth
	AccountPortfolioAuth ServiceAuth
	// PracticeCommandsEnabled defaults to false. #166 is the only cutover
	// window allowed to enable browser-visible QuizCraft writes.
	PracticeCommandsEnabled bool

	// FoodPostsURL and its two independent credential triples are optional.
	// An empty URL leaves the Food Post boundary unconfigured and every Food
	// Post route fails closed with an honest 503; there is no _ENABLED flag.
	FoodPostsURL       string
	FoodPostCreateAuth ServiceAuth
	FoodPostReadAuth   ServiceAuth

	// CareerURL and its actor-bound credential triple are optional. An empty
	// URL leaves the Career boundary unconfigured and every Career route fails
	// closed with an honest 503; there is no _ENABLED flag.
	CareerURL  string
	CareerAuth ServiceAuth

	PortalOrigin           string
	LocalOAuthCookieName   string
	LocalSessionCookieName string
	TrustedProxyCIDRs      []string
}

func FromEnv() (Config, error) {
	sessionKey := envBytes("PORTAL_SESSION_KEY")
	if len(sessionKey) != 32 {
		return Config{}, fmt.Errorf("PORTAL_SESSION_KEY must be 32 bytes (base64)")
	}

	cfg := Config{
		ListenAddr:            envOrDefault("LISTEN_ADDR", ":8084"),
		PlatformCoreURL:       mustEnv("PLATFORM_CORE_URL"),
		PlatformCorePublicURL: envOrDefault("PLATFORM_CORE_PUBLIC_URL", ""),
		PlatformClientID:      mustEnv("PLATFORM_CLIENT_ID"),
		PlatformSecret:        mustEnv("PLATFORM_CLIENT_SECRET"),
		PlatformKeyID:         mustEnv("PLATFORM_KEY_ID"),
		PortalRedirectURI:     mustEnv("PORTAL_REDIRECT_URI"),
		SessionKey:            sessionKey,
		RedisURL:              envOrDefault("REDIS_URL", "redis://127.0.0.1:6379/2"),
		PortalAPIURL:          envOrDefault("PORTAL_API_URL", "http://127.0.0.1:8085"),
		LibraryDownloadURL:    strings.TrimSpace(os.Getenv("LIBRARY_DOWNLOAD_URL")),
		PracticeURL:           mustEnv("PRACTICE_SERVICE_URL"),
		// This remains dark by default and is aligned with the Portal build's
		// NEXT_PUBLIC_PORTAL_ENABLE_QUIZCRAFT_CATALOG flag: both default to 0
		// and both are set to 1 together in the #166 cutover bundle (see
		// scripts/ops/henukit-release-images.sh, which bakes 1 only for that
		// release build — never a default). The baked 1 without this server
		// flag is a deliberate fail-closed mismatch: the browser may render
		// the catalog surface, but the Gateway answers an honest 404/503 until
		// PORTAL_ENABLE_QUIZCRAFT_CATALOG is enabled server-side.
		QuizCraftCatalogEnabled: os.Getenv("PORTAL_ENABLE_QUIZCRAFT_CATALOG") == "1",
		NoticeURL:               mustEnv("NOTICE_SERVICE_URL"),
		AccountPortfolioURL:     mustEnv("ACCOUNT_PORTFOLIO_URL"),
		PracticeAuth: ServiceAuth{
			ClientID:     mustEnv("PRACTICE_CLIENT_ID"),
			ClientSecret: mustEnv("PRACTICE_CLIENT_SECRET"),
			KeyID:        mustEnv("PRACTICE_KEY_ID"),
		},
		PracticeCommandAuth: ServiceAuth{
			ClientID:     os.Getenv("PRACTICE_COMMAND_CLIENT_ID"),
			ClientSecret: os.Getenv("PRACTICE_COMMAND_CLIENT_SECRET"),
			KeyID:        os.Getenv("PRACTICE_COMMAND_KEY_ID"),
		},
		PracticeCommandsEnabled: os.Getenv("PORTAL_PRACTICE_COMMANDS_ENABLED") == "1",
		NoticeAuth: ServiceAuth{
			ClientID:     mustEnv("NOTICE_CLIENT_ID"),
			ClientSecret: mustEnv("NOTICE_CLIENT_SECRET"),
			KeyID:        mustEnv("NOTICE_KEY_ID"),
		},
		AccountPortfolioAuth: ServiceAuth{
			ClientID:     mustEnv("ACCOUNT_PORTFOLIO_CLIENT_ID"),
			ClientSecret: mustEnv("ACCOUNT_PORTFOLIO_CLIENT_SECRET"),
			KeyID:        mustEnv("ACCOUNT_PORTFOLIO_KEY_ID"),
		},
		LibraryDownloadAuth: ServiceAuth{
			ClientID:     strings.TrimSpace(os.Getenv("LIBRARY_DOWNLOAD_CLIENT_ID")),
			ClientSecret: os.Getenv("LIBRARY_DOWNLOAD_CLIENT_SECRET"),
			KeyID:        strings.TrimSpace(os.Getenv("LIBRARY_DOWNLOAD_KEY_ID")),
		},
		FoodPostsURL: strings.TrimSpace(os.Getenv("FOOD_POSTS_URL")),
		FoodPostCreateAuth: ServiceAuth{
			ClientID:     strings.TrimSpace(os.Getenv("FOOD_POST_CREATE_CLIENT_ID")),
			ClientSecret: os.Getenv("FOOD_POST_CREATE_SECRET"),
			KeyID:        strings.TrimSpace(os.Getenv("FOOD_POST_CREATE_KEY_ID")),
		},
		FoodPostReadAuth: ServiceAuth{
			ClientID:     strings.TrimSpace(os.Getenv("FOOD_POST_READ_CLIENT_ID")),
			ClientSecret: os.Getenv("FOOD_POST_READ_SECRET"),
			KeyID:        strings.TrimSpace(os.Getenv("FOOD_POST_READ_KEY_ID")),
		},
		CareerURL: strings.TrimSpace(os.Getenv("CAREER_URL")),
		CareerAuth: ServiceAuth{
			ClientID:     strings.TrimSpace(os.Getenv("CAREER_CLIENT_ID")),
			ClientSecret: os.Getenv("CAREER_CLIENT_SECRET"),
			KeyID:        strings.TrimSpace(os.Getenv("CAREER_KEY_ID")),
		},
		PortalOrigin:           mustEnv("PORTAL_ORIGIN"),
		LocalOAuthCookieName:   envOrDefault("PORTAL_LOCAL_OAUTH_COOKIE_NAME", "henukit_portal_oauth_local"),
		LocalSessionCookieName: envOrDefault("PORTAL_LOCAL_SESSION_COOKIE_NAME", "henukit_portal_session_local"),
		TrustedProxyCIDRs:      splitNonEmpty(os.Getenv("PORTAL_TRUSTED_PROXY_CIDRS")),
	}
	quizCraftReadsEnabled, quizCraftCoreURL, quizCraftCoreAuth, err := quizCraftV2ReadsFromEnv(os.Getenv)
	if err != nil {
		return Config{}, err
	}
	cfg.QuizCraftV2ReadsEnabled = quizCraftReadsEnabled
	cfg.QuizCraftCoreURL = quizCraftCoreURL
	cfg.QuizCraftCoreAuth = quizCraftCoreAuth
	if err := validateLocalCookieNames(cfg.LocalOAuthCookieName, cfg.LocalSessionCookieName); err != nil {
		return Config{}, err
	}
	if os.Getenv("ACCOUNT_PORTFOLIO_REQUIRE_STRONG_SECRET") == "1" && isPlaceholderSecret(cfg.AccountPortfolioAuth.ClientSecret) {
		return Config{}, fmt.Errorf("ACCOUNT_PORTFOLIO_CLIENT_SECRET is a deployment placeholder")
	}
	return cfg, nil
}

func isPlaceholderSecret(secret string) bool {
	value := strings.ToLower(strings.TrimSpace(secret))
	return strings.HasPrefix(value, "replace-") ||
		strings.HasPrefix(value, "change-me") ||
		strings.HasPrefix(value, "example-") ||
		strings.Contains(value, "placeholder")
}

func quizCraftV2ReadsFromEnv(getenv func(string) string) (bool, string, ServiceAuth, error) {
	switch strings.TrimSpace(getenv("PORTAL_ENABLE_QUIZCRAFT_V2_READS")) {
	case "", "0":
		return false, "", ServiceAuth{}, nil
	case "1":
	default:
		return false, "", ServiceAuth{}, fmt.Errorf("PORTAL_ENABLE_QUIZCRAFT_V2_READS must be 0 or 1")
	}
	coreURL := strings.TrimSpace(getenv("QUIZCRAFT_CORE_URL"))
	auth := ServiceAuth{
		ClientID:     strings.TrimSpace(getenv("QUIZCRAFT_PORTAL_CATALOG_CLIENT_ID")),
		ClientSecret: getenv("QUIZCRAFT_PORTAL_CATALOG_CLIENT_SECRET"),
		KeyID:        strings.TrimSpace(getenv("QUIZCRAFT_PORTAL_CATALOG_KEY_ID")),
	}
	if coreURL == "" || auth.ClientID == "" || auth.ClientSecret == "" || auth.KeyID == "" {
		return false, "", ServiceAuth{}, fmt.Errorf("QuizCraft V2 reads require QUIZCRAFT_CORE_URL and QUIZCRAFT_PORTAL_CATALOG_* credentials")
	}
	return true, coreURL, auth, nil
}

func validateLocalCookieNames(oauth, session string) error {
	if oauth == session {
		return fmt.Errorf("local OAuth and Session cookie names must be distinct")
	}
	for label, name := range map[string]string{"OAuth": oauth, "Session": session} {
		if strings.HasPrefix(name, "__Host-") {
			return fmt.Errorf("local %s cookie name must not use the __Host- prefix", label)
		}
		if err := (&http.Cookie{Name: name, Value: "valid", Path: "/"}).Valid(); err != nil {
			return fmt.Errorf("invalid local %s cookie name: %w", label, err)
		}
	}
	return nil
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		panic(fmt.Sprintf("missing required env var %s", key))
	}
	return v
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envBytes(key string) []byte {
	v := os.Getenv(key)
	return []byte(v)
}

func splitNonEmpty(value string) []string {
	var values []string
	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			values = append(values, item)
		}
	}
	return values
}
