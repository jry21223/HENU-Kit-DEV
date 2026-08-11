package library

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMaterialJSONExposesOwnerDownloadCapabilityNotStorageKey(t *testing.T) {
	encoded, err := json.Marshal(Material{ID: "11111111-1111-4111-8111-111111111111", Price: 0, DownloadAvailable: true, FilePath: "releases/internal/object.pdf"})
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	if strings.Contains(text, "filePath") || strings.Contains(text, "releases/internal") {
		t.Fatalf("storage key leaked: %s", text)
	}
	if !strings.Contains(text, `"downloadAvailable":true`) {
		t.Fatalf("owner download capability missing: %s", text)
	}
}

func TestPublicDownloadAvailabilityRequiresFreeStoredMaterial(t *testing.T) {
	stored := sql.NullString{String: "releases/example/object.pdf", Valid: true}
	if !publicDownloadAvailable("free", stored) {
		t.Fatal("free stored material should expose the owner download action")
	}
	if publicDownloadAvailable("paid", stored) || publicDownloadAvailable("free", sql.NullString{}) {
		t.Fatal("paid or unstored material must not expose the owner download action")
	}
}

func TestMaterialPublicContractStaysAlignedAcrossOpenAPIAndPortal(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", "..", "..", ".."))
	for _, check := range []struct {
		path string
		want string
	}{
		{filepath.Join(root, "packages/api-contracts/openapi/portal-api.yaml"), "downloadAvailable"},
		{filepath.Join(root, "apps/portal/src/lib/api/types.ts"), "downloadAvailable: boolean"},
	} {
		content, err := os.ReadFile(check.path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(content), check.want) {
			t.Fatalf("%s is missing %q", check.path, check.want)
		}
		if strings.Contains(string(content), "filePath") {
			t.Fatalf("%s exposes the retired storage field", check.path)
		}
	}
}
