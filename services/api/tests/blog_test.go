package tests

import (
	"net/http"
	"strings"
	"testing"

	"final-review-platform/services/api/internal/platform/model"
	"final-review-platform/services/api/internal/server"
	applogger "final-review-platform/services/api/pkg/logger"
)

func TestBlogSubmissionAndReviewWorkflow(t *testing.T) {
	db := newTestDB(t)
	router := server.NewRouter(testConfig(), applogger.New("test"), db, nil)

	unauthorizedCreate := performJSON(router, http.MethodPost, "/api/v1/blog/posts", `{"title":"No auth","slug":"no-auth","content":"body"}`, "")
	if unauthorizedCreate.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthenticated blog create 401, got %d: %s", unauthorizedCreate.Code, unauthorizedCreate.Body.String())
	}

	studentToken := loginTestUser(t, router, "blog-author@stu.henu.edu.cn")
	invalidSlug := performJSON(router, http.MethodPost, "/api/v1/blog/posts", `{"title":"Bad slug","slug":"Bad_Slug","content":"body"}`, studentToken)
	if invalidSlug.Code != http.StatusBadRequest || !strings.Contains(invalidSlug.Body.String(), "invalid_slug") {
		t.Fatalf("expected invalid slug rejection, got %d: %s", invalidSlug.Code, invalidSlug.Body.String())
	}

	createBody := `{"title":"离散数学复习顺序","slug":"discrete-review-order","content":"先过定义，再做关系和图论综合题。"}`
	createResponse := performJSON(router, http.MethodPost, "/api/v1/blog/posts", createBody, studentToken)
	if createResponse.Code != http.StatusOK {
		t.Fatalf("expected blog create 200, got %d: %s", createResponse.Code, createResponse.Body.String())
	}
	var post model.BlogPost
	if err := db.First(&post, "slug = ?", "discrete-review-order").Error; err != nil {
		t.Fatal(err)
	}
	if post.Status != model.StatusPending || post.AuthorID == "" {
		t.Fatalf("expected pending post with author, got %#v", post)
	}

	publicList := performJSON(router, http.MethodGet, "/api/v1/blog/posts", "", "")
	if publicList.Code != http.StatusOK || strings.Contains(publicList.Body.String(), post.ID) {
		t.Fatalf("expected pending blog hidden from public list, got %d: %s", publicList.Code, publicList.Body.String())
	}
	publicDetail := performJSON(router, http.MethodGet, "/api/v1/blog/posts/"+post.ID, "", "")
	if publicDetail.Code != http.StatusNotFound {
		t.Fatalf("expected pending blog public detail 404, got %d: %s", publicDetail.Code, publicDetail.Body.String())
	}

	forbiddenReviewList := performJSON(router, http.MethodGet, "/api/v1/admin/blog/posts", "", studentToken)
	if forbiddenReviewList.Code != http.StatusForbidden {
		t.Fatalf("expected student blog review list 403, got %d: %s", forbiddenReviewList.Code, forbiddenReviewList.Body.String())
	}
	forbiddenApprove := performJSON(router, http.MethodPost, "/api/v1/admin/blog/posts/"+post.ID+"/approve", `{"reviewReason":"ok"}`, studentToken)
	if forbiddenApprove.Code != http.StatusForbidden {
		t.Fatalf("expected student blog approve 403, got %d: %s", forbiddenApprove.Code, forbiddenApprove.Body.String())
	}

	reviewer := createTestUser(t, db, "blog-reviewer@stu.henu.edu.cn", model.RoleReviewer)
	reviewerToken := loginTestUser(t, router, "blog-reviewer@stu.henu.edu.cn")
	reviewList := performJSON(router, http.MethodGet, "/api/v1/admin/blog/posts", "", reviewerToken)
	if reviewList.Code != http.StatusOK || !strings.Contains(reviewList.Body.String(), post.ID) {
		t.Fatalf("expected reviewer blog list to include pending post, got %d: %s", reviewList.Code, reviewList.Body.String())
	}
	rejectWithoutReason := performJSON(router, http.MethodPost, "/api/v1/admin/blog/posts/"+post.ID+"/reject", "", reviewerToken)
	if rejectWithoutReason.Code != http.StatusBadRequest || !strings.Contains(rejectWithoutReason.Body.String(), "review_reason_required") {
		t.Fatalf("expected reject reason required, got %d: %s", rejectWithoutReason.Code, rejectWithoutReason.Body.String())
	}

	approve := performJSON(router, http.MethodPost, "/api/v1/admin/blog/posts/"+post.ID+"/approve", `{"reviewReason":"内容真实"}`, reviewerToken)
	if approve.Code != http.StatusOK {
		t.Fatalf("expected reviewer blog approve 200, got %d: %s", approve.Code, approve.Body.String())
	}
	var approved model.BlogPost
	if err := db.First(&approved, "id = ?", post.ID).Error; err != nil {
		t.Fatal(err)
	}
	if approved.Status != model.StatusPublished || approved.ReviewerID == nil || *approved.ReviewerID != reviewer.ID || approved.ReviewedAt == nil || approved.ReviewReason != "内容真实" {
		t.Fatalf("expected approved blog review metadata, got %#v", approved)
	}
	if countOperationLogs(t, db, "blog_post.published", "blog_post", post.ID, reviewer.ID) != 1 {
		t.Fatal("expected blog approval operation log")
	}

	publicListAfterApprove := performJSON(router, http.MethodGet, "/api/v1/blog/posts", "", "")
	if publicListAfterApprove.Code != http.StatusOK || !strings.Contains(publicListAfterApprove.Body.String(), post.ID) {
		t.Fatalf("expected approved blog in public list, got %d: %s", publicListAfterApprove.Code, publicListAfterApprove.Body.String())
	}
	reviewAgain := performJSON(router, http.MethodPost, "/api/v1/admin/blog/posts/"+post.ID+"/reject", `{"reviewReason":"overwrite"}`, reviewerToken)
	if reviewAgain.Code != http.StatusConflict || !strings.Contains(reviewAgain.Body.String(), "post_not_reviewable") {
		t.Fatalf("expected reviewed blog conflict, got %d: %s", reviewAgain.Code, reviewAgain.Body.String())
	}
	if countOperationLogs(t, db, "blog_post.published", "blog_post", post.ID, reviewer.ID) != 1 {
		t.Fatal("expected repeated blog review to avoid duplicate approval logs")
	}

	secondCreate := performJSON(router, http.MethodPost, "/api/v1/blog/posts", `{"title":"概率论错题复盘","slug":"probability-wrong-review","content":"把条件概率和随机变量函数分开复习。"}`, studentToken)
	if secondCreate.Code != http.StatusOK {
		t.Fatalf("expected second blog create 200, got %d: %s", secondCreate.Code, secondCreate.Body.String())
	}
	var rejectedPost model.BlogPost
	if err := db.First(&rejectedPost, "slug = ?", "probability-wrong-review").Error; err != nil {
		t.Fatal(err)
	}
	admin := createTestUser(t, db, "blog-admin@stu.henu.edu.cn", model.RoleAdmin)
	adminToken := loginTestUser(t, router, "blog-admin@stu.henu.edu.cn")
	reject := performJSON(router, http.MethodPost, "/api/v1/admin/blog/posts/"+rejectedPost.ID+"/reject", `{"reviewReason":"缺少具体例题"}`, adminToken)
	if reject.Code != http.StatusOK {
		t.Fatalf("expected admin blog reject 200, got %d: %s", reject.Code, reject.Body.String())
	}
	var rejected model.BlogPost
	if err := db.First(&rejected, "id = ?", rejectedPost.ID).Error; err != nil {
		t.Fatal(err)
	}
	if rejected.Status != model.StatusRejected || rejected.ReviewerID == nil || *rejected.ReviewerID != admin.ID || rejected.ReviewReason != "缺少具体例题" {
		t.Fatalf("expected rejected blog review metadata, got %#v", rejected)
	}
	if countOperationLogs(t, db, "blog_post.rejected", "blog_post", rejectedPost.ID, admin.ID) != 1 {
		t.Fatal("expected blog rejection operation log")
	}
	publicRejected := performJSON(router, http.MethodGet, "/api/v1/blog/posts/"+rejectedPost.ID, "", "")
	if publicRejected.Code != http.StatusNotFound {
		t.Fatalf("expected rejected blog hidden from public detail, got %d: %s", publicRejected.Code, publicRejected.Body.String())
	}

	frozenCreate := performJSON(router, http.MethodPost, "/api/v1/blog/posts", `{"title":"Frozen target","slug":"frozen-target","content":"body"}`, studentToken)
	if frozenCreate.Code != http.StatusOK {
		t.Fatalf("expected frozen target create 200, got %d: %s", frozenCreate.Code, frozenCreate.Body.String())
	}
	var frozenTarget model.BlogPost
	if err := db.First(&frozenTarget, "slug = ?", "frozen-target").Error; err != nil {
		t.Fatal(err)
	}
	frozenReviewer := createTestUser(t, db, "frozen-reviewer@stu.henu.edu.cn", model.RoleReviewer)
	frozenReviewerToken := loginTestUser(t, router, "frozen-reviewer@stu.henu.edu.cn")
	if err := db.Model(&model.User{}).Where("id = ?", frozenReviewer.ID).Update("status", "frozen").Error; err != nil {
		t.Fatal(err)
	}
	frozenReview := performJSON(router, http.MethodPost, "/api/v1/admin/blog/posts/"+frozenTarget.ID+"/approve", `{"reviewReason":"ok"}`, frozenReviewerToken)
	if frozenReview.Code != http.StatusForbidden || !strings.Contains(frozenReview.Body.String(), "user_frozen") {
		t.Fatalf("expected frozen reviewer blog approve 403, got %d: %s", frozenReview.Code, frozenReview.Body.String())
	}
	if countOperationLogs(t, db, "blog_post.published", "blog_post", frozenTarget.ID, frozenReviewer.ID) != 0 {
		t.Fatal("expected frozen reviewer to avoid blog approval logs")
	}
}
