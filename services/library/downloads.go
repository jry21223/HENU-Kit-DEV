package library

import (
	"context"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const (
	downloadGrantTTL = 55 * time.Second
	publicOSSHost    = "henukit.oss-cn-beijing.aliyuncs.com"
)

type DownloadObjectState struct {
	Bytes      int64
	SHA256     string
	Encryption string
	VersionID  string
}

type SignedDownload struct {
	URL       string
	ExpiresAt time.Time
}

type DownloadObjectStore interface {
	Head(ctx context.Context, key, versionID string) (DownloadObjectState, bool, error)
	AnonymousDenied(ctx context.Context, key, versionID string) error
	PresignGet(ctx context.Context, key, versionID, contentDisposition string, ttl time.Duration) (SignedDownload, error)
}

type publicMaterialSnapshot struct {
	MaterialID      uuid.UUID
	ReleaseID       string
	ReceiptSHA256   string
	FileName        string
	ObjectKey       string
	ObjectVersionID string
	SHA256          string
	ByteSize        int64
}

func (h *service) startPublicDownload(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil || len(body) != 0 || r.URL.RawQuery != "" {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "download start accepts only a material id")
		return
	}
	materialID, err := uuid.Parse(r.PathValue("material_id"))
	if err != nil {
		writeError(w, r, http.StatusNotFound, "MATERIAL_NOT_AVAILABLE", "public material is unavailable")
		return
	}
	snapshot, found, err := h.activePublicMaterial(r.Context(), materialID)
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DOWNLOAD_TEMPORARILY_UNAVAILABLE", "download grant is temporarily unavailable")
		return
	}
	if !found {
		writeError(w, r, http.StatusNotFound, "MATERIAL_NOT_AVAILABLE", "public material is unavailable")
		return
	}
	if !validSnapshotObjectKey(snapshot) {
		writeError(w, r, http.StatusServiceUnavailable, "DOWNLOAD_TEMPORARILY_UNAVAILABLE", "download grant is temporarily unavailable")
		return
	}
	state, objectFound, err := h.downloadStore.Head(r.Context(), snapshot.ObjectKey, snapshot.ObjectVersionID)
	if err != nil || !objectFound || state.Bytes != snapshot.ByteSize || state.SHA256 != snapshot.SHA256 || state.Encryption != "AES256" || state.VersionID != snapshot.ObjectVersionID {
		writeError(w, r, http.StatusServiceUnavailable, "DOWNLOAD_TEMPORARILY_UNAVAILABLE", "download grant is temporarily unavailable")
		return
	}
	if err := h.downloadStore.AnonymousDenied(r.Context(), snapshot.ObjectKey, snapshot.ObjectVersionID); err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DOWNLOAD_TEMPORARILY_UNAVAILABLE", "download grant is temporarily unavailable")
		return
	}
	disposition, ok := attachmentDisposition(snapshot.FileName)
	if !ok {
		writeError(w, r, http.StatusServiceUnavailable, "DOWNLOAD_TEMPORARILY_UNAVAILABLE", "download grant is temporarily unavailable")
		return
	}
	issuedAt := h.now().UTC()
	signed, err := h.downloadStore.PresignGet(r.Context(), snapshot.ObjectKey, snapshot.ObjectVersionID, disposition, downloadGrantTTL)
	expiresAt, valid := validateSignedDownload(signed, issuedAt, snapshot, disposition)
	if err != nil || !valid {
		writeError(w, r, http.StatusServiceUnavailable, "DOWNLOAD_TEMPORARILY_UNAVAILABLE", "download grant is temporarily unavailable")
		return
	}
	eventID := uuid.New()
	created, err := h.recordDownloadStart(r.Context(), eventID, snapshot, issuedAt, expiresAt, requestID(r))
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DOWNLOAD_TEMPORARILY_UNAVAILABLE", "download grant is temporarily unavailable")
		return
	}
	if !created {
		writeError(w, r, http.StatusNotFound, "MATERIAL_NOT_AVAILABLE", "public material is unavailable")
		return
	}
	writeData(w, r, http.StatusCreated, map[string]any{
		"download_start_id": eventID,
		"method":            http.MethodGet,
		"location":          signed.URL,
		"expires_at":        expiresAt.Format(time.RFC3339),
	})
}

