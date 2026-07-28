package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	quizcraft "henukit.dev/quizcraft"
)

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("closed output") }

func TestPrintJSONPropagatesOutputErrors(t *testing.T) {
	if err := printJSON(failingWriter{}, map[string]bool{"ready": true}); err == nil {
		t.Fatal("stdout write failure was ignored")
	}
}

func TestRunRejectsLegacyTargetURLBeforeOpeningIt(t *testing.T) {
	t.Setenv("QUIZCRAFT_V2_DATABASE_URL", "postgres://quizcraft:secret@localhost/quizcraft_legacy?sslmode=disable")
	if err := run(context.Background(), []string{"-mode", "full", "-snapshot-file", "ignored.json"}); err == nil || !strings.Contains(err.Error(), "quizcraft_v2") {
		t.Fatalf("legacy target URL error = %v", err)
	}
}

func TestReconcileCLIBlocksARealPartialImportThenResumesTheSameRun(t *testing.T) {
	ctx := context.Background()
	binary := buildReconcileCLIBinary(t)
	adminURL := startReconcilePostgres(t, ctx)
	targetURL := reconcileDatabaseURL(t, adminURL, quizcraft.QuizcraftV2DatabaseName)
	legacyURL := reconcileDatabaseURL(t, adminURL, "quizcraft_legacy")
	admin, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = admin.Close(context.Background()) }()
	for _, name := range []string{quizcraft.QuizcraftV2DatabaseName, "quizcraft_legacy"} {
		if _, err := admin.Exec(ctx, `CREATE DATABASE `+pgx.Identifier{name}.Sanitize()); err != nil {
			t.Fatal(err)
		}
	}

	target, err := pgxpool.New(ctx, targetURL)
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	if _, err := quizcraft.ApplyVersionedMigrations(ctx, target, "../../db/migrations"); err != nil {
		t.Fatal(err)
	}
	legacy, err := pgx.Connect(ctx, legacyURL)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = legacy.Close(context.Background()) }()
	seedLegacyCLIImportFixture(t, ctx, legacy)

	snapshotPath := filepath.Join(t.TempDir(), "legacy-snapshot.json")
	snapshot := invokeReconcileCLI(ctx, binary, targetURL, legacyURL, "-mode", "snapshot", "-source-name", "cli-recovery-source", "-snapshot-file", snapshotPath)
	if snapshot.Err != nil {
		t.Fatalf("snapshot CLI: %v\nstderr:\n%s", snapshot.Err, snapshot.Stderr)
	}
	var artifact quizcraft.LegacySnapshotArtifact
	if err := json.Unmarshal([]byte(snapshot.Stdout), &artifact); err != nil {
		t.Fatalf("snapshot CLI did not emit an artifact: %v\nstdout:\n%s", err, snapshot.Stdout)
	}
	if artifact.Snapshot.SourceName != "cli-recovery-source" || artifact.SnapshotSHA256 == "" {
		t.Fatalf("snapshot CLI artifact = %+v", artifact)
	}
	seedPostSnapshotCLICatchUpEvent(t, ctx, legacy)
	if _, err := target.Exec(ctx, `CREATE OR REPLACE FUNCTION quizcraft_test_fail_second_bank() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.bank_key='cli-z-failing-bank' THEN RAISE EXCEPTION 'injected partial import failure'; END IF; RETURN NEW; END $$`); err != nil {
		t.Fatal(err)
	}
	if _, err := target.Exec(ctx, `CREATE TRIGGER quizcraft_test_fail_second_bank BEFORE INSERT ON quizcraft_banks FOR EACH ROW EXECUTE FUNCTION quizcraft_test_fail_second_bank()`); err != nil {
		t.Fatal(err)
	}

	full := invokeReconcileCLI(ctx, binary, targetURL, legacyURL, "-mode", "full", "-snapshot-file", snapshotPath)
	if full.Err == nil {
		t.Fatal("full CLI succeeded despite injected partial-import failure")
	}
	if _, ok := full.Err.(*exec.ExitError); !ok {
		t.Fatalf("full CLI error = %T %v, want exit error", full.Err, full.Err)
	}
	if !strings.Contains(full.Stderr, "QuizCraft reconciliation gate failed") {
		t.Fatalf("full CLI stderr = %q", full.Stderr)
	}
	var blocked quizcraft.MigrationReport
	if err := json.Unmarshal([]byte(full.Stdout), &blocked); err != nil {
		t.Fatalf("blocked full CLI did not emit a migration report: %v\nstdout:\n%s", err, full.Stdout)
	}
	if blocked.RunID.String() == "00000000-0000-0000-0000-000000000000" || blocked.State != "blocked" || !strings.Contains(strings.Join(blocked.Differences, ","), "migration_execution_failed") {
		t.Fatalf("blocked full CLI report = %+v", blocked)
	}
	var state string
	var attempts int
	if err := target.QueryRow(ctx, `SELECT state,resume_attempt_count FROM quizcraft_migration_runs WHERE id=$1`, blocked.RunID).Scan(&state, &attempts); err != nil {
		t.Fatal(err)
	}
	if state != "blocked" || attempts != 1 {
		t.Fatalf("partial migration audit facts = state:%q resume_attempt_count:%d", state, attempts)
	}
	first := importedCLIContentIdentity(t, ctx, target, "cli-good-bank", "cli-q1")
	if _, err := target.Exec(ctx, `DROP TRIGGER quizcraft_test_fail_second_bank ON quizcraft_banks; DROP FUNCTION quizcraft_test_fail_second_bank()`); err != nil {
		t.Fatal(err)
	}
	resumed := invokeReconcileCLI(ctx, binary, targetURL, legacyURL, "-mode", "resume", "-run-id", blocked.RunID.String(), "-snapshot-file", snapshotPath)
	if resumed.Err != nil {
		t.Fatalf("resume CLI: %v\nstderr:\n%s", resumed.Err, resumed.Stderr)
	}
	var resumedReport quizcraft.MigrationReport
	if err := json.Unmarshal([]byte(resumed.Stdout), &resumedReport); err != nil {
		t.Fatalf("resume CLI did not emit a migration report: %v\nstdout:\n%s", err, resumed.Stdout)
	}
	if resumedReport.RunID != blocked.RunID || resumedReport.State != "passed" || !resumedReport.ContentReconciled || len(resumedReport.Differences) != 0 || resumedReport.FeedbackExceptionCount != 0 {
		t.Fatalf("resumed CLI report = %+v", resumedReport)
	}
	if err := target.QueryRow(ctx, `SELECT state,resume_attempt_count FROM quizcraft_migration_runs WHERE id=$1`, blocked.RunID).Scan(&state, &attempts); err != nil {
		t.Fatal(err)
	}
	if state != "passed" || attempts != 2 {
		t.Fatalf("resumed migration audit facts = state:%q resume_attempt_count:%d", state, attempts)
	}
	if resumedIdentity := importedCLIContentIdentity(t, ctx, target, "cli-good-bank", "cli-q1"); resumedIdentity != first {
		t.Fatalf("resume changed stable good-bank identities: %+v became %+v", first, resumedIdentity)
	}
	catchUp := invokeReconcileCLI(ctx, binary, targetURL, legacyURL, "-mode", "catch-up", "-run-id", blocked.RunID.String())
	if catchUp.Err != nil {
		t.Fatalf("catch-up CLI: %v\nstderr:\n%s", catchUp.Err, catchUp.Stderr)
	}
	var catchUpReport quizcraft.CatchUpReport
	if err := json.Unmarshal([]byte(catchUp.Stdout), &catchUpReport); err != nil {
		t.Fatalf("catch-up CLI did not emit a report: %v\nstdout:\n%s", err, catchUp.Stdout)
	}
	if catchUpReport.RunID != blocked.RunID || catchUpReport.PreviousCursor != 1 || catchUpReport.CurrentCursor != 2 || catchUpReport.SourceHead != 2 || catchUpReport.AppliedCount != 1 || !catchUpReport.CaughtUp || !catchUpReport.Ready || catchUpReport.ExceptionCount != 0 {
		t.Fatalf("catch-up CLI report = %+v", catchUpReport)
	}
	caughtUp := importedCLIContentIdentity(t, ctx, target, "cli-catch-up-bank", "cli-catch-up-q1")
	if caughtUp.BankID == first.BankID || caughtUp.QuestionID == first.QuestionID {
		t.Fatalf("catch-up CLI did not create distinct target content: %+v", caughtUp)
	}
	var caughtUpCursor int64
	if err := target.QueryRow(ctx, `SELECT caught_up_event_id FROM quizcraft_migration_runs WHERE id=$1`, blocked.RunID).Scan(&caughtUpCursor); err != nil {
		t.Fatal(err)
	}
	if caughtUpCursor != 2 {
		t.Fatalf("catch-up cursor = %d, want 2", caughtUpCursor)
	}
	var receiptRunID, receiptType, receiptKey string
	if err := target.QueryRow(ctx, `SELECT run_id::text,event_type,aggregate_key FROM quizcraft_migration_event_receipts WHERE source_name='cli-recovery-source' AND source_event_id=2`).Scan(&receiptRunID, &receiptType, &receiptKey); err != nil {
		t.Fatal(err)
	}
	if receiptRunID != blocked.RunID.String() || receiptType != "bank.upserted" || receiptKey != "cli-catch-up-bank" {
		t.Fatalf("catch-up receipt = %q/%q/%q", receiptRunID, receiptType, receiptKey)
	}

	var banks, bankVersions, questions, questionVersions, memberships, unexplainedExceptions, practiceFacts int
	if err := target.QueryRow(ctx, `SELECT
  (SELECT count(*) FROM quizcraft_banks),
  (SELECT count(*) FROM quizcraft_bank_versions),
  (SELECT count(*) FROM quizcraft_questions),
  (SELECT count(*) FROM quizcraft_question_versions),
  (SELECT count(*) FROM quizcraft_bank_version_questions),
  (SELECT count(*) FROM quizcraft_migration_exceptions e LEFT JOIN quizcraft_migration_exception_resolutions r ON r.exception_id=e.id WHERE e.run_id=$1 AND r.exception_id IS NULL),
  (SELECT count(*) FROM quizcraft_practice_sessions) +
  (SELECT count(*) FROM quizcraft_practice_session_claims) +
  (SELECT count(*) FROM quizcraft_practice_session_questions) +
  (SELECT count(*) FROM quizcraft_practice_attempts) +
  (SELECT count(*) FROM quizcraft_learning_state) +
  (SELECT count(*) FROM quizcraft_idempotency_results) +
  (SELECT count(*) FROM quizcraft_question_stats)`, blocked.RunID).Scan(&banks, &bankVersions, &questions, &questionVersions, &memberships, &unexplainedExceptions, &practiceFacts); err != nil {
		t.Fatal(err)
	}
	if banks != 3 || bankVersions != 3 || questions != 3 || questionVersions != 3 || memberships != 3 || unexplainedExceptions != 0 || practiceFacts != 0 {
		t.Fatalf("CLI recovery target facts = banks:%d bank_versions:%d questions:%d question_versions:%d memberships:%d unexplained_exceptions:%d practice:%d", banks, bankVersions, questions, questionVersions, memberships, unexplainedExceptions, practiceFacts)
	}
}

