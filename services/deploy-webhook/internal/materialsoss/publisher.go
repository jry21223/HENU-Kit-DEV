package materialsoss

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"
)

var (
	releasePattern = regexp.MustCompile(`^[a-f0-9]{40}-[a-f0-9]{16}$`)
	hashPattern    = regexp.MustCompile(`^[a-f0-9]{64}$`)
)

const approvedSourceRepository = "https://github.com/jry21223/HENU-Final-Review.git"

type Config struct {
	SealedRoot string
	AuditRoot  string
	Bucket     string
	Region     string
	Prefix     string
}

type Request struct {
	ReleaseID     string
	ReceiptSHA256 string
	AssetSHA256   string
}

type BucketState struct {
	Region, ACL, StorageClass, Redundancy, Versioning, Encryption string
}

type ObjectState struct {
	Bytes      int64
	SHA256     string
	Encryption string
	VersionID  string
}

type ObjectStore interface {
	BucketState(context.Context) (BucketState, error)
	Head(context.Context, string, string) (ObjectState, bool, error)
	Put(context.Context, string, io.Reader, int64, string) (string, error)
	Get(context.Context, string, string) (io.ReadCloser, error)
	AnonymousDenied(context.Context, string, string) error
}

type Result struct {
	Version                  int    `json:"version"`
	State                    string `json:"state"`
	ReleaseID                string `json:"release_id"`
	ReceiptSHA256            string `json:"receipt_sha256"`
	AssetSHA256              string `json:"asset_sha256"`
	ObjectKey                string `json:"object_key"`
	ObjectVersionID          string `json:"object_version_id"`
	Bytes                    int64  `json:"bytes"`
	PublicationReceiptSHA256 string `json:"publication_receipt_sha256"`
}

