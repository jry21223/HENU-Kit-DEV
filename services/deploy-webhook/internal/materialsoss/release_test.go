package materialsoss

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
)

type releaseFakeStore struct {
	mu          sync.Mutex
	objects     map[string][]byte
	versions    map[string]string
	puts        int
	failGetAt   int
	gets        int
	bucketHook  func()
	getOverride func([]byte) io.ReadCloser
}

func (s *releaseFakeStore) BucketState(context.Context) (BucketState, error) {
	if s.bucketHook != nil {
		s.bucketHook()
	}
	return validBucket(), nil
}
func (s *releaseFakeStore) Head(_ context.Context, key, version string) (ObjectState, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	body, ok := s.objects[key]
	if !ok {
		return ObjectState{}, false, nil
	}
	actual := s.versions[key]
	if version != "" && version != actual {
		return ObjectState{}, false, nil
	}
	return ObjectState{Bytes: int64(len(body)), SHA256: digest(body), Encryption: "AES256", VersionID: actual}, true, nil
}
func (s *releaseFakeStore) Put(_ context.Context, key string, body io.Reader, _ int64, _ string) (string, error) {
	b, err := io.ReadAll(body)
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.puts++
	version := "version-" + digest(b)[:16]
	s.objects[key] = b
	s.versions[key] = version
	return version, nil
}
func (s *releaseFakeStore) Get(_ context.Context, key, version string) (io.ReadCloser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gets++
	if s.failGetAt > 0 && s.gets == s.failGetAt {
		return nil, errors.New("deterministic readback failure")
	}
	if s.versions[key] != version {
		return nil, errors.New("wrong version")
	}
	body := append([]byte(nil), s.objects[key]...)
	if s.getOverride != nil {
		return s.getOverride(body), nil
	}
	return io.NopCloser(bytes.NewReader(body)), nil
}
func (s *releaseFakeStore) AnonymousDenied(context.Context, string, string) error { return nil }

type releaseFixtureInput struct{ path, role, body string }

type oversizedReadCloser struct {
	prefix         *bytes.Reader
	extraRemaining int64
	bytesRead      int64
}

func (r *oversizedReadCloser) Read(p []byte) (int, error) {
	if r.prefix.Len() > 0 {
		n, err := r.prefix.Read(p)
		r.bytesRead += int64(n)
		return n, err
	}
	if r.extraRemaining == 0 {
		return 0, io.EOF
	}
	n := int64(len(p))
	if n > r.extraRemaining {
		n = r.extraRemaining
	}
	for index := range p[:n] {
		p[index] = 'x'
	}
	r.extraRemaining -= n
	r.bytesRead += n
	return int(n), nil
}

func (*oversizedReadCloser) Close() error { return nil }

func releaseFixture(t *testing.T) (Config, ReleaseRequest, []asset) {
	return releaseFixtureWithInputs(t, []releaseFixtureInput{
		{"软件工程/讲义.pdf", "复习讲义", "first reviewed material\n"},
		{"高等数学/题库.pdf", "题库", "second reviewed material\n"},
	}, true)
}

