package httpapi

import (
	"bytes"
	"crypto/hmac"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"henukit.dev/portal-api/internal/campus"
	"henukit.dev/portal-api/internal/serviceauth"
)

func buildCampusVerifiers() (*serviceauth.Verifier, *serviceauth.Verifier, error) {
	pClient := strings.TrimSpace(os.Getenv("CAMPUS_PORTAL_CLIENT_ID"))
	pKey := strings.TrimSpace(os.Getenv("CAMPUS_PORTAL_KEY_ID"))
	pSecret := os.Getenv("CAMPUS_PORTAL_SECRET")

	cClient := strings.TrimSpace(os.Getenv("CAMPUS_CONSOLE_CLIENT_ID"))
	cKey := strings.TrimSpace(os.Getenv("CAMPUS_CONSOLE_KEY_ID"))
	cSecret := os.Getenv("CAMPUS_CONSOLE_SECRET")

	if pSecret != "" && cSecret != "" && hmac.Equal([]byte(pSecret), []byte(cSecret)) {
		return nil, nil, errors.New("CAMPUS_PORTAL_SECRET and CAMPUS_CONSOLE_SECRET must be distinct")
	}
	return serviceauth.NewVerifier(pClient, pKey, pSecret), serviceauth.NewVerifier(cClient, cKey, cSecret), nil
}

func mountCampusWorkflowRoutes(r chi.Router, src campusSource, portalVerifier, consoleVerifier *serviceauth.Verifier) {
	// Limit concurrent signed upload buffers as well as image decoding.
	uploads := make(chan struct{}, 1)
	create := portalVerifier.MiddlewareWithMaxBody(true, false, 20<<20)(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		handleCreateCampusItem(w, req, src)
	}))
	r.Post("/api/v1/campus/items", func(w http.ResponseWriter, req *http.Request) {
		select {
		case uploads <- struct{}{}:
			defer func() { <-uploads }()
		case <-req.Context().Done():
			return
		}
		controller := http.NewResponseController(w)
		if controller.SetReadDeadline(time.Now().Add(2*time.Minute)) == nil {
			defer controller.SetReadDeadline(time.Time{})
		}
		create.ServeHTTP(w, req)
	})
	// Student authenticated campus write & privacy contact-book routes (signed by portal-gateway).
	r.Group(func(pr chi.Router) {
		pr.Use(portalVerifier.Middleware(true, false))
		pr.Get("/api/v1/campus/contact-book", func(w http.ResponseWriter, req *http.Request) {
			handleGetContactBook(w, req, src)
		})
		pr.Put("/api/v1/campus/contact-book", func(w http.ResponseWriter, req *http.Request) {
			handlePutContactBook(w, req, src)
		})
		pr.Post("/api/v1/campus/items/{id}/messages", func(w http.ResponseWriter, req *http.Request) {
			handleCreateCampusMessage(w, req, src)
		})
		pr.Get("/api/v1/campus/items/{id}/exchanges", func(w http.ResponseWriter, req *http.Request) {
			handleListCampusExchanges(w, req, src)
		})
		pr.Post("/api/v1/campus/items/{id}/exchanges", func(w http.ResponseWriter, req *http.Request) {
			handleRequestCampusExchange(w, req, src)
		})
		pr.Post("/api/v1/campus/items/{id}/exchanges/{exchange_id}/respond", func(w http.ResponseWriter, req *http.Request) {
			handleRespondCampusExchange(w, req, src)
		})
		pr.Post("/api/v1/campus/items/{id}/complete", func(w http.ResponseWriter, req *http.Request) {
			handleCompleteCampusItem(w, req, src)
		})
	})

	// Console summary route (5-line HMAC, no actor required).
	r.With(consoleVerifier.Middleware(false, false)).Get("/api/v1/campus/console-summary", func(w http.ResponseWriter, req *http.Request) {
		handleCampusConsoleSummary(w, req, src)
	})

	// Console governance routes (5-line HMAC + product:campus scope + permission).
	r.Group(func(cr chi.Router) {
		cr.Use(consoleVerifier.Middleware(true, true))
		cr.Get("/api/v1/campus/console/workspace", func(w http.ResponseWriter, req *http.Request) {
			handleCampusConsoleWorkspace(w, req, src)
		})
		cr.Post("/api/v1/campus/console/items/{id}/moderate", func(w http.ResponseWriter, req *http.Request) {
			handleCampusModerateItem(w, req, src)
		})
		cr.Post("/api/v1/campus/console/messages/{id}/moderate", func(w http.ResponseWriter, req *http.Request) {
			handleCampusModerateMessage(w, req, src)
		})
	})
}

