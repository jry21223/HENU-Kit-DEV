package tests

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	quizcraft "henukit.dev/quizcraft"
)

func TestVersionedMigrationArtifactsApplyOnceAndRejectChangedSource(t *testing.T) {
	pool := isolatedArtifactDatabase(t)
	ctx := context.Background()

	first, err := quizcraft.ApplyVersionedMigrations(ctx, pool, "../db/migrations")
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Applied) == 0 || len(first.Skipped) != 0 {
		t.Fatalf("first migration report = %+v", first)
	}
	var historyCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM quizcraft_schema_migrations`).Scan(&historyCount); err != nil {
		t.Fatal(err)
	}
	if historyCount != len(first.Applied) {
		t.Fatalf("migration history count = %d, want %d", historyCount, len(first.Applied))
	}

	second, err := quizcraft.ApplyVersionedMigrations(ctx, pool, "../db/migrations")
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Applied) != 0 || len(second.Skipped) != historyCount {
		t.Fatalf("repeated migration report = %+v", second)
	}

	downSource, err := os.ReadFile("../db/migrations/000011_learning_state_latest_attempt.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(downSource)); err != nil {
		t.Fatalf("roll back tracked migration: %v", err)
	}
	reapplied, err := quizcraft.ApplyVersionedMigrations(ctx, pool, "../db/migrations")
	if err != nil {
		t.Fatalf("reapply tracked migration after rollback: %v", err)
	}
	if len(reapplied.Applied) != 1 || reapplied.Applied[0].Version != "000011" || len(reapplied.Skipped) != historyCount-1 {
		t.Fatalf("reapplied migration report = %+v", reapplied)
	}

	probeDir := t.TempDir()
	probePath := filepath.Join(probeDir, "000099_artifact_probe.up.sql")
	if err := os.WriteFile(probePath, []byte(`CREATE TABLE quizcraft_artifact_probe(id integer PRIMARY KEY);`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := quizcraft.ApplyVersionedMigrations(ctx, pool, probeDir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(probePath, []byte(`CREATE TABLE quizcraft_artifact_probe(id integer PRIMARY KEY, changed boolean NOT NULL DEFAULT false);`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := quizcraft.ApplyVersionedMigrations(ctx, pool, probeDir); err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("changed migration source was accepted: %v", err)
	}
}

func TestVersionedMigrationArtifactsAdoptTheReleasedPreHistoryBaseline(t *testing.T) {
	pool := isolatedArtifactDatabase(t)
	ctx := context.Background()
	applyReleasedPreHistoryBaseline(t, pool)

	report, err := quizcraft.ApplyVersionedMigrations(ctx, pool, "../db/migrations")
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Adopted) != 8 || len(report.Applied) != 3 || report.Applied[0].Version != "000009" || report.Applied[1].Version != "000010" || report.Applied[2].Version != "000011" || len(report.Skipped) != 0 {
		t.Fatalf("pre-history adoption report = %+v", report)
	}
	var historyCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM quizcraft_schema_migrations`).Scan(&historyCount); err != nil {
		t.Fatal(err)
	}
	if historyCount != 11 {
		t.Fatalf("adopted migration history count = %d, want 11", historyCount)
	}
}

func TestVersionedMigrationArtifactsRefuseACompleteLookingBaselineMissingOneRelation(t *testing.T) {
	pool := isolatedArtifactDatabase(t)
	ctx := context.Background()
	applyReleasedPreHistoryBaseline(t, pool)
	if _, err := pool.Exec(ctx, `DROP TABLE quizcraft_question_versions CASCADE`); err != nil {
		t.Fatal(err)
	}
	if _, err := quizcraft.ApplyVersionedMigrations(ctx, pool, "../db/migrations"); err == nil || !strings.Contains(err.Error(), "partial pre-history") {
		t.Fatalf("baseline missing question versions was accepted: %v", err)
	}
	var history *string
	if err := pool.QueryRow(ctx, `SELECT to_regclass('public.quizcraft_schema_migrations')`).Scan(&history); err != nil {
		t.Fatal(err)
	}
	if history != nil {
		t.Fatalf("incomplete baseline left migration history relation %q", *history)
	}
}

