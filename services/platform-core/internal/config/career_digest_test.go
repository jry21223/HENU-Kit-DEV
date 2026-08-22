package config

import "testing"

func TestCredentialPlaceholderRejectsPublicExampleValues(t *testing.T) {
	for _, value := range []string{
		"replace-career-digest-secret-32bytes-min!!",
		"career-example-secret-32bytes-minimum!!",
		"change-me-career-digest-secret-32bytes!!",
		"career-digest-test-only-secret-32bytes!!",
		"local-career-digest-secret-32bytes-only!",
	} {
		if !credentialPlaceholder(value) {
			t.Fatalf("placeholder %q was accepted", value)
		}
	}
	if credentialPlaceholder("90f4c8bf1d0241adb350cd370388783b948bc7f2") {
		t.Fatal("random secret was treated as a placeholder")
	}
}