type cliContentIdentity struct {
	BankID            string
	BankVersionID     string
	QuestionID        string
	QuestionVersionID string
}

func importedCLIContentIdentity(t *testing.T, ctx context.Context, target *pgxpool.Pool, bankKey, sourceQuestionID string) cliContentIdentity {
	t.Helper()
	var identity cliContentIdentity
	if err := target.QueryRow(ctx, `SELECT b.id::text,b.active_version_id::text,q.id::text,m.question_version_id::text
FROM quizcraft_banks b
JOIN quizcraft_questions q ON q.bank_id=b.id AND q.source_question_id=$2
JOIN quizcraft_bank_version_questions m ON m.bank_id=b.id AND m.bank_version_id=b.active_version_id AND m.question_id=q.id
WHERE b.bank_key=$1`, bankKey, sourceQuestionID).Scan(&identity.BankID, &identity.BankVersionID, &identity.QuestionID, &identity.QuestionVersionID); err != nil {
		t.Fatal(err)
	}
	return identity
}

type reconcileCLIResult struct {
	Stdout string
	Stderr string
	Err    error
}

func buildReconcileCLIBinary(t *testing.T) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "quizcraft-reconcile")
	command := exec.Command("go", "build", "-o", binary, ".")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("build reconcile CLI: %v\n%s", err, output)
	}
	return binary
}

