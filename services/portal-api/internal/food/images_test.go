package food

import "testing"

func TestImagePath(t *testing.T) {
	if got := ImagePath("survey-01", 0); got != "/api/v1/food/posts/survey-01/images/0" {
		t.Errorf("ImagePath() = %q", got)
	}
	if got := ImagePath("survey-07", 2); got != "/api/v1/food/posts/survey-07/images/2" {
		t.Errorf("ImagePath() = %q", got)
	}
}

// The array literal is interpolated into SQL rather than bound, so a value
// carrying a quote, backslash, or brace must not be able to end the literal
// early and add elements of its own.
func TestPQStringArrayQuotesAndEscapes(t *testing.T) {
	for _, tc := range []struct {
		name   string
		values []string
		want   string
	}{
		{"plain", []string{"survey-01", "survey-02"}, `{"survey-01","survey-02"}`},
		{"single", []string{"survey-01"}, `{"survey-01"}`},
		{"empty", nil, `{}`},
		{"embedded quote", []string{`a"b`}, `{"a\"b"}`},
		{"embedded backslash", []string{`a\b`}, `{"a\\b"}`},
		{"literal terminator", []string{`x","y`}, `{"x\",\"y"}`},
		{"braces", []string{"{drop}"}, `{"{drop}"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := pqStringArray(tc.values); got != tc.want {
				t.Errorf("pqStringArray(%q) = %s, want %s", tc.values, got, tc.want)
			}
		})
	}
}
