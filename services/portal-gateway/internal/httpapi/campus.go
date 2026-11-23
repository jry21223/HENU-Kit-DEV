package httpapi

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

func (h *Handler) proxyAuthenticatedCampus(w http.ResponseWriter, r *http.Request, upstreamPath string) {
	setPrivateResponseHeaders(w)
	sess, err := h.readSession(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "AUTH_REQUIRED", "请先登录后再操作。")
		return
	}
	if h.campus == nil {
		writeError(w, r, http.StatusServiceUnavailable, "CAMPUS_WRITES_UNAVAILABLE", "互助发单与联系方式交换服务暂未开启。")
		return
	}
	var body []byte
	if r.Body != nil && (r.Method == http.MethodPost || r.Method == http.MethodPut) {
		limit := int64(256 << 10)
		if r.Method == http.MethodPost && upstreamPath == "/api/v1/campus/items" {
			limit = 20 << 20
			// Limit large request buffers before reading or signing the body.
			select {
			case h.campusUploadSlots <- struct{}{}:
				defer func() { <-h.campusUploadSlots }()
			case <-r.Context().Done():
				return
			}
			// A stalled client must release the upload slot, too.
			controller := http.NewResponseController(w)
			if controller.SetReadDeadline(time.Now().Add(2*time.Minute)) == nil {
				defer controller.SetReadDeadline(time.Time{})
			}
		}
		body, err = io.ReadAll(http.MaxBytesReader(w, r.Body, limit))
		if err != nil {
			var oversized *http.MaxBytesError
			if errors.As(err, &oversized) {
				writeError(w, r, http.StatusRequestEntityTooLarge, "INVALID_REQUEST", "请求内容过大，请缩减内容后重试。")
				return
			}
			writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "请求内容读取失败。")
			return
		}
	}
	resp, err := h.campus.Do(r.Context(), r.Method, upstreamPath, sess.UserID, sess.DisplayName, requestIDOf(w, r), body)
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "CAMPUS_UPSTREAM_UNAVAILABLE", "互助服务暂时不可用，请稍后重试。")
		return
	}
	w.Header().Set("Content-Type", resp.ContentType)
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(resp.Body)
}

func (h *Handler) campusGetContactBook(w http.ResponseWriter, r *http.Request) {
	h.proxyAuthenticatedCampus(w, r, "/api/v1/campus/contact-book")
}

func (h *Handler) campusPutContactBook(w http.ResponseWriter, r *http.Request) {
	h.proxyAuthenticatedCampus(w, r, "/api/v1/campus/contact-book")
}

func (h *Handler) campusCreateItem(w http.ResponseWriter, r *http.Request) {
	h.proxyAuthenticatedCampus(w, r, "/api/v1/campus/items")
}

func (h *Handler) campusCreateMessage(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	h.proxyAuthenticatedCampus(w, r, "/api/v1/campus/items/"+url.PathEscape(id)+"/messages")
}

func (h *Handler) campusListExchanges(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	h.proxyAuthenticatedCampus(w, r, "/api/v1/campus/items/"+url.PathEscape(id)+"/exchanges")
}

func (h *Handler) campusRequestExchange(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	h.proxyAuthenticatedCampus(w, r, "/api/v1/campus/items/"+url.PathEscape(id)+"/exchanges")
}

func (h *Handler) campusRespondExchange(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	exID := strings.TrimSpace(chi.URLParam(r, "exchange_id"))
	h.proxyAuthenticatedCampus(w, r, "/api/v1/campus/items/"+url.PathEscape(id)+"/exchanges/"+url.PathEscape(exID)+"/respond")
}

func (h *Handler) campusCompleteItem(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	h.proxyAuthenticatedCampus(w, r, "/api/v1/campus/items/"+url.PathEscape(id)+"/complete")
}
