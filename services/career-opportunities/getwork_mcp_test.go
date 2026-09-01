package career

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"henukit.dev/career/internal/mcprelay"
)

type upstreamCrawlInput struct {
	Source    string `json:"source"`
	SinceDays int    `json:"since_days"`
}

type getWorkRoundTripFunc func(*http.Request) (*http.Response, error)

func (function getWorkRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func TestGetWorkMCPProducesMatchedCareerJobs(t *testing.T) {
	server := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "getwork-test", Version: "0.1.0"}, nil)
	mcpsdk.AddTool(server, &mcpsdk.Tool{Name: "list_sources"}, func(context.Context, *mcpsdk.CallToolRequest, struct{}) (*mcpsdk.CallToolResult, any, error) {
		return nil, map[string]any{
			"status":  "ok",
			"sources": []map[string]any{{"key": "meituan", "name": "美团校招", "strategy": "platform"}},
		}, nil
	})
	mcpsdk.AddTool(server, &mcpsdk.Tool{Name: "crawl_jobs"}, func(_ context.Context, _ *mcpsdk.CallToolRequest, input upstreamCrawlInput) (*mcpsdk.CallToolResult, any, error) {
		if input.Source != "meituan" || input.SinceDays != 7 {
			return nil, nil, fmt.Errorf("unexpected crawl input: %+v", input)
		}
		return nil, map[string]any{
			"status":     "ok",
			"source":     "meituan",
			"fetched_at": "2026-08-25T00:00:00Z",
			"count":      1,
			"jobs": []map[string]any{{
				"title": "Go 后端开发实习生", "company": "美团校招", "source": "spoofed-upstream-key",
				"location": "北京", "job_type": "实习", "description": "负责后端服务开发",
				"requirement": "熟悉 Go 和 PostgreSQL", "apply_url": "https://zhaopin.meituan.com/job/1",
				"publish_date": "2026-08-24",
			}},
		}, nil
	})
	handler := mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server { return server }, &mcpsdk.StreamableHTTPOptions{
		Stateless: true, JSONResponse: true,
	})
	var crawlProtocolChecks atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer getwork-test-token-0000000000000000" {
			http.Error(writer, "unauthorized", http.StatusUnauthorized)
			return
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read MCP request: %v", err)
			return
		}
		request.Body = io.NopCloser(bytes.NewReader(body))
		var envelope struct {
			Method string `json:"method"`
			Params struct {
				Name string         `json:"name"`
				Meta map[string]any `json:"_meta"`
			} `json:"params"`
		}
		if json.Unmarshal(body, &envelope) == nil && envelope.Method == "tools/call" && envelope.Params.Name == "crawl_jobs" {
			headerVersion := request.Header.Get("MCP-Protocol-Version")
			metaVersion, _ := envelope.Params.Meta[mcpsdk.MetaKeyProtocolVersion].(string)
			if headerVersion == "" || (headerVersion >= "2026-07-28" && headerVersion != metaVersion) {
				t.Errorf("crawl protocol version header = %q, meta = %q", headerVersion, metaVersion)
			}
			if headerVersion >= "2026-07-28" {
				if request.Header.Get("MCP-Method") != "tools/call" || request.Header.Get("MCP-Name") != "crawl_jobs" {
					t.Errorf("crawl standard headers: method = %q, name = %q", request.Header.Get("MCP-Method"), request.Header.Get("MCP-Name"))
				}
				if _, ok := envelope.Params.Meta[mcpsdk.MetaKeyClientInfo].(map[string]any); !ok {
					t.Errorf("crawl client info meta = %#v", envelope.Params.Meta[mcpsdk.MetaKeyClientInfo])
				}
				if _, ok := envelope.Params.Meta[mcpsdk.MetaKeyClientCapabilities].(map[string]any); !ok {
					t.Errorf("crawl client capabilities meta = %#v", envelope.Params.Meta[mcpsdk.MetaKeyClientCapabilities])
				}
			}
			crawlProtocolChecks.Add(1)
		}
		handler.ServeHTTP(writer, request)
	}))
	t.Cleanup(upstream.Close)
	relayHandler, err := mcprelay.NewWithTransport(
		"http://127.0.0.1:18100",
		getWorkRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			cloned := request.Clone(request.Context())
			cloned.URL.Scheme = "http"
			cloned.URL.Host = strings.TrimPrefix(upstream.URL, "http://")
			return http.DefaultTransport.RoundTrip(cloned)
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	relay := httptest.NewServer(relayHandler)
	t.Cleanup(relay.Close)

	work, err := NewGetWorkMCPWork(context.Background(), GetWorkMCPConfig{
		Endpoint:    relay.URL + "/mcp",
		AccessToken: "getwork-test-token-0000000000000000",
		SinceDays:   7,
		HTTPClient:  relay.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := work(context.Background(), map[string]any{
		"target_roles": "后端开发", "tech_stack": "Go、PostgreSQL", "locations": "北京", "job_type": "daily_intern",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.SourceCount != 1 || result.JobCount != 1 || result.MatchedCount != 1 {
		t.Fatalf("result counts = %+v", result)
	}
	jobs := result.Payload.(map[string]any)["jobs"].([]Job)
	if len(jobs) != 1 {
		t.Fatalf("jobs = %+v", jobs)
	}
	job := jobs[0]
	if job.SourceKey != "getwork.meituan" || job.Company != "美团校招" || job.URL != "https://zhaopin.meituan.com/job/1" {
		t.Fatalf("job mapping = %+v", job)
	}
	if job.MatchScore < 50 || len(job.MatchReasons) == 0 {
		t.Fatalf("job matching = %+v", job)
	}
	mismatched, err := work(context.Background(), map[string]any{
		"target_roles": "后端开发", "tech_stack": "Go、PostgreSQL", "locations": "北京", "job_type": "campus_recruit",
	})
	if err != nil {
		t.Fatal(err)
	}
	if mismatched.JobCount != 0 || mismatched.MatchedCount != 0 {
		t.Fatalf("job type mismatch result = %+v", mismatched)
	}
	if crawlProtocolChecks.Load() != 2 {
		t.Fatalf("crawl protocol checks = %d, want 2", crawlProtocolChecks.Load())
	}
}

func TestDecodeGetWorkToolResultAcceptsUpstreamTextJSON(t *testing.T) {
	result := &mcpsdk.CallToolResult{Content: []mcpsdk.Content{
		&mcpsdk.TextContent{Text: `{"status":"ok","sources":[{"key":"meituan"}]}`},
	}}
	var decoded getWorkMCPSourceList
	if err := decodeGetWorkToolResult("list_sources", result, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Status != "ok" || len(decoded.Sources) != 1 || decoded.Sources[0].Key != "meituan" {
		t.Fatalf("decoded = %+v", decoded)
	}
}

func TestGetWorkMCPHeaderAnnotationBoundary(t *testing.T) {
	plain := map[string]any{"type": "object", "properties": map[string]any{"source": map[string]any{"type": "string"}}}
	annotated := map[string]any{"type": "object", "properties": map[string]any{"source": map[string]any{"type": "string", "x-mcp-header": "Source"}}}
	if getWorkMCPHasHeaderAnnotation(plain) {
		t.Fatal("plain crawl schema was treated as header-annotated")
	}
	if !getWorkMCPHasHeaderAnnotation(annotated) {
		t.Fatal("x-mcp-header crawl schema was accepted")
	}
}

func TestGetWorkMCPResponsePayloadsAcceptsBoundedEventStream(t *testing.T) {
	body := []byte(": keepalive\r\n\r\nevent: message\r\ndata: {\"jsonrpc\":\"2.0\",\"id\":\"crawl-alibaba\",\"result\":{}}\r\n\r\n")
	payloads, err := getWorkMCPResponsePayloads(body, "text/event-stream; charset=utf-8")
	if err != nil {
		t.Fatal(err)
	}
	if len(payloads) != 1 || string(payloads[0]) != `{"jsonrpc":"2.0","id":"crawl-alibaba","result":{}}` {
		t.Fatalf("payloads = %q", payloads)
	}
}

func TestCallGetWorkToolHTTPRejectsTruncatedEventStream(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/event-stream")
		_, _ = writer.Write([]byte("event: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":\"crawl-meituan\",\"result\":{}}\n"))
	}))
	t.Cleanup(upstream.Close)

	var target getWorkMCPCrawl
	err := callGetWorkToolHTTP(context.Background(), upstream.Client(), upstream.URL, "2025-11-25", "crawl-meituan", "crawl_jobs", map[string]any{}, &target)
	if err == nil || !strings.Contains(err.Error(), "truncated") {
		t.Fatalf("truncated event stream error = %v", err)
	}
}

func TestGetWorkMCPResponsePayloadsRejectsMislabeledJSON(t *testing.T) {
	body := []byte(`{"jsonrpc":"2.0","id":"crawl-alibaba","result":{}}`)
	for _, contentType := range []string{"", "text/plain", "text/html", "application/problem+json", "text/event-stream"} {
		if _, err := getWorkMCPResponsePayloads(body, contentType); err == nil {
			t.Fatalf("content type %q accepted an unframed JSON response", contentType)
		}
	}
}

func TestCallGetWorkToolHTTPRejectsResponseOverBoundedLimit(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write([]byte(strings.Repeat("x", maxGetWorkMCPResponseBytes+1)))
	}))
	t.Cleanup(upstream.Close)

	var target getWorkMCPCrawl
	err := callGetWorkToolHTTP(context.Background(), upstream.Client(), upstream.URL, "2025-11-25", "crawl-meituan", "crawl_jobs", map[string]any{}, &target)
	if err == nil || !strings.Contains(err.Error(), "bounded limit") {
		t.Fatalf("oversized response error = %v", err)
	}
}

