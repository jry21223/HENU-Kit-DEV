package quizcraft

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

var serviceRequestIDPattern = regexp.MustCompile(`^req_[A-Za-z0-9_-]+$`)

type serviceAuthRequirement struct {
	clientID           string
	keys               map[string]string
	unavailableCode    string
	label              string
	requiredPermission string
	actorBound         bool
}

const maxPortalPracticeCommandBodyBytes = 2 << 20

type portalPracticeCommandContextKey struct{}

// portalPracticeCommandIdentity is installed only after the command HMAC has
// bound the optional Portal user UUID to the complete HTTP request.
// A nil userID is an authenticated guest command, not an unauthenticated one.
type portalPracticeCommandIdentity struct {
	userID *uuid.UUID
}

func portalPracticeCommandIdentityFromContext(ctx context.Context) (portalPracticeCommandIdentity, bool) {
	identity, ok := ctx.Value(portalPracticeCommandContextKey{}).(portalPracticeCommandIdentity)
	return identity, ok
}

func (service *practiceHTTP) authenticateSignedGET(requirement serviceAuthRequirement, next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if requirement.actorBound {
			writer.Header().Set("X-Request-Id", serviceRequestIDFromHeader(request))
		}
		if request.Method != http.MethodGet || request.ContentLength != 0 {
			writeError(writer, http.StatusUnauthorized, "invalid_service_auth", "QuizCraft "+requirement.label+" accepts signed GET requests without a body")
			return
		}
		if requirement.clientID == "" || len(requirement.keys) == 0 {
			writeError(writer, http.StatusServiceUnavailable, requirement.unavailableCode, "QuizCraft "+requirement.label+" authentication is not configured")
			return
		}
		clientID, basicSecret, basic := request.BasicAuth()
		secret, knownKey := requirement.keys[request.Header.Get("X-Key-Id")]
		if !basic || clientID != requirement.clientID || request.Header.Get("X-Service-Id") != clientID || !knownKey || !hmac.Equal([]byte(secret), []byte(basicSecret)) {
			writeError(writer, http.StatusUnauthorized, "invalid_service_auth", "QuizCraft "+requirement.label+" service credentials are invalid")
			return
		}
		timestamp, err := strconv.ParseInt(request.Header.Get("X-Timestamp"), 10, 64)
		if err != nil || absInt64(service.now().Unix()-timestamp) > int64((5*time.Minute)/time.Second) {
			writeError(writer, http.StatusUnauthorized, "invalid_service_auth", "QuizCraft "+requirement.label+" service timestamp is invalid")
			return
		}
		nonce := request.Header.Get("X-Nonce")
		decoded, err := base64.RawURLEncoding.DecodeString(nonce)
		if err != nil || len(decoded) != 24 {
			writeError(writer, http.StatusUnauthorized, "invalid_service_auth", "QuizCraft "+requirement.label+" service nonce is invalid")
			return
		}
		digest := sha256.Sum256(nil)
		canonicalParts := []string{request.Method, request.URL.RequestURI(), request.Header.Get("X-Timestamp"), nonce, hex.EncodeToString(digest[:])}
		if requirement.actorBound {
			// Actor-bound Portal reads opt into this branch. The strict parser
			// keeps malformed and guest actors out before any fact query, while
			// the original header text remains the sixth canonical HMAC line.
			if _, err := portalActorUserID(request); err != nil {
				writeError(writer, http.StatusUnauthorized, "invalid_service_auth", "QuizCraft "+requirement.label+" actor is invalid")
				return
			}
			canonicalParts = append(canonicalParts, request.Header.Get("X-Actor-User-Id"))
		}
		canonical := strings.Join(canonicalParts, "\n")
		mac := hmac.New(sha256.New, []byte(secret))
		_, _ = mac.Write([]byte(canonical))
		if !hmac.Equal([]byte(request.Header.Get("X-Signature")), []byte(base64.RawURLEncoding.EncodeToString(mac.Sum(nil)))) {
			writeError(writer, http.StatusUnauthorized, "invalid_service_auth", "QuizCraft "+requirement.label+" service signature is invalid")
			return
		}
		if requirement.requiredPermission != "" && (request.Header.Get("X-Permission-Code") != requirement.requiredPermission || request.Header.Get("X-Scope-Kind") != "product" || request.Header.Get("X-Product-Code") != "quizcraft") {
			writeError(writer, http.StatusForbidden, "permission_denied", "QuizCraft "+requirement.label+" permission or scope is denied")
			return
		}
		result, err := service.database.Exec(request.Context(), `INSERT INTO quizcraft_service_nonces(client_id,key_id,nonce) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, clientID, request.Header.Get("X-Key-Id"), nonce)
		if err != nil {
			writeError(writer, http.StatusServiceUnavailable, requirement.unavailableCode, "QuizCraft "+requirement.label+" replay protection is temporarily unavailable")
			return
		}
		if result.RowsAffected() != 1 {
			writeError(writer, http.StatusConflict, "service_replay", "QuizCraft "+requirement.label+" service nonce was already used")
			return
		}
		_, _ = service.database.Exec(request.Context(), `DELETE FROM quizcraft_service_nonces WHERE received_at < now()-interval '10 minutes'`)
		next.ServeHTTP(writer, request)
	})
}

func (service *practiceHTTP) authenticateConsoleSummary(next http.Handler) http.Handler {
	return service.authenticateSignedGET(serviceAuthRequirement{
		clientID:        service.summaryClientID,
		keys:            service.summaryKeys,
		unavailableCode: "summary_auth_unavailable",
		label:           "summary",
	}, next)
}

func (service *practiceHTTP) authenticatePortalRead(next http.Handler) http.Handler {
	return service.authenticateSignedGET(serviceAuthRequirement{
		clientID:           service.catalogClientID,
		keys:               service.catalogKeys,
		unavailableCode:    "portal_read_auth_unavailable",
		label:              "Portal read",
		requiredPermission: "portal.practice.read",
	}, next)
}

func (service *practiceHTTP) authenticatePortalActorRead(next http.Handler) http.Handler {
	return service.authenticateSignedGET(serviceAuthRequirement{
		clientID:           service.catalogClientID,
		keys:               service.catalogKeys,
		unavailableCode:    "portal_actor_read_auth_unavailable",
		label:              "Portal actor read",
		requiredPermission: "portal.practice.read",
		actorBound:         true,
	}, next)
}

// authenticatePortalCommand authorizes the internal Portal -> QuizCraft write
// routes. It intentionally does not reuse authenticateSignedGET: command
// signatures cover the exact raw JSON bytes and may additionally bind a Portal
// user UUID. The Basic credential proves the caller is Portal Gateway; the
// optional sixth canonical line proves the Gateway selected that user.
func (service *practiceHTTP) authenticatePortalCommand(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		// POST covers session/answer/feedback/favorites-session commands; PUT and
		// DELETE cover the favorite and unfavorite commands. The method is one of
		// the signed canonical lines below, so a signature computed for one
		// method cannot be replayed against another.
		if request.Method != http.MethodPost && request.Method != http.MethodPut && request.Method != http.MethodDelete {
			writeError(writer, http.StatusUnauthorized, "invalid_service_auth", "QuizCraft Portal commands accept signed POST, PUT, and DELETE requests")
			return
		}
		if service.portalCommandClientID == "" || len(service.portalCommandKeys) == 0 {
			writeError(writer, http.StatusServiceUnavailable, "portal_command_auth_unavailable", "QuizCraft Portal command authentication is not configured")
			return
		}

		raw, err := readPortalPracticeCommandBody(request)
		if err != nil {
			writeError(writer, http.StatusUnauthorized, "invalid_service_auth", "QuizCraft Portal command body is invalid")
			return
		}
		request.Body = io.NopCloser(bytes.NewReader(raw))
		request.ContentLength = int64(len(raw))

		clientID, basicSecret, basic := request.BasicAuth()
		keyID := request.Header.Get("X-Key-Id")
		secret, knownKey := service.portalCommandKeys[keyID]
		if !basic || clientID != service.portalCommandClientID || request.Header.Get("X-Service-Id") != clientID || !knownKey || !hmac.Equal([]byte(secret), []byte(basicSecret)) {
			writeError(writer, http.StatusUnauthorized, "invalid_service_auth", "QuizCraft Portal command service credentials are invalid")
			return
		}
		timestampText := request.Header.Get("X-Timestamp")
		timestamp, err := strconv.ParseInt(timestampText, 10, 64)
		if err != nil || absInt64(service.now().Unix()-timestamp) > int64((5*time.Minute)/time.Second) {
			writeError(writer, http.StatusUnauthorized, "invalid_service_auth", "QuizCraft Portal command service timestamp is invalid")
			return
		}
		nonce := request.Header.Get("X-Nonce")
		decodedNonce, err := base64.RawURLEncoding.DecodeString(nonce)
		if err != nil || len(decodedNonce) != 24 {
			writeError(writer, http.StatusUnauthorized, "invalid_service_auth", "QuizCraft Portal command service nonce is invalid")
			return
		}

		actorHeader := request.Header.Get("X-Actor-User-Id")
		if len(request.Header.Values("X-Actor-User-Id")) > 1 || (actorHeader != "" && strings.TrimSpace(actorHeader) != actorHeader) {
			writeError(writer, http.StatusUnauthorized, "invalid_service_auth", "QuizCraft Portal command actor is invalid")
			return
		}
		identity := portalPracticeCommandIdentity{}
		if actorHeader != "" {
			actorID, parseErr := uuid.Parse(actorHeader)
			if parseErr != nil {
				writeError(writer, http.StatusUnauthorized, "invalid_service_auth", "QuizCraft Portal command actor is invalid")
				return
			}
			identity.userID = &actorID
		}

		digest := sha256.Sum256(raw)
		canonicalParts := []string{request.Method, request.URL.RequestURI(), timestampText, nonce, hex.EncodeToString(digest[:])}
		if actorHeader != "" {
			canonicalParts = append(canonicalParts, actorHeader)
		}
		mac := hmac.New(sha256.New, []byte(secret))
		_, _ = mac.Write([]byte(strings.Join(canonicalParts, "\n")))
		expectedSignature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
		if !hmac.Equal([]byte(request.Header.Get("X-Signature")), []byte(expectedSignature)) {
			writeError(writer, http.StatusUnauthorized, "invalid_service_auth", "QuizCraft Portal command service signature is invalid")
			return
		}

		result, err := service.database.Exec(request.Context(), `INSERT INTO quizcraft_service_nonces(client_id,key_id,nonce) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, clientID, keyID, nonce)
		if err != nil {
			writeError(writer, http.StatusServiceUnavailable, "portal_command_auth_unavailable", "QuizCraft Portal command replay protection is temporarily unavailable")
			return
		}
		if result.RowsAffected() != 1 {
			writeError(writer, http.StatusConflict, "service_replay", "QuizCraft Portal command service nonce was already used")
			return
		}
		_, _ = service.database.Exec(request.Context(), `DELETE FROM quizcraft_service_nonces WHERE received_at < now()-interval '10 minutes'`)
		next.ServeHTTP(writer, request.WithContext(context.WithValue(request.Context(), portalPracticeCommandContextKey{}, identity)))
	})
}

