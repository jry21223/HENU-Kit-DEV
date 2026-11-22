package tests

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	library "henukit.dev/library"
)

const serviceSecret = "library-gateway-secret-at-least-32-bytes"

func TestBoundedWorkspaceWithoutLeakingLegacyDomains(t *testing.T) {
	server, pool := newLibraryServer(t)
	defer server.Close()
	defer pool.Close()

	response := send(t, server.URL, "library.read", http.MethodGet, "/api/v1/workspace", nil, "")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("workspace status = %d", response.StatusCode)
	}
	body := readBody(t, response)
	for _, expected := range []string{`"courses"`, `"materials"`, `"downloads"`, `"submissions"`, `"corrections"`, `"status":"ok"`, `"degraded":false`} {
		if !bytes.Contains(body, []byte(expected)) {
			t.Fatalf("workspace omitted %s: %s", expected, body)
		}
	}
	for _, forbidden := range []string{"payment", "membership", "member_only", "paid", "points", "forum", "quiz", "userAgent", "127.0.0.1"} {
		if bytes.Contains(bytes.ToLower(body), []byte(strings.ToLower(forbidden))) {
			t.Fatalf("workspace leaked %q: %s", forbidden, body)
		}
	}
	if _, err := pool.Exec(context.Background(), "TRUNCATE library_adapter_audit_events"); err == nil {
		t.Fatal("append-only audit accepted TRUNCATE")
	}
}

func TestNativeCatalogCommandsAndOperationStatus(t *testing.T) {
	server, pool := newLibraryServer(t)
	defer server.Close()
	defer pool.Close()

	courseCode := "TEST-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	createCourseKey := "idem_course_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:16]
	createCourseBody := []byte(fmt.Sprintf(`{"kind":"course_create","payload":{"code":%q,"name":"数据结构测试课","department":"计算机学院"}}`, courseCode))
	resp := send(t, server.URL, "library.manage", http.MethodPost, "/api/v1/commands", createCourseBody, createCourseKey)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("course_create status = %d, body = %s", resp.StatusCode, readBody(t, resp))
	}
	var courseEnvelope struct {
		Data struct {
			Operation  string `json:"operation"`
			ResourceID string `json:"resource_id"`
			State      string `json:"state"`
			Version    int    `json:"version"`
		} `json:"data"`
	}
	if err := json.Unmarshal(readBody(t, resp), &courseEnvelope); err != nil {
		t.Fatal(err)
	}
	courseID := courseEnvelope.Data.ResourceID
	if courseID == "" || courseEnvelope.Data.State != "succeeded" || courseEnvelope.Data.Version != 1 {
		t.Fatalf("unexpected course_create result: %+v", courseEnvelope.Data)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM library_materials WHERE course_id = $1::uuid`, courseID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM library_courses WHERE id = $1::uuid`, courseID)
	})

	// Replay with identical Idempotency-Key returns the original succeeded result.
	replay := send(t, server.URL, "library.manage", http.MethodPost, "/api/v1/commands", createCourseBody, createCourseKey)
	if replay.StatusCode != http.StatusOK {
		t.Fatalf("course_create replay status = %d", replay.StatusCode)
	}
	if !bytes.Contains(readBody(t, replay), []byte(courseID)) {
		t.Fatal("replay response did not preserve resource_id")
	}

	operation := send(t, server.URL, "library.manage", http.MethodGet, "/api/v1/operations/course_create", nil, createCourseKey)
	if operation.StatusCode != http.StatusOK {
		t.Fatalf("operation status = %d", operation.StatusCode)
	}
	if !bytes.Contains(readBody(t, operation), []byte(courseID)) {
		t.Fatal("operation status omitted resource_id")
	}

	// Create a pending_review material and approve it via library.review.
	createMaterialKey := "idem_mat_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:16]
	createMaterialBody := []byte(fmt.Sprintf(`{"kind":"material_create","payload":{"course_id":%q,"title":"测试讲义","material_type":"handout","access_level":"public_free","status":"pending_review"}}`, courseID))
	matResp := send(t, server.URL, "library.manage", http.MethodPost, "/api/v1/commands", createMaterialBody, createMaterialKey)
	if matResp.StatusCode != http.StatusOK {
		t.Fatalf("material_create status = %d, body = %s", matResp.StatusCode, readBody(t, matResp))
	}
	var matEnvelope struct {
		Data struct {
			ResourceID string `json:"resource_id"`
			Version    int    `json:"version"`
		} `json:"data"`
	}
	if err := json.Unmarshal(readBody(t, matResp), &matEnvelope); err != nil {
		t.Fatal(err)
	}
	materialID := matEnvelope.Data.ResourceID

	approveKey := "idem_approve_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:16]
	approveBody := []byte(fmt.Sprintf(`{"kind":"submission_approve","resource_id":%q,"expected_version":"1","payload":{"note":"checked"}}`, materialID))
	approveResp := send(t, server.URL, "library.review", http.MethodPost, "/api/v1/commands", approveBody, approveKey)
	if approveResp.StatusCode != http.StatusOK {
		t.Fatalf("submission_approve status = %d, body = %s", approveResp.StatusCode, readBody(t, approveResp))
	}
	if !bytes.Contains(readBody(t, approveResp), []byte(`"version":2`)) {
		t.Fatal("submission_approve did not bump version to 2")
	}

	// Stale expected_version must be rejected with 409 VERSION_CONFLICT.
	staleResp := send(t, server.URL, "library.review", http.MethodPost, "/api/v1/commands", approveBody, "idem_stale_"+strings.ReplaceAll(uuid.NewString(), "-", "")[:16])
	if staleResp.StatusCode != http.StatusConflict {
		t.Fatalf("stale submission_approve status = %d", staleResp.StatusCode)
	}
	stalePayload, _ := io.ReadAll(staleResp.Body)
	staleResp.Body.Close()
	if !bytes.Contains(stalePayload, []byte("VERSION_CONFLICT")) {
		t.Fatalf("stale response missing VERSION_CONFLICT: %s", stalePayload)
	}
}

