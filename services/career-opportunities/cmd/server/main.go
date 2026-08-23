package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	career "henukit.dev/career"
)

const approvedInsecureAIURL = "http://125.46.96.207:30000/v1"

func main() {
	databaseURL, redisURL := os.Getenv("CAREER_DATABASE_URL"), os.Getenv("CAREER_REDIS_URL")
	clientID, keyID, secret := os.Getenv("CAREER_SERVICE_CLIENT_ID"), os.Getenv("CAREER_SERVICE_KEY_ID"), os.Getenv("CAREER_SERVICE_SECRET")
	if databaseURL == "" || redisURL == "" || clientID == "" || keyID == "" || secret == "" {
		log.Fatal("Career configuration is incomplete")
	}
	pool, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	redisOptions, err := redis.ParseURL(redisURL)
	if err != nil {
		log.Fatal(err)
	}
	redisClient := redis.NewClient(redisOptions)
	defer redisClient.Close()
	work, err := buildWork()
	if err != nil {
		log.Fatal(err)
	}
	extractor, err := buildExtractor()
	if err != nil {
		log.Fatal(err)
	}
	suifier, err := buildSuifier()
	if err != nil {
		log.Fatal(err)
	}
	if strings.TrimSpace(os.Getenv("CAREER_REQUIRE_AI")) == "1" {
		probeContext, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		if err := probeExtractor(probeContext, extractor); err != nil {
			log.Fatalf("Career extraction LLM startup probe failed: %v", err)
		}
	}
	service, err := career.New(career.Config{
		Database: pool, Redis: redisClient, ClientID: clientID, Keys: map[string]string{keyID: secret},
		Work:              work,
		Extract:           extractor,
		Suify:             suifier,
		ExtractRateLimit:  intEnv("CAREER_EXTRACT_RATE_LIMIT", 5),
		SuifyRateLimit:    intEnv("CAREER_SUIFY_RATE_LIMIT", 5),
		SearchRateLimit:   intEnv("CAREER_SEARCH_RATE_LIMIT", 10),
		SearchActiveLimit: intEnv("CAREER_SEARCH_ACTIVE_LIMIT", 1),
		DigestSender:      digestSender(),
		DigestResultURL:   strings.TrimSpace(os.Getenv("CAREER_RESULT_URL")),
	})
	if err != nil {
		log.Fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		if err := service.Claims().Run(ctx); err != nil && err != context.Canceled {
			log.Printf("career worker stopped: %v", err)
		}
	}()
	address := strings.TrimSpace(os.Getenv("CAREER_ADDR"))
	if address == "" {
		address = ":8097"
	}
	server := &http.Server{Addr: address, Handler: service, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 70 * time.Second, IdleTimeout: 60 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	log.Printf("Career service listening on %s", address)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

// buildWork turns the operator-owned allowlist into the exact registered
// official sources. Unknown keys fail startup instead of producing a healthy
// service that can only return empty results.
func buildWork() (career.WorkFunc, error) {
	raw := strings.TrimSpace(os.Getenv("CAREER_SOURCE_ALLOWLIST"))
	if raw == "" {
		return career.NewGetWorkWork(career.GetWorkConfig{}), nil
	}
	allowlist := map[string]bool{}
	for _, value := range strings.Split(raw, ",") {
		key := strings.TrimSpace(value)
		if key == "" {
			continue
		}
		if key != career.MeituanSourceKey {
			return nil, fmt.Errorf("CAREER_SOURCE_ALLOWLIST contains unsupported source %q", key)
		}
		allowlist[key] = true
	}
	if len(allowlist) == 0 {
		return career.NewGetWorkWork(career.GetWorkConfig{}), nil
	}
	meituan, err := career.NewMeituanSource(
		strings.TrimSpace(os.Getenv("CAREER_MEITUAN_API_URL")),
		&http.Client{Timeout: 20 * time.Second},
	)
	if err != nil {
		return nil, err
	}
	return career.NewGetWorkWork(career.GetWorkConfig{
		AllowSources: allowlist,
		Sources:      []career.Source{meituan},
	}), nil
}

// buildExtractor wires the resume AI extraction seam from the operator's
// environment. CAREER_AI_MODE=mock selects the deterministic test extractor;
// otherwise CAREER_AI_BASE_URL / CAREER_AI_API_KEY / CAREER_AI_MODEL configure
// a custom OpenAI-compatible provider. With neither set the seam stays nil
// (production-safe off state): uploads are rejected with a clear error instead
// of pretending to work.
func buildExtractor() (career.ExtractFunc, error) {
	provider, err := loadCareerAIProvider()
	if err != nil {
		return nil, err
	}
	if provider.mode == "mock" {
		if provider.required {
			return nil, errors.New("production Career cannot use the mock extraction LLM")
		}
		return career.NewMockExtractor(), nil
	}
	if !provider.configured {
		if provider.required {
			return nil, errors.New("production Career extraction LLM is not configured")
		}
		return nil, nil
	}
	extractor, err := career.NewOpenAICompatibleExtractor(career.ExtractConfig{
		BaseURL: provider.baseURL, APIKey: provider.apiKey, Model: provider.model,
	})
	if err != nil {
		return nil, fmt.Errorf("career AI extractor configuration is invalid: %w", err)
	}
	return extractor, nil
}

// buildSuifier wires the entertainment rewrite to the same operator-owned
// Career LLM as resume extraction. It never accepts browser-selected provider
// configuration and remains disabled when the shared provider is absent.
func buildSuifier() (career.SuifyFunc, error) {
	provider, err := loadCareerAIProvider()
	if err != nil {
		return nil, err
	}
	allowInsecureSuify := strings.TrimSpace(os.Getenv("CAREER_SUIFY_ALLOW_INSECURE_AI_HTTP"))
	if allowInsecureSuify != "" && allowInsecureSuify != "0" && allowInsecureSuify != "1" {
		return nil, errors.New("CAREER_SUIFY_ALLOW_INSECURE_AI_HTTP must be 0 or 1")
	}
	if provider.mode == "mock" {
		if provider.required {
			return nil, errors.New("production Career cannot use the mock suification LLM")
		}
		return career.NewMockSuifier(), nil
	}
	if !provider.configured {
		return nil, nil
	}
	if err := validateSuificationTransport(provider.baseURL, allowInsecureSuify == "1"); err != nil {
		return nil, err
	}
	if strings.TrimRight(provider.baseURL, "/") == approvedInsecureAIURL && allowInsecureSuify != "1" {
		// Extraction may keep using the existing operator-approved exception,
		// while Suification stays safely unavailable until its separate data
		// disclosure decision is explicitly enabled.
		return nil, nil
	}
	suifier, err := career.NewOpenAICompatibleSuifier(career.SuifyConfig{
		BaseURL: provider.baseURL, APIKey: provider.apiKey, Model: provider.model,
	})
	if err != nil {
		return nil, fmt.Errorf("career AI suifier configuration is invalid: %w", err)
	}
	return suifier, nil
}

type careerAIProvider struct {
	required   bool
	configured bool
	mode       string
	baseURL    string
	apiKey     string
	model      string
}

func loadCareerAIProvider() (careerAIProvider, error) {
	required := strings.TrimSpace(os.Getenv("CAREER_REQUIRE_AI"))
	if required != "" && required != "0" && required != "1" {
		return careerAIProvider{}, errors.New("CAREER_REQUIRE_AI must be 0 or 1")
	}
	allowInsecureHTTP := strings.TrimSpace(os.Getenv("CAREER_ALLOW_INSECURE_AI_HTTP"))
	if allowInsecureHTTP != "" && allowInsecureHTTP != "0" && allowInsecureHTTP != "1" {
		return careerAIProvider{}, errors.New("CAREER_ALLOW_INSECURE_AI_HTTP must be 0 or 1")
	}
	provider := careerAIProvider{
		required: required == "1",
		mode:     strings.TrimSpace(os.Getenv("CAREER_AI_MODE")),
		baseURL:  strings.TrimSpace(os.Getenv("CAREER_AI_BASE_URL")),
		apiKey:   os.Getenv("CAREER_AI_API_KEY"),
		model:    os.Getenv("CAREER_AI_MODEL"),
	}
	if provider.mode != "" && provider.mode != "mock" {
		return careerAIProvider{}, fmt.Errorf("unsupported CAREER_AI_MODE %q", provider.mode)
	}
	provider.configured = provider.baseURL != "" || strings.TrimSpace(provider.apiKey) != "" || strings.TrimSpace(provider.model) != ""
	if provider.mode == "mock" || !provider.configured || !provider.required {
		return provider, nil
	}
	if err := validateProductionAIConfig(provider.baseURL, provider.apiKey, provider.model, allowInsecureHTTP == "1"); err != nil {
		return careerAIProvider{}, err
	}
	return provider, nil
}

func validateSuificationTransport(baseURL string, allowInsecureHTTP bool) error {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("Career suification LLM URL is invalid")
	}
	host := parsed.Hostname()
	loopback := strings.EqualFold(host, "localhost")
	if address := net.ParseIP(host); address != nil {
		loopback = address.IsLoopback()
	}
	approvedException := strings.TrimRight(parsed.String(), "/") == approvedInsecureAIURL
	if allowInsecureHTTP && !approvedException {
		return errors.New("CAREER_SUIFY_ALLOW_INSECURE_AI_HTTP=1 is valid only for the exact approved HTTP endpoint")
	}
	if parsed.Scheme == "https" || (parsed.Scheme == "http" && loopback) || approvedException {
		return nil
	}
	return errors.New("Career suification LLM must use HTTPS, loopback, or the exact approved HTTP endpoint")
}

func validateProductionAIConfig(baseURL, apiKey, model string, allowInsecureHTTP bool) error {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("production Career extraction LLM URL is invalid")
	}
	host := parsed.Hostname()
	localHTTP := strings.EqualFold(host, "localhost")
	if address := net.ParseIP(host); address != nil {
		localHTTP = address.IsLoopback()
	}
	approvedException := allowInsecureHTTP && strings.TrimRight(parsed.String(), "/") == approvedInsecureAIURL
	if parsed.Scheme != "https" && !(parsed.Scheme == "http" && localHTTP) && !approvedException {
		return errors.New("production Career extraction LLM must use HTTPS, loopback, or the explicitly approved HTTP endpoint")
	}
	values := []string{strings.ToLower(baseURL), strings.ToLower(apiKey), strings.ToLower(model)}
	for _, marker := range []string{"your-ai-provider", "replace-career", "your-model", "change-me", "example.test"} {
		for _, value := range values {
			if strings.Contains(value, marker) {
				return errors.New("production Career extraction LLM still contains placeholder configuration")
			}
		}
	}
	return nil
}

