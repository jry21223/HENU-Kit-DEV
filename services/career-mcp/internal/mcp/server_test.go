package mcp_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"henukit.dev/career-mcp/internal/careerclient"
	mcp "henukit.dev/career-mcp/internal/mcp"
)

const (
	testClientID = "portal-gateway-career"
	testSecret   = "career-mcp-test-secret-at-least-32-bytes"
	testActorID  = "44444444-4444-4444-8444-444444444444"
)

// fakeCareer is a minimal Career service: it verifies the six-line actor-bound
// signature, records the signed request, and answers canned responses.
type fakeCareer struct {
	requests []*http.Request
	bodies   [][]byte
	nextPost func(body []byte) (int, map[string]any)
	nextGet  func() (int, map[string]any)
}

func (f *fakeCareer) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !validSignature(r, body) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":{"code":"INVALID_SERVICE_AUTH","message":"bad signature"},"request_id":"req_fake"}`))
			return
		}
		f.requests = append(f.requests, r)
		f.bodies = append(f.bodies, body)
		if r.Method == http.MethodPost {
			status, data := f.nextPost(body)
			if status != 200 {
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"error":{"code":"` + errorCodeOf(data) + `","message":"upstream failure"},"request_id":"req_fake"}`))
				return
			}
			_, _ = w.Write([]byte(fmt.Sprintf(`{"data":%s,"request_id":"req_fake"}`, jsonOf(data))))
			return
		}
		status, data := f.nextGet()
		if status != 200 {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"error":{"code":"EXTRACTION_NOT_FOUND","message":"not found"},"request_id":"req_fake"}`))
			return
		}
		_, _ = w.Write([]byte(fmt.Sprintf(`{"data":%s,"request_id":"req_fake"}`, jsonOf(data))))
	})
}

func errorCodeOf(data map[string]any) string {
	if data == nil {
		return "UNKNOWN"
	}
	code, _ := data["error_code"].(string)
	return code
}

// validSignature reproduces Career's six-line canonical check (actor is the
// sixth line), so the test proves the signer binds the actor.
func validSignature(r *http.Request, body []byte) bool {
	if r.Header.Get("X-Service-Id") != testClientID {
		return false
	}
	timestamp := r.Header.Get("X-Timestamp")
	nonce := r.Header.Get("X-Nonce")
	actor := r.Header.Get("X-Actor-User-Id")
	digest := sha256.Sum256(body)
	canonical := r.Method + "\n" + r.URL.RequestURI() + "\n" + timestamp + "\n" + nonce + "\n" + hex.EncodeToString(digest[:]) + "\n" + actor
	mac := hmac.New(sha256.New, []byte(testSecret))
	_, _ = mac.Write([]byte(canonical))
	return hmac.Equal([]byte(r.Header.Get("X-Signature")), []byte(base64.RawURLEncoding.EncodeToString(mac.Sum(nil))))
}

func jsonOf(value any) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

type harness struct {
	mcpServer *httptest.Server
	career    *fakeCareer
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	career := &fakeCareer{
		nextPost: func(body []byte) (int, map[string]any) {
			return 200, map[string]any{
				"extraction": map[string]any{
					"id": "22222222-2222-4222-8222-222222222222", "status": "queued",
					"user_id": testActorID, "file_name": "resume.pdf", "created_at": "2026-08-16T00:00:00Z",
				},
			}
		},
		nextGet: func() (int, map[string]any) {
			return 200, map[string]any{
				"extraction": map[string]any{
					"id": "22222222-2222-4222-8222-222222222222", "status": "completed",
					"user_id": testActorID, "file_name": "resume.pdf", "created_at": "2026-08-16T00:00:00Z",
					"extracted": map[string]any{
						"target_roles": "后端开发", "tech_stack": "go,postgres", "locations": "郑州",
						"job_type": "daily_intern", "graduation_year": 2027, "resume_text": "校内项目经历",
					},
				},
			}
		},
	}
	careerServer := httptest.NewServer(career.handler())
	t.Cleanup(careerServer.Close)
	client, err := careerclient.NewClient(careerServer.URL, testClientID, testSecret, "active")
	if err != nil {
		t.Fatal(err)
	}
	handler, err := mcp.NewHandler(mcp.Options{Client: client, AccessToken: "test-token"})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler.Handler())
	t.Cleanup(server.Close)
	return &harness{mcpServer: server, career: career}
}

