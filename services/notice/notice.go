package notice

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"henukit.dev/notice/internal/contract"
)

const (
	nonceTTL = 5 * time.Minute
	// A 100,000-rune JSON string can occupy up to about 1.2 MiB when each
	// non-BMP rune is represented as a surrogate-pair escape. Two MiB retains
	// a bounded signed-request read while admitting the public body contract.
	noticeRequestBodyByteLimit  = 2 << 20
	noticeTitleRuneLimit        = 200
	noticeBodyRuneLimit         = 100000
	noticeSourceNameRuneLimit   = 120
	portalFeedResponseByteLimit = 5 << 20
)

var requestIDPattern = regexp.MustCompile(`^req_[A-Za-z0-9_-]+$`)

type Config struct {
	Database       *pgxpool.Pool
	Redis          redis.UniversalClient
	ClientID       string
	Keys           map[string]string
	PortalClientID string
	PortalKeys     map[string]string
}

type service struct {
	database       *pgxpool.Pool
	redis          redis.UniversalClient
	clientID       string
	keys           map[string]string
	portalClientID string
	portalKeys     map[string]string
	now            func() time.Time
	lifecycle      *lifecycleStore
}

type actor struct {
	userID, permission string
}

type contextKey string

const (
	requestIDKey contextKey = "request-id"
	actorKey     contextKey = "actor"
)

func New(config Config) (http.Handler, error) {
	if config.Database == nil || config.Redis == nil || config.ClientID == "" || config.PortalClientID == "" || config.ClientID == config.PortalClientID || len(config.Keys) == 0 || len(config.Keys) > 2 || len(config.PortalKeys) == 0 || len(config.PortalKeys) > 2 {
		return nil, errors.New("notice database and service credentials are required")
	}
	secrets := map[string]struct{}{}
	keyIDs := map[string]struct{}{}
	for _, keyring := range []map[string]string{config.Keys, config.PortalKeys} {
		for keyID, secret := range keyring {
			if keyID == "" || len(secret) < 32 {
				return nil, errors.New("notice service key ring is invalid")
			}
			if _, exists := keyIDs[keyID]; exists {
				return nil, errors.New("notice service key IDs must be distinct")
			}
			keyIDs[keyID] = struct{}{}
			if _, exists := secrets[secret]; exists {
				return nil, errors.New("notice service secrets must be distinct")
			}
			secrets[secret] = struct{}{}
		}
	}
	h := &service{database: config.Database, redis: config.Redis, clientID: config.ClientID, keys: config.Keys, portalClientID: config.PortalClientID, portalKeys: config.PortalKeys, now: time.Now, lifecycle: &lifecycleStore{database: config.Database}}
	router := chi.NewRouter()
	router.Use(h.requestContext)
	router.Get(contract.HealthRoute, func(w http.ResponseWriter, r *http.Request) {
		writeData(w, r, http.StatusOK, map[string]bool{"ok": true})
	})
	router.Group(func(protected chi.Router) {
		protected.Use(h.authenticateConsole)
		protected.Get(contract.ConsoleSummaryRoute, h.consoleSummary)
		protected.Get(contract.SnapshotRoute, h.snapshot)
		protected.Post(contract.SourceCreateRoute, h.createSource)
		protected.Post(contract.VersionCreateRoute, h.createVersion)
		protected.Post(contract.ReviewRoute, h.review)
		protected.Post(contract.DistributionRoute, h.distribute)
		protected.Get(contract.OperationRoute, h.operationStatus)
	})
	router.Group(func(protected chi.Router) {
		protected.Use(h.authenticatePortal)
		protected.Get(contract.PortalFeedRoute, h.portalFeed)
	})
	return router, nil
}

func (h *service) consoleSummary(w http.ResponseWriter, r *http.Request) {
	counts, err := h.lifecycle.summary(r.Context())
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Notice summary is unavailable")
		return
	}
	writeData(w, r, http.StatusOK, map[string]any{"id": "notice", "status": "ok", "status_message": "通知服务运行正常", "as_of": h.now().UTC(), "metrics": []map[string]any{{"label": "待审核", "value": fmt.Sprint(counts["pending_review"])}, {"label": "已批准", "value": fmt.Sprint(counts["approved"])}, {"label": "已分发", "value": fmt.Sprint(counts["distributed"])}}})
}

func (h *service) require(w http.ResponseWriter, r *http.Request, permission string) (actor, bool) {
	value, _ := r.Context().Value(actorKey).(actor)
	if value.permission != permission {
		writeError(w, r, http.StatusForbidden, "PERMISSION_DENIED", "required Notice permission is missing")
		return actor{}, false
	}
	return value, true
}

type sourceInput struct {
	Code         string `json:"code"`
	Name         string `json:"name"`
	CanonicalURL string `json:"canonical_url"`
}
type versionInput struct {
	Title             string     `json:"title"`
	Body              string     `json:"body"`
	SourceURL         string     `json:"source_url"`
	SourcePublishedAt *time.Time `json:"source_published_at"`
}
type reviewInput struct {
	Decision         string `json:"decision"`
	Note             string `json:"note"`
	ExpectedRevision int64  `json:"expected_revision"`
}
type distributionInput struct {
	Channel  string `json:"channel"`
	Audience struct {
		Kind  string  `json:"kind"`
		Value *string `json:"value"`
	} `json:"audience"`
	ExpectedRevision int64 `json:"expected_revision"`
}

