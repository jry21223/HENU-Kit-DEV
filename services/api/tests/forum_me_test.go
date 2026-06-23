package tests

import (
	"net/http"
	"strings"
	"testing"

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
