package quizcraft

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type LegacyBank struct {
	BankKey  string          `json:"bank_key"`
	Document json.RawMessage `json:"document"`
}

type LegacyFeedback struct {
	LegacyID       string     `json:"legacy_id"`
	BankKey        string     `json:"bank_key"`
	QuestionID     string     `json:"question_id"`
	Suggestion     string     `json:"suggestion"`
	Status         string     `json:"status"`
	ResolutionNote string     `json:"resolution_note"`
	CreatedAt      time.Time  `json:"created_at"`
	ResolvedAt     *time.Time `json:"resolved_at,omitempty"`
}

type LegacySnapshot struct {
	SourceName    string           `json:"source_name"`
	CutoffEventID int64            `json:"cutoff_event_id"`
	Banks         []LegacyBank     `json:"banks"`
	Feedback      []LegacyFeedback `json:"feedback"`
	Rankings      json.RawMessage  `json:"rankings"`
}

type ReconciliationMetrics struct {
	BankCount     int            `json:"bank_count"`
	QuestionCount int            `json:"question_count"`
	AnsweredCount int            `json:"answered_count"`
	TypeCounts    map[string]int `json:"type_counts"`
	ChapterCounts map[string]int `json:"chapter_counts"`
	AnswerSHA256  string         `json:"answer_sha256"`
	ContentSHA256 string         `json:"content_sha256"`
}

type MigrationReport struct {
	RunID                   uuid.UUID             `json:"run_id"`
	Source                  ReconciliationMetrics `json:"source"`
	Target                  ReconciliationMetrics `json:"target"`
	Differences             []string              `json:"differences"`
	ContentReconciled       bool                  `json:"content_reconciled"`
	FeedbackSourceCount     int                   `json:"feedback_source_count"`
	FeedbackMigratedCount   int                   `json:"feedback_migrated_count"`
	FeedbackExceptionCount  int                   `json:"feedback_exception_count"`
	LegacyRankingEntryCount int                   `json:"legacy_ranking_entry_count"`
	MappedLegacyUserCount   int                   `json:"mapped_legacy_user_count"`
	CutoffEventID           int64                 `json:"cutoff_event_id"`
	State                   string                `json:"state"`
}

type LegacyEvent struct {
	ID                  int64           `json:"id"`
	Type                string          `json:"type"`
	AggregateKey        string          `json:"aggregate_key"`
	Payload             json.RawMessage `json:"payload"`
	sourcePayloadSHA256 string
}

type CatchUpReport struct {
	RunID          uuid.UUID `json:"run_id"`
	PreviousCursor int64     `json:"previous_cursor"`
	CurrentCursor  int64     `json:"current_cursor"`
	SourceHead     int64     `json:"source_head"`
	AppliedCount   int       `json:"applied_count"`
	CaughtUp       bool      `json:"caught_up"`
	Ready          bool      `json:"ready"`
	ExceptionCount int       `json:"exception_count"`
	BlockingReason string    `json:"blocking_reason,omitempty"`
}

type ShadowGateReport struct {
	ID                 uuid.UUID `json:"id"`
	WindowStart        time.Time `json:"window_start"`
	WindowEnd          time.Time `json:"window_end"`
	SampleCount        int64     `json:"sample_count"`
	MismatchCount      int64     `json:"mismatch_count"`
	LegacyErrorCount   int64     `json:"legacy_error_count"`
	MismatchRate       float64   `json:"mismatch_rate"`
	MismatchThreshold  float64   `json:"mismatch_threshold"`
	MinimumSampleCount int64     `json:"minimum_sample_count"`
	Decision           string    `json:"decision"`
	Reasons            []string  `json:"reasons"`
}

