package httpapi

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"henukit.dev/platform-core/internal/identity"
	"henukit.dev/platform-core/internal/securebox"
)

var qqSubjectPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

type qqBindingRequest struct {
	Subject      string `json:"subject,omitempty"`
	Token        string `json:"token,omitempty"`
	RequestID    string `json:"request_id,omitempty"`
	SessionToken string `json:"session_token,omitempty"`
}

// qqBinding is a signed service-only boundary. Browser credentials are never
// accepted as Bot identities; the Portal's exchange Session is checked in Core.
func (h *Handler) qqBinding(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	raw, body, ok := decodeInboxBody[qqBindingRequest](w, r)
	if !ok {
		return
	}
	action := strings.TrimPrefix(r.URL.Path, "/api/v1/qq-bindings/")
	client, secret, ok := r.BasicAuth()
	if !ok || client != r.Header.Get("X-Service-Id") {
		writeError(w, r, 401, "CLIENT_AUTH_FAILED", "服务身份验证失败")
		return
	}
	hash := sha256.Sum256(raw)
	if err := h.flow.AuthenticateServiceRequest(r.Context(), identity.ServiceRequestCredentials{
		HTTPMethod: r.Method, ClientID: client, ClientSecret: secret, KeyID: r.Header.Get("X-Key-Id"),
		Timestamp: r.Header.Get("X-Timestamp"), Nonce: r.Header.Get("X-Nonce"), Signature: r.Header.Get("X-Signature"),
		BodyHash: hash[:], PathAndQuery: r.URL.RequestURI(), NonceNamespace: "qq-binding",
	}); err != nil {
		h.writeFlowError(w, r, err)
		return
	}
	portal := client == "portal-gateway"
	if portal {
		if (action != "authorize" && action != "status" && action != "unlink") || body.Subject != "" || body.RequestID != "" || len(body.SessionToken) < 32 || len(body.SessionToken) > 256 {
			writeError(w, r, 403, "BINDING_FORBIDDEN", "此操作不可用")
			return
		}
	} else if !qqSubjectPattern.MatchString(body.Subject) || body.SessionToken != "" || action == "authorize" {
		writeError(w, r, 403, "BINDING_FORBIDDEN", "此操作不可用")
		return
	}
	// Bound caller/subject rate limiting, atomic expiry, and fail closed on Redis loss.
	rateHash := sha256.Sum256([]byte(client + ":" + body.Subject + ":" + body.SessionToken))
	count, err := h.redis.Eval(r.Context(), `local n=redis.call('INCR',KEYS[1]); if n==1 then redis.call('EXPIRE',KEYS[1],60) end; return n`, []string{"qq-binding-rate:" + base64.RawURLEncoding.EncodeToString(rateHash[:])}).Int()
	if err != nil {
		h.writeFlowError(w, r, identity.ErrDependency)
		return
	}
	if count > 60 {
		writeError(w, r, 429, "RATE_LIMITED", "操作过于频繁，请稍后重试")
		return
	}
	tx, err := h.database.Begin(r.Context())
	if err != nil {
		h.writeFlowError(w, r, err)
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()
	// Serialize short binding transactions across both identity uniqueness axes.
	// This also fences unlink against in-flight confirms and starts.
	if _, err = tx.Exec(r.Context(), `SELECT pg_advisory_xact_lock(522)`); err != nil {
		h.writeFlowError(w, r, err)
		return
	}
	var app, user, session string
	if portal {
		tokenHash := sha256.Sum256([]byte(body.SessionToken))
		err = tx.QueryRow(r.Context(), `SELECT s.user_id::text,s.id::text FROM sessions s JOIN sessions p ON p.id=s.parent_session_id JOIN users u ON u.id=s.user_id WHERE s.token_hash=$1 AND s.kind='client_exchange' AND s.client_id='portal-gateway' AND s.revoked_at IS NULL AND s.expires_at>now() AND p.revoked_at IS NULL AND p.expires_at>now() AND u.status='active' AND u.email_verified FOR SHARE OF s,p,u`, tokenHash[:]).Scan(&user, &session)
	} else {
		err = tx.QueryRow(r.Context(), `SELECT app_id FROM qq_binding_apps WHERE client_id=$1`, client).Scan(&app)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, r, 403, "BINDING_FORBIDDEN", "登录已失效或绑定服务未开通，请重新登录后重试")
		return
	}
	if err != nil {
		h.writeFlowError(w, r, err)
		return
	}
	result, err := h.performQQBinding(r, tx, action, body, app, user, session)
	if err != nil {
		var failure *qqBindingError
		if errors.As(err, &failure) {
			writeError(w, r, failure.status, failure.code, failure.message)
		} else {
			h.writeFlowError(w, r, err)
		}
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		h.writeFlowError(w, r, err)
		return
	}
	writeSuccess(w, r, 200, result)
}

type qqBindingError struct {
	status        int
	code, message string
}

