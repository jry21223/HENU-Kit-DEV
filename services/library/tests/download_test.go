package tests

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
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	library "henukit.dev/library"
)

const downloadServiceSecret = "library-download-gateway-secret-32-bytes"

type fakeDownloadStore struct {
	mu              sync.Mutex
	state           library.DownloadObjectState
	found           bool
	headErr         error
	presignErr      error
	anonymousErr    error
	signed          library.SignedDownload
	headCalls       int
	presignCalls    int
	anonymousCalls  int
	lastKey         string
	lastVersion     string
	lastDisposition string
	lastTTL         time.Duration
	onPresign       func()
	transformSigned func(library.SignedDownload) library.SignedDownload
}

func (s *fakeDownloadStore) Head(ctx context.Context, key, versionID string) (library.DownloadObjectState, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.headCalls++
	s.lastKey = key
	s.lastVersion = versionID
	return s.state, s.found, s.headErr
}

func (s *fakeDownloadStore) PresignGet(ctx context.Context, key, versionID, contentDisposition string, ttl time.Duration) (library.SignedDownload, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.presignCalls++
	s.lastKey = key
	s.lastVersion = versionID
	s.lastDisposition = contentDisposition
	s.lastTTL = ttl
	if s.onPresign != nil {
		s.onPresign()
	}
	if ttl > time.Minute {
		return library.SignedDownload{}, errors.New("ttl exceeded one minute")
	}
	result := signedFixture(key, versionID, contentDisposition, ttl)
	if s.signed.URL != "" {
		result.URL = s.signed.URL
	}
	if !s.signed.ExpiresAt.IsZero() {
		result.ExpiresAt = s.signed.ExpiresAt
	}
	if s.transformSigned != nil {
		result = s.transformSigned(result)
	}
	return result, s.presignErr
}

func (s *fakeDownloadStore) AnonymousDenied(context.Context, string, string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.anonymousCalls++
	return s.anonymousErr
}

func TestPublicDownloadStartRedirectsAndCountsEachSuccessfulGrant(t *testing.T) {
	materialID := uuid.New()
	releaseID := newReleaseID()
	sha := strings.Repeat("a", 64)
	receiptSHA := strings.Repeat("b", 64)
	versionID := "CAEQARiBgIDtest-version"
	store := &fakeDownloadStore{
		state: library.DownloadObjectState{Bytes: 2048, SHA256: sha, Encryption: "AES256", VersionID: versionID},
		found: true,
	}
	server, pool := newLibraryDownloadServer(t, store)
	defer server.Close()
	defer pool.Close()
	seedActivePublicMaterial(t, pool, releaseID, materialID, sha, versionID, 2048, "线性代数复习.pdf", "public_free", "published")
	globalBefore := totalLedgerCount(t, pool)

	for range 2 {
		response := sendDownload(t, server.URL, http.MethodPost, "/api/v1/public-materials/"+materialID.String()+"/download-starts", nil)
		if response.StatusCode != http.StatusCreated {
			t.Fatalf("download start status = %d", response.StatusCode)
		}
		var grant struct {
			Data struct {
				DownloadStartID string `json:"download_start_id"`
				Method          string `json:"method"`
				Location        string `json:"location"`
				ExpiresAt       string `json:"expires_at"`
			} `json:"data"`
		}
		if err := json.NewDecoder(response.Body).Decode(&grant); err != nil {
			t.Fatal(err)
		}
		if grant.Data.DownloadStartID == "" || grant.Data.Method != http.MethodGet || grant.Data.Location == "" || grant.Data.ExpiresAt == "" {
			t.Fatalf("grant = %#v", grant.Data)
		}
		if response.Header.Get("Cache-Control") != "no-store" {
			t.Fatalf("unsafe redirect headers: %#v", response.Header)
		}
		response.Body.Close()
	}

	materialAggregate := sendDownload(t, server.URL, http.MethodGet, "/api/v1/public-materials/"+materialID.String()+"/download-starts/aggregate", nil)
	assertAggregate(t, materialAggregate, 2)
	globalAggregate := sendDownload(t, server.URL, http.MethodGet, "/api/v1/public-materials/download-starts/aggregate", nil)
	assertAggregate(t, globalAggregate, globalBefore+2)

	var count int
	var leaked bool
	if err := pool.QueryRow(context.Background(), `SELECT count(*), bool_or(to_jsonb(e)::text LIKE '%x-oss-%' OR to_jsonb(e)::text LIKE '%127.0.0.1%' OR to_jsonb(e)::text LIKE '%user-agent%') FROM library_download_start_events e WHERE material_id=$1`, materialID).Scan(&count, &leaked); err != nil {
		t.Fatal(err)
	}
	if count != 2 || leaked {
		t.Fatalf("ledger count=%d leaked=%t", count, leaked)
	}
	expectedKey := "releases/" + releaseID + "/receipts/" + receiptSHA + "/objects/" + sha + "/object.pdf"
	if store.lastKey != expectedKey || store.lastVersion != versionID || store.lastTTL > time.Minute || !strings.HasPrefix(store.lastDisposition, "attachment;") {
		t.Fatalf("unsafe signing inputs: key=%q version=%q ttl=%s disposition=%q", store.lastKey, store.lastVersion, store.lastTTL, store.lastDisposition)
	}
	if _, err := pool.Exec(context.Background(), `UPDATE library_download_start_events SET request_id='req_changed' WHERE material_id=$1`, materialID); err == nil {
		t.Fatal("append-only ledger accepted update")
	}
}