func validSnapshotObjectKey(snapshot publicMaterialSnapshot) bool {
	prefix := "releases/" + snapshot.ReleaseID + "/receipts/" + snapshot.ReceiptSHA256 + "/objects/" + snapshot.SHA256 + "/"
	if !releaseIDPattern.MatchString(snapshot.ReleaseID) || !digestPattern.MatchString(snapshot.ReceiptSHA256) || !digestPattern.MatchString(snapshot.SHA256) || !safeObjectVersionID(snapshot.ObjectVersionID) || !validOSSObjectKey(snapshot.ObjectKey) || !strings.HasPrefix(snapshot.ObjectKey, prefix) || len(snapshot.ObjectKey) <= len(prefix) {
		return false
	}
	for _, segment := range strings.Split(snapshot.ObjectKey[len(prefix):], "/") {
		if segment == "" || segment == "." || segment == ".." || strings.Contains(segment, `\`) {
			return false
		}
		for _, char := range segment {
			if unicode.IsControl(char) {
				return false
			}
		}
	}
	return true
}

func (h *service) activePublicMaterial(ctx context.Context, materialID uuid.UUID) (publicMaterialSnapshot, bool, error) {
	var snapshot publicMaterialSnapshot
	err := h.database.QueryRow(ctx, `
		SELECT m.material_id,m.release_id,r.receipt_sha256,m.file_name,m.object_key,m.object_version_id,m.sha256,m.byte_size
		FROM library_public_material_snapshots m
		JOIN library_public_releases r ON r.release_id=m.release_id
		WHERE m.material_id=$1 AND r.state='active' AND m.status='published' AND m.access_level='public_free'`, materialID).
		Scan(&snapshot.MaterialID, &snapshot.ReleaseID, &snapshot.ReceiptSHA256, &snapshot.FileName, &snapshot.ObjectKey, &snapshot.ObjectVersionID, &snapshot.SHA256, &snapshot.ByteSize)
	if errors.Is(err, pgx.ErrNoRows) {
		return publicMaterialSnapshot{}, false, nil
	}
	return snapshot, err == nil, err
}

func (h *service) recordDownloadStart(ctx context.Context, id uuid.UUID, snapshot publicMaterialSnapshot, issuedAt, expiresAt time.Time, requestID string) (bool, error) {
	result, err := h.database.Exec(ctx, `
		INSERT INTO library_download_start_events
			(id,material_id,release_id,receipt_sha256,object_version_id,request_id,issued_at,expires_at)
		SELECT $1,m.material_id,m.release_id,r.receipt_sha256,m.object_version_id,$2,$3,$4
		FROM library_public_material_snapshots m
		JOIN library_public_releases r ON r.release_id=m.release_id
		WHERE m.material_id=$5 AND m.release_id=$6 AND r.receipt_sha256=$7
		  AND m.object_key=$8 AND m.object_version_id=$9 AND m.sha256=$10 AND m.byte_size=$11 AND m.file_name=$12
		  AND r.state='active' AND m.status='published' AND m.access_level='public_free'`,
		id, requestID, issuedAt, expiresAt, snapshot.MaterialID, snapshot.ReleaseID, snapshot.ReceiptSHA256,
		snapshot.ObjectKey, snapshot.ObjectVersionID, snapshot.SHA256, snapshot.ByteSize, snapshot.FileName)
	if err != nil {
		return false, err
	}
	return result.RowsAffected() == 1, nil
}

func (h *service) globalDownloadAggregate(w http.ResponseWriter, r *http.Request) {
	var count int64
	var since time.Time
	err := h.database.QueryRow(r.Context(), `
		SELECT
			(SELECT count(*) FROM library_download_start_events),
			COALESCE(
				(SELECT min(activated_at) FROM library_public_releases),
				now()
			)`).Scan(&count, &since)
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "download aggregate is temporarily unavailable")
		return
	}
	writeData(w, r, http.StatusOK, map[string]any{"download_starts": count, "counting_since": since.UTC().Format(time.RFC3339), "as_of": h.now().UTC().Format(time.RFC3339)})
}

func (h *service) materialDownloadAggregate(w http.ResponseWriter, r *http.Request) {
	materialID, err := uuid.Parse(r.PathValue("material_id"))
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "material id is invalid")
		return
	}
	var count int64
	var since time.Time
	err = h.database.QueryRow(r.Context(), `
		SELECT
			(SELECT count(*) FROM library_download_start_events WHERE material_id=$1),
			COALESCE(
				(SELECT min(r.activated_at) FROM library_public_releases r JOIN library_public_material_snapshots m ON m.release_id=r.release_id WHERE m.material_id=$1),
				now()
			)`, materialID).Scan(&count, &since)
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "download aggregate is temporarily unavailable")
		return
	}
	writeData(w, r, http.StatusOK, map[string]any{"material_id": materialID, "download_starts": count, "counting_since": since.UTC().Format(time.RFC3339), "as_of": h.now().UTC().Format(time.RFC3339)})
}

func attachmentDisposition(fileName string) (string, bool) {
	if !safeDownloadFileName(fileName) {
		return "", false
	}
	return mime.FormatMediaType("attachment", map[string]string{"filename": fileName}), true
}

func validateSignedDownload(signed SignedDownload, issuedAt time.Time, snapshot publicMaterialSnapshot, disposition string) (time.Time, bool) {
	parsed, err := url.Parse(signed.URL)
	if err != nil || parsed.Scheme != "https" || parsed.Host != publicOSSHost || parsed.User != nil || parsed.Fragment != "" || parsed.Path != "/"+snapshot.ObjectKey || parsed.RawQuery == "" {
		return time.Time{}, false
	}
	query, err := url.ParseQuery(parsed.RawQuery)
	if err != nil {
		return time.Time{}, false
	}
	allowed := map[string]bool{"versionId": true, "response-cache-control": true, "response-content-disposition": true, "x-oss-credential": true, "x-oss-date": true, "x-oss-expires": true, "x-oss-security-token": true, "x-oss-signature": true, "x-oss-signature-version": true}
	for name := range query {
		if !allowed[name] {
			return time.Time{}, false
		}
	}
	single := func(name string) (string, bool) {
		values := query[name]
		returnValue := ""
		if len(values) == 1 {
			returnValue = values[0]
		}
		return returnValue, len(values) == 1 && strings.TrimSpace(returnValue) != ""
	}
	for name, expected := range map[string]string{
		"versionId":                    snapshot.ObjectVersionID,
		"response-cache-control":       "private, no-store",
		"response-content-disposition": disposition,
		"x-oss-signature-version":      "OSS4-HMAC-SHA256",
	} {
		value, ok := single(name)
		if !ok || value != expected {
			return time.Time{}, false
		}
	}
	for _, name := range []string{"x-oss-signature", "x-oss-security-token", "x-oss-credential"} {
		if _, ok := single(name); !ok {
			return time.Time{}, false
		}
	}
	expiresRaw, ok := single("x-oss-expires")
	if !ok {
		return time.Time{}, false
	}
	expiresSeconds, err := strconv.Atoi(expiresRaw)
	if err != nil || expiresSeconds < 1 || expiresSeconds > 60 {
		return time.Time{}, false
	}
	signedAtRaw, ok := single("x-oss-date")
	if !ok {
		return time.Time{}, false
	}
	signedAt, err := time.Parse("20060102T150405Z", signedAtRaw)
	if err != nil || signedAt.After(issuedAt.Add(5*time.Second)) {
		return time.Time{}, false
	}
	queryExpiresAt := signedAt.Add(time.Duration(expiresSeconds) * time.Second)
	valid := queryExpiresAt.After(issuedAt) && !queryExpiresAt.After(issuedAt.Add(time.Minute)) && queryExpiresAt.Equal(signed.ExpiresAt.UTC().Truncate(time.Second))
	return queryExpiresAt, valid
}
