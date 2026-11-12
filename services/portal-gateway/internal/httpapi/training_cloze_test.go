package httpapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func clozeSnapshot(t *testing.T, hints ...string) []byte {
	t.Helper()
	var envelope map[string]any
	if err := json.Unmarshal(trainingPending(t), &envelope); err != nil {
		t.Fatal(err)
	}
	data := envelope["data"].(map[string]any)
	unit := data["unit"].(map[string]any)
	unit["kind"] = "cloze"
	delete(unit, "media")
	unit["parts"] = []any{map[string]any{"part_id": "context", "kind": "text", "text": "Read the whole passage. "}, map[string]any{"part_id": "blank-part-a", "kind": "blank", "blank_id": "blank-a"}, map[string]any{"part_id": "blank-part-b", "kind": "blank", "blank_id": "blank-b"}}
	unit["blanks"] = []any{map[string]any{"blank_id": "blank-a", "question_id": "77777777-7777-4777-8777-777777777777"}, map[string]any{"blank_id": "blank-b", "question_id": "99999999-9999-4999-8999-999999999999"}}
	unit["hints"] = []any{map[string]any{"hint_id": "context-hint", "level_count": 2}}
	disclosures := []any{}
	for _, hint := range hints {
		var value any
		if err := json.Unmarshal([]byte(hint), &value); err != nil {
			t.Fatal(err)
		}
		disclosures = append(disclosures, value)
	}
	data["hint_disclosures"] = disclosures
	raw, _ := json.Marshal(envelope)
	return raw
}
func clozeHint() string {
	return `{"disclosure_id":"dddddddd-dddd-4ddd-8ddd-dddddddddddd","hint_id":"context-hint","level":1,"text":"Read both sides of the blank.","affected_question_ids":["77777777-7777-4777-8777-777777777777","99999999-9999-4999-8999-999999999999"],"disclosed_at":"2026-11-12T08:01:00Z"}`
}

func TestClozeHintVerifiesCommittedOwnerSnapshotBeforeReturningText(t *testing.T) {
	hint := clozeHint()
	calls := []string{}
	nonces := map[string]bool{}
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		assertTrainingSignature(t, r, raw, practiceCommandUserID)
		if nonces[r.Header.Get("X-Nonce")] {
			t.Error("nonce reused")
		}
		nonces[r.Header.Get("X-Nonce")] = true
		calls = append(calls, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "POST" {
			if string(raw) != `{"unit_version_id":"44444444-4444-4444-8444-444444444444","hint_id":"context-hint","level":1}` || r.Header.Get("X-Key-Id") != "portal-practice-command-key" || r.Header.Get("Cookie") != "quizcraft_anonymous=original-guest" {
				t.Errorf("unsafe command %s %v", raw, r.Header)
			}
			w.Header().Set("Set-Cookie", "quizcraft_anonymous=claimed; Secure; HttpOnly; SameSite=Lax; Path=/")
			_, _ = fmt.Fprintf(w, `{"request_id":"req_hint","data":%s}`, hint)
		} else {
			if r.Header.Get("X-Key-Id") != "training-read-key" || r.Header.Get("Cookie") != "quizcraft_anonymous=claimed" {
				t.Errorf("unsafe owner verification %v", r.Header)
			}
			_, _ = w.Write(clozeSnapshot(t, hint))
		}
	}))
	defer core.Close()
	h := trainingHandler(t, core.URL, true)
	req := authenticatedPracticeCommandRequest(t, h, "POST", "/api/v1/practice/training/sessions/"+trainingSessionID+"/hints", `{"unit_version_id":"44444444-4444-4444-8444-444444444444","hint_id":"context-hint","level":1}`, "cloze-hint-exact-key-01")
	req.Header.Set("X-Actor-User-Id", "spoofed")
	req.AddCookie(&http.Cookie{Name: "quizcraft_anonymous", Value: "original-guest"})
	req.AddCookie(&http.Cookie{Name: "unrelated", Value: "hidden"})
	rec := httptest.NewRecorder()
	h.Router().ServeHTTP(rec, req)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "Read both sides") || rec.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("hint %d %s", rec.Code, rec.Body.String())
	}
	if len(calls) != 2 || calls[0] != "POST /api/v1/portal/practice/training/sessions/"+trainingSessionID+"/hints" || calls[1] != "GET /api/v1/portal/practice/training/sessions/"+trainingSessionID {
		t.Fatalf("verification %v", calls)
	}
}

