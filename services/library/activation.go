package library

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var publicMaterialNamespace = uuid.MustParse("5a1ed44f-2c16-5fe9-aac5-d52e44c61531")

var (
	releaseIDPattern                 = regexp.MustCompile(`^[a-f0-9]{40}-[a-f0-9]{16}$`)
	digestPattern                    = regexp.MustCompile(`^[a-f0-9]{64}$`)
	unsafeDownloadFileNamePattern    = regexp.MustCompile(`[,:?*<>|"\\/]`)
	temporaryDownloadFileNamePattern = regexp.MustCompile(`(?i)(副本|final_final|未命名|新建文件|^~\$|\.tmp$|\.crdownload$|\.download$)`)
)

// manifestRoleContract is the local, fail-closed projection of
// HENU-Final-Review's canonical material roles. It is intentionally exact:
// roles describe content semantics, so neither filenames nor substrings may
// invent a new public catalog type.
type manifestRoleContract struct {
	CanonicalRole string
	MaterialType  string
	Publishable   bool
}

var canonicalManifestRoles = map[string]manifestRoleContract{
	"复习讲义":  {CanonicalRole: "复习讲义", MaterialType: "handout", Publishable: true},
	"往年真题":  {CanonicalRole: "往年真题", MaterialType: "exam", Publishable: true},
	"课件":    {CanonicalRole: "课件", MaterialType: "slides", Publishable: true},
	"题库练习":  {CanonicalRole: "题库练习", MaterialType: "exercise", Publishable: true},
	"答案解析":  {CanonicalRole: "答案解析", MaterialType: "answer", Publishable: true},
	"笔记总结":  {CanonicalRole: "笔记总结", MaterialType: "note", Publishable: true},
	"电子版教材": {CanonicalRole: "电子版教材", MaterialType: "textbook", Publishable: true},
	"待复核资料": {CanonicalRole: "待复核资料", Publishable: false},
}

var legacyManifestRoleAliases = map[string]string{
	"课件PPT":    "课件",
	"课件资料":     "课件",
	"课件资料包":    "课件",
	"待复核课件PPT": "待复核资料",
}

func resolveManifestRole(role string) (manifestRoleContract, bool) {
	if canonical, ok := legacyManifestRoleAliases[role]; ok {
		role = canonical
	}
	contract, ok := canonicalManifestRoles[role]
	return contract, ok
}

// Publication provenance policy. HENU Kit activation is an independent
// fail-closed publication boundary: it does not trust the upstream
// source-repository validator to have applied the publication policy
// correctly. These allowlists are synchronized with HENU-Final-Review's
// PUBLICATION_POLICY.md, docs/manifest.md, and scripts/validate-materials.mjs;
// contract tests pin them so drift is caught (see activation_manifest_test.go).
//
// A manifest asset that reaches activation must satisfy every rule:
//   - containsPersonalInfo must not be true.
//   - reviewStatus and licenseStatus are advisory for general materials,
//     matching the upstream validator. teacher_shared_exception remains
//     restricted to the approved historical whitelist below (path + SHA-256).
//   - electronic textbooks require exact reviewed redistribution evidence.
//   - uncertainty, when present, must not be a review-only state: the source
//     policy requires those assets to live under a 待复核 role, which Library
//     never activates.
//
// HENU-Final-Review treats general provenance fields as recommended metadata
// that is being backfilled gradually. Library must not invent a stricter
// generic allowlist after the exact upstream SHA has already been accepted.
var (
	reviewOnlyUncertainties = map[string]bool{
		"source_uncertain":          true,
		"year_uncertain":            true,
		"course_uncertain":          true,
		"public_boundary_uncertain": true,
	}
	approvedTeacherSharedExceptions = map[string]string{
		"思想道德与法治/复习讲义/思想道德与法治_复习讲义_2025年冬最新考试重点.pdf":                        "bfda62a15cfefb53c1413a244a4ff9f95e11a9fc959032f4ebff83adc1b8530c",
		"思想道德与法治/复习讲义/思想道德与法治_复习讲义_2026年夏最新考试重点.pdf":                        "62605c70458a8da91a90e38f88fb9a628ba4283233262e3554b9085ab0acee73",
		"思想道德与法治/题库练习/思想道德与法治_题库练习_2025年冬最新考试习题库.pdf":                       "863593807fb03560c9fd351faa33176fbe38e5063897d1efe0d2df657bb65aeb",
		"思想道德与法治/题库练习/思想道德与法治_题库练习_2026年夏最新考试习题库.pdf":                       "2202c6481c4e20484d2dee269d40203b3ce297903d81ccfa3b9e30d17ad1df2c",
		"习近平新时代中国特色社会主义思想概论/复习讲义/习近平新时代中国特色社会主义思想概论_复习讲义_2025年冬最新教材重点.pdf":  "9a9b0b52a35fbee614b33e0fb231ef5df982e9cb460d7ece0d09faaf4c266bd7",
		"习近平新时代中国特色社会主义思想概论/题库练习/习近平新时代中国特色社会主义思想概论_题库练习_2025年冬最新教材习题库.pdf": "b53770144dc0db848f00036d593689ef12339002cc1ab24df856213fe03944ca",
	}
)

