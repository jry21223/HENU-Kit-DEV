package food

import "testing"

func TestNewAcceptsProductionFoodComposeService(t *testing.T) {
	client, err := New(
		"http://food:8096",
		"console-gateway-food",
		"0123456789abcdef0123456789abcdef",
		"food-active",
		nil,
	)
	if err != nil {
		t.Fatalf("New() rejected the production Food service URL: %v", err)
	}
	if client == nil {
		t.Fatal("New() returned a nil client")
	}
}

func TestNewRejectsUntrustedHTTPService(t *testing.T) {
	client, err := New(
		"http://attacker.internal:8096",
		"console-gateway-food",
		"0123456789abcdef0123456789abcdef",
		"food-active",
		nil,
	)
	if err == nil || client != nil {
		t.Fatalf("New() = (%v, %v), want an untrusted HTTP service rejection", client, err)
	}
}
