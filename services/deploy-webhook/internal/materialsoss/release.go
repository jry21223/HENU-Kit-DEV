package materialsoss

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type ReleaseRequest struct {
	ReleaseID     string
	ReceiptSHA256 string
}

type ReleaseAsset struct {
	PublicPath      string `json:"public_path"`
	SHA256          string `json:"sha256"`
	Bytes           int64  `json:"bytes"`
	ObjectKey       string `json:"object_key"`
	ObjectVersionID string `json:"object_version_id"`
}

type releaseCommit struct {
	Version         int            `json:"version"`
	State           string         `json:"state"`
	ReleaseID       string         `json:"release_id"`
	ReceiptSHA256   string         `json:"receipt_sha256"`
	ManifestSHA256  string         `json:"manifest_sha256"`
	InventorySHA256 string         `json:"inventory_sha256"`
	TreeSHA256      string         `json:"tree_sha256"`
	AssetCount      int            `json:"asset_count"`
	Assets          []ReleaseAsset `json:"assets"`
}

type ReleaseResult struct {
	Version             int            `json:"version"`
	State               string         `json:"state"`
	ReleaseID           string         `json:"release_id"`
	ReceiptSHA256       string         `json:"receipt_sha256"`
	ManifestSHA256      string         `json:"manifest_sha256"`
	InventorySHA256     string         `json:"inventory_sha256"`
	TreeSHA256          string         `json:"tree_sha256"`
	AssetCount          int            `json:"asset_count"`
	Assets              []ReleaseAsset `json:"assets"`
	ReleaseCommitSHA256 string         `json:"release_commit_sha256"`
}

type validatedReleaseAsset struct {
	asset
}

type releaseObjectReceipt struct {
	Version         int    `json:"version"`
	State           string `json:"state"`
	ReleaseID       string `json:"release_id"`
	ReceiptSHA256   string `json:"receipt_sha256"`
	PublicPath      string `json:"public_path"`
	SHA256          string `json:"sha256"`
	Bytes           int64  `json:"bytes"`
	ObjectKey       string `json:"object_key"`
	ObjectVersionID string `json:"object_version_id"`
}

// PublishRelease verifies and pins every reviewed public asset before writing
// the single release commit receipt. Per-object publication receipts may remain
// after a failed attempt as audit evidence, but their presence never means that
// the release is complete or eligible for activation.
func PublishRelease(ctx context.Context, cfg Config, req ReleaseRequest, store ObjectStore) (ReleaseResult, error) {
	assets, rec, err := validateCompleteRelease(cfg, req)
	if err != nil {
		return ReleaseResult{}, err
	}
	if store == nil {
		return ReleaseResult{}, errors.New("OSS object store is required")
	}
	var unlock func()
	for {
		unlock, err = lockAuditRoot(cfg.AuditRoot)
		if !errors.Is(err, errPublicationBusy) {
			break
		}
		select {
		case <-ctx.Done():
			return ReleaseResult{}, fmt.Errorf("waiting for concurrent OSS publication: %w", ctx.Err())
		case <-time.After(10 * time.Millisecond):
		}
	}
	if err != nil {
		return ReleaseResult{}, err
	}
	defer unlock()
	if _, err := ensureAuditReleaseDir(cfg.AuditRoot, req.ReleaseID); err != nil {
		return ReleaseResult{}, err
	}
	assetRequest := Request{ReleaseID: req.ReleaseID, ReceiptSHA256: req.ReceiptSHA256}
	if err := checkReleaseBinding(cfg.AuditRoot, assetRequest); err != nil {
		return ReleaseResult{}, err
	}
	if err := persistReleaseBinding(cfg.AuditRoot, assetRequest); err != nil {
		return ReleaseResult{}, err
	}
	bucket, err := store.BucketState(ctx)
	if err != nil {
		return ReleaseResult{}, errors.New("could not verify OSS bucket policy")
	}
	if bucket != (BucketState{Region: "cn-beijing", ACL: "private", StorageClass: "Standard", Redundancy: "ZRS", Versioning: "Enabled", Encryption: "AES256"}) {
		return ReleaseResult{}, errors.New("OSS bucket policy does not match the approved private publication boundary")
	}
	published := make([]ReleaseAsset, 0, len(assets))
	for _, item := range assets {
		result, publishErr := publishCompleteAsset(ctx, cfg, req, item, store)
		if publishErr != nil {
			return ReleaseResult{}, fmt.Errorf("complete OSS release was not committed: asset_sha256=%s: %w", item.SHA256, publishErr)
		}
		published = append(published, result)
	}
	sort.Slice(published, func(i, j int) bool { return published[i].PublicPath < published[j].PublicPath })
	commit := releaseCommit{
		Version: 1, State: "release_committed_not_activated", ReleaseID: req.ReleaseID, ReceiptSHA256: req.ReceiptSHA256,
		ManifestSHA256: rec.ManifestSHA256, InventorySHA256: rec.InventorySHA256, TreeSHA256: rec.TreeSHA256,
		AssetCount: len(published), Assets: published,
	}
	commitBytes, err := json.Marshal(commit)
	if err != nil {
		return ReleaseResult{}, errors.New("could not encode OSS release commit receipt")
	}
	commitPath := filepath.Join(cfg.AuditRoot, req.ReleaseID, "release-commit.json")
	if err := persistAtomic(commitPath, commitBytes); err != nil {
		return ReleaseResult{}, errors.New("could not persist OSS release commit receipt")
	}
	return ReleaseResult{
		Version: commit.Version, State: commit.State, ReleaseID: commit.ReleaseID, ReceiptSHA256: commit.ReceiptSHA256,
		ManifestSHA256: commit.ManifestSHA256, InventorySHA256: commit.InventorySHA256, TreeSHA256: commit.TreeSHA256,
		AssetCount: commit.AssetCount, Assets: commit.Assets, ReleaseCommitSHA256: sum(commitBytes),
	}, nil
}

