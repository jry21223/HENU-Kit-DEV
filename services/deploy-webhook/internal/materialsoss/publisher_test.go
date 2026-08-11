package materialsoss

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeStore struct {
	bucket          BucketState
	objects         map[string][]byte
	putErr          error
	getErr          error
	getBytes        []byte
	headOverride    *ObjectState
	headErrAfterPut error
	anonymousErr    error
	headVersions    []string
	puts            int
}

func (s *fakeStore) BucketState(context.Context) (BucketState, error) { return s.bucket, nil }
func (s *fakeStore) Head(_ context.Context, key, versionID string) (ObjectState, bool, error) {
	s.headVersions = append(s.headVersions, versionID)
	b, ok := s.objects[key]
	if !ok {
		return ObjectState{}, false, nil
	}
	if s.headErrAfterPut != nil {
		return ObjectState{}, false, s.headErrAfterPut
	}
	if versionID != "" && versionID != "version-1" {
		return ObjectState{}, false, errors.New("wrong version")
	}
	if s.headOverride != nil {
		return *s.headOverride, true, nil
	}
	return ObjectState{Bytes: int64(len(b)), SHA256: digest(b), Encryption: "AES256", VersionID: "version-1"}, true, nil
}
func (s *fakeStore) Put(_ context.Context, key string, body io.Reader, _ int64, _ string) (string, error) {
	s.puts++
	if s.putErr != nil {
		return "", s.putErr
	}
	b, err := io.ReadAll(body)
	if err != nil {
		return "", err
	}
	s.objects[key] = b
	return "version-1", nil
}
func (s *fakeStore) Get(_ context.Context, key, versionID string) (io.ReadCloser, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	if versionID != "version-1" {
		return nil, errors.New("wrong version")
	}
	if s.getBytes != nil {
		return io.NopCloser(bytes.NewReader(s.getBytes)), nil
	}
	return io.NopCloser(bytes.NewReader(s.objects[key])), nil
}
func (s *fakeStore) AnonymousDenied(_ context.Context, _ string, versionID string) error {
	if versionID != "version-1" {
		return errors.New("wrong anonymous version")
	}
	return s.anonymousErr
}

func digest(b []byte) string { sum := sha256.Sum256(b); return hex.EncodeToString(sum[:]) }

func fixture(t *testing.T) (Config, Request, []byte) {
	t.Helper()
	root := t.TempDir()
	audit := t.TempDir()
	body := []byte("reviewed public material\n")
	assetHash := digest(body)
	manifest, _ := json.Marshal(map[string]any{
		"version": 1,
		"subjects": []any{map[string]any{
			"name": "软件工程",
			"assets": []any{map[string]any{
				"role": "复习讲义", "publicPath": "软件工程/复习讲义.pdf", "bytes": len(body), "sha256": assetHash,
			}},
		}},
	})
	manifestHash := digest(manifest)
	releaseID := "0123456789abcdef0123456789abcdef01234567-" + manifestHash[:16]
	tree := canonicalJSON([]struct {
		Path   string `json:"path"`
		Bytes  int    `json:"bytes"`
		SHA256 string `json:"sha256"`
	}{{"public/软件工程/复习讲义.pdf", len(body), assetHash}})
	inventory, _ := json.Marshal(map[string]any{
		"version":         1,
		"source":          map[string]any{"repository": approvedSourceRepository, "ref": "refs/heads/main", "sha": releaseID[:40]},
		"manifest_sha256": manifestHash,
		"assets":          []any{map[string]any{"public_path": "软件工程/复习讲义.pdf", "bytes": len(body), "sha256": assetHash}},
		"slides":          map[string]any{"status": "deferred", "source_slide_assets": 0},
		"tree_sha256":     digest(tree),
	})
	receipt, _ := json.Marshal(map[string]any{
		"version": 1, "release_id": releaseID,
		"source":          map[string]any{"repository": approvedSourceRepository, "ref": "refs/heads/main", "sha": releaseID[:40]},
		"manifest_sha256": manifestHash, "inventory_sha256": digest(inventory), "tree_sha256": digest(tree),
		"reviewed_assets": 1, "slides": map[string]any{"status": "deferred", "source_slide_assets": 0},
	})
	release := filepath.Join(root, releaseID)
	if err := os.MkdirAll(filepath.Join(release, "public", "软件工程"), 0o700); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{"manifest.json": manifest, "inventory.json": inventory, "sealed-release.json": receipt, "public/软件工程/复习讲义.pdf": body} {
		if err := os.WriteFile(filepath.Join(release, name), data, 0o400); err != nil {
			t.Fatal(err)
		}
	}
	return Config{SealedRoot: root, AuditRoot: audit, Bucket: "henukit", Region: "cn-beijing", Prefix: "releases"}, Request{ReleaseID: releaseID, ReceiptSHA256: digest(receipt), AssetSHA256: assetHash}, body
}

