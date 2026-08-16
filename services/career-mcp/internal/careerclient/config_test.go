package careerclient

import "testing"

func TestNewClientRejectsInvalidConfiguration(t *testing.T) {
	cases := []struct {
		name     string
		baseURL  string
		clientID string
		secret   string
		keyID    string
	}{
		{name: "empty url", baseURL: "", clientID: "c", secret: "secret-that-is-long-enough-32b", keyID: "k"},
		{name: "url with userinfo", baseURL: "http://user:pass@career.test", clientID: "c", secret: "secret-that-is-long-enough-32b", keyID: "k"},
		{name: "not http", baseURL: "file:///etc/passwd", clientID: "c", secret: "secret-that-is-long-enough-32b", keyID: "k"},
		{name: "missing client id", baseURL: "http://career.test", clientID: "", secret: "secret-that-is-long-enough-32b", keyID: "k"},
		{name: "short secret", baseURL: "http://career.test", clientID: "c", secret: "too-short", keyID: "k"},
		{name: "placeholder secret", baseURL: "http://career.test", clientID: "c", secret: "replace-career-service-secret-32bytes!!", keyID: "k"},
		{name: "missing key id", baseURL: "http://career.test", clientID: "c", secret: "secret-that-is-long-enough-32b", keyID: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewClient(tc.baseURL, tc.clientID, tc.secret, tc.keyID); err == nil {
				t.Fatal("invalid configuration accepted")
			}
		})
	}
}

func TestNewClientAcceptsValidConfiguration(t *testing.T) {
	client, err := NewClient("https://career.internal", "career-mcp", "a-real-secret-that-is-long-enough-32bytes", "active")
	if err != nil {
		t.Fatal(err)
	}
	if client.baseURL != "https://career.internal" {
		t.Fatalf("baseURL = %q", client.baseURL)
	}
}