func TestPublicDownloadStartFailuresNeverCreateAFalseCount(t *testing.T) {
	for _, tc := range []struct {
		name       string
		access     string
		status     string
		fileName   string
		seed       bool
		retire     bool
		mutate     func(*fakeDownloadStore)
		wantStatus int
	}{
		{name: "unknown", seed: false, wantStatus: http.StatusNotFound},
		{name: "inactive", seed: true, retire: true, access: "public_free", status: "published", wantStatus: http.StatusNotFound},
		{name: "withdrawn", seed: true, access: "public_free", status: "withdrawn", wantStatus: http.StatusNotFound},
		{name: "non free", seed: true, access: "authenticated", status: "published", wantStatus: http.StatusNotFound},
		{name: "object missing", seed: true, access: "public_free", status: "published", mutate: func(s *fakeDownloadStore) { s.found = false }, wantStatus: http.StatusServiceUnavailable},
		{name: "head failure", seed: true, access: "public_free", status: "published", mutate: func(s *fakeDownloadStore) { s.headErr = errors.New("head failed") }, wantStatus: http.StatusServiceUnavailable},
		{name: "bytes mismatch", seed: true, access: "public_free", status: "published", mutate: func(s *fakeDownloadStore) { s.state.Bytes++ }, wantStatus: http.StatusServiceUnavailable},
		{name: "sha mismatch", seed: true, access: "public_free", status: "published", mutate: func(s *fakeDownloadStore) { s.state.SHA256 = strings.Repeat("c", 64) }, wantStatus: http.StatusServiceUnavailable},
		{name: "encryption mismatch", seed: true, access: "public_free", status: "published", mutate: func(s *fakeDownloadStore) { s.state.Encryption = "" }, wantStatus: http.StatusServiceUnavailable},
		{name: "version mismatch", seed: true, access: "public_free", status: "published", mutate: func(s *fakeDownloadStore) { s.state.VersionID = "other-version" }, wantStatus: http.StatusServiceUnavailable},
		{name: "anonymous object is public", seed: true, access: "public_free", status: "published", mutate: func(s *fakeDownloadStore) { s.anonymousErr = errors.New("anonymous request was not denied") }, wantStatus: http.StatusServiceUnavailable},
		{name: "unsafe filename", seed: true, access: "public_free", status: "published", fileName: "../secret.pdf", wantStatus: http.StatusServiceUnavailable},
		{name: "sign failure", seed: true, access: "public_free", status: "published", mutate: func(s *fakeDownloadStore) { s.presignErr = errors.New("sign failed") }, wantStatus: http.StatusServiceUnavailable},
		{name: "unsafe signed host", seed: true, access: "public_free", status: "published", mutate: func(s *fakeDownloadStore) { s.signed.URL = "https://attacker.example/object?signature=x" }, wantStatus: http.StatusServiceUnavailable},
		{name: "missing temporary token", seed: true, access: "public_free", status: "published", mutate: func(s *fakeDownloadStore) { s.transformSigned = deleteSignedQuery("x-oss-security-token") }, wantStatus: http.StatusServiceUnavailable},
		{name: "missing attachment disposition", seed: true, access: "public_free", status: "published", mutate: func(s *fakeDownloadStore) { s.transformSigned = deleteSignedQuery("response-content-disposition") }, wantStatus: http.StatusServiceUnavailable},
		{name: "long query ttl", seed: true, access: "public_free", status: "published", mutate: func(s *fakeDownloadStore) { s.transformSigned = replaceSignedQuery("x-oss-expires", "3600") }, wantStatus: http.StatusServiceUnavailable},
		{name: "unexpected query", seed: true, access: "public_free", status: "published", mutate: func(s *fakeDownloadStore) { s.transformSigned = replaceSignedQuery("unexpected", "value") }, wantStatus: http.StatusServiceUnavailable},
		{name: "malformed query", seed: true, access: "public_free", status: "published", mutate: func(s *fakeDownloadStore) { s.transformSigned = appendMalformedSignedQuery }, wantStatus: http.StatusServiceUnavailable},
		{name: "expired grant", seed: true, access: "public_free", status: "published", mutate: func(s *fakeDownloadStore) { s.signed.ExpiresAt = time.Now().Add(-time.Second) }, wantStatus: http.StatusServiceUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			materialID := uuid.New()
			releaseID := newReleaseID()
			sha := strings.Repeat("a", 64)
			versionID := "CAEQARiBgIDfailure-version"
			store := validFakeStore(releaseID, sha, versionID, 2048)
			if tc.mutate != nil {
				tc.mutate(store)
			}
			server, pool := newLibraryDownloadServer(t, store)
			defer server.Close()
			defer pool.Close()
			if tc.seed {
				fileName := tc.fileName
				if fileName == "" {
					fileName = "复习资料.pdf"
				}
				seedActivePublicMaterial(t, pool, releaseID, materialID, sha, versionID, 2048, fileName, tc.access, tc.status)
				if tc.retire {
					if _, err := pool.Exec(context.Background(), `UPDATE library_public_releases SET state='retired' WHERE release_id=$1`, releaseID); err != nil {
						t.Fatal(err)
					}
				}
			}

			response := sendDownload(t, server.URL, http.MethodPost, "/api/v1/public-materials/"+materialID.String()+"/download-starts", nil)
			defer response.Body.Close()
			if response.StatusCode != tc.wantStatus {
				t.Fatalf("status = %d, want %d", response.StatusCode, tc.wantStatus)
			}
			if got := ledgerCount(t, pool, materialID); got != 0 {
				t.Fatalf("failed request created %d events", got)
			}
		})
	}
}

