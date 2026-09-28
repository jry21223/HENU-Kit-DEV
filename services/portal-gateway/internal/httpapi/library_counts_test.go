package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"henukit.dev/portal-gateway/internal/librarydownload"
)

// The Portal home page's library block needs only per-type counts (#555): the
// Gateway reads them from the Library owner and never sends the catalog.
func ownerTypeCounts(releaseID any, materialCount int, typeCounts map[string]any) map[string]any {
	return map[string]any{
		"data": map[string]any{
			"release_id": releaseID, "material_count": materialCount,
			"type_counts": typeCounts, "as_of": "2026-08-11T01:00:00Z",
		},
		"request_id": "req_library_owner_counts",
	}
}

func allTypes(handout, exam, slides, exercise, answer, note, textbook int) map[string]any {
	return map[string]any{
		"handout": handout, "exam": exam, "slides": slides, "exercise": exercise,
		"answer": answer, "note": note, "textbook": textbook,
	}
}

const countsReleaseID = "0123456789abcdef0123456789abcdef01234567-0123456789abcdef"

func TestLibraryMaterialCountsReturnsOwnerTypeCountsWithoutTheCatalog(t *testing.T) {
	owner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/public-materials/type-counts" || r.URL.RawQuery != "" {
			t.Fatalf("owner request = %s %s", r.Method, r.URL.RequestURI())
		}
		if r.Header.Get("X-Service-Id") != "portal-gateway-library-download" || r.Header.Get("X-Signature") == "" {
			t.Fatal("owner counts request is missing service authentication")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ownerTypeCounts(countsReleaseID, 6, allTypes(2, 3, 0, 0, 0, 1, 0)))
	}))
	defer owner.Close()

	handler := newLibraryDownloadHandler(t, owner.URL, "http://portal-api.invalid")
	response := httptest.NewRecorder()
	handler.Router().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/library/material-counts", nil))

	if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("counts status = %d cache=%q: %s", response.Code, response.Header().Get("Cache-Control"), response.Body.String())
	}
	var body struct {
		Counts struct {
			ReleaseID     *string          `json:"releaseId"`
			MaterialCount int64            `json:"materialCount"`
			ByType        map[string]int64 `json:"byType"`
			AsOf          string           `json:"asOf"`
		} `json:"counts"`
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	want := map[string]int64{"handout": 2, "exam": 3, "slides": 0, "exercise": 0, "answer": 0, "note": 1, "textbook": 0}
	if body.Counts.ReleaseID == nil || *body.Counts.ReleaseID != countsReleaseID || body.Counts.MaterialCount != 6 || body.Counts.AsOf != "2026-08-11T01:00:00Z" || len(body.Counts.ByType) != len(want) {
		t.Fatalf("browser counts = %#v", body.Counts)
	}
	for materialType, count := range want {
		if body.Counts.ByType[materialType] != count {
			t.Fatalf("byType[%s] = %d, want %d", materialType, body.Counts.ByType[materialType], count)
		}
	}
	if !strings.HasPrefix(body.RequestID, "req_") || body.RequestID != response.Header().Get("X-Request-Id") {
		t.Fatalf("request_id = %q", body.RequestID)
	}
	if strings.Contains(response.Body.String(), "materials") {
		t.Fatalf("counts response carried the catalog: %s", response.Body.String())
	}
}

