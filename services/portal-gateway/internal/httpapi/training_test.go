package httpapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"henukit.dev/portal-gateway/internal/config"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

const trainingSessionID = "22222222-2222-4222-8222-222222222222"

func trainingHandler(t *testing.T, coreURL string, enabled bool) *Handler {
	t.Helper()
	rdb := redis.NewClient(&redis.Options{Addr: miniredis.RunT(t).Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	h, err := New(config.Config{SessionKey: []byte("0123456789abcdef0123456789abcdef"), PracticeURL: coreURL, QuizCraftCoreURL: coreURL,
		PracticeCommandAuth:     config.ServiceAuth{ClientID: "portal-gateway", ClientSecret: practiceCommandSecret, KeyID: "portal-practice-command-key"},
		PracticeAuth:            config.ServiceAuth{ClientID: "portal-gateway", ClientSecret: practiceCommandSecret, KeyID: "training-read-key"},
		QuizCraftCoreAuth:       config.ServiceAuth{ClientID: "portal-gateway", ClientSecret: practiceCommandSecret, KeyID: "training-read-key"},
		PracticeCommandsEnabled: enabled, QuizCraftCatalogEnabled: enabled, QuizCraftV2ReadsEnabled: enabled}, rdb)
	if err != nil {
		t.Fatal(err)
	}
	return h
}
func TestTrainingRoutesStayDarkAndNeverReachLegacyProxy(t *testing.T) {
	called := false
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true; w.WriteHeader(200) }))
	defer core.Close()
	h := trainingHandler(t, core.URL, false)
	for _, route := range []struct {
		method, path string
		status       int
	}{
		{"GET", "/catalog", 404}, {"GET", "/sessions/" + trainingSessionID, 503}, {"GET", "/sessions/" + trainingSessionID + "/review", 503},
		{"GET", "/review-items", 503}, {"POST", "/sessions", 503}, {"POST", "/sessions/" + trainingSessionID + "/answers", 503},
		{"POST", "/sessions/" + trainingSessionID + "/hints", 503}, {"POST", "/sessions/" + trainingSessionID + "/reveals", 503},
	} {
		rec := httptest.NewRecorder()
		h.Router().ServeHTTP(rec, httptest.NewRequest(route.method, "https://portal.test/api/v1/practice/training"+route.path, strings.NewReader("{}")))
		if rec.Code != route.status || !strings.Contains(rec.Body.String(), `"code"`) || rec.Header().Get("Cache-Control") != "private, no-store" {
			t.Errorf("%s %s = %d %s", route.method, route.path, rec.Code, rec.Body.String())
		}
	}
	if called {
		t.Fatal("dark training route contacted Core or legacy")
	}
}
func TestTrainingCatalogIsSignedAnonymousAndRejectsUnexpectedPrivateContent(t *testing.T) {
	leak := false
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RequestURI() != "/api/v1/portal/practice/training/catalog" || r.Header.Get("X-Actor-User-Id") != "" || r.Header.Get("Cookie") != "" || r.Header.Get("X-Permission-Code") != "portal.practice.read" || r.Header.Get("X-Key-Id") != "training-read-key" {
			t.Errorf("unsafe catalog request: %s %v", r.URL, r.Header)
		}
		w.Header().Set("Content-Type", "application/json")
		if leak {
			_, _ = w.Write([]byte(`{"request_id":"req_training","data":{"units":[],"transcript":"hidden"}}`))
		} else {
			_, _ = w.Write([]byte(`{"request_id":"req_training","data":{"units":[]}}`))
		}
	}))
	defer core.Close()
	h := trainingHandler(t, core.URL, true)
	for _, want := range []int{200, 502} {
		req := authenticatedPracticeCommandRequest(t, h, "GET", "/api/v1/practice/training/catalog", "", "unused-idempotency-key")
		req.Header.Set("X-Actor-User-Id", "spoof")
		req.AddCookie(&http.Cookie{Name: "quizcraft_anonymous", Value: "private"})
		rec := httptest.NewRecorder()
		h.Router().ServeHTTP(rec, req)
		if rec.Code != want {
			t.Fatalf("catalog = %d %s, want %d", rec.Code, rec.Body.String(), want)
		}
		if strings.Contains(rec.Body.String(), "hidden") {
			t.Fatal("private content crossed boundary")
		}
		leak = true
	}
}
func TestTrainingCreateValidatesClosedBodyBeforeSigning(t *testing.T) {
	called := false
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))
	defer core.Close()
	h := trainingHandler(t, core.URL, true)
	for _, body := range []string{`{}`, `{"unit_id":"invalid","unit_version_id":"invalid"}`, `{"unit_id":"33333333-3333-4333-8333-333333333333","unit_version_id":"44444444-4444-4444-8444-444444444444","actor":"spoof"}`, `{"unit_id":"33333333-3333-4333-8333-333333333333","unit_id":"44444444-4444-4444-8444-444444444444","unit_version_id":"44444444-4444-4444-8444-444444444444"}`} {
		req := authenticatedPracticeCommandRequest(t, h, "POST", "/api/v1/practice/training/sessions", body, "training-create-invalid-01")
		rec := httptest.NewRecorder()
		h.Router().ServeHTTP(rec, req)
		var envelope struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &envelope)
		if rec.Code != 400 || envelope.Error.Code != "INVALID_REQUEST" {
			t.Errorf("invalid create = %d %s", rec.Code, rec.Body.String())
		}
	}
	if called {
		t.Fatal("invalid body reached Core")
	}
}

