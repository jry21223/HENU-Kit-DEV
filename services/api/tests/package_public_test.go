package tests

import (
	"net/http"
	"strings"
	"testing"

	"final-review-platform/services/api/internal/platform/model"
	"final-review-platform/services/api/internal/server"
	applogger "final-review-platform/services/api/pkg/logger"
)

func TestPublicPackageResponsesUseNarrowDTOs(t *testing.T) {
	db := newTestDB(t)
	router := server.NewRouter(testConfig(), applogger.New("test"), db, nil)
	course := createTestCourse(t, db)
	coursePackage := createTestPackage(t, db, course, "public-package-dto", model.StatusPublished)
	material := createTestMaterial(t, db, course.ID, "Public package material", model.MaterialAccessPaid, "materials/public-package.txt")
	if err := db.Create(&model.CoursePackageItem{PackageID: coursePackage.ID, ResourceType: "material", ResourceID: material.ID, SortOrder: 1}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&material).Updates(map[string]interface{}{
		"created_by":    coursePackage.ID,
		"reviewer_id":   coursePackage.ID,
		"review_reason": "internal package material note",
	}).Error; err != nil {
		t.Fatal(err)
	}

	endpoints := []string{
		"/api/v1/packages",
		"/api/v1/courses/" + course.ID + "/packages",
		"/api/v1/packages/" + coursePackage.ID,
	}
	for _, endpoint := range endpoints {
		response := performJSON(router, http.MethodGet, endpoint, "", "")
		if response.Code != http.StatusOK {
			t.Fatalf("expected %s to return 200, got %d: %s", endpoint, response.Code, response.Body.String())
		}
		body := response.Body.String()
		if !strings.Contains(body, coursePackage.ID) {
			t.Fatalf("expected %s to include package id, got %s", endpoint, body)
		}
		hiddenFields := []string{material.StorageKey, "createdBy", "reviewerId", "reviewReason", "internal package material note"}
		if endpoint != "/api/v1/packages/"+coursePackage.ID {
			hiddenFields = append(hiddenFields, "createdAt", "updatedAt", `"status"`)
		}
		for _, hiddenField := range hiddenFields {
			if strings.Contains(body, hiddenField) {
				t.Fatalf("public package endpoint %s leaked %s: %s", endpoint, hiddenField, body)
			}
		}
	}
}
