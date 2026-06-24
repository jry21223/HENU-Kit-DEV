package smoke

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRunnerPassesInternalSmokeFlow(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{"ready": true})
	})
	mux.HandleFunc("/schools", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{"schools": []map[string]string{{"id": "school_1", "name": "HENU"}}})
	})
	mux.HandleFunc("/packages", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{
			"packages": []map[string]interface{}{{"id": "pkg_1", "title": "Discrete Math", "status": "published", "priceFen": 1990}},
		})
	})
	mux.HandleFunc("/packages/pkg_1", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{
			"package": map[string]interface{}{"id": "pkg_1", "title": "Discrete Math", "status": "published", "priceFen": 1990},
			"materials": []map[string]interface{}{
				{"id": "mat_free", "title": "Sample", "accessLevel": "free", "status": "published"},
				{"id": "mat_paid", "title": "Mock Paper", "accessLevel": "paid", "status": "published"},
			},
			"items": []map[string]string{{"resourceType": "material", "resourceId": "mat_paid"}},
		})
	})
	mux.HandleFunc("/auth/send-code", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{"devCode": "123456"})
	})
	mux.HandleFunc("/auth/login", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{"accessToken": "token_123"})
	})
	mux.HandleFunc("/auth/me", func(w http.ResponseWriter, r *http.Request) {
		requireBearer(t, w, r)
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{"email": "smoke@stu.henu.edu.cn"})
	})
	mux.HandleFunc("/materials/mat_paid/download", func(w http.ResponseWriter, r *http.Request) {
		requireBearer(t, w, r)
		writeEnvelopeWithMessage(t, w, http.StatusForbidden, 40003, "entitlement_required", nil)
	})
	mux.HandleFunc("/orders", func(w http.ResponseWriter, r *http.Request) {
		requireBearer(t, w, r)
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if _, ok := body["amount"]; ok {
			t.Fatal("smoke order request must not send amount")
		}
		if body["packageId"] != "pkg_1" {
			t.Fatalf("unexpected package id: %#v", body)
		}
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{
			"order": map[string]interface{}{"id": "ord_1", "status": "pending", "amountTotal": 1990, "currency": "CNY"},
		})
	})
	mux.HandleFunc("/orders/ord_1/status", func(w http.ResponseWriter, r *http.Request) {
		requireBearer(t, w, r)
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{"orderId": "ord_1", "status": "pending", "entitlementGranted": false})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	runner, err := NewRunner(Config{BaseURL: server.URL, Email: "smoke@stu.henu.edu.cn", CreateOrder: true})
	if err != nil {
		t.Fatal(err)
	}
	result := runner.Run(context.Background())
	if !result.Passed {
		t.Fatalf("expected smoke pass, got %#v", result)
	}
	if result.PackageID != "pkg_1" || result.PaidMaterialID != "mat_paid" {
		t.Fatalf("unexpected smoke ids: %#v", result)
	}
	if len(result.Checks) < 10 {
		t.Fatalf("expected full smoke checks, got %#v", result.Checks)
	}
}

func TestRunnerFailsWhenPackageDetailLeaksStorageKey(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{"ready": true})
	})
	mux.HandleFunc("/schools", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{"schools": []map[string]string{{"id": "school_1"}}})
	})
	mux.HandleFunc("/packages", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{"packages": []map[string]string{{"id": "pkg_1"}}})
	})
	mux.HandleFunc("/packages/pkg_1", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{
			"package":   map[string]string{"id": "pkg_1"},
			"materials": []map[string]string{{"id": "mat_paid", "accessLevel": "paid", "storageKey": "materials/secret.pdf"}},
		})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	runner, err := NewRunner(Config{BaseURL: server.URL, SkipLogin: true})
	if err != nil {
		t.Fatal(err)
	}
	result := runner.Run(context.Background())
	if result.Passed {
		t.Fatalf("expected smoke failure for storageKey leak, got %#v", result)
	}
	last := result.Checks[len(result.Checks)-1]
	if last.Name != "package detail hides storage keys" || !strings.Contains(last.Detail, "storageKey") {
		t.Fatalf("expected storageKey failure, got %#v", last)
	}
}

func writeEnvelope(t *testing.T, w http.ResponseWriter, status int, data interface{}) {
	t.Helper()
	writeEnvelopeWithMessage(t, w, status, 0, "ok", data)
}

func writeEnvelopeWithMessage(t *testing.T, w http.ResponseWriter, status int, code int, message string, data interface{}) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	payload := map[string]interface{}{"code": code, "message": message, "data": data}
	if status >= 400 {
		payload = map[string]interface{}{"code": code, "message": message, "details": data}
	}
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		t.Fatal(err)
	}
}

func requireBearer(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()
	if r.Header.Get("Authorization") != "Bearer token_123" {
		writeEnvelopeWithMessage(t, w, http.StatusUnauthorized, 40001, "unauthorized", nil)
	}
}
