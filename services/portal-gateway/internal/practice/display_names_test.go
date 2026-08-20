package practice

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRankingNicknamePrefersDisplayNameAndDerivesStableGuestLabel(t *testing.T) {
	const userA = "5f03dac8-7f7f-4513-9dcd-e4cc5f592c85"
	const userB = "10ca9b18-c303-4b7a-ab14-1241e41b665a"
	const guestA = "guest:0b2c3d4e-5f60-4718-8290-1a2b3c4d5e6f"
	const guestB = "guest:9f8e7d6c-5b4a-4321-809a-bcdef0123456"

	if got := RankingNickname(userA, "认真刷题"); got != "认真刷题" {
		t.Fatalf("display name nickname = %q", got)
	}
	if got := RankingNickname(userA, "   "); got == "认真刷题" || got == "" {
		t.Fatalf("whitespace display name must fall back, got %q", got)
	}
	// The same identity keeps the same label and the same avatar across calls
	// (week-to-week stability); different identities differ.
	a := RankingNickname(userA, "")
	b := RankingNickname(userB, "")
	if a == "" || a != RankingNickname(userA, "") || b == "" || b != RankingNickname(userB, "") {
		t.Fatalf("unstable user labels a=%q b=%q", a, b)
	}
	if a == b {
		t.Fatalf("distinct users share the guest label %q", a)
	}
	// Two distinct guests (each with their own stable guest_key, ADR-0038)
	// must also get distinct 游客x numbers, not one shared neutral label.
	ga := RankingNickname(guestA, "")
	gb := RankingNickname(guestB, "")
	if ga == "" || ga != RankingNickname(guestA, "") || gb == "" || gb != RankingNickname(guestB, "") || ga == gb {
		t.Fatalf("unstable/shared guest labels ga=%q gb=%q", ga, gb)
	}
	// A guest key never equals a user_id-shaped key: signed-in learners derive
	// their label from their user_id, so the same bytes in both roles would be
	// a derivation bug. Here the guest label must differ from the user label
	// of a different-shaped key.
	if ga == a || gb == b {
		t.Fatalf("guest and user identity keys collapse: %q %q %q %q", ga, gb, a, b)
	}
	for _, key := range []string{userA, userB, guestA, guestB} {
		avatar := RankingSystemAvatar(key)
		valid := false
		for _, pattern := range rankingAvatarPatterns {
			if avatar == pattern {
				valid = true
			}
		}
		if !valid {
			t.Fatalf("avatar %q is not in the pattern set %v", avatar, rankingAvatarPatterns)
		}
	}
}

func TestDisplayNamesResolverCachesWithinTTLAndRefetchesAfterExpiry(t *testing.T) {
	var calls atomic.Int32
	source := DisplayNameSource(func(_ context.Context, _ string, userIDs []string) (map[string]string, error) {
		calls.Add(1)
		result := make(map[string]string, len(userIDs))
		for _, id := range userIDs {
			result[id] = "名字_" + id[:8]
		}
		return result, nil
	})
	resolver := NewDisplayNamesResolver(source)
	const userID = "5f03dac8-7f7f-4513-9dcd-e4cc5f592c85"

	first := resolver.Resolve(context.Background(), "req_1", []string{userID})
	if first[userID] != "名字_5f03dac8" {
		t.Fatalf("first resolution = %v", first)
	}
	second := resolver.Resolve(context.Background(), "req_2", []string{userID, userID})
	if second[userID] != "名字_5f03dac8" {
		t.Fatalf("second resolution = %v", second)
	}
	if calls.Load() != 1 {
		t.Fatalf("source calls = %d, want 1 (cache hit + dedupe)", calls.Load())
	}

	// Force the cached entry to expire, then resolve again: the source is
	// consulted once more and the fresh value is returned.
	resolver.mu.Lock()
	resolver.cache[userID] = displayNameCacheEntry{name: "名字_5f03dac8", expiresAt: time.Now().Add(-time.Millisecond)}
	resolver.mu.Unlock()
	third := resolver.Resolve(context.Background(), "req_3", []string{userID})
	if third[userID] != "名字_5f03dac8" {
		t.Fatalf("post-expiry resolution = %v", third)
	}
	if calls.Load() != 2 {
		t.Fatalf("source calls = %d, want 2 after TTL expiry", calls.Load())
	}
}

func TestDisplayNamesResolverSingleflightCoalescesConcurrentMisses(t *testing.T) {
	var calls atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	source := DisplayNameSource(func(_ context.Context, _ string, userIDs []string) (map[string]string, error) {
		if calls.Add(1) == 1 {
			close(started)
			<-release
		}
		result := make(map[string]string, len(userIDs))
		for _, id := range userIDs {
			result[id] = "并发_" + id[:8]
		}
		return result, nil
	})
	resolver := NewDisplayNamesResolver(source)
	const userID = "10ca9b18-c303-4b7a-ab14-1241e41b665a"

	results := make(chan map[string]string, 2)
	var group sync.WaitGroup
	for index := 0; index < 2; index++ {
		group.Add(1)
		go func() {
			defer group.Done()
			results <- resolver.Resolve(context.Background(), fmt.Sprintf("req_concurrent_%d", index), []string{userID})
		}()
	}
	<-started
	close(release)
	group.Wait()
	close(results)
	for result := range results {
		if result[userID] != "并发_10ca9b18" {
			t.Fatalf("concurrent resolution = %v", result)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("source calls = %d, want 1 (singleflight)", calls.Load())
	}
}

func TestDisplayNamesResolverDegradesOnUpstreamFailure(t *testing.T) {
	var calls atomic.Int32
	source := DisplayNameSource(func(_ context.Context, _ string, userIDs []string) (map[string]string, error) {
		calls.Add(1)
		return nil, fmt.Errorf("platform-core unavailable")
	})
	resolver := NewDisplayNamesResolver(source)
	const userID = "5f03dac8-7f7f-4513-9dcd-e4cc5f592c85"

	result := resolver.Resolve(context.Background(), "req_degraded", []string{userID})
	if result[userID] != "" {
		t.Fatalf("degraded resolution = %v, want empty name", result)
	}
	// A failed fetch is not cached: the next call retries the source.
	resolver.Resolve(context.Background(), "req_degraded_2", []string{userID})
	if calls.Load() != 2 {
		t.Fatalf("source calls = %d, want 2 (failures are not cached)", calls.Load())
	}
}
