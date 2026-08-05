package library

import (
	"database/sql"
	"testing"
)

func TestDownloadURL(t *testing.T) {
	for _, tc := range []struct {
		name string
		key  sql.NullString
		want string
	}{
		{
			// What the materials mirror writes: a repository-relative path.
			name: "repository relative key resolves to the mirror",
			key:  sql.NullString{String: "高等数学A（二）/复习讲义/讲义.pdf", Valid: true},
			want: "/materials/高等数学A（二）/复习讲义/讲义.pdf",
		},
		{
			name: "absolute path is left alone",
			key:  sql.NullString{String: "/uploads/materials/legacy.pdf", Valid: true},
			want: "/uploads/materials/legacy.pdf",
		},
		{
			name: "external URL is left alone",
			key:  sql.NullString{String: "https://cdn.example.com/a.pdf", Valid: true},
			want: "https://cdn.example.com/a.pdf",
		},
		{
			// No file means the Portal must offer no download rather than a
			// link that 404s.
			name: "null key yields no download",
			key:  sql.NullString{Valid: false},
			want: "",
		},
		{
			name: "blank key yields no download",
			key:  sql.NullString{String: "   ", Valid: true},
			want: "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := downloadURL(tc.key); got != tc.want {
				t.Errorf("downloadURL() = %q, want %q", got, tc.want)
			}
		})
	}
}

// Courseware is the bulk of the mirrored catalogue; filing it under a notes
// type would make the Library's type filter misleading.
func TestMapMaterialTypeCoversCourseware(t *testing.T) {
	for input, want := range map[string]string{
		"courseware":     "slides",
		"slides":         "slides",
		"past_exam":      "exam",
		"mock_paper":     "mock",
		"knowledge_note": "note",
		"lab_report":     "lab",
		"something_else": "note",
	} {
		if got := mapMaterialType(input); got != want {
			t.Errorf("mapMaterialType(%q) = %q, want %q", input, got, want)
		}
	}
}