func writeCampusError(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, status, map[string]any{
		"error":      map[string]string{"code": code, "message": msg},
		"request_id": requestIDOf(w),
	})
}

func handleGetContactBook(w http.ResponseWriter, r *http.Request, src campusSource) {
	if src.portalDB == nil {
		writeCampusError(w, http.StatusServiceUnavailable, "PORTAL_DATABASE_UNAVAILABLE", "互助数据库暂不可用")
		return
	}
	actor, _ := serviceauth.ActorFromContext(r.Context())
	book, err := src.portalDB.GetContactBook(actor.UserID)
	if err != nil {
		writeCampusError(w, http.StatusServiceUnavailable, "DATABASE_ERROR", "读取联系方式簿失败")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"contact": book, "request_id": requestIDOf(w)})
}

func handlePutContactBook(w http.ResponseWriter, r *http.Request, src campusSource) {
	if src.portalDB == nil {
		writeCampusError(w, http.StatusServiceUnavailable, "PORTAL_DATABASE_UNAVAILABLE", "互助数据库暂不可用")
		return
	}
	actor, _ := serviceauth.ActorFromContext(r.Context())
	var in campus.ContactBook
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeCampusError(w, http.StatusBadRequest, "INVALID_REQUEST", "联系方式格式有误")
		return
	}
	saved, err := src.portalDB.UpsertContactBook(actor.UserID, in)
	if errors.Is(err, campus.ErrInvalidInput) {
		writeCampusError(w, http.StatusBadRequest, "INVALID_INPUT", "联系方式字段超长或无效")
		return
	}
	if err != nil {
		writeCampusError(w, http.StatusServiceUnavailable, "DATABASE_ERROR", "保存联系方式失败")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"contact": saved, "request_id": requestIDOf(w)})
}

func handleCreateCampusItem(w http.ResponseWriter, r *http.Request, src campusSource) {
	if src.portalDB == nil {
		writeCampusError(w, http.StatusServiceUnavailable, "PORTAL_DATABASE_UNAVAILABLE", "互助数据库暂不可用")
		return
	}
	actor, _ := serviceauth.ActorFromContext(r.Context())
	var in campus.CreateItemInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeCampusError(w, http.StatusBadRequest, "INVALID_REQUEST", "发布内容格式有误")
		return
	}
	item, err := src.portalDB.CreateItem(r.Context(), actor.UserID, actor.DisplayName, in)
	if errors.Is(err, campus.ErrInvalidImage) {
		writeCampusError(w, http.StatusBadRequest, "INVALID_IMAGE", "请上传最多 6 张 JPG、PNG 或 WebP 图片，单张不能超过 2 MiB。图片尺寸过大时请缩小后重试。")
		return
	}
	if errors.Is(err, campus.ErrInvalidInput) {
		writeCampusError(w, http.StatusBadRequest, "INVALID_INPUT", "请完整填写类型、标题、详情、地点与价格")
		return
	}
	if err != nil {
		writeCampusError(w, http.StatusServiceUnavailable, "DATABASE_ERROR", "暂时无法发布单子，请稍后重试。")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusCreated, map[string]any{"item": item, "request_id": requestIDOf(w)})
}

func handleCampusItemImage(w http.ResponseWriter, r *http.Request, src campusSource) {
	position, err := strconv.Atoi(chi.URLParam(r, "position"))
	if err != nil || position < 0 || position > 5 {
		writeCampusError(w, http.StatusNotFound, "IMAGE_NOT_FOUND", "这张图片不存在。")
		return
	}
	if src.portalDB == nil {
		writeCampusError(w, http.StatusServiceUnavailable, "PORTAL_DATABASE_UNAVAILABLE", "暂时无法加载图片，请稍后再试。")
		return
	}
	image, err := src.portalDB.GetItemImage(r.Context(), chi.URLParam(r, "id"), position)
	if err != nil {
		writeCampusError(w, http.StatusServiceUnavailable, "DATABASE_ERROR", "暂时无法加载图片，请稍后再试。")
		return
	}
	if image == nil {
		writeCampusError(w, http.StatusNotFound, "IMAGE_NOT_FOUND", "这张图片不存在。")
		return
	}
	w.Header().Set("Content-Type", image.ContentType)
	w.Header().Set("ETag", `"`+image.SHA256+`"`)
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(image.Bytes))
}

