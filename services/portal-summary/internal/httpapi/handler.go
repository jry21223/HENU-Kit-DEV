package httpapi

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/redis/go-redis/v9"

	"henukit.dev/portal-summary/internal/contract"
	"henukit.dev/portal-summary/internal/summary"
)

const nonceTTL = 2 * time.Minute

var requestIDPattern = regexp.MustCompile(`^req_[A-Za-z0-9_-]+$`)

type Config struct {
	ClientID string
	Keys     map[string]string
}

type Handler struct {
	config  Config
	redis   *redis.Client
	summary *summary.Service
	now     func() time.Time
}

func New(config Config, redisClient *redis.Client, service *summary.Service) (http.Handler, error) {
	if config.ClientID == "" || len(config.Keys) == 0 || len(config.Keys) > 2 || redisClient == nil || service == nil {
		return nil, errors.New("portal summary HTTP dependencies are required")
	}
	secrets := make(map[string]struct{}, len(config.Keys))
	for keyID, secret := range config.Keys {
		if keyID == "" || len(secret) < 16 {
			return nil, errors.New("portal summary key ring is invalid")
		}
		if _, duplicate := secrets[secret]; duplicate {
			return nil, errors.New("portal summary key ring secrets must be distinct")
		}
		secrets[secret] = struct{}{}
	}
	handler := &Handler{config: config, redis: redisClient, summary: service, now: time.Now}
	router := chi.NewRouter()
	router.Use(handler.requestContext)
	router.Get("/healthz", handler.health)
	router.Get("/readyz", handler.ready)
	router.Get("/api/v1/console-summary", handler.consoleSummary)
	return router, nil
}

func (h *Handler) health(writer http.ResponseWriter, request *http.Request) {
	writeSuccess(writer, request, http.StatusOK, map[string]bool{"ok": true})
}

func (h *Handler) ready(writer http.ResponseWriter, request *http.Request) {
	if err := h.redis.Ping(request.Context()).Err(); err != nil {
		writeError(writer, requestID(request), http.StatusServiceUnavailable, "NOT_READY", "nonce store is unavailable")
		return
	}
	writeSuccess(writer, request, http.StatusOK, map[string]bool{"ready": true})
}

func (h *Handler) consoleSummary(writer http.ResponseWriter, request *http.Request) {
	requestID := requestID(request)
	if authErr := h.authenticate(request); authErr != nil {
		writeError(writer, requestID, authErr.status, authErr.code, authErr.message)
		return
	}
	writer.Header().Set("Cache-Control", "no-store")
	result := h.summary.Build(request.Context())
	result.RequestID = requestID
	envelope := contract.PortalSummaryEnvelope{Data: result, RequestID: requestID}
	if err := contract.ValidatePortalSummaryEnvelope(envelope); err != nil {
		writeError(writer, requestID, http.StatusServiceUnavailable, contract.ErrorInvalidOwnerSummary, "Portal summary violates its versioned contract")
		return
	}
	writeJSON(writer, http.StatusOK, envelope)
}

func (h *Handler) requestContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		id := request.Header.Get("X-Request-Id")
		if len(id) > 120 || !requestIDPattern.MatchString(id) {
			id = generatedRequestID()
		}
		writer.Header().Set("X-Request-Id", id)
		next.ServeHTTP(writer, request.WithContext(context.WithValue(request.Context(), requestIDKey{}, id)))
	})
}

type requestIDKey struct{}

func generatedRequestID() string {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err == nil {
		return "req_" + hex.EncodeToString(value)
	}
	return "req_" + strconv.FormatInt(time.Now().UnixNano(), 36)
}

type authError struct {
	status        int
	code, message string
}

func (h *Handler) authenticate(request *http.Request) *authError {
	clientID, secret, basic := request.BasicAuth()
	keyID := request.Header.Get("X-Key-Id")
	selectedSecret, knownKey := h.config.Keys[keyID]
	if !basic || clientID != h.config.ClientID || !knownKey || !hmac.Equal([]byte(secret), []byte(selectedSecret)) || request.Header.Get("X-Service-Id") != clientID {
		return &authError{http.StatusUnauthorized, contract.ErrorInvalidServiceAuth, "service credentials are invalid"}
	}
	timestamp, err := strconv.ParseInt(request.Header.Get("X-Timestamp"), 10, 64)
	if err != nil || abs(h.now().Unix()-timestamp) > 60 {
		return &authError{http.StatusUnauthorized, contract.ErrorInvalidServiceAuth, "service timestamp is invalid"}
	}
	nonce := request.Header.Get("X-Nonce")
	decodedNonce, err := base64.RawURLEncoding.DecodeString(nonce)
	if err != nil || len(decodedNonce) != 24 {
		return &authError{http.StatusUnauthorized, contract.ErrorInvalidServiceAuth, "service nonce is invalid"}
	}
	digest := sha256.Sum256(nil)
	canonical := strings.Join([]string{request.Method, request.URL.RequestURI(), request.Header.Get("X-Timestamp"), nonce, hex.EncodeToString(digest[:])}, "\n")
	mac := hmac.New(sha256.New, []byte(selectedSecret))
	_, _ = mac.Write([]byte(canonical))
	want := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(request.Header.Get("X-Signature")), []byte(want)) {
		return &authError{http.StatusUnauthorized, contract.ErrorInvalidServiceAuth, "service signature is invalid"}
	}
	claimed, err := h.redis.SetNX(request.Context(), "portal-summary:nonce:"+clientID+":"+nonce, "1", nonceTTL).Result()
	if err != nil {
		return &authError{http.StatusServiceUnavailable, contract.ErrorDependencyUnavailable, "nonce store is unavailable"}
	}
	if !claimed {
		return &authError{http.StatusConflict, contract.ErrorReplayDetected, "service nonce was already used"}
	}
	return nil
}

func requestID(request *http.Request) string {
	value, _ := request.Context().Value(requestIDKey{}).(string)
	return value
}
func abs(value int64) int64 {
	if value < 0 {
		return -value
	}
	return value
}

func writeError(writer http.ResponseWriter, requestID string, status int, code, message string) {
	writeJSON(writer, status, map[string]any{"error": map[string]string{"code": code, "message": message}, "request_id": requestID})
}

func writeSuccess(writer http.ResponseWriter, request *http.Request, status int, data any) {
	writeJSON(writer, status, map[string]any{"data": data, "request_id": requestID(request)})
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}
