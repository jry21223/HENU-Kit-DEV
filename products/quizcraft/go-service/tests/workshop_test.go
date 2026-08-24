package tests

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	quizcraft "henukit.dev/quizcraft"
)

func TestWorkshopLifecycleIsScopedVersionedValidatedAndAudited(t *testing.T) {
	pool := practicePool(t)
	handler, err := quizcraft.NewPracticeHTTP(quizcraft.PracticeHTTPConfig{Database: pool, AuthHMACSecret: []byte(practiceAuthSecret), AllowTestWorkshopClaims: true})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	actorID := uuid.NewString()
	writeAuth := "quizcraft_session=" + workshopToken(t, actorID, []string{"quizcraft.workshop.write"}, []map[string]string{{"kind": "product", "product_code": "quizcraft"}})
	publishAuth := "quizcraft_session=" + workshopToken(t, actorID, []string{"quizcraft.workshop.publish"}, []map[string]string{{"kind": "product", "product_code": "quizcraft"}})
	readAuth := "quizcraft_session=" + workshopToken(t, actorID, []string{"quizcraft.workshop.read"}, []map[string]string{{"kind": "product", "product_code": "quizcraft"}})

	guest, _ := requestJSON(t, http.MethodPost, server.URL+"/api/v1/workshop/banks", map[string]string{"Idempotency-Key": "workshop-guest-create"}, map[string]any{"bank_key": "scope-bank", "name": "Scope Bank"})
	if guest != http.StatusUnauthorized {
		t.Fatalf("guest workshop create = %d", guest)
	}
	wrongScopeAuth := "quizcraft_session=" + workshopToken(t, actorID, []string{"quizcraft.workshop.write"}, []map[string]string{{"kind": "product", "product_code": "food"}})
	wrongScope, _ := requestJSON(t, http.MethodPost, server.URL+"/api/v1/workshop/banks", map[string]string{"Cookie": wrongScopeAuth, "Idempotency-Key": "workshop-wrong-scope"}, map[string]any{"bank_key": "scope-bank", "name": "Scope Bank"})
	if wrongScope != http.StatusForbidden {
		t.Fatalf("wrong product scope = %d", wrongScope)
	}

	createStatus, createBody := requestJSON(t, http.MethodPost, server.URL+"/api/v1/workshop/banks", map[string]string{"Cookie": writeAuth, "Idempotency-Key": "workshop-create-bank-001"}, map[string]any{"bank_key": "scope-bank", "name": "Scope Bank"})
	if createStatus != http.StatusCreated {
		t.Fatalf("create bank = %d %s", createStatus, createBody)
	}
	bankID := operationResourceID(t, createBody)
	bankWriteAuth := "quizcraft_session=" + workshopToken(t, actorID, []string{"quizcraft.workshop.write"}, []map[string]string{{"kind": "resource", "product_code": "quizcraft", "resource_type": "bank", "resource_id": bankID}})
	replayStatus, replayBody := requestJSON(t, http.MethodPost, server.URL+"/api/v1/workshop/banks", map[string]string{"Cookie": writeAuth, "Idempotency-Key": "workshop-create-bank-001"}, map[string]any{"bank_key": "scope-bank", "name": "Scope Bank"})
	if replayStatus != createStatus || !bytes.Equal(replayBody, createBody) {
		t.Fatalf("create replay changed = %d %s", replayStatus, replayBody)
	}
	conflict, _ := requestJSON(t, http.MethodPost, server.URL+"/api/v1/workshop/banks", map[string]string{"Cookie": writeAuth, "Idempotency-Key": "workshop-create-bank-001"}, map[string]any{"bank_key": "scope-other", "name": "Other"})
	if conflict != http.StatusConflict {
		t.Fatalf("idempotency conflict = %d", conflict)
	}

	questions := []map[string]any{{"source_question_id": "q1", "type": "single", "chapter_id": "ch1", "chapter": "第一章", "content": "1+1=?", "options": []string{"1", "2"}, "answer": 1, "analysis": "2"}}
	draftStatus, draftBody := requestJSON(t, http.MethodPost, fmt.Sprintf("%s/api/v1/workshop/banks/%s/versions", server.URL, bankID), map[string]string{"Cookie": bankWriteAuth, "Idempotency-Key": "workshop-create-version-001"}, map[string]any{"expected_version": 1, "questions": questions})
	if draftStatus != http.StatusCreated {
		t.Fatalf("create draft = %d %s", draftStatus, draftBody)
	}
	versionID := operationResourceID(t, draftBody)
	detailStatus, _ := requestJSON(t, http.MethodGet, fmt.Sprintf("%s/api/v1/workshop/banks/%s/versions/%s", server.URL, bankID, versionID), map[string]string{"Cookie": bankWriteAuth}, nil)
	if detailStatus != http.StatusForbidden {
		t.Fatalf("write-only actor read version detail = %d", detailStatus)
	}
	bankReadAuth := "quizcraft_session=" + workshopToken(t, actorID, []string{"quizcraft.workshop.read"}, []map[string]string{{"kind": "resource", "product_code": "quizcraft", "resource_type": "bank", "resource_id": bankID}})
	detailStatus, detailBody := requestJSON(t, http.MethodGet, fmt.Sprintf("%s/api/v1/workshop/banks/%s/versions/%s", server.URL, bankID, versionID), map[string]string{"Cookie": bankReadAuth}, nil)
	if detailStatus != http.StatusOK || !bytes.Contains(detailBody, []byte(`"content":"1+1=?"`)) || !bytes.Contains(detailBody, []byte(`"answer":1`)) {
		t.Fatalf("human validation detail = %d %s", detailStatus, detailBody)
	}
	duplicateStatus, _ := requestJSON(t, http.MethodPost, fmt.Sprintf("%s/api/v1/workshop/banks/%s/versions", server.URL, bankID), map[string]string{"Cookie": bankWriteAuth, "Idempotency-Key": "workshop-duplicate-version"}, map[string]any{"expected_version": 2, "questions": questions})
	if duplicateStatus != http.StatusConflict {
		t.Fatalf("duplicate immutable draft = %d", duplicateStatus)
	}
	premature, _ := requestJSON(t, http.MethodPost, fmt.Sprintf("%s/api/v1/workshop/banks/%s/versions/%s/publish", server.URL, bankID, versionID), map[string]string{"Cookie": publishAuth, "Idempotency-Key": "workshop-premature-publish"}, map[string]any{"expected_version": 2, "note": "not reviewed"})
	if premature != http.StatusConflict {
		t.Fatalf("unvalidated publish = %d", premature)
	}
	validateStatus, _ := requestJSON(t, http.MethodPost, fmt.Sprintf("%s/api/v1/workshop/banks/%s/versions/%s/validate", server.URL, bankID, versionID), map[string]string{"Cookie": bankWriteAuth, "Idempotency-Key": "workshop-validate-version"}, map[string]any{"expected_version": 2, "note": "人工逐题校验"})
	if validateStatus != http.StatusOK {
		t.Fatalf("validate = %d", validateStatus)
	}
	publishStatus, _ := requestJSON(t, http.MethodPost, fmt.Sprintf("%s/api/v1/workshop/banks/%s/versions/%s/publish", server.URL, bankID, versionID), map[string]string{"Cookie": publishAuth, "Idempotency-Key": "workshop-publish-version"}, map[string]any{"expected_version": 3, "note": "publish"})
	if publishStatus != http.StatusOK {
		t.Fatalf("publish = %d", publishStatus)
	}
	stale, _ := requestJSON(t, http.MethodPost, fmt.Sprintf("%s/api/v1/workshop/banks/%s/versions/%s/unpublish", server.URL, bankID, versionID), map[string]string{"Cookie": publishAuth, "Idempotency-Key": "workshop-stale-unpublish"}, map[string]any{"expected_version": 3})
	if stale != http.StatusConflict {
		t.Fatalf("stale lifecycle version = %d", stale)
	}
	unpublishStatus, _ := requestJSON(t, http.MethodPost, fmt.Sprintf("%s/api/v1/workshop/banks/%s/versions/%s/unpublish", server.URL, bankID, versionID), map[string]string{"Cookie": publishAuth, "Idempotency-Key": "workshop-unpublish-version"}, map[string]any{"expected_version": 4, "note": "withdraw"})
	if unpublishStatus != http.StatusOK {
		t.Fatalf("unpublish = %d", unpublishStatus)
	}
	rollbackStatus, _ := requestJSON(t, http.MethodPost, fmt.Sprintf("%s/api/v1/workshop/banks/%s/rollback", server.URL, bankID), map[string]string{"Cookie": publishAuth, "Idempotency-Key": "workshop-rollback-version"}, map[string]any{"expected_version": 5, "target_bank_version_id": versionID, "note": "restore"})
	if rollbackStatus != http.StatusOK {
		t.Fatalf("rollback = %d", rollbackStatus)
	}
	importQuestions := []map[string]any{{"source_question_id": "q2", "type": "blank", "chapter_id": "ch2", "chapter": "第二章", "content": "Go 入口包是____", "answer": "main"}}
	importStatus, importBody := requestJSON(t, http.MethodPost, fmt.Sprintf("%s/api/v1/workshop/banks/%s/imports", server.URL, bankID), map[string]string{"Cookie": bankWriteAuth, "Idempotency-Key": "workshop-import-version"}, map[string]any{"expected_version": 6, "source_sha256": strings.Repeat("a", 64), "questions": importQuestions})
	if importStatus != http.StatusCreated || !bytes.Contains(importBody, []byte(`"accepted":true`)) || !bytes.Contains(importBody, []byte(`"source_sha256":"`+strings.Repeat("a", 64)+`"`)) {
		t.Fatalf("import draft = %d %s", importStatus, importBody)
	}
	var importedVersionID, activeVersionID string
	var importedState string
	if err := pool.QueryRow(context.Background(), `SELECT s.bank_version_id::text,s.state,b.active_version_id::text FROM quizcraft_workshop_version_states s JOIN quizcraft_banks b ON b.id=s.bank_id WHERE s.bank_id=$1 ORDER BY s.created_at DESC LIMIT 1`, bankID).Scan(&importedVersionID, &importedState, &activeVersionID); err != nil || importedState != "draft" || activeVersionID != versionID || importedVersionID == versionID {
		t.Fatalf("import publication boundary = imported %s state=%s active=%s err=%v", importedVersionID, importedState, activeVersionID, err)
	}
	legacyQuestions := []map[string]any{{"source_question_id": "q3", "type": "judge", "chapter_id": "ch3", "chapter": "第三章", "content": "Go 是编译型语言", "answer": true}}
	legacyStatus, legacyBody := requestJSON(t, http.MethodPost, fmt.Sprintf("%s/api/v1/workshop/banks/%s/imports", server.URL, bankID), map[string]string{"Cookie": bankWriteAuth, "Idempotency-Key": "workshop-import-legacy-omitted-version"}, map[string]any{"source_sha256": strings.Repeat("b", 64), "questions": legacyQuestions})
	if legacyStatus != http.StatusCreated || !bytes.Contains(legacyBody, []byte(`"accepted":true`)) {
		t.Fatalf("legacy omitted-version import = %d %s", legacyStatus, legacyBody)
	}
	explicitZeroStatus, _ := requestJSON(t, http.MethodPost, fmt.Sprintf("%s/api/v1/workshop/banks/%s/imports", server.URL, bankID), map[string]string{"Cookie": bankWriteAuth, "Idempotency-Key": "workshop-import-explicit-zero"}, map[string]any{"expected_version": 0, "source_sha256": strings.Repeat("c", 64), "questions": []map[string]any{{"source_question_id": "q4", "type": "blank", "chapter_id": "ch4", "content": "stale", "answer": "no"}}})
	if explicitZeroStatus != http.StatusConflict {
		t.Fatalf("explicit stale zero version = %d", explicitZeroStatus)
	}

	listStatus, listBody := requestJSON(t, http.MethodGet, server.URL+"/api/v1/workshop/catalog", map[string]string{"Cookie": readAuth}, nil)
	if listStatus != http.StatusOK || !bytes.Contains(listBody, []byte(`"lifecycle_version":8`)) || !bytes.Contains(listBody, []byte(`"state":"validated"`)) || !bytes.Contains(listBody, []byte(`"state":"draft"`)) || !bytes.Contains(listBody, []byte(versionID)) {
		t.Fatalf("workshop list = %d %s", listStatus, listBody)
	}
	secondStatus, secondBody := requestJSON(t, http.MethodPost, server.URL+"/api/v1/workshop/banks", map[string]string{"Cookie": writeAuth, "Idempotency-Key": "workshop-create-bank-002"}, map[string]any{"bank_key": "scope-second", "name": "Second Bank"})
	if secondStatus != http.StatusCreated {
		t.Fatalf("create second bank = %d %s", secondStatus, secondBody)
	}
	secondBankID := operationResourceID(t, secondBody)
	resourceAuth := "quizcraft_session=" + workshopToken(t, actorID, []string{"quizcraft.workshop.read"}, []map[string]string{{"kind": "resource", "product_code": "quizcraft", "resource_type": "bank", "resource_id": bankID}, {"kind": "resource", "product_code": "quizcraft", "resource_type": "bank", "resource_id": secondBankID}})
	resourceStatus, resourceBody := requestJSON(t, http.MethodGet, server.URL+"/api/v1/workshop/catalog", map[string]string{"Cookie": resourceAuth}, nil)
	if resourceStatus != http.StatusOK || !bytes.Contains(resourceBody, []byte(bankID)) || !bytes.Contains(resourceBody, []byte(secondBankID)) {
		t.Fatalf("resource scope list = %d %s", resourceStatus, resourceBody)
	}
	var audits int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM quizcraft_workshop_audit_events WHERE bank_id=$1 AND actor_user_id=$2`, bankID, actorID).Scan(&audits); err != nil || audits != 8 {
		t.Fatalf("workshop audits = %d err=%v", audits, err)
	}
}

func TestFeedbackPersistsStableReferenceAndMetadataOnlyInboxFact(t *testing.T) {
	pool := practicePool(t)
	report := importPracticeBank(t, pool, "feedback-stable-"+uuid.NewString())
	handler, _ := quizcraft.NewPracticeHTTP(quizcraft.PracticeHTTPConfig{Database: pool, AuthHMACSecret: []byte(practiceAuthSecret), AllowTestWorkshopClaims: true})
	server := httptest.NewServer(handler)
	defer server.Close()
	userID := uuid.NewString()
	auth := "quizcraft_session=" + practiceToken(t, userID)
	payload := map[string]any{"bank_id": report.BankID, "question_id": report.Questions[0].QuestionID, "question_version_id": report.Questions[0].QuestionVersionID, "category": "wrong_answer", "detail": "正确答案与解析矛盾"}
	status, body := requestJSON(t, http.MethodPost, server.URL+"/api/v1/feedback", map[string]string{"Cookie": auth, "Idempotency-Key": "stable-feedback-create-001"}, payload)
	if status != http.StatusAccepted {
		t.Fatalf("feedback = %d %s", status, body)
	}
	feedbackID := operationResourceID(t, body)
	initialOperationID := operationID(t, body)
	statusRead, statusBody := requestJSON(t, http.MethodGet, server.URL+"/api/v1/feedback/"+feedbackID+"/status", map[string]string{"Cookie": auth}, nil)
	if statusRead != http.StatusOK || !bytes.Contains(statusBody, []byte(`"feedback_id":"`+feedbackID+`"`)) || !bytes.Contains(statusBody, []byte(`"status":"pending"`)) || !bytes.Contains(statusBody, []byte(`"question_version_id":"`+report.Questions[0].QuestionVersionID+`"`)) {
		t.Fatalf("feedback status = %d %s", statusRead, statusBody)
	}
	statusList, statusListBody := requestJSON(t, http.MethodGet, server.URL+"/api/v1/feedback", map[string]string{"Cookie": auth}, nil)
	if statusList != http.StatusOK || !bytes.Contains(statusListBody, []byte(`"feedback_id":"`+feedbackID+`"`)) || !bytes.Contains(statusListBody, []byte(`"status":"pending"`)) {
		t.Fatalf("owned feedback statuses = %d %s", statusList, statusListBody)
	}
	foreignStatus, _ := requestJSON(t, http.MethodGet, server.URL+"/api/v1/feedback/"+feedbackID+"/status", map[string]string{"Cookie": "quizcraft_session=" + practiceToken(t, uuid.NewString())}, nil)
	if foreignStatus != http.StatusNotFound {
		t.Fatalf("foreign actor read feedback status = %d", foreignStatus)
	}
	replayStatus, replayBody := requestJSON(t, http.MethodPost, server.URL+"/api/v1/feedback", map[string]string{"Cookie": auth, "Idempotency-Key": "stable-feedback-create-001"}, payload)
	if replayStatus != status || !bytes.Equal(replayBody, body) {
		t.Fatalf("feedback replay = %d %s", replayStatus, replayBody)
	}
	invalid := map[string]any{"bank_id": report.BankID, "question_id": uuid.NewString(), "question_version_id": report.Questions[0].QuestionVersionID, "category": "typo", "detail": "bad ref"}
	invalidStatus, _ := requestJSON(t, http.MethodPost, server.URL+"/api/v1/feedback", map[string]string{"Cookie": auth, "Idempotency-Key": "stable-feedback-invalid"}, invalid)
	if invalidStatus != http.StatusBadRequest {
		t.Fatalf("invalid stable reference = %d", invalidStatus)
	}
	operationStatus, operationBody := requestJSON(t, http.MethodGet, server.URL+"/api/v1/operations/create_feedback", map[string]string{"Cookie": auth, "Idempotency-Key": "stable-feedback-create-001"}, nil)
	if operationStatus != http.StatusOK || operationID(t, operationBody) != initialOperationID {
		t.Fatalf("feedback operation identity changed = %d %s", operationStatus, operationBody)
	}
	readAuth := "quizcraft_session=" + workshopToken(t, userID, []string{"quizcraft.workshop.read"}, []map[string]string{{"kind": "resource", "product_code": "quizcraft", "resource_type": "bank", "resource_id": report.BankID}})
	detailStatus, detailBody := requestJSON(t, http.MethodGet, server.URL+"/api/v1/workshop/feedback/"+feedbackID, map[string]string{"Cookie": readAuth}, nil)
	if detailStatus != http.StatusOK || !bytes.Contains(detailBody, []byte(`"detail":"正确答案与解析矛盾"`)) {
		t.Fatalf("feedback deep-link detail = %d %s", detailStatus, detailBody)
	}
	var detail, sourceProduct, resourceType, resourceID, category string
	if err := pool.QueryRow(context.Background(), `SELECT f.detail,o.source_product_code,o.source_resource_type,o.source_resource_id,o.category FROM quizcraft_feedbacks f JOIN quizcraft_feedback_inbox_outbox o ON o.feedback_id=f.id WHERE f.id=$1`, feedbackID).Scan(&detail, &sourceProduct, &resourceType, &resourceID, &category); err != nil {
		t.Fatal(err)
	}
	if detail != "正确答案与解析矛盾" || sourceProduct != "quizcraft" || resourceType != "feedback" || resourceID != feedbackID || category != "wrong_answer" {
		t.Fatalf("feedback/inbox split = %q %q %q %q %q", detail, sourceProduct, resourceType, resourceID, category)
	}
	var copiedContentColumns int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM information_schema.columns WHERE table_name='quizcraft_feedback_inbox_outbox' AND column_name IN ('detail','body','content','feedback_text','question_content')`).Scan(&copiedContentColumns); err != nil || copiedContentColumns != 0 {
		t.Fatalf("Inbox outbox copied product content columns = %d err=%v", copiedContentColumns, err)
	}
	if _, err := pool.Exec(context.Background(), `UPDATE quizcraft_feedbacks SET detail='mutated' WHERE id=$1`, feedbackID); err == nil {
		t.Fatal("feedback fact mutation succeeded")
	}
}

