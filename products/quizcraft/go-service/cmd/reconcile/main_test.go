package main

import (
	"context"
	"errors"
	"fmt"
	"net/url"
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

	t.Setenv("QUIZCRAFT_V2_DATABASE_URL", targetURL)
	t.Setenv("LEGACY_DATABASE_URL", legacyURL)
	snapshotPath := filepath.Join(t.TempDir(), "legacy-snapshot.json")
	if err := run(ctx, []string{"-mode", "snapshot", "-source-name", "cli-recovery-source", "-snapshot-file", snapshotPath}); err != nil {
		t.Fatalf("snapshot CLI: %v", err)
	}
	if _, err := target.Exec(ctx, `CREATE OR REPLACE FUNCTION quizcraft_test_fail_second_bank() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.bank_key='cli-z-failing-bank' THEN RAISE EXCEPTION 'injected partial import failure'; END IF; RETURN NEW; END $$`); err != nil {
		t.Fatal(err)
	}
	if _, err := target.Exec(ctx, `CREATE TRIGGER quizcraft_test_fail_second_bank BEFORE INSERT ON quizcraft_banks FOR EACH ROW EXECUTE FUNCTION quizcraft_test_fail_second_bank()`); err != nil {
		t.Fatal(err)
	}

	err = run(ctx, []string{"-mode", "full", "-snapshot-file", snapshotPath})
	if err == nil || !strings.Contains(err.Error(), "import legacy bank") {
		t.Fatalf("full CLI did not expose the injected partial-import failure: %v", err)
	}
	var runID, state, firstBankID, firstVersionID string
	if err := target.QueryRow(ctx, `SELECT id::text,state FROM quizcraft_migration_runs`).Scan(&runID, &state); err != nil {
		t.Fatal(err)
	}
	if state != "blocked" {
		t.Fatalf("partial migration state = %q, want blocked", state)
	}
	if err := target.QueryRow(ctx, `SELECT id::text,active_version_id::text FROM quizcraft_banks WHERE bank_key='cli-good-bank'`).Scan(&firstBankID, &firstVersionID); err != nil {
		t.Fatal(err)
	}
	if _, err := target.Exec(ctx, `DROP TRIGGER quizcraft_test_fail_second_bank ON quizcraft_banks; DROP FUNCTION quizcraft_test_fail_second_bank()`); err != nil {
		t.Fatal(err)
	}
	if err := run(ctx, []string{"-mode", "resume", "-run-id", runID, "-snapshot-file", snapshotPath}); err != nil {
		t.Fatalf("resume CLI: %v", err)
	}

	var resumedState, resumedBankID, resumedVersionID string
	if err := target.QueryRow(ctx, `SELECT state FROM quizcraft_migration_runs WHERE id=$1`, runID).Scan(&resumedState); err != nil {
		t.Fatal(err)
	}
	if resumedState != "passed" {
		t.Fatalf("resumed migration state = %q", resumedState)
	}
	if err := target.QueryRow(ctx, `SELECT id::text,active_version_id::text FROM quizcraft_banks WHERE bank_key='cli-good-bank'`).Scan(&resumedBankID, &resumedVersionID); err != nil {
		t.Fatal(err)
	}
	if resumedBankID != firstBankID || resumedVersionID != firstVersionID {
		t.Fatalf("resume changed stable good-bank identities: %s/%s became %s/%s", firstBankID, firstVersionID, resumedBankID, resumedVersionID)
	}
	var banks, questions, practiceFacts int
	if err := target.QueryRow(ctx, `SELECT (SELECT count(*) FROM quizcraft_banks),(SELECT count(*) FROM quizcraft_questions),(SELECT count(*) FROM quizcraft_practice_sessions)+(SELECT count(*) FROM quizcraft_practice_attempts)`).Scan(&banks, &questions, &practiceFacts); err != nil {
		t.Fatal(err)
	}
	if banks != 2 || questions != 2 || practiceFacts != 0 {
		t.Fatalf("CLI recovery target facts = banks:%d questions:%d practice:%d", banks, questions, practiceFacts)
	}
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