func TestDownloadStartRejectsStorageAndFilenameInjection(t *testing.T) {
	materialID := uuid.New()
	releaseID := newReleaseID()
	sha, versionID := strings.Repeat("a", 64), "exact-version"
	store := validFakeStore(releaseID, sha, versionID, 2)
	server, pool := newLibraryDownloadServer(t, store)
	defer server.Close()
	defer pool.Close()
	seedActivePublicMaterial(t, pool, releaseID, materialID, sha, versionID, 2, "safe.pdf", "public_free", "published")

	for _, request := range []struct {
		name, path string
		body       []byte
	}{
		{name: "query", path: "/api/v1/public-materials/" + materialID.String() + "/download-starts?object_key=evil&ttl=3600&filename=evil.exe"},
		{name: "body", path: "/api/v1/public-materials/" + materialID.String() + "/download-starts", body: []byte(`{"version_id":"evil"}`)},
	} {
		t.Run(request.name, func(t *testing.T) {
			response := sendDownload(t, server.URL, http.MethodPost, request.path, request.body)
			defer response.Body.Close()
			if response.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d", response.StatusCode)
			}
		})
	}
	if store.headCalls != 0 || store.presignCalls != 0 || ledgerCount(t, pool, materialID) != 0 {
		t.Fatalf("injected input reached object store or ledger: head=%d sign=%d", store.headCalls, store.presignCalls)
	}
}

