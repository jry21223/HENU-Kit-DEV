package tests

import (
	"net/http"
	"strings"
	"testing"

	"gorm.io/datatypes"

	"final-review-platform/services/api/internal/platform/model"
	"final-review-platform/services/api/internal/server"
	applogger "final-review-platform/services/api/pkg/logger"
)

func TestMyNotificationsUserIsolationAndReadState(t *testing.T) {
	db := newTestDB(t)
	router := server.NewRouter(testConfig(), applogger.New("test"), db, nil)
	owner := createTestUser(t, db, "notify-owner@stu.henu.edu.cn", model.RoleUser)
	other := createTestUser(t, db, "notify-other@stu.henu.edu.cn", model.RoleUser)
	ownerToken := loginTestUser(t, router, owner.Email)
	otherToken := loginTestUser(t, router, other.Email)

	ownerNotification := model.Notification{UserID: owner.ID, Type: "forum_review", Title: "Owner notification", Body: "Only owner can see this."}
	otherNotification := model.Notification{UserID: other.ID, Type: "forum_review", Title: "Other notification", Body: "This belongs to another user."}
	if err := db.Create(&ownerNotification).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&otherNotification).Error; err != nil {
		t.Fatal(err)
	}

	unauthList := performJSON(router, http.MethodGet, "/api/v1/me/notifications", "", "")
	if unauthList.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthenticated notification list 401, got %d: %s", unauthList.Code, unauthList.Body.String())
	}
	invalidLimit := performJSON(router, http.MethodGet, "/api/v1/me/notifications?limit=bad", "", ownerToken)
	if invalidLimit.Code != http.StatusBadRequest || !strings.Contains(invalidLimit.Body.String(), "invalid_limit") {
		t.Fatalf("expected invalid limit rejection, got %d: %s", invalidLimit.Code, invalidLimit.Body.String())
	}

	ownerList := performJSON(router, http.MethodGet, "/api/v1/me/notifications", "", ownerToken)
	if ownerList.Code != http.StatusOK || !strings.Contains(ownerList.Body.String(), ownerNotification.ID) || strings.Contains(ownerList.Body.String(), otherNotification.ID) {
		t.Fatalf("expected owner list to contain only owner notification, got %d: %s", ownerList.Code, ownerList.Body.String())
	}
	if !strings.Contains(ownerList.Body.String(), `"unreadCount":1`) {
		t.Fatalf("expected unread count 1, got %s", ownerList.Body.String())
	}
	otherRead := performJSON(router, http.MethodPost, "/api/v1/me/notifications/"+ownerNotification.ID+"/read", `{}`, otherToken)
	if otherRead.Code != http.StatusNotFound {
		t.Fatalf("expected other user read attempt 404, got %d: %s", otherRead.Code, otherRead.Body.String())
	}
	readOne := performJSON(router, http.MethodPost, "/api/v1/me/notifications/"+ownerNotification.ID+"/read", `{}`, ownerToken)
	if readOne.Code != http.StatusOK || !strings.Contains(readOne.Body.String(), `"read":true`) {
		t.Fatalf("expected mark read success, got %d: %s", readOne.Code, readOne.Body.String())
	}
	readAgain := performJSON(router, http.MethodPost, "/api/v1/me/notifications/"+ownerNotification.ID+"/read", `{}`, ownerToken)
	if readAgain.Code != http.StatusOK {
		t.Fatalf("expected repeat mark read to stay idempotent, got %d: %s", readAgain.Code, readAgain.Body.String())
	}
	unreadOnly := performJSON(router, http.MethodGet, "/api/v1/me/notifications?unread=true", "", ownerToken)
	if unreadOnly.Code != http.StatusOK || strings.Contains(unreadOnly.Body.String(), ownerNotification.ID) || !strings.Contains(unreadOnly.Body.String(), `"unreadCount":0`) {
		t.Fatalf("expected read notification hidden from unread filter, got %d: %s", unreadOnly.Code, unreadOnly.Body.String())
	}

	secondOwnerNotification := model.Notification{UserID: owner.ID, Type: "forum_review", Title: "Second owner notification", Body: "Unread again."}
	if err := db.Create(&secondOwnerNotification).Error; err != nil {
		t.Fatal(err)
	}
	readAll := performJSON(router, http.MethodPost, "/api/v1/me/notifications/read-all", `{}`, ownerToken)
	if readAll.Code != http.StatusOK || !strings.Contains(readAll.Body.String(), `"count":1`) {
		t.Fatalf("expected read-all to mark one unread notification, got %d: %s", readAll.Code, readAll.Body.String())
	}
	otherList := performJSON(router, http.MethodGet, "/api/v1/me/notifications", "", otherToken)
	if otherList.Code != http.StatusOK || !strings.Contains(otherList.Body.String(), otherNotification.ID) || !strings.Contains(otherList.Body.String(), `"unreadCount":1`) {
		t.Fatalf("expected other user's notification to remain unread, got %d: %s", otherList.Code, otherList.Body.String())
	}
}

