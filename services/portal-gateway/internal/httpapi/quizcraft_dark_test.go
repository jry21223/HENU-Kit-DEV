package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"henukit.dev/portal-gateway/internal/config"
	"henukit.dev/portal-gateway/internal/practice"
	"henukit.dev/portal-gateway/internal/session"
)

const (
	testQuizCraftCatalogClientID = "portal-gateway"
	testQuizCraftCatalogSecret   = "portal-catalog-secret-with-enough-entropy"
	testQuizCraftCatalogKeyID    = "portal-catalog-key-1"
)

func TestRouterKeepsQuizCraftCatalogDarkBeforeCutover(t *testing.T) {
	var portalAPICalls atomic.Int32
	portalAPI := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		portalAPICalls.Add(1)
		if request.URL.Path != "/api/v1/practice/banks" {
			t.Fatalf("Portal API path = %q", request.URL.Path)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"banks":[],"request_id":"req_legacy_portal"}`))
	}))
	defer portalAPI.Close()

	var quizCraftCalls atomic.Int32
	quizCraft := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		quizCraftCalls.Add(1)
		writer.WriteHeader(http.StatusInternalServerError)
	}))
	defer quizCraft.Close()

	handler, err := New(config.Config{
		SessionKey:   []byte("0123456789abcdef0123456789abcdef"),
		PortalAPIURL: portalAPI.URL,
		PracticeURL:  quizCraft.URL,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	banks := httptest.NewRecorder()
	handler.Router().ServeHTTP(banks, httptest.NewRequest(http.MethodGet, "http://portal.test/api/v1/practice/banks", nil))
	if banks.Code != http.StatusOK || banks.Body.String() != `{"banks":[],"request_id":"req_legacy_portal"}` {
		t.Fatalf("dark bank response = %d %s", banks.Code, banks.Body.String())
	}

	stats := httptest.NewRecorder()
	handler.Router().ServeHTTP(stats, httptest.NewRequest(http.MethodGet, "http://portal.test/api/v1/practice/stats", nil))
	if stats.Code != http.StatusServiceUnavailable || stats.Header().Get("Cache-Control") != "no-store" || stats.Body.String() == `{"banks":[],"request_id":"req_legacy_portal"}` {
		t.Fatalf("dark stats response = %d headers=%v body=%s", stats.Code, stats.Header(), stats.Body.String())
	}
	if portalAPICalls.Load() != 1 || quizCraftCalls.Load() != 0 {
		t.Fatalf("before #166 public route calls = portal-api:%d QuizCraft:%d", portalAPICalls.Load(), quizCraftCalls.Load())
	}
}

func TestRouterFailsClosedForV2CatalogBeforeCutover(t *testing.T) {
	var portalAPICalls atomic.Int32
	portalAPI := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		portalAPICalls.Add(1)
		writer.WriteHeader(http.StatusInternalServerError)
	}))
	defer portalAPI.Close()

	var quizCraftCalls atomic.Int32
	quizCraft := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		quizCraftCalls.Add(1)
		writer.WriteHeader(http.StatusInternalServerError)
	}))
	defer quizCraft.Close()

	handler, err := New(config.Config{
		SessionKey:   []byte("0123456789abcdef0123456789abcdef"),
		PortalAPIURL: portalAPI.URL,
		PracticeURL:  quizCraft.URL,
		PracticeAuth: config.ServiceAuth{
			ClientID:     testQuizCraftCatalogClientID,
			ClientSecret: testQuizCraftCatalogSecret,
			KeyID:        testQuizCraftCatalogKeyID,
		},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodGet, "http://portal.test/api/v1/practice/catalog", nil)
	request.Header.Set("X-Request-Id", "req_catalog_dark")
	response := httptest.NewRecorder()
	handler.Router().ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("dark V2 catalog status = %d, want 404: %s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), `"banks"`) || strings.Contains(strings.ToLower(response.Body.String()), "mock") {
		t.Fatalf("dark V2 catalog substituted a success response: %s", response.Body.String())
	}
	if portalAPICalls.Load() != 0 || quizCraftCalls.Load() != 0 {
		t.Fatalf("dark V2 catalog calls = portal-api:%d QuizCraft:%d, want 0:0", portalAPICalls.Load(), quizCraftCalls.Load())
	}
}

func TestRouterServesRealQuizCraftCatalogOnlyWhenExplicitlyEnabled(t *testing.T) {
	var portalAPICalls atomic.Int32
	portalAPI := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		portalAPICalls.Add(1)
		writer.WriteHeader(http.StatusInternalServerError)
	}))
	defer portalAPI.Close()

	var quizCraftCalls atomic.Int32
	quizCraft := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		quizCraftCalls.Add(1)
		actors := map[string]string{
			"req_catalog_guest":   practice.AnonymousCatalogActor,
			"req_catalog_invalid": practice.AnonymousCatalogActor,
			"req_catalog_signed":  practice.AnonymousCatalogActor,
		}
		requestID := request.Header.Get("X-Request-Id")
		actor, ok := actors[requestID]
		if !ok {
			t.Fatalf("unexpected enabled catalog request id %q", requestID)
		}
		assertQuizCraftCatalogRead(t, request, requestID, actor)
		_ = json.NewEncoder(writer).Encode(practice.BankListEnvelope{
			RequestID: "req_core_catalog",
			Data: []practice.BankVersion{{
				BankID:        "11111111-1111-4111-8111-111111111111",
				BankVersionID: "22222222-2222-4222-8222-222222222222",
				BankKey:       "computer-fundamentals",
				Name:          "计算机基础",
				ContentSHA256: strings.Repeat("a", 64),
				QuestionCount: 42,
				Chapters:      []practice.Chapter{{ID: "chapter-1", Name: "绪论"}},
			}},
		})
	}))
	defer quizCraft.Close()

	handler, err := New(config.Config{
		SessionKey:              []byte("0123456789abcdef0123456789abcdef"),
		PortalAPIURL:            portalAPI.URL,
		PracticeURL:             quizCraft.URL,
		QuizCraftCatalogEnabled: true,
		PracticeAuth: config.ServiceAuth{
			ClientID:     testQuizCraftCatalogClientID,
			ClientSecret: testQuizCraftCatalogSecret,
			KeyID:        testQuizCraftCatalogKeyID,
		},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	for _, browser := range []struct {
		name           string
		requestID      string
		userID         string
		invalidSession bool
	}{
		{name: "guest", requestID: "req_catalog_guest"},
		{name: "invalid session stays guest", requestID: "req_catalog_invalid", invalidSession: true},
		{name: "signed in remains public guest", requestID: "req_catalog_signed", userID: "33333333-3333-4333-8333-333333333333"},
	} {
		t.Run(browser.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "http://portal.test/api/v1/practice/catalog", nil)
			request.Header.Set("X-Request-Id", browser.requestID)
			if browser.invalidSession {
				request.AddCookie(&http.Cookie{Name: "henukit_portal_session_local", Value: "not-a-valid-session"})
			} else if browser.userID != "" {
				encoded, err := handler.sessionCodec.Encode(session.Value{
					UserID:        browser.userID,
					ExchangeToken: strings.Repeat("x", 32),
					ExpiresAt:     time.Now().Add(time.Hour),
				})
				if err != nil {
					t.Fatal(err)
				}
				request.AddCookie(&http.Cookie{Name: "henukit_portal_session_local", Value: encoded})
			}
			response := httptest.NewRecorder()
			handler.Router().ServeHTTP(response, request)

			if response.Code != http.StatusOK {
				t.Fatalf("enabled V2 catalog status = %d, want 200: %s", response.Code, response.Body.String())
			}
			var payload struct {
				Banks []struct {
					BankID        string `json:"bank_id"`
					BankVersionID string `json:"bank_version_id"`
					Name          string `json:"name"`
					QuestionCount int    `json:"question_count"`
					Available     bool   `json:"available"`
					Chapters      []struct {
						ID   string `json:"id"`
						Name string `json:"name"`
					} `json:"chapters"`
				} `json:"banks"`
				RequestID string `json:"request_id"`
			}
			if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload.RequestID != browser.requestID || len(payload.Banks) != 1 {
				t.Fatalf("enabled V2 catalog payload = %+v", payload)
			}
			bank := payload.Banks[0]
			if bank.BankID != "11111111-1111-4111-8111-111111111111" || bank.BankVersionID != "22222222-2222-4222-8222-222222222222" || bank.Name != "计算机基础" || bank.QuestionCount != 42 || !bank.Available || len(bank.Chapters) != 1 || bank.Chapters[0].ID != "chapter-1" || bank.Chapters[0].Name != "绪论" {
				t.Fatalf("enabled V2 catalog bank = %+v", bank)
			}
		})
	}
	if portalAPICalls.Load() != 0 || quizCraftCalls.Load() != 3 {
		t.Fatalf("enabled V2 catalog calls = portal-api:%d QuizCraft:%d, want 0:3", portalAPICalls.Load(), quizCraftCalls.Load())
	}
}

func TestRouterReturnsHonestQuizCraftCatalogDependencyFailure(t *testing.T) {
	const upstreamDetail = "quizcraft-upstream-detail-must-not-reach-browser"
	quizCraft := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		assertQuizCraftCatalogRead(t, request, "req_catalog_unavailable", practice.AnonymousCatalogActor)
		writer.WriteHeader(http.StatusServiceUnavailable)
		_, _ = writer.Write([]byte(`{"error":"` + upstreamDetail + `"}`))
	}))
	defer quizCraft.Close()

	handler, err := New(config.Config{
		SessionKey:              []byte("0123456789abcdef0123456789abcdef"),
		PortalAPIURL:            "http://127.0.0.1:1",
		PracticeURL:             quizCraft.URL,
		QuizCraftCatalogEnabled: true,
		PracticeAuth: config.ServiceAuth{
			ClientID:     testQuizCraftCatalogClientID,
			ClientSecret: testQuizCraftCatalogSecret,
			KeyID:        testQuizCraftCatalogKeyID,
		},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodGet, "http://portal.test/api/v1/practice/catalog", nil)
	request.Header.Set("X-Request-Id", "req_catalog_unavailable")
	response := httptest.NewRecorder()
	handler.Router().ServeHTTP(response, request)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("unavailable V2 catalog status = %d, want 503: %s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	if !strings.Contains(body, "quizcraft_catalog_unavailable") || strings.Contains(body, upstreamDetail) || strings.Contains(body, `"banks"`) || strings.Contains(strings.ToLower(body), "mock") {
		t.Fatalf("unavailable V2 catalog substituted or leaked response: %s", body)
	}
}

func TestRouterPreservesTrueEmptyQuizCraftCatalogWhenExplicitlyEnabled(t *testing.T) {
	quizCraft := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		assertQuizCraftCatalogRead(t, request, "req_catalog_empty", practice.AnonymousCatalogActor)
		_, _ = writer.Write([]byte(`{"request_id":"req_core_empty","data":[]}`))
	}))
	defer quizCraft.Close()

	handler, err := New(config.Config{
		SessionKey:              []byte("0123456789abcdef0123456789abcdef"),
		PortalAPIURL:            "http://127.0.0.1:1",
		PracticeURL:             quizCraft.URL,
		QuizCraftCatalogEnabled: true,
		PracticeAuth: config.ServiceAuth{
			ClientID:     testQuizCraftCatalogClientID,
			ClientSecret: testQuizCraftCatalogSecret,
			KeyID:        testQuizCraftCatalogKeyID,
		},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodGet, "http://portal.test/api/v1/practice/catalog", nil)
	request.Header.Set("X-Request-Id", "req_catalog_empty")
	response := httptest.NewRecorder()
	handler.Router().ServeHTTP(response, request)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"banks":[]`) {
		t.Fatalf("true empty V2 catalog response = %d %s", response.Code, response.Body.String())
	}
}

