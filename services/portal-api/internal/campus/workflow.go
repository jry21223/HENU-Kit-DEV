package campus

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrNotFound             = errors.New("not found")
	ErrForbidden            = errors.New("forbidden")
	ErrConflict             = errors.New("conflict")
	ErrContactNotConfigured = errors.New("contact not configured")
	ErrInvalidInput         = errors.New("invalid input")
)

type ContactBook struct {
	QQ        string     `json:"qq"`
	Wechat    string     `json:"wechat"`
	Phone     string     `json:"phone"`
	Note      string     `json:"note"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}

type ContactSnapshot struct {
	QQ     string `json:"qq,omitempty"`
	Wechat string `json:"wechat,omitempty"`
	Phone  string `json:"phone,omitempty"`
	Note   string `json:"note,omitempty"`
}

type CreateItemInput struct {
	Type     string             `json:"type"`
	Category string             `json:"category"`
	Title    string             `json:"title"`
	Desc     string             `json:"desc"`
	Price    float64            `json:"price"`
	Place    string             `json:"place"`
	Deadline string             `json:"deadline"`
	Images   []CampusImageInput `json:"images,omitempty"`
}

type Exchange struct {
	ID                string           `json:"id"`
	ItemID            string           `json:"item_id"`
	RequesterUserID   string           `json:"requester_user_id"`
	RequesterName     string           `json:"requester_name"`
	OwnerUserID       string           `json:"owner_user_id"`
	Status            string           `json:"status"`
	RequesterChannels []string         `json:"requester_channels"`
	OwnerChannels     []string         `json:"owner_channels"`
	RequesterSnapshot *ContactSnapshot `json:"requester_snapshot,omitempty"`
	OwnerSnapshot     *ContactSnapshot `json:"owner_snapshot,omitempty"`
	CreatedAt         time.Time        `json:"created_at"`
	UpdatedAt         time.Time        `json:"updated_at"`
}

type ConsoleCounts struct {
	Open              int `json:"open"`
	Ongoing           int `json:"ongoing"`
	Done              int `json:"done"`
	Hidden            int `json:"hidden"`
	MessagesTotal     int `json:"messages_total"`
	MessagesHidden    int `json:"messages_hidden"`
	ExchangesTotal    int `json:"exchanges_total"`
	ExchangesAccepted int `json:"exchanges_accepted"`
}

type ConsoleItem struct {
	ID               string    `json:"id"`
	Type             string    `json:"type"`
	Category         string    `json:"category"`
	Title            string    `json:"title"`
	Desc             string    `json:"desc"`
	Price            float64   `json:"price"`
	Seller           string    `json:"seller"`
	AuthorUserID     string    `json:"author_user_id,omitempty"`
	AcceptedByUserID string    `json:"accepted_by_user_id,omitempty"`
	AcceptedByName   string    `json:"accepted_by_name,omitempty"`
	Place            string    `json:"place"`
	Deadline         *string   `json:"deadline,omitempty"`
	Status           string    `json:"status"`
	ModerationNote   string    `json:"moderation_note"`
	Version          int       `json:"version"`
	Time             string    `json:"time"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type ConsoleMessage struct {
	ID           string    `json:"id"`
	ItemID       string    `json:"item_id"`
	ItemTitle    string    `json:"item_title,omitempty"`
	Author       string    `json:"author"`
	AuthorUserID string    `json:"author_user_id,omitempty"`
	Text         string    `json:"text"`
	Hidden       bool      `json:"hidden"`
	Time         string    `json:"time"`
	CreatedAt    time.Time `json:"created_at"`
}

type ConsoleWorkspace struct {
	GeneratedAt time.Time        `json:"generated_at"`
	Counts      ConsoleCounts    `json:"counts"`
	Items       []ConsoleItem    `json:"items"`
	Messages    []ConsoleMessage `json:"messages"`
}

func randomID(prefix string) string {
	buf := make([]byte, 6)
	_, _ = rand.Read(buf)
	return prefix + "-" + hex.EncodeToString(buf)
}

func sanitizeChannels(channels []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(channels))
	for _, ch := range channels {
		c := strings.ToLower(strings.TrimSpace(ch))
		if (c == "qq" || c == "wechat" || c == "phone") && !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	return out
}

func buildSnapshot(book ContactBook, channels []string) (ContactSnapshot, []string) {
	var snap ContactSnapshot
	var granted []string
	for _, ch := range channels {
		switch ch {
		case "qq":
			if strings.TrimSpace(book.QQ) != "" {
				snap.QQ = strings.TrimSpace(book.QQ)
				granted = append(granted, "qq")
			}
		case "wechat":
			if strings.TrimSpace(book.Wechat) != "" {
				snap.Wechat = strings.TrimSpace(book.Wechat)
				granted = append(granted, "wechat")
			}
		case "phone":
			if strings.TrimSpace(book.Phone) != "" {
				snap.Phone = strings.TrimSpace(book.Phone)
				granted = append(granted, "phone")
			}
		}
	}
	if len(granted) > 0 && strings.TrimSpace(book.Note) != "" {
		snap.Note = strings.TrimSpace(book.Note)
	}
	return snap, granted
}

func (db *PortalDB) GetContactBook(userID string) (ContactBook, error) {
	var b ContactBook
	var updated time.Time
	err := db.conn.QueryRow(`
		SELECT qq, wechat, phone, note, updated_at
		FROM portal_campus_contacts WHERE user_id = $1::uuid
	`, userID).Scan(&b.QQ, &b.Wechat, &b.Phone, &b.Note, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return ContactBook{}, nil
	}
	if err != nil {
		return ContactBook{}, err
	}
	b.UpdatedAt = &updated
	return b, nil
}

func (db *PortalDB) UpsertContactBook(userID string, input ContactBook) (ContactBook, error) {
	qq := strings.TrimSpace(input.QQ)
	wechat := strings.TrimSpace(input.Wechat)
	phone := strings.TrimSpace(input.Phone)
	note := strings.TrimSpace(input.Note)
	if len(qq) > 64 || len(wechat) > 64 || len(phone) > 32 || len(note) > 200 {
		return ContactBook{}, ErrInvalidInput
	}
	var updated time.Time
	err := db.conn.QueryRow(`
		INSERT INTO portal_campus_contacts (user_id, qq, wechat, phone, note, updated_at)
		VALUES ($1::uuid, $2, $3, $4, $5, now())
		ON CONFLICT (user_id) DO UPDATE
		SET qq = EXCLUDED.qq, wechat = EXCLUDED.wechat, phone = EXCLUDED.phone, note = EXCLUDED.note, updated_at = now()
		RETURNING updated_at
	`, userID, qq, wechat, phone, note).Scan(&updated)
	if err != nil {
		return ContactBook{}, err
	}
	return ContactBook{QQ: qq, Wechat: wechat, Phone: phone, Note: note, UpdatedAt: &updated}, nil
}

func (db *PortalDB) CreateItem(ctx context.Context, actorUserID, sellerName string, in CreateItemInput) (Item, error) {
	in.Type = strings.TrimSpace(in.Type)
	in.Category = strings.TrimSpace(in.Category)
	in.Title = strings.TrimSpace(in.Title)
	in.Desc = strings.TrimSpace(in.Desc)
	in.Place = strings.TrimSpace(in.Place)
	in.Deadline = strings.TrimSpace(in.Deadline)
	if (in.Type != "help" && in.Type != "sell") || in.Title == "" || len(in.Title) > 120 || in.Desc == "" || len(in.Desc) > 1000 || in.Place == "" || len(in.Place) > 120 || in.Price < 0 || in.Price > 100000 {
		return Item{}, ErrInvalidInput
	}
	if in.Category == "" {
		if in.Type == "sell" {
			in.Category = "flea"
		} else {
			in.Category = "errand"
		}
	}
	seller := strings.TrimSpace(sellerName)
	if seller == "" {
		seller = "河大同学"
	}
	prefix := "h"
	if in.Type == "sell" {
		prefix = "s"
	}
	id := randomID(prefix)
	nowLabel := time.Now().In(time.FixedZone("CST", 8*3600)).Format("01-02 15:04")
	var deadlinePtr *string
	if in.Deadline != "" {
		d := in.Deadline
		deadlinePtr = &d
	}
	images, err := sanitizeCampusImages(ctx, in.Images)
	if err != nil {
		return Item{}, err
	}
	tx, err := db.conn.BeginTx(ctx, nil)
	if err != nil {
		return Item{}, err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `
		INSERT INTO portal_campus_items
		  (id, type, category, title, "desc", price, seller, credit, deals_done, wants, place, deadline, status, time, images, author_user_id, version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 90, 0, 0, $8, $9, 'open', $10, '[]'::jsonb, $11::uuid, 1, now(), now())
	`, id, in.Type, in.Category, in.Title, in.Desc, in.Price, seller, in.Place, deadlinePtr, nowLabel, actorUserID)
	if err != nil {
		return Item{}, err
	}
	urls := make([]string, 0, len(images))
	for position, image := range images {
		_, err := tx.ExecContext(ctx, `INSERT INTO portal_campus_item_images
			(id, item_id, position, content_type, byte_size, sha256, bytes) VALUES ($1,$2,$3,$4,$5,$6,$7)`,
			randomID("img"), id, position, image.ContentType, len(image.Bytes), image.SHA256, image.Bytes)
		if err != nil {
			return Item{}, err
		}
		urls = append(urls, campusImageURL(id, position, image.SHA256))
	}
	if err := tx.Commit(); err != nil {
		return Item{}, err
	}
	return Item{
		ID: id, Type: in.Type, Category: in.Category, Title: in.Title, Desc: in.Desc,
		Price: in.Price, Seller: seller, Credit: 90, Place: in.Place,
		Deadline: deadlinePtr, Status: "open", Time: nowLabel, Images: urls,
	}, nil
}

func (db *PortalDB) CreateMessage(itemID, actorUserID, authorName, text string) (DealMessage, error) {
	text = strings.TrimSpace(text)
	if text == "" || len(text) > 500 {
		return DealMessage{}, ErrInvalidInput
	}
	var status string
	if err := db.conn.QueryRow(`SELECT status FROM portal_campus_items WHERE id = $1`, itemID).Scan(&status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return DealMessage{}, ErrNotFound
		}
		return DealMessage{}, err
	}
	if status == "hidden" {
		return DealMessage{}, ErrNotFound
	}
	author := strings.TrimSpace(authorName)
	if author == "" {
		author = "河大同学"
	}
	id := randomID("m")
	nowLabel := time.Now().In(time.FixedZone("CST", 8*3600)).Format("01-02 15:04")
	_, err := db.conn.Exec(`
		INSERT INTO portal_campus_messages (id, item_id, author, author_user_id, time, text, hidden, created_at)
		VALUES ($1, $2, $3, $4::uuid, $5, $6, false, now())
	`, id, itemID, author, actorUserID, nowLabel, text)
	if err != nil {
		return DealMessage{}, err
	}
	return DealMessage{ID: id, ItemID: itemID, Author: author, Time: nowLabel, Text: text}, nil
}