// provenanceViolation reports the first publication-policy violation for one
// manifest asset, or "" when the asset is publishable. It implements the
// independent fail-closed activation boundary; see the policy block above.
func provenanceViolation(asset manifestAsset) string {
	if asset.ContainsPersonalInfo {
		return "containsPersonalInfo=true"
	}
	roleContract, roleKnown := resolveManifestRole(asset.Role)
	isTextbook := roleKnown && roleContract.CanonicalRole == "电子版教材"
	if isTextbook {
		if asset.ReviewStatus != "verified" {
			return "electronic textbook reviewStatus must be verified"
		}
		if asset.LicenseStatus != "authorized-redistribution" {
			return "electronic textbook licenseStatus must be authorized-redistribution"
		}
		if !safeText(asset.SourceNote, 4000) {
			return "electronic textbook sourceNote is required"
		}
	}
	if !isTextbook && asset.LicenseStatus == "teacher_shared_exception" {
		approved, ok := approvedTeacherSharedExceptions[asset.PublicPath]
		if !ok || approved != asset.SHA256 {
			return "teacher_shared_exception is restricted to the approved historical files"
		}
	}
	if asset.Uncertainty != "" && reviewOnlyUncertainties[asset.Uncertainty] {
		return "uncertainty " + strconv.Quote(asset.Uncertainty) + " requires a review role"
	}
	return ""
}

type PublicReleaseActivation struct {
	Version           int                           `json:"version"`
	ReleaseID         string                        `json:"release_id"`
	ManifestJSON      []byte                        `json:"manifest_json"`
	SealedReceiptJSON []byte                        `json:"sealed_receipt_json"`
	ReleaseCommitJSON []byte                        `json:"release_commit_json"`
	Derived           PublicReleaseDerivedArtifacts `json:"derived"`
	Objects           []PublicReleaseObject         `json:"objects"`
}

type PublicReleaseDerivedArtifacts struct {
	ReleaseID    string `json:"release_id"`
	SlidesSHA256 string `json:"slides_sha256"`
	IndexSHA256  string `json:"index_sha256"`
}

type PublicReleaseObject struct {
	PublicPath      string `json:"public_path"`
	ObjectKey       string `json:"object_key"`
	ObjectVersionID string `json:"object_version_id"`
}

type PublicReleaseActivationResult struct {
	ReleaseID         string `json:"release_id"`
	PreviousReleaseID string `json:"previous_release_id"`
	MaterialCount     int    `json:"material_count"`
	Replayed          bool   `json:"replayed"`
}

type reviewedManifest struct {
	Version     int               `json:"version"`
	GeneratedAt string            `json:"generatedAt"`
	Subjects    []manifestSubject `json:"subjects"`
}

type manifestSubject struct {
	Name   string          `json:"name"`
	Note   string          `json:"note"`
	Assets []manifestAsset `json:"assets"`
}