func (s *Service) RunFullMigration(ctx context.Context, snapshot LegacySnapshot) (MigrationReport, error) {
	snapshot.SourceName = strings.TrimSpace(snapshot.SourceName)
	if snapshot.SourceName == "" || snapshot.CutoffEventID < 0 {
		return MigrationReport{}, errors.New("migration source and non-negative cutoff event are required")
	}
	runID := uuid.New()
	if _, err := s.database.Exec(ctx, `INSERT INTO quizcraft_migration_runs(id,source_name,source_cutoff_event_id,caught_up_event_id,state) VALUES($1,$2,$3,$3,'running')`, runID, snapshot.SourceName, snapshot.CutoffEventID); err != nil {
		return MigrationReport{}, err
	}

	reports := make([]struct {
		key    string
		report ImportReport
	}, 0, len(snapshot.Banks))
	for _, bank := range snapshot.Banks {
		report, err := s.importJSON(ctx, bank.BankKey, bank.Document, importOptions{activate: true})
		if err != nil {
			return MigrationReport{}, s.blockMigrationRun(ctx, runID, fmt.Errorf("import legacy bank %s: %w", bank.BankKey, err))
		}
		reports = append(reports, struct {
			key    string
			report ImportReport
		}{key: bank.BankKey, report: report})
	}

	migrated, exceptions, err := s.migrateLegacyFeedback(ctx, runID, snapshot.SourceName, snapshot.CutoffEventID, snapshot.Feedback)
	if err != nil {
		return MigrationReport{}, s.blockMigrationRun(ctx, runID, err)
	}
	if err := s.database.QueryRow(ctx, `SELECT count(*) FROM quizcraft_migration_exceptions e LEFT JOIN quizcraft_migration_exception_resolutions r ON r.exception_id=e.id WHERE e.run_id=$1 AND r.exception_id IS NULL`, runID).Scan(&exceptions); err != nil {
		return MigrationReport{}, s.blockMigrationRun(ctx, runID, err)
	}
	rankingCount, err := s.storeLegacyRankingSnapshot(ctx, runID, snapshot.CutoffEventID, snapshot.Rankings)
	if err != nil {
		return MigrationReport{}, s.blockMigrationRun(ctx, runID, err)
	}
	sourceMetrics := metricsFromImportReports(reports)
	bankKeys := make([]string, 0, len(reports))
	for _, item := range reports {
		bankKeys = append(bankKeys, item.key)
	}
	targetMetrics, err := s.currentContentMetrics(ctx, bankKeys)
	if err != nil {
		return MigrationReport{}, s.blockMigrationRun(ctx, runID, err)
	}
	differences := compareMetrics(sourceMetrics, targetMetrics)
	contentReconciled := len(differences) == 0
	if exceptions > 0 {
		differences = append(differences, "unresolved_feedback_references")
	}
	state := "passed"
	if len(differences) > 0 {
		state = "blocked"
	}
	report := MigrationReport{RunID: runID, Source: sourceMetrics, Target: targetMetrics, Differences: differences, ContentReconciled: contentReconciled, FeedbackSourceCount: len(snapshot.Feedback), FeedbackMigratedCount: migrated, FeedbackExceptionCount: exceptions, LegacyRankingEntryCount: rankingCount, MappedLegacyUserCount: 0, CutoffEventID: snapshot.CutoffEventID, State: state}
	encoded, _ := json.Marshal(report)
	if _, err := s.database.Exec(ctx, `UPDATE quizcraft_migration_runs SET state=$2,report=$3,report_sha256=$4,completed_at=now() WHERE id=$1 AND state='running'`, runID, state, encoded, sha256Hex(encoded)); err != nil {
		return MigrationReport{}, err
	}
	return report, nil
}

func (s *Service) blockMigrationRun(ctx context.Context, runID uuid.UUID, cause error) error {
	report, _ := json.Marshal(map[string]string{"error": cause.Error()})
	_, _ = s.database.Exec(ctx, `UPDATE quizcraft_migration_runs SET state='blocked',report=$2,report_sha256=$3,completed_at=now() WHERE id=$1 AND state='running'`, runID, report, sha256Hex(report))
	return cause
}

