package practice

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// GetPortalLearningStatePath is the signed-in persistent learning-state read
// on QuizCraft Core. The operation is not yet on the curated
// quizcraftcontractgen emit whitelist, so this constant is hand-written
// alongside the generated contract; cutover should fold it into the
// generator's whitelist.
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
	body, err := c.actorBoundRead(ctx, GetPortalLearningStatePath, actorUserID, requestID, PortalReadPermission)
	if err != nil {
		return LearningStateEnvelope{}, err
	}
	defer body.Close()
	var result LearningStateEnvelope
	decoder := json.NewDecoder(io.LimitReader(body, 2<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return LearningStateEnvelope{}, fmt.Errorf("learning state decode: %w", ErrActorReadInvalid)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return LearningStateEnvelope{}, fmt.Errorf("learning state decode: %w", ErrActorReadInvalid)
	}
	if err := validateLearningState(result); err != nil {
		return LearningStateEnvelope{}, err
	}
	return result, nil
}

func validateLearningState(result LearningStateEnvelope) error {
	if strings.TrimSpace(result.RequestID) == "" {
		return fmt.Errorf("learning state request id: %w", ErrActorReadInvalid)
	}
	for _, item := range result.Data {
		if !validUUID(item.BankID) || !validUUID(item.QuestionID) || !validUUID(item.QuestionVersionID) || item.AttemptCount < 1 || item.CorrectCount < 0 || item.CorrectCount > item.AttemptCount || strings.TrimSpace(item.UpdatedAt) == "" {
			return fmt.Errorf("learning state item: %w", ErrActorReadInvalid)
		}
	}
	return nil
}
