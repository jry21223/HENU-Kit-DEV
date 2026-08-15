package food

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"henukit.dev/food/internal/contract"
)

// Food Post credentials are a separate ring from the Console ring: the three
// roles (console / food-post-create / food-post-read) are not interchangeable.
const (
	postRoleCreate = "food-post-create"
	postRoleRead   = "food-post-read"

	postBodyLimit     = 20 << 20
	maxPostImages     = 6
	maxPostImageBytes = 2 << 20
	dailyPostCap      = 3

	postImagePathFormat = "/api/v1/food/posts/%s/images/%d"
	postImageCache      = "public, max-age=31536000, immutable"
)

var (
	errPostIdempotencyConflict = errors.New("post idempotency key conflict")
	errDailyPostCap            = errors.New("daily post cap reached")
)

var postTierLabels = map[string]string{"hang": "夯", "top": "顶级", "elite": "人上人", "npc": "NPC", "bad": "拉完了"}

type foodPostBlockWire struct {
	Type  string   `json:"type"`
	Text  string   `json:"text,omitempty"`
	Items []string `json:"items,omitempty"`
	Src   string   `json:"src,omitempty"`
}
type foodPostShopWire struct {
	Name string  `json:"name"`
	Lat  float64 `json:"lat"`
	Lng  float64 `json:"lng"`
}
type foodPostWire struct {
	ID      string              `json:"id"`
	Campus  string              `json:"campus"`
	Title   string              `json:"title"`
	Excerpt string              `json:"excerpt"`
	Blocks  []foodPostBlockWire `json:"blocks"`
	Author  string              `json:"author"`
	Likes   int                 `json:"likes"`
	Stars   float64             `json:"stars"`
	Tags    []string            `json:"tags"`
	Shop    foodPostShopWire    `json:"shop"`
	Time    string              `json:"time"`
	Hidden  bool                `json:"hidden"`
	Images  []string            `json:"images"`
}
type foodVenueSummary struct {
	ID     string  `json:"id"`
	Name   string  `json:"name"`
	Rating float64 `json:"rating"`
	Tier   string  `json:"tier"`
	Campus string  `json:"campus"`
}

type postCreateInput struct {
	VenueName      string           `json:"venue_name"`
	Campus         string           `json:"campus"`
	Tier           string           `json:"tier"`
	ReviewText     string           `json:"review_text"`
	PriceReference string           `json:"price_reference"`
	HoursReference string           `json:"hours_reference"`
	Dishes         []postDishInput  `json:"dishes"`
	Images         []postImageInput `json:"images"`
}
type postDishInput struct {
	Name   string `json:"name"`
	Price  string `json:"price"`
	Reason string `json:"reason"`
}
type postImageInput struct {
	ContentType string `json:"content_type"`
	Data        string `json:"data"`
}
type decodedPostImage struct {
	ContentType string
	Bytes       []byte
	SHA256      string
}

// postCredentials returns the key ring bound to one Food Post role.
func (h *service) postCredentials(role string) (string, map[string]string) {
	if role == postRoleCreate {
		return h.postCreateClientID, h.postCreateKeys
	}
	return h.postReadClientID, h.postReadKeys
}

