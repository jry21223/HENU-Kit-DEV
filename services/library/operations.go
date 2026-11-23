package library

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type commandInput struct {
	Kind            string          `json:"kind"`
	ResourceID      string          `json:"resource_id"`
	ExpectedVersion string          `json:"expected_version"`
	Payload         json.RawMessage `json:"payload"`
}

type commandResult struct {
	Operation  string `json:"operation"`
	ResourceID string `json:"resource_id"`
	State      string `json:"state"`
	Version    int    `json:"version"`
}

var allowedLibraryMaterialTypes = map[string]bool{
	"handout":  true,
	"exam":     true,
	"slides":   true,
	"exercise": true,
	"answer":   true,
	"note":     true,
	"textbook": true,
}

var allowedLibraryAccessLevels = map[string]bool{
	"public_free":   true,
	"authenticated": true,
	"restricted":    true,
}

var allowedLibraryMaterialStatuses = map[string]bool{
	"draft":          true,
	"pending_review": true,
	"published":      true,
	"archived":       true,
}

var allowedLibraryCorrectionStatuses = map[string]bool{
	"open":        true,
	"in_progress": true,
	"resolved":    true,
	"dismissed":   true,
}

func (h *service) loadWorkspace(ctx context.Context) (Workspace, error) {
	now := h.now()
	ws := Workspace{
		Status:        "ok",
		StatusMessage: "资料库原生目录与审核工作台运行正常",
		Degraded:      false,
		Courses:       []Course{},
		Materials:     []Material{},
		Downloads:     []Download{},
		Submissions:   []Material{},
		Corrections:   []Correction{},
		Releases:      []ReleaseSummary{},
		GeneratedAt:   now,
	}

	courseRows, err := h.database.Query(ctx, `
		SELECT id::text, code, name, department, status, version, updated_at
		FROM library_courses
		ORDER BY updated_at DESC, code ASC
		LIMIT 200
	`)
	if err != nil {
		return ws, fmt.Errorf("query library_courses: %w", err)
	}
	defer courseRows.Close()
	for courseRows.Next() {
		var c Course
		if err := courseRows.Scan(&c.ID, &c.Code, &c.Name, &c.Department, &c.Status, &c.Version, &c.UpdatedAt); err != nil {
			return ws, fmt.Errorf("scan library_courses: %w", err)
		}
		c.Slug = c.Code
		c.Grade = c.Department
		ws.Courses = append(ws.Courses, c)
	}
	if err := courseRows.Err(); err != nil {
		return ws, err
	}

	matRows, err := h.database.Query(ctx, `
		SELECT m.id::text, m.course_id::text, c.name, m.title, m.material_type, m.description,
		       m.access_level, m.status, m.source_note, m.version, m.updated_at
		FROM library_materials m
		JOIN library_courses c ON c.id = m.course_id
		ORDER BY m.updated_at DESC
		LIMIT 300
	`)
	if err != nil {
		return ws, fmt.Errorf("query library_materials: %w", err)
	}
	defer matRows.Close()
	for matRows.Next() {
		var m Material
		if err := matRows.Scan(&m.ID, &m.CourseID, &m.CourseName, &m.Title, &m.Type, &m.Description, &m.AccessLevel, &m.Status, &m.SourceNote, &m.Version, &m.UpdatedAt); err != nil {
			return ws, fmt.Errorf("scan library_materials: %w", err)
		}
		ws.Materials = append(ws.Materials, m)
		if m.Status == "pending_review" || m.Status == "draft" {
			ws.Submissions = append(ws.Submissions, m)
		}
	}
	if err := matRows.Err(); err != nil {
		return ws, err
	}

	corrRows, err := h.database.Query(ctx, `
		SELECT k.id::text, COALESCE(k.material_id::text, ''), COALESCE(m.title, ''),
		       k.title, k.details, k.status, k.resolution_note, k.version, k.updated_at
		FROM library_corrections k
		LEFT JOIN library_materials m ON m.id = k.material_id
		ORDER BY CASE WHEN k.status = 'open' THEN 0 WHEN k.status = 'in_progress' THEN 1 ELSE 2 END, k.updated_at DESC
		LIMIT 200
	`)
	if err != nil {
		return ws, fmt.Errorf("query library_corrections: %w", err)
	}
	defer corrRows.Close()
	for corrRows.Next() {
		var item Correction
		if err := corrRows.Scan(&item.ID, &item.MaterialID, &item.MaterialTitle, &item.Title, &item.Description, &item.Status, &item.ResolutionNote, &item.Version, &item.UpdatedAt); err != nil {
			return ws, fmt.Errorf("scan library_corrections: %w", err)
		}
		item.TargetType = "material"
		item.TargetID = item.MaterialID
		item.Reason = item.Title
		ws.Corrections = append(ws.Corrections, item)
	}
	if err := corrRows.Err(); err != nil {
		return ws, err
	}

	relRows, err := h.database.Query(ctx, `
		SELECT r.release_id, r.state, r.activated_at, r.receipt_sha256,
		       (SELECT count(*) FROM library_public_material_snapshots s WHERE s.release_id = r.release_id) AS snapshot_count
		FROM library_public_releases r
		ORDER BY r.activated_at DESC
		LIMIT 30
	`)
	if err == nil {
		defer relRows.Close()
		for relRows.Next() {
			var rel ReleaseSummary
			if scanErr := relRows.Scan(&rel.ReleaseID, &rel.State, &rel.ActivatedAt, &rel.ReceiptSHA256, &rel.SnapshotCount); scanErr == nil {
				ws.Releases = append(ws.Releases, rel)
			}
		}
	}

	_ = h.database.QueryRow(ctx, `
		SELECT count(*), count(*) FILTER (WHERE started_at >= now() - interval '24 hours')
		FROM library_download_start_events
	`).Scan(&ws.DownloadStats.TotalStarts, &ws.DownloadStats.Last24hStarts)

	dlRows, err := h.database.Query(ctx, `
		SELECT e.id::text, e.material_id, COALESCE(s.title, e.material_id), COALESCE(s.access_level, 'public_free'), e.started_at
		FROM library_download_start_events e
		LEFT JOIN library_public_material_snapshots s ON s.release_id = e.release_id AND s.material_id = e.material_id
		ORDER BY e.started_at DESC
		LIMIT 30
	`)
	if err == nil {
		defer dlRows.Close()
		for dlRows.Next() {
			var d Download
			if scanErr := dlRows.Scan(&d.ID, &d.MaterialID, &d.MaterialTitle, &d.AccessLevel, &d.DownloadedAt); scanErr == nil {
				ws.Downloads = append(ws.Downloads, d)
			}
		}
	}

	return ws, nil
}