func publishCompleteAsset(ctx context.Context, cfg Config, req ReleaseRequest, item validatedReleaseAsset, store ObjectStore) (ReleaseAsset, error) {
	key := strings.Join([]string{cfg.Prefix, req.ReleaseID, "receipts", req.ReceiptSHA256, "objects", item.SHA256, item.PublicPath}, "/")
	receiptPath, err := releaseObjectReceiptPath(cfg.AuditRoot, req.ReleaseID, item.PublicPath)
	if err != nil {
		return ReleaseAsset{}, err
	}
	var prior *releaseObjectReceipt
	if data, readErr := readExistingRegular(receiptPath); readErr == nil {
		var decoded releaseObjectReceipt
		if decodeErr := decodeExact(data, &decoded); decodeErr != nil || decoded.Version != 1 || decoded.State != "published_not_activated" || decoded.ReleaseID != req.ReleaseID || decoded.ReceiptSHA256 != req.ReceiptSHA256 || decoded.PublicPath != item.PublicPath || decoded.SHA256 != item.SHA256 || decoded.Bytes != item.Bytes || decoded.ObjectKey != key || decoded.ObjectVersionID == "" {
			return ReleaseAsset{}, errors.New("existing complete-release object receipt conflicts with this publication")
		}
		prior = &decoded
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return ReleaseAsset{}, errors.New("could not read complete-release object receipt")
	}
	pinnedVersion := ""
	if prior != nil {
		pinnedVersion = prior.ObjectVersionID
	}
	state, exists, err := store.Head(ctx, key, pinnedVersion)
	if err != nil {
		return ReleaseAsset{}, verificationError("head", key, pinnedVersion, err)
	}
	if prior != nil && !exists {
		return ReleaseAsset{}, errors.New("recorded complete-release OSS object version no longer exists")
	}
	if !exists {
		assetFile, openErr := openCompleteReleaseAsset(cfg.SealedRoot, req.ReleaseID, item.asset)
		if openErr != nil {
			return ReleaseAsset{}, openErr
		}
		hasher := sha256.New()
		stream := &countedReader{reader: io.TeeReader(io.LimitReader(assetFile, item.Bytes), hasher)}
		versionID, putErr := store.Put(ctx, key, stream, item.Bytes, item.SHA256)
		verifyErr := verifyCompleteAssetStream(assetFile, stream.bytesRead, hasher.Sum(nil), item.asset)
		closeErr := assetFile.Close()
		if putErr != nil {
			return ReleaseAsset{}, verificationError("put", key, "unknown", putErr)
		}
		if verifyErr != nil || closeErr != nil {
			return ReleaseAsset{}, verificationError("put_digest", key, "unknown", errors.New("publish-time asset stream does not match the sealed inventory"))
		}
		if versionID == "" {
			return ReleaseAsset{}, verificationError("put_response", key, "missing", errors.New("OSS did not return an immutable object version"))
		}
		pinnedVersion = versionID
		state, exists, err = store.Head(ctx, key, pinnedVersion)
		if err != nil {
			return ReleaseAsset{}, verificationError("head_after_put", key, pinnedVersion, err)
		}
		if !exists {
			return ReleaseAsset{}, verificationError("head_after_put", key, pinnedVersion, errors.New("uploaded version was not found"))
		}
	}
	if state.VersionID == "" || (pinnedVersion != "" && state.VersionID != pinnedVersion) || state.Bytes != item.Bytes || state.Encryption != "AES256" || (state.SHA256 != "" && state.SHA256 != item.SHA256) {
		return ReleaseAsset{}, verificationError("head_metadata", key, pinnedVersion, errors.New("object metadata or immutable version does not match the sealed asset"))
	}
	body, err := store.Get(ctx, key, state.VersionID)
	if err != nil {
		return ReleaseAsset{}, verificationError("get", key, state.VersionID, err)
	}
	readErr := verifyBoundedStream(body, item.Bytes, item.SHA256)
	closeErr := body.Close()
	if readErr != nil || closeErr != nil {
		return ReleaseAsset{}, verificationError("get_digest", key, state.VersionID, errors.New("read-back does not match the sealed asset"))
	}
	if err := store.AnonymousDenied(ctx, key, state.VersionID); err != nil {
		return ReleaseAsset{}, verificationError("anonymous_get", key, state.VersionID, err)
	}
	stored := releaseObjectReceipt{Version: 1, State: "published_not_activated", ReleaseID: req.ReleaseID, ReceiptSHA256: req.ReceiptSHA256, PublicPath: item.PublicPath, SHA256: item.SHA256, Bytes: item.Bytes, ObjectKey: key, ObjectVersionID: state.VersionID}
	storedBytes, _ := json.Marshal(stored)
	if prior != nil {
		stored = *prior
		storedBytes, _ = json.Marshal(stored)
	}
	if err := persistAtomic(receiptPath, storedBytes); err != nil {
		return ReleaseAsset{}, errors.New("could not persist complete-release object receipt")
	}
	return ReleaseAsset{PublicPath: item.PublicPath, SHA256: item.SHA256, Bytes: item.Bytes, ObjectKey: key, ObjectVersionID: stored.ObjectVersionID}, nil
}

