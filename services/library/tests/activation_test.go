package tests

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	library "henukit.dev/library"
)

func TestActivatePublicReleaseVerifiesCompleteManifestAndPublishesOneCatalog(t *testing.T) {
	pool := activationPool(t)
	store := &activationStore{objects: map[string]library.DownloadObjectState{}}
	bundle := activationBundle(t, []manifestAsset{
		{Subject: "离散数学", Role: "复习讲义", Title: "离散数学复习提纲.pdf", PublicPath: "materials/discrete/outline.pdf", Body: "outline"},
		{Subject: "高等数学", Role: "课件PPT", Title: "高等数学极限课件.pptx", PublicPath: "materials/calculus/limits.pptx", Body: "slides"},
		{Subject: "高等数学", Role: "待复核-答案", Title: "未审核答案.pdf", PublicPath: "materials/calculus/unreviewed.pdf", Body: "not-published"},
	}, store)

	result, err := library.ActivatePublicRelease(context.Background(), pool, store, bundle, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if result.Replayed || result.MaterialCount != 2 || result.PreviousReleaseID != "" {
		t.Fatalf("activation result = %#v", result)
	}
	if store.headCalls != 2 || store.anonymousCalls != 2 {
		t.Fatalf("verified head=%d anonymous=%d, want 2 each", store.headCalls, store.anonymousCalls)
	}

	var activeRelease string
	var materialCount int
	if err := pool.QueryRow(context.Background(), `
		SELECT r.release_id, count(m.material_id)
		FROM library_public_releases r
		JOIN library_public_material_snapshots m ON m.release_id=r.release_id
		WHERE r.state='active'
		GROUP BY r.release_id`).Scan(&activeRelease, &materialCount); err != nil {
		t.Fatal(err)
	}
	if activeRelease != bundle.ReleaseID || materialCount != 2 {
		t.Fatalf("active release=%q materials=%d", activeRelease, materialCount)
	}
	var leaked int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM library_public_material_snapshots WHERE title LIKE '%未审核%'`).Scan(&leaked); err != nil {
		t.Fatal(err)
	}
	if leaked != 0 {
		t.Fatalf("activation published %d unreviewed assets", leaked)
	}
}

func TestActivatePublicReleaseFailureLeavesPreviousCatalogActive(t *testing.T) {
	pool := activationPool(t)
	firstStore := &activationStore{objects: map[string]library.DownloadObjectState{}}
	first := activationBundle(t, []manifestAsset{{Subject: "数学", Role: "讲义", Title: "旧资料.pdf", PublicPath: "old.pdf", Body: "old"}}, firstStore)
	if _, err := library.ActivatePublicRelease(context.Background(), pool, firstStore, first, time.Now); err != nil {
		t.Fatal(err)
	}

	badStore := &activationStore{objects: map[string]library.DownloadObjectState{}}
	bad := activationBundle(t, []manifestAsset{
		{Subject: "数学", Role: "讲义", Title: "新资料.pdf", PublicPath: "new.pdf", Body: "new"},
		{Subject: "英语", Role: "真题", Title: "真题.pdf", PublicPath: "exam.pdf", Body: "exam"},
	}, badStore)
	for key, state := range badStore.objects {
		state.Bytes++
		badStore.objects[key] = state
		break
	}
	if _, err := library.ActivatePublicRelease(context.Background(), pool, badStore, bad, time.Now); err == nil {
		t.Fatal("activation accepted a mismatched object")
	}
	assertActiveRelease(t, pool, first.ReleaseID, 1)
	var badRows int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM library_public_releases WHERE release_id=$1`, bad.ReleaseID).Scan(&badRows); err != nil {
		t.Fatal(err)
	}
	if badRows != 0 {
		t.Fatalf("failed activation left %d release rows", badRows)
	}
}

