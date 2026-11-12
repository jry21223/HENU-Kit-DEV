package practice

import (
	"encoding/json"
	"errors"
	"henukit.dev/portal-gateway/internal/trainingcontract"
)

type clozePart struct {
	PartID  string `json:"part_id"`
	Kind    string `json:"kind"`
	BlankID string `json:"blank_id"`
	Text    string `json:"text"`
}

func clozeParts(unit trainingcontract.TrainingClozeUnit) ([]clozePart, error) {
	parts := make([]clozePart, 0, len(unit.Parts))
	for _, part := range unit.Parts {
		raw, err := part.MarshalJSON()
		if err != nil {
			return nil, err
		}
		var value clozePart
		if err := json.Unmarshal(raw, &value); err != nil {
			return nil, err
		}
		parts = append(parts, value)
	}
	return parts, nil
}

func validateClozeSnapshot(snapshot trainingcontract.TrainingSession, unit trainingcontract.TrainingClozeUnit) error {
	questions := map[string]bool{}
	for _, q := range unit.Questions {
		questions[q.QuestionId.String()] = true
	}
	blanks := map[string]bool{}
	bound := map[string]bool{}
	if len(unit.Blanks) != len(unit.Questions) {
		return errors.New("cloze blank membership differs from questions")
	}
	for _, blank := range unit.Blanks {
		id := blank.QuestionId.String()
		if blanks[blank.BlankId] || bound[id] || !questions[id] {
			return errors.New("invalid cloze blank binding")
		}
		blanks[blank.BlankId] = true
		bound[id] = true
	}
	parts, err := clozeParts(unit)
	if err != nil {
		return err
	}
	partIDs := map[string]bool{}
	partBlanks := map[string]bool{}
	for _, part := range parts {
		if partIDs[part.PartID] {
			return errors.New("duplicate cloze passage identity")
		}
		partIDs[part.PartID] = true
		if part.Kind == "blank" {
			if !blanks[part.BlankID] || partBlanks[part.BlankID] {
				return errors.New("invalid cloze passage blank")
			}
			partBlanks[part.BlankID] = true
		}
	}
	if len(partBlanks) != len(blanks) {
		return errors.New("missing cloze passage blank")
	}
	descriptors := map[string]int{}
	for _, hint := range unit.Hints {
		if descriptors[hint.HintId] != 0 {
			return errors.New("duplicate cloze hint")
		}
		descriptors[hint.HintId] = hint.LevelCount
	}
	levels := map[string]int{}
	disclosures := map[string]bool{}
	assisted := map[string]bool{}
	for i, hint := range snapshot.HintDisclosures {
		if hint.Level != levels[hint.HintId]+1 || hint.Level > descriptors[hint.HintId] || disclosures[hint.DisclosureId.String()] {
			return errors.New("invalid cloze hint sequence")
		}
		if i > 0 {
			prior := snapshot.HintDisclosures[i-1]
			if hint.DisclosedAt.Before(prior.DisclosedAt) || hint.DisclosedAt.Equal(prior.DisclosedAt) && hint.DisclosureId.String() <= prior.DisclosureId.String() {
				return errors.New("invalid cloze hint ordering")
			}
		}
		levels[hint.HintId] = hint.Level
		disclosures[hint.DisclosureId.String()] = true
		affected := map[string]bool{}
		for _, id := range hint.AffectedQuestionIds {
			key := id.String()
			if !questions[key] || affected[key] {
				return errors.New("invalid cloze assisted membership")
			}
			affected[key] = true
			assisted[key] = true
		}
		unsubmitted := make(map[string]bool, len(questions))
		for id := range questions {
			unsubmitted[id] = true
		}
		for _, receipt := range snapshot.Receipts {
			delete(unsubmitted, receipt.QuestionId.String())
		}
		for id := range unsubmitted {
			if !affected[id] {
				return errors.New("cloze hint omits an unsubmitted pinned question")
			}
		}
		for _, receipt := range snapshot.Receipts {
			included := affected[receipt.QuestionId.String()]
			if receipt.SubmittedAt.Before(hint.DisclosedAt) && included || receipt.SubmittedAt.After(hint.DisclosedAt) && !included {
				return errors.New("cloze hint contradicts saved answer ordering")
			}
		}
	}
	for _, receipt := range snapshot.Receipts {
		if receipt.Assisted != assisted[receipt.QuestionId.String()] {
			return errors.New("cloze assistance differs from disclosure facts")
		}
	}
	return nil
}

func validatePinnedClozeReview(unit trainingcontract.TrainingClozeUnit, review trainingcontract.TrainingClozeReview) error {
	parts, err := clozeParts(unit)
	if err != nil {
		return err
	}
	partIDs := map[string]bool{}
	for _, part := range parts {
		partIDs[part.PartID] = true
	}
	answers := map[string]string{}
	for i, q := range unit.Questions {
		if i >= len(review.Questions) || review.Questions[i].ExpectedAnswer < 0 || review.Questions[i].ExpectedAnswer >= len(q.Options) {
			return errors.New("invalid cloze review key")
		}
		answers[q.QuestionId.String()] = q.Options[review.Questions[i].ExpectedAnswer]
	}
	blankAnswers := map[string]string{}
	for _, blank := range unit.Blanks {
		blankAnswers[blank.BlankId] = answers[blank.QuestionId.String()]
	}
	for _, question := range review.Questions {
		evidence, err := question.Evidence.AsTrainingClozeEvidence()
		if err != nil || evidence.Kind != "cloze" {
			return errors.New("invalid cloze evidence kind")
		}
		seen := map[string]bool{}
		for _, id := range evidence.PartIds {
			if !partIDs[id] || seen[id] {
				return errors.New("cloze evidence differs from pinned passage")
			}
			seen[id] = true
		}
	}
	for _, disclosure := range review.Layers {
		layer, err := disclosure.Content.AsTrainingClozeLayer()
		if err != nil || layer.Kind != "cloze" || len(layer.Parts) != len(parts) {
			return errors.New("cloze layer differs from pinned passage")
		}
		for i, text := range layer.Parts {
			part := parts[i]
			if text.PartId != part.PartID {
				return errors.New("cloze layer part identity differs from pinned passage")
			}
			if layer.Layer == "english" {
				expected := part.Text
				if part.Kind == "blank" {
					expected = blankAnswers[part.BlankID]
				}
				if text.Text != expected {
					return errors.New("filled cloze differs from pinned context and key")
				}
			}
		}
	}
	return nil
}
