package career

import (
	"net/http"
	"strings"
	"testing"
)

func TestNewHTTPDigestSenderRejectsPublishedPlaceholderSecrets(t *testing.T) {
	for _, secret := range []string{
		"replace-career-digest-secret-32bytes-min!!",
		"career-example-secret-32bytes-minimum!!",
		"change-me-career-digest-secret-32bytes!!",
		"career-digest-test-only-secret-32bytes!!",
		"local-career-digest-secret-32bytes-only!",
	} {
		if _, err := NewHTTPDigestSender("http://platform-core:8081", "career-opportunities", secret, "active", http.DefaultClient); err == nil {
			t.Fatalf("accepted placeholder digest secret %q", secret)
		}
	}
	if _, err := NewHTTPDigestSender("http://platform-core:8081", "career-opportunities", strings.Repeat("s", 40), "active", http.DefaultClient); err != nil {
		t.Fatalf("rejected strong digest secret: %v", err)
	}
}
