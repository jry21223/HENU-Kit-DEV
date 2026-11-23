package httpapi

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"henukit.dev/portal-api/internal/campus"
)

func signCampusReq(req *http.Request, clientID, keyID, secret string, body string) {
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	var nonceBytes [24]byte
	_, _ = rand.Read(nonceBytes[:])
	nonce := base64.RawURLEncoding.EncodeToString(nonceBytes[:])
	sum := sha256.Sum256([]byte(body))
	canonical := strings.Join([]string{req.Method, req.URL.RequestURI(), ts, nonce, hex.EncodeToString(sum[:])}, "\n")
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(canonical))
	req.SetBasicAuth(clientID, secret)
	req.Header.Set("X-Service-Id", clientID)
	req.Header.Set("X-Key-Id", keyID)
	req.Header.Set("X-Timestamp", ts)
	req.Header.Set("X-Nonce", nonce)
	req.Header.Set("X-Signature", base64.RawURLEncoding.EncodeToString(mac.Sum(nil)))
}

func TestCampusWorkflowSecretSeparationAndFailClosed(t *testing.T) {
	t.Setenv("PORTAL_API_MODE", "mock")
	t.Setenv("STUDY_DATABASE_URL", "")
	t.Setenv("PORTAL_DATABASE_URL", "")

	// Overlapping portal & console secrets must fail closed at router construction.
	t.Setenv("CAMPUS_PORTAL_CLIENT_ID", "portal-gw")
	t.Setenv("CAMPUS_PORTAL_KEY_ID", "pk1")
	t.Setenv("CAMPUS_PORTAL_SECRET", "same-campus-secret-at-least-32-bytes")
	t.Setenv("CAMPUS_CONSOLE_CLIENT_ID", "console-gw")
	t.Setenv("CAMPUS_CONSOLE_KEY_ID", "ck1")
	t.Setenv("CAMPUS_CONSOLE_SECRET", "same-campus-secret-at-least-32-bytes")
	if _, err := NewRouter(); err == nil {
		t.Fatal("expected identical portal/console campus secret to fail")
	}

	// Unconfigured verifiers must fail closed with 503.
	t.Setenv("CAMPUS_PORTAL_SECRET", "")
	t.Setenv("CAMPUS_CONSOLE_SECRET", "")
	h, err := NewRouter()
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/campus/contact-book", nil))
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), `"code":"CAMPUS_WRITES_UNAVAILABLE"`) {
		t.Fatalf("unconfigured contact-book = %d %s", rec.Code, rec.Body)
	}

	// Configured verifiers enforce HMAC + actor + scope + permission.
	const pSecret = "campus-portal-secret-at-least-32-bytes"
	const cSecret = "campus-console-secret-at-least-32-bytes"
	t.Setenv("CAMPUS_PORTAL_SECRET", pSecret)
	t.Setenv("CAMPUS_CONSOLE_SECRET", cSecret)
	h, err = NewRouter()
	if err != nil {
		t.Fatalf("NewRouter configured: %v", err)
	}

	// Raw unsigned actor header must be rejected with 401.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/campus/contact-book", nil)
	req.Header.Set("X-Actor-User-Id", "22222222-2222-4222-8222-222222222222")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unsigned contact-book status = %d, want 401", rec.Code)
	}

	// Signed console summary works without actor header.
	sumReq := httptest.NewRequest(http.MethodGet, "/api/v1/campus/console-summary", nil)
	signCampusReq(sumReq, "console-gw", "ck1", cSecret, "")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, sumReq)
	if rec.Code != http.StatusOK {
		t.Fatalf("signed console-summary status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}

	// Signed console workspace with wrong permission -> 403.
	wsReq := httptest.NewRequest(http.MethodGet, "/api/v1/campus/console/workspace", nil)
	wsReq.Header.Set("X-Actor-User-Id", "11111111-1111-4111-8111-111111111111")
	wsReq.Header.Set("X-Scope-Kind", "product")
	wsReq.Header.Set("X-Product-Code", "campus")
	wsReq.Header.Set("X-Permission-Code", "food.read")
	signCampusReq(wsReq, "console-gw", "ck1", cSecret, "")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, wsReq)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("wrong permission workspace status = %d, want 403", rec.Code)
	}
}

