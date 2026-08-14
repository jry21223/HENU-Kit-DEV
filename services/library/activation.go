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
	releaseIDPattern = regexp.MustCompile(`^[a-f0-9]{40}-[a-f0-9]{16}$`)
	digestPattern    = regexp.MustCompile(`^[a-f0-9]{64}$`)
)

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
	Subject    string `json:"subject"`
	Role       string `json:"role"`
	Title      string `json:"title"`
	PublicPath string `json:"publicPath"`
	Bytes      int64  `json:"bytes"`
	SHA256     string `json:"sha256"`
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
		if _, duplicate := committedObjects[asset.PublicPath]; duplicate || !safePublicPath(asset.PublicPath) || !digestPattern.MatchString(asset.SHA256) || asset.Bytes < 0 || strings.TrimSpace(asset.ObjectVersionID) == "" {
			return preparedActivation{}, errors.New("OSS release commit object inventory is invalid")
		}
		committedObjects[asset.PublicPath] = asset
	}
	objects := make(map[string]PublicReleaseObject, len(bundle.Objects))
	for _, object := range bundle.Objects {
		if _, duplicate := objects[object.PublicPath]; duplicate || !safePublicPath(object.PublicPath) || strings.TrimSpace(object.ObjectVersionID) == "" || len(object.ObjectVersionID) > 1024 {
			return preparedActivation{}, errors.New("public release object inventory is invalid")
		}
		committed, ok := committedObjects[object.PublicPath]
		if !ok || committed.ObjectKey != object.ObjectKey || committed.ObjectVersionID != object.ObjectVersionID {
			return preparedActivation{}, errors.New("activation object inventory does not match the OSS release commit")
		}
		objects[object.PublicPath] = object
	}
	materials := make([]activatedMaterial, 0, len(objects))
	seenPaths := map[string]bool{}
	for _, subject := range manifest.Subjects {
		if !safeText(subject.Name, 160) || (subject.Note != "" && !safeText(subject.Note, 4000)) || subject.Assets == nil {
			return preparedActivation{}, errors.New("reviewed manifest subject is invalid")
		}
		for _, asset := range subject.Assets {
			if strings.HasPrefix(asset.Role, "待复核") {
				continue
			}
			if (asset.Subject != "" && asset.Subject != subject.Name) || !safeText(asset.Role, 160) || !safeText(asset.Title, 255) || !safePublicPath(asset.PublicPath) || !digestPattern.MatchString(asset.SHA256) || asset.Bytes < 0 || seenPaths[asset.PublicPath] {
				return preparedActivation{}, fmt.Errorf("reviewed manifest asset is invalid: %s", asset.PublicPath)
			}
			title := strings.TrimSuffix(asset.Title, path.Ext(asset.Title))
			fileName := path.Base(asset.PublicPath)
			if !safeText(title, 200) || !safeText(fileName, 255) {
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
			if utf8.RuneCountInString(expectedKey) > 1024 || object.ObjectKey != expectedKey {
				return preparedActivation{}, fmt.Errorf("public release object identity is invalid: %s", asset.PublicPath)
			}
			materials = append(materials, activatedMaterial{
				MaterialID: uuid.NewSHA1(publicMaterialNamespace, []byte(asset.PublicPath)), Subject: subject.Name,
				Role: asset.Role, MaterialType: manifestMaterialType(asset.Role), Title: title,
				FileName: fileName, PublicPath: asset.PublicPath, ObjectKey: object.ObjectKey, ObjectVersionID: object.ObjectVersionID,
				SHA256: asset.SHA256, ByteSize: asset.Bytes,
			})
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
	catalogJSON, err := json.Marshal(materials)
	if err != nil {
		return preparedActivation{}, err
	}
	catalogHash := sha256.Sum256(catalogJSON)
	catalogDigest := hex.EncodeToString(catalogHash[:])
	activationHash := sha256.Sum256([]byte(strings.Join([]string{bundle.ReleaseID, receiptDigest, commitDigest, manifestDigest, catalogDigest, bundle.Derived.IndexSHA256, bundle.Derived.SlidesSHA256}, "\n")))
	return preparedActivation{
		releaseID: bundle.ReleaseID, receiptSHA256: receiptDigest, ossCommitSHA256: commitDigest, manifestSHA256: manifestDigest,
		catalogSHA256: catalogDigest, indexSHA256: bundle.Derived.IndexSHA256, slidesSHA256: bundle.Derived.SlidesSHA256,
		activationDigest: hex.EncodeToString(activationHash[:]), materials: materials,
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
		match, err := persistedReleaseMatches(ctx, tx, prepared)
		if err != nil {
			return PublicReleaseActivationResult{}, err
		}
		if !match {
			return PublicReleaseActivationResult{}, errors.New("active release identity conflicts with the activation bundle")
		}
		if err := tx.Commit(ctx); err != nil {
			return PublicReleaseActivationResult{}, err
		}
		return PublicReleaseActivationResult{ReleaseID: prepared.releaseID, MaterialCount: len(prepared.materials), Replayed: true}, nil
	}
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM library_public_releases WHERE release_id=$1)`, prepared.releaseID).Scan(&exists); err != nil {
		return PublicReleaseActivationResult{}, err
	}
	if exists {
		match, err := persistedReleaseMatches(ctx, tx, prepared)
		if err != nil || !match {
			if err != nil {
				return PublicReleaseActivationResult{}, err
			}
			return PublicReleaseActivationResult{}, errors.New("retained release identity conflicts with the activation bundle")
		}
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
	if value == "" || len(value) > 1024 || strings.Contains(value, `\`) || strings.HasPrefix(value, "/") {
		return false
	}
	for _, segment := range strings.Split(value, "/") {
		if segment == "" || segment == "." || segment == ".." || strings.HasPrefix(segment, ".") || !safeText(segment, 255) {
			return false
		}
	}
	return true
}

func manifestMaterialType(role string) string {
	switch {
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
