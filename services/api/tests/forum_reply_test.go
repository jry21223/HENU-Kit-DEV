package tests

import (
	"net/http"
	"strings"
	"testing"

	"final-review-platform/services/api/internal/platform/model"
	"final-review-platform/services/api/internal/server"
	applogger "final-review-platform/services/api/pkg/logger"
)

func TestForumReplySubmissionAndReviewWorkflow(t *testing.T) {
	db := newTestDB(t)
	router := server.NewRouter(testConfig(), applogger.New("test"), db, nil)
	board := model.ForumBoard{Name: "Course Help", Slug: "course-help", Description: "Course discussion", Status: model.StatusPublished}
	if err := db.Create(&board).Error; err != nil {
		t.Fatal(err)
	}
	author := createTestUser(t, db, "forum-post-owner@stu.henu.edu.cn", model.RoleUser)
	post := model.ForumPost{
		ContentStats: model.ContentStats{Visibility: "public"},
		AuthorID:     author.ID,
		BoardID:      board.ID,
		Title:        "Graph theory review",
		Content:      "How should I review graph theory?",
		Type:         "question",
		Status:       model.StatusPublished,
	}
	if err := db.Create(&post).Error; err != nil {
		t.Fatal(err)
	}
	pendingPost := model.ForumPost{
		ContentStats: model.ContentStats{Visibility: "public"},
		AuthorID:     author.ID,
		BoardID:      board.ID,
		Title:        "Pending post",
		Content:      "Hidden until review",
		Type:         "normal",
		Status:       model.StatusPending,
	}
	if err := db.Create(&pendingPost).Error; err != nil {
		t.Fatal(err)
	}

	unauthorizedCreate := performJSON(router, http.MethodPost, "/api/v1/forum/posts/"+post.ID+"/replies", `{"content":"No auth"}`, "")
	if unauthorizedCreate.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthenticated reply create 401, got %d: %s", unauthorizedCreate.Code, unauthorizedCreate.Body.String())
	}

	studentToken := loginTestUser(t, router, "forum-replier@stu.henu.edu.cn")
	emptyReply := performJSON(router, http.MethodPost, "/api/v1/forum/posts/"+post.ID+"/replies", `{"content":"   "}`, studentToken)
	if emptyReply.Code != http.StatusBadRequest || !strings.Contains(emptyReply.Body.String(), "missing_required_fields") {
		t.Fatalf("expected empty reply rejection, got %d: %s", emptyReply.Code, emptyReply.Body.String())
	}
	hiddenPostReply := performJSON(router, http.MethodPost, "/api/v1/forum/posts/"+pendingPost.ID+"/replies", `{"content":"Should not attach"}`, studentToken)
	if hiddenPostReply.Code != http.StatusNotFound || !strings.Contains(hiddenPostReply.Body.String(), "post_not_found") {
		t.Fatalf("expected pending-post reply rejection, got %d: %s", hiddenPostReply.Code, hiddenPostReply.Body.String())
	}

	createReply := performJSON(router, http.MethodPost, "/api/v1/forum/posts/"+post.ID+"/replies", `{"content":"Start with trees, then shortest path exercises."}`, studentToken)
	if createReply.Code != http.StatusOK {
		t.Fatalf("expected reply create 200, got %d: %s", createReply.Code, createReply.Body.String())
	}
	var reply model.ForumReply
	if err := db.First(&reply, "content = ?", "Start with trees, then shortest path exercises.").Error; err != nil {
		t.Fatal(err)
	}
	if reply.Status != model.StatusPending || reply.AuthorID == "" || reply.PostID != post.ID {
		t.Fatalf("expected pending forum reply with author/post, got %#v", reply)
	}
	publicDetail := performJSON(router, http.MethodGet, "/api/v1/forum/posts/"+post.ID, "", "")
	if publicDetail.Code != http.StatusOK {
		t.Fatalf("expected public forum detail 200, got %d: %s", publicDetail.Code, publicDetail.Body.String())
	}
	if strings.Contains(publicDetail.Body.String(), reply.ID) {
		t.Fatalf("expected pending reply hidden from public detail, got %s", publicDetail.Body.String())
	}

	forbiddenReviewList := performJSON(router, http.MethodGet, "/api/v1/admin/forum/replies", "", studentToken)
	if forbiddenReviewList.Code != http.StatusForbidden {
		t.Fatalf("expected student forum reply review list 403, got %d: %s", forbiddenReviewList.Code, forbiddenReviewList.Body.String())
	}
	forbiddenApprove := performJSON(router, http.MethodPost, "/api/v1/admin/forum/replies/"+reply.ID+"/approve", `{"reviewReason":"ok"}`, studentToken)
	if forbiddenApprove.Code != http.StatusForbidden {
		t.Fatalf("expected student forum reply approve 403, got %d: %s", forbiddenApprove.Code, forbiddenApprove.Body.String())
	}

	reviewer := createTestUser(t, db, "forum-reply-reviewer@stu.henu.edu.cn", model.RoleReviewer)
	reviewerToken := loginTestUser(t, router, "forum-reply-reviewer@stu.henu.edu.cn")
	reviewList := performJSON(router, http.MethodGet, "/api/v1/admin/forum/replies", "", reviewerToken)
	if reviewList.Code != http.StatusOK || !strings.Contains(reviewList.Body.String(), reply.ID) {
		t.Fatalf("expected reviewer reply list to include pending reply, got %d: %s", reviewList.Code, reviewList.Body.String())
	}
	rejectWithoutReason := performJSON(router, http.MethodPost, "/api/v1/admin/forum/replies/"+reply.ID+"/reject", "", reviewerToken)
	if rejectWithoutReason.Code != http.StatusBadRequest || !strings.Contains(rejectWithoutReason.Body.String(), "review_reason_required") {
		t.Fatalf("expected reply reject reason required, got %d: %s", rejectWithoutReason.Code, rejectWithoutReason.Body.String())
	}

	approve := performJSON(router, http.MethodPost, "/api/v1/admin/forum/replies/"+reply.ID+"/approve", `{"reviewReason":"helpful answer"}`, reviewerToken)
	if approve.Code != http.StatusOK {
		t.Fatalf("expected reviewer reply approve 200, got %d: %s", approve.Code, approve.Body.String())
	}
	var approvedReply model.ForumReply
	if err := db.First(&approvedReply, "id = ?", reply.ID).Error; err != nil {
		t.Fatal(err)
	}
	if approvedReply.Status != model.StatusPublished || approvedReply.ReviewerID == nil || *approvedReply.ReviewerID != reviewer.ID || approvedReply.ReviewedAt == nil || approvedReply.ReviewReason != "helpful answer" {
		t.Fatalf("expected approved reply review metadata, got %#v", approvedReply)
	}
	var updatedPost model.ForumPost
	if err := db.First(&updatedPost, "id = ?", post.ID).Error; err != nil {
		t.Fatal(err)
	}
	if updatedPost.CommentCount != 1 {
		t.Fatalf("expected approved reply to increment post comment count once, got %d", updatedPost.CommentCount)
	}
	if countOperationLogs(t, db, "forum_reply.published", "forum_reply", reply.ID, reviewer.ID) != 1 {
		t.Fatal("expected forum reply approval operation log")
	}

	publicDetailAfterApprove := performJSON(router, http.MethodGet, "/api/v1/forum/posts/"+post.ID, "", "")
	if publicDetailAfterApprove.Code != http.StatusOK || !strings.Contains(publicDetailAfterApprove.Body.String(), reply.ID) {
		t.Fatalf("expected approved reply in public forum detail, got %d: %s", publicDetailAfterApprove.Code, publicDetailAfterApprove.Body.String())
	}
	if strings.Contains(publicDetailAfterApprove.Body.String(), "reviewerId") || strings.Contains(publicDetailAfterApprove.Body.String(), "reviewReason") {
		t.Fatalf("expected public forum replies to hide review metadata, got %s", publicDetailAfterApprove.Body.String())
	}
	reviewAgain := performJSON(router, http.MethodPost, "/api/v1/admin/forum/replies/"+reply.ID+"/reject", `{"reviewReason":"overwrite"}`, reviewerToken)
	if reviewAgain.Code != http.StatusConflict || !strings.Contains(reviewAgain.Body.String(), "forum_reply_not_reviewable") {
		t.Fatalf("expected reviewed reply conflict, got %d: %s", reviewAgain.Code, reviewAgain.Body.String())
	}
	var postAfterRepeat model.ForumPost
	if err := db.First(&postAfterRepeat, "id = ?", post.ID).Error; err != nil {
		t.Fatal(err)
	}
	if postAfterRepeat.CommentCount != 1 {
		t.Fatalf("expected repeated reply review to avoid duplicate comment count, got %d", postAfterRepeat.CommentCount)
	}
	if countOperationLogs(t, db, "forum_reply.published", "forum_reply", reply.ID, reviewer.ID) != 1 {
		t.Fatal("expected repeated reply review to avoid duplicate approval logs")
	}

	secondReplyResponse := performJSON(router, http.MethodPost, "/api/v1/forum/posts/"+post.ID+"/replies", `{"content":"Too vague."}`, studentToken)
	if secondReplyResponse.Code != http.StatusOK {
		t.Fatalf("expected second reply create 200, got %d: %s", secondReplyResponse.Code, secondReplyResponse.Body.String())
	}
	var rejectedReply model.ForumReply
	if err := db.First(&rejectedReply, "content = ?", "Too vague.").Error; err != nil {
		t.Fatal(err)
	}
	admin := createTestUser(t, db, "forum-reply-admin@stu.henu.edu.cn", model.RoleAdmin)
	adminToken := loginTestUser(t, router, "forum-reply-admin@stu.henu.edu.cn")
	reject := performJSON(router, http.MethodPost, "/api/v1/admin/forum/replies/"+rejectedReply.ID+"/reject", `{"reviewReason":"not constructive"}`, adminToken)
	if reject.Code != http.StatusOK {
		t.Fatalf("expected admin reply reject 200, got %d: %s", reject.Code, reject.Body.String())
	}
	var rejected model.ForumReply
	if err := db.First(&rejected, "id = ?", rejectedReply.ID).Error; err != nil {
		t.Fatal(err)
	}
	if rejected.Status != model.StatusRejected || rejected.ReviewerID == nil || *rejected.ReviewerID != admin.ID || rejected.ReviewReason != "not constructive" {
		t.Fatalf("expected rejected reply review metadata, got %#v", rejected)
	}
	if countOperationLogs(t, db, "forum_reply.rejected", "forum_reply", rejectedReply.ID, admin.ID) != 1 {
		t.Fatal("expected forum reply rejection operation log")
	}
	publicAfterReject := performJSON(router, http.MethodGet, "/api/v1/forum/posts/"+post.ID, "", "")
	if publicAfterReject.Code != http.StatusOK || strings.Contains(publicAfterReject.Body.String(), rejectedReply.ID) {
		t.Fatalf("expected rejected reply hidden from public detail, got %d: %s", publicAfterReject.Code, publicAfterReject.Body.String())
	}

	frozenReplyResponse := performJSON(router, http.MethodPost, "/api/v1/forum/posts/"+post.ID+"/replies", `{"content":"Frozen target reply"}`, studentToken)
	if frozenReplyResponse.Code != http.StatusOK {
		t.Fatalf("expected frozen target reply create 200, got %d: %s", frozenReplyResponse.Code, frozenReplyResponse.Body.String())
	}
	var frozenTarget model.ForumReply
	if err := db.First(&frozenTarget, "content = ?", "Frozen target reply").Error; err != nil {
		t.Fatal(err)
	}
	frozenReviewer := createTestUser(t, db, "frozen-forum-reply-reviewer@stu.henu.edu.cn", model.RoleReviewer)
	frozenReviewerToken := loginTestUser(t, router, "frozen-forum-reply-reviewer@stu.henu.edu.cn")
	if err := db.Model(&model.User{}).Where("id = ?", frozenReviewer.ID).Update("status", "frozen").Error; err != nil {
		t.Fatal(err)
	}
	frozenReview := performJSON(router, http.MethodPost, "/api/v1/admin/forum/replies/"+frozenTarget.ID+"/approve", `{"reviewReason":"ok"}`, frozenReviewerToken)
	if frozenReview.Code != http.StatusForbidden || !strings.Contains(frozenReview.Body.String(), "user_frozen") {
		t.Fatalf("expected frozen reviewer reply approve 403, got %d: %s", frozenReview.Code, frozenReview.Body.String())
	}
	if countOperationLogs(t, db, "forum_reply.published", "forum_reply", frozenTarget.ID, frozenReviewer.ID) != 0 {
		t.Fatal("expected frozen reviewer to avoid forum reply approval logs")
	}
}