func TestCampusSignedBodyLimits(t *testing.T) {
	const clientID, keyID, secret = "portal-gw", "pk1", "campus-portal-secret-at-least-32-bytes"
	t.Setenv("PORTAL_API_MODE", "mock")
	t.Setenv("STUDY_DATABASE_URL", "")
	t.Setenv("PORTAL_DATABASE_URL", "")
	t.Setenv("CAMPUS_PORTAL_CLIENT_ID", clientID)
	t.Setenv("CAMPUS_PORTAL_KEY_ID", keyID)
	t.Setenv("CAMPUS_PORTAL_SECRET", secret)
	router, err := NewRouter()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, path   string
		method       string
		size, status int
		code         string
		tamper       bool
	}{
		{"accept large signed create", "/api/v1/campus/items", http.MethodPost, 20 << 20, 503, "PORTAL_DATABASE_UNAVAILABLE", false},
		{"reject oversized create", "/api/v1/campus/items", http.MethodPost, (20 << 20) + 1, 413, "INVALID_REQUEST", false},
		{"hash final byte", "/api/v1/campus/items", http.MethodPost, (512 << 10) + 1, 401, "INVALID_SERVICE_AUTH", true},
		{"retain contact book limit", "/api/v1/campus/contact-book", http.MethodPut, (512 << 10) + 1, 413, "INVALID_REQUEST", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := strings.Repeat(" ", tc.size)
			signedBody := body
			if tc.tamper {
				signedBody = body[:len(body)-1] + "x"
			}
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(body))
			req.Header.Set("X-Actor-User-Id", "22222222-2222-4222-8222-222222222222")
			signCampusReq(req, clientID, keyID, secret, signedBody)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != tc.status || !strings.Contains(rec.Body.String(), tc.code) {
				t.Fatalf("response = %d %s, want %d %s", rec.Code, rec.Body, tc.status, tc.code)
			}
		})
	}
}