type manifestAsset struct {
	Subject     string `json:"subject"`
	Role        string `json:"role"`
	Title       string `json:"title"`
	PublicPath  string `json:"publicPath"`
	Bytes       int64  `json:"bytes"`
	SHA256      string `json:"sha256"`
	Attribution struct {
		Authors    []string `json:"authors"`
		Collectors []string `json:"collectors"`
	} `json:"attribution"`
	College              string       `json:"college"`
	ContainsPersonalInfo bool         `json:"containsPersonalInfo"`
	LicenseStatus        string       `json:"licenseStatus"`
	ReviewStatus         string       `json:"reviewStatus"`
	SourceNote           string       `json:"sourceNote"`
	SourceType           string       `json:"sourceType"`
	Uncertainty          string       `json:"uncertainty"`
	Year                 manifestYear `json:"year"`
}

// manifestYear accepts the documented string shape plus the integer shape
// already present in the reviewed upstream manifest. The field is provenance
// metadata only; normalizing it here keeps strict unknown-field decoding while
// refusing booleans, arrays, objects, and fractional numbers.
type manifestYear string

func (year *manifestYear) UnmarshalJSON(value []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(value))
	decoder.UseNumber()
	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return err
	}
	switch item := decoded.(type) {
	case nil:
		*year = ""
		return nil
	case string:
		*year = manifestYear(item)
		return nil
	case json.Number:
		if _, err := strconv.Atoi(item.String()); err != nil {
			return errors.New("manifest year number must be an integer")
		}
		*year = manifestYear(item.String())
		return nil
	default:
		return errors.New("manifest year must be a string, integer, or null")
	}
}

type sealedReceipt struct {
	Version   int    `json:"version"`
	ReleaseID string `json:"release_id"`
	Source    struct {
		Repository string `json:"repository"`
		Ref        string `json:"ref"`
		SHA        string `json:"sha"`
	} `json:"source"`
	ManifestSHA256  string `json:"manifest_sha256"`
	InventorySHA256 string `json:"inventory_sha256"`
	TreeSHA256      string `json:"tree_sha256"`
	ReviewedAssets  int    `json:"reviewed_assets"`
	Slides          struct {
		Status            string `json:"status"`
		SourceSlideAssets int    `json:"source_slide_assets"`
	} `json:"slides"`
}

type ossReleaseCommit struct {
	Version         int                     `json:"version"`
	State           string                  `json:"state"`
	ReleaseID       string                  `json:"release_id"`
	ReceiptSHA256   string                  `json:"receipt_sha256"`
	ManifestSHA256  string                  `json:"manifest_sha256"`
	InventorySHA256 string                  `json:"inventory_sha256"`
	TreeSHA256      string                  `json:"tree_sha256"`
	AssetCount      int                     `json:"asset_count"`
	Assets          []ossReleaseCommitAsset `json:"assets"`
}

type ossReleaseCommitAsset struct {
	PublicPath      string `json:"public_path"`
	SHA256          string `json:"sha256"`
	Bytes           int64  `json:"bytes"`
	ObjectKey       string `json:"object_key"`
	ObjectVersionID string `json:"object_version_id"`
}

type activatedMaterial struct {
	MaterialID      uuid.UUID `json:"material_id"`
	Subject         string    `json:"subject"`
	Role            string    `json:"role"`
	MaterialType    string    `json:"material_type"`
	Title           string    `json:"title"`
	FileName        string    `json:"file_name"`
	PublicPath      string    `json:"public_path"`
	ObjectKey       string    `json:"object_key"`
	ObjectVersionID string    `json:"object_version_id"`
	SHA256          string    `json:"sha256"`
	ByteSize        int64     `json:"byte_size"`
}

type preparedActivation struct {
	releaseID        string
	receiptSHA256    string
	ossCommitSHA256  string
	manifestSHA256   string
	catalogSHA256    string
	indexSHA256      string
	slidesSHA256     string
	activationDigest string
	materials        []activatedMaterial
	legacyCatalogSHA string
	legacyActivation string
	legacyMaterials  []activatedMaterial
}