func trainingPending(t *testing.T) []byte {
	t.Helper()
	body, err := os.ReadFile("testdata/training-pending.json")
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func TestTrainingCreateBindsExactRawBodyActorAndSafeCookie(t *testing.T) {
	input := `{ "unit_id":"33333333-3333-4333-8333-333333333333", "unit_version_id":"44444444-4444-4444-8444-444444444444" }`
	pending := trainingPending(t)
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		if string(raw) != input || r.URL.RequestURI() != "/api/v1/portal/practice/training/sessions" || r.Header.Get("X-Key-Id") != "portal-practice-command-key" || r.Header.Get("Idempotency-Key") != "training-create-exact-0001" {
			t.Errorf("command mutated: %s %s %v", r.URL, raw, r.Header)
		}
		assertSignedPracticeCommand(t, r, raw, practiceCommandUserID)
		if r.Header.Get("Cookie") != "quizcraft_anonymous=original-guest" {
			t.Errorf("unsafe cookie: %s", r.Header.Get("Cookie"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Set-Cookie", "quizcraft_anonymous=claimed; Secure; HttpOnly; SameSite=Lax; Path=/")
		w.WriteHeader(201)
		_, _ = w.Write(pending)
	}))
	defer core.Close()
	h := trainingHandler(t, core.URL, true)
	req := authenticatedPracticeCommandRequest(t, h, "POST", "/api/v1/practice/training/sessions", input, "training-create-exact-0001")
	req.AddCookie(&http.Cookie{Name: "quizcraft_anonymous", Value: "original-guest"})
	req.AddCookie(&http.Cookie{Name: "other", Value: "hidden"})
	req.Header.Set("X-Actor-User-Id", "spoof")
	rec := httptest.NewRecorder()
	h.Router().ServeHTTP(rec, req)
	if rec.Code != 201 || !strings.Contains(rec.Header().Get("Set-Cookie"), "quizcraft_anonymous=claimed") {
		t.Fatalf("create = %d %s", rec.Code, rec.Body.String())
	}
}

func TestTrainingRecoveryUsesSignedGuestReadAndNeverReissuesCookie(t *testing.T) {
	pending := trainingPending(t)
	reissue := false
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.RequestURI() != "/api/v1/portal/practice/training/sessions/"+trainingSessionID || r.Header.Get("X-Key-Id") != "training-read-key" || r.Header.Get("X-Permission-Code") != "portal.practice.read" || r.Header.Get("X-Actor-User-Id") != "" || r.Header.Get("Cookie") != "quizcraft_anonymous=owner-cookie" {
			t.Errorf("unsafe read: %s %v", r.URL, r.Header)
		}
		assertTrainingSignature(t, r, nil, "")
		w.Header().Set("Content-Type", "application/json")
		if reissue {
			w.Header().Set("Set-Cookie", "quizcraft_anonymous=new; Secure; HttpOnly; SameSite=Lax; Path=/")
		}
		_, _ = w.Write(pending)
	}))
	defer core.Close()
	h := trainingHandler(t, core.URL, true)
	for _, want := range []int{200, 502} {
		req := httptest.NewRequest("GET", "https://portal.test/api/v1/practice/training/sessions/"+trainingSessionID, nil)
		req.AddCookie(&http.Cookie{Name: "quizcraft_anonymous", Value: "owner-cookie"})
		req.AddCookie(&http.Cookie{Name: "other", Value: "hidden"})
		req.Header.Set("X-Actor-User-Id", practiceCommandUserID)
		rec := httptest.NewRecorder()
		h.Router().ServeHTTP(rec, req)
		if rec.Code != want || rec.Header().Get("Set-Cookie") != "" {
			t.Errorf("read = %d %s", rec.Code, rec.Body.String())
		}
		reissue = true
	}
}