func assertQuizCraftCatalogRead(t *testing.T, request *http.Request, requestID, actor string) {
	t.Helper()
	if request.Method != http.MethodGet || request.URL.Path != practice.ListPracticeBanksPath {
		t.Fatalf("QuizCraft request = %s %s, want GET %s", request.Method, request.URL.Path, practice.ListPracticeBanksPath)
	}
	if request.Header.Get("X-Actor-User-Id") != actor || request.Header.Get("X-Request-Id") != requestID || request.Header.Get("X-Permission-Code") != practice.CatalogReadPermission || request.Header.Get("X-Scope-Kind") != "product" || request.Header.Get("X-Product-Code") != "quizcraft" {
		t.Fatalf("QuizCraft catalog headers = actor=%q request_id=%q permission=%q scope=%q product=%q", request.Header.Get("X-Actor-User-Id"), request.Header.Get("X-Request-Id"), request.Header.Get("X-Permission-Code"), request.Header.Get("X-Scope-Kind"), request.Header.Get("X-Product-Code"))
	}
	user, password, ok := request.BasicAuth()
	if !ok || user != testQuizCraftCatalogClientID || password != testQuizCraftCatalogSecret || request.Header.Get("X-Service-Id") != testQuizCraftCatalogClientID || request.Header.Get("X-Key-Id") != testQuizCraftCatalogKeyID {
		t.Fatal("QuizCraft catalog request omitted service authentication")
	}
}