func TestVersionedMigrationArtifactsRefuseDamagedPreHistorySchemaBeforeCreatingHistory(t *testing.T) {
	tests := []struct {
		name   string
		mutate string
	}{
		{name: "missing column", mutate: `ALTER TABLE quizcraft_banks DROP COLUMN name`},
		{name: "missing constraint", mutate: `ALTER TABLE quizcraft_banks DROP CONSTRAINT quizcraft_banks_bank_key_check`},
		{name: "missing index", mutate: `DROP INDEX quizcraft_bank_version_questions_order_idx`},
		{name: "missing trigger", mutate: `DROP TRIGGER quizcraft_question_versions_immutable ON quizcraft_question_versions`},
		{name: "changed trigger function", mutate: `CREATE OR REPLACE FUNCTION quizcraft_reject_immutable_mutation() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RETURN NEW; END; $$`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pool := isolatedArtifactDatabase(t)
			ctx := context.Background()
			applyReleasedPreHistoryBaseline(t, pool)
			if _, err := pool.Exec(ctx, test.mutate); err != nil {
				t.Fatal(err)
			}
			if _, err := quizcraft.ApplyVersionedMigrations(ctx, pool, "../db/migrations"); err == nil || !strings.Contains(err.Error(), "partial pre-history") {
				t.Fatalf("damaged pre-history baseline was adopted: %v", err)
			}
			var history *string
			if err := pool.QueryRow(ctx, `SELECT to_regclass('public.quizcraft_schema_migrations')`).Scan(&history); err != nil {
				t.Fatal(err)
			}
			if history != nil {
				t.Fatalf("damaged baseline left migration history relation %q", *history)
			}
		})
	}
}

