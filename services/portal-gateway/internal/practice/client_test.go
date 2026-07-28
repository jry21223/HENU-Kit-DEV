package practice

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
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	testCatalogClientID = "portal-gateway"
	testCatalogSecret   = "portal-catalog-secret-with-enough-entropy"
	testCatalogKeyID    = "portal-catalog-key-1"
	testStatsUserID     = "5f03dac8-7f7f-4513-9dcd-e4cc5f592c85"
)

func TestBanksUsesGeneratedQuizCraftCatalogContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		assertCatalogRequest(t, request, "user-123", "req_catalog_success")
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(BankListEnvelope{
			RequestID: "req_core_catalog",
			Data: []BankVersion{{
				BankID:        "10ca9b18-c303-4b7a-ab14-1241e41b665a",
				BankVersionID: "154bc407-87bc-4942-838d-d0635256a53f",
				BankKey:       "computer-fundamentals",
				Name:          "计算机基础",
				ContentSHA256: strings.Repeat("a", 64),
				QuestionCount: 42,
				Chapters:      []Chapter{{ID: "chapter-1", Name: "绪论"}},
			}},
		})
	}))
	defer server.Close()

	client := testCatalogClient(t, server)
	result, err := client.Banks(context.Background(), "user-123", "req_catalog_success")
	if err != nil {
		t.Fatalf("Banks() error = %v", err)
	}
	if result.RequestID != "req_core_catalog" || len(result.Data) != 1 {
		t.Fatalf("Banks() = %+v", result)
	}
	bank := result.Data[0]
	if bank.BankID != "10ca9b18-c303-4b7a-ab14-1241e41b665a" || bank.BankVersionID != "154bc407-87bc-4942-838d-d0635256a53f" || bank.Name != "计算机基础" || bank.QuestionCount != 42 || len(bank.Chapters) != 1 {
		t.Fatalf("decoded bank = %+v", bank)
	}
}

func TestBanksAcceptsTrueEmptyCatalogButRejectsLegacyMockShape(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr bool
	}{
		{
			name: "true empty catalog",
			body: `{"request_id":"req_empty","data":[]}`,
		},
		{
			name:    "legacy mock response is not a Core catalog",
			body:    `{"request_id":"req_mock","banks":[{"id":"mock-bank","name":"示例题库","subject":"mock","question_count":10}]}`,
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				assertCatalogRequest(t, request, AnonymousCatalogActor, "req_catalog_"+strings.ReplaceAll(test.name, " ", "_"))
				writer.Header().Set("Content-Type", "application/json")
				_, _ = writer.Write([]byte(test.body))
			}))
			defer server.Close()

			result, err := testCatalogClient(t, server).Banks(context.Background(), "", "req_catalog_"+strings.ReplaceAll(test.name, " ", "_"))
			if test.wantErr {
				if err == nil {
					t.Fatalf("Banks() = %+v, want malformed legacy response error", result)
				}
				if len(result.Data) != 0 || result.RequestID != "" {
					t.Fatalf("Banks() returned a successful catalog on malformed response: %+v", result)
				}
				return
			}
			if err != nil {
				t.Fatalf("Banks() error = %v", err)
			}
			if result.RequestID != "req_empty" || result.Data == nil || len(result.Data) != 0 {
				t.Fatalf("Banks() true empty result = %+v", result)
			}
		})
	}
}

