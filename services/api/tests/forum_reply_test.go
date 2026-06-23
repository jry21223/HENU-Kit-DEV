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

func TestForumBestAnswerSettlementWorkflow(t *testing.T) {
	db := newTestDB(t)
	router := server.NewRouter(testConfig(), applogger.New("test"), db, nil)
	board := model.ForumBoard{Name: "Reward Help", Slug: "reward-help", Description: "Reward discussion", Status: model.StatusPublished}
	if err := db.Create(&board).Error; err != nil {
		t.Fatal(err)
	}
	owner := createTestUser(t, db, "forum-best-owner@stu.henu.edu.cn", model.RoleUser)
	if err := db.Model(&model.User{}).Where("id = ?", owner.ID).Update("points_balance", 100).Error; err != nil {
		t.Fatal(err)
	}
	ownerToken := loginTestUser(t, router, owner.Email)
	answerer := createTestUser(t, db, "forum-best-answerer@stu.henu.edu.cn", model.RoleUser)
	answererToken := loginTestUser(t, router, answerer.Email)
	reviewer := createTestUser(t, db, "forum-best-reviewer@stu.henu.edu.cn", model.RoleReviewer)
	reviewerToken := loginTestUser(t, router, reviewer.Email)

	createPost := performJSON(router, http.MethodPost, "/api/v1/forum/posts", `{"boardId":"`+board.ID+`","title":"Reward answer selection","content":"Need a complete solution with proof steps.","type":"reward","rewardPoints":45}`, ownerToken)
	if createPost.Code != http.StatusOK {
		t.Fatalf("expected reward post create 200, got %d: %s", createPost.Code, createPost.Body.String())
	}
	var rewardPost model.ForumPost
	if err := db.First(&rewardPost, "title = ?", "Reward answer selection").Error; err != nil {
		t.Fatal(err)
	}
	if rewardPost.RewardStatus != "escrowed" {
		t.Fatalf("expected reward post escrowed before settlement, got %#v", rewardPost)
	}
	if err := db.First(&owner, "id = ?", owner.ID).Error; err != nil {
		t.Fatal(err)
	}
	if owner.PointsBalance != 55 {
		t.Fatalf("expected owner balance 55 after escrow, got %d", owner.PointsBalance)
	}
	approvePost := performJSON(router, http.MethodPost, "/api/v1/admin/forum/posts/"+rewardPost.ID+"/approve", `{"reviewReason":"clear reward question"}`, reviewerToken)
	if approvePost.Code != http.StatusOK {
		t.Fatalf("expected reward post approve 200, got %d: %s", approvePost.Code, approvePost.Body.String())
	}
	createReply := performJSON(router, http.MethodPost, "/api/v1/forum/posts/"+rewardPost.ID+"/replies", `{"content":"Use induction on the number of edges and keep the invariant explicit."}`, answererToken)
	if createReply.Code != http.StatusOK {
		t.Fatalf("expected reward reply create 200, got %d: %s", createReply.Code, createReply.Body.String())
	}
	var reply model.ForumReply
	if err := db.First(&reply, "content = ?", "Use induction on the number of edges and keep the invariant explicit.").Error; err != nil {
		t.Fatal(err)
	}
	approveReply := performJSON(router, http.MethodPost, "/api/v1/admin/forum/replies/"+reply.ID+"/approve", `{"reviewReason":"answers the question"}`, reviewerToken)
	if approveReply.Code != http.StatusOK {
		t.Fatalf("expected reward reply approve 200, got %d: %s", approveReply.Code, approveReply.Body.String())
	}

	unauthorized := performJSON(router, http.MethodPost, "/api/v1/forum/replies/"+reply.ID+"/mark-best", `{}`, "")
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthenticated mark best 401, got %d: %s", unauthorized.Code, unauthorized.Body.String())
	}
	intruderToken := loginTestUser(t, router, "forum-best-intruder@stu.henu.edu.cn")
	forbidden := performJSON(router, http.MethodPost, "/api/v1/forum/replies/"+reply.ID+"/mark-best", `{}`, intruderToken)
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("expected unrelated user mark best 403, got %d: %s", forbidden.Code, forbidden.Body.String())
	}

	markBest := performJSON(router, http.MethodPost, "/api/v1/forum/replies/"+reply.ID+"/mark-best", `{}`, ownerToken)
	if markBest.Code != http.StatusOK {
		t.Fatalf("expected owner mark best 200, got %d: %s", markBest.Code, markBest.Body.String())
	}
	if err := db.First(&reply, "id = ?", reply.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !reply.IsBest {
		t.Fatal("expected reply to be marked best")
	}
	if err := db.First(&rewardPost, "id = ?", rewardPost.ID).Error; err != nil {
		t.Fatal(err)
	}
	if rewardPost.RewardStatus != "settled" {
		t.Fatalf("expected reward status settled, got %s", rewardPost.RewardStatus)
	}
	if err := db.First(&answerer, "id = ?", answerer.ID).Error; err != nil {
		t.Fatal(err)
	}
	if answerer.PointsBalance != 45 {
		t.Fatalf("expected answerer to receive 45 reward points, got %d", answerer.PointsBalance)
	}
	var settlementLogs int64
	if err := db.Model(&model.PointsLog{}).
		Where("user_id = ? AND delta = ? AND balance_after = ? AND reason = ? AND reference_type = ? AND reference_id = ?", answerer.ID, 45, 45, "forum_reward_settlement", "forum_reply", reply.ID).
		Count(&settlementLogs).Error; err != nil {
		t.Fatal(err)
	}
	if settlementLogs != 1 {
		t.Fatalf("expected one reward settlement log, got %d", settlementLogs)
	}
	if countOperationLogs(t, db, "forum_reply.best_selected", "forum_reply", reply.ID, owner.ID) != 1 {
		t.Fatal("expected best answer selection operation log")
	}
	publicDetail := performJSON(router, http.MethodGet, "/api/v1/forum/posts/"+rewardPost.ID, "", "")
	if publicDetail.Code != http.StatusOK ||
		!strings.Contains(publicDetail.Body.String(), `"isBest":true`) ||
		!strings.Contains(publicDetail.Body.String(), `"rewardStatus":"settled"`) {
		t.Fatalf("expected public detail to expose best answer and settled reward, got %d: %s", publicDetail.Code, publicDetail.Body.String())
	}

	repeat := performJSON(router, http.MethodPost, "/api/v1/forum/replies/"+reply.ID+"/mark-best", `{}`, ownerToken)
	if repeat.Code != http.StatusConflict || !strings.Contains(repeat.Body.String(), "best_answer_already_selected") {
		t.Fatalf("expected repeated mark best conflict, got %d: %s", repeat.Code, repeat.Body.String())
	}
	if err := db.Model(&model.PointsLog{}).
		Where("reason = ? AND reference_type = ? AND reference_id = ?", "forum_reward_settlement", "forum_reply", reply.ID).
		Count(&settlementLogs).Error; err != nil {
		t.Fatal(err)
	}
	if settlementLogs != 1 {
		t.Fatalf("expected repeated mark best to avoid duplicate settlement logs, got %d", settlementLogs)
	}
	if err := db.First(&answerer, "id = ?", answerer.ID).Error; err != nil {
		t.Fatal(err)
	}
	if answerer.PointsBalance != 45 {
		t.Fatalf("expected repeated mark best to avoid duplicate reward points, got %d", answerer.PointsBalance)
	}

	normalPost := model.ForumPost{
		ContentStats: model.ContentStats{Visibility: "public"},
		AuthorID:     owner.ID,
		BoardID:      board.ID,
		Title:        "Normal best answer",
		Content:      "This question has no reward points.",
		Type:         "question",
		Status:       model.StatusPublished,
	}
	if err := db.Create(&normalPost).Error; err != nil {
		t.Fatal(err)
	}
	normalReply := model.ForumReply{AuthorID: answerer.ID, PostID: normalPost.ID, Content: "A useful normal answer.", Status: model.StatusPublished}
	if err := db.Create(&normalReply).Error; err != nil {
		t.Fatal(err)
	}
	normalMark := performJSON(router, http.MethodPost, "/api/v1/forum/replies/"+normalReply.ID+"/mark-best", `{}`, ownerToken)
	if normalMark.Code != http.StatusOK {
		t.Fatalf("expected normal post mark best 200, got %d: %s", normalMark.Code, normalMark.Body.String())
	}
	if err := db.First(&normalReply, "id = ?", normalReply.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !normalReply.IsBest {
		t.Fatal("expected normal reply to be marked best")
	}
	if err := db.First(&answerer, "id = ?", answerer.ID).Error; err != nil {
		t.Fatal(err)
	}
	if answerer.PointsBalance != 45 {
		t.Fatalf("expected normal best answer to avoid point changes, got %d", answerer.PointsBalance)
	}
	if countOperationLogs(t, db, "forum_reply.best_selected", "forum_reply", normalReply.ID, owner.ID) != 1 {
		t.Fatal("expected normal best answer operation log")
	}

	refundedPost := model.ForumPost{
		ContentStats: model.ContentStats{Visibility: "public"},
		AuthorID:     owner.ID,
		BoardID:      board.ID,
		Title:        "Already refunded reward",
		Content:      "This reward is no longer settleable.",
		Type:         "reward",
		RewardPoints: 30,
		RewardStatus: "refunded",
		Status:       model.StatusPublished,
	}
	if err := db.Create(&refundedPost).Error; err != nil {
		t.Fatal(err)
	}
	refundedReply := model.ForumReply{AuthorID: answerer.ID, PostID: refundedPost.ID, Content: "Cannot receive refunded reward.", Status: model.StatusPublished}
	if err := db.Create(&refundedReply).Error; err != nil {
		t.Fatal(err)
	}
	refundedMark := performJSON(router, http.MethodPost, "/api/v1/forum/replies/"+refundedReply.ID+"/mark-best", `{}`, ownerToken)
	if refundedMark.Code != http.StatusConflict || !strings.Contains(refundedMark.Body.String(), "reward_not_settleable") {
		t.Fatalf("expected refunded reward mark best conflict, got %d: %s", refundedMark.Code, refundedMark.Body.String())
	}

	ownPost := model.ForumPost{
		ContentStats: model.ContentStats{Visibility: "public"},
		AuthorID:     owner.ID,
		BoardID:      board.ID,
		Title:        "Owner answer target",
		Content:      "Owner should not self-select.",
		Type:         "question",
		Status:       model.StatusPublished,
	}
	if err := db.Create(&ownPost).Error; err != nil {
		t.Fatal(err)
	}
	ownReply := model.ForumReply{AuthorID: owner.ID, PostID: ownPost.ID, Content: "Owner answer.", Status: model.StatusPublished}
	if err := db.Create(&ownReply).Error; err != nil {
		t.Fatal(err)
	}
	selfMark := performJSON(router, http.MethodPost, "/api/v1/forum/replies/"+ownReply.ID+"/mark-best", `{}`, ownerToken)
	if selfMark.Code != http.StatusBadRequest || !strings.Contains(selfMark.Body.String(), "cannot_mark_own_reply") {
		t.Fatalf("expected owner self-answer rejection, got %d: %s", selfMark.Code, selfMark.Body.String())
	}

	pendingReply := model.ForumReply{AuthorID: answerer.ID, PostID: ownPost.ID, Content: "Pending answer.", Status: model.StatusPending}
	if err := db.Create(&pendingReply).Error; err != nil {
		t.Fatal(err)
	}
	pendingMark := performJSON(router, http.MethodPost, "/api/v1/forum/replies/"+pendingReply.ID+"/mark-best", `{}`, ownerToken)
	if pendingMark.Code != http.StatusNotFound || !strings.Contains(pendingMark.Body.String(), "reply_not_found") {
		t.Fatalf("expected pending reply hidden from mark best, got %d: %s", pendingMark.Code, pendingMark.Body.String())
	}
}
