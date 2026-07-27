package httpapi

import (
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"henukit.dev/portal-gateway/internal/session"
)

func TestPracticeStatsRequiresSessionAndForwardsVerifiedActor(t *testing.T) {
	var (
		mu     sync.Mutex
		actors []string
	)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		actors = append(actors, r.Header.Get("X-Actor-User-Id"))
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"request_id":"upstream"}`)
	}))
	defer upstream.Close()

	codec, err := session.NewCodec([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	handler := &Handler{
		sessionCodec:       codec,
		portalAPI:          upstream.Client(),
		portalAPIURL:       upstream.URL,
		localSessionCookie: "local_session",
	}
	router := handler.Router()

	anonymous := httptest.NewRequest(http.MethodGet, "http://portal.test/api/v1/practice/stats", nil)
	anonymousRecorder := httptest.NewRecorder()
	router.ServeHTTP(anonymousRecorder, anonymous)
	if anonymousRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous status = %d, want 401", anonymousRecorder.Code)
	}

	for _, actor := range []string{"user-a", "user-b"} {
		encoded, err := codec.Encode(session.Value{
			UserID:        actor,
			ExchangeToken: strings.Repeat("x", 32),
			ExpiresAt:     time.Now().Add(time.Hour),
		})
		if err != nil {
			t.Fatal(err)
		}
		request := httptest.NewRequest(http.MethodGet, "https://portal.test/api/v1/practice/stats", nil)
		request.TLS = &tls.ConnectionState{}
		request.Header.Set("X-Actor-User-Id", "spoofed")
		request.AddCookie(&http.Cookie{
			Name:  "__Host-henukit_portal_session",
			Value: encoded,
		})
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want 200", actor, recorder.Code)
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if strings.Join(actors, ",") != "user-a,user-b" {
		t.Fatalf("forwarded actors = %q, want verified session identities", actors)
	}
}