func (h *service) command(w http.ResponseWriter, r *http.Request) {
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if len(idempotencyKey) < 8 || len(idempotencyKey) > 200 {
		writeError(w, r, http.StatusBadRequest, "INVALID_IDEMPOTENCY_KEY", "Idempotency-Key is required")
		return
	}
	var input commandInput
	rawBody, ok := decode(w, r, &input)
	if !ok {
		return
	}
	input.Kind = strings.TrimSpace(input.Kind)
	if input.Kind == "" {
		writeError(w, r, http.StatusBadRequest, "INVALID_COMMAND", "command kind is required")
		return
	}
	perm := commandPermission(input.Kind)
	caller, ok := h.require(w, r, perm)
	if !ok {
		return
	}

	reqHashBytes := sha256.Sum256(rawBody)
	reqHash := hex.EncodeToString(reqHashBytes[:])
	normalizedRoute := "/api/v1/commands:" + input.Kind

	// Check existing idempotency record first.
	var existingState, existingHash string
	var existingResp []byte
	err := h.database.QueryRow(r.Context(), `
		SELECT state, request_hash, response
		FROM library_adapter_operations
		WHERE client_id = $1 AND actor_user_id = $2::uuid AND method = 'POST'
		  AND normalized_route = $3 AND idempotency_key = $4 AND expires_at > now()
	`, h.clientID, caller.userID, normalizedRoute, idempotencyKey).Scan(&existingState, &existingHash, &existingResp)
	if err == nil {
		if existingHash != reqHash {
			writeError(w, r, http.StatusConflict, "IDEMPOTENCY_CONFLICT", "Idempotency-Key was already used with a different payload")
			return
		}
		if existingState == "succeeded" && len(existingResp) > 0 {
			var res commandResult
			if json.Unmarshal(existingResp, &res) == nil {
				writeData(w, r, http.StatusOK, res)
				return
			}
		}
		writeError(w, r, http.StatusConflict, "OPERATION_IN_PROGRESS", "operation is already recorded")
		return
	} else if !errors.Is(err, pgx.ErrNoRows) {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "database query failed")
		return
	}

	tx, err := h.database.Begin(r.Context())
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "database transaction failed")
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()

	res, targetType, status, errCode, errMsg := h.executeCatalogCommand(r.Context(), tx, caller.userID, input)
	if status != http.StatusOK {
		writeError(w, r, status, errCode, errMsg)
		return
	}

	respJSON, _ := json.Marshal(res)
	opID := uuid.New()
	reqID := requestID(r)
	if _, err := tx.Exec(r.Context(), `
		INSERT INTO library_adapter_operations
		  (id, client_id, method, normalized_route, idempotency_key, operation, request_hash, state, response, actor_user_id, request_id, target_type, target_id, completed_at)
		VALUES ($1, $2, 'POST', $3, $4, $5, $6, 'succeeded', $7::jsonb, $8::uuid, $9, $10, $11, now())
	`, opID, h.clientID, normalizedRoute, idempotencyKey, input.Kind, reqHash, string(respJSON), caller.userID, reqID, targetType, res.ResourceID); err != nil {
		writeError(w, r, http.StatusConflict, "IDEMPOTENCY_CONFLICT", "operation already recorded")
		return
	}
	if _, err := tx.Exec(r.Context(), `
		INSERT INTO library_adapter_audit_events (id, operation_id, actor_user_id, request_id, action, target_type, target_id, outcome)
		VALUES ($1, $2, $3::uuid, $4, $5, $6, $7, 'succeeded')
	`, uuid.New(), opID, caller.userID, reqID, input.Kind, targetType, res.ResourceID); err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "AUDIT_WRITE_FAILED", "failed to record audit event")
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "failed to commit library command")
		return
	}
	writeData(w, r, http.StatusOK, res)
}