func TestDownloadStartLedgerFailureAndActivationRaceDiscardGrant(t *testing.T) {
	for _, tc := range []struct {
		name          string
		beforeRequest func(*testing.T, *pgxpool.Pool, *fakeDownloadStore, string)
		wantStatus    int
	}{
		{
			name: "ledger failure",
			beforeRequest: func(t *testing.T, pool *pgxpool.Pool, store *fakeDownloadStore, releaseID string) {
				_, err := pool.Exec(context.Background(), `
					CREATE OR REPLACE FUNCTION library_test_reject_download_start() RETURNS trigger AS $$ BEGIN RAISE EXCEPTION 'fixture ledger failure'; END; $$ LANGUAGE plpgsql;
					CREATE TRIGGER library_test_reject_download_start BEFORE INSERT ON library_download_start_events FOR EACH STATEMENT EXECUTE FUNCTION library_test_reject_download_start();`)
				if err != nil {
					t.Fatal(err)
				}
			},
			wantStatus: http.StatusServiceUnavailable,
		},
		{
			name: "activation race",
			beforeRequest: func(t *testing.T, pool *pgxpool.Pool, store *fakeDownloadStore, releaseID string) {
				store.onPresign = func() {
					if _, err := pool.Exec(context.Background(), `UPDATE library_public_releases SET state='retired' WHERE release_id=$1`, releaseID); err != nil {
						t.Error(err)
					}
				}
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name: "filename race",
			beforeRequest: func(t *testing.T, pool *pgxpool.Pool, store *fakeDownloadStore, releaseID string) {
				store.onPresign = func() {
					if _, err := pool.Exec(context.Background(), `UPDATE library_public_material_snapshots SET file_name='changed.pdf' WHERE release_id=$1`, releaseID); err != nil {
						t.Error(err)
					}
				}
			},
			wantStatus: http.StatusNotFound,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			materialID := uuid.New()
			releaseID := newReleaseID()
			sha, versionID := strings.Repeat("a", 64), "exact-version"
			store := validFakeStore(releaseID, sha, versionID, 2)
			server, pool := newLibraryDownloadServer(t, store)
			defer server.Close()
			defer pool.Close()
			seedActivePublicMaterial(t, pool, releaseID, materialID, sha, versionID, 2, "safe.pdf", "public_free", "published")
			tc.beforeRequest(t, pool, store, releaseID)

			response := sendDownload(t, server.URL, http.MethodPost, "/api/v1/public-materials/"+materialID.String()+"/download-starts", nil)
			defer response.Body.Close()
			if response.StatusCode != tc.wantStatus {
				t.Fatalf("status = %d, want %d", response.StatusCode, tc.wantStatus)
			}
			if tc.name == "ledger failure" {
				if _, err := pool.Exec(context.Background(), `DROP TRIGGER IF EXISTS library_test_reject_download_start ON library_download_start_events; DROP FUNCTION IF EXISTS library_test_reject_download_start()`); err != nil {
					t.Fatal(err)
				}
			}
			if ledgerCount(t, pool, materialID) != 0 {
				t.Fatal("discarded grant created a ledger event")
			}
		})
	}
}