// postAuthenticate verifies the dedicated Food Post credential ring for one
// role. It uses the same five-line canonical string as the Console
// authenticate middleware; X-Actor-* headers are trusted signed-request
// headers, never an extra canonical line. Create additionally binds the actor
// UUID and display-name snapshot.
func (h *service) postAuthenticate(role string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			clientID, keys := h.postCredentials(role)
			if clientID == "" {
				writeError(w, r, http.StatusUnauthorized, "INVALID_SERVICE_AUTH", "service credentials are invalid")
				return
			}
			basicClientID, basicSecret, basic := r.BasicAuth()
			secret, keyKnown := keys[r.Header.Get("X-Key-Id")]
			if !basic || basicClientID != clientID || r.Header.Get("X-Service-Id") != clientID || !keyKnown || !hmac.Equal([]byte(secret), []byte(basicSecret)) {
				writeError(w, r, http.StatusUnauthorized, "INVALID_SERVICE_AUTH", "service credentials are invalid")
				return
			}
			timestamp, err := strconv.ParseInt(r.Header.Get("X-Timestamp"), 10, 64)
			if err != nil || abs(h.now().Unix()-timestamp) > int64(nonceTTL/time.Second) {
				writeError(w, r, http.StatusUnauthorized, "INVALID_SERVICE_AUTH", "service timestamp is invalid")
				return
			}
			nonce := r.Header.Get("X-Nonce")
			decoded, err := base64.RawURLEncoding.DecodeString(nonce)
			if err != nil || len(decoded) != 24 {
				writeError(w, r, http.StatusUnauthorized, "INVALID_SERVICE_AUTH", "service nonce is invalid")
				return
			}
			body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, postBodyLimit))
			if err != nil {
				writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "request body is too large")
				return
			}
			r.Body = io.NopCloser(strings.NewReader(string(body)))
			digest := sha256.Sum256(body)
			canonical := strings.Join([]string{r.Method, r.URL.RequestURI(), r.Header.Get("X-Timestamp"), nonce, hex.EncodeToString(digest[:])}, "\n")
			mac := hmac.New(sha256.New, []byte(secret))
			_, _ = mac.Write([]byte(canonical))
			if !hmac.Equal([]byte(r.Header.Get("X-Signature")), []byte(base64.RawURLEncoding.EncodeToString(mac.Sum(nil)))) {
				writeError(w, r, http.StatusUnauthorized, "INVALID_SERVICE_AUTH", "service signature is invalid")
				return
			}
			accepted, err := h.redis.SetNX(r.Context(), "food:nonce:"+clientID+":"+nonce, "1", nonceTTL).Result()
			if err != nil {
				writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "nonce store is unavailable")
				return
			}
			if !accepted {
				writeError(w, r, http.StatusConflict, "REPLAY_DETECTED", "service nonce was already used")
				return
			}
			if role == postRoleCreate {
				userID := r.Header.Get("X-Actor-User-Id")
				if _, err := uuid.Parse(userID); err != nil {
					writeError(w, r, http.StatusUnauthorized, "INVALID_ACTOR", "actor context is invalid")
					return
				}
				displayName := strings.TrimSpace(r.Header.Get("X-Actor-Display-Name"))
				if displayName == "" || utf8.RuneCountInString(displayName) > 120 {
					writeError(w, r, http.StatusUnauthorized, "INVALID_ACTOR", "actor display name is invalid")
					return
				}
				r.Header.Set("X-Actor-Display-Name", displayName)
				next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), actorKey, actor{userID: userID})))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// postActorRequired binds the signed X-Actor-User-Id for the mine read. The
// public reads never pass through it.
func (h *service) postActorRequired(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID := r.Header.Get("X-Actor-User-Id")
		if _, err := uuid.Parse(userID); err != nil {
			writeError(w, r, http.StatusUnauthorized, "INVALID_ACTOR", "actor context is invalid")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), actorKey, actor{userID: userID})))
	})
}

func postActor(r *http.Request) (actor, bool) {
	value, ok := r.Context().Value(actorKey).(actor)
	return value, ok && value.userID != ""
}

func validCampus(campus string) bool {
	switch campus {
	case "minglun", "jinming", "longzihu":
		return true
	}
	return false
}

func (h *service) createPost(w http.ResponseWriter, r *http.Request) {
	value, ok := postActor(r)
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "INVALID_ACTOR", "actor context is invalid")
		return
	}
	var input postCreateInput
	body, ok := decode(w, r, &input)
	if !ok {
		return
	}
	if !validPostFields(input) {
		writeError(w, r, http.StatusBadRequest, "INVALID_POST", "Food post input is invalid")
		return
	}
	images, ok := decodePostImages(w, r, input.Images)
	if !ok {
		return
	}
	key := r.Header.Get("Idempotency-Key")
	if len(key) < 8 || len(key) > 200 {
		writeError(w, r, http.StatusBadRequest, "INVALID_IDEMPOTENCY_KEY", "Idempotency-Key is required")
		return
	}
	digest := sha256.Sum256(append([]byte(r.Method+"\n"+contract.CreatePostRoute+"\n"), body...))
	post, err := h.storePost(r, value, key, input, images, hex.EncodeToString(digest[:]))
	if errors.Is(err, errPostIdempotencyConflict) {
		writeError(w, r, http.StatusConflict, "IDEMPOTENCY_KEY_CONFLICT", "Idempotency-Key was used for another request")
		return
	}
	if errors.Is(err, errDailyPostCap) {
		writeError(w, r, http.StatusTooManyRequests, "DAILY_POST_CAP_REACHED", "daily Food post limit reached")
		return
	}
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Food post creation is unavailable")
		return
	}
	writeData(w, r, http.StatusOK, map[string]any{"post": post})
}

