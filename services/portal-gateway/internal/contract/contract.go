// Package contract defines Portal Gateway API types that are not generated
// from the public OpenAPI schema.
package contract

import (
	"encoding/json"
	"time"
)

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
	RequestID string          `json:"request_id"`
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
	ID     string  `json:"id"`
	Name   string  `json:"name"`
	Rating float64 `json:"rating"`
	Tier   string  `json:"tier"`
	Campus string  `json:"campus"`
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

// NoticeFeedEnvelope mirrors the Notice owner's bounded snapshot envelope.
// Data is the owner's raw {"items": [...], "generated_at": ...} snapshot,
// forwarded to Portal as-is; the schema is documented in portal-gateway.yaml
// (NoticeFeedEnvelope/NoticeFeed/NoticeFeedItem).
type NoticeFeedEnvelope struct {
	Data      json.RawMessage `json:"data"`
	RequestID string          `json:"request_id"`
}

// ErrorEnvelope is the standard error response. Error is the machine-readable
// code clients may branch on; Message is the user-facing Chinese text.
type ErrorEnvelope struct {
	Error     string `json:"error"`
	Detail    string `json:"detail,omitempty"`
	Message   string `json:"message,omitempty"`
	RequestID string `json:"request_id"`
}
