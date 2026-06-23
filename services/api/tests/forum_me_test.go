package tests

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"final-review-platform/services/api/internal/platform/model"
	"final-review-platform/services/api/internal/server"
	applogger "final-review-platform/services/api/pkg/logger"
)

func TestMyForumSubmissionEndpointsAreUserScoped(t *testing.T) {
	db := newTestDB(t)
	router := server.NewRouter(testConfig(), applogger.New("test"), db, nil)
	board := model.ForumBoard{Name: "My Discussion", Slug: "my-discussion", Description: "Track my forum submissions", Status: model.StatusPublished}
	if err := db.Create(&board).Error; err != nil {
		t.Fatal(err)
	}
	owner := createTestUser(t, db, "forum-me-owner@stu.henu.edu.cn", model.RoleUser)
	other := createTestUser(t, db, "forum-me-other@stu.henu.edu.cn", model.RoleUser)
	ownerToken := loginTestUser(t, router, owner.Email)
	otherToken := loginTestUser(t, router, other.Email)

	publishedPost := model.ForumPost{
		ContentStats: model.ContentStats{Visibility: "public"},
		AuthorID:     owner.ID,
		BoardID:      board.ID,
		Title:        "Owner published question",
		Content:      "This approved question is visible publicly.",
		Type:         "question",
		Status:       model.StatusPublished,
		ReviewReason: "approved by reviewer",
	}
	pendingPost := model.ForumPost{
		ContentStats: model.ContentStats{Visibility: "public"},
		AuthorID:     owner.ID,
		BoardID:      board.ID,
		Title:        "Owner pending question",
		Content:      "This question is waiting for review.",
		Type:         "normal",
		Status:       model.StatusPending,
	}
	rejectedPost := model.ForumPost{
		ContentStats: model.ContentStats{Visibility: "public"},
		AuthorID:     owner.ID,
		BoardID:      board.ID,
		Title:        "Owner rejected question",
		Content:      "This question was rejected.",
		Type:         "normal",
		Status:       model.StatusRejected,
		ReviewReason: "missing context",
	}
	otherPost := model.ForumPost{
		ContentStats: model.ContentStats{Visibility: "public"},
		AuthorID:     other.ID,
		BoardID:      board.ID,
		Title:        "Other user question",
		Content:      "Another user owns this row.",
		Type:         "normal",
		Status:       model.StatusPublished,
	}
	for _, post := range []*model.ForumPost{&publishedPost, &pendingPost, &rejectedPost, &otherPost} {
		if err := db.Create(post).Error; err != nil {
			t.Fatal(err)
		}
	}

	pendingReply := model.ForumReply{AuthorID: owner.ID, PostID: publishedPost.ID, Content: "Owner pending reply", Status: model.StatusPending}
	rejectedReply := model.ForumReply{AuthorID: owner.ID, PostID: otherPost.ID, Content: "Owner rejected reply", Status: model.StatusRejected, ReviewReason: "not specific enough"}
	publishedReply := model.ForumReply{AuthorID: owner.ID, PostID: publishedPost.ID, Content: "Owner published reply", Status: model.StatusPublished, IsBest: true}
	otherReply := model.ForumReply{AuthorID: other.ID, PostID: publishedPost.ID, Content: "Other user reply", Status: model.StatusPublished}
	for _, reply := range []*model.ForumReply{&pendingReply, &rejectedReply, &publishedReply, &otherReply} {
		if err := db.Create(reply).Error; err != nil {
			t.Fatal(err)
		}
	}

	unauthPosts := performJSON(router, http.MethodGet, "/api/v1/me/forum-posts", "", "")
	if unauthPosts.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthenticated my forum posts 401, got %d: %s", unauthPosts.Code, unauthPosts.Body.String())
	}
	unauthReplies := performJSON(router, http.MethodGet, "/api/v1/me/forum-replies", "", "")
	if unauthReplies.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthenticated my forum replies 401, got %d: %s", unauthReplies.Code, unauthReplies.Body.String())
	}
	invalidLimit := performJSON(router, http.MethodGet, "/api/v1/me/forum-posts?limit=bad", "", ownerToken)
	if invalidLimit.Code != http.StatusBadRequest || !strings.Contains(invalidLimit.Body.String(), "invalid_limit") {
		t.Fatalf("expected invalid limit rejection, got %d: %s", invalidLimit.Code, invalidLimit.Body.String())
	}

	ownerPosts := performJSON(router, http.MethodGet, "/api/v1/me/forum-posts", "", ownerToken)
	if ownerPosts.Code != http.StatusOK {
		t.Fatalf("expected owner forum posts 200, got %d: %s", ownerPosts.Code, ownerPosts.Body.String())
	}
	ownerPostsBody := ownerPosts.Body.String()
	for _, expected := range []string{publishedPost.ID, pendingPost.ID, rejectedPost.ID, `"status":"pending"`, `"status":"rejected"`, `"reviewReason":"missing context"`} {
		if !strings.Contains(ownerPostsBody, expected) {
			t.Fatalf("expected owner posts response to contain %q, got %s", expected, ownerPostsBody)
		}
	}
	for _, forbidden := range []string{otherPost.ID, "reviewerId", "reviewedAt"} {
		if strings.Contains(ownerPostsBody, forbidden) {
			t.Fatalf("expected owner posts response to hide %q, got %s", forbidden, ownerPostsBody)
		}
	}

	otherPosts := performJSON(router, http.MethodGet, "/api/v1/me/forum-posts", "", otherToken)
	if otherPosts.Code != http.StatusOK || strings.Contains(otherPosts.Body.String(), pendingPost.ID) || !strings.Contains(otherPosts.Body.String(), otherPost.ID) {
		t.Fatalf("expected other user to see only own posts, got %d: %s", otherPosts.Code, otherPosts.Body.String())
	}

	ownerReplies := performJSON(router, http.MethodGet, "/api/v1/me/forum-replies", "", ownerToken)
	if ownerReplies.Code != http.StatusOK {
		t.Fatalf("expected owner forum replies 200, got %d: %s", ownerReplies.Code, ownerReplies.Body.String())
	}
	ownerRepliesBody := ownerReplies.Body.String()
	for _, expected := range []string{pendingReply.ID, rejectedReply.ID, publishedReply.ID, `"postTitle":"Owner published question"`, `"postStatus":"published"`, `"reviewReason":"not specific enough"`} {
		if !strings.Contains(ownerRepliesBody, expected) {
			t.Fatalf("expected owner replies response to contain %q, got %s", expected, ownerRepliesBody)
		}
	}
	for _, forbidden := range []string{otherReply.ID, "reviewerId", "reviewedAt"} {
		if strings.Contains(ownerRepliesBody, forbidden) {
			t.Fatalf("expected owner replies response to hide %q, got %s", forbidden, ownerRepliesBody)
		}
	}

	publicList := performJSON(router, http.MethodGet, "/api/v1/forum/posts", "", "")
	if publicList.Code != http.StatusOK {
		t.Fatalf("expected public forum list 200, got %d: %s", publicList.Code, publicList.Body.String())
	}
	publicListBody := publicList.Body.String()
	if strings.Contains(publicListBody, pendingPost.ID) || strings.Contains(publicListBody, rejectedPost.ID) {
		t.Fatalf("expected public list to hide pending/rejected posts, got %s", publicListBody)
	}
	publicDetail := performJSON(router, http.MethodGet, "/api/v1/forum/posts/"+publishedPost.ID, "", "")
	if publicDetail.Code != http.StatusOK {
		t.Fatalf("expected public detail 200, got %d: %s", publicDetail.Code, publicDetail.Body.String())
	}
	publicDetailBody := publicDetail.Body.String()
	if strings.Contains(publicDetailBody, pendingReply.ID) || strings.Contains(publicDetailBody, rejectedReply.ID) || strings.Contains(publicDetailBody, "reviewReason") {
		t.Fatalf("expected public detail to hide pending/rejected reply and review metadata, got %s", publicDetailBody)
	}
}

