package food

import (
	"database/sql"
	"encoding/json"
	"fmt"
)

// PortalDB reads from the portal_food_* tables (MySQL or PostgreSQL).
type PortalDB struct {
	conn *sql.DB
}

// NewPortalDB creates a PortalDB.
func NewPortalDB(conn *sql.DB) *PortalDB {
	return &PortalDB{conn: conn}
}

// GetPosts returns food posts, optionally filtered by campus.
func (db *PortalDB) GetPosts(campusFilter string) ([]Post, error) {
	query := `
		SELECT id, campus, title, excerpt, blocks, author, likes, stars, tags,
		       shop_name, shop_lat, shop_lng, time, hidden, images
		FROM portal_food_posts
		WHERE hidden = 0
	`
	var args []any
	if campusFilter != "" {
		query += " AND campus = ?"
		args = append(args, campusFilter)
	}
	query += " ORDER BY created_at DESC"

	rows, err := db.conn.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query posts: %w", err)
	}
	defer rows.Close()

	var posts []Post
	for rows.Next() {
		var (
			p          Post
			blocksJSON []byte
			tagsJSON   []byte
			imagesJSON []byte
		)
		if err := rows.Scan(
			&p.ID, &p.Campus, &p.Title, &p.Excerpt, &blocksJSON,
			&p.Author, &p.Likes, &p.Stars, &tagsJSON,
			&p.Shop.Name, &p.Shop.Lat, &p.Shop.Lng,
			&p.Time, &p.Hidden, &imagesJSON,
		); err != nil {
			return nil, fmt.Errorf("scan post: %w", err)
		}
		json.Unmarshal(blocksJSON, &p.Blocks)
		json.Unmarshal(tagsJSON, &p.Tags)
		json.Unmarshal(imagesJSON, &p.Images)
		if p.Tags == nil {
			p.Tags = []string{}
		}
		posts = append(posts, p)
	}

	if posts == nil {
		posts = []Post{}
	}
	return posts, nil
}

// GetPost returns a single food post by ID.
func (db *PortalDB) GetPost(id string) (*Post, error) {
	var (
		p          Post
		blocksJSON []byte
		tagsJSON   []byte
		imagesJSON []byte
	)
	err := db.conn.QueryRow(`
		SELECT id, campus, title, excerpt, blocks, author, likes, stars, tags,
		       shop_name, shop_lat, shop_lng, time, hidden, images
		FROM portal_food_posts WHERE id = ?
	`, id).Scan(
		&p.ID, &p.Campus, &p.Title, &p.Excerpt, &blocksJSON,
		&p.Author, &p.Likes, &p.Stars, &tagsJSON,
		&p.Shop.Name, &p.Shop.Lat, &p.Shop.Lng,
		&p.Time, &p.Hidden, &imagesJSON,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query post: %w", err)
	}
	json.Unmarshal(blocksJSON, &p.Blocks)
	json.Unmarshal(tagsJSON, &p.Tags)
	json.Unmarshal(imagesJSON, &p.Images)
	if p.Tags == nil {
		p.Tags = []string{}
	}
	return &p, nil
}

// GetComments returns comments for a post.
func (db *PortalDB) GetComments(postID string) ([]Comment, error) {
	rows, err := db.conn.Query(`
		SELECT id, post_id, author, time, text
		FROM portal_food_comments WHERE post_id = ? ORDER BY created_at
	`, postID)
	if err != nil {
		return nil, fmt.Errorf("query comments: %w", err)
	}
	defer rows.Close()

	var comments []Comment
	for rows.Next() {
		var c Comment
		if err := rows.Scan(&c.ID, &c.PostID, &c.Author, &c.Time, &c.Text); err != nil {
			return nil, fmt.Errorf("scan comment: %w", err)
		}
		comments = append(comments, c)
	}
	if comments == nil {
		comments = []Comment{}
	}
	return comments, nil
}
