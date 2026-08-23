package career

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAICompatibleSuifierSendsSkillAndOriginalText(t *testing.T) {
	const original = "负责校园资料检索网站开发"
	const draft = "负责校园资料检索网站开发，聚焦校园资料检索场景"

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/chat/completions" {
			t.Fatalf("provider request = %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("provider authorization = %q", got)
		}
		var payload struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload.Model != "career-test" || len(payload.Messages) != 2 {
			t.Fatalf("provider payload = %+v", payload)
		}
		if payload.Messages[0].Role != "system" ||
			!strings.Contains(payload.Messages[0].Content, "酥化") ||
			!strings.Contains(payload.Messages[0].Content, "不得新增") {
			t.Fatalf("system message does not contain the resume suification skill: %q", payload.Messages[0].Content)
		}
		if payload.Messages[1].Role != "user" || payload.Messages[1].Content != original {
			t.Fatalf("original resume text changed before provider call: %+v", payload.Messages[1])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"` + draft + `"}}]}`))
	}))
	defer upstream.Close()

	suify, err := NewOpenAICompatibleSuifier(SuifyConfig{
		BaseURL: upstream.URL,
		APIKey:  "test-key",
		Model:   "career-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := suify(context.Background(), original)
	if err != nil {
		t.Fatal(err)
	}
	if got != draft {
		t.Fatalf("suified draft = %q, want %q", got, draft)
	}
}