func TestRouterKeepsQuizCraftV2RankingsDarkBeforeCutover(t *testing.T) {
	var portalAPICalls atomic.Int32
	portalAPI := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		portalAPICalls.Add(1)
		writer.WriteHeader(http.StatusInternalServerError)
	}))
	defer portalAPI.Close()

	var quizCraftCalls atomic.Int32
	quizCraft := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		quizCraftCalls.Add(1)
		writer.WriteHeader(http.StatusInternalServerError)
	}))
	defer quizCraft.Close()

	handler, err := New(config.Config{
		SessionKey:   []byte("0123456789abcdef0123456789abcdef"),
		PortalAPIURL: portalAPI.URL,
		PracticeURL:  quizCraft.URL,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{"/api/v1/rankings/overall", "/api/v1/banks/data-structures/rankings"} {
		recorder := httptest.NewRecorder()
		handler.Router().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "http://portal.test"+path, nil))

		if recorder.Code != http.StatusNotFound {
			t.Fatalf("public V2 ranking response for %s = %d, want 404", path, recorder.Code)
		}
	}
	if portalAPICalls.Load() != 0 || quizCraftCalls.Load() != 0 {
		t.Fatalf("before #166 public V2 ranking route calls = portal-api:%d QuizCraft:%d", portalAPICalls.Load(), quizCraftCalls.Load())
	}
}

