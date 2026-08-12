package notice

import (
	"net/http"
	"testing"
)

func TestNewAcceptsOnlyTheNoticeComposeHostOverHTTP(t *testing.T) {
	t.Parallel()

	if _, err := New("http://notice:8094", "console-gateway-notice", "notice-gateway-secret-at-least-32-bytes", "notice-key", http.DefaultClient); err != nil {
		t.Fatalf("trusted Notice compose URL was rejected: %v", err)
	}
	if _, err := New("http://attacker.internal:8094", "console-gateway-notice", "notice-gateway-secret-at-least-32-bytes", "notice-key", http.DefaultClient); err == nil {
		t.Fatal("untrusted HTTP host was accepted")
	}
}
