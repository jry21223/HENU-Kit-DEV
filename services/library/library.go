package library

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"henukit.dev/library/internal/contract"
)

var requestIDPattern = regexp.MustCompile(`^req_[A-Za-z0-9_-]+$`)

type Config struct {
	Database      *pgxpool.Pool
	Redis         redis.UniversalClient
	ClientID      string
	Keys          map[string]string
	LegacyBaseURL string
	LegacyToken   string
	HTTPClient    *http.Client
}

type service struct {
	database *pgxpool.Pool
	redis    redis.UniversalClient
	clientID string
	keys     map[string]string
	now      func() time.Time
	legacy   *legacyAdapter
}

type actor struct{ userID, permission string }
type contextKey string

const (
	requestIDKey contextKey = "request-id"
	actorKey     contextKey = "actor"
)

func New(config Config) (http.Handler, error) {
	if config.Database == nil || config.Redis == nil || config.ClientID == "" || len(config.Keys) == 0 || len(config.Keys) > 2 {
		return nil, errors.New("library database and service credentials are required")
	}
	for keyID, secret := range config.Keys {
		if keyID == "" || len(secret) < 32 {
			return nil, errors.New("library service key ring is invalid")
		}
	}
	legacy, err := newLegacyAdapter(config.LegacyBaseURL, config.LegacyToken, config.HTTPClient)
	if err != nil {
		return nil, err
	}
	h := &service{database: config.Database, redis: config.Redis, clientID: config.ClientID, keys: config.Keys, now: time.Now, legacy: legacy}
	router := chi.NewRouter()
	router.Use(h.requestContext)
	router.Get(contract.HealthRoute, func(w http.ResponseWriter, r *http.Request) {
		writeData(w, r, http.StatusOK, map[string]bool{"ok": true})
	})
	router.Group(func(protected chi.Router) {
		protected.Use(h.authenticate)
		protected.Get(contract.SummaryRoute, h.consoleSummary)
		protected.Get(contract.WorkspaceRoute, h.workspace)
		protected.Post(contract.CommandRoute, h.command)
		protected.Get(contract.OperationRoute, h.operationStatus)
	})
	return router, nil
}

func (h *service) require(w http.ResponseWriter, r *http.Request, permission string) (actor, bool) {
	value, _ := r.Context().Value(actorKey).(actor)
	if value.permission != permission {
		writeError(w, r, http.StatusForbidden, "PERMISSION_DENIED", "required Library permission is missing")
		return actor{}, false
	}
	return value, true
}

func (h *service) workspace(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.require(w, r, "library.read"); !ok {
		return
	}
	writeData(w, r, http.StatusOK, h.legacy.workspace(r.Context()))
}

func (h *service) consoleSummary(w http.ResponseWriter, r *http.Request) {
	workspace := h.legacy.workspace(r.Context())
	writeData(w, r, http.StatusOK, map[string]any{"id": "library", "status": workspace.Status, "status_message": workspace.StatusMessage, "as_of": workspace.GeneratedAt, "metrics": []map[string]string{{"label": "课程", "value": fmt.Sprint(len(workspace.Courses))}, {"label": "资料", "value": fmt.Sprint(len(workspace.Materials))}, {"label": "待审核", "value": fmt.Sprint(len(workspace.Submissions))}, {"label": "纠错", "value": fmt.Sprint(len(workspace.Corrections))}}})
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
