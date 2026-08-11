package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"henukit.dev/portal-gateway/internal/config"
)

const libraryDownloadSecret = "library-download-gateway-secret-at-least-32-bytes"

func TestLibraryDownloadUsesExactOwnerCommandAndReturnsBoundedRedirect(t *testing.T) {
	signedAt := time.Now().UTC().Truncate(time.Second)
	expiresAt := signedAt.Add(45 * time.Second).Format(time.RFC3339)
	owner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/public-materials/mat-01/download-starts" {
			t.Fatalf("owner request = %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("X-Service-Id") != "portal-gateway-library-download" || r.Header.Get("X-Signature") == "" {
			t.Fatal("owner request is missing required service authentication")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"download_start_id": "11111111-1111-4111-8111-111111111111",
				"method":            "GET",
				"location":          validOSSLocation(signedAt, 45, nil),
				"expires_at":        expiresAt,
			},
			"request_id": "req_library_owner",
		})
	}))
	defer owner.Close()

	handler := newLibraryDownloadHandler(t, owner.URL, "http://portal-api.invalid")
	response := httptest.NewRecorder()
	handler.Router().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/library/materials/mat-01/download", nil))

	if response.Code != http.StatusSeeOther {
		t.Fatalf("download status = %d: %s", response.Code, response.Body.String())
	}
	if got := response.Header().Get("Location"); !strings.HasPrefix(got, "https://henukit.oss-cn-beijing.aliyuncs.com/") {
		t.Fatalf("Location = %q", got)
	}
	for name, want := range map[string]string{
		"Cache-Control":          "no-store",
		"Referrer-Policy":        "no-referrer",
		"X-Content-Type-Options": "nosniff",
	} {
		if got := response.Header().Get(name); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
}