func handleCreateCampusMessage(w http.ResponseWriter, r *http.Request, src campusSource) {
	if src.portalDB == nil {
		writeCampusError(w, http.StatusServiceUnavailable, "PORTAL_DATABASE_UNAVAILABLE", "互助数据库暂不可用")
		return
	}
	actor, _ := serviceauth.ActorFromContext(r.Context())
	var in struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeCampusError(w, http.StatusBadRequest, "INVALID_REQUEST", "留言格式有误")
		return
	}
	msg, err := src.portalDB.CreateMessage(chi.URLParam(r, "id"), actor.UserID, actor.DisplayName, in.Text)
	if errors.Is(err, campus.ErrInvalidInput) {
		writeCampusError(w, http.StatusBadRequest, "INVALID_INPUT", "留言不能为空且不超过 500 字")
		return
	}
	if errors.Is(err, campus.ErrNotFound) {
		writeCampusError(w, http.StatusNotFound, "NOT_FOUND", "单子不存在或已隐藏")
		return
	}
	if err != nil {
		writeCampusError(w, http.StatusServiceUnavailable, "DATABASE_ERROR", "发表留言失败")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusCreated, map[string]any{"message": msg, "request_id": requestIDOf(w)})
}

func handleListCampusExchanges(w http.ResponseWriter, r *http.Request, src campusSource) {
	if src.portalDB == nil {
		writeCampusError(w, http.StatusServiceUnavailable, "PORTAL_DATABASE_UNAVAILABLE", "互助数据库暂不可用")
		return
	}
	actor, _ := serviceauth.ActorFromContext(r.Context())
	isOwner, list, err := src.portalDB.ListExchangesForActor(chi.URLParam(r, "id"), actor.UserID)
	if errors.Is(err, campus.ErrNotFound) {
		writeCampusError(w, http.StatusNotFound, "NOT_FOUND", "单子不存在或已隐藏")
		return
	}
	if err != nil {
		writeCampusError(w, http.StatusServiceUnavailable, "DATABASE_ERROR", "加载联系方式交换状态失败")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{
		"viewer_user_id": actor.UserID,
		"is_owner":       isOwner,
		"exchanges":      list,
		"request_id":     requestIDOf(w),
	})
}

func handleRequestCampusExchange(w http.ResponseWriter, r *http.Request, src campusSource) {
	if src.portalDB == nil {
		writeCampusError(w, http.StatusServiceUnavailable, "PORTAL_DATABASE_UNAVAILABLE", "互助数据库暂不可用")
		return
	}
	actor, _ := serviceauth.ActorFromContext(r.Context())
	var in struct {
		Channels []string `json:"channels"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeCampusError(w, http.StatusBadRequest, "INVALID_REQUEST", "请求格式有误")
		return
	}
	ex, err := src.portalDB.RequestExchange(chi.URLParam(r, "id"), actor.UserID, actor.DisplayName, in.Channels)
	switch {
	case errors.Is(err, campus.ErrInvalidInput):
		writeCampusError(w, http.StatusBadRequest, "INVALID_CHANNELS", "请至少选择一种联系方式（QQ / 微信 / 手机号）")
		return
	case errors.Is(err, campus.ErrContactNotConfigured):
		writeCampusError(w, http.StatusBadRequest, "CONTACT_NOT_CONFIGURED", "你所选渠道尚未填写，请先在「联系方式簿」中配置")
		return
	case errors.Is(err, campus.ErrForbidden):
		writeCampusError(w, http.StatusForbidden, "SELF_EXCHANGE_FORBIDDEN", "不能对自己发布的单子发起交换")
		return
	case errors.Is(err, campus.ErrNotFound):
		writeCampusError(w, http.StatusNotFound, "NOT_FOUND", "单子不存在")
		return
	case errors.Is(err, campus.ErrConflict):
		writeCampusError(w, http.StatusConflict, "ITEM_NOT_OPEN", "当前单子状态不可发起接单交换")
		return
	case err != nil:
		writeCampusError(w, http.StatusServiceUnavailable, "DATABASE_ERROR", "发起联系方式交换失败")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusCreated, map[string]any{"exchange": ex, "request_id": requestIDOf(w)})
}

func handleRespondCampusExchange(w http.ResponseWriter, r *http.Request, src campusSource) {
	if src.portalDB == nil {
		writeCampusError(w, http.StatusServiceUnavailable, "PORTAL_DATABASE_UNAVAILABLE", "互助数据库暂不可用")
		return
	}
	actor, _ := serviceauth.ActorFromContext(r.Context())
	var in struct {
		Decision string   `json:"decision"`
		Channels []string `json:"channels"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeCampusError(w, http.StatusBadRequest, "INVALID_REQUEST", "请求格式有误")
		return
	}
	ex, itemStatus, err := src.portalDB.RespondExchange(chi.URLParam(r, "id"), chi.URLParam(r, "exchange_id"), actor.UserID, in.Decision, in.Channels)
	switch {
	case errors.Is(err, campus.ErrInvalidInput):
		writeCampusError(w, http.StatusBadRequest, "INVALID_DECISION", "decision 必须为 accept 或 decline")
		return
	case errors.Is(err, campus.ErrContactNotConfigured):
		writeCampusError(w, http.StatusBadRequest, "CONTACT_NOT_CONFIGURED", "请先在「联系方式簿」填写你希望共享的联系方式")
		return
	case errors.Is(err, campus.ErrForbidden):
		writeCampusError(w, http.StatusForbidden, "FORBIDDEN", "只有发单人可以处理交换申请")
		return
	case errors.Is(err, campus.ErrNotFound):
		writeCampusError(w, http.StatusNotFound, "NOT_FOUND", "单子或交换记录不存在")
		return
	case errors.Is(err, campus.ErrItemNotOpen):
		writeCampusError(w, http.StatusConflict, "ITEM_NOT_OPEN", "单子状态已变化，无法同意交换。请刷新查看最新状态。")
		return
	case errors.Is(err, campus.ErrConflict):
		writeCampusError(w, http.StatusConflict, "EXCHANGE_ALREADY_SETTLED", "该申请已被处理，请刷新查看最新状态")
		return
	case err != nil:
		writeCampusError(w, http.StatusServiceUnavailable, "DATABASE_ERROR", "处理交换申请失败")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"exchange": ex, "item_status": itemStatus, "request_id": requestIDOf(w)})
}

