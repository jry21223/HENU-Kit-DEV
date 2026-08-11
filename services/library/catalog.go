package library

import (
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
)

type publicCatalogMaterial struct {
	ID                string `json:"id"`
	Type              string `json:"type"`
	Subject           string `json:"subject"`
	Title             string `json:"title"`
	Role              string `json:"role"`
	FileName          string `json:"file_name"`
	FileSize          int64  `json:"file_size"`
	Downloads         int64  `json:"downloads"`
	DownloadAvailable bool   `json:"download_available"`
}

func (h *service) publicMaterialCatalog(w http.ResponseWriter, r *http.Request) {
	tx, err := h.database.BeginTx(r.Context(), pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "public material catalog is temporarily unavailable")
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()

	var releaseID string
	err = tx.QueryRow(r.Context(), `SELECT release_id FROM library_public_releases WHERE state='active' AND activation_digest IS NOT NULL`).Scan(&releaseID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "public material catalog is temporarily unavailable")
		return
	}

	materials := make([]publicCatalogMaterial, 0)
	if releaseID != "" {
		rows, queryErr := tx.Query(r.Context(), `
			SELECT m.material_id::text,m.material_type,m.subject,m.title,m.role,m.file_name,m.byte_size,
				(SELECT count(*) FROM library_download_start_events e JOIN library_public_releases er ON er.release_id=e.release_id WHERE e.material_id=m.material_id AND er.activation_digest IS NOT NULL)
			FROM library_public_material_snapshots m
			WHERE m.release_id=$1 AND m.status='published' AND m.access_level='public_free'
			ORDER BY m.public_path,m.material_id`, releaseID)
		if queryErr != nil {
			writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "public material catalog is temporarily unavailable")
			return
		}
		for rows.Next() {
			var material publicCatalogMaterial
			if err := rows.Scan(&material.ID, &material.Type, &material.Subject, &material.Title, &material.Role, &material.FileName, &material.FileSize, &material.Downloads); err != nil {
				rows.Close()
				writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "public material catalog is temporarily unavailable")
				return
			}
			material.DownloadAvailable = true
			materials = append(materials, material)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "public material catalog is temporarily unavailable")
			return
		}
		rows.Close()
	}

	var total int64
	var countingSince *time.Time
	if err := tx.QueryRow(r.Context(), `
		SELECT count(e.id),(SELECT min(a.activated_at) FROM library_public_release_activation_events a)
		FROM library_download_start_events e
		JOIN library_public_releases er ON er.release_id=e.release_id
		WHERE er.activation_digest IS NOT NULL`).Scan(&total, &countingSince); err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "public material catalog is temporarily unavailable")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "public material catalog is temporarily unavailable")
		return
	}

	var releaseValue any
	if releaseID != "" {
		releaseValue = releaseID
	}
	var sinceValue any
	if countingSince != nil {
		sinceValue = countingSince.UTC().Format(time.RFC3339)
	}
	writeData(w, r, http.StatusOK, map[string]any{
		"release_id": releaseValue, "materials": materials, "material_count": len(materials),
		"download_starts": total, "counting_since": sinceValue, "as_of": h.now().UTC().Format(time.RFC3339),
	})
}
