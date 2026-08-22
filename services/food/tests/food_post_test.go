package tests

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	postActorID    = "44444444-4444-4444-8444-444444444444"
	postActorName  = "测试同学"
	postOtherActor = "55555555-5555-4555-8555-555555555555"
)

// cleanPosts removes every Food Post row so count assertions stay exact. The
// package shares one test database across tests, and the legacy tables use
// ON CONFLICT seeding while the new tables do not.
func cleanPosts(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `TRUNCATE food_post_operations, food_post_images, food_post_dishes, food_posts`); err != nil {
		t.Fatal(err)
	}
}

// sendPost performs one signed request against a Food Post credential ring.
// Actor headers are only added when the caller supplies them, so public reads
// can prove they need no actor.
func sendPost(t *testing.T, baseURL, clientID, keyID, secret, method, path string, body []byte, headers map[string]string) *http.Response {
	t.Helper()
	request, err := http.NewRequest(method, baseURL+path, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("X-Service-Id", clientID)
	request.Header.Set("X-Key-Id", keyID)
	request.Header.Set("X-Request-Id", "req_food_post_"+strings.ReplaceAll(uuid.NewString(), "-", ""))
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	nonce := base64.RawURLEncoding.EncodeToString([]byte(uuid.NewString()[:24]))
	digest := sha256.Sum256(body)
	canonical := strings.Join([]string{method, path, timestamp, nonce, hex.EncodeToString(digest[:])}, "\n")
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(canonical))
	request.Header.Set("X-Timestamp", timestamp)
	request.Header.Set("X-Nonce", nonce)
	request.Header.Set("X-Signature", base64.RawURLEncoding.EncodeToString(mac.Sum(nil)))
	request.SetBasicAuth(clientID, secret)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func postCreateHeaders(idempotencyKey string) map[string]string {
	return map[string]string{
		"X-Actor-User-Id":      postActorID,
		"X-Actor-Display-Name": postActorName,
		"Idempotency-Key":      idempotencyKey,
	}
}

func createPostVia(t *testing.T, serverURL string, body []byte, idempotencyKey string) *http.Response {
	t.Helper()
	return sendPost(t, serverURL, postCreateClientID, "active", postCreateSecret, http.MethodPost, "/api/v1/food/posts", body, postCreateHeaders(idempotencyKey))
}

func readPostVia(t *testing.T, serverURL, method, path string) *http.Response {
	t.Helper()
	return sendPost(t, serverURL, postReadClientID, "active", postReadSecret, method, path, nil, nil)
}

// expectPostBody reads the response and asserts the status; the payload is
// returned for further assertions.
func expectPostBody(t *testing.T, response *http.Response, want int) []byte {
	t.Helper()
	payload := readBody(t, response)
	if response.StatusCode != want {
		t.Fatalf("status = %d, want %d: %s", response.StatusCode, want, payload)
	}
	var envelope struct {
		Error *map[string]string `json:"error"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil || envelope.Error != nil {
		t.Fatalf("unexpected error envelope: %s", payload)
	}
	return payload
}

func TestCreatePostPublishesImmediatelyAndIsPubliclyReadable(t *testing.T) {
	server, pool := newFoodServer(t)
	defer server.Close()
	defer pool.Close()
	cleanPosts(t, pool)

	body := []byte(`{"venue_name":"仁和食堂","campus":"minglun","tier":"hang","review_text":"胡辣汤很顶，早餐必去。","price_reference":"18元"}`)
	created := expectPostBody(t, createPostVia(t, server.URL, body, "idem_public_1"), http.StatusOK)
	for _, expected := range []string{`"hidden":false`, `"title":"仁和食堂"`, `"author":"测试同学"`, `"tags":["夯"]`, `"campus":"minglun"`, `"text":"价格参考：18元"`} {
		if !bytes.Contains(created, []byte(expected)) {
			t.Fatalf("create payload omitted %s: %s", expected, created)
		}
	}
	if bytes.Contains(created, []byte(`"lat"`)) || bytes.Contains(created, []byte(`"lng"`)) {
		t.Fatalf("map coordinates remain in the Food post contract: %s", created)
	}
	postID := decodeData(t, created)["post"].(map[string]any)["id"].(string)
	if _, err := uuid.Parse(postID); err != nil {
		t.Fatalf("post id is not a uuid: %s", postID)
	}

	list := expectPostBody(t, readPostVia(t, server.URL, http.MethodGet, "/api/v1/food/posts"), http.StatusOK)
	if !bytes.Contains(list, []byte(`"title":"仁和食堂"`)) || !bytes.Contains(list, []byte(`"text":"价格参考：18元"`)) {
		t.Fatalf("public list omitted the new post or its price reference: %s", list)
	}
	detail := expectPostBody(t, readPostVia(t, server.URL, http.MethodGet, "/api/v1/food/posts/"+postID), http.StatusOK)
	if !bytes.Contains(detail, []byte(`"comments":[]`)) || !bytes.Contains(detail, []byte(`"text":"价格参考：18元"`)) {
		t.Fatalf("detail omitted comments or the price reference: %s", detail)
	}
	venues := expectPostBody(t, readPostVia(t, server.URL, http.MethodGet, "/api/v1/food/venues?campus=minglun"), http.StatusOK)
	if !bytes.Contains(venues, []byte(`"name":"仁和食堂"`)) {
		t.Fatalf("venue summary omitted the new venue: %s", venues)
	}
	var hidden bool
	if err := pool.QueryRow(context.Background(), `SELECT hidden FROM food_posts WHERE venue_name='仁和食堂'`).Scan(&hidden); err != nil || hidden {
		t.Fatalf("stored post hidden=%v, err=%v", hidden, err)
	}
}

func TestCreatePostRejectsMissingRequiredFieldsAndBadEnums(t *testing.T) {
	server, pool := newFoodServer(t)
	defer server.Close()
	defer pool.Close()
	cleanPosts(t, pool)

	tests := []struct {
		name string
		body string
	}{
		{"missing venue", `{"campus":"minglun","tier":"hang","review_text":"好吃好吃。"}`},
		{"missing campus", `{"venue_name":"仁和食堂","tier":"hang","review_text":"好吃好吃。"}`},
		{"invalid campus", `{"venue_name":"仁和食堂","campus":"north","tier":"hang","review_text":"好吃好吃。"}`},
		{"missing tier", `{"venue_name":"仁和食堂","campus":"minglun","review_text":"好吃好吃。"}`},
		{"invalid tier", `{"venue_name":"仁和食堂","campus":"minglun","tier":"ssr","review_text":"好吃好吃。"}`},
		{"missing review", `{"venue_name":"仁和食堂","campus":"minglun","tier":"hang"}`},
		{"whitespace venue", `{"venue_name":"　 ","campus":"minglun","tier":"hang","review_text":"好吃好吃。"}`},
		{"whitespace review", `{"venue_name":"仁和食堂","campus":"minglun","tier":"hang","review_text":"　 \n"}`},
		{"short review", `{"venue_name":"仁和食堂","campus":"minglun","tier":"hang","review_text":"好"}`},
		{"placeholder venue", `{"venue_name":"test","campus":"minglun","tier":"hang","review_text":"这是一条能够公开展示的完整推荐理由。"}`},
		{"placeholder review", `{"venue_name":"仁和食堂","campus":"minglun","tier":"hang","review_text":"测试"}`},
		{"oversize price", fmt.Sprintf(`{"venue_name":"仁和食堂","campus":"minglun","tier":"hang","review_text":"好吃好吃。","price_reference":%q}`, strings.Repeat("贵", 201))},
		{"too many dishes", `{"venue_name":"仁和食堂","campus":"minglun","tier":"hang","review_text":"好吃好吃。","dishes":[{"name":"一"},{"name":"二"},{"name":"三"},{"name":"四"},{"name":"五"},{"name":"六"},{"name":"七"}]}`},
		{"empty dish name", `{"venue_name":"仁和食堂","campus":"minglun","tier":"hang","review_text":"好吃好吃。","dishes":[{"name":""}]}`},
	}
	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := createPostVia(t, server.URL, []byte(test.body), fmt.Sprintf("idem_invalid_%d", index))
			payload := readBody(t, response)
			wantCode := `"code":"INVALID_POST"`
			if strings.Contains(test.name, "placeholder") {
				wantCode = `"code":"PLACEHOLDER_POST"`
			}
			if response.StatusCode != http.StatusBadRequest || !bytes.Contains(payload, []byte(wantCode)) {
				t.Fatalf("status = %d, payload = %s", response.StatusCode, payload)
			}
		})
	}
	var count int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM food_posts`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("invalid creates wrote rows: count=%d, err=%v", count, err)
	}
}

func TestCreatePostRejectsInvalidImages(t *testing.T) {
	server, pool := newFoodServer(t)
	defer server.Close()
	defer pool.Close()
	cleanPosts(t, pool)

	tinyPNG := base64.StdEncoding.EncodeToString([]byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a})
	var validPNGBuffer bytes.Buffer
	validCanvas := image.NewRGBA(image.Rect(0, 0, 1, 1))
	validCanvas.Set(0, 0, color.RGBA{R: 255, A: 255})
	if err := png.Encode(&validPNGBuffer, validCanvas); err != nil {
		t.Fatal(err)
	}
	validPNG := base64.StdEncoding.EncodeToString(validPNGBuffer.Bytes())
	oversized := base64.StdEncoding.EncodeToString(make([]byte, 2<<20+1))
	sevenImages := make([]string, 7)
	for index := range sevenImages {
		sevenImages[index] = fmt.Sprintf(`{"content_type":"image/png","data":%q}`, tinyPNG)
	}
	tests := []struct {
		name string
		body string
	}{
		{"more than six images", fmt.Sprintf(`{"venue_name":"仁和食堂","campus":"minglun","tier":"hang","review_text":"好吃好吃。","images":[%s]}`, strings.Join(sevenImages, ","))},
		{"single image over 2MiB", fmt.Sprintf(`{"venue_name":"仁和食堂","campus":"minglun","tier":"hang","review_text":"好吃好吃。","images":[{"content_type":"image/png","data":%q}]}`, oversized)},
		{"unlisted content type", fmt.Sprintf(`{"venue_name":"仁和食堂","campus":"minglun","tier":"hang","review_text":"好吃好吃。","images":[{"content_type":"image/gif","data":%q}]}`, tinyPNG)},
		{"invalid base64", `{"venue_name":"仁和食堂","campus":"minglun","tier":"hang","review_text":"好吃好吃。","images":[{"content_type":"image/png","data":"%%%not-base64%%%"}]}`},
		{"declared type does not match bytes", fmt.Sprintf(`{"venue_name":"仁和食堂","campus":"minglun","tier":"hang","review_text":"好吃好吃。","images":[{"content_type":"image/jpeg","data":%q}]}`, validPNG)},
	}
	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := createPostVia(t, server.URL, []byte(test.body), fmt.Sprintf("idem_image_%d", index))
			payload := readBody(t, response)
			if response.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d, payload = %s", response.StatusCode, payload)
			}
		})
	}
	var count int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM food_posts`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("invalid images wrote rows: count=%d, err=%v", count, err)
	}
}

func TestCreatePostAcceptsAValidDecodedImage(t *testing.T) {
	server, pool := newFoodServer(t)
	defer server.Close()
	defer pool.Close()
	cleanPosts(t, pool)

	canvas := image.NewRGBA(image.Rect(0, 0, 2, 2))
	canvas.Set(0, 0, color.RGBA{R: 255, G: 80, B: 20, A: 255})
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, canvas); err != nil {
		t.Fatal(err)
	}
	body := []byte(fmt.Sprintf(`{"venue_name":"仁和食堂","campus":"minglun","tier":"hang","review_text":"图片和文字都是真实投稿。","images":[{"content_type":"image/png","data":%q}]}`, base64.StdEncoding.EncodeToString(encoded.Bytes())))
	payload := expectPostBody(t, createPostVia(t, server.URL, body, "idem_valid_image"), http.StatusOK)
	if !bytes.Contains(payload, []byte(`"images":["/api/v1/food/posts/`)) {
		t.Fatalf("valid image URL missing: %s", payload)
	}
}

