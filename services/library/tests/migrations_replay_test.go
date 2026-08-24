package tests

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func TestMigrationsReplayAfterCanonicalMaterialTypesExist(t *testing.T) {
	ctx := context.Background()
	connection, err := pgx.Connect(ctx, os.Getenv("LIBRARY_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	identity := strings.ReplaceAll(uuid.NewString()+uuid.NewString(), "-", "")
	releaseID := identity[:40] + "-" + identity[40:56]
	materialTypes := []string{"handout", "exercise", "answer"}
	t.Cleanup(func() {
		if _, err := connection.Exec(context.Background(), `DELETE FROM library_public_material_snapshots WHERE release_id=$1`, releaseID); err != nil {
			t.Errorf("cleanup migration replay snapshot: %v", err)
		}
		if _, err := connection.Exec(context.Background(), `DELETE FROM library_public_releases WHERE release_id=$1`, releaseID); err != nil {
			t.Errorf("cleanup migration replay release: %v", err)
		}
		if err := connection.Close(context.Background()); err != nil {
			t.Errorf("close migration replay database: %v", err)
		}
	})
	if _, err := connection.Exec(ctx, `
		INSERT INTO library_public_releases (release_id, receipt_sha256, state, activated_at)
		VALUES ($1, repeat('c', 64), 'retired', now())`, releaseID); err != nil {
		t.Fatal(err)
	}
	for _, materialType := range materialTypes {
		if _, err := connection.Exec(ctx, `
			INSERT INTO library_public_material_snapshots (
				release_id, material_id, title, file_name, access_level, status,
				object_key, object_version_id, sha256, byte_size, material_type
			) VALUES (
				$1, $2, $3, $3 || '.pdf', 'public_free', 'published',
				'releases/' || $1 || '/' || $3 || '.pdf', 'version-' || $3,
				repeat(substr(md5($3), 1, 1), 64), length($3), $3
			)`, releaseID, uuid.NewString(), materialType); err != nil {
			t.Fatal(err)
		}
	}
	before := snapshotRowsJSON(t, ctx, connection, releaseID)
	beforeConstraintOID := materialTypeConstraintOID(t, ctx, connection)
	if !strings.Contains(before, `"material_type": "handout"`) ||
		!strings.Contains(before, `"material_type": "exercise"`) ||
		!strings.Contains(before, `"material_type": "answer"`) {
		t.Fatalf("fixture does not contain all canonical material types: %s", before)
	}
	migrations, err := filepath.Glob("../db/migrations/*.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(migrations)
	for _, path := range migrations {
		migration, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := connection.Exec(ctx, string(migration)); err != nil {
			t.Fatalf("replay %s with canonical rows: %v", filepath.Base(path), err)
		}
	}
	after := snapshotRowsJSON(t, ctx, connection, releaseID)
	if after != before {
		t.Fatalf("migration replay changed immutable snapshot rows\nbefore: %s\nafter:  %s", before, after)
	}
	afterConstraintOID := materialTypeConstraintOID(t, ctx, connection)
	if afterConstraintOID != beforeConstraintOID {
		t.Fatalf("steady-state migration replay rebuilt the canonical constraint: before oid %d, after oid %d", beforeConstraintOID, afterConstraintOID)
	}
}

func TestElectronicTextbookMigrationFailureKeepsPriorConstraint(t *testing.T) {
	ctx := context.Background()
	connection, err := pgx.Connect(ctx, os.Getenv("LIBRARY_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := connection.Close(context.Background()); err != nil {
			t.Errorf("close atomic migration database: %v", err)
		}
	})

	if _, err := connection.Exec(ctx, `
		CREATE TEMPORARY TABLE library_public_material_snapshots (material_type text NOT NULL);
		ALTER TABLE library_public_material_snapshots
			ADD CONSTRAINT library_public_material_snapshots_type_check
			CHECK (material_type IN ('note', 'future_type'));
		INSERT INTO library_public_material_snapshots (material_type) VALUES ('future_type');
		SET search_path TO pg_temp;`); err != nil {
		t.Fatal(err)
	}
	beforeConstraint := temporaryConstraintDefinition(t, ctx, connection)
	migration, err := os.ReadFile("../db/migrations/000004_electronic_textbooks.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := connection.Exec(ctx, string(migration)); err == nil {
		t.Fatal("000004 unexpectedly accepted a future material type")
	}
	if _, err := connection.Exec(ctx, "ROLLBACK"); err != nil {
		t.Fatal(err)
	}
	assertTemporaryConstraintAndRows(t, ctx, connection, beforeConstraint, "future_type")
}

func TestElectronicTextbookDownMigrationDoesNotRewriteImmutableSnapshots(t *testing.T) {
	ctx := context.Background()
	connection, err := pgx.Connect(ctx, os.Getenv("LIBRARY_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := connection.Close(context.Background()); err != nil {
			t.Errorf("close down migration database: %v", err)
		}
	})

	if _, err := connection.Exec(ctx, `
		CREATE TEMPORARY TABLE library_public_material_snapshots (material_type text NOT NULL);
		ALTER TABLE library_public_material_snapshots
			ADD CONSTRAINT library_public_material_snapshots_type_check
			CHECK (material_type IN ('note', 'textbook', 'handout'));
		INSERT INTO library_public_material_snapshots (material_type) VALUES ('textbook');
		SET search_path TO pg_temp;`); err != nil {
		t.Fatal(err)
	}
	beforeConstraint := temporaryConstraintDefinition(t, ctx, connection)
	migration, err := os.ReadFile("../db/migrations/000004_electronic_textbooks.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := connection.Exec(ctx, string(migration)); err == nil {
		t.Fatal("000004 down migration unexpectedly rewrote an immutable textbook snapshot")
	}
	if _, err := connection.Exec(ctx, "ROLLBACK"); err != nil {
		t.Fatal(err)
	}
	assertTemporaryConstraintAndRows(t, ctx, connection, beforeConstraint, "textbook")
}

func snapshotRowsJSON(t *testing.T, ctx context.Context, connection *pgx.Conn, releaseID string) string {
	t.Helper()
	var rows string
	if err := connection.QueryRow(ctx, `
		SELECT COALESCE(jsonb_agg(to_jsonb(snapshot) ORDER BY material_id::text), '[]'::jsonb)::text
		FROM library_public_material_snapshots AS snapshot
		WHERE release_id=$1`, releaseID).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	return rows
}

func materialTypeConstraintOID(t *testing.T, ctx context.Context, connection *pgx.Conn) uint32 {
	t.Helper()
	var oid uint32
	if err := connection.QueryRow(ctx, `
		SELECT oid FROM pg_constraint
		WHERE conrelid='library_public_material_snapshots'::regclass
		  AND conname='library_public_material_snapshots_type_check'`).Scan(&oid); err != nil {
		t.Fatal(err)
	}
	return oid
}

func assertTemporaryConstraintAndRows(
	t *testing.T,
	ctx context.Context,
	connection *pgx.Conn,
	wantConstraint string,
	wantMaterialType string,
) {
	t.Helper()
	constraintDefinition := temporaryConstraintDefinition(t, ctx, connection)
	if constraintDefinition != wantConstraint {
		t.Fatalf("failed migration changed the prior constraint: got %s, want %s", constraintDefinition, wantConstraint)
	}
	var materialTypes []string
	rows, err := connection.Query(ctx, `
		SELECT material_type FROM library_public_material_snapshots ORDER BY material_type`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var materialType string
		if err := rows.Scan(&materialType); err != nil {
			t.Fatal(err)
		}
		materialTypes = append(materialTypes, materialType)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(materialTypes) != 1 || materialTypes[0] != wantMaterialType {
		t.Fatalf("failed migration changed immutable rows: got %v, want [%s]", materialTypes, wantMaterialType)
	}
}

func temporaryConstraintDefinition(t *testing.T, ctx context.Context, connection *pgx.Conn) string {
	t.Helper()
	var definition string
	if err := connection.QueryRow(ctx, `
		SELECT pg_get_constraintdef(oid) FROM pg_constraint
		WHERE conrelid='pg_temp.library_public_material_snapshots'::regclass
		  AND conname='library_public_material_snapshots_type_check'`).Scan(&definition); err != nil {
		t.Fatal(err)
	}
	return definition
}