func TestMyForumResubmitWorkflow(t *testing.T) {
	db := newTestDB(t)
	router := server.NewRouter(testConfig(), applogger.New("test"), db, nil)
	board := model.ForumBoard{Name: "Resubmit Discussion", Slug: "resubmit-discussion", Description: "Resubmit review flow", Status: model.StatusPublished}
	if err := db.Create(&board).Error; err != nil {
		t.Fatal(err)
	}
	owner := createTestUser(t, db, "forum-resubmit-owner@stu.henu.edu.cn", model.RoleUser)
	other := createTestUser(t, db, "forum-resubmit-other@stu.henu.edu.cn", model.RoleUser)
	reviewer := createTestUser(t, db, "forum-resubmit-reviewer@stu.henu.edu.cn", model.RoleReviewer)
	ownerToken := loginTestUser(t, router, owner.Email)
	otherToken := loginTestUser(t, router, other.Email)
	reviewerID := reviewer.ID
	reviewedAt := time.Now().UTC()

	if err := db.Model(&model.User{}).Where("id = ?", owner.ID).Update("points_balance", 70).Error; err != nil {
		t.Fatal(err)
	}

	rejectedPost := model.ForumPost{
		ContentStats: model.ContentStats{Visibility: "public"},
		AuthorID:     owner.ID,
		BoardID:      board.ID,
		Title:        "Rejected post",
		Content:      "Thin post",
		Type:         "normal",
		Status:       model.StatusRejected,
		ReviewerID:   &reviewerID,
		ReviewedAt:   &reviewedAt,
		ReviewReason: "missing details",
	}
	publishedPost := model.ForumPost{
		ContentStats: model.ContentStats{Visibility: "public"},
		AuthorID:     owner.ID,
		BoardID:      board.ID,
		Title:        "Published post",
		Content:      "Already public.",
		Type:         "normal",
		Status:       model.StatusPublished,
	}
	otherRejectedPost := model.ForumPost{
		ContentStats: model.ContentStats{Visibility: "public"},
		AuthorID:     other.ID,
		BoardID:      board.ID,
		Title:        "Other rejected post",
		Content:      "Other user content.",
		Type:         "normal",
		Status:       model.StatusRejected,
	}
	rewardPost := model.ForumPost{
		ContentStats: model.ContentStats{Visibility: "public"},
		AuthorID:     owner.ID,
		BoardID:      board.ID,
		Title:        "Rejected reward post",
		Content:      "Reward content needs detail.",
		Type:         "reward",
		RewardPoints: 30,
		RewardStatus: "refunded",
		Status:       model.StatusRejected,
		ReviewerID:   &reviewerID,
		ReviewedAt:   &reviewedAt,
		ReviewReason: "unclear reward request",
	}
	expensiveRewardPost := model.ForumPost{
		ContentStats: model.ContentStats{Visibility: "public"},
		AuthorID:     owner.ID,
		BoardID:      board.ID,
		Title:        "Expensive rejected reward post",
		Content:      "This one costs too much to resubmit.",
		Type:         "reward",
		RewardPoints: 100,
		RewardStatus: "refunded",
		Status:       model.StatusRejected,
	}
	for _, post := range []*model.ForumPost{&rejectedPost, &publishedPost, &otherRejectedPost, &rewardPost, &expensiveRewardPost} {
		if err := db.Create(post).Error; err != nil {
			t.Fatal(err)
		}
	}

	rejectedReply := model.ForumReply{
		AuthorID:     owner.ID,
		PostID:       publishedPost.ID,
		Content:      "Rejected reply",
		Status:       model.StatusRejected,
		ReviewerID:   &reviewerID,
		ReviewedAt:   &reviewedAt,
		ReviewReason: "not constructive",
	}
	publishedReply := model.ForumReply{AuthorID: owner.ID, PostID: publishedPost.ID, Content: "Published reply", Status: model.StatusPublished}
	otherRejectedReply := model.ForumReply{AuthorID: other.ID, PostID: publishedPost.ID, Content: "Other rejected reply", Status: model.StatusRejected}
	for _, reply := range []*model.ForumReply{&rejectedReply, &publishedReply, &otherRejectedReply} {
		if err := db.Create(reply).Error; err != nil {
			t.Fatal(err)
		}
	}

	unauthPost := performJSON(router, http.MethodPatch, "/api/v1/me/forum-posts/"+rejectedPost.ID, `{"title":"x","content":"y"}`, "")
	if unauthPost.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthenticated post resubmit 401, got %d: %s", unauthPost.Code, unauthPost.Body.String())
	}
	otherPost := performJSON(router, http.MethodPatch, "/api/v1/me/forum-posts/"+rejectedPost.ID, `{"title":"x","content":"y"}`, otherToken)
	if otherPost.Code != http.StatusNotFound {
		t.Fatalf("expected other user post resubmit 404, got %d: %s", otherPost.Code, otherPost.Body.String())
	}
	invalidPost := performJSON(router, http.MethodPatch, "/api/v1/me/forum-posts/"+rejectedPost.ID, `{"title":" ","content":"More context"}`, ownerToken)
	if invalidPost.Code != http.StatusBadRequest || !strings.Contains(invalidPost.Body.String(), "missing_required_fields") {
		t.Fatalf("expected invalid post resubmit rejection, got %d: %s", invalidPost.Code, invalidPost.Body.String())
	}
	publishedPostEdit := performJSON(router, http.MethodPatch, "/api/v1/me/forum-posts/"+publishedPost.ID, `{"title":"Edit public","content":"Nope"}`, ownerToken)
	if publishedPostEdit.Code != http.StatusConflict || !strings.Contains(publishedPostEdit.Body.String(), "forum_post_not_editable") {
		t.Fatalf("expected published post edit conflict, got %d: %s", publishedPostEdit.Code, publishedPostEdit.Body.String())
	}

	resubmitPost := performJSON(router, http.MethodPatch, "/api/v1/me/forum-posts/"+rejectedPost.ID, `{"title":"Resubmitted post","content":"I added course, chapter, and the exact stuck point."}`, ownerToken)
	if resubmitPost.Code != http.StatusOK || !strings.Contains(resubmitPost.Body.String(), `"status":"pending"`) || strings.Contains(resubmitPost.Body.String(), "missing details") {
		t.Fatalf("expected rejected post to return pending without stale reason, got %d: %s", resubmitPost.Code, resubmitPost.Body.String())
	}
	var updatedPost model.ForumPost
	if err := db.First(&updatedPost, "id = ?", rejectedPost.ID).Error; err != nil {
		t.Fatal(err)
	}
	if updatedPost.Status != model.StatusPending || updatedPost.Title != "Resubmitted post" || updatedPost.ReviewReason != "" || updatedPost.ReviewerID != nil || updatedPost.ReviewedAt != nil {
		t.Fatalf("expected post resubmitted pending with cleared review metadata, got %#v", updatedPost)
	}
	publicPost := performJSON(router, http.MethodGet, "/api/v1/forum/posts/"+rejectedPost.ID, "", "")
	if publicPost.Code != http.StatusNotFound {
		t.Fatalf("expected resubmitted pending post hidden publicly, got %d: %s", publicPost.Code, publicPost.Body.String())
	}

	resubmitReward := performJSON(router, http.MethodPatch, "/api/v1/me/forum-posts/"+rewardPost.ID, `{"title":"Resubmitted reward post","content":"I clarified the expected proof format and reward conditions."}`, ownerToken)
	if resubmitReward.Code != http.StatusOK || !strings.Contains(resubmitReward.Body.String(), `"rewardStatus":"escrowed"`) {
		t.Fatalf("expected reward post re-escrow on resubmit, got %d: %s", resubmitReward.Code, resubmitReward.Body.String())
	}
	var ownerAfterReward model.User
	if err := db.First(&ownerAfterReward, "id = ?", owner.ID).Error; err != nil {
		t.Fatal(err)
	}
	if ownerAfterReward.PointsBalance != 40 {
		t.Fatalf("expected owner balance 40 after reward re-escrow, got %d", ownerAfterReward.PointsBalance)
	}
	var reescrowLogs int64
	if err := db.Model(&model.PointsLog{}).Where("user_id = ? AND delta = ? AND balance_after = ? AND reason = ? AND reference_type = ? AND reference_id = ?", owner.ID, -30, 40, "forum_reward_reescrow", "forum_post", rewardPost.ID).Count(&reescrowLogs).Error; err != nil {
		t.Fatal(err)
	}
	if reescrowLogs != 1 {
		t.Fatalf("expected one reward re-escrow log, got %d", reescrowLogs)
	}

	insufficientReward := performJSON(router, http.MethodPatch, "/api/v1/me/forum-posts/"+expensiveRewardPost.ID, `{"title":"Still expensive","content":"More detail but not enough points."}`, ownerToken)
	if insufficientReward.Code != http.StatusBadRequest || !strings.Contains(insufficientReward.Body.String(), "insufficient_points") {
		t.Fatalf("expected insufficient points on reward resubmit, got %d: %s", insufficientReward.Code, insufficientReward.Body.String())
	}
	var expensiveAfter model.ForumPost
	if err := db.First(&expensiveAfter, "id = ?", expensiveRewardPost.ID).Error; err != nil {
		t.Fatal(err)
	}
	if expensiveAfter.Status != model.StatusRejected || expensiveAfter.RewardStatus != "refunded" {
		t.Fatalf("expected failed reward resubmit to keep rejected/refunded, got %#v", expensiveAfter)
	}

	unauthReply := performJSON(router, http.MethodPatch, "/api/v1/me/forum-replies/"+rejectedReply.ID, `{"content":"updated"}`, "")
	if unauthReply.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthenticated reply resubmit 401, got %d: %s", unauthReply.Code, unauthReply.Body.String())
	}
	otherReply := performJSON(router, http.MethodPatch, "/api/v1/me/forum-replies/"+rejectedReply.ID, `{"content":"updated"}`, otherToken)
	if otherReply.Code != http.StatusNotFound {
		t.Fatalf("expected other user reply resubmit 404, got %d: %s", otherReply.Code, otherReply.Body.String())
	}
	publishedReplyEdit := performJSON(router, http.MethodPatch, "/api/v1/me/forum-replies/"+publishedReply.ID, `{"content":"No edit"}`, ownerToken)
	if publishedReplyEdit.Code != http.StatusConflict || !strings.Contains(publishedReplyEdit.Body.String(), "forum_reply_not_editable") {
		t.Fatalf("expected published reply edit conflict, got %d: %s", publishedReplyEdit.Code, publishedReplyEdit.Body.String())
	}
	resubmitReply := performJSON(router, http.MethodPatch, "/api/v1/me/forum-replies/"+rejectedReply.ID, `{"content":"I rewrote the answer with concrete steps."}`, ownerToken)
	if resubmitReply.Code != http.StatusOK || !strings.Contains(resubmitReply.Body.String(), `"status":"pending"`) || strings.Contains(resubmitReply.Body.String(), "not constructive") {
		t.Fatalf("expected rejected reply to return pending without stale reason, got %d: %s", resubmitReply.Code, resubmitReply.Body.String())
	}
	var updatedReply model.ForumReply
	if err := db.First(&updatedReply, "id = ?", rejectedReply.ID).Error; err != nil {
		t.Fatal(err)
	}
	if updatedReply.Status != model.StatusPending || updatedReply.Content != "I rewrote the answer with concrete steps." || updatedReply.ReviewReason != "" || updatedReply.ReviewerID != nil || updatedReply.ReviewedAt != nil {
		t.Fatalf("expected reply resubmitted pending with cleared review metadata, got %#v", updatedReply)
	}
	publicDetail := performJSON(router, http.MethodGet, "/api/v1/forum/posts/"+publishedPost.ID, "", "")
	if publicDetail.Code != http.StatusOK || strings.Contains(publicDetail.Body.String(), rejectedReply.ID) {
		t.Fatalf("expected resubmitted pending reply hidden publicly, got %d: %s", publicDetail.Code, publicDetail.Body.String())
	}

	if err := db.Model(&model.User{}).Where("id = ?", owner.ID).Update("status", "frozen").Error; err != nil {
		t.Fatal(err)
	}
	frozenResubmit := performJSON(router, http.MethodPatch, "/api/v1/me/forum-posts/"+otherRejectedPost.ID, `{"title":"Frozen","content":"Frozen user cannot edit."}`, ownerToken)
	if frozenResubmit.Code != http.StatusForbidden || !strings.Contains(frozenResubmit.Body.String(), "user_frozen") {
		t.Fatalf("expected frozen user resubmit 403, got %d: %s", frozenResubmit.Code, frozenResubmit.Body.String())
	}
}