func validPostFields(input postCreateInput) bool {
	if utf8.RuneCountInString(input.VenueName) < 1 || utf8.RuneCountInString(input.VenueName) > 160 {
		return false
	}
	if !validCampus(input.Campus) {
		return false
	}
	if _, ok := postTierLabels[input.Tier]; !ok {
		return false
	}
	if utf8.RuneCountInString(input.ReviewText) < 2 || utf8.RuneCountInString(input.ReviewText) > 2000 {
		return false
	}
	if utf8.RuneCountInString(input.PriceReference) > 200 || utf8.RuneCountInString(input.HoursReference) > 200 {
		return false
	}
	if len(input.Dishes) > maxPostImages {
		return false
	}
	for _, dish := range input.Dishes {
		if utf8.RuneCountInString(dish.Name) < 1 || utf8.RuneCountInString(dish.Name) > 80 || utf8.RuneCountInString(dish.Price) > 40 || utf8.RuneCountInString(dish.Reason) > 200 {
			return false
		}
	}
	return true
}

func decodePostImages(w http.ResponseWriter, r *http.Request, inputs []postImageInput) ([]decodedPostImage, bool) {
	if len(inputs) > maxPostImages {
		writeError(w, r, http.StatusBadRequest, "INVALID_POST", "Food post image count exceeds the limit")
		return nil, false
	}
	images := make([]decodedPostImage, 0, len(inputs))
	for _, item := range inputs {
		if item.ContentType != "image/jpeg" && item.ContentType != "image/png" && item.ContentType != "image/webp" {
			writeError(w, r, http.StatusBadRequest, "INVALID_POST", "Food post image content type is invalid")
			return nil, false
		}
		bytes, err := base64.StdEncoding.DecodeString(item.Data)
		if err != nil || len(bytes) == 0 || len(bytes) > maxPostImageBytes {
			writeError(w, r, http.StatusBadRequest, "INVALID_POST", "Food post image bytes are invalid")
			return nil, false
		}
		sum := sha256.Sum256(bytes)
		images = append(images, decodedPostImage{ContentType: item.ContentType, Bytes: bytes, SHA256: hex.EncodeToString(sum[:])})
	}
	return images, true
}

