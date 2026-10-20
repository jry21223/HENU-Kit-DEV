package food

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type postStoredImage struct {
	Position int    `json:"position"`
	URL      string `json:"url"`
}

type postEditWire struct {
	postCreateInput
	ID      string            `json:"id"`
	Version int               `json:"version"`
	Images  []postStoredImage `json:"images"`
}

type postUpdateInput struct {
	postCreateInput
	ExpectedVersion        int   `json:"expected_version"`
	RetainedImagePositions []int `json:"retained_image_positions"`
}

var (
	errAuthorPostMissing = errors.New("author post not found")
	errAuthorPostHidden  = errors.New("author post is hidden")
)

// lockAuthorPost checks ownership in Food, independently of Portal UI/Gateway.
// The shared row lock/version also serializes Console moderation and editing.
func lockAuthorPost(r *http.Request, tx pgx.Tx, id, actorID string) (postEditWire, string, time.Time, error) {
	post := postEditWire{ID: id, Images: []postStoredImage{}}
	post.Dishes = []postDishInput{}
	var displayName string
	var createdAt time.Time
	var hidden bool
	err := tx.QueryRow(r.Context(), `SELECT venue_name,campus,tier,review_text,price_reference,hours_reference,version,hidden,author_display_name,created_at FROM food_posts WHERE id=$1 AND author_user_id=$2 FOR UPDATE`, id, actorID).Scan(&post.VenueName, &post.Campus, &post.Tier, &post.ReviewText, &post.PriceReference, &post.HoursReference, &post.Version, &hidden, &displayName, &createdAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return post, "", createdAt, errAuthorPostMissing
	}
	if err != nil {
		return post, "", createdAt, err
	}
	if hidden {
		return post, "", createdAt, errAuthorPostHidden
	}
	post.Tier = postTierWireLabels[post.Tier]
	return post, displayName, createdAt, nil
}

func (h *service) getAuthorPost(w http.ResponseWriter, r *http.Request) {
	value, _ := postActor(r)
	id := chi.URLParam(r, "id")
	if _, err := uuid.Parse(id); err != nil {
		writeAuthorPostError(w, r, errAuthorPostMissing)
		return
	}
	tx, err := h.database.Begin(r.Context())
	if err != nil {
		writeAuthorPostError(w, r, err)
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()
	post, _, _, err := lockAuthorPost(r, tx, id, value.userID)
	if err != nil {
		writeAuthorPostError(w, r, err)
		return
	}
	rows, err := tx.Query(r.Context(), `SELECT name,price,reason FROM food_post_dishes WHERE post_id=$1 ORDER BY position`, id)
	if err != nil {
		writeAuthorPostError(w, r, err)
		return
	}
	post.Dishes, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (postDishInput, error) {
		var dish postDishInput
		err := row.Scan(&dish.Name, &dish.Price, &dish.Reason)
		return dish, err
	})
	if err != nil {
		writeAuthorPostError(w, r, err)
		return
	}
	rows, err = tx.Query(r.Context(), `SELECT position,sha256 FROM food_post_images WHERE post_id=$1 ORDER BY position`, id)
	if err != nil {
		writeAuthorPostError(w, r, err)
		return
	}
	post.Images, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (postStoredImage, error) {
		var image postStoredImage
		var hash string
		err := row.Scan(&image.Position, &hash)
		image.URL = postImageURL(id, image.Position, hash)
		return image, err
	})
	if err != nil {
		writeAuthorPostError(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		writeAuthorPostError(w, r, err)
		return
	}
	writeData(w, r, http.StatusOK, map[string]any{"post": post})
}