func TestCallGetWorkToolHTTPRejectsMismatchedResponseID(t *testing.T) {
	for _, testCase := range []struct {
		name        string
		contentType string
		body        string
	}{
		{name: "json", contentType: "application/json", body: `{"jsonrpc":"2.0","id":"crawl-other","result":{}}`},
		{name: "event stream", contentType: "text/event-stream", body: "event: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":\"crawl-other\",\"result\":{}}\n\n"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Type", testCase.contentType)
				_, _ = writer.Write([]byte(testCase.body))
			}))
			t.Cleanup(upstream.Close)

			var target getWorkMCPCrawl
			err := callGetWorkToolHTTP(context.Background(), upstream.Client(), upstream.URL, "2025-11-25", "crawl-meituan", "crawl_jobs", map[string]any{}, &target)
			if err == nil || !strings.Contains(err.Error(), "identity is invalid") {
				t.Fatalf("mismatched response ID error = %v", err)
			}
		})
	}
}

func TestCallGetWorkToolHTTPRejectsInvalidEventBeforeMatchingResponse(t *testing.T) {
	valid := `{"jsonrpc":"2.0","id":"crawl-meituan","result":{"structuredContent":{"status":"ok","jobs":[]}}}`
	for _, testCase := range []struct {
		name      string
		first     string
		wantError string
	}{
		{name: "invalid JSON", first: `not-json`, wantError: "invalid JSON"},
		{name: "wrong JSON-RPC version", first: `{"jsonrpc":"1.0","id":"crawl-meituan","result":{}}`, wantError: "JSON-RPC version"},
		{name: "wrong response ID", first: `{"jsonrpc":"2.0","id":"crawl-other","result":{}}`, wantError: "identity is invalid"},
		{name: "server request", first: `{"jsonrpc":"2.0","id":"server-call","method":"sampling/createMessage","params":{}}`, wantError: "server message is invalid"},
		{name: "invalid notification params", first: `{"jsonrpc":"2.0","method":"notifications/progress","params":"bad"}`, wantError: "server message is invalid"},
		{name: "multiple matching responses", first: valid, wantError: "multiple responses"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			body := "data: " + testCase.first + "\n\ndata: " + valid + "\n\n"
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Type", "text/event-stream")
				_, _ = writer.Write([]byte(body))
			}))
			t.Cleanup(upstream.Close)

			var target getWorkMCPCrawl
			err := callGetWorkToolHTTP(context.Background(), upstream.Client(), upstream.URL, "2025-11-25", "crawl-meituan", "crawl_jobs", map[string]any{}, &target)
			if err == nil || !strings.Contains(err.Error(), testCase.wantError) {
				t.Fatalf("mixed event stream error = %v, want %q", err, testCase.wantError)
			}
		})
	}
}

