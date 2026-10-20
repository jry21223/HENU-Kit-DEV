package quizcraft

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"henukit.dev/console-gateway/internal/clientutil"
)

type Client struct {
	baseURL, clientID, secret, keyID string
	httpClient                       *http.Client
}

func New(base, clientID, secret, keyID string, client *http.Client) (*Client, error) {
	u, err := url.Parse(base)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || (u.Scheme != "https" && !clientutil.IsTrustedLoopbackHTTP(u, "quizcraft")) || clientID == "" || keyID == "" || len(secret) < 32 {
		return nil, errors.New("invalid QuizCraft management configuration")
	}
	c := http.Client{Timeout: 15 * time.Second}
	if client != nil {
		c = *client
	}
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if c.Timeout == 0 {
		c.Timeout = 15 * time.Second
	}
	return &Client{strings.TrimRight(base, "/"), clientID, secret, keyID, &c}, nil
}

// Match is an allowlist, never a general-purpose owner proxy.
func Match(method, path string) (permission, bank string, ok bool) {
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) == 1 {
		if method == "GET" && (parts[0] == "catalog" || parts[0] == "banks") {
			return "quizcraft.workshop.read", "", true
		}
		if method == "POST" && parts[0] == "banks" {
			return "quizcraft.workshop.write", "", true
		}
		return
	}
	if len(parts) < 3 || parts[0] != "banks" {
		return
	}
	id, err := uuid.Parse(parts[1])
	if err != nil || id == uuid.Nil || id.String() != parts[1] {
		return
	}
	bank = id.String()
	if len(parts) == 3 && method == "POST" {
		switch parts[2] {
		case "versions", "imports":
			return "quizcraft.workshop.write", bank, true
		case "rollback":
			return "quizcraft.workshop.publish", bank, true
		}
	}
	if len(parts) >= 4 && parts[2] == "versions" {
		version, err := uuid.Parse(parts[3])
		if err != nil || version == uuid.Nil || version.String() != parts[3] {
			return
		}
		if len(parts) == 4 && method == "GET" {
			return "quizcraft.workshop.read", bank, true
		}
		if len(parts) == 5 && method == "POST" {
			switch parts[4] {
			case "validate":
				return "quizcraft.workshop.write", bank, true
			case "publish", "unpublish":
				return "quizcraft.workshop.publish", bank, true
			}
		}
	}
	return "", "", false
}
func (c *Client) Request(ctx context.Context, method, path, actor, permission, bank, key string, body []byte) (int, []byte, error) {
	expected, resource, valid := Match(method, path)
	if !valid || expected != permission || resource != bank {
		return 0, nil, errors.New("unsupported management operation")
	}
	r, err := http.NewRequestWithContext(ctx, method, c.baseURL+"/api/v1/console/workshop"+path, bytes.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	n := make([]byte, 24)
	if _, err = rand.Read(n); err != nil {
		return 0, nil, err
	}
	nonce := base64.RawURLEncoding.EncodeToString(n)
	ts := fmt.Sprint(time.Now().Unix())
	d := sha256.Sum256(body)
	canonical := strings.Join([]string{method, r.URL.RequestURI(), ts, nonce, hex.EncodeToString(d[:]), actor, permission, bank, key}, "\n")
	mac := hmac.New(sha256.New, []byte(c.secret))
	_, _ = mac.Write([]byte(canonical))
	r.SetBasicAuth(c.clientID, c.secret)
	for k, v := range map[string]string{"Content-Type": "application/json", "X-Service-Id": c.clientID, "X-Key-Id": c.keyID, "X-Timestamp": ts, "X-Nonce": nonce, "X-Signature": base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), "X-Actor-User-Id": actor, "X-Permission-Code": permission, "X-Resource-Id": bank, "Idempotency-Key": key} {
		r.Header.Set(k, v)
	}
	res, err := c.httpClient.Do(r)
	if err != nil {
		return 0, nil, err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, (8<<20)+1))
	if err != nil || len(raw) > 8<<20 || !json.Valid(raw) {
		return 0, nil, errors.New("invalid QuizCraft management response")
	}
	switch res.StatusCode {
	case 200, 201, 400, 401, 403, 404, 409, 413, 422, 503:
		return res.StatusCode, raw, nil
	default:
		return 0, nil, errors.New("unexpected QuizCraft management status")
	}
}
