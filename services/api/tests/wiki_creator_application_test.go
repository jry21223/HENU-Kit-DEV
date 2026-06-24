package tests

import (
	"net/http"
	"strings"
	"testing"

	"final-review-platform/services/api/internal/platform/model"
	"final-review-platform/services/api/internal/server"
	applogger "final-review-platform/services/api/pkg/logger"
)

func TestWikiCreatorApplicationWorkflow(t *testing.T) {
	db := newTestDB(t)
	router := server.NewRouter(testConfig(), applogger.New("test"), db, nil)

	unauthorized := performJSON(router, http.MethodPost, "/api/v1/wiki/creator-applications", `{"reason":"I can write","sampleTitle":"Sample","sampleBody":"Body"}`, "")
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthenticated creator application 401, got %d: %s", unauthorized.Code, unauthorized.Body.String())
	}

	applicant := createTestUser(t, db, "creator-apply@stu.henu.edu.cn", model.RoleUser)
	applicantToken := loginTestUser(t, router, applicant.Email)
	emptyMine := performJSON(router, http.MethodGet, "/api/v1/wiki/creator-applications/me", "", applicantToken)
	if emptyMine.Code != http.StatusOK || !strings.Contains(emptyMine.Body.String(), `"applications":[]`) {
		t.Fatalf("expected empty creator application self list, got %d: %s", emptyMine.Code, emptyMine.Body.String())
	}
	invalid := performJSON(router, http.MethodPost, "/api/v1/wiki/creator-applications", `{"reason":"","sampleTitle":"Sample","sampleBody":"Body"}`, applicantToken)
	if invalid.Code != http.StatusBadRequest || !strings.Contains(invalid.Body.String(), "missing_required_fields") {
		t.Fatalf("expected invalid creator application rejection, got %d: %s", invalid.Code, invalid.Body.String())
	}

	create := performJSON(router, http.MethodPost, "/api/v1/wiki/creator-applications", `{"reason":"I want to maintain course notes","sampleTitle":"Logic summary","sampleBody":"A useful sample body for review."}`, applicantToken)
	if create.Code != http.StatusOK {
		t.Fatalf("expected creator application create 200, got %d: %s", create.Code, create.Body.String())
	}
	var application model.WikiCreatorApplication
	if err := db.First(&application, "user_id = ?", applicant.ID).Error; err != nil {
		t.Fatal(err)
	}
	if application.Status != model.StatusPending || application.SampleTitle != "Logic summary" {
		t.Fatalf("expected pending creator application, got %#v", application)
	}
	mine := performJSON(router, http.MethodGet, "/api/v1/wiki/creator-applications/me", "", applicantToken)
	if mine.Code != http.StatusOK || !strings.Contains(mine.Body.String(), application.ID) || strings.Contains(mine.Body.String(), "reviewerId") {
		t.Fatalf("expected self list with redacted creator application, got %d: %s", mine.Code, mine.Body.String())
	}
	otherUser := createTestUser(t, db, "creator-apply-other@stu.henu.edu.cn", model.RoleUser)
	otherToken := loginTestUser(t, router, otherUser.Email)
	otherMine := performJSON(router, http.MethodGet, "/api/v1/wiki/creator-applications/me", "", otherToken)
	if otherMine.Code != http.StatusOK || strings.Contains(otherMine.Body.String(), application.ID) {
		t.Fatalf("expected other user self list not to include applicant application, got %d: %s", otherMine.Code, otherMine.Body.String())
	}

	duplicate := performJSON(router, http.MethodPost, "/api/v1/wiki/creator-applications", `{"reason":"Again","sampleTitle":"Another","sampleBody":"Another body"}`, applicantToken)
	if duplicate.Code != http.StatusConflict || !strings.Contains(duplicate.Body.String(), "creator_application_pending") {
		t.Fatalf("expected duplicate pending application conflict, got %d: %s", duplicate.Code, duplicate.Body.String())
	}

	creator := createTestUser(t, db, "already-creator@stu.henu.edu.cn", model.RoleCreator)
	creatorToken := loginTestUser(t, router, creator.Email)
	alreadyCreator := performJSON(router, http.MethodPost, "/api/v1/wiki/creator-applications", `{"reason":"Already","sampleTitle":"Sample","sampleBody":"Body"}`, creatorToken)
	if alreadyCreator.Code != http.StatusConflict || !strings.Contains(alreadyCreator.Body.String(), "already_creator") {
		t.Fatalf("expected already creator conflict, got %d: %s", alreadyCreator.Code, alreadyCreator.Body.String())
	}
	reviewerApplicant := createTestUser(t, db, "reviewer-apply@stu.henu.edu.cn", model.RoleReviewer)
	reviewerApplicantToken := loginTestUser(t, router, reviewerApplicant.Email)
	unsupportedRole := performJSON(router, http.MethodPost, "/api/v1/wiki/creator-applications", `{"reason":"Reviewer wants creator","sampleTitle":"Sample","sampleBody":"Body"}`, reviewerApplicantToken)
	if unsupportedRole.Code != http.StatusConflict || !strings.Contains(unsupportedRole.Body.String(), "creator_application_role_not_supported") {
		t.Fatalf("expected reviewer application role conflict, got %d: %s", unsupportedRole.Code, unsupportedRole.Body.String())
	}

	forbiddenList := performJSON(router, http.MethodGet, "/api/v1/admin/wiki/creator-applications", "", applicantToken)
	if forbiddenList.Code != http.StatusForbidden {
		t.Fatalf("expected student creator application list 403, got %d: %s", forbiddenList.Code, forbiddenList.Body.String())
	}
	forbiddenApprove := performJSON(router, http.MethodPost, "/api/v1/admin/wiki/creator-applications/"+application.ID+"/approve", `{"reviewReason":"ok"}`, applicantToken)
	if forbiddenApprove.Code != http.StatusForbidden {
		t.Fatalf("expected student creator application approve 403, got %d: %s", forbiddenApprove.Code, forbiddenApprove.Body.String())
	}

	reviewer := createTestUser(t, db, "creator-application-reviewer@stu.henu.edu.cn", model.RoleReviewer)
	reviewerToken := loginTestUser(t, router, reviewer.Email)
	list := performJSON(router, http.MethodGet, "/api/v1/admin/wiki/creator-applications", "", reviewerToken)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), application.ID) {
		t.Fatalf("expected reviewer list to include pending application, got %d: %s", list.Code, list.Body.String())
	}
	approve := performJSON(router, http.MethodPost, "/api/v1/admin/wiki/creator-applications/"+application.ID+"/approve", `{"reviewReason":"sample is useful"}`, reviewerToken)
	if approve.Code != http.StatusOK {
		t.Fatalf("expected reviewer approve 200, got %d: %s", approve.Code, approve.Body.String())
	}
	var approved model.WikiCreatorApplication
	if err := db.First(&approved, "id = ?", application.ID).Error; err != nil {
		t.Fatal(err)
	}
	if approved.Status != model.StatusApproved || approved.ReviewerID == nil || *approved.ReviewerID != reviewer.ID || approved.ReviewedAt == nil {
		t.Fatalf("expected approved creator application metadata, got %#v", approved)
	}
	var promoted model.User
	if err := db.First(&promoted, "id = ?", applicant.ID).Error; err != nil {
		t.Fatal(err)
	}
	if promoted.Role != model.RoleCreator {
		t.Fatalf("expected applicant promoted to creator, got role %s", promoted.Role)
	}
	if countOperationLogs(t, db, "wiki_creator_application.approved", "wiki_creator_application", application.ID, reviewer.ID) != 1 {
		t.Fatal("expected creator application approval operation log")
	}
	var notificationCount int64
	if err := db.Model(&model.Notification{}).Where("user_id = ? AND type = ? AND data LIKE ?", applicant.ID, "content_review", "%wiki_creator_application%").Count(&notificationCount).Error; err != nil {
		t.Fatal(err)
	}
	if notificationCount != 1 {
		t.Fatalf("expected one creator application notification, got %d", notificationCount)
	}
	approvedMine := performJSON(router, http.MethodGet, "/api/v1/wiki/creator-applications/me", "", applicantToken)
	if approvedMine.Code != http.StatusOK || !strings.Contains(approvedMine.Body.String(), model.StatusApproved) || !strings.Contains(approvedMine.Body.String(), "sample is useful") || strings.Contains(approvedMine.Body.String(), "reviewerId") {
		t.Fatalf("expected approved application self status without reviewer id, got %d: %s", approvedMine.Code, approvedMine.Body.String())
	}
	reviewAgain := performJSON(router, http.MethodPost, "/api/v1/admin/wiki/creator-applications/"+application.ID+"/reject", `{"reviewReason":"late"}`, reviewerToken)
	if reviewAgain.Code != http.StatusConflict || !strings.Contains(reviewAgain.Body.String(), "creator_application_not_reviewable") {
		t.Fatalf("expected reviewed application conflict, got %d: %s", reviewAgain.Code, reviewAgain.Body.String())
	}
}

