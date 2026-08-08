package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateLoopbackAddress(t *testing.T) {
	for _, address := range []string{"127.0.0.1:10087", "[::1]:10087", "localhost:10087"} {
		if err := validateLoopbackAddress(address); err != nil {
			t.Fatalf("loopback address %q rejected: %v", address, err)
		}
	}
	for _, address := range []string{"0.0.0.0:10087", "192.0.2.1:10087", ":10087", "127.0.0.1"} {
		if err := validateLoopbackAddress(address); err == nil {
			t.Fatalf("non-loopback or invalid address %q accepted", address)
		}
	}
}

func TestLoadSecretRequiresPrivateRegularFile(t *testing.T) {
	dir := t.TempDir()
	secretPath := filepath.Join(dir, "secret")
	secret := []byte("0123456789abcdef0123456789abcdef\n")
	if err := os.WriteFile(secretPath, secret, 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadSecret(secretPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(loaded) != "0123456789abcdef0123456789abcdef" {
		t.Fatalf("loaded secret = %q", loaded)
	}
	if err := os.Chmod(secretPath, 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := loadSecret(secretPath); err == nil {
		t.Fatal("group-readable secret was accepted")
	}
}

func TestLoadSecretRejectsSymbolicLink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	link := filepath.Join(dir, "secret-link")
	if err := os.WriteFile(target, []byte("0123456789abcdef0123456789abcdef"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := loadSecret(link); err == nil {
		t.Fatal("symbolic-link secret was accepted")
	}
}

func TestLoadMaterialsConfigDoesNotReadGenericQueueCapacity(t *testing.T) {
	t.Setenv("HENUKIT_WEBHOOK_STATE_DIR", "/var/lib/henukit-materials-webhook")
	t.Setenv("HENUKIT_WEBHOOK_MAX_QUEUE", "not-a-number")
	config, err := loadMaterialsConfig()
	if err != nil {
		t.Fatal(err)
	}
	if config.StateDir != "/var/lib/henukit-materials-webhook" {
		t.Fatalf("materials state directory = %q", config.StateDir)
	}
}

func TestMaterialsPreparationCommandIsFixed(t *testing.T) {
	if materialsPreparationCommand != "/usr/local/libexec/henukit/henukit-materials-prepare" {
		t.Fatalf("materials preparation command = %q", materialsPreparationCommand)
	}
}