func (h *service) createSource(w http.ResponseWriter, r *http.Request) {
	value, ok := h.require(w, r, "notice.manage")
	if !ok {
		return
	}
	var input sourceInput
	body, ok := decode(w, r, &input)
	if !ok {
		return
	}
	if !regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,62}$`).MatchString(input.Code) || strings.TrimSpace(input.Name) == "" || utf8.RuneCountInString(input.Name) > noticeSourceNameRuneLimit || !validPublicHTTPSURL(input.CanonicalURL) {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "source fields are invalid")
		return
	}
	h.writeOperation(w, r, "source_create", body, func(tx pgx.Tx) (map[string]any, error) {
		return h.lifecycle.createSource(r.Context(), tx, value, requestID(r), input)
	})
}

func (h *service) createVersion(w http.ResponseWriter, r *http.Request) {
	value, ok := h.require(w, r, "notice.manage")
	if !ok {
		return
	}
	sourceID, err := uuid.Parse(chi.URLParam(r, "source_id"))
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "source id is invalid")
		return
	}
	var input versionInput
	body, ok := decode(w, r, &input)
	if !ok {
		return
	}
	if strings.TrimSpace(input.Title) == "" || utf8.RuneCountInString(input.Title) > noticeTitleRuneLimit || strings.TrimSpace(input.Body) == "" || utf8.RuneCountInString(input.Body) > noticeBodyRuneLimit || !validPublicHTTPSURL(input.SourceURL) {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "version fields are invalid")
		return
	}
	h.writeOperation(w, r, "version_create", body, func(tx pgx.Tx) (map[string]any, error) {
		return h.lifecycle.createVersion(r.Context(), tx, value, requestID(r), sourceID, input)
	})
}

func (h *service) review(w http.ResponseWriter, r *http.Request) {
	value, ok := h.require(w, r, "notice.review")
	if !ok {
		return
	}
	versionID, err := uuid.Parse(chi.URLParam(r, "version_id"))
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "version id is invalid")
		return
	}
	var input reviewInput
	body, ok := decode(w, r, &input)
	if !ok {
		return
	}
	if input.Decision != "approved" && input.Decision != "rejected" {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "review decision is invalid")
		return
	}
	h.writeOperation(w, r, "review", body, func(tx pgx.Tx) (map[string]any, error) {
		return h.lifecycle.review(r.Context(), tx, value, requestID(r), versionID, input)
	})
}

func (h *service) distribute(w http.ResponseWriter, r *http.Request) {
	value, ok := h.require(w, r, "notice.distribute")
	if !ok {
		return
	}
	versionID, err := uuid.Parse(chi.URLParam(r, "version_id"))
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "version id is invalid")
		return
	}
	var input distributionInput
	body, ok := decode(w, r, &input)
	if !ok {
		return
	}
	if (input.Channel != "in_app" && input.Channel != "email") || (input.Audience.Kind != "all_students" && input.Audience.Kind != "college" && input.Audience.Kind != "role") || (input.Audience.Kind == "all_students" && input.Audience.Value != nil) || (input.Audience.Kind != "all_students" && (input.Audience.Value == nil || *input.Audience.Value == "")) {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "distribution fields are invalid")
		return
	}
	h.writeOperation(w, r, "distribution", body, func(tx pgx.Tx) (map[string]any, error) {
		return h.lifecycle.distribute(r.Context(), tx, value, requestID(r), versionID, input)
	})
}

func (h *service) snapshot(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.require(w, r, "notice.read"); !ok {
		return
	}
	items, err := h.lifecycle.snapshot(r.Context())
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Notice database is unavailable")
		return
	}
	writeData(w, r, http.StatusOK, map[string]any{"items": items, "generated_at": h.now().UTC()})
}

func (h *service) portalFeed(w http.ResponseWriter, r *http.Request) {
	items, err := h.lifecycle.portalFeed(r.Context(), requestID(r))
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Notice database is unavailable")
		return
	}
	writePortalFeed(w, r, items)
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
func abs(value int64) int64 {
	if value < 0 {
		return -value
	}
	return value
}
func writeData(w http.ResponseWriter, r *http.Request, status int, data any) {
	writeJSON(w, status, map[string]any{"data": data, "request_id": requestID(r)})
}

type portalFeedResponse struct {
	Data      portalFeedData `json:"data"`
	RequestID string         `json:"request_id"`
}

type portalFeedData struct {
	Notices []map[string]any `json:"notices"`
}

// portalFeedResponseBytes is both the exact Owner response representation and
// the source of truth for the feed's byte budget. Keeping selection and write
// serialization together prevents valid 100,000-rune UTF-8 bodies from
// exceeding Gateway's separate 6 MiB read bound.
func portalFeedResponseBytes(items []map[string]any, responseRequestID string) ([]byte, error) {
	payload, err := json.Marshal(portalFeedResponse{
		Data:      portalFeedData{Notices: items},
		RequestID: responseRequestID,
	})
	if err != nil {
		return nil, err
	}
	return append(payload, '\n'), nil
}

func writePortalFeed(w http.ResponseWriter, r *http.Request, items []map[string]any) {
	payload, err := portalFeedResponseBytes(items, requestID(r))
	if err != nil || len(payload) > portalFeedResponseByteLimit {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Notice feed is unavailable")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
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
