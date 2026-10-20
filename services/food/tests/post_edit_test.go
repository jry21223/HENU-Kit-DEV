package tests

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"strings"
	"sync"
	"testing"
)

func authorPostRequest(t *testing.T, serverURL, actorID, method, path string, body []byte, key string) *http.Response {
	t.Helper()
	clientID, secret := postReadClientID, postReadSecret
	if method == http.MethodPut {
		clientID, secret = postCreateClientID, postCreateSecret
	}
	headers := postCreateHeaders(key)
	headers["X-Actor-User-Id"] = actorID
	return sendPost(t, serverURL, clientID, "active", secret, method, path, body, headers)
}

func TestAuthorEditsOwnPostWithStructuredFieldsAndReplay(t *testing.T) {
	server, pool := newFoodServer(t)
	defer server.Close()
	defer pool.Close()
	cleanPosts(t, pool)
	body := []byte(`{"venue_name":"仁和食堂","campus":"minglun","tier":"hang","review_text":"胡辣汤很顶。","price_reference":"18元","dishes":[{"name":"胡辣汤","price":"5元","reason":"料足"}]}`)
	created := decodeData(t, expectPostBody(t, createPostVia(t, server.URL, body, "edit_create_1"), 200))["post"].(map[string]any)
	path := "/api/v1/food/posts/" + created["id"].(string)
	loaded := decodeData(t, expectPostBody(t, authorPostRequest(t, server.URL, postActorID, http.MethodGet, path+"/edit", nil, ""), 200))["post"].(map[string]any)
	if loaded["version"] != float64(1) || loaded["tier"] != "hang" || loaded["price_reference"] != "18元" || len(loaded["dishes"].([]any)) != 1 {
		t.Fatalf("edit payload loses structured data: %#v", loaded)
	}
	for _, method := range []string{http.MethodGet, http.MethodPut} {
		target := path
		if method == http.MethodGet {
			target += "/edit"
		}
		response := authorPostRequest(t, server.URL, postOtherActor, method, target, []byte(`{}`), "foreign_edit")
		if response.StatusCode != 404 {
			t.Fatalf("foreign %s status %d: %s", method, response.StatusCode, readBody(t, response))
		}
		_ = response.Body.Close()
	}
	update := []byte(`{"expected_version":1,"venue_name":"仁和小馆","campus":"jinming","tier":"top","review_text":"牛肉面也很推荐。","price_reference":"20元","hours_reference":"午间营业","dishes":[{"name":"牛肉面","price":"20元","reason":"汤鲜"}],"retained_image_positions":[],"images":[]}`)
	first := expectPostBody(t, authorPostRequest(t, server.URL, postActorID, http.MethodPut, path, update, "edit_save_1"), 200)
	second := expectPostBody(t, authorPostRequest(t, server.URL, postActorID, http.MethodPut, path, update, "edit_save_1"), 200)
	var one, two map[string]any
	_ = json.Unmarshal(first, &one)
	_ = json.Unmarshal(second, &two)
	if fmt.Sprint(one["data"]) != fmt.Sprint(two["data"]) {
		t.Fatalf("replay differs: %s / %s", first, second)
	}
	public := expectPostBody(t, readPostVia(t, server.URL, http.MethodGet, path), 200)
	if !bytes.Contains(public, []byte(`"title":"仁和小馆"`)) || !bytes.Contains(public, []byte(`"author":"测试同学"`)) || !bytes.Contains(public, []byte("牛肉面：20元 — 汤鲜")) {
		t.Fatalf("edit not public: %s", public)
	}
	loaded = decodeData(t, expectPostBody(t, authorPostRequest(t, server.URL, postActorID, http.MethodGet, path+"/edit", nil, ""), 200))["post"].(map[string]any)
	if loaded["version"] != float64(2) {
		t.Fatalf("replay bumped version: %#v", loaded)
	}
}

