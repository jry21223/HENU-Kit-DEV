package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"henukit.dev/console-gateway/internal/platformcore"
	quizcraftapi "henukit.dev/console-gateway/internal/quizcraft"
)

type quizcraftAuthorizer interface {
	CheckQuizcraft(context.Context, string, string, string) error
}
type quizcraftClient interface {
	Request(context.Context, string, string, string, string, string, string, []byte) (int, []byte, error)
}

func (h *Handler) quizcraftManagement(w http.ResponseWriter, r *http.Request) {
	if isUnconfigured(h.quizcraft) {
		writeError(w, r, 404, "NOT_FOUND", "此功能暂未开放")
		return
	}
	value, ok := h.readSession(w, r)
	if !ok {
		return
	}
	path := "/" + chi.URLParam(r, "*")
	permission, bank, valid := quizcraftapi.Match(r.Method, path)
	if !valid {
		writeError(w, r, 404, "NOT_FOUND", "未找到此操作")
		return
	}
	if r.Method != "GET" {
		if r.Header.Get("Origin") != "https://"+r.Host {
			writeError(w, r, 403, "ORIGIN_DENIED", "请求来源无效，请从控制台重新提交")
			return
		}
		if strings.TrimSpace(strings.Split(r.Header.Get("Content-Type"), ";")[0]) != "application/json" {
			writeError(w, r, 400, "INVALID_REQUEST", "请使用有效的题库请求内容")
			return
		}
		if n := len(r.Header.Get("Idempotency-Key")); n < 16 || n > 160 {
			writeError(w, r, 400, "INVALID_IDEMPOTENCY_KEY", "请求标识无效，请重新提交")
			return
		}
	}
	authorizer, ok := h.platform.(quizcraftAuthorizer)
	if !ok {
		h.writePlatformError(w, r, platformcore.ErrForbidden)
		return
	}
	if err := authorizer.CheckQuizcraft(r.Context(), value.ExchangeToken, permission, bank); err != nil {
		h.writePlatformError(w, r, err)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, (2<<20)+1))
	if err != nil || len(body) > 2<<20 {
		writeError(w, r, 413, "REQUEST_TOO_LARGE", "请求内容过大，请联系管理员处理后重试")
		return
	}
	if (r.Method == "GET" && len(body) > 0) || (r.Method != "GET" && !json.Valid(body)) {
		writeError(w, r, 400, "INVALID_REQUEST", "题库请求内容无效，请检查后重试")
		return
	}
	status, raw, err := h.quizcraft.Request(r.Context(), r.Method, path, value.UserID, permission, bank, r.Header.Get("Idempotency-Key"), body)
	if err != nil {
		h.logger.Error("quizcraft_management_unavailable", "request_id", requestID(r), "error", err)
		writeError(w, r, 503, "DEPENDENCY_UNAVAILABLE", "题库服务暂时不可用，请稍后重试")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write(raw)
}