func TestClozeHintRejectsUnverifiableDisclosureAndRetainsGates(t *testing.T) {
	for _, name := range []string{"different-text", "different-id", "different-time", "different-hint", "different-level", "different-affected", "uncommitted", "foreign-version", "foreign-session", "read-failed", "invalid-login", "reads-off", "commands-off", "unknown-input", "duplicate-input"} {
		t.Run(name, func(t *testing.T) {
			returned := clozeHint()
			snapshot := clozeSnapshot(t, clozeHint())
			calls := 0
			switch name {
			case "different-text":
				returned = strings.ReplaceAll(returned, "Read both sides of the blank.", "private foreign hint")
			case "different-id":
				returned = strings.ReplaceAll(returned, "dddddddd-dddd-4ddd-8ddd-dddddddddddd", "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee")
			case "different-time":
				returned = strings.ReplaceAll(returned, "08:01:00Z", "08:01:01Z")
			case "different-hint":
				returned = strings.ReplaceAll(returned, "context-hint", "other-hint")
			case "different-level":
				returned = strings.ReplaceAll(returned, `"level":1`, `"level":2`)
			case "different-affected":
				returned = strings.ReplaceAll(returned, `,"99999999-9999-4999-8999-999999999999"`, "")
			case "uncommitted":
				snapshot = clozeSnapshot(t)
			case "foreign-version":
				snapshot = []byte(strings.ReplaceAll(string(snapshot), "44444444-4444-4444-8444-444444444444", "11111111-1111-4111-8111-111111111111"))
			case "foreign-session":
				snapshot = []byte(strings.ReplaceAll(string(snapshot), trainingSessionID, "11111111-1111-4111-8111-111111111111"))
			}
			core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("Content-Type", "application/json")
				if r.Method == "POST" {
					_, _ = fmt.Fprintf(w, `{"request_id":"req_hint","data":%s}`, returned)
				} else if name == "read-failed" {
					w.WriteHeader(404)
					_, _ = w.Write([]byte(`{"request_id":"req_missing","error":{"code":"NOT_FOUND","message":"Training is unavailable"}}`))
				} else {
					_, _ = w.Write(snapshot)
				}
			}))
			defer core.Close()
			h := trainingHandler(t, core.URL, true)
			want := 502
			if name == "reads-off" {
				h.quizCraft = nil
				want = 503
			}
			if name == "commands-off" {
				h.practiceCommands = nil
				want = 503
			}
			input := `{"unit_version_id":"44444444-4444-4444-8444-444444444444","hint_id":"context-hint","level":1}`
			if name == "unknown-input" {
				input = strings.TrimSuffix(input, "}") + `,"assisted":false}`
				want = 400
			}
			if name == "duplicate-input" {
				input = strings.TrimSuffix(input, "}") + `,"level":2}`
				want = 400
			}
			req := authenticatedPracticeCommandRequest(t, h, "POST", "/api/v1/practice/training/sessions/"+trainingSessionID+"/hints", input, "cloze-hint-invalid-key-01")
			if name == "invalid-login" {
				req.Header.Set("Cookie", "__Host-henukit_portal_session=invalid; quizcraft_anonymous=guest")
				want = 401
			}
			rec := httptest.NewRecorder()
			h.Router().ServeHTTP(rec, req)
			if rec.Code != want || strings.Contains(rec.Body.String(), "Read both sides") || strings.Contains(rec.Body.String(), "private foreign hint") {
				t.Fatalf("hint %d %s, want %d", rec.Code, rec.Body.String(), want)
			}
			if want != 502 && calls != 0 {
				t.Fatalf("rejected browser request reached owner %d times", calls)
			}
		})
	}
}

