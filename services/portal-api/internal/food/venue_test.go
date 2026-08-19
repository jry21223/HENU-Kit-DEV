package food

import "testing"

// venueID becomes part of a URL, so anything outside [a-z0-9] must collapse to
// a separator and never leak through.
//
// Only ASCII shop names are asserted here on purpose. Every rune outside
// [a-zA-Z0-9] collapses to a separator that is then trimmed, so an all-CJK
// shop name reduces to the campus alone and distinct shops collide on one ID.
// That is a defect in venueID, not behavior worth pinning in a test; see the
// note raised with this change.
func TestVenueID(t *testing.T) {
	for _, tc := range []struct {
		name   string
		campus string
		shop   string
		want   string
	}{
		{"plain", "minglun", "ShopA", "minglun-shopa"},
		{"uppercase folds down", "MINGLUN", "SHOP", "minglun-shop"},
		{"spaces become separators", "minglun", "Shop A", "minglun-shop-a"},
		{"digits survive", "minglun", "Shop 24", "minglun-shop-24"},
		{"slashes cannot open a path segment", "minglun", "a/b", "minglun-a-b"},
		{"leading and trailing separators are trimmed", "minglun", " x ", "minglun-x"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := venueID(tc.campus, tc.shop); got != tc.want {
				t.Errorf("venueID(%q, %q) = %q, want %q", tc.campus, tc.shop, got, tc.want)
			}
		})
	}
}

func TestVenueTier(t *testing.T) {
	for _, tc := range []struct {
		name      string
		rating    float64
		postCount int
		want      string
	}{
		{"featured needs both rating and volume", 4.5, 3, "featured"},
		{"high rating with too few posts is only recommended", 4.9, 2, "recommended"},
		{"volume without rating is standard", 3.9, 100, "standard"},
		{"recommended lower boundary", 4.0, 1, "recommended"},
		{"just below recommended", 3.99, 1, "standard"},
		{"featured lower boundary", 4.5, 3, "featured"},
		{"just below the featured post count", 4.5, 2, "recommended"},
		{"zero rating", 0, 0, "standard"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := venueTier(tc.rating, tc.postCount); got != tc.want {
				t.Errorf("venueTier(%v, %d) = %q, want %q", tc.rating, tc.postCount, got, tc.want)
			}
		})
	}
}