func handleCompleteCampusItem(w http.ResponseWriter, r *http.Request, src campusSource) {
	if src.portalDB == nil {
		writeCampusError(w, http.StatusServiceUnavailable, "PORTAL_DATABASE_UNAVAILABLE", "互助数据库暂不可用")
		return
	}
	actor, _ := serviceauth.ActorFromContext(r.Context())
	itemID := chi.URLParam(r, "id")
	err := src.portalDB.CompleteItem(itemID, actor.UserID)
	switch {
	case errors.Is(err, campus.ErrNotFound):
		writeCampusError(w, http.StatusNotFound, "NOT_FOUND", "单子不存在")
		return
	case errors.Is(err, campus.ErrForbidden):
		writeCampusError(w, http.StatusForbidden, "FORBIDDEN", "只有发单人可以标记完成")
		return
	case errors.Is(err, campus.ErrConflict):
		writeCampusError(w, http.StatusConflict, "INVALID_STATE", "当前状态不可标记为已完成")
		return
	case err != nil:
		writeCampusError(w, http.StatusServiceUnavailable, "DATABASE_ERROR", "标记完成失败")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"item_id": itemID, "status": "done", "request_id": requestIDOf(w)})
}

func handleCampusConsoleSummary(w http.ResponseWriter, r *http.Request, src campusSource) {
	now := time.Now().UTC()
	if src.portalDB == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"data": map[string]any{
				"id":             "campus",
				"status":         "partial",
				"status_message": "校园互助数据库未连接",
				"as_of":          now,
				"metrics":        []map[string]string{},
			},
			"request_id": requestIDOf(w),
		})
		return
	}
	ws, err := src.portalDB.ConsoleWorkspace()
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"data": map[string]any{
				"id":             "campus",
				"status":         "partial",
				"status_message": "校园互助统计暂时不可读",
				"as_of":          now,
				"metrics":        []map[string]string{},
			},
			"request_id": requestIDOf(w),
		})
		return
	}
	metrics := []map[string]string{
		{"label": "待接单/在售", "value": fmt.Sprintf("%d", ws.Counts.Open)},
		{"label": "进行中", "value": fmt.Sprintf("%d", ws.Counts.Ongoing)},
		{"label": "已完成", "value": fmt.Sprintf("%d", ws.Counts.Done)},
		{"label": "已隐藏", "value": fmt.Sprintf("%d", ws.Counts.Hidden)},
		{"label": "成功交换联系", "value": fmt.Sprintf("%d", ws.Counts.ExchangesAccepted)},
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"id":             "campus",
			"status":         "ok",
			"status_message": fmt.Sprintf("公开单子 %d 条、进行中 %d 条、已隐藏 %d 条，已达成联系交换 %d 次", ws.Counts.Open, ws.Counts.Ongoing, ws.Counts.Hidden, ws.Counts.ExchangesAccepted),
			"as_of":          ws.GeneratedAt,
			"metrics":        metrics,
		},
		"request_id": requestIDOf(w),
	})
}