func TestTrainingPrivateReadRejectsInvalidPortalSessionAndMissingGuest(t *testing.T) {
	called := false
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))
	defer core.Close()
	h := trainingHandler(t, core.URL, true)
	for _, cookie := range []string{"", "__Host-henukit_portal_session=invalid; quizcraft_anonymous=valid-guest"} {
		req := httptest.NewRequest("GET", "https://portal.test/api/v1/practice/training/sessions/"+trainingSessionID, nil)
		req.TLS = practiceCommandTLSState()
		if cookie != "" {
			req.Header.Set("Cookie", cookie)
		}
		rec := httptest.NewRecorder()
		h.Router().ServeHTTP(rec, req)
		want := 404
		if cookie != "" {
			want = 401
		}
		if rec.Code != want {
			t.Errorf("private read = %d %s", rec.Code, rec.Body.String())
		}
	}
	if called {
		t.Fatal("unauthenticated read reached Core")
	}
}

func TestTrainingReviewQueryIsCanonicalAndRejectsOtherSelection(t *testing.T) {
	calls := 0
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.RequestURI() != "/api/v1/portal/practice/training/review-items?cursor=YWJj&page_size=2" {
			t.Errorf("URI = %s", r.URL.RequestURI())
		}
		assertTrainingSignature(t, r, nil, practiceCommandUserID)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"request_id":"req_index","data":{"items":[],"next_cursor":""}}`))
	}))
	defer core.Close()
	h := trainingHandler(t, core.URL, true)
	for i, path := range []string{"/review-items?page_size=2&cursor=YWJj", "/review-items?page_size=0", "/review-items?page_size=51", "/review-items?page_size=1&page_size=2", "/review-items?actor=other", "/review-items?cursor=%2Fbad", "/sessions/" + trainingSessionID + "?complete=true", "/catalog?kind=listening"} {
		req := authenticatedPracticeCommandRequest(t, h, "GET", "/api/v1/practice/training"+path, "", "unused-idempotency-key")
		rec := httptest.NewRecorder()
		h.Router().ServeHTTP(rec, req)
		want := 400
		if i == 0 {
			want = 200
		}
		if rec.Code != want {
			t.Errorf("%s = %d %s", path, rec.Code, rec.Body.String())
		}
	}
	if calls != 1 {
		t.Fatalf("Core calls = %d", calls)
	}
}

func assertTrainingSignature(t *testing.T, r *http.Request, body []byte, actor string) {
	t.Helper()
	digest := sha256.Sum256(body)
	parts := []string{r.Method, r.URL.RequestURI(), r.Header.Get("X-Timestamp"), r.Header.Get("X-Nonce"), hex.EncodeToString(digest[:])}
	if actor != "" {
		parts = append(parts, actor)
	}
	mac := hmac.New(sha256.New, []byte(practiceCommandSecret))
	_, _ = mac.Write([]byte(strings.Join(parts, "\n")))
	if r.Header.Get("X-Signature") != base64.RawURLEncoding.EncodeToString(mac.Sum(nil)) {
		t.Fatal("training read signature does not bind actor and canonical URI")
	}
}

func TestTrainingBoundaryRejectsUnsupportedPathMediaTypeAndReadBody(t *testing.T) {
	called := false
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))
	defer core.Close()
	h := trainingHandler(t, core.URL, true)
	for _, test := range []struct {
		method, path, body, contentType string
		want                            int
	}{
		{"GET", "", "", "", 404},
		{"GET", "/sessions/" + trainingSessionID + "/private", "", "", 404},
		{"POST", "/sessions", `{"unit_id":"33333333-3333-4333-8333-333333333333","unit_version_id":"44444444-4444-4444-8444-444444444444"}`, "text/plain", 400},
		{"GET", "/catalog", "{}", "application/json", 400},
	} {
		req := authenticatedPracticeCommandRequest(t, h, test.method, "/api/v1/practice/training"+test.path, test.body, "training-invalid-boundary-01")
		req.Header.Set("Content-Type", test.contentType)
		rec := httptest.NewRecorder()
		h.Router().ServeHTTP(rec, req)
		if rec.Code != test.want {
			t.Errorf("%s %s = %d %s", test.method, test.path, rec.Code, rec.Body.String())
		}
	}
	if called {
		t.Fatal("invalid boundary request reached owner")
	}
}

func TestTrainingAnswerRequiresReceiptToMatchExactSubmission(t *testing.T) {
	answer := 1
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		assertSignedPracticeCommand(t, r, raw, practiceCommandUserID)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"request_id":"req_answer","data":{"session_id":"22222222-2222-4222-8222-222222222222","unit_version_id":"44444444-4444-4444-8444-444444444444","question_id":"77777777-7777-4777-8777-777777777777","question_version_id":"88888888-8888-4888-8888-888888888888","answer":%d,"correct":false,"assisted":false,"outcome":"review_required","review_required":true,"submitted_at":"2026-11-12T08:00:00Z","replayed":false,"saved_count":1,"total_count":2,"phase":"partial"}}`, answer)
	}))
	defer core.Close()
	h := trainingHandler(t, core.URL, true)
	input := `{"unit_version_id":"44444444-4444-4444-8444-444444444444","question_id":"77777777-7777-4777-8777-777777777777","question_version_id":"88888888-8888-4888-8888-888888888888","answer":1}`
	for _, want := range []int{200, 502} {
		req := authenticatedPracticeCommandRequest(t, h, "POST", "/api/v1/practice/training/sessions/"+trainingSessionID+"/answers", input, "training-answer-exact-0001")
		rec := httptest.NewRecorder()
		h.Router().ServeHTTP(rec, req)
		if rec.Code != want {
			t.Errorf("answer = %d %s, want %d", rec.Code, rec.Body.String(), want)
		}
		answer = 0
	}
}

