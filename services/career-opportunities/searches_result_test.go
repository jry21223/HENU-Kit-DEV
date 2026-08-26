package career

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDecodeSearchResultKeepsSourceOutcomesAndPositiveRankedJobs(t *testing.T) {
	payload, err := json.Marshal(map[string]any{
		"sources": map[string]any{
			"getwork.meituan": map[string]any{"status": "success", "found": 47},
			"getwork.tencent": map[string]any{"status": "failed"},
		},
		"jobs": []Job{
			{SourceKey: "getwork.meituan", Title: "低相关岗位", URL: "https://jobs.example.test/0", MatchScore: 8, MatchReasons: []string{"匹配技术栈 Go"}},
			{SourceKey: "getwork.meituan", Title: "AI 工具开发实习生", URL: "https://jobs.example.test/1", MatchScore: 24, MatchReasons: []string{"匹配技术栈 Go、MCP"}},
			{SourceKey: "getwork.meituan", Title: "其他岗位", URL: "https://jobs.example.test/2", MatchScore: 0},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := decodeSearchResult(payload, 2, 3, 0, "旧摘要")
	if err != nil {
		t.Fatal(err)
	}
	if result.MatchedCount != 2 {
		t.Fatalf("matched_count = %d, want 2 positive explainable matches", result.MatchedCount)
	}
	if len(result.Sources) != 2 || result.Sources[0].Key != "getwork.meituan" || result.Sources[1].Status != "failed" {
		t.Fatalf("sources = %+v", result.Sources)
	}
	if !strings.Contains(result.Summary, "2 个相关岗位") {
		t.Fatalf("summary = %q", result.Summary)
	}
	if result.Jobs[0].Title != "AI 工具开发实习生" || result.Jobs[1].Title != "低相关岗位" {
		t.Fatalf("jobs are not ranked by relevance: %+v", result.Jobs)
	}
}
