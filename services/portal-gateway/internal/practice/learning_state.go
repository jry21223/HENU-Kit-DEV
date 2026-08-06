package practice

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// GetPortalLearningStatePath is the signed-in persistent learning-state read
// on QuizCraft Core. It is intentionally not emitted by the curated contract
// generator: like personal stats and favorites it is an actor-bound read, and
// the generator only emits the operations it is explicitly programmed to.
const GetPortalLearningStatePath = "/api/v1/learning-state"

// LearningStateEnvelope is one signed-in Portal user's fact-derived
// per-question learning state. It never represents a mock response.
type LearningStateEnvelope struct {
	RequestID string              `json:"request_id"`
	Data      []LearningStateItem `json:"data"`
}

// LearningStateItem is one question's derived facts: the wrong mark and the
// immutable attempt/correct counts that produced it. It carries no question
// content, so callers must display these facts without fabricating a stem.
type LearningStateItem struct {
	BankID            string `json:"bank_id"`
	QuestionID        string `json:"question_id"`
	QuestionVersionID string `json:"question_version_id"`
	Wrong             bool   `json:"wrong"`
	AttemptCount      int64  `json:"attempt_count"`
	CorrectCount      int64  `json:"correct_count"`
	UpdatedAt         string `json:"updated_at"`
}

// LearningState reads one signed-in Portal user's persistent learning state
// through the six-part actor-bound read contract. Like PersonalStats and
// FavoritesOverview it is an account-bound read: Core rejects the request
// unless the actor UUID is the sixth HMAC canonical line.
func (c *Client) LearningState(ctx context.Context, actorUserID, requestID string) (LearningStateEnvelope, error) {
	if c == nil || c.signer == nil || c.httpClient == nil || !validUUID(actorUserID) || strings.TrimSpace(requestID) == "" {
		return LearningStateEnvelope{}, ErrStatsUnauthorized
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+GetPortalLearningStatePath, nil)
	if err != nil {
		return LearningStateEnvelope{}, ErrStatsUnavailable
	}
	body, err := c.actorBoundLearningStateRead(ctx, req, actorUserID, requestID)
	if err != nil {
		return LearningStateEnvelope{}, err
	}
	defer body.Close()
	var result LearningStateEnvelope
	decoder := json.NewDecoder(io.LimitReader(body, 2<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return LearningStateEnvelope{}, fmt.Errorf("learning state decode: %w", ErrInvalidStats)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return LearningStateEnvelope{}, fmt.Errorf("learning state decode: %w", ErrInvalidStats)
	}
	if err := validateLearningState(result); err != nil {
		return LearningStateEnvelope{}, err
	}
	return result, nil
}

// actorBoundLearningStateRead performs the six-part signed GET and returns the
// response body; the caller decodes and validates the typed envelope.
func (c *Client) actorBoundLearningStateRead(ctx context.Context, req *http.Request, actorUserID, requestID string) (io.ReadCloser, error) {
	req.Header.Set("X-Request-Id", requestID)
	req.Header.Set("X-Permission-Code", PortalReadPermission)
	req.Header.Set("X-Scope-Kind", "product")
	req.Header.Set("X-Product-Code", "quizcraft")
	if err := c.signer.SignWithActor(req, actorUserID); err != nil {
		return nil, fmt.Errorf("portal learning state sign: %w", ErrStatsUnavailable)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("portal learning state request: %w", ErrStatsUnavailable)
	}
	switch resp.StatusCode {
	case http.StatusOK:
		return resp.Body, nil
	case http.StatusUnauthorized, http.StatusForbidden:
		resp.Body.Close()
		// Portal Gateway has already checked the browser's session and live
		// permission. A Core 401/403 here is an internal service-auth failure,
		// never evidence that the browser should be asked to sign in again.
		return nil, fmt.Errorf("portal learning state service authentication: %w", ErrStatsUnavailable)
	default:
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
		resp.Body.Close()
		return nil, fmt.Errorf("portal learning state %d: %w", resp.StatusCode, ErrStatsUnavailable)
	}
}

func validateLearningState(result LearningStateEnvelope) error {
	if strings.TrimSpace(result.RequestID) == "" {
		return fmt.Errorf("learning state request id: %w", ErrInvalidStats)
	}
	for _, item := range result.Data {
		if !validUUID(item.BankID) || !validUUID(item.QuestionID) || !validUUID(item.QuestionVersionID) || item.AttemptCount < 1 || item.CorrectCount < 0 || item.CorrectCount > item.AttemptCount || strings.TrimSpace(item.UpdatedAt) == "" {
			return fmt.Errorf("learning state item: %w", ErrInvalidStats)
		}
	}
	return nil
}