func TestCreatePostStripsEmbeddedImageMetadataBeforePublishing(t *testing.T) {
	server, pool := newFoodServer(t)
	defer server.Close()
	defer pool.Close()
	cleanPosts(t, pool)

	canvas := image.NewRGBA(image.Rect(0, 0, 2, 1))
	canvas.Set(0, 0, color.RGBA{R: 20, G: 80, B: 255, A: 255})
	canvas.Set(1, 0, color.RGBA{R: 255, G: 80, B: 20, A: 255})
	var base bytes.Buffer
	if err := jpeg.Encode(&base, canvas, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	marker := []byte{
		'E', 'x', 'i', 'f', 0, 0,
		'I', 'I', 0x2a, 0x00, 0x08, 0x00, 0x00, 0x00,
		0x01, 0x00,
		0x12, 0x01, 0x03, 0x00, 0x01, 0x00, 0x00, 0x00, 0x06, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
	}
	marker = append(marker, []byte("GPSLatitude=34.797;GPSLongitude=114.307")...)
	app1Length := len(marker) + 2
	withMetadata := []byte{0xff, 0xd8, 0xff, 0xe1, byte(app1Length >> 8), byte(app1Length)}
	withMetadata = append(withMetadata, marker...)
	withMetadata = append(withMetadata, base.Bytes()[2:]...)

	body := []byte(fmt.Sprintf(`{"venue_name":"仁和食堂","campus":"minglun","tier":"hang","review_text":"照片公开前应移除位置元数据。","images":[{"content_type":"image/jpeg","data":%q}]}`, base64.StdEncoding.EncodeToString(withMetadata)))
	created := expectPostBody(t, createPostVia(t, server.URL, body, "idem_strip_metadata"), http.StatusOK)
	postID := decodeData(t, created)["post"].(map[string]any)["id"].(string)
	response := readPostVia(t, server.URL, http.MethodGet, "/api/v1/food/posts/"+postID+"/images/0")
	published := readBody(t, response)
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "image/jpeg" {
		t.Fatalf("published image status/type = %d/%q", response.StatusCode, response.Header.Get("Content-Type"))
	}
	if bytes.Contains(published, []byte("GPSLatitude")) || bytes.Contains(published, marker) {
		t.Fatal("published image retained uploader metadata")
	}
	decoded, _, err := image.Decode(bytes.NewReader(published))
	if err != nil {
		t.Fatalf("sanitized image is not decodable: %v", err)
	}
	if decoded.Bounds().Dx() != 1 || decoded.Bounds().Dy() != 2 {
		t.Fatalf("EXIF orientation was not applied before metadata stripping: %v", decoded.Bounds())
	}
}

func TestCreatePostIdempotencyReplayAndConflict(t *testing.T) {
	server, pool := newFoodServer(t)
	defer server.Close()
	defer pool.Close()
	cleanPosts(t, pool)

	body := []byte(`{"venue_name":"南苑餐厅","campus":"jinming","tier":"top","review_text":"大盘鸡拌面量大管饱。"}`)
	key := "idem_replay_key"
	first := expectPostBody(t, createPostVia(t, server.URL, body, key), http.StatusOK)
	second := expectPostBody(t, createPostVia(t, server.URL, body, key), http.StatusOK)
	if fmt.Sprint(decodeData(t, first)) != fmt.Sprint(decodeData(t, second)) {
		t.Fatalf("replay result diverged:\n%s\n%s", first, second)
	}
	var posts, ops int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM food_posts`).Scan(&posts); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM food_post_operations`).Scan(&ops); err != nil {
		t.Fatal(err)
	}
	if posts != 1 || ops != 1 {
		t.Fatalf("replay created extra rows: posts=%d ops=%d", posts, ops)
	}

	conflict := readBody(t, createPostVia(t, server.URL, []byte(`{"venue_name":"南苑餐厅","campus":"jinming","tier":"bad","review_text":"同一把 key 换了个请求。"}`), key))
	if !bytes.Contains(conflict, []byte(`"code":"IDEMPOTENCY_KEY_CONFLICT"`)) {
		t.Fatalf("conflicting replay = %s", conflict)
	}
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM food_posts`).Scan(&posts); err != nil || posts != 1 {
		t.Fatalf("conflict wrote rows: posts=%d, err=%v", posts, err)
	}

	missing := readBody(t, sendPost(t, server.URL, postCreateClientID, "active", postCreateSecret, http.MethodPost, "/api/v1/food/posts", body, map[string]string{"X-Actor-User-Id": postActorID, "X-Actor-Display-Name": postActorName}))
	if !bytes.Contains(missing, []byte(`"code":"INVALID_IDEMPOTENCY_KEY"`)) {
		t.Fatalf("missing idempotency key = %s", missing)
	}
	short := readBody(t, createPostVia(t, server.URL, body, "short"))
	if !bytes.Contains(short, []byte(`"code":"INVALID_IDEMPOTENCY_KEY"`)) {
		t.Fatalf("short idempotency key = %s", short)
	}
}

func TestCreatePostDailyCapRejectsFourthSubmission(t *testing.T) {
	server, pool := newFoodServer(t)
	defer server.Close()
	defer pool.Close()
	cleanPosts(t, pool)

	for index := 0; index < 3; index++ {
		body := []byte(fmt.Sprintf(`{"venue_name":"第%d食堂","campus":"longzihu","tier":"npc","review_text":"第 %d 条投稿。","price_reference":""}`, index, index))
		expectPostBody(t, createPostVia(t, server.URL, body, fmt.Sprintf("idem_cap_%d", index)), http.StatusOK)
	}
	fourth := readBody(t, createPostVia(t, server.URL, []byte(`{"venue_name":"第四食堂","campus":"longzihu","tier":"npc","review_text":"今天的第四条投稿。"}`), "idem_cap_4"))
	if !bytes.Contains(fourth, []byte(`"code":"DAILY_POST_CAP_REACHED"`)) {
		t.Fatalf("fourth submission = %s", fourth)
	}
	var count int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM food_posts WHERE author_user_id=$1`, postActorID).Scan(&count); err != nil || count != 3 {
		t.Fatalf("actor rows = %d, err=%v", count, err)
	}
	other := expectPostBody(t, sendPost(t, server.URL, postCreateClientID, "active", postCreateSecret, http.MethodPost, "/api/v1/food/posts", []byte(`{"venue_name":"别人食堂","campus":"longzihu","tier":"npc","review_text":"别的同学的投稿。"}`), map[string]string{"X-Actor-User-Id": postOtherActor, "X-Actor-Display-Name": "别人", "Idempotency-Key": "idem_cap_other"}), http.StatusOK)
	if !bytes.Contains(other, []byte(`"title":"别人食堂"`)) {
		t.Fatalf("other actor blocked by someone else's cap: %s", other)
	}
}