func TestQuizCraftConsoleSummaryIsSignedAndBounded(t *testing.T) {
	pool := practicePool(t)
	const clientID = "console-gateway-quizcraft"
	const keyID = "quizcraft-active-key"
	const secret = "quizcraft-summary-secret-at-least-32-bytes"
	handler, err := quizcraft.NewPracticeHTTP(quizcraft.PracticeHTTPConfig{Database: pool, AuthHMACSecret: []byte(practiceAuthSecret), SummaryClientID: clientID, SummaryKeys: map[string]string{keyID: secret}})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	unsigned, err := http.Get(server.URL + "/api/v1/console-summary")
	if err != nil {
		t.Fatal(err)
	}
	_ = unsigned.Body.Close()
	if unsigned.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unsigned summary = %d", unsigned.StatusCode)
	}
	request, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/console-summary", nil)
	request.Header.Set("X-Request-Id", "req_quizcraft_summary_test")
	timestamp := fmt.Sprint(time.Now().Unix())
	nonce := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{7}, 24))
	digest := sha256.Sum256(nil)
	canonical := strings.Join([]string{http.MethodGet, "/api/v1/console-summary", timestamp, nonce, hex.EncodeToString(digest[:])}, "\n")
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(canonical))
	request.SetBasicAuth(clientID, secret)
	request.Header.Set("X-Service-Id", clientID)
	request.Header.Set("X-Key-Id", keyID)
	request.Header.Set("X-Timestamp", timestamp)
	request.Header.Set("X-Nonce", nonce)
	request.Header.Set("X-Signature", base64.RawURLEncoding.EncodeToString(mac.Sum(nil)))
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var body bytes.Buffer
	_, _ = body.ReadFrom(response.Body)
	if response.StatusCode != http.StatusOK || !bytes.Contains(body.Bytes(), []byte(`"request_id":"req_quizcraft_summary_test"`)) || !bytes.Contains(body.Bytes(), []byte(`"id":"quizcraft"`)) || !bytes.Contains(body.Bytes(), []byte(`"练习会话"`)) || bytes.Contains(body.Bytes(), []byte(`"待人工校验"`)) || bytes.Contains(body.Bytes(), []byte(`"detail"`)) {
		t.Fatalf("signed summary = %d %s", response.StatusCode, body.Bytes())
	}
	replay, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/console-summary", nil)
	replay.Header = request.Header.Clone()
	replayed, err := http.DefaultClient.Do(replay)
	if err != nil {
		t.Fatal(err)
	}
	defer replayed.Body.Close()
	if replayed.StatusCode != http.StatusConflict {
		t.Fatalf("replayed summary nonce = %d", replayed.StatusCode)
	}
}

func workshopToken(t *testing.T, userID string, permissions []string, scopes []map[string]string) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"sub": userID, "iss": "quizcraft-session", "exp": time.Now().Add(time.Hour).Unix(), "aud": "quizcraft", "permissions": permissions, "scopes": scopes})
	signed, err := token.SignedString([]byte(practiceAuthSecret))
	if err != nil {
		t.Fatal(err)
	}
	return signed
}

func operationResourceID(t *testing.T, body []byte) string {
	t.Helper()
	var envelope struct {
		Data struct {
			ResourceID string `json:"resource_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil || envelope.Data.ResourceID == "" {
		t.Fatalf("operation resource id missing: %s err=%v", body, err)
	}
	return envelope.Data.ResourceID
}

func operationID(t *testing.T, body []byte) string {
	t.Helper()
	var envelope struct {
		Data struct {
			OperationID string `json:"operation_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil || envelope.Data.OperationID == "" {
		t.Fatalf("operation id missing: %s err=%v", body, err)
	}
	return envelope.Data.OperationID
}
