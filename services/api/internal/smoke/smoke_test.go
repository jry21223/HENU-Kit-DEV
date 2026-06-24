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
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{"id": "user_1", "email": "smoke@stu.henu.edu.cn", "role": "user"})
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

func TestRunnerCanGrantPackageAndVerifyPaidDownload(t *testing.T) {
	grantCreated := false
	mux := http.NewServeMux()
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{"ready": true})
	})
	mux.HandleFunc("/schools", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{"schools": []map[string]string{{"id": "school_1", "name": "HENU"}}})
	})
	mux.HandleFunc("/packages", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{
			"packages": []map[string]interface{}{{"id": "pkg_1", "title": "Discrete Math", "status": "published", "priceFen": 1990}},
		})
	})
	mux.HandleFunc("/packages/pkg_1", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{
			"package": map[string]interface{}{"id": "pkg_1", "title": "Discrete Math", "status": "published", "priceFen": 1990},
			"materials": []map[string]interface{}{
				{"id": "mat_paid", "title": "Mock Paper", "accessLevel": "paid", "status": "published"},
			},
		})
	})
	mux.HandleFunc("/auth/send-code", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{"devCode": "123456"})
	})
	mux.HandleFunc("/auth/login", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		token := "student_token"
		if body["email"] == "admin@stu.henu.edu.cn" {
			token = "admin_token"
		}
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{"accessToken": token})
	})
	mux.HandleFunc("/auth/me", func(w http.ResponseWriter, r *http.Request) {
		switch r.Header.Get("Authorization") {
		case "Bearer student_token":
			writeEnvelope(t, w, http.StatusOK, map[string]interface{}{"id": "user_1", "email": "smoke@stu.henu.edu.cn", "role": "user"})
		case "Bearer admin_token":
			writeEnvelope(t, w, http.StatusOK, map[string]interface{}{"id": "admin_1", "email": "admin@stu.henu.edu.cn", "role": "admin"})
		default:
			writeEnvelopeWithMessage(t, w, http.StatusUnauthorized, 40001, "unauthorized", nil)
		}
	})
	mux.HandleFunc("/materials/mat_paid/download", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer student_token" {
			writeEnvelopeWithMessage(t, w, http.StatusUnauthorized, 40001, "unauthorized", nil)
			return
		}
		if !grantCreated {
			writeEnvelopeWithMessage(t, w, http.StatusForbidden, 40003, "entitlement_required", nil)
			return
		}
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("paid content")); err != nil {
			t.Fatal(err)
		}
	})
	mux.HandleFunc("/admin/access-grants", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer admin_token" {
			writeEnvelopeWithMessage(t, w, http.StatusForbidden, 40003, "forbidden", nil)
			return
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["userId"] != "user_1" || body["packageId"] != "pkg_1" {
			t.Fatalf("unexpected grant body: %#v", body)
		}
		if _, ok := body["amount"]; ok {
			t.Fatal("smoke grant request must not send amount")
		}
		grantCreated = true
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{"grant": map[string]string{"id": "grant_1"}, "alreadyGranted": false})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	runner, err := NewRunner(Config{
		BaseURL:            server.URL,
		Email:              "smoke@stu.henu.edu.cn",
		AdminEmail:         "admin@stu.henu.edu.cn",
		ExpectPaidDenied:   true,
		GrantPackageAccess: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	result := runner.Run(context.Background())
	if !result.Passed {
		t.Fatalf("expected smoke pass, got %#v", result)
	}
	if !grantCreated {
		t.Fatal("expected smoke runner to create a manual package grant")
	}
	wantChecks := []string{"paid download denied before entitlement", "admin login", "manual package grant", "paid download after grant"}
	for _, want := range wantChecks {
		if !hasPassedCheck(result, want) {
			t.Fatalf("expected passed check %q in %#v", want, result.Checks)
		}
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

func hasPassedCheck(result Result, name string) bool {
	for _, check := range result.Checks {
		if check.Name == name && check.Status == "passed" {
			return true
		}
	}
	return false
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