func TestVersionedMigrationArtifactsAdoptTheReleasedBaselineWithUnrelatedSchemaObjects(t *testing.T) {
	pool := isolatedArtifactDatabase(t)
	ctx := context.Background()
	applyReleasedPreHistoryBaseline(t, pool)
	if _, err := pool.Exec(ctx, `CREATE TABLE operator_notes(id bigint PRIMARY KEY,note text NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	report, err := quizcraft.ApplyVersionedMigrations(ctx, pool, "../db/migrations")
	if err != nil || len(report.Adopted) != 8 || len(report.Applied) != 3 || report.Applied[0].Version != "000009" || report.Applied[1].Version != "000010" || report.Applied[2].Version != "000011" {
		t.Fatalf("baseline adoption with unrelated object = %+v / %v", report, err)
	}
}

func TestVersionedMigrationArtifactsRefusePartialPreHistorySchemaWithoutCreatingHistory(t *testing.T) {
	pool := isolatedArtifactDatabase(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `CREATE TABLE quizcraft_banks(id uuid PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	if _, err := quizcraft.ApplyVersionedMigrations(ctx, pool, "../db/migrations"); err == nil || !strings.Contains(err.Error(), "partial pre-history") {
		t.Fatalf("partial legacy schema was accepted: %v", err)
	}
	var history *string
	if err := pool.QueryRow(ctx, `SELECT to_regclass('public.quizcraft_schema_migrations')`).Scan(&history); err != nil {
		t.Fatal(err)
	}
	if history != nil {
		t.Fatalf("partial schema left migration history relation %q", *history)
	}
}

func TestVersionedMigrationArtifactsRefuseAnOmittedLegacyRelationWithoutCreatingHistory(t *testing.T) {
	pool := isolatedArtifactDatabase(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `CREATE TABLE quizcraft_question_versions(id uuid PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	if _, err := quizcraft.ApplyVersionedMigrations(ctx, pool, "../db/migrations"); err == nil || !strings.Contains(err.Error(), "partial pre-history") {
		t.Fatalf("omitted legacy relation was accepted: %v", err)
	}
	var history *string
	if err := pool.QueryRow(ctx, `SELECT to_regclass('public.quizcraft_schema_migrations')`).Scan(&history); err != nil {
		t.Fatal(err)
	}
	if history != nil {
		t.Fatalf("omitted relation left migration history relation %q", *history)
	}
}

func TestQuizcraftV2TargetURLRejectsLegacyDatabase(t *testing.T) {
	if err := quizcraft.RequireQuizcraftV2DatabaseURL("postgres://quizcraft:secret@localhost/quizcraft_legacy?sslmode=disable"); err == nil {
		t.Fatal("legacy database URL was accepted as a QuizCraft V2 target")
	}
	if err := quizcraft.RequireQuizcraftV2DatabaseURL("postgres://quizcraft:secret@localhost/quizcraft_v2?sslmode=disable"); err != nil {
		t.Fatalf("quizcraft_v2 database URL was rejected: %v", err)
	}
}

func TestSnapshotArtifactMakesAStoppedMigrationSafelyResumable(t *testing.T) {
	pool := migrationPool(t)
	ctx := context.Background()
	service, err := quizcraft.New(quizcraft.Config{Database: pool})
	if err != nil {
		t.Fatal(err)
	}
	bankKey := "resume-artifact-" + strings.ReplaceAll(uuid.NewString(), "-", "")
	snapshot := quizcraft.LegacySnapshot{
		SourceName:    "representative-production-snapshot",
		CutoffEventID: 41,
		Banks:         []quizcraft.LegacyBank{{BankKey: bankKey, Document: json.RawMessage(validBank)}},
		Rankings:      json.RawMessage(`[]`),
	}
	path := filepath.Join(t.TempDir(), "quizcraft-v2.snapshot.json")
	artifact, err := quizcraft.WriteLegacySnapshotArtifact(path, "legacy-system/41", snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("snapshot artifact permissions = %v / %v", info.Mode().Perm(), err)
	}
	reloaded, err := quizcraft.ReadLegacySnapshotArtifact(path)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.SnapshotSHA256 != artifact.SnapshotSHA256 || reloaded.SourceDatabaseIdentity != "legacy-system/41" {
		t.Fatalf("snapshot artifact = %+v", reloaded)
	}

	first, err := service.RunFullMigration(ctx, reloaded.Snapshot)
	if err != nil || first.State != "passed" {
		t.Fatalf("initial migration = %+v / %v", first, err)
	}
	var banks, questions int
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM quizcraft_banks WHERE bank_key=$1),(SELECT count(*) FROM quizcraft_questions q JOIN quizcraft_banks b ON b.id=q.bank_id WHERE b.bank_key=$1)`, bankKey).Scan(&banks, &questions); err != nil {
		t.Fatal(err)
	}

	// This is the durable state a process crash can leave after content import but
	// before its final report is committed. The public resume boundary must make
	// the same snapshot safe to run again without duplicating immutable facts.
	if _, err := pool.Exec(ctx, `UPDATE quizcraft_migration_runs SET state='running',completed_at=NULL WHERE id=$1`, first.RunID); err != nil {
		t.Fatal(err)
	}
	resumed, err := service.ResumeFullMigration(ctx, first.RunID, reloaded.Snapshot)
	if err != nil || resumed.State != "passed" || resumed.RunID != first.RunID || resumed.SourceSnapshotSHA256 != artifact.SnapshotSHA256 {
		t.Fatalf("resumed migration = %+v / %v", resumed, err)
	}
	var resumedBanks, resumedQuestions int
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM quizcraft_banks WHERE bank_key=$1),(SELECT count(*) FROM quizcraft_questions q JOIN quizcraft_banks b ON b.id=q.bank_id WHERE b.bank_key=$1)`, bankKey).Scan(&resumedBanks, &resumedQuestions); err != nil {
		t.Fatal(err)
	}
	if resumedBanks != banks || resumedQuestions != questions {
		t.Fatalf("resume duplicated target facts: %d/%d became %d/%d", banks, questions, resumedBanks, resumedQuestions)
	}
	var practiceAttempts int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM quizcraft_practice_attempts a JOIN quizcraft_banks b ON b.id=a.bank_id WHERE b.bank_key=$1`, bankKey).Scan(&practiceAttempts); err != nil {
		t.Fatal(err)
	}
	if practiceAttempts != 0 {
		t.Fatalf("content migration created V2 practice attempts: %d", practiceAttempts)
	}

	tampered := reloaded.Snapshot
	tampered.CutoffEventID++
	if _, err := service.ResumeFullMigration(ctx, first.RunID, tampered); err == nil || !strings.Contains(err.Error(), "snapshot") {
		t.Fatalf("changed snapshot was accepted for resume: %v", err)
	}
}

