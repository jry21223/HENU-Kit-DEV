package quizcraft

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	tc "henukit.dev/quizcraft/internal/trainingcontract"
	"strings"
)

// Common transaction operations use the exact resolved membership. Kind-specific
// private content remains in its closed contract, never in a learner projection.
type trainingVersion struct {
	Unit            trainingUnit
	ReviewQuestions []tc.TrainingQuestionReview
	Listening       *tc.TrainingListeningVersion
	Cloze           *tc.TrainingClozeVersion
}

// Shared identity and membership have no JSON representation: learner responses
// use the appropriate closed unit type below.
type trainingUnit struct {
	UnitId, UnitVersionId, BankId, BankVersionId uuid.UUID
	ContentSha256                                string
	Title                                        string
	Kind                                         tc.TrainingKind
	Attributions                                 []tc.TrainingAttribution
	Questions                                    []tc.TrainingQuestion
}

func decodeTrainingVersion(raw []byte) (trainingVersion, error) {
	var v trainingVersion
	if err := tc.Validate("TrainingUnitVersion", raw); err != nil {
		return v, err
	}
	var kind struct {
		Unit struct {
			Kind string `json:"kind"`
		} `json:"unit"`
	}
	_ = json.Unmarshal(raw, &kind)
	if kind.Unit.Kind == "listening" {
		var listening tc.TrainingListeningVersion
		if err := json.Unmarshal(raw, &listening); err != nil {
			return v, err
		}
		u := listening.Unit
		v.Listening = &listening
		v.ReviewQuestions = listening.ReviewQuestions
		v.Unit = trainingUnit{UnitId: u.UnitId, UnitVersionId: u.UnitVersionId, BankId: u.BankId, BankVersionId: u.BankVersionId, ContentSha256: u.ContentSha256, Title: u.Title, Kind: "listening", Attributions: u.Attributions, Questions: u.Questions}
	} else {
		var cloze tc.TrainingClozeVersion
		if err := json.Unmarshal(raw, &cloze); err != nil {
			return v, err
		}
		u := cloze.Unit
		v.Cloze = &cloze
		v.ReviewQuestions = cloze.ReviewQuestions
		v.Unit = trainingUnit{UnitId: u.UnitId, UnitVersionId: u.UnitVersionId, BankId: u.BankId, BankVersionId: u.BankVersionId, ContentSha256: u.ContentSha256, Title: u.Title, Kind: "cloze", Attributions: u.Attributions, Questions: u.Questions}
	}
	return v, nil
}

