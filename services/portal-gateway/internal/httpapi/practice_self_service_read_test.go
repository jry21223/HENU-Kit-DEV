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

	"henukit.dev/portal-gateway/internal/practice"
	"henukit.dev/portal-gateway/internal/session"
)

const selfServiceBankID = "10ca9b18-c303-4b7a-ab14-1241e41b665a"

func TestPracticeFavoritesReadsUseOnlyTheVerifiedPortalSessionActor(t *testing.T) {
	var platformCalls atomic.Int32
	platform := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		platformCalls.Add(1)
		http.Error(w, "no learning-data RBAC permission", http.StatusForbidden)
	}))
	defer platform.Close()
	var actors []string
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		actor := r.Header.Get("X-Actor-User-Id")
		actors = append(actors, actor)
		assertActorBoundStatsSignature(t, r, actor, "catalog-secret-with-enough-entropy")
		if r.Method != http.MethodGet || r.Header.Get("X-Permission-Code") != practice.PortalReadPermission || r.Header.Get("X-Scope-Kind") != "product" || r.Header.Get("X-Product-Code") != "quizcraft" || r.Header.Get("Cookie") != "" {
			t.Errorf("owner read lost service credential scope or forwarded browser cookies")
		}
		requestID := r.Header.Get("X-Request-Id")
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-Id", requestID)
		var data any
		switch r.URL.Path {
		case practice.GetPortalFavoritesOverviewPath:
			data = []practice.FavoriteFolder{{BankID: selfServiceBankID, BankName: "Fixture bank", AvailableCount: 1}}
		case "/api/v1/portal/practice/banks/" + selfServiceBankID + "/favorites":
			data = []practice.FavoriteQuestion{{BankID: selfServiceBankID, QuestionID: "5d838137-d3f0-4e27-b6e9-17a919e62c49", Available: true, QuestionVersionID: "27d773c7-6988-4bf8-9052-c7a4a36bce8b"}}
		default:
			t.Errorf("unexpected owner read %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{"request_id": requestID, "data": data})
	}))
	defer core.Close()
	h := newPersonalStatsHandler(t, platform.URL, core.URL, "http://127.0.0.1:9")
	users := []string{personalStatsUserID, "80901c97-6d9e-4648-8446-386532aee652"}
	for _, userID := range users {
		for _, path := range []string{"/api/v1/practice/favorites", "/api/v1/practice/banks/" + selfServiceBankID + "/favorites"} {
			r := httptest.NewRequest(http.MethodGet, "http://portal.test"+path, nil)
			r.AddCookie(sessionCookie(t, h, userID))
			r.AddCookie(&http.Cookie{Name: "quizcraft_anonymous", Value: "must-not-replace-signed-in-actor"})
			r.Header.Set("X-Actor-User-Id", "forged-browser-actor")
			r.Header.Set("X-Request-Id", "req_self_service")
			w := httptest.NewRecorder()
			h.Router().ServeHTTP(w, r)
			if w.Code != http.StatusOK || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("X-Content-Type-Options") != "nosniff" {
				t.Fatalf("favorites read = %d %s headers=%v", w.Code, w.Body.String(), w.Header())
			}
			if got := actors[len(actors)-1]; got != userID {
				t.Fatalf("owner actor=%q want session actor %q", got, userID)
			}
		}
	}
	if platformCalls.Load() != 0 || len(actors) != 4 {
		t.Fatalf("read calls platform=%d owner=%d", platformCalls.Load(), len(actors))
	}
}

func TestPracticeSelfServiceReadsRejectInvalidSessionsBeforeOwnerRequest(t *testing.T) {
	var ownerCalls atomic.Int32
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { ownerCalls.Add(1); w.WriteHeader(500) }))
	defer core.Close()
	h := newPersonalStatsHandler(t, "http://127.0.0.1:9", core.URL, "http://127.0.0.1:9")
	cookieFor := func(userID string, expiresAt time.Time) *http.Cookie {
		encoded, err := h.sessionCodec.Encode(session.Value{UserID: userID, ExchangeToken: strings.Repeat("x", 32), ExpiresAt: expiresAt})
		if err != nil {
			t.Fatal(err)
		}
		return &http.Cookie{Name: "henukit_portal_session_local", Value: encoded}
	}
	cases := []struct {
		name   string
		cookie *http.Cookie
	}{
		{name: "missing"},
		{name: "tampered", cookie: &http.Cookie{Name: "henukit_portal_session_local", Value: "tampered"}},
		{name: "expired", cookie: cookieFor(personalStatsUserID, time.Now().Add(-time.Minute))},
		{name: "malformed_actor", cookie: cookieFor("not-a-uuid", time.Now().Add(time.Hour))},
		{name: "nil_actor", cookie: cookieFor("00000000-0000-0000-0000-000000000000", time.Now().Add(time.Hour))},
		{name: "padded_actor", cookie: cookieFor(" "+personalStatsUserID, time.Now().Add(time.Hour))},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, path := range []string{"/api/v1/practice/stats", "/api/v1/practice/favorites", "/api/v1/practice/banks/" + selfServiceBankID + "/favorites"} {
				r := httptest.NewRequest(http.MethodGet, "http://portal.test"+path, nil)
				if tc.cookie != nil {
					r.AddCookie(tc.cookie)
				}
				r.AddCookie(&http.Cookie{Name: "quizcraft_anonymous", Value: "not-a-signed-in-session"})
				r.Header.Set("X-Actor-User-Id", personalStatsUserID)
				w := httptest.NewRecorder()
				h.Router().ServeHTTP(w, r)
				if w.Code != http.StatusUnauthorized || strings.Contains(w.Body.String(), `"data"`) || w.Header().Get("Cache-Control") != "no-store" {
					t.Fatalf("%s=%d %s", path, w.Code, w.Body.String())
				}
			}
		})
	}
	if ownerCalls.Load() != 0 {
		t.Fatalf("invalid session reached owner %d times", ownerCalls.Load())
	}
}

func TestPracticeFavoritesReadsFailClosedOnOwnerRejectionAndOutage(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusServiceUnavailable} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(status)
				w.Write([]byte(`{"internal":"owner-detail-must-not-leak"}`))
			}))
			defer core.Close()
			h := newPersonalStatsHandler(t, "http://127.0.0.1:9", core.URL, "http://127.0.0.1:9")
			for _, path := range []string{"/api/v1/practice/favorites", "/api/v1/practice/banks/" + selfServiceBankID + "/favorites"} {
				r := httptest.NewRequest(http.MethodGet, "http://portal.test"+path, nil)
				r.AddCookie(sessionCookie(t, h, personalStatsUserID))
				w := httptest.NewRecorder()
				h.Router().ServeHTTP(w, r)
				if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "practice favorites are temporarily unavailable") || strings.Contains(w.Body.String(), "owner-detail-must-not-leak") || strings.Contains(w.Body.String(), `"data"`) {
					t.Fatalf("owner %d=%d %s", status, w.Code, w.Body.String())
				}
			}
		})
	}
}