func TestForumReviewCreatesAuthorNotifications(t *testing.T) {
	db := newTestDB(t)
	router := server.NewRouter(testConfig(), applogger.New("test"), db, nil)
	board := model.ForumBoard{Name: "Notification Review", Slug: "notification-review", Description: "Review notifications", Status: model.StatusPublished}
	if err := db.Create(&board).Error; err != nil {
		t.Fatal(err)
	}
	author := createTestUser(t, db, "notify-forum-author@stu.henu.edu.cn", model.RoleUser)
	reviewer := createTestUser(t, db, "notify-forum-reviewer@stu.henu.edu.cn", model.RoleReviewer)
	authorToken := loginTestUser(t, router, author.Email)
	reviewerToken := loginTestUser(t, router, reviewer.Email)

	post := model.ForumPost{
		ContentStats: model.ContentStats{Visibility: "public"},
		AuthorID:     author.ID,
		BoardID:      board.ID,
		Title:        "Notification target post",
		Content:      "This forum post should notify after review.",
		Type:         "normal",
		Status:       model.StatusPending,
	}
	if err := db.Create(&post).Error; err != nil {
		t.Fatal(err)
	}
	approvePost := performJSON(router, http.MethodPost, "/api/v1/admin/forum/posts/"+post.ID+"/approve", `{"reviewReason":"clear enough"}`, reviewerToken)
	if approvePost.Code != http.StatusOK {
		t.Fatalf("expected forum post approve 200, got %d: %s", approvePost.Code, approvePost.Body.String())
	}
	var postNotifications int64
	if err := db.Model(&model.Notification{}).Where("user_id = ? AND type = ? AND body LIKE ?", author.ID, "forum_review", "%Notification target post%").Count(&postNotifications).Error; err != nil {
		t.Fatal(err)
	}
	if postNotifications != 1 {
		t.Fatalf("expected one post review notification, got %d", postNotifications)
	}
	repeatPostReview := performJSON(router, http.MethodPost, "/api/v1/admin/forum/posts/"+post.ID+"/reject", `{"reviewReason":"repeat"}`, reviewerToken)
	if repeatPostReview.Code != http.StatusConflict {
		t.Fatalf("expected repeated post review conflict, got %d: %s", repeatPostReview.Code, repeatPostReview.Body.String())
	}
	if err := db.Model(&model.Notification{}).Where("user_id = ? AND type = ? AND body LIKE ?", author.ID, "forum_review", "%Notification target post%").Count(&postNotifications).Error; err != nil {
		t.Fatal(err)
	}
	if postNotifications != 1 {
		t.Fatalf("expected repeated review to avoid duplicate notification, got %d", postNotifications)
	}

	reply := model.ForumReply{AuthorID: author.ID, PostID: post.ID, Content: "Reply should notify after rejection.", Status: model.StatusPending}
	if err := db.Create(&reply).Error; err != nil {
		t.Fatal(err)
	}
	rejectReply := performJSON(router, http.MethodPost, "/api/v1/admin/forum/replies/"+reply.ID+"/reject", `{"reviewReason":"needs details"}`, reviewerToken)
	if rejectReply.Code != http.StatusOK {
		t.Fatalf("expected forum reply reject 200, got %d: %s", rejectReply.Code, rejectReply.Body.String())
	}
	notificationList := performJSON(router, http.MethodGet, "/api/v1/me/notifications", "", authorToken)
	if notificationList.Code != http.StatusOK {
		t.Fatalf("expected author notification list 200, got %d: %s", notificationList.Code, notificationList.Body.String())
	}
	body := notificationList.Body.String()
	for _, expected := range []string{"\u8ba8\u8bba\u56de\u590d\u5ba1\u6838\u672a\u901a\u8fc7", "needs details", `"unreadCount":2`, `"resourceType":"forum_reply"`, `"status":"rejected"`} {
		if !strings.Contains(body, expected) {
			t.Fatalf("expected notification response to contain %q, got %s", expected, body)
		}
	}
	if strings.Contains(body, reviewer.ID) {
		t.Fatalf("expected notification response to avoid reviewer id, got %s", body)
	}
}

