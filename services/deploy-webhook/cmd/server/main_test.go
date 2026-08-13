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

func TestSafeSystemdCredentialMetadata(t *testing.T) {
	const credentialDirectory = "/run/credentials/henukit-materials-webhook.service"
	const credentialPath = credentialDirectory + "/webhook_secret"

	if !isSafeSystemdCredential(
		credentialPath,
		credentialDirectory,
		0o440,
		0,
		0,
	) {
		t.Fatal("root-owned systemd credential copy with mode 0440 was rejected")
	}

	for name, test := range map[string]struct {
		path string
		dir  string
		mode os.FileMode
		uid  uint32
		gid  uint32
	}{
		"ordinary path":                {"/tmp/webhook_secret", "/tmp", 0o440, 0, 0},
		"outside credential directory": {"/run/credentials/other.service/webhook_secret", credentialDirectory, 0o440, 0, 0},
		"non-root owner":               {credentialPath, credentialDirectory, 0o440, 1000, 0},
		"non-root group":               {credentialPath, credentialDirectory, 0o440, 0, 1000},
		"group writable":               {credentialPath, credentialDirectory, 0o460, 0, 0},
		"other readable":               {credentialPath, credentialDirectory, 0o444, 0, 0},
		"setuid":                       {credentialPath, credentialDirectory, 0o440 | os.ModeSetuid, 0, 0},
		"setgid":                       {credentialPath, credentialDirectory, 0o440 | os.ModeSetgid, 0, 0},
		"sticky":                       {credentialPath, credentialDirectory, 0o440 | os.ModeSticky, 0, 0},
	} {
		t.Run(name, func(t *testing.T) {
			if isSafeSystemdCredential(test.path, test.dir, test.mode, test.uid, test.gid) {
				t.Fatal("unsafe credential metadata was accepted")
			}
		})
	}
}

func TestLoadSecretAcceptsOnlySafeSystemdCredentialCopy(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root to exercise root-owned systemd credential metadata")
	}

	if err := os.MkdirAll("/run/credentials", 0o755); err != nil {
		t.Fatal(err)
	}
	credentialDirectory, err := os.MkdirTemp("/run/credentials", "henukit-load-secret-test-*.service")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(credentialDirectory)
	if err := os.Chmod(credentialDirectory, 0o550); err != nil {
		t.Fatal(err)
	}
	secretPath := filepath.Join(credentialDirectory, "webhook_secret")
	if err := os.WriteFile(secretPath, []byte("0123456789abcdef0123456789abcdef\n"), 0o440); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CREDENTIALS_DIRECTORY", credentialDirectory)

	loaded, err := loadSecret(secretPath)
	if err != nil {
		t.Fatalf("safe systemd credential copy rejected: %v", err)
	}
	if string(loaded) != "0123456789abcdef0123456789abcdef" {
		t.Fatalf("loaded secret = %q", loaded)
	}

	if err := os.Chmod(credentialDirectory, 0o570); err != nil {
		t.Fatal(err)
	}
	if _, err := loadSecret(secretPath); err == nil {
		t.Fatal("credential inside group-writable directory was accepted")
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

func TestMaterialsOrchestrationCommandIsFixed(t *testing.T) {
	if materialsOrchestrationCommand != "/usr/local/libexec/henukit/henukit-materials-orchestrate" {
		t.Fatalf("materials orchestration command = %q", materialsOrchestrationCommand)
	}
}
