package httpapi

import (
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"henukit.dev/portal-gateway/internal/practice"
	"henukit.dev/portal-gateway/internal/trainingcontract"
)

func (h *Handler) training(w http.ResponseWriter, r *http.Request) {
	setPrivateResponseHeaders(w)
	w.Header().Set("Cache-Control", "private, no-store")
	suffix := strings.TrimPrefix(r.URL.Path, "/api/v1/practice/training")
	catalog := r.Method == http.MethodGet && suffix == "/catalog"
	if catalog && h.quizCraftCatalog == nil {
		h.trainingError(w, r, 404, "NOT_FOUND", "训练内容不存在或已下架")
		return
	}
	if r.Method == http.MethodGet && !catalog && h.quizCraft == nil || r.Method == http.MethodPost && h.practiceCommands == nil {
		h.trainingError(w, r, 503, "TRAINING_DISABLED", "训练暂时不可用，请稍后重试")
		return
	}

	if r.Method == http.MethodGet && r.Body != nil {
		body, err := io.ReadAll(io.LimitReader(r.Body, 1))
		if err != nil || len(body) > 0 {
			h.trainingError(w, r, 400, "INVALID_REQUEST", "请求内容无效，请刷新后重试")
			return
		}
	}
	if r.Method == http.MethodPost {
		contentType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || contentType != "application/json" {
			h.trainingError(w, r, 400, "INVALID_REQUEST", "请求内容无效，请刷新后重试")
			return
		}
	}
	// Disclosure verification requires the existing private-read gate before
	// any hint or language write can reach Core.
	if (strings.HasSuffix(suffix, "/hints") || strings.HasSuffix(suffix, "/reveals")) && h.quizCraft == nil {
		h.trainingError(w, r, 503, "TRAINING_DISABLED", "训练暂时不可用，请稍后重试")
		return
	}
	if strings.Contains(suffix, "/sessions/") && !practice.ValidUUID(chi.URLParam(r, "session_id")) {
		h.trainingError(w, r, 404, "NOT_FOUND", "训练记录不存在，请返回训练目录")
		return
	}
	if suffix == "/review-items" {
		query, err := practice.CanonicalTrainingReviewQuery(r.URL.RawQuery)
		if err != nil {
			h.trainingError(w, r, 400, "INVALID_REQUEST", "请求内容无效，请刷新后重试")
			return
		}
		if query != "" {
			suffix += "?" + query
		}
	} else if r.URL.RawQuery != "" || r.URL.ForceQuery {
		h.trainingError(w, r, 400, "INVALID_REQUEST", "请求内容无效，请刷新后重试")
		return
	}
	actor := ""
	var cookie *http.Cookie
	if !catalog {
		var err error
		actor, cookie, _, err = h.practiceCommandActor(r, true)
		if actor != "" && (strings.TrimSpace(actor) != actor || actor == "00000000-0000-0000-0000-000000000000") {
			err = errors.New("invalid training actor")
		}
		if err != nil {
			h.trainingError(w, r, 401, "UNAUTHENTICATED", "登录已过期，请重新登录")
			return
		}
		if actor == "" && cookie == nil && r.Method == http.MethodGet {
			h.trainingError(w, r, 404, "NOT_FOUND", "训练记录不存在，请返回训练目录")
			return
		}
	}
	var result practice.CommandResult
	var err error
	status := http.StatusOK
	if r.Method == http.MethodGet {
		client := h.quizCraft
		if catalog {
			client = h.quizCraftCatalog
		}
		result, err = client.TrainingRead(r.Context(), suffix, actor, requestIDOf(w, r), cookie)
	} else {
		key := r.Header.Get("Idempotency-Key")
		raw, bodyErr := readGatewayPracticeCommandBody(r)
		if bodyErr != nil || !practice.ValidIdempotencyKey(key) {
			h.trainingError(w, r, 400, "INVALID_REQUEST", "请求内容不完整，请检查后重试")
			return
		}
		if strings.HasSuffix(suffix, "/reveals") {
			result, err = h.practiceCommands.TrainingReveal(r.Context(), h.quizCraft, suffix, actor, requestIDOf(w, r), key, raw, cookie)
		} else if strings.HasSuffix(suffix, "/hints") {
			result, err = h.practiceCommands.TrainingHint(r.Context(), h.quizCraft, suffix, actor, requestIDOf(w, r), key, raw, cookie)
		} else {
			result, err = h.practiceCommands.TrainingCommand(r.Context(), suffix, actor, requestIDOf(w, r), key, raw, cookie)
		}
		if suffix == "/sessions" {
			status = http.StatusCreated
		}
	}
	if err != nil {
		var failure *practice.TrainingFailure
		switch {
		case errors.As(err, &failure):
			writeJSON(w, failure.Status, failure.Envelope)
		case errors.Is(err, practice.ErrPracticeCommandBadRequest):
			h.trainingError(w, r, 400, "INVALID_REQUEST", "请求内容不完整，请检查后重试")
		case errors.Is(err, practice.ErrPracticeCommandInvalid):
			h.trainingError(w, r, 502, "DEPENDENCY_UNAVAILABLE", "训练响应暂时不可用，请稍后重试")
		default:
			h.trainingError(w, r, 502, "DEPENDENCY_UNAVAILABLE", "训练暂时不可用，请稍后重试")
		}
		return
	}
	if result.AnonymousCookie != nil {
		http.SetCookie(w, result.AnonymousCookie)
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(result.Raw)
}
func (h *Handler) trainingError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	writeJSON(w, status, trainingcontract.TrainingErrorEnvelope{RequestId: requestIDOf(w, r), Error: trainingcontract.TrainingError{Code: trainingcontract.TrainingErrorCode(code), Message: message}})
}

func (h *Handler) trainingNotFound(w http.ResponseWriter, r *http.Request) {
	setPrivateResponseHeaders(w)
	w.Header().Set("Cache-Control", "private, no-store")
	h.trainingError(w, r, 404, "NOT_FOUND", "训练内容不存在或已下架")
}