func TestDownloadStartRejectsForeignReceiptObjectKeyBeforeOSS(t *testing.T) {
	materialID := uuid.New()
	releaseID := newReleaseID()
	sha, versionID := strings.Repeat("a", 64), "exact-version"
	store := validFakeStore(releaseID, sha, versionID, 2)
	server, pool := newLibraryDownloadServer(t, store)
	defer server.Close()
	defer pool.Close()
	seedActivePublicMaterial(t, pool, releaseID, materialID, sha, versionID, 2, "safe.pdf", "public_free", "published")
	foreignKey := "releases/" + releaseID + "/receipts/" + strings.Repeat("c", 64) + "/objects/" + sha + "/safe.pdf"
	if _, err := pool.Exec(context.Background(), `UPDATE library_public_material_snapshots SET object_key=$1 WHERE release_id=$2 AND material_id=$3`, foreignKey, releaseID, materialID); err != nil {
		t.Fatal(err)
	}

	response := sendDownload(t, server.URL, http.MethodPost, "/api/v1/public-materials/"+materialID.String()+"/download-starts", nil)
	defer response.Body.Close()
	if response.StatusCode != http.StatusServiceUnavailable || store.headCalls != 0 || store.presignCalls != 0 || ledgerCount(t, pool, materialID) != 0 {
		t.Fatalf("foreign receipt status=%d head=%d sign=%d", response.StatusCode, store.headCalls, store.presignCalls)
	}
}

func TestDownloadCapabilityIsIndependentFromConsoleActorAuth(t *testing.T) {
	server, pool := newLibraryDownloadServer(t, &fakeDownloadStore{})
	defer server.Close()
	defer pool.Close()

	consoleCredential := sendSignedService(t, server.URL, "console-gateway", serviceSecret, http.MethodGet, "/api/v1/public-materials/download-starts/aggregate", nil)
	defer consoleCredential.Body.Close()
	if consoleCredential.StatusCode != http.StatusUnauthorized {
		t.Fatalf("console credential status = %d", consoleCredential.StatusCode)
	}
	downloadCredential := sendDownload(t, server.URL, http.MethodGet, "/api/v1/public-materials/download-starts/aggregate", nil)
	defer downloadCredential.Body.Close()
	if downloadCredential.StatusCode != http.StatusOK {
		t.Fatalf("download credential status = %d", downloadCredential.StatusCode)
	}

	redisClient := redis.NewClient(&redis.Options{Addr: envRedisAddr(t)})
	defer redisClient.Close()
	if _, err := library.New(library.Config{
		Database: pool, Redis: redisClient,
		ClientID: "console-gateway", Keys: map[string]string{"console": serviceSecret},
		DownloadClientID: "portal-gateway", DownloadKeys: map[string]string{"download": serviceSecret}, DownloadStore: &fakeDownloadStore{},
		LegacyBaseURL: "http://127.0.0.1:1", LegacyToken: "legacy-token",
	}); err == nil {
		t.Fatal("Library accepted overlapping Console and download HMAC secrets")
	}
}

