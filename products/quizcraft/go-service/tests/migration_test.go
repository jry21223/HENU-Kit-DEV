package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	quizcraft "henukit.dev/quizcraft"
)

func TestFullMigrationReconcilesContentQuarantinesFeedbackAndSnapshotsRanking(t *testing.T) {
	pool := migrationPool(t)
	service, err := quizcraft.New(quizcraft.Config{Database: pool})
	if err != nil {
		t.Fatal(err)
	}
	bankKey := "migration-full-" + uuid.NewString()
	createdAt := time.Date(2026, 7, 1, 1, 2, 3, 0, time.UTC)
	var profilesBefore int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM quizcraft_ranking_profiles`).Scan(&profilesBefore); err != nil {
		t.Fatal(err)
	}
	snapshot := quizcraft.LegacySnapshot{
		SourceName:    "legacy-production",
		CutoffEventID: 41,
		Banks:         []quizcraft.LegacyBank{{BankKey: bankKey, Document: json.RawMessage(validBank)}},
		Feedback: []quizcraft.LegacyFeedback{
			{LegacyID: "feedback-1", BankKey: bankKey, QuestionID: "q0001", Suggestion: "答案说明有误", Status: "resolved", ResolutionNote: "旧后台已处理", CreatedAt: createdAt, ResolvedAt: &createdAt},
			{LegacyID: "feedback-2", BankKey: bankKey, QuestionID: "missing", Suggestion: "无法定位原题", Status: "pending", CreatedAt: createdAt},
		},
		Rankings: json.RawMessage(`{"users":{"generated-old-user":{"name":"旧榜用户","correct":9,"total":10}},"name_to_id":{}}`),
	}
	report, err := service.RunFullMigration(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if report.State != "blocked" || !report.ContentReconciled || len(report.Discrepancies) != 0 || report.Source.BankCount != 1 || report.Source.QuestionCount != 4 || report.Source.AnsweredCount != 4 || report.Source.TypeCounts["single"] != 1 || report.Source.ChapterCounts["ch01"] != 3 || report.Source.AnswerSHA256 == "" || report.Source.AnswerSHA256 != report.Target.AnswerSHA256 || report.Source.ContentSHA256 == "" || report.Source.ContentSHA256 != report.Target.ContentSHA256 {
		t.Fatalf("reconciliation report = %+v", report)
	}
	if report.FeedbackSourceCount != 2 || report.FeedbackMigratedCount != 1 || report.FeedbackExceptionCount != 1 || report.LegacyRankingEntryCount != 1 || report.MappedLegacyUserCount != 0 {
		t.Fatalf("migration boundaries = %+v", report)
	}
	var actorUserID *uuid.UUID
	var actorKey, legacyStatus, note string
	if err := pool.QueryRow(context.Background(), `SELECT actor_user_id,actor_key,legacy_status,legacy_resolution_note FROM quizcraft_feedbacks WHERE legacy_feedback_id='feedback-1'`).Scan(&actorUserID, &actorKey, &legacyStatus, &note); err != nil {
		t.Fatal(err)
	}
	if actorUserID != nil || actorKey != "legacy-unmapped" || legacyStatus != "resolved" || note != "旧后台已处理" {
		t.Fatalf("legacy feedback identity/status = %v/%s/%s/%s", actorUserID, actorKey, legacyStatus, note)
	}
	var reason string
	if err := pool.QueryRow(context.Background(), `SELECT reason_code FROM quizcraft_migration_exceptions WHERE run_id=$1 AND legacy_record_id='feedback-2'`, report.RunID).Scan(&reason); err != nil || reason != "missing_question_reference" {
		t.Fatalf("migration exception = %q / %v", reason, err)
	}
	var snapshotJSON []byte
	if err := pool.QueryRow(context.Background(), `SELECT standings FROM quizcraft_legacy_ranking_snapshots WHERE run_id=$1`, report.RunID).Scan(&snapshotJSON); err != nil {
		t.Fatal(err)
	}
	var profiles int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM quizcraft_ranking_profiles`).Scan(&profiles); err != nil {
		t.Fatal(err)
	}
	if !json.Valid(snapshotJSON) || !bytes.Contains(snapshotJSON, []byte("generated-old-user")) || profiles != profilesBefore {
		t.Fatalf("legacy ranking was mapped or lost: %s profiles=%d", snapshotJSON, profiles)
	}
	handler, err := quizcraft.NewPracticeHTTP(quizcraft.PracticeHTTPConfig{Database: pool, AuthHMACSecret: []byte(practiceAuthSecret)})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	status, publicRanking := requestJSON(t, http.MethodGet, server.URL+"/api/v1/rankings/legacy", nil, nil)
	if status != http.StatusOK || !bytes.Contains(publicRanking, []byte(`"name":"旧榜用户"`)) || !bytes.Contains(publicRanking, []byte(`"correct":9`)) || bytes.Contains(publicRanking, []byte("generated-old-user")) || bytes.Contains(publicRanking, []byte("legacy_subject_id")) {
		t.Fatalf("public legacy ranking boundary = %d %s", status, publicRanking)
	}
	if _, err := pool.Exec(context.Background(), `UPDATE quizcraft_legacy_ranking_snapshots SET standings='[]' WHERE run_id=$1`, report.RunID); err == nil {
		t.Fatal("legacy ranking snapshot was mutable")
	}
	resolvedPayload := mustJSON(quizcraft.LegacyFeedback{LegacyID: "feedback-2", BankKey: bankKey, QuestionID: "q0002", Suggestion: "已补齐稳定引用", Status: "pending", CreatedAt: createdAt})
	catchUp, err := service.ApplyIncrementalEvents(context.Background(), report.RunID, 42, []quizcraft.LegacyEvent{{ID: 42, Type: "feedback.upserted", AggregateKey: "feedback-2", Payload: resolvedPayload}})
	if err != nil || !catchUp.CaughtUp || !catchUp.Ready || catchUp.ExceptionCount != 0 {
		t.Fatalf("resolved migration exception did not unblock: %+v / %v", catchUp, err)
	}
	var resolutions int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM quizcraft_migration_exception_resolutions r JOIN quizcraft_migration_exceptions e ON e.id=r.exception_id WHERE e.run_id=$1`, report.RunID).Scan(&resolutions); err != nil || resolutions != 1 {
		t.Fatalf("exception resolution fact = %d / %v", resolutions, err)
	}
}

func TestFullMigrationRejectsAUsedTargetBeforeCreatingAMigrationRun(t *testing.T) {
	pool := migrationPool(t)
	ctx := context.Background()
	service, err := quizcraft.New(quizcraft.Config{Database: pool})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO quizcraft_banks(id,bank_key,name) VALUES($1,'existing-target-fact','Existing target fact')`, uuid.New()); err != nil {
		t.Fatal(err)
	}
	_, err = service.RunFullMigration(ctx, quizcraft.LegacySnapshot{
		SourceName:    "legacy-existing-target",
		CutoffEventID: 1,
		Banks:         []quizcraft.LegacyBank{{BankKey: "migration-existing-target", Document: json.RawMessage(validBank)}},
		Rankings:      json.RawMessage(`[]`),
	})
	if err == nil || !strings.Contains(err.Error(), "empty quizcraft_v2 target") {
		t.Fatalf("non-empty target was accepted: %v", err)
	}
	var runs int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM quizcraft_migration_runs`).Scan(&runs); err != nil {
		t.Fatal(err)
	}
	if runs != 0 {
		t.Fatalf("non-empty target created %d migration runs", runs)
	}
}

func TestFullMigrationPersistsFieldLevelContentDiscrepancies(t *testing.T) {
	pool := migrationPool(t)
	ctx := context.Background()
	service, err := quizcraft.New(quizcraft.Config{Database: pool})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `CREATE OR REPLACE FUNCTION quizcraft_test_corrupt_migrated_answer() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.type='single' THEN NEW.answer='"corrupted"'::jsonb; END IF; RETURN NEW; END $$`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `CREATE TRIGGER quizcraft_test_corrupt_migrated_answer BEFORE INSERT ON quizcraft_question_versions FOR EACH ROW EXECUTE FUNCTION quizcraft_test_corrupt_migrated_answer()`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DROP TRIGGER IF EXISTS quizcraft_test_corrupt_migrated_answer ON quizcraft_question_versions; DROP FUNCTION IF EXISTS quizcraft_test_corrupt_migrated_answer()`)
	})

	bankKey := "migration-discrepancy-" + uuid.NewString()
	report, err := service.RunFullMigration(ctx, quizcraft.LegacySnapshot{
		SourceName:    "legacy-discrepancy",
		CutoffEventID: 1,
		Banks:         []quizcraft.LegacyBank{{BankKey: bankKey, Document: json.RawMessage(validBank)}},
		Rankings:      json.RawMessage(`[]`),
	})
	if err != nil || report.State != "blocked" || report.ContentReconciled {
		t.Fatalf("corrupted migration report = %+v / %v", report, err)
	}
	found := false
	for _, discrepancy := range report.Discrepancies {
		if discrepancy.Code == "question_answer_sha256_mismatch" && discrepancy.BankKey == bankKey && discrepancy.SourceQuestionID == "q0001" && discrepancy.Expected != "" && discrepancy.Actual != "" {
			found = true
		}
	}
	if !found {
		t.Fatalf("field-level answer discrepancy missing from %+v", report.Discrepancies)
	}
	var persistedReport []byte
	if err := pool.QueryRow(ctx, `SELECT report FROM quizcraft_migration_runs WHERE id=$1`, report.RunID).Scan(&persistedReport); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(persistedReport, []byte(`"question_answer_sha256_mismatch"`)) {
		t.Fatalf("persisted discrepancy report = %s", persistedReport)
	}
}