func persistClozeVersion(ctx context.Context, tx pgx.Tx, report ImportReport, authoring tc.TrainingAuthoring, questions []normalizedQuestion) error {
	invalid := func(message string) error { return workshopHTTPError{422, "invalid_training", message} }
	source, err := authoring.AsTrainingClozeAuthoring()
	if err != nil || source.Kind != "cloze" {
		return invalid("Invalid cloze source")
	}
	if len(questions) < 1 || len(questions) > 20 || len(source.QuestionReviews) != len(questions) || len(source.Blanks) != len(questions) {
		return invalid("Cloze blanks and review must cover every question")
	}
	components := map[string]bool{}
	for _, a := range source.Attributions {
		components[string(a.Component)] = true
		if !strings.HasPrefix(a.SourceUrl, "https://") || !strings.HasPrefix(a.LicenseUrl, "https://") {
			return invalid("Attribution requires HTTPS sources")
		}
	}
	for _, component := range []string{"text", "translation"} {
		if !components[component] {
			return invalid("Every authored component requires attribution")
		}
	}
	bankID := uuid.MustParse(report.BankID)
	bankVersion := uuid.MustParse(report.BankVersionID)
	var existing string
	if err := tx.QueryRow(ctx, `SELECT source_unit_id FROM quizcraft_training_units WHERE bank_id=$1`, bankID).Scan(&existing); err == nil && existing != source.SourceUnitId {
		return invalid("A training bank retains its stable source unit identity")
	} else if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	unitID := stableID(bankID, "training-unit:"+source.SourceUnitId)
	version := tc.TrainingClozeVersion{Unit: tc.TrainingClozeUnit{UnitId: unitID, BankId: bankID, BankVersionId: bankVersion, Title: source.Title, Kind: "cloze", Attributions: source.Attributions, Parts: source.Parts, Blanks: []tc.TrainingBlank{}, Hints: []tc.TrainingHintDescriptor{}, Questions: []tc.TrainingQuestion{}}, Hints: source.Hints, Layers: source.Layers, ReviewQuestions: []tc.TrainingQuestionReview{}}
	qBySource := map[string]normalizedQuestion{}
	reviews := map[string]tc.TrainingSourceReview{}
	for _, r := range source.QuestionReviews {
		if _, ok := reviews[r.SourceQuestionId]; ok {
			return invalid("Duplicate question review")
		}
		reviews[r.SourceQuestionId] = r
	}
	partIDs := map[string]bool{}
	blankParts := map[string]bool{}
	for _, part := range source.Parts {
		raw, _ := part.MarshalJSON()
		var data map[string]string
		_ = json.Unmarshal(raw, &data)
		partID, kind, blankID := data["part_id"], data["kind"], data["blank_id"]
		if partIDs[partID] {
			return invalid("Duplicate passage part")
		}
		partIDs[partID] = true
		if kind == "blank" {
			if blankParts[blankID] {
				return invalid("Duplicate blank in passage")
			}
			blankParts[blankID] = true
		}
	}
	for _, q := range questions {
		r, ok := reviews[q.SourceID]
		if !ok || q.Type != "single" {
			return invalid("Every cloze question needs its exact single-choice review")
		}
		evidence, err := r.Evidence.AsTrainingClozeEvidence()
		if err != nil || evidence.Kind != "cloze" {
			return invalid("Cloze evidence is required")
		}
		seen := map[string]bool{}
		for _, id := range evidence.PartIds {
			if !partIDs[id] || seen[id] {
				return invalid("Evidence must reference unique known passage parts")
			}
			seen[id] = true
		}
		options, ok := q.Options.([]any)
		if !ok {
			return invalid("Invalid options")
		}
		texts := []string{}
		for _, option := range options {
			text, ok := option.(string)
			if !ok || strings.TrimSpace(text) == "" {
				return invalid("Options must be nonempty text")
			}
			texts = append(texts, text)
		}
		key, ok := q.Answer.(float64)
		if !ok || int(key) < 0 || int(key) >= len(texts) || float64(int(key)) != key {
			return invalid("Answer must be a valid option index")
		}
		qBySource[q.SourceID] = q
		version.Unit.Questions = append(version.Unit.Questions, tc.TrainingQuestion{QuestionId: q.QuestionID, QuestionVersionId: q.VersionID, Content: q.Content, Options: texts})
		version.ReviewQuestions = append(version.ReviewQuestions, tc.TrainingQuestionReview{QuestionId: q.QuestionID, QuestionVersionId: q.VersionID, ExpectedAnswer: int(key), Analysis: q.Analysis, ErrorCategory: tc.TrainingQuestionReviewErrorCategory(r.ErrorCategory), Evidence: r.Evidence})
	}
	blankIDs := map[string]bool{}
	blankQuestions := map[string]bool{}
	blankAnswers := map[string]string{}
	for _, blank := range source.Blanks {
		q, ok := qBySource[blank.SourceQuestionId]
		if !ok || blankQuestions[blank.SourceQuestionId] || blankIDs[blank.BlankId] || !blankParts[blank.BlankId] {
			return invalid("Blanks must map one-to-one to questions and passage")
		}
		blankIDs[blank.BlankId] = true
		blankQuestions[blank.SourceQuestionId] = true
		version.Unit.Blanks = append(version.Unit.Blanks, tc.TrainingBlank{BlankId: blank.BlankId, QuestionId: q.QuestionID})
		blankAnswers[blank.BlankId] = q.Options.([]any)[int(q.Answer.(float64))].(string)
	}
	if len(blankParts) != len(blankIDs) {
		return invalid("Every passage blank needs an exact question")
	}
	hints := map[string]bool{}
	for _, hint := range source.Hints {
		if hints[hint.HintId] {
			return invalid("Duplicate hint identity")
		}
		hints[hint.HintId] = true
		version.Unit.Hints = append(version.Unit.Hints, tc.TrainingHintDescriptor{HintId: hint.HintId, LevelCount: len(hint.Levels)})
	}
	if len(source.Layers) != 2 {
		return invalid("English then Chinese layers are required")
	}
	for i, layer := range source.Layers {
		if string(layer.Layer) != []string{"english", "chinese"}[i] || len(layer.Parts) != len(source.Parts) {
			return invalid("Language layers must cover every stable passage part in order")
		}
		for j, text := range layer.Parts {
			raw, _ := source.Parts[j].MarshalJSON()
			var p map[string]string
			_ = json.Unmarshal(raw, &p)
			if text.PartId != p["part_id"] {
				return invalid("Language layer passage identity mismatch")
			}
			if i == 0 {
				expected := p["text"]
				if p["kind"] == "blank" {
					expected = blankAnswers[p["blank_id"]]
				}
				if text.Text != expected {
					return invalid("Filled English passage must match its exact authored answers and context")
				}
			}
		}
	}
	encoded, _ := json.Marshal(version)
	fingerprint := trainingDocumentFingerprint(encoded)
	version.Unit.UnitVersionId = stableID(unitID, "version:"+fingerprint)
	version.Unit.ContentSha256 = fingerprint
	encoded, _ = json.Marshal(version)
	if err := tc.Validate("TrainingClozeVersion", encoded); err != nil {
		return invalid("Resolved cloze violates its closed contract")
	}
	return persistTrainingDocument(ctx, tx, source.SourceUnitId, encoded)
}

