package tests

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"final-review-platform/services/api/internal/platform/model"
	"final-review-platform/services/api/internal/server"
	applogger "final-review-platform/services/api/pkg/logger"
)

func TestMomentAndRelationWorkflow(t *testing.T) {
	db := newTestDB(t)
	cfg := testConfig()
	cfg.LocalUploadDir = t.TempDir()
	router := server.NewRouter(cfg, applogger.New("test"), db, nil)

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

	unauthorizedUpload := performMultipart(router, "/api/v1/moments/images", "", nil, "file", "progress.png", validPNGBytes())
	if unauthorizedUpload.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthenticated image upload 401, got %d: %s", unauthorizedUpload.Code, unauthorizedUpload.Body.String())
	}
	unsupportedUpload := performMultipart(router, "/api/v1/moments/images", aliceToken, nil, "file", "note.txt", []byte("plain text"))
	if unsupportedUpload.Code != http.StatusBadRequest || !strings.Contains(unsupportedUpload.Body.String(), "unsupported_image_type") {
		t.Fatalf("expected unsupported image upload rejection, got %d: %s", unsupportedUpload.Code, unsupportedUpload.Body.String())
	}
	invalidContentUpload := performMultipart(router, "/api/v1/moments/images", aliceToken, nil, "file", "fake.png", []byte("not actually png"))
	if invalidContentUpload.Code != http.StatusBadRequest || !strings.Contains(invalidContentUpload.Body.String(), "invalid_image_content") {
		t.Fatalf("expected invalid image content rejection, got %d: %s", invalidContentUpload.Code, invalidContentUpload.Body.String())
	}
	largeImage := append(validPNGBytes(), make([]byte, 5*1024*1024+1)...)
	tooLargeUpload := performMultipart(router, "/api/v1/moments/images", aliceToken, nil, "file", "large.png", largeImage)
	if tooLargeUpload.Code != http.StatusBadRequest || !strings.Contains(tooLargeUpload.Body.String(), "file_too_large") {
		t.Fatalf("expected oversized image upload rejection, got %d: %s", tooLargeUpload.Code, tooLargeUpload.Body.String())
	}
	imageUpload := performMultipart(router, "/api/v1/moments/images", aliceToken, nil, "file", "progress.png", validPNGBytes())
	if imageUpload.Code != http.StatusOK {
		t.Fatalf("expected image upload 200, got %d: %s", imageUpload.Code, imageUpload.Body.String())
	}
	var imagePayload struct {
		Data struct {
			Image struct {
				URL         string `json:"url"`
				FileName    string `json:"fileName"`
				FileSize    int64  `json:"fileSize"`
				ContentType string `json:"contentType"`
			} `json:"image"`
		} `json:"data"`
	}
	if err := json.Unmarshal(imageUpload.Body.Bytes(), &imagePayload); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(imagePayload.Data.Image.URL, "/uploads/moments/"+alice.ID+"/") || strings.Contains(imagePayload.Data.Image.URL, "progress.png") {
		t.Fatalf("expected generated user-scoped upload URL, got %#v", imagePayload.Data.Image)
	}
	if imagePayload.Data.Image.FileName != "progress.png" || imagePayload.Data.Image.ContentType != "image/png" || imagePayload.Data.Image.FileSize <= 0 {
		t.Fatalf("unexpected upload metadata: %#v", imagePayload.Data.Image)
	}
	uploadedPath := filepath.Join(cfg.LocalUploadDir, filepath.FromSlash(strings.TrimPrefix(imagePayload.Data.Image.URL, "/uploads/")))
	if _, err := os.Stat(uploadedPath); err != nil {
		t.Fatalf("expected uploaded image on disk: %v", err)
	}
	servedImage := performJSON(router, http.MethodGet, imagePayload.Data.Image.URL, "", "")
	if servedImage.Code != http.StatusOK {
		t.Fatalf("expected uploaded image to be served, got %d: %s", servedImage.Code, servedImage.Body.String())
	}
	otherUserImage := strings.Replace(imagePayload.Data.Image.URL, alice.ID, bob.ID, 1)
	foreignImageCreate := performJSON(router, http.MethodPost, "/api/v1/moments", `{"content":"foreign image","images":["`+otherUserImage+`"]}`, aliceToken)
	if foreignImageCreate.Code != http.StatusBadRequest || !strings.Contains(foreignImageCreate.Body.String(), "invalid_image_url") {
		t.Fatalf("expected foreign image URL rejection, got %d: %s", foreignImageCreate.Code, foreignImageCreate.Body.String())
	}
	traversalImage := "/uploads/moments/" + alice.ID + "/../secret.png"
	traversalImageCreate := performJSON(router, http.MethodPost, "/api/v1/moments", `{"content":"traversal image","images":["`+traversalImage+`"]}`, aliceToken)
	if traversalImageCreate.Code != http.StatusBadRequest || !strings.Contains(traversalImageCreate.Body.String(), "invalid_image_url") {
		t.Fatalf("expected traversal image URL rejection, got %d: %s", traversalImageCreate.Code, traversalImageCreate.Body.String())
	}

	publicCreate := performJSON(router, http.MethodPost, "/api/v1/moments", `{"content":"Public review note","images":["`+imagePayload.Data.Image.URL+`"]}`, aliceToken)
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

func validPNGBytes() []byte {
	return []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0, 0, 0, 0}
}
