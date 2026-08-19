package library

import (
	"encoding/json"
	"net/http"
	"testing"
)

// legacyCommandRoute is the allowlist deciding which legacy Study admin
// endpoints a Console command may reach. Anything not named here must be
// refused rather than forwarded.
func TestLegacyCommandRouteAllowsOnlyBoundedCommands(t *testing.T) {
	const id = "8f14e45f-ceea-4d67-a1b0-2d3e4f5a6b7c"

	for _, tc := range []struct {
		kind   string
		method string
		path   string
	}{
		{"course_create", http.MethodPost, "/api/v1/admin/courses"},
		{"course_update", http.MethodPatch, "/api/v1/admin/courses/" + id},
		{"course_archive", http.MethodDelete, "/api/v1/admin/courses/" + id},
		{"material_create", http.MethodPost, "/api/v1/admin/materials"},
		{"material_update", http.MethodPatch, "/api/v1/admin/materials/" + id},
		{"material_archive", http.MethodDelete, "/api/v1/admin/materials/" + id},
		{"submission_approve", http.MethodPost, "/api/v1/admin/materials/" + id + "/approve"},
		{"submission_reject", http.MethodPost, "/api/v1/admin/materials/" + id + "/reject"},
		{"correction_resolve", http.MethodPost, "/api/v1/admin/reports/" + id + "/resolve"},
		{"correction_reject", http.MethodPost, "/api/v1/admin/reports/" + id + "/reject"},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			method, path, err := legacyCommandRoute(commandInput{Kind: tc.kind, ResourceID: id})
			if err != nil {
				t.Fatalf("legacyCommandRoute(%s) returned %v", tc.kind, err)
			}
			if method != tc.method || path != tc.path {
				t.Errorf("legacyCommandRoute(%s) = %s %s, want %s %s", tc.kind, method, path, tc.method, tc.path)
			}
		})
	}
}

func TestLegacyCommandRouteRefusesUnknownCommands(t *testing.T) {
	for _, kind := range []string{
		"", "course_delete", "material_publish", "user_delete",
		"COURSE_CREATE", "course_create ", "../admin/users",
	} {
		t.Run(kind, func(t *testing.T) {
			if _, _, err := legacyCommandRoute(commandInput{Kind: kind, ResourceID: "x"}); err == nil {
				t.Errorf("legacyCommandRoute(%q) was accepted, want a boundary error", kind)
			}
		})
	}
}

// The resource id lands inside the forwarded path, so it must be escaped and
// unable to open a path segment of its own.
func TestLegacyCommandRouteEscapesTheResourceID(t *testing.T) {
	for _, tc := range []struct {
		name string
		id   string
		want string
	}{
		{"path traversal", "../../admin/users", "/api/v1/admin/courses/..%2F..%2Fadmin%2Fusers"},
		{"slash", "a/b", "/api/v1/admin/courses/a%2Fb"},
		{"query separator", "a?b=c", "/api/v1/admin/courses/a%3Fb=c"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, path, err := legacyCommandRoute(commandInput{Kind: "course_update", ResourceID: tc.id})
			if err != nil {
				t.Fatalf("legacyCommandRoute() returned %v", err)
			}
			if path != tc.want {
				t.Errorf("path = %q, want %q", path, tc.want)
			}
		})
	}
}

