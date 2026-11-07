package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestConsoleNoticePreservesVerifiedSessionErrorsBeforeMissingConnection(t *testing.T) {
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
		t.Run(outcome.code, func(t *testing.T) {
			server, cookie := sessionBoundaryGateway(t, func(w http.ResponseWriter, r *http.Request) {
				var check struct {
					Permission string `json:"permission_code"`
					Scope      struct {
						Kind    string `json:"kind"`
						Product string `json:"product_code"`
					} `json:"scope"`
				}
				if err := json.NewDecoder(r.Body).Decode(&check); err != nil || check.Permission != "notice.read" || check.Scope.Kind != "product" || check.Scope.Product != "notice" {
					t.Errorf("incorrect Notice permission/Scope: %+v err=%v", check, err)
				}
				w.WriteHeader(outcome.coreStatus)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": outcome.code}})
			})
			request, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/notices", nil)
			request.AddCookie(cookie)
			response, err := server.Client().Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			body, _ := io.ReadAll(response.Body)
			if response.StatusCode != outcome.want || strings.Contains(string(body), "NOTICE_NOT_CONFIGURED") {
				t.Fatalf("Session result hidden by connection state: %d %s", response.StatusCode, body)
			}
			cookies := response.Cookies()
			if outcome.clear {
				if len(cookies) != 1 || cookies[0].MaxAge != -1 {
					t.Fatalf("confirmed expiry did not clear Session: %v", cookies)
				}
			} else if len(cookies) != 0 {
				t.Fatalf("unconfirmed expiry cleared Session: %v", cookies)
			}
		})
	}
}

func TestConsoleNoticeMissingManagementURLIsNotReportedAsProductOutage(t *testing.T) {
	server, cookie := sessionBoundaryGateway(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	request, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/notices", nil)
	request.AddCookie(cookie)
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusServiceUnavailable || !strings.Contains(string(body), `"code":"NOTICE_NOT_CONFIGURED"`) {
		t.Fatalf("missing Notice management URL = %d %s; want explicit not-configured state", response.StatusCode, body)
	}
}

func TestConsoleNoticeChecksExactPermissionBeforeReportingMissingConnection(t *testing.T) {
	server, cookie := sessionBoundaryGateway(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusForbidden) })
	request, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/notices", nil)
	request.AddCookie(cookie)
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("ungranted notice.read with no management URL = %d; want exact Core denial before readiness", response.StatusCode)
	}
}
