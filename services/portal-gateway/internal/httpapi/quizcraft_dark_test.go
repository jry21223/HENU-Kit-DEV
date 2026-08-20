package httpapi

import (
	"encoding/json"
	"fmt"
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

func TestRouterKeepsQuizCraftLegacyPracticeReadsFailClosed(t *testing.T) {
	// ADR-0036: the legacy practice read endpoints no longer proxy to
	// portal-api at all. With no QuizCraft client configured they fail closed
	// with an honest 404, and the actor-bound stats read stays an honest 503.
	var portalAPICalls atomic.Int32
	portalAPI := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		portalAPICalls.Add(1)
		t.Fatalf("Portal API must not be contacted for legacy practice reads: %s", request.URL.Path)
	}))
	defer portalAPI.Close()

	handler, err := New(config.Config{
		SessionKey:   []byte("0123456789abcdef0123456789abcdef"),
		PortalAPIURL: portalAPI.URL,
		PracticeURL:  "http://127.0.0.1:1",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{
		"/api/v1/practice/banks",
		"/api/v1/practice/schools",
		"/api/v1/practice/lists/ds-final",
		"/api/v1/practice/leaderboard",
	} {
		recorder := httptest.NewRecorder()
		handler.Router().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "http://portal.test"+path, nil))
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("legacy practice read %s = %d, want 404: %s", path, recorder.Code, recorder.Body.String())
		}
		if strings.Contains(strings.ToLower(recorder.Body.String()), "mock") || strings.Contains(recorder.Body.String(), `"banks":[]`) {
			t.Fatalf("legacy practice read %s substituted a success response: %s", path, recorder.Body.String())
		}
	}

	stats := httptest.NewRecorder()
	handler.Router().ServeHTTP(stats, httptest.NewRequest(http.MethodGet, "http://portal.test/api/v1/practice/stats", nil))
	if stats.Code != http.StatusServiceUnavailable || stats.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("dark stats response = %d headers=%v body=%s", stats.Code, stats.Header(), stats.Body.String())
	}
	if portalAPICalls.Load() != 0 {
		t.Fatalf("legacy practice reads reached portal-api %d times", portalAPICalls.Load())
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

	for _, path := range []string{
		"/api/v1/rankings/overall",
		"/api/v1/banks/11111111-1111-4111-8111-111111111111/rankings",
	} {
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
	overallUser := "33333333-3333-4333-8333-333333333333"
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
			_ = json.NewEncoder(writer).Encode(practice.RankingEnvelope{RequestID: "req_core_overall", Data: practice.RankingPage{Scope: "overall", Period: practice.RankingPeriodWeekly, Metric: "correct_answer_count", Entries: []practice.RankingEntry{{Rank: 1, UserID: &overallUser, CorrectAnswerCount: 12}}}})
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
		// No Platform Core boundary configured: display names degrade to 游客x.
		PlatformCoreURL: "http://127.0.0.1:1",
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

// TestRouterRankingContractNeverLeaksUserID pins the ADR-0036 privacy
// contract and the ADR-0038 nickname synthesis at the Gateway boundary: the
// internal user_id and guest_key are accepted, resolved to a display name
// through the Platform Core batch boundary (guest_key drives the stable 游客x
// label), and stripped before the browser response; a missing user_id or a
// guest entry without guest_key is a contract violation (502), never a guest;
// and a Platform Core outage degrades nicknames to 游客x without failing the
// read.
func TestRouterRankingContractNeverLeaksUserID(t *testing.T) {
	namedUser := "99999999-9999-4999-8999-999999999999"
	unnamedUser := "88888888-8888-4888-8888-888888888888"

	var displayNamesCalls atomic.Int32
	platformCore := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		displayNamesCalls.Add(1)
		if request.URL.Path != "/api/v1/users/display-names" || request.Method != http.MethodPost {
			t.Fatalf("platform-core display-names request = %s %s", request.Method, request.URL.Path)
		}
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"data":       map[string]*string{namedUser: stringPointer("认真刷题"), unnamedUser: nil},
			"request_id": "req_core_display_names",
		})
	}))
	defer platformCore.Close()

	var quizCraftCalls atomic.Int32
	quizCraft := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		quizCraftCalls.Add(1)
		switch request.Header.Get("X-Request-Id") {
		case "req_ranking_missing_user_id":
			// A contract-violating Core response that omits user_id: the
			// Gateway must reject it instead of treating everyone as a guest.
			_, _ = writer.Write([]byte(`{"request_id":"req_core_missing","data":{"scope":"overall","period":"weekly","metric":"correct_answer_count","entries":[{"rank":1,"correct_answer_count":12}]}}`))
		case "req_ranking_guest":
			// A guest entry (user_id null) carries its stable guest_key; the
			// Gateway derives 游客x from it and leaks neither field.
			guestKey := "guest:2f4d5e6c-7b8a-4901-8c2d-3e4f5a6b7c8d"
			_ = json.NewEncoder(writer).Encode(practice.RankingEnvelope{RequestID: "req_core_guest", Data: practice.RankingPage{Scope: "overall", Period: practice.RankingPeriodWeekly, Metric: "correct_answer_count", Entries: []practice.RankingEntry{{Rank: 1, UserID: nil, GuestKey: &guestKey, CorrectAnswerCount: 12}}}})
		case "req_ranking_two_guests":
			// Two distinct guests get two distinct 游客x labels: the number is
			// derived from each guest's own guest_key (ADR-0038).
			guestA := "guest:aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
			guestB := "guest:bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
			_ = json.NewEncoder(writer).Encode(practice.RankingEnvelope{RequestID: "req_core_two_guests", Data: practice.RankingPage{Scope: "overall", Period: practice.RankingPeriodWeekly, Metric: "correct_answer_count", Entries: []practice.RankingEntry{{Rank: 1, UserID: nil, GuestKey: &guestA, CorrectAnswerCount: 9}, {Rank: 2, UserID: nil, GuestKey: &guestB, CorrectAnswerCount: 4}}}})
		default:
			_ = json.NewEncoder(writer).Encode(practice.RankingEnvelope{RequestID: "req_core_ok", Data: practice.RankingPage{Scope: "overall", Period: practice.RankingPeriodWeekly, Metric: "correct_answer_count", Entries: []practice.RankingEntry{{Rank: 1, UserID: &namedUser, CorrectAnswerCount: 12}, {Rank: 2, UserID: &unnamedUser, CorrectAnswerCount: 5}}}})
		}
	}))
	defer quizCraft.Close()

	newHandler := func(platformCoreURL string) *Handler {
		t.Helper()
		handler, err := New(config.Config{
			SessionKey:              []byte("0123456789abcdef0123456789abcdef"),
			PortalAPIURL:            "http://127.0.0.1:1",
			QuizCraftV2ReadsEnabled: true,
			QuizCraftCoreURL:        quizCraft.URL,
			QuizCraftCoreAuth:       config.ServiceAuth{ClientID: testQuizCraftCatalogClientID, ClientSecret: testQuizCraftCatalogSecret, KeyID: testQuizCraftCatalogKeyID},
			PlatformCoreURL:         platformCoreURL,
			PlatformClientID:        testQuizCraftCatalogClientID,
			PlatformSecret:          testQuizCraftCatalogSecret,
			PlatformKeyID:           testQuizCraftCatalogKeyID,
		}, nil)
		if err != nil {
			t.Fatal(err)
		}
		return handler
	}
	handler := newHandler(platformCore.URL)

	get := func(handler *Handler, requestID string) *httptest.ResponseRecorder {
		t.Helper()
		request := httptest.NewRequest(http.MethodGet, "http://portal.test/api/v1/rankings/overall?period=weekly", nil)
		request.Header.Set("X-Request-Id", requestID)
		response := httptest.NewRecorder()
		handler.Router().ServeHTTP(response, request)
		return response
	}

	// A missing user_id is a contract violation: 502, no identifier leaks.
	missing := get(handler, "req_ranking_missing_user_id")
	if missing.Code != http.StatusBadGateway {
		t.Fatalf("missing user_id = %d %s, want 502", missing.Code, missing.Body.String())
	}
	if strings.Contains(missing.Body.String(), `"user_id"`) || strings.Contains(missing.Body.String(), "99999999-9999-4999-8999-999999999999") {
		t.Fatalf("missing user_id response leaked an account identifier: %s", missing.Body.String())
	}

	// A healthy internal response resolves display names and strips user_id.
	healthy := get(handler, "req_ranking_healthy")
	if healthy.Code != http.StatusOK {
		t.Fatalf("healthy ranking = %d %s", healthy.Code, healthy.Body.String())
	}
	healthyBody := healthy.Body.String()
	if strings.Contains(healthyBody, `"user_id"`) || strings.Contains(healthyBody, namedUser) || strings.Contains(healthyBody, unnamedUser) {
		t.Fatalf("healthy ranking leaked an internal identifier: %s", healthyBody)
	}
	if !strings.Contains(healthyBody, "认真刷题") {
		t.Fatalf("healthy ranking lost the resolved display name: %s", healthyBody)
	}
	var healthyPayload struct {
		Data struct {
			Entries []struct {
				Rank               int64  `json:"rank"`
				Nickname           string `json:"nickname"`
				SystemAvatar       string `json:"system_avatar"`
				CorrectAnswerCount int64  `json:"correct_answer_count"`
			} `json:"entries"`
		} `json:"data"`
	}
	if err := json.Unmarshal(healthy.Body.Bytes(), &healthyPayload); err != nil {
		t.Fatal(err)
	}
	if len(healthyPayload.Data.Entries) != 2 {
		t.Fatalf("healthy ranking entries = %+v", healthyPayload.Data.Entries)
	}
	if healthyPayload.Data.Entries[0].Nickname != "认真刷题" || healthyPayload.Data.Entries[1].Nickname != guestLabelFor(unnamedUser) {
		t.Fatalf("healthy ranking nicknames = %+v", healthyPayload.Data.Entries)
	}
	for _, entry := range healthyPayload.Data.Entries {
		if !validSystemAvatar(entry.SystemAvatar) {
			t.Fatalf("derived avatar %q is not a system pattern", entry.SystemAvatar)
		}
	}
	if displayNamesCalls.Load() < 1 {
		t.Fatalf("Platform Core display-names was never consulted")
	}

	// A guest entry (user_id null) renders 游客x and never leaks a key.
	guest := get(handler, "req_ranking_guest")
	if guest.Code != http.StatusOK {
		t.Fatalf("guest ranking = %d %s", guest.Code, guest.Body.String())
	}
	guestBody := guest.Body.String()
	if strings.Contains(guestBody, `"user_id"`) || strings.Contains(guestBody, `"guest_key"`) {
		t.Fatalf("guest ranking leaked an internal identity: %s", guestBody)
	}
	var guestPayload struct {
		Data struct {
			Entries []struct {
				Nickname     string `json:"nickname"`
				SystemAvatar string `json:"system_avatar"`
			} `json:"entries"`
		} `json:"data"`
	}
	if err := json.Unmarshal(guest.Body.Bytes(), &guestPayload); err != nil {
		t.Fatal(err)
	}
	if len(guestPayload.Data.Entries) != 1 || !strings.HasPrefix(guestPayload.Data.Entries[0].Nickname, "游客") || !validSystemAvatar(guestPayload.Data.Entries[0].SystemAvatar) {
		t.Fatalf("guest ranking = %+v", guestPayload.Data.Entries)
	}

	// Two distinct guests must render two distinct 游客x labels (each derived
	// from its own guest_key), still without leaking either key.
	twoGuests := get(handler, "req_ranking_two_guests")
	if twoGuests.Code != http.StatusOK {
		t.Fatalf("two-guests ranking = %d %s", twoGuests.Code, twoGuests.Body.String())
	}
	twoGuestsBody := twoGuests.Body.String()
	if strings.Contains(twoGuestsBody, `"user_id"`) || strings.Contains(twoGuestsBody, `"guest_key"`) {
		t.Fatalf("two-guests ranking leaked an internal identity: %s", twoGuestsBody)
	}
	var twoGuestsPayload struct {
		Data struct {
			Entries []struct {
				Nickname string `json:"nickname"`
			} `json:"entries"`
		} `json:"data"`
	}
	if err := json.Unmarshal(twoGuests.Body.Bytes(), &twoGuestsPayload); err != nil {
		t.Fatal(err)
	}
	if len(twoGuestsPayload.Data.Entries) != 2 || !strings.HasPrefix(twoGuestsPayload.Data.Entries[0].Nickname, "游客") || !strings.HasPrefix(twoGuestsPayload.Data.Entries[1].Nickname, "游客") || twoGuestsPayload.Data.Entries[0].Nickname == twoGuestsPayload.Data.Entries[1].Nickname {
		t.Fatalf("two distinct guests share one label: %+v", twoGuestsPayload.Data.Entries)
	}

	// A Platform Core outage degrades every nickname to 游客x; the ranking
	// stays available and still never leaks user_id.
	dead := newHandler("http://127.0.0.1:1")
	degraded := get(dead, "req_ranking_degraded")
	if degraded.Code != http.StatusOK {
		t.Fatalf("degraded ranking = %d %s", degraded.Code, degraded.Body.String())
	}
	degradedBody := degraded.Body.String()
	if strings.Contains(degradedBody, `"user_id"`) || strings.Contains(degradedBody, namedUser) {
		t.Fatalf("degraded ranking leaked an internal identifier: %s", degradedBody)
	}
	if strings.Count(degradedBody, "游客") < 2 {
		t.Fatalf("degraded ranking did not fall back to 游客x: %s", degradedBody)
	}
}

func stringPointer(value string) *string { return &value }

// guestLabelFor recomputes the stable 游客x label for an identity key so the
// privacy test can assert exact nickname derivation.
func guestLabelFor(identityKey string) string {
	hash := uint32(2166136261)
	for index := 0; index < len(identityKey); index++ {
		hash ^= uint32(identityKey[index])
		hash *= 16777619
	}
	return fmt.Sprintf("游客%d", hash%9000+1000)
}

func validSystemAvatar(value string) bool {
	switch value {
	case "scholar-blue", "coder-green", "reader-amber", "owl-purple":
		return true
	default:
		return false
	}
}
