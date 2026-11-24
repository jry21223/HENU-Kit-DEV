package career

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type stubRow struct {
	values []any
	err    error
}

func (r stubRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	return assignValues(dest, r.values)
}

type stubRows struct {
	rows    [][]any
	index   int
	scanErr error
	iterErr error
	closed  bool
}

func (r *stubRows) Close()                                       { r.closed = true }
func (r *stubRows) Err() error                                   { return r.iterErr }
func (r *stubRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (r *stubRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *stubRows) RawValues() [][]byte                          { return nil }
func (r *stubRows) Conn() *pgx.Conn                              { return nil }
func (r *stubRows) Values() ([]any, error)                       { return nil, nil }

func (r *stubRows) Next() bool {
	if r.closed || r.index >= len(r.rows) {
		return false
	}
	r.index++
	return true
}

func (r *stubRows) Scan(dest ...any) error {
	if r.scanErr != nil {
		return r.scanErr
	}
	return assignValues(dest, r.rows[r.index-1])
}

func assignValues(dest []any, values []any) error {
	if len(dest) != len(values) {
		return errors.New("scan column count mismatch")
	}
	for i := range dest {
		dv := reflect.ValueOf(dest[i])
		if dv.Kind() != reflect.Ptr || dv.IsNil() {
			return errors.New("scan destination must be non-nil pointer")
		}
		if values[i] == nil {
			dv.Elem().Set(reflect.Zero(dv.Elem().Type()))
			continue
		}
		dv.Elem().Set(reflect.ValueOf(values[i]))
	}
	return nil
}

type fakeConsoleDB struct {
	countRows  []stubRow
	countIndex int
	queries    []string
	queryFn    func(sql string) (pgx.Rows, error)
}

func (db *fakeConsoleDB) QueryRow(_ context.Context, _ string, _ ...any) pgx.Row {
	if db.countIndex >= len(db.countRows) {
		return stubRow{err: errors.New("unexpected QueryRow call")}
	}
	row := db.countRows[db.countIndex]
	db.countIndex++
	return row
}

func (db *fakeConsoleDB) Query(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
	db.queries = append(db.queries, sql)
	return db.queryFn(sql)
}

func okConsoleCountRows() []stubRow {
	return []stubRow{
		{values: []any{2}},
		{values: []any{3, 0, 1, 2, 0}},
		{values: []any{1}},
		{values: []any{1, 1}},
	}
}

func TestConsoleWorkspaceOmitsRawResumeFileName(t *testing.T) {
	now := time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC)
	completedAt := now.Add(-time.Minute)
	searchID := "22222222-2222-4222-8222-222222222222"
	extractID := "33333333-3333-4333-8333-333333333333"
	userID := "11111111-1111-4111-8111-111111111111"

	db := &fakeConsoleDB{
		countRows: okConsoleCountRows(),
		queryFn: func(sql string) (pgx.Rows, error) {
			switch {
			case strings.Contains(sql, "FROM career_searches"):
				return &stubRows{rows: [][]any{{
					searchID, userID, "completed", "rendering",
					2, 10, 4, "", "",
					now, &now, &completedAt, (*time.Time)(nil), &completedAt,
				}}}, nil
			case strings.Contains(sql, "FROM career_resume_extractions"):
				if strings.Contains(strings.ToLower(sql), "file_name") {
					t.Fatalf("career_resume_extractions query must not select file_name: %s", sql)
				}
				return &stubRows{rows: [][]any{{
					extractID, userID, "completed", "", "",
					now, &completedAt, (*time.Time)(nil),
				}}}, nil
			case strings.Contains(sql, "FROM career_digest_deliveries"):
				return &stubRows{rows: [][]any{{
					searchID, "sent", &completedAt, &now, now, completedAt,
				}}}, nil
			default:
				return nil, errors.New("unexpected query")
			}
		},
	}

	svc := &service{aiMode: "mock", now: func() time.Time { return now }}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/console/workspace", nil)
	rec := httptest.NewRecorder()
	svc.writeConsoleWorkspace(rec, req, db)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	raw := rec.Body.String()
	if strings.Contains(raw, `"file_name"`) {
		t.Fatalf("workspace response must not contain file_name key: %s", raw)
	}
	var envelope struct {
		Data struct {
			Extractions []map[string]any `json:"extractions"`
			Searches    []map[string]any `json:"searches"`
			Digests     []map[string]any `json:"digest_deliveries"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(envelope.Data.Extractions) != 1 || len(envelope.Data.Searches) != 1 || len(envelope.Data.Digests) != 1 {
		t.Fatalf("unexpected workspace lengths: extractions=%d searches=%d digests=%d",
			len(envelope.Data.Extractions), len(envelope.Data.Searches), len(envelope.Data.Digests))
	}
	if _, hasFileName := envelope.Data.Extractions[0]["file_name"]; hasFileName {
		t.Fatalf("extraction object unexpectedly contains file_name: %+v", envelope.Data.Extractions[0])
	}
	if envelope.Data.Extractions[0]["id"] != extractID || envelope.Data.Extractions[0]["status"] != "completed" {
		t.Fatalf("unexpected extraction payload: %+v", envelope.Data.Extractions[0])
	}
}

func TestConsoleWorkspacePropagatesSubqueryErrors(t *testing.T) {
	now := time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC)
	dbErr := errors.New("simulated db failure")

	cases := []struct {
		name    string
		queryFn func(sql string) (pgx.Rows, error)
	}{
		{
			name: "extractions query error",
			queryFn: func(sql string) (pgx.Rows, error) {
				if strings.Contains(sql, "FROM career_resume_extractions") {
					return nil, dbErr
				}
				return &stubRows{}, nil
			},
		},
		{
			name: "extractions scan error",
			queryFn: func(sql string) (pgx.Rows, error) {
				if strings.Contains(sql, "FROM career_resume_extractions") {
					return &stubRows{rows: [][]any{{1}}, scanErr: dbErr}, nil
				}
				return &stubRows{}, nil
			},
		},
		{
			name: "extractions rows err",
			queryFn: func(sql string) (pgx.Rows, error) {
				if strings.Contains(sql, "FROM career_resume_extractions") {
					return &stubRows{iterErr: dbErr}, nil
				}
				return &stubRows{}, nil
			},
		},
		{
			name: "digests query error",
			queryFn: func(sql string) (pgx.Rows, error) {
				if strings.Contains(sql, "FROM career_digest_deliveries") {
					return nil, dbErr
				}
				return &stubRows{}, nil
			},
		},
		{
			name: "digests scan error",
			queryFn: func(sql string) (pgx.Rows, error) {
				if strings.Contains(sql, "FROM career_digest_deliveries") {
					return &stubRows{rows: [][]any{{1}}, scanErr: dbErr}, nil
				}
				return &stubRows{}, nil
			},
		},
		{
			name: "digests rows err",
			queryFn: func(sql string) (pgx.Rows, error) {
				if strings.Contains(sql, "FROM career_digest_deliveries") {
					return &stubRows{iterErr: dbErr}, nil
				}
				return &stubRows{}, nil
			},
		},
		{
			name: "searches rows err",
			queryFn: func(sql string) (pgx.Rows, error) {
				if strings.Contains(sql, "FROM career_searches") {
					return &stubRows{iterErr: dbErr}, nil
				}
				return &stubRows{}, nil
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := &fakeConsoleDB{
				countRows: okConsoleCountRows(),
				queryFn:   tc.queryFn,
			}
			svc := &service{aiMode: "mock", now: func() time.Time { return now }}
			req := httptest.NewRequest(http.MethodGet, "/api/v1/console/workspace", nil)
			rec := httptest.NewRecorder()
			svc.writeConsoleWorkspace(rec, req, db)

			if rec.Code != http.StatusServiceUnavailable {
				t.Fatalf("status = %d, want 503; body=%s", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), `"DEPENDENCY_UNAVAILABLE"`) {
				t.Fatalf("expected DEPENDENCY_UNAVAILABLE error, got %s", rec.Body.String())
			}
		})
	}
}

func TestConsoleCredentialIndependence(t *testing.T) {
	redisClient := redis.NewClient(&redis.Options{Addr: "127.0.0.1:6379"})
	t.Cleanup(func() { _ = redisClient.Close() })
	const shared = "shared-secret-value-at-least-32-bytes-long"
	if _, err := New(Config{
		Database:        &pgxpool.Pool{},
		Redis:           redisClient,
		ClientID:        "portal-gateway",
		Keys:            map[string]string{"k1": shared},
		ConsoleClientID: "console-gateway",
		ConsoleKeys:     map[string]string{"ck1": shared},
	}); err == nil {
		t.Fatal("expected overlapping portal/console secret to be rejected")
	}
	if _, err := New(Config{
		Database:        &pgxpool.Pool{},
		Redis:           redisClient,
		ClientID:        "portal-gateway",
		Keys:            map[string]string{"k1": shared},
		ConsoleClientID: "portal-gateway",
		ConsoleKeys:     map[string]string{"ck1": "distinct-console-secret-at-least-32-bytes"},
	}); err == nil {
		t.Fatal("expected identical portal/console client id to be rejected")
	}
}

func TestConsoleWorkspaceAuthAndScopeGuards(t *testing.T) {
	addr := os.Getenv("CAREER_TEST_REDIS_ADDR")
	if addr == "" {
		addr = "redis:6379"
	}
	redisClient := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() { _ = redisClient.Close() })
	if err := redisClient.Ping(context.Background()).Err(); err != nil {
		t.Skipf("redis unavailable at %s: %v", addr, err)
	}
	const consoleSecret = "career-console-secret-at-least-32-bytes-long"
	svc, err := New(Config{
		Database:        &pgxpool.Pool{},
		Redis:           redisClient,
		ClientID:        "portal-gateway",
		Keys:            map[string]string{"k1": "career-portal-secret-at-least-32-bytes-long"},
		ConsoleClientID: "console-gateway-career",
		ConsoleKeys:     map[string]string{"ck1": consoleSecret},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	srv := httptest.NewServer(svc)
	defer srv.Close()

	signReq := func(method, path, actorID, scopeKind, product, perm string) *http.Response {
		req, _ := http.NewRequest(method, srv.URL+path, nil)
		ts := strconv.FormatInt(time.Now().Unix(), 10)
		nonce := base64.RawURLEncoding.EncodeToString([]byte(uuid.NewString()[:24]))
		sum := sha256.Sum256(nil)
		canonical := strings.Join([]string{method, path, ts, nonce, hex.EncodeToString(sum[:]), actorID}, "\n")
		mac := hmac.New(sha256.New, []byte(consoleSecret))
		_, _ = mac.Write([]byte(canonical))
		req.SetBasicAuth("console-gateway-career", consoleSecret)
		req.Header.Set("X-Service-Id", "console-gateway-career")
		req.Header.Set("X-Key-Id", "ck1")
		req.Header.Set("X-Actor-User-Id", actorID)
		req.Header.Set("X-Timestamp", ts)
		req.Header.Set("X-Nonce", nonce)
		req.Header.Set("X-Signature", base64.RawURLEncoding.EncodeToString(mac.Sum(nil)))
		req.Header.Set("X-Scope-Kind", scopeKind)
		req.Header.Set("X-Product-Code", product)
		req.Header.Set("X-Permission-Code", perm)
		resp, err := srv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = resp.Body.Close() })
		return resp
	}

	const actorID = "11111111-1111-4111-8111-111111111111"
	// Unauthenticated -> 401
	unauth, err := srv.Client().Get(srv.URL + "/api/v1/console/workspace")
	if err != nil {
		t.Fatal(err)
	}
	_ = unauth.Body.Close()
	if unauth.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauth status = %d, want 401", unauth.StatusCode)
	}
	// Wrong product scope -> 403
	if resp := signReq(http.MethodGet, "/api/v1/console/workspace", actorID, "product", "food", "career.read"); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("wrong scope status = %d, want 403", resp.StatusCode)
	}
	// Wrong permission -> 403
	if resp := signReq(http.MethodGet, "/api/v1/console/workspace", actorID, "product", "career", "food.read"); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("wrong perm status = %d, want 403", resp.StatusCode)
	}
	// Retry digest missing Idempotency-Key -> 400
	if resp := signReq(http.MethodPost, "/api/v1/console/searches/"+actorID+"/retry-digest", actorID, "product", "career", "career.manage"); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("missing idempotency status = %d, want 400", resp.StatusCode)
	}
}
