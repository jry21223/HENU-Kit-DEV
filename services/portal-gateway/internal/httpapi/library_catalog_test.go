package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLibraryCatalogUsesExactOwnerSnapshotAndMapsBrowserSafeFields(t *testing.T) {
	owner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/public-materials" || r.URL.RawQuery != "" {
			t.Fatalf("owner request = %s %s", r.Method, r.URL.RequestURI())
		}
		if r.Header.Get("X-Service-Id") != "portal-gateway-library-download" || r.Header.Get("X-Signature") == "" {
			t.Fatal("owner catalog request is missing service authentication")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"release_id": "0123456789abcdef0123456789abcdef01234567-0123456789abcdef",
				"materials": []map[string]any{{
					"id": "11111111-1111-4111-8111-111111111111", "type": "textbook", "subject": "高等数学",
					"title": "高等数学电子版教材", "role": "电子版教材", "file_name": "高等数学电子版教材.pdf",
					"file_size": 4096, "downloads": 12, "download_available": true,
				}},
				"material_count": 1, "download_starts": 99,
				"counting_since": "2026-08-11T00:00:00Z", "as_of": "2026-08-11T01:00:00Z",
			},
			"request_id": "req_library_owner",
		})
	}))
	defer owner.Close()

	handler := newLibraryDownloadHandler(t, owner.URL, "http://portal-api.invalid")
	response := httptest.NewRecorder()
	handler.Router().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/library/materials", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("catalog status = %d: %s", response.Code, response.Body.String())
	}
	var body struct {
		Materials []struct {
			ID, Type, Subject, Title, Author string
			Price, Downloads                 int64
			DownloadAvailable                bool  `json:"downloadAvailable"`
			FileSize                         int64 `json:"fileSize"`
		} `json:"materials"`
		Statistics struct {
			MaterialCount, DownloadStarts int64
			ReleaseID                     string `json:"releaseId"`
		} `json:"statistics"`
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Materials) != 1 || body.Materials[0].ID != "11111111-1111-4111-8111-111111111111" || body.Materials[0].Type != "textbook" || body.Materials[0].Author != "资料库收录" || body.Materials[0].Downloads != 12 || !body.Materials[0].DownloadAvailable || body.Materials[0].FileSize != 4096 {
		t.Fatalf("browser materials = %#v", body.Materials)
	}
	if body.Statistics.MaterialCount != 1 || body.Statistics.DownloadStarts != 99 || body.Statistics.ReleaseID == "" || !strings.HasPrefix(body.RequestID, "req_") || body.RequestID != response.Header().Get("X-Request-Id") {
		t.Fatalf("browser statistics = %#v request_id=%q", body.Statistics, body.RequestID)
	}
	if text := response.Body.String(); strings.Contains(text, "object_key") || strings.Contains(text, "version_id") || strings.Contains(text, "oss-cn") || strings.Contains(text, "location") {
		t.Fatalf("browser response leaked storage authority: %s", text)
	}
}