func invokeReconcileCLI(ctx context.Context, binary, targetURL, legacyURL string, arguments ...string) reconcileCLIResult {
	command := exec.CommandContext(ctx, binary, arguments...)
	command.Env = append(os.Environ(), "QUIZCRAFT_V2_DATABASE_URL="+targetURL, "LEGACY_DATABASE_URL="+legacyURL)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	return reconcileCLIResult{Stdout: stdout.String(), Stderr: stderr.String(), Err: err}
}

func startReconcilePostgres(t *testing.T, ctx context.Context) string {
	t.Helper()
	container, err := testcontainers.Run(ctx, "postgres:16-alpine",
		testcontainers.WithEnv(map[string]string{"POSTGRES_DB": "postgres", "POSTGRES_USER": "quizcraft", "POSTGRES_PASSWORD": "quizcraft"}),
		testcontainers.WithExposedPorts("5432/tcp"),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(60*time.Second)),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = testcontainers.TerminateContainer(container) })
	host, err := container.Host(ctx)
	if err != nil {
		t.Fatal(err)
	}
	port, err := container.MappedPort(ctx, "5432/tcp")
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("postgres://quizcraft:quizcraft@%s:%s/postgres?sslmode=disable", host, port.Port())
}

func reconcileDatabaseURL(t *testing.T, adminURL, database string) string {
	t.Helper()
	parsed, err := url.Parse(adminURL)
	if err != nil {
		t.Fatal(err)
	}
	parsed.Path = "/" + database
	return parsed.String()
}

