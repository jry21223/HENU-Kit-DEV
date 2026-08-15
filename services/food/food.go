package food

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"henukit.dev/food/internal/contract"
)

var requestIDPattern = regexp.MustCompile(`^req_[A-Za-z0-9_-]+$`)

type Config struct {
	Database *pgxpool.Pool
	Redis    redis.UniversalClient
	ClientID string
	Keys     map[string]string

	// PostCreate/PostRead are the dedicated Food Post credential pairs
	// (see tmp/food-post-seam.md §1). Each pair must be fully configured
	// (non-empty client ID plus a 1-2 key ring with secrets ≥32 bytes) or
	// left entirely empty; an empty pair leaves the new routes answering
	// 401 to every caller while the service still starts.
	PostCreateClientID string
	PostCreateKeys     map[string]string
	PostReadClientID   string
	PostReadKeys       map[string]string
}

type service struct {
	database *pgxpool.Pool
	redis    redis.UniversalClient
	clientID string
	keys     map[string]string

	postCreateClientID string
	postCreateKeys     map[string]string
	postReadClientID   string
	postReadKeys       map[string]string
	now                func() time.Time
}

type actor struct{ userID, permission string }
type contextKey string

const (
	requestIDKey contextKey = "request-id"
	actorKey     contextKey = "actor"
)

func New(config Config) (http.Handler, error) {
	if config.Database == nil || config.Redis == nil || config.ClientID == "" || len(config.Keys) == 0 || len(config.Keys) > 2 {
		return nil, errors.New("food database and service credentials are required")
	}
	for keyID, secret := range config.Keys {
		if keyID == "" || len(secret) < 32 {
			return nil, errors.New("food service key ring is invalid")
		}
	}
	if err := validatePostCredentialPair("post create", config.PostCreateClientID, config.PostCreateKeys); err != nil {
		return nil, err
	}
	if err := validatePostCredentialPair("post read", config.PostReadClientID, config.PostReadKeys); err != nil {
		return nil, err
	}
	h := &service{database: config.Database, redis: config.Redis, clientID: config.ClientID, keys: config.Keys, postCreateClientID: config.PostCreateClientID, postCreateKeys: config.PostCreateKeys, postReadClientID: config.PostReadClientID, postReadKeys: config.PostReadKeys, now: time.Now}
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
	router.Group(func(posts chi.Router) {
		posts.Use(h.postAuthenticate(postRoleCreate))
		posts.Post(contract.CreatePostRoute, h.createPost)
	})
	router.Group(func(posts chi.Router) {
		posts.Use(h.postAuthenticate(postRoleRead))
		posts.Get(contract.ListPostsRoute, h.listPosts)
		posts.Get(contract.PostRoute, h.getPost)
		posts.Get(contract.PostImageRoute, h.getPostImage)
		posts.Get(contract.VenuesRoute, h.listVenues)
	})
	router.Group(func(posts chi.Router) {
		posts.Use(h.postAuthenticate(postRoleRead))
		posts.Use(h.postActorRequired)
		posts.Get(contract.MyPostsRoute, h.myPosts)
	})
	return router, nil
}

func validatePostCredentialPair(label, clientID string, keys map[string]string) error {
	if clientID == "" && len(keys) == 0 {
		return nil
	}
	if clientID == "" || len(keys) == 0 || len(keys) > 2 {
		return errors.New("food " + label + " credentials are incomplete")
	}
	for keyID, secret := range keys {
		if keyID == "" || len(secret) < 32 {
			return errors.New("food " + label + " key ring is invalid")
		}
	}
	return nil
}

func (h *service) require(w http.ResponseWriter, r *http.Request, permission string) (actor, bool) {
	value, _ := r.Context().Value(actorKey).(actor)
	if value.permission != permission {
		writeError(w, r, http.StatusForbidden, "PERMISSION_DENIED", "required Food permission is missing")
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