func TestWikiCreatorApplicationRejectAndFrozenBoundary(t *testing.T) {
	db := newTestDB(t)
	router := server.NewRouter(testConfig(), applogger.New("test"), db, nil)

	rejectedUser := createTestUser(t, db, "creator-reject@stu.henu.edu.cn", model.RoleUser)
	rejectedToken := loginTestUser(t, router, rejectedUser.Email)
	create := performJSON(router, http.MethodPost, "/api/v1/wiki/creator-applications", `{"reason":"Try creator","sampleTitle":"Weak sample","sampleBody":"Too short but present."}`, rejectedToken)
	if create.Code != http.StatusOK {
		t.Fatalf("expected creator application create 200, got %d: %s", create.Code, create.Body.String())
	}
	var application model.WikiCreatorApplication
	if err := db.First(&application, "user_id = ?", rejectedUser.ID).Error; err != nil {
		t.Fatal(err)
	}
	admin := createTestUser(t, db, "creator-application-admin@stu.henu.edu.cn", model.RoleAdmin)
	adminToken := loginTestUser(t, router, admin.Email)
	rejectWithoutReason := performJSON(router, http.MethodPost, "/api/v1/admin/wiki/creator-applications/"+application.ID+"/reject", "", adminToken)
	if rejectWithoutReason.Code != http.StatusBadRequest || !strings.Contains(rejectWithoutReason.Body.String(), "review_reason_required") {
		t.Fatalf("expected reject reason required, got %d: %s", rejectWithoutReason.Code, rejectWithoutReason.Body.String())
	}
	reject := performJSON(router, http.MethodPost, "/api/v1/admin/wiki/creator-applications/"+application.ID+"/reject", `{"reviewReason":"sample needs more structure"}`, adminToken)
	if reject.Code != http.StatusOK {
		t.Fatalf("expected creator application reject 200, got %d: %s", reject.Code, reject.Body.String())
	}
	var userAfterReject model.User
	if err := db.First(&userAfterReject, "id = ?", rejectedUser.ID).Error; err != nil {
		t.Fatal(err)
	}
	if userAfterReject.Role != model.RoleUser {
		t.Fatalf("expected rejected applicant to remain user, got %s", userAfterReject.Role)
	}
	if countOperationLogs(t, db, "wiki_creator_application.rejected", "wiki_creator_application", application.ID, admin.ID) != 1 {
		t.Fatal("expected creator application rejection operation log")
	}
	rejectedMine := performJSON(router, http.MethodGet, "/api/v1/wiki/creator-applications/me", "", rejectedToken)
	if rejectedMine.Code != http.StatusOK || !strings.Contains(rejectedMine.Body.String(), model.StatusRejected) || !strings.Contains(rejectedMine.Body.String(), "sample needs more structure") {
		t.Fatalf("expected rejected application self status, got %d: %s", rejectedMine.Code, rejectedMine.Body.String())
	}

	frozenUser := createTestUser(t, db, "creator-frozen@stu.henu.edu.cn", model.RoleUser)
	frozenToken := loginTestUser(t, router, frozenUser.Email)
	if err := db.Model(&model.User{}).Where("id = ?", frozenUser.ID).Update("status", "frozen").Error; err != nil {
		t.Fatal(err)
	}
	frozenCreate := performJSON(router, http.MethodPost, "/api/v1/wiki/creator-applications", `{"reason":"Frozen","sampleTitle":"Frozen sample","sampleBody":"Frozen body"}`, frozenToken)
	if frozenCreate.Code != http.StatusForbidden || !strings.Contains(frozenCreate.Body.String(), "user_frozen") {
		t.Fatalf("expected frozen user creator application 403, got %d: %s", frozenCreate.Code, frozenCreate.Body.String())
	}
}