func TestMineReadsRequireActorAndFilterByAuthor(t *testing.T) {
	server, pool := newFoodServer(t)
	defer server.Close()
	defer pool.Close()
	cleanPosts(t, pool)

	bodyA := []byte(`{"venue_name":"甲餐厅","campus":"minglun","tier":"elite","review_text":"甲的投稿，要看得见。"}`)
	expectPostBody(t, createPostVia(t, server.URL, bodyA, "idem_mine_a"), http.StatusOK)
	bodyB := []byte(`{"venue_name":"乙餐厅","campus":"minglun","tier":"elite","review_text":"乙的投稿，甲的列表不该有。"}`)
	expectPostBody(t, sendPost(t, server.URL, postCreateClientID, "active", postCreateSecret, http.MethodPost, "/api/v1/food/posts", bodyB, map[string]string{"X-Actor-User-Id": postOtherActor, "X-Actor-Display-Name": "别人", "Idempotency-Key": "idem_mine_b"}), http.StatusOK)

	mine := expectPostBody(t, sendPost(t, server.URL, postReadClientID, "active", postReadSecret, http.MethodGet, "/api/v1/food/posts/mine", nil, map[string]string{"X-Actor-User-Id": postActorID}), http.StatusOK)
	if !bytes.Contains(mine, []byte(`"title":"甲餐厅"`)) || bytes.Contains(mine, []byte(`"title":"乙餐厅"`)) {
		t.Fatalf("mine list wrong: %s", mine)
	}

	noActor := readBody(t, readPostVia(t, server.URL, http.MethodGet, "/api/v1/food/posts/mine"))
	if !bytes.Contains(noActor, []byte(`"code":"INVALID_ACTOR"`)) {
		t.Fatalf("mine without actor = %s", noActor)
	}
	badActor := readBody(t, sendPost(t, server.URL, postReadClientID, "active", postReadSecret, http.MethodGet, "/api/v1/food/posts/mine", nil, map[string]string{"X-Actor-User-Id": "not-a-uuid"}))
	if !bytes.Contains(badActor, []byte(`"code":"INVALID_ACTOR"`)) {
		t.Fatalf("mine with bad actor = %s", badActor)
	}
}

