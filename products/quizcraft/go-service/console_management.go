package quizcraft

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type consoleManagementContextKey struct{}
type consoleManagementIdentity struct {
	actorID            uuid.UUID
	permission, bankID string
}

func validateConsoleManagementConfig(config PracticeHTTPConfig) error {
	if (config.ConsoleManagementClientID == "") != (len(config.ConsoleManagementKeys) == 0) || (config.ConsoleManagementEnabled && len(config.ConsoleManagementKeys) == 0) {
		return errors.New("invalid configuration: Console management requires a client and dedicated key ring")
	}
	if config.ConsoleManagementClientID != "" && (config.ConsoleManagementClientID == config.SummaryClientID || config.ConsoleManagementClientID == config.CatalogClientID || config.ConsoleManagementClientID == config.PortalCommandClientID) {
		return errors.New("invalid configuration: Console management client must be separate from summary and Portal clients")
	}
	for id, secret := range config.ConsoleManagementKeys {
		if id == "" || len(secret) < 32 || secret == string(config.AuthHMACSecret) || secret == string(config.CutoverEvidenceSecret) || secret == config.PlatformClientSecret || secret == string(config.SessionEncryptionKey) {
			return errors.New("invalid configuration: Console management key ring is invalid or shared")
		}
		for _, ring := range []map[string]string{config.SummaryKeys, config.CatalogKeys, config.PortalCommandKeys} {
			for _, other := range ring {
				if hmac.Equal([]byte(secret), []byte(other)) {
					return errors.New("invalid configuration: Console management keys must not be shared")
				}
			}
		}
	}
	return nil
}
func (service *practiceHTTP) registerConsoleManagement(router chi.Router) {
	const root = "/api/v1/console/workshop"
	add := func(method, path, permission string, handler http.HandlerFunc) {
		var next http.Handler = handler
		if method != "GET" {
			next = service.requireWritesEnabled(next)
		}
		router.Method(method, root+path, service.authenticateConsoleManagement(permission, next))
	}
	add("GET", "/catalog", "quizcraft.workshop.read", service.listWorkshopCatalog)
	add("GET", "/banks", "quizcraft.workshop.read", service.listWorkshopBanks)
	add("POST", "/banks", "quizcraft.workshop.write", service.createWorkshopBank)
	add("POST", "/banks/{bank_id}/versions", "quizcraft.workshop.write", service.createWorkshopVersion)
	add("POST", "/banks/{bank_id}/imports", "quizcraft.workshop.write", service.importWorkshopVersion)
	add("GET", "/banks/{bank_id}/versions/{bank_version_id}", "quizcraft.workshop.read", service.getWorkshopVersion)
	add("POST", "/banks/{bank_id}/versions/{bank_version_id}/validate", "quizcraft.workshop.write", service.validateWorkshopVersion)
	add("POST", "/banks/{bank_id}/versions/{bank_version_id}/publish", "quizcraft.workshop.publish", service.publishWorkshopVersion)
	add("POST", "/banks/{bank_id}/versions/{bank_version_id}/unpublish", "quizcraft.workshop.publish", service.unpublishWorkshopVersion)
	add("POST", "/banks/{bank_id}/rollback", "quizcraft.workshop.publish", service.rollbackWorkshopBank)
}
func (service *practiceHTTP) authenticateConsoleManagement(permission string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reject := func() { writeError(w, 401, "invalid_service_auth", "Console management authentication is invalid") }
		client, provided, basic := r.BasicAuth()
		key, known := service.consoleManagementKeys[r.Header.Get("X-Key-Id")]
		if !basic || !known || client != service.consoleManagementClientID || client != r.Header.Get("X-Service-Id") || !hmac.Equal([]byte(provided), []byte(key)) {
			reject()
			return
		}
		ts, err := strconv.ParseInt(r.Header.Get("X-Timestamp"), 10, 64)
		if err != nil || absInt64(service.now().Unix()-ts) > 300 {
			reject()
			return
		}
		nonce := r.Header.Get("X-Nonce")
		decoded, err := base64.RawURLEncoding.DecodeString(nonce)
		if err != nil || len(decoded) != 24 {
			reject()
			return
		}
		actor, err := uuid.Parse(r.Header.Get("X-Actor-User-Id"))
		if err != nil || actor == uuid.Nil || actor.String() != r.Header.Get("X-Actor-User-Id") {
			reject()
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, (2<<20)+1))
		if err != nil || len(body) > 2<<20 {
			writeError(w, 413, "body_too_large", "Management request exceeds the supported size")
			return
		}
		if r.Method == "GET" && len(body) != 0 {
			reject()
			return
		}
		d := sha256.Sum256(body)
		bank := r.Header.Get("X-Resource-Id")
		canonical := strings.Join([]string{r.Method, r.URL.RequestURI(), r.Header.Get("X-Timestamp"), nonce, hex.EncodeToString(d[:]), r.Header.Get("X-Actor-User-Id"), r.Header.Get("X-Permission-Code"), bank, r.Header.Get("Idempotency-Key")}, "\n")
		mac := hmac.New(sha256.New, []byte(key))
		_, _ = mac.Write([]byte(canonical))
		if !hmac.Equal([]byte(r.Header.Get("X-Signature")), []byte(base64.RawURLEncoding.EncodeToString(mac.Sum(nil)))) {
			reject()
			return
		}
		if r.Header.Get("X-Permission-Code") != permission || bank != chi.URLParam(r, "bank_id") {
			writeError(w, 403, "scope_denied", "Console management permission or bank scope is denied")
			return
		}
		// The new Console boundary requires an explicit optimistic version. The
		// frozen legacy Workshop import endpoint retains its omitted-version compatibility.
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/imports") {
			var input struct {
				ExpectedVersion *int64 `json:"expected_version"`
			}
			if err := json.Unmarshal(body, &input); err != nil || input.ExpectedVersion == nil || *input.ExpectedVersion < 0 {
				writeError(w, 400, "invalid_request", "An explicit non-negative expected_version is required")
				return
			}
		}
		result, err := service.database.Exec(r.Context(), `INSERT INTO quizcraft_service_nonces(client_id,key_id,nonce) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, client, r.Header.Get("X-Key-Id"), nonce)
		if err != nil {
			writeError(w, 503, "authorization_unavailable", "Management replay protection is unavailable")
			return
		}
		if result.RowsAffected() != 1 {
			writeError(w, 409, "service_replay", "Management nonce was already used")
			return
		}
		_, _ = service.database.Exec(r.Context(), `DELETE FROM quizcraft_service_nonces WHERE received_at < now()-interval '10 minutes'`)
		r.Body = io.NopCloser(bytes.NewReader(body))
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), consoleManagementContextKey{}, consoleManagementIdentity{actorID: actor, permission: permission, bankID: bank})))
	})
}
