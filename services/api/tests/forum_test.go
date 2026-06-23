package tests

import (
	"net/http"
	"strings"
	"testing"

	"final-review-platform/services/api/internal/platform/model"
	"final-review-platform/services/api/internal/server"
	applogger "final-review-platform/services/api/pkg/logger"
)

func TestForumSubmissionAndReviewWorkflow(t *testing.T) {
	db := newTestDB(t)
	router := server.NewRouter(testConfig(), applogger.New("test"), db, nil)
	board := model.ForumBoard{Name: "Course Help", Slug: "course-help", Description: "Course discussion", Status: model.StatusPublished}
	if err := db.Create(&board).Error; err != nil {
		t.Fatal(err)
	}

	boardsResponse := performJSON(router, http.MethodGet, "/api/v1/forum/boards", "", "")
	if boardsResponse.Code != http.StatusOK || !strings.Contains(boardsResponse.Body.String(), board.ID) {
		t.Fatalf("expected public forum board list, got %d: %s", boardsResponse.Code, boardsResponse.Body.String())
	}

	unauthorizedCreate := performJSON(router, http.MethodPost, "/api/v1/forum/posts", `{"boardId":"`+board.ID+`","title":"No auth","content":"body"}`, "")
	if unauthorizedCreate.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthenticated forum create 401, got %d: %s", unauthorizedCreate.Code, unauthorizedCreate.Body.String())
	}

	studentToken := loginTestUser(t, router, "forum-author@stu.henu.edu.cn")
	invalidType := performJSON(router, http.MethodPost, "/api/v1/forum/posts", `{"boardId":"`+board.ID+`","title":"Bad type","content":"body","type":"reward"}`, studentToken)
	if invalidType.Code != http.StatusBadRequest || !strings.Contains(invalidType.Body.String(), "invalid_post_type") {
		t.Fatalf("expected invalid forum post type rejection, got %d: %s", invalidType.Code, invalidType.Body.String())
	}
	missingBoard := performJSON(router, http.MethodPost, "/api/v1/forum/posts", `{"boardId":"00000000-0000-0000-0000-000000000000","title":"Missing board","content":"body"}`, studentToken)
	if missingBoard.Code != http.StatusBadRequest || !strings.Contains(missingBoard.Body.String(), "board_not_found") {
		t.Fatalf("expected missing board rejection, got %d: %s", missingBoard.Code, missingBoard.Body.String())
	}

	createBody := `{"boardId":"` + board.ID + `","title":"How to review graph theory","content":"Trees and shortest paths are confusing.","type":"question"}`
	createResponse := performJSON(router, http.MethodPost, "/api/v1/forum/posts", createBody, studentToken)
	if createResponse.Code != http.StatusOK {
		t.Fatalf("expected forum create 200, got %d: %s", createResponse.Code, createResponse.Body.String())
	}
	var post model.ForumPost
	if err := db.First(&post, "title = ?", "How to review graph theory").Error; err != nil {
		t.Fatal(err)
	}
	if post.Status != model.StatusPending || post.AuthorID == "" || post.BoardID != board.ID || post.Type != "question" {
		t.Fatalf("expected pending forum post with author/board/type, got %#v", post)
	}

	publicList := performJSON(router, http.MethodGet, "/api/v1/forum/posts", "", "")
	if publicList.Code != http.StatusOK || strings.Contains(publicList.Body.String(), post.ID) {
		t.Fatalf("expected pending forum post hidden from public list, got %d: %s", publicList.Code, publicList.Body.String())
	}
	publicDetail := performJSON(router, http.MethodGet, "/api/v1/forum/posts/"+post.ID, "", "")
	if publicDetail.Code != http.StatusNotFound {
		t.Fatalf("expected pending forum public detail 404, got %d: %s", publicDetail.Code, publicDetail.Body.String())
	}

	forbiddenReviewList := performJSON(router, http.MethodGet, "/api/v1/admin/forum/posts", "", studentToken)
	if forbiddenReviewList.Code != http.StatusForbidden {
		t.Fatalf("expected student forum review list 403, got %d: %s", forbiddenReviewList.Code, forbiddenReviewList.Body.String())
	}
	forbiddenApprove := performJSON(router, http.MethodPost, "/api/v1/admin/forum/posts/"+post.ID+"/approve", `{"reviewReason":"ok"}`, studentToken)
	if forbiddenApprove.Code != http.StatusForbidden {
		t.Fatalf("expected student forum approve 403, got %d: %s", forbiddenApprove.Code, forbiddenApprove.Body.String())
	}

	reviewer := createTestUser(t, db, "forum-reviewer@stu.henu.edu.cn", model.RoleReviewer)
	reviewerToken := loginTestUser(t, router, "forum-reviewer@stu.henu.edu.cn")
	reviewList := performJSON(router, http.MethodGet, "/api/v1/admin/forum/posts", "", reviewerToken)
	if reviewList.Code != http.StatusOK || !strings.Contains(reviewList.Body.String(), post.ID) {
		t.Fatalf("expected reviewer forum list to include pending post, got %d: %s", reviewList.Code, reviewList.Body.String())
	}
	rejectWithoutReason := performJSON(router, http.MethodPost, "/api/v1/admin/forum/posts/"+post.ID+"/reject", "", reviewerToken)
	if rejectWithoutReason.Code != http.StatusBadRequest || !strings.Contains(rejectWithoutReason.Body.String(), "review_reason_required") {
		t.Fatalf("expected reject reason required, got %d: %s", rejectWithoutReason.Code, rejectWithoutReason.Body.String())
	}

	approve := performJSON(router, http.MethodPost, "/api/v1/admin/forum/posts/"+post.ID+"/approve", `{"reviewReason":"useful question"}`, reviewerToken)
	if approve.Code != http.StatusOK {
		t.Fatalf("expected reviewer forum approve 200, got %d: %s", approve.Code, approve.Body.String())
	}
	var approved model.ForumPost
	if err := db.First(&approved, "id = ?", post.ID).Error; err != nil {
		t.Fatal(err)
	}
	if approved.Status != model.StatusPublished || approved.ReviewerID == nil || *approved.ReviewerID != reviewer.ID || approved.ReviewedAt == nil || approved.ReviewReason != "useful question" {
		t.Fatalf("expected approved forum review metadata, got %#v", approved)
	}
	if countOperationLogs(t, db, "forum_post.published", "forum_post", post.ID, reviewer.ID) != 1 {
		t.Fatal("expected forum approval operation log")
	}

	publicListAfterApprove := performJSON(router, http.MethodGet, "/api/v1/forum/posts?boardId="+board.ID, "", "")
	if publicListAfterApprove.Code != http.StatusOK || !strings.Contains(publicListAfterApprove.Body.String(), post.ID) {
		t.Fatalf("expected approved forum post in public list, got %d: %s", publicListAfterApprove.Code, publicListAfterApprove.Body.String())
	}
	publicApprovedDetail := performJSON(router, http.MethodGet, "/api/v1/forum/posts/"+post.ID, "", "")
	if publicApprovedDetail.Code != http.StatusOK {
		t.Fatalf("expected approved forum detail 200, got %d: %s", publicApprovedDetail.Code, publicApprovedDetail.Body.String())
	}
	if strings.Contains(publicApprovedDetail.Body.String(), "reviewerId") || strings.Contains(publicApprovedDetail.Body.String(), "reviewReason") {
		t.Fatalf("expected public forum detail to hide review metadata, got %s", publicApprovedDetail.Body.String())
	}
	reviewAgain := performJSON(router, http.MethodPost, "/api/v1/admin/forum/posts/"+post.ID+"/reject", `{"reviewReason":"overwrite"}`, reviewerToken)
	if reviewAgain.Code != http.StatusConflict || !strings.Contains(reviewAgain.Body.String(), "forum_post_not_reviewable") {
		t.Fatalf("expected reviewed forum conflict, got %d: %s", reviewAgain.Code, reviewAgain.Body.String())
	}
	if countOperationLogs(t, db, "forum_post.published", "forum_post", post.ID, reviewer.ID) != 1 {
		t.Fatal("expected repeated forum review to avoid duplicate approval logs")
	}

	secondCreate := performJSON(router, http.MethodPost, "/api/v1/forum/posts", `{"boardId":"`+board.ID+`","title":"Thin post","content":"too short"}`, studentToken)
	if secondCreate.Code != http.StatusOK {
		t.Fatalf("expected second forum create 200, got %d: %s", secondCreate.Code, secondCreate.Body.String())
	}
	var rejectedPost model.ForumPost
	if err := db.First(&rejectedPost, "title = ?", "Thin post").Error; err != nil {
		t.Fatal(err)
	}
	admin := createTestUser(t, db, "forum-admin@stu.henu.edu.cn", model.RoleAdmin)
	adminToken := loginTestUser(t, router, "forum-admin@stu.henu.edu.cn")
	reject := performJSON(router, http.MethodPost, "/api/v1/admin/forum/posts/"+rejectedPost.ID+"/reject", `{"reviewReason":"missing details"}`, adminToken)
	if reject.Code != http.StatusOK {
		t.Fatalf("expected admin forum reject 200, got %d: %s", reject.Code, reject.Body.String())
	}
	var rejected model.ForumPost
	if err := db.First(&rejected, "id = ?", rejectedPost.ID).Error; err != nil {
		t.Fatal(err)
	}
	if rejected.Status != model.StatusRejected || rejected.ReviewerID == nil || *rejected.ReviewerID != admin.ID || rejected.ReviewReason != "missing details" {
		t.Fatalf("expected rejected forum review metadata, got %#v", rejected)
	}
	if countOperationLogs(t, db, "forum_post.rejected", "forum_post", rejectedPost.ID, admin.ID) != 1 {
		t.Fatal("expected forum rejection operation log")
	}
	publicRejected := performJSON(router, http.MethodGet, "/api/v1/forum/posts/"+rejectedPost.ID, "", "")
	if publicRejected.Code != http.StatusNotFound {
		t.Fatalf("expected rejected forum post hidden from public detail, got %d: %s", publicRejected.Code, publicRejected.Body.String())
	}

	frozenCreate := performJSON(router, http.MethodPost, "/api/v1/forum/posts", `{"boardId":"`+board.ID+`","title":"Frozen target","content":"body"}`, studentToken)
	if frozenCreate.Code != http.StatusOK {
		t.Fatalf("expected frozen target create 200, got %d: %s", frozenCreate.Code, frozenCreate.Body.String())
	}
	var frozenTarget model.ForumPost
	if err := db.First(&frozenTarget, "title = ?", "Frozen target").Error; err != nil {
		t.Fatal(err)
	}
	frozenReviewer := createTestUser(t, db, "frozen-forum-reviewer@stu.henu.edu.cn", model.RoleReviewer)
	frozenReviewerToken := loginTestUser(t, router, "frozen-forum-reviewer@stu.henu.edu.cn")
	if err := db.Model(&model.User{}).Where("id = ?", frozenReviewer.ID).Update("status", "frozen").Error; err != nil {
		t.Fatal(err)
	}
	frozenReview := performJSON(router, http.MethodPost, "/api/v1/admin/forum/posts/"+frozenTarget.ID+"/approve", `{"reviewReason":"ok"}`, frozenReviewerToken)
	if frozenReview.Code != http.StatusForbidden || !strings.Contains(frozenReview.Body.String(), "user_frozen") {
		t.Fatalf("expected frozen reviewer forum approve 403, got %d: %s", frozenReview.Code, frozenReview.Body.String())
	}
	if countOperationLogs(t, db, "forum_post.published", "forum_post", frozenTarget.ID, frozenReviewer.ID) != 0 {
		t.Fatal("expected frozen reviewer to avoid forum approval logs")
	}
}