func TestPostCredentialsAreIsolatedFromConsoleAndEachOther(t *testing.T) {
	server, pool := newFoodServer(t)
	defer server.Close()
	defer pool.Close()
	cleanPosts(t, pool)

	createBody := []byte(`{"venue_name":"隔离测试食堂","campus":"minglun","tier":"hang","review_text":"隔离测试投稿。"}`)
	cases := []struct {
		name     string
		clientID string
		keyID    string
		secret   string
		method   string
		path     string
		body     []byte
		headers  map[string]string
	}{
		{"console creds cannot create", "console-gateway", "active", serviceSecret, http.MethodPost, "/api/v1/food/posts", createBody, postCreateHeaders("idem_iso_1")},
		{"console creds cannot read posts", "console-gateway", "active", serviceSecret, http.MethodGet, "/api/v1/food/posts", nil, nil},
		{"read creds cannot create", postReadClientID, "active", postReadSecret, http.MethodPost, "/api/v1/food/posts", createBody, postCreateHeaders("idem_iso_2")},
		{"create creds cannot read posts", postCreateClientID, "active", postCreateSecret, http.MethodGet, "/api/v1/food/posts", nil, nil},
		{"read creds cannot use console workspace", postReadClientID, "active", postReadSecret, http.MethodGet, "/api/v1/workspace", nil, nil},
		{"create creds cannot use console commands", postCreateClientID, "active", postCreateSecret, http.MethodPost, "/api/v1/commands", []byte(`{}`), nil},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			response := sendPost(t, server.URL, test.clientID, test.keyID, test.secret, test.method, test.path, test.body, test.headers)
			payload := readBody(t, response)
			if response.StatusCode != http.StatusUnauthorized {
				t.Fatalf("status = %d, payload = %s", response.StatusCode, payload)
			}
		})
	}
	var count int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM food_posts`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("isolated credentials wrote rows: count=%d, err=%v", count, err)
	}
}
