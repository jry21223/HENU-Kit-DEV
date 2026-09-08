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

type platformInboxItem struct {
	ID                 string    `json:"id"`
	SourceProductCode  string    `json:"source_product_code"`
	SourceResourceType string    `json:"source_resource_type"`
	SourceResourceID   string    `json:"source_resource_id"`
	Status             string    `json:"status"`
	Version            int64     `json:"version"`
	UpdatedAt          time.Time `json:"updated_at"`
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

func (client *platformClient) getInboxItem(ctx context.Context, token string, itemID uuid.UUID, productCode, resourceType, resourceID string) (platformInboxItem, error) {
	query := url.Values{}
	query.Set("source_product_code", productCode)
	query.Set("source_resource_type", resourceType)
	query.Set("source_resource_id", resourceID)
	request, err := client.signedRequest(ctx, http.MethodGet, "/api/v1/operations-inbox/items/"+url.PathEscape(itemID.String())+"?"+query.Encode(), nil)
	if err != nil {
		return platformInboxItem{}, err
	}
	request.Header.Set("X-Session-Exchange-Token", token)
	response, err := client.httpClient.Do(request)
	if err != nil {
		return platformInboxItem{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
		return platformInboxItem{}, fmt.Errorf("operations Inbox status returned %d", response.StatusCode)
	}
	var envelope struct {
		Data platformInboxItem `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&envelope); err != nil || envelope.Data.ID != itemID.String() || envelope.Data.Version < 1 || envelope.Data.UpdatedAt.IsZero() || envelope.Data.Status == "" {
		return platformInboxItem{}, errors.New("invalid operations Inbox status response")
	}
	return envelope.Data, nil
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

type localPlatformSession struct {
	UserID        string    `json:"user_id"`
	ExchangeToken string    `json:"exchange_token"`
	ExpiresAt     time.Time `json:"expires_at"`
}