func (e *qqBindingError) Error() string                { return e.code }
func qqFailure(status int, code, message string) error { return &qqBindingError{status, code, message} }

func (h *Handler) performQQBinding(r *http.Request, tx pgx.Tx, action string, b qqBindingRequest, app, user, session string) (map[string]any, error) {
	ctx := r.Context()
	if action == "start" {
		if _, err := uuid.Parse(b.RequestID); err != nil || b.Token != "" {
			return nil, qqFailure(400, "INVALID_REQUEST", "请重新发起绑定")
		}
		codec, err := securebox.New(h.deviceKey, "qq-binding-link")
		if err != nil {
			return nil, err
		}
		var encrypted []byte
		var state string
		var expires time.Time
		err = tx.QueryRow(ctx, `SELECT token_ciphertext,state,expires_at FROM qq_binding_challenges WHERE app_id=$1 AND subject=$2 AND request_id=$3`, app, b.Subject, b.RequestID).Scan(&encrypted, &state, &expires)
		if err == nil {
			if state == "cancelled" || !time.Now().Before(expires) {
				return nil, qqFailure(410, "LINK_EXPIRED", "绑定链接已失效，请重新发起")
			}
			token, err := codec.Open(encrypted)
			return map[string]any{"token": string(token), "expires_at": expires, "state": state}, err
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
		var exists bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM qq_bindings WHERE app_id=$1 AND subject=$2)`, app, b.Subject).Scan(&exists); err != nil {
			return nil, err
		}
		if exists {
			return nil, qqFailure(409, "ALREADY_BOUND", "已绑定账号，请先解绑再换绑")
		}
		// A new request invalidates old links for this QQ, including already-authorized links.
		if _, err = tx.Exec(ctx, `UPDATE qq_binding_challenges SET state='cancelled' WHERE app_id=$1 AND subject=$2 AND state IN ('pending','authorized')`, app, b.Subject); err != nil {
			return nil, err
		}
		token := make([]byte, 32)
		if _, err = rand.Read(token); err != nil {
			return nil, err
		}
		value := base64.RawURLEncoding.EncodeToString(token)
		digest := sha256.Sum256([]byte(value))
		encrypted, err = codec.Seal([]byte(value))
		if err != nil {
			return nil, err
		}
		err = tx.QueryRow(ctx, `INSERT INTO qq_binding_challenges(token_hash,token_ciphertext,app_id,subject,request_id) VALUES($1,$2,$3,$4,$5) RETURNING expires_at`, digest[:], encrypted, app, b.Subject, b.RequestID).Scan(&expires)
		return map[string]any{"token": value, "expires_at": expires, "state": "pending"}, err
	}
	if action == "status" || action == "resolve" || action == "unlink" {
		if b.Token != "" || b.RequestID != "" {
			return nil, qqFailure(400, "INVALID_REQUEST", "请求无效，请重试")
		}
		var boundApp, boundSubject, boundUser, name string
		// An inactive account loses resolve authority, but its QQ owner must still
		// be able to remove the binding permanently (including after reactivation).
		err := tx.QueryRow(ctx, `SELECT b.app_id,b.subject,b.user_id::text,COALESCE(u.display_name,'') FROM qq_bindings b JOIN users u ON u.id=b.user_id WHERE (($1<>'' AND b.user_id::text=$1) OR ($1='' AND b.app_id=$2 AND b.subject=$3)) AND ($4='unlink' OR (u.status='active' AND u.email_verified))`, user, app, b.Subject, action).Scan(&boundApp, &boundSubject, &boundUser, &name)
		if errors.Is(err, pgx.ErrNoRows) {
			if action == "resolve" {
				return nil, qqFailure(404, "NOT_BOUND", "请先绑定 HENU KIT 账号")
			}
			// Cancel pre-binding approvals as well; unlink must not leave a live approval behind.
			if action == "unlink" {
				_, err = tx.Exec(ctx, `UPDATE qq_binding_challenges SET state='cancelled' WHERE (user_id::text=$1 AND $1<>'') OR (app_id=$2 AND subject=$3 AND $2<>'')`, user, app, b.Subject)
				if err != nil {
					return nil, err
				}
			}
			return map[string]any{"bound": false}, nil
		}
		if err != nil {
			return nil, err
		}
		if action == "unlink" {
			if _, err = tx.Exec(ctx, `UPDATE qq_binding_challenges SET state='cancelled' WHERE (app_id=$1 AND subject=$2) OR user_id=$3`, boundApp, boundSubject, boundUser); err != nil {
				return nil, err
			}
			if _, err = tx.Exec(ctx, `DELETE FROM qq_bindings WHERE app_id=$1 AND subject=$2`, boundApp, boundSubject); err != nil {
				return nil, err
			}
			if _, err = tx.Exec(ctx, `INSERT INTO qq_binding_audit(app_id,user_id,action) VALUES($1,$2,'unlinked')`, boundApp, boundUser); err != nil {
				return nil, err
			}
			return map[string]any{"bound": false}, nil
		}
		return map[string]any{"bound": true, "user_id": boundUser, "display_name": name}, nil
	}
	if action != "authorize" && action != "confirm" && action != "pending" {
		return nil, qqFailure(404, "NOT_FOUND", "操作不存在")
	}
	token, err := base64.RawURLEncoding.DecodeString(b.Token)
	if err != nil || len(token) != 32 || b.RequestID != "" {
		return nil, qqFailure(400, "INVALID_LINK", "绑定链接无效，请重新发起")
	}
	digest := sha256.Sum256([]byte(b.Token))
	var targetApp, subject, state, approvedUser, approvedSession string
	var expires time.Time
	err = tx.QueryRow(ctx, `SELECT app_id,subject,state,COALESCE(user_id::text,''),COALESCE(session_id::text,''),expires_at FROM qq_binding_challenges WHERE token_hash=$1 FOR UPDATE`, digest[:]).Scan(&targetApp, &subject, &state, &approvedUser, &approvedSession, &expires)
	if errors.Is(err, pgx.ErrNoRows) || (app != "" && (targetApp != app || subject != b.Subject)) {
		return nil, qqFailure(404, "LINK_NOT_FOUND", "绑定链接无效，请重新发起")
	}
	if err != nil {
		return nil, err
	}
	if state == "cancelled" || !time.Now().Before(expires) {
		return nil, qqFailure(410, "LINK_EXPIRED", "绑定链接已失效，请重新发起")
	}
	if action == "authorize" {
		if state != "pending" && approvedUser != user {
			return nil, qqFailure(409, "LINK_ALREADY_AUTHORIZED", "此链接已授权其他账号，请回到 QQ 重新发起")
		}
		var conflict bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM qq_bindings WHERE (user_id=$1 OR (app_id=$2 AND subject=$3)) AND NOT (user_id=$1 AND app_id=$2 AND subject=$3))`, user, targetApp, subject).Scan(&conflict); err != nil {
			return nil, err
		}
		if conflict {
			return nil, qqFailure(409, "ALREADY_BOUND", "账号已存在其他绑定，请先解绑")
		}
		if state == "pending" {
			_, err = tx.Exec(ctx, `UPDATE qq_binding_challenges SET state='authorized',user_id=$2,session_id=$3 WHERE token_hash=$1`, digest[:], user, session)
		}
		return map[string]any{"state": "authorized"}, err
	}
	if state == "pending" {
		if action == "pending" {
			return map[string]any{"state": "pending"}, nil
		}
		return nil, qqFailure(409, "WEB_APPROVAL_REQUIRED", "请先在网页登录并授权，再回到 QQ 确认")
	}
	// Revalidate the exact consenting Session and parent at the final confirmation.
	var name string
	err = tx.QueryRow(ctx, `SELECT COALESCE(u.display_name,'') FROM sessions s JOIN sessions p ON p.id=s.parent_session_id JOIN users u ON u.id=s.user_id WHERE s.id=$1 AND s.user_id=$2 AND s.revoked_at IS NULL AND s.expires_at>now() AND p.revoked_at IS NULL AND p.expires_at>now() AND u.status='active' AND u.email_verified FOR SHARE OF s,p,u`, approvedSession, approvedUser).Scan(&name)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, qqFailure(410, "APPROVAL_EXPIRED", "授权登录已失效，请重新发起绑定")
	}
	if err != nil {
		return nil, err
	}
	if action == "pending" {
		return map[string]any{"state": state, "display_name": name}, nil
	}
	if state == "confirmed" {
		var matches bool
		err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM qq_bindings WHERE app_id=$1 AND subject=$2 AND user_id=$3)`, app, b.Subject, approvedUser).Scan(&matches)
		if err != nil {
			return nil, err
		}
		if !matches {
			return nil, qqFailure(410, "LINK_EXPIRED", "绑定已解除，请重新发起")
		}
		return map[string]any{"bound": true, "display_name": name}, nil
	}
	var conflict bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM qq_bindings WHERE user_id=$1 OR (app_id=$2 AND subject=$3))`, approvedUser, app, b.Subject).Scan(&conflict); err != nil {
		return nil, err
	}
	if conflict {
		return nil, qqFailure(409, "ALREADY_BOUND", "账号已存在其他绑定，请先解绑")
	}
	if _, err = tx.Exec(ctx, `INSERT INTO qq_bindings(app_id,subject,user_id) VALUES($1,$2,$3)`, app, b.Subject, approvedUser); err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `UPDATE qq_binding_challenges SET state='confirmed' WHERE token_hash=$1`, digest[:]); err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO qq_binding_audit(app_id,user_id,action) VALUES($1,$2,'bound')`, app, approvedUser); err != nil {
		return nil, err
	}
	return map[string]any{"bound": true, "display_name": name}, nil
}
