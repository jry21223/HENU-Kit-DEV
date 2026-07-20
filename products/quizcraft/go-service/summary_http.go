package quizcraft

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var consoleRequestIDPattern = regexp.MustCompile(`^req_[A-Za-z0-9_-]+$`)

func (service *practiceHTTP) authenticateConsoleSummary(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if service.summaryClientID == "" || len(service.summaryKeys) == 0 {
			writeError(writer, http.StatusServiceUnavailable, "summary_auth_unavailable", "QuizCraft summary authentication is not configured")
			return
		}
		clientID, basicSecret, basic := request.BasicAuth()
		secret, knownKey := service.summaryKeys[request.Header.Get("X-Key-Id")]
		if !basic || clientID != service.summaryClientID || request.Header.Get("X-Service-Id") != clientID || !knownKey || !hmac.Equal([]byte(secret), []byte(basicSecret)) {
			writeError(writer, http.StatusUnauthorized, "invalid_service_auth", "summary service credentials are invalid")
			return
		}
		timestamp, err := strconv.ParseInt(request.Header.Get("X-Timestamp"), 10, 64)
		if err != nil || absInt64(service.now().Unix()-timestamp) > int64((5*time.Minute)/time.Second) {
			writeError(writer, http.StatusUnauthorized, "invalid_service_auth", "summary service timestamp is invalid")
			return
		}
		nonce := request.Header.Get("X-Nonce")
		decoded, err := base64.RawURLEncoding.DecodeString(nonce)
		if err != nil || len(decoded) != 24 {
			writeError(writer, http.StatusUnauthorized, "invalid_service_auth", "summary service nonce is invalid")
			return
		}
		digest := sha256.Sum256(nil)
		canonical := strings.Join([]string{request.Method, request.URL.RequestURI(), request.Header.Get("X-Timestamp"), nonce, hex.EncodeToString(digest[:])}, "\n")
		mac := hmac.New(sha256.New, []byte(secret))
		_, _ = mac.Write([]byte(canonical))
		if !hmac.Equal([]byte(request.Header.Get("X-Signature")), []byte(base64.RawURLEncoding.EncodeToString(mac.Sum(nil)))) {
			writeError(writer, http.StatusUnauthorized, "invalid_service_auth", "summary service signature is invalid")
			return
		}
		result, err := service.database.Exec(request.Context(), `INSERT INTO quizcraft_service_nonces(client_id,key_id,nonce) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, clientID, request.Header.Get("X-Key-Id"), nonce)
		if err != nil {
			writeError(writer, http.StatusServiceUnavailable, "summary_auth_unavailable", "summary replay protection is temporarily unavailable")
			return
		}
		if result.RowsAffected() != 1 {
			writeError(writer, http.StatusConflict, "service_replay", "summary service nonce was already used")
			return
		}
		_, _ = service.database.Exec(request.Context(), `DELETE FROM quizcraft_service_nonces WHERE received_at < now()-interval '10 minutes'`)
		next.ServeHTTP(writer, request)
	})
}

func (service *practiceHTTP) consoleSummary(writer http.ResponseWriter, request *http.Request) {
	var publishedBanks, drafts, pendingFeedback int64
	err := service.database.QueryRow(request.Context(), `SELECT (SELECT count(*) FROM quizcraft_banks WHERE active_version_id IS NOT NULL),(SELECT count(*) FROM quizcraft_workshop_version_states WHERE state='draft'),(SELECT count(*) FROM quizcraft_feedback_inbox_deliveries WHERE delivered_at IS NULL)`).Scan(&publishedBanks, &drafts, &pendingFeedback)
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft summary is temporarily unavailable")
		return
	}
	requestID := request.Header.Get("X-Request-Id")
	if len(requestID) > 120 || !consoleRequestIDPattern.MatchString(requestID) {
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
