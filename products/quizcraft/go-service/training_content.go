package quizcraft

import (
	"bytes"
	"context"
	"embed"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	tc "henukit.dev/quizcraft/internal/trainingcontract"
)

// Public, licensed immutable media only. Private authoring is never embedded.
//
//go:embed media/*.mp3
var trainingMedia embed.FS

func listeningAsset(media tc.TrainingAuthoredMedia) ([]byte, error) {
	raw, err := trainingMedia.ReadFile("media/" + media.Sha256 + ".mp3")
	if err != nil || hash(raw) != media.Sha256 || media.ContentType != "audio/mpeg" {
		return nil, errors.New("verified MP3 asset is unavailable")
	}
	duration, err := mp3DurationMS(raw)
	if err != nil || duration != media.DurationMs {
		return nil, errors.New("MP3 duration differs from authored duration")
	}
	return raw, nil
}

// Read the MPEG1 Layer III Xing/Info frame and gapless delay/padding. Unsupported
// media fails closed: an operator must supply verifiable metadata, never estimates.
func mp3DurationMS(raw []byte) (int, error) {
	start := 0
	if len(raw) > 10 && string(raw[:3]) == "ID3" {
		start = 10 + int(raw[6]&127)<<21 + int(raw[7]&127)<<14 + int(raw[8]&127)<<7 + int(raw[9]&127)
	}
	if start+4 > len(raw) {
		return 0, errors.New("missing MPEG header")
	}
	header := binary.BigEndian.Uint32(raw[start : start+4])
	if header&0xfffe0000 != 0xfffa0000 {
		return 0, errors.New("unsupported MPEG version/layer")
	}
	rates := []int{44100, 48000, 32000}
	rateIndex := (header >> 10) & 3
	if rateIndex == 3 {
		return 0, errors.New("invalid sample rate")
	}
	side := 32
	if (header>>6)&3 == 3 {
		side = 17
	}
	crc := 0
	if (header>>16)&1 == 0 {
		crc = 2
	}
	offset := start + 4 + side + crc
	if offset+12 > len(raw) || (string(raw[offset:offset+4]) != "Info" && string(raw[offset:offset+4]) != "Xing") {
		return 0, errors.New("missing MP3 timing index")
	}
	flags := binary.BigEndian.Uint32(raw[offset+4 : offset+8])
	if flags&1 == 0 {
		return 0, errors.New("missing frame count")
	}
	samples := int(binary.BigEndian.Uint32(raw[offset+8:offset+12])) * 1152
	encoder := offset + 8
	for _, flag := range []uint32{1, 2, 4, 8} {
		if flags&flag != 0 {
			if flag == 4 {
				encoder += 100
			} else {
				encoder += 4
			}
		}
	}
	if encoder+24 > len(raw) {
		return 0, errors.New("missing gapless metadata")
	}
	delay := int(raw[encoder+21])<<4 | int(raw[encoder+22]>>4)
	padding := int(raw[encoder+22]&15)<<8 | int(raw[encoder+23])
	samples -= delay + padding
	if samples <= 0 {
		return 0, errors.New("invalid audio samples")
	}
	return (samples*1000 + rates[rateIndex]/2) / rates[rateIndex], nil
}

func serveTrainingMedia(writer http.ResponseWriter, request *http.Request) {
	digest := chi.URLParam(request, "digest")
	if !sha256Pattern.MatchString(digest) {
		http.NotFound(writer, request)
		return
	}
	raw, err := trainingMedia.ReadFile("media/" + digest + ".mp3")
	if err != nil || hash(raw) != digest {
		http.NotFound(writer, request)
		return
	}
	writer.Header().Set("Content-Type", "audio/mpeg")
	writer.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	writer.Header().Set("ETag", `"`+digest+`"`)
	http.ServeContent(writer, request, digest+".mp3", time.Time{}, bytes.NewReader(raw))
}

