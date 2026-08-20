package library

import (
	"database/sql"
	"encoding/json"
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
	rows, err := db.conn.Query(`
		SELECT m.id, m.type, c.name as subject, m.title, m.description,
		       m.access_level, m.file_size, m.storage_key, m.file_name
		FROM materials m
		JOIN courses c ON c.id = m.course_id
		WHERE m.status = 'published' AND m.deleted_at IS NULL
		ORDER BY m.created_at DESC
		LIMIT 500
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
			storageKey  sql.NullString
			fileName    sql.NullString
		)
		if err := rows.Scan(&id, &mtype, &subject, &title, &description, &accessLevel, &fileSize, &storageKey, &fileName); err != nil {
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
			ID:                id,
			Type:              portalType,
			Subject:           subject,
			Title:             title,
			Author:            "HENU Kit",
			Intro:             intro,
			TOC:               []string{},
			Pages:             [][]string{{"内容请下载查看"}},
			Price:             price,
			PreviewPages:      1,
			Rating:            4.5,
			Downloads:         0,
			Favs:              0,
			FilePath:          storageKey.String,
			DownloadAvailable: publicDownloadAvailable(accessLevel, storageKey),
			FileSize:          fileSize.Int64,
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

// GetMaterialByID returns one published material including its converted slides.
func (db *StudyDB) GetMaterialByID(id string) (Material, error) {
	// The slides column is added by the mirror import; tolerate databases that
	// have not been imported yet.
	var hasSlides bool
	if err := db.conn.QueryRow(
		`SELECT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'materials' AND column_name = 'slides')`,
	).Scan(&hasSlides); err != nil {
		return Material{}, fmt.Errorf("check slides column: %w", err)
	}

	var (
		material    Material
		mtype       string
		description sql.NullString
		accessLevel string
		fileSize    sql.NullInt64
		storageKey  sql.NullString
		fileName    sql.NullString
		slidesJSON  []byte
	)
	if hasSlides {
		if err := db.conn.QueryRow(`
			SELECT m.id, m.type, c.name AS subject, m.title, m.description,
			       m.access_level, m.file_size, m.storage_key, m.file_name, m.slides
			FROM materials m
			JOIN courses c ON c.id = m.course_id
			WHERE m.id = $1 AND m.status = 'published' AND m.deleted_at IS NULL
		`, id).Scan(
			&material.ID, &mtype, &material.Subject, &material.Title, &description,
			&accessLevel, &fileSize, &storageKey, &fileName, &slidesJSON,
		); err != nil {
			return Material{}, fmt.Errorf("query material by id: %w", err)
		}
	} else {
		if err := db.conn.QueryRow(`
			SELECT m.id, m.type, c.name AS subject, m.title, m.description,
			       m.access_level, m.file_size, m.storage_key, m.file_name
			FROM materials m
			JOIN courses c ON c.id = m.course_id
			WHERE m.id = $1 AND m.status = 'published' AND m.deleted_at IS NULL
		`, id).Scan(
			&material.ID, &mtype, &material.Subject, &material.Title, &description,
			&accessLevel, &fileSize, &storageKey, &fileName,
		); err != nil {
			return Material{}, fmt.Errorf("query material by id: %w", err)
		}
	}

	material.Type = mapMaterialType(mtype)
	material.Author = "HENU Kit"
	material.TOC = []string{}
	material.Pages = [][]string{}
	material.PreviewPages = 1
	material.Rating = 4.5
	material.Price = 0
	if accessLevel != "free" {
		material.Price = 50
	}
	material.FilePath = storageKey.String
	material.DownloadAvailable = publicDownloadAvailable(accessLevel, storageKey)
	material.FileSize = fileSize.Int64
	if description.Valid {
		material.Intro = description.String
	}
	if len(slidesJSON) > 0 && string(slidesJSON) != "null" {
		if err := json.Unmarshal(slidesJSON, &material.Slides); err != nil {
			return Material{}, fmt.Errorf("decode slides: %w", err)
		}
	}
	return material, nil
}

func publicDownloadAvailable(accessLevel string, storageKey sql.NullString) bool {
	return accessLevel == "free" && storageKey.Valid && strings.TrimSpace(storageKey.String) != ""
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
	case "slides":
		return "slides"
	default:
		return "note"
	}
}