func metricsFromImportReports(reports []struct {
	key    string
	report ImportReport
}) ReconciliationMetrics {
	metrics := ReconciliationMetrics{BankCount: len(reports), TypeCounts: map[string]int{}, ChapterCounts: map[string]int{}}
	hashes := make([]string, 0, len(reports))
	answerHashes := make([]string, 0)
	for _, item := range reports {
		metrics.QuestionCount += item.report.QuestionCount
		metrics.AnsweredCount += item.report.AnsweredCount
		for key, count := range item.report.TypeCounts {
			metrics.TypeCounts[key] += count
		}
		for key, count := range item.report.ChapterCounts {
			metrics.ChapterCounts[key] += count
		}
		hashes = append(hashes, item.key+":"+item.report.ContentSHA256)
		for _, question := range item.report.Questions {
			answerHashes = append(answerHashes, item.key+":"+question.SourceQuestionID+":"+question.AnswerSHA256)
		}
	}
	sort.Strings(hashes)
	sort.Strings(answerHashes)
	metrics.AnswerSHA256 = sha256Hex([]byte(strings.Join(answerHashes, "\n")))
	metrics.ContentSHA256 = sha256Hex([]byte(strings.Join(hashes, "\n")))
	return metrics
}

func (s *Service) currentContentMetrics(ctx context.Context, bankKeys []string) (ReconciliationMetrics, error) {
	metrics := ReconciliationMetrics{TypeCounts: map[string]int{}, ChapterCounts: map[string]int{}}
	rows, err := s.database.Query(ctx, `SELECT b.bank_key,bv.content_sha256,q.source_question_id,qv.type,qv.chapter_id,qv.answer FROM quizcraft_banks b JOIN quizcraft_bank_versions bv ON bv.id=b.active_version_id JOIN quizcraft_bank_version_questions m ON m.bank_version_id=bv.id JOIN quizcraft_questions q ON q.id=m.question_id JOIN quizcraft_question_versions qv ON qv.id=m.question_version_id WHERE b.bank_key=ANY($1::text[]) ORDER BY b.bank_key,m.position`, bankKeys)
	if err != nil {
		return metrics, err
	}
	defer rows.Close()
	bankHashes := map[string]string{}
	answerHashes := make([]string, 0)
	for rows.Next() {
		var bankKey, bankHash, sourceQuestionID, questionType, chapter string
		var answer []byte
		if err := rows.Scan(&bankKey, &bankHash, &sourceQuestionID, &questionType, &chapter, &answer); err != nil {
			return metrics, err
		}
		bankHashes[bankKey] = bankHash
		metrics.QuestionCount++
		if string(answer) != "null" {
			metrics.AnsweredCount++
		}
		var answerValue any
		if err := json.Unmarshal(answer, &answerValue); err != nil {
			return metrics, err
		}
		canonicalAnswer, _ := json.Marshal(answerValue)
		answerHashes = append(answerHashes, bankKey+":"+sourceQuestionID+":"+sha256Hex(canonicalAnswer))
		metrics.TypeCounts[questionType]++
		metrics.ChapterCounts[chapter]++
	}
	if err := rows.Err(); err != nil {
		return metrics, err
	}
	metrics.BankCount = len(bankHashes)
	hashes := make([]string, 0, len(bankHashes))
	for key, value := range bankHashes {
		hashes = append(hashes, key+":"+value)
	}
	sort.Strings(hashes)
	sort.Strings(answerHashes)
	metrics.AnswerSHA256 = sha256Hex([]byte(strings.Join(answerHashes, "\n")))
	metrics.ContentSHA256 = sha256Hex([]byte(strings.Join(hashes, "\n")))
	return metrics, nil
}