// Exercise the public router and actual migrations in a fresh PostgreSQL schema.
func TestCampusImageLifecycle(t *testing.T) {
	conn, dsn := campusImageTestDatabase(t)
	t.Setenv("PORTAL_API_MODE", "mock")
	t.Setenv("STUDY_DATABASE_URL", "")
	t.Setenv("PORTAL_DATABASE_URL", dsn)
	const clientID, keyID, secret = "portal-gw", "pk1", "campus-portal-secret-at-least-32-bytes"
	t.Setenv("CAMPUS_PORTAL_CLIENT_ID", clientID)
	t.Setenv("CAMPUS_PORTAL_KEY_ID", keyID)
	t.Setenv("CAMPUS_PORTAL_SECRET", secret)
	t.Setenv("CAMPUS_CONSOLE_SECRET", "")
	router, err := NewRouter()
	if err != nil {
		t.Fatal(err)
	}

	const itemsPath = "/api/v1/campus/items"
	doRequest := func(method, path, body, etag string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if method == http.MethodPost {
			req.Header.Set("X-Actor-User-Id", "22222222-2222-4222-8222-222222222222")
			signCampusReq(req, clientID, keyID, secret, body)
		}
		if etag != "" {
			req.Header.Set("If-None-Match", etag)
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}
	var inputs []campus.CampusImageInput
	for width := 1; width <= 6; width++ {
		var encoded bytes.Buffer
		if err := png.Encode(&encoded, image.NewGray(image.Rect(0, 0, width, 1))); err != nil {
			t.Fatal(err)
		}
		inputs = append(inputs, campus.CampusImageInput{
			ContentType: "image/png", Data: base64.StdEncoding.EncodeToString(encoded.Bytes()),
		})
	}
	input := campus.CreateItemInput{
		Type: "sell", Category: "books", Title: "二手教材", Desc: "自取", Place: "明伦校区", Price: 12, Images: inputs,
	}
	create := func(in campus.CreateItemInput) *httptest.ResponseRecorder {
		raw, err := json.Marshal(in)
		if err != nil {
			t.Fatal(err)
		}
		return doRequest(http.MethodPost, itemsPath, string(raw), "")
	}
	decodeItem := func(rec *httptest.ResponseRecorder, status int) campus.Item {
		t.Helper()
		var payload struct{ Item campus.Item }
		if rec.Code != status {
			t.Fatalf("response = %d %s, want %d", rec.Code, rec.Body, status)
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		return payload.Item
	}

	badInput := input
	badInput.Images = []campus.CampusImageInput{{ContentType: "image/jpeg", Data: inputs[0].Data}}
	if rec := create(badInput); rec.Code != 400 || !strings.Contains(rec.Body.String(), "INVALID_IMAGE") {
		t.Fatalf("invalid image response = %d %s", rec.Code, rec.Body)
	}

	// Reject the second image at the DB boundary after the listing and first
	// image have been inserted. Both inserts must roll back.
	if _, err := conn.Exec(`ALTER TABLE portal_campus_item_images ADD CONSTRAINT reject_second_image CHECK (position <> 1)`); err != nil {
		t.Fatal(err)
	}
	if rec := create(input); rec.Code != 503 || !strings.Contains(rec.Body.String(), "DATABASE_ERROR") {
		t.Fatalf("rolled-back create = %d %s", rec.Code, rec.Body)
	}
	if rec := doRequest(http.MethodGet, itemsPath, "", ""); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"items":[]`) {
		t.Fatalf("list after rollback = %d %s", rec.Code, rec.Body)
	}
	var imageCount int
	if err := conn.QueryRow(`SELECT count(*) FROM portal_campus_item_images`).Scan(&imageCount); err != nil || imageCount != 0 {
		t.Fatalf("image count after rollback = %d, err = %v", imageCount, err)
	}
	if _, err := conn.Exec(`ALTER TABLE portal_campus_item_images DROP CONSTRAINT reject_second_image`); err != nil {
		t.Fatal(err)
	}

	item := decodeItem(create(input), http.StatusCreated)
	if len(item.Images) != 6 {
		t.Fatalf("created item images = %v, want six", item.Images)
	}
	if detail := decodeItem(doRequest(http.MethodGet, itemsPath+"/"+item.ID, "", ""), 200); !slices.Equal(detail.Images, item.Images) {
		t.Fatalf("detail images = %v, want %v", detail.Images, item.Images)
	}
	rec := doRequest(http.MethodGet, itemsPath, "", "")
	var list struct{ Items []campus.Item }
	if err := json.Unmarshal(rec.Body.Bytes(), &list); rec.Code != 200 || err != nil || len(list.Items) != 1 || !slices.Equal(list.Items[0].Images, item.Images) {
		t.Fatalf("list response = %d %s, err = %v", rec.Code, rec.Body, err)
	}

	for i, imageURL := range item.Images {
		rec := doRequest(http.MethodGet, imageURL, "", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("image GET %s = %d %s", imageURL, rec.Code, rec.Body)
		}
		digest := fmt.Sprintf("%x", sha256.Sum256(rec.Body.Bytes()))
		wantURL := fmt.Sprintf("%s/%s/images/%d?v=%s", itemsPath, item.ID, i, digest[:12])
		if imageURL != wantURL {
			t.Fatalf("image URL = %q, want %q", imageURL, wantURL)
		}
		assertCampusImageHeaders(t, rec, digest)
		config, err := png.DecodeConfig(rec.Body)
		if err != nil || config.Width != i+1 {
			t.Fatalf("image %d dimensions = %v, err = %v", i, config, err)
		}
		rec = doRequest(http.MethodGet, imageURL, "", `"different", W/"`+digest+`"`)
		if rec.Code != 304 || rec.Body.Len() != 0 || rec.Header().Get("ETag") != `"`+digest+`"` {
			t.Fatalf("conditional image GET = %d %s, ETag = %q", rec.Code, rec.Body, rec.Header().Get("ETag"))
		}
	}

	if _, err := conn.Exec(`UPDATE portal_campus_items SET status = 'hidden' WHERE id = $1`, item.ID); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{item.Images[0], itemsPath + "/missing/images/0"} {
		if rec := doRequest(http.MethodGet, path, "", "*"); rec.Code != 404 || rec.Header().Get("ETag") != "" {
			t.Fatalf("unavailable image GET %s = %d %s", path, rec.Code, rec.Body)
		}
	}
	input.Images = nil
	if item := decodeItem(create(input), http.StatusCreated); item.Images == nil || len(item.Images) != 0 {
		t.Fatalf("listing without attachments should return images: [], got %#v", item.Images)
	}
}

func assertCampusImageHeaders(t *testing.T, rec *httptest.ResponseRecorder, digest string) {
	t.Helper()
	for header, want := range map[string]string{
		"Content-Type":            "image/png",
		"ETag":                    `"` + digest + `"`,
		"Cache-Control":           "public, max-age=31536000, immutable",
		"X-Content-Type-Options":  "nosniff",
		"Content-Security-Policy": "default-src 'none'; sandbox",
	} {
		if got := rec.Header().Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}
}

func campusImageTestDatabase(t *testing.T) (*sql.DB, string) {
	t.Helper()
	dsn := os.Getenv("PORTAL_API_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("PORTAL_API_TEST_DATABASE_URL is required for PostgreSQL integration")
	}
	dbURL, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	schema := fmt.Sprintf("campus_images_%d", time.Now().UnixNano())
	if _, err := admin.ExecContext(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := admin.ExecContext(ctx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Error(err)
		}
	})

	query := dbURL.Query()
	query.Set("search_path", schema)
	dbURL.RawQuery = query.Encode()
	conn, err := sql.Open("pgx", dbURL.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	migrations, err := filepath.Glob("../../db/migrations/postgres/*.up.sql")
	if err != nil || len(migrations) == 0 {
		t.Fatalf("find portal migrations: %v", err)
	}
	for _, migration := range migrations {
		source, err := os.ReadFile(migration)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := conn.ExecContext(ctx, string(source)); err != nil {
			t.Fatalf("apply %s: %v", migration, err)
		}
	}
	return conn, dbURL.String()
}
