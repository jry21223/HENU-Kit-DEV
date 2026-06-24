package tests

import (
	"net/http"
	"strings"
	"testing"

	"final-review-platform/services/api/internal/platform/model"
	"final-review-platform/services/api/internal/server"
	applogger "final-review-platform/services/api/pkg/logger"
)

func TestMomentAndRelationWorkflow(t *testing.T) {
	db := newTestDB(t)
	router := server.NewRouter(testConfig(), applogger.New("test"), db, nil)

	aliceToken := loginTestUser(t, router, "moment-alice@stu.henu.edu.cn")
	bobToken := loginTestUser(t, router, "moment-bob@stu.henu.edu.cn")

	var alice model.User
	if err := db.First(&alice, "email = ?", "moment-alice@stu.henu.edu.cn").Error; err != nil {
		t.Fatal(err)
	}
	var bob model.User
	if err := db.First(&bob, "email = ?", "moment-bob@stu.henu.edu.cn").Error; err != nil {
		t.Fatal(err)
	}

	unauthorizedCreate := performJSON(router, http.MethodPost, "/api/v1/moments", `{"content":"no auth"}`, "")
	if unauthorizedCreate.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthenticated moment create 401, got %d: %s", unauthorizedCreate.Code, unauthorizedCreate.Body.String())
	}

	invalidImage := performJSON(router, http.MethodPost, "/api/v1/moments", `{"content":"bad image","images":["../../secret.png"]}`, aliceToken)
	if invalidImage.Code != http.StatusBadRequest || !strings.Contains(invalidImage.Body.String(), "invalid_image_url") {
		t.Fatalf("expected invalid image rejection, got %d: %s", invalidImage.Code, invalidImage.Body.String())
	}

	publicCreate := performJSON(router, http.MethodPost, "/api/v1/moments", `{"content":"Public review note","images":["/uploads/moments/public.png"]}`, aliceToken)
	if publicCreate.Code != http.StatusOK {
		t.Fatalf("expected public moment create 200, got %d: %s", publicCreate.Code, publicCreate.Body.String())
	}
	var publicMoment model.Moment
	if err := db.First(&publicMoment, "content = ?", "Public review note").Error; err != nil {
		t.Fatal(err)
	}
	if publicMoment.Visibility != "public" || publicMoment.Status != model.StatusPublished {
		t.Fatalf("expected published public moment, got %#v", publicMoment)
	}

	publicList := performJSON(router, http.MethodGet, "/api/v1/moments", "", "")
	if publicList.Code != http.StatusOK || !strings.Contains(publicList.Body.String(), publicMoment.ID) || strings.Contains(publicList.Body.String(), "moment-alice@") {
		t.Fatalf("expected anonymous public moment list without author email, got %d: %s", publicList.Code, publicList.Body.String())
	}

	mutualCreate := performJSON(router, http.MethodPost, "/api/v1/moments", `{"content":"Mutual friends only note","visibility":"mutual_friends"}`, aliceToken)
	if mutualCreate.Code != http.StatusOK {
		t.Fatalf("expected mutual-friends moment create 200, got %d: %s", mutualCreate.Code, mutualCreate.Body.String())
	}
	var mutualMoment model.Moment
	if err := db.First(&mutualMoment, "content = ?", "Mutual friends only note").Error; err != nil {
		t.Fatal(err)
	}

	anonymousList := performJSON(router, http.MethodGet, "/api/v1/moments", "", "")
	if anonymousList.Code != http.StatusOK || strings.Contains(anonymousList.Body.String(), mutualMoment.ID) {
		t.Fatalf("expected anonymous list to hide mutual-friends moment, got %d: %s", anonymousList.Code, anonymousList.Body.String())
	}
	bobBeforeFollow := performJSON(router, http.MethodGet, "/api/v1/moments", "", bobToken)
	if bobBeforeFollow.Code != http.StatusOK || strings.Contains(bobBeforeFollow.Body.String(), mutualMoment.ID) {
		t.Fatalf("expected one-way non-friend to miss mutual-friends moment, got %d: %s", bobBeforeFollow.Code, bobBeforeFollow.Body.String())
	}

	bobFollowsAlice := performJSON(router, http.MethodPost, "/api/v1/users/"+alice.ID+"/follow", "", bobToken)
	if bobFollowsAlice.Code != http.StatusOK {
		t.Fatalf("expected bob follow alice 200, got %d: %s", bobFollowsAlice.Code, bobFollowsAlice.Body.String())
	}
	oneWayList := performJSON(router, http.MethodGet, "/api/v1/moments", "", bobToken)
	if oneWayList.Code != http.StatusOK || strings.Contains(oneWayList.Body.String(), mutualMoment.ID) {
		t.Fatalf("expected one-way follow to still hide mutual-friends moment, got %d: %s", oneWayList.Code, oneWayList.Body.String())
	}

	aliceFollowsBob := performJSON(router, http.MethodPost, "/api/v1/users/"+bob.ID+"/follow", "", aliceToken)
	if aliceFollowsBob.Code != http.StatusOK {
		t.Fatalf("expected alice follow bob 200, got %d: %s", aliceFollowsBob.Code, aliceFollowsBob.Body.String())
	}
	bobAsFriend := performJSON(router, http.MethodGet, "/api/v1/moments", "", bobToken)
	if bobAsFriend.Code != http.StatusOK || !strings.Contains(bobAsFriend.Body.String(), mutualMoment.ID) {
		t.Fatalf("expected mutual friend to see mutual-friends moment, got %d: %s", bobAsFriend.Code, bobAsFriend.Body.String())
	}

	aliceFriends := performJSON(router, http.MethodGet, "/api/v1/me/friends", "", aliceToken)
	if aliceFriends.Code != http.StatusOK || !strings.Contains(aliceFriends.Body.String(), bob.ID) || strings.Contains(aliceFriends.Body.String(), "moment-bob@") {
		t.Fatalf("expected bob in alice friends, got %d: %s", aliceFriends.Code, aliceFriends.Body.String())
	}
	bobFollowing := performJSON(router, http.MethodGet, "/api/v1/me/following", "", bobToken)
	if bobFollowing.Code != http.StatusOK || !strings.Contains(bobFollowing.Body.String(), alice.ID) || strings.Contains(bobFollowing.Body.String(), "moment-alice@") {
		t.Fatalf("expected alice in bob following, got %d: %s", bobFollowing.Code, bobFollowing.Body.String())
	}
	aliceFollowers := performJSON(router, http.MethodGet, "/api/v1/me/followers", "", aliceToken)
	if aliceFollowers.Code != http.StatusOK || !strings.Contains(aliceFollowers.Body.String(), bob.ID) || strings.Contains(aliceFollowers.Body.String(), "moment-bob@") {
		t.Fatalf("expected bob in alice followers, got %d: %s", aliceFollowers.Code, aliceFollowers.Body.String())
	}

	like := performJSON(router, http.MethodPost, "/api/v1/moments/"+mutualMoment.ID+"/like", "", bobToken)
	if like.Code != http.StatusOK || !strings.Contains(like.Body.String(), `"created":true`) {
		t.Fatalf("expected first like to create relation, got %d: %s", like.Code, like.Body.String())
	}
	likeAgain := performJSON(router, http.MethodPost, "/api/v1/moments/"+mutualMoment.ID+"/like", "", bobToken)
	if likeAgain.Code != http.StatusOK || !strings.Contains(likeAgain.Body.String(), `"created":false`) {
		t.Fatalf("expected duplicate like idempotent, got %d: %s", likeAgain.Code, likeAgain.Body.String())
	}
	if err := db.First(&mutualMoment, "id = ?", mutualMoment.ID).Error; err != nil {
		t.Fatal(err)
	}
	if mutualMoment.LikeCount != 1 {
		t.Fatalf("expected one like after duplicate like, got %d", mutualMoment.LikeCount)
	}

	comment := performJSON(router, http.MethodPost, "/api/v1/moments/"+mutualMoment.ID+"/comments", `{"content":"This helps."}`, bobToken)
	if comment.Code != http.StatusOK {
		t.Fatalf("expected comment create 200, got %d: %s", comment.Code, comment.Body.String())
	}
	var savedComment model.MomentComment
	if err := db.First(&savedComment, "moment_id = ? AND author_id = ?", mutualMoment.ID, bob.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&mutualMoment, "id = ?", mutualMoment.ID).Error; err != nil {
		t.Fatal(err)
	}
	if mutualMoment.CommentCount != 1 {
		t.Fatalf("expected comment count 1, got %d", mutualMoment.CommentCount)
	}
	deleteComment := performJSON(router, http.MethodDelete, "/api/v1/moments/comments/"+savedComment.ID, "", aliceToken)
	if deleteComment.Code != http.StatusOK {
		t.Fatalf("expected moment author to delete comment 200, got %d: %s", deleteComment.Code, deleteComment.Body.String())
	}
	if err := db.First(&mutualMoment, "id = ?", mutualMoment.ID).Error; err != nil {
		t.Fatal(err)
	}
	if mutualMoment.CommentCount != 0 {
		t.Fatalf("expected comment count 0 after delete, got %d", mutualMoment.CommentCount)
	}

	block := performJSON(router, http.MethodPost, "/api/v1/users/"+bob.ID+"/block", "", aliceToken)
	if block.Code != http.StatusOK {
		t.Fatalf("expected alice block bob 200, got %d: %s", block.Code, block.Body.String())
	}
	followAfterBlock := performJSON(router, http.MethodPost, "/api/v1/users/"+alice.ID+"/follow", "", bobToken)
	if followAfterBlock.Code != http.StatusForbidden || !strings.Contains(followAfterBlock.Body.String(), "blocked_relation") {
		t.Fatalf("expected blocked follow rejection, got %d: %s", followAfterBlock.Code, followAfterBlock.Body.String())
	}
	bobAfterBlock := performJSON(router, http.MethodGet, "/api/v1/moments", "", bobToken)
	if bobAfterBlock.Code != http.StatusOK || strings.Contains(bobAfterBlock.Body.String(), publicMoment.ID) || strings.Contains(bobAfterBlock.Body.String(), mutualMoment.ID) {
		t.Fatalf("expected block to hide alice moments from bob, got %d: %s", bobAfterBlock.Code, bobAfterBlock.Body.String())
	}
	aliceFriendsAfterBlock := performJSON(router, http.MethodGet, "/api/v1/me/friends", "", aliceToken)
	if aliceFriendsAfterBlock.Code != http.StatusOK || strings.Contains(aliceFriendsAfterBlock.Body.String(), bob.ID) {
		t.Fatalf("expected block to remove friendship, got %d: %s", aliceFriendsAfterBlock.Code, aliceFriendsAfterBlock.Body.String())
	}

	unblock := performJSON(router, http.MethodPost, "/api/v1/users/"+bob.ID+"/unblock", "", aliceToken)
	if unblock.Code != http.StatusOK {
		t.Fatalf("expected unblock 200, got %d: %s", unblock.Code, unblock.Body.String())
	}
	selfFollow := performJSON(router, http.MethodPost, "/api/v1/users/"+alice.ID+"/follow", "", aliceToken)
	if selfFollow.Code != http.StatusBadRequest || !strings.Contains(selfFollow.Body.String(), "invalid_target") {
		t.Fatalf("expected self-follow rejection, got %d: %s", selfFollow.Code, selfFollow.Body.String())
	}
}
