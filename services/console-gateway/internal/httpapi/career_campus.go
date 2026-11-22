package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	campusapi "henukit.dev/console-gateway/internal/campus"
	careerapi "henukit.dev/console-gateway/internal/career"
	"henukit.dev/console-gateway/internal/platformcore"
	"henukit.dev/console-gateway/internal/session"
)

type careerClient interface {
	Workspace(context.Context, string) (json.RawMessage, error)
	RetryDigest(context.Context, string, string, string) (json.RawMessage, error)
}

type campusClient interface {
	Workspace(context.Context, string) (json.RawMessage, error)
	ModerateItem(context.Context, string, string, []byte) (json.RawMessage, error)
	ModerateMessage(context.Context, string, string, []byte) (json.RawMessage, error)
}

type careerAuthorizer interface {
	CheckCareer(context.Context, string, string) error
}

type campusAuthorizer interface {
	CheckCampus(context.Context, string, string) error
}

func (h *Handler) authorizeCareer(writer http.ResponseWriter, request *http.Request, permission string) (session.Value, bool) {
	value, ok := h.readSession(writer, request)
	if !ok {
		return session.Value{}, false
	}
	if isUnconfigured(h.career) {
		writeError(writer, request, http.StatusServiceUnavailable, "CAREER_CONSOLE_DISABLED", "求职雷达监控接口尚未开启")
		return session.Value{}, false
	}
	authorizer, ok := h.platform.(careerAuthorizer)
	if !ok {
		writeError(writer, request, http.StatusForbidden, "ACCESS_DENIED", "暂无操作权限，请联系管理员")
		return session.Value{}, false
	}
	if err := authorizer.CheckCareer(request.Context(), value.ExchangeToken, permission); err != nil {
		h.handlePlatformSessionError(writer, request, err)
		return session.Value{}, false
	}
	return value, true
}

func (h *Handler) authorizeCampus(writer http.ResponseWriter, request *http.Request, permission string) (session.Value, bool) {
	value, ok := h.readSession(writer, request)
	if !ok {
		return session.Value{}, false
	}
	if isUnconfigured(h.campus) {
		writeError(writer, request, http.StatusServiceUnavailable, "CAMPUS_CONSOLE_DISABLED", "校园互助治理接口尚未开启")
		return session.Value{}, false
	}
	authorizer, ok := h.platform.(campusAuthorizer)
	if !ok {
		writeError(writer, request, http.StatusForbidden, "ACCESS_DENIED", "暂无操作权限，请联系管理员")
		return session.Value{}, false
	}
	if err := authorizer.CheckCampus(request.Context(), value.ExchangeToken, permission); err != nil {
		h.handlePlatformSessionError(writer, request, err)
		return session.Value{}, false
	}
	return value, true
}

func (h *Handler) getCareerWorkspace(writer http.ResponseWriter, request *http.Request) {
	value, ok := h.authorizeCareer(writer, request, "career.read")
	if !ok {
		return
	}
	data, err := h.career.Workspace(careerapi.WithRequestID(request.Context(), requestID(request)), value.UserID)
	h.writeCareerResult(writer, request, data, err)
}

func (h *Handler) retryCareerDigest(writer http.ResponseWriter, request *http.Request) {
	if !sameOriginAllowed(request) {
		writeError(writer, request, http.StatusForbidden, "ORIGIN_DENIED", "请从控制台页面提交操作")
		return
	}
	key := strings.TrimSpace(request.Header.Get("Idempotency-Key"))
	if len(key) < 8 || len(key) > 200 {
		writeError(writer, request, http.StatusBadRequest, "INVALID_IDEMPOTENCY_KEY", "请求内容不完整，请检查后重试")
		return
	}
	searchID := strings.TrimSpace(chi.URLParam(request, "search_id"))
	if _, err := uuid.Parse(searchID); err != nil {
		writeError(writer, request, http.StatusBadRequest, "INVALID_SEARCH_ID", "扫描任务编号格式无效")
		return
	}
	value, ok := h.authorizeCareer(writer, request, "career.manage")
	if !ok {
		return
	}
	data, err := h.career.RetryDigest(careerapi.WithRequestID(request.Context(), requestID(request)), value.UserID, searchID, key)
	h.writeCareerResult(writer, request, data, err)
}

