package career

import (
	"context"
	"fmt"
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
	MatchScore   int      `json:"match_score"`             // 0..100
	MatchReasons []string `json:"match_reasons,omitempty"` // stable, displayable
}

// Source is the GetWork decoupling seam. #372/#374 settle whether real sources
// reuse GetWork Core, fork it, or stay independent; until then only a fake
// source (or an allowlisted real source) may implement it. The crawler must
// accept an external profile snapshot, never a shared per-user config file.
type Source interface {
	// Key is the stable allowlist key (e.g. "getwork.liepin").
	Key() string
	// Fetch crawls and matches one frozen profile snapshot, returning
	// structured jobs. A failing source returns an error; the adapter catches
	// it and degrades rather than losing already-successful sources.
	Fetch(ctx context.Context, profile any) ([]Job, error)
}

// GetWorkConfig builds the Work seam from a source allowlist. Only source keys
// present in the allowlist are ever consulted; the browser can never add a
// source, URL, selector, or platform config. An empty allowlist is the safe
// default and yields zero jobs until #372/#374 authorize real sources.
type GetWorkConfig struct {
	// AllowSources is the set of authorized source keys. A nil/empty set runs
	// no source at all (production-safe off state).
	AllowSources map[string]bool
	// Sources is the registry the worker draws from. Sources not in
	// AllowSources are ignored even if present here.
	Sources []Source
}

// NewGetWorkWork returns a WorkFunc that runs every allowlisted source against
// the frozen profile snapshot and aggregates a single authoritative result. It
// degrades per source: one source timing out or failing never discards the
// already-successful sources.
func NewGetWorkWork(config GetWorkConfig) WorkFunc {
	byKey := map[string]Source{}
	for _, source := range config.Sources {
		byKey[source.Key()] = source
	}
	enabled := make([]Source, 0, len(config.AllowSources))
	for key := range config.AllowSources {
		if source, ok := byKey[key]; ok {
			enabled = append(enabled, source)
		} else {
			log.Printf("career: allowlisted source %q has no registered implementation; skipping", key)
		}
	}
	return func(ctx context.Context, profile any) (WorkResult, error) {
		if len(enabled) == 0 {
			// Production-safe off state: no authorized source is enabled, so a
			// search completes with an empty result rather than failing.
			return WorkResult{Payload: map[string]any{"jobs": []Job{}, "sources": map[string]any{}}, SourceCount: 0, JobCount: 0, MatchedCount: 0, Summary: "暂无可用的岗位来源"}, nil
		}
		all := make([]Job, 0, 64)
		sourceState := map[string]any{}
		matched := 0
		for _, source := range enabled {
			jobs, err := source.Fetch(ctx, profile)
			if err != nil {
				log.Printf("career: source %q failed: %v", source.Key(), err)
				sourceState[source.Key()] = map[string]any{"status": "failed"}
				continue
			}
			sourceState[source.Key()] = map[string]any{"status": "success", "found": len(jobs)}
			for _, job := range jobs {
				if job.MatchScore >= 50 {
					matched++
				}
			}
			all = append(all, jobs...)
		}
		return WorkResult{
			Payload:      map[string]any{"jobs": all, "sources": sourceState},
			SourceCount:  len(enabled),
			JobCount:     len(all),
			MatchedCount: matched,
			Summary:      fmt.Sprintf("已扫描 %d 个来源，发现 %d 个岗位，%d 个推荐", len(enabled), len(all), matched),
		}, nil
	}
}

// fakeSource is the #396 test-only source: deterministic, allowlist-gated, no
// network, no shared config. It proves the adapter path end-to-end before any
// real GetWork source is authorized.
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