func TestActivatePublicReleaseAcceptsDuplicateContentAtDistinctReviewedPaths(t *testing.T) {
	pool := activationPool(t)
	store := &activationStore{objects: map[string]library.DownloadObjectState{}}
	bundle := activationBundle(t, []manifestAsset{
		{Subject: "数学", Role: "讲义", Title: "讲义一.pdf", PublicPath: "one.pdf", Body: "same-content"},
		{Subject: "数学", Role: "讲义", Title: "讲义二.pdf", PublicPath: "two.pdf", Body: "same-content"},
	}, store)
	result, err := library.ActivatePublicRelease(context.Background(), pool, store, bundle, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if result.MaterialCount != 2 {
		t.Fatalf("material count=%d", result.MaterialCount)
	}
}

func TestActivatePublicReleaseAllowsAnExplicitEmptyReviewedCatalog(t *testing.T) {
	pool := activationPool(t)
	store := &activationStore{objects: map[string]library.DownloadObjectState{}}
	bundle := activationBundle(t, []manifestAsset{{Subject: "数学", Role: "待复核-讲义", Title: "未审核.pdf", PublicPath: "pending.pdf", Body: "pending"}}, store)
	result, err := library.ActivatePublicRelease(context.Background(), pool, store, bundle, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if result.MaterialCount != 0 || store.headCalls != 0 || store.anonymousCalls != 0 {
		t.Fatalf("empty activation=%#v head=%d anonymous=%d", result, store.headCalls, store.anonymousCalls)
	}
	assertActiveRelease(t, pool, bundle.ReleaseID, 0)
}

func TestActivatePublicReleaseEnforcesTheBoundedCompleteCatalogSize(t *testing.T) {
	makeAssets := func(count int) []manifestAsset {
		assets := make([]manifestAsset, 0, count)
		for index := range count {
			assets = append(assets, manifestAsset{Subject: "数学", Role: "讲义", Title: fmt.Sprintf("资料%03d.pdf", index), PublicPath: fmt.Sprintf("bounded/%03d.pdf", index), Body: "shared"})
		}
		return assets
	}
	pool := activationPool(t)
	store := &activationStore{objects: map[string]library.DownloadObjectState{}}
	accepted := activationBundle(t, makeAssets(500), store)
	result, err := library.ActivatePublicRelease(context.Background(), pool, store, accepted, time.Now)
	if err != nil || result.MaterialCount != 500 {
		t.Fatalf("500-material activation=%#v, %v", result, err)
	}

	rejectedStore := &activationStore{objects: map[string]library.DownloadObjectState{}}
	rejected := activationBundle(t, makeAssets(501), rejectedStore)
	if _, err := library.ActivatePublicRelease(context.Background(), pool, rejectedStore, rejected, time.Now); err == nil {
		t.Fatal("501-material activation was accepted")
	}
	if rejectedStore.headCalls != 0 {
		t.Fatalf("oversized activation reached OSS %d times", rejectedStore.headCalls)
	}
	assertActiveRelease(t, pool, accepted.ReleaseID, 500)
}

func TestActivatePublicReleaseRejectsObjectKeyBeyondDatabaseLimitBeforeOSSOrTransaction(t *testing.T) {
	pool := activationPool(t)
	store := &activationStore{objects: map[string]library.DownloadObjectState{}}
	longPath := strings.Join([]string{
		strings.Repeat("a", 210),
		strings.Repeat("b", 210),
		strings.Repeat("c", 210),
		strings.Repeat("d", 210),
	}, "/")
	bundle := activationBundle(t, []manifestAsset{{Subject: "数学", Role: "讲义", Title: "长路径资料", PublicPath: longPath, Body: "safe"}}, store)

	if _, err := library.ActivatePublicRelease(context.Background(), pool, store, bundle, time.Now); err == nil {
		t.Fatal("activation accepted an object key beyond the database limit")
	}
	if store.headCalls != 0 || store.anonymousCalls != 0 {
		t.Fatalf("oversized object key reached OSS: head=%d anonymous=%d", store.headCalls, store.anonymousCalls)
	}
	var releases int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM library_public_releases WHERE release_id=$1`, bundle.ReleaseID).Scan(&releases); err != nil {
		t.Fatal(err)
	}
	if releases != 0 {
		t.Fatalf("oversized object key reached the activation transaction: releases=%d", releases)
	}
}

func TestActivatePublicReleaseRejectsMalformedMetadataWithoutTouchingOSSOrCatalog(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*library.PublicReleaseActivation)
	}{
		{name: "manifest trailing JSON", mutate: func(bundle *library.PublicReleaseActivation) {
			bundle.ManifestJSON = append(bundle.ManifestJSON, []byte(` {}`)...)
		}},
		{name: "receipt trailing garbage", mutate: func(bundle *library.PublicReleaseActivation) {
			bundle.SealedReceiptJSON = append(bundle.SealedReceiptJSON, []byte(` garbage`)...)
		}},
		{name: "receipt unknown field", mutate: func(bundle *library.PublicReleaseActivation) {
			var receipt map[string]any
			if err := json.Unmarshal(bundle.SealedReceiptJSON, &receipt); err != nil {
				t.Fatal(err)
			}
			receipt["storage_endpoint"] = "internal.example"
			bundle.SealedReceiptJSON, _ = json.Marshal(receipt)
		}},
		{name: "foreign source", mutate: func(bundle *library.PublicReleaseActivation) {
			var receipt map[string]any
			if err := json.Unmarshal(bundle.SealedReceiptJSON, &receipt); err != nil {
				t.Fatal(err)
			}
			receipt["source"].(map[string]any)["repository"] = "https://example.invalid/foreign.git"
			bundle.SealedReceiptJSON, _ = json.Marshal(receipt)
		}},
		{name: "online preview digest", mutate: func(bundle *library.PublicReleaseActivation) {
			bundle.Derived.SlidesSHA256 = strings.Repeat("d", 64)
		}},
		{name: "non-canonical derived inventory digest", mutate: func(bundle *library.PublicReleaseActivation) {
			bundle.Derived.IndexSHA256 = strings.Repeat("e", 64)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pool := activationPool(t)
			store := &activationStore{objects: map[string]library.DownloadObjectState{}}
			bundle := activationBundle(t, []manifestAsset{{Subject: "数学", Role: "讲义", Title: "安全标题.pdf", PublicPath: "safe.pdf", Body: "safe"}}, store)
			tc.mutate(&bundle)
			if _, err := library.ActivatePublicRelease(context.Background(), pool, store, bundle, time.Now); err == nil {
				t.Fatal("malformed activation bundle was accepted")
			}
			if store.headCalls != 0 || store.anonymousCalls != 0 {
				t.Fatalf("invalid metadata reached OSS: head=%d anonymous=%d", store.headCalls, store.anonymousCalls)
			}
		})
	}
}

func TestActivatePublicReleaseRejectsProvenanceViolationsBeforeOSSOrCatalog(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*manifestAsset)
	}{
		{name: "contains personal info", mutate: func(asset *manifestAsset) {
			asset.ContainsPersonalInfo = true
		}},
		{name: "unreviewed reviewStatus", mutate: func(asset *manifestAsset) {
			asset.ReviewStatus = "needs_review"
		}},
		{name: "pending maintainer review", mutate: func(asset *manifestAsset) {
			asset.ReviewStatus = "待维护者复核"
		}},
		{name: "unknown reviewStatus", mutate: func(asset *manifestAsset) {
			asset.ReviewStatus = "totally-made-up"
		}},
		{name: "disallowed licenseStatus", mutate: func(asset *manifestAsset) {
			asset.LicenseStatus = "贡献者自有学习笔记，提交后可按仓库公开资料协议共享。"
		}},
		{name: "unknown licenseStatus", mutate: func(asset *manifestAsset) {
			asset.LicenseStatus = "made-up-license"
		}},
		{name: "teacher_shared_exception not whitelisted", mutate: func(asset *manifestAsset) {
			asset.LicenseStatus = "teacher_shared_exception"
		}},
		{name: "review-only uncertainty", mutate: func(asset *manifestAsset) {
			asset.Uncertainty = "source_uncertain"
		}},
		{name: "year uncertainty", mutate: func(asset *manifestAsset) {
			asset.Uncertainty = "year_uncertain"
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pool := activationPool(t)
			store := &activationStore{objects: map[string]library.DownloadObjectState{}}
			asset := manifestAsset{Subject: "数学", Role: "讲义", Title: "安全标题.pdf", PublicPath: "safe.pdf", Body: "safe",
				ReviewStatus: "basic-reviewed", LicenseStatus: "learning-reference"}
			tc.mutate(&asset)
			bundle := activationBundle(t, []manifestAsset{asset}, store)
			if _, err := library.ActivatePublicRelease(context.Background(), pool, store, bundle, time.Now); err == nil {
				t.Fatal("provenance-violating activation bundle was accepted")
			}
			if store.headCalls != 0 || store.anonymousCalls != 0 {
				t.Fatalf("provenance violation reached OSS: head=%d anonymous=%d", store.headCalls, store.anonymousCalls)
			}
			var releases int
			if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM library_public_releases WHERE release_id=$1`, bundle.ReleaseID).Scan(&releases); err != nil {
				t.Fatal(err)
			}
			if releases != 0 {
				t.Fatalf("provenance violation reached the activation transaction: releases=%d", releases)
			}
		})
	}
}