func trainingLayerName(disclosure tc.TrainingLayerDisclosure) (string, error) {
	raw, err := disclosure.Content.MarshalJSON()
	if err != nil {
		return "", err
	}
	var layer struct {
		Layer string `json:"layer"`
	}
	err = json.Unmarshal(raw, &layer)
	return layer.Layer, err
}
func trainingHints(ctx context.Context, tx pgx.Tx, session uuid.UUID, version tc.TrainingClozeVersion) ([]tc.TrainingHintDisclosure, error) {
	result := []tc.TrainingHintDisclosure{}
	rows, err := tx.Query(ctx, `SELECT id,target_id,level,affected_question_ids,disclosed_at FROM quizcraft_training_disclosures WHERE session_id=$1 AND kind='hint' ORDER BY disclosed_at,id`, session)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var hint tc.TrainingHintDisclosure
		if err := rows.Scan(&hint.DisclosureId, &hint.HintId, &hint.Level, &hint.AffectedQuestionIds, &hint.DisclosedAt); err != nil {
			return nil, err
		}
		found := false
		for _, authored := range version.Hints {
			if authored.HintId == hint.HintId && hint.Level >= 1 && hint.Level <= len(authored.Levels) {
				hint.Text = authored.Levels[hint.Level-1]
				found = true
				break
			}
		}
		if !found {
			return nil, errors.New("persisted hint missing from pinned version")
		}
		hint.DisclosedAt = hint.DisclosedAt.UTC()
		result = append(result, hint)
	}
	return result, rows.Err()
}

// Both authored kinds share one canonical semantic identity and immutable write
// path; their closed validation remains in their respective authoring resolvers.
func trainingDocumentFingerprint(encoded []byte) string {
	var canonical map[string]any
	_ = json.Unmarshal(encoded, &canonical)
	unit := canonical["unit"].(map[string]any)
	delete(unit, "unit_version_id")
	delete(unit, "content_sha256")
	canonicalJSON, _ := json.Marshal(canonical)
	return hash(canonicalJSON)
}
func persistTrainingDocument(ctx context.Context, tx pgx.Tx, sourceID string, encoded []byte) error {
	version, err := decodeTrainingVersion(encoded)
	if err != nil {
		return err
	}
	u := version.Unit
	if _, err := tx.Exec(ctx, `INSERT INTO quizcraft_training_units(id,bank_id,source_unit_id) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, u.UnitId, u.BankId, sourceID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO quizcraft_training_unit_versions(id,unit_id,bank_id,bank_version_id,content_sha256,document) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT DO NOTHING`, u.UnitVersionId, u.UnitId, u.BankId, u.BankVersionId, u.ContentSha256, encoded); err != nil {
		return err
	}
	for i, q := range u.Questions {
		if _, err := tx.Exec(ctx, `INSERT INTO quizcraft_training_members(unit_version_id,bank_id,bank_version_id,question_id,question_version_id,position) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT DO NOTHING`, u.UnitVersionId, u.BankId, u.BankVersionId, q.QuestionId, q.QuestionVersionId, i+1); err != nil {
			return err
		}
	}
	return nil
}