func (db *PortalDB) RequestExchange(itemID, requesterUserID, requesterName string, rawChannels []string) (Exchange, error) {
	channels := sanitizeChannels(rawChannels)
	if len(channels) == 0 {
		return Exchange{}, ErrInvalidInput
	}
	var ownerID sql.NullString
	var status string
	err := db.conn.QueryRow(`SELECT COALESCE(author_user_id::text, ''), status FROM portal_campus_items WHERE id = $1`, itemID).Scan(&ownerID, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return Exchange{}, ErrNotFound
	}
	if err != nil {
		return Exchange{}, err
	}
	if status != "open" {
		return Exchange{}, ErrConflict
	}
	if ownerID.String != "" && ownerID.String == requesterUserID {
		return Exchange{}, ErrForbidden
	}

	book, err := db.GetContactBook(requesterUserID)
	if err != nil {
		return Exchange{}, err
	}
	snap, granted := buildSnapshot(book, channels)
	if len(granted) == 0 {
		return Exchange{}, ErrContactNotConfigured
	}
	name := strings.TrimSpace(requesterName)
	if name == "" {
		name = "河大同学"
	}
	grantedJSON, _ := json.Marshal(granted)
	snapJSON, _ := json.Marshal(snap)
	var ownerParam any
	if ownerID.String != "" {
		ownerParam = ownerID.String
	}

	var ex Exchange
	var exOwner sql.NullString
	err = db.conn.QueryRow(`
		INSERT INTO portal_campus_exchanges
		  (item_id, requester_user_id, requester_name, owner_user_id, status, requester_channels, requester_snapshot, created_at, updated_at)
		VALUES ($1, $2::uuid, $3, $4::uuid, 'requested', $5::jsonb, $6::jsonb, now(), now())
		ON CONFLICT (item_id, requester_user_id) DO UPDATE
		SET requester_name = EXCLUDED.requester_name,
		    requester_channels = EXCLUDED.requester_channels,
		    requester_snapshot = EXCLUDED.requester_snapshot,
		    status = CASE WHEN portal_campus_exchanges.status = 'accepted' THEN 'accepted' ELSE 'requested' END,
		    updated_at = now()
		RETURNING id::text, item_id, requester_user_id::text, requester_name, COALESCE(owner_user_id::text, ''), status, created_at, updated_at
	`, itemID, requesterUserID, name, ownerParam, string(grantedJSON), string(snapJSON)).Scan(
		&ex.ID, &ex.ItemID, &ex.RequesterUserID, &ex.RequesterName, &exOwner, &ex.Status, &ex.CreatedAt, &ex.UpdatedAt,
	)
	if err != nil {
		return Exchange{}, err
	}
	ex.OwnerUserID = exOwner.String
	ex.RequesterChannels = granted
	ex.OwnerChannels = []string{}
	_, _ = db.conn.Exec(`UPDATE portal_campus_items SET wants = wants + 1, updated_at = now() WHERE id = $1`, itemID)
	return ex, nil
}