func TestAggregatesDoNotMultiplyEventsAcrossReleases(t *testing.T) {
	materialID := uuid.New()
	sha := strings.Repeat("a", 64)
	firstRelease, firstVersion := newReleaseID(), "version-one"
	store := validFakeStore(firstRelease, sha, firstVersion, 2)
	server, pool := newLibraryDownloadServer(t, store)
	defer server.Close()
	defer pool.Close()
	seedActivePublicMaterial(t, pool, firstRelease, materialID, sha, firstVersion, 2, "safe.pdf", "public_free", "published")
	start := sendDownload(t, server.URL, http.MethodPost, "/api/v1/public-materials/"+materialID.String()+"/download-starts", nil)
	start.Body.Close()
	if start.StatusCode != http.StatusCreated {
		t.Fatalf("first start status = %d", start.StatusCode)
	}

	secondRelease, secondVersion := newReleaseID(), "version-two"
	store.state.VersionID = secondVersion
	store.signed = library.SignedDownload{}
	seedActivePublicMaterial(t, pool, secondRelease, materialID, sha, secondVersion, 2, "safe.pdf", "public_free", "published")
	start = sendDownload(t, server.URL, http.MethodPost, "/api/v1/public-materials/"+materialID.String()+"/download-starts", nil)
	start.Body.Close()
	if start.StatusCode != http.StatusCreated {
		t.Fatalf("second start status = %d", start.StatusCode)
	}
	materialAggregate := sendDownload(t, server.URL, http.MethodGet, "/api/v1/public-materials/"+materialID.String()+"/download-starts/aggregate", nil)
	assertAggregate(t, materialAggregate, 2)

	var globalCount int64
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM library_download_start_events`).Scan(&globalCount); err != nil {
		t.Fatal(err)
	}
	globalAggregate := sendDownload(t, server.URL, http.MethodGet, "/api/v1/public-materials/download-starts/aggregate", nil)
	assertAggregate(t, globalAggregate, globalCount)
}

func TestSnapshotRejectsForeignReleaseObjectKey(t *testing.T) {
	server, pool := newLibraryDownloadServer(t, &fakeDownloadStore{})
	defer server.Close()
	defer pool.Close()
	releaseID := newReleaseID()
	if _, err := pool.Exec(context.Background(), `UPDATE library_public_releases SET state='retired' WHERE state='active'`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), `INSERT INTO library_public_releases (release_id,receipt_sha256,state,activated_at) VALUES ($1,$2,'active',now())`, releaseID, strings.Repeat("b", 64)); err != nil {
		t.Fatal(err)
	}
	_, err := pool.Exec(context.Background(), `INSERT INTO library_public_material_snapshots (release_id,material_id,title,file_name,access_level,status,object_key,object_version_id,sha256,byte_size) VALUES ($1,$2,'title','file.pdf','public_free','published',$3,'v1',$4,1)`, releaseID, uuid.New(), "releases/"+newReleaseID()+"/file.pdf", strings.Repeat("a", 64))
	if err == nil {
		t.Fatal("snapshot accepted an object key from another release")
	}
}

func validFakeStore(releaseID, sha, versionID string, size int64) *fakeDownloadStore {
	return &fakeDownloadStore{
		state: library.DownloadObjectState{Bytes: size, SHA256: sha, Encryption: "AES256", VersionID: versionID},
		found: true,
	}
}

func signedFixture(key, versionID, disposition string, ttl time.Duration) library.SignedDownload {
	signedAt := time.Now().UTC().Truncate(time.Second)
	query := url.Values{
		"versionId":                    {versionID},
		"response-cache-control":       {"private, no-store"},
		"response-content-disposition": {disposition},
		"x-oss-credential":             {"temporary/20260811/cn-beijing/oss/aliyun_v4_request"},
		"x-oss-date":                   {signedAt.Format("20060102T150405Z")},
		"x-oss-expires":                {strconv.Itoa(int(ttl / time.Second))},
		"x-oss-security-token":         {"temporary-token"},
		"x-oss-signature":              {"bounded-signature"},
		"x-oss-signature-version":      {"OSS4-HMAC-SHA256"},
	}
	location := url.URL{Scheme: "https", Host: "henukit.oss-cn-beijing.aliyuncs.com", Path: "/" + key, RawQuery: query.Encode()}
	return library.SignedDownload{URL: location.String(), ExpiresAt: signedAt.Add(ttl)}
}

func deleteSignedQuery(name string) func(library.SignedDownload) library.SignedDownload {
	return func(signed library.SignedDownload) library.SignedDownload {
		parsed, _ := url.Parse(signed.URL)
		query := parsed.Query()
		query.Del(name)
		parsed.RawQuery = query.Encode()
		signed.URL = parsed.String()
		return signed
	}
}

func replaceSignedQuery(name, value string) func(library.SignedDownload) library.SignedDownload {
	return func(signed library.SignedDownload) library.SignedDownload {
		parsed, _ := url.Parse(signed.URL)
		query := parsed.Query()
		query.Set(name, value)
		parsed.RawQuery = query.Encode()
		signed.URL = parsed.String()
		return signed
	}
}

func appendMalformedSignedQuery(signed library.SignedDownload) library.SignedDownload {
	signed.URL += "&unexpected=a;b"
	return signed
}

func ledgerCount(t *testing.T, pool *pgxpool.Pool, materialID uuid.UUID) int64 {
	t.Helper()
	var count int64
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM library_download_start_events WHERE material_id=$1`, materialID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func totalLedgerCount(t *testing.T, pool *pgxpool.Pool) int64 {
	t.Helper()
	var count int64
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM library_download_start_events`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func newLibraryDownloadServer(t *testing.T, store library.DownloadObjectStore) (*httptest.Server, *pgxpool.Pool) {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), envDatabaseURL(t))
	if err != nil {
		t.Fatal(err)
	}
	redisClient := redis.NewClient(&redis.Options{Addr: envRedisAddr(t)})
	if err := redisClient.FlushDB(context.Background()).Err(); err != nil {
		t.Fatal(err)
	}
	legacy := newLegacyServer(t)
	t.Cleanup(legacy.Close)
	handler, err := library.New(library.Config{
		Database: pool, Redis: redisClient,
		ClientID: "console-gateway", Keys: map[string]string{"active": serviceSecret},
		DownloadClientID: "portal-gateway", DownloadKeys: map[string]string{"active": downloadServiceSecret}, DownloadStore: store,
		LegacyBaseURL: legacy.URL, LegacyToken: "legacy-admin-token", HTTPClient: http.DefaultClient,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = redisClient.Close() })
	return httptest.NewServer(handler), pool
}

func seedActivePublicMaterial(t *testing.T, pool *pgxpool.Pool, releaseID string, materialID uuid.UUID, sha, versionID string, size int64, fileName, access, status string) {
	t.Helper()
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `UPDATE library_public_releases SET state='retired' WHERE state='active'`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO library_public_releases (release_id, receipt_sha256, state, activated_at) VALUES ($1,$2,'active',now())`, releaseID, strings.Repeat("b", 64)); err != nil {
		t.Fatal(err)
	}
	objectKey := "releases/" + releaseID + "/receipts/" + strings.Repeat("b", 64) + "/objects/" + sha + "/object.pdf"
	if _, err := pool.Exec(ctx, `INSERT INTO library_public_material_snapshots (release_id,material_id,title,file_name,access_level,status,object_key,object_version_id,sha256,byte_size) VALUES ($1,$2,'线性代数复习资料',$3,$4,$5,$6,$7,$8,$9)`, releaseID, materialID, fileName, access, status, objectKey, versionID, sha, size); err != nil {
		t.Fatal(err)
	}
}

