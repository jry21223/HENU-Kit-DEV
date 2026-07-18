package audit

import "testing"

func TestIPPrefix(t *testing.T) {
	tests := map[string]string{
		"192.0.2.99":             "192.0.2.0/24",
		"2001:db8:abcd:1234::42": "2001:db8:abcd:1234::/64",
		"not-an-ip":              "",
	}
	for input, expected := range tests {
		if actual := ipPrefix(input); actual != expected {
			t.Fatalf("ipPrefix(%q): expected %q, got %q", input, expected, actual)
		}
	}
}