func TestCallGetWorkToolHTTPRequiresNegotiatedProtocolVersion(t *testing.T) {
	var target getWorkMCPCrawl
	err := callGetWorkToolHTTP(context.Background(), http.DefaultClient, "http://127.0.0.1/mcp", "", "crawl-meituan", "crawl_jobs", map[string]any{}, &target)
	if err == nil || !strings.Contains(err.Error(), "negotiated no protocol version") {
		t.Fatalf("missing protocol version error = %v", err)
	}
}

func TestCallGetWorkToolHTTPAllowsNotificationBeforeMatchingResponse(t *testing.T) {
	body := "data: {\"jsonrpc\":\"2.0\",\"method\":\"notifications/progress\",\"params\":{}}\n\n" +
		"data: {\"jsonrpc\":\"2.0\",\"id\":\"crawl-meituan\",\"result\":{\"structuredContent\":{\"status\":\"ok\",\"jobs\":[]}}}\n\n"
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/event-stream")
		_, _ = writer.Write([]byte(body))
	}))
	t.Cleanup(upstream.Close)

	var target getWorkMCPCrawl
	if err := callGetWorkToolHTTP(context.Background(), upstream.Client(), upstream.URL, "2025-11-25", "crawl-meituan", "crawl_jobs", map[string]any{}, &target); err != nil {
		t.Fatal(err)
	}
}

