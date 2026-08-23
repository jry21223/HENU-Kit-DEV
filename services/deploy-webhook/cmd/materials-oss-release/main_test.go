package main

import (
	"encoding/json"
	"strings"
	"testing"

	"henukit.dev/deploy-webhook/internal/materialsoss"
)

func TestReleaseAttestationOmitsPerObjectInventory(t *testing.T) {
	result := materialsoss.ReleaseResult{
		Version:         1,
		State:           "release_committed_not_activated",
		ReleaseID:       strings.Repeat("a", 40) + "-" + strings.Repeat("b", 16),
		ReceiptSHA256:   strings.Repeat("c", 64),
		ManifestSHA256:  strings.Repeat("d", 64),
		InventorySHA256: strings.Repeat("e", 64),
		TreeSHA256:      strings.Repeat("f", 64),
		AssetCount:      500,
		Assets: []materialsoss.ReleaseAsset{{
			PublicPath:      "course/example.pdf",
			SHA256:          strings.Repeat("1", 64),
			Bytes:           42,
			ObjectKey:       "releases/example/object.pdf",
			ObjectVersionID: "version-1",
		}},
		ReleaseCommitSHA256: strings.Repeat("2", 64),
	}

	encoded, err := json.Marshal(newReleaseAttestation(result))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), `"assets"`) || strings.Contains(string(encoded), "example.pdf") {
		t.Fatalf("attestation leaked per-object inventory: %s", encoded)
	}
	var got releaseAttestation
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatal(err)
	}
	if got != newReleaseAttestation(result) {
		t.Fatalf("attestation did not preserve the bounded release fields: got %#v", got)
	}
}
