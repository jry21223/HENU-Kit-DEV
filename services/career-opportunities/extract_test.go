package career

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestNewOpenAICompatibleExtractorRejectsEmptyConfig(t *testing.T) {
	if _, err := NewOpenAICompatibleExtractor(ExtractConfig{}); !errors.Is(err, ErrAIUnconfigured) {
		t.Fatalf("empty config error = %v, want ErrAIUnconfigured", err)
	}
	if _, err := NewOpenAICompatibleExtractor(ExtractConfig{BaseURL: "https://ai.example", APIKey: "key"}); !errors.Is(err, ErrAIUnconfigured) {
		t.Fatalf("config without model error = %v, want ErrAIUnconfigured", err)
	}
	if _, err := NewOpenAICompatibleExtractor(ExtractConfig{BaseURL: "not-a-url", APIKey: "key", Model: "model"}); !errors.Is(err, ErrAIUnconfigured) {
		t.Fatalf("invalid URL error = %v, want ErrAIUnconfigured", err)
	}
}

func TestOpenAICompatibleExtractorParsesProviderResponse(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("provider path = %q, want /chat/completions", r.URL.Path)
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer test-key" {
			t.Fatalf("provider Authorization = %q, want Bearer test-key", auth)
		}
		if contentType := r.Header.Get("Content-Type"); contentType != "application/json" {
			t.Fatalf("provider Content-Type = %q, want application/json", contentType)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"target_roles\":\"后端开发\",\"tech_stack\":\"go,postgres\",\"locations\":\"郑州\",\"job_type\":\"daily_intern\",\"graduation_year\":2027,\"resume_text\":\"校内项目经历\"}"}}]}`))
	}))
	defer upstream.Close()
	extract, err := NewOpenAICompatibleExtractor(ExtractConfig{
		BaseURL: upstream.URL, APIKey: "test-key", Model: "test-model",
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := extract(context.Background(), "resume.txt", []byte("简历内容"))
	if err != nil {
		t.Fatal(err)
	}
	if result.TargetRoles != "后端开发" || result.TechStack != "go,postgres" || result.Locations != "郑州" {
		t.Fatalf("extracted fields mismatch: %+v", result)
	}
	if result.JobType != "daily_intern" || result.GraduationYear == nil || *result.GraduationYear != 2027 {
		t.Fatalf("extracted enum/year mismatch: %+v", result)
	}
	if result.ResumeText != "校内项目经历" {
		t.Fatalf("extracted resume_text = %q", result.ResumeText)
	}
}

func TestOpenAICompatibleExtractorSendsPDFPagesAndReadableDOCXText(t *testing.T) {
	var received any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []struct {
				Role    string `json:"role"`
				Content any    `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		for _, message := range body.Messages {
			if message.Role == "user" {
				received = message.Content
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"target_roles\":\"后端开发\"}"}}]}`))
	}))
	defer upstream.Close()
	extract, err := NewOpenAICompatibleExtractor(ExtractConfig{BaseURL: upstream.URL, APIKey: "test-key", Model: "test-model"})
	if err != nil {
		t.Fatal(err)
	}
	received = nil
	if _, err := extract(context.Background(), "resume.docx", testDOCX(t, "Backend Developer Go PostgreSQL")); err != nil {
		t.Fatal(err)
	}
	docx, ok := received.(string)
	if !ok || !strings.Contains(docx, "Backend Developer Go PostgreSQL") || strings.Contains(docx, "PK") {
		t.Fatalf("provider received non-text DOCX content: %#v", received)
	}

	received = nil
	if _, err := extract(context.Background(), "resume.pdf", testPDF("Backend Developer Go PostgreSQL")); err != nil {
		t.Fatal(err)
	}
	parts, ok := received.([]any)
	if !ok || len(parts) != 2 {
		t.Fatalf("provider PDF content parts = %#v", received)
	}
	imagePart, ok := parts[1].(map[string]any)
	if !ok || imagePart["type"] != "image_url" {
		t.Fatalf("provider PDF image part = %#v", parts[1])
	}
	imageURL, ok := imagePart["image_url"].(map[string]any)
	if !ok || !strings.HasPrefix(imageURL["url"].(string), "data:image/jpeg;base64,") {
		t.Fatalf("provider PDF image URL = %#v", imagePart["image_url"])
	}
}

func TestOpenAICompatibleExtractorAcceptsMarkdownFencedJSON(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":` + strconv.Quote("```json\n{\"target_roles\":\"数据分析\"}\n```") + `}}]}`))
	}))
	defer upstream.Close()
	extract, err := NewOpenAICompatibleExtractor(ExtractConfig{
		BaseURL: upstream.URL, APIKey: "test-key", Model: "test-model",
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := extract(context.Background(), "resume.txt", []byte("简历"))
	if err != nil {
		t.Fatal(err)
	}
	if result.TargetRoles != "数据分析" {
		t.Fatalf("fenced JSON target_roles = %q", result.TargetRoles)
	}
}

func TestNormalizeExtractedAcceptsBoundedKeywordListsAndNullJobType(t *testing.T) {
	result, err := normalizeExtracted(`{
		"target_roles":["Go 后端开发"],
		"tech_stack":["Go","PostgreSQL"],
		"locations":["郑州"],
		"job_type":null,
		"graduation_year":null,
		"resume_text":"校内项目经历"
	}`)
	if err != nil {
		t.Fatal(err)
	}
	if result.TargetRoles != "Go 后端开发" || result.TechStack != "Go、PostgreSQL" || result.Locations != "郑州" || result.JobType != "" {
		t.Fatalf("normalized multimodal response = %+v", result)
	}
}

func TestOpenAICompatibleExtractorTruncatesToProfileLimits(t *testing.T) {
	longRoles := strings.Repeat("岗", profileTargetRolesLimit+200)
	longText := strings.Repeat("经", profileResumeTextLimit+500)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		payload := `{"choices":[{"message":{"content":"{\"target_roles\":\"` + longRoles + `\",\"resume_text\":\"` + longText + `\"}"}}]}`
		_, _ = w.Write([]byte(payload))
	}))
	defer upstream.Close()
	extract, err := NewOpenAICompatibleExtractor(ExtractConfig{
		BaseURL: upstream.URL, APIKey: "test-key", Model: "test-model",
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := extract(context.Background(), "resume.txt", []byte("简历"))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.TargetRoles) > profileTargetRolesLimit || len(result.ResumeText) > profileResumeTextLimit {
		t.Fatalf("truncated fields exceed limits: target_roles=%d resume_text=%d", len(result.TargetRoles), len(result.ResumeText))
	}
	if !strings.HasPrefix(longRoles, result.TargetRoles) || !strings.HasPrefix(longText, result.ResumeText) {
		t.Fatalf("truncated fields are not prefixes of the original text")
	}
	if !utf8.ValidString(result.TargetRoles) || !utf8.ValidString(result.ResumeText) {
		t.Fatal("truncated fields are not valid UTF-8")
	}
}

