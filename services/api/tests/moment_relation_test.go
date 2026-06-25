package tests

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gorm.io/datatypes"

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
				ID          string `json:"id"`
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
	if imagePayload.Data.Image.ID == "" || imagePayload.Data.Image.URL != "/api/v1/moments/images/"+imagePayload.Data.Image.ID {
		t.Fatalf("expected generated user-scoped upload URL, got %#v", imagePayload.Data.Image)
	}
	if imagePayload.Data.Image.FileName != "progress.png" || imagePayload.Data.Image.ContentType != "image/png" || imagePayload.Data.Image.FileSize <= 0 {
		t.Fatalf("unexpected upload metadata: %#v", imagePayload.Data.Image)
	}
	var uploadedAsset model.MediaAsset
	if err := db.First(&uploadedAsset, "id = ?", imagePayload.Data.Image.ID).Error; err != nil {
		t.Fatal(err)
	}
	if uploadedAsset.OwnerID != alice.ID || uploadedAsset.Status != "uploaded" || uploadedAsset.MomentID != nil {
		t.Fatalf("unexpected uploaded media asset: %#v", uploadedAsset)
	}
	uploadedPath := filepath.Join(cfg.LocalUploadDir, filepath.FromSlash(uploadedAsset.StorageKey))
	if _, err := os.Stat(uploadedPath); err != nil {
		t.Fatalf("expected uploaded image on disk: %v", err)
	}
	ownerPreview := performJSON(router, http.MethodGet, imagePayload.Data.Image.URL, "", aliceToken)
	if ownerPreview.Code != http.StatusOK {
		t.Fatalf("expected owner to preview unattached image, got %d: %s", ownerPreview.Code, ownerPreview.Body.String())
	}
	anonymousPreview := performJSON(router, http.MethodGet, imagePayload.Data.Image.URL, "", "")
	if anonymousPreview.Code != http.StatusNotFound {
		t.Fatalf("expected anonymous preview of unattached image to be denied, got %d: %s", anonymousPreview.Code, anonymousPreview.Body.String())
	}
	bobImageUpload := performMultipart(router, "/api/v1/moments/images", bobToken, nil, "file", "bob.png", validPNGBytes())
	if bobImageUpload.Code != http.StatusOK {
		t.Fatalf("expected bob image upload 200, got %d: %s", bobImageUpload.Code, bobImageUpload.Body.String())
	}
	var bobImagePayload struct {
		Data struct {
			Image struct {
				URL string `json:"url"`
			} `json:"image"`
		} `json:"data"`
	}
	if err := json.Unmarshal(bobImageUpload.Body.Bytes(), &bobImagePayload); err != nil {
		t.Fatal(err)
	}
	foreignImageCreate := performJSON(router, http.MethodPost, "/api/v1/moments", `{"content":"foreign image","images":["`+bobImagePayload.Data.Image.URL+`"]}`, aliceToken)
	if foreignImageCreate.Code != http.StatusBadRequest || !strings.Contains(foreignImageCreate.Body.String(), "image_not_found") {
		t.Fatalf("expected foreign image URL rejection, got %d: %s", foreignImageCreate.Code, foreignImageCreate.Body.String())
	}
	traversalImage := "/api/v1/moments/images/" + imagePayload.Data.Image.ID + "/../secret"
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
	if err := db.First(&uploadedAsset, "id = ?", uploadedAsset.ID).Error; err != nil {
		t.Fatal(err)
	}
	if uploadedAsset.MomentID == nil || *uploadedAsset.MomentID != publicMoment.ID || uploadedAsset.Status != "attached" {
		t.Fatalf("expected image to be attached to public moment, got %#v", uploadedAsset)
	}
	anonymousPublicImage := performJSON(router, http.MethodGet, imagePayload.Data.Image.URL, "", "")
	if anonymousPublicImage.Code != http.StatusOK {
		t.Fatalf("expected anonymous to read public moment image, got %d: %s", anonymousPublicImage.Code, anonymousPublicImage.Body.String())
	}
	reuseImage := performJSON(router, http.MethodPost, "/api/v1/moments", `{"content":"reuse image","images":["`+imagePayload.Data.Image.URL+`"]}`, aliceToken)
	if reuseImage.Code != http.StatusBadRequest || !strings.Contains(reuseImage.Body.String(), "image_not_found") {
		t.Fatalf("expected already attached image to be rejected, got %d: %s", reuseImage.Code, reuseImage.Body.String())
	}

	publicList := performJSON(router, http.MethodGet, "/api/v1/moments", "", "")
	if publicList.Code != http.StatusOK || !strings.Contains(publicList.Body.String(), publicMoment.ID) || strings.Contains(publicList.Body.String(), "moment-alice@") {
		t.Fatalf("expected anonymous public moment list without author email, got %d: %s", publicList.Code, publicList.Body.String())
	}

	mutualImageUpload := performMultipart(router, "/api/v1/moments/images", aliceToken, nil, "file", "mutual.png", validPNGBytes())
	if mutualImageUpload.Code != http.StatusOK {
		t.Fatalf("expected mutual image upload 200, got %d: %s", mutualImageUpload.Code, mutualImageUpload.Body.String())
	}
	var mutualImagePayload struct {
		Data struct {
			Image struct {
				URL string `json:"url"`
			} `json:"image"`
		} `json:"data"`
	}
	if err := json.Unmarshal(mutualImageUpload.Body.Bytes(), &mutualImagePayload); err != nil {
		t.Fatal(err)
	}
	mutualCreate := performJSON(router, http.MethodPost, "/api/v1/moments", `{"content":"Mutual friends only note","visibility":"mutual_friends","images":["`+mutualImagePayload.Data.Image.URL+`"]}`, aliceToken)
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
	anonymousMutualImage := performJSON(router, http.MethodGet, mutualImagePayload.Data.Image.URL, "", "")
	if anonymousMutualImage.Code != http.StatusNotFound {
		t.Fatalf("expected anonymous mutual image read denial, got %d: %s", anonymousMutualImage.Code, anonymousMutualImage.Body.String())
	}
	bobBeforeFollow := performJSON(router, http.MethodGet, "/api/v1/moments", "", bobToken)
	if bobBeforeFollow.Code != http.StatusOK || strings.Contains(bobBeforeFollow.Body.String(), mutualMoment.ID) {
		t.Fatalf("expected one-way non-friend to miss mutual-friends moment, got %d: %s", bobBeforeFollow.Code, bobBeforeFollow.Body.String())
	}
	bobBeforeFollowImage := performJSON(router, http.MethodGet, mutualImagePayload.Data.Image.URL, "", bobToken)
	if bobBeforeFollowImage.Code != http.StatusNotFound {
		t.Fatalf("expected one-way non-friend image read denial, got %d: %s", bobBeforeFollowImage.Code, bobBeforeFollowImage.Body.String())
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
	bobAsFriendImage := performJSON(router, http.MethodGet, mutualImagePayload.Data.Image.URL, "", bobToken)
	if bobAsFriendImage.Code != http.StatusOK {
		t.Fatalf("expected mutual friend to read mutual-friends image, got %d: %s", bobAsFriendImage.Code, bobAsFriendImage.Body.String())
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
	bobImageAfterBlock := performJSON(router, http.MethodGet, mutualImagePayload.Data.Image.URL, "", bobToken)
	if bobImageAfterBlock.Code != http.StatusNotFound {
		t.Fatalf("expected block to deny mutual image read, got %d: %s", bobImageAfterBlock.Code, bobImageAfterBlock.Body.String())
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

func TestMomentListFiltersUnsafeStoredImageURLs(t *testing.T) {
	db := newTestDB(t)
	router := server.NewRouter(testConfig(), applogger.New("test"), db, nil)
	author := createTestUser(t, db, "moment-dirty-images@stu.henu.edu.cn", model.RoleUser)
	moment := model.Moment{
		AuthorID: author.ID,
		Content:  "Dirty image URLs should not leak to clients.",
		Images: datatypes.JSON([]byte(`[
			"/api/v1/moments/images/safe-image-id",
			"/uploads/moments/private.png",
			"javascript:alert(1)",
			"https://tracker.example/pixel.png",
			"/api/v1/moments/images/safe-image-id/../secret"
		]`)),
		Status: model.StatusPublished,
	}
	if err := db.Create(&moment).Error; err != nil {
		t.Fatal(err)
	}

	list := performJSON(router, http.MethodGet, "/api/v1/moments", "", "")
	if list.Code != http.StatusOK {
		t.Fatalf("expected moment list 200, got %d: %s", list.Code, list.Body.String())
	}
	body := list.Body.String()
	if !strings.Contains(body, "/api/v1/moments/images/safe-image-id") {
		t.Fatalf("expected safe API image URL to remain, got %s", body)
	}
	for _, unsafe := range []string{"/uploads/moments/private.png", "javascript:alert(1)", "https://tracker.example/pixel.png", "../secret"} {
		if strings.Contains(body, unsafe) {
			t.Fatalf("unsafe stored image URL %q leaked in response: %s", unsafe, body)
		}
	}
}

func validPNGBytes() []byte {
	return []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0, 0, 0, 0}
}