func TestActivatePublicReleaseAcceptsCanonicalProvenanceAndApprovedTeacherException(t *testing.T) {
	pool := activationPool(t)
	store := &activationStore{objects: map[string]library.DownloadObjectState{}}
	approvedPath := "思想道德与法治/复习讲义/思想道德与法治_复习讲义_2025年冬最新考试重点.pdf"
	approvedSHA := "bfda62a15cfefb53c1413a244a4ff9f95e11a9fc959032f4ebff83adc1b8530c"
	bundle := activationBundle(t, []manifestAsset{
		{Subject: "数学", Role: "讲义", Title: "讲义.pdf", PublicPath: "note.pdf", Body: "note",
			ReviewStatus: "verified", LicenseStatus: "public_review_only", Uncertainty: "format_lossy"},
		{Subject: "思想道德与法治", Role: "复习讲义", Title: "白名单讲义.pdf", PublicPath: approvedPath, Body: "approved",
			ReviewStatus: "basic-reviewed", LicenseStatus: "teacher_shared_exception",
			SHA256Override: approvedSHA, BytesOverride: "12345"},
	}, store)
	result, err := library.ActivatePublicRelease(context.Background(), pool, store, bundle, time.Now)
	if err != nil {
		t.Fatalf("canonical provenance activation failed: %v", err)
	}
	if result.MaterialCount != 2 {
		t.Fatalf("material count=%d, want 2", result.MaterialCount)
	}
}