func TestFullMigrationPersistsAnUnexpectedTargetBankDiscrepancy(t *testing.T) {
	pool := migrationPool(t)
	ctx := context.Background()
	service, err := quizcraft.New(quizcraft.Config{Database: pool})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `CREATE OR REPLACE FUNCTION quizcraft_test_add_unexpected_target_facts() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.bank_key <> 'unexpected-target-bank' THEN INSERT INTO quizcraft_bank_versions(id,bank_id,name,source_version,source_sha256,content_sha256,import_report) VALUES('00000000-0000-0000-0000-000000000161',NEW.id,'Unexpected inactive version','',repeat('a',64),repeat('b',64),'{}'::jsonb); INSERT INTO quizcraft_questions(id,bank_id,source_question_id) VALUES('00000000-0000-0000-0000-000000000162',NEW.id,'unexpected-question'); INSERT INTO quizcraft_question_versions(id,bank_id,question_id,type,chapter_id,chapter_name,content,options,answer,analysis,content_sha256) VALUES('00000000-0000-0000-0000-000000000163',NEW.id,'00000000-0000-0000-0000-000000000162','single','unexpected','Unexpected','Unexpected target content','["A","B"]'::jsonb,'0'::jsonb,'',repeat('c',64)); INSERT INTO quizcraft_bank_version_questions(bank_id,bank_version_id,question_id,question_version_id,position) VALUES(NEW.id,'00000000-0000-0000-0000-000000000161','00000000-0000-0000-0000-000000000162','00000000-0000-0000-0000-000000000163',99); INSERT INTO quizcraft_banks(id,bank_key,name) VALUES('00000000-0000-0000-0000-000000000164','unexpected-target-bank','Unexpected target bank') ON CONFLICT(bank_key) DO NOTHING; END IF; RETURN NEW; END $$`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `CREATE TRIGGER quizcraft_test_add_unexpected_target_facts AFTER INSERT ON quizcraft_banks FOR EACH ROW EXECUTE FUNCTION quizcraft_test_add_unexpected_target_facts()`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DROP TRIGGER IF EXISTS quizcraft_test_add_unexpected_target_facts ON quizcraft_banks; DROP FUNCTION IF EXISTS quizcraft_test_add_unexpected_target_facts()`)
	})

	report, err := service.RunFullMigration(ctx, quizcraft.LegacySnapshot{
		SourceName:    "legacy-unexpected-target-bank",
		CutoffEventID: 1,
		Banks:         []quizcraft.LegacyBank{{BankKey: "migration-unexpected-" + uuid.NewString(), Document: json.RawMessage(validBank)}},
		Rankings:      json.RawMessage(`[]`),
	})
	if err != nil || report.State != "blocked" || report.ContentReconciled {
		t.Fatalf("unexpected target migration report = %+v / %v", report, err)
	}
	remaining := map[string]bool{
		"unexpected_bank":             true,
		"unexpected_bank_version":     true,
		"unexpected_question":         true,
		"unexpected_question_version": true,
		"unexpected_membership":       true,
	}
	for _, discrepancy := range report.Discrepancies {
		delete(remaining, discrepancy.Code)
	}
	if len(remaining) != 0 {
		t.Fatalf("unexpected target fact discrepancies missing %v from %+v", remaining, report.Discrepancies)
	}
}

func TestFullMigrationPreservesRetiredArchivedFeedbackWithoutInventingAQuestionReference(t *testing.T) {
	pool := migrationPool(t)
	service, err := quizcraft.New(quizcraft.Config{Database: pool})
	if err != nil {
		t.Fatal(err)
	}
	createdAt := time.Date(2026, 6, 10, 1, 2, 3, 0, time.UTC)
	report, err := service.RunFullMigration(context.Background(), quizcraft.LegacySnapshot{
		SourceName:    "legacy-archive",
		CutoffEventID: 7,
		Feedback: []quizcraft.LegacyFeedback{{
			LegacyID: "retired-feedback", BankKey: "retired-bank", QuestionIndex: 45, UserID: "legacy-user", SourcePage: "feedback-page",
			Suggestion: "旧题反馈", Status: "archived", ResolutionNote: "题目已下线", CreatedAt: createdAt,
		}},
		Rankings: json.RawMessage(`[]`),
	})
	if err != nil || report.State != "passed" || report.FeedbackMigratedCount != 1 || report.FeedbackExceptionCount != 0 {
		t.Fatalf("archived feedback migration = %+v / %v", report, err)
	}
	var bankKey, sourceQuestionID, userID, sourcePage, detail, status, note string
	var questionIndex int
	if err := pool.QueryRow(context.Background(), `SELECT bank_key,source_question_id,question_index,legacy_user_id,source_page,detail,legacy_status,resolution_note FROM quizcraft_legacy_feedback_archives WHERE legacy_feedback_id='retired-feedback'`).Scan(&bankKey, &sourceQuestionID, &questionIndex, &userID, &sourcePage, &detail, &status, &note); err != nil {
		t.Fatal(err)
	}
	if bankKey != "retired-bank" || sourceQuestionID != "" || questionIndex != 45 || userID != "legacy-user" || sourcePage != "feedback-page" || detail != "旧题反馈" || status != "archived" || note != "题目已下线" {
		t.Fatalf("archived feedback facts = %q/%q/%d/%q/%q/%q/%q/%q", bankKey, sourceQuestionID, questionIndex, userID, sourcePage, detail, status, note)
	}
	if _, err := pool.Exec(context.Background(), `UPDATE quizcraft_legacy_feedback_archives SET detail='mutated' WHERE legacy_feedback_id='retired-feedback'`); err == nil {
		t.Fatal("retired feedback archive was mutable")
	}
}

func TestArchivedFeedbackEventResolvesAnExistingMissingReferenceException(t *testing.T) {
	pool := migrationPool(t)
	service, err := quizcraft.New(quizcraft.Config{Database: pool})
	if err != nil {
		t.Fatal(err)
	}
	createdAt := time.Date(2026, 6, 10, 1, 2, 3, 0, time.UTC)
	full, err := service.RunFullMigration(context.Background(), quizcraft.LegacySnapshot{
		SourceName: "legacy-archive-event", CutoffEventID: 4,
		Feedback: []quizcraft.LegacyFeedback{{LegacyID: "retired-after-cutoff", BankKey: "removed", QuestionIndex: 9, Suggestion: "旧反馈", Status: "pending", CreatedAt: createdAt}},
		Rankings: json.RawMessage(`[]`),
	})
	if err != nil || full.State != "blocked" || full.FeedbackExceptionCount != 1 {
		t.Fatalf("initial missing reference = %+v / %v", full, err)
	}
	payload := mustJSON(quizcraft.LegacyFeedback{LegacyID: "retired-after-cutoff", BankKey: "removed", QuestionIndex: 9, Suggestion: "旧反馈", Status: "archived", CreatedAt: createdAt})
	catchUp, err := service.ApplyIncrementalEvents(context.Background(), full.RunID, 5, []quizcraft.LegacyEvent{{ID: 5, Type: "feedback.upserted", AggregateKey: "retired-after-cutoff", Payload: payload}})
	if err != nil || !catchUp.Ready || catchUp.ExceptionCount != 0 {
		t.Fatalf("archive event did not resolve exception = %+v / %v", catchUp, err)
	}
	var resolution string
	if err := pool.QueryRow(context.Background(), `SELECT r.resolution FROM quizcraft_migration_exception_resolutions r JOIN quizcraft_migration_exceptions e ON e.id=r.exception_id WHERE e.run_id=$1`, full.RunID).Scan(&resolution); err != nil || resolution != "archive_preserved" {
		t.Fatalf("archive resolution = %q / %v", resolution, err)
	}
}

func TestIncrementalEventsRequireAMonotonicCaughtUpCursor(t *testing.T) {
	pool := migrationPool(t)
	service, err := quizcraft.New(quizcraft.Config{Database: pool})
	if err != nil {
		t.Fatal(err)
	}
	bankKey := "migration-events-" + uuid.NewString()
	full, err := service.RunFullMigration(context.Background(), quizcraft.LegacySnapshot{SourceName: "legacy-events", CutoffEventID: 0, Banks: []quizcraft.LegacyBank{{BankKey: bankKey, Document: json.RawMessage(validBank)}}, Rankings: json.RawMessage(`[]`)})
	if err != nil || full.State != "passed" {
		t.Fatalf("full migration = %+v / %v", full, err)
	}
	feedbackPayload := mustJSON(quizcraft.LegacyFeedback{LegacyID: "event-feedback", BankKey: bankKey, QuestionID: "q0001", Suggestion: "增量反馈", Status: "pending", CreatedAt: time.Now().UTC()})
	if _, err := service.ApplyIncrementalEvents(context.Background(), full.RunID, 2, []quizcraft.LegacyEvent{{ID: 2, Type: "feedback.upserted", AggregateKey: "event-feedback", Payload: feedbackPayload}, {ID: 1, Type: "ranking.changed", AggregateKey: "ranking", Payload: json.RawMessage(`[]`)}}); err == nil {
		t.Fatal("out-of-order incremental events were accepted")
	}
	first, err := service.ApplyIncrementalEvents(context.Background(), full.RunID, 2, []quizcraft.LegacyEvent{{ID: 1, Type: "ranking.changed", AggregateKey: "ranking", Payload: json.RawMessage(`[{"name":"旧榜","correct":10,"total":12}]`)}})
	if err != nil || first.CaughtUp || first.BlockingReason != "incremental_event_lag" || first.CurrentCursor != 1 {
		t.Fatalf("partial catch-up = %+v / %v", first, err)
	}
	second, err := service.ApplyIncrementalEvents(context.Background(), full.RunID, 2, []quizcraft.LegacyEvent{{ID: 2, Type: "feedback.upserted", AggregateKey: "event-feedback", Payload: feedbackPayload}})
	if err != nil || !second.CaughtUp || !second.Ready || second.CurrentCursor != 2 || second.AppliedCount != 1 {
		t.Fatalf("complete catch-up = %+v / %v", second, err)
	}
	var receipts, feedback int
	if err := pool.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM quizcraft_migration_event_receipts WHERE run_id=$1),(SELECT count(*) FROM quizcraft_feedbacks WHERE legacy_feedback_id='event-feedback')`, full.RunID).Scan(&receipts, &feedback); err != nil || receipts != 2 || feedback != 1 {
		t.Fatalf("incremental facts = %d/%d / %v", receipts, feedback, err)
	}
}

