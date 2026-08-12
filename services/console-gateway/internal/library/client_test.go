package library

import "testing"

func TestNewAcceptsLibraryComposeService(t *testing.T) {
	client, err := New(
		"http://library:8095",
		"console-gateway-library",
		"0123456789abcdef0123456789abcdef",
		"library-active",
		nil,
	)
	if err != nil {
		t.Fatalf("New() rejected the production Library service URL: %v", err)
	}
	if client == nil {
		t.Fatal("New() returned a nil client")
	}
}

func TestNewRejectsUntrustedHTTPService(t *testing.T) {
	client, err := New(
		"http://attacker.internal:8095",
		"console-gateway-library",
		"0123456789abcdef0123456789abcdef",
		"library-active",
		nil,
	)
	if err == nil || client != nil {
		t.Fatalf("New() = (%v, %v), want an untrusted HTTP service rejection", client, err)
	}
}