func (db *PortalDB) ListExchangesForActor(itemID, actorUserID string) (bool, []Exchange, error) {
	var ownerID string
	var status string
	err := db.conn.QueryRow(`SELECT COALESCE(author_user_id::text, ''), status FROM portal_campus_items WHERE id = $1`, itemID).Scan(&ownerID, &status)
	if errors.Is(err, sql.ErrNoRows) || status == "hidden" {
		return false, nil, ErrNotFound
	}
	if err != nil {
		return false, nil, err
	}
	isOwner := ownerID != "" && ownerID == actorUserID

	rows, err := db.conn.Query(`
		SELECT id::text, item_id, requester_user_id::text, requester_name, COALESCE(owner_user_id::text, ''),
		       status, requester_channels, owner_channels, requester_snapshot, owner_snapshot, created_at, updated_at
		FROM portal_campus_exchanges
		WHERE item_id = $1 AND (requester_user_id = $2::uuid OR owner_user_id = $2::uuid)
		ORDER BY created_at DESC
	`, itemID, actorUserID)
	if err != nil {
		return isOwner, nil, err
	}
	defer rows.Close()

	list := []Exchange{}
	for rows.Next() {
		var ex Exchange
		var reqChJSON, ownChJSON, reqSnapJSON, ownSnapJSON []byte
		if err := rows.Scan(
			&ex.ID, &ex.ItemID, &ex.RequesterUserID, &ex.RequesterName, &ex.OwnerUserID,
			&ex.Status, &reqChJSON, &ownChJSON, &reqSnapJSON, &ownSnapJSON, &ex.CreatedAt, &ex.UpdatedAt,
		); err != nil {
			return isOwner, nil, err
		}
		_ = json.Unmarshal(reqChJSON, &ex.RequesterChannels)
		_ = json.Unmarshal(ownChJSON, &ex.OwnerChannels)
		if ex.RequesterChannels == nil {
			ex.RequesterChannels = []string{}
		}
		if ex.OwnerChannels == nil {
			ex.OwnerChannels = []string{}
		}
		// Privacy boundary: snapshots are ONLY revealed after explicit bilateral acceptance.
		if ex.Status == "accepted" {
			if len(reqSnapJSON) > 0 {
				var rs ContactSnapshot
				if json.Unmarshal(reqSnapJSON, &rs) == nil {
					ex.RequesterSnapshot = &rs
				}
			}
			if len(ownSnapJSON) > 0 {
				var osnap ContactSnapshot
				if json.Unmarshal(ownSnapJSON, &osnap) == nil {
					ex.OwnerSnapshot = &osnap
				}
			}
		}
		list = append(list, ex)
	}
	return isOwner, list, rows.Err()
}