func TestSnapshotArtifactResumesAfterARealPartialImportFailure(t *testing.T) {
	pool := migrationPool(t)
	ctx := context.Background()
	service, err := quizcraft.New(quizcraft.Config{Database: pool})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `CREATE OR REPLACE FUNCTION quizcraft_test_fail_second_migration_bank() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.name='force-resume-failure' THEN RAISE EXCEPTION 'intentional partial import failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER quizcraft_test_fail_second_migration_bank BEFORE INSERT ON quizcraft_banks FOR EACH ROW EXECUTE FUNCTION quizcraft_test_fail_second_migration_bank()`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DROP TRIGGER IF EXISTS quizcraft_test_fail_second_migration_bank ON quizcraft_banks; DROP FUNCTION IF EXISTS quizcraft_test_fail_second_migration_bank()`)
	})

	var failingDocument map[string]any
	if err := json.Unmarshal([]byte(validBank), &failingDocument); err != nil {
		t.Fatal(err)
	}
	failingDocument["meta"].(map[string]any)["name"] = "force-resume-failure"
	failingJSON, err := json.Marshal(failingDocument)
	if err != nil {
		t.Fatal(err)
	}
	firstBankKey := "partial-first-" + strings.ReplaceAll(uuid.NewString(), "-", "")
	secondBankKey := "partial-second-" + strings.ReplaceAll(uuid.NewString(), "-", "")
	snapshot := quizcraft.LegacySnapshot{
		SourceName:    "representative-partial-production-snapshot",
		CutoffEventID: 42,
		Banks: []quizcraft.LegacyBank{
			{BankKey: firstBankKey, Document: json.RawMessage(validBank)},
			{BankKey: secondBankKey, Document: failingJSON},
		},
		Rankings: json.RawMessage(`[]`),
	}
	path := filepath.Join(t.TempDir(), "quizcraft-v2.partial.snapshot.json")
	artifact, err := quizcraft.WriteLegacySnapshotArtifact(path, "legacy-system/42", snapshot)
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := quizcraft.ReadLegacySnapshotArtifact(path)
	if err != nil {
		t.Fatal(err)
	}
	failed, err := service.RunFullMigration(ctx, reloaded.Snapshot)
	if err == nil || failed.RunID == uuid.Nil || failed.State != "blocked" {
		t.Fatalf("partial migration = %+v / %v", failed, err)
	}
	var importedFirst, importedSecond int
	if err := pool.QueryRow(ctx, `SELECT count(*) FILTER (WHERE bank_key=$1),count(*) FILTER (WHERE bank_key=$2) FROM quizcraft_banks`, firstBankKey, secondBankKey).Scan(&importedFirst, &importedSecond); err != nil {
		t.Fatal(err)
	}
	if importedFirst != 1 || importedSecond != 0 {
		t.Fatalf("partial migration facts = first:%d second:%d", importedFirst, importedSecond)
	}
	if _, err := pool.Exec(ctx, `DROP TRIGGER quizcraft_test_fail_second_migration_bank ON quizcraft_banks; DROP FUNCTION quizcraft_test_fail_second_migration_bank()`); err != nil {
		t.Fatal(err)
	}
	resumed, err := service.ResumeFullMigration(ctx, failed.RunID, reloaded.Snapshot)
	if err != nil || resumed.State != "passed" || resumed.SourceSnapshotSHA256 != artifact.SnapshotSHA256 {
		t.Fatalf("resumed partial migration = %+v / %v", resumed, err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FILTER (WHERE bank_key=$1),count(*) FILTER (WHERE bank_key=$2) FROM quizcraft_banks`, firstBankKey, secondBankKey).Scan(&importedFirst, &importedSecond); err != nil {
		t.Fatal(err)
	}
	if importedFirst != 1 || importedSecond != 1 {
		t.Fatalf("resumed migration facts = first:%d second:%d", importedFirst, importedSecond)
	}
}

