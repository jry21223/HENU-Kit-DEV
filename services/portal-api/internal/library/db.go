package library

import (
	"database/sql"
	"fmt"
)

// StudyDB reads from the legacy Study API database.
type StudyDB struct {
	conn *sql.DB
}

// NewStudyDB creates a StudyDB.
func NewStudyDB(conn *sql.DB) *StudyDB {
	return &StudyDB{conn: conn}
}

// GetMaterials returns materials from the Study API database.
// Joins with courses to get subject info.
func (db *StudyDB) GetMaterials() ([]Material, error) {
	rows, err := db.conn.Query(`
		SELECT m.id, m.type, c.name as subject, m.title, m.description,
		       m.access_level, m.file_size
		FROM materials m
		JOIN courses c ON c.id = m.course_id
		WHERE m.status = 'published' AND m.deleted_at IS NULL
		ORDER BY m.created_at DESC
		LIMIT 100
	`)
	if err != nil {
		return nil, fmt.Errorf("query materials: %w", err)
	}
	defer rows.Close()

	var materials []Material
	for rows.Next() {
		var (
			id          string
			mtype       string
			subject     string
			title       string
			description sql.NullString
			accessLevel string
			fileSize    sql.NullInt64
		)
		if err := rows.Scan(&id, &mtype, &subject, &title, &description, &accessLevel, &fileSize); err != nil {
			return nil, fmt.Errorf("scan material: %w", err)
		}

		// Map Study API type to Portal type
		portalType := mapMaterialType(mtype)

		// Price: access_level "free" = 0, otherwise use a default
		price := 0
		if accessLevel != "free" {
			price = 50 // default price for paid materials
		}

		intro := ""
		if description.Valid {
			intro = description.String
		}

		materials = append(materials, Material{
			ID:           id,
			Type:         portalType,
			Subject:      subject,
			Title:        title,
			Author:       "HENU Kit",
			Intro:        intro,
			TOC:          []string{},
			Pages:        [][]string{{"内容请下载查看"}},
			Price:        price,
			PreviewPages: 1,
			Rating:       4.5,
			Downloads:    0,
			Favs:         0,
		})
	}

	if materials == nil {
		materials = []Material{}
	}
	return materials, nil
}

// mapMaterialType maps Study API material types to Portal types.
func mapMaterialType(t string) string {
	switch t {
	case "knowledge_note", "note":
		return "note"
	case "past_exam", "exam":
		return "exam"
	case "mock_paper", "mock":
		return "mock"
	case "quick_review", "path":
		return "path"
	case "lab_report", "lab":
		return "lab"
	default:
		return "note"
	}
}
