package tests

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"gopkg.in/yaml.v3"

	library "henukit.dev/library"
)

// The Portal home page's library block needs only how many materials of each
// type are published (#555), so this read counts the active release without
// listing it, and answers an explicit all-zero success when none is active.
func TestPublicMaterialTypeCountsCountTheActiveReleaseWithoutListingIt(t *testing.T) {
	store := &activationStore{objects: map[string]library.DownloadObjectState{}}
	server, pool := newLibraryDownloadServer(t, store)
	defer server.Close()
	defer pool.Close()
	clearPublicCatalog(t, pool)
	contract := typeCountsContract(t)

	expect := func(releaseID string, total int64, want map[string]int64) map[string]int64 {
		t.Helper()
		data, counts := readTypeCounts(t, server.URL, contract)
		if !slices.Equal(sortedKeys(counts), sortedCopy(contract.Properties["type_counts"].Required)) {
			t.Fatalf("type_counts names %v, want every contract type %v", sortedKeys(counts), contract.Properties["type_counts"].Required)
		}
		for materialType, count := range counts {
			if count != want[materialType] {
				t.Fatalf("type_counts[%s] = %d, want %d (all: %v)", materialType, count, want[materialType], counts)
			}
		}
		var materialCount int64
		if err := json.Unmarshal(data["material_count"], &materialCount); err != nil || materialCount != total {
			t.Fatalf("material_count = %s (%v), want %d", data["material_count"], err, total)
		}
		var release *string
		if err := json.Unmarshal(data["release_id"], &release); err != nil || (releaseID == "") != (release == nil) || (release != nil && *release != releaseID) {
			t.Fatalf("release_id = %s (%v), want %q", data["release_id"], err, releaseID)
		}
		var asOf string
		if err := json.Unmarshal(data["as_of"], &asOf); err != nil {
			t.Fatal(err)
		}
		if _, err := time.Parse(time.RFC3339, asOf); err != nil {
			t.Fatalf("as_of = %q: %v", asOf, err)
		}
		return counts
	}

	expect("", 0, map[string]int64{})

	bundle := activationBundle(t, []manifestAsset{
		{Subject: "数学", Role: "复习讲义", Title: "讲义一.pdf", PublicPath: "handout-1.pdf", Body: "handout one"},
		{Subject: "数学", Role: "复习讲义", Title: "讲义二.pdf", PublicPath: "handout-2.pdf", Body: "handout two"},
		{Subject: "数学", Role: "往年真题", Title: "真题.pdf", PublicPath: "exam.pdf", Body: "exam"},
	}, store)
	if _, err := library.ActivatePublicRelease(context.Background(), pool, store, bundle, time.Now); err != nil {
		t.Fatal(err)
	}
	// Rows the catalog does not list must not be counted either.
	insertSnapshotRow(t, pool, bundle.ReleaseID, "33333333-3333-4333-8333-333333333331", "exam", "withdrawn", "public_free")
	insertSnapshotRow(t, pool, bundle.ReleaseID, "33333333-3333-4333-8333-333333333332", "handout", "published", "authenticated")
	counts := expect(bundle.ReleaseID, 3, map[string]int64{"handout": 2, "exam": 1})

	// The home page's numbers must match the catalog the /library page lists.
	catalogResponse := sendDownload(t, server.URL, "GET", "/api/v1/public-materials", nil)
	var catalog struct {
		Data struct {
			Materials []struct {
				Type string `json:"type"`
			} `json:"materials"`
		} `json:"data"`
	}
	if err := json.NewDecoder(catalogResponse.Body).Decode(&catalog); err != nil {
		t.Fatal(err)
	}
	catalogResponse.Body.Close()
	listed := map[string]int64{}
	for _, material := range catalog.Data.Materials {
		listed[material.Type]++
	}
	for materialType, count := range counts {
		if listed[materialType] != count {
			t.Fatalf("type_counts[%s] = %d but the catalog lists %d", materialType, count, listed[materialType])
		}
	}

	clearPublicCatalog(t, pool)
	expect("", 0, map[string]int64{})
}