func TestGetWorkMCPRefusesCrawlRedirectWithoutLeakingBearer(t *testing.T) {
	attackerAuthorization := make(chan string, 1)
	attacker := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		attackerAuthorization <- request.Header.Get("Authorization")
		http.Error(writer, "unexpected redirect", http.StatusBadRequest)
	}))
	t.Cleanup(attacker.Close)

	server := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "getwork-test", Version: "0.1.0"}, nil)
	mcpsdk.AddTool(server, &mcpsdk.Tool{Name: "list_sources"}, func(context.Context, *mcpsdk.CallToolRequest, struct{}) (*mcpsdk.CallToolResult, any, error) {
		return nil, map[string]any{"status": "ok", "sources": []map[string]any{{"key": "meituan"}}}, nil
	})
	mcpsdk.AddTool(server, &mcpsdk.Tool{Name: "crawl_jobs"}, func(context.Context, *mcpsdk.CallToolRequest, upstreamCrawlInput) (*mcpsdk.CallToolResult, any, error) {
		return nil, map[string]any{"status": "ok", "jobs": []map[string]any{}}, nil
	})
	handler := mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server { return server }, nil)
	crawlAuthorization := make(chan string, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read MCP request: %v", err)
			return
		}
		request.Body = io.NopCloser(bytes.NewReader(body))
		var envelope struct {
			Method string `json:"method"`
			Params struct {
				Name string `json:"name"`
			} `json:"params"`
		}
		if json.Unmarshal(body, &envelope) == nil && envelope.Method == "tools/call" && envelope.Params.Name == "crawl_jobs" {
			crawlAuthorization <- request.Header.Get("Authorization")
			http.Redirect(writer, request, attacker.URL+"/mcp", http.StatusTemporaryRedirect)
			return
		}
		handler.ServeHTTP(writer, request)
	}))
	t.Cleanup(upstream.Close)

	const token = "crawl-redirect-safety-token-000000000"
	work, err := NewGetWorkMCPWork(context.Background(), GetWorkMCPConfig{
		Endpoint: upstream.URL + "/mcp", AccessToken: token, SinceDays: 7, HTTPClient: upstream.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := work(context.Background(), map[string]any{}); err == nil {
		t.Fatal("redirecting crawl unexpectedly succeeded")
	}
	select {
	case authorization := <-crawlAuthorization:
		if authorization != "Bearer "+token {
			t.Fatal("original crawl endpoint did not receive the deployment bearer")
		}
	default:
		t.Fatal("crawl request did not reach the original endpoint")
	}
	select {
	case authorization := <-attackerAuthorization:
		t.Fatalf("redirect target received authorization %q", authorization)
	default:
	}
}

func TestGetWorkMCPRejectsDeploymentPlaceholderToken(t *testing.T) {
	for _, token := range []string{"short", "replace-getwork-mcp-access-token-32chars!!"} {
		_, err := NewGetWorkMCPWork(context.Background(), GetWorkMCPConfig{
			Endpoint: "http://127.0.0.1:18100/mcp", AccessToken: token, SinceDays: 7,
		})
		if err == nil || !strings.Contains(err.Error(), "access token") {
			t.Fatalf("token %q error = %v", token, err)
		}
	}
}

func TestGetWorkMCPAcceptsTheProductionPrivateRelayEndpoint(t *testing.T) {
	endpoint, err := validGetWorkMCPEndpoint("http://getwork-mcp-relay:18101/mcp")
	if err != nil {
		t.Fatal(err)
	}
	if endpoint != "http://getwork-mcp-relay:18101/mcp" {
		t.Fatalf("endpoint = %q", endpoint)
	}
}

func TestGetWorkMCPRejectsPrivateRelayEndpointsThatCouldLeakTheBearer(t *testing.T) {
	for _, endpoint := range []string{
		"http://getwork-mcp-relay/mcp",
		"http://getwork-mcp-relay:18102/mcp",
		"http://getwork-mcp-relay:18101/healthz",
	} {
		if _, err := validGetWorkMCPEndpoint(endpoint); err == nil {
			t.Fatalf("endpoint %q was accepted", endpoint)
		}
	}
}

func TestGetWorkMCPKeepsSuccessfulSourcesWhenOneSourceFails(t *testing.T) {
	server := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "getwork-test", Version: "0.1.0"}, nil)
	mcpsdk.AddTool(server, &mcpsdk.Tool{Name: "list_sources"}, func(context.Context, *mcpsdk.CallToolRequest, struct{}) (*mcpsdk.CallToolResult, any, error) {
		return nil, map[string]any{"status": "ok", "sources": []map[string]any{{"key": "tencent"}, {"key": "meituan"}}}, nil
	})
	mcpsdk.AddTool(server, &mcpsdk.Tool{Name: "crawl_jobs"}, func(_ context.Context, _ *mcpsdk.CallToolRequest, input upstreamCrawlInput) (*mcpsdk.CallToolResult, any, error) {
		if input.Source == "tencent" {
			return nil, map[string]any{"status": "error", "reason": "source_unavailable"}, nil
		}
		return nil, map[string]any{"status": "ok", "source": input.Source, "jobs": []map[string]any{}}, nil
	})
	handler := mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server { return server }, &mcpsdk.StreamableHTTPOptions{
		Stateless: true, JSONResponse: true,
	})
	upstream := httptest.NewServer(handler)
	t.Cleanup(upstream.Close)

	work, err := NewGetWorkMCPWork(context.Background(), GetWorkMCPConfig{
		Endpoint: upstream.URL + "/mcp", AccessToken: "degraded-source-test-token-000000000000", SinceDays: 7,
		HTTPClient: upstream.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := work(context.Background(), map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if result.SourceCount != 2 || result.JobCount != 0 {
		t.Fatalf("result = %+v", result)
	}
	states := result.Payload.(map[string]any)["sources"].(map[string]any)
	if states["getwork.tencent"].(map[string]any)["status"] != "failed" || states["getwork.meituan"].(map[string]any)["status"] != "success" {
		t.Fatalf("states = %+v", states)
	}
}

func TestGetWorkMCPScansAllSourcesWithBoundedConcurrency(t *testing.T) {
	sources := []string{"alibaba", "baidu", "beike", "bytedance", "ctrip", "dewu"}
	server := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "getwork-test", Version: "0.1.0"}, nil)
	mcpsdk.AddTool(server, &mcpsdk.Tool{Name: "list_sources"}, func(context.Context, *mcpsdk.CallToolRequest, struct{}) (*mcpsdk.CallToolResult, any, error) {
		listed := make([]map[string]any, 0, len(sources))
		for _, source := range sources {
			listed = append(listed, map[string]any{"key": source})
		}
		return nil, map[string]any{"status": "ok", "sources": listed}, nil
	})
	var active atomic.Int32
	var maximum atomic.Int32
	var calls atomic.Int32
	mcpsdk.AddTool(server, &mcpsdk.Tool{Name: "crawl_jobs"}, func(_ context.Context, _ *mcpsdk.CallToolRequest, input upstreamCrawlInput) (*mcpsdk.CallToolResult, any, error) {
		current := active.Add(1)
		defer active.Add(-1)
		calls.Add(1)
		for observed := maximum.Load(); current > observed && !maximum.CompareAndSwap(observed, current); observed = maximum.Load() {
		}
		time.Sleep(50 * time.Millisecond)
		return nil, map[string]any{"status": "ok", "source": input.Source, "jobs": []map[string]any{}}, nil
	})
	handler := mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server { return server }, &mcpsdk.StreamableHTTPOptions{
		Stateless: true, JSONResponse: true,
	})
	var initializeCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read MCP request: %v", err)
			return
		}
		request.Body = io.NopCloser(bytes.NewReader(body))
		var envelope struct {
			Method string `json:"method"`
		}
		if json.Unmarshal(body, &envelope) == nil && envelope.Method == "initialize" {
			initializeCalls.Add(1)
		}
		handler.ServeHTTP(writer, request)
	}))
	t.Cleanup(upstream.Close)

	work, err := NewGetWorkMCPWork(context.Background(), GetWorkMCPConfig{
		Endpoint: upstream.URL + "/mcp", AccessToken: "bounded-concurrency-test-token-000000000", SinceDays: 7,
		HTTPClient: upstream.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := work(context.Background(), map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if result.SourceCount != len(sources) || calls.Load() != int32(len(sources)) {
		t.Fatalf("source_count=%d calls=%d, want %d", result.SourceCount, calls.Load(), len(sources))
	}
	if maximum.Load() <= 1 || maximum.Load() > 4 {
		t.Fatalf("maximum concurrent crawls = %d, want 2..4", maximum.Load())
	}
	if initializeCalls.Load() != 0 {
		t.Fatalf("MCP initialize calls = %d, want no stateful sessions against the stateless source", initializeCalls.Load())
	}
}

func TestGetWorkMCPRefusesHTTPRedirectsBeforeSendingBearerToken(t *testing.T) {
	attackerAuthorization := make(chan string, 1)
	attacker := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		attackerAuthorization <- request.Header.Get("Authorization")
		http.Error(writer, "unexpected redirect", http.StatusBadRequest)
	}))
	t.Cleanup(attacker.Close)
	redirect := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, attacker.URL+"/mcp", http.StatusTemporaryRedirect)
	}))
	t.Cleanup(redirect.Close)

	_, err := NewGetWorkMCPWork(context.Background(), GetWorkMCPConfig{
		Endpoint: redirect.URL + "/mcp", AccessToken: "redirect-safety-test-token-00000000000", SinceDays: 7,
		HTTPClient: redirect.Client(),
	})
	if err == nil {
		t.Fatal("redirecting MCP endpoint was accepted")
	}
	select {
	case authorization := <-attackerAuthorization:
		if authorization != "" {
			t.Fatalf("redirect target received authorization %q", authorization)
		}
	default:
	}
}

