package career

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type failingSource struct{ key string }

func (f *failingSource) Key() string { return f.key }
func (f *failingSource) Fetch(ctx context.Context, profile any) ([]Job, error) {
	return nil, errors.New("source timed out")
}

// TestGetWorkEmptyAllowlistYieldsNoJobs verifies the production-safe off state:
// with no authorized source, a search completes with an empty result and never
// fails.
func TestGetWorkEmptyAllowlistYieldsNoJobs(t *testing.T) {
	work := NewGetWorkWork(GetWorkConfig{})
	result, err := work(context.Background(), map[string]any{"target_roles": "后端"})
	if err != nil {
		t.Fatal(err)
	}
	if result.SourceCount != 0 || result.JobCount != 0 {
		t.Fatalf("empty allowlist produced source_count=%d job_count=%d", result.SourceCount, result.JobCount)
	}
	payload := result.Payload.(map[string]any)
	if jobs := payload["jobs"].([]Job); len(jobs) != 0 {
		t.Fatalf("empty allowlist produced jobs: %v", jobs)
	}
}

// TestGetWorkAllowlistGatesSources verifies only allowlisted sources run: a
// registered-but-not-allowlisted source is ignored.
func TestGetWorkAllowlistGatesSources(t *testing.T) {
	work := NewGetWorkWork(GetWorkConfig{
		AllowSources: map[string]bool{"authorized": true},
		Sources: []Source{
			&fakeSource{key: "authorized"},
			&fakeSource{key: "unlisted"},
		},
	})
	result, err := work(context.Background(), map[string]any{"target_roles": "后端"})
	if err != nil {
		t.Fatal(err)
	}
	if result.SourceCount != 1 {
		t.Fatalf("source_count = %d, want 1 (only the allowlisted source)", result.SourceCount)
	}
	payload := result.Payload.(map[string]any)
	jobs := payload["jobs"].([]Job)
	if len(jobs) != 1 || jobs[0].SourceKey != "authorized" {
		t.Fatalf("jobs = %+v, want one job from authorized", jobs)
	}
	if jobs[0].MatchScore != 90 || len(jobs[0].MatchReasons) == 0 || jobs[0].URL == "" {
		t.Fatalf("structured result incomplete: %+v", jobs[0])
	}
	if !strings.Contains(result.Summary, "1 个来源") {
		t.Fatalf("summary = %q", result.Summary)
	}
}

// TestGetWorkAllSourcesFailedFailsSearch verifies an upstream outage is never
// disguised as a successful zero-result scan.
func TestGetWorkAllSourcesFailedFailsSearch(t *testing.T) {
	work := NewGetWorkWork(GetWorkConfig{
		AllowSources: map[string]bool{"bad.a": true, "bad.b": true},
		Sources: []Source{
			&failingSource{key: "bad.a"},
			&failingSource{key: "bad.b"},
		},
	})
	if _, err := work(context.Background(), map[string]any{"target_roles": "后端"}); err == nil {
		t.Fatal("all failed sources were reported as a successful scan")
	}
}

// TestGetWorkSingleSourceFailureDegrades verifies one failing source does not
// lose the successful sources' results.
func TestGetWorkSingleSourceFailureDegrades(t *testing.T) {
	work := NewGetWorkWork(GetWorkConfig{
		AllowSources: map[string]bool{"ok": true, "bad": true},
		Sources: []Source{
			&fakeSource{key: "ok"},
			&failingSource{key: "bad"},
		},
	})
	result, err := work(context.Background(), map[string]any{"target_roles": "后端"})
	if err != nil {
		t.Fatal(err)
	}
	if result.SourceCount != 2 {
		t.Fatalf("source_count = %d, want 2", result.SourceCount)
	}
	payload := result.Payload.(map[string]any)
	jobs := payload["jobs"].([]Job)
	if len(jobs) != 1 || jobs[0].SourceKey != "ok" {
		t.Fatalf("jobs = %+v, want the successful source's job only", jobs)
	}
	sources := payload["sources"].(map[string]any)
	if sources["ok"].(map[string]any)["status"] != "success" || sources["bad"].(map[string]any)["status"] != "failed" {
		t.Fatalf("source state = %v", sources)
	}
}
