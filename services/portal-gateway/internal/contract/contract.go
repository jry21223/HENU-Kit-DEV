// Package contract defines Portal Gateway API types that are not generated
// from the public OpenAPI schema.
//
// cmd/contractgen emits only the PortalSession and practice-stats schemas
// (portal_session.generated.go). The Notice snapshot types below are
// handwritten but mirror the schemas documented in portal-gateway.yaml
// (NoticeFeedEnvelope/NoticeFeed/NoticeFeedItem/NoticeSource), which the
// Gateway forwards to Portal as raw JSON.
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

// NoticeFeed is the Notice owner's bounded snapshot. Items and GeneratedAt
// stay raw JSON because the Gateway validates only that items is present
// (non-null) and that each item's lifecycle state is "distributed"; every
// other field is the owner's shape, forwarded unchanged. Documented in
// portal-gateway.yaml (NoticeFeed).
type NoticeFeed struct {
	Items       []json.RawMessage `json:"items"`
	GeneratedAt json.RawMessage   `json:"generated_at"`
}

// NoticeItemLifecycle is the per-item lifecycle subset the Gateway inspects
// to filter the snapshot: only notices in the distributed state may leave
// the Gateway.
type NoticeItemLifecycle struct {
	State string `json:"state"`
}

// ErrorEnvelope is the standard error response. Error is the machine-readable
// code clients may branch on; Message is the user-facing Chinese text.
type ErrorEnvelope struct {
	Error     string `json:"error"`
	Detail    string `json:"detail,omitempty"`
	Message   string `json:"message,omitempty"`
	RequestID string `json:"request_id"`
}