func TestRouterPublishesRealQuizCraftV2RankingsAtCutover(t *testing.T) {
	const bankID = "11111111-1111-4111-8111-111111111111"
	var quizCraftCalls atomic.Int32
	quizCraft := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		quizCraftCalls.Add(1)
		if request.Header.Get("X-Actor-User-Id") != practice.AnonymousCatalogActor || request.Header.Get("X-Permission-Code") != practice.PortalReadPermission {
			t.Fatalf("ranking read headers = actor %q permission %q", request.Header.Get("X-Actor-User-Id"), request.Header.Get("X-Permission-Code"))
		}
		switch request.URL.Path {
		case practice.OverallRankingPath:
			if request.URL.RawQuery != "period=weekly" {
				t.Fatalf("overall ranking query = %q", request.URL.RawQuery)
			}
			_ = json.NewEncoder(writer).Encode(practice.RankingEnvelope{RequestID: "req_core_overall", Data: practice.RankingPage{Scope: "overall", Period: practice.RankingPeriodWeekly, Metric: "correct_answer_count", Entries: []practice.RankingEntry{{Rank: 1, Nickname: "认真刷题", SystemAvatar: "scholar-blue", CorrectAnswerCount: 12}}}})
		case strings.Replace(practice.BankRankingPath, "{bank_id}", bankID, 1):
			if request.URL.RawQuery != "period=lifetime" {
				t.Fatalf("bank ranking query = %q", request.URL.RawQuery)
			}
			_ = json.NewEncoder(writer).Encode(practice.RankingEnvelope{RequestID: "req_core_bank", Data: practice.RankingPage{Scope: "bank", BankID: bankID, Period: practice.RankingPeriodLifetime, Metric: "correct_answer_count", Entries: []practice.RankingEntry{}}})
		default:
			t.Fatalf("unexpected ranking path %q", request.URL.Path)
		}
	}))
	defer quizCraft.Close()

	handler, err := New(config.Config{
		SessionKey:              []byte("0123456789abcdef0123456789abcdef"),
		PortalAPIURL:            "http://127.0.0.1:1",
		QuizCraftV2ReadsEnabled: true,
		QuizCraftCoreURL:        quizCraft.URL,
		QuizCraftCoreAuth:       config.ServiceAuth{ClientID: testQuizCraftCatalogClientID, ClientSecret: testQuizCraftCatalogSecret, KeyID: testQuizCraftCatalogKeyID},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		path        string
		requestID   string
		wantScope   string
		wantPeriod  string
		wantEntries int
	}{
		{path: "/api/v1/rankings/overall?period=weekly", requestID: "req_public_overall", wantScope: "overall", wantPeriod: "weekly", wantEntries: 1},
		{path: "/api/v1/banks/" + bankID + "/rankings?period=lifetime", requestID: "req_public_bank", wantScope: "bank", wantPeriod: "lifetime", wantEntries: 0},
	} {
		request := httptest.NewRequest(http.MethodGet, "http://portal.test"+test.path, nil)
		request.Header.Set("X-Request-Id", test.requestID)
		response := httptest.NewRecorder()
		handler.Router().ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("ranking %s = %d %s", test.path, response.Code, response.Body.String())
		}
		var payload practice.RankingEnvelope
		if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload.Data.Scope != test.wantScope || string(payload.Data.Period) != test.wantPeriod || len(payload.Data.Entries) != test.wantEntries {
			t.Fatalf("ranking %s payload = %+v", test.path, payload)
		}
	}
	if quizCraftCalls.Load() != 2 {
		t.Fatalf("ranking Core calls = %d, want 2", quizCraftCalls.Load())
	}
	invalid := httptest.NewRecorder()
	handler.Router().ServeHTTP(invalid, httptest.NewRequest(http.MethodGet, "http://portal.test/api/v1/banks/not-a-uuid/rankings", nil))
	if invalid.Code != http.StatusBadRequest || !strings.Contains(invalid.Body.String(), "invalid_bank_id") || quizCraftCalls.Load() != 2 {
		t.Fatalf("invalid bank ranking = %d %s Core calls=%d", invalid.Code, invalid.Body.String(), quizCraftCalls.Load())
	}
}