// ActivatePublicRelease validates one complete reviewed manifest against exact
// private OSS object versions, then atomically switches Library's owner catalog.
// It is deliberately not registered as an HTTP route in this ticket: production
// publication wiring and authority remain a later operational release gate.
func ActivatePublicRelease(ctx context.Context, database *pgxpool.Pool, store DownloadObjectStore, bundle PublicReleaseActivation, now func() time.Time) (PublicReleaseActivationResult, error) {
	if database == nil || store == nil || now == nil {
		return PublicReleaseActivationResult{}, errors.New("public release activation dependencies are required")
	}
	prepared, err := preparePublicReleaseActivation(bundle)
	if err != nil {
		return PublicReleaseActivationResult{}, err
	}
	for _, material := range prepared.materials {
		state, found, headErr := store.Head(ctx, material.ObjectKey, material.ObjectVersionID)
		if headErr != nil || !found || state.Bytes != material.ByteSize || state.SHA256 != material.SHA256 || state.Encryption != "AES256" || state.VersionID != material.ObjectVersionID {
			return PublicReleaseActivationResult{}, fmt.Errorf("public release object verification failed for %s", material.PublicPath)
		}
		if err := store.AnonymousDenied(ctx, material.ObjectKey, material.ObjectVersionID); err != nil {
			return PublicReleaseActivationResult{}, fmt.Errorf("public release object is not private for %s", material.PublicPath)
		}
	}
	return commitPublicReleaseActivation(ctx, database, prepared, now().UTC())
}

