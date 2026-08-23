// Package career is Portal Gateway's private client for the Career
// Opportunities owner. Every Career route is actor-scoped (Work Radar searches
// belong to one user), so a single service credential carries the verified
// Portal Session actor; browser callers never choose an actor identity or
// receive service secrets. Create, history, and status are exact routes that
// shadow the legacy Portal API wildcard and never fall back to it.
package career

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"henukit.dev/portal-gateway/internal/serviceauth"
)

const (
	SearchesPath = "/api/v1/career/searches"
	ProfilePath  = "/api/v1/career/profile"
	// ExtractionsPath is the resume upload boundary. The browser body may be a
	// 10 MiB file plus multipart framing, so this route alone gets a larger
	// cap; every other Career route keeps the small JSON cap.
	ExtractionsPath      = "/api/v1/career/profile/extractions"
	SuificationsPath     = "/api/v1/career/profile/suifications"
	maxRequestBodyBytes  = 128 << 10 // career profile snapshots stay small
	maxResponseBodyBytes = 1 << 20   // up to 150 bounded display-only job rows
	maxExtractBodyBytes  = (10 << 20) + (1 << 20)
)

var (
	ErrUnconfigured = errors.New("Career client is not configured")
	ErrUnavailable  = errors.New("Career service is unavailable")
	ErrInvalid      = errors.New("Career service returned an invalid response")
	ErrBadRequest   = errors.New("Career request was rejected")

	idempotencyKeyPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{8,200}$`)
)

// ValidIdempotencyKey reports whether a browser Idempotency-Key matches the
// Career create contract.
func ValidIdempotencyKey(value string) bool {
	return idempotencyKeyPattern.MatchString(value)
}

// UpstreamError carries a non-2xx Career response. The Gateway forwards the
// exact status and body verbatim so the frontend branches on Career's
// error.code.
type UpstreamError struct {
	StatusCode  int
	ContentType string
	Body        []byte
}

func (e *UpstreamError) Error() string {
	return fmt.Sprintf("Career service responded %d", e.StatusCode)
}

// Client is a typed internal boundary for Career reads and the create command.
type Client struct {
	baseURL    string
	httpClient *http.Client
	signer     *serviceauth.Signer
}

// SearchPath builds the signed request path for one search status.
func SearchPath(searchID string) string {
	return SearchesPath + "/" + url.PathEscape(searchID)
}

// NewClient creates a private Career client. It accepts HTTP only for local
// compose and test origins; public deployments use an isolated Docker network.
func NewClient(baseURL, clientID, clientSecret, keyID string) (*Client, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, errors.New("invalid Career client configuration")
	}
	return &Client{
		baseURL:    strings.TrimRight(parsed.String(), "/"),
		httpClient: &http.Client{Timeout: 15 * time.Second},
		signer:     serviceauth.NewSigner(clientID, clientSecret, keyID),
	}, nil
}

// CreateSearch forwards a signed-in actor's create command. The actor comes
// exclusively from the verified Portal Session and the body is re-signed
// byte-for-byte with the service credential.
func (c *Client) CreateSearch(ctx context.Context, actorUserID, requestID, idempotencyKey string, raw []byte) (json.RawMessage, error) {
	if strings.TrimSpace(actorUserID) == "" || !ValidIdempotencyKey(idempotencyKey) || len(raw) == 0 || len(raw) > maxRequestBodyBytes {
		return nil, ErrBadRequest
	}
	body, _, _, err := c.call(ctx, http.MethodPost, SearchesPath, actorUserID, requestID, raw, "application/json", func(request *http.Request) {
		request.Header.Set("X-Actor-User-Id", actorUserID)
		request.Header.Set("Idempotency-Key", idempotencyKey)
	})
	if err != nil {
		return nil, err
	}
	return unwrapEnvelope(body)
}

// ListSearches forwards the signed-in actor's search history.
func (c *Client) ListSearches(ctx context.Context, actorUserID, requestID string) (json.RawMessage, error) {
	return c.read(ctx, SearchesPath, actorUserID, requestID)
}

// Search forwards one search's status, binding only the actor user ID.
func (c *Client) Search(ctx context.Context, actorUserID, requestID, searchID string) (json.RawMessage, error) {
	if strings.TrimSpace(searchID) == "" {
		return nil, ErrBadRequest
	}
	return c.read(ctx, SearchPath(searchID), actorUserID, requestID)
}

// ExtractionPath builds the signed request path for one extraction status.
func ExtractionPath(extractionID string) string {
	return ExtractionsPath + "/" + url.PathEscape(extractionID)
}

// CreateExtraction forwards and re-signs the browser's multipart body
// byte-for-byte. Preserving its boundary and part headers keeps the original
// file name, media type, and file bytes intact without touching Gateway disk.
func (c *Client) CreateExtraction(ctx context.Context, actorUserID, requestID string, raw []byte, contentType string) (json.RawMessage, error) {
	mediaType, params, err := mime.ParseMediaType(contentType)
	if strings.TrimSpace(actorUserID) == "" || len(raw) == 0 || len(raw) > maxExtractBodyBytes ||
		err != nil || mediaType != "multipart/form-data" || strings.TrimSpace(params["boundary"]) == "" {
		return nil, ErrBadRequest
	}
	rawResponse, _, _, err := c.call(ctx, http.MethodPost, ExtractionsPath, actorUserID, requestID, raw, contentType, nil)
	if err != nil {
		return nil, err
	}
	return unwrapEnvelope(rawResponse)
}

// Extraction forwards one extraction's status, binding only the actor user ID.
func (c *Client) Extraction(ctx context.Context, actorUserID, requestID, extractionID string) (json.RawMessage, error) {
	if strings.TrimSpace(extractionID) == "" {
		return nil, ErrBadRequest
	}
	return c.read(ctx, ExtractionPath(extractionID), actorUserID, requestID)
}

// Profile forwards the signed-in actor's Career profile read.
func (c *Client) Profile(ctx context.Context, actorUserID, requestID string) (json.RawMessage, error) {
	return c.read(ctx, ProfilePath, actorUserID, requestID)
}

// UpdateProfile forwards the signed-in actor's profile replacement.
func (c *Client) UpdateProfile(ctx context.Context, actorUserID, requestID string, raw []byte) (json.RawMessage, error) {
	if strings.TrimSpace(actorUserID) == "" || len(raw) == 0 || len(raw) > maxRequestBodyBytes {
		return nil, ErrBadRequest
	}
	body, _, _, err := c.call(ctx, http.MethodPut, ProfilePath, actorUserID, requestID, raw, "application/json", nil)
	if err != nil {
		return nil, err
	}
	return unwrapEnvelope(body)
}

// CreateSuification forwards one transient Resume Text rewrite command. The
// Career owner returns a draft only; neither the Gateway nor Career persists it.
func (c *Client) CreateSuification(ctx context.Context, actorUserID, requestID, idempotencyKey string, raw []byte) (json.RawMessage, error) {
	if c == nil || c.httpClient == nil {
		return nil, ErrUnconfigured
	}
	if strings.TrimSpace(actorUserID) == "" || !ValidIdempotencyKey(idempotencyKey) || len(raw) == 0 || len(raw) > maxRequestBodyBytes {
		return nil, ErrBadRequest
	}
	longClient := *c.httpClient
	longClient.Timeout = 65 * time.Second
	body, _, _, err := c.callWithClient(&longClient, ctx, http.MethodPost, SuificationsPath, actorUserID, requestID, raw, "application/json", func(request *http.Request) {
		request.Header.Set("Idempotency-Key", idempotencyKey)
	})
	if err != nil {
		return nil, err
	}
	return unwrapEnvelope(body)
}

func (c *Client) read(ctx context.Context, requestPath, actorUserID, requestID string) (json.RawMessage, error) {
	body, _, _, err := c.call(ctx, http.MethodGet, requestPath, actorUserID, requestID, nil, "", nil)
	if err != nil {
		return nil, err
	}
	return unwrapEnvelope(body)
}

func (c *Client) call(ctx context.Context, method, requestPath, actorUserID, requestID string, raw []byte, contentType string, setHeaders func(*http.Request)) ([]byte, int, http.Header, error) {
	if c == nil || c.httpClient == nil {
		return nil, 0, nil, ErrUnconfigured
	}
	return c.callWithClient(c.httpClient, ctx, method, requestPath, actorUserID, requestID, raw, contentType, setHeaders)
}

func (c *Client) callWithClient(httpClient *http.Client, ctx context.Context, method, requestPath, actorUserID, requestID string, raw []byte, contentType string, setHeaders func(*http.Request)) ([]byte, int, http.Header, error) {
	if strings.TrimSpace(requestID) == "" {
		return nil, 0, nil, ErrUnavailable
	}
	var body io.Reader
	if raw != nil {
		body = bytes.NewReader(raw)
	}
	request, err := http.NewRequestWithContext(ctx, method, c.baseURL+requestPath, body)
	if err != nil {
		return nil, 0, nil, fmt.Errorf("create Career request: %w", ErrUnavailable)
	}
	request.Header.Set("X-Request-Id", requestID)
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	if setHeaders != nil {
		setHeaders(request)
	}
	if err := c.signer.SignWithActor(request, actorUserID); err != nil {
		return nil, 0, nil, fmt.Errorf("sign Career request: %w", ErrUnavailable)
	}

	response, err := httpClient.Do(request)
	if err != nil {
		return nil, 0, nil, fmt.Errorf("call Career service: %w", ErrUnavailable)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBodyBytes+1))
	if err != nil {
		return nil, 0, nil, fmt.Errorf("read Career service response: %w", ErrUnavailable)
	}
	if len(responseBody) > maxResponseBodyBytes {
		return nil, 0, nil, ErrInvalid
	}
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return nil, 0, nil, &UpstreamError{
			StatusCode:  response.StatusCode,
			ContentType: response.Header.Get("Content-Type"),
			Body:        responseBody,
		}
	}
	return responseBody, response.StatusCode, response.Header, nil
}

// unwrapEnvelope turns Career's {data, request_id} envelope into the browser
// shape {...data, request_id}, mirroring the account-portfolio read unwrap.
func unwrapEnvelope(body []byte) (json.RawMessage, error) {
	var envelope struct {
		Data      json.RawMessage `json:"data"`
		RequestID string          `json:"request_id"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil || len(envelope.Data) == 0 || string(envelope.Data) == "null" || strings.TrimSpace(envelope.RequestID) == "" {
		return nil, ErrInvalid
	}
	var data map[string]json.RawMessage
	if err := json.Unmarshal(envelope.Data, &data); err != nil {
		return nil, ErrInvalid
	}
	data["request_id"] = json.RawMessage(strconv.Quote(envelope.RequestID))
	out, err := json.Marshal(data)
	if err != nil {
		return nil, ErrInvalid
	}
	return out, nil
}
