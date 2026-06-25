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

func TestPublicUserProfileAggregation(t *testing.T) {
	db := newTestDB(t)
	router := server.NewRouter(testConfig(), applogger.New("test"), db, nil)

	aliceToken := loginTestUser(t, router, "profile-alice@stu.henu.edu.cn")
	bobToken := loginTestUser(t, router, "profile-bob@stu.henu.edu.cn")

	var alice model.User
	if err := db.First(&alice, "email = ?", "profile-alice@stu.henu.edu.cn").Error; err != nil {
		t.Fatal(err)
	}
	var bob model.User
	if err := db.First(&bob, "email = ?", "profile-bob@stu.henu.edu.cn").Error; err != nil {
		t.Fatal(err)
	}

	publicMoment := performJSON(router, http.MethodPost, "/api/v1/moments", `{"content":"Profile public moment"}`, aliceToken)
	if publicMoment.Code != http.StatusOK {
		t.Fatalf("expected public moment create, got %d: %s", publicMoment.Code, publicMoment.Body.String())
	}
	var savedPublicMoment model.Moment
	if err := db.First(&savedPublicMoment, "author_id = ? AND content = ?", alice.ID, "Profile public moment").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&savedPublicMoment).Update("images", datatypes.JSON([]byte(`[
		"/api/v1/moments/images/profile-safe-image",
		"/uploads/moments/private.png",
		"javascript:alert(1)",
		"https://tracker.example/pixel.png",
		"/api/v1/moments/images/profile-safe-image/../secret"
	]`))).Error; err != nil {
		t.Fatal(err)
	}
	mutualMoment := performJSON(router, http.MethodPost, "/api/v1/moments", `{"content":"Profile mutual moment","visibility":"mutual_friends"}`, aliceToken)
	if mutualMoment.Code != http.StatusOK {
		t.Fatalf("expected mutual moment create, got %d: %s", mutualMoment.Code, mutualMoment.Body.String())
	}

	publishedBlog := model.BlogPost{
		ReviewFields: model.ReviewFields{Status: model.StatusPublished},
		ContentStats: model.ContentStats{Visibility: "public"},
		AuthorID:     alice.ID,
		Title:        "Published profile blog",
		Slug:         "published-profile-blog",
		Content:      "Visible blog body",
	}
	if err := db.Create(&publishedBlog).Error; err != nil {
		t.Fatal(err)
	}
	hiddenBlog := model.BlogPost{
		ReviewFields: model.ReviewFields{Status: model.StatusPending, ReviewReason: "hidden reason"},
		ContentStats: model.ContentStats{Visibility: "public"},
		AuthorID:     alice.ID,
		Title:        "Hidden profile blog",
		Slug:         "hidden-profile-blog",
		Content:      "Hidden blog body",
	}
	if err := db.Create(&hiddenBlog).Error; err != nil {
		t.Fatal(err)
	}
	board := model.ForumBoard{Name: "Profile Board", Slug: "profile-board", Status: model.StatusPublished}
	if err := db.Create(&board).Error; err != nil {
		t.Fatal(err)
	}
	forumPost := model.ForumPost{
		ContentStats: model.ContentStats{Visibility: "public"},
		AuthorID:     alice.ID,
		BoardID:      board.ID,
		Title:        "Published profile forum",
		Content:      "Visible forum body",
		Type:         "normal",
		Status:       model.StatusPublished,
	}
	if err := db.Create(&forumPost).Error; err != nil {
		t.Fatal(err)
	}
	pendingForum := model.ForumPost{
		ContentStats: model.ContentStats{Visibility: "public"},
		AuthorID:     alice.ID,
		BoardID:      board.ID,
		Title:        "Hidden profile forum",
		Content:      "Hidden forum body",
		Type:         "normal",
		Status:       model.StatusPending,
		ReviewReason: "hidden forum reason",
	}
	if err := db.Create(&pendingForum).Error; err != nil {
		t.Fatal(err)
	}
	publishedReply := model.ForumReply{AuthorID: alice.ID, PostID: forumPost.ID, Content: "Visible profile reply", Status: model.StatusPublished}
	if err := db.Create(&publishedReply).Error; err != nil {
		t.Fatal(err)
	}

	anonymousProfile := performJSON(router, http.MethodGet, "/api/v1/users/"+alice.ID, "", "")
	if anonymousProfile.Code != http.StatusOK {
		t.Fatalf("expected anonymous profile 200, got %d: %s", anonymousProfile.Code, anonymousProfile.Body.String())
	}
	anonymousBody := anonymousProfile.Body.String()
	for _, expected := range []string{alice.ID, "Profile public moment", "Published profile blog", "Published profile forum", "Visible profile reply"} {
		if !strings.Contains(anonymousBody, expected) {
			t.Fatalf("expected anonymous profile to include %q, got %s", expected, anonymousBody)
		}
	}
	if !strings.Contains(anonymousBody, "/api/v1/moments/images/profile-safe-image") {
		t.Fatalf("expected profile to keep safe moment image URL, got %s", anonymousBody)
	}
	for _, forbidden := range []string{"profile-alice@", "Profile mutual moment", "Hidden profile blog", "hidden reason", "Hidden profile forum", "hidden forum reason", "reviewerId", "reviewedAt", "/uploads/moments/private.png", "javascript:alert(1)", "https://tracker.example/pixel.png", "../secret"} {
		if strings.Contains(anonymousBody, forbidden) {
			t.Fatalf("profile leaked forbidden value %q: %s", forbidden, anonymousBody)
		}
	}

	bobFollowsAlice := performJSON(router, http.MethodPost, "/api/v1/users/"+alice.ID+"/follow", "", bobToken)
	if bobFollowsAlice.Code != http.StatusOK {
		t.Fatalf("expected bob follow alice 200, got %d: %s", bobFollowsAlice.Code, bobFollowsAlice.Body.String())
	}
	aliceFollowsBob := performJSON(router, http.MethodPost, "/api/v1/users/"+bob.ID+"/follow", "", aliceToken)
	if aliceFollowsBob.Code != http.StatusOK {
		t.Fatalf("expected alice follow bob 200, got %d: %s", aliceFollowsBob.Code, aliceFollowsBob.Body.String())
	}
	bobProfile := performJSON(router, http.MethodGet, "/api/v1/users/"+alice.ID, "", bobToken)
	if bobProfile.Code != http.StatusOK || !strings.Contains(bobProfile.Body.String(), "Profile mutual moment") || !strings.Contains(bobProfile.Body.String(), `"mutualFriend":true`) {
		t.Fatalf("expected mutual friend profile to include mutual moment and relation state, got %d: %s", bobProfile.Code, bobProfile.Body.String())
	}

	block := performJSON(router, http.MethodPost, "/api/v1/users/"+bob.ID+"/block", "", aliceToken)
	if block.Code != http.StatusOK {
		t.Fatalf("expected alice block bob 200, got %d: %s", block.Code, block.Body.String())
	}
	blockedProfile := performJSON(router, http.MethodGet, "/api/v1/users/"+alice.ID, "", bobToken)
	if blockedProfile.Code != http.StatusNotFound {
		t.Fatalf("expected blocked profile to be hidden, got %d: %s", blockedProfile.Code, blockedProfile.Body.String())
	}
}
