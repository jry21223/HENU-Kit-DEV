package library

import (
	"database/sql"
	"fmt"
	"strings"
)

// StudyDB reads from the legacy Study API database.
type StudyDB struct {
	conn *sql.DB
}

// NewStudyDB creates a StudyDB.
func NewStudyDB(conn *sql.DB) *StudyDB {
	return &StudyDB{conn: conn}
}

// GetCourses aggregates published courses and material counts from Study DB.
func (db *StudyDB) GetCourses() ([]Course, error) {
	rows, err := db.conn.Query(`
		SELECT c.id::text, c.name, c.name AS subject,
		       COUNT(m.id) FILTER (
		         WHERE m.status = 'published' AND m.deleted_at IS NULL
		       )::int AS material_count
		FROM courses c
		LEFT JOIN materials m ON m.course_id = c.id
		WHERE c.status = 'published' AND c.deleted_at IS NULL
		GROUP BY c.id, c.name
		ORDER BY c.name
	`)
	if err != nil {
		return nil, fmt.Errorf("query courses: %w", err)
	}
	defer rows.Close()

	var courses []Course
	for rows.Next() {
		var c Course
		if err := rows.Scan(&c.ID, &c.Name, &c.Subject, &c.MaterialCount); err != nil {
			return nil, fmt.Errorf("scan course: %w", err)
		}
		courses = append(courses, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate courses: %w", err)
	}
	if courses == nil {
		courses = []Course{}
	}
	return courses, nil
}

// GetMaterials returns materials from the Study API database.
// Joins with courses to get subject info.
func (db *StudyDB) GetMaterials() ([]Material, error) {
	// No LIMIT: the catalogue is the whole point of the page, and truncating it
	// silently hid a third of the mirrored materials.
	rows, err := db.conn.Query(`
		SELECT m.id, m.type, c.name as subject, m.title, m.description,
		       m.access_level, m.file_size, m.file_name, m.storage_key
		FROM materials m
		JOIN courses c ON c.id = m.course_id
		WHERE m.status = 'published' AND m.deleted_at IS NULL
		ORDER BY c.name, m.title
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
			fileName    sql.NullString
			storageKey  sql.NullString
		)
		if err := rows.Scan(
			&id, &mtype, &subject, &title, &description,
			&accessLevel, &fileSize, &fileName, &storageKey,
		); err != nil {
			return nil, fmt.Errorf("scan material: %w", err)
		}

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
			ID:      id,
			Type:    mapMaterialType(mtype),
			Subject: subject,
			Title:   title,
			Intro:   intro,
			TOC:     []string{},
			Pages:   [][]string{},
			Price:   price,
			// Ratings, download counts and favourites are not recorded anywhere,
			// so they stay at zero rather than being invented. Author is left to
			// the owner: these materials are contributed, not written by us.
			PreviewPages: 0,
			DownloadURL:  downloadURL(storageKey),
			FileName:     fileName.String,
			FileSize:     fileSize.Int64,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate materials: %w", err)
	}

	if materials == nil {
		materials = []Material{}
	}
	return materials, nil
}

// downloadURL turns a storage key into the path the mirror serves it from.
//
// Keys written by the materials mirror are repository-relative paths; anything
// already absolute is passed through so a differently hosted material keeps
// working. An empty key yields an empty URL, and the Portal hides the download
// rather than offering a broken one.
func downloadURL(storageKey sql.NullString) string {
	key := strings.TrimSpace(storageKey.String)
	if !storageKey.Valid || key == "" {
		return ""
	}
	if strings.HasPrefix(key, "http://") || strings.HasPrefix(key, "https://") || strings.HasPrefix(key, "/") {
		return key
	}
	return materialsMirrorPrefix + key
}

// Path prefix nginx serves the course materials mirror from.
const materialsMirrorPrefix = "/materials/"

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
	case "courseware", "slides":
		return "slides"
	default:
		return "note"
	}
}