func TestGetWorkMCPJobTypeFilteringKeepsUnknownUpstreamFamilies(t *testing.T) {
	for _, testCase := range []struct {
		profileType string
		jobType     string
		want        bool
	}{
		{profileType: "campus_recruit", jobType: "daily_intern", want: false},
		{profileType: "daily_intern", jobType: "campus_recruit", want: false},
		{profileType: "campus_recruit", jobType: "技术类", want: true},
		{profileType: "daily_intern", jobType: "产品类", want: true},
	} {
		if got := getWorkJobTypeMatches(testCase.profileType, testCase.jobType); got != testCase.want {
			t.Fatalf("getWorkJobTypeMatches(%q, %q) = %t, want %t", testCase.profileType, testCase.jobType, got, testCase.want)
		}
	}
}

func TestGetWorkMCPApplyURLAcceptsEveryConfiguredSourceOfficialHTTPSHost(t *testing.T) {
	for _, testCase := range []struct {
		source string
		url    string
		want   bool
	}{
		{source: "alibaba", url: "https://campus-talent.alibaba.com/campus/position/1", want: true},
		{source: "baidu", url: "https://talent.baidu.com/jobs/1", want: true},
		{source: "beike", url: "https://campus.ke.com/jobs/1", want: true},
		{source: "ctrip", url: "https://job.ctrip.com/jobs/1", want: true},
		{source: "dewu", url: "https://campus.dewu.com/jobs/1", want: true},
		{source: "didi", url: "https://app.mokahr.com/jobs/1", want: true},
		{source: "jd", url: "https://campus.jd.com/jobs/1", want: true},
		{source: "kuaishou", url: "https://zhaopin.kuaishou.cn/jobs/1", want: true},
		{source: "meituan", url: "https://zhaopin.meituan.com/web/position/detail?id=1", want: true},
		{source: "netease", url: "https://hr.163.com/jobs/1", want: true},
		{source: "pdd", url: "https://careers.pddglobalhr.com/jobs/1", want: true},
		{source: "meituan", url: "http://zhaopin.meituan.com/web/position/detail?id=1", want: false},
		{source: "bytedance", url: "https://jobs.bytedance.com/campus/position/1", want: true},
		{source: "tencent", url: "https://join.qq.com/post.html?id=1", want: true},
		{source: "tencentmusic", url: "https://join.tencentmusic.com/jobs/1", want: true},
		{source: "tongcheng", url: "https://mhr.ly.com/jobs/1", want: true},
		{source: "vipshop", url: "https://app-tc.mokahr.com/jobs/1", want: true},
		{source: "xfusion", url: "https://career.xfusion.com/jobs/1", want: true},
		{source: "xiaohongshu", url: "https://job.xiaohongshu.com/jobs/1", want: true},
		{source: "unknown", url: "https://jobs.example.test/position/1", want: false},
		{source: "bytedance", url: "https://user@jobs.bytedance.com/campus/position/1", want: false},
		{source: "bytedance", url: "https://jobs.bytedance.com:8443/campus/position/1", want: false},
	} {
		if got := approvedGetWorkApplyURL(testCase.source, testCase.url); got != testCase.want {
			t.Fatalf("approvedGetWorkApplyURL(%q, %q) = %t, want %t", testCase.source, testCase.url, got, testCase.want)
		}
	}
}