func parseExpectedVersion(raw string) (int, bool) {
	v, err := strconv.Atoi(strings.TrimSpace(raw))
	return v, err == nil && v >= 1
}

func (h *service) executeCatalogCommand(ctx context.Context, tx pgx.Tx, actorUserID string, input commandInput) (commandResult, string, int, string, string) {
	switch input.Kind {
	case "course_create":
		var payload struct {
			Code       string `json:"code"`
			Name       string `json:"name"`
			Department string `json:"department"`
		}
		if err := json.Unmarshal(input.Payload, &payload); err != nil {
			return commandResult{}, "course", http.StatusBadRequest, "INVALID_PAYLOAD", "course payload is invalid"
		}
		code := strings.TrimSpace(payload.Code)
		name := strings.TrimSpace(payload.Name)
		dept := strings.TrimSpace(payload.Department)
		if code == "" || len(code) > 64 || name == "" || len(name) > 120 {
			return commandResult{}, "course", http.StatusBadRequest, "INVALID_COURSE", "course code and name are required"
		}
		var id string
		var version int
		err := tx.QueryRow(ctx, `
			INSERT INTO library_courses (code, name, department, status)
			VALUES ($1, $2, $3, 'active')
			RETURNING id::text, version
		`, code, name, dept).Scan(&id, &version)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				return commandResult{}, "course", http.StatusConflict, "COURSE_CODE_CONFLICT", "course code already exists"
			}
			return commandResult{}, "course", http.StatusServiceUnavailable, "DATABASE_ERROR", "failed to create course"
		}
		return commandResult{Operation: input.Kind, ResourceID: id, State: "succeeded", Version: version}, "course", http.StatusOK, "", ""

	case "course_update":
		var payload struct {
			CourseID   string  `json:"course_id"`
			Name       *string `json:"name"`
			Department *string `json:"department"`
			Status     *string `json:"status"`
		}
		if len(input.Payload) > 0 {
			if err := json.Unmarshal(input.Payload, &payload); err != nil {
				return commandResult{}, "course", http.StatusBadRequest, "INVALID_PAYLOAD", "course update payload is invalid"
			}
		}
		courseID := strings.TrimSpace(input.ResourceID)
		if courseID == "" {
			courseID = strings.TrimSpace(payload.CourseID)
		}
		if _, err := uuid.Parse(courseID); err != nil {
			return commandResult{}, "course", http.StatusBadRequest, "INVALID_RESOURCE_ID", "valid course_id is required"
		}
		expectedVersion, ok := parseExpectedVersion(input.ExpectedVersion)
		if !ok {
			return commandResult{}, "course", http.StatusBadRequest, "INVALID_VERSION", "expected_version must be >= 1"
		}
		if payload.Status != nil && *payload.Status != "active" && *payload.Status != "archived" {
			return commandResult{}, "course", http.StatusBadRequest, "INVALID_STATUS", "course status must be active or archived"
		}
		var newVersion int
		err := tx.QueryRow(ctx, `
			UPDATE library_courses
			SET name = COALESCE(NULLIF(trim($3), ''), name),
			    department = COALESCE($4, department),
			    status = COALESCE($5, status),
			    version = version + 1,
			    updated_at = now()
			WHERE id = $1::uuid AND version = $2
			RETURNING version
		`, courseID, expectedVersion, payload.Name, payload.Department, payload.Status).Scan(&newVersion)
		if errors.Is(err, pgx.ErrNoRows) {
			return commandResult{}, "course", http.StatusConflict, "VERSION_CONFLICT", "course not found or version changed"
		}
		if err != nil {
			return commandResult{}, "course", http.StatusServiceUnavailable, "DATABASE_ERROR", "failed to update course"
		}
		return commandResult{Operation: input.Kind, ResourceID: courseID, State: "succeeded", Version: newVersion}, "course", http.StatusOK, "", ""

	case "material_create":
		var payload struct {
			CourseID     string `json:"course_id"`
			Title        string `json:"title"`
			MaterialType string `json:"material_type"`
			Description  string `json:"description"`
			AccessLevel  string `json:"access_level"`
			SourceNote   string `json:"source_note"`
			Status       string `json:"status"`
		}
		if err := json.Unmarshal(input.Payload, &payload); err != nil {
			return commandResult{}, "material", http.StatusBadRequest, "INVALID_PAYLOAD", "material payload is invalid"
		}
		if _, err := uuid.Parse(strings.TrimSpace(payload.CourseID)); err != nil {
			return commandResult{}, "material", http.StatusBadRequest, "INVALID_COURSE_ID", "valid course_id is required"
		}
		title := strings.TrimSpace(payload.Title)
		if title == "" || len(title) > 200 {
			return commandResult{}, "material", http.StatusBadRequest, "INVALID_TITLE", "material title is required"
		}
		if !allowedLibraryMaterialTypes[payload.MaterialType] {
			return commandResult{}, "material", http.StatusBadRequest, "INVALID_MATERIAL_TYPE", "material_type must be one of the 7 canonical types"
		}
		accessLevel := strings.TrimSpace(payload.AccessLevel)
		if accessLevel == "" {
			accessLevel = "public_free"
		}
		if !allowedLibraryAccessLevels[accessLevel] {
			return commandResult{}, "material", http.StatusBadRequest, "INVALID_ACCESS_LEVEL", "access_level is invalid"
		}
		status := strings.TrimSpace(payload.Status)
		if status == "" {
			status = "pending_review"
		}
		if !allowedLibraryMaterialStatuses[status] {
			return commandResult{}, "material", http.StatusBadRequest, "INVALID_STATUS", "status is invalid"
		}
		var id string
		var version int
		err := tx.QueryRow(ctx, `
			INSERT INTO library_materials (course_id, title, material_type, description, access_level, status, source_note, created_by, updated_by)
			VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, $8::uuid, $8::uuid)
			RETURNING id::text, version
		`, payload.CourseID, title, payload.MaterialType, strings.TrimSpace(payload.Description), accessLevel, status, strings.TrimSpace(payload.SourceNote), actorUserID).Scan(&id, &version)
		if err != nil {
			return commandResult{}, "material", http.StatusBadRequest, "MATERIAL_CREATE_FAILED", "failed to create material (verify course exists)"
		}
		return commandResult{Operation: input.Kind, ResourceID: id, State: "succeeded", Version: version}, "material", http.StatusOK, "", ""

	case "material_update":
		var payload struct {
			MaterialID   string  `json:"material_id"`
			Title        *string `json:"title"`
			MaterialType *string `json:"material_type"`
			Description  *string `json:"description"`
			AccessLevel  *string `json:"access_level"`
			Status       *string `json:"status"`
			SourceNote   *string `json:"source_note"`
		}
		if len(input.Payload) > 0 {
			if err := json.Unmarshal(input.Payload, &payload); err != nil {
				return commandResult{}, "material", http.StatusBadRequest, "INVALID_PAYLOAD", "material update payload is invalid"
			}
		}
		materialID := strings.TrimSpace(input.ResourceID)
		if materialID == "" {
			materialID = strings.TrimSpace(payload.MaterialID)
		}
		if _, err := uuid.Parse(materialID); err != nil {
			return commandResult{}, "material", http.StatusBadRequest, "INVALID_RESOURCE_ID", "valid material_id is required"
		}
		expectedVersion, ok := parseExpectedVersion(input.ExpectedVersion)
		if !ok {
			return commandResult{}, "material", http.StatusBadRequest, "INVALID_VERSION", "expected_version must be >= 1"
		}
		if payload.MaterialType != nil && !allowedLibraryMaterialTypes[*payload.MaterialType] {
			return commandResult{}, "material", http.StatusBadRequest, "INVALID_MATERIAL_TYPE", "material_type is invalid"
		}
		if payload.AccessLevel != nil && !allowedLibraryAccessLevels[*payload.AccessLevel] {
			return commandResult{}, "material", http.StatusBadRequest, "INVALID_ACCESS_LEVEL", "access_level is invalid"
		}
		if payload.Status != nil && !allowedLibraryMaterialStatuses[*payload.Status] {
			return commandResult{}, "material", http.StatusBadRequest, "INVALID_STATUS", "status is invalid"
		}
		var newVersion int
		err := tx.QueryRow(ctx, `
			UPDATE library_materials
			SET title = COALESCE(NULLIF(trim($3), ''), title),
			    material_type = COALESCE($4, material_type),
			    description = COALESCE($5, description),
			    access_level = COALESCE($6, access_level),
			    status = COALESCE($7, status),
			    source_note = COALESCE($8, source_note),
			    updated_by = $9::uuid,
			    version = version + 1,
			    updated_at = now()
			WHERE id = $1::uuid AND version = $2
			RETURNING version
		`, materialID, expectedVersion, payload.Title, payload.MaterialType, payload.Description, payload.AccessLevel, payload.Status, payload.SourceNote, actorUserID).Scan(&newVersion)
		if errors.Is(err, pgx.ErrNoRows) {
			return commandResult{}, "material", http.StatusConflict, "VERSION_CONFLICT", "material not found or version changed"
		}
		if err != nil {
			return commandResult{}, "material", http.StatusServiceUnavailable, "DATABASE_ERROR", "failed to update material"
		}
		return commandResult{Operation: input.Kind, ResourceID: materialID, State: "succeeded", Version: newVersion}, "material", http.StatusOK, "", ""

	case "submission_approve", "submission_reject":
		var payload struct {
			MaterialID string `json:"material_id"`
			Note       string `json:"note"`
		}
		if len(input.Payload) > 0 {
			_ = json.Unmarshal(input.Payload, &payload)
		}
		materialID := strings.TrimSpace(input.ResourceID)
		if materialID == "" {
			materialID = strings.TrimSpace(payload.MaterialID)
		}
		if _, err := uuid.Parse(materialID); err != nil {
			return commandResult{}, "material", http.StatusBadRequest, "INVALID_RESOURCE_ID", "valid material_id is required"
		}
		expectedVersion, ok := parseExpectedVersion(input.ExpectedVersion)
		if !ok {
			return commandResult{}, "material", http.StatusBadRequest, "INVALID_VERSION", "expected_version must be >= 1"
		}
		targetStatus := "published"
		if input.Kind == "submission_reject" {
			targetStatus = "archived"
		}
		note := strings.TrimSpace(payload.Note)
		var newVersion int
		err := tx.QueryRow(ctx, `
			UPDATE library_materials
			SET status = $3,
			    source_note = CASE WHEN $4 <> '' THEN $4 ELSE source_note END,
			    updated_by = $5::uuid,
			    version = version + 1,
			    updated_at = now()
			WHERE id = $1::uuid AND version = $2
			RETURNING version
		`, materialID, expectedVersion, targetStatus, note, actorUserID).Scan(&newVersion)
		if errors.Is(err, pgx.ErrNoRows) {
			return commandResult{}, "material", http.StatusConflict, "VERSION_CONFLICT", "material not found or version changed"
		}
		if err != nil {
			return commandResult{}, "material", http.StatusServiceUnavailable, "DATABASE_ERROR", "failed to review material"
		}
		return commandResult{Operation: input.Kind, ResourceID: materialID, State: "succeeded", Version: newVersion}, "material", http.StatusOK, "", ""

	case "correction_resolve":
		var payload struct {
			CorrectionID   string `json:"correction_id"`
			Status         string `json:"status"`
			ResolutionNote string `json:"resolution_note"`
		}
		if err := json.Unmarshal(input.Payload, &payload); err != nil {
			return commandResult{}, "correction", http.StatusBadRequest, "INVALID_PAYLOAD", "correction payload is invalid"
		}
		correctionID := strings.TrimSpace(input.ResourceID)
		if correctionID == "" {
			correctionID = strings.TrimSpace(payload.CorrectionID)
		}
		if _, err := uuid.Parse(correctionID); err != nil {
			return commandResult{}, "correction", http.StatusBadRequest, "INVALID_RESOURCE_ID", "valid correction_id is required"
		}
		expectedVersion, ok := parseExpectedVersion(input.ExpectedVersion)
		if !ok {
			return commandResult{}, "correction", http.StatusBadRequest, "INVALID_VERSION", "expected_version must be >= 1"
		}
		status := strings.TrimSpace(payload.Status)
		if status == "" {
			status = "resolved"
		}
		if !allowedLibraryCorrectionStatuses[status] {
			return commandResult{}, "correction", http.StatusBadRequest, "INVALID_STATUS", "correction status is invalid"
		}
		var newVersion int
		err := tx.QueryRow(ctx, `
			UPDATE library_corrections
			SET status = $3,
			    resolution_note = $4,
			    version = version + 1,
			    updated_at = now()
			WHERE id = $1::uuid AND version = $2
			RETURNING version
		`, correctionID, expectedVersion, status, strings.TrimSpace(payload.ResolutionNote)).Scan(&newVersion)
		if errors.Is(err, pgx.ErrNoRows) {
			return commandResult{}, "correction", http.StatusConflict, "VERSION_CONFLICT", "correction not found or version changed"
		}
		if err != nil {
			return commandResult{}, "correction", http.StatusServiceUnavailable, "DATABASE_ERROR", "failed to update correction"
		}
		return commandResult{Operation: input.Kind, ResourceID: correctionID, State: "succeeded", Version: newVersion}, "correction", http.StatusOK, "", ""

	default:
		return commandResult{}, "library", http.StatusBadRequest, "UNSUPPORTED_COMMAND", "unsupported library command kind"
	}
}