func TestTrainingRecoveryRejectsIncompleteOrInconsistentOwnerProjection(t *testing.T) {
	pending := trainingPending(t)
	var content map[string]any
	if err := json.Unmarshal(pending, &content); err != nil {
		t.Fatal(err)
	}
	response := pending
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(response)
	}))
	defer core.Close()
	h := trainingHandler(t, core.URL, true)
	mutations := []func(map[string]any){
		func(s map[string]any) { s["saved_count"] = 1 },
		func(s map[string]any) { s["receipts"] = nil },
		func(s map[string]any) { s["phase"] = "completed" },
		func(s map[string]any) { s["revealed_layers"] = []string{"english"} },
		func(s map[string]any) { s["unit"].(map[string]any)["transcript"] = "private shared answer" },
		func(s map[string]any) {
			s["unit"].(map[string]any)["media"].(map[string]any)["sha256"] = strings.Repeat("f", 64)
		},
	}
	for _, mutate := range mutations {
		var copy map[string]any
		_ = json.Unmarshal(pending, &copy)
		mutate(copy["data"].(map[string]any))
		response, _ = json.Marshal(copy)
		req := authenticatedPracticeCommandRequest(t, h, "GET", "/api/v1/practice/training/sessions/"+trainingSessionID, "", "unused-idempotency-key")
		rec := httptest.NewRecorder()
		h.Router().ServeHTTP(rec, req)
		if rec.Code != 502 || strings.Contains(rec.Body.String(), "private shared answer") {
			t.Errorf("invalid recovery = %d %s", rec.Code, rec.Body.String())
		}
	}
}

func TestTrainingReviewRejectsForeignSessionAndMalformedEvidence(t *testing.T) {
	review := `{"request_id":"req_review","data":{"kind":"listening","session_id":"22222222-2222-4222-8222-222222222222","unit_version_id":"44444444-4444-4444-8444-444444444444","questions":[{"question_id":"77777777-7777-4777-8777-777777777777","question_version_id":"88888888-8888-4888-8888-888888888888","expected_answer":0,"analysis":"The sun made the traveller warm.","error_category":"detail","evidence":{"kind":"listening","segment_ids":["sentence-a"]}}],"segments":[{"segment_id":"sentence-a","start_ms":0,"end_ms":1000}],"layers":[]}}`
	var base map[string]any
	_ = json.Unmarshal([]byte(review), &base)
	q := base["data"].(map[string]any)["questions"].([]any)[0].(map[string]any)
	q2 := map[string]any{}
	for name, value := range q {
		q2[name] = value
	}
	q2["question_id"] = "99999999-9999-4999-8999-999999999999"
	q2["question_version_id"] = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	base["data"].(map[string]any)["questions"] = []any{q, q2}
	baseRaw, _ := json.Marshal(base)
	review = string(baseRaw)
	response := []byte(review)
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/review") {
			_, _ = w.Write(response)
		} else {
			_, _ = w.Write(trainingCompleted(t))
		}
	}))
	defer core.Close()
	h := trainingHandler(t, core.URL, true)
	for _, change := range []func(map[string]any){nil,
		func(r map[string]any) { r["session_id"] = "11111111-1111-4111-8111-111111111111" },
		func(r map[string]any) {
			r["questions"].([]any)[0].(map[string]any)["evidence"].(map[string]any)["segment_ids"] = []string{"foreign-segment"}
		},
		func(r map[string]any) { r["segments"].([]any)[0].(map[string]any)["end_ms"] = 0 },
	} {
		var envelope map[string]any
		_ = json.Unmarshal([]byte(review), &envelope)
		if change != nil {
			change(envelope["data"].(map[string]any))
		}
		response, _ = json.Marshal(envelope)
		req := authenticatedPracticeCommandRequest(t, h, "GET", "/api/v1/practice/training/sessions/"+trainingSessionID+"/review", "", "unused-idempotency-key")
		rec := httptest.NewRecorder()
		h.Router().ServeHTTP(rec, req)
		want := 502
		if change == nil {
			want = 200
		}
		if rec.Code != want {
			t.Errorf("review = %d %s, want %d", rec.Code, rec.Body.String(), want)
		}
	}
}