func (h *Handler) writeCareerResult(writer http.ResponseWriter, request *http.Request, data json.RawMessage, err error) {
	if err == nil {
		writeJSON(writer, request, http.StatusOK, data)
		return
	}
	switch {
	case errors.Is(err, careerapi.ErrForbidden):
		writeError(writer, request, http.StatusForbidden, "ACCESS_DENIED", "暂无操作权限，请联系管理员")
	case errors.Is(err, careerapi.ErrNotFound):
		writeError(writer, request, http.StatusNotFound, "CAREER_NOT_FOUND", "未找到该扫描任务")
	case errors.Is(err, careerapi.ErrConflict):
		writeError(writer, request, http.StatusConflict, "CAREER_CONFLICT", "当前任务状态不支持重投简报")
	case errors.Is(err, careerapi.ErrInvalid):
		writeError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "请求参数无效")
	default:
		h.unavailable(writer, request, err)
	}
}

func (h *Handler) getCampusWorkspace(writer http.ResponseWriter, request *http.Request) {
	value, ok := h.authorizeCampus(writer, request, "campus.read")
	if !ok {
		return
	}
	data, err := h.campus.Workspace(campusapi.WithRequestID(request.Context(), requestID(request)), value.UserID)
	h.writeCampusResult(writer, request, data, err)
}

func (h *Handler) moderateCampusItem(writer http.ResponseWriter, request *http.Request) {
	if !sameOriginAllowed(request) {
		writeError(writer, request, http.StatusForbidden, "ORIGIN_DENIED", "请从控制台页面提交操作")
		return
	}
	itemID := strings.TrimSpace(chi.URLParam(request, "id"))
	if itemID == "" || len(itemID) > 100 {
		writeError(writer, request, http.StatusBadRequest, "INVALID_ITEM_ID", "帖子编号无效")
		return
	}
	body, err := io.ReadAll(io.LimitReader(request.Body, 64<<10))
	if err != nil || len(body) == 0 {
		writeError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "请提供完整的治理参数")
		return
	}
	value, ok := h.authorizeCampus(writer, request, "campus.moderate")
	if !ok {
		return
	}
	data, err := h.campus.ModerateItem(campusapi.WithRequestID(request.Context(), requestID(request)), value.UserID, itemID, body)
	h.writeCampusResult(writer, request, data, err)
}

func (h *Handler) moderateCampusMessage(writer http.ResponseWriter, request *http.Request) {
	if !sameOriginAllowed(request) {
		writeError(writer, request, http.StatusForbidden, "ORIGIN_DENIED", "请从控制台页面提交操作")
		return
	}
	messageID := strings.TrimSpace(chi.URLParam(request, "id"))
	if messageID == "" || len(messageID) > 100 {
		writeError(writer, request, http.StatusBadRequest, "INVALID_MESSAGE_ID", "留言编号无效")
		return
	}
	body, err := io.ReadAll(io.LimitReader(request.Body, 64<<10))
	if err != nil || len(body) == 0 {
		writeError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "请提供完整的治理参数")
		return
	}
	value, ok := h.authorizeCampus(writer, request, "campus.moderate")
	if !ok {
		return
	}
	data, err := h.campus.ModerateMessage(campusapi.WithRequestID(request.Context(), requestID(request)), value.UserID, messageID, body)
	h.writeCampusResult(writer, request, data, err)
}

func (h *Handler) writeCampusResult(writer http.ResponseWriter, request *http.Request, data json.RawMessage, err error) {
	if err == nil {
		writeJSON(writer, request, http.StatusOK, data)
		return
	}
	switch {
	case errors.Is(err, campusapi.ErrForbidden), errors.Is(err, platformcore.ErrForbidden):
		writeError(writer, request, http.StatusForbidden, "ACCESS_DENIED", "暂无操作权限，请联系管理员")
	case errors.Is(err, campusapi.ErrNotFound):
		writeError(writer, request, http.StatusNotFound, "CAMPUS_NOT_FOUND", "未找到对应的互助单子或留言")
	case errors.Is(err, campusapi.ErrConflict):
		writeError(writer, request, http.StatusConflict, "CAMPUS_CONFLICT", "内容有更新，请刷新后重试")
	case errors.Is(err, campusapi.ErrInvalid):
		writeError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "治理参数无效，请检查后重试")
	default:
		h.unavailable(writer, request, err)
	}
}