func TestLibraryDownloadRejectsUnsafeOwnerCapabilities(t *testing.T) {
	signedAt := time.Now().UTC().Truncate(time.Second)
	validExpiry := signedAt.Add(45 * time.Second).Format(time.RFC3339)
	validLocation := validOSSLocation(signedAt, 45, nil)
	for _, tc := range []struct {
		name, method, location, expiresAt string
	}{
		{"non GET", "POST", validLocation, validExpiry},
		{"plain HTTP", "GET", strings.Replace(validLocation, "https://", "http://", 1), validExpiry},
		{"wrong host", "GET", strings.Replace(validLocation, "henukit.oss-cn-beijing.aliyuncs.com", "attacker.example", 1), validExpiry},
		{"userinfo", "GET", strings.Replace(validLocation, "https://", "https://user@", 1), validExpiry},
		{"fragment", "GET", validLocation + "#leak", validExpiry},
		{"over 60 seconds", "GET", validOSSLocation(signedAt, 61, nil), signedAt.Add(61 * time.Second).Format(time.RFC3339)},
		{"missing version", "GET", validOSSLocation(signedAt, 45, func(q url.Values) { q.Del("versionId") }), validExpiry},
		{"missing signature", "GET", validOSSLocation(signedAt, 45, func(q url.Values) { q.Del("x-oss-signature") }), validExpiry},
		{"missing security token", "GET", validOSSLocation(signedAt, 45, func(q url.Values) { q.Del("x-oss-security-token") }), validExpiry},
		{"missing credential", "GET", validOSSLocation(signedAt, 45, func(q url.Values) { q.Del("x-oss-credential") }), validExpiry},
		{"missing signature version", "GET", validOSSLocation(signedAt, 45, func(q url.Values) { q.Del("x-oss-signature-version") }), validExpiry},
		{"missing attachment", "GET", validOSSLocation(signedAt, 45, func(q url.Values) { q.Del("response-content-disposition") }), validExpiry},
		{"inline attachment", "GET", validOSSLocation(signedAt, 45, func(q url.Values) { q.Set("response-content-disposition", "inline") }), validExpiry},
		{"unknown capability parameter", "GET", validOSSLocation(signedAt, 45, func(q url.Values) { q.Set("x-oss-process", "image/resize,w_100") }), validExpiry},
		{"malformed capability query", "GET", validLocation + "&unexpected=a;b", validExpiry},
		{"duplicate security token", "GET", validOSSLocation(signedAt, 45, func(q url.Values) { q.Add("x-oss-security-token", "another-placeholder") }), validExpiry},
		{"missing query expiry", "GET", validOSSLocation(signedAt, 45, func(q url.Values) { q.Del("x-oss-expires") }), validExpiry},
		{"noninteger query expiry", "GET", validOSSLocation(signedAt, 45, func(q url.Values) { q.Set("x-oss-expires", "soon") }), validExpiry},
		{"zero query expiry", "GET", validOSSLocation(signedAt, 45, func(q url.Values) { q.Set("x-oss-expires", "0") }), validExpiry},
		{"missing signing date", "GET", validOSSLocation(signedAt, 45, func(q url.Values) { q.Del("x-oss-date") }), validExpiry},
		{"invalid signing date", "GET", validOSSLocation(signedAt, 45, func(q url.Values) { q.Set("x-oss-date", "not-a-date") }), validExpiry},
		{"query and response expiry disagree", "GET", validLocation, signedAt.Add(44 * time.Second).Format(time.RFC3339)},
		{"expired query", "GET", validOSSLocation(signedAt.Add(-2*time.Minute), 45, nil), signedAt.Add(-75 * time.Second).Format(time.RFC3339)},
		{"future signing date", "GET", validOSSLocation(signedAt.Add(10*time.Second), 45, nil), signedAt.Add(55 * time.Second).Format(time.RFC3339)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			owner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"data": map[string]any{
						"download_start_id": "11111111-1111-4111-8111-111111111111",
						"method":            tc.method, "location": tc.location, "expires_at": tc.expiresAt,
					},
					"request_id": "req_library_owner",
				})
			}))
			defer owner.Close()

			handler := newLibraryDownloadHandler(t, owner.URL, "http://portal-api.invalid")
			response := httptest.NewRecorder()
			handler.Router().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/library/materials/mat-01/download", nil))
			if response.Code != http.StatusServiceUnavailable || response.Header().Get("Location") != "" {
				t.Fatalf("unsafe capability response = %d Location=%q: %s", response.Code, response.Header().Get("Location"), response.Body.String())
			}
		})
	}
}

func TestLibraryDownloadRejectsTrailingJSON(t *testing.T) {
	signedAt := time.Now().UTC().Truncate(time.Second)
	owner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"download_start_id": "11111111-1111-4111-8111-111111111111",
				"method":            "GET",
				"location":          validOSSLocation(signedAt, 45, nil),
				"expires_at":        signedAt.Add(45 * time.Second).Format(time.RFC3339),
			},
			"request_id": "req_library_owner",
		})
		_, _ = w.Write([]byte("{}"))
	}))
	defer owner.Close()

	handler := newLibraryDownloadHandler(t, owner.URL, "http://portal-api.invalid")
	response := httptest.NewRecorder()
	handler.Router().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/library/materials/mat-01/download", nil))
	if response.Code != http.StatusServiceUnavailable || response.Header().Get("Location") != "" {
		t.Fatalf("trailing JSON response = %d Location=%q: %s", response.Code, response.Header().Get("Location"), response.Body.String())
	}
}

