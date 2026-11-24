package library

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestWorkspaceDownloadStarts(t *testing.T) {
	h := workspaceTestService(t)
	ctx := t.Context()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := h.database.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	const material = "11111111-1111-4111-8111-111111111111"
	const missing = "22222222-2222-4222-8222-222222222222"
	release, receipt := strings.Repeat("a", 40)+"-"+strings.Repeat("b", 16), strings.Repeat("c", 64)
	exec(`INSERT INTO library_public_releases VALUES ($1, $2, 'active', now())`, release, receipt)
	exec(`INSERT INTO library_public_material_snapshots
		(release_id, material_id, title, file_name, access_level, status, object_key, object_version_id, sha256, byte_size)
		VALUES ($1, $2, 'Calculus', 'notes.pdf', 'authenticated', 'published',
		        'releases/' || $1 || '/notes.pdf', 'v1', $3, 100)`, release, material, receipt)

	check := func(total, recent int) Workspace {
		t.Helper()
		ws, err := h.loadWorkspace(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if int(ws.DownloadStats.TotalStarts) != total || int(ws.DownloadStats.Last24hStarts) != recent || len(ws.Downloads) != total {
			t.Fatalf("stats = %+v, history rows = %d, want total %d, last 24h %d", ws.DownloadStats, len(ws.Downloads), total, recent)
		}
		return ws
	}
	insert := func(id, materialID string, age time.Duration) time.Time {
		t.Helper()
		issued := time.Now().UTC().Truncate(time.Microsecond).Add(-age)
		exec(`INSERT INTO library_download_start_events
			(id, material_id, release_id, receipt_sha256, object_version_id, request_id, issued_at, expires_at)
			VALUES ($1, $2, $3, $4, 'v1', 'req_test', $5::timestamptz, $5::timestamptz + interval '30 seconds')`, id, materialID, release, receipt, issued)
		return issued
	}
	check(0, 0)
	insert("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", material, 48*time.Hour)
	check(1, 0)

	// The production FK normally guarantees a snapshot. Simulate a historical
	// event with its snapshot missing to exercise the defensive LEFT JOIN.
	exec(`ALTER TABLE library_download_start_events DROP CONSTRAINT library_download_start_events_release_id_material_id_fkey`)
	issued := insert("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", missing, time.Hour)
	ws := check(2, 1)
	newest, oldest := ws.Downloads[0], ws.Downloads[1]
	if newest.MaterialID != missing || newest.MaterialTitle != missing || newest.AccessLevel != "public_free" || !newest.DownloadedAt.Equal(issued) {
		t.Errorf("download without snapshot = %+v", newest)
	}
	if oldest.MaterialTitle != "Calculus" || oldest.AccessLevel != "authenticated" {
		t.Errorf("download with snapshot = %+v", oldest)
	}
}

func TestWorkspacePropagatesDownloadAndReleaseQueryErrors(t *testing.T) {
	for _, tc := range []struct{ table, column, want string }{
		{"library_public_releases", "receipt_sha256", "library_public_releases"},
		{"library_download_start_events", "issued_at", "count library_download_start_events"},
		{"library_public_material_snapshots", "access_level", "query library_download_start_events"},
	} {
		t.Run(tc.column, func(t *testing.T) {
			h := workspaceTestService(t)
			if _, err := h.database.Exec(t.Context(), fmt.Sprintf("ALTER TABLE %s RENAME COLUMN %s TO unavailable", tc.table, tc.column)); err != nil {
				t.Fatal(err)
			}
			if _, err := h.loadWorkspace(t.Context()); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("loadWorkspace error = %v, want %q", err, tc.want)
			}
		})
	}
}

func workspaceTestService(t *testing.T) *service {
	t.Helper()
	dsn := os.Getenv("LIBRARY_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("LIBRARY_TEST_DATABASE_URL is required for PostgreSQL integration")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	schema := fmt.Sprintf("workspace_test_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Error(err)
		}
	})
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	database, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(database.Close)
	migrations, err := filepath.Glob("db/migrations/*.up.sql")
	if err != nil || len(migrations) == 0 {
		t.Fatalf("find migrations: %v", err)
	}
	for _, filename := range migrations {
		migration, err := os.ReadFile(filename)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := database.Exec(ctx, string(migration)); err != nil {
			t.Fatalf("apply %s: %v", filename, err)
		}
	}
	return &service{database: database, now: time.Now}
}
