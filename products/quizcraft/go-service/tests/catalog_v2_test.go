package tests

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	quizcraft "henukit.dev/quizcraft"
)

const (
	portalCatalogClientID = "portal-gateway"
	portalCatalogKeyID    = "portal-catalog-key-1"
	portalCatalogSecret   = "portal-catalog-secret-at-least-32-bytes"
)

type catalogBankResponse struct {
	BankID        string `json:"bank_id"`
	BankVersionID string `json:"bank_version_id"`
	BankKey       string `json:"bank_key"`
	Name          string `json:"name"`
	ContentSHA256 string `json:"content_sha256"`
	QuestionCount int    `json:"question_count"`
	Chapters      []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"chapters"`
}

func TestPracticeHTTPCatalogUsesPublishedQuizCraftV2Facts(t *testing.T) {
	pool := isolatedQuizcraftV2Database(t)
	if err := quizcraft.RequireQuizcraftV2Target(context.Background(), pool); err != nil {
		t.Fatalf("isolated catalog source is not quizcraft_v2: %v", err)
	}
	darkHandler, err := quizcraft.NewPracticeHTTP(quizcraft.PracticeHTTPConfig{
		Database:       pool,
		AuthHMACSecret: []byte(practiceAuthSecret),
	})
	if err != nil {
		t.Fatal(err)
	}
	darkServer := httptest.NewServer(darkHandler)
	defer darkServer.Close()
	status, _ := requestJSON(t, http.MethodGet, darkServer.URL+"/api/v1/banks", nil, nil)
	if status != http.StatusNotFound {
		t.Fatalf("unconfigured V2 catalog route = %d, want 404", status)
	}

	handler, err := quizcraft.NewPracticeHTTP(quizcraft.PracticeHTTPConfig{
		Database:        pool,
		AuthHMACSecret:  []byte(practiceAuthSecret),
		CatalogClientID: portalCatalogClientID,
		CatalogKeys:     map[string]string{portalCatalogKeyID: portalCatalogSecret},
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()

	status, body := requestJSON(t, http.MethodGet, server.URL+"/api/v1/banks", nil, nil)
	if status != http.StatusUnauthorized || bytes.Contains(body, []byte(`"data"`)) {
		t.Fatalf("unsigned V2 catalog = %d %s", status, body)
	}
	bodyRequest := newCatalogRequest(t, server.URL, "portal.practice.read")
	bodyRequest.Body = io.NopCloser(strings.NewReader("unexpected body"))
	bodyRequest.ContentLength = int64(len("unexpected body"))
	status, body = sendCatalogRequest(t, bodyRequest)
	if status != http.StatusUnauthorized || bytes.Contains(body, []byte(`"data"`)) {
		t.Fatalf("body-bearing V2 catalog = %d %s", status, body)
	}
	status, body = requestCatalog(t, server.URL, "portal.library.read")
	if status != http.StatusForbidden || bytes.Contains(body, []byte(`"data"`)) {
		t.Fatalf("wrong-permission V2 catalog = %d %s", status, body)
	}

	status, body = requestCatalog(t, server.URL, "portal.practice.read")
	if status != http.StatusOK {
		t.Fatalf("empty V2 catalog = %d %s", status, body)
	}
	var empty apiEnvelope[[]catalogBankResponse]
	decodeJSON(t, body, &empty)
	if empty.RequestID == "" || empty.Data == nil || len(empty.Data) != 0 {
		t.Fatalf("empty V2 catalog response = %+v", empty)
	}
	replayed := newCatalogRequest(t, server.URL, "portal.practice.read")
	status, body = sendCatalogRequest(t, replayed)
	if status != http.StatusOK {
		t.Fatalf("first replayable V2 catalog request = %d %s", status, body)
	}
	status, body = sendCatalogRequest(t, replayed)
	if status != http.StatusConflict || !bytes.Contains(body, []byte(`"code":"service_replay"`)) || bytes.Contains(body, []byte(`"data"`)) {
		t.Fatalf("replayed V2 catalog request = %d %s", status, body)
	}

	report := importPracticeBank(t, pool, "v2-catalog-contract")
	status, body = requestCatalog(t, server.URL, "portal.practice.read")
	if status != http.StatusOK {
		t.Fatalf("published V2 catalog = %d %s", status, body)
	}
	var published apiEnvelope[[]catalogBankResponse]
	decodeJSON(t, body, &published)
	if published.RequestID == "" || len(published.Data) != 1 {
		t.Fatalf("published V2 catalog = %+v", published)
	}
	bank := published.Data[0]
	if bank.BankID != report.BankID || bank.BankVersionID != report.BankVersionID || bank.BankKey != "v2-catalog-contract" || bank.Name == "" || len(bank.ContentSHA256) != 64 || bank.QuestionCount != len(report.Questions) || len(bank.Chapters) == 0 {
		t.Fatalf("published V2 bank = %+v / import = %+v", bank, report)
	}

	// Catalog membership is availability: an inactive version is not exposed as
	// a false-positive bank that a later Practice session cannot use.
	if _, err := pool.Exec(context.Background(), `UPDATE quizcraft_banks SET active_version_id=NULL WHERE id=$1`, report.BankID); err != nil {
		t.Fatal(err)
	}
	status, body = requestCatalog(t, server.URL, "portal.practice.read")
	if status != http.StatusOK {
		t.Fatalf("inactive V2 catalog = %d %s", status, body)
	}
	var inactive apiEnvelope[[]catalogBankResponse]
	decodeJSON(t, body, &inactive)
	if inactive.Data == nil || len(inactive.Data) != 0 {
		t.Fatalf("inactive bank remained available in catalog: %+v", inactive)
	}

	if _, err := pool.Exec(context.Background(), `DROP TABLE quizcraft_banks CASCADE`); err != nil {
		t.Fatal(err)
	}
	status, body = requestCatalog(t, server.URL, "portal.practice.read")
	if status != http.StatusServiceUnavailable || !bytes.Contains(body, []byte(`"code":"database_unavailable"`)) || bytes.Contains(body, []byte(`"data"`)) {
		t.Fatalf("failed V2 database catalog = %d %s", status, body)
	}
}

func requestCatalog(t *testing.T, baseURL, permission string) (int, []byte) {
	t.Helper()
	return sendCatalogRequest(t, newCatalogRequest(t, baseURL, permission))
}

func newCatalogRequest(t *testing.T, baseURL, permission string) *http.Request {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, baseURL+"/api/v1/banks", nil)
	if err != nil {
		t.Fatal(err)
	}
	nonce := make([]byte, 24)
	if _, err := rand.Read(nonce); err != nil {
		t.Fatal(err)
	}
	nonceText := base64.RawURLEncoding.EncodeToString(nonce)
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	digest := sha256.Sum256(nil)
	canonical := strings.Join([]string{http.MethodGet, request.URL.RequestURI(), timestamp, nonceText, hex.EncodeToString(digest[:])}, "\n")
	mac := hmac.New(sha256.New, []byte(portalCatalogSecret))
	_, _ = mac.Write([]byte(canonical))
	request.SetBasicAuth(portalCatalogClientID, portalCatalogSecret)
	request.Header.Set("X-Service-Id", portalCatalogClientID)
	request.Header.Set("X-Key-Id", portalCatalogKeyID)
	request.Header.Set("X-Timestamp", timestamp)
	request.Header.Set("X-Nonce", nonceText)
	request.Header.Set("X-Signature", base64.RawURLEncoding.EncodeToString(mac.Sum(nil)))
	request.Header.Set("X-Permission-Code", permission)
	request.Header.Set("X-Scope-Kind", "product")
	request.Header.Set("X-Product-Code", "quizcraft")
	request.Header.Set("X-Request-Id", "req_catalog_v2_test")
	return request
}

func sendCatalogRequest(t *testing.T, request *http.Request) (int, []byte) {
	t.Helper()
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var body bytes.Buffer
	if _, err := body.ReadFrom(response.Body); err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, body.Bytes()
}