// The type column still admits the legacy mock, path and lab types. The
// contract cannot name them, so a release holding one fails closed instead of
// being counted short.
func TestPublicMaterialTypeCountsFailClosedOnATypeTheContractCannotName(t *testing.T) {
	store := &activationStore{objects: map[string]library.DownloadObjectState{}}
	server, pool := newLibraryDownloadServer(t, store)
	defer server.Close()
	defer pool.Close()
	clearPublicCatalog(t, pool)
	defer clearPublicCatalog(t, pool)

	bundle := activationBundle(t, []manifestAsset{
		{Subject: "数学", Role: "复习讲义", Title: "讲义.pdf", PublicPath: "legacy-handout.pdf", Body: "legacy handout"},
	}, store)
	if _, err := library.ActivatePublicRelease(context.Background(), pool, store, bundle, time.Now); err != nil {
		t.Fatal(err)
	}
	insertSnapshotRow(t, pool, bundle.ReleaseID, "33333333-3333-4333-8333-333333333333", "mock", "published", "public_free")

	response := sendDownload(t, server.URL, "GET", "/api/v1/public-materials/type-counts", nil)
	defer response.Body.Close()
	if response.StatusCode != 503 {
		t.Fatalf("type counts with a legacy type status=%d, want 503", response.StatusCode)
	}
}

type typeCountsSchema struct {
	Required   []string                    `yaml:"required"`
	Properties map[string]typeCountsSchema `yaml:"properties"`
}

// typeCountsContract reads PublicMaterialTypeCounts from library.yaml, so the
// response's field names and type list are checked against the contract
// rather than a second hand-written copy.
func typeCountsContract(t *testing.T) typeCountsSchema {
	t.Helper()
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate type counts test source")
	}
	contents, err := os.ReadFile(filepath.Join(filepath.Dir(sourceFile), "../../../packages/api-contracts/openapi/library.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		Components struct {
			Schemas map[string]typeCountsSchema `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(contents, &document); err != nil {
		t.Fatal(err)
	}
	schema, found := document.Components.Schemas["PublicMaterialTypeCounts"]
	if !found || len(schema.Properties["type_counts"].Required) == 0 {
		t.Fatal("library.yaml has no PublicMaterialTypeCounts type list")
	}
	return schema
}

func readTypeCounts(t *testing.T, baseURL string, contract typeCountsSchema) (map[string]json.RawMessage, map[string]int64) {
	t.Helper()
	response := sendDownload(t, baseURL, "GET", "/api/v1/public-materials/type-counts", nil)
	defer response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatalf("type counts status=%d", response.StatusCode)
	}
	var payload struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(sortedKeys(payload.Data), sortedCopy(contract.Required)) {
		t.Fatalf("type counts fields = %v, contract requires %v", sortedKeys(payload.Data), contract.Required)
	}
	counts := map[string]int64{}
	if err := json.Unmarshal(payload.Data["type_counts"], &counts); err != nil {
		t.Fatal(err)
	}
	return payload.Data, counts
}

func insertSnapshotRow(t *testing.T, pool *pgxpool.Pool, releaseID, materialID, materialType, status, accessLevel string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO library_public_material_snapshots
			(release_id,material_id,title,file_name,access_level,status,object_key,object_version_id,sha256,byte_size,subject,role,material_type,public_path)
		VALUES ($1,$2,'额外资料','extra.pdf',$3,$4,$5,'extra-version',$6,1,'数学','复习讲义',$7,$8)`,
		releaseID, materialID, accessLevel, status, "releases/"+releaseID+"/extra-"+materialID,
		strings.Repeat("a", 64), materialType, "extra/"+materialID+".pdf"); err != nil {
		t.Fatal(err)
	}
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