func TestTrainingOwnerConflictIsPreservedAndServiceAuthFailureIsClosed(t *testing.T) {
	status := 409
	body := `{"request_id":"req_conflict","error":{"code":"FIRST_ANSWER_IMMUTABLE","message":"首次答案已保存，请刷新后继续"}}`
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	defer core.Close()
	h := trainingHandler(t, core.URL, true)
	for _, want := range []int{409, 502} {
		req := authenticatedPracticeCommandRequest(t, h, "GET", "/api/v1/practice/training/sessions/"+trainingSessionID, "", "unused-idempotency-key")
		rec := httptest.NewRecorder()
		h.Router().ServeHTTP(rec, req)
		if rec.Code != want || want == 409 && !strings.Contains(rec.Body.String(), "FIRST_ANSWER_IMMUTABLE") || strings.Contains(rec.Body.String(), "internal-secret") {
			t.Errorf("owner failure = %d %s", rec.Code, rec.Body.String())
		}
		status = 401
		body = `{"error":"internal-secret-service-auth-details"}`
	}
}

func TestTrainingCompletedRecoveryRejectsChineseWithoutEnglish(t *testing.T) {
	var envelope map[string]any
	_ = json.Unmarshal(trainingPending(t), &envelope)
	data := envelope["data"].(map[string]any)
	data["phase"] = "completed"
	data["saved_count"] = 2
	data["revealed_layers"] = []string{"chinese"}
	questions := data["unit"].(map[string]any)["questions"].([]any)
	receipts := []any{}
	for _, value := range questions {
		q := value.(map[string]any)
		receipts = append(receipts, map[string]any{"session_id": trainingSessionID, "unit_version_id": "44444444-4444-4444-8444-444444444444", "question_id": q["question_id"], "question_version_id": q["question_version_id"], "answer": 0, "correct": true, "assisted": false, "outcome": "independent_correct", "review_required": false, "submitted_at": "2026-11-12T08:00:00Z", "replayed": true, "saved_count": 2, "total_count": 2, "phase": "completed"})
	}
	data["receipts"] = receipts
	body, _ := json.Marshal(envelope)
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer core.Close()
	h := trainingHandler(t, core.URL, true)
	req := authenticatedPracticeCommandRequest(t, h, "GET", "/api/v1/practice/training/sessions/"+trainingSessionID, "", "unused-idempotency-key")
	rec := httptest.NewRecorder()
	h.Router().ServeHTTP(rec, req)
	if rec.Code != 502 {
		t.Fatalf("nonsequential revealed layer = %d %s", rec.Code, rec.Body.String())
	}
}

func TestTrainingOwnerTransportAndAuthenticationFailuresAre502(t *testing.T) {
	for _, status := range []int{0, 401} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(401) }))
			defer core.Close()
			url := core.URL
			if status == 0 {
				url = "http://127.0.0.1:1"
			}
			handler := trainingHandler(t, url, true)
			rec := httptest.NewRecorder()
			handler.Router().ServeHTTP(rec, httptest.NewRequest("GET", "https://portal.test/api/v1/practice/training/catalog", nil))
			if rec.Code != 502 || !strings.Contains(rec.Body.String(), `"code":"DEPENDENCY_UNAVAILABLE"`) {
				t.Fatalf("owner failure %d = %d %s", status, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestTrainingReviewMustMatchPinnedSessionMembership(t *testing.T) {
	var snapshot map[string]any
	_ = json.Unmarshal(trainingPending(t), &snapshot)
	data := snapshot["data"].(map[string]any)
	data["phase"] = "completed"
	data["saved_count"] = 2
	var receipts []any
	for _, value := range data["unit"].(map[string]any)["questions"].([]any) {
		q := value.(map[string]any)
		receipts = append(receipts, map[string]any{"session_id": trainingSessionID, "unit_version_id": "44444444-4444-4444-8444-444444444444", "question_id": q["question_id"], "question_version_id": q["question_version_id"], "answer": 0, "correct": true, "assisted": false, "outcome": "independent_correct", "review_required": false, "submitted_at": "2026-11-12T08:00:00Z", "replayed": true, "saved_count": 2, "total_count": 2, "phase": "completed"})
	}
	data["receipts"] = receipts
	snapshotBody, _ := json.Marshal(snapshot)
	review := `{"request_id":"req_review","data":{"kind":"listening","session_id":"22222222-2222-4222-8222-222222222222","unit_version_id":"11111111-1111-4111-8111-111111111111","questions":[{"question_id":"77777777-7777-4777-8777-777777777777","question_version_id":"88888888-8888-4888-8888-888888888888","expected_answer":0,"analysis":"private review from another version","error_category":"detail","evidence":{"kind":"listening","segment_ids":["sentence-a"]}}],"segments":[{"segment_id":"sentence-a","start_ms":0,"end_ms":1000}],"layers":[]}}`
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/review") {
			_, _ = w.Write([]byte(review))
		} else {
			_, _ = w.Write(snapshotBody)
		}
	}))
	defer core.Close()
	h := trainingHandler(t, core.URL, true)
	rec := httptest.NewRecorder()
	h.Router().ServeHTTP(rec, authenticatedPracticeCommandRequest(t, h, "GET", "/api/v1/practice/training/sessions/"+trainingSessionID+"/review", "", "unused-idempotency-key"))
	if rec.Code != 502 || strings.Contains(rec.Body.String(), "private review from another version") {
		t.Fatalf("foreign review leaked %d %s", rec.Code, rec.Body.String())
	}
}