func releaseObjectReceiptPath(auditRoot, releaseID, publicPath string) (string, error) {
	directory := filepath.Join(auditRoot, releaseID, "objects")
	created := false
	if err := os.Mkdir(directory, 0o700); err == nil {
		created = true
	} else if !errors.Is(err, os.ErrExist) {
		return "", errors.New("could not create complete-release object audit directory")
	}
	metadata, err := os.Lstat(directory)
	if err != nil || !metadata.IsDir() || metadata.Mode()&os.ModeSymlink != 0 || metadata.Mode().Perm()&0o022 != 0 {
		return "", errors.New("complete-release object audit directory is unsafe")
	}
	if created {
		parent, openErr := os.Open(filepath.Dir(directory))
		if openErr != nil {
			return "", errors.New("could not sync complete-release object audit parent")
		}
		syncErr := parent.Sync()
		closeErr := parent.Close()
		if syncErr == nil {
			syncErr = closeErr
		}
		if syncErr != nil {
			return "", errors.New("could not sync complete-release object audit parent")
		}
	}
	return filepath.Join(directory, sum([]byte(publicPath))+".json"), nil
}

// validateCompleteRelease deliberately reads and hashes the whole local release
// before the first remote call. This keeps malformed or partially sealed input
// from producing even a partial OSS publication attempt.
func validateCompleteRelease(cfg Config, req ReleaseRequest) ([]validatedReleaseAsset, receipt, error) {
	if !releasePattern.MatchString(req.ReleaseID) || !hashPattern.MatchString(req.ReceiptSHA256) {
		return nil, receipt{}, errors.New("sealed release identity is invalid")
	}
	if cfg.Bucket != "henukit" || cfg.Region != "cn-beijing" || cfg.Prefix != "releases" || !filepath.IsAbs(cfg.SealedRoot) || !filepath.IsAbs(cfg.AuditRoot) || pathsOverlap(cfg.SealedRoot, cfg.AuditRoot) {
		return nil, receipt{}, errors.New("OSS publication configuration is invalid")
	}
	releaseDir := filepath.Join(filepath.Clean(cfg.SealedRoot), req.ReleaseID)
	receiptBytes, err := readFixed(releaseDir, "sealed-release.json")
	if err != nil {
		return nil, receipt{}, err
	}
	if sum(receiptBytes) != req.ReceiptSHA256 {
		return nil, receipt{}, errors.New("sealed receipt digest does not match the requested identity")
	}
	var rec receipt
	if err := decodeExact(receiptBytes, &rec); err != nil {
		return nil, receipt{}, fmt.Errorf("sealed receipt is invalid: %w", err)
	}
	if rec.Version != 1 || rec.ReleaseID != req.ReleaseID || rec.Source.SHA != req.ReleaseID[:40] || rec.Source.Repository != approvedSourceRepository || rec.Source.Ref != "refs/heads/main" {
		return nil, receipt{}, errors.New("sealed receipt does not match the release identity")
	}
	manifestBytes, err := readFixed(releaseDir, "manifest.json")
	if err != nil {
		return nil, receipt{}, err
	}
	inventoryBytes, err := readFixed(releaseDir, "inventory.json")
	if err != nil {
		return nil, receipt{}, err
	}
	if sum(manifestBytes) != rec.ManifestSHA256 || rec.ManifestSHA256[:16] != req.ReleaseID[41:] || sum(inventoryBytes) != rec.InventorySHA256 {
		return nil, receipt{}, errors.New("sealed metadata digests do not match the receipt")
	}
	var inv inventory
	if err := decodeExact(inventoryBytes, &inv); err != nil {
		return nil, receipt{}, fmt.Errorf("sealed inventory is invalid: %w", err)
	}
	if inv.Version != 1 || inv.Source != rec.Source || inv.ManifestSHA256 != rec.ManifestSHA256 || inv.TreeSHA256 != rec.TreeSHA256 || len(inv.Assets) != rec.ReviewedAssets {
		return nil, receipt{}, errors.New("sealed inventory does not match the receipt")
	}
	if err := validateCompleteInventory(inv); err != nil {
		return nil, receipt{}, err
	}
	if err := validateCompleteManifest(manifestBytes, inv.Assets); err != nil {
		return nil, receipt{}, err
	}
	validated := make([]validatedReleaseAsset, 0, len(inv.Assets))
	for _, item := range inv.Assets {
		assetFile, openErr := openCompleteReleaseAsset(cfg.SealedRoot, req.ReleaseID, item)
		if openErr != nil {
			return nil, receipt{}, openErr
		}
		verifyErr := verifyBoundedStream(assetFile, item.Bytes, item.SHA256)
		closeErr := assetFile.Close()
		if verifyErr != nil || closeErr != nil {
			return nil, receipt{}, errors.New("sealed asset does not match its inventory")
		}
		validated = append(validated, validatedReleaseAsset{asset: item})
	}
	return validated, rec, nil
}