func TestBanksWaitsForCoreInsteadOfFallingBackWhileLoading(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseCore := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(releaseCore)

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		assertCatalogRequest(t, request, AnonymousCatalogActor, "req_catalog_loading")
		close(started)
		<-release
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"request_id":"req_loading_complete","data":[]}`))
	}))
	defer server.Close()

	type result struct {
		value BankListEnvelope
		err   error
	}
	resultCh := make(chan result, 1)
	client := testCatalogClient(t, server)
	go func() {
		value, err := client.Banks(context.Background(), "", "req_catalog_loading")
		resultCh <- result{value: value, err: err}
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("client did not call Core")
	}
	select {
	case returned := <-resultCh:
		t.Fatalf("Banks() returned before Core completed: %+v", returned)
	case <-time.After(120 * time.Millisecond):
	}

	releaseCore()
	select {
	case returned := <-resultCh:
		if returned.err != nil || returned.value.RequestID != "req_loading_complete" || returned.value.Data == nil {
			t.Fatalf("Banks() after Core completed = %+v", returned)
		}
	case <-time.After(time.Second):
		t.Fatal("client did not return after Core completed")
	}
}

func TestBanksReturnsDependencyFailureWithoutLeakingCredentials(t *testing.T) {
	const upstreamDetail = "upstream-failure-must-not-expose-portal-catalog-secret"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		assertCatalogRequest(t, request, "user-123", "req_catalog_failure")
		writer.WriteHeader(http.StatusServiceUnavailable)
		_, _ = writer.Write([]byte(`{"error":{"code":"database_unavailable","message":"` + upstreamDetail + `"}}`))
	}))
	defer server.Close()

	result, err := testCatalogClient(t, server).Banks(context.Background(), "user-123", "req_catalog_failure")
	if !errors.Is(err, ErrCatalogUnavailable) {
		t.Fatalf("Banks() error = %v, want ErrCatalogUnavailable", err)
	}
	if strings.Contains(err.Error(), upstreamDetail) || strings.Contains(err.Error(), testCatalogSecret) {
		t.Fatalf("Banks() leaked an upstream detail or credential: %v", err)
	}
	if result.RequestID != "" || result.Data != nil {
		t.Fatalf("Banks() returned catalog data on dependency failure: %+v", result)
	}
}

func TestPersonalStatsUsesGeneratedQuizCraftContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		assertStatsRequest(t, request, testStatsUserID, "req_stats_success")
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(PersonalPracticeStatsEnvelope{
			RequestID: "req_core_stats",
			Data: PersonalPracticeStats{
				TotalAnswers: 12, CorrectAnswers: 9, Accuracy: 75, StreakDays: 3,
				Mastery: []MasterySubject{{
					BankID: "10ca9b18-c303-4b7a-ab14-1241e41b665a", Label: "计算机基础", Value: 50, TotalQuestions: 4, CorrectQuestions: 2,
				}},
			},
		})
	}))
	defer server.Close()

	result, err := testCatalogClient(t, server).PersonalStats(context.Background(), testStatsUserID, "req_stats_success")
	if err != nil {
		t.Fatalf("PersonalStats() error = %v", err)
	}
	if result.RequestID != "req_core_stats" || result.Data.TotalAnswers != 12 || result.Data.CorrectAnswers != 9 || result.Data.Accuracy != 75 || result.Data.StreakDays != 3 || len(result.Data.Mastery) != 1 {
		t.Fatalf("PersonalStats() = %+v", result)
	}
}

func TestPersonalStatsAcceptsTrueZeroButRejectsMockShape(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr bool
	}{
		{
			name: "true empty personal stats",
			body: `{"request_id":"req_empty_stats","data":{"total_answers":0,"correct_answers":0,"accuracy":0,"streak_days":0,"mastery":[]}}`,
		},
		{
			name:    "legacy mock response is not personal stats",
			body:    `{"request_id":"req_mock_stats","stats":{"total":486,"mastery":[{"label":"示例"}]}}`,
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				assertStatsRequest(t, request, testStatsUserID, "req_stats_"+strings.ReplaceAll(test.name, " ", "_"))
				writer.Header().Set("Content-Type", "application/json")
				_, _ = writer.Write([]byte(test.body))
			}))
			defer server.Close()

			result, err := testCatalogClient(t, server).PersonalStats(context.Background(), testStatsUserID, "req_stats_"+strings.ReplaceAll(test.name, " ", "_"))
			if test.wantErr {
				if !errors.Is(err, ErrInvalidStats) || result.RequestID != "" || result.Data.Mastery != nil {
					t.Fatalf("PersonalStats() = %+v, %v; want malformed mock error", result, err)
				}
				return
			}
			if err != nil || result.RequestID != "req_empty_stats" || result.Data.TotalAnswers != 0 || result.Data.Mastery == nil || len(result.Data.Mastery) != 0 {
				t.Fatalf("PersonalStats() true empty = %+v, %v", result, err)
			}
		})
	}
}

func TestPersonalStatsRejectsAnonymousOrMalformedActorWithoutCallingCore(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))
	defer server.Close()

	for _, actor := range []string{"", AnonymousCatalogActor, "user-123"} {
		if _, err := testCatalogClient(t, server).PersonalStats(context.Background(), actor, "req_bad_actor"); !errors.Is(err, ErrStatsUnauthorized) {
			t.Fatalf("PersonalStats(%q) error = %v, want ErrStatsUnauthorized", actor, err)
		}
	}
	if calls != 0 {
		t.Fatalf("PersonalStats called Core for invalid actors %d times", calls)
	}
}

func TestGeneratedQuizCraftCatalogContractMatchesOpenAPI(t *testing.T) {
	source, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "packages", "api-contracts", "openapi", "quizcraft.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(source)
	if got, want := QuizCraftCatalogContractSHA256, hex.EncodeToString(digest[:]); got != want {
		t.Fatalf("generated QuizCraft catalog contract SHA = %q, want %q; run go generate ./internal/practice", got, want)
	}
}

func testCatalogClient(t *testing.T, server *httptest.Server) *Client {
	t.Helper()
	client, err := NewClient(server.URL, testCatalogClientID, testCatalogSecret, testCatalogKeyID)
	if err != nil {
		t.Fatal(err)
	}
	client.httpClient = server.Client()
	return client
}

func assertCatalogRequest(t *testing.T, request *http.Request, wantActor, wantRequestID string) {
	t.Helper()
	assertPortalReadRequest(t, request, ListPracticeBanksPath, wantActor, wantRequestID)
}

func assertStatsRequest(t *testing.T, request *http.Request, wantActor, wantRequestID string) {
	t.Helper()
	assertPortalReadRequest(t, request, GetPersonalPracticeStatsPath, wantActor, wantRequestID)
}

func assertPortalReadRequest(t *testing.T, request *http.Request, wantPath, wantActor, wantRequestID string) {
	t.Helper()
	if request.Method != http.MethodGet || request.URL.Path != wantPath {
		t.Fatalf("Core request = %s %s, want GET %s", request.Method, request.URL.Path, wantPath)
	}
	if got := request.Header.Get("X-Actor-User-Id"); got != wantActor {
		t.Fatalf("X-Actor-User-Id = %q, want %q", got, wantActor)
	}
	if got := request.Header.Get("X-Request-Id"); got != wantRequestID {
		t.Fatalf("X-Request-Id = %q, want %q", got, wantRequestID)
	}
	if request.Header.Get("X-Permission-Code") != CatalogReadPermission || request.Header.Get("X-Scope-Kind") != "product" || request.Header.Get("X-Product-Code") != "quizcraft" {
		t.Fatalf("catalog permission headers = permission=%q scope=%q product=%q", request.Header.Get("X-Permission-Code"), request.Header.Get("X-Scope-Kind"), request.Header.Get("X-Product-Code"))
	}
	user, password, ok := request.BasicAuth()
	if !ok || user != testCatalogClientID || password != testCatalogSecret || request.Header.Get("X-Service-Id") != testCatalogClientID || request.Header.Get("X-Key-Id") != testCatalogKeyID {
		t.Fatal("catalog request omitted valid service authentication")
	}
	timestamp := request.Header.Get("X-Timestamp")
	nonce := request.Header.Get("X-Nonce")
	digest := sha256.Sum256(nil)
	canonical := strings.Join([]string{request.Method, request.URL.RequestURI(), timestamp, nonce, hex.EncodeToString(digest[:])}, "\n")
	mac := hmac.New(sha256.New, []byte(testCatalogSecret))
	_, _ = mac.Write([]byte(canonical))
	wantSignature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if timestamp == "" || nonce == "" || request.Header.Get("X-Signature") != wantSignature {
		t.Fatal("catalog request signature is invalid")
	}
}