func handleCampusConsoleWorkspace(w http.ResponseWriter, r *http.Request, src campusSource) {
	actor, _ := serviceauth.ActorFromContext(r.Context())
	if actor.Permission != "campus.read" {
		writeCampusError(w, http.StatusForbidden, "PERMISSION_DENIED", "campus.read permission is required")
		return
	}
	if src.portalDB == nil {
		writeCampusError(w, http.StatusServiceUnavailable, "PORTAL_DATABASE_UNAVAILABLE", "互助数据库暂不可用")
		return
	}
	ws, err := src.portalDB.ConsoleWorkspace()
	if err != nil {
		writeCampusError(w, http.StatusServiceUnavailable, "DATABASE_ERROR", "加载校园互助治理快照失败")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"data": ws, "request_id": requestIDOf(w)})
}

func handleCampusModerateItem(w http.ResponseWriter, r *http.Request, src campusSource) {
	actor, _ := serviceauth.ActorFromContext(r.Context())
	if actor.Permission != "campus.moderate" {
		writeCampusError(w, http.StatusForbidden, "PERMISSION_DENIED", "campus.moderate permission is required")
		return
	}
	if src.portalDB == nil {
		writeCampusError(w, http.StatusServiceUnavailable, "PORTAL_DATABASE_UNAVAILABLE", "互助数据库暂不可用")
		return
	}
	var in struct {
		ExpectedVersion int    `json:"expected_version"`
		Action          string `json:"action"`
		Note            string `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeCampusError(w, http.StatusBadRequest, "INVALID_REQUEST", "治理请求体无效")
		return
	}
	itemID := chi.URLParam(r, "id")
	nextStatus, newVersion, err := src.portalDB.ModerateItem(itemID, actor.UserID, in.ExpectedVersion, in.Action, in.Note)
	switch {
	case errors.Is(err, campus.ErrInvalidInput):
		writeCampusError(w, http.StatusBadRequest, "INVALID_INPUT", "action 或 expected_version 无效")
		return
	case errors.Is(err, campus.ErrConflict):
		writeCampusError(w, http.StatusConflict, "VERSION_CONFLICT", "帖子不存在或版本已变动")
		return
	case err != nil:
		writeCampusError(w, http.StatusServiceUnavailable, "DATABASE_ERROR", "执行治理操作失败")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"id":              itemID,
			"status":          nextStatus,
			"version":         newVersion,
			"moderation_note": strings.TrimSpace(in.Note),
		},
		"request_id": requestIDOf(w),
	})
}

func handleCampusModerateMessage(w http.ResponseWriter, r *http.Request, src campusSource) {
	actor, _ := serviceauth.ActorFromContext(r.Context())
	if actor.Permission != "campus.moderate" {
		writeCampusError(w, http.StatusForbidden, "PERMISSION_DENIED", "campus.moderate permission is required")
		return
	}
	if src.portalDB == nil {
		writeCampusError(w, http.StatusServiceUnavailable, "PORTAL_DATABASE_UNAVAILABLE", "互助数据库暂不可用")
		return
	}
	var in struct {
		Action string `json:"action"`
		Note   string `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeCampusError(w, http.StatusBadRequest, "INVALID_REQUEST", "留言治理请求体无效")
		return
	}
	msgID := chi.URLParam(r, "id")
	hidden, err := src.portalDB.ModerateMessage(msgID, actor.UserID, in.Action, in.Note)
	switch {
	case errors.Is(err, campus.ErrInvalidInput):
		writeCampusError(w, http.StatusBadRequest, "INVALID_INPUT", "action 必须为 hide 或 restore")
		return
	case errors.Is(err, campus.ErrNotFound):
		writeCampusError(w, http.StatusNotFound, "NOT_FOUND", "留言不存在")
		return
	case err != nil:
		writeCampusError(w, http.StatusServiceUnavailable, "DATABASE_ERROR", "留言治理失败")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{
		"data":       map[string]any{"id": msgID, "hidden": hidden},
		"request_id": requestIDOf(w),
	})
}
