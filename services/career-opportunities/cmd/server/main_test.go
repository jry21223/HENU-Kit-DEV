package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	career "henukit.dev/career"
)

func TestBuildWorkEnablesAuthorizedOfficialSource(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"message":"成功","data":{"page":{"totalPage":1},"list":[{"jobUnionId":"job-1","name":"后端开发实习生","cityList":[{"name":"北京市"}],"jobDuty":"Go 服务研发","jobRequirement":"熟悉 Go"}]}}`))
	}))
	defer server.Close()

	t.Setenv("CAREER_SOURCE_ALLOWLIST", "official.meituan")
	t.Setenv("CAREER_MEITUAN_API_URL", server.URL)
	work, err := buildWork()
	if err != nil {
		t.Fatal(err)
	}
	result, err := work(context.Background(), map[string]any{
		"target_roles": "后端开发",
		"tech_stack":   "Go",
		"locations":    "北京",
		"job_type":     "daily_intern",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.SourceCount != 1 || result.JobCount != 1 || result.MatchedCount != 1 {
		t.Fatalf("result = %+v, want one real matched job from one source", result)
	}
}

func TestBuildWorkRejectsUnknownAuthorizedSource(t *testing.T) {
	t.Setenv("CAREER_SOURCE_ALLOWLIST", "official.unknown")
	if _, err := buildWork(); err == nil {
		t.Fatal("unknown allowlisted source was silently accepted")
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
