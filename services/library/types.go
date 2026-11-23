package library

import "time"

type Course struct {
	ID         string    `json:"id"`
	Code       string    `json:"code"`
	Name       string    `json:"name"`
	Slug       string    `json:"slug"`
	Department string    `json:"department"`
	Grade      string    `json:"grade"`
	Status     string    `json:"status"`
	Version    int       `json:"version"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type Material struct {
	ID          string    `json:"id"`
	CourseID    string    `json:"course_id"`
	CourseName  string    `json:"course_name,omitempty"`
	Title       string    `json:"title"`
	Type        string    `json:"type"`
	Description string    `json:"description,omitempty"`
	FileName    string    `json:"file_name"`
	FileSize    int64     `json:"file_size"`
	AccessLevel string    `json:"access_level"`
	Status      string    `json:"status"`
	SourceNote  string    `json:"source_note,omitempty"`
	Version     int       `json:"version"`
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
	ID             string    `json:"id"`
	MaterialID     string    `json:"material_id,omitempty"`
	MaterialTitle  string    `json:"material_title,omitempty"`
	TargetType     string    `json:"target_type"`
	TargetID       string    `json:"target_id"`
	Reason         string    `json:"reason"`
	Title          string    `json:"title,omitempty"`
	Description    string    `json:"description"`
	Status         string    `json:"status"`
	ResolutionNote string    `json:"resolution_note,omitempty"`
	Version        int       `json:"version"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type ReleaseSummary struct {
	ReleaseID     string    `json:"release_id"`
	State         string    `json:"state"`
	ActivatedAt   time.Time `json:"activated_at"`
	SnapshotCount int       `json:"snapshot_count"`
	ReceiptSHA256 string    `json:"receipt_sha256"`
}

type DownloadStats struct {
	TotalStarts   int64 `json:"total_starts"`
	Last24hStarts int64 `json:"last_24h_starts"`
}

type Workspace struct {
	Status        string           `json:"status"`
	StatusMessage string           `json:"status_message"`
	Degraded      bool             `json:"degraded"`
	Courses       []Course         `json:"courses"`
	Materials     []Material       `json:"materials"`
	Downloads     []Download       `json:"downloads"`
	Submissions   []Material       `json:"submissions"`
	Corrections   []Correction     `json:"corrections"`
	Releases      []ReleaseSummary `json:"releases"`
	DownloadStats DownloadStats    `json:"download_stats"`
	GeneratedAt   time.Time        `json:"generated_at"`
}