func guardTrainingImport(ctx context.Context, tx pgx.Tx, bankID uuid.UUID, authoring *tc.TrainingAuthoring) error {
	var training, ordinary bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM quizcraft_training_units WHERE bank_id=$1),EXISTS(SELECT 1 FROM quizcraft_bank_versions v WHERE v.bank_id=$1 AND NOT EXISTS(SELECT 1 FROM quizcraft_training_unit_versions t WHERE t.bank_version_id=v.id))`, bankID).Scan(&training, &ordinary); err != nil {
		return err
	}
	if (training && authoring == nil) || (authoring != nil && ordinary) {
		return workshopHTTPError{409, "training_binding_required", "Training banks require their immutable training source and cannot convert ordinary content"}
	}
	return nil
}

func persistListeningVersion(ctx context.Context, tx pgx.Tx, report ImportReport, authoring tc.TrainingAuthoring, questions []normalizedQuestion) error {
	raw, _ := json.Marshal(authoring)
	invalid := func(message string) error { return workshopHTTPError{422, "invalid_training", message} }
	if err := tc.Validate("TrainingAuthoring", raw); err != nil {
		return invalid("Training source violates its closed contract")
	}
	source, err := authoring.AsTrainingListeningAuthoring()
	if err != nil || source.Kind != "listening" {
		return invalid("Only listening import is available")
	}
	if _, err = listeningAsset(source.Media); err != nil {
		return invalid(err.Error())
	}
	if len(questions) < 1 || len(questions) > 20 || len(source.QuestionReviews) != len(questions) {
		return invalid("Training membership/review must cover every question")
	}
	components := map[string]bool{}
	for _, a := range source.Attributions {
		components[string(a.Component)] = true
		if !strings.HasPrefix(a.SourceUrl, "https://") || !strings.HasPrefix(a.LicenseUrl, "https://") {
			return invalid("Attribution requires HTTPS sources")
		}
	}
	for _, name := range []string{"audio", "text", "translation"} {
		if !components[name] {
			return invalid("Every authored component requires attribution")
		}
	}
	segments := map[string]bool{}
	end := 0
	for _, segment := range source.Segments {
		if segments[segment.SegmentId] || segment.StartMs < end || segment.EndMs <= segment.StartMs || segment.EndMs > source.Media.DurationMs {
			return invalid("Sentence ranges must be unique, ordered and within actual audio")
		}
		segments[segment.SegmentId] = true
		end = segment.EndMs
	}
	if len(source.Layers) != 2 {
		return invalid("English then Chinese layers are required")
	}
	for index, layer := range source.Layers {
		if string(layer.Layer) != []string{"english", "chinese"}[index] || len(layer.Segments) != len(segments) {
			return invalid("Language layers must cover all sentence IDs in order")
		}
		for i, text := range layer.Segments {
			if text.SegmentId != source.Segments[i].SegmentId {
				return invalid("Language layer sentence identity mismatch")
			}
		}
	}
	bankID := uuid.MustParse(report.BankID)
	var existingSource string
	if err := tx.QueryRow(ctx, `SELECT source_unit_id FROM quizcraft_training_units WHERE bank_id=$1`, bankID).Scan(&existingSource); err == nil && existingSource != source.SourceUnitId {
		return invalid("A training bank retains its stable source unit identity")
	} else if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	bankVersion := uuid.MustParse(report.BankVersionID)
	unitID := stableID(bankID, "training-unit:"+source.SourceUnitId)
	version := tc.TrainingListeningVersion{Unit: tc.TrainingListeningUnit{UnitId: unitID, BankId: bankID, BankVersionId: bankVersion, Title: source.Title, Kind: "listening", Attributions: source.Attributions, Questions: []tc.TrainingQuestion{}, Media: tc.TrainingMedia{AssetId: stableID(quizcraftNamespace, "training-media:"+source.Media.Sha256), Sha256: source.Media.Sha256, ContentType: "audio/mpeg", DurationMs: source.Media.DurationMs, Url: "/practice-media/" + source.Media.Sha256 + ".mp3"}}, Segments: source.Segments, Layers: source.Layers, ReviewQuestions: []tc.TrainingQuestionReview{}}
	reviews := map[string]tc.TrainingSourceReview{}
	for _, r := range source.QuestionReviews {
		if _, exists := reviews[r.SourceQuestionId]; exists {
			return invalid("Duplicate question review")
		}
		reviews[r.SourceQuestionId] = r
	}
	for _, q := range questions {
		r, found := reviews[q.SourceID]
		if !found || q.Type != "single" {
			return invalid("Each single-choice question needs its exact source review")
		}
		evidence, err := r.Evidence.AsTrainingListeningEvidence()
		if err != nil || evidence.Kind != "listening" {
			return invalid("Listening evidence is required")
		}
		for _, id := range evidence.SegmentIds {
			if !segments[id] {
				return invalid("Evidence refers to an unknown sentence")
			}
		}
		options, ok := q.Options.([]any)
		if !ok {
			return invalid("Invalid options")
		}
		texts := []string{}
		for _, o := range options {
			v, ok := o.(string)
			if !ok || strings.TrimSpace(v) == "" {
				return invalid("Options must be nonempty text")
			}
			texts = append(texts, v)
		}
		key, ok := q.Answer.(float64)
		if !ok || int(key) < 0 || int(key) >= len(texts) || float64(int(key)) != key {
			return invalid("Answer must be a valid option index")
		}
		version.Unit.Questions = append(version.Unit.Questions, tc.TrainingQuestion{QuestionId: q.QuestionID, QuestionVersionId: q.VersionID, Content: q.Content, Options: texts})
		version.ReviewQuestions = append(version.ReviewQuestions, tc.TrainingQuestionReview{QuestionId: q.QuestionID, QuestionVersionId: q.VersionID, ExpectedAnswer: int(key), Analysis: q.Analysis, ErrorCategory: tc.TrainingQuestionReviewErrorCategory(r.ErrorCategory), Evidence: r.Evidence})
	}
	// Fingerprint the whole resolved object, omitting only the two derived fields.
	encoded, _ := json.Marshal(version)
	var canonical map[string]any
	_ = json.Unmarshal(encoded, &canonical)
	unit := canonical["unit"].(map[string]any)
	delete(unit, "unit_version_id")
	delete(unit, "content_sha256")
	encoded, _ = json.Marshal(canonical)
	fingerprint := hash(encoded)
	version.Unit.UnitVersionId = stableID(unitID, "version:"+fingerprint)
	version.Unit.ContentSha256 = fingerprint
	encoded, _ = json.Marshal(version)
	if err := tc.Validate("TrainingListeningVersion", encoded); err != nil {
		return invalid(fmt.Sprintf("Resolved training is invalid: %v", err))
	}
	if _, err := tx.Exec(ctx, `INSERT INTO quizcraft_training_units(id,bank_id,source_unit_id) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, unitID, bankID, source.SourceUnitId); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO quizcraft_training_unit_versions(id,unit_id,bank_id,bank_version_id,content_sha256,document) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT DO NOTHING`, version.Unit.UnitVersionId, unitID, bankID, bankVersion, fingerprint, encoded); err != nil {
		return err
	}
	for i, q := range version.Unit.Questions {
		if _, err := tx.Exec(ctx, `INSERT INTO quizcraft_training_members(unit_version_id,bank_id,bank_version_id,question_id,question_version_id,position) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT DO NOTHING`, version.Unit.UnitVersionId, bankID, bankVersion, q.QuestionId, q.QuestionVersionId, i+1); err != nil {
			return err
		}
	}
	return nil
}

func validateTrainingBinding(ctx context.Context, tx pgx.Tx, bank, version uuid.UUID) error {
	var training bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM quizcraft_training_units WHERE bank_id=$1)`, bank).Scan(&training); err != nil {
		return err
	}
	if !training {
		return nil
	}
	var raw []byte
	if err := tx.QueryRow(ctx, `SELECT document FROM quizcraft_training_unit_versions WHERE bank_id=$1 AND bank_version_id=$2`, bank, version).Scan(&raw); err != nil {
		return workshopHTTPError{409, "training_binding_required", "This bank version has no exact training binding"}
	}
	var document tc.TrainingListeningVersion
	if tc.Validate("TrainingListeningVersion", raw) != nil || json.Unmarshal(raw, &document) != nil {
		return workshopHTTPError{409, "invalid_training", "Training content failed validation"}
	}
	_, err := listeningAsset(tc.TrainingAuthoredMedia{Sha256: document.Unit.Media.Sha256, DurationMs: document.Unit.Media.DurationMs, ContentType: "audio/mpeg"})
	if err != nil {
		return workshopHTTPError{409, "invalid_training", err.Error()}
	}
	var members int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM quizcraft_training_members WHERE unit_version_id=$1`, document.Unit.UnitVersionId).Scan(&members); err != nil {
		return err
	}
	if members != len(document.Unit.Questions) {
		return workshopHTTPError{409, "invalid_training", "Training membership is incomplete"}
	}
	return nil
}