func compareMetrics(source, target ReconciliationMetrics) []string {
	differences := make([]string, 0)
	if source.BankCount != target.BankCount {
		differences = append(differences, "bank_count")
	}
	if source.QuestionCount != target.QuestionCount {
		differences = append(differences, "question_count")
	}
	if source.AnsweredCount != target.AnsweredCount {
		differences = append(differences, "answered_count")
	}
	if !reflect.DeepEqual(source.TypeCounts, target.TypeCounts) {
		differences = append(differences, "type_counts")
	}
	if !reflect.DeepEqual(source.ChapterCounts, target.ChapterCounts) {
		differences = append(differences, "chapter_counts")
	}
	if source.AnswerSHA256 != target.AnswerSHA256 {
		differences = append(differences, "answer_sha256")
	}
	if source.ContentSHA256 != target.ContentSHA256 {
		differences = append(differences, "content_sha256")
	}
	return differences
}

func (s *Service) migrateLegacyFeedback(ctx context.Context, runID uuid.UUID, sourceName string, sourceEventID int64, feedback []LegacyFeedback) (int, int, error) {
	migrated, exceptions := 0, 0
	for _, item := range feedback {
		item.LegacyID = strings.TrimSpace(item.LegacyID)
		item.BankKey = strings.TrimSpace(item.BankKey)
		item.QuestionID = strings.TrimSpace(item.QuestionID)
		item.Suggestion = strings.TrimSpace(item.Suggestion)
		status := strings.ToLower(strings.TrimSpace(item.Status))
		if status == "" {
			status = "pending"
		}
		if item.LegacyID == "" || item.Suggestion == "" || (status != "pending" && status != "resolved" && status != "archived") {
			if err := s.storeMigrationException(ctx, runID, item.LegacyID, "invalid_record", item); err != nil {
				return migrated, exceptions, err
			}
			exceptions++
			continue
		}
		var bankID, questionID, versionID uuid.UUID
		err := s.database.QueryRow(ctx, `SELECT b.id,q.id,m.question_version_id FROM quizcraft_banks b JOIN quizcraft_questions q ON q.bank_id=b.id JOIN quizcraft_bank_version_questions m ON m.bank_version_id=b.active_version_id AND m.question_id=q.id WHERE b.bank_key=$1 AND q.source_question_id=$2`, item.BankKey, item.QuestionID).Scan(&bankID, &questionID, &versionID)
		if errors.Is(err, pgx.ErrNoRows) {
			reason := "missing_question_reference"
			var bankExists bool
			if queryErr := s.database.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM quizcraft_banks WHERE bank_key=$1)`, item.BankKey).Scan(&bankExists); queryErr != nil {
				return migrated, exceptions, queryErr
			}
			if !bankExists {
				reason = "missing_bank_reference"
			}
			if err := s.storeMigrationException(ctx, runID, item.LegacyID, reason, item); err != nil {
				return migrated, exceptions, err
			}
			exceptions++
			continue
		}
		if err != nil {
			return migrated, exceptions, err
		}
		feedbackID := stableID(quizcraftNamespace, "legacy-feedback:"+sourceName+":"+item.LegacyID)
		createdAt := item.CreatedAt
		if createdAt.IsZero() {
			createdAt = time.Unix(0, 0).UTC()
		}
		command, err := s.database.Exec(ctx, `INSERT INTO quizcraft_feedbacks(id,bank_id,question_id,question_version_id,actor_user_id,actor_key,category,detail,created_at,legacy_feedback_id,legacy_status,legacy_resolved_at,legacy_resolution_note) VALUES($1,$2,$3,$4,NULL,'legacy-unmapped','other',$5,$6,$7,$8,$9,$10) ON CONFLICT(legacy_feedback_id) WHERE legacy_feedback_id IS NOT NULL DO NOTHING`, feedbackID, bankID, questionID, versionID, item.Suggestion, createdAt, item.LegacyID, status, item.ResolvedAt, item.ResolutionNote)
		if err != nil {
			return migrated, exceptions, err
		}
		if _, err := s.database.Exec(ctx, `INSERT INTO quizcraft_legacy_feedback_state_events(id,run_id,legacy_feedback_id,source_event_id,status,resolved_at,resolution_note) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(run_id,legacy_feedback_id,source_event_id) DO NOTHING`, uuid.New(), runID, item.LegacyID, sourceEventID, status, item.ResolvedAt, item.ResolutionNote); err != nil {
			return migrated, exceptions, err
		}
		if command.RowsAffected() > 0 {
			migrated++
		}
		if sourceEventID > 0 {
			if _, err := s.database.Exec(ctx, `INSERT INTO quizcraft_migration_exception_resolutions(id,exception_id,resolved_by_event_id,resolution) SELECT $1,e.id,$4,'reference_resolved' FROM quizcraft_migration_exceptions e LEFT JOIN quizcraft_migration_exception_resolutions r ON r.exception_id=e.id WHERE e.run_id=$2 AND e.record_type='feedback' AND e.legacy_record_id=$3 AND r.exception_id IS NULL ON CONFLICT(exception_id) DO NOTHING`, uuid.New(), runID, item.LegacyID, sourceEventID); err != nil {
				return migrated, exceptions, err
			}
		}
	}
	return migrated, exceptions, nil
}

func (s *Service) storeMigrationException(ctx context.Context, runID uuid.UUID, legacyID, reason string, detail any) error {
	if strings.TrimSpace(legacyID) == "" {
		legacyID = "invalid:" + uuid.NewString()
	}
	encoded, _ := json.Marshal(detail)
	_, err := s.database.Exec(ctx, `INSERT INTO quizcraft_migration_exceptions(id,run_id,record_type,legacy_record_id,reason_code,detail) VALUES($1,$2,'feedback',$3,$4,$5) ON CONFLICT(run_id,record_type,legacy_record_id) DO NOTHING`, uuid.New(), runID, legacyID, reason, encoded)
	return err
}

func (s *Service) storeLegacyRankingSnapshot(ctx context.Context, runID uuid.UUID, eventID int64, standings json.RawMessage) (int, error) {
	if len(standings) == 0 {
		standings = json.RawMessage(`[]`)
	}
	var value any
	if err := json.Unmarshal(standings, &value); err != nil {
		return 0, errors.New("legacy ranking snapshot must be valid JSON")
	}
	canonical, _ := json.Marshal(value)
	count := 0
	switch typed := value.(type) {
	case []any:
		count = len(typed)
	case map[string]any:
		if users, ok := typed["users"].(map[string]any); ok {
			count = len(users)
		}
	}
	_, err := s.database.Exec(ctx, `INSERT INTO quizcraft_legacy_ranking_snapshots(id,run_id,source_event_id,standings,content_sha256) VALUES($1,$2,$3,$4,$5) ON CONFLICT(run_id,source_event_id) DO NOTHING`, uuid.New(), runID, eventID, canonical, sha256Hex(canonical))
	return count, err
}

func (s *Service) ApplyIncrementalEvents(ctx context.Context, runID uuid.UUID, sourceHead int64, events []LegacyEvent) (CatchUpReport, error) {
	var sourceName string
	var cursor int64
	if err := s.database.QueryRow(ctx, `SELECT source_name,caught_up_event_id FROM quizcraft_migration_runs WHERE id=$1`, runID).Scan(&sourceName, &cursor); err != nil {
		return CatchUpReport{}, err
	}
	report := CatchUpReport{RunID: runID, PreviousCursor: cursor, CurrentCursor: cursor, SourceHead: sourceHead}
	if sourceHead < cursor {
		return report, errors.New("source head cannot move behind the migration cursor")
	}
	previousEventID := cursor
	for _, event := range events {
		if event.ID <= previousEventID || event.ID > sourceHead {
			return report, fmt.Errorf("incremental events must be strictly increasing after cursor %d and not exceed source head %d", cursor, sourceHead)
		}
		previousEventID = event.ID
	}
	for _, event := range events {
		if err := s.applyLegacyEvent(ctx, runID, sourceName, report.CurrentCursor, event); err != nil {
			return report, err
		}
		report.CurrentCursor = event.ID
		report.AppliedCount++
	}
	report.CaughtUp = report.CurrentCursor == sourceHead
	if !report.CaughtUp {
		report.BlockingReason = "incremental_event_lag"
	}
	var contentReconciled bool
	if err := s.database.QueryRow(ctx, `SELECT COALESCE((report->>'content_reconciled')::boolean,false),(SELECT count(*) FROM quizcraft_migration_exceptions e LEFT JOIN quizcraft_migration_exception_resolutions r ON r.exception_id=e.id WHERE e.run_id=$1 AND r.exception_id IS NULL) FROM quizcraft_migration_runs WHERE id=$1`, runID).Scan(&contentReconciled, &report.ExceptionCount); err != nil {
		return report, err
	}
	if report.CaughtUp && report.ExceptionCount > 0 {
		report.BlockingReason = "unresolved_migration_exceptions"
	}
	if report.CaughtUp && report.ExceptionCount == 0 && !contentReconciled {
		report.BlockingReason = "content_reconciliation_failed"
	}
	report.Ready = report.CaughtUp && report.ExceptionCount == 0 && contentReconciled
	if report.Ready {
		if _, err := s.database.Exec(ctx, `UPDATE quizcraft_migration_runs SET state='passed',completed_at=now() WHERE id=$1`, runID); err != nil {
			return report, err
		}
	}
	return report, nil
}

func (s *Service) applyLegacyEvent(ctx context.Context, runID uuid.UUID, sourceName string, previousCursor int64, event LegacyEvent) error {
	if event.Type != "bank.upserted" && event.Type != "feedback.upserted" && event.Type != "ranking.changed" {
		return fmt.Errorf("unsupported legacy event type %q", event.Type)
	}
	if strings.TrimSpace(event.AggregateKey) == "" || !json.Valid(event.Payload) {
		return errors.New("legacy event aggregate key and JSON payload are required")
	}
	var exists bool
	if err := s.database.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM quizcraft_migration_event_receipts WHERE source_name=$1 AND source_event_id=$2)`, sourceName, event.ID).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return errors.New("legacy event already applied outside the current cursor")
	}
	switch event.Type {
	case "bank.upserted":
		var bank LegacyBank
		if err := json.Unmarshal(event.Payload, &bank); err != nil {
			return err
		}
		if _, err := s.importJSON(ctx, bank.BankKey, bank.Document, importOptions{activate: true}); err != nil {
			return err
		}
		if err := s.retryFeedbackExceptionsForBank(ctx, runID, sourceName, event.ID, bank.BankKey); err != nil {
			return err
		}
	case "feedback.upserted":
		var feedback LegacyFeedback
		if err := json.Unmarshal(event.Payload, &feedback); err != nil {
			return err
		}
		if _, _, err := s.migrateLegacyFeedback(ctx, runID, sourceName, event.ID, []LegacyFeedback{feedback}); err != nil {
			return err
		}
	case "ranking.changed":
		if _, err := s.storeLegacyRankingSnapshot(ctx, runID, event.ID, event.Payload); err != nil {
			return err
		}
	}
	tx, err := s.database.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	payloadSHA256 := event.sourcePayloadSHA256
	if payloadSHA256 == "" {
		payloadSHA256 = sha256Hex(event.Payload)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO quizcraft_migration_event_receipts(source_name,source_event_id,run_id,event_type,aggregate_key,payload_sha256) VALUES($1,$2,$3,$4,$5,$6)`, sourceName, event.ID, runID, event.Type, event.AggregateKey, payloadSHA256); err != nil {
		return err
	}
	command, err := tx.Exec(ctx, `UPDATE quizcraft_migration_runs SET caught_up_event_id=$2 WHERE id=$1 AND caught_up_event_id=$3`, runID, event.ID, previousCursor)
	if err != nil {
		return err
	}
	if command.RowsAffected() != 1 {
		return errors.New("migration cursor changed concurrently")
	}
	return tx.Commit(ctx)
}

func (s *Service) retryFeedbackExceptionsForBank(ctx context.Context, runID uuid.UUID, sourceName string, sourceEventID int64, bankKey string) error {
	rows, err := s.database.Query(ctx, `SELECT e.detail FROM quizcraft_migration_exceptions e LEFT JOIN quizcraft_migration_exception_resolutions r ON r.exception_id=e.id WHERE e.run_id=$1 AND e.record_type='feedback' AND e.detail->>'bank_key'=$2 AND r.exception_id IS NULL ORDER BY e.created_at,e.id`, runID, bankKey)
	if err != nil {
		return err
	}
	defer rows.Close()
	feedback := make([]LegacyFeedback, 0)
	for rows.Next() {
		var detail json.RawMessage
		if err := rows.Scan(&detail); err != nil {
			return err
		}
		var item LegacyFeedback
		if err := json.Unmarshal(detail, &item); err != nil {
			return fmt.Errorf("decode feedback migration exception: %w", err)
		}
		feedback = append(feedback, item)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(feedback) == 0 {
		return nil
	}
	_, _, err = s.migrateLegacyFeedback(ctx, runID, sourceName, sourceEventID, feedback)
	return err
}

func (s *Service) EvaluateShadowGate(ctx context.Context, windowStart, windowEnd time.Time, minimumSamples int64, mismatchThreshold float64) (ShadowGateReport, error) {
	if !windowEnd.After(windowStart) || minimumSamples < 1 || mismatchThreshold < 0 || mismatchThreshold > 1 {
		return ShadowGateReport{}, errors.New("valid shadow window, positive sample minimum, and threshold from 0 to 1 are required")
	}
	var databaseNow time.Time
	if err := s.database.QueryRow(ctx, `SELECT now()`).Scan(&databaseNow); err != nil {
		return ShadowGateReport{}, err
	}
	if windowEnd.After(databaseNow) {
		return ShadowGateReport{}, errors.New("shadow observation window must be closed before evaluation")
	}
	report := ShadowGateReport{ID: uuid.New(), WindowStart: windowStart, WindowEnd: windowEnd, MinimumSampleCount: minimumSamples, MismatchThreshold: mismatchThreshold, Decision: "pass", Reasons: []string{}}
	if err := s.database.QueryRow(ctx, `SELECT count(*),count(*) FILTER (WHERE outcome='mismatch'),count(*) FILTER (WHERE outcome='legacy_error') FROM quizcraft_shadow_comparisons WHERE compared_at >= $1 AND compared_at < $2`, windowStart, windowEnd).Scan(&report.SampleCount, &report.MismatchCount, &report.LegacyErrorCount); err != nil {
		return ShadowGateReport{}, err
	}
	if report.SampleCount > 0 {
		report.MismatchRate = float64(report.MismatchCount+report.LegacyErrorCount) / float64(report.SampleCount)
	}
	if report.SampleCount < minimumSamples {
		report.Reasons = append(report.Reasons, "insufficient_samples")
	}
	if report.MismatchRate > mismatchThreshold {
		report.Reasons = append(report.Reasons, "mismatch_threshold_exceeded")
	}
	if len(report.Reasons) > 0 {
		report.Decision = "block"
	}
	reasons, _ := json.Marshal(report.Reasons)
	_, err := s.database.Exec(ctx, `INSERT INTO quizcraft_shadow_gate_reports(id,window_start,window_end,sample_count,mismatch_count,legacy_error_count,mismatch_rate,mismatch_threshold,minimum_sample_count,decision,reasons) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, report.ID, windowStart, windowEnd, report.SampleCount, report.MismatchCount, report.LegacyErrorCount, report.MismatchRate, mismatchThreshold, minimumSamples, report.Decision, reasons)
	return report, err
}

func sha256Hex(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}