func (db *PortalDB) RespondExchange(itemID, exchangeID, ownerUserID, decision string, rawChannels []string) (Exchange, string, error) {
	if decision != "accept" && decision != "decline" {
		return Exchange{}, "", ErrInvalidInput
	}
	var itemOwner, itemStatus string
	err := db.conn.QueryRow(`SELECT COALESCE(author_user_id::text, ''), status FROM portal_campus_items WHERE id = $1`, itemID).Scan(&itemOwner, &itemStatus)
	if errors.Is(err, sql.ErrNoRows) {
		return Exchange{}, "", ErrNotFound
	}
	if err != nil {
		return Exchange{}, "", err
	}
	if itemOwner == "" || itemOwner != ownerUserID {
		return Exchange{}, "", ErrForbidden
	}

	if decision == "decline" {
		var ex Exchange
		var reqChJSON []byte
		err = db.conn.QueryRow(`
			UPDATE portal_campus_exchanges
			SET status = 'declined', updated_at = now()
			WHERE id = $1::uuid AND item_id = $2 AND status = 'requested'
			RETURNING id::text, item_id, requester_user_id::text, requester_name, COALESCE(owner_user_id::text, ''), status, requester_channels, created_at, updated_at
		`, exchangeID, itemID).Scan(
			&ex.ID, &ex.ItemID, &ex.RequesterUserID, &ex.RequesterName, &ex.OwnerUserID, &ex.Status, &reqChJSON, &ex.CreatedAt, &ex.UpdatedAt,
		)
		if errors.Is(err, sql.ErrNoRows) {
			return Exchange{}, "", ErrConflict
		}
		if err != nil {
			return Exchange{}, "", err
		}
		_ = json.Unmarshal(reqChJSON, &ex.RequesterChannels)
		ex.OwnerChannels = []string{}
		return ex, itemStatus, nil
	}

	channels := sanitizeChannels(rawChannels)
	if len(channels) == 0 {
		channels = []string{"qq", "wechat", "phone"}
	}
	book, err := db.GetContactBook(ownerUserID)
	if err != nil {
		return Exchange{}, "", err
	}
	ownerSnap, granted := buildSnapshot(book, channels)
	if len(granted) == 0 {
		return Exchange{}, "", ErrContactNotConfigured
	}
	grantedJSON, _ := json.Marshal(granted)
	snapJSON, _ := json.Marshal(ownerSnap)

	tx, err := db.conn.Begin()
	if err != nil {
		return Exchange{}, "", err
	}
	defer func() { _ = tx.Rollback() }()

	var ex Exchange
	var reqChJSON, reqSnapJSON []byte
	err = tx.QueryRow(`
		UPDATE portal_campus_exchanges
		SET status = 'accepted', owner_channels = $3::jsonb, owner_snapshot = $4::jsonb, updated_at = now()
		WHERE id = $1::uuid AND item_id = $2 AND status = 'requested'
		RETURNING id::text, item_id, requester_user_id::text, requester_name, COALESCE(owner_user_id::text, ''), status, requester_channels, requester_snapshot, created_at, updated_at
	`, exchangeID, itemID, string(grantedJSON), string(snapJSON)).Scan(
		&ex.ID, &ex.ItemID, &ex.RequesterUserID, &ex.RequesterName, &ex.OwnerUserID, &ex.Status, &reqChJSON, &reqSnapJSON, &ex.CreatedAt, &ex.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Exchange{}, "", ErrConflict
	}
	if err != nil {
		return Exchange{}, "", err
	}

	if _, err := tx.Exec(`
		UPDATE portal_campus_items
		SET status = 'ongoing', accepted_by_user_id = $2::uuid, accepted_by_name = $3, accepted_at = now(), version = version + 1, updated_at = now()
		WHERE id = $1 AND status = 'open'
	`, itemID, ex.RequesterUserID, ex.RequesterName); err != nil {
		return Exchange{}, "", err
	}

	if err := tx.Commit(); err != nil {
		return Exchange{}, "", err
	}

	_ = json.Unmarshal(reqChJSON, &ex.RequesterChannels)
	ex.OwnerChannels = granted
	if len(reqSnapJSON) > 0 {
		var rs ContactSnapshot
		if json.Unmarshal(reqSnapJSON, &rs) == nil {
			ex.RequesterSnapshot = &rs
		}
	}
	ex.OwnerSnapshot = &ownerSnap
	return ex, "ongoing", nil
}

