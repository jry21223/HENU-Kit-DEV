package tests

import (
	"net/http"
	"strings"
	"testing"

	"final-review-platform/services/api/internal/platform/model"
	"final-review-platform/services/api/internal/server"
	applogger "final-review-platform/services/api/pkg/logger"
)

func TestWikiSubmissionAndReviewWorkflow(t *testing.T) {
	db := newTestDB(t)
	router := server.NewRouter(testConfig(), applogger.New("test"), db, nil)
	course := createTestCourse(t, db)

	unauthorizedCreate := performJSON(router, http.MethodPost, "/api/v1/wiki/entries", `{"title":"No auth","slug":"no-auth","content":"body"}`, "")
	if unauthorizedCreate.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthenticated wiki create 401, got %d: %s", unauthorizedCreate.Code, unauthorizedCreate.Body.String())
	}

	studentToken := loginTestUser(t, router, "wiki-student@stu.henu.edu.cn")
	studentCreate := performJSON(router, http.MethodPost, "/api/v1/wiki/entries", `{"title":"Student entry","slug":"student-entry","content":"body"}`, studentToken)
	if studentCreate.Code != http.StatusForbidden {
		t.Fatalf("expected non-creator wiki create 403, got %d: %s", studentCreate.Code, studentCreate.Body.String())
	}

	createTestUser(t, db, "wiki-creator@stu.henu.edu.cn", model.RoleCreator)
	creatorToken := loginTestUser(t, router, "wiki-creator@stu.henu.edu.cn")
	invalidSlug := performJSON(router, http.MethodPost, "/api/v1/wiki/entries", `{"title":"Bad slug","slug":"Bad_Slug","content":"body"}`, creatorToken)
	if invalidSlug.Code != http.StatusBadRequest || !strings.Contains(invalidSlug.Body.String(), "invalid_slug") {
		t.Fatalf("expected invalid slug rejection, got %d: %s", invalidSlug.Code, invalidSlug.Body.String())
	}
	missingCourse := performJSON(router, http.MethodPost, "/api/v1/wiki/entries", `{"courseId":"00000000-0000-0000-0000-000000000000","title":"Missing course","slug":"missing-course","content":"body"}`, creatorToken)
	if missingCourse.Code != http.StatusBadRequest || !strings.Contains(missingCourse.Body.String(), "course_not_found") {
		t.Fatalf("expected missing course rejection, got %d: %s", missingCourse.Code, missingCourse.Body.String())
	}

	createBody := `{"courseId":"` + course.ID + `","title":"Propositional Logic Review","slug":"propositional-logic-review","content":"Truth tables and equivalence laws first.","summary":"first version"}`
	createResponse := performJSON(router, http.MethodPost, "/api/v1/wiki/entries", createBody, creatorToken)
	if createResponse.Code != http.StatusOK {
		t.Fatalf("expected wiki create 200, got %d: %s", createResponse.Code, createResponse.Body.String())
	}
	var entry model.WikiEntry
	if err := db.First(&entry, "slug = ?", "propositional-logic-review").Error; err != nil {
		t.Fatal(err)
	}
	if entry.Status != model.StatusPending || entry.AuthorID == "" || entry.CourseID == nil || *entry.CourseID != course.ID || entry.Version != 1 {
		t.Fatalf("expected pending wiki with author/course/version, got %#v", entry)
	}
	var historyCount int64
	if err := db.Model(&model.WikiEditHistory{}).Where("entry_id = ? AND editor_id = ? AND version = ?", entry.ID, entry.AuthorID, 1).Count(&historyCount).Error; err != nil {
		t.Fatal(err)
	}
	if historyCount != 1 {
		t.Fatalf("expected initial wiki edit history, got %d", historyCount)
	}

	publicList := performJSON(router, http.MethodGet, "/api/v1/wiki/entries", "", "")
	if publicList.Code != http.StatusOK || strings.Contains(publicList.Body.String(), entry.ID) {
		t.Fatalf("expected pending wiki hidden from public list, got %d: %s", publicList.Code, publicList.Body.String())
	}
	publicDetail := performJSON(router, http.MethodGet, "/api/v1/wiki/entries/"+entry.ID, "", "")
	if publicDetail.Code != http.StatusNotFound {
		t.Fatalf("expected pending wiki public detail 404, got %d: %s", publicDetail.Code, publicDetail.Body.String())
	}

	forbiddenReviewList := performJSON(router, http.MethodGet, "/api/v1/admin/wiki/entries", "", studentToken)
	if forbiddenReviewList.Code != http.StatusForbidden {
		t.Fatalf("expected student wiki review list 403, got %d: %s", forbiddenReviewList.Code, forbiddenReviewList.Body.String())
	}
	forbiddenApprove := performJSON(router, http.MethodPost, "/api/v1/admin/wiki/entries/"+entry.ID+"/approve", `{"reviewReason":"ok"}`, studentToken)
	if forbiddenApprove.Code != http.StatusForbidden {
		t.Fatalf("expected student wiki approve 403, got %d: %s", forbiddenApprove.Code, forbiddenApprove.Body.String())
	}

	reviewer := createTestUser(t, db, "wiki-reviewer@stu.henu.edu.cn", model.RoleReviewer)
	reviewerToken := loginTestUser(t, router, "wiki-reviewer@stu.henu.edu.cn")
	reviewList := performJSON(router, http.MethodGet, "/api/v1/admin/wiki/entries", "", reviewerToken)
	if reviewList.Code != http.StatusOK || !strings.Contains(reviewList.Body.String(), entry.ID) {
		t.Fatalf("expected reviewer wiki list to include pending entry, got %d: %s", reviewList.Code, reviewList.Body.String())
	}
	rejectWithoutReason := performJSON(router, http.MethodPost, "/api/v1/admin/wiki/entries/"+entry.ID+"/reject", "", reviewerToken)
	if rejectWithoutReason.Code != http.StatusBadRequest || !strings.Contains(rejectWithoutReason.Body.String(), "review_reason_required") {
		t.Fatalf("expected reject reason required, got %d: %s", rejectWithoutReason.Code, rejectWithoutReason.Body.String())
	}

	approve := performJSON(router, http.MethodPost, "/api/v1/admin/wiki/entries/"+entry.ID+"/approve", `{"reviewReason":"useful structure"}`, reviewerToken)
	if approve.Code != http.StatusOK {
		t.Fatalf("expected reviewer wiki approve 200, got %d: %s", approve.Code, approve.Body.String())
	}
	var approved model.WikiEntry
	if err := db.First(&approved, "id = ?", entry.ID).Error; err != nil {
		t.Fatal(err)
	}
	if approved.Status != model.StatusPublished || approved.ReviewerID == nil || *approved.ReviewerID != reviewer.ID || approved.ReviewedAt == nil || approved.ReviewReason != "useful structure" {
		t.Fatalf("expected approved wiki review metadata, got %#v", approved)
	}
	if countOperationLogs(t, db, "wiki_entry.published", "wiki_entry", entry.ID, reviewer.ID) != 1 {
		t.Fatal("expected wiki approval operation log")
	}

	publicListAfterApprove := performJSON(router, http.MethodGet, "/api/v1/wiki/entries?courseId="+course.ID, "", "")
	if publicListAfterApprove.Code != http.StatusOK || !strings.Contains(publicListAfterApprove.Body.String(), entry.ID) {
		t.Fatalf("expected approved wiki in public list, got %d: %s", publicListAfterApprove.Code, publicListAfterApprove.Body.String())
	}
	publicApprovedDetail := performJSON(router, http.MethodGet, "/api/v1/wiki/entries/"+entry.ID, "", "")
	if publicApprovedDetail.Code != http.StatusOK {
		t.Fatalf("expected approved wiki public detail 200, got %d: %s", publicApprovedDetail.Code, publicApprovedDetail.Body.String())
	}
	if strings.Contains(publicApprovedDetail.Body.String(), "reviewerId") || strings.Contains(publicApprovedDetail.Body.String(), "reviewReason") {
		t.Fatalf("expected public wiki detail to hide review metadata, got %s", publicApprovedDetail.Body.String())
	}
	reviewAgain := performJSON(router, http.MethodPost, "/api/v1/admin/wiki/entries/"+entry.ID+"/reject", `{"reviewReason":"overwrite"}`, reviewerToken)
	if reviewAgain.Code != http.StatusConflict || !strings.Contains(reviewAgain.Body.String(), "entry_not_reviewable") {
		t.Fatalf("expected reviewed wiki conflict, got %d: %s", reviewAgain.Code, reviewAgain.Body.String())
	}
	if countOperationLogs(t, db, "wiki_entry.published", "wiki_entry", entry.ID, reviewer.ID) != 1 {
		t.Fatal("expected repeated wiki review to avoid duplicate approval logs")
	}

	secondCreate := performJSON(router, http.MethodPost, "/api/v1/wiki/entries", `{"title":"Probability Review","slug":"probability-review","content":"Separate conditional probability from random variables."}`, creatorToken)
	if secondCreate.Code != http.StatusOK {
		t.Fatalf("expected second wiki create 200, got %d: %s", secondCreate.Code, secondCreate.Body.String())
	}
	var rejectedEntry model.WikiEntry
	if err := db.First(&rejectedEntry, "slug = ?", "probability-review").Error; err != nil {
		t.Fatal(err)
	}
	admin := createTestUser(t, db, "wiki-admin@stu.henu.edu.cn", model.RoleAdmin)
	adminToken := loginTestUser(t, router, "wiki-admin@stu.henu.edu.cn")
	reject := performJSON(router, http.MethodPost, "/api/v1/admin/wiki/entries/"+rejectedEntry.ID+"/reject", `{"reviewReason":"missing course context"}`, adminToken)
	if reject.Code != http.StatusOK {
		t.Fatalf("expected admin wiki reject 200, got %d: %s", reject.Code, reject.Body.String())
	}
	var rejected model.WikiEntry
	if err := db.First(&rejected, "id = ?", rejectedEntry.ID).Error; err != nil {
		t.Fatal(err)
	}
	if rejected.Status != model.StatusRejected || rejected.ReviewerID == nil || *rejected.ReviewerID != admin.ID || rejected.ReviewReason != "missing course context" {
		t.Fatalf("expected rejected wiki review metadata, got %#v", rejected)
	}
	if countOperationLogs(t, db, "wiki_entry.rejected", "wiki_entry", rejectedEntry.ID, admin.ID) != 1 {
		t.Fatal("expected wiki rejection operation log")
	}
	publicRejected := performJSON(router, http.MethodGet, "/api/v1/wiki/entries/"+rejectedEntry.ID, "", "")
	if publicRejected.Code != http.StatusNotFound {
		t.Fatalf("expected rejected wiki hidden from public detail, got %d: %s", publicRejected.Code, publicRejected.Body.String())
	}

	frozenCreate := performJSON(router, http.MethodPost, "/api/v1/wiki/entries", `{"title":"Frozen target","slug":"frozen-wiki-target","content":"body"}`, creatorToken)
	if frozenCreate.Code != http.StatusOK {
		t.Fatalf("expected frozen target create 200, got %d: %s", frozenCreate.Code, frozenCreate.Body.String())
	}
	var frozenTarget model.WikiEntry
	if err := db.First(&frozenTarget, "slug = ?", "frozen-wiki-target").Error; err != nil {
		t.Fatal(err)
	}
	frozenReviewer := createTestUser(t, db, "frozen-wiki-reviewer@stu.henu.edu.cn", model.RoleReviewer)
	frozenReviewerToken := loginTestUser(t, router, "frozen-wiki-reviewer@stu.henu.edu.cn")
	if err := db.Model(&model.User{}).Where("id = ?", frozenReviewer.ID).Update("status", "frozen").Error; err != nil {
		t.Fatal(err)
	}
	frozenReview := performJSON(router, http.MethodPost, "/api/v1/admin/wiki/entries/"+frozenTarget.ID+"/approve", `{"reviewReason":"ok"}`, frozenReviewerToken)
	if frozenReview.Code != http.StatusForbidden || !strings.Contains(frozenReview.Body.String(), "user_frozen") {
		t.Fatalf("expected frozen reviewer wiki approve 403, got %d: %s", frozenReview.Code, frozenReview.Body.String())
	}
	if countOperationLogs(t, db, "wiki_entry.published", "wiki_entry", frozenTarget.ID, frozenReviewer.ID) != 0 {
		t.Fatal("expected frozen reviewer to avoid wiki approval logs")
	}
}
