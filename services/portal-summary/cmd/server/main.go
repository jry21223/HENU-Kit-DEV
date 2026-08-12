package main

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"

	"henukit.dev/portal-summary/internal/contract"
	"henukit.dev/portal-summary/internal/httpapi"
	"henukit.dev/portal-summary/internal/summary"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if len(os.Args) == 2 && os.Args[1] == "verify-summary" {
		if err := verifySummary(); err != nil {
			logger.Error("verify_summary", "error", err)
			os.Exit(1)
		}
		return
	}
	deployedAt, err := time.Parse(time.RFC3339, os.Getenv("PORTAL_DEPLOYED_AT"))
	if err != nil {
		logger.Error("invalid_deployed_at", "error", err)
		os.Exit(1)
	}
	keyProbes, err := probes("PORTAL_KEY_PROBES_JSON")
	if err != nil {
		logger.Error("invalid_key_probes", "error", err)
		os.Exit(1)
	}
	entryProbes, err := probes("PORTAL_ENTRY_PROBES_JSON")
	if err != nil {
		logger.Error("invalid_entry_probes", "error", err)
		os.Exit(1)
	}
	service, err := summary.New(summary.Config{
		Version: os.Getenv("PORTAL_VERSION"), CommitSHA: os.Getenv("PORTAL_COMMIT_SHA"), DeployedAt: deployedAt,
		ReadinessURL: os.Getenv("PORTAL_READINESS_URL"), KeyProbes: keyProbes, EntryProbes: entryProbes, FeedbackURL: os.Getenv("PORTAL_FEEDBACK_SUMMARY_URL"),
		FeedbackCredentials: summary.Credentials{ClientID: os.Getenv("PORTAL_FEEDBACK_CLIENT_ID"), ClientSecret: os.Getenv("PORTAL_FEEDBACK_CLIENT_SECRET"), KeyID: os.Getenv("PORTAL_FEEDBACK_KEY_ID")},
	}, &http.Client{})
	if err != nil {
		logger.Error("invalid_summary_config", "error", err)
		os.Exit(1)
	}
	redisAddress := strings.TrimSpace(os.Getenv("REDIS_ADDR"))
	if redisAddress == "" {
		logger.Error("invalid_redis_config", "error", "REDIS_ADDR is required")
		os.Exit(1)
	}
	redisClient := redis.NewClient(&redis.Options{Addr: redisAddress})
	keys, err := keyRing(
		os.Getenv("PORTAL_SUMMARY_ACTIVE_KEY_ID"), os.Getenv("PORTAL_SUMMARY_ACTIVE_SECRET"),
		os.Getenv("PORTAL_SUMMARY_RETIRING_KEY_ID"), os.Getenv("PORTAL_SUMMARY_RETIRING_SECRET"),
	)
	if err != nil {
		logger.Error("invalid_key_ring", "error", err)
		os.Exit(1)
	}
	feedbackSecret := os.Getenv("PORTAL_FEEDBACK_CLIENT_SECRET")
	for _, secret := range keys {
		if feedbackSecret != "" && feedbackSecret == secret {
			logger.Error("invalid_key_separation", "error", "feedback and Gateway verifier secrets must differ")
			os.Exit(1)
		}
	}
	handler, err := httpapi.New(httpapi.Config{ClientID: os.Getenv("PORTAL_SUMMARY_CLIENT_ID"), Keys: keys}, redisClient, service)
	if err != nil {
		logger.Error("invalid_http_config", "error", err)
		os.Exit(1)
	}
	address := strings.TrimSpace(os.Getenv("LISTEN_ADDR"))
	if address == "" {
		address = ":8083"
	}
	server := &http.Server{Addr: address, Handler: handler, ReadHeaderTimeout: 5 * time.Second}
	logger.Info("portal_summary_listen", "address", address)
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("serve", "error", err)
			os.Exit(1)
		}
	}()
	shutdown, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-shutdown.Done()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
}

func verifySummary() error {
	target := strings.TrimSpace(os.Getenv("PORTAL_SUMMARY_VERIFY_URL"))
	if target == "" {
		target = "http://127.0.0.1:8083/api/v1/console-summary"
	}
	request, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		return err
	}
	clientID := strings.TrimSpace(os.Getenv("PORTAL_SUMMARY_CLIENT_ID"))
	keyID := strings.TrimSpace(os.Getenv("PORTAL_SUMMARY_ACTIVE_KEY_ID"))
	secret := os.Getenv("PORTAL_SUMMARY_ACTIVE_SECRET")
	if clientID == "" || keyID == "" || len(secret) < 16 {
		return errors.New("Portal summary verification credentials are incomplete")
	}
	nonceBytes := make([]byte, 24)
	if _, err := rand.Read(nonceBytes); err != nil {
		return err
	}
	timestamp := fmt.Sprintf("%d", time.Now().Unix())
	nonce := base64.RawURLEncoding.EncodeToString(nonceBytes)
	digest := sha256.Sum256(nil)
	canonical := strings.Join([]string{request.Method, request.URL.RequestURI(), timestamp, nonce, hex.EncodeToString(digest[:])}, "\n")
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(canonical))
	request.SetBasicAuth(clientID, secret)
	request.Header.Set("X-Service-Id", clientID)
	request.Header.Set("X-Key-Id", keyID)
	request.Header.Set("X-Timestamp", timestamp)
	request.Header.Set("X-Nonce", nonce)
	request.Header.Set("X-Signature", base64.RawURLEncoding.EncodeToString(mac.Sum(nil)))
	request.Header.Set("X-Request-Id", "req_portal_release_verify")
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("Portal summary returned HTTP %d", response.StatusCode)
	}
	var envelope contract.PortalSummaryEnvelope
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		return err
	}
	return contract.ValidatePortalSummaryEnvelope(envelope)
}

func keyRing(activeID, activeSecret, retiringID, retiringSecret string) (map[string]string, error) {
	activeID = strings.TrimSpace(activeID)
	retiringID = strings.TrimSpace(retiringID)
	if activeID == "" {
		return nil, errors.New("active key ID is required")
	}
	if retiringID != "" && retiringID == activeID {
		return nil, errors.New("active and retiring key IDs must differ")
	}
	keys := map[string]string{activeID: activeSecret}
	if retiringID != "" {
		keys[retiringID] = retiringSecret
	}
	return keys, nil
}

func probes(name string) ([]summary.Probe, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return []summary.Probe{}, nil
	}
	var probes []summary.Probe
	err := json.Unmarshal([]byte(value), &probes)
	return probes, err
}