func TestOpenAICompatibleExtractorRejectsInvalidModelOutput(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{name: "not json", content: "抱歉，我无法提取"},
		{name: "bad job type", content: `{"job_type":"full_time"}`},
		{name: "bad graduation year", content: `{"graduation_year":1200}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"choices":[{"message":{"content":` + strconv.Quote(tc.content) + `}}]}`))
			}))
			defer upstream.Close()
			extract, err := NewOpenAICompatibleExtractor(ExtractConfig{
				BaseURL: upstream.URL, APIKey: "test-key", Model: "test-model",
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := extract(context.Background(), "resume.txt", []byte("简历")); !errors.Is(err, ErrExtractionFailed) {
				t.Fatalf("extract error = %v, want ErrExtractionFailed", err)
			}
		})
	}
}

func TestOpenAICompatibleExtractorSurfacesProviderFailure(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer upstream.Close()
	extract, err := NewOpenAICompatibleExtractor(ExtractConfig{
		BaseURL: upstream.URL, APIKey: "test-key", Model: "test-model",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := extract(context.Background(), "resume.txt", []byte("简历")); !errors.Is(err, ErrExtractionFailed) {
		t.Fatalf("extract error = %v, want ErrExtractionFailed", err)
	}
}

func TestOpenAICompatibleExtractorDoesNotForwardCredentialsAcrossRedirects(t *testing.T) {
	targetHit := false
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		targetHit = true
	}))
	defer target.Close()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer upstream.Close()
	extract, err := NewOpenAICompatibleExtractor(ExtractConfig{BaseURL: upstream.URL, APIKey: "secret-key", Model: "test-model"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := extract(context.Background(), "resume.txt", []byte("简历")); !errors.Is(err, ErrExtractionFailed) {
		t.Fatalf("redirect error = %v, want ErrExtractionFailed", err)
	}
	if targetHit {
		t.Fatal("provider redirect was followed with a credential-bearing request")
	}
}

func TestMockExtractorReturnsDeterministicProfile(t *testing.T) {
	extract := NewMockExtractor()
	result, err := extract(context.Background(), "resume.txt", []byte("一段经历"))
	if err != nil {
		t.Fatal(err)
	}
	if result.TargetRoles == "" || result.TechStack == "" || result.Locations == "" || result.ResumeText == "" {
		t.Fatalf("mock extractor left fields empty: %+v", result)
	}
	if result.GraduationYear == nil {
		t.Fatal("mock extractor must set graduation_year")
	}
	if _, err := extract(context.Background(), "resume.txt", nil); !errors.Is(err, ErrExtractionFailed) {
		t.Fatalf("empty file error = %v, want ErrExtractionFailed", err)
	}
}