func editBody(version int, retained string, images string) []byte {
	return []byte(fmt.Sprintf(`{"expected_version":%d,"venue_name":"仁和食堂","campus":"minglun","tier":"hang","review_text":"汤鲜料足，值得再来。","dishes":[],"retained_image_positions":%s,"images":%s}`, version, retained, images))
}
func editFixture(t *testing.T, url string) string {
	t.Helper()
	created := decodeData(t, expectPostBody(t, createPostVia(t, url, []byte(`{"venue_name":"仁和食堂","campus":"minglun","tier":"hang","review_text":"汤鲜料足。"}`), "edit_fixture_create"), 200))["post"].(map[string]any)
	return "/api/v1/food/posts/" + created["id"].(string)
}
func imageInput(t *testing.T, red uint8) string {
	t.Helper()
	var b bytes.Buffer
	canvas := image.NewRGBA(image.Rect(0, 0, 2, 2))
	canvas.Set(0, 0, color.RGBA{R: red, A: 255})
	if err := png.Encode(&b, canvas); err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf(`{"content_type":"image/png","data":%q}`, base64.StdEncoding.EncodeToString(b.Bytes()))
}
func assertEditError(t *testing.T, response *http.Response, status int, code string) {
	t.Helper()
	body := readBody(t, response)
	if response.StatusCode != status || !bytes.Contains(body, []byte(`"code":"`+code+`"`)) {
		t.Fatalf("status=%d body=%s, want %d %s", response.StatusCode, body, status, code)
	}
}
func TestAuthorEditPhotosKeepRemoveReplaceAndCacheIdentity(t *testing.T) {
	server, pool := newFoodServer(t)
	defer server.Close()
	defer pool.Close()
	cleanPosts(t, pool)
	path := editFixture(t, server.URL)
	first := expectPostBody(t, authorPostRequest(t, server.URL, postActorID, http.MethodPut, path, editBody(1, "[]", "["+imageInput(t, 30)+","+imageInput(t, 60)+"]"), "photos_first"), 200)
	photos := decodeData(t, first)["post"].(map[string]any)["images"].([]any)
	oldURL := photos[0].(string)
	if !strings.Contains(oldURL, "?v=") {
		t.Fatalf("photo URL unversioned: %s", oldURL)
	}
	keepURL := photos[1].(string)
	original := readBody(t, readPostVia(t, server.URL, http.MethodGet, keepURL))
	second := expectPostBody(t, authorPostRequest(t, server.URL, postActorID, http.MethodPut, path, editBody(2, "[1]", "["+imageInput(t, 90)+"]"), "photos_second"), 200)
	photos = decodeData(t, second)["post"].(map[string]any)["images"].([]any)
	kept := readBody(t, readPostVia(t, server.URL, http.MethodGet, photos[0].(string)))
	if !bytes.Equal(original, kept) || photos[0].(string) == oldURL {
		t.Fatal("retained photo lost or replacement reused old URL")
	}
	assertEditError(t, readPostVia(t, server.URL, http.MethodGet, oldURL), 404, "IMAGE_NOT_FOUND")
	detail := decodeData(t, expectPostBody(t, readPostVia(t, server.URL, http.MethodGet, path), 200))["post"].(map[string]any)
	if fmt.Sprint(detail["images"]) != fmt.Sprint(photos) {
		t.Fatalf("detail uses stale URLs: %#v", detail)
	}
	expectPostBody(t, authorPostRequest(t, server.URL, postActorID, http.MethodPut, path, editBody(3, "[]", "[]"), "photos_remove_all"), 200)
	assertEditError(t, readPostVia(t, server.URL, http.MethodGet, photos[0].(string)), 404, "IMAGE_NOT_FOUND")
}
func TestAuthorEditValidationCredentialAndModerationBoundaries(t *testing.T) {
	server, pool := newFoodServer(t)
	defer server.Close()
	defer pool.Close()
	cleanPosts(t, pool)
	path := editFixture(t, server.URL)
	for _, client := range []struct{ id, secret string }{{postReadClientID, postReadSecret}, {"console-gateway", serviceSecret}} {
		response := sendPost(t, server.URL, client.id, "active", client.secret, http.MethodPut, path, editBody(1, "[]", "[]"), postCreateHeaders("wrong_credential"))
		assertEditError(t, response, 401, "INVALID_SERVICE_AUTH")
	}
	response := sendPost(t, server.URL, postCreateClientID, "active", postCreateSecret, http.MethodPut, path, editBody(1, "[]", "[]"), nil)
	assertEditError(t, response, 401, "INVALID_ACTOR")
	for i, body := range []string{
		string(editBody(0, "[]", "[]")), string(editBody(1, "[0]", "[]")), string(editBody(1, "[0,0]", "[]")),
		strings.Replace(string(editBody(1, "[]", "[]")), `"minglun"`, `"other"`, 1),
		strings.Replace(string(editBody(1, "[]", "[]")), `"hang"`, `"invalid"`, 1),
		strings.Replace(string(editBody(1, "[]", "[]")), `"dishes":[]`, `"dishes":[{"name":""}]`, 1),
		strings.Replace(string(editBody(1, "[]", "[]")), `"venue_name":"仁和食堂"`, `"venue_name":"`+strings.Repeat("长", 161)+`"`, 1),
		string(editBody(1, "[]", `[{"content_type":"image/png","data":"bad"}]`)),
		strings.Replace(string(editBody(1, "[]", "[]")), `"expected_version":1`, `"expected_version":1,"hidden":false`, 1),
		strings.Replace(string(editBody(1, "[]", "[]")), `"expected_version":1`, `"expected_version":1,"author_user_id":"`+postOtherActor+`"`, 1),
	} {
		response := authorPostRequest(t, server.URL, postActorID, http.MethodPut, path, []byte(body), fmt.Sprintf("invalid_edit_%d", i))
		if response.StatusCode != 400 {
			t.Fatalf("invalid payload accepted: status=%d %s", response.StatusCode, readBody(t, response))
		}
		_ = response.Body.Close()
	}
	// A Console correction shares the optimistic version, so the author cannot overwrite it.
	postID := strings.TrimPrefix(path, "/api/v1/food/posts/")
	command := []byte(fmt.Sprintf(`{"kind":"post_edit","resource_id":%q,"expected_version":1,"payload":{"note":"校区核验","campus":"jinming"}}`, postID))
	expectPostBody(t, send(t, server.URL, "food.review", http.MethodPost, "/api/v1/commands", command, "console_correction_"+postID), 200)
	assertEditError(t, authorPostRequest(t, server.URL, postActorID, http.MethodPut, path, editBody(1, "[]", "[]"), "author_stale"), 409, "VERSION_CONFLICT")
	command = []byte(fmt.Sprintf(`{"kind":"post_edit","resource_id":%q,"expected_version":2,"payload":{"note":"运营隐藏","hidden":true}}`, postID))
	expectPostBody(t, send(t, server.URL, "food.review", http.MethodPost, "/api/v1/commands", command, "console_hide_"+postID), 200)
	assertEditError(t, authorPostRequest(t, server.URL, postActorID, http.MethodGet, path+"/edit", nil, ""), 409, "POST_HIDDEN")
	assertEditError(t, authorPostRequest(t, server.URL, postActorID, http.MethodPut, path, editBody(3, "[]", "[]"), "author_hidden"), 409, "POST_HIDDEN")
	assertEditError(t, readPostVia(t, server.URL, http.MethodGet, path), 404, "POST_NOT_FOUND")
}
func TestAuthorEditConcurrentVersionsKeysAndQuota(t *testing.T) {
	server, pool := newFoodServer(t)
	defer server.Close()
	defer pool.Close()
	cleanPosts(t, pool)
	path := editFixture(t, server.URL)
	statuses := make([]int, 2)
	var wg sync.WaitGroup
	for i := range statuses {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			response := authorPostRequest(t, server.URL, postActorID, http.MethodPut, path, editBody(1, "[]", "[]"), fmt.Sprintf("concurrent_edit_%d", i))
			statuses[i] = response.StatusCode
			_ = response.Body.Close()
		}(i)
	}
	wg.Wait()
	if !((statuses[0] == 200 && statuses[1] == 409) || (statuses[1] == 200 && statuses[0] == 409)) {
		t.Fatalf("concurrent statuses %v", statuses)
	}
	expectPostBody(t, authorPostRequest(t, server.URL, postActorID, http.MethodPut, path, editBody(2, "[]", "[]"), "edit_replay_key"), 200)
	assertEditError(t, authorPostRequest(t, server.URL, postActorID, http.MethodPut, path, editBody(3, "[]", "[]"), "edit_replay_key"), 409, "IDEMPOTENCY_KEY_CONFLICT")
	assertEditError(t, authorPostRequest(t, server.URL, postActorID, http.MethodPut, path, editBody(3, "[]", "[]"), "edit_fixture_create"), 409, "IDEMPOTENCY_KEY_CONFLICT")
	body := []byte(`{"venue_name":"小面馆","campus":"minglun","tier":"top","review_text":"热乎好吃。"}`)
	for i := 0; i < 2; i++ {
		expectPostBody(t, createPostVia(t, server.URL, body, fmt.Sprintf("quota_create_%d", i)), 200)
	}
	assertEditError(t, createPostVia(t, server.URL, body, "quota_fourth"), 429, "DAILY_POST_CAP_REACHED")
	expectPostBody(t, authorPostRequest(t, server.URL, postActorID, http.MethodPut, path, editBody(3, "[]", "[]"), "edit_at_create_cap"), 200)
}
func TestAuthorEditPersistenceFailureRollsBackAndSameKeyRecovers(t *testing.T) {
	server, pool := newFoodServer(t)
	defer server.Close()
	defer pool.Close()
	cleanPosts(t, pool)
	path := editFixture(t, server.URL)
	_, err := pool.Exec(context.Background(), `CREATE FUNCTION fail_author_edit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.idempotency_key='edit_fail_recover' THEN RAISE EXCEPTION 'injected persistence failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER fail_author_edit BEFORE INSERT ON food_post_operations FOR EACH ROW EXECUTE FUNCTION fail_author_edit()`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = pool.Exec(context.Background(), `DROP TRIGGER IF EXISTS fail_author_edit ON food_post_operations; DROP FUNCTION IF EXISTS fail_author_edit()`)
	}()
	body := editBody(1, "[]", "["+imageInput(t, 90)+"]")
	assertEditError(t, authorPostRequest(t, server.URL, postActorID, http.MethodPut, path, body, "edit_fail_recover"), 503, "DEPENDENCY_UNAVAILABLE")
	loaded := decodeData(t, expectPostBody(t, authorPostRequest(t, server.URL, postActorID, http.MethodGet, path+"/edit", nil, ""), 200))["post"].(map[string]any)
	if loaded["version"] != float64(1) || len(loaded["images"].([]any)) != 0 || loaded["review_text"] != "汤鲜料足。" {
		t.Fatalf("failure left partial update: %#v", loaded)
	}
	_, err = pool.Exec(context.Background(), `DROP TRIGGER fail_author_edit ON food_post_operations; DROP FUNCTION fail_author_edit()`)
	if err != nil {
		t.Fatal(err)
	}
	expectPostBody(t, authorPostRequest(t, server.URL, postActorID, http.MethodPut, path, body, "edit_fail_recover"), 200)
}
