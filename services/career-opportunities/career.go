package career

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"henukit.dev/career/internal/contract"
)

var requestIDPattern = regexp.MustCompile(`^req_[A-Za-z0-9_-]+$`)

// WorkFunc executes the actual crawling+matching work for one claimed search.
// #392 ships a deterministic placeholder that turns a frozen profile snapshot
// into a normalized result; #396 swaps in the real GetWork crawler behind this
// same seam. The worker only ever calls it after freezing status to running.
type WorkFunc func(ctx context.Context, profile any) (WorkResult, error)

type Config struct {
	Database *pgxpool.Pool
	Redis    redis.UniversalClient
	ClientID string
	Keys     map[string]string
	Work     WorkFunc
	// DigestSender is the #397 enqueue boundary: the worker posts one
	// Opportunity Digest per completed search through this seam. Nil disables
	// digest mail entirely (production-safe off state).
	DigestSender DigestSender
	// DigestResultURL is the public Career result page base URL used in digest
	// mail; when empty, digests omit the result link.
	DigestResultURL string
}

type service struct {
	database        *pgxpool.Pool
	redis           redis.UniversalClient
	clientID        string
	keys            map[string]string
	work            WorkFunc
	now             func() time.Time
	digestSender    DigestSender
	digestResultURL string
}

type actor struct{ userID string }

type contextKey string

const (
	requestIDKey contextKey = "request-id"
	actorKey     contextKey = "actor"
	nonceTTL                = 5 * time.Minute
)

func New(config Config) (*Service, error) {
	if config.Database == nil || config.Redis == nil || config.ClientID == "" || len(config.Keys) == 0 || len(config.Keys) > 2 {
		return nil, errors.New("career database and service credentials are required")
	}
	for keyID, secret := range config.Keys {
		if keyID == "" || len(secret) < 32 {
			return nil, errors.New("career service key ring is invalid")
		}
	}
	work := config.Work
	if work == nil {
		work = NewGetWorkWork(GetWorkConfig{})
	}
	if config.DigestResultURL != "" && !validDigestResultURL(config.DigestResultURL) {
		return nil, errors.New("career digest result URL must be an http(s) URL")
	}
	h := &service{database: config.Database, redis: config.Redis, clientID: config.ClientID, keys: config.Keys, work: work, now: time.Now, digestSender: config.DigestSender, digestResultURL: config.DigestResultURL}
	router := chi.NewRouter()
	router.Use(h.requestContext)
	router.Get(contract.HealthRoute, func(w http.ResponseWriter, r *http.Request) {
		writeData(w, r, http.StatusOK, map[string]bool{"ok": true})
	})
	router.Group(func(protected chi.Router) {
		protected.Use(h.authenticate)
		protected.Post(contract.CreateSearchRoute, h.createSearch)
		protected.Get(contract.SearchRoute, h.searchStatus)
		protected.Get(contract.ListSearchesRoute, h.listSearches)
		protected.Get(contract.ProfileRoute, h.getProfile)
		protected.Put(contract.UpdateProfileRoute, h.updateProfile)
	})
	return &Service{h: h, router: router}, nil
}

// Service is the running Career HTTP service plus its background worker. It
// implements http.Handler; its Claims worker drives queued searches.
type Service struct {
	h      *service
	router *chi.Mux
}

func (s *Service) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.router.ServeHTTP(w, r) }

// Claims returns the worker that drives queued searches to completion. The
// server starts it in the background; tests drive it one step at a time.
func (s *Service) Claims() *worker { return &worker{h: s.h} }

func (h *service) requireActor(w http.ResponseWriter, r *http.Request) (actor, bool) {
	value, ok := r.Context().Value(actorKey).(actor)
	if !ok || value.userID == "" {
		writeError(w, r, http.StatusUnauthorized, "INVALID_ACTOR", "actor context is invalid")
		return actor{}, false
	}
	return value, true
}

func decode(w http.ResponseWriter, r *http.Request, target any) ([]byte, bool) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "request body is invalid")
		return nil, false
	}
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "request body is invalid")
		return nil, false
	}
	return body, true
}

func requestID(r *http.Request) string {
	value, _ := r.Context().Value(requestIDKey).(string)
	return value
}
func writeData(w http.ResponseWriter, r *http.Request, status int, data any) {
	writeJSON(w, status, map[string]any{"data": data, "request_id": requestID(r)})
}
func writeError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}, "request_id": requestID(r)})
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func (h *service) requestContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-Id")
		if len(id) > 120 || !requestIDPattern.MatchString(id) {
			id = "req_" + strings.ReplaceAll(uuid.NewString(), "-", "")
		}
		w.Header().Set("X-Request-Id", id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey, id)))
	})
}

// authenticate verifies the signed request the same way Food does, then binds
// the gateway-verified X-Actor-User-Id. The browser never supplies the actor;
// the Portal Gateway signs it, so a search is always owned by the real actor.
func (h *service) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientID, basicSecret, basic := r.BasicAuth()
		secret, keyKnown := h.keys[r.Header.Get("X-Key-Id")]
		if !basic || clientID != h.clientID || r.Header.Get("X-Service-Id") != clientID || !keyKnown || !hmac.Equal([]byte(secret), []byte(basicSecret)) {
			writeError(w, r, http.StatusUnauthorized, "INVALID_SERVICE_AUTH", "service credentials are invalid")
			return
		}
		timestamp, err := strconv.ParseInt(r.Header.Get("X-Timestamp"), 10, 64)
		if err != nil || abs(h.now().Unix()-timestamp) > int64(nonceTTL/time.Second) {
			writeError(w, r, http.StatusUnauthorized, "INVALID_SERVICE_AUTH", "service timestamp is invalid")
			return
		}
		nonce := r.Header.Get("X-Nonce")
		decoded, err := base64.RawURLEncoding.DecodeString(nonce)
		if err != nil || len(decoded) != 24 {
			writeError(w, r, http.StatusUnauthorized, "INVALID_SERVICE_AUTH", "service nonce is invalid")
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 512<<10))
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "request body is too large")
			return
		}
		r.Body = io.NopCloser(strings.NewReader(string(body)))
		digest := sha256.Sum256(body)
		canonical := strings.Join([]string{r.Method, r.URL.RequestURI(), r.Header.Get("X-Timestamp"), nonce, hex.EncodeToString(digest[:])}, "\n")
		mac := hmac.New(sha256.New, []byte(secret))
		_, _ = mac.Write([]byte(canonical))
		if !hmac.Equal([]byte(r.Header.Get("X-Signature")), []byte(base64.RawURLEncoding.EncodeToString(mac.Sum(nil)))) {
			writeError(w, r, http.StatusUnauthorized, "INVALID_SERVICE_AUTH", "service signature is invalid")
			return
		}
		accepted, err := h.redis.SetNX(r.Context(), "career:nonce:"+clientID+":"+nonce, "1", nonceTTL).Result()
		if err != nil {
			writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "nonce store is unavailable")
			return
		}
		if !accepted {
			writeError(w, r, http.StatusConflict, "REPLAY_DETECTED", "service nonce was already used")
			return
		}
		userID := r.Header.Get("X-Actor-User-Id")
		if _, err := uuid.Parse(userID); err != nil {
			writeError(w, r, http.StatusUnauthorized, "INVALID_ACTOR", "actor context is invalid")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), actorKey, actor{userID: userID})))
	})
}

func abs(value int64) int64 {
	if value < 0 {
		return -value
	}
	return value
}
