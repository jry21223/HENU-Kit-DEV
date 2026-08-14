package library

import "testing"

func TestReviewedManifestAcceptsCanonicalProvenanceFields(t *testing.T) {
	raw := []byte(`{
  "version": 1,
  "generatedAt": "2026-08-14T00:00:00Z",
  "subjects": [{
    "name": "高等数学",
    "note": "受审资料",
    "assets": [{
      "subject": "高等数学",
      "role": "复习讲义",
      "title": "高等数学复习讲义.pdf",
      "publicPath": "高等数学/高等数学复习讲义.pdf",
      "bytes": 123,
      "sha256": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
      "attribution": {"authors": ["作者"], "collectors": ["整理者"]},
      "college": "计算机与信息工程学院",
      "containsPersonalInfo": false,
      "licenseStatus": "cleared",
      "reviewStatus": "reviewed",
      "sourceNote": "公开资料",
      "sourceType": "repository",
      "uncertainty": "none",
      "year": "2026"
    }]
  }]
}`)
	var manifest reviewedManifest
	if err := decodeSingleJSON(raw, &manifest); err != nil {
		t.Fatalf("canonical reviewed manifest was rejected: %v", err)
	}
	if len(manifest.Subjects) != 1 || len(manifest.Subjects[0].Assets) != 1 {
		t.Fatalf("decoded manifest shape = %#v", manifest)
	}
}
