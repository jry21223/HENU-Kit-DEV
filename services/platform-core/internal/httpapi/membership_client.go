package httpapi

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

var (
	membershipConfigError    = errors.New("membership owner configuration unavailable")
	membershipTransportError = errors.New("membership owner transport unavailable")
	membershipContractError  = errors.New("membership owner contract invalid")
	membershipStatusError    = errors.New("membership owner rejected read")
)

// MembershipClient reads only the owner membership route. It never uses Portal credentials.
type MembershipClient struct {
	baseURL, clientID, keyID, secret string
	client                           *http.Client
}

func NewMembershipClient(baseURL, clientID, keyID, secret string) (*MembershipClient, error) {
	if baseURL == "" && clientID == "" && keyID == "" && secret == "" {
		return nil, nil
	}
	u, err := url.Parse(baseURL)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || (u.Scheme != "https" && u.Scheme != "http") || (strings.TrimSpace(clientID) == "" || clientID == "portal-gateway" || clientID == "console-gateway") || strings.TrimSpace(keyID) == "" || len(secret) < 32 {
		return nil, errors.New("membership owner configuration is incomplete or invalid")
	}
	return &MembershipClient{strings.TrimSuffix(baseURL, "/"), clientID, keyID, secret, &http.Client{Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func (c *MembershipClient) read(ctx context.Context, user string) (string, error) {
	if c == nil {
		return "", membershipConfigError
	}
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/api/v1/account/membership", nil)
	if err != nil {
		return "", membershipTransportError
	}
	nonceBytes := make([]byte, 24)
	if _, err = rand.Read(nonceBytes); err != nil {
		return "", membershipTransportError
	}
	nonce := base64.RawURLEncoding.EncodeToString(nonceBytes)
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	digest := sha256.Sum256(nil)
	canonical := strings.Join([]string{req.Method, req.URL.RequestURI(), timestamp, nonce, hex.EncodeToString(digest[:]), user}, "\n")
	mac := hmac.New(sha256.New, []byte(c.secret))
	_, _ = mac.Write([]byte(canonical))
	req.SetBasicAuth(c.clientID, c.secret)
	for k, v := range map[string]string{"X-Service-Id": c.clientID, "X-Key-Id": c.keyID, "X-Timestamp": timestamp, "X-Nonce": nonce, "X-Actor-User-Id": user, "X-Signature": base64.RawURLEncoding.EncodeToString(mac.Sum(nil))} {
		req.Header.Set(k, v)
	}
	response, err := c.client.Do(req)
	if err != nil {
		return "", membershipTransportError
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return "", membershipStatusError
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, 4097))
	if err != nil || len(raw) > 4096 {
		return "", membershipContractError
	}
	var envelope struct {
		Data *struct {
			Plan     string `json:"plan"`
			Lifetime *bool  `json:"lifetime"`
		} `json:"data"`
	}
	if err = json.Unmarshal(raw, &envelope); err != nil || envelope.Data == nil || envelope.Data.Lifetime == nil {
		return "", membershipContractError
	}
	data := envelope.Data
	if (data.Plan != "free" && data.Plan != "lifetime") || *data.Lifetime != (data.Plan == "lifetime") {
		return "", membershipContractError
	}
	return data.Plan, nil
}
func (h *Handler) qqBenefits(w http.ResponseWriter, r *http.Request, resolved map[string]any, app, subject string) {
	clientID, botSecret, _ := r.BasicAuth()
	if h.membership != nil && (h.membership.clientID == clientID || hmac.Equal([]byte(h.membership.secret), []byte(botSecret))) {
		h.membershipUnavailable(w, r, "owner_config")
		return
	}
	user, _ := resolved["user_id"].(string)
	plan, err := h.membership.read(r.Context(), user)
	if err != nil {
		category := "owner_contract"
		switch {
		case errors.Is(err, membershipConfigError):
			category = "owner_config"
		case errors.Is(err, membershipTransportError):
			category = "owner_transport"
		case errors.Is(err, membershipStatusError):
			category = "owner_status"
		}
		h.membershipUnavailable(w, r, category)
		return
	}
	// Owner calls run outside the binding transaction. Recheck current binding before replying.
	var current string
	err = h.database.QueryRow(r.Context(), `SELECT b.user_id::text FROM qq_bindings b JOIN users u ON u.id=b.user_id WHERE b.app_id=$1 AND b.subject=$2 AND u.status='active' AND u.email_verified`, app, subject).Scan(&current)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		h.membershipUnavailable(w, r, "binding_recheck")
		return
	}
	if err != nil || current != user {
		writeError(w, r, 404, "NOT_BOUND", "请先绑定 HENU KIT 账号")
		return
	}
	benefits := []string{}
	if plan == "lifetime" {
		benefits = []string{"yuketang", "ucampus", "chaoxing", "library_schedule"}
	}
	writeSuccess(w, r, 200, map[string]any{"bound": true, "plan": plan, "lifetime": plan == "lifetime", "allowed": plan == "lifetime", "benefits": benefits, "checked_at": time.Now().UTC().Format(time.RFC3339)})
}

// Only bounded categories enter logs; errors may contain URLs or transport credentials.
func (h *Handler) membershipUnavailable(w http.ResponseWriter, r *http.Request, category string) {
	switch category {
	case "owner_config", "owner_transport", "owner_contract", "owner_status", "binding_recheck":
	default:
		category = "owner_contract"
	}
	h.logger.Warn("membership_check_failed", "request_id", requestIDFrom(r.Context()), "category", category)
	writeError(w, r, 503, "MEMBERSHIP_UNAVAILABLE", "会员资格暂时无法核验，请稍后重试")
}
