package campus

import (
	"database/sql"
	"encoding/json"
	"fmt"
)

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
		       deals_done, wants, place, deadline, status, time, images
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
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate items: %w", err)
	}
	if items == nil {
		items = []Item{}
	}
	return items, nil
}

// GetItem returns a single item by ID.
func (db *PortalDB) GetItem(id string) (*Item, error) {
	var it Item
	var imagesJSON []byte
	err := db.conn.QueryRow(`
		SELECT id, type, category, title, "desc", price, seller, credit,
		       deals_done, wants, place, deadline, status, time, images
		FROM portal_campus_items WHERE id = $1
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
	return &it, nil
}

// GetMessages returns messages for an item.
func (db *PortalDB) GetMessages(itemID string) ([]DealMessage, error) {
	rows, err := db.conn.Query(`
		SELECT id, item_id, author, time, text
		FROM portal_campus_messages WHERE item_id = $1 ORDER BY created_at
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
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate messages: %w", err)
	}
	if msgs == nil {
		msgs = []DealMessage{}
	}
	return msgs, nil
}