func newReleaseID() string {
	return strings.ReplaceAll(uuid.NewString(), "-", "") + "abcdef12-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:16]
}

func sendDownload(t *testing.T, baseURL, method, path string, body []byte) *http.Response {
	t.Helper()
	return sendSignedService(t, baseURL, "portal-gateway", downloadServiceSecret, method, path, body)
}

func sendSignedService(t *testing.T, baseURL, clientID, secret, method, path string, body []byte) *http.Response {
	t.Helper()
	request, err := http.NewRequest(method, baseURL+path, strings.NewReader(string(body)))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("X-Service-Id", clientID)
	request.Header.Set("X-Key-Id", "active")
	request.Header.Set("X-Request-Id", "req_download_test_"+strings.ReplaceAll(uuid.NewString(), "-", ""))
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	nonce := base64.RawURLEncoding.EncodeToString([]byte(uuid.NewString()[:24]))
	digest := sha256.Sum256(body)
	canonical := strings.Join([]string{method, path, timestamp, nonce, hex.EncodeToString(digest[:])}, "\n")
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(canonical))
	request.Header.Set("X-Timestamp", timestamp)
	request.Header.Set("X-Nonce", nonce)
	request.Header.Set("X-Signature", base64.RawURLEncoding.EncodeToString(mac.Sum(nil)))
	request.SetBasicAuth(clientID, secret)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func assertAggregate(t *testing.T, response *http.Response, expected int64) {
	t.Helper()
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("aggregate status = %d", response.StatusCode)
	}
	var payload struct {
		Data struct {
			DownloadStarts int64 `json:"download_starts"`
		} `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.Data.DownloadStarts != expected {
		t.Fatalf("aggregate = %d, want %d", payload.Data.DownloadStarts, expected)
	}
}

func envDatabaseURL(t *testing.T) string { t.Helper(); return mustEnv(t, "LIBRARY_TEST_DATABASE_URL") }
func envRedisAddr(t *testing.T) string   { t.Helper(); return mustEnv(t, "LIBRARY_TEST_REDIS_ADDR") }
func mustEnv(t *testing.T, key string) string {
	t.Helper()
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		t.Fatalf("%s is required", key)
	}
	return value
}