func preparePublicReleaseActivation(bundle PublicReleaseActivation) (preparedActivation, error) {
	if bundle.Version != 1 || !releaseIDPattern.MatchString(bundle.ReleaseID) || bundle.Derived.ReleaseID != bundle.ReleaseID || !digestPattern.MatchString(bundle.Derived.SlidesSHA256) || !digestPattern.MatchString(bundle.Derived.IndexSHA256) {
		return preparedActivation{}, errors.New("public release identity or derived artifacts are invalid")
	}
	manifestSHA := sha256.Sum256(bundle.ManifestJSON)
	manifestDigest := hex.EncodeToString(manifestSHA[:])
	if !strings.HasSuffix(bundle.ReleaseID, "-"+manifestDigest[:16]) {
		return preparedActivation{}, errors.New("public release identity does not bind the manifest")
	}
	var manifest reviewedManifest
	if err := decodeSingleJSON(bundle.ManifestJSON, &manifest); err != nil || manifest.Version != 1 || manifest.Subjects == nil {
		return preparedActivation{}, errors.New("reviewed manifest is invalid")
	}
	var receipt sealedReceipt
	emptySlidesHash := sha256.Sum256([]byte("[]\n"))
	emptyIndex := []byte(fmt.Sprintf("{\n  \"version\": 1,\n  \"release_id\": %q,\n  \"assets\": []\n}\n", bundle.ReleaseID))
	emptyIndexHash := sha256.Sum256(emptyIndex)
	if err := decodeSingleJSON(bundle.SealedReceiptJSON, &receipt); err != nil || receipt.Version != 1 || receipt.ReleaseID != bundle.ReleaseID || receipt.Source.Repository != "https://github.com/jry21223/HENU-Final-Review.git" || receipt.Source.Ref != "refs/heads/main" || receipt.Source.SHA != bundle.ReleaseID[:40] || receipt.ManifestSHA256 != manifestDigest || !digestPattern.MatchString(receipt.InventorySHA256) || !digestPattern.MatchString(receipt.TreeSHA256) || receipt.Slides.Status != "disabled" || receipt.Slides.SourceSlideAssets < 0 || bundle.Derived.SlidesSHA256 != hex.EncodeToString(emptySlidesHash[:]) || bundle.Derived.IndexSHA256 != hex.EncodeToString(emptyIndexHash[:]) {
		return preparedActivation{}, errors.New("sealed receipt does not bind the release")
	}
	receiptHash := sha256.Sum256(bundle.SealedReceiptJSON)
	receiptDigest := hex.EncodeToString(receiptHash[:])
	var commit ossReleaseCommit
	if err := decodeSingleJSON(bundle.ReleaseCommitJSON, &commit); err != nil || commit.Version != 1 || commit.State != "release_committed_not_activated" || commit.ReleaseID != bundle.ReleaseID || commit.ReceiptSHA256 != receiptDigest || commit.ManifestSHA256 != manifestDigest || commit.InventorySHA256 != receipt.InventorySHA256 || commit.TreeSHA256 != receipt.TreeSHA256 || commit.AssetCount < 0 || commit.AssetCount != len(commit.Assets) {
		return preparedActivation{}, errors.New("OSS release commit does not bind the sealed release")
	}
	commitHash := sha256.Sum256(bundle.ReleaseCommitJSON)
	commitDigest := hex.EncodeToString(commitHash[:])
	committedObjects := make(map[string]ossReleaseCommitAsset, len(commit.Assets))
	for _, asset := range commit.Assets {
		if _, duplicate := committedObjects[asset.PublicPath]; duplicate || !safePublicPath(asset.PublicPath) || !digestPattern.MatchString(asset.SHA256) || asset.Bytes < 0 || !safeObjectVersionID(asset.ObjectVersionID) || !validOSSObjectKey(asset.ObjectKey) {
			return preparedActivation{}, errors.New("OSS release commit object inventory is invalid")
		}
		committedObjects[asset.PublicPath] = asset
	}
	objects := make(map[string]PublicReleaseObject, len(bundle.Objects))
	for _, object := range bundle.Objects {
		if _, duplicate := objects[object.PublicPath]; duplicate || !safePublicPath(object.PublicPath) || !safeObjectVersionID(object.ObjectVersionID) || !validOSSObjectKey(object.ObjectKey) {
			return preparedActivation{}, errors.New("public release object inventory is invalid")
		}
		committed, ok := committedObjects[object.PublicPath]
		if !ok || committed.ObjectKey != object.ObjectKey || committed.ObjectVersionID != object.ObjectVersionID {
			return preparedActivation{}, errors.New("activation object inventory does not match the OSS release commit")
		}
		objects[object.PublicPath] = object
	}
	materials := make([]activatedMaterial, 0, len(objects))
	legacyMaterials := make([]activatedMaterial, 0, len(objects))
	seenPaths := map[string]bool{}
	for _, subject := range manifest.Subjects {
		if !safeText(subject.Name, 160) || (subject.Note != "" && !safeText(subject.Note, 4000)) || subject.Assets == nil {
			return preparedActivation{}, errors.New("reviewed manifest subject is invalid")
		}
		for _, asset := range subject.Assets {
			roleContract, roleOK := resolveManifestRole(asset.Role)
			if !roleOK {
				return preparedActivation{}, fmt.Errorf("reviewed manifest role is unsupported: %s", asset.Role)
			}
			if !roleContract.Publishable {
				continue
			}
			if violation := provenanceViolation(asset); violation != "" {
				return preparedActivation{}, fmt.Errorf("publication provenance is not publishable for %s: %s", asset.PublicPath, violation)
			}
			if (asset.Subject != "" && asset.Subject != subject.Name) || !safeText(asset.Role, 160) || !safeText(asset.Title, 255) || !safePublicPath(asset.PublicPath) || !digestPattern.MatchString(asset.SHA256) || asset.Bytes < 0 || seenPaths[asset.PublicPath] {
				return preparedActivation{}, fmt.Errorf("reviewed manifest asset is invalid: %s", asset.PublicPath)
			}
			title := strings.TrimSuffix(asset.Title, path.Ext(asset.Title))
			fileName := path.Base(asset.PublicPath)
			if !safeText(title, 200) || !safeDownloadFileName(fileName) {
				return preparedActivation{}, fmt.Errorf("reviewed manifest presentation metadata is invalid: %s", asset.PublicPath)
			}
			seenPaths[asset.PublicPath] = true
			object, ok := objects[asset.PublicPath]
			if !ok {
				return preparedActivation{}, fmt.Errorf("reviewed manifest object is missing: %s", asset.PublicPath)
			}
			committed := committedObjects[asset.PublicPath]
			if committed.SHA256 != asset.SHA256 || committed.Bytes != asset.Bytes {
				return preparedActivation{}, fmt.Errorf("OSS release commit object evidence is invalid: %s", asset.PublicPath)
			}
			expectedKey := "releases/" + bundle.ReleaseID + "/receipts/" + receiptDigest + "/objects/" + asset.SHA256 + "/" + asset.PublicPath
			if !validOSSObjectKey(expectedKey) || object.ObjectKey != expectedKey {
				return preparedActivation{}, fmt.Errorf("public release object identity is invalid: %s", asset.PublicPath)
			}
			material := activatedMaterial{
				MaterialID: uuid.NewSHA1(publicMaterialNamespace, []byte(asset.PublicPath)), Subject: subject.Name,
				Role: roleContract.CanonicalRole, MaterialType: roleContract.MaterialType, Title: title,
				FileName: fileName, PublicPath: asset.PublicPath, ObjectKey: object.ObjectKey, ObjectVersionID: object.ObjectVersionID,
				SHA256: asset.SHA256, ByteSize: asset.Bytes,
			}
			materials = append(materials, material)
			legacyMaterial := material
			legacyMaterial.Role = asset.Role
			legacyMaterial.MaterialType = legacyManifestMaterialType(asset.Role)
			legacyMaterials = append(legacyMaterials, legacyMaterial)
			delete(objects, asset.PublicPath)
		}
	}
	if len(objects) != 0 || len(committedObjects) != len(materials) || receipt.ReviewedAssets != len(materials) {
		return preparedActivation{}, errors.New("public release object inventory is not the complete reviewed manifest")
	}
	if len(materials) > 500 {
		return preparedActivation{}, errors.New("public release owner catalog exceeds its bounded size")
	}
	sort.Slice(materials, func(i, j int) bool { return materials[i].PublicPath < materials[j].PublicPath })
	sort.Slice(legacyMaterials, func(i, j int) bool { return legacyMaterials[i].PublicPath < legacyMaterials[j].PublicPath })
	catalogJSON, err := json.Marshal(materials)
	if err != nil {
		return preparedActivation{}, err
	}
	catalogHash := sha256.Sum256(catalogJSON)
	catalogDigest := hex.EncodeToString(catalogHash[:])
	activationHash := sha256.Sum256([]byte(strings.Join([]string{bundle.ReleaseID, receiptDigest, commitDigest, manifestDigest, catalogDigest, bundle.Derived.IndexSHA256, bundle.Derived.SlidesSHA256}, "\n")))
	legacyCatalogJSON, err := json.Marshal(legacyMaterials)
	if err != nil {
		return preparedActivation{}, err
	}
	legacyCatalogHash := sha256.Sum256(legacyCatalogJSON)
	legacyCatalogDigest := hex.EncodeToString(legacyCatalogHash[:])
	legacyActivationHash := sha256.Sum256([]byte(strings.Join([]string{bundle.ReleaseID, receiptDigest, commitDigest, manifestDigest, legacyCatalogDigest, bundle.Derived.IndexSHA256, bundle.Derived.SlidesSHA256}, "\n")))
	return preparedActivation{
		releaseID: bundle.ReleaseID, receiptSHA256: receiptDigest, ossCommitSHA256: commitDigest, manifestSHA256: manifestDigest,
		catalogSHA256: catalogDigest, indexSHA256: bundle.Derived.IndexSHA256, slidesSHA256: bundle.Derived.SlidesSHA256,
		activationDigest: hex.EncodeToString(activationHash[:]), materials: materials,
		legacyCatalogSHA: legacyCatalogDigest, legacyActivation: hex.EncodeToString(legacyActivationHash[:]), legacyMaterials: legacyMaterials,
	}, nil
}

