package career

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type upstreamCrawlInput struct {
	Source    string `json:"source"`
	SinceDays int    `json:"since_days"`
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
				"title": "Go 后端开发实习生", "company": "美团校招", "source": "meituan",
				"location": "北京", "job_type": "实习", "description": "负责后端服务开发",
				"requirement": "熟悉 Go 和 PostgreSQL", "apply_url": "https://zhaopin.meituan.com/job/1",
				"publish_date": "2026-08-24",
			}},
		}, nil
	})
	handler := mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server { return server }, nil)
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer getwork-test-token-0000000000000000" {
			http.Error(writer, "unauthorized", http.StatusUnauthorized)
			return
		}
		handler.ServeHTTP(writer, request)
	}))
	t.Cleanup(upstream.Close)

	work, err := NewGetWorkMCPWork(context.Background(), GetWorkMCPConfig{
		Endpoint:     upstream.URL + "/mcp",
		AccessToken:  "getwork-test-token-0000000000000000",
		AllowSources: []string{"meituan"},
		SinceDays:    7,
		HTTPClient:   upstream.Client(),
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

func TestGetWorkMCPRejectsDeploymentPlaceholderToken(t *testing.T) {
	for _, token := range []string{"short", "replace-getwork-mcp-access-token-32chars!!"} {
		_, err := NewGetWorkMCPWork(context.Background(), GetWorkMCPConfig{
			Endpoint:     "http://127.0.0.1:18100/mcp",
			AccessToken:  token,
			AllowSources: []string{"meituan"},
			SinceDays:    7,
		})
		if err == nil || !strings.Contains(err.Error(), "access token") {
			t.Fatalf("token %q error = %v", token, err)
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
	handler := mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server { return server }, nil)
	upstream := httptest.NewServer(handler)
	t.Cleanup(upstream.Close)

	work, err := NewGetWorkMCPWork(context.Background(), GetWorkMCPConfig{
		Endpoint: upstream.URL + "/mcp", AccessToken: "degraded-source-test-token-000000000000", AllowSources: []string{"meituan", "tencent"}, SinceDays: 7,
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
		Endpoint: redirect.URL + "/mcp", AccessToken: "redirect-safety-test-token-00000000000", AllowSources: []string{"meituan"}, SinceDays: 7,
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

func TestGetWorkMCPApplyURLRequiresApprovedHTTPSHost(t *testing.T) {
	for _, testCase := range []struct {
		source string
		url    string
		want   bool
	}{
		{source: "meituan", url: "https://zhaopin.meituan.com/web/position/detail?id=1", want: true},
		{source: "meituan", url: "http://zhaopin.meituan.com/web/position/detail?id=1", want: false},
		{source: "meituan", url: "https://example.test/phishing", want: false},
		{source: "tencent", url: "https://join.qq.com/post.html?id=1", want: true},
		{source: "unknown", url: "https://zhaopin.meituan.com/web/position/detail?id=1", want: false},
	} {
		if got := approvedGetWorkApplyURL(testCase.source, testCase.url); got != testCase.want {
			t.Fatalf("approvedGetWorkApplyURL(%q, %q) = %t, want %t", testCase.source, testCase.url, got, testCase.want)
		}
	}
}

func TestGetWorkMCPRejectsSourceWithoutApplyURLPolicy(t *testing.T) {
	_, err := NewGetWorkMCPWork(context.Background(), GetWorkMCPConfig{
		Endpoint: "http://127.0.0.1:18100/mcp", AccessToken: "source-policy-test-token-0000000000000", AllowSources: []string{"unknown"}, SinceDays: 7,
	})
	if err == nil || !strings.Contains(err.Error(), "apply URL policy") {
		t.Fatalf("error = %v", err)
	}
}