func releaseFixtureWithInputs(t *testing.T, inputs []releaseFixtureInput, includePending bool) (Config, ReleaseRequest, []asset) {
	t.Helper()
	root, audit := t.TempDir(), t.TempDir()
	sourceSHA := strings.Repeat("1", 40)
	manifestAssets := make([]any, 0, len(inputs)+1)
	assets := make([]asset, 0, len(inputs))
	for _, input := range inputs {
		body := []byte(input.body)
		a := asset{PublicPath: input.path, Bytes: int64(len(body)), SHA256: digest(body)}
		assets = append(assets, a)
		manifestAssets = append(manifestAssets, map[string]any{"role": input.role, "publicPath": input.path, "bytes": len(body), "sha256": a.SHA256})
	}
	if includePending {
		manifestAssets = append(manifestAssets, map[string]any{"role": "待复核答案", "publicPath": "待复核/答案.pdf", "bytes": 1, "sha256": strings.Repeat("f", 64)})
	}
	manifestBytes, _ := json.Marshal(map[string]any{"version": 1, "subjects": []any{map[string]any{"name": "公开资料", "assets": manifestAssets}}})
	manifestSHA := digest(manifestBytes)
	releaseID := sourceSHA + "-" + manifestSHA[:16]
	entries := make([]treeEntry, 0, len(assets))
	for _, a := range assets {
		entries = append(entries, treeEntry{Path: "public/" + a.PublicPath, Bytes: a.Bytes, SHA256: a.SHA256})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	treeSHA := digest(canonicalJSON(entries))
	src := map[string]any{"repository": approvedSourceRepository, "ref": "refs/heads/main", "sha": sourceSHA}
	inventoryBytes, _ := json.Marshal(map[string]any{"version": 1, "source": src, "manifest_sha256": manifestSHA, "assets": assets, "slides": map[string]any{"status": "deferred", "source_slide_assets": 0}, "tree_sha256": treeSHA})
	receiptBytes, _ := json.Marshal(map[string]any{"version": 1, "release_id": releaseID, "source": src, "manifest_sha256": manifestSHA, "inventory_sha256": digest(inventoryBytes), "tree_sha256": treeSHA, "reviewed_assets": len(assets), "slides": map[string]any{"status": "deferred", "source_slide_assets": 0}})
	releaseRoot := filepath.Join(root, releaseID)
	if err := os.MkdirAll(releaseRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, a := range assets {
		if err := os.MkdirAll(filepath.Join(releaseRoot, "public", filepath.Dir(filepath.FromSlash(a.PublicPath))), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for name, data := range map[string][]byte{"manifest.json": manifestBytes, "inventory.json": inventoryBytes, "sealed-release.json": receiptBytes} {
		if err := os.WriteFile(filepath.Join(releaseRoot, name), data, 0o400); err != nil {
			t.Fatal(err)
		}
	}
	for i, a := range assets {
		if err := os.WriteFile(filepath.Join(releaseRoot, "public", filepath.FromSlash(a.PublicPath)), []byte(inputs[i].body), 0o400); err != nil {
			t.Fatal(err)
		}
	}
	return Config{SealedRoot: root, AuditRoot: audit, Bucket: "henukit", Region: "cn-beijing", Prefix: "releases"}, ReleaseRequest{ReleaseID: releaseID, ReceiptSHA256: digest(receiptBytes)}, assets
}

func TestPublishReleaseCommitsOnlyAfterEveryReviewedAssetAndReplays(t *testing.T) {
	cfg, req, assets := releaseFixture(t)
	store := &releaseFakeStore{objects: map[string][]byte{}, versions: map[string]string{}}
	result, err := PublishRelease(context.Background(), cfg, req, store)
	if err != nil {
		t.Fatal(err)
	}
	if result.State != "release_committed_not_activated" || result.AssetCount != len(assets) || len(result.Assets) != len(assets) {
		t.Fatalf("unexpected result: %#v", result)
	}
	if store.puts != len(assets) {
		t.Fatalf("puts=%d", store.puts)
	}
	commitPath := filepath.Join(cfg.AuditRoot, req.ReleaseID, "release-commit.json")
	commit, err := os.ReadFile(commitPath)
	if err != nil || digest(commit) != result.ReleaseCommitSHA256 {
		t.Fatalf("commit receipt missing or mismatched: %v", err)
	}
	again, err := PublishRelease(context.Background(), cfg, req, store)
	if err != nil {
		t.Fatal(err)
	}
	if again.ReleaseCommitSHA256 != result.ReleaseCommitSHA256 || store.puts != len(assets) {
		t.Fatalf("replay drifted: %#v puts=%d", again, store.puts)
	}
}

func TestPublishReleaseFailureHasNoCommitReceipt(t *testing.T) {
	cfg, req, _ := releaseFixture(t)
	store := &releaseFakeStore{objects: map[string][]byte{}, versions: map[string]string{}, failGetAt: 2}
	if _, err := PublishRelease(context.Background(), cfg, req, store); err == nil {
		t.Fatal("expected failure")
	}
	if _, err := os.Stat(filepath.Join(cfg.AuditRoot, req.ReleaseID, "release-commit.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("partial release was committed: %v", err)
	}
}

func TestPublishReleaseValidatesEveryLocalAssetBeforeOSS(t *testing.T) {
	cfg, req, assets := releaseFixture(t)
	path := filepath.Join(cfg.SealedRoot, req.ReleaseID, "public", filepath.FromSlash(assets[1].PublicPath))
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("corrupt"), 0o400); err != nil {
		t.Fatal(err)
	}
	store := &releaseFakeStore{objects: map[string][]byte{}, versions: map[string]string{}}
	if _, err := PublishRelease(context.Background(), cfg, req, store); err == nil {
		t.Fatal("expected failure")
	}
	if store.puts != 0 {
		t.Fatalf("remote publication began before the complete local release was proven: %d", store.puts)
	}
}

func TestPublishReleaseConcurrentReplayHasOneImmutableResult(t *testing.T) {
	cfg, req, assets := releaseFixture(t)
	store := &releaseFakeStore{objects: map[string][]byte{}, versions: map[string]string{}}
	var wg sync.WaitGroup
	results := make(chan ReleaseResult, 2)
	errs := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := PublishRelease(context.Background(), cfg, req, store)
			results <- result
			errs <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var sha string
	for result := range results {
		if sha != "" && sha != result.ReleaseCommitSHA256 {
			t.Fatalf("concurrent results differ: %q %q", sha, result.ReleaseCommitSHA256)
		}
		sha = result.ReleaseCommitSHA256
	}
	if store.puts != len(assets) {
		t.Fatalf("concurrency duplicated immutable uploads: puts=%d", store.puts)
	}
}

func TestPublishReleaseAllowsAnEmptyReviewedReleaseWithoutObjectWrites(t *testing.T) {
	cfg, req, _ := releaseFixtureWithInputs(t, nil, true)
	store := &releaseFakeStore{objects: map[string][]byte{}, versions: map[string]string{}}
	result, err := PublishRelease(context.Background(), cfg, req, store)
	if err != nil {
		t.Fatal(err)
	}
	if result.AssetCount != 0 || len(result.Assets) != 0 || store.puts != 0 {
		t.Fatalf("empty release result=%#v puts=%d", result, store.puts)
	}
	if _, err := os.Stat(filepath.Join(cfg.AuditRoot, req.ReleaseID, "release-commit.json")); err != nil {
		t.Fatal(err)
	}
}

func TestPublishReleaseTreatsSameBytesAtDifferentPathsAsDistinctObjects(t *testing.T) {
	inputs := []releaseFixtureInput{
		{"软件工程/讲义.pdf", "复习讲义", "same bytes\n"},
		{"高等数学/讲义.pdf", "复习讲义", "same bytes\n"},
	}
	cfg, req, _ := releaseFixtureWithInputs(t, inputs, false)
	store := &releaseFakeStore{objects: map[string][]byte{}, versions: map[string]string{}}
	result, err := PublishRelease(context.Background(), cfg, req, store)
	if err != nil {
		t.Fatal(err)
	}
	if result.AssetCount != 2 || store.puts != 2 || result.Assets[0].SHA256 != result.Assets[1].SHA256 || result.Assets[0].ObjectKey == result.Assets[1].ObjectKey {
		t.Fatalf("duplicate-content paths were not published independently: %#v puts=%d", result, store.puts)
	}
}

func TestPublishReleaseReopensAndReverifiesLargeAssetBeforeCommit(t *testing.T) {
	largeBody := strings.Repeat("a", 8<<20)
	cfg, req, assets := releaseFixtureWithInputs(t, []releaseFixtureInput{{"软件工程/大讲义.pdf", "复习讲义", largeBody}}, false)
	assetPath := filepath.Join(cfg.SealedRoot, req.ReleaseID, "public", filepath.FromSlash(assets[0].PublicPath))
	store := &releaseFakeStore{objects: map[string][]byte{}, versions: map[string]string{}}
	store.bucketHook = func() {
		if err := os.Chmod(assetPath, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(assetPath, []byte("changed after complete local validation\n"), 0o400); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := PublishRelease(context.Background(), cfg, req, store); err == nil {
		t.Fatal("expected the publish-time asset revalidation to fail")
	}
	if _, err := os.Stat(filepath.Join(cfg.AuditRoot, req.ReleaseID, "release-commit.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("changed large asset produced a complete release commit: %v", err)
	}
}

func TestPublishReleaseReadbackRejectsOversizeAfterOneExtraByte(t *testing.T) {
	cfg, req, assets := releaseFixtureWithInputs(t, []releaseFixtureInput{{"软件工程/大讲义.pdf", "复习讲义", strings.Repeat("a", 4<<20)}}, false)
	store := &releaseFakeStore{objects: map[string][]byte{}, versions: map[string]string{}}
	var readback *oversizedReadCloser
	store.getOverride = func(body []byte) io.ReadCloser {
		readback = &oversizedReadCloser{prefix: bytes.NewReader(body), extraRemaining: 4 << 20}
		return readback
	}

	if _, err := PublishRelease(context.Background(), cfg, req, store); err == nil {
		t.Fatal("expected oversized OSS readback to fail")
	}
	if readback == nil {
		t.Fatal("OSS readback was not attempted")
	}
	if maximum := assets[0].Bytes + 1; readback.bytesRead > maximum {
		t.Fatalf("readback consumed %d bytes; bounded verification needed at most %d", readback.bytesRead, maximum)
	}
	if _, err := os.Stat(filepath.Join(cfg.AuditRoot, req.ReleaseID, "release-commit.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("oversized readback produced a complete release commit: %v", err)
	}
}
