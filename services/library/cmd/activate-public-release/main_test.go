package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

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

func TestActivationOutcomeSurfacesTheLibraryCause(t *testing.T) {
	cause := errors.New("public release object verification failed for 线性代数/电子版教材/x.pdf")
	message, failed := activationOutcome(io.Discard, library.PublicReleaseActivationResult{}, cause)
	if !failed {
		t.Fatal("activationOutcome() reported success, want a failure")
	}
	if !strings.HasPrefix(message, "Library public release activation failed: ") {
		t.Fatalf("activationOutcome() = %q, want the stable operator-facing prefix", message)
	}
	if !strings.Contains(message, cause.Error()) {
		t.Fatalf("activationOutcome() = %q, want it to include %q", message, cause.Error())
	}
}

func TestActivationOutcomeWithholdsInfrastructureDetail(t *testing.T) {
	secrets := []string{
		"172.19.0.2", "5432", "henukit", "library",
		"library_public_releases_pkey", "duplicate key value", "release_id",
		"Key (release_id) already exists.", "dial", "connection refused",
	}
	for name, cause := range map[string]error{
		"PgError": &pgconn.PgError{
			Code: "23505", Message: "duplicate key value violates unique constraint",
			ConstraintName: "library_public_releases_pkey", Detail: "Key (release_id) already exists.",
		},
		"ConnectError": &pgconn.ConnectError{
			Config: &pgconn.Config{User: "henukit", Database: "library", Host: "172.19.0.2", Port: 5432},
		},
		"OpError": &net.OpError{
			Op: "dial", Net: "tcp",
			Addr: &net.TCPAddr{IP: net.ParseIP("172.19.0.2"), Port: 5432},
			Err:  errors.New("connection refused"),
		},
		"wrapped":      fmt.Errorf("commit activation: %w", &pgconn.PgError{Code: "23505", ConstraintName: "library_public_releases_pkey"}),
		"ScanArgError": pgx.ScanArgError{ColumnIndex: 3, FieldName: "release_id", Err: errors.New("cannot scan")},
	} {
		t.Run(name, func(t *testing.T) {
			message, failed := activationOutcome(io.Discard, library.PublicReleaseActivationResult{}, cause)
			if !failed {
				t.Fatal("activationOutcome() reported success, want a failure")
			}
			if !strings.HasPrefix(message, "Library public release activation failed: "+databaseCause) {
				t.Fatalf("activationOutcome() = %q, want the fixed database classification", message)
			}
			for _, secret := range secrets {
				if strings.Contains(message, secret) {
					t.Fatalf("activationOutcome() = %q, must not disclose %q", message, secret)
				}
			}
		})
	}
}

func TestActivationOutcomeEncodesSuccessAndReportsNoFailure(t *testing.T) {
	var encoded strings.Builder
	if message, failed := activationOutcome(&encoded, library.PublicReleaseActivationResult{ReleaseID: "abc"}, nil); failed {
		t.Fatalf("activationOutcome() = %q, want no failure on success", message)
	}
	if !strings.Contains(encoded.String(), "abc") {
		t.Fatalf("activationOutcome() wrote %q, want the encoded result", encoded.String())
	}
}

func TestActivationOutcomeKeepsTheSQLStateButNotItsDetail(t *testing.T) {
	cause := &pgconn.PgError{
		Code: "23505", Message: "duplicate key value violates unique constraint",
		ConstraintName: "library_public_releases_pkey", Detail: "Key (release_id) already exists.",
	}
	message, failed := activationOutcome(io.Discard, library.PublicReleaseActivationResult{}, cause)
	if !failed {
		t.Fatal("activationOutcome() reported success, want a failure")
	}
	if !strings.Contains(message, "SQLSTATE 23505") {
		t.Fatalf("activationOutcome() = %q, want the SQLSTATE so database failures stay distinguishable", message)
	}
	for _, detail := range []string{"library_public_releases_pkey", "duplicate key value", "Key (release_id) already exists."} {
		if strings.Contains(message, detail) {
			t.Fatalf("activationOutcome() = %q, must not disclose %q", message, detail)
		}
	}
}