type countedReader struct {
	reader    io.Reader
	bytesRead int64
}

func (r *countedReader) Read(buffer []byte) (int, error) {
	n, err := r.reader.Read(buffer)
	r.bytesRead += int64(n)
	return n, err
}

func openCompleteReleaseAsset(sealedRoot, releaseID string, item asset) (*os.File, error) {
	releaseDir := filepath.Join(filepath.Clean(sealedRoot), releaseID)
	rootInfo, err := os.Lstat(releaseDir)
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("sealed release root must be a real directory")
	}
	segments, err := safeSegments(item.PublicPath)
	if err != nil {
		return nil, errors.New("sealed asset path is unsafe")
	}
	cursor := filepath.Join(releaseDir, "public")
	publicInfo, err := os.Lstat(cursor)
	if err != nil || !publicInfo.IsDir() || publicInfo.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("sealed asset path contains an unsafe directory")
	}
	for _, segment := range segments[:len(segments)-1] {
		cursor = filepath.Join(cursor, segment)
		info, walkErr := os.Lstat(cursor)
		if walkErr != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, errors.New("sealed asset path contains an unsafe directory")
		}
	}
	path := filepath.Join(releaseDir, "public", filepath.FromSlash(item.PublicPath))
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("sealed asset must be a regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.New("could not open sealed asset")
	}
	opened, statErr := file.Stat()
	if statErr != nil || !os.SameFile(info, opened) || opened.Size() != item.Bytes {
		file.Close()
		return nil, errors.New("sealed asset changed while being opened")
	}
	return file, nil
}

