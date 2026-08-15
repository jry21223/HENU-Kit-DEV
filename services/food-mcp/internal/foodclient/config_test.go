package foodclient

import (
	"strings"
	"testing"
)

const (
	validCreateSecret = "food-create-secret-with-at-least-32-bytes"
	validReadSecret   = "food-read-secret-with-at-least-32-bytes!!"
)

func TestNewClientRequiresCompleteFoodPostCredentialRings(t *testing.T) {
	_, err := NewClient(
		"http://food:8096",
		"portal-create", "", "create-key",
		"portal-read", validReadSecret, "read-key",
	)
	if err == nil || !strings.Contains(err.Error(), "create credential is incomplete") {
		t.Fatalf("NewClient error = %v, want incomplete create credential", err)
	}
}

func TestNewClientRejectsPublicPlaceholderSecrets(t *testing.T) {
	_, err := NewClient(
		"http://food:8096",
		"portal-create", "replace-food-post-create-secret-32bytes!!", "create-key",
		"portal-read", validReadSecret, "read-key",
	)
	if err == nil || !strings.Contains(err.Error(), "not deployment-safe") {
		t.Fatalf("NewClient error = %v, want deployment-safe rejection", err)
	}
}

func TestNewClientAcceptsExplicitNonPlaceholderSecrets(t *testing.T) {
	client, err := NewClient(
		"http://food:8096",
		"portal-create", validCreateSecret, "create-key",
		"portal-read", validReadSecret, "read-key",
	)
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}
	if client.createSigner == nil || client.readSigner == nil {
		t.Fatal("NewClient did not configure both Food Post signers")
	}
}