func (db *PortalDB) CompleteItem(itemID, ownerUserID string) error {
	var owner string
	err := db.conn.QueryRow(`SELECT COALESCE(author_user_id::text, '') FROM portal_campus_items WHERE id = $1`, itemID).Scan(&owner)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if owner == "" || owner != ownerUserID {
		return ErrForbidden
	}
	res, err := db.conn.Exec(`
		UPDATE portal_campus_items
		SET status = 'done', deals_done = deals_done + 1, version = version + 1, updated_at = now()
		WHERE id = $1 AND status IN ('open', 'ongoing')
	`, itemID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrConflict
	}
	return nil
}

func (db *PortalDB) ConsoleWorkspace() (ConsoleWorkspace, error) {
	ws := ConsoleWorkspace{
		GeneratedAt: time.Now().UTC(),
		Items:       []ConsoleItem{},
		Messages:    []ConsoleMessage{},
	}
	_ = db.conn.QueryRow(`
		SELECT count(*) FILTER (WHERE status = 'open'),
		       count(*) FILTER (WHERE status = 'ongoing'),
		       count(*) FILTER (WHERE status = 'done'),
		       count(*) FILTER (WHERE status = 'hidden')
		FROM portal_campus_items
	`).Scan(&ws.Counts.Open, &ws.Counts.Ongoing, &ws.Counts.Done, &ws.Counts.Hidden)
	_ = db.conn.QueryRow(`
		SELECT count(*), count(*) FILTER (WHERE hidden = true)
		FROM portal_campus_messages
	`).Scan(&ws.Counts.MessagesTotal, &ws.Counts.MessagesHidden)
	_ = db.conn.QueryRow(`
		SELECT count(*), count(*) FILTER (WHERE status = 'accepted')
		FROM portal_campus_exchanges
	`).Scan(&ws.Counts.ExchangesTotal, &ws.Counts.ExchangesAccepted)

	itemRows, err := db.conn.Query(`
		SELECT id, type, category, title, "desc", price, seller,
		       COALESCE(author_user_id::text, ''), COALESCE(accepted_by_user_id::text, ''), COALESCE(accepted_by_name, ''),
		       place, deadline, status, moderation_note, version, time, created_at, updated_at
		FROM portal_campus_items
		ORDER BY created_at DESC
		LIMIT 200
	`)
	if err != nil {
		return ws, fmt.Errorf("query console campus items: %w", err)
	}
	defer itemRows.Close()
	for itemRows.Next() {
		var it ConsoleItem
		if err := itemRows.Scan(
			&it.ID, &it.Type, &it.Category, &it.Title, &it.Desc, &it.Price, &it.Seller,
			&it.AuthorUserID, &it.AcceptedByUserID, &it.AcceptedByName,
			&it.Place, &it.Deadline, &it.Status, &it.ModerationNote, &it.Version, &it.Time, &it.CreatedAt, &it.UpdatedAt,
		); err != nil {
			return ws, err
		}
		ws.Items = append(ws.Items, it)
	}

	msgRows, err := db.conn.Query(`
		SELECT m.id, m.item_id, COALESCE(i.title, ''), m.author, COALESCE(m.author_user_id::text, ''),
		       m.text, m.hidden, m.time, m.created_at
		FROM portal_campus_messages m
		LEFT JOIN portal_campus_items i ON i.id = m.item_id
		ORDER BY m.created_at DESC
		LIMIT 200
	`)
	if err == nil {
		defer msgRows.Close()
		for msgRows.Next() {
			var m ConsoleMessage
			if scanErr := msgRows.Scan(&m.ID, &m.ItemID, &m.ItemTitle, &m.Author, &m.AuthorUserID, &m.Text, &m.Hidden, &m.Time, &m.CreatedAt); scanErr == nil {
				ws.Messages = append(ws.Messages, m)
			}
		}
	}
	return ws, nil
}

