// Package businessfacts validates only the two fresh owner read contracts.
// Required fields must be present; zero is valid only with an empty matching
// window. It does not read an owner database or consume disabled summaries.
package businessfacts

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

const maxSafe int64 = 9007199254740991

var ErrInvalid = errors.New("invalid business facts response")

type object = map[string]any

func decode(raw json.RawMessage) (object, bool) {
	var value object
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if d.Decode(&value) != nil || value == nil {
		return nil, false
	}
	var tail any
	return value, d.Decode(&tail) == io.EOF
}
func obj(v any, keys ...string) (object, bool) {
	o, ok := v.(map[string]any)
	if !ok || len(o) != len(keys) {
		return nil, false
	}
	for _, k := range keys {
		if _, ok := o[k]; !ok {
			return nil, false
		}
	}
	return o, true
}
func number(v any) (int64, bool) {
	n, ok := v.(json.Number)
	if !ok {
		return 0, false
	}
	i, e := n.Int64()
	return i, e == nil && i >= 0 && i <= maxSafe
}
func timestamp(v any) bool {
	s, ok := v.(string)
	if !ok {
		return false
	}
	t, e := time.Parse(time.RFC3339Nano, s)
	return e == nil && !t.IsZero()
}
func meta(o object) bool {
	return o["scope"] == "global" && o["stale"] == false && timestamp(o["as_of"])
}
func counts(v any, keys ...string) (map[string]int64, bool) {
	o, ok := obj(v, keys...)
	if !ok {
		return nil, false
	}
	out := map[string]int64{}
	for _, k := range keys {
		n, ok := number(o[k])
		if !ok {
			return nil, false
		}
		out[k] = n
	}
	return out, true
}
func window(v any, total int64, filter string, limit int, tickets bool) bool {
	keys := []string{"filter", "limit", "returned", "has_more", "items"}
	if tickets {
		keys = append(keys, "matched_total")
	}
	o, ok := obj(v, keys...)
	if !ok || o["filter"] != filter {
		return false
	}
	bound, bok := number(o["limit"])
	returned, rok := number(o["returned"])
	hasMore, hok := o["has_more"].(bool)
	items, iok := o["items"].([]any)
	if !bok || bound != int64(limit) || !rok || returned != min(total, int64(limit)) || !hok || hasMore != (total > returned) || !iok || int64(len(items)) != returned {
		return false
	}
	if tickets {
		matched, ok := number(o["matched_total"])
		if !ok || matched != total {
			return false
		}
	}
	seen := map[string]bool{}
	for _, item := range items {
		keys := []string{"id", "status", "version", "updated_at"}
		if tickets {
			keys = append(keys, "reference", "title")
		}
		row, ok := obj(item, keys...)
		if !ok {
			return false
		}
		id, idok := row["id"].(string)
		version, vok := number(row["version"])
		status, sok := row["status"].(string)
		parsed, err := uuid.Parse(id)
		if !idok || err != nil || parsed.String() != strings.ToLower(id) || seen[id] || !vok || version < 1 || !sok || !timestamp(row["updated_at"]) {
			return false
		}
		seen[id] = true
		if tickets {
			title, tok := row["title"].(string)
			if !tok || utf8.RuneCountInString(title) < 1 || utf8.RuneCountInString(title) > 160 || row["reference"] != "HKT-"+strings.ToLower(id) || !(status == "open" || status == "in_progress" || status == "resolved") || (filter == "unresolved" && status == "resolved") || (filter != "all" && filter != "unresolved" && status != filter) {
				return false
			}
		} else if status != filter {
			return false
		}
	}
	return true
}
func ValidateFood(raw json.RawMessage) error {
	value, ok := decode(raw)
	if !ok {
		return ErrInvalid
	}
	o, ok := obj(value, "scope", "as_of", "stale", "counts", "windows")
	if !ok || !meta(o) {
		return ErrInvalid
	}
	c, ok := counts(o["counts"], "pending_submissions", "open_anomalies", "pending_tier_adjustments")
	if !ok {
		return ErrInvalid
	}
	w, ok := obj(o["windows"], "submissions", "anomalies", "tier_adjustments")
	if !ok || !window(w["submissions"], c["pending_submissions"], "pending", 200, false) || !window(w["anomalies"], c["open_anomalies"], "open", 200, false) || !window(w["tier_adjustments"], c["pending_tier_adjustments"], "pending", 200, false) {
		return ErrInvalid
	}
	return nil
}
func ValidTicketFilter(filter string) bool {
	switch filter {
	case "unresolved", "all", "open", "in_progress", "resolved":
		return true
	}
	return false
}
func ValidateTickets(raw json.RawMessage, filter string) error {
	if !ValidTicketFilter(filter) {
		return ErrInvalid
	}
	value, ok := decode(raw)
	if !ok {
		return ErrInvalid
	}
	o, ok := obj(value, "scope", "as_of", "stale", "counts", "window")
	if !ok || !meta(o) {
		return ErrInvalid
	}
	c, ok := counts(o["counts"], "open", "in_progress", "resolved", "unresolved")
	if !ok || c["unresolved"] != c["open"]+c["in_progress"] || c["unresolved"]+c["resolved"] > maxSafe {
		return ErrInvalid
	}
	total := c[filter]
	if filter == "all" {
		total = c["unresolved"] + c["resolved"]
	}
	if !window(o["window"], total, filter, 100, true) {
		return ErrInvalid
	}
	return nil
}
