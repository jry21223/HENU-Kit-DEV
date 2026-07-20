package quizcraft

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
)

type platformClient struct {
	baseURL, clientID, secret, keyID string
	httpClient                       *http.Client
}

type platformExchange struct {
	UserID, Token string
	ExpiresAt     time.Time
}

func newPlatformClient(rawURL, clientID, secret, keyID string, client *http.Client) (*platformClient, error) {
	parsed, err := url.Parse(strings.TrimRight(rawURL, "/"))
	loopback := err == nil && parsed.Scheme == "http" && (parsed.Hostname() == "localhost" || net.ParseIP(parsed.Hostname()).IsLoopback())
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Scheme != "https" && !loopback) || clientID == "" || len(secret) < 32 || keyID == "" {
		return nil, errors.New("invalid Platform Core client configuration")
	}
	return &platformClient{baseURL: strings.TrimRight(rawURL, "/"), clientID: clientID, secret: secret, keyID: keyID, httpClient: client}, nil
}

func (client *platformClient) signedRequest(ctx context.Context, method, path string, body []byte) (*http.Request, error) {
	request, err := http.NewRequestWithContext(ctx, method, client.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	nonceBytes := make([]byte, 24)
	if _, err := rand.Read(nonceBytes); err != nil {
		return nil, err
	}
	nonce := base64.RawURLEncoding.EncodeToString(nonceBytes)
	timestamp := fmt.Sprintf("%d", time.Now().Unix())
	digest := sha256.Sum256(body)
	canonical := strings.Join([]string{method, request.URL.RequestURI(), timestamp, nonce, hex.EncodeToString(digest[:])}, "\n")
	mac := hmac.New(sha256.New, []byte(client.secret))
	_, _ = mac.Write([]byte(canonical))
	request.SetBasicAuth(client.clientID, client.secret)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Service-Id", client.clientID)
	request.Header.Set("X-Key-Id", client.keyID)
	request.Header.Set("X-Timestamp", timestamp)
	request.Header.Set("X-Nonce", nonce)
	request.Header.Set("X-Signature", base64.RawURLEncoding.EncodeToString(mac.Sum(nil)))
	return request, nil
}

func (client *platformClient) exchange(ctx context.Context, code, redirectURI, verifier, idempotencyKey string) (platformExchange, error) {
	body, _ := json.Marshal(map[string]string{"grant_type": "authorization_code", "code": code, "redirect_uri": redirectURI, "client_id": client.clientID, "code_verifier": verifier})
	request, err := client.signedRequest(ctx, http.MethodPost, "/api/v1/oauth/token", body)
	if err != nil {
		return platformExchange{}, err
	}
	request.Header.Set("Idempotency-Key", idempotencyKey)
	response, err := client.httpClient.Do(request)
	if err != nil {
		return platformExchange{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
		return platformExchange{}, errors.New("platform Core rejected authorization code")
	}
	var envelope struct {
		Data struct {
			User struct {
				ID string `json:"id"`
			} `json:"user"`
			SessionExchangeToken string    `json:"session_exchange_token"`
			ExpiresAt            time.Time `json:"expires_at"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&envelope); err != nil || len(envelope.Data.SessionExchangeToken) < 32 || envelope.Data.ExpiresAt.IsZero() {
		return platformExchange{}, errors.New("invalid Platform Core exchange response")
	}
	if _, err := uuid.Parse(envelope.Data.User.ID); err != nil {
		return platformExchange{}, errors.New("invalid Platform Core user")
	}
	return platformExchange{UserID: envelope.Data.User.ID, Token: envelope.Data.SessionExchangeToken, ExpiresAt: envelope.Data.ExpiresAt}, nil
}

func (client *platformClient) check(ctx context.Context, token, permission string, scope map[string]string) error {
	body, _ := json.Marshal(map[string]any{"session_exchange_token": token, "permission_code": permission, "scope": scope})
	request, err := client.signedRequest(ctx, http.MethodPost, "/api/v1/authorization/check", body)
	if err != nil {
		return err
	}
	response, err := client.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
	switch response.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusUnauthorized:
		return errPlatformUnauthorized
	case http.StatusForbidden:
		return errPlatformForbidden
	default:
		return errors.New("platform Core authorization unavailable")
	}
}

func (client *platformClient) createInboxItem(ctx context.Context, token, idempotencyKey string, body []byte) (uuid.UUID, error) {
	request, err := client.signedRequest(ctx, http.MethodPost, "/api/v1/operations-inbox/items", body)
	if err != nil {
		return uuid.Nil, err
	}
	request.Header.Set("X-Session-Exchange-Token", token)
	request.Header.Set("Idempotency-Key", idempotencyKey)
	response, err := client.httpClient.Do(request)
	if err != nil {
		return uuid.Nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
		return uuid.Nil, fmt.Errorf("operations Inbox returned %d", response.StatusCode)
	}
	var envelope struct {
		Data struct {
			ID uuid.UUID `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&envelope); err != nil || envelope.Data.ID == uuid.Nil {
		return uuid.Nil, errors.New("invalid operations Inbox response")
	}
	return envelope.Data.ID, nil
}

var (
	errPlatformUnauthorized = errors.New("platform Core session is invalid")
	errPlatformForbidden    = errors.New("platform Core denied access")
)

type quizcraftSessionCodec struct{ aead cipher.AEAD }

func newQuizcraftSessionCodec(key []byte) (*quizcraftSessionCodec, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &quizcraftSessionCodec{aead: aead}, nil
}

func (codec *quizcraftSessionCodec) encode(value any, purpose string) (string, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, codec.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := codec.aead.Seal(nonce, nonce, payload, []byte(purpose))
	return base64.RawURLEncoding.EncodeToString(sealed), nil
}

func (codec *quizcraftSessionCodec) decode(encoded, purpose string, value any) error {
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil || len(raw) < codec.aead.NonceSize() {
		return errors.New("invalid encrypted cookie")
	}
	nonce := raw[:codec.aead.NonceSize()]
	payload, err := codec.aead.Open(nil, nonce, raw[codec.aead.NonceSize():], []byte(purpose))
	if err != nil {
		return errors.New("invalid encrypted cookie")
	}
	return json.Unmarshal(payload, value)
}

type oauthState struct {
	State       string    `json:"state"`
	Verifier    string    `json:"verifier"`
	ExchangeKey string    `json:"exchange_key"`
	ReturnTo    string    `json:"return_to"`
	ExpiresAt   time.Time `json:"expires_at"`
}
type localPlatformSession struct {
	UserID        string    `json:"user_id"`
	ExchangeToken string    `json:"exchange_token"`
	ExpiresAt     time.Time `json:"expires_at"`
}

func (service *practiceHTTP) startPlatformLogin(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Referrer-Policy", "no-referrer")
	if service.platform == nil {
		writeError(writer, http.StatusServiceUnavailable, "login_unavailable", "Platform Core login is not configured")
		return
	}
	stateBytes, verifierBytes := make([]byte, 24), make([]byte, 32)
	if _, err := rand.Read(stateBytes); err != nil {
		writeError(writer, http.StatusServiceUnavailable, "login_unavailable", "could not start login")
		return
	}
	if _, err := rand.Read(verifierBytes); err != nil {
		writeError(writer, http.StatusServiceUnavailable, "login_unavailable", "could not start login")
		return
	}
	state, verifier := base64.RawURLEncoding.EncodeToString(stateBytes), base64.RawURLEncoding.EncodeToString(verifierBytes)
	returnTo := request.URL.Query().Get("return_to")
	if !validQuizcraftReturnTo(returnTo) {
		returnTo = "/extract"
	}
	encoded, err := service.sessionCodec.encode(oauthState{State: state, Verifier: verifier, ExchangeKey: "idem_quizcraft_oauth_" + uuid.NewString(), ReturnTo: returnTo, ExpiresAt: service.now().Add(5 * time.Minute)}, "quizcraft-oauth-state-v1")
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, "login_unavailable", "could not start login")
		return
	}
	http.SetCookie(writer, &http.Cookie{Name: "__Host-quizcraft_oauth", Value: encoded, Path: "/", MaxAge: 300, HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode})
	challenge := sha256.Sum256([]byte(verifier))
	query := url.Values{"response_type": {"code"}, "client_id": {service.platform.clientID}, "redirect_uri": {service.publicURL + "/auth/callback"}, "state": {state}, "code_challenge": {base64.RawURLEncoding.EncodeToString(challenge[:])}, "code_challenge_method": {"S256"}}
	http.Redirect(writer, request, service.platform.baseURL+"/api/v1/oauth/authorize?"+query.Encode(), http.StatusFound)
}

func (service *practiceHTTP) finishPlatformLogin(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Referrer-Policy", "no-referrer")
	if service.platform == nil {
		writeError(writer, http.StatusServiceUnavailable, "login_unavailable", "Platform Core login is not configured")
		return
	}
	cookie, err := request.Cookie("__Host-quizcraft_oauth")
	var state oauthState
	if err != nil || service.sessionCodec.decode(cookie.Value, "quizcraft-oauth-state-v1", &state) != nil || service.now().After(state.ExpiresAt) || request.URL.Query().Get("state") != state.State || request.URL.Query().Get("code") == "" || !strings.HasPrefix(state.ExchangeKey, "idem_quizcraft_oauth_") {
		writeError(writer, http.StatusBadRequest, "invalid_oauth_callback", "login state is invalid or expired")
		return
	}
	exchange, err := service.platform.exchange(request.Context(), request.URL.Query().Get("code"), service.publicURL+"/auth/callback", state.Verifier, state.ExchangeKey)
	if err != nil {
		writeError(writer, http.StatusUnauthorized, "login_failed", "Platform Core rejected login")
		return
	}
	encoded, err := service.sessionCodec.encode(localPlatformSession{UserID: exchange.UserID, ExchangeToken: exchange.Token, ExpiresAt: exchange.ExpiresAt}, "quizcraft-platform-session-v1")
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, "login_unavailable", "could not create QuizCraft session")
		return
	}
	maxAge := int(time.Until(exchange.ExpiresAt).Seconds())
	if maxAge < 1 {
		writeError(writer, http.StatusUnauthorized, "login_failed", "Platform Core session already expired")
		return
	}
	http.SetCookie(writer, &http.Cookie{Name: "__Host-quizcraft_session", Value: encoded, Path: "/", MaxAge: maxAge, Expires: exchange.ExpiresAt, HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode})
	http.SetCookie(writer, &http.Cookie{Name: "__Host-quizcraft_oauth", Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode})
	http.Redirect(writer, request, state.ReturnTo, http.StatusFound)
}

func validQuizcraftReturnTo(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && strings.HasPrefix(value, "/") && !strings.HasPrefix(value, "//") && !strings.ContainsAny(value, "\\\r\n") && !parsed.IsAbs() && parsed.Host == ""
}