func trainingCompleted(t *testing.T) []byte {
	t.Helper()
	var envelope map[string]any
	_ = json.Unmarshal(trainingPending(t), &envelope)
	data := envelope["data"].(map[string]any)
	data["phase"] = "completed"
	data["saved_count"] = 2
	receipts := []any{}
	for _, value := range data["unit"].(map[string]any)["questions"].([]any) {
		q := value.(map[string]any)
		receipts = append(receipts, map[string]any{"session_id": trainingSessionID, "unit_version_id": "44444444-4444-4444-8444-444444444444", "question_id": q["question_id"], "question_version_id": q["question_version_id"], "answer": 0, "correct": true, "assisted": false, "outcome": "independent_correct", "review_required": false, "submitted_at": "2026-11-12T08:00:00Z", "replayed": true, "saved_count": 2, "total_count": 2, "phase": "completed"})
	}
	data["receipts"] = receipts
	raw, _ := json.Marshal(envelope)
	return raw
}

func TestTrainingRevealCommitsThenValidatesPinnedOwnerReview(t *testing.T) {
	disclosure := trainingDisclosure("english")
	calls := []string{}
	nonces := map[string]bool{}
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		assertTrainingSignature(t, r, raw, practiceCommandUserID)
		if nonces[r.Header.Get("X-Nonce")] {
			t.Error("service retry reused a nonce")
		}
		nonces[r.Header.Get("X-Nonce")] = true
		calls = append(calls, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "POST" {
			if r.URL.Path != "/api/v1/portal/practice/training/sessions/"+trainingSessionID+"/reveals" || string(raw) != `{"unit_version_id":"44444444-4444-4444-8444-444444444444","layer":"english"}` || r.Header.Get("Idempotency-Key") != "training-reveal-exact-0001" || r.Header.Get("X-Key-Id") != "portal-practice-command-key" || r.Header.Get("Cookie") != "quizcraft_anonymous=original-guest" {
				t.Errorf("unsafe reveal: %s %s %v", r.URL, raw, r.Header)
			}
			w.Header().Set("Set-Cookie", "quizcraft_anonymous=claimed; Secure; HttpOnly; SameSite=Lax; Path=/")
			_, _ = fmt.Fprintf(w, `{"request_id":"req_reveal","data":%s}`, disclosure)
			return
		}
		if r.Header.Get("X-Key-Id") != "training-read-key" || r.Header.Get("X-Permission-Code") != "portal.practice.read" || r.Header.Get("Cookie") != "quizcraft_anonymous=claimed" {
			t.Errorf("unsafe verification read: %v", r.Header)
		}
		if strings.HasSuffix(r.URL.Path, "/review") {
			_, _ = w.Write(trainingReviewWithLayers(t, disclosure))
		} else {
			_, _ = w.Write(trainingSnapshotWithLayers(t, "english"))
		}
	}))
	defer core.Close()
	h := trainingHandler(t, core.URL, true)
	input := `{"unit_version_id":"44444444-4444-4444-8444-444444444444","layer":"english"}`
	req := authenticatedPracticeCommandRequest(t, h, "POST", "/api/v1/practice/training/sessions/"+trainingSessionID+"/reveals", input, "training-reveal-exact-0001")
	req.Header.Set("X-Actor-User-Id", "spoofed")
	req.AddCookie(&http.Cookie{Name: "quizcraft_anonymous", Value: "original-guest"})
	req.AddCookie(&http.Cookie{Name: "other", Value: "hidden"})
	rec := httptest.NewRecorder()
	h.Router().ServeHTTP(rec, req)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "The sun made him warm.") || rec.Header().Get("Cache-Control") != "private, no-store" || !strings.Contains(rec.Header().Get("Set-Cookie"), "quizcraft_anonymous=claimed") {
		t.Fatalf("reveal = %d %s", rec.Code, rec.Body.String())
	}
	if len(calls) != 3 || calls[0] != "POST /api/v1/portal/practice/training/sessions/"+trainingSessionID+"/reveals" || !strings.HasSuffix(calls[2], "/review") {
		t.Fatalf("reveal validation requests = %v", calls)
	}
}

