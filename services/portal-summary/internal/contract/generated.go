// Code generated from portal-summary.yaml and console-gateway.yaml (SHA256 8726234818488631d1b64615813b8e4c34eed6e710662a0b0fd6de94675cad05); DO NOT EDIT.
package contract

import (
	"errors"
	"regexp"
	"time"
	"unicode/utf8"
)

const (
	ContractSHA256             = "8726234818488631d1b64615813b8e4c34eed6e710662a0b0fd6de94675cad05"
	ErrorDependencyUnavailable = "DEPENDENCY_UNAVAILABLE"
	ErrorInvalidOwnerSummary   = "INVALID_OWNER_SUMMARY"
	ErrorInvalidServiceAuth    = "INVALID_SERVICE_AUTH"
	ErrorReplayDetected        = "REPLAY_DETECTED"
)

type Metric struct {
	Label string `json:"label"`
	Value string `json:"value"`
	Hint  string `json:"hint,omitempty"`
}
type PortalSummary struct {
	ID            string    `json:"id"`
	Status        string    `json:"status"`
	Metrics       []Metric  `json:"metrics"`
	StatusMessage string    `json:"status_message"`
	AsOf          time.Time `json:"as_of"`
	RequestID     string    `json:"request_id"`
}
type PortalSummaryEnvelope struct {
	Data      PortalSummary `json:"data"`
	RequestID string        `json:"request_id"`
}

var requestIDPattern = regexp.MustCompile("^req_[A-Za-z0-9_-]+$")
var liveStatuses = map[string]bool{"ok": true, "partial": true}
var errorStatuses = map[string]int{"DEPENDENCY_UNAVAILABLE": 503, "INVALID_OWNER_SUMMARY": 503, "INVALID_SERVICE_AUTH": 401, "REPLAY_DETECTED": 409}

func ValidErrorStatus(status int, code string) bool { return errorStatuses[code] == status }

func ValidatePortalSummaryEnvelope(value PortalSummaryEnvelope) error {
	if value.Data.ID != "portal" || !liveStatuses[value.Data.Status] || len(value.Data.Metrics) != 8 || value.Data.AsOf.IsZero() || value.Data.RequestID != value.RequestID {
		return errors.New("portal summary identity, status, metrics, time, or trace is invalid")
	}
	if utf8.RuneCountInString(value.RequestID) > 120 || !requestIDPattern.MatchString(value.RequestID) || utf8.RuneCountInString(value.Data.StatusMessage) > 240 {
		return errors.New("portal summary trace or message is invalid")
	}
	for _, metric := range value.Data.Metrics {
		if metric.Label == "" || metric.Value == "" || utf8.RuneCountInString(metric.Label) > 40 || utf8.RuneCountInString(metric.Value) > 80 || utf8.RuneCountInString(metric.Hint) > 120 {
			return errors.New("portal summary metric is invalid")
		}
	}
	return nil
}
