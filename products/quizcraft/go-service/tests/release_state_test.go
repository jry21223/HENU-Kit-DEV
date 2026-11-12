package tests

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	quizcraft "henukit.dev/quizcraft"
)

func TestCoreBackupVerifierMatchesTheExactDumpSnapshotAfterRestore(t *testing.T) {
	pool := migrationPool(t)
	ctx := context.Background()
	config := pool.Config().ConnConfig
	if _, err := pool.Exec(ctx, `INSERT INTO quizcraft_banks(id,bank_key,name) VALUES('00000000-0000-0000-0000-000000000651','snapshot-bank','before snapshot')`); err != nil {
		t.Fatal(err)
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	var snapshot string
	if err = tx.QueryRow(ctx, `SELECT pg_export_snapshot()`).Scan(&snapshot); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile("../db/release/backup-000012.sql")
	if err != nil {
		t.Fatal(err)
	}
	read := func(query func(context.Context, string, ...any) (pgx.Rows, error)) string {
		rows, err := query(ctx, string(body))
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		h := sha256.New()
		for rows.Next() {
			var table, hash string
			if err = rows.Scan(&table, &hash); err != nil {
				t.Fatal(err)
			}
			fmt.Fprintf(h, "%s\t%s\n", table, hash)
		}
		if err = rows.Err(); err != nil {
			t.Fatal(err)
		}
		return fmt.Sprintf("%x", h.Sum(nil))
	}
	before := read(tx.Query)
	if _, err = pool.Exec(ctx, `UPDATE quizcraft_banks SET name='after snapshot' WHERE bank_key='snapshot-bank'`); err != nil {
		t.Fatal(err)
	}
	if current := read(pool.Query); current == before {
		t.Fatal("fixture did not change after snapshot")
	}
	environment := append(os.Environ(), "PGHOST="+config.Host, fmt.Sprintf("PGPORT=%d", config.Port), "PGUSER="+config.User, "PGPASSWORD="+config.Password, "PGDATABASE="+config.Database, "PGSSLMODE=disable")
	command := func(name string, args ...string) {
		t.Helper()
		tool, err := exec.LookPath(name)
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(tool, args...)
		cmd.Env = environment
		if err = cmd.Run(); err != nil {
			t.Fatalf("%s failed", name)
		}
	}
	dump := filepath.Join(t.TempDir(), "snapshot.dump")
	command("pg_dump", "--format=custom", "--snapshot", snapshot, "--file", dump)
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	adminConfig := config.Copy()
	adminConfig.Database = "postgres"
	admin, err := pgx.ConnectConfig(ctx, adminConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(ctx)
	name := "henukit_quizcraft_verify_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err = admin.Exec(ctx, `CREATE DATABASE `+pgx.Identifier{name}.Sanitize()+` TEMPLATE template0`); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(ctx, `DROP DATABASE `+pgx.Identifier{name}.Sanitize())
	command("pg_restore", "--exit-on-error", "--no-owner", "--no-privileges", "--dbname", name, dump)
	restoreConfig := pool.Config()
	restoreConfig.ConnConfig.Database = name
	restored, err := pgxpool.NewWithConfig(ctx, restoreConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if actual := read(restored.Query); actual != before {
		t.Fatalf("restore is not the exported source snapshot: %s / %s", actual, before)
	}
}

func TestReleaseStateRequiresAwareConsumersBeforeAnyLearnerAttempt(t *testing.T) {
	pool := migrationPool(t)
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	var snapshot string
	if _, err = tx.Exec(ctx, `SET TRANSACTION ISOLATION LEVEL REPEATABLE READ READ ONLY`); err != nil {
		t.Fatal(err)
	}
	if err = tx.QueryRow(ctx, `SELECT pg_export_snapshot()`).Scan(&snapshot); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO quizcraft_banks(id,bank_key,name) VALUES('00000000-0000-0000-0000-000000000651','release-state','Release fixture'); INSERT INTO quizcraft_training_units(id,bank_id,source_unit_id) VALUES('00000000-0000-0000-0000-000000000652','00000000-0000-0000-0000-000000000651','unpublished')`); err != nil {
		t.Fatal(err)
	}
	state, err := quizcraft.ReadReleaseState(ctx, pool, "../db/migrations", "")
	if err != nil {
		t.Fatal(err)
	}
	if !state.RequiresEnglishTrainingDataV1 {
		t.Fatal("unpublished authored training was treated as safe for old consumers")
	}
	state, err = quizcraft.ReadReleaseState(ctx, pool, "../db/migrations", snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if state.RequiresEnglishTrainingDataV1 {
		t.Fatal("probe ignored the exported backup snapshot")
	}
	if _, err = pool.Exec(ctx, `INSERT INTO quizcraft_schema_migrations(version,filename,source_sha256) VALUES('999999','unknown',repeat('0',64))`); err != nil {
		t.Fatal(err)
	}
	if _, err = quizcraft.ReadReleaseState(ctx, pool, "../db/migrations", ""); err == nil {
		t.Fatal("ahead history was accepted")
	}
}

func TestCoreBackupVerifierDetectsSameCountContentChangesWithoutReturningRows(t *testing.T) {
	pool := migrationPool(t)
	ctx := context.Background()
	body, err := os.ReadFile("../db/release/backup-000012.sql")
	if err != nil {
		t.Fatal(err)
	}
	query := string(body)
	read := func() string {
		rows, err := pool.Query(ctx, query)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var records strings.Builder
		for rows.Next() {
			var relation, digest string
			if err = rows.Scan(&relation, &digest); err != nil {
				t.Fatal(err)
			}
			fmt.Fprintf(&records, "%s\t%s\n", relation, digest)
		}
		if err = rows.Err(); err != nil {
			t.Fatal(err)
		}
		return records.String()
	}
	if _, err = pool.Exec(ctx, `INSERT INTO quizcraft_banks(id,bank_key,name) VALUES('00000000-0000-0000-0000-000000000651','backup-stream','private original title')`); err != nil {
		t.Fatal(err)
	}
	first := read()
	if strings.Contains(first, "private original title") {
		t.Fatal("backup verifier disclosed row content")
	}
	if _, err = pool.Exec(ctx, `UPDATE quizcraft_banks SET name='private changed title' WHERE bank_key='backup-stream'`); err != nil {
		t.Fatal(err)
	}
	second := read()
	if sha256.Sum256([]byte(first)) == sha256.Sum256([]byte(second)) {
		t.Fatal("same-count data change escaped content proof")
	}
}

func TestReleaseStateRecognizesOnlyHealthyTrackedSchemas(t *testing.T) {
	pool := isolatedArtifactDatabase(t)
	ctx := context.Background()
	if _, err := quizcraft.ReadReleaseState(ctx, pool, "../db/migrations", ""); err == nil {
		t.Fatal("empty database was reported as safely pre-training")
	}
	before := t.TempDir()
	files, _ := filepath.Glob("../db/migrations/*.up.sql")
	for _, path := range files {
		if filepath.Base(path)[:6] > "000011" {
			continue
		}
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(before, filepath.Base(path)), body, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := quizcraft.ApplyVersionedMigrations(ctx, pool, before); err != nil {
		t.Fatal(err)
	}
	state, err := quizcraft.ReadReleaseState(ctx, pool, "../db/migrations", "")
	if err != nil {
		t.Fatal(err)
	}
	if state.Schema.Version != "000011" || state.RequiresEnglishTrainingDataV1 || len(state.Schema.History) != 11 {
		t.Fatalf("tracked11 state = %+v", state)
	}
	if _, err = quizcraft.ApplyVersionedMigrations(ctx, pool, "../db/migrations"); err != nil {
		t.Fatal(err)
	}
	state, err = quizcraft.ReadReleaseState(ctx, pool, "../db/migrations", "")
	if err != nil {
		t.Fatal(err)
	}
	if state.Schema.Version != "000012" || state.RequiresEnglishTrainingDataV1 {
		t.Fatalf("empty12 = %+v", state)
	}
	if _, err = pool.Exec(ctx, `ALTER TABLE quizcraft_training_units ADD COLUMN unreviewed text`); err != nil {
		t.Fatal(err)
	}
	if _, err = quizcraft.ReadReleaseState(ctx, pool, "../db/migrations", ""); err == nil {
		t.Fatal("unrecognized catalog accepted")
	}
}