func readPortalPracticeCommandBody(request *http.Request) ([]byte, error) {
	if request.Body == nil {
		return nil, nil
	}
	raw, err := io.ReadAll(io.LimitReader(request.Body, maxPortalPracticeCommandBodyBytes+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > maxPortalPracticeCommandBodyBytes {
		return nil, io.ErrShortBuffer
	}
	return raw, nil
}

func (service *practiceHTTP) consoleSummary(writer http.ResponseWriter, request *http.Request) {
	var publishedBanks, drafts, pendingFeedback int64
	err := service.database.QueryRow(request.Context(), `SELECT (SELECT count(*) FROM quizcraft_banks WHERE active_version_id IS NOT NULL),(SELECT count(*) FROM quizcraft_workshop_version_states WHERE state='draft'),(SELECT count(*) FROM quizcraft_feedback_inbox_deliveries WHERE delivered_at IS NULL)`).Scan(&publishedBanks, &drafts, &pendingFeedback)
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft summary is temporarily unavailable")
		return
	}
	requestID := request.Header.Get("X-Request-Id")
	if len(requestID) > 120 || !serviceRequestIDPattern.MatchString(requestID) {
		requestID = "req_summary_invalid"
	}
	data := map[string]any{
		"id": "quizcraft", "status": "ok", "status_message": "QuizCraft Workshop and feedback facts are available.", "as_of": service.now().UTC(),
		"metrics": []map[string]string{{"label": "已发布题库", "value": strconv.FormatInt(publishedBanks, 10)}, {"label": "待人工校验", "value": strconv.FormatInt(drafts, 10)}, {"label": "纠错反馈", "value": strconv.FormatInt(pendingFeedback, 10)}},
	}
	encoded, _ := json.Marshal(responseEnvelope{RequestID: requestID, Data: data})
	writeRawJSON(writer, http.StatusOK, encoded)
}

func absInt64(value int64) int64 {
	if value < 0 {
		return -value
	}
	return value
}