func TestGetWorkMCPRejectsConfiguredSourceWithoutFixedHostPolicy(t *testing.T) {
	server := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "getwork-test", Version: "0.1.0"}, nil)
	mcpsdk.AddTool(server, &mcpsdk.Tool{Name: "list_sources"}, func(context.Context, *mcpsdk.CallToolRequest, struct{}) (*mcpsdk.CallToolResult, any, error) {
		return nil, map[string]any{"status": "ok", "sources": []map[string]any{{"key": "unknown"}}}, nil
	})
	mcpsdk.AddTool(server, &mcpsdk.Tool{Name: "crawl_jobs"}, func(context.Context, *mcpsdk.CallToolRequest, upstreamCrawlInput) (*mcpsdk.CallToolResult, any, error) {
		return nil, map[string]any{"status": "ok", "jobs": []map[string]any{}}, nil
	})
	handler := mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server { return server }, nil)
	upstream := httptest.NewServer(handler)
	t.Cleanup(upstream.Close)

	_, err := NewGetWorkMCPWork(context.Background(), GetWorkMCPConfig{
		Endpoint: upstream.URL + "/mcp", AccessToken: "unknown-source-policy-test-token-00000000", SinceDays: 7,
		HTTPClient: upstream.Client(),
	})
	if err == nil || !strings.Contains(err.Error(), "no fixed upstream apply URL policy") {
		t.Fatalf("unknown source policy error = %v", err)
	}
}