func trainingDisclosure(layer string) string {
	id, text, at := "dddddddd-dddd-4ddd-8ddd-dddddddddddd", "The sun made him warm.", "2026-11-12T08:01:00Z"
	if layer == "chinese" {
		id, text, at = "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee", "太阳使他暖和起来。", "2026-11-12T08:02:00Z"
	}
	return fmt.Sprintf(`{"disclosure_id":%q,"disclosed_at":%q,"content":{"kind":"listening","layer":%q,"segments":[{"segment_id":"sentence-a","text":%q}]}}`, id, at, layer, text)
}

func trainingSnapshotWithLayers(t *testing.T, layers ...string) []byte {
	t.Helper()
	var envelope map[string]any
	if err := json.Unmarshal(trainingCompleted(t), &envelope); err != nil {
		t.Fatal(err)
	}
	envelope["data"].(map[string]any)["revealed_layers"] = layers
	raw, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func trainingReviewWithLayers(t *testing.T, layers ...string) []byte {
	t.Helper()
	return []byte(fmt.Sprintf(`{"request_id":"req_review","data":{"kind":"listening","session_id":"22222222-2222-4222-8222-222222222222","unit_version_id":"44444444-4444-4444-8444-444444444444","questions":[{"question_id":"77777777-7777-4777-8777-777777777777","question_version_id":"88888888-8888-4888-8888-888888888888","expected_answer":0,"analysis":"The sun made the traveller warm.","error_category":"detail","evidence":{"kind":"listening","segment_ids":["sentence-a"]}},{"question_id":"99999999-9999-4999-8999-999999999999","question_version_id":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa","expected_answer":0,"analysis":"The sun won.","error_category":"detail","evidence":{"kind":"listening","segment_ids":["sentence-a"]}}],"segments":[{"segment_id":"sentence-a","start_ms":0,"end_ms":1000}],"layers":[%s]}}`, strings.Join(layers, ",")))
}

func TestTrainingRevealRejectsUnverifiableOwnerDisclosure(t *testing.T) {
	for _, name := range []string{"different-text", "different-id", "different-time", "different-language", "unknown-sentence", "incomplete-layer", "uncommitted-layer", "foreign-version", "foreign-session", "incomplete-session", "read-failed"} {
		t.Run(name, func(t *testing.T) {
			disclosure := trainingDisclosure("english")
			returned := disclosure
			snapshot := trainingSnapshotWithLayers(t, "english")
			review := trainingReviewWithLayers(t, disclosure)
			switch name {
			case "different-text":
				returned = strings.ReplaceAll(disclosure, "The sun made him warm.", "private text from another unit")
			case "different-id":
				returned = strings.ReplaceAll(disclosure, "dddddddd-dddd-4ddd-8ddd-dddddddddddd", "ffffffff-ffff-4fff-8fff-ffffffffffff")
			case "different-time":
				returned = strings.ReplaceAll(disclosure, "08:01:00Z", "08:01:01Z")
			case "different-language":
				returned = trainingDisclosure("chinese")
			case "unknown-sentence":
				returned = strings.ReplaceAll(disclosure, "sentence-a", "foreign-sentence")
			case "incomplete-layer":
				review = []byte(strings.ReplaceAll(string(review), `"segment_id":"sentence-a","start_ms":0,"end_ms":1000`, `"segment_id":"sentence-a","start_ms":0,"end_ms":1000},{"segment_id":"sentence-b","start_ms":1000,"end_ms":2000`))
			case "uncommitted-layer":
				snapshot = trainingCompleted(t)
				review = trainingReviewWithLayers(t)
			case "foreign-version":
				review = []byte(strings.ReplaceAll(string(review), "44444444-4444-4444-8444-444444444444", "11111111-1111-4111-8111-111111111111"))
			case "foreign-session":
				review = []byte(strings.ReplaceAll(string(review), trainingSessionID, "11111111-1111-4111-8111-111111111111"))
			case "incomplete-session":
				snapshot = trainingPending(t)
			}
			core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == "POST" {
					_, _ = fmt.Fprintf(w, `{"request_id":"req_reveal","data":%s}`, returned)
				} else if name == "read-failed" {
					w.WriteHeader(503)
					_, _ = w.Write([]byte(`{"request_id":"req_failed","error":{"code":"DEPENDENCY_UNAVAILABLE","message":"训练暂时不可用，请稍后重试"}}`))
				} else if strings.HasSuffix(r.URL.Path, "/review") {
					_, _ = w.Write(review)
				} else {
					_, _ = w.Write(snapshot)
				}
			}))
			defer core.Close()
			h := trainingHandler(t, core.URL, true)
			req := authenticatedPracticeCommandRequest(t, h, "POST", "/api/v1/practice/training/sessions/"+trainingSessionID+"/reveals", `{"unit_version_id":"44444444-4444-4444-8444-444444444444","layer":"english"}`, "training-reveal-unverified-01")
			rec := httptest.NewRecorder()
			h.Router().ServeHTTP(rec, req)
			if rec.Code != 502 || !strings.Contains(rec.Body.String(), `"code":"DEPENDENCY_UNAVAILABLE"`) || strings.Contains(rec.Body.String(), "The sun made him warm.") || strings.Contains(rec.Body.String(), "private text") {
				t.Fatalf("unverified reveal = %d %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestTrainingRevealRequiresClosedRequestActorAndBothExistingGates(t *testing.T) {
	calls := 0
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++ }))
	defer core.Close()
	for _, test := range []struct {
		name, input string
		want        int
	}{
		{"unknown-field", `{"unit_version_id":"44444444-4444-4444-8444-444444444444","layer":"english","submitted":true}`, 400},
		{"duplicate-field", `{"unit_version_id":"44444444-4444-4444-8444-444444444444","layer":"english","layer":"chinese"}`, 400},
		{"invalid-layer", `{"unit_version_id":"44444444-4444-4444-8444-444444444444","layer":"all"}`, 400},
		{"invalid-login", `{"unit_version_id":"44444444-4444-4444-8444-444444444444","layer":"english"}`, 401},
		{"private-reads-off", `{"unit_version_id":"44444444-4444-4444-8444-444444444444","layer":"english"}`, 503},
		{"commands-off", `{"unit_version_id":"44444444-4444-4444-8444-444444444444","layer":"english"}`, 503},
	} {
		t.Run(test.name, func(t *testing.T) {
			h := trainingHandler(t, core.URL, true)
			if test.name == "private-reads-off" {
				h.quizCraft = nil
			}
			if test.name == "commands-off" {
				h.practiceCommands = nil
			}
			req := authenticatedPracticeCommandRequest(t, h, "POST", "/api/v1/practice/training/sessions/"+trainingSessionID+"/reveals", test.input, "training-reveal-invalid-01")
			if test.name == "invalid-login" {
				req.Header.Set("Cookie", "__Host-henukit_portal_session=invalid; quizcraft_anonymous=guest")
			}
			rec := httptest.NewRecorder()
			h.Router().ServeHTTP(rec, req)
			if rec.Code != test.want || rec.Header().Get("Cache-Control") != "private, no-store" {
				t.Fatalf("reveal = %d %s", rec.Code, rec.Body.String())
			}
		})
	}
	if calls != 0 {
		t.Fatalf("invalid/disabled reveal reached Core %d times", calls)
	}
}

