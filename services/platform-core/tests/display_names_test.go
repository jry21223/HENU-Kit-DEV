package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func stringPointer(value string) *string { return &value }

func TestResolveUserDisplayNamesReturnsOnlyDisplayNamesForSignedServices(t *testing.T) {
	fixture := newInboxFixture(t)
	ctx := context.Background()
	namedID := uuid.New()
	unnamedID := uuid.New()
	blankID := uuid.New()
	unknownID := uuid.New()
	for _, seed := range []struct {
		id   uuid.UUID
		name *string
	}{
		{namedID, stringPointer("认真刷题")},
		{unnamedID, nil},
		// The users.display_name CHECK allows whitespace-only text; the
		// resolution boundary treats it as unset, mirroring the OAuth
		// exchange display-name normalization.
		{blankID, stringPointer("   ")},
	} {
		if _, err := fixture.pool.Exec(ctx, `INSERT INTO users (id, email_verified, status, display_name) VALUES ($1, true, 'active', $2)`, seed.id, seed.name); err != nil {
			t.Fatalf("seed display-name user %s: %v", seed.id, err)
		}
	}

	body := fmt.Sprintf(`{"user_ids":["%s","%s","%s","%s"]}`, namedID, unnamedID, blankID, unknownID)
	response := sendInboxRequest(t, fixture, http.MethodPost, "/api/v1/users/display-names", body, "", "nonce_dn_"+uuid.NewString(), "req_display_names_ok")
	payload, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("display-names = %d: %s, want 200", response.StatusCode, payload)
	}
	var envelope struct {
		RequestID string             `json:"request_id"`
		Data      map[string]*string `json:"data"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		t.Fatalf("decode display-names response: %v", err)
	}
	if !strings.HasPrefix(envelope.RequestID, "req_") {
		t.Fatalf("display-names request_id = %q", envelope.RequestID)
	}
	if len(envelope.Data) != 4 {
		t.Fatalf("display-names data = %v, want one entry per requested id", envelope.Data)
	}
	if got := envelope.Data[namedID.String()]; got == nil || *got != "认真刷题" {
		t.Fatalf("named user display_name = %v, want 认真刷题", got)
	}
	if got := envelope.Data[unnamedID.String()]; got != nil {
		t.Fatalf("unnamed user display_name = %v, want null", got)
	}
	if got := envelope.Data[blankID.String()]; got != nil {
		t.Fatalf("whitespace display_name = %v, want null", got)
	}
	if got := envelope.Data[unknownID.String()]; got != nil {
		t.Fatalf("unknown user display_name = %v, want null", got)
	}
	// Only display names may cross this boundary: no email/status/other
	// account fields, and no request echo of the batch.
	lower := strings.ToLower(string(payload))
	for _, forbidden := range []string{"email", "status", "user_ids", "email_verified", "created_at"} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("display-names response leaked %q: %s", forbidden, payload)
		}
	}
}

func TestResolveUserDisplayNamesRejectsInvalidBatches(t *testing.T) {
	fixture := newInboxFixture(t)

	tooMany := make([]string, 0, 101)
	for index := 0; index < 101; index++ {
		tooMany = append(tooMany, uuid.NewString())
	}
	quoted := make([]string, 0, len(tooMany))
	for _, id := range tooMany {
		quoted = append(quoted, fmt.Sprintf("%q", id))
	}
	cases := []struct {
		name string
		body string
	}{
		{name: "empty batch", body: `{"user_ids":[]}`},
		{name: "missing user_ids", body: `{}`},
		{name: "over 100", body: `{"user_ids":[` + strings.Join(quoted, ",") + `]}`},
		{name: "duplicate id", body: fmt.Sprintf(`{"user_ids":[%q,%q]}`, namedFixtureID, namedFixtureID)},
		{name: "not a uuid", body: `{"user_ids":["not-a-uuid"]}`},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			response := sendInboxRequest(t, fixture, http.MethodPost, "/api/v1/users/display-names", test.body, "", "nonce_dn_"+uuid.NewString(), "req_display_names_invalid")
			payload, _ := io.ReadAll(response.Body)
			response.Body.Close()
			if response.StatusCode != http.StatusBadRequest || !strings.Contains(string(payload), "INVALID_BATCH_SIZE") {
				t.Fatalf("%s = %d: %s, want 400 INVALID_BATCH_SIZE", test.name, response.StatusCode, payload)
			}
		})
	}
}

const namedFixtureID = "10ca9b18-c303-4b7a-ab14-1241e41b665a"

func TestResolveUserDisplayNamesAuthenticatesTheCallingService(t *testing.T) {
	fixture := newInboxFixture(t)
	ctx := context.Background()
	userID := uuid.New()
	if _, err := fixture.pool.Exec(ctx, `INSERT INTO users (id, email_verified, status, display_name) VALUES ($1, true, 'active', '认证用户')`, userID); err != nil {
		t.Fatalf("seed auth user: %v", err)
	}
	body := fmt.Sprintf(`{"user_ids":[%q]}`, userID.String())

	// An unregistered service credential is rejected before any lookup.
	unregistered := sendInboxRequestAs(t, fixture, "unregistered-client", "primary", "unregistered-secret-with-enough-entropy", fixture.token, http.MethodPost, "/api/v1/users/display-names", body, "", "nonce_dn_"+uuid.NewString(), "req_display_names_unknown_client")
	payload, _ := io.ReadAll(unregistered.Body)
	unregistered.Body.Close()
	if unregistered.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unregistered client = %d: %s, want 401", unregistered.StatusCode, payload)
	}

	// A wrong client secret is rejected (the secret hash never matches).
	wrongSecret := sendInboxRequestAs(t, fixture, testClientID, testKeyID, "wrong-secret-with-enough-entropy-000", fixture.token, http.MethodPost, "/api/v1/users/display-names", body, "", "nonce_dn_"+uuid.NewString(), "req_display_names_wrong_secret")
	payload, _ = io.ReadAll(wrongSecret.Body)
	wrongSecret.Body.Close()
	if wrongSecret.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong secret = %d: %s, want 401", wrongSecret.StatusCode, payload)
	}

	// Replaying the same nonce is rejected with 409.
	replayedNonce := "nonce_dn_replay_" + uuid.NewString()
	first := sendInboxRequest(t, fixture, http.MethodPost, "/api/v1/users/display-names", body, "", replayedNonce, "req_display_names_first")
	first.Body.Close()
	if first.StatusCode != http.StatusOK {
		t.Fatalf("first signed request = %d, want 200", first.StatusCode)
	}
	replay := sendInboxRequest(t, fixture, http.MethodPost, "/api/v1/users/display-names", body, "", replayedNonce, "req_display_names_replay")
	payload, _ = io.ReadAll(replay.Body)
	replay.Body.Close()
	if replay.StatusCode != http.StatusConflict || !strings.Contains(string(payload), "NONCE_ALREADY_USED") {
		t.Fatalf("nonce replay = %d: %s, want 409 NONCE_ALREADY_USED", replay.StatusCode, payload)
	}

	// The valid credential still resolves names (request id order preserved).
	ok := sendInboxRequest(t, fixture, http.MethodPost, "/api/v1/users/display-names", body, "", "nonce_dn_"+uuid.NewString(), "req_display_names_auth_ok")
	payload, _ = io.ReadAll(ok.Body)
	ok.Body.Close()
	if ok.StatusCode != http.StatusOK || !strings.Contains(string(payload), "认证用户") {
		t.Fatalf("signed display-names = %d: %s, want 200 with 认证用户", ok.StatusCode, payload)
	}
}