func (db *PortalDB) ModerateItem(itemID, actorUserID string, expectedVersion int, action, note string) (string, int, error) {
	var nextStatus string
	switch action {
	case "hide":
		nextStatus = "hidden"
	case "restore":
		nextStatus = "open"
	case "close":
		nextStatus = "done"
	default:
		return "", 0, ErrInvalidInput
	}
	if expectedVersion < 1 {
		return "", 0, ErrInvalidInput
	}
	note = strings.TrimSpace(note)
	tx, err := db.conn.Begin()
	if err != nil {
		return "", 0, err
	}
	defer func() { _ = tx.Rollback() }()

	var newVersion int
	err = tx.QueryRow(`
		UPDATE portal_campus_items
		SET status = $3, moderation_note = $4, version = version + 1, updated_at = now()
		WHERE id = $1 AND version = $2
		RETURNING version
	`, itemID, expectedVersion, nextStatus, note).Scan(&newVersion)
	if errors.Is(err, sql.ErrNoRows) {
		return "", 0, ErrConflict
	}
	if err != nil {
		return "", 0, err
	}
	if _, err := tx.Exec(`
		INSERT INTO portal_campus_audit_events (item_id, actor_user_id, action, note)
		VALUES ($1, $2::uuid, $3, $4)
	`, itemID, actorUserID, "item_"+action, note); err != nil {
		return "", 0, err
	}
	if err := tx.Commit(); err != nil {
		return "", 0, err
	}
	return nextStatus, newVersion, nil
}

func (db *PortalDB) ModerateMessage(messageID, actorUserID, action, note string) (bool, error) {
	if action != "hide" && action != "restore" {
		return false, ErrInvalidInput
	}
	hidden := action == "hide"
	tx, err := db.conn.Begin()
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()

	var itemID string
	err = tx.QueryRow(`
		UPDATE portal_campus_messages SET hidden = $2 WHERE id = $1 RETURNING item_id
	`, messageID, hidden).Scan(&itemID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, ErrNotFound
	}
	if err != nil {
		return false, err
	}
	_, err = tx.Exec(`
		INSERT INTO portal_campus_audit_events (item_id, actor_user_id, action, note)
		VALUES ($1, $2::uuid, $3, $4)
	`, itemID, actorUserID, "message_"+action, strings.TrimSpace(note))
	if err != nil {
		return false, err
	}
	return hidden, tx.Commit()
}