func TestTrainingGuestRevealAndReplayRecoverBothCommittedLanguages(t *testing.T) {
	english, chinese := trainingDisclosure("english"), trainingDisclosure("chinese")
	nonces := map[string]bool{}
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		assertTrainingSignature(t, r, raw, "")
		if nonces[r.Header.Get("X-Nonce")] {
			t.Error("replay reused service nonce")
		}
		nonces[r.Header.Get("X-Nonce")] = true
		if r.Header.Get("X-Actor-User-Id") != "" || r.Header.Get("Cookie") != "quizcraft_anonymous=owner-cookie" {
			t.Errorf("guest identity changed: %v", r.Header)
		}
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "POST" {
			if string(raw) != `{"unit_version_id":"44444444-4444-4444-8444-444444444444","layer":"chinese"}` || r.Header.Get("Idempotency-Key") != "training-reveal-guest-replay-01" {
				t.Errorf("retry changed command: %s %v", raw, r.Header)
			}
			_, _ = fmt.Fprintf(w, `{"request_id":"req_reveal","data":%s}`, chinese)
		} else if strings.HasSuffix(r.URL.Path, "/review") {
			_, _ = w.Write(trainingReviewWithLayers(t, english, chinese))
		} else {
			_, _ = w.Write(trainingSnapshotWithLayers(t, "english", "chinese"))
		}
	}))
	defer core.Close()
	h := trainingHandler(t, core.URL, true)
	for range 2 {
		req := httptest.NewRequest("POST", "https://portal.test/api/v1/practice/training/sessions/"+trainingSessionID+"/reveals", strings.NewReader(`{"unit_version_id":"44444444-4444-4444-8444-444444444444","layer":"chinese"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", "training-reveal-guest-replay-01")
		req.AddCookie(&http.Cookie{Name: "quizcraft_anonymous", Value: "owner-cookie"})
		req.AddCookie(&http.Cookie{Name: "other", Value: "hidden"})
		req.Header.Set("X-Actor-User-Id", practiceCommandUserID)
		rec := httptest.NewRecorder()
		h.Router().ServeHTTP(rec, req)
		if rec.Code != 200 || !strings.Contains(rec.Body.String(), "太阳使他暖和起来。") || strings.Contains(rec.Body.String(), "The sun made him warm.") || rec.Header().Get("Set-Cookie") != "" {
			t.Fatalf("guest reveal replay = %d %s", rec.Code, rec.Body.String())
		}
	}
}
