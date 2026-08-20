package practice

import (
	"context"
	"fmt"
	"hash/fnv"
	"log"
	"strings"
	"sync"
	"time"
)

// Ranking display-name synthesis (ADR-0038): the external nickname is the
// platform display name or the neutral 游客x label with a stable number
// derived from the internal identity key, and the system avatar is derived
// deterministically from the same key. Both are computed by the Gateway and
// never trusted from an upstream response.
//
// The identity key is user_id for signed-in learners and guest_key for guest
// learners (the Core's stable anonymous actor_key), so every guest gets a
// distinct, week-stable 游客x number instead of sharing one neutral label.

const (
	// guestNumberBase and guestNumberMod shape the stable 游客x suffix:
	// FNV-1a(identity key) % 9000 + 1000, so the same identity keeps the same
	// label across weeks and the label never collides with a short display
	// name range.
	guestNumberBase = 1000
	guestNumberMod  = 9000
)

// rankingAvatarPatterns are the system avatar patterns the Portal UI renders.
// The order is fixed here and must stay aligned with the external enum in
// packages/api-contracts/openapi/portal-gateway.yaml.
var rankingAvatarPatterns = []string{"scholar-blue", "coder-green", "reader-amber", "owl-purple"}

// identityHash derives a stable 32-bit fingerprint from an internal identity
// key (user_id ?? guest_key). A blank key must never reach this function in
// production: validateRanking requires every entry to carry one of the two.
func identityHash(identityKey string) uint32 {
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(identityKey))
	return hash.Sum32()
}

// RankingNickname returns the external nickname for one ranking entry. A
// trimmed non-empty platform display name wins; otherwise the neutral 游客x
// label derives a stable number from the identity key.
func RankingNickname(identityKey, displayName string) string {
	if name := strings.TrimSpace(displayName); name != "" {
		return name
	}
	return fmt.Sprintf("游客%d", identityHash(identityKey)%guestNumberMod+guestNumberBase)
}

// RankingSystemAvatar derives a system avatar deterministically from the
// identity key. The derivation carries no personal data and is not
// user-controlled (ADR-0038).
func RankingSystemAvatar(identityKey string) string {
	return rankingAvatarPatterns[identityHash(identityKey)%uint32(len(rankingAvatarPatterns))]
}

// DisplayNameSource resolves a batch of user ids to display names (empty
// string when unset or unknown). It is the seam that keeps the resolver
// independent of the Platform Core HTTP client.
type DisplayNameSource func(ctx context.Context, requestID string, userIDs []string) (map[string]string, error)

type displayNameCacheEntry struct {
	name      string
	expiresAt time.Time
}

// inflightDisplayNames coalesces concurrent misses for one id: every caller
// joins the same call and reads its name after the fetch completes.
type inflightDisplayNames struct {
	done chan struct{}
	name string
}

type pendingDisplayName struct {
	id   string
	call *inflightDisplayNames
}

// DisplayNamesResolver is the in-process display-name cache for ranking
// synthesis (ADR-0038): 10-minute TTL, a bounded entry cap, singleflight on
// concurrent misses, and graceful degradation to empty names when the
// Platform Core boundary is unavailable (the ranking stays available and the
// caller renders 游客x).
type DisplayNamesResolver struct {
	source DisplayNameSource
	ttl    time.Duration
	max    int

	mu       sync.Mutex
	cache    map[string]displayNameCacheEntry
	inflight map[string]*inflightDisplayNames
}

// NewDisplayNamesResolver creates a resolver with the default 10-minute TTL
// and 2048-entry cap.
func NewDisplayNamesResolver(source DisplayNameSource) *DisplayNamesResolver {
	return &DisplayNamesResolver{
		source:   source,
		ttl:      10 * time.Minute,
		max:      2048,
		cache:    make(map[string]displayNameCacheEntry),
		inflight: make(map[string]*inflightDisplayNames),
	}
}

// Resolve returns the display names for the requested ids. Unknown or unset
// ids map to "". On upstream failure every requested id degrades to "" and
// the failure is logged; nothing is cached from a failed fetch.
func (r *DisplayNamesResolver) Resolve(ctx context.Context, requestID string, userIDs []string) map[string]string {
	ordered := make([]string, 0, len(userIDs))
	seen := make(map[string]struct{}, len(userIDs))
	for _, id := range userIDs {
		if strings.TrimSpace(id) == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		ordered = append(ordered, id)
	}
	if len(ordered) == 0 {
		return map[string]string{}
	}

	now := time.Now()
	results := make(map[string]string, len(ordered))
	var pending []pendingDisplayName
	var misses []string
	r.mu.Lock()
	for _, id := range ordered {
		if entry, ok := r.cache[id]; ok && entry.expiresAt.After(now) {
			results[id] = entry.name
			continue
		}
		if call, ok := r.inflight[id]; ok {
			pending = append(pending, pendingDisplayName{id: id, call: call})
			continue
		}
		call := &inflightDisplayNames{done: make(chan struct{})}
		r.inflight[id] = call
		pending = append(pending, pendingDisplayName{id: id, call: call})
		misses = append(misses, id)
	}
	r.mu.Unlock()

	if len(misses) > 0 {
		fetched, fetchErr := r.source(ctx, requestID, misses)
		if fetchErr != nil {
			// Ranking availability outranks nickname freshness (ADR-0038):
			// degrade every miss to the neutral label instead of failing the
			// ranking read. A failed fetch is never cached, so the next
			// ranking request retries the boundary.
			log.Printf("portal-gateway display-names resolution degraded ids=%d request_id=%s: %v", len(misses), requestID, fetchErr)
		}
		storedAt := time.Now()
		r.mu.Lock()
		for _, id := range misses {
			name := fetched[id]
			if fetchErr == nil {
				r.cache[id] = displayNameCacheEntry{name: name, expiresAt: storedAt.Add(r.ttl)}
			}
			if call, ok := r.inflight[id]; ok {
				call.name = name
				close(call.done)
				delete(r.inflight, id)
			}
			results[id] = name
		}
		if fetchErr == nil {
			r.evictLocked(storedAt)
		}
		r.mu.Unlock()
	}
	for _, item := range pending {
		<-item.call.done
		if _, exists := results[item.id]; !exists {
			results[item.id] = item.call.name
		}
	}
	return results
}

// evictLocked drops expired entries, then the soonest-expiring entries, until
// the cache fits under the cap. Callers hold r.mu.
func (r *DisplayNamesResolver) evictLocked(now time.Time) {
	if len(r.cache) <= r.max {
		return
	}
	for id, entry := range r.cache {
		if !entry.expiresAt.After(now) {
			delete(r.cache, id)
		}
	}
	for len(r.cache) > r.max {
		oldestID := ""
		oldestAt := time.Time{}
		for id, entry := range r.cache {
			if oldestID == "" || entry.expiresAt.Before(oldestAt) {
				oldestID, oldestAt = id, entry.expiresAt
			}
		}
		delete(r.cache, oldestID)
	}
}