func validBucket() BucketState {
	return BucketState{Region: "cn-beijing", ACL: "private", StorageClass: "Standard", Redundancy: "ZRS", Versioning: "Enabled", Encryption: "AES256"}
}

func TestPublishCanaryUploadsVerifiesAndReplaysWithoutActivation(t *testing.T) {
	cfg, req, body := fixture(t)
	store := &fakeStore{bucket: validBucket(), objects: map[string][]byte{}}
	result, err := Publish(context.Background(), cfg, req, store)
	if err != nil {
		t.Fatal(err)
	}
	if result.State != "published_not_activated" {
		t.Fatalf("state=%q", result.State)
	}
	if result.ObjectVersionID != "version-1" {
		t.Fatalf("version=%q", result.ObjectVersionID)
	}
	if store.puts != 1 {
		t.Fatalf("puts=%d", store.puts)
	}
	if got := store.objects[result.ObjectKey]; !bytes.Equal(got, body) {
		t.Fatal("uploaded bytes differ")
	}
	if _, err := os.Stat(filepath.Join(cfg.AuditRoot, req.ReleaseID, req.AssetSHA256+".json")); err != nil {
		t.Fatal(err)
	}

	again, err := Publish(context.Background(), cfg, req, store)
	if err != nil {
		t.Fatal(err)
	}
	if again != result {
		t.Fatalf("replay changed result: %#v %#v", result, again)
	}
	if store.puts != 1 {
		t.Fatalf("replay uploaded again: puts=%d", store.puts)
	}
	if got := store.headVersions[len(store.headVersions)-1]; got != "version-1" {
		t.Fatalf("replay did not pin recorded version: %q", got)
	}
	different := req
	different.ReceiptSHA256 = strings.Repeat("e", 64)
	if _, err := Publish(context.Background(), cfg, different, store); err == nil {
		t.Fatal("different receipt occupied an existing release identity")
	}
	if store.puts != 1 {
		t.Fatalf("conflicting receipt wrote OSS: %d", store.puts)
	}
}

func TestPublishFailsClosedBeforePut(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config, *Request, *fakeStore)
	}{
		{"receipt mismatch", func(_ *Config, r *Request, _ *fakeStore) {
			r.ReceiptSHA256 = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		}},
		{"asset absent", func(_ *Config, r *Request, _ *fakeStore) {
			r.AssetSHA256 = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
		}},
		{"public acl", func(_ *Config, _ *Request, s *fakeStore) { s.bucket.ACL = "public-read" }},
		{"wrong region", func(_ *Config, _ *Request, s *fakeStore) { s.bucket.Region = "cn-hangzhou" }},
		{"wrong storage", func(_ *Config, _ *Request, s *fakeStore) { s.bucket.StorageClass = "IA" }},
		{"wrong redundancy", func(_ *Config, _ *Request, s *fakeStore) { s.bucket.Redundancy = "LRS" }},
		{"versioning off", func(_ *Config, _ *Request, s *fakeStore) { s.bucket.Versioning = "Suspended" }},
		{"encryption off", func(_ *Config, _ *Request, s *fakeStore) { s.bucket.Encryption = "" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, req, _ := fixture(t)
			store := &fakeStore{bucket: validBucket(), objects: map[string][]byte{}}
			tt.mutate(&cfg, &req, store)
			if _, err := Publish(context.Background(), cfg, req, store); err == nil {
				t.Fatal("expected failure")
			}
			if store.puts != 0 {
				t.Fatalf("put occurred: %d", store.puts)
			}
		})
	}
}