func TestLibraryDownloadMapsOwnerStatusesToFrozenBrowserErrors(t *testing.T) {
	for _, tc := range []struct {
		ownerStatus int
		wantStatus  int
		wantCode    string
		wantMessage string
	}{
		{http.StatusNotFound, http.StatusNotFound, "MATERIAL_NOT_AVAILABLE", "资料不存在或已下架，请返回资料库重新选择。"},
		{http.StatusGone, http.StatusNotFound, "MATERIAL_NOT_AVAILABLE", "资料不存在或已下架，请返回资料库重新选择。"},
		{http.StatusConflict, http.StatusServiceUnavailable, "DOWNLOAD_TEMPORARILY_UNAVAILABLE", "暂时无法生成下载链接，请稍后重试。"},
	} {
		t.Run(strconv.Itoa(tc.ownerStatus), func(t *testing.T) {
			owner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.ownerStatus)
			}))
			defer owner.Close()

			handler := newLibraryDownloadHandler(t, owner.URL, "http://portal-api.invalid")
			response := httptest.NewRecorder()
			handler.Router().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/library/materials/mat-01/download", nil))
			var body struct {
				Error   string `json:"error"`
				Message string `json:"message"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if response.Code != tc.wantStatus || body.Error != tc.wantCode || body.Message != tc.wantMessage || response.Header().Get("Location") != "" {
				t.Fatalf("owner %d => status=%d code=%q message=%q Location=%q", tc.ownerStatus, response.Code, body.Error, body.Message, response.Header().Get("Location"))
			}
		})
	}
}

func validOSSLocation(signedAt time.Time, expiresSeconds int, mutate func(url.Values)) string {
	query := url.Values{
		"versionId":                    {"version-placeholder"},
		"response-cache-control":       {"private, no-store"},
		"response-content-disposition": {`attachment; filename="material.pdf"`},
		"x-oss-signature":              {"signature-placeholder"},
		"x-oss-signature-version":      {"OSS4-HMAC-SHA256"},
		"x-oss-expires":                {strconv.Itoa(expiresSeconds)},
		"x-oss-date":                   {signedAt.UTC().Format("20060102T150405Z")},
		"x-oss-security-token":         {"security-token-placeholder"},
		"x-oss-credential":             {"temporary-credential-placeholder/" + signedAt.UTC().Format("20060102") + "/cn-beijing/oss/aliyun_v4_request"},
	}
	if mutate != nil {
		mutate(query)
	}
	return "https://henukit.oss-cn-beijing.aliyuncs.com/releases/r1/file.pdf?" + query.Encode()
}

func TestLibraryDownloadDoesNotFallBackToPortalAPI(t *testing.T) {
	portalAPICalled := false
	portalAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		portalAPICalled = true
		w.WriteHeader(http.StatusOK)
	}))
	defer portalAPI.Close()

	handler := newLibraryDownloadHandler(t, "", portalAPI.URL)
	response := httptest.NewRecorder()
	handler.Router().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/library/materials/mat-01/download", nil))
	if response.Code != http.StatusServiceUnavailable || portalAPICalled {
		t.Fatalf("unconfigured response = %d portal_api_called=%t: %s", response.Code, portalAPICalled, response.Body.String())
	}
}

func TestLibraryDownloadOwnerRedirectIsNotFollowed(t *testing.T) {
	redirectFollowed := false
	redirectTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirectFollowed = true
		w.WriteHeader(http.StatusCreated)
	}))
	defer redirectTarget.Close()
	owner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, redirectTarget.URL+"/capability", http.StatusFound)
	}))
	defer owner.Close()

	handler := newLibraryDownloadHandler(t, owner.URL, "http://portal-api.invalid")
	response := httptest.NewRecorder()
	handler.Router().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/library/materials/mat-01/download", nil))
	if response.Code != http.StatusServiceUnavailable || redirectFollowed || response.Header().Get("Location") != "" {
		t.Fatalf("owner redirect response = %d followed=%t Location=%q", response.Code, redirectFollowed, response.Header().Get("Location"))
	}
}

func newLibraryDownloadHandler(t *testing.T, ownerURL, portalAPIURL string) *Handler {
	t.Helper()
	handler, err := New(config.Config{
		SessionKey:             []byte("0123456789abcdef0123456789abcdef"),
		PortalAPIURL:           portalAPIURL,
		LibraryDownloadURL:     ownerURL,
		LibraryDownloadAuth:    config.ServiceAuth{ClientID: "portal-gateway-library-download", ClientSecret: libraryDownloadSecret, KeyID: "library-download-key"},
		LocalOAuthCookieName:   "portal_oauth_local",
		LocalSessionCookieName: "portal_session_local",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return handler
}
