package librarydownload

import "testing"

func TestPublicMaterialTypesMatchCanonicalPublishedManifestRoles(t *testing.T) {
	base := PublicMaterial{
		ID:                "11111111-1111-4111-8111-111111111111",
		Subject:           "软件工程",
		Title:             "公开资料",
		Role:              "复习讲义",
		FileName:          "软件工程_复习讲义_公开资料.pdf",
		FileSize:          1,
		DownloadAvailable: true,
	}
	for _, materialType := range []string{"handout", "exam", "slides", "exercise", "answer", "note", "textbook"} {
		material := base
		material.Type = materialType
		if !validPublicMaterial(material) {
			t.Fatalf("canonical material type %q was rejected", materialType)
		}
	}
	for _, materialType := range []string{"mock", "path", "lab", "courseware", "electronic_textbook", ""} {
		material := base
		material.Type = materialType
		if validPublicMaterial(material) {
			t.Fatalf("unsupported material type %q was accepted", materialType)
		}
	}
}