type source struct{ Repository, Ref, SHA string }
type slides struct {
	Status            string `json:"status"`
	SourceSlideAssets int    `json:"source_slide_assets"`
}
type receipt struct {
	Version         int    `json:"version"`
	ReleaseID       string `json:"release_id"`
	Source          source `json:"source"`
	ManifestSHA256  string `json:"manifest_sha256"`
	InventorySHA256 string `json:"inventory_sha256"`
	TreeSHA256      string `json:"tree_sha256"`
	ReviewedAssets  int    `json:"reviewed_assets"`
	Slides          slides `json:"slides"`
}
type asset struct {
	PublicPath string `json:"public_path"`
	Bytes      int64  `json:"bytes"`
	SHA256     string `json:"sha256"`
}
type inventory struct {
	Version        int     `json:"version"`
	Source         source  `json:"source"`
	ManifestSHA256 string  `json:"manifest_sha256"`
	Assets         []asset `json:"assets"`
	Slides         slides  `json:"slides"`
	TreeSHA256     string  `json:"tree_sha256"`
}
type manifest struct {
	Subjects []struct {
		Assets []struct {
			Role, PublicPath, SHA256 string
			Bytes                    int64
		} `json:"assets"`
	} `json:"subjects"`
}
type treeEntry struct {
	Path   string `json:"path"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
}
type storedReceipt struct {
	Version         int    `json:"version"`
	State           string `json:"state"`
	ReleaseID       string `json:"release_id"`
	ReceiptSHA256   string `json:"receipt_sha256"`
	AssetSHA256     string `json:"asset_sha256"`
	ObjectKey       string `json:"object_key"`
	ObjectVersionID string `json:"object_version_id"`
	Bytes           int64  `json:"bytes"`
}

func Publish(ctx context.Context, cfg Config, req Request, store ObjectStore) (Result, error) {
	if store == nil {
		return Result{}, errors.New("OSS object store is required")
	}
	if !releasePattern.MatchString(req.ReleaseID) || !hashPattern.MatchString(req.ReceiptSHA256) || !hashPattern.MatchString(req.AssetSHA256) {
		return Result{}, errors.New("sealed publication identity is invalid")
	}
	if cfg.Bucket != "henukit" || cfg.Region != "cn-beijing" || cfg.Prefix != "releases" || !filepath.IsAbs(cfg.SealedRoot) || !filepath.IsAbs(cfg.AuditRoot) || pathsOverlap(cfg.SealedRoot, cfg.AuditRoot) {
		return Result{}, errors.New("OSS publication configuration is invalid")
	}
	unlock, err := lockAuditRoot(cfg.AuditRoot)
	if err != nil {
		return Result{}, err
	}
	defer unlock()
	if _, err := ensureAuditReleaseDir(cfg.AuditRoot, req.ReleaseID); err != nil {
		return Result{}, err
	}
	if err := checkReleaseBinding(cfg.AuditRoot, req); err != nil {
		return Result{}, err
	}
	releaseDir := filepath.Join(filepath.Clean(cfg.SealedRoot), req.ReleaseID)
	receiptBytes, err := readFixed(releaseDir, "sealed-release.json")
	if err != nil {
		return Result{}, err
	}
	if sum(receiptBytes) != req.ReceiptSHA256 {
		return Result{}, errors.New("sealed receipt digest does not match the requested identity")
	}
	var rec receipt
	if err := decodeExact(receiptBytes, &rec); err != nil {
		return Result{}, fmt.Errorf("sealed receipt is invalid: %w", err)
	}
	if rec.Version != 1 || rec.ReleaseID != req.ReleaseID || rec.Source.SHA != req.ReleaseID[:40] || rec.Source.Repository != approvedSourceRepository || rec.Source.Ref != "refs/heads/main" {
		return Result{}, errors.New("sealed receipt does not match the release identity")
	}

	manifestBytes, err := readFixed(releaseDir, "manifest.json")
	if err != nil {
		return Result{}, err
	}
	inventoryBytes, err := readFixed(releaseDir, "inventory.json")
	if err != nil {
		return Result{}, err
	}
	if sum(manifestBytes) != rec.ManifestSHA256 || rec.ManifestSHA256[:16] != req.ReleaseID[41:] || sum(inventoryBytes) != rec.InventorySHA256 {
		return Result{}, errors.New("sealed metadata digests do not match the receipt")
	}
	var inv inventory
	if err := decodeExact(inventoryBytes, &inv); err != nil {
		return Result{}, fmt.Errorf("sealed inventory is invalid: %w", err)
	}
	if inv.Version != 1 || inv.Source != rec.Source || inv.ManifestSHA256 != rec.ManifestSHA256 || inv.TreeSHA256 != rec.TreeSHA256 || len(inv.Assets) != rec.ReviewedAssets {
		return Result{}, errors.New("sealed inventory does not match the receipt")
	}
	selected, err := validateInventory(inv, req.AssetSHA256)
	if err != nil {
		return Result{}, err
	}
	if err := validateManifest(manifestBytes, inv.Assets, selected); err != nil {
		return Result{}, err
	}
	assetBytes, err := readFixed(releaseDir, filepath.Join("public", filepath.FromSlash(selected.PublicPath)))
	if err != nil {
		return Result{}, err
	}
	if int64(len(assetBytes)) != selected.Bytes || sum(assetBytes) != selected.SHA256 {
		return Result{}, errors.New("sealed asset does not match its inventory")
	}
	// Reserve the locally proven release-to-receipt identity before any remote
	// write. This is not a success receipt or activation record; it prevents a
	// different receipt from creating a second orphan under the same release.
	if err := persistReleaseBinding(cfg.AuditRoot, req); err != nil {
		return Result{}, err
	}

	bucket, err := store.BucketState(ctx)
	if err != nil {
		return Result{}, errors.New("could not verify OSS bucket policy")
	}
	if bucket != (BucketState{Region: "cn-beijing", ACL: "private", StorageClass: "Standard", Redundancy: "ZRS", Versioning: "Enabled", Encryption: "AES256"}) {
		return Result{}, errors.New("OSS bucket policy does not match the approved private publication boundary")
	}
	key := strings.Join([]string{cfg.Prefix, req.ReleaseID, "receipts", req.ReceiptSHA256, "objects", req.AssetSHA256, selected.PublicPath}, "/")
	prior, priorBytes, err := loadPublicationReceipt(cfg.AuditRoot, req, key, selected.Bytes)
	if err != nil {
		return Result{}, err
	}
	pinnedVersion := ""
	if prior != nil {
		pinnedVersion = prior.ObjectVersionID
	}
	state, exists, err := store.Head(ctx, key, pinnedVersion)
	if err != nil {
		return Result{}, verificationError("head", key, pinnedVersion, err)
	}
	if prior != nil && !exists {
		return Result{}, errors.New("recorded OSS canary object version no longer exists")
	}
	if !exists {
		versionID, putErr := store.Put(ctx, key, bytes.NewReader(assetBytes), selected.Bytes, selected.SHA256)
		if putErr != nil {
			return Result{}, verificationError("put", key, "unknown", putErr)
		}
		if versionID == "" {
			return Result{}, verificationError("put_response", key, "missing", errors.New("OSS did not return an immutable object version"))
		}
		pinnedVersion = versionID
		state, exists, err = store.Head(ctx, key, versionID)
		if err != nil {
			return Result{}, verificationError("head_after_put", key, versionID, err)
		}
		if !exists {
			return Result{}, verificationError("head_after_put", key, versionID, errors.New("uploaded version was not found"))
		}
	}
	if pinnedVersion != "" && state.VersionID != pinnedVersion {
		return Result{}, verificationError("head_version", key, pinnedVersion, errors.New("OSS returned a different object version"))
	}
	if state.Bytes != selected.Bytes || state.Encryption != "AES256" || (state.SHA256 != "" && state.SHA256 != selected.SHA256) {
		return Result{}, verificationError("head_metadata", key, state.VersionID, errors.New("object metadata does not match the sealed asset"))
	}
	if state.VersionID == "" {
		return Result{}, errors.New("OSS canary object has no immutable version identity")
	}
	body, err := store.Get(ctx, key, state.VersionID)
	if err != nil {
		return Result{}, verificationError("get", key, state.VersionID, err)
	}
	readback, readErr := io.ReadAll(body)
	closeErr := body.Close()
	if readErr != nil || closeErr != nil || int64(len(readback)) != selected.Bytes || sum(readback) != selected.SHA256 {
		return Result{}, verificationError("get_digest", key, state.VersionID, errors.New("read-back does not match the sealed asset"))
	}
	if err := store.AnonymousDenied(ctx, key, state.VersionID); err != nil {
		return Result{}, verificationError("anonymous_get", key, state.VersionID, err)
	}

	stored := storedReceipt{Version: 1, State: "published_not_activated", ReleaseID: req.ReleaseID, ReceiptSHA256: req.ReceiptSHA256, AssetSHA256: req.AssetSHA256, ObjectKey: key, ObjectVersionID: state.VersionID, Bytes: selected.Bytes}
	storedBytes, _ := json.Marshal(stored)
	if prior != nil {
		stored = *prior
		storedBytes = priorBytes
	}
	result := Result{Version: 1, State: stored.State, ReleaseID: req.ReleaseID, ReceiptSHA256: req.ReceiptSHA256, AssetSHA256: req.AssetSHA256, ObjectKey: key, ObjectVersionID: state.VersionID, Bytes: selected.Bytes, PublicationReceiptSHA256: sum(storedBytes)}
	if err := persistReceipt(cfg.AuditRoot, req, storedBytes); err != nil {
		return Result{}, err
	}
	return result, nil
}

func verificationError(stage, key, versionID string, cause error) error {
	diagnostic := "verification_failed"
	if cause != nil {
		diagnostic = cause.Error()
	}
	return fmt.Errorf("OSS canary verification failed: stage=%s object_key=%q version_id=%q diagnostic=%s", stage, key, versionID, diagnostic)
}

func validateInventory(inv inventory, wanted string) (asset, error) {
	paths, hashes := map[string]bool{}, map[string]bool{}
	entries := make([]treeEntry, 0, len(inv.Assets))
	var selected asset
	found := 0
	for _, a := range inv.Assets {
		if _, err := safeSegments(a.PublicPath); err != nil || !hashPattern.MatchString(a.SHA256) || a.Bytes < 0 || paths[a.PublicPath] || hashes[a.SHA256] {
			return asset{}, errors.New("sealed inventory contains an unsafe or duplicate asset")
		}
		paths[a.PublicPath] = true
		hashes[a.SHA256] = true
		entries = append(entries, treeEntry{Path: "public/" + a.PublicPath, Bytes: a.Bytes, SHA256: a.SHA256})
		if a.SHA256 == wanted {
			selected = a
			found++
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	b := canonicalJSON(entries)
	if sum(b) != inv.TreeSHA256 {
		return asset{}, errors.New("sealed inventory tree digest is invalid")
	}
	if found != 1 {
		return asset{}, errors.New("requested asset digest is not unique in the sealed inventory")
	}
	return selected, nil
}

func validateManifest(data []byte, inventoryAssets []asset, selected asset) error {
	var doc manifest
	if err := json.Unmarshal(data, &doc); err != nil {
		return errors.New("sealed manifest is invalid")
	}
	paths, hashes := map[string]bool{}, map[string]bool{}
	approved := map[string]asset{}
	selectedApproved := false
	for _, s := range doc.Subjects {
		for _, a := range s.Assets {
			if _, err := safeSegments(a.PublicPath); err != nil || !hashPattern.MatchString(a.SHA256) || a.Bytes < 0 || paths[a.PublicPath] || hashes[a.SHA256] {
				return errors.New("sealed manifest contains an unsafe or duplicate asset")
			}
			paths[a.PublicPath] = true
			hashes[a.SHA256] = true
			if strings.HasPrefix(a.Role, "待复核") {
				if a.SHA256 == selected.SHA256 {
					return errors.New("requested asset is still pending review")
				}
				continue
			}
			approved[a.SHA256] = asset{PublicPath: a.PublicPath, Bytes: a.Bytes, SHA256: a.SHA256}
			if a.SHA256 == selected.SHA256 {
				selectedApproved = true
			}
		}
	}
	if !selectedApproved || len(approved) != len(inventoryAssets) {
		return errors.New("sealed manifest and inventory do not describe the same reviewed assets")
	}
	for _, a := range inventoryAssets {
		if approved[a.SHA256] != a {
			return errors.New("sealed manifest and inventory do not describe the same reviewed assets")
		}
	}
	return nil
}

func safeSegments(path string) ([]string, error) {
	if path == "" || strings.ContainsAny(path, "\\\x00") || strings.HasPrefix(path, "/") {
		return nil, errors.New("unsafe path")
	}
	parts := strings.Split(path, "/")
	for _, p := range parts {
		if p == "" || p == "." || p == ".." || strings.HasPrefix(p, ".") {
			return nil, errors.New("unsafe path")
		}
	}
	return parts, nil
}
func pathsOverlap(left, right string) bool {
	left = filepath.Clean(left)
	right = filepath.Clean(right)
	if left == right {
		return true
	}
	leftToRight, err1 := filepath.Rel(left, right)
	rightToLeft, err2 := filepath.Rel(right, left)
	return (err1 == nil && leftToRight != ".." && !strings.HasPrefix(leftToRight, ".."+string(filepath.Separator))) || (err2 == nil && rightToLeft != ".." && !strings.HasPrefix(rightToLeft, ".."+string(filepath.Separator)))
}
func readFixed(root, relative string) ([]byte, error) {
	rootInfo, err := os.Lstat(root)
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("sealed release root must be a real directory")
	}
	full := filepath.Join(root, relative)
	rel, err := filepath.Rel(root, full)
	if err != nil || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, errors.New("sealed path escapes the release")
	}
	cursor := root
	parts := strings.Split(rel, string(filepath.Separator))
	for _, part := range parts[:len(parts)-1] {
		cursor = filepath.Join(cursor, part)
		info, walkErr := os.Lstat(cursor)
		if walkErr != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, errors.New("sealed path contains an unsafe directory")
		}
	}
	info, err := os.Lstat(full)
	if err != nil {
		return nil, fmt.Errorf("read sealed file: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("sealed file must be regular")
	}
	file, err := os.Open(full)
	if err != nil {
		return nil, errors.New("could not open sealed file")
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return nil, errors.New("sealed file changed while being opened")
	}
	return io.ReadAll(file)
}
func decodeExact(data []byte, out any) error {
	d := json.NewDecoder(strings.NewReader(string(data)))
	d.DisallowUnknownFields()
	if err := d.Decode(out); err != nil {
		return err
	}
	var trailing any
	if err := d.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("trailing JSON")
	}
	return nil
}
func sum(b []byte) string { v := sha256.Sum256(b); return hex.EncodeToString(v[:]) }
func canonicalJSON(value any) []byte {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(value)
	return buffer.Bytes()
}

func releaseBindingPath(root string, req Request) string {
	return filepath.Join(root, req.ReleaseID, "release-receipt.sha256")
}
func checkReleaseBinding(root string, req Request) error {
	b, err := readExistingRegular(releaseBindingPath(root, req))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return errors.New("could not read OSS release receipt binding")
	}
	if string(b) != req.ReceiptSHA256+"\n" {
		return errors.New("a different sealed receipt already occupies this release identity")
	}
	return nil
}
func persistReleaseBinding(root string, req Request) error {
	_, err := ensureAuditReleaseDir(root, req.ReleaseID)
	if err != nil {
		return err
	}
	path := releaseBindingPath(root, req)
	if err := persistAtomic(path, []byte(req.ReceiptSHA256+"\n")); err != nil {
		return errors.New("could not persist OSS release receipt binding")
	}
	return nil
}
func loadPublicationReceipt(root string, req Request, key string, size int64) (*storedReceipt, []byte, error) {
	b, err := readExistingRegular(filepath.Join(root, req.ReleaseID, req.AssetSHA256+".json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, errors.New("could not read OSS publication receipt")
	}
	var stored storedReceipt
	if err := decodeExact(b, &stored); err != nil || stored.Version != 1 || stored.State != "published_not_activated" || stored.ReleaseID != req.ReleaseID || stored.ReceiptSHA256 != req.ReceiptSHA256 || stored.AssetSHA256 != req.AssetSHA256 || stored.ObjectKey != key || stored.ObjectVersionID == "" || stored.Bytes != size {
		return nil, nil, errors.New("existing OSS publication receipt conflicts with this publication")
	}
	return &stored, b, nil
}
func persistReceipt(root string, req Request, data []byte) error {
	dir, err := ensureAuditReleaseDir(root, req.ReleaseID)
	if err != nil {
		return err
	}
	path := filepath.Join(dir, req.AssetSHA256+".json")
	if err := persistAtomic(path, data); err != nil {
		return errors.New("could not persist OSS publication receipt")
	}
	return nil
}

func persistAtomic(path string, data []byte) error {
	if old, err := readExistingRegular(path); err == nil {
		if bytes.Equal(old, data) {
			return nil
		}
		return errors.New("conflicting file")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".receipt.*")
	if err != nil {
		return errors.New("could not stage OSS publication receipt")
	}
	name := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(name)
		}
	}()
	if err := tmp.Chmod(0o600); err == nil {
		_, err = tmp.Write(data)
	}
	if err == nil {
		err = tmp.Sync()
	}
	closeErr := tmp.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Link(name, path)
	}
	if errors.Is(err, os.ErrExist) {
		old, readErr := readExistingRegular(path)
		if readErr != nil || !bytes.Equal(old, data) {
			return errors.New("conflicting file")
		}
		return nil
	}
	if err != nil {
		return err
	}
	if err := os.Remove(name); err != nil {
		return err
	}
	cleanup = false
	directory, err := os.Open(dir)
	if err != nil {
		return err
	}
	err = directory.Sync()
	closeErr = directory.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return nil
}

func ensureAuditReleaseDir(root, releaseID string) (string, error) {
	rootInfo, err := os.Lstat(root)
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("OSS publication audit root must be a real directory")
	}
	dir := filepath.Join(root, releaseID)
	created := false
	if err := os.Mkdir(dir, 0o700); err == nil {
		created = true
	} else if !errors.Is(err, os.ErrExist) {
		return "", errors.New("could not create OSS publication audit directory")
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o022 != 0 {
		return "", errors.New("OSS publication audit release directory is unsafe")
	}
	if created {
		parent, openErr := os.Open(root)
		if openErr != nil {
			return "", errors.New("could not sync OSS publication audit root")
		}
		syncErr := parent.Sync()
		closeErr := parent.Close()
		if syncErr == nil {
			syncErr = closeErr
		}
		if syncErr != nil {
			return "", errors.New("could not sync OSS publication audit root")
		}
	}
	return dir, nil
}

func lockAuditRoot(root string) (func(), error) {
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("OSS publication audit root must be a real directory")
	}
	lock, err := os.OpenFile(filepath.Join(root, ".publish.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, errors.New("could not open OSS publication lock")
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		lock.Close()
		return nil, errors.New("another OSS canary publication is already running")
	}
	return func() { _ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN); _ = lock.Close() }, nil
}

func readExistingRegular(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("OSS publication receipt must be a regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return nil, errors.New("OSS publication receipt changed while being opened")
	}
	return io.ReadAll(file)
}