// No active release and an active release with nothing public are both honest
// zero successes, never an error.
func TestLibraryMaterialCountsPreservesExplicitEmptyOwnerSuccess(t *testing.T) {
	for _, releaseID := range []any{nil, countsReleaseID} {
		owner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(ownerTypeCounts(releaseID, 0, allTypes(0, 0, 0, 0, 0, 0, 0)))
		}))

		handler := newLibraryDownloadHandler(t, owner.URL, "http://portal-api.invalid")
		response := httptest.NewRecorder()
		handler.Router().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/library/material-counts", nil))
		owner.Close()
		var body struct {
			Counts struct {
				ReleaseID     *string          `json:"releaseId"`
				MaterialCount int64            `json:"materialCount"`
				ByType        map[string]int64 `json:"byType"`
			} `json:"counts"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || response.Code != http.StatusOK {
			t.Fatalf("empty counts for release %v = %d %s (%v)", releaseID, response.Code, response.Body.String(), err)
		}
		if (releaseID == nil) != (body.Counts.ReleaseID == nil) || body.Counts.MaterialCount != 0 || len(body.Counts.ByType) != 7 {
			t.Fatalf("empty counts for release %v = %#v", releaseID, body.Counts)
		}
	}
}

func TestLibraryMaterialCountsRejectsInvalidOwnerFacts(t *testing.T) {
	valid := ownerTypeCounts(countsReleaseID, 1, allTypes(1, 0, 0, 0, 0, 0, 0))
	validJSON, err := json.Marshal(valid)
	if err != nil {
		t.Fatal(err)
	}
	// A total read as 0 would still add up against all-zero type counts, so
	// these prove a null or missing total is rejected rather than defaulted.
	zeroJSON, err := json.Marshal(ownerTypeCounts(countsReleaseID, 0, allTypes(0, 0, 0, 0, 0, 0, 0)))
	if err != nil {
		t.Fatal(err)
	}
	withData := func(field string, value any) map[string]any {
		data := map[string]any{"release_id": countsReleaseID, "material_count": 1, "type_counts": allTypes(1, 0, 0, 0, 0, 0, 0), "as_of": "2026-08-11T01:00:00Z"}
		data[field] = value
		return map[string]any{"data": data, "request_id": "req_library_owner_counts"}
	}
	const quarter = int64(1) << 62
	for _, tc := range []struct {
		name   string
		status int
		body   any
		raw    string
	}{
		{name: "owner failure", status: http.StatusServiceUnavailable, body: map[string]any{"error": map[string]any{"code": "DEPENDENCY_UNAVAILABLE", "message": "down"}, "request_id": "req_owner_down"}},
		{name: "missing type", status: http.StatusOK, body: ownerTypeCounts(countsReleaseID, 1, map[string]any{"handout": 1, "exam": 0, "slides": 0, "exercise": 0, "answer": 0, "note": 0})},
		{name: "unknown type", status: http.StatusOK, body: ownerTypeCounts(countsReleaseID, 1, map[string]any{"handout": 1, "exam": 0, "slides": 0, "exercise": 0, "answer": 0, "note": 0, "textbook": 0, "mock": 0})},
		{name: "null count", status: http.StatusOK, body: ownerTypeCounts(countsReleaseID, 0, map[string]any{"handout": nil, "exam": 0, "slides": 0, "exercise": 0, "answer": 0, "note": 0, "textbook": 0})},
		{name: "null total", status: http.StatusOK, raw: strings.Replace(string(zeroJSON), `"material_count":0,`, `"material_count":null,`, 1)},
		{name: "missing total", status: http.StatusOK, raw: strings.Replace(string(zeroJSON), `"material_count":0,`, "", 1)},
		{name: "negative count", status: http.StatusOK, body: ownerTypeCounts(countsReleaseID, 0, allTypes(1, -1, 0, 0, 0, 0, 0))},
		{name: "counts do not add up", status: http.StatusOK, body: ownerTypeCounts(countsReleaseID, 5, allTypes(1, 1, 0, 0, 0, 0, 0))},
		{name: "total beyond the catalog bound", status: http.StatusOK, body: ownerTypeCounts(countsReleaseID, 501, allTypes(300, 201, 0, 0, 0, 0, 0))},
		{name: "counts that overflow to the total", status: http.StatusOK, body: ownerTypeCounts(countsReleaseID, 0, map[string]any{"handout": quarter, "exam": quarter, "slides": quarter, "exercise": quarter, "answer": 0, "note": 0, "textbook": 0})},
		{name: "materials without a release", status: http.StatusOK, body: ownerTypeCounts(nil, 1, allTypes(1, 0, 0, 0, 0, 0, 0))},
		{name: "malformed release", status: http.StatusOK, body: ownerTypeCounts("release-1", 1, allTypes(1, 0, 0, 0, 0, 0, 0))},
		{name: "unknown field", status: http.StatusOK, body: withData("materials", []any{})},
		{name: "missing request id", status: http.StatusOK, body: map[string]any{"data": valid["data"]}},
		{name: "malformed as_of", status: http.StatusOK, body: withData("as_of", "yesterday")},
		{name: "trailing data", status: http.StatusOK, raw: string(validJSON) + `{"request_id":"req_second"}`},
		{name: "body beyond the size bound", status: http.StatusOK, body: map[string]any{"data": valid["data"], "request_id": "req_" + strings.Repeat("r", 64<<10)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			owner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				if tc.raw != "" {
					_, _ = w.Write([]byte(tc.raw))
					return
				}
				_ = json.NewEncoder(w).Encode(tc.body)
			}))
			defer owner.Close()

			handler := newLibraryDownloadHandler(t, owner.URL, "http://portal-api.invalid")
			response := httptest.NewRecorder()
			handler.Router().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/library/material-counts", nil))
			if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "LIBRARY_TEMPORARILY_UNAVAILABLE") {
				t.Fatalf("%s: status = %d body = %s", tc.name, response.Code, response.Body.String())
			}
		})
	}
}

func TestLibraryMaterialCountsRejectsBrowserFiltersAndFailsClosedWithoutOwner(t *testing.T) {
	owner := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("owner contacted for a filtered counts request")
	}))
	defer owner.Close()
	handler := newLibraryDownloadHandler(t, owner.URL, "http://portal-api.invalid")
	filtered := httptest.NewRecorder()
	handler.Router().ServeHTTP(filtered, httptest.NewRequest(http.MethodGet, "/api/v1/library/material-counts?type=exam", nil))
	if filtered.Code != http.StatusBadRequest || !strings.Contains(filtered.Body.String(), "INVALID_REQUEST") {
		t.Fatalf("filtered counts = %d %s", filtered.Code, filtered.Body.String())
	}

	unconfigured := newLibraryDownloadHandler(t, "", "http://portal-api.invalid")
	response := httptest.NewRecorder()
	unconfigured.Router().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/library/material-counts", nil))
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "LIBRARY_TEMPORARILY_UNAVAILABLE") {
		t.Fatalf("unconfigured counts = %d %s", response.Code, response.Body.String())
	}
}

type countsContractSchema struct {
	Required   []string                        `yaml:"required"`
	Enum       []string                        `yaml:"enum"`
	Properties map[string]countsContractSchema `yaml:"properties"`
}

func countsContractSchemas(t *testing.T, name string) map[string]countsContractSchema {
	t.Helper()
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate library counts test source")
	}
	contents, err := os.ReadFile(filepath.Join(filepath.Dir(sourceFile), "../../../../packages/api-contracts/openapi", name))
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		Components struct {
			Schemas map[string]countsContractSchema `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(contents, &document); err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	return document.Components.Schemas
}

func sortedKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func sortedCopy(values []string) []string {
	sorted := slices.Clone(values)
	slices.Sort(sorted)
	return sorted
}

// Neither contract is generated from the other, and the owner, the Gateway
// and the catalog each hand-write their type list; this keeps all of them
// naming the same types, and the Gateway writing exactly its documented fields.
func TestLibraryMaterialCountsMatchTheirContracts(t *testing.T) {
	owner := countsContractSchemas(t, "library.yaml")
	portal := countsContractSchemas(t, "portal-gateway.yaml")
	types := sortedCopy(librarydownload.PublicMaterialTypes())
	for name, got := range map[string][]string{
		"library.yaml PublicCatalogMaterial.type":                       owner["PublicCatalogMaterial"].Properties["type"].Enum,
		"library.yaml PublicMaterialTypeCounts.type_counts":             owner["PublicMaterialTypeCounts"].Properties["type_counts"].Required,
		"library.yaml PublicMaterialTypeCounts.type_counts fields":      sortedKeys(owner["PublicMaterialTypeCounts"].Properties["type_counts"].Properties),
		"portal-gateway.yaml PublicLibraryMaterial.type":                portal["PublicLibraryMaterial"].Properties["type"].Enum,
		"portal-gateway.yaml PublicLibraryMaterialCounts.byType":        portal["PublicLibraryMaterialCounts"].Properties["byType"].Required,
		"portal-gateway.yaml PublicLibraryMaterialCounts.byType fields": sortedKeys(portal["PublicLibraryMaterialCounts"].Properties["byType"].Properties),
	} {
		if !slices.Equal(sortedCopy(got), types) {
			t.Fatalf("%s = %v, want %v", name, got, types)
		}
	}

	owner200 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(ownerTypeCounts(countsReleaseID, 1, allTypes(0, 1, 0, 0, 0, 0, 0)))
	}))
	defer owner200.Close()
	handler := newLibraryDownloadHandler(t, owner200.URL, "http://portal-api.invalid")
	response := httptest.NewRecorder()
	handler.Router().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/library/material-counts", nil))
	var body map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || response.Code != http.StatusOK {
		t.Fatalf("counts = %d %s (%v)", response.Code, response.Body.String(), err)
	}
	var counts map[string]json.RawMessage
	if err := json.Unmarshal(body["counts"], &counts); err != nil {
		t.Fatal(err)
	}
	var byType map[string]json.RawMessage
	if err := json.Unmarshal(counts["byType"], &byType); err != nil {
		t.Fatal(err)
	}
	for name, pair := range map[string][2][]string{
		"response":      {sortedKeys(body), portal["PublicLibraryMaterialCountsResponse"].Required},
		"counts":        {sortedKeys(counts), portal["PublicLibraryMaterialCounts"].Required},
		"counts.byType": {sortedKeys(byType), portal["PublicLibraryMaterialCounts"].Properties["byType"].Required},
	} {
		if !slices.Equal(pair[0], sortedCopy(pair[1])) {
			t.Fatalf("%s fields = %v, contract requires %v", name, pair[0], pair[1])
		}
	}
}