func commitPublicReleaseActivation(ctx context.Context, database *pgxpool.Pool, prepared preparedActivation, activatedAt time.Time) (PublicReleaseActivationResult, error) {
	tx, err := database.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return PublicReleaseActivationResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(484821409, 334)`); err != nil {
		return PublicReleaseActivationResult{}, err
	}
	var currentRelease string
	err = tx.QueryRow(ctx, `SELECT release_id FROM library_public_releases WHERE state='active'`).Scan(&currentRelease)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return PublicReleaseActivationResult{}, err
	}
	if currentRelease == prepared.releaseID {
		matched, match, err := persistedReleaseMatch(ctx, tx, prepared)
		if err != nil {
			return PublicReleaseActivationResult{}, err
		}
		if !match {
			return PublicReleaseActivationResult{}, errors.New("active release identity conflicts with the activation bundle")
		}
		if err := tx.Commit(ctx); err != nil {
			return PublicReleaseActivationResult{}, err
		}
		return PublicReleaseActivationResult{ReleaseID: prepared.releaseID, MaterialCount: len(matched.materials), Replayed: true}, nil
	}
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM library_public_releases WHERE release_id=$1)`, prepared.releaseID).Scan(&exists); err != nil {
		return PublicReleaseActivationResult{}, err
	}
	if exists {
		matched, match, err := persistedReleaseMatch(ctx, tx, prepared)
		if err != nil || !match {
			if err != nil {
				return PublicReleaseActivationResult{}, err
			}
			return PublicReleaseActivationResult{}, errors.New("retained release identity conflicts with the activation bundle")
		}
		prepared = matched
	} else {
		if _, err := tx.Exec(ctx, `
			INSERT INTO library_public_releases
				(release_id,receipt_sha256,state,activated_at,oss_commit_sha256,manifest_sha256,catalog_sha256,index_sha256,slides_sha256,activation_digest)
			VALUES ($1,$2,'retired',$3,$4,$5,$6,$7,$8,$9)`,
			prepared.releaseID, prepared.receiptSHA256, activatedAt, prepared.ossCommitSHA256, prepared.manifestSHA256, prepared.catalogSHA256, prepared.indexSHA256, prepared.slidesSHA256, prepared.activationDigest); err != nil {
			return PublicReleaseActivationResult{}, err
		}
		for _, material := range prepared.materials {
			if _, err := tx.Exec(ctx, `
				INSERT INTO library_public_material_snapshots
					(release_id,material_id,title,file_name,access_level,status,object_key,object_version_id,sha256,byte_size,subject,role,material_type,public_path)
				VALUES ($1,$2,$3,$4,'public_free','published',$5,$6,$7,$8,$9,$10,$11,$12)`,
				prepared.releaseID, material.MaterialID, material.Title, material.FileName, material.ObjectKey, material.ObjectVersionID,
				material.SHA256, material.ByteSize, material.Subject, material.Role, material.MaterialType, material.PublicPath); err != nil {
				return PublicReleaseActivationResult{}, err
			}
		}
	}
	if currentRelease != "" {
		if _, err := tx.Exec(ctx, `UPDATE library_public_releases SET state='retired' WHERE release_id=$1 AND state='active'`, currentRelease); err != nil {
			return PublicReleaseActivationResult{}, err
		}
	}
	result, err := tx.Exec(ctx, `UPDATE library_public_releases SET state='active' WHERE release_id=$1 AND state='retired'`, prepared.releaseID)
	if err != nil || result.RowsAffected() != 1 {
		if err != nil {
			return PublicReleaseActivationResult{}, err
		}
		return PublicReleaseActivationResult{}, errors.New("public release activation did not switch exactly one release")
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO library_public_release_activation_events
			(id,release_id,previous_release_id,activation_digest,activated_at,material_count)
		VALUES ($1,$2,NULLIF($3,''),$4,$5,$6)`, uuid.New(), prepared.releaseID, currentRelease, prepared.activationDigest, activatedAt, len(prepared.materials)); err != nil {
		return PublicReleaseActivationResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return PublicReleaseActivationResult{}, err
	}
	return PublicReleaseActivationResult{ReleaseID: prepared.releaseID, PreviousReleaseID: currentRelease, MaterialCount: len(prepared.materials)}, nil
}

func persistedReleaseMatches(ctx context.Context, tx pgx.Tx, prepared preparedActivation) (bool, error) {
	var receipt, commit, manifest, catalog, index, slides, activation string
	if err := tx.QueryRow(ctx, `SELECT receipt_sha256,oss_commit_sha256,manifest_sha256,catalog_sha256,index_sha256,slides_sha256,activation_digest FROM library_public_releases WHERE release_id=$1`, prepared.releaseID).Scan(&receipt, &commit, &manifest, &catalog, &index, &slides, &activation); err != nil {
		return false, err
	}
	if receipt != prepared.receiptSHA256 || commit != prepared.ossCommitSHA256 || manifest != prepared.manifestSHA256 || catalog != prepared.catalogSHA256 || index != prepared.indexSHA256 || slides != prepared.slidesSHA256 || activation != prepared.activationDigest {
		return false, nil
	}
	var count int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM library_public_material_snapshots WHERE release_id=$1`, prepared.releaseID).Scan(&count); err != nil {
		return false, err
	}
	if count != len(prepared.materials) {
		return false, nil
	}
	for _, material := range prepared.materials {
		var matches bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS(
				SELECT 1 FROM library_public_material_snapshots
				WHERE release_id=$1 AND material_id=$2 AND title=$3 AND file_name=$4
				  AND access_level='public_free' AND status='published'
				  AND object_key=$5 AND object_version_id=$6 AND sha256=$7 AND byte_size=$8
				  AND subject=$9 AND role=$10 AND material_type=$11 AND public_path=$12
			)`, prepared.releaseID, material.MaterialID, material.Title, material.FileName,
			material.ObjectKey, material.ObjectVersionID, material.SHA256, material.ByteSize,
			material.Subject, material.Role, material.MaterialType, material.PublicPath).Scan(&matches); err != nil {
			return false, err
		}
		if !matches {
			return false, nil
		}
	}
	return true, nil
}

func persistedReleaseMatch(ctx context.Context, tx pgx.Tx, prepared preparedActivation) (preparedActivation, bool, error) {
	match, err := persistedReleaseMatches(ctx, tx, prepared)
	if err != nil || match || prepared.legacyCatalogSHA == prepared.catalogSHA256 {
		return prepared, match, err
	}
	legacy := prepared
	legacy.catalogSHA256 = prepared.legacyCatalogSHA
	legacy.activationDigest = prepared.legacyActivation
	legacy.materials = prepared.legacyMaterials
	match, err = persistedReleaseMatches(ctx, tx, legacy)
	return legacy, match, err
}

func decodeSingleJSON(value []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(value))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("JSON value has trailing content")
	}
	return nil
}

func safeText(value string, max int) bool {
	if value == "" || len(value) > max || strings.TrimSpace(value) != value {
		return false
	}
	for _, char := range value {
		if unicode.IsControl(char) {
			return false
		}
	}
	return true
}

func safePublicPath(value string) bool {
	if value == "" || len(value) > 1023 || !utf8.ValidString(value) || strings.Contains(value, `\`) || strings.HasPrefix(value, "/") {
		return false
	}
	for _, segment := range strings.Split(value, "/") {
		if segment == "" || segment == "." || segment == ".." || strings.HasPrefix(segment, ".") || !safeText(segment, 255) {
			return false
		}
	}
	return true
}

func validOSSObjectKey(value string) bool {
	if value == "" || len(value) > 1023 || !utf8.ValidString(value) || strings.HasPrefix(value, "/") || strings.HasPrefix(value, `\`) {
		return false
	}
	for _, char := range value {
		if unicode.IsControl(char) {
			return false
		}
	}
	return true
}

func safeObjectVersionID(value string) bool {
	if value == "" || value == "null" || len(value) > 1024 || !utf8.ValidString(value) || strings.TrimSpace(value) != value {
		return false
	}
	for _, char := range value {
		if unicode.IsControl(char) {
			return false
		}
	}
	return true
}

func safeDownloadFileName(value string) bool {
	if !safeText(value, 255) || value == "." || value == ".." || strings.HasPrefix(value, ".") || unsafeDownloadFileNamePattern.MatchString(value) || temporaryDownloadFileNamePattern.MatchString(value) {
		return false
	}
	return true
}

// legacyManifestMaterialType reproduces the pre-000005 projection only for
// exact identity checks of already-retained releases. New releases always use
// resolveManifestRole's canonical seven-type contract.
func legacyManifestMaterialType(role string) string {
	switch {
	case strings.Contains(role, "电子版教材") || strings.Contains(role, "电子教材"):
		return "textbook"
	case strings.Contains(role, "PPT") || strings.Contains(role, "课件"):
		return "slides"
	case strings.Contains(role, "真题") || strings.Contains(role, "试卷"):
		return "exam"
	case strings.Contains(role, "模拟"):
		return "mock"
	case strings.Contains(role, "实验"):
		return "lab"
	case strings.Contains(role, "路径"):
		return "path"
	default:
		return "note"
	}
}
