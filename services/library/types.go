package library

import "time"

type Course struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	Grade     string    `json:"grade"`
	Status    string    `json:"status"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Material struct {
	ID          string    `json:"id"`
	CourseID    string    `json:"course_id"`
	Title       string    `json:"title"`
	Type        string    `json:"type"`
	FileName    string    `json:"file_name"`
	FileSize    int64     `json:"file_size"`
	AccessLevel string    `json:"access_level"`
	Status      string    `json:"status"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Download struct {
	ID            string    `json:"id"`
	MaterialID    string    `json:"material_id"`
	MaterialTitle string    `json:"material_title"`
	AccessLevel   string    `json:"access_level"`
	DownloadedAt  time.Time `json:"downloaded_at"`
}

type Correction struct {
	ID          string    `json:"id"`
	TargetType  string    `json:"target_type"`
	TargetID    string    `json:"target_id"`
	Reason      string    `json:"reason"`
	Description string    `json:"description"`
	Status      string    `json:"status"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Workspace struct {
	Status        string       `json:"status"`
	StatusMessage string       `json:"status_message"`
	Degraded      bool         `json:"degraded"`
	Courses       []Course     `json:"courses"`
	Materials     []Material   `json:"materials"`
	Downloads     []Download   `json:"downloads"`
	Submissions   []Material   `json:"submissions"`
	Corrections   []Correction `json:"corrections"`
	GeneratedAt   time.Time    `json:"generated_at"`
}
