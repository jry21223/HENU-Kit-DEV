package career

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"henukit.dev/career/internal/contract"
)

func TestDisabledJobSourceRejectsAuthenticatedScanWithoutTaskDependencies(t *testing.T) {
	// Authentication normally supplies this actor before calling the HTTP
	// handler. Nil task dependencies prove disabled mode never starts storage,
	// rate-limit accounting or crawling.
	h := &service{jobSourceDisabled: true}
	r := httptest.NewRequest(http.MethodPost, contract.CreateSearchRoute, strings.NewReader(`{"profile":{"target_roles":"后端"}}`))
	r = r.WithContext(context.WithValue(r.Context(), actorKey, actor{userID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"}))
	w := httptest.NewRecorder()
	h.createSearch(w, r)
	if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), `"code":"JOB_SOURCE_DISABLED"`) {
		t.Fatalf("disabled scan status=%d body=%s", w.Code, w.Body)
	}
	if strings.Contains(w.Body.String(), "WSL") || strings.Contains(w.Body.String(), "getWork") {
		t.Fatal("disabled scan exposed internal source details")
	}
}

func TestDisabledJobSourceKeepsAuthenticationAndReportsItsOffState(t *testing.T) {
	service, err := New(Config{
		Database: &pgxpool.Pool{}, Redis: &redis.Client{},
		ClientID: "career-client", Keys: map[string]string{"active": strings.Repeat("s", 40)},
		JobSourceDisabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	service.ServeHTTP(w, httptest.NewRequest(http.MethodPost, contract.CreateSearchRoute, strings.NewReader(`{}`)))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unsigned scan status=%d", w.Code)
	}
	w = httptest.NewRecorder()
	service.ServeHTTP(w, httptest.NewRequest(http.MethodGet, contract.HealthRoute, nil))
	var response struct {
		Data map[string]bool `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	enabled, present := response.Data["job_source_enabled"]
	if w.Code != http.StatusOK || !response.Data["ok"] || !present || enabled {
		t.Fatalf("disabled source health=%d %s", w.Code, w.Body)
	}
}
