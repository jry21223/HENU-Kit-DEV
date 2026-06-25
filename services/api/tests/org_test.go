package tests

import (
	"net/http"
	"strings"
	"testing"

	"final-review-platform/services/api/internal/server"
	applogger "final-review-platform/services/api/pkg/logger"
)

func TestPublicOrganizationResponsesUseNarrowDTOs(t *testing.T) {
	db := newTestDB(t)
	router := server.NewRouter(testConfig(), applogger.New("test"), db, nil)
	course := createTestCourse(t, db)

	endpoints := []string{
		"/api/v1/schools",
		"/api/v1/colleges?schoolId=" + course.SchoolID,
		"/api/v1/majors?schoolId=" + course.SchoolID + "&collegeId=" + course.CollegeID,
		"/api/v1/courses?schoolId=" + course.SchoolID + "&majorId=" + course.MajorID + "&grade=" + course.Grade,
		"/api/v1/courses/" + course.ID,
	}
	for _, endpoint := range endpoints {
		response := performJSON(router, http.MethodGet, endpoint, "", "")
		if response.Code != http.StatusOK {
			t.Fatalf("expected %s to return 200, got %d: %s", endpoint, response.Code, response.Body.String())
		}
		body := response.Body.String()
		for _, hiddenField := range []string{"emailDomains", `"status"`, "createdAt", "updatedAt", "deletedAt"} {
			if strings.Contains(body, hiddenField) {
				t.Fatalf("public org endpoint %s leaked %s: %s", endpoint, hiddenField, body)
			}
		}
	}
}
