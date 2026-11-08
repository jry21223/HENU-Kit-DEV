package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	accountapi "henukit.dev/console-gateway/internal/accountportfolio"
	foodapi "henukit.dev/console-gateway/internal/food"
	libraryapi "henukit.dev/console-gateway/internal/library"
	noticeapi "henukit.dev/console-gateway/internal/notice"
	"henukit.dev/console-gateway/internal/platformcore"
	"henukit.dev/console-gateway/internal/session"
)

// Exercise the browser HTTP boundary through the real signed Core client. The
// Core test server is the external dependency; no production credentials exist.
func sessionBoundaryGateway(t *testing.T, coreHandler http.HandlerFunc) (*httptest.Server, *http.Cookie) {
	t.Helper()
	core := httptest.NewServer(coreHandler)
	t.Cleanup(core.Close)
	platform, err := platformcore.New(core.URL, "console-gateway", "isolated-console-test-secret-32-bytes", "test-key", core.Client())
	if err != nil {
		t.Fatal(err)
	}
	codec, err := session.New([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	redisClient := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	t.Cleanup(func() { _ = redisClient.Close() })
	handler, err := New("https://account.henukit.test", "console-gateway", "https://console.henukit.test/api/v1/auth/callback", platform, nil, fakeOverview{}, redisClient, codec, slog.New(slog.NewTextHandler(io.Discard, nil)), &fakeLibrary{}, &fakeFood{}, &fakeAccountPortfolio{})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	encoded, err := codec.Encode(session.Value{UserID: "171f1c6f-7b10-4c92-91a2-b39bf5af5302", ExchangeToken: "isolated_exchange_token_at_least_32_characters", ExpiresAt: time.Now().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	return server, &http.Cookie{Name: sessionCookie, Value: encoded}
}

type rejectedNoticeService struct{ fakeNotice }

func (*rejectedNoticeService) Snapshot(context.Context, string) (json.RawMessage, error) {
	return nil, noticeapi.ErrUnauthorized
}

type rejectedLibraryService struct{ fakeLibrary }

func (*rejectedLibraryService) Workspace(context.Context, string) (json.RawMessage, error) {
	return nil, libraryapi.ErrUnauthorized
}

type rejectedFoodService struct{ fakeFood }

func (*rejectedFoodService) Workspace(context.Context, string, string) (json.RawMessage, error) {
	return nil, foodapi.ErrUnauthorized
}

func (*rejectedFoodService) Operation(context.Context, string, string, string) (json.RawMessage, error) {
	return nil, foodapi.ErrUnauthorized
}

type rejectedAccountService struct{ fakeAccountPortfolio }

func (*rejectedAccountService) Tickets(context.Context, string) (json.RawMessage, error) {
	return nil, accountapi.ErrUnauthorized
}

func (*rejectedAccountService) Operation(context.Context, string, string, string) (json.RawMessage, error) {
	return nil, accountapi.ErrUnauthorized
}

func TestConsoleOwnerServiceAuthenticationFailureDoesNotExpireBrowserSession(t *testing.T) {
	for _, path := range []string{"/api/v1/notices", "/api/v1/library", "/api/v1/food", "/api/v1/account/tickets", "/api/v1/account/operations/ticket_reply/idem_original_operation", "/api/v1/food/operations/post_edit", "/api/v1/food/operations/post_edit/outcome"} {
		t.Run(path, func(t *testing.T) {
			codec, _ := session.New([]byte("0123456789abcdef0123456789abcdef"))
			redisClient := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
			t.Cleanup(func() { _ = redisClient.Close() })
			platform := &fakePlatform{exchange: platformcore.Exchange{ExchangeToken: "isolated_exchange_token_at_least_32_characters"}}
			handler, err := New("https://account.henukit.test", "console-gateway", "https://console.henukit.test/api/v1/auth/callback", platform, &rejectedNoticeService{}, fakeOverview{}, redisClient, codec, slog.New(slog.NewTextHandler(io.Discard, nil)), &rejectedLibraryService{}, &rejectedFoodService{}, &rejectedAccountService{})
			if err != nil {
				t.Fatal(err)
			}
			encoded, _ := codec.Encode(session.Value{UserID: "171f1c6f-7b10-4c92-91a2-b39bf5af5302", ExchangeToken: platform.exchange.ExchangeToken, ExpiresAt: time.Now().Add(time.Hour)})
			request := httptest.NewRequest(http.MethodGet, path, nil)
			request.AddCookie(&http.Cookie{Name: sessionCookie, Value: encoded})
			request.Header.Set("Idempotency-Key", "idem_original_operation")
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "DEPENDENCY_UNAVAILABLE") {
				t.Fatalf("owner rejects signed service request = %d %s; want dependency error, not browser expiry", response.Code, response.Body.String())
			}
			if len(response.Result().Cookies()) != 0 {
				t.Fatal("owner service failure must not clear the verified browser Session")
			}
		})
	}
}

func TestConsoleSessionDoesNotPresentPartialVerificationAsMissingPermission(t *testing.T) {
	server, cookie := sessionBoundaryGateway(t, func(w http.ResponseWriter, r *http.Request) {
		var check struct {
			Permission string `json:"permission_code"`
		}
		_ = json.NewDecoder(r.Body).Decode(&check)
		switch check.Permission {
		case "console.overview.read":
			w.WriteHeader(http.StatusOK)
		case "account.tickets.reply":
			w.WriteHeader(http.StatusServiceUnavailable)
		default:
			w.WriteHeader(http.StatusForbidden)
		}
	})
	request, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/session", nil)
	request.AddCookie(cookie)
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusServiceUnavailable || !strings.Contains(string(body), "DEPENDENCY_UNAVAILABLE") {
		t.Fatalf("partial authorization verification = %d %s; want honest dependency-unavailable without a verified access context", response.StatusCode, body)
	}
	if len(response.Cookies()) != 0 {
		t.Fatal("dependency failure must not clear the operator's Session")
	}
}

func TestConsoleSessionBoundsAllNineteenCoreChecksByOneDeadline(t *testing.T) {
	server, cookie := sessionBoundaryGateway(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(200 * time.Millisecond):
			w.WriteHeader(http.StatusOK)
		case <-r.Context().Done():
		}
	})
	request, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/session", nil)
	request.AddCookie(cookie)
	client := server.Client()
	client.Timeout = 5 * time.Second
	started := time.Now()
	response, err := client.Do(request)
	elapsed := time.Since(started)
	if err != nil {
		t.Fatalf("Session did not terminate under its own budget: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusServiceUnavailable || elapsed > 3500*time.Millisecond {
		t.Fatalf("slow Core Session = %d after %s; want 503 within the single three-second deadline plus 500ms scheduling tolerance", response.StatusCode, elapsed)
	}
	if len(response.Cookies()) != 0 {
		t.Fatal("slow Core must not clear the operator's Session")
	}
}

func TestConsoleSessionDoesNotTreatAmbiguousOrServiceCore401AsVerifiedSessionExpiry(t *testing.T) {
	for _, code := range []string{"AUTHENTICATION_FAILED", "CLIENT_AUTH_FAILED", "SIGNATURE_INVALID", "TIMESTAMP_INVALID", ""} {
		t.Run(code, func(t *testing.T) {
			server, cookie := sessionBoundaryGateway(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": code}})
			})
			request, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/session", nil)
			request.AddCookie(cookie)
			response, err := server.Client().Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if response.StatusCode != http.StatusServiceUnavailable || len(response.Cookies()) != 0 {
				t.Fatalf("Core %s = %d, cookies=%v; want dependency-unknown without discarding Session", code, response.StatusCode, response.Cookies())
			}
		})
	}
}

func TestConsoleSignedCoreBusinessServiceFailureDoesNotExpireBrowserSession(t *testing.T) {
	server, cookie := sessionBoundaryGateway(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/authorization/check" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":{"code":"SIGNATURE_INVALID","message":"request signature is invalid"}}`)
	})
	request, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/operations", nil)
	request.AddCookie(cookie)
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusServiceUnavailable || len(response.Cookies()) != 0 {
		t.Fatalf("Core operation service rejection = %d cookies=%v; want dependency error without browser expiry", response.StatusCode, response.Cookies())
	}
}

func TestConsoleSessionDistinguishesVerifiedInvalidSessionFromPermissionDenial(t *testing.T) {
	for _, status := range []int{http.StatusForbidden, http.StatusUnauthorized} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			server, cookie := sessionBoundaryGateway(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(status)
				_, _ = io.WriteString(w, `{"error":{"code":"SESSION_INVALID","message":"Session is invalid"}}`)
			})
			request, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/session", nil)
			request.AddCookie(cookie)
			response, err := server.Client().Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if response.StatusCode != status {
				t.Fatalf("verified rejection = %d, want %d", response.StatusCode, status)
			}
			if status == http.StatusUnauthorized {
				if len(response.Cookies()) != 1 || response.Cookies()[0].MaxAge != -1 {
					t.Fatal("verified invalid Session must expire the local cookie")
				}
			} else if len(response.Cookies()) != 0 {
				t.Fatal("permission denial is not Session expiry")
			}
		})
	}
}

func TestConsoleSessionPreservesExactlyGrantedPermissionsAndProductScopes(t *testing.T) {
	server, cookie := sessionBoundaryGateway(t, func(w http.ResponseWriter, r *http.Request) {
		var check struct {
			Permission string                             `json:"permission_code"`
			Scope      struct{ Kind, ProductCode string } `json:"scope"`
		}
		var payload map[string]json.RawMessage
		_ = json.NewDecoder(r.Body).Decode(&payload)
		_ = json.Unmarshal(payload["permission_code"], &check.Permission)
		var scope map[string]string
		_ = json.Unmarshal(payload["scope"], &scope)
		if check.Permission == "notice.read" && scope["kind"] == "product" && scope["product_code"] == "notice" || check.Permission == "account.points.adjust" && scope["kind"] == "product" && scope["product_code"] == "account-portfolio" {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusForbidden)
		}
	})
	request, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/session", nil)
	request.AddCookie(cookie)
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var envelope struct {
		Data struct {
			AccessContext struct {
				Permissions []string            `json:"permissions"`
				Scopes      []map[string]string `json:"scopes"`
			} `json:"access_context"`
		} `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || !reflect.DeepEqual(envelope.Data.AccessContext.Permissions, []string{"notice.read", "account.points.adjust"}) || !reflect.DeepEqual(envelope.Data.AccessContext.Scopes, []map[string]string{{"kind": "platform"}, {"kind": "product", "product_code": "notice"}, {"kind": "product", "product_code": "account-portfolio"}}) {
		t.Fatalf("access context is not the precise Core-verified subset: status=%d %+v", response.StatusCode, envelope.Data.AccessContext)
	}
}

func TestOperationQueriesDistinguishCoreAuthenticationFailureFromSessionExpiry(t *testing.T) {
	for _, route := range []struct{ path, permission, product string }{
		{"/api/v1/account/operations/ticket_reply/idem_original_operation", "account.tickets.reply", "account-portfolio"},
		{"/api/v1/food/operations/post_edit", "food.review", "food"},
		{"/api/v1/food/operations/post_edit/outcome", "food.review", "food"},
	} {
		for _, outcome := range []struct {
			code             string
			coreStatus, want int
			clear            bool
		}{
			{"SESSION_INVALID", http.StatusUnauthorized, http.StatusUnauthorized, true},
			{"SIGNATURE_INVALID", http.StatusUnauthorized, http.StatusServiceUnavailable, false},
			{"", http.StatusUnauthorized, http.StatusServiceUnavailable, false},
			{"ACCESS_DENIED", http.StatusForbidden, http.StatusForbidden, false},
		} {
			t.Run(route.permission+"/"+outcome.code, func(t *testing.T) {
				server, cookie := sessionBoundaryGateway(t, func(w http.ResponseWriter, r *http.Request) {
					var check struct {
						Permission string `json:"permission_code"`
						Scope      struct {
							Kind    string `json:"kind"`
							Product string `json:"product_code"`
						} `json:"scope"`
					}
					if err := json.NewDecoder(r.Body).Decode(&check); err != nil || r.URL.Path != "/api/v1/authorization/check" || check.Permission != route.permission || check.Scope.Kind != "product" || check.Scope.Product != route.product {
						t.Errorf("incorrect original-operation authorization: %s %+v err=%v", r.URL.Path, check, err)
					}
					w.WriteHeader(outcome.coreStatus)
					_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": outcome.code}})
				})
				request, _ := http.NewRequest(http.MethodGet, server.URL+route.path, nil)
				request.AddCookie(cookie)
				request.Header.Set("Idempotency-Key", "idem_original_operation")
				response, err := server.Client().Do(request)
				if err != nil {
					t.Fatal(err)
				}
				defer response.Body.Close()
				body, _ := io.ReadAll(response.Body)
				if response.StatusCode != outcome.want {
					t.Fatalf("response=%d %s; want %d", response.StatusCode, body, outcome.want)
				}
				cookies := response.Cookies()
				if outcome.clear {
					if len(cookies) != 1 || cookies[0].MaxAge != -1 {
						t.Fatalf("confirmed expiry did not clear browser Session: %v", cookies)
					}
				} else if len(cookies) != 0 {
					t.Fatalf("unconfirmed expiry cleared browser Session: %v", cookies)
				}
			})
		}
	}
}