func TestLibraryCatalogPreservesExplicitEmptyOwnerSuccess(t *testing.T) {
	for _, tc := range []struct {
		name          string
		releaseID     any
		countingSince any
	}{
		{name: "no active release", releaseID: nil, countingSince: nil},
		{name: "active empty release", releaseID: "0123456789abcdef0123456789abcdef01234567-0123456789abcdef", countingSince: "2026-08-11T00:00:00Z"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			owner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"data": map[string]any{
						"release_id": tc.releaseID, "materials": []any{}, "material_count": 0,
						"download_starts": 7, "counting_since": tc.countingSince, "as_of": "2026-08-11T01:00:00Z",
					},
					"request_id": "req_empty_owner",
				})
			}))
			defer owner.Close()

			handler := newLibraryDownloadHandler(t, owner.URL, "http://portal-api.invalid")
			response := httptest.NewRecorder()
			handler.Router().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/library/materials", nil))
			if response.Code != http.StatusOK {
				t.Fatalf("empty catalog status=%d body=%s", response.Code, response.Body.String())
			}
			var body struct {
				Materials  []any `json:"materials"`
				Statistics struct {
					ReleaseID      *string `json:"releaseId"`
					MaterialCount  int64   `json:"materialCount"`
					DownloadStarts int64   `json:"downloadStarts"`
					CountingSince  *string `json:"countingSince"`
				} `json:"statistics"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if len(body.Materials) != 0 || body.Statistics.MaterialCount != 0 || body.Statistics.DownloadStarts != 7 {
				t.Fatalf("empty browser catalog = %#v", body)
			}
			if tc.releaseID == nil && (body.Statistics.ReleaseID != nil || body.Statistics.CountingSince != nil) {
				t.Fatalf("no-release facts = %#v", body.Statistics)
			}
			if tc.releaseID != nil && (body.Statistics.ReleaseID == nil || body.Statistics.CountingSince == nil) {
				t.Fatalf("active-empty facts = %#v", body.Statistics)
			}
		})
	}
}

func TestLibraryCatalogEnforcesOwnerCatalogBound(t *testing.T) {
	for _, tc := range []struct {
		count      int
		wantStatus int
	}{{count: 500, wantStatus: http.StatusOK}, {count: 501, wantStatus: http.StatusServiceUnavailable}} {
		t.Run(fmt.Sprintf("count_%d", tc.count), func(t *testing.T) {
			materials := make([]map[string]any, 0, tc.count)
			subject, title, role, fileName := "数学", "资料", "讲义", "material.pdf"
			if tc.count == 500 {
				subject = strings.Repeat("&", 160)
				title = strings.Repeat("&", 200)
				role = strings.Repeat("&", 160)
				fileName = strings.Repeat("&", 251) + ".pdf"
			}
			for index := range tc.count {
				materials = append(materials, map[string]any{
					"id": fmt.Sprintf("11111111-1111-4111-8111-%012x", index), "type": "note", "subject": subject,
					"title": title, "role": role, "file_name": fileName,
					"file_size": 1, "downloads": 0, "download_available": true,
				})
			}
			owner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"data": map[string]any{
						"release_id": "0123456789abcdef0123456789abcdef01234567-0123456789abcdef",
						"materials":  materials, "material_count": tc.count, "download_starts": 0,
						"counting_since": "2026-08-11T00:00:00Z", "as_of": "2026-08-11T01:00:00Z",
					},
					"request_id": "req_bounded_owner",
				})
			}))
			defer owner.Close()

			handler := newLibraryDownloadHandler(t, owner.URL, "http://portal-api.invalid")
			response := httptest.NewRecorder()
			handler.Router().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/library/materials", nil))
			if response.Code != tc.wantStatus {
				t.Fatalf("count=%d status=%d body=%s", tc.count, response.Code, response.Body.String())
			}
		})
	}
}

func TestLibraryCatalogFailsClosedWithoutOwnerAndDoesNotUsePortalAPI(t *testing.T) {
	portalCalled := false
	portal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		portalCalled = true
		_, _ = w.Write([]byte(`{"materials":[{"id":"mock"}]}`))
	}))
	defer portal.Close()

	handler := newLibraryDownloadHandler(t, "", portal.URL)
	response := httptest.NewRecorder()
	handler.Router().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/library/materials", nil))
	if response.Code != http.StatusServiceUnavailable || portalCalled {
		t.Fatalf("status=%d portalCalled=%t body=%s", response.Code, portalCalled, response.Body.String())
	}
}

func TestLibraryCatalogRejectsPartialOrInvalidOwnerFacts(t *testing.T) {
	for _, data := range []map[string]any{
		{"release_id": "bad", "materials": []any{}, "material_count": 0, "download_starts": 0, "counting_since": "2026-08-11T00:00:00Z", "as_of": "2026-08-11T01:00:00Z"},
		{"release_id": nil, "materials": []map[string]any{{"id": "11111111-1111-4111-8111-111111111111", "type": "note", "subject": "数学", "title": "资料", "role": "编辑", "file_name": "a.pdf", "file_size": 1, "downloads": 0, "download_available": true}}, "material_count": 1, "download_starts": 0, "counting_since": "2026-08-11T00:00:00Z", "as_of": "2026-08-11T01:00:00Z"},
	} {
		owner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{"data": data, "request_id": "req_owner"})
		}))
		handler := newLibraryDownloadHandler(t, owner.URL, "http://portal-api.invalid")
		response := httptest.NewRecorder()
		handler.Router().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/library/materials", nil))
		owner.Close()
		if response.Code != http.StatusServiceUnavailable {
			t.Fatalf("invalid owner fact status=%d body=%s", response.Code, response.Body.String())
		}
	}
}