func TestSnapshotArtifactRejectsChecksumTampering(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snapshot.json")
	artifact, err := quizcraft.WriteLegacySnapshotArtifact(path, "legacy-system/1", quizcraft.LegacySnapshot{SourceName: "legacy", CutoffEventID: 0, Rankings: json.RawMessage(`[]`)})
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	tampered := strings.Replace(string(body), artifact.SnapshotSHA256, strings.Repeat("0", 64), 1)
	if err := os.WriteFile(path, []byte(tampered), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := quizcraft.ReadLegacySnapshotArtifact(path); err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("tampered snapshot artifact was accepted: %v", err)
	}
}

func TestBackupRestoreDrillRebuildsAnIsolatedDatabaseAndReportsEvidence(t *testing.T) {
	if _, err := exec.LookPath("pg_dump"); err != nil {
		t.Skip("pg_dump is required for the restore-drill boundary")
	}
	if _, err := exec.LookPath("pg_restore"); err != nil {
		t.Skip("pg_restore is required for the restore-drill boundary")
	}
	pool := isolatedQuizcraftV2Database(t)
	if _, err := pool.Exec(context.Background(), `INSERT INTO quizcraft_banks(id,bank_key,name) VALUES('00000000-0000-0000-0000-000000000159','backup-restore-evidence','Backup restore evidence')`); err != nil {
		t.Fatal(err)
	}
	url := testDatabaseURL(t, "quizcraft_v2")
	report, err := quizcraft.RunBackupRestoreDrill(context.Background(), quizcraft.BackupRestoreDrillOptions{
		DatabaseURL:      url,
		RestoreAdminURL:  url,
		BackupDirectory:  t.TempDir(),
		RequiredTables:   []string{"quizcraft_banks", "quizcraft_migration_runs"},
		DatabaseNameHint: "quizcraft_v2_test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.BackupSHA256) != 64 || report.RestoredDatabase == "" || report.Duration <= 0 || report.TableCounts["quizcraft_banks"] != 1 || report.TableCounts["quizcraft_migration_runs"] < 0 {
		t.Fatalf("restore drill report = %+v", report)
	}
	if report.SourceDatabaseIdentity == "" || report.RestoreAdminDatabaseIdentity == "" || report.RestoredDatabaseIdentity == "" || len(report.DumpCommand) == 0 || len(report.RestoreCommand) == 0 || strings.Join(report.VerifiedTables, ",") != "quizcraft_banks,quizcraft_migration_runs" {
		t.Fatalf("restore drill audit evidence = %+v", report)
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "postgres://") {
		t.Fatalf("restore drill report exposes a database URL: %s", encoded)
	}
	var audit struct {
		SourceTableVerification map[string]struct {
			RowCount      int64  `json:"row_count"`
			ContentSHA256 string `json:"content_sha256"`
		} `json:"source_table_verification"`
		RestoredTableVerification map[string]struct {
			RowCount      int64  `json:"row_count"`
			ContentSHA256 string `json:"content_sha256"`
		} `json:"restored_table_verification"`
	}
	if err := json.Unmarshal(encoded, &audit); err != nil {
		t.Fatal(err)
	}
	sourceBanks, sourceOK := audit.SourceTableVerification["quizcraft_banks"]
	restoredBanks, restoredOK := audit.RestoredTableVerification["quizcraft_banks"]
	if !sourceOK || !restoredOK || sourceBanks.RowCount != 1 || restoredBanks.RowCount != 1 || len(sourceBanks.ContentSHA256) != 64 || sourceBanks.ContentSHA256 != restoredBanks.ContentSHA256 {
		t.Fatalf("backup source/restore content verification = %+v / %+v", audit.SourceTableVerification, audit.RestoredTableVerification)
	}
	if info, err := os.Stat(report.BackupPath); err != nil || info.Size() == 0 {
		t.Fatalf("backup artifact = %q / %v", report.BackupPath, err)
	}
}

func isolatedArtifactDatabase(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()
	testURL := os.Getenv("QUIZCRAFT_TEST_DATABASE_URL")
	config, err := pgxpool.ParseConfig(testURL)
	if err != nil {
		t.Fatal(err)
	}
	adminConfig := config.ConnConfig.Copy()
	adminConfig.Database = "postgres"
	admin, err := pgx.ConnectConfig(ctx, adminConfig)
	if err != nil {
		t.Fatal(err)
	}
	name := "quizcraft_artifacts_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	if _, err := admin.Exec(ctx, `CREATE DATABASE `+pgx.Identifier{name}.Sanitize()); err != nil {
		_ = admin.Close(ctx)
		t.Fatal(err)
	}
	config.ConnConfig.Database = name
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		_, _ = admin.Exec(ctx, `DROP DATABASE IF EXISTS `+pgx.Identifier{name}.Sanitize())
		_ = admin.Close(ctx)
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		_, _ = admin.Exec(context.Background(), `SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname=$1`, name)
		_, _ = admin.Exec(context.Background(), `DROP DATABASE IF EXISTS `+pgx.Identifier{name}.Sanitize())
		_ = admin.Close(context.Background())
	})
	return pool
}

