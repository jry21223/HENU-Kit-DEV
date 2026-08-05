package food

import (
	"database/sql"
	"fmt"
	"strconv"
)

// ImagePathPrefix and imagePathFormat build the URL a browser fetches a stored
// photo from. Posts carry these paths in their images field so the Portal keeps
// treating every photo as a plain URL.
const imagePathFormat = "/api/v1/food/posts/%s/images/%d"

// StoredImage is one photo attached to a post, served straight from Postgres.
type StoredImage struct {
	ContentType string
	SHA256      string
	Bytes       []byte
}

// ImagePath returns the URL a stored photo is served from.
func ImagePath(postID string, position int) string {
	return fmt.Sprintf(imagePathFormat, postID, position)
}

// attachStoredImages replaces the images of every post that has stored photos.
//
// A post keeps whatever its images column held when it has no stored photos, so
// posts whose photos are hosted elsewhere are unaffected.
func (db *PortalDB) attachStoredImages(posts []Post) error {
	if len(posts) == 0 {
		return nil
	}

	ids := make([]string, 0, len(posts))
	for _, post := range posts {
		ids = append(ids, post.ID)
	}

	rows, err := db.conn.Query(`
		SELECT post_id, position
		FROM portal_food_post_images
		WHERE post_id = ANY($1)
		ORDER BY post_id, position
	`, pqStringArray(ids))
	if err != nil {
		return fmt.Errorf("query post images: %w", err)
	}
	defer rows.Close()

	stored := make(map[string][]string, len(posts))
	for rows.Next() {
		var (
			postID   string
			position int
		)
		if err := rows.Scan(&postID, &position); err != nil {
			return fmt.Errorf("scan post image: %w", err)
		}
		stored[postID] = append(stored[postID], ImagePath(postID, position))
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate post images: %w", err)
	}

	for i := range posts {
		if paths, ok := stored[posts[i].ID]; ok {
			posts[i].Images = paths
		}
	}
	return nil
}

// GetImage returns one stored photo, or nil when the post has no photo at that
// position.
func (db *PortalDB) GetImage(postID string, position int) (*StoredImage, error) {
	var image StoredImage
	err := db.conn.QueryRow(`
		SELECT content_type, sha256, bytes
		FROM portal_food_post_images
		WHERE post_id = $1 AND position = $2
	`, postID, position).Scan(&image.ContentType, &image.SHA256, &image.Bytes)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query post image: %w", err)
	}
	return &image, nil
}

// pqStringArray renders a Postgres text[] literal for `= ANY($1)`.
//
// The driver cannot bind a Go slice directly and portal-api deliberately keeps
// its dependency surface to database/sql, so the literal is built here. Values
// are post IDs read back from our own table, but they are quoted and escaped
// anyway rather than trusting that.
func pqStringArray(values []string) string {
	literal := make([]byte, 0, len(values)*16+2)
	literal = append(literal, '{')
	for i, value := range values {
		if i > 0 {
			literal = append(literal, ',')
		}
		literal = append(literal, []byte(strconv.Quote(value))...)
	}
	return string(append(literal, '}'))
}