// storePost creates one public post inside a transaction. The advisory lock is
// taken on the client+actor dimension so the daily cap and the idempotency
// ledger stay correct under concurrent creates from the same actor.
func (h *service) storePost(r *http.Request, value actor, key string, input postCreateInput, images []decodedPostImage, hash string) (foodPostWire, error) {
	tx, err := h.database.BeginTx(r.Context(), pgx.TxOptions{})
	if err != nil {
		return foodPostWire{}, err
	}
	defer func() { _ = tx.Rollback(r.Context()) }()
	lock := strings.Join([]string{h.postCreateClientID, value.userID, postRoleCreate}, "\n")
	if _, err = tx.Exec(r.Context(), `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, lock); err != nil {
		return foodPostWire{}, err
	}
	var storedHash, state string
	var storedResponse []byte
	err = tx.QueryRow(r.Context(), `SELECT request_hash,state,COALESCE(response,'null'::jsonb) FROM food_post_operations WHERE client_id=$1 AND actor_user_id=$2 AND idempotency_key=$3`, h.postCreateClientID, value.userID, key).Scan(&storedHash, &state, &storedResponse)
	if err == nil {
		if storedHash != hash {
			return foodPostWire{}, errPostIdempotencyConflict
		}
		if err = tx.Commit(r.Context()); err != nil {
			return foodPostWire{}, err
		}
		if state != "succeeded" {
			return foodPostWire{}, errors.New("stored Food post creation failed")
		}
		var post foodPostWire
		if json.Unmarshal(storedResponse, &post) != nil {
			return foodPostWire{}, errors.New("stored Food post creation is invalid")
		}
		return post, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return foodPostWire{}, err
	}
	var count int
	if err = tx.QueryRow(r.Context(), `SELECT count(*) FROM food_posts WHERE author_user_id=$1 AND (created_at AT TIME ZONE 'Asia/Shanghai')::date = (now() AT TIME ZONE 'Asia/Shanghai')::date`, value.userID).Scan(&count); err != nil {
		return foodPostWire{}, err
	}
	if count >= dailyPostCap {
		return foodPostWire{}, errDailyPostCap
	}
	id := uuid.New()
	displayName := r.Header.Get("X-Actor-Display-Name")
	var createdAt time.Time
	tierLabel := postTierLabels[input.Tier]
	if err = tx.QueryRow(r.Context(), `INSERT INTO food_posts(id,venue_name,campus,tier,review_text,price_reference,hours_reference,author_user_id,author_display_name,hidden) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,false) RETURNING created_at`, id, input.VenueName, input.Campus, tierLabel, input.ReviewText, input.PriceReference, input.HoursReference, value.userID, displayName).Scan(&createdAt); err != nil {
		return foodPostWire{}, err
	}
	for position, dish := range input.Dishes {
		if _, err = tx.Exec(r.Context(), `INSERT INTO food_post_dishes(id,post_id,position,name,price,reason) VALUES($1,$2,$3,$4,$5,$6)`, uuid.New(), id, position, dish.Name, dish.Price, dish.Reason); err != nil {
			return foodPostWire{}, err
		}
	}
	for position, image := range images {
		if _, err = tx.Exec(r.Context(), `INSERT INTO food_post_images(id,post_id,position,content_type,byte_size,sha256,bytes) VALUES($1,$2,$3,$4,$5,$6,$7)`, uuid.New(), id, position, image.ContentType, len(image.Bytes), image.SHA256, image.Bytes); err != nil {
			return foodPostWire{}, err
		}
	}
	wire := buildPostWire(id, input, displayName, createdAt, len(images))
	encoded, err := json.Marshal(wire)
	if err != nil {
		return foodPostWire{}, err
	}
	if _, err = tx.Exec(r.Context(), `INSERT INTO food_post_operations(id,client_id,actor_user_id,idempotency_key,request_hash,request_id,post_id,state,response) VALUES($1,$2,$3,$4,$5,$6,$7,'succeeded',$8)`, uuid.New(), h.postCreateClientID, value.userID, key, hash, requestID(r), id, encoded); err != nil {
		return foodPostWire{}, err
	}
	if err = tx.Commit(r.Context()); err != nil {
		return foodPostWire{}, err
	}
	return wire, nil
}

func buildPostWire(id uuid.UUID, input postCreateInput, displayName string, createdAt time.Time, imageCount int) foodPostWire {
	blocks := make([]foodPostBlockWire, 0, 4)
	if input.PriceReference != "" {
		blocks = append(blocks, foodPostBlockWire{Type: "p", Text: input.PriceReference})
	}
	if input.HoursReference != "" {
		blocks = append(blocks, foodPostBlockWire{Type: "p", Text: "营业时间参考：" + input.HoursReference})
	}
	if len(input.Dishes) > 0 {
		blocks = append(blocks, foodPostBlockWire{Type: "h2", Text: "点什么"})
		items := make([]string, 0, len(input.Dishes))
		for _, dish := range input.Dishes {
			item := dish.Name
			if dish.Price != "" {
				item += "：" + dish.Price
			}
			if dish.Reason != "" {
				item += " — " + dish.Reason
			}
			items = append(items, item)
		}
		blocks = append(blocks, foodPostBlockWire{Type: "list", Items: items})
	}
	imagePaths := make([]string, 0, imageCount)
	for position := 0; position < imageCount; position++ {
		imagePaths = append(imagePaths, fmt.Sprintf(postImagePathFormat, id, position))
	}
	return foodPostWire{
		ID:      id.String(),
		Campus:  input.Campus,
		Title:   input.VenueName,
		Excerpt: input.ReviewText,
		Blocks:  blocks,
		Author:  displayName,
		Likes:   0,
		Stars:   0,
		Tags:    []string{postTierLabels[input.Tier]},
		Shop:    foodPostShopWire{Name: input.VenueName, Lat: 0, Lng: 0},
		Time:    createdAt.UTC().Format(time.RFC3339),
		Hidden:  false,
		Images:  imagePaths,
	}
}

func (h *service) listPosts(w http.ResponseWriter, r *http.Request) {
	campus := r.URL.Query().Get("campus")
	if campus != "" && !validCampus(campus) {
		writeError(w, r, http.StatusBadRequest, "INVALID_CAMPUS", "campus is invalid")
		return
	}
	posts, err := h.loadPosts(r, campus, "")
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Food posts are unavailable")
		return
	}
	writeData(w, r, http.StatusOK, map[string]any{"posts": posts})
}

func (h *service) myPosts(w http.ResponseWriter, r *http.Request) {
	value, ok := postActor(r)
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "INVALID_ACTOR", "actor context is invalid")
		return
	}
	posts, err := h.loadPosts(r, "", value.userID)
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Food posts are unavailable")
		return
	}
	writeData(w, r, http.StatusOK, map[string]any{"posts": posts})
}

func (h *service) loadPosts(r *http.Request, campus, authorUserID string) ([]foodPostWire, error) {
	query := `SELECT id,campus,venue_name,review_text,price_reference,hours_reference,author_display_name,tier,created_at FROM food_posts WHERE hidden=false`
	args := []any{}
	if campus != "" {
		query += ` AND campus=$` + strconv.Itoa(len(args)+1)
		args = append(args, campus)
	}
	if authorUserID != "" {
		query += ` AND author_user_id=$` + strconv.Itoa(len(args)+1)
		args = append(args, authorUserID)
	}
	query += ` ORDER BY created_at DESC`
	rows, err := h.database.Query(r.Context(), query, args...)
	if err != nil {
		return nil, err
	}
	type row struct {
		id, campus, venueName, reviewText, priceReference, hoursReference, displayName, tier string
		createdAt                                                                            time.Time
	}
	posts := make([]foodPostWire, 0)
	ids := []string{}
	for rows.Next() {
		var item row
		if err := rows.Scan(&item.id, &item.campus, &item.venueName, &item.reviewText, &item.priceReference, &item.hoursReference, &item.displayName, &item.tier, &item.createdAt); err != nil {
			rows.Close()
			return nil, err
		}
		blocks := make([]foodPostBlockWire, 0, 4)
		if item.priceReference != "" {
			blocks = append(blocks, foodPostBlockWire{Type: "p", Text: item.priceReference})
		}
		if item.hoursReference != "" {
			blocks = append(blocks, foodPostBlockWire{Type: "p", Text: "营业时间参考：" + item.hoursReference})
		}
		posts = append(posts, foodPostWire{
			ID:      item.id,
			Campus:  item.campus,
			Title:   item.venueName,
			Excerpt: item.reviewText,
			Blocks:  blocks,
			Author:  item.displayName,
			Likes:   0,
			Stars:   0,
			Tags:    []string{item.tier},
			Shop:    foodPostShopWire{Name: item.venueName, Lat: 0, Lng: 0},
			Time:    item.createdAt.UTC().Format(time.RFC3339),
			Hidden:  false,
			Images:  []string{},
		})
		ids = append(ids, item.id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	if len(ids) == 0 {
		return posts, nil
	}
	dishesByPost := map[string][]string{}
	dishRows, err := h.database.Query(r.Context(), `SELECT post_id::text,position,name,price,reason FROM food_post_dishes WHERE post_id::text = ANY($1) ORDER BY post_id,position`, ids)
	if err != nil {
		return nil, err
	}
	for dishRows.Next() {
		var postID, name, price, reason string
		var position int
		if err := dishRows.Scan(&postID, &position, &name, &price, &reason); err != nil {
			dishRows.Close()
			return nil, err
		}
		item := name
		if price != "" {
			item += "：" + price
		}
		if reason != "" {
			item += " — " + reason
		}
		dishesByPost[postID] = append(dishesByPost[postID], item)
	}
	if err := dishRows.Err(); err != nil {
		dishRows.Close()
		return nil, err
	}
	dishRows.Close()
	imageRows, err := h.database.Query(r.Context(), `SELECT post_id::text,position FROM food_post_images WHERE post_id::text = ANY($1) ORDER BY post_id,position`, ids)
	if err != nil {
		return nil, err
	}
	imagesByPost := map[string][]string{}
	for imageRows.Next() {
		var postID string
		var position int
		if err := imageRows.Scan(&postID, &position); err != nil {
			imageRows.Close()
			return nil, err
		}
		imagesByPost[postID] = append(imagesByPost[postID], fmt.Sprintf(postImagePathFormat, postID, position))
	}
	if err := imageRows.Err(); err != nil {
		imageRows.Close()
		return nil, err
	}
	imageRows.Close()
	for index := range posts {
		if items := dishesByPost[posts[index].ID]; len(items) > 0 {
			posts[index].Blocks = append(posts[index].Blocks, foodPostBlockWire{Type: "h2", Text: "点什么"}, foodPostBlockWire{Type: "list", Items: items})
		}
		if paths := imagesByPost[posts[index].ID]; len(paths) > 0 {
			posts[index].Images = paths
		}
	}
	return posts, nil
}

func (h *service) getPost(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, err := uuid.Parse(id); err != nil {
		writeError(w, r, http.StatusNotFound, "POST_NOT_FOUND", "Food post does not exist")
		return
	}
	post, found, err := h.loadPost(r, id)
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Food post is unavailable")
		return
	}
	if !found {
		writeError(w, r, http.StatusNotFound, "POST_NOT_FOUND", "Food post does not exist")
		return
	}
	writeData(w, r, http.StatusOK, map[string]any{"post": post, "comments": []any{}})
}

func (h *service) loadPost(r *http.Request, id string) (foodPostWire, bool, error) {
	var campus, venueName, reviewText, priceReference, hoursReference, displayName, tier string
	var createdAt time.Time
	err := h.database.QueryRow(r.Context(), `SELECT campus,venue_name,review_text,price_reference,hours_reference,author_display_name,tier,created_at FROM food_posts WHERE id=$1 AND hidden=false`, id).Scan(&campus, &venueName, &reviewText, &priceReference, &hoursReference, &displayName, &tier, &createdAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return foodPostWire{}, false, nil
	}
	if err != nil {
		return foodPostWire{}, false, err
	}
	blocks := make([]foodPostBlockWire, 0, 4)
	if priceReference != "" {
		blocks = append(blocks, foodPostBlockWire{Type: "p", Text: priceReference})
	}
	if hoursReference != "" {
		blocks = append(blocks, foodPostBlockWire{Type: "p", Text: "营业时间参考：" + hoursReference})
	}
	dishRows, err := h.database.Query(r.Context(), `SELECT name,price,reason FROM food_post_dishes WHERE post_id=$1 ORDER BY position`, id)
	if err != nil {
		return foodPostWire{}, false, err
	}
	dishItems := []string{}
	for dishRows.Next() {
		var name, price, reason string
		if err := dishRows.Scan(&name, &price, &reason); err != nil {
			dishRows.Close()
			return foodPostWire{}, false, err
		}
		item := name
		if price != "" {
			item += "：" + price
		}
		if reason != "" {
			item += " — " + reason
		}
		dishItems = append(dishItems, item)
	}
	if err := dishRows.Err(); err != nil {
		dishRows.Close()
		return foodPostWire{}, false, err
	}
	dishRows.Close()
	if len(dishItems) > 0 {
		blocks = append(blocks, foodPostBlockWire{Type: "h2", Text: "点什么"}, foodPostBlockWire{Type: "list", Items: dishItems})
	}
	imageRows, err := h.database.Query(r.Context(), `SELECT position FROM food_post_images WHERE post_id=$1 ORDER BY position`, id)
	if err != nil {
		return foodPostWire{}, false, err
	}
	imagePaths := []string{}
	for imageRows.Next() {
		var position int
		if err := imageRows.Scan(&position); err != nil {
			imageRows.Close()
			return foodPostWire{}, false, err
		}
		imagePaths = append(imagePaths, fmt.Sprintf(postImagePathFormat, id, position))
	}
	if err := imageRows.Err(); err != nil {
		imageRows.Close()
		return foodPostWire{}, false, err
	}
	imageRows.Close()
	return foodPostWire{
		ID:      id,
		Campus:  campus,
		Title:   venueName,
		Excerpt: reviewText,
		Blocks:  blocks,
		Author:  displayName,
		Likes:   0,
		Stars:   0,
		Tags:    []string{tier},
		Shop:    foodPostShopWire{Name: venueName, Lat: 0, Lng: 0},
		Time:    createdAt.UTC().Format(time.RFC3339),
		Hidden:  false,
		Images:  imagePaths,
	}, true, nil
}

func (h *service) getPostImage(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, err := uuid.Parse(id); err != nil {
		writeError(w, r, http.StatusNotFound, "IMAGE_NOT_FOUND", "Food post image does not exist")
		return
	}
	position, err := strconv.Atoi(chi.URLParam(r, "position"))
	if err != nil || position < 0 || position > maxPostImages-1 {
		writeError(w, r, http.StatusNotFound, "IMAGE_NOT_FOUND", "Food post image does not exist")
		return
	}
	var contentType, sha string
	var bytes []byte
	err = h.database.QueryRow(r.Context(), `SELECT content_type,sha256,bytes FROM food_post_images WHERE post_id=$1 AND position=$2`, id, position).Scan(&contentType, &sha, &bytes)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, r, http.StatusNotFound, "IMAGE_NOT_FOUND", "Food post image does not exist")
		return
	}
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Food post image is unavailable")
		return
	}
	w.Header().Set("ETag", `"`+sha+`"`)
	w.Header().Set("Cache-Control", postImageCache)
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", "inline")
	w.Header().Set("Content-Length", strconv.Itoa(len(bytes)))
	if strings.Contains(r.Header.Get("If-None-Match"), sha) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(bytes)
}

func (h *service) listVenues(w http.ResponseWriter, r *http.Request) {
	campus := r.URL.Query().Get("campus")
	if campus == "" {
		writeError(w, r, http.StatusBadRequest, "CAMPUS_REQUIRED", "campus query parameter is required")
		return
	}
	if !validCampus(campus) {
		writeError(w, r, http.StatusBadRequest, "INVALID_CAMPUS", "campus is invalid")
		return
	}
	rows, err := h.database.Query(r.Context(), `SELECT venue_name FROM food_posts WHERE hidden=false AND campus=$1 GROUP BY venue_name ORDER BY venue_name`, campus)
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Food venues are unavailable")
		return
	}
	defer rows.Close()
	venues := []foodVenueSummary{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Food venues are unavailable")
			return
		}
		venues = append(venues, foodVenueSummary{ID: venueSlug(campus, name), Name: name, Rating: 0, Tier: "standard", Campus: campus})
	}
	if err := rows.Err(); err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Food venues are unavailable")
		return
	}
	writeData(w, r, http.StatusOK, map[string]any{"campus": campus, "venues": venues})
}

// venueSlug keeps the exact portal-api venueID slug algorithm (lowercase
// alphanumerics, everything else to "-", duplicate dashes collapsed once, then
// trimmed).
func venueSlug(campus, name string) string {
	safe := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= 'A' && r <= 'Z':
			return r + ('a' - 'A')
		case r >= '0' && r <= '9':
			return r
		default:
			return '-'
		}
	}, campus+"-"+name)
	return strings.Trim(strings.ReplaceAll(safe, "--", "-"), "-")
}