func verifyCompleteAssetStream(file *os.File, bytesRead int64, digestBytes []byte, item asset) error {
	if bytesRead != item.Bytes || hex.EncodeToString(digestBytes) != item.SHA256 {
		return errors.New("asset stream digest does not match")
	}
	extra, err := io.CopyN(io.Discard, file, 1)
	if extra != 0 || !errors.Is(err, io.EOF) {
		return errors.New("asset stream size does not match")
	}
	return nil
}

func verifyBoundedStream(reader io.Reader, expectedBytes int64, expectedSHA256 string) error {
	hasher := sha256.New()
	readBytes, err := io.CopyN(hasher, reader, expectedBytes)
	if err != nil || readBytes != expectedBytes || hex.EncodeToString(hasher.Sum(nil)) != expectedSHA256 {
		return errors.New("stream digest does not match")
	}
	extra, err := io.CopyN(io.Discard, reader, 1)
	if extra != 0 || !errors.Is(err, io.EOF) {
		return errors.New("stream size does not match")
	}
	return nil
}

func validateCompleteInventory(inv inventory) error {
	paths := map[string]bool{}
	entries := make([]treeEntry, 0, len(inv.Assets))
	for _, item := range inv.Assets {
		if _, err := safeSegments(item.PublicPath); err != nil || !hashPattern.MatchString(item.SHA256) || item.Bytes < 0 || paths[item.PublicPath] {
			return errors.New("sealed inventory contains an unsafe or duplicate asset path")
		}
		paths[item.PublicPath] = true
		entries = append(entries, treeEntry{Path: "public/" + item.PublicPath, Bytes: item.Bytes, SHA256: item.SHA256})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	if sum(canonicalJSON(entries)) != inv.TreeSHA256 {
		return errors.New("sealed inventory tree digest is invalid")
	}
	return nil
}

func validateCompleteManifest(data []byte, inventoryAssets []asset) error {
	var document manifest
	if err := json.Unmarshal(data, &document); err != nil {
		return errors.New("sealed manifest is invalid")
	}
	paths := map[string]bool{}
	approved := map[string]asset{}
	for _, subject := range document.Subjects {
		for _, item := range subject.Assets {
			if _, err := safeSegments(item.PublicPath); err != nil || !hashPattern.MatchString(item.SHA256) || item.Bytes < 0 || paths[item.PublicPath] {
				return errors.New("sealed manifest contains an unsafe or duplicate asset path")
			}
			paths[item.PublicPath] = true
			if !strings.HasPrefix(item.Role, "待复核") {
				approved[item.PublicPath] = asset{PublicPath: item.PublicPath, Bytes: item.Bytes, SHA256: item.SHA256}
			}
		}
	}
	if len(approved) != len(inventoryAssets) {
		return errors.New("sealed manifest and inventory do not describe the same reviewed assets")
	}
	for _, item := range inventoryAssets {
		if approved[item.PublicPath] != item {
			return errors.New("sealed manifest and inventory do not describe the same reviewed assets")
		}
	}
	return nil
}