func TestOfflineShadowComparisonPersistsImmutableThresholdReport(t *testing.T) {
	pool := practicePool(t)
	report := importPracticeBank(t, pool, "migration-shadow-"+uuid.NewString())
	windowStart := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	windowEnd := windowStart.Add(time.Hour)
	bankID := uuid.MustParse(report.BankID)
	bankVersionID := uuid.MustParse(report.BankVersionID)
	questionID := uuid.MustParse(report.Questions[0].QuestionID)
	sessionID := uuid.New()
	if _, err := pool.Exec(context.Background(), `INSERT INTO quizcraft_practice_sessions(id,bank_id,bank_version_id,actor_key,mode,created_at) VALUES($1,$2,$3,$4,'random',$5)`, sessionID, bankID, bankVersionID, "guest:"+uuid.NewString(), windowStart); err != nil {
		t.Fatal(err)
	}
	for index, outcome := range []string{"match", "mismatch", "legacy_error"} {
		if _, err := pool.Exec(context.Background(), `INSERT INTO quizcraft_shadow_comparisons(id,session_id,question_id,new_response,legacy_response,outcome,detail,compared_at) VALUES($1,$2,$3,'{}','{}',$4,'',$5)`, uuid.New(), sessionID, questionID, outcome, windowStart.Add(time.Duration(index)*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	service, _ := quizcraft.New(quizcraft.Config{Database: pool})
	if _, err := service.EvaluateShadowGate(context.Background(), time.Now().UTC(), time.Now().UTC().Add(time.Hour), 1, 0.1); err == nil {
		t.Fatal("open future shadow window was evaluated")
	}
	gate, err := service.EvaluateShadowGate(context.Background(), windowStart, windowEnd, 3, 0.1)
	if err != nil {
		t.Fatal(err)
	}
	if gate.Decision != "block" || gate.SampleCount != 3 || gate.MismatchCount != 1 || gate.LegacyErrorCount != 1 || gate.MismatchRate < 0.66 || len(gate.Reasons) != 1 || gate.Reasons[0] != "mismatch_threshold_exceeded" {
		t.Fatalf("shadow gate = %+v", gate)
	}
	var decision string
	if err := pool.QueryRow(context.Background(), `SELECT decision FROM quizcraft_shadow_gate_reports WHERE id=$1`, gate.ID).Scan(&decision); err != nil || decision != "block" {
		t.Fatalf("stored shadow gate = %q / %v", decision, err)
	}
	if _, err := pool.Exec(context.Background(), `DELETE FROM quizcraft_shadow_gate_reports WHERE id=$1`, gate.ID); err == nil {
		t.Fatal("shadow gate report was mutable")
	}
}

func TestLegacyPostgresSourceReadsProductionTablesAndEventLog(t *testing.T) {
	pool := practicePool(t)
	ctx := context.Background()
	statements := []string{
		`CREATE TABLE IF NOT EXISTS question_banks(bank_key text PRIMARY KEY,name text NOT NULL,color text NOT NULL,source_file text NOT NULL,metadata jsonb NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS bank_questions(bank_key text NOT NULL,question_id text NOT NULL,payload jsonb NOT NULL,PRIMARY KEY(bank_key,question_id))`,
		`CREATE TABLE IF NOT EXISTS feedbacks(feedback_id bigserial PRIMARY KEY,question_bank text,question_id text,question_index integer NOT NULL,question_content text,user_id text,source_page text NOT NULL,suggestion text NOT NULL,status text NOT NULL,resolution_note text NOT NULL,created_at timestamptz NOT NULL,resolved_at timestamptz)`,
		`CREATE TABLE IF NOT EXISTS users(user_id text PRIMARY KEY,display_name text NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS user_stats(user_id text PRIMARY KEY,correct bigint NOT NULL,total bigint NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS quizcraft_migration_events(event_id bigserial PRIMARY KEY,event_type text NOT NULL,aggregate_key text NOT NULL,source_transaction_id bigint,payload jsonb NOT NULL,occurred_at timestamptz NOT NULL DEFAULT now())`,
		`ALTER TABLE quizcraft_migration_events ADD COLUMN IF NOT EXISTS source_transaction_id bigint`,
		`CREATE UNIQUE INDEX IF NOT EXISTS test_migration_event_transaction_aggregate ON quizcraft_migration_events(source_transaction_id,event_type,aggregate_key) WHERE source_transaction_id IS NOT NULL`,
	}
	for _, statement := range statements {
		if _, err := pool.Exec(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	bankKey := "legacy-source-" + uuid.NewString()
	var document map[string]any
	if err := json.Unmarshal([]byte(validBank), &document); err != nil {
		t.Fatal(err)
	}
	question := document["questions"].([]any)[0].(map[string]any)
	if _, err := pool.Exec(ctx, `INSERT INTO question_banks(bank_key,name,color,source_file,metadata) VALUES($1,'旧题库','#123456','postgresql', $2)`, bankKey, mustJSON(document["meta"])); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO bank_questions(bank_key,question_id,payload) VALUES($1,'q0001',$2)`, bankKey, mustJSON(question)); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO feedbacks(question_bank,question_id,question_index,question_content,user_id,source_page,suggestion,status,resolution_note,created_at) VALUES($1,'q0001',1,'题目正文','legacy-user','quiz','旧反馈','pending','',now())`, bankKey); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO users(user_id,display_name) VALUES('generated-legacy','旧用户')`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO user_stats(user_id,correct,total) VALUES('generated-legacy',3,4)`); err != nil {
		t.Fatal(err)
	}
	triggerSQL := `CREATE OR REPLACE FUNCTION quizcraft_capture_migration_event() RETURNS trigger LANGUAGE plpgsql AS $$
	DECLARE event_type_value text; aggregate_key_value text;
	BEGIN
	  PERFORM pg_advisory_xact_lock(hashtextextended('quizcraft-migration-events',0));
	  IF TG_TABLE_NAME='bank_questions' THEN event_type_value='bank.upserted'; aggregate_key_value=NEW.bank_key;
	  ELSIF TG_TABLE_NAME='feedbacks' THEN event_type_value='feedback.upserted'; aggregate_key_value=NEW.feedback_id::text;
	  ELSE event_type_value='ranking.changed'; aggregate_key_value=NEW.user_id; END IF;
	  INSERT INTO quizcraft_migration_events(source_transaction_id,event_type,aggregate_key,payload) VALUES(txid_current(),event_type_value,aggregate_key_value,jsonb_build_object('changed_table',TG_TABLE_NAME)) ON CONFLICT DO NOTHING;
	  RETURN NULL;
	END $$;
	CREATE TRIGGER test_bank_migration_event AFTER UPDATE ON bank_questions FOR EACH ROW EXECUTE FUNCTION quizcraft_capture_migration_event();
	CREATE TRIGGER test_feedback_migration_event AFTER UPDATE ON feedbacks FOR EACH ROW EXECUTE FUNCTION quizcraft_capture_migration_event();
	CREATE TRIGGER test_ranking_migration_event AFTER UPDATE ON user_stats FOR EACH ROW EXECUTE FUNCTION quizcraft_capture_migration_event();`
	if _, err := pool.Exec(ctx, triggerSQL); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE bank_questions SET payload=jsonb_set(payload,'{analysis}','"direct script repair"') WHERE bank_key=$1`, bankKey); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE feedbacks SET status='resolved',resolution_note='direct script repair',resolved_at=now()`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE user_stats SET correct=4,total=5 WHERE user_id='generated-legacy'`); err != nil {
		t.Fatal(err)
	}
	bulkRepair, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 1000; index++ {
		if _, err := bulkRepair.Exec(ctx, `UPDATE bank_questions SET payload=payload WHERE bank_key=$1`, bankKey); err != nil {
			_ = bulkRepair.Rollback(ctx)
			t.Fatal(err)
		}
	}
	if err := bulkRepair.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var coalescedEvents int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM quizcraft_migration_events WHERE event_type='bank.upserted'`).Scan(&coalescedEvents); err != nil || coalescedEvents != 2 {
		t.Fatalf("1000-row-equivalent bank transaction produced %d markers / %v", coalescedEvents, err)
	}
	source, err := quizcraft.NewLegacyPostgresSource(pool)
	if err != nil {
		t.Fatal(err)
	}
	targetIdentity, err := quizcraft.DatabaseIdentity(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	legacyIdentity, err := source.DatabaseIdentity(ctx)
	if err != nil || targetIdentity == "" || targetIdentity != legacyIdentity {
		t.Fatalf("stable database identity = %q/%q / %v", targetIdentity, legacyIdentity, err)
	}
	snapshot, err := source.Snapshot(ctx, "legacy-test")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.CutoffEventID < 1 || len(snapshot.Banks) != 1 || snapshot.Banks[0].BankKey != bankKey || len(snapshot.Feedback) != 1 || !bytes.Contains(snapshot.Rankings, []byte("generated-legacy")) {
		t.Fatalf("legacy snapshot = %+v", snapshot)
	}
	events, head, err := source.EventsAfter(ctx, 0, 100)
	if err != nil || head != snapshot.CutoffEventID || len(events) != 4 || events[0].Type != "bank.upserted" || !bytes.Contains(events[0].Payload, []byte("direct script repair")) || events[1].Type != "feedback.upserted" || !bytes.Contains(events[1].Payload, []byte("direct script repair")) || events[2].Type != "ranking.changed" || !bytes.Contains(events[2].Payload, []byte("generated-legacy")) || events[3].Type != "bank.upserted" {
		t.Fatalf("legacy events = %+v head=%d / %v", events, head, err)
	}
}

func TestBankEventRetriesFeedbackExceptionsWithoutAFeedbackRewrite(t *testing.T) {
	pool := migrationPool(t)
	service, err := quizcraft.New(quizcraft.Config{Database: pool})
	if err != nil {
		t.Fatal(err)
	}
	bankKey := "migration-repair-" + uuid.NewString()
	full, err := service.RunFullMigration(context.Background(), quizcraft.LegacySnapshot{
		SourceName:    "legacy-bank-repair",
		CutoffEventID: 7,
		Feedback: []quizcraft.LegacyFeedback{{
			LegacyID: "orphan-feedback", BankKey: bankKey, QuestionID: "q0001", Suggestion: "等待题库修复", Status: "pending",
		}},
		Rankings: json.RawMessage(`[]`),
	})
	if err != nil || full.State != "blocked" || full.FeedbackExceptionCount != 1 {
		t.Fatalf("initial exception = %+v / %v", full, err)
	}
	bankPayload := mustJSON(quizcraft.LegacyBank{BankKey: bankKey, Document: json.RawMessage(validBank)})
	catchUp, err := service.ApplyIncrementalEvents(context.Background(), full.RunID, 8, []quizcraft.LegacyEvent{{ID: 8, Type: "bank.upserted", AggregateKey: bankKey, Payload: bankPayload}})
	if err != nil || !catchUp.Ready || catchUp.ExceptionCount != 0 {
		t.Fatalf("bank-only repair did not unblock feedback = %+v / %v", catchUp, err)
	}
	var feedback, resolutions int
	if err := pool.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM quizcraft_feedbacks WHERE legacy_feedback_id='orphan-feedback'),(SELECT count(*) FROM quizcraft_migration_exception_resolutions r JOIN quizcraft_migration_exceptions e ON e.id=r.exception_id WHERE e.run_id=$1)`, full.RunID).Scan(&feedback, &resolutions); err != nil || feedback != 1 || resolutions != 1 {
		t.Fatalf("bank repair facts = %d/%d / %v", feedback, resolutions, err)
	}
}

func TestLegacyEventIDsAreAssignedInCommitOrder(t *testing.T) {
	pool := practicePool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS quizcraft_migration_events(event_id bigserial PRIMARY KEY,event_type text NOT NULL,aggregate_key text NOT NULL,payload jsonb NOT NULL,occurred_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		t.Fatal(err)
	}
	first, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = first.Rollback(ctx) }()
	if _, err := first.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('quizcraft-migration-events',0))`); err != nil {
		t.Fatal(err)
	}
	var firstID int64
	if err := first.QueryRow(ctx, `INSERT INTO quizcraft_migration_events(event_type,aggregate_key,payload) VALUES('ranking.changed','first','{}') RETURNING event_id`).Scan(&firstID); err != nil {
		t.Fatal(err)
	}
	type result struct {
		id  int64
		err error
	}
	secondResult := make(chan result, 1)
	go func() {
		second, beginErr := pool.Begin(ctx)
		if beginErr != nil {
			secondResult <- result{err: beginErr}
			return
		}
		defer func() { _ = second.Rollback(ctx) }()
		if _, lockErr := second.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('quizcraft-migration-events',0))`); lockErr != nil {
			secondResult <- result{err: lockErr}
			return
		}
		var secondID int64
		if insertErr := second.QueryRow(ctx, `INSERT INTO quizcraft_migration_events(event_type,aggregate_key,payload) VALUES('ranking.changed','second','{}') RETURNING event_id`).Scan(&secondID); insertErr != nil {
			secondResult <- result{err: insertErr}
			return
		}
		secondResult <- result{id: secondID, err: second.Commit(ctx)}
	}()
	select {
	case premature := <-secondResult:
		t.Fatalf("second event bypassed the commit-order lock: %+v", premature)
	case <-time.After(100 * time.Millisecond):
	}
	if err := first.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	second := <-secondResult
	if second.err != nil || second.id <= firstID {
		t.Fatalf("commit-ordered IDs = %d then %d / %v", firstID, second.id, second.err)
	}
}

func TestRequireEmptyTargetRejectsFactsInAnyQuizCraftTable(t *testing.T) {
	pool := practicePool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO quizcraft_service_nonces(client_id,key_id,nonce) VALUES('migration-test','key','nonce')`); err != nil {
		t.Fatal(err)
	}
	if err := quizcraft.RequireEmptyTarget(ctx, pool); err == nil {
		t.Fatalf("non-empty target was accepted: %v", err)
	}
}

