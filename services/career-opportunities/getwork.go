package career

import (
	"context"
	"errors"
	"log"
	"time"
)

// Job is one normalized, source-attributed opening produced by a Career
// Source. The matching explanation is stable, structured data, not a bare
// score, so the Portal and the completion email can both render it.
type Job struct {
	SourceKey    string   `json:"source_key"`
	Company      string   `json:"company"`
	Title        string   `json:"title"`
	Location     string   `json:"location"`
	JobType      string   `json:"job_type,omitempty"`
	Description  string   `json:"description,omitempty"`
	Requirements []string `json:"requirements,omitempty"`
	URL          string   `json:"url"`
	PublishedAt  string   `json:"published_at,omitempty"`
	FetchedAt    string   `json:"fetched_at,omitempty"`
	MatchScore   int      `json:"match_score"`   // 0..100
	MatchReasons []string `json:"match_reasons"` // stable, displayable
}

// Source is the opportunity-source decoupling seam. Every implementation is
// independently registered by the operator. A source accepts the frozen
// profile snapshot, never shared per-user config.
type Source interface {
	// Key is the stable source key (e.g. "official.meituan").
	Key() string
	// Fetch crawls and matches one frozen profile snapshot, returning
	// structured jobs. A failing source returns an error; the adapter catches
	// it and degrades rather than losing already-successful sources.
	Fetch(ctx context.Context, profile any) ([]Job, error)
}

// GetWorkConfig builds the legacy direct-source Work seam. Every registered
// source is enabled; the browser can never add a source, URL, selector, or
// platform config. An empty registry yields zero jobs for local/degraded use.
type GetWorkConfig struct {
	// Sources is the operator-owned registry the worker draws from.
	Sources []Source
}

// NewGetWorkWork returns a WorkFunc that runs every registered source against
// the frozen profile snapshot and aggregates a single authoritative result. It
// degrades per source: one source timing out or failing never discards the
// already-successful sources.
func NewGetWorkWork(config GetWorkConfig) WorkFunc {
	enabled := append([]Source(nil), config.Sources...)
	return func(ctx context.Context, profile any) (WorkResult, error) {
		if len(enabled) == 0 {
			// Local/degraded off state: no registered source is enabled, so a
			// search completes with an empty result rather than failing.
			return WorkResult{Payload: map[string]any{"jobs": []Job{}, "sources": map[string]any{}}, SourceCount: 0, JobCount: 0, MatchedCount: 0, Summary: "暂无可用的岗位来源"}, nil
		}
		all := make([]Job, 0, 64)
		sourceState := map[string]any{}
		matched := 0
		succeeded := 0
		for _, source := range enabled {
			jobs, err := source.Fetch(ctx, profile)
			if err != nil {
				log.Printf("career: source %q failed: %v", source.Key(), err)
				sourceState[source.Key()] = map[string]any{"status": "failed"}
				continue
			}
			sourceState[source.Key()] = map[string]any{"status": "success", "found": len(jobs)}
			succeeded++
			for _, job := range jobs {
				if careerJobIsRelevant(job) {
					matched++
				}
			}
			all = append(all, jobs...)
		}
		if succeeded == 0 {
			return WorkResult{}, errors.New("all configured career sources failed")
		}
		return WorkResult{
			Payload:      map[string]any{"jobs": all, "sources": sourceState},
			SourceCount:  len(enabled),
			JobCount:     len(all),
			MatchedCount: matched,
			Summary:      careerScanSummary(len(enabled), succeeded, len(all), matched),
		}, nil
	}
}

// fakeSource is test-only: deterministic, registry-gated, no network or shared
// config.
type fakeSource struct {
	key string
}

func (f *fakeSource) Key() string { return f.key }

func (f *fakeSource) Fetch(ctx context.Context, profile any) ([]Job, error) {
	_ = ctx
	now := time.Now().UTC().Format(time.RFC3339)
	return []Job{{
		SourceKey:    f.key,
		Company:      "测试公司",
		Title:        "后端开发实习生",
		Location:     "郑州",
		JobType:      "daily_intern",
		URL:          "https://example.test/jobs/1",
		FetchedAt:    now,
		MatchScore:   90,
		MatchReasons: []string{"匹配目标岗位 后端开发", "匹配技术栈 go"},
	}}, nil
}
