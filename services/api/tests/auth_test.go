package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"final-review-platform/services/api/internal/platform/model"
	"final-review-platform/services/api/internal/server"
	"final-review-platform/services/api/pkg/config"
	applogger "final-review-platform/services/api/pkg/logger"
)

func TestEmailCodeLoginAndPermissions(t *testing.T) {
	db := newTestDB(t)
	router := server.NewRouter(testConfig(), applogger.New("test"), db, nil)

	sendBody := `{"email":"student@stu.henu.edu.cn"}`
	sendResponse := performJSON(router, http.MethodPost, "/api/v1/auth/send-code", sendBody, "")
	if sendResponse.Code != http.StatusOK {
		t.Fatalf("expected send-code 200, got %d: %s", sendResponse.Code, sendResponse.Body.String())
	}

	loginBody := `{"email":"student@stu.henu.edu.cn","code":"123456","name":"测试用户","grade":"2023级"}`
	loginResponse := performJSON(router, http.MethodPost, "/api/v1/auth/login", loginBody, "")
	if loginResponse.Code != http.StatusOK {
		t.Fatalf("expected login 200, got %d: %s", loginResponse.Code, loginResponse.Body.String())
	}

	accessToken := extractAccessToken(t, loginResponse.Body.Bytes())
	meResponse := performJSON(router, http.MethodGet, "/api/v1/auth/me", "", accessToken)
	if meResponse.Code != http.StatusOK {
		t.Fatalf("expected me 200, got %d: %s", meResponse.Code, meResponse.Body.String())
	}

	adminResponse := performJSON(router, http.MethodGet, "/api/v1/admin/healthz", "", accessToken)
	if adminResponse.Code != http.StatusForbidden {
		t.Fatalf("expected admin 403, got %d: %s", adminResponse.Code, adminResponse.Body.String())
	}
}

func TestSendCodeRejectsUnknownDomain(t *testing.T) {
	db := newTestDB(t)
	router := server.NewRouter(testConfig(), applogger.New("test"), db, nil)

	response := performJSON(router, http.MethodPost, "/api/v1/auth/send-code", `{"email":"user@gmail.com"}`, "")
	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected rejected email 400, got %d: %s", response.Code, response.Body.String())
	}
}

func TestUpdateMeProfileBinding(t *testing.T) {
	db := newTestDB(t)
	router := server.NewRouter(testConfig(), applogger.New("test"), db, nil)
	course := createTestCourse(t, db)

	unauthorized := performJSON(router, http.MethodPatch, "/api/v1/auth/me", `{"grade":"2024"}`, "")
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthenticated profile update 401, got %d: %s", unauthorized.Code, unauthorized.Body.String())
	}

	token := loginTestUser(t, router, "profile@stu.henu.edu.cn")
	updateBody := `{"name":"Profile Student","schoolId":"` + course.SchoolID + `","majorId":"` + course.MajorID + `","grade":"2024级"}`
	updateResponse := performJSON(router, http.MethodPatch, "/api/v1/auth/me", updateBody, token)
	if updateResponse.Code != http.StatusOK {
		t.Fatalf("expected profile update 200, got %d: %s", updateResponse.Code, updateResponse.Body.String())
	}

	var payload struct {
		Data struct {
			Name     string  `json:"name"`
			SchoolID *string `json:"schoolId"`
			MajorID  *string `json:"majorId"`
			Grade    string  `json:"grade"`
		} `json:"data"`
	}
	if err := json.Unmarshal(updateResponse.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Data.Name != "Profile Student" || payload.Data.Grade != "2024级" {
		t.Fatalf("unexpected updated profile: %#v", payload.Data)
	}
	if payload.Data.SchoolID == nil || *payload.Data.SchoolID != course.SchoolID || payload.Data.MajorID == nil || *payload.Data.MajorID != course.MajorID {
		t.Fatalf("expected profile school/major binding, got %#v", payload.Data)
	}

	var otherSchool = model.School{Name: "Other School", Slug: "other-school", EmailDomains: "other.edu.cn", Status: model.StatusPublished}
	if err := db.Create(&otherSchool).Error; err != nil {
		t.Fatal(err)
	}
	mismatch := performJSON(router, http.MethodPatch, "/api/v1/auth/me", `{"schoolId":"`+otherSchool.ID+`","majorId":"`+course.MajorID+`"}`, token)
	if mismatch.Code != http.StatusBadRequest {
		t.Fatalf("expected mismatched school/major 400, got %d: %s", mismatch.Code, mismatch.Body.String())
	}

	clearSchoolUseMajor := performJSON(router, http.MethodPatch, "/api/v1/auth/me", `{"majorId":"`+course.MajorID+`","grade":"2023级"}`, token)
	if clearSchoolUseMajor.Code != http.StatusOK {
		t.Fatalf("expected major-only update 200, got %d: %s", clearSchoolUseMajor.Code, clearSchoolUseMajor.Body.String())
	}
	var inferred struct {
		Data struct {
			SchoolID *string `json:"schoolId"`
			MajorID  *string `json:"majorId"`
			Grade    string  `json:"grade"`
		} `json:"data"`
	}
	if err := json.Unmarshal(clearSchoolUseMajor.Body.Bytes(), &inferred); err != nil {
		t.Fatal(err)
	}
	if inferred.Data.SchoolID == nil || *inferred.Data.SchoolID != course.SchoolID || inferred.Data.MajorID == nil || *inferred.Data.MajorID != course.MajorID {
		t.Fatalf("expected major-only update to infer school, got %#v", inferred.Data)
	}
}

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(model.AllModels()...); err != nil {
		t.Fatal(err)
	}
	school := model.School{Name: "河南大学", Slug: "henu", EmailDomains: "stu.henu.edu.cn,henu.edu.cn", Status: model.StatusPublished}
	if err := db.Create(&school).Error; err != nil {
		t.Fatal(err)
	}
	return db
}

func testConfig() config.Config {
	return config.Config{
		Environment:        "test",
		Port:               "0",
		Version:            "test",
		CORSAllowedOrigins: []string{"http://localhost:3000"},
		RateLimitRPS:       1000,
		RateLimitBurst:     1000,
		DevFixedCode:       "123456",
		JWT: config.JWTConfig{
			Issuer:           "test",
			AccessTTLMinutes: 15,
			RefreshTTLHours:  168,
		},
	}
}

func performJSON(router http.Handler, method string, path string, body string, token string) *httptest.ResponseRecorder {
	requestBody := bytes.NewBufferString(body)
	request := httptest.NewRequest(method, path, requestBody)
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func extractAccessToken(t *testing.T, body []byte) string {
	t.Helper()
	var payload struct {
		Data struct {
			AccessToken string `json:"accessToken"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Data.AccessToken == "" {
		t.Fatal("missing access token")
	}
	return payload.Data.AccessToken
}
