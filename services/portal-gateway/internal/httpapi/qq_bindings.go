package httpapi

import (
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"strings"
)

func (h *Handler) qqBinding(w http.ResponseWriter, r *http.Request) {
	setPrivateResponseHeaders(w)
	w.Header().Set("Referrer-Policy", "no-referrer")
	// Exact Origin + JSON + same-origin fetch metadata prevents cross-site form
	// and simple-request CSRF. Missing Origin also fails closed on this new API.
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if r.Header.Get("Origin") != h.portalOrigin || (r.Header.Get("Sec-Fetch-Site") != "" && r.Header.Get("Sec-Fetch-Site") != "same-origin") || err != nil || media != "application/json" {
		writeError(w, r, 403, "ORIGIN_REJECTED", "请在 HENU KIT 网站内重试")
		return
	}
	value, err := h.readSession(r)
	if err != nil {
		writeError(w, r, 401, "LOGIN_REQUIRED", "请先登录 HENU KIT")
		return
	}
	var body struct {
		Token string `json:"token"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&body); err != nil {
		writeError(w, r, 400, "INVALID_REQUEST", "请求无效，请重新打开绑定链接")
		return
	}
	if err = decoder.Decode(new(any)); err != io.EOF {
		writeError(w, r, 400, "INVALID_REQUEST", "请求无效，请重新打开绑定链接")
		return
	}
	action := strings.TrimPrefix(r.URL.Path, "/api/v1/account/qq-binding/")
	status, raw, err := h.platform.QQBinding(r.Context(), action, value.ExchangeToken, body.Token)
	if err != nil {
		writeError(w, r, 503, "BINDING_UNAVAILABLE", "绑定服务暂时不可用，请稍后重试")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(raw)
}