func TestActivatePublicReleaseDerivesSafeFileNameFromReviewedPath(t *testing.T) {
	pool := activationPool(t)
	store := &activationStore{objects: map[string]library.DownloadObjectState{}}
	bundle := activationBundle(t, []manifestAsset{{Subject: "数学", Role: "讲义", Title: "标题不是文件名", PublicPath: "nested/safe.pdf", Body: "safe"}}, store)
	if _, err := library.ActivatePublicRelease(context.Background(), pool, store, bundle, time.Now); err != nil {
		t.Fatal(err)
	}
	var fileName string
	if err := pool.QueryRow(context.Background(), `SELECT file_name FROM library_public_material_snapshots WHERE release_id=$1`, bundle.ReleaseID).Scan(&fileName); err != nil {
		t.Fatal(err)
	}
	if fileName != "safe.pdf" {
		t.Fatalf("file name=%q", fileName)
	}
}

func TestActivatePublicReleaseIsIdempotentConcurrentAndSupportsAuditedRollback(t *testing.T) {
	pool := activationPool(t)
	var eventsBefore int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM library_public_release_activation_events`).Scan(&eventsBefore); err != nil {
		t.Fatal(err)
	}
	store := &activationStore{objects: map[string]library.DownloadObjectState{}}
	first := activationBundle(t, []manifestAsset{{Subject: "数学", Role: "讲义", Title: "版本一.pdf", PublicPath: "v1.pdf", Body: "v1"}}, store)

	const workers = 8
	results := make(chan library.PublicReleaseActivationResult, workers)
	errors := make(chan error, workers)
	var wait sync.WaitGroup
	for range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			result, err := library.ActivatePublicRelease(context.Background(), pool, store, first, time.Now)
			if err != nil {
				errors <- err
				return
			}
			results <- result
		}()
	}
	wait.Wait()
	close(results)
	close(errors)
	for err := range errors {
		t.Fatal(err)
	}
	created := 0
	for result := range results {
		if !result.Replayed {
			created++
		}
	}
	if created != 1 {
		t.Fatalf("new activation results=%d, want 1", created)
	}
	assertActiveRelease(t, pool, first.ReleaseID, 1)

	secondStore := &activationStore{objects: map[string]library.DownloadObjectState{}}
	second := activationBundle(t, []manifestAsset{{Subject: "数学", Role: "讲义", Title: "版本二.pdf", PublicPath: "v2.pdf", Body: "v2"}}, secondStore)
	if _, err := library.ActivatePublicRelease(context.Background(), pool, secondStore, second, time.Now); err != nil {
		t.Fatal(err)
	}
	assertActiveRelease(t, pool, second.ReleaseID, 1)

	rollback, err := library.ActivatePublicRelease(context.Background(), pool, store, first, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if rollback.PreviousReleaseID != second.ReleaseID || rollback.Replayed {
		t.Fatalf("rollback result = %#v", rollback)
	}
	assertActiveRelease(t, pool, first.ReleaseID, 1)
	var events int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM library_public_release_activation_events`).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events != eventsBefore+3 {
		t.Fatalf("activation audit events=%d, want %d", events, eventsBefore+3)
	}
}