func isolatedQuizcraftV2Database(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()
	baseURL := os.Getenv("QUIZCRAFT_TEST_DATABASE_URL")
	config, err := pgxpool.ParseConfig(baseURL)
	if err != nil {
		t.Fatal(err)
	}
	adminConfig := config.ConnConfig.Copy()
	adminConfig.Database = "postgres"
	admin, err := pgx.ConnectConfig(ctx, adminConfig)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, `CREATE DATABASE quizcraft_v2`); err != nil {
		_ = admin.Close(ctx)
		t.Fatalf("create isolated quizcraft_v2 database: %v", err)
	}
	config.ConnConfig.Database = "quizcraft_v2"
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		_, _ = admin.Exec(ctx, `DROP DATABASE IF EXISTS quizcraft_v2`)
		_ = admin.Close(ctx)
		t.Fatal(err)
	}
	if _, err := quizcraft.ApplyVersionedMigrations(ctx, pool, "../db/migrations"); err != nil {
		pool.Close()
		_, _ = admin.Exec(ctx, `DROP DATABASE IF EXISTS quizcraft_v2`)
		_ = admin.Close(ctx)
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		_, _ = admin.Exec(context.Background(), `SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname='quizcraft_v2'`)
		_, _ = admin.Exec(context.Background(), `DROP DATABASE IF EXISTS quizcraft_v2`)
		_ = admin.Close(context.Background())
	})
	return pool
}

func migrationPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool := isolatedArtifactDatabase(t)
	if _, err := quizcraft.ApplyVersionedMigrations(context.Background(), pool, "../db/migrations"); err != nil {
		t.Fatal(err)
	}
	return pool
}

func testDatabaseURL(t *testing.T, name string) string {
	t.Helper()
	parsed, err := url.Parse(os.Getenv("QUIZCRAFT_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	parsed.Path = "/" + name
	return parsed.String()
}

func applyReleasedPreHistoryBaseline(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	paths, err := filepath.Glob("../db/migrations/00000[1-8]_*.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(paths)
	if len(paths) != 8 {
		t.Fatalf("pre-history migration files = %v", paths)
	}
	for _, path := range paths {
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, string(source)); err != nil {
			t.Fatalf("apply released baseline %s: %v", path, err)
		}
	}
}
