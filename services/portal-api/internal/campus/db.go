package campus

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

const campusImageURLsSQL = `COALESCE((SELECT jsonb_agg(
    '/api/v1/campus/items/' || i.item_id || '/images/' || i.position || '?v=' || left(i.sha256, 12)
    ORDER BY i.position) FROM portal_campus_item_images i WHERE i.item_id = portal_campus_items.id), images)`

func campusImageURL(id string, position int, sha string) string {
	return fmt.Sprintf("/api/v1/campus/items/%s/images/%d?v=%s", id, position, sha[:12])
}

func (db *PortalDB) GetItemImage(ctx context.Context, id string, position int) (*SanitizedCampusImage, error) {
	var img SanitizedCampusImage
	err := db.conn.QueryRowContext(ctx, `SELECT i.content_type, i.sha256, i.bytes
		FROM portal_campus_item_images i JOIN portal_campus_items item ON item.id = i.item_id
		WHERE i.item_id = $1 AND i.position = $2 AND item.status != 'hidden'`, id, position).
		Scan(&img.ContentType, &img.SHA256, &img.Bytes)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query campus image: %w", err)
	}
	return &img, nil
}

// PortalDB reads from the portal_campus_* tables.
type PortalDB struct {
	conn *sql.DB
}

// NewPortalDB creates a PortalDB.
func NewPortalDB(conn *sql.DB) *PortalDB {
	return &PortalDB{conn: conn}
}

// GetItems returns campus items with optional filters (PostgreSQL placeholders).
func (db *PortalDB) GetItems(typeFilter, categoryFilter, qFilter string) ([]Item, error) {
	query := `
		SELECT id, type, category, title, "desc", price, seller, credit,
		       deals_done, wants, place, deadline, status, time, ` + campusImageURLsSQL + `
		FROM portal_campus_items
		WHERE status != 'hidden'
	`
	var args []any
	n := 0
	if typeFilter != "" {
		n++
		query += fmt.Sprintf(" AND type = $%d", n)
		args = append(args, typeFilter)
	}
	if categoryFilter != "" {
		n++
		query += fmt.Sprintf(" AND category = $%d", n)
		args = append(args, categoryFilter)
	}
	if qFilter != "" {
		n++
		a := n
		n++
		b := n
		query += fmt.Sprintf(` AND (title ILIKE $%d OR "desc" ILIKE $%d)`, a, b)
		like := "%" + qFilter + "%"
		args = append(args, like, like)
	}
	query += " ORDER BY created_at DESC"

	rows, err := db.conn.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query items: %w", err)
	}
	defer rows.Close()

	var items []Item
	for rows.Next() {
		var it Item
		var imagesJSON []byte
		if err := rows.Scan(
			&it.ID, &it.Type, &it.Category, &it.Title, &it.Desc,
			&it.Price, &it.Seller, &it.Credit, &it.DealsDone,
			&it.Wants, &it.Place, &it.Deadline, &it.Status, &it.Time, &imagesJSON,
		); err != nil {
			return nil, fmt.Errorf("scan item: %w", err)
		}
		_ = json.Unmarshal(imagesJSON, &it.Images)
		if it.Images == nil {
			it.Images = []string{}
		}
		items = append(items, it)
	}
	if items == nil {
		items = []Item{}
	}
	return items, rows.Err()
}

// GetItem returns a single item by ID.
func (db *PortalDB) GetItem(id string) (*Item, error) {
	var it Item
	var imagesJSON []byte
	err := db.conn.QueryRow(`
		SELECT id, type, category, title, "desc", price, seller, credit,
		       deals_done, wants, place, deadline, status, time, `+campusImageURLsSQL+`
		FROM portal_campus_items WHERE id = $1 AND status != 'hidden'
	`, id).Scan(
		&it.ID, &it.Type, &it.Category, &it.Title, &it.Desc,
		&it.Price, &it.Seller, &it.Credit, &it.DealsDone,
		&it.Wants, &it.Place, &it.Deadline, &it.Status, &it.Time, &imagesJSON,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query item: %w", err)
	}
	_ = json.Unmarshal(imagesJSON, &it.Images)
	if it.Images == nil {
		it.Images = []string{}
	}
	return &it, nil
}

// GetMessages returns visible messages for an item.
func (db *PortalDB) GetMessages(itemID string) ([]DealMessage, error) {
	rows, err := db.conn.Query(`
		SELECT id, item_id, author, time, text
		FROM portal_campus_messages WHERE item_id = $1 AND hidden = false ORDER BY created_at
	`, itemID)
	if err != nil {
		return nil, fmt.Errorf("query messages: %w", err)
	}
	defer rows.Close()

	var msgs []DealMessage
	for rows.Next() {
		var m DealMessage
		if err := rows.Scan(&m.ID, &m.ItemID, &m.Author, &m.Time, &m.Text); err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}
		msgs = append(msgs, m)
	}
	if msgs == nil {
		msgs = []DealMessage{}
	}
	return msgs, nil
}