func TestReviewNotificationsForContentAndAIDrafts(t *testing.T) {
	db := newTestDB(t)
	router := server.NewRouter(testConfig(), applogger.New("test"), db, nil)
	course := createTestCourse(t, db)
	author := createTestUser(t, db, "notify-content-author@stu.henu.edu.cn", model.RoleCreator)
	reviewer := createTestUser(t, db, "notify-content-reviewer@stu.henu.edu.cn", model.RoleReviewer)
	authorToken := loginTestUser(t, router, author.Email)
	reviewerToken := loginTestUser(t, router, reviewer.Email)

	createdBy := author.ID
	material := model.Material{
		CourseID:       course.ID,
		Title:          "Notification material",
		Type:           "knowledge_note",
		StorageKey:     "materials/notification-material.txt",
		FileName:       "notification-material.txt",
		AccessLevel:    model.MaterialAccessFree,
		PreviewContent: "preview",
		Status:         model.StatusPending,
		CreatedBy:      &createdBy,
	}
	if err := db.Create(&material).Error; err != nil {
		t.Fatal(err)
	}
	approveMaterial := performJSON(router, http.MethodPost, "/api/v1/admin/materials/"+material.ID+"/approve", `{"reviewReason":"ready"}`, reviewerToken)
	if approveMaterial.Code != http.StatusOK {
		t.Fatalf("expected material approve 200, got %d: %s", approveMaterial.Code, approveMaterial.Body.String())
	}

	blogPost := model.BlogPost{
		ReviewFields: model.ReviewFields{Status: model.StatusPending},
		ContentStats: model.ContentStats{Visibility: "public"},
		AuthorID:     author.ID,
		Title:        "Notification blog",
		Slug:         "notification-blog",
		Content:      "This pending blog should notify the author.",
	}
	if err := db.Create(&blogPost).Error; err != nil {
		t.Fatal(err)
	}
	rejectBlog := performJSON(router, http.MethodPost, "/api/v1/admin/blog/posts/"+blogPost.ID+"/reject", `{"reviewReason":"needs references"}`, reviewerToken)
	if rejectBlog.Code != http.StatusOK {
		t.Fatalf("expected blog reject 200, got %d: %s", rejectBlog.Code, rejectBlog.Body.String())
	}

	wikiEntry := model.WikiEntry{
		ReviewFields: model.ReviewFields{Status: model.StatusPending},
		ContentStats: model.ContentStats{Visibility: "public"},
		AuthorID:     author.ID,
		CourseID:     &course.ID,
		Title:        "Notification wiki",
		Slug:         "notification-wiki",
		Content:      "This pending wiki should notify the creator.",
		Version:      1,
	}
	if err := db.Create(&wikiEntry).Error; err != nil {
		t.Fatal(err)
	}
	approveWiki := performJSON(router, http.MethodPost, "/api/v1/admin/wiki/entries/"+wikiEntry.ID+"/approve", `{"reviewReason":"useful"}`, reviewerToken)
	if approveWiki.Code != http.StatusOK {
		t.Fatalf("expected wiki approve 200, got %d: %s", approveWiki.Code, approveWiki.Body.String())
	}

	aiTask := model.AITask{
		UserID: &createdBy,
		Type:   "paper_generation",
		Status: model.AITaskCompleted,
		Input:  datatypes.JSON([]byte(`{"topic":"logic"}`)),
		Result: datatypes.JSON([]byte(`{"title":"AI draft"}`)),
	}
	if err := db.Create(&aiTask).Error; err != nil {
		t.Fatal(err)
	}
	aiDraft := model.AIDraft{
		ReviewFields: model.ReviewFields{Status: model.StatusPending},
		TaskID:       aiTask.ID,
		CourseID:     &course.ID,
		OutputType:   "paper_generation",
		DraftContent: datatypes.JSON([]byte(`{"title":"AI draft"}`)),
	}
	if err := db.Create(&aiDraft).Error; err != nil {
		t.Fatal(err)
	}
	rejectDraft := performJSON(router, http.MethodPost, "/api/v1/admin/ai/drafts/"+aiDraft.ID+"/reject", `{"reviewReason":"answer is incomplete"}`, reviewerToken)
	if rejectDraft.Code != http.StatusOK {
		t.Fatalf("expected AI draft reject 200, got %d: %s", rejectDraft.Code, rejectDraft.Body.String())
	}

	list := performJSON(router, http.MethodGet, "/api/v1/me/notifications", "", authorToken)
	if list.Code != http.StatusOK {
		t.Fatalf("expected author notifications 200, got %d: %s", list.Code, list.Body.String())
	}
	body := list.Body.String()
	for _, expected := range []string{
		`"unreadCount":4`,
		`"type":"content_review"`,
		`"resourceType":"material"`,
		`"resourceType":"blog_post"`,
		`"resourceType":"wiki_entry"`,
		`"resourceType":"ai_draft"`,
		"Notification material",
		"Notification blog",
		"Notification wiki",
		"paper_generation",
		"needs references",
		"answer is incomplete",
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("expected notification response to contain %q, got %s", expected, body)
		}
	}
	if strings.Contains(body, reviewer.ID) {
		t.Fatalf("expected content review notifications to avoid reviewer id, got %s", body)
	}
}