func TestPublishDoesNotRecordPartialSuccess(t *testing.T) {
	for _, tc := range []struct {
		name           string
		putErr, getErr error
		configure      func(*fakeStore)
	}{
		{name: "put", putErr: errors.New("put failed")},
		{name: "readback", getErr: errors.New("get failed")},
		{name: "readback hash", configure: func(s *fakeStore) { s.getBytes = []byte("corrupt") }},
		{name: "head metadata", configure: func(s *fakeStore) {
			s.headOverride = &ObjectState{Bytes: 1, SHA256: "bad", Encryption: "AES256", VersionID: "version-1"}
		}},
		{name: "anonymous access", configure: func(s *fakeStore) { s.anonymousErr = errors.New("public") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, req, _ := fixture(t)
			store := &fakeStore{bucket: validBucket(), objects: map[string][]byte{}, putErr: tc.putErr, getErr: tc.getErr}
			if tc.configure != nil {
				tc.configure(store)
			}
			if _, err := Publish(context.Background(), cfg, req, store); err == nil {
				t.Fatal("expected failure")
			}
			if _, err := os.Stat(filepath.Join(cfg.AuditRoot, req.ReleaseID, req.AssetSHA256+".json")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("receipt exists or unexpected error: %v", err)
			}
		})
	}
}

func TestManifestRejectsPendingAndDuplicateAssets(t *testing.T) {
	hash := strings.Repeat("a", 64)
	wanted := asset{PublicPath: "资料/a.pdf", Bytes: 1, SHA256: hash}
	pending, _ := json.Marshal(map[string]any{
		"subjects": []any{map[string]any{
			"assets": []any{map[string]any{"role": "待复核资料", "publicPath": wanted.PublicPath, "bytes": 1, "sha256": hash}},
		}},
	})
	if err := validateManifest(pending, []asset{wanted}, wanted); err == nil {
		t.Fatal("pending asset was accepted")
	}
	duplicate, _ := json.Marshal(map[string]any{
		"subjects": []any{map[string]any{
			"assets": []any{
				map[string]any{"role": "复习讲义", "publicPath": wanted.PublicPath, "bytes": 1, "sha256": hash},
				map[string]any{"role": "待复核资料", "publicPath": wanted.PublicPath, "bytes": 1, "sha256": hash},
			},
		}},
	})
	if err := validateManifest(duplicate, []asset{wanted}, wanted); err == nil {
		t.Fatal("duplicate manifest asset was accepted")
	}
}

func TestPostPutHeadFailurePreservesSafeOrphanEvidence(t *testing.T) {
	cfg, req, _ := fixture(t)
	store := &fakeStore{bucket: validBucket(), objects: map[string][]byte{}, headErrAfterPut: errors.New("operation=head_object category=transport")}
	_, err := Publish(context.Background(), cfg, req, store)
	if err == nil {
		t.Fatal("expected post-upload failure")
	}
	message := err.Error()
	for _, evidence := range []string{"stage=head_after_put", `version_id="version-1"`, "object_key=", "category=transport"} {
		if !strings.Contains(message, evidence) {
			t.Fatalf("missing %q in %q", evidence, message)
		}
	}
	if _, statErr := os.Stat(filepath.Join(cfg.AuditRoot, req.ReleaseID, req.AssetSHA256+".json")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("success receipt exists: %v", statErr)
	}
	different := req
	different.ReceiptSHA256 = strings.Repeat("e", 64)
	store.headErrAfterPut = nil
	if _, secondErr := Publish(context.Background(), cfg, different, store); secondErr == nil {
		t.Fatal("different receipt reused a release after an orphaned upload")
	}
	if store.puts != 1 {
		t.Fatalf("conflicting receipt created another orphan: %d", store.puts)
	}
}

func TestReceiptSourceSHAMustMatchRelease(t *testing.T) {
	cfg, req, _ := fixture(t)
	path := filepath.Join(cfg.SealedRoot, req.ReleaseID, "sealed-release.json")
	var document map[string]any
	bytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(bytes, &document); err != nil {
		t.Fatal(err)
	}
	document["source"].(map[string]any)["sha"] = strings.Repeat("f", 40)
	bytes, _ = json.Marshal(document)
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, bytes, 0o400); err != nil {
		t.Fatal(err)
	}
	req.ReceiptSHA256 = digest(bytes)
	store := &fakeStore{bucket: validBucket(), objects: map[string][]byte{}}
	if _, err := Publish(context.Background(), cfg, req, store); err == nil {
		t.Fatal("mismatched source SHA was accepted")
	}
	if store.puts != 0 {
		t.Fatalf("mismatched source reached OSS: %d", store.puts)
	}
}

func TestCanonicalTreeMatchesNodeSealerBytes(t *testing.T) {
	entries := []treeEntry{{Path: "public/软件&工程/复习讲义.pdf", Bytes: 25, SHA256: strings.Repeat("a", 64)}}
	if got := digest(canonicalJSON(entries)); got != "9616366b326d76b84dbbcb733fcb68a39eece15108cd47a6fff47fb90c867b5d" {
		t.Fatalf("canonical tree digest drifted from #306 sealer: %s", got)
	}
}
