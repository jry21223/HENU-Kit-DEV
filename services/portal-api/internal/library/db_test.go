package library

import "testing"

// The importer records storage_key as a repository-relative path, and the
// Portal resolves the value against the site origin. A bare key would resolve
// to the site root, miss /materials/ entirely, and 404 on every download.
func TestMaterialFileURL(t *testing.T) {
	for _, tc := range []struct {
		name string
		key  string
		want string
	}{
		{
			name: "repository relative key resolves to the mirror",
			key:  "高等数学A（二）/复习讲义/讲义.pdf",
			want: "/materials/高等数学A（二）/复习讲义/讲义.pdf",
		},
		{
			name: "absolute path is left alone",
			key:  "/uploads/materials/legacy.pdf",
			want: "/uploads/materials/legacy.pdf",
		},
		{
			name: "external URL is left alone",
			key:  "https://cdn.example.com/a.pdf",
			want: "https://cdn.example.com/a.pdf",
		},
		{
			// No file means the Portal must offer no download rather than a link
			// that 404s.
			name: "empty key yields no download",
			key:  "",
			want: "",
		},
		{
			name: "blank key yields no download",
			key:  "   ",
			want: "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := materialFileURL(tc.key); got != tc.want {
				t.Errorf("materialFileURL(%q) = %q, want %q", tc.key, got, tc.want)
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
