package contract

// Career digest mail enqueue route and types (#397). These mirror the
// career-digest-mails OpenAPI operation; contractgen's fixed template cannot
// render array-typed request fields, so the wire types stay here next to the
// generated contract while the OpenAPI document remains the documentation of
// record.

const CareerDigestMailEnqueueRoute = "/api/v1/career-digest-mails"

// CareerDigestJob is the browser-safe subset of one top Career match. It never
// carries the raw profile, crawler internals, or full job description.
type CareerDigestJob struct {
	Company      string   `json:"company"`
	Title        string   `json:"title"`
	Location     string   `json:"location"`
	URL          string   `json:"url"`
	MatchScore   int      `json:"match_score"`
	MatchReasons []string `json:"match_reasons,omitempty"`
}

// CareerDigestMailEnqueueRequest is what a service caller may submit: the
// recipient is resolved server-side from user_id and never travels on the wire
// from the caller's point of view.
type CareerDigestMailEnqueueRequest struct {
	UserID       string            `json:"user_id"`
	SearchID     string            `json:"search_id"`
	CompletedAt  string            `json:"completed_at"`
	SourceCount  int               `json:"source_count"`
	JobCount     int               `json:"job_count"`
	MatchedCount int               `json:"matched_count"`
	Summary      string            `json:"summary"`
	CareerURL    string            `json:"career_url,omitempty"`
	TopJobs      []CareerDigestJob `json:"top_jobs,omitempty"`
}

// CareerDigestMailEnqueued is the stable, non-disclosing acceptance result.
type CareerDigestMailEnqueued struct {
	Enqueued bool `json:"enqueued"`
}