func TestCutoverWriteGateKeepsReadsOpenAndRejectsMutations(t *testing.T) {
	pool := practicePool(t)
	report := importPracticeBank(t, pool, "cutover-gate-"+uuid.NewString())
	handler, err := quizcraft.NewPracticeHTTP(quizcraft.PracticeHTTPConfig{
		Database:       pool,
		AuthHMACSecret: []byte(practiceAuthSecret),
		WritesDisabled: true,
		ReleaseSHA:     "cutover-test-sha",
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	status, readiness := requestJSON(t, http.MethodGet, server.URL+"/readyz", nil, nil)
	if status != http.StatusOK || !bytes.Contains(readiness, []byte(`"status":"ok"`)) || bytes.Contains(readiness, []byte("release_sha")) {
		t.Fatalf("readiness = %d %s", status, readiness)
	}
	status, _ = requestJSON(t, http.MethodGet, server.URL+"/api/v1/banks", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("read path was gated: %d", status)
	}
	status, body := requestJSON(t, http.MethodPost, server.URL+"/api/v1/practice/sessions", map[string]string{"Idempotency-Key": "cutover-disabled-write-0001"}, map[string]any{
		"bank_id": report.BankID, "bank_version_id": report.BankVersionID, "mode": "random", "question_count": 1,
	})
	if status != http.StatusServiceUnavailable || !bytes.Contains(body, []byte(`"code":"writes_disabled"`)) {
		t.Fatalf("disabled write = %d %s", status, body)
	}
	var sessions int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM quizcraft_practice_sessions WHERE bank_id=$1`, uuid.MustParse(report.BankID)).Scan(&sessions); err != nil || sessions != 0 {
		t.Fatalf("disabled write mutated sessions = %d / %v", sessions, err)
	}
}

func TestAuthenticatedCutoverEvidenceBindsMigrationAndReleaseWithoutTrafficGate(t *testing.T) {
	pool := migrationPool(t)
	service, err := quizcraft.New(quizcraft.Config{Database: pool})
	if err != nil {
		t.Fatal(err)
	}
	full, err := service.RunFullMigration(context.Background(), quizcraft.LegacySnapshot{
		SourceName: "cutover-evidence", CutoffEventID: 19,
		Banks:    []quizcraft.LegacyBank{{BankKey: "cutover-evidence-" + uuid.NewString(), Document: json.RawMessage(validBank)}},
		Rankings: json.RawMessage(`[]`),
	})
	if err != nil || full.State != "passed" {
		t.Fatalf("migration evidence = %+v / %v", full, err)
	}
	secret := "cutover-evidence-secret-at-least-32-bytes"
	handler, err := quizcraft.NewPracticeHTTP(quizcraft.PracticeHTTPConfig{Database: pool, AuthHMACSecret: []byte(practiceAuthSecret), WritesDisabled: true, ReleaseSHA: "actual-binary-sha", CutoverEvidenceSecret: []byte(secret)})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	evidenceURL := fmt.Sprintf("%s/api/v1/cutover-evidence?run_id=%s&source_head=19", server.URL, full.RunID)
	status, _ := requestJSON(t, http.MethodGet, evidenceURL, nil, nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated evidence = %d", status)
	}
	status, body := requestJSON(t, http.MethodGet, evidenceURL, map[string]string{"X-QuizCraft-Cutover-Secret": secret}, nil)
	if status != http.StatusOK || !bytes.Contains(body, []byte(full.RunID.String())) || !bytes.Contains(body, []byte(`"release_sha":"actual-binary-sha"`)) || !bytes.Contains(body, []byte(`"writes_enabled":false`)) || bytes.Contains(body, []byte("shadow_gate")) {
		t.Fatalf("cutover evidence = %d %s", status, body)
	}
}