// filteredPayload is a strict allowlist: a field the Library boundary does not
// model is refused outright rather than quietly dropped, so a Console bug
// cannot smuggle an attribute into the legacy Study admin API.
func TestFilteredPayloadRefusesFieldsOutsideTheBoundary(t *testing.T) {
	for _, tc := range []struct {
		name    string
		kind    string
		payload string
	}{
		{"unmodelled field on a course", "course_update", `{"name":"离散数学","isAdmin":true}`},
		{"unmodelled field on a material", "material_update", `{"title":"讲义","ownerId":"someone"}`},
		{"unmodelled field on a review", "submission_approve", `{"reviewReason":"ok","status":"published"}`},
		{"archive takes no fields at all", "course_archive", `{"name":"x"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := filteredPayload(tc.kind, json.RawMessage(tc.payload)); err == nil {
				t.Errorf("filteredPayload(%s, %s) was accepted, want a boundary error", tc.kind, tc.payload)
			}
		})
	}
}

func TestFilteredPayloadRefusesUnknownCommandKinds(t *testing.T) {
	if _, err := filteredPayload("user_delete", json.RawMessage(`{}`)); err == nil {
		t.Error("filteredPayload accepted an unknown command kind")
	}
}

func TestFilteredPayloadEnforcesRequiredFields(t *testing.T) {
	const courseID = "8f14e45f-ceea-4d67-a1b0-2d3e4f5a6b7c"

	// course_create requires the full identity of the course.
	if _, err := filteredPayload("course_create", json.RawMessage(`{"name":"离散数学"}`)); err == nil {
		t.Error("course_create without its required fields was accepted")
	}

	complete := `{"schoolId":"` + courseID + `","collegeId":"` + courseID + `","majorId":"` + courseID +
		`","grade":"2026","name":"离散数学","slug":"discrete-math"}`
	if _, err := filteredPayload("course_create", json.RawMessage(complete)); err != nil {
		t.Errorf("complete course_create payload rejected: %v", err)
	}

	// The same fields are optional on an update, and the create rules must not
	// leak across into it.
	if _, err := filteredPayload("course_update", json.RawMessage(`{"name":"离散数学"}`)); err != nil {
		t.Errorf("partial course_update rejected: %v", err)
	}

	if _, err := filteredPayload("material_create", json.RawMessage(`{"title":"讲义"}`)); err == nil {
		t.Error("material_create without courseId and storageKey was accepted")
	}
}

func TestFilteredPayloadValidatesFieldShapes(t *testing.T) {
	const id = "8f14e45f-ceea-4d67-a1b0-2d3e4f5a6b7c"

	for _, tc := range []struct {
		name    string
		kind    string
		payload string
	}{
		{"uuid field is not a uuid", "course_update", `{"schoolId":"not-a-uuid"}`},
		{"uuid field is a number", "course_update", `{"schoolId":123}`},
		{"string field is a number", "course_update", `{"name":42}`},
		{"status outside the enum", "course_update", `{"status":"deleted"}`},
		{"material type outside the enum", "material_update", `{"type":"video"}`},
		{"access level outside the enum", "material_update", `{"accessLevel":"paid"}`},
		{"negative file size", "material_create", `{"courseId":"` + id + `","title":"t","storageKey":"k","fileSize":-1}`},
		{"payload is not an object", "course_update", `["name"]`},
		{"payload is malformed json", "course_update", `{"name":`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := filteredPayload(tc.kind, json.RawMessage(tc.payload)); err == nil {
				t.Errorf("filteredPayload(%s, %s) was accepted, want a validation error", tc.kind, tc.payload)
			}
		})
	}
}

// Length caps are expressed in runes, so a field of Chinese text is measured by
// characters rather than by UTF-8 bytes.
func TestFilteredPayloadMeasuresLengthInRunes(t *testing.T) {
	within := make([]rune, 160)
	for i := range within {
		within[i] = '离'
	}
	payload, err := json.Marshal(map[string]string{"name": string(within)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := filteredPayload("course_update", payload); err != nil {
		t.Errorf("a 160-character name was rejected: %v", err)
	}

	over := append(within, '离')
	payload, err = json.Marshal(map[string]string{"name": string(over)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := filteredPayload("course_update", payload); err == nil {
		t.Error("a 161-character name was accepted")
	}
}

func TestFilteredPayloadTreatsAnEmptyPayloadAsAnEmptyObject(t *testing.T) {
	for _, kind := range []string{"course_archive", "material_archive"} {
		t.Run(kind, func(t *testing.T) {
			filtered, err := filteredPayload(kind, nil)
			if err != nil {
				t.Fatalf("filteredPayload(%s, nil) = %v", kind, err)
			}
			if string(filtered) != "{}" {
				t.Errorf("filteredPayload(%s, nil) = %s, want {}", kind, filtered)
			}
		})
	}
}

// The legacy access vocabulary is narrower than the Library one, and anything
// unrecognized must close down to restricted rather than open up.
func TestLibraryAccessLevelFailsClosed(t *testing.T) {
	for _, tc := range []struct{ legacy, want string }{
		{"free", "public"},
		{"login_required", "authenticated"},
		{"", "restricted"},
		{"paid", "restricted"},
		{"public", "restricted"},
		{"FREE", "restricted"},
	} {
		t.Run(tc.legacy, func(t *testing.T) {
			if got := libraryAccessLevel(tc.legacy); got != tc.want {
				t.Errorf("libraryAccessLevel(%q) = %q, want %q", tc.legacy, got, tc.want)
			}
		})
	}
}
