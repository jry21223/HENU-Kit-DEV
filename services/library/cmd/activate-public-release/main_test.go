package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	library "henukit.dev/library"
)

func TestReadBundleAcceptsOnlyOneRegularStrictJSONDocument(t *testing.T) {
	bundle := library.PublicReleaseActivation{
		Version:      1,
		ReleaseID:    strings.Repeat("a", 40) + "-" + strings.Repeat("b", 16),
		ManifestJSON: []byte(`{"version":1,"subjects":[]}`), SealedReceiptJSON: []byte(`{"version":1}`),
		Derived: library.PublicReleaseDerivedArtifacts{ReleaseID: strings.Repeat("a", 40) + "-" + strings.Repeat("b", 16), SlidesSHA256: strings.Repeat("c", 64), IndexSHA256: strings.Repeat("d", 64)},
		Objects: []library.PublicReleaseObject{},
	}
	encoded, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	valid := filepath.Join(directory, "bundle.json")
	if err := os.WriteFile(valid, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	actual, err := readBundle(valid)
	if err != nil || actual.ReleaseID != bundle.ReleaseID {
		t.Fatalf("readBundle() = %#v, %v", actual, err)
	}

	unknown := filepath.Join(directory, "unknown.json")
	if err := os.WriteFile(unknown, append(encoded[:len(encoded)-1], []byte(`,"endpoint":"internal"}`)...), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readBundle(unknown); err == nil {
		t.Fatal("bundle reader accepted an unknown field")
	}

	trailing := filepath.Join(directory, "trailing.json")
	if err := os.WriteFile(trailing, append(encoded, []byte(` {}`)...), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readBundle(trailing); err == nil {
		t.Fatal("bundle reader accepted trailing JSON")
	}

	symlink := filepath.Join(directory, "bundle-link.json")
	if err := os.Symlink(valid, symlink); err != nil {
		t.Fatal(err)
	}
	if _, err := readBundle(symlink); err == nil {
		t.Fatal("bundle reader followed a symlink")
	}
}

func TestUnsupportedEnvironmentRejectsCredentialAuthorityAndProxyOverrides(t *testing.T) {
	for _, name := range []string{"ALIBABA_CLOUD_ACCESS_KEY_ID", "LIBRARY_OSS_INTERNAL_ENDPOINT", "HTTPS_PROXY", "all_proxy"} {
		t.Run(name, func(t *testing.T) {
			for _, candidate := range []string{
				"ALIBABA_CLOUD_ACCESS_KEY_ID", "ALIBABA_CLOUD_ACCESS_KEY_SECRET", "ALIBABA_CLOUD_SECURITY_TOKEN",
				"LIBRARY_OSS_BUCKET", "LIBRARY_OSS_REGION", "LIBRARY_OSS_INTERNAL_ENDPOINT", "LIBRARY_OSS_PUBLIC_ENDPOINT",
				"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY", "http_proxy", "https_proxy", "all_proxy", "no_proxy",
			} {
				t.Setenv(candidate, "")
			}
			t.Setenv(name, "attacker-controlled")
			if actual := unsupportedEnvironment(); actual != name {
				t.Fatalf("unsupportedEnvironment()=%q, want %q", actual, name)
			}
		})
	}
}