func (h *service) operationStatus(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if len(key) < 8 || len(key) > 200 {
		writeError(w, r, http.StatusBadRequest, "INVALID_IDEMPOTENCY_KEY", "Idempotency-Key is required")
		return
	}
	operation := strings.TrimSpace(chi.URLParam(r, "operation"))
	caller, ok := h.require(w, r, commandPermission(operation))
	if !ok {
		return
	}
	normalizedRoute := "/api/v1/commands:" + operation
	var state string
	var respBytes []byte
	var completedAt *time.Time
	err := h.database.QueryRow(r.Context(), `
		SELECT state, response, completed_at
		FROM library_adapter_operations
		WHERE client_id = $1 AND actor_user_id = $2::uuid AND method = 'POST'
		  AND normalized_route = $3 AND idempotency_key = $4
	`, h.clientID, caller.userID, normalizedRoute, key).Scan(&state, &respBytes, &completedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, r, http.StatusNotFound, "OPERATION_NOT_FOUND", "operation record was not found")
		return
	}
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "failed to load operation status")
		return
	}
	var res commandResult
	if len(respBytes) > 0 && json.Unmarshal(respBytes, &res) == nil {
		writeData(w, r, http.StatusOK, res)
		return
	}
	writeData(w, r, http.StatusOK, map[string]any{
		"operation":    operation,
		"state":        state,
		"completed_at": completedAt,
	})
}

func commandPermission(kind string) string {
	if strings.HasPrefix(kind, "submission_") || strings.HasPrefix(kind, "correction_") {
		return "library.review"
	}
	return "library.manage"
}
