// Package contract defines Portal Gateway API types.
// Generated from packages/api-contracts/openapi/portal-gateway.yaml
package contract

import "time"

// PortalSession is the authenticated user context returned to the browser.
type PortalSession struct {
	UserID    string    `json:"user_id"`
	ExpiresAt time.Time `json:"expires_at"`
}

// ModuleSummary is a read-only product summary for Portal's module cards.
type ModuleSummary struct {
	ID        string         `json:"id"`
	Status    string         `json:"status"` // ok | empty | stale | unavailable
	Metrics   map[string]any `json:"metrics,omitempty"`
	AsOf      *time.Time     `json:"as_of,omitempty"`
	RequestID string         `json:"request_id"`
}

// LibraryCoursesResponse represents available courses.
type LibraryCoursesResponse struct {
	Courses   []CourseSummary `json:"courses"`
	RequestID string         `json:"request_id"`
}

type CourseSummary struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Subject  string `json:"subject"`
	Material int    `json:"material_count"`
}

// FoodVenuesResponse represents campus food venues.
type FoodVenuesResponse struct {
	Campus    string         `json:"campus"`
	Venues    []VenueSummary `json:"venues"`
	RequestID string         `json:"request_id"`
}

type VenueSummary struct {
	ID      string  `json:"id"`
	Name    string  `json:"name"`
	Rating  float64 `json:"rating"`
	Tier    string  `json:"tier"`
	Campus  string  `json:"campus"`
}

// PracticeBanksResponse represents available question banks.
type PracticeBanksResponse struct {
	Banks     []BankSummary `json:"banks"`
	RequestID string        `json:"request_id"`
}

type BankSummary struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Subject    string `json:"subject"`
	QuestionCT int    `json:"question_count"`
}

// NoticeListResponse represents published notices.
type NoticeListResponse struct {
	Notices   []NoticeSummary `json:"notices"`
	RequestID string          `json:"request_id"`
}

type NoticeSummary struct {
	ID        string     `json:"id"`
	Title     string     `json:"title"`
	Source    string     `json:"source"`
	Published time.Time  `json:"published_at"`
}

// ErrorEnvelope is the standard error response.
type ErrorEnvelope struct {
	Error   string `json:"error"`
	Detail  string `json:"detail,omitempty"`
	RequestID string `json:"request_id"`
}