func TestScopeAndCommandAllowlistDefaultDeny(t *testing.T) {
	server, pool := newLibraryServer(t)
	defer server.Close()
	defer pool.Close()

	denied := send(t, server.URL, "notice.read", http.MethodGet, "/api/v1/workspace", nil, "")
	if denied.StatusCode != http.StatusForbidden {
		t.Fatalf("foreign scope status = %d", denied.StatusCode)
	}
	command := send(t, server.URL, "library.manage", http.MethodPost, "/api/v1/commands", []byte(`{"kind":"payment_refund","resource_id":"x","expected_version":"1","payload":{}}`), "idem_forbidden_command")
	if command.StatusCode != http.StatusBadRequest {
		t.Fatalf("forbidden command status = %d", command.StatusCode)
	}
	cmdPayload, _ := io.ReadAll(command.Body)
	command.Body.Close()
	if !bytes.Contains(cmdPayload, []byte("UNSUPPORTED_COMMAND")) {
		t.Fatalf("forbidden command code missing: %s", cmdPayload)
	}
	commercial := send(t, server.URL, "library.manage", http.MethodPost, "/api/v1/commands", []byte(`{"kind":"material_update","resource_id":"22222222-2222-4222-8222-222222222222","expected_version":"1","payload":{"access_level":"member_only"}}`), "idem_forbidden_access")
	if commercial.StatusCode != http.StatusBadRequest {
		t.Fatalf("commercial access level status = %d", commercial.StatusCode)
	}
	commPayload, _ := io.ReadAll(commercial.Body)
	commercial.Body.Close()
	if !bytes.Contains(commPayload, []byte("INVALID_ACCESS_LEVEL")) {
		t.Fatalf("commercial access level code missing: %s", commPayload)
	}
	missingCreate := send(t, server.URL, "library.manage", http.MethodPost, "/api/v1/commands", []byte(`{"kind":"course_create","payload":{}}`), "idem_invalid_create_missing")
	if missingCreate.StatusCode != http.StatusBadRequest {
		t.Fatalf("missing create fields status = %d", missingCreate.StatusCode)
	}
	// A foreign permission still cannot reach the command surface.
	deniedCommand := send(t, server.URL, "notice.read", http.MethodPost, "/api/v1/commands", []byte(`{"kind":"course_create","payload":{}}`), "idem_denied_command")
	if deniedCommand.StatusCode != http.StatusForbidden {
		t.Fatalf("foreign-scope command status = %d", deniedCommand.StatusCode)
	}
}

func newLibraryServer(t *testing.T) (*httptest.Server, *pgxpool.Pool) {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), os.Getenv("LIBRARY_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	redisClient := redis.NewClient(&redis.Options{Addr: os.Getenv("LIBRARY_TEST_REDIS_ADDR")})
	if err := redisClient.FlushDB(context.Background()).Err(); err != nil {
		t.Fatal(err)
	}
	handler, err := library.New(library.Config{Database: pool, Redis: redisClient, ClientID: "console-gateway", Keys: map[string]string{"active": serviceSecret}, DownloadClientID: "portal-gateway", DownloadKeys: map[string]string{"active": downloadServiceSecret}, DownloadStore: &fakeDownloadStore{}, HTTPClient: http.DefaultClient})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(func() { _ = redisClient.Close() })
	return server, pool
}

func send(t *testing.T, baseURL, permission, method, path string, body []byte, key string) *http.Response {
	return sendAs(t, baseURL, "99999999-9999-4999-8999-999999999999", permission, method, path, body, key)
}

func sendAs(t *testing.T, baseURL, actorID, permission, method, path string, body []byte, key string) *http.Response {
	t.Helper()
	request, err := http.NewRequest(method, baseURL+path, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Service-Id", "console-gateway")
	request.Header.Set("X-Key-Id", "active")
	request.Header.Set("X-Actor-User-Id", actorID)
	request.Header.Set("X-Permission-Code", permission)
	request.Header.Set("X-Scope-Kind", "product")
	request.Header.Set("X-Product-Code", "library")
	request.Header.Set("X-Request-Id", "req_library_test_"+strings.ReplaceAll(uuid.NewString(), "-", ""))
	if key != "" {
		request.Header.Set("Idempotency-Key", key)
	}
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	nonce := base64.RawURLEncoding.EncodeToString([]byte(uuid.NewString()[:24]))
	digest := sha256.Sum256(body)
	canonical := strings.Join([]string{method, path, timestamp, nonce, hex.EncodeToString(digest[:])}, "\n")
	mac := hmac.New(sha256.New, []byte(serviceSecret))
	_, _ = mac.Write([]byte(canonical))
	request.Header.Set("X-Timestamp", timestamp)
	request.Header.Set("X-Nonce", nonce)
	request.Header.Set("X-Signature", base64.RawURLEncoding.EncodeToString(mac.Sum(nil)))
	request.SetBasicAuth("console-gateway", serviceSecret)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func readBody(t *testing.T, response *http.Response) []byte {
	t.Helper()
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode >= 500 {
		t.Fatalf("server error %d: %s", response.StatusCode, body)
	}
	return body
}