func (h *harness) connect(t *testing.T) *mcpsdk.ClientSession {
	t.Helper()
	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test-client"}, nil)
	session, err := client.Connect(context.Background(), &mcpsdk.StreamableClientTransport{
		Endpoint:             h.mcpServer.URL + "/mcp",
		DisableStandaloneSSE: true,
		HTTPClient:           &http.Client{Transport: bearerRoundTripper{next: h.mcpServer.Client().Transport}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

type bearerRoundTripper struct{ next http.RoundTripper }

func (b bearerRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	request.Header.Set("Authorization", "Bearer test-token")
	return b.next.RoundTrip(request)
}

func callTool(t *testing.T, session *mcpsdk.ClientSession, name string, args map[string]any) (string, error) {
	t.Helper()
	result, err := session.CallTool(context.Background(), &mcpsdk.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		return "", err
	}
	var text strings.Builder
	for _, content := range result.Content {
		if item, ok := content.(*mcpsdk.TextContent); ok {
			text.WriteString(item.Text)
		}
	}
	if result.IsError {
		return text.String(), fmt.Errorf("%s", strings.TrimSpace(text.String()))
	}
	return text.String(), nil
}

func resumeContent() string {
	return base64.StdEncoding.EncodeToString([]byte("姓名：测试同学\n目标：后端开发"))
}

func TestUploadResumeSignsActorAndReturnsQueuedExtraction(t *testing.T) {
	h := newHarness(t)
	session := h.connect(t)

	text, err := callTool(t, session, "upload_resume", map[string]any{
		"file_name": "resume.pdf", "content": resumeContent(), "actor_user_id": testActorID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "提取任务已创建") || !strings.Contains(text, "queued") {
		t.Fatalf("upload result = %q", text)
	}
	if len(h.career.requests) != 1 {
		t.Fatalf("career requests = %d", len(h.career.requests))
	}
	request := h.career.requests[0]
	if request.URL.Path != "/api/v1/career/profile/extractions" || request.Method != http.MethodPost {
		t.Fatalf("upload request = %s %s", request.Method, request.URL.Path)
	}
	if request.Header.Get("X-Actor-User-Id") != testActorID {
		t.Fatalf("actor header = %q", request.Header.Get("X-Actor-User-Id"))
	}
	if !strings.HasPrefix(request.Header.Get("Content-Type"), "multipart/form-data") {
		t.Fatalf("content type = %q", request.Header.Get("Content-Type"))
	}
	if !strings.Contains(string(h.career.bodies[0]), "姓名：测试同学") {
		t.Fatal("multipart body lost the resume bytes")
	}
}

func TestUploadResumeRejectsInvalidInputBeforeUpstream(t *testing.T) {
	h := newHarness(t)
	session := h.connect(t)

	cases := []struct {
		name string
		args map[string]any
	}{
		{"missing actor uuid", map[string]any{"file_name": "r.txt", "content": resumeContent(), "actor_user_id": "nope"}},
		{"dangerous file name", map[string]any{"file_name": "../r.txt", "content": resumeContent(), "actor_user_id": testActorID}},
		{"empty content", map[string]any{"file_name": "r.txt", "content": "", "actor_user_id": testActorID}},
		{"invalid base64", map[string]any{"file_name": "r.txt", "content": "!!!not-base64!!!", "actor_user_id": testActorID}},
		{"oversize content", map[string]any{"file_name": "r.txt", "content": base64.StdEncoding.EncodeToString(make([]byte, 10<<20+1)), "actor_user_id": testActorID}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := callTool(t, session, "upload_resume", tc.args); err == nil {
				t.Fatal("invalid input accepted")
			}
			if len(h.career.requests) != 0 {
				t.Fatalf("invalid input reached upstream (%d calls)", len(h.career.requests))
			}
		})
	}
}

func TestUploadResumeSurfacesUpstreamErrorCodes(t *testing.T) {
	h := newHarness(t)
	h.career.nextPost = func(body []byte) (int, map[string]any) {
		return 503, map[string]any{"error_code": "AI_UNCONFIGURED"}
	}
	session := h.connect(t)

	_, err := callTool(t, session, "upload_resume", map[string]any{
		"file_name": "resume.pdf", "content": resumeContent(), "actor_user_id": testActorID,
	})
	if err == nil || !strings.Contains(err.Error(), "AI_UNCONFIGURED") {
		t.Fatalf("unconfigured error = %v", err)
	}

	h.career.nextPost = func(body []byte) (int, map[string]any) {
		return 429, map[string]any{"error_code": "EXTRACT_RATE_LIMITED"}
	}
	if _, err := callTool(t, session, "upload_resume", map[string]any{
		"file_name": "resume.pdf", "content": resumeContent(), "actor_user_id": testActorID,
	}); err == nil || !strings.Contains(err.Error(), "EXTRACT_RATE_LIMITED") {
		t.Fatalf("rate limit error = %v", err)
	}
}

func TestGetResumeExtractionReturnsCompletedDraft(t *testing.T) {
	h := newHarness(t)
	session := h.connect(t)

	text, err := callTool(t, session, "get_resume_extraction", map[string]any{
		"extraction_id": "22222222-2222-4222-8222-222222222222", "actor_user_id": testActorID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "completed") || !strings.Contains(text, "后端开发") {
		t.Fatalf("status result = %q", text)
	}
	if len(h.career.requests) != 1 {
		t.Fatalf("career requests = %d", len(h.career.requests))
	}
	request := h.career.requests[0]
	if request.URL.Path != "/api/v1/career/profile/extractions/22222222-2222-4222-8222-222222222222" {
		t.Fatalf("status path = %q", request.URL.Path)
	}
	if request.Header.Get("X-Actor-User-Id") != testActorID {
		t.Fatalf("actor header = %q", request.Header.Get("X-Actor-User-Id"))
	}

	if _, err := callTool(t, session, "get_resume_extraction", map[string]any{
		"extraction_id": "not-a-uuid", "actor_user_id": testActorID,
	}); err == nil {
		t.Fatal("invalid extraction id accepted")
	}
}

func TestAuthRejectsBadTokenAndUnconfiguredServer(t *testing.T) {
	h := newHarness(t)
	response, err := http.Get(h.mcpServer.URL + "/mcp")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d", response.StatusCode)
	}

	// An unconfigured server (no access token) fails closed with 503 before
	// any MCP handshake.
	placeholder := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer placeholder.Close()
	client, err := careerclient.NewClient(placeholder.URL, testClientID, testSecret, "active")
	if err != nil {
		t.Fatal(err)
	}
	handler, err := mcp.NewHandler(mcp.Options{Client: client, AccessToken: ""})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler.Handler())
	defer server.Close()
	unconfigured, err := http.Post(server.URL+"/mcp", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	defer unconfigured.Body.Close()
	if unconfigured.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("unconfigured server status = %d", unconfigured.StatusCode)
	}
}