func probeExtractor(ctx context.Context, extractor career.ExtractFunc) error {
	if extractor == nil {
		return errors.New("production Career extraction LLM is not configured")
	}
	const probePDF = "JVBERi0xLjQKMSAwIG9iago8PCAvVHlwZSAvQ2F0YWxvZyAvUGFnZXMgMiAwIFIgPj4KZW5kb2JqCjIgMCBvYmoKPDwgL1R5cGUgL1BhZ2VzIC9LaWRzIFszIDAgUl0gL0NvdW50IDEgPj4KZW5kb2JqCjMgMCBvYmoKPDwgL1R5cGUgL1BhZ2UgL1BhcmVudCAyIDAgUiAvTWVkaWFCb3ggWzAgMCA2MTIgNzkyXSAvUmVzb3VyY2VzIDw8IC9Gb250IDw8IC9GMSA0IDAgUiA+PiA+PiAvQ29udGVudHMgNSAwIFIgPj4KZW5kb2JqCjQgMCBvYmoKPDwgL1R5cGUgL0ZvbnQgL1N1YnR5cGUgL1R5cGUxIC9CYXNlRm9udCAvSGVsdmV0aWNhID4+CmVuZG9iago1IDAgb2JqCjw8IC9MZW5ndGggMTEwID4+CnN0cmVhbQpCVCAvRjEgMTIgVGYgNzIgNzIwIFRkIChUYXJnZXQgcm9sZTogR28gYmFja2VuZCBkZXZlbG9wZXIuIFNraWxsczogR28sIFBvc3RncmVTUUwuIExvY2F0aW9uOiBaaGVuZ3pob3UuKSBUaiBFVAplbmRzdHJlYW0KZW5kb2JqCnhyZWYKMCA2CjAwMDAwMDAwMDAgNjU1MzUgZiAKMDAwMDAwMDAwOSAwMDAwMCBuIAowMDAwMDAwMDU4IDAwMDAwIG4gCjAwMDAwMDAxMTUgMDAwMDAgbiAKMDAwMDAwMDI0MSAwMDAwMCBuIAowMDAwMDAwMzExIDAwMDAwIG4gCnRyYWlsZXIKPDwgL1NpemUgNiAvUm9vdCAxIDAgUiA+PgpzdGFydHhyZWYKNDcyCiUlRU9GCg=="
	content, err := base64.StdEncoding.DecodeString(probePDF)
	if err != nil {
		return errors.New("production Career extraction LLM probe fixture is invalid")
	}
	profile, err := extractor(ctx, "startup-probe.pdf", content)
	if err != nil {
		return err
	}
	if strings.TrimSpace(profile.TargetRoles) == "" {
		return errors.New("production Career extraction LLM probe returned no target role")
	}
	return nil
}

func intEnv(name string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		log.Fatalf("%s must be a non-negative integer", name)
	}
	return parsed
}

// digestSender builds the #397 Platform Core digest client from the shared
// credential triple. An endpoint without credentials fails closed at startup;
// with no endpoint at all the digest seam stays disabled and no digest mail is
// enqueued.
func digestSender() career.DigestSender {
	endpoint := strings.TrimSpace(os.Getenv("PLATFORM_CORE_CAREER_DIGEST_URL"))
	if endpoint == "" {
		return nil
	}
	clientID := os.Getenv("PLATFORM_CORE_CAREER_DIGEST_CLIENT_ID")
	keyID := os.Getenv("PLATFORM_CORE_CAREER_DIGEST_KEY_ID")
	secret := os.Getenv("PLATFORM_CORE_CAREER_DIGEST_SECRET")
	sender, err := career.NewHTTPDigestSender(endpoint, clientID, secret, keyID, &http.Client{Timeout: 10 * time.Second})
	if err != nil {
		log.Fatalf("career digest sender configuration is invalid: %v", err)
	}
	return sender
}
