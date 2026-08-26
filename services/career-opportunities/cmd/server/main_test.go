package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	career "henukit.dev/career"
)

func TestBuildWorkUsesAuthorizedGetWorkMCP(t *testing.T) {
	mcpServer := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "getwork-test", Version: "0.1.0"}, nil)
	mcpsdk.AddTool(mcpServer, &mcpsdk.Tool{Name: "list_sources"}, func(context.Context, *mcpsdk.CallToolRequest, struct{}) (*mcpsdk.CallToolResult, any, error) {
		return nil, map[string]any{"status": "ok", "sources": []map[string]any{{"key": "meituan"}}}, nil
	})
	type crawlInput struct {
		Source    string `json:"source"`
		SinceDays int    `json:"since_days"`
	}
	mcpsdk.AddTool(mcpServer, &mcpsdk.Tool{Name: "crawl_jobs"}, func(_ context.Context, _ *mcpsdk.CallToolRequest, input crawlInput) (*mcpsdk.CallToolResult, any, error) {
		if input.Source != "meituan" {
			return nil, nil, fmt.Errorf("unexpected source %q", input.Source)
		}
		return nil, map[string]any{
			"status": "ok", "source": "meituan", "fetched_at": "2026-08-26T00:00:00Z",
			"jobs": []map[string]any{{
				"title": "Go 后端实习生", "company": "美团", "source": "meituan", "location": "北京",
				"job_type": "实习", "description": "Go 后端开发", "apply_url": "https://zhaopin.meituan.com/job/1",
			}},
		}, nil
	})
	handler := mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server { return mcpServer }, nil)
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer getwork-test-access-token-32-bytes" {
			http.Error(writer, "unauthorized", http.StatusUnauthorized)
			return
		}
		handler.ServeHTTP(writer, request)
	}))
	defer upstream.Close()

	t.Setenv("CAREER_GETWORK_MCP_URL", upstream.URL+"/mcp")
	t.Setenv("CAREER_GETWORK_MCP_ACCESS_TOKEN", "getwork-test-access-token-32-bytes")
	t.Setenv("CAREER_GETWORK_SINCE_DAYS", "7")
	work, err := buildWork()
	if err != nil {
		t.Fatal(err)
	}
	result, err := work(context.Background(), map[string]any{"target_roles": "后端", "tech_stack": "Go", "locations": "北京", "job_type": "daily_intern"})
	if err != nil {
		t.Fatal(err)
	}
	if result.SourceCount != 1 || result.JobCount != 1 || result.MatchedCount != 1 {
		t.Fatalf("MCP result = %+v", result)
	}
}

func TestBuildExtractorRequiresRealProductionLLM(t *testing.T) {
	t.Setenv("CAREER_REQUIRE_AI", "1")
	t.Setenv("CAREER_AI_MODE", "")
	t.Setenv("CAREER_AI_BASE_URL", "")
	t.Setenv("CAREER_AI_API_KEY", "")
	t.Setenv("CAREER_AI_MODEL", "")
	if _, err := buildExtractor(); err == nil {
		t.Fatal("production startup accepted a missing extraction LLM")
	}

	t.Setenv("CAREER_AI_MODE", "mock")
	if _, err := buildExtractor(); err == nil {
		t.Fatal("production startup accepted the mock extraction LLM")
	}

	t.Setenv("CAREER_AI_MODE", "")
	t.Setenv("CAREER_AI_BASE_URL", "https://llm.provider.internal/v1")
	t.Setenv("CAREER_AI_API_KEY", "sk-production-secret")
	t.Setenv("CAREER_AI_MODEL", "qwen-production")
	t.Setenv("CAREER_ALLOW_INSECURE_AI_HTTP", "0")
	extractor, err := buildExtractor()
	if err != nil {
		t.Fatal(err)
	}
	if extractor == nil {
		t.Fatal("configured production extraction LLM returned a nil extractor")
	}

	t.Setenv("CAREER_AI_BASE_URL", "http://125.46.96.207:30000/v1")
	if _, err := buildExtractor(); err == nil {
		t.Fatal("production startup accepted the plaintext public LLM without an explicit exception")
	}
	t.Setenv("CAREER_ALLOW_INSECURE_AI_HTTP", "1")
	if _, err := buildExtractor(); err != nil {
		t.Fatalf("production startup rejected the exact approved plaintext LLM: %v", err)
	}
	t.Setenv("CAREER_ALLOW_INSECURE_AI_HTTP", "0")
	for _, endpoint := range []string{
		"http://10.0.0.8:30000/v1",
		"http://192.168.1.8:30000/v1",
		"http://169.254.169.254/v1",
	} {
		t.Setenv("CAREER_AI_BASE_URL", endpoint)
		if _, err := buildExtractor(); err == nil {
			t.Fatalf("production startup accepted plaintext non-loopback LLM endpoint %s", endpoint)
		}
	}
	t.Setenv("CAREER_AI_BASE_URL", "http://127.0.0.1:30000/v1")
	if _, err := buildExtractor(); err != nil {
		t.Fatalf("production startup rejected literal loopback endpoint: %v", err)
	}
}

