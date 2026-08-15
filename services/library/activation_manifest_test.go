package library

import (
	"strings"
	"testing"
)

// Contract tests for the fail-closed publication provenance boundary. These
// pin the canonical values so policy drift in either HENU-Final-Review or HENU
// Kit is caught at test time. Sources: HENU-Final-Review PUBLICATION_POLICY.md,
// docs/manifest.md, and scripts/validate-materials.mjs (APPROVED_CONTACT_EXCEPTIONS).
func TestPublicationProvenancePolicyCanonicalValues(t *testing.T) {
	if !publishableReviewStatuses["verified"] || !publishableReviewStatuses["basic-reviewed"] || !publishableReviewStatuses["community_review"] {
		t.Fatalf("canonical publishable reviewStatuses changed: %#v", publishableReviewStatuses)
	}
	if len(publishableReviewStatuses) != 3 {
		t.Fatalf("publishable reviewStatus allowlist = %#v, want exactly verified/basic-reviewed/community_review", publishableReviewStatuses)
	}
	if !publishableLicenseStatuses["learning-reference"] || !publishableLicenseStatuses["public_review_only"] || !publishableLicenseStatuses["public-review-only"] {
		t.Fatalf("canonical publishable licenseStatuses changed: %#v", publishableLicenseStatuses)
	}
	if len(publishableLicenseStatuses) != 3 {
		t.Fatalf("publishable licenseStatus allowlist = %#v, want exactly learning-reference/public_review_only/public-review-only", publishableLicenseStatuses)
	}
	for _, uncertainty := range []string{"source_uncertain", "year_uncertain", "course_uncertain", "public_boundary_uncertain"} {
		if !reviewOnlyUncertainties[uncertainty] {
			t.Fatalf("review-only uncertainty %q dropped from the policy", uncertainty)
		}
	}
	if len(reviewOnlyUncertainties) != 4 {
		t.Fatalf("review-only uncertainty set = %#v, want the four REVIEW_ONLY_UNCERTAINTIES", reviewOnlyUncertainties)
	}
	if len(approvedTeacherSharedExceptions) != 6 {
		t.Fatalf("teacher_shared_exception whitelist has %d entries, want 6 (upstream APPROVED_CONTACT_EXCEPTIONS)", len(approvedTeacherSharedExceptions))
	}
}

func TestPublicationProvenanceViolation(t *testing.T) {
	clean := manifestAsset{Subject: "数学", Role: "讲义", Title: "讲义.pdf", PublicPath: "note.pdf", SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		ReviewStatus: "basic-reviewed", LicenseStatus: "learning-reference"}
	if violation := provenanceViolation(clean); violation != "" {
		t.Fatalf("clean asset flagged: %s", violation)
	}
	missing := manifestAsset{Subject: "数学", Role: "讲义", Title: "讲义.pdf", PublicPath: "note.pdf", SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
	if violation := provenanceViolation(missing); violation != "" {
		t.Fatalf("legacy asset without provenance fields flagged: %s", violation)
	}
	for _, tc := range []struct {
		name     string
		asset    manifestAsset
		contains string
	}{
		{"personal info", manifestAsset{PublicPath: "p.pdf", ContainsPersonalInfo: true}, "containsPersonalInfo"},
		{"unreviewed", manifestAsset{PublicPath: "p.pdf", ReviewStatus: "needs_review"}, "reviewStatus"},
		{"maintainer review", manifestAsset{PublicPath: "p.pdf", ReviewStatus: "待维护者复核"}, "reviewStatus"},
		{"unknown review", manifestAsset{PublicPath: "p.pdf", ReviewStatus: "made-up"}, "reviewStatus"},
		{"unknown license", manifestAsset{PublicPath: "p.pdf", LicenseStatus: "made-up"}, "licenseStatus"},
		{"non-allowlisted license", manifestAsset{PublicPath: "p.pdf", LicenseStatus: "贡献者自有学习笔记，提交后可按仓库公开资料协议共享。"}, "licenseStatus"},
		{"exception not whitelisted", manifestAsset{PublicPath: "p.pdf", LicenseStatus: "teacher_shared_exception", SHA256: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}, "teacher_shared_exception"},
		{"review-only uncertainty", manifestAsset{PublicPath: "p.pdf", Uncertainty: "source_uncertain"}, "uncertainty"},
		{"year uncertainty", manifestAsset{PublicPath: "p.pdf", Uncertainty: "year_uncertain"}, "uncertainty"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			violation := provenanceViolation(tc.asset)
			if violation == "" || !strings.Contains(violation, tc.contains) {
				t.Fatalf("violation=%q, want substring %q", violation, tc.contains)
			}
		})
	}
	approved := manifestAsset{PublicPath: "思想道德与法治/复习讲义/思想道德与法治_复习讲义_2025年冬最新考试重点.pdf",
		LicenseStatus: "teacher_shared_exception",
		SHA256:        "bfda62a15cfefb53c1413a244a4ff9f95e11a9fc959032f4ebff83adc1b8530c"}
	if violation := provenanceViolation(approved); violation != "" {
		t.Fatalf("approved teacher_shared_exception flagged: %s", violation)
	}
}

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
