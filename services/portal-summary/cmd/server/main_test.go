package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestKeyRingRejectsMissingOrDuplicateKeyIDs(t *testing.T) {
	if _, err := keyRing("", "active-secret-with-entropy", "", ""); err == nil {
		t.Fatal("missing active key ID must be rejected")
	}
	if _, err := keyRing("portal-key", "active-secret-with-entropy", "portal-key", "retiring-secret-with-entropy"); err == nil {
		t.Fatal("duplicate active and retiring key IDs must be rejected")
	}
	keys, err := keyRing("portal-active", "active-secret-with-entropy", "portal-retiring", "retiring-secret-with-entropy")
	if err != nil || len(keys) != 2 {
		t.Fatalf("valid rotating key ring = %#v, %v", keys, err)
	}
}

func TestVerifySummaryUsesSignedOwnerContract(t *testing.T) {
	const secret = "portal-summary-verification-secret"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		clientID, basicSecret, basic := request.BasicAuth()
		digest := sha256.Sum256(nil)
		canonical := strings.Join([]string{request.Method, request.URL.RequestURI(), request.Header.Get("X-Timestamp"), request.Header.Get("X-Nonce"), hex.EncodeToString(digest[:])}, "\n")
		mac := hmac.New(sha256.New, []byte(secret))
		_, _ = mac.Write([]byte(canonical))
		if !basic || clientID != "console-gateway" || basicSecret != secret || request.Header.Get("X-Service-Id") != clientID || request.Header.Get("X-Key-Id") != "portal-active" || request.Header.Get("X-Request-Id") != "req_portal_release_verify" || !hmac.Equal([]byte(request.Header.Get("X-Signature")), []byte(base64.RawURLEncoding.EncodeToString(mac.Sum(nil)))) {
			http.Error(writer, "bad signature", http.StatusUnauthorized)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		fmt.Fprint(writer, `{"data":{"id":"portal","status":"partial","metrics":[{"label":"a","value":"1"},{"label":"b","value":"1"},{"label":"c","value":"1"},{"label":"d","value":"1"},{"label":"e","value":"1"},{"label":"f","value":"1"},{"label":"g","value":"1"},{"label":"h","value":"1"}],"status_message":"partial","as_of":"2026-08-13T00:00:00Z","request_id":"req_portal_release_verify"},"request_id":"req_portal_release_verify"}`)
	}))
	t.Cleanup(server.Close)
	t.Setenv("PORTAL_SUMMARY_VERIFY_URL", server.URL+"/api/v1/console-summary")
	t.Setenv("PORTAL_SUMMARY_CLIENT_ID", "console-gateway")
	t.Setenv("PORTAL_SUMMARY_ACTIVE_KEY_ID", "portal-active")
	t.Setenv("PORTAL_SUMMARY_ACTIVE_SECRET", secret)
	if err := verifySummary(); err != nil {
		t.Fatal(err)
	}
}
