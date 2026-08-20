package database

import "testing"

func TestEscapeLike(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value string
		want  string
	}{
		{"plain text is unchanged", "alice@example.edu", "alice@example.edu"},
		{"percent loses its wildcard meaning", "%", `\%`},
		{"underscore loses its wildcard meaning", "_", `\_`},
		{"backslash is escaped", `a\b`, `a\\b`},
		{"mixed", `100%_off\`, `100\%\_off\\`},
		{"empty", "", ""},
		// The backslash must be escaped before the others, or the escapes this
		// function adds would themselves be escaped and the metacharacters would
		// survive.
		{"escape ordering", `\%`, `\\\%`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := EscapeLike(tc.value); got != tc.want {
				t.Errorf("EscapeLike(%q) = %q, want %q", tc.value, got, tc.want)
			}
		})
	}
}

func TestLikeContains(t *testing.T) {
	if got := LikeContains("alice"); got != "%alice%" {
		t.Errorf("LikeContains() = %q", got)
	}
	// A term that is nothing but wildcards must not become a match-everything
	// pattern — that is the bug this exists to prevent.
	if got := LikeContains("%"); got != `%\%%` {
		t.Errorf("LikeContains(%%) = %q, want the wildcard escaped", got)
	}
}
