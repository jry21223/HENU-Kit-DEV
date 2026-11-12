package tests

import (
	"encoding/json"
	"github.com/google/uuid"
	quizcraft "henukit.dev/quizcraft"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Invoked only by scripts/test-training-stack.sh, against a disposable database.
// Publication uses the normal authenticated Workshop HTTP lifecycle explicitly.
func TestTrainingBrowserCoreHarness(t *testing.T) {
	directory := os.Getenv("TRAINING_STACK_DIRECTORY")
	if directory == "" {
		t.Skip("explicit browser stack acceptance harness")
	}
	pool := isolatedQuizcraftV2Database(t)
	handler, err := quizcraft.NewPracticeHTTP(quizcraft.PracticeHTTPConfig{Database: pool, AuthHMACSecret: []byte(practiceAuthSecret), AllowTestWorkshopClaims: true, CatalogClientID: portalCatalogClientID, CatalogKeys: map[string]string{portalCatalogKeyID: portalCatalogSecret}, PortalCommandsEnabled: true, PortalCommandClientID: portalCommandClientID, PortalCommandKeys: map[string]string{portalCommandKeyID: portalCommandSecret}})
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:"+os.Getenv("TRAINING_CORE_PORT"))
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(handler)
	server.Listener = listener
	server.Start()
	defer server.Close()
	for _, filename := range []string{"north-wind.import.json", "campus-repair.import.json"} {
		actor := uuid.NewString()
		auth := "quizcraft_session=" + workshopToken(t, actor, []string{"quizcraft.workshop.write", "quizcraft.workshop.publish"}, []map[string]string{{"kind": "product", "product_code": "quizcraft"}})
		status, body := requestJSON(t, "POST", server.URL+"/api/v1/workshop/banks", map[string]string{"Cookie": auth, "Idempotency-Key": "browser-training-bank-" + actor}, map[string]any{"bank_key": "browser-listening-" + actor, "name": "English listening"})
		if status != 201 {
			t.Fatalf("create bank %d %s", status, body)
		}
		bank := operationResourceID(t, body)
		raw, err := os.ReadFile("../content/english-training/" + filename)
		if err != nil {
			t.Fatal(err)
		}
		var input map[string]any
		_ = json.Unmarshal(raw, &input)
		status, body = requestJSON(t, "POST", server.URL+"/api/v1/workshop/banks/"+bank+"/imports", map[string]string{"Cookie": auth, "Idempotency-Key": "browser-training-import-" + actor}, input)
		if status != 201 {
			t.Fatalf("import %d %s", status, body)
		}
		var report apiEnvelope[quizcraft.ImportReport]
		decodeJSON(t, body, &report)
		for i, action := range []string{"validate", "publish"} {
			status, body = requestJSON(t, "POST", server.URL+"/api/v1/workshop/banks/"+bank+"/versions/"+report.Data.BankVersionID+"/"+action, map[string]string{"Cookie": auth, "Idempotency-Key": "browser-training-" + action + actor}, map[string]any{"expected_version": 2 + i, "note": "Isolated browser acceptance only; listening human ear review pending"})
			if status != 200 {
				t.Fatalf("%s %d %s", action, status, body)
			}
		}
	}
	if err = os.WriteFile(filepath.Join(directory, "core-ready"), []byte(server.URL), 0600); err != nil {
		t.Fatal(err)
	}
	deadline := time.NewTimer(15 * time.Minute)
	defer deadline.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-deadline.C:
			t.Fatal("browser acceptance timed out")
		case <-ticker.C:
			if _, err := os.Stat(filepath.Join(directory, "stop")); err == nil {
				return
			}
		}
	}
}
