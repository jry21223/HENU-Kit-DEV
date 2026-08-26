package career

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestGetWorkMCPProtocolSmoke verifies the real deployment transport rather
// than a repository-local fake. CI starts the pinned upstream container and
// opts into this test through environment variables.
func TestGetWorkMCPProtocolSmoke(t *testing.T) {
	endpoint := os.Getenv("CAREER_GETWORK_MCP_SMOKE_URL")
	token := os.Getenv("CAREER_GETWORK_MCP_SMOKE_TOKEN")
	if endpoint == "" || token == "" {
		t.Skip("getWork MCP smoke endpoint is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	work, err := NewGetWorkMCPWork(ctx, GetWorkMCPConfig{
		Endpoint: endpoint, AccessToken: token, SinceDays: 7,
	})
	if err != nil {
		t.Fatal(err)
	}
	if os.Getenv("CAREER_GETWORK_MCP_SMOKE_CRAWL") != "1" {
		return
	}
	result, err := work(ctx, map[string]any{
		"target_roles": "后端开发",
		"tech_stack":   "Go",
		"locations":    "北京",
		"job_type":     "daily_intern",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.SourceCount != 18 {
		t.Fatalf("source count = %d, want every pinned upstream source", result.SourceCount)
	}
	if os.Getenv("CAREER_GETWORK_MCP_EXPECT_JOBS") == "1" && result.JobCount == 0 {
		t.Fatal("real getWork MCP crawl returned no normalized jobs")
	}
}
