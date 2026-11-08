package businessfacts

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTicketFactsTitleLengthUsesUnicodeCodePoints(t *testing.T) {
	for _, tc := range []struct {
		name, unit string
		length     int
		valid      bool
	}{
		{"Chinese54", "中", 54, true}, {"Chinese160", "中", 160, true}, {"Chinese161", "中", 161, false},
		{"Emoji100", "😀", 100, true}, {"Emoji160", "😀", 160, true}, {"Emoji161", "😀", 161, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := json.Marshal(map[string]any{
				"scope": "global", "as_of": "2026-11-07T12:00:00Z", "stale": false, "counts": map[string]int{"open": 1, "in_progress": 0, "resolved": 0, "unresolved": 1},
				"window": map[string]any{"filter": "unresolved", "matched_total": 1, "limit": 100, "returned": 1, "has_more": false, "items": []any{map[string]any{"id": "11111111-1111-4111-8111-111111111111", "reference": "HKT-11111111-1111-4111-8111-111111111111", "title": strings.Repeat(tc.unit, tc.length), "status": "open", "version": 1, "updated_at": "2026-11-07T12:00:00Z"}}},
			})
			if err != nil {
				t.Fatal(err)
			}
			err = ValidateTickets(raw, "unresolved")
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%t want%t err=%v", err == nil, tc.valid, err)
			}
		})
	}
}