func TestPublicCatalogReturnsOneReleaseSnapshotWithStableAggregates(t *testing.T) {
	store := &activationStore{objects: map[string]library.DownloadObjectState{}}
	server, pool := newLibraryDownloadServer(t, store)
	defer server.Close()
	defer pool.Close()
	clearPublicCatalog(t, pool)
	downloadsBefore := totalLedgerCount(t, pool)
	bundle := activationBundle(t, []manifestAsset{
		{Subject: "数学", Role: "复习讲义", Title: "讲义.pdf", PublicPath: "note.pdf", Body: "note"},
		{Subject: "数学", Role: "真题", Title: "真题.pdf", PublicPath: "exam.pdf", Body: "exam"},
	}, store)
	if _, err := library.ActivatePublicRelease(context.Background(), pool, store, bundle, time.Now); err != nil {
		t.Fatal(err)
	}

	response := sendDownload(t, server.URL, "GET", "/api/v1/public-materials", nil)
	defer response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatalf("catalog status=%d", response.StatusCode)
	}
	var payload struct {
		Data struct {
			ReleaseID      *string `json:"release_id"`
			MaterialCount  int     `json:"material_count"`
			DownloadStarts int64   `json:"download_starts"`
			CountingSince  string  `json:"counting_since"`
			AsOf           string  `json:"as_of"`
			Materials      []struct {
				ID                string `json:"id"`
				Type              string `json:"type"`
				Subject           string `json:"subject"`
				Title             string `json:"title"`
				Role              string `json:"role"`
				FileName          string `json:"file_name"`
				FileSize          int64  `json:"file_size"`
				Downloads         int64  `json:"downloads"`
				DownloadAvailable bool   `json:"download_available"`
			} `json:"materials"`
		} `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.Data.ReleaseID == nil || *payload.Data.ReleaseID != bundle.ReleaseID || payload.Data.MaterialCount != 2 || payload.Data.DownloadStarts != downloadsBefore || len(payload.Data.Materials) != 2 || payload.Data.CountingSince == "" || payload.Data.AsOf == "" {
		t.Fatalf("catalog = %#v", payload.Data)
	}
	for _, material := range payload.Data.Materials {
		if material.ID == "" || material.Subject == "" || material.Title == "" || material.Role == "" || material.FileName == "" || material.FileSize <= 0 || material.Downloads != 0 || !material.DownloadAvailable {
			t.Fatalf("unsafe/incomplete material = %#v", material)
		}
	}
	start := sendDownload(t, server.URL, "POST", "/api/v1/public-materials/"+payload.Data.Materials[0].ID+"/download-starts", nil)
	start.Body.Close()
	if start.StatusCode != 201 {
		t.Fatalf("download start status=%d", start.StatusCode)
	}
	refreshed := sendDownload(t, server.URL, "GET", "/api/v1/public-materials", nil)
	defer refreshed.Body.Close()
	var refreshedPayload struct {
		Data struct {
			DownloadStarts int64 `json:"download_starts"`
			Materials      []struct {
				ID        string `json:"id"`
				Downloads int64  `json:"downloads"`
			} `json:"materials"`
		} `json:"data"`
	}
	if err := json.NewDecoder(refreshed.Body).Decode(&refreshedPayload); err != nil {
		t.Fatal(err)
	}
	if refreshedPayload.Data.DownloadStarts != downloadsBefore+1 {
		t.Fatalf("refreshed global=%d", refreshedPayload.Data.DownloadStarts)
	}
	foundCount := false
	for _, material := range refreshedPayload.Data.Materials {
		if material.ID == payload.Data.Materials[0].ID && material.Downloads == 1 {
			foundCount = true
		}
	}
	if !foundCount {
		t.Fatal("refreshed catalog did not include the stable material count")
	}

	clearPublicCatalog(t, pool)
	empty := sendDownload(t, server.URL, "GET", "/api/v1/public-materials", nil)
	defer empty.Body.Close()
	if empty.StatusCode != 200 {
		t.Fatalf("empty catalog status=%d", empty.StatusCode)
	}
	var emptyPayload struct {
		Data struct {
			ReleaseID      *string `json:"release_id"`
			MaterialCount  int     `json:"material_count"`
			DownloadStarts int64   `json:"download_starts"`
			Materials      []any   `json:"materials"`
		} `json:"data"`
	}
	if err := json.NewDecoder(empty.Body).Decode(&emptyPayload); err != nil {
		t.Fatal(err)
	}
	if emptyPayload.Data.ReleaseID != nil || emptyPayload.Data.MaterialCount != 0 || emptyPayload.Data.DownloadStarts != downloadsBefore+1 || len(emptyPayload.Data.Materials) != 0 {
		t.Fatalf("empty catalog = %#v", emptyPayload.Data)
	}
}

type manifestAsset struct {
	Subject, Role, Title, PublicPath, Body   string
	ContainsPersonalInfo                     bool
	LicenseStatus, ReviewStatus, Uncertainty string
	SHA256Override, BytesOverride            string
}

// bytesValue returns the asset's manifest byte count, honoring the override
// used by the approved teacher_shared_exception fixture (whose stored SHA must
// match the upstream whitelist even though the object bytes differ).
func (a manifestAsset) bytesValue() int {
	if a.BytesOverride != "" {
		n, err := strconv.Atoi(a.BytesOverride)
		if err == nil {
			return n
		}
	}
	return len(a.Body)
}

type activationStore struct {
	mu             sync.Mutex
	objects        map[string]library.DownloadObjectState
	headCalls      int
	anonymousCalls int
}

func (s *activationStore) Head(_ context.Context, key, versionID string) (library.DownloadObjectState, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.headCalls++
	state, ok := s.objects[key]
	return state, ok && state.VersionID == versionID, nil
}

func (s *activationStore) AnonymousDenied(_ context.Context, key, versionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.anonymousCalls++
	state, ok := s.objects[key]
	if !ok || state.VersionID != versionID {
		return fmt.Errorf("object is unavailable")
	}
	return nil
}

func (s *activationStore) PresignGet(_ context.Context, key, versionID, disposition string, ttl time.Duration) (library.SignedDownload, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.objects[key]
	if !ok || state.VersionID != versionID {
		return library.SignedDownload{}, fmt.Errorf("object is unavailable")
	}
	return signedFixture(key, versionID, disposition, ttl), nil
}

func activationBundle(t *testing.T, assets []manifestAsset, store *activationStore) library.PublicReleaseActivation {
	t.Helper()
	subjects := make([]map[string]any, 0)
	grouped := map[string][]map[string]any{}
	order := make([]string, 0)
	for _, asset := range assets {
		body := []byte(asset.Body)
		digest := sha256.Sum256(body)
		sha := hex.EncodeToString(digest[:])
		if asset.SHA256Override != "" {
			sha = asset.SHA256Override
		}
		if _, ok := grouped[asset.Subject]; !ok {
			order = append(order, asset.Subject)
		}
		grouped[asset.Subject] = append(grouped[asset.Subject], map[string]any{
			"subject": asset.Subject, "role": asset.Role, "title": asset.Title, "publicPath": asset.PublicPath,
			"bytes": asset.bytesValue(), "sha256": sha,
			"containsPersonalInfo": asset.ContainsPersonalInfo, "licenseStatus": asset.LicenseStatus,
			"reviewStatus": asset.ReviewStatus, "uncertainty": asset.Uncertainty,
		})
	}
	for _, subject := range order {
		subjects = append(subjects, map[string]any{"name": subject, "note": "受审资料", "assets": grouped[subject]})
	}
	manifest, err := json.Marshal(map[string]any{"version": 1, "generatedAt": "2026-08-11T00:00:00Z", "subjects": subjects})
	if err != nil {
		t.Fatal(err)
	}
	manifestDigest := sha256.Sum256(manifest)
	sourceSHA := strings.Repeat("a", 40)
	releaseID := sourceSHA + "-" + hex.EncodeToString(manifestDigest[:])[:16]
	reviewed := 0
	for _, asset := range assets {
		if !strings.HasPrefix(asset.Role, "待复核") {
			reviewed++
		}
	}
	receipt, err := json.Marshal(map[string]any{
		"version": 1, "release_id": releaseID,
		"source":           map[string]any{"repository": "https://github.com/jry21223/HENU-Final-Review.git", "ref": "refs/heads/main", "sha": sourceSHA},
		"manifest_sha256":  hex.EncodeToString(manifestDigest[:]),
		"inventory_sha256": strings.Repeat("b", 64), "tree_sha256": strings.Repeat("c", 64), "reviewed_assets": reviewed,
		"slides": map[string]any{"status": "disabled", "source_slide_assets": 0},
	})
	if err != nil {
		t.Fatal(err)
	}
	receiptDigest := sha256.Sum256(receipt)
	emptySlidesDigest := sha256.Sum256([]byte("[]\n"))
	emptyIndex := []byte(fmt.Sprintf("{\n  \"version\": 1,\n  \"release_id\": %q,\n  \"assets\": []\n}\n", releaseID))
	emptyIndexDigest := sha256.Sum256(emptyIndex)
	bundle := library.PublicReleaseActivation{
		Version: 1, ReleaseID: releaseID, ManifestJSON: manifest, SealedReceiptJSON: receipt,
		Derived: library.PublicReleaseDerivedArtifacts{ReleaseID: releaseID, SlidesSHA256: hex.EncodeToString(emptySlidesDigest[:]), IndexSHA256: hex.EncodeToString(emptyIndexDigest[:])},
	}
	commitAssets := make([]map[string]any, 0, reviewed)
	for _, asset := range assets {
		if strings.HasPrefix(asset.Role, "待复核") {
			continue
		}
		bodyDigest := sha256.Sum256([]byte(asset.Body))
		sha := hex.EncodeToString(bodyDigest[:])
		if asset.SHA256Override != "" {
			sha = asset.SHA256Override
		}
		key := "releases/" + releaseID + "/receipts/" + hex.EncodeToString(receiptDigest[:]) + "/objects/" + sha + "/" + asset.PublicPath
		version := "version-" + sha[:16]
		bundle.Objects = append(bundle.Objects, library.PublicReleaseObject{PublicPath: asset.PublicPath, ObjectKey: key, ObjectVersionID: version})
		commitAssets = append(commitAssets, map[string]any{"public_path": asset.PublicPath, "sha256": sha, "bytes": asset.bytesValue(), "object_key": key, "object_version_id": version})
		store.objects[key] = library.DownloadObjectState{Bytes: int64(asset.bytesValue()), SHA256: sha, Encryption: "AES256", VersionID: version}
	}
	bundle.ReleaseCommitJSON, err = json.Marshal(map[string]any{
		"version": 1, "state": "release_committed_not_activated", "release_id": releaseID,
		"receipt_sha256": hex.EncodeToString(receiptDigest[:]), "manifest_sha256": hex.EncodeToString(manifestDigest[:]),
		"inventory_sha256": strings.Repeat("b", 64), "tree_sha256": strings.Repeat("c", 64),
		"asset_count": reviewed, "assets": commitAssets,
	})
	if err != nil {
		t.Fatal(err)
	}
	return bundle
}

func activationPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), envDatabaseURL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	clearPublicCatalog(t, pool)
	return pool
}

func clearPublicCatalog(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `UPDATE library_public_releases SET state='retired' WHERE state='active'`); err != nil {
		t.Fatal(err)
	}
}

func assertActiveRelease(t *testing.T, pool *pgxpool.Pool, releaseID string, count int) {
	t.Helper()
	var actual string
	var materialCount int
	if err := pool.QueryRow(context.Background(), `SELECT r.release_id,count(m.material_id) FROM library_public_releases r LEFT JOIN library_public_material_snapshots m ON m.release_id=r.release_id WHERE r.state='active' GROUP BY r.release_id`).Scan(&actual, &materialCount); err != nil {
		t.Fatal(err)
	}
	if actual != releaseID || materialCount != count {
		t.Fatalf("active release=%s count=%d, want %s/%d", actual, materialCount, releaseID, count)
	}
}