func TestBuildExtractorDoesNotBroadenTheHTTPException(t *testing.T) {
	t.Setenv("CAREER_REQUIRE_AI", "1")
	t.Setenv("CAREER_AI_MODE", "")
	t.Setenv("CAREER_AI_API_KEY", "sk-production-secret")
	t.Setenv("CAREER_AI_MODEL", "kaifengfu-chat")
	t.Setenv("CAREER_ALLOW_INSECURE_AI_HTTP", "1")
	for _, endpoint := range []string{
		"http://125.46.96.207:30000",
		"http://125.46.96.207:30000/v1/other",
		"http://125.46.96.208:30000/v1",
	} {
		t.Setenv("CAREER_AI_BASE_URL", endpoint)
		if _, err := buildExtractor(); err == nil {
			t.Fatalf("HTTP exception broadened to %s", endpoint)
		}
	}
}

func TestBuildSuifierUsesTheConfiguredCareerLLM(t *testing.T) {
	t.Setenv("CAREER_REQUIRE_AI", "1")
	t.Setenv("CAREER_AI_MODE", "")
	t.Setenv("CAREER_AI_BASE_URL", "https://llm.provider.internal/v1")
	t.Setenv("CAREER_AI_API_KEY", "sk-production-secret")
	t.Setenv("CAREER_AI_MODEL", "qwen-production")
	t.Setenv("CAREER_ALLOW_INSECURE_AI_HTTP", "0")
	suifier, err := buildSuifier()
	if err != nil {
		t.Fatal(err)
	}
	if suifier == nil {
		t.Fatal("configured production Career LLM returned a nil suifier")
	}
}

func TestBuildSuifierRequiresItsOwnPlaintextDisclosureGate(t *testing.T) {
	t.Setenv("CAREER_REQUIRE_AI", "1")
	t.Setenv("CAREER_AI_MODE", "")
	t.Setenv("CAREER_AI_BASE_URL", approvedInsecureAIURL)
	t.Setenv("CAREER_AI_API_KEY", "sk-production-secret")
	t.Setenv("CAREER_AI_MODEL", "qwen-production")
	t.Setenv("CAREER_ALLOW_INSECURE_AI_HTTP", "1")
	t.Setenv("CAREER_SUIFY_ALLOW_INSECURE_AI_HTTP", "0")

	suifier, err := buildSuifier()
	if err != nil {
		t.Fatal(err)
	}
	if suifier != nil {
		t.Fatal("plaintext Suification was enabled without its separate disclosure gate")
	}

	t.Setenv("CAREER_SUIFY_ALLOW_INSECURE_AI_HTTP", "1")
	suifier, err = buildSuifier()
	if err != nil {
		t.Fatal(err)
	}
	if suifier == nil {
		t.Fatal("explicitly approved plaintext Suification returned a nil suifier")
	}
}

func TestBuildSuifierDoesNotBroadenThePlaintextException(t *testing.T) {
	t.Setenv("CAREER_REQUIRE_AI", "1")
	t.Setenv("CAREER_AI_MODE", "")
	t.Setenv("CAREER_AI_BASE_URL", "https://llm.provider.internal/v1")
	t.Setenv("CAREER_AI_API_KEY", "sk-production-secret")
	t.Setenv("CAREER_AI_MODEL", "qwen-production")
	t.Setenv("CAREER_ALLOW_INSECURE_AI_HTTP", "0")
	t.Setenv("CAREER_SUIFY_ALLOW_INSECURE_AI_HTTP", "1")

	if _, err := buildSuifier(); err == nil {
		t.Fatal("Suification plaintext exception was accepted for an HTTPS provider")
	}
}

func TestProbeExtractorRequiresARealStructuredResponse(t *testing.T) {
	good := func(_ context.Context, fileName string, content []byte) (career.ExtractedProfile, error) {
		if fileName != "startup-probe.pdf" || !strings.HasPrefix(string(content), "%PDF-") {
			t.Fatalf("startup probe did not exercise the PDF path: %q", fileName)
		}
		return career.ExtractedProfile{TargetRoles: "后端开发"}, nil
	}
	if err := probeExtractor(context.Background(), good); err != nil {
		t.Fatal(err)
	}
	bad := func(context.Context, string, []byte) (career.ExtractedProfile, error) {
		return career.ExtractedProfile{}, errors.New("provider unavailable")
	}
	if err := probeExtractor(context.Background(), bad); err == nil {
		t.Fatal("failed provider passed the startup probe")
	}
}
