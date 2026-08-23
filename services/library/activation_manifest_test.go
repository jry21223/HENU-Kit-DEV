package library

import (
	"fmt"
	"strings"
	"testing"
)

// Contract tests for the fail-closed publication provenance boundary. These
// pin the enforced safety values so policy drift in either HENU-Final-Review
// or HENU Kit is caught at test time. Sources: HENU-Final-Review
// PUBLICATION_POLICY.md, docs/manifest.md, and scripts/validate-materials.mjs.
func TestPublicationProvenancePolicyCanonicalValues(t *testing.T) {
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

func TestManifestRoleContractMatchesHENUFinalReviewMain(t *testing.T) {
	// First-party source: jry21223/HENU-Final-Review main at
	// fcd9e86b60856188b81868e5c96f26a8720b18db, PUBLICATION_POLICY.md and
	// docs/manifest.md. Keep this table exact: content roles are not inferred
	// from substrings or file extensions.
	want := map[string]manifestRoleContract{
		"复习讲义":  {CanonicalRole: "复习讲义", MaterialType: "handout", Publishable: true},
		"往年真题":  {CanonicalRole: "往年真题", MaterialType: "exam", Publishable: true},
		"课件":    {CanonicalRole: "课件", MaterialType: "slides", Publishable: true},
		"题库练习":  {CanonicalRole: "题库练习", MaterialType: "exercise", Publishable: true},
		"答案解析":  {CanonicalRole: "答案解析", MaterialType: "answer", Publishable: true},
		"笔记总结":  {CanonicalRole: "笔记总结", MaterialType: "note", Publishable: true},
		"电子版教材": {CanonicalRole: "电子版教材", MaterialType: "textbook", Publishable: true},
		"待复核资料": {CanonicalRole: "待复核资料", Publishable: false},
	}
	if len(canonicalManifestRoles) != len(want) {
		t.Fatalf("canonical role count = %d, want %d: %#v", len(canonicalManifestRoles), len(want), canonicalManifestRoles)
	}
	for role, expected := range want {
		actual, ok := resolveManifestRole(role)
		if !ok || actual != expected {
			t.Fatalf("resolveManifestRole(%q) = %#v/%v, want %#v/true", role, actual, ok, expected)
		}
	}

	aliases := map[string]manifestRoleContract{
		"课件PPT":    want["课件"],
		"课件资料":     want["课件"],
		"课件资料包":    want["课件"],
		"待复核课件PPT": want["待复核资料"],
	}
	if len(legacyManifestRoleAliases) != len(aliases) {
		t.Fatalf("legacy role alias count = %d, want %d: %#v", len(legacyManifestRoleAliases), len(aliases), legacyManifestRoleAliases)
	}
	for role, expected := range aliases {
		actual, ok := resolveManifestRole(role)
		if !ok || actual != expected {
			t.Fatalf("resolveManifestRole(%q) = %#v/%v, want %#v/true", role, actual, ok, expected)
		}
	}

	for _, unsupported := range []string{"电子教材", "教材重点复习讲义", "讲义", "真题", "待复核-讲义", "mock"} {
		if actual, ok := resolveManifestRole(unsupported); ok {
			t.Fatalf("unsupported role %q resolved to %#v", unsupported, actual)
		}
	}
}

func TestOSSIdentityAndDownloadFilenameBoundaries(t *testing.T) {
	if !validOSSObjectKey(strings.Repeat("a", 1023)) {
		t.Fatal("a 1023-byte OSS Object key was rejected")
	}
	if validOSSObjectKey(strings.Repeat("a", 1024)) {
		t.Fatal("a 1024-byte OSS Object key was accepted")
	}
	if !validOSSObjectKey(strings.Repeat("资", 341)) {
		t.Fatal("a valid 1023-byte UTF-8 OSS Object key was rejected")
	}
	if validOSSObjectKey(strings.Repeat("资", 342)) {
		t.Fatal("an oversized UTF-8 OSS Object key was accepted by rune count")
	}
	for _, key := range []string{"", "/leading.pdf", `\\leading.pdf`, "line\nbreak.pdf"} {
		if validOSSObjectKey(key) {
			t.Fatalf("unsafe OSS Object key %q was accepted", key)
		}
	}

	if !safeObjectVersionID(strings.Repeat("v", 1024)) {
		t.Fatal("a bounded OSS VersionId was rejected")
	}
	for _, versionID := range []string{"", "null", " version-1", "version-1 ", "version\n1", strings.Repeat("v", 1025)} {
		if safeObjectVersionID(versionID) {
			t.Fatalf("unsafe OSS VersionId %q was accepted", versionID)
		}
	}

	for _, fileName := range []string{"高等数学_复习讲义_极限.pdf", "数据库系统_教材_概论.pdf"} {
		if !safeDownloadFileName(fileName) {
			t.Fatalf("safe download filename %q was rejected", fileName)
		}
	}
	for _, fileName := range []string{"", ".hidden.pdf", "..", "资料,副本.pdf", "资料:答案.pdf", "资料?.pdf", "资料*.pdf", "资料<1>.pdf", `资料\\1.pdf`, "资料/1.pdf", "资料\n1.pdf", strings.Repeat("资", 86) + ".pdf"} {
		if safeDownloadFileName(fileName) {
			t.Fatalf("unsafe download filename %q was accepted", fileName)
		}
	}
}

func TestPublicationProvenanceViolation(t *testing.T) {
	clean := manifestAsset{Subject: "数学", Role: "复习讲义", Title: "讲义.pdf", PublicPath: "note.pdf", SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		ReviewStatus: "basic-reviewed", LicenseStatus: "learning-reference"}
	if violation := provenanceViolation(clean); violation != "" {
		t.Fatalf("clean asset flagged: %s", violation)
	}
	missing := manifestAsset{Subject: "数学", Role: "复习讲义", Title: "讲义.pdf", PublicPath: "note.pdf", SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
	if violation := provenanceViolation(missing); violation != "" {
		t.Fatalf("legacy asset without provenance fields flagged: %s", violation)
	}
	for _, advisory := range []manifestAsset{
		{PublicPath: "note.pdf", Role: "笔记总结", ReviewStatus: "待维护者复核"},
		{PublicPath: "exam.pdf", Role: "往年真题", ReviewStatus: "needs_review"},
		{PublicPath: "note.pdf", Role: "笔记总结", LicenseStatus: "贡献者自有学习笔记，提交后可按仓库公开资料协议共享。"},
	} {
		if violation := provenanceViolation(advisory); violation != "" {
			t.Fatalf("upstream-publishable advisory metadata was rejected: %s", violation)
		}
	}
	for _, tc := range []struct {
		name     string
		asset    manifestAsset
		contains string
	}{
		{"personal info", manifestAsset{PublicPath: "p.pdf", ContainsPersonalInfo: true}, "containsPersonalInfo"},
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

func TestElectronicTextbookRequiresExactRedistributionAuthorization(t *testing.T) {
	// HENU-Final-Review PR #20 merged at
	// fcd9e86b60856188b81868e5c96f26a8720b18db with ten textbooks using this
	// exact verified redistribution contract.
	pr20 := manifestAsset{
		Subject: "高等数学A（二）", Role: "电子版教材", Title: "高等数学下册第八版.pdf",
		PublicPath: "高等数学A（二）/电子版教材/高等数学A（二）_教材_高等数学下册第八版.pdf",
		SHA256:     strings.Repeat("a", 64), ReviewStatus: "needs_review", LicenseStatus: "public-review-only",
		SourceType: "other", SourceNote: "公开渠道获取的课程教材电子版。",
	}
	if violation := provenanceViolation(pr20); !strings.Contains(violation, "reviewStatus must be verified") {
		t.Fatalf("PR #20 negative fixture violation = %q", violation)
	}
	pr20.ReviewStatus = "verified"
	if violation := provenanceViolation(pr20); !strings.Contains(violation, "authorized-redistribution") {
		t.Fatalf("public-review-only was treated as textbook authorization: %q", violation)
	}
	pr20.LicenseStatus = "public_review_only"
	if violation := provenanceViolation(pr20); !strings.Contains(violation, "authorized-redistribution") {
		t.Fatalf("public_review_only was treated as textbook authorization: %q", violation)
	}
	pr20.LicenseStatus = "authorized-redistribution"
	pr20.SourceNote = ""
	if violation := provenanceViolation(pr20); !strings.Contains(violation, "sourceNote is required") {
		t.Fatalf("textbook without source note violation = %q", violation)
	}
	pr20.SourceNote = "资料提供者确认允许公开再分发。"
	if violation := provenanceViolation(pr20); violation != "" {
		t.Fatalf("authorized textbook was rejected: %q", violation)
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

func TestReviewedManifestAcceptsDocumentedAndExistingYearShapes(t *testing.T) {
	for _, year := range []string{`"2026"`, `2026`, `null`} {
		t.Run(year, func(t *testing.T) {
			var manifest reviewedManifest
			if err := decodeSingleJSON(reviewedManifestWithYear(year), &manifest); err != nil {
				t.Fatalf("reviewed manifest year %s was rejected: %v", year, err)
			}
		})
	}
}

func TestReviewedManifestRejectsUnsupportedYearShapes(t *testing.T) {
	for _, year := range []string{`true`, `{}`, `[]`, `2026.5`, `2e3`} {
		t.Run(year, func(t *testing.T) {
			var manifest reviewedManifest
			if err := decodeSingleJSON(reviewedManifestWithYear(year), &manifest); err == nil {
				t.Fatalf("unsupported reviewed manifest year %s was accepted", year)
			}
		})
	}
}

func reviewedManifestWithYear(year string) []byte {
	return []byte(fmt.Sprintf(`{
  "version": 1,
  "generatedAt": "2026-08-23T00:00:00Z",
  "subjects": [{
    "name": "高等数学",
    "note": "受审资料",
    "assets": [{
      "subject": "高等数学",
      "role": "复习讲义",
      "title": "讲义.pdf",
      "publicPath": "高等数学/复习讲义/讲义.pdf",
      "bytes": 1,
      "sha256": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
      "year": %s
    }]
  }]
}`, year))
}