func TestGetWorkMCPCanonicalizesOnlyPinnedXiaohongshuHTTPFallback(t *testing.T) {
	if got := canonicalGetWorkApplyURL("xiaohongshu", "http://job.xiaohongshu.com/campus"); got != "https://job.xiaohongshu.com/campus" {
		t.Fatalf("canonical Xiaohongshu URL = %q", got)
	}
	for _, testCase := range []struct {
		source string
		url    string
	}{
		{source: "meituan", url: "http://zhaopin.meituan.com/jobs/1"},
		{source: "xiaohongshu", url: "http://evil.example/jobs/1"},
		{source: "xiaohongshu", url: "http://user@job.xiaohongshu.com/jobs/1"},
		{source: "xiaohongshu", url: "http://job.xiaohongshu.com:8080/jobs/1"},
	} {
		if got := canonicalGetWorkApplyURL(testCase.source, testCase.url); got != testCase.url {
			t.Fatalf("canonicalGetWorkApplyURL(%q, %q) = %q", testCase.source, testCase.url, got)
		}
	}
}

func TestGetWorkMCPBoundsPersistedJobsAfterStableRelevanceSorting(t *testing.T) {
	jobs := make([]Job, 0, maxGetWorkResultJobs+2)
	for index := 0; index < maxGetWorkResultJobs+2; index++ {
		jobs = append(jobs, Job{
			SourceKey: "getwork.meituan", Title: fmt.Sprintf("岗位 %03d", index),
			MatchScore: index % 101, MatchReasons: []string{"匹配技术栈 Go"},
		})
	}
	bounded, matched, retained := boundedGetWorkJobs(jobs)
	if len(bounded) != maxGetWorkResultJobs || matched != maxGetWorkResultJobs {
		t.Fatalf("bounded jobs=%d matched=%d", len(bounded), matched)
	}
	if bounded[0].MatchScore != 100 || bounded[len(bounded)-1].MatchScore != 1 {
		t.Fatalf("bounded score range = %d..%d", bounded[0].MatchScore, bounded[len(bounded)-1].MatchScore)
	}
	if retained["getwork.meituan"] != maxGetWorkResultJobs {
		t.Fatalf("retained by source = %v", retained)
	}
}
