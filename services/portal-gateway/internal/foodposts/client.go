// Package foodposts is Portal Gateway's private client for the Food Post
// owner. Public reads and the signed-in actor's create command go through two
// independent service credentials so Food can tell the roles apart; browser
// callers never choose an actor identity or receive service secrets.
package foodposts

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"henukit.dev/portal-gateway/internal/serviceauth"
)

const (
	PostsPath   = "/api/v1/food/posts"
	MyPostsPath = "/api/v1/food/posts/mine"
	VenuesPath  = "/api/v1/food/venues"

	// maxBodyBytes caps a create command body and a Food response body. Six
	// 2MiB photos plus base64 overhead fit inside 20MiB; nothing larger is
	// legitimate on either side of the boundary.
	maxBodyBytes = 20 << 20
)

var (
	ErrUnconfigured = errors.New("Food Post client is not configured")
	ErrUnavailable  = errors.New("Food service is unavailable")
	ErrInvalid      = errors.New("Food service returned an invalid response")
	ErrBadRequest   = errors.New("Food Post request was rejected")

	idempotencyKeyPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{8,200}$`)
)

// ValidIdempotencyKey reports whether a browser Idempotency-Key matches the
// Food create contract: 8..200 characters of letters, digits, and . _ : -.
func ValidIdempotencyKey(value string) bool {
	return idempotencyKeyPattern.MatchString(value)
}

// UpstreamError carries a non-2xx Food response. The Gateway forwards the
// exact status and body verbatim so the frontend keeps branching on Food's
// error.code, for example DAILY_POST_CAP_REACHED on a 429.
type UpstreamError struct {
	StatusCode  int
	ContentType string
	Body        []byte
}

func (e *UpstreamError) Error() string {
	return fmt.Sprintf("Food service responded %d", e.StatusCode)
}

// Image is one stored Food Post photo plus the cache headers the browser
// needs to render and revalidate it.
type Image struct {
	Bytes        []byte
	ContentType  string
	ETag         string
	CacheControl string
}

// Client is a typed internal boundary for Food Post reads and the create
// command. Create and read requests are signed with independent credentials.
type Client struct {
	baseURL      string
	httpClient   *http.Client
	createSigner *serviceauth.Signer
	readSigner   *serviceauth.Signer
}

// PostPath builds the signed request path for one post detail.
func PostPath(postID string) string {
	return PostsPath + "/" + url.PathEscape(postID)
}

// PostImagePath builds the signed request path for one stored photo.
func PostImagePath(postID, position string) string {
	return PostPath(postID) + "/images/" + url.PathEscape(position)
}

// NewClient creates a private Food Post client. It accepts HTTP only for
// local compose and test origins; public deployments use an isolated Docker
// network. A credential triple counts as configured only when all three parts
// are present; an operation that needs a missing credential fails closed.
func NewClient(baseURL, createClientID, createClientSecret, createKeyID, readClientID, readClientSecret, readKeyID string) (*Client, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, errors.New("invalid Food Post client configuration")
	}
	client := &Client{
		baseURL:    strings.TrimRight(parsed.String(), "/"),
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
	if strings.TrimSpace(createClientID) != "" && createClientSecret != "" && strings.TrimSpace(createKeyID) != "" {
		client.createSigner = serviceauth.NewSigner(createClientID, createClientSecret, createKeyID)
	}
	if strings.TrimSpace(readClientID) != "" && readClientSecret != "" && strings.TrimSpace(readKeyID) != "" {
		client.readSigner = serviceauth.NewSigner(readClientID, readClientSecret, readKeyID)
	}
	return client, nil
}

// CreatePost forwards a signed-in actor's create command. The actor and the
// display-name snapshot come exclusively from the verified Portal Session,
// and the body is re-signed byte-for-byte with the create credential.
func (c *Client) CreatePost(ctx context.Context, actorUserID, actorDisplayName, requestID, idempotencyKey string, raw []byte) (json.RawMessage, error) {
	if strings.TrimSpace(actorUserID) == "" || strings.TrimSpace(actorDisplayName) == "" || !ValidIdempotencyKey(idempotencyKey) || len(raw) == 0 || len(raw) > maxBodyBytes {
		return nil, ErrBadRequest
	}
	body, _, _, err := c.call(ctx, http.MethodPost, PostsPath, c.createSigner, requestID, raw, func(request *http.Request) {
		request.Header.Set("X-Actor-User-Id", actorUserID)
		request.Header.Set("X-Actor-Display-Name", actorDisplayName)
		request.Header.Set("Idempotency-Key", idempotencyKey)
	})
	if err != nil {
		return nil, err
	}
	return unwrapEnvelope(body)
}

// ListPosts forwards the public post list. The browser's campus query rides
// along unchanged so Food's signature covers the exact request URI.
func (c *Client) ListPosts(ctx context.Context, requestID, rawQuery string) (json.RawMessage, error) {
	return c.readJSON(ctx, withQuery(PostsPath, rawQuery), requestID)
}

// MyPosts forwards the signed-in actor's own posts, binding only the actor
// user ID header.
func (c *Client) MyPosts(ctx context.Context, actorUserID, requestID string) (json.RawMessage, error) {
	if strings.TrimSpace(actorUserID) == "" {
		return nil, ErrBadRequest
	}
	body, _, _, err := c.call(ctx, http.MethodGet, MyPostsPath, c.readSigner, requestID, nil, func(request *http.Request) {
		request.Header.Set("X-Actor-User-Id", actorUserID)
	})
	if err != nil {
		return nil, err
	}
	return unwrapEnvelope(body)
}

// Post forwards one public post detail. Food's 404 passes through unchanged.
func (c *Client) Post(ctx context.Context, requestID, postID string) (json.RawMessage, error) {
	if strings.TrimSpace(postID) == "" {
		return nil, ErrBadRequest
	}
	return c.readJSON(ctx, PostPath(postID), requestID)
}

// PostImage forwards one stored photo and its cache headers.
func (c *Client) PostImage(ctx context.Context, requestID, postID, position string) (*Image, error) {
	if strings.TrimSpace(postID) == "" || strings.TrimSpace(position) == "" {
		return nil, ErrBadRequest
	}
	body, _, header, err := c.call(ctx, http.MethodGet, PostImagePath(postID, position), c.readSigner, requestID, nil, nil)
	if err != nil {
		return nil, err
	}
	return &Image{
		Bytes:        body,
		ContentType:  header.Get("Content-Type"),
		ETag:         header.Get("ETag"),
		CacheControl: header.Get("Cache-Control"),
	}, nil
}

// Venues forwards the campus-scoped venue summary, passing the browser's
// campus query through unchanged.
func (c *Client) Venues(ctx context.Context, requestID, rawQuery string) (json.RawMessage, error) {
	return c.readJSON(ctx, withQuery(VenuesPath, rawQuery), requestID)
}

func (c *Client) readJSON(ctx context.Context, requestPath, requestID string) (json.RawMessage, error) {
	body, _, _, err := c.call(ctx, http.MethodGet, requestPath, c.readSigner, requestID, nil, nil)
	if err != nil {
		return nil, err
	}
	return unwrapEnvelope(body)
}

func withQuery(path, rawQuery string) string {
	if rawQuery == "" {
		return path
	}
	return path + "?" + rawQuery
}

func (c *Client) call(ctx context.Context, method, requestPath string, signer *serviceauth.Signer, requestID string, raw []byte, setHeaders func(*http.Request)) ([]byte, int, http.Header, error) {
	if c == nil || c.httpClient == nil {
		return nil, 0, nil, ErrUnconfigured
	}
	if signer == nil {
		return nil, 0, nil, ErrUnconfigured
	}
	if strings.TrimSpace(requestID) == "" {
		return nil, 0, nil, ErrUnavailable
	}
	var body io.Reader
	if raw != nil {
		body = bytes.NewReader(raw)
	}
	request, err := http.NewRequestWithContext(ctx, method, c.baseURL+requestPath, body)
	if err != nil {
		return nil, 0, nil, fmt.Errorf("create Food Post request: %w", ErrUnavailable)
	}
	request.Header.Set("X-Request-Id", requestID)
	if raw != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if setHeaders != nil {
		setHeaders(request)
	}
	if err := signer.Sign(request); err != nil {
		return nil, 0, nil, fmt.Errorf("sign Food Post request: %w", ErrUnavailable)
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, 0, nil, fmt.Errorf("call Food service: %w", ErrUnavailable)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, maxBodyBytes+1))
	if err != nil {
		return nil, 0, nil, fmt.Errorf("read Food service response: %w", ErrUnavailable)
	}
	if len(responseBody) > maxBodyBytes {
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

// unwrapEnvelope turns Food's {data, request_id} envelope into the browser
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