func TestClozeRecoveryRejectsInvalidStableBindingsAndHintFacts(t *testing.T) {
	for _, name := range []string{"duplicate-part", "duplicate-blank", "missing-blank", "foreign-blank-question", "duplicate-question-blank", "duplicate-hint", "unknown-hint", "skipped-hint", "foreign-affected", "duplicate-affected", "missing-unsubmitted-affected", "duplicate-disclosure"} {
		t.Run(name, func(t *testing.T) {
			var envelope map[string]any
			_ = json.Unmarshal(clozeSnapshot(t, clozeHint()), &envelope)
			data := envelope["data"].(map[string]any)
			unit := data["unit"].(map[string]any)
			parts := unit["parts"].([]any)
			blanks := unit["blanks"].([]any)
			hints := unit["hints"].([]any)
			disclosures := data["hint_disclosures"].([]any)
			hint := disclosures[0].(map[string]any)
			switch name {
			case "duplicate-part":
				parts[1].(map[string]any)["part_id"] = "context"
			case "duplicate-blank":
				parts[2].(map[string]any)["blank_id"] = "blank-a"
			case "missing-blank":
				parts[2] = map[string]any{"part_id": "blank-part-b", "kind": "text", "text": "A missing blank"}
			case "foreign-blank-question":
				blanks[0].(map[string]any)["question_id"] = "11111111-1111-4111-8111-111111111111"
			case "duplicate-question-blank":
				blanks[1].(map[string]any)["question_id"] = blanks[0].(map[string]any)["question_id"]
			case "duplicate-hint":
				unit["hints"] = append(hints, hints[0])
			case "unknown-hint":
				hint["hint_id"] = "foreign-hint"
			case "skipped-hint":
				hint["level"] = 2
			case "foreign-affected":
				hint["affected_question_ids"] = []any{"11111111-1111-4111-8111-111111111111"}
			case "duplicate-affected":
				hint["affected_question_ids"] = []any{"77777777-7777-4777-8777-777777777777", "77777777-7777-4777-8777-777777777777"}
			case "missing-unsubmitted-affected":
				hint["affected_question_ids"] = []any{"77777777-7777-4777-8777-777777777777"}
			case "duplicate-disclosure":
				data["hint_disclosures"] = append(disclosures, disclosures[0])
			}
			snapshot, _ := json.Marshal(envelope)
			core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(snapshot)
			}))
			defer core.Close()
			h := trainingHandler(t, core.URL, true)
			rec := httptest.NewRecorder()
			h.Router().ServeHTTP(rec, authenticatedPracticeCommandRequest(t, h, "GET", "/api/v1/practice/training/sessions/"+trainingSessionID, "", "unused-idempotency-key"))
			if rec.Code != 502 || strings.Contains(rec.Body.String(), "Read both sides") {
				t.Fatalf("invalid recovery %d %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func clozeCompleted(t *testing.T) []byte {
	t.Helper()
	var envelope, completed map[string]any
	_ = json.Unmarshal(clozeSnapshot(t), &envelope)
	_ = json.Unmarshal(trainingCompleted(t), &completed)
	data := envelope["data"].(map[string]any)
	data["phase"] = "completed"
	data["saved_count"] = 2
	data["receipts"] = completed["data"].(map[string]any)["receipts"]
	data["revealed_layers"] = []string{"english", "chinese"}
	raw, _ := json.Marshal(envelope)
	return raw
}
func clozeReview(t *testing.T) []byte {
	t.Helper()
	var envelope map[string]any
	_ = json.Unmarshal(trainingReviewWithLayers(t), &envelope)
	data := envelope["data"].(map[string]any)
	data["kind"] = "cloze"
	delete(data, "segments")
	for _, value := range data["questions"].([]any) {
		value.(map[string]any)["evidence"] = map[string]any{"kind": "cloze", "part_ids": []string{"context", "blank-part-a"}}
	}
	data["layers"] = []any{
		map[string]any{"disclosure_id": "dddddddd-dddd-4ddd-8ddd-dddddddddddd", "disclosed_at": "2026-11-12T08:01:00Z", "content": map[string]any{"kind": "cloze", "layer": "english", "parts": []any{map[string]any{"part_id": "context", "text": "Read the whole passage. "}, map[string]any{"part_id": "blank-part-a", "text": "The sun made him warm."}, map[string]any{"part_id": "blank-part-b", "text": "The wind"}}}},
		map[string]any{"disclosure_id": "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee", "disclosed_at": "2026-11-12T08:02:00Z", "content": map[string]any{"kind": "cloze", "layer": "chinese", "parts": []any{map[string]any{"part_id": "context", "text": "先读完整篇文章。"}, map[string]any{"part_id": "blank-part-a", "text": "太阳使他暖和起来。"}, map[string]any{"part_id": "blank-part-b", "text": "风"}}}},
	}
	raw, _ := json.Marshal(envelope)
	return raw
}
func TestClozeReviewUsesExactPinnedContextEvidenceAndLayers(t *testing.T) {
	for _, name := range []string{"valid", "foreign-evidence", "duplicate-evidence", "foreign-layer-part", "missing-layer-part", "duplicate-layer-part", "reordered-layer", "changed-context", "wrong-filled-answer", "foreign-key", "foreign-member", "unrevealed-layer"} {
		t.Run(name, func(t *testing.T) {
			var envelope map[string]any
			_ = json.Unmarshal(clozeReview(t), &envelope)
			data := envelope["data"].(map[string]any)
			questions := data["questions"].([]any)
			first := questions[0].(map[string]any)
			layers := data["layers"].([]any)
			parts := layers[0].(map[string]any)["content"].(map[string]any)["parts"].([]any)
			snapshot := clozeCompleted(t)
			switch name {
			case "foreign-evidence":
				first["evidence"] = map[string]any{"kind": "cloze", "part_ids": []string{"foreign-part"}}
			case "duplicate-evidence":
				first["evidence"] = map[string]any{"kind": "cloze", "part_ids": []string{"context", "context"}}
			case "foreign-layer-part":
				parts[0].(map[string]any)["part_id"] = "foreign-part"
			case "missing-layer-part":
				layers[0].(map[string]any)["content"].(map[string]any)["parts"] = parts[:2]
			case "duplicate-layer-part":
				parts[1].(map[string]any)["part_id"] = "context"
			case "reordered-layer":
				parts[0], parts[1] = parts[1], parts[0]
			case "changed-context":
				parts[0].(map[string]any)["text"] = "private foreign passage"
			case "wrong-filled-answer":
				parts[1].(map[string]any)["text"] = "The wind tore it away."
			case "foreign-key":
				first["expected_answer"] = 1
			case "foreign-member":
				first["question_version_id"] = "11111111-1111-4111-8111-111111111111"
			case "unrevealed-layer":
				snapshot = []byte(strings.ReplaceAll(string(snapshot), `["english","chinese"]`, `["english"]`))
			}
			review, _ := json.Marshal(envelope)
			core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if strings.HasSuffix(r.URL.Path, "/review") {
					_, _ = w.Write(review)
				} else {
					_, _ = w.Write(snapshot)
				}
			}))
			defer core.Close()
			h := trainingHandler(t, core.URL, true)
			rec := httptest.NewRecorder()
			h.Router().ServeHTTP(rec, authenticatedPracticeCommandRequest(t, h, "GET", "/api/v1/practice/training/sessions/"+trainingSessionID+"/review", "", "unused-idempotency-key"))
			want := 502
			if name == "valid" {
				want = 200
			}
			if rec.Code != want {
				t.Fatalf("review %d %s, want %d", rec.Code, rec.Body.String(), want)
			}
			if want == 502 && (strings.Contains(rec.Body.String(), "private foreign passage") || strings.Contains(rec.Body.String(), "The sun made him warm")) {
				t.Fatal("invalid review text crossed Gateway")
			}
		})
	}
}