func (h *service) updateAuthorPost(w http.ResponseWriter, r *http.Request) {
	value, _ := postActor(r)
	id := chi.URLParam(r, "id")
	if _, err := uuid.Parse(id); err != nil {
		writeAuthorPostError(w, r, errAuthorPostMissing)
		return
	}
	tx, err := h.database.Begin(r.Context())
	if err != nil {
		writeAuthorPostError(w, r, err)
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()
	// Use the same actor lock/ledger as create so a key cannot cross commands.
	lock := h.postCreateClientID + "\n" + value.userID + "\n" + postRoleCreate
	if _, err = tx.Exec(r.Context(), `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, lock); err != nil {
		writeAuthorPostError(w, r, err)
		return
	}
	current, displayName, createdAt, err := lockAuthorPost(r, tx, id, value.userID)
	if err != nil {
		writeAuthorPostError(w, r, err)
		return
	}
	var input postUpdateInput
	body, ok := decode(w, r, &input)
	if !ok {
		return
	}
	key := r.Header.Get("Idempotency-Key")
	if len(key) < 8 || len(key) > 200 {
		writeError(w, r, 400, "INVALID_IDEMPOTENCY_KEY", "Idempotency-Key is required")
		return
	}
	digest := sha256.Sum256(append([]byte(r.Method+"\n"+r.URL.Path+"\n"), body...))
	hash := hex.EncodeToString(digest[:])
	var savedHash string
	var saved []byte
	err = tx.QueryRow(r.Context(), `SELECT request_hash,response FROM food_post_operations WHERE client_id=$1 AND actor_user_id=$2 AND idempotency_key=$3`, h.postCreateClientID, value.userID, key).Scan(&savedHash, &saved)
	if err == nil {
		if savedHash != hash {
			writeError(w, r, 409, "IDEMPOTENCY_KEY_CONFLICT", "Idempotency-Key was used for another request")
			return
		}
		var post foodPostWire
		if json.Unmarshal(saved, &post) != nil {
			writeAuthorPostError(w, r, errors.New("invalid replay"))
			return
		}
		writeData(w, r, 200, map[string]any{"post": post})
		return
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		writeAuthorPostError(w, r, err)
		return
	}
	normalizePostInput(&input.postCreateInput)
	if obviousPlaceholder(input.VenueName) || obviousPlaceholder(input.ReviewText) {
		writeError(w, r, 400, "PLACEHOLDER_POST", "Replace the test placeholder with a real venue and review")
		return
	}
	if !validPostFields(input.postCreateInput) || input.ExpectedVersion < 1 || input.RetainedImagePositions == nil || len(input.RetainedImagePositions)+len(input.Images) > maxPostImages {
		writeError(w, r, 400, "INVALID_POST", "Food post input is invalid")
		return
	}
	if current.Version != input.ExpectedVersion {
		writeError(w, r, 409, "VERSION_CONFLICT", "Food post changed; reload before saving")
		return
	}
	images := []decodedPostImage{}
	seen := map[int]bool{}
	for _, position := range input.RetainedImagePositions {
		if position < 0 || position >= maxPostImages || seen[position] {
			writeError(w, r, 400, "INVALID_POST", "retained image is invalid")
			return
		}
		seen[position] = true
		var image decodedPostImage
		err = tx.QueryRow(r.Context(), `SELECT content_type,bytes,sha256 FROM food_post_images WHERE post_id=$1 AND position=$2`, id, position).Scan(&image.ContentType, &image.Bytes, &image.SHA256)
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, r, 400, "INVALID_POST", "retained image is invalid")
			return
		}
		if err != nil {
			writeAuthorPostError(w, r, err)
			return
		}
		images = append(images, image)
	}
	added, ok := decodePostImages(w, r, input.Images)
	if !ok {
		return
	}
	images = append(images, added...)
	var totalPixels int64
	for _, stored := range images {
		config, _, decodeErr := image.DecodeConfig(bytes.NewReader(stored.Bytes))
		var valid bool
		if decodeErr == nil {
			totalPixels, valid = addPostImagePixels(config.Width, config.Height, totalPixels)
		}
		if !valid {
			writeError(w, r, 400, "INVALID_POST", "Food post image dimensions exceed the limit")
			return
		}
	}
	if _, err = tx.Exec(r.Context(), `UPDATE food_posts SET venue_name=$2,campus=$3,tier=$4,review_text=$5,price_reference=$6,hours_reference=$7,version=version+1,updated_at=now() WHERE id=$1`, id, input.VenueName, input.Campus, postTierLabels[input.Tier], input.ReviewText, input.PriceReference, input.HoursReference); err != nil {
		writeAuthorPostError(w, r, err)
		return
	}
	if _, err = tx.Exec(r.Context(), `DELETE FROM food_post_dishes WHERE post_id=$1`, id); err != nil {
		writeAuthorPostError(w, r, err)
		return
	}
	for position, dish := range input.Dishes {
		if _, err = tx.Exec(r.Context(), `INSERT INTO food_post_dishes(id,post_id,position,name,price,reason) VALUES($1,$2,$3,$4,$5,$6)`, uuid.New(), id, position, dish.Name, dish.Price, dish.Reason); err != nil {
			writeAuthorPostError(w, r, err)
			return
		}
	}
	if _, err = tx.Exec(r.Context(), `DELETE FROM food_post_images WHERE post_id=$1`, id); err != nil {
		writeAuthorPostError(w, r, err)
		return
	}
	for position, image := range images {
		if _, err = tx.Exec(r.Context(), `INSERT INTO food_post_images(id,post_id,position,content_type,byte_size,sha256,bytes) VALUES($1,$2,$3,$4,$5,$6,$7)`, uuid.New(), id, position, image.ContentType, len(image.Bytes), image.SHA256, image.Bytes); err != nil {
			writeAuthorPostError(w, r, err)
			return
		}
	}
	post := buildPostWire(uuid.MustParse(id), input.postCreateInput, displayName, createdAt, len(images))
	for position, image := range images {
		post.Images[position] = postImageURL(id, position, image.SHA256)
	}
	encoded, err := json.Marshal(post)
	if err != nil {
		writeAuthorPostError(w, r, err)
		return
	}
	if _, err = tx.Exec(r.Context(), `INSERT INTO food_post_operations(id,client_id,actor_user_id,idempotency_key,request_hash,request_id,post_id,state,response) VALUES($1,$2,$3,$4,$5,$6,$7,'succeeded',$8)`, uuid.New(), h.postCreateClientID, value.userID, key, hash, requestID(r), id, encoded); err != nil {
		writeAuthorPostError(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		writeAuthorPostError(w, r, err)
		return
	}
	writeData(w, r, 200, map[string]any{"post": post})
}

func writeAuthorPostError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, errAuthorPostMissing):
		writeError(w, r, 404, "POST_NOT_FOUND", "Food post does not exist")
	case errors.Is(err, errAuthorPostHidden):
		writeError(w, r, 409, "POST_HIDDEN", "Food post is hidden and cannot be edited")
	default:
		writeError(w, r, 503, "DEPENDENCY_UNAVAILABLE", "Food post editing is unavailable")
	}
}

func postImageURL(id string, position int, hash string) string {
	return fmt.Sprintf(postImagePathFormat, id, position) + "?v=" + hash
}
