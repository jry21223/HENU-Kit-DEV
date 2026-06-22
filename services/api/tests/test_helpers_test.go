package tests

import (
	"net/http"
	"testing"

	"gorm.io/gorm"

	"final-review-platform/services/api/internal/platform/model"
)

func createTestCourse(t *testing.T, db *gorm.DB) model.Course {
	t.Helper()
	var school model.School
	if err := db.First(&school, "slug = ?", "henu").Error; err != nil {
		t.Fatal(err)
	}
	college := model.College{SchoolID: school.ID, Name: "Software College", Status: model.StatusPublished}
	if err := db.Create(&college).Error; err != nil {
		t.Fatal(err)
	}
	major := model.Major{SchoolID: school.ID, CollegeID: college.ID, Name: "Network Engineering", Slug: "network-engineering", Status: model.StatusPublished}
	if err := db.Create(&major).Error; err != nil {
		t.Fatal(err)
	}
	course := model.Course{
		SchoolID:    school.ID,
		CollegeID:   college.ID,
		MajorID:     major.ID,
		Grade:       "2023",
		Name:        "Discrete Math",
		Slug:        "discrete-math",
		Description: "Test course",
		Status:      model.StatusPublished,
	}
	if err := db.Create(&course).Error; err != nil {
		t.Fatal(err)
	}
	return course
}

func createTestMaterial(t *testing.T, db *gorm.DB, courseID string, title string, accessLevel string, storageKey string) model.Material {
	t.Helper()
	material := model.Material{
		CourseID:       courseID,
		Title:          title,
		Type:           "knowledge_note",
		Description:    "Test material",
		StorageKey:     storageKey,
		FileName:       title + ".txt",
		PreviewContent: "Preview",
		AccessLevel:    accessLevel,
		Status:         model.StatusPublished,
	}
	if err := db.Create(&material).Error; err != nil {
		t.Fatal(err)
	}
	return material
}

func loginTestUser(t *testing.T, router http.Handler, email string) string {
	t.Helper()
	sendResponse := performJSON(router, http.MethodPost, "/api/v1/auth/send-code", `{"email":"`+email+`"}`, "")
	if sendResponse.Code != http.StatusOK {
		t.Fatalf("expected send-code 200, got %d: %s", sendResponse.Code, sendResponse.Body.String())
	}
	loginBody := `{"email":"` + email + `","code":"123456","name":"Student","grade":"2023"}`
	loginResponse := performJSON(router, http.MethodPost, "/api/v1/auth/login", loginBody, "")
	if loginResponse.Code != http.StatusOK {
		t.Fatalf("expected login 200, got %d: %s", loginResponse.Code, loginResponse.Body.String())
	}
	return extractAccessToken(t, loginResponse.Body.Bytes())
}