func seedLegacyCLIImportFixture(t *testing.T, ctx context.Context, database *pgx.Conn) {
	t.Helper()
	for _, statement := range []string{
		`CREATE TABLE quizcraft_migration_events(event_id bigint PRIMARY KEY,event_type text NOT NULL,aggregate_key text NOT NULL,payload jsonb NOT NULL)`,
		`CREATE TABLE question_banks(bank_key text PRIMARY KEY,name text NOT NULL,color text NOT NULL,source_file text NOT NULL,metadata jsonb NOT NULL)`,
		`CREATE TABLE bank_questions(bank_key text NOT NULL,question_id text NOT NULL,payload jsonb NOT NULL,PRIMARY KEY(bank_key,question_id))`,
		`CREATE TABLE feedbacks(feedback_id uuid PRIMARY KEY,question_bank text,question_id text,question_index integer NOT NULL,question_content text,user_id text,source_page text NOT NULL,suggestion text NOT NULL,status text NOT NULL,resolution_note text NOT NULL,created_at timestamptz NOT NULL,resolved_at timestamptz)`,
		`CREATE TABLE users(user_id text PRIMARY KEY,display_name text NOT NULL)`,
		`CREATE TABLE user_stats(user_id text PRIMARY KEY,correct bigint NOT NULL,total bigint NOT NULL)`,
	} {
		if _, err := database.Exec(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	question := `{"id":"cli-q1","type":"single","chapter_id":"intro","chapter":"Introduction","content":"Which value is correct?","options":["A","B"],"answer":1,"analysis":""}`
	for _, bankKey := range []string{"cli-good-bank", "cli-z-failing-bank"} {
		if _, err := database.Exec(ctx, `INSERT INTO question_banks(bank_key,name,color,source_file,metadata) VALUES($1,$2,'#2563eb','cli.json','{}'::jsonb)`, bankKey, bankKey); err != nil {
			t.Fatal(err)
		}
		if _, err := database.Exec(ctx, `INSERT INTO bank_questions(bank_key,question_id,payload) VALUES($1,'cli-q1',$2::jsonb)`, bankKey, question); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := database.Exec(ctx, `INSERT INTO quizcraft_migration_events(event_id,event_type,aggregate_key,payload) VALUES(1,'bank.upserted','cli-good-bank','{}'::jsonb)`); err != nil {
		t.Fatal(err)
	}
}

func seedPostSnapshotCLICatchUpEvent(t *testing.T, ctx context.Context, database *pgx.Conn) {
	t.Helper()
	const bankKey = "cli-catch-up-bank"
	question := `{"id":"cli-catch-up-q1","type":"single","chapter_id":"catch-up","chapter":"Catch up","content":"Which event arrived after the snapshot?","options":["A","B"],"answer":1,"analysis":""}`
	if _, err := database.Exec(ctx, `INSERT INTO question_banks(bank_key,name,color,source_file,metadata) VALUES($1,'CLI catch-up bank','#2563eb','cli-catch-up.json','{}'::jsonb)`, bankKey); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(ctx, `INSERT INTO bank_questions(bank_key,question_id,payload) VALUES($1,'cli-catch-up-q1',$2::jsonb)`, bankKey, question); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(ctx, `INSERT INTO quizcraft_migration_events(event_id,event_type,aggregate_key,payload) VALUES(2,'bank.upserted',$1,'{"after_snapshot":true}'::jsonb)`, bankKey); err != nil {
		t.Fatal(err)
	}
}
