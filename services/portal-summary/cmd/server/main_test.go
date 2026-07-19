package main

import "testing"

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
