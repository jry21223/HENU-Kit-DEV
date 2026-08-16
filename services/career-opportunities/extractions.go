package career

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// maxResumeBytes is the upload cap for one resume file. The authentication
// middleware allows maxResumeBytes + 1 MiB of body on the extraction route to
// cover multipart framing; the file itself is re-checked here.
const maxResumeBytes = 10 << 20

// defaultExtractRateLimit is how many resume extractions one actor may start
// per rolling hour unless Config.ExtractRateLimit overrides it. It bounds AI
// provider cost per user.
const defaultExtractRateLimit = 5

var (
	// errResumeTooLarge distinguishes an oversize upload (413) from any other
	// invalid file (400).
	errResumeTooLarge       = errors.New("resume file too large")
	allowedResumeExtensions = map[string]bool{
		".pdf":  true,
		".docx": true,
		".txt":  true,
	}
)

type extractionWire struct {
	ID        string            `json:"id"`
	Status    string            `json:"status"`
	UserID    string            `json:"user_id"`
	FileName  string            `json:"file_name"`
	ErrorCode string            `json:"error_code,omitempty"`
	ErrorMsg  string            `json:"error_message,omitempty"`
	Extracted *ExtractedProfile `json:"extracted,omitempty"`
	CreatedAt string            `json:"created_at"`
}

func (h *service) createExtraction(w http.ResponseWriter, r *http.Request) {
	value, ok := h.requireActor(w, r)
	if !ok {
		return
	}
	if h.extract == nil {
		// Production-safe off state: no AI provider is configured, so the
		// upload is rejected loudly instead of enqueuing a job that can only
		// fail. It is a 503, not a pretend success.
		writeError(w, r, http.StatusServiceUnavailable, "AI_UNCONFIGURED", "resume extraction is not configured")
		return
	}
	if !h.allowExtraction(r, value.userID) {
		writeError(w, r, http.StatusTooManyRequests, "EXTRACT_RATE_LIMITED", "resume extraction rate limit exceeded")
		return
	}
	fileName, content, err := readResumeUpload(r)
	if err != nil {
		if errors.Is(err, errResumeTooLarge) {
			writeError(w, r, http.StatusRequestEntityTooLarge, "FILE_TOO_LARGE", "resume file is too large")
			return
		}
		writeError(w, r, http.StatusBadRequest, "INVALID_FILE", "resume file is invalid")
		return
	}
	id := uuid.New()
	digest := sha256.Sum256(content)
	if _, err := h.database.Exec(r.Context(), `INSERT INTO career_resume_extractions(id,user_id,status,file_name,file_sha256,file_content) VALUES($1,$2,'queued',$3,$4,$5)`, id, value.userID, fileName, hex.EncodeToString(digest[:]), content); err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "career extraction is unavailable")
		return
	}
	extraction, found, err := h.loadExtraction(r, id.String(), value.userID)
	if err != nil || !found {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "career extraction is unavailable")
		return
	}
	writeData(w, r, http.StatusOK, map[string]any{"extraction": extraction})
}

// allowExtraction enforces the per-actor hourly budget with a short-lived
// Redis counter. A Redis failure fails closed: the actor is not silently
// granted an unlimited budget because the limiter is down.
func (h *service) allowExtraction(r *http.Request, userID string) bool {
	if h.extractRateLimit <= 0 {
		return true
	}
	key := "career:extract:rl:" + userID + ":" + h.now().UTC().Format("2006010215")
	count, err := h.redis.Incr(r.Context(), key).Result()
	if err != nil {
		return false
	}
	if count == 1 {
		_ = h.redis.Expire(r.Context(), key, time.Hour).Err()
	}
	return count <= int64(h.extractRateLimit)
}

// readResumeUpload extracts the single "file" part and validates it: a safe
// base file name, one of the allowed extensions, and matching magic bytes so
// a renamed payload cannot slip through as a different type. The request body
// is already capped by the authentication middleware; the per-file check here
// is what rejects a resume that fills the whole allowance.
func readResumeUpload(r *http.Request) (string, []byte, error) {
	if err := r.ParseMultipartForm(maxResumeBytes); err != nil {
		return "", nil, err
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		return "", nil, err
	}
	defer file.Close()
	fileName := strings.TrimSpace(header.Filename)
	if fileName == "" || strings.ContainsAny(fileName, `/\`) || fileName != filepath.Base(fileName) {
		return "", nil, errors.New("unsafe file name")
	}
	ext := strings.ToLower(filepath.Ext(fileName))
	if !allowedResumeExtensions[ext] {
		return "", nil, errors.New("unsupported file type")
	}
	content, err := io.ReadAll(io.LimitReader(file, maxResumeBytes+1))
	if err != nil || len(content) == 0 {
		return "", nil, errors.New("unreadable file")
	}
	if len(content) > maxResumeBytes {
		return "", nil, errResumeTooLarge
	}
	if !validResumeContent(ext, content) {
		return "", nil, errors.New("file content does not match its type")
	}
	return fileName, content, nil
}

func validResumeContent(ext string, content []byte) bool {
	switch ext {
	case ".pdf":
		return len(content) >= 5 && string(content[:4]) == "%PDF"
	case ".docx":
		return len(content) >= 4 && content[0] == 'P' && content[1] == 'K' && content[2] == 3 && content[3] == 4
	case ".txt":
		return utf8.Valid(content)
	default:
		return false
	}
}

func (h *service) extractionStatus(w http.ResponseWriter, r *http.Request) {
	value, ok := h.requireActor(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "extraction_id")
	if _, err := uuid.Parse(id); err != nil {
		writeError(w, r, http.StatusNotFound, "EXTRACTION_NOT_FOUND", "career extraction does not exist")
		return
	}
	extraction, found, err := h.loadExtraction(r, id, value.userID)
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "career extraction is unavailable")
		return
	}
	if !found {
		writeError(w, r, http.StatusNotFound, "EXTRACTION_NOT_FOUND", "career extraction does not exist")
		return
	}
	writeData(w, r, http.StatusOK, map[string]any{"extraction": extraction})
}

func (h *service) loadExtraction(r *http.Request, id, userID string) (extractionWire, bool, error) {
	var item extractionWire
	var extracted []byte
	var errorCode, errorMsg string
	var createdAt time.Time
	err := h.database.QueryRow(r.Context(), `SELECT id,status,user_id,file_name,COALESCE(error_code,''),COALESCE(error_message,''),extracted,created_at FROM career_resume_extractions WHERE id=$1 AND user_id=$2`, id, userID).Scan(&item.ID, &item.Status, &item.UserID, &item.FileName, &errorCode, &errorMsg, &extracted, &createdAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return extractionWire{}, false, nil
	}
	if err != nil {
		return extractionWire{}, false, err
	}
	item.ErrorCode = errorCode
	item.ErrorMsg = errorMsg
	if extracted != nil && item.Status == "completed" {
		var profile ExtractedProfile
		if err := json.Unmarshal(extracted, &profile); err == nil {
			item.Extracted = &profile
		}
	}
	item.CreatedAt = createdAt.UTC().Format(time.RFC3339)
	return item, true, nil
}
