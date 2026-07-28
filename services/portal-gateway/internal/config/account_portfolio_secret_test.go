package config

import "testing"

func TestIsPlaceholderSecret(t *testing.T) {
	tests := []struct {
		secret string
		want   bool
	}{
		{secret: "replace-account-portfolio-client-secret-32bytes-min!!", want: true},
		{secret: "placeholder-with-at-least-32-characters", want: true},
		{secret: "correct-horse-battery-staple-with-entropy-123", want: false},
	}
	for _, test := range tests {
		if got := isPlaceholderSecret(test.secret); got != test.want {
			t.Fatalf("isPlaceholderSecret(%q) = %v, want %v", test.secret, got, test.want)
		}
	}
}
