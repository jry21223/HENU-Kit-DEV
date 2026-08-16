package tests

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	career "henukit.dev/career"
)

const extractionsPath = "/api/v1/career/profile/extractions"

func resumeTXT() []byte {
	return []byte("姓名：测试同学\n目标：后端开发\n技能：go、postgres")
}

func TestCreateExtractionRejectsUnconfiguredAI(t *testing.T) {
	server, _ := newCareerServer(t, nil)
	defer server.Close()
	response := sendMultipart(t, server.URL, actorA, extractionsPath, "resume.txt", resumeTXT())
	body := readBody(t, response)
	if response.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("unconfigured AI status = %d, want 503: %s", response.StatusCode, body)
	}
	if !strings.Contains(string(body), "AI_UNCONFIGURED") {
		t.Fatalf("unconfigured AI body missing code: %s", body)
	}
}

func TestCreateExtractionValidatesUpload(t *testing.T) {
	server, pool := newCareerServerWithExtract(t, nil, career.NewMockExtractor(), 0)
	defer server.Close()

	cases := []struct {
		name      string
		fileName  string
		content   []byte
		wantCode  int
		wantError string
	}{
		{name: "missing file", fileName: "", content: nil, wantCode: http.StatusBadRequest, wantError: "INVALID_FILE"},
		{name: "unsupported extension", fileName: "resume.exe", content: []byte("MZ..."), wantCode: http.StatusBadRequest, wantError: "INVALID_FILE"},
		{name: "renamed pdf with txt content", fileName: "resume.pdf", content: []byte("不是PDF内容"), wantCode: http.StatusBadRequest, wantError: "INVALID_FILE"},
		{name: "renamed docx with txt content", fileName: "resume.docx", content: []byte("不是DOCX内容"), wantCode: http.StatusBadRequest, wantError: "INVALID_FILE"},
		{name: "empty txt", fileName: "resume.txt", content: []byte{}, wantCode: http.StatusBadRequest, wantError: "INVALID_FILE"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			response := sendMultipart(t, server.URL, actorA, extractionsPath, tc.fileName, tc.content)
			body := readBody(t, response)
			if response.StatusCode != tc.wantCode {
				t.Fatalf("status = %d, want %d: %s", response.StatusCode, tc.wantCode, body)
			}
			if !strings.Contains(string(body), tc.wantError) {
				t.Fatalf("body missing %q: %s", tc.wantError, body)
			}
		})
	}
	var count int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM career_resume_extractions`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("invalid uploads created %d rows, want 0", count)
	}
}

func TestCreateExtractionRejectsOversizedFile(t *testing.T) {
	server, _ := newCareerServerWithExtract(t, nil, career.NewMockExtractor(), 0)
	defer server.Close()
	big := bytes.Repeat([]byte("a"), 10<<20+1)
	response := sendMultipart(t, server.URL, actorA, extractionsPath, "resume.txt", big)
	body := readBody(t, response)
	if response.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized status = %d, want 413: %s", response.StatusCode, body)
	}
	if !strings.Contains(string(body), "FILE_TOO_LARGE") {
		t.Fatalf("oversized body missing code: %s", body)
	}
}

func TestCreateExtractionRateLimitsPerActor(t *testing.T) {
	server, _ := newCareerServerWithExtract(t, nil, career.NewMockExtractor(), 2)
	defer server.Close()
	for attempt := 1; attempt <= 2; attempt++ {
		response := sendMultipart(t, server.URL, actorA, extractionsPath, "resume.txt", resumeTXT())
		if response.StatusCode != http.StatusOK {
			t.Fatalf("attempt %d status = %d, want 200", attempt, response.StatusCode)
		}
		_ = readBody(t, response)
	}
	limited := sendMultipart(t, server.URL, actorA, extractionsPath, "resume.txt", resumeTXT())
	body := readBody(t, limited)
	if limited.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("over-limit status = %d, want 429: %s", limited.StatusCode, body)
	}
	if !strings.Contains(string(body), "EXTRACT_RATE_LIMITED") {
		t.Fatalf("over-limit body missing code: %s", body)
	}
	// A different actor has its own budget.
	other := sendMultipart(t, server.URL, actorB, extractionsPath, "resume.txt", resumeTXT())
	if other.StatusCode != http.StatusOK {
		t.Fatalf("other actor status = %d, want 200: %s", other.StatusCode, readBody(t, other))
	}
	_ = readBody(t, other)
}

func TestExtractionLifecycleCompletesAndPurgesFile(t *testing.T) {
	server, pool := newCareerServerWithExtract(t, nil, career.NewMockExtractor(), 0)
	defer server.Close()
	response := sendMultipart(t, server.URL, actorA, extractionsPath, "resume.txt", resumeTXT())
	body := readBody(t, response)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("create status = %d, want 200: %s", response.StatusCode, body)
	}
	envelope := decodeData(t, body)
	extraction := envelope["extraction"].(map[string]any)
	id := extraction["id"].(string)
	if extraction["status"] != "queued" || extraction["file_name"] != "resume.txt" {
		t.Fatalf("created extraction = %+v", extraction)
	}

	service := server.Config.Handler.(*career.Service)
	done, err := service.Claims().Step(context.Background())
	if err != nil || !done {
		t.Fatalf("worker step done=%v err=%v", done, err)
	}

	status := send(t, server.URL, actorA, http.MethodGet, extractionsPath+"/"+id, nil, "")
	statusBody := readBody(t, status)
	if status.StatusCode != http.StatusOK {
		t.Fatalf("status code = %d: %s", status.StatusCode, statusBody)
	}
	statusData := decodeData(t, statusBody)
	item := statusData["extraction"].(map[string]any)
	if item["status"] != "completed" {
		t.Fatalf("extraction status = %+v", item)
	}
	extracted, ok := item["extracted"].(map[string]any)
	if !ok {
		t.Fatalf("completed extraction missing extracted fields: %+v", item)
	}
	if extracted["target_roles"] != "后端开发、数据分析" || extracted["tech_stack"] != "go、postgres、vue" {
		t.Fatalf("extracted fields = %+v", extracted)
	}
	// The transient file bytes are purged; only the text remains.
	var fileContent []byte
	if err := pool.QueryRow(context.Background(), `SELECT file_content FROM career_resume_extractions WHERE id=$1`, id).Scan(&fileContent); err != nil {
		t.Fatal(err)
	}
	if fileContent != nil {
		t.Fatalf("file bytes not purged after completion (%d bytes)", len(fileContent))
	}
	var extractedStored []byte
	if err := pool.QueryRow(context.Background(), `SELECT extracted FROM career_resume_extractions WHERE id=$1`, id).Scan(&extractedStored); err != nil {
		t.Fatal(err)
	}
	if len(extractedStored) == 0 {
		t.Fatal("extracted text not stored")
	}
}

func TestExtractionFailureLandsOnFailedWithStableCode(t *testing.T) {
	failing := func(context.Context, string, []byte) (career.ExtractedProfile, error) {
		return career.ExtractedProfile{}, errors.New("provider exploded")
	}
	server, pool := newCareerServerWithExtract(t, nil, failing, 0)
	defer server.Close()
	response := sendMultipart(t, server.URL, actorA, extractionsPath, "resume.txt", resumeTXT())
	body := readBody(t, response)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("create status = %d: %s", response.StatusCode, body)
	}
	id := decodeData(t, body)["extraction"].(map[string]any)["id"].(string)

	service := server.Config.Handler.(*career.Service)
	if done, err := service.Claims().Step(context.Background()); err != nil || !done {
		t.Fatalf("worker step done=%v err=%v", done, err)
	}

	status := send(t, server.URL, actorA, http.MethodGet, extractionsPath+"/"+id, nil, "")
	item := decodeData(t, readBody(t, status))["extraction"].(map[string]any)
	if item["status"] != "failed" {
		t.Fatalf("extraction status = %+v, want failed", item)
	}
	if item["error_code"] != "EXTRACT_FAILED" {
		t.Fatalf("error_code = %v, want EXTRACT_FAILED", item["error_code"])
	}
	if strings.Contains(item["error_message"].(string), "provider exploded") {
		t.Fatalf("internal cause leaked to the browser: %v", item["error_message"])
	}
	var fileContent []byte
	if err := pool.QueryRow(context.Background(), `SELECT file_content FROM career_resume_extractions WHERE id=$1`, id).Scan(&fileContent); err != nil {
		t.Fatal(err)
	}
	if fileContent != nil {
		t.Fatal("file bytes not purged after failure")
	}
}

func TestExtractionIsActorScoped(t *testing.T) {
	server, _ := newCareerServerWithExtract(t, nil, career.NewMockExtractor(), 0)
	defer server.Close()
	response := sendMultipart(t, server.URL, actorA, extractionsPath, "resume.txt", resumeTXT())
	id := decodeData(t, readBody(t, response))["extraction"].(map[string]any)["id"].(string)

	other := send(t, server.URL, actorB, http.MethodGet, extractionsPath+"/"+id, nil, "")
	if other.StatusCode != http.StatusNotFound {
		t.Fatalf("other actor status = %d, want 404: %s", other.StatusCode, readBody(t, other))
	}
	bad := send(t, server.URL, actorA, http.MethodGet, extractionsPath+"/not-a-uuid", nil, "")
	if bad.StatusCode != http.StatusNotFound {
		t.Fatalf("bad id status = %d, want 404", bad.StatusCode)
	}
}

func TestExtractionStaleRunningIsReclaimed(t *testing.T) {
	server, pool := newCareerServerWithExtract(t, nil, career.NewMockExtractor(), 0)
	defer server.Close()
	response := sendMultipart(t, server.URL, actorA, extractionsPath, "resume.txt", resumeTXT())
	id := decodeData(t, readBody(t, response))["extraction"].(map[string]any)["id"].(string)
	// Simulate a crashed worker: the row is stuck running since before the
	// stale window.
	if _, err := pool.Exec(context.Background(), `UPDATE career_resume_extractions SET status='running',started_at=now()-interval '20 minutes' WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	service := server.Config.Handler.(*career.Service)
	if done, err := service.Claims().Step(context.Background()); err != nil || !done {
		t.Fatalf("worker step done=%v err=%v", done, err)
	}
	status := send(t, server.URL, actorA, http.MethodGet, extractionsPath+"/"+id, nil, "")
	item := decodeData(t, readBody(t, status))["extraction"].(map[string]any)
	if item["status"] != "completed" {
		t.Fatalf("reclaimed extraction status = %+v, want completed", item)
	}
}

func TestCreateExtractionRejectsMalformedMultipart(t *testing.T) {
	server, _ := newCareerServerWithExtract(t, nil, career.NewMockExtractor(), 0)
	defer server.Close()
	// A signed body that is not multipart at all.
	response := send(t, server.URL, actorA, http.MethodPost, extractionsPath, []byte(`{"file":"not a file"}`), "")
	body := readBody(t, response)
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("malformed status = %d, want 400: %s", response.StatusCode, body)
	}
}
