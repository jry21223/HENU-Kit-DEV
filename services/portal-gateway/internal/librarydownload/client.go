// Package librarydownload is Portal Gateway's narrow anonymous download-start
// client for the Library owner. It accepts no browser-selected storage input.
package librarydownload

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"henukit.dev/portal-gateway/internal/serviceauth"
)

// 500 contract-bounded rows can exceed 1 MiB after encoding because Go escapes
// HTML-sensitive characters as six-byte JSON sequences. The field maxima total
// less than 2.5 MiB for 500 rows; 4 MiB leaves bounded envelope overhead.
const maxCatalogBodyBytes = 4 << 20

const PublicOSSHost = "henukit.oss-cn-beijing.aliyuncs.com"

var (
	materialIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
	releaseIDPattern  = regexp.MustCompile(`^[a-f0-9]{40}-[a-f0-9]{16}$`)
	ownerUUIDPattern  = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	ErrBadRequest     = errors.New("invalid Library download request")
	ErrNotFound       = errors.New("Library material is unavailable")
	ErrUnavailable    = errors.New("Library download owner is unavailable")
	ErrInvalid        = errors.New("Library download owner returned an invalid capability")
)

type PublicMaterial struct {
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

type Catalog struct {
	ReleaseID      *string          `json:"release_id"`
	Materials      []PublicMaterial `json:"materials"`
	MaterialCount  int64            `json:"material_count"`
	DownloadStarts int64            `json:"download_starts"`
	CountingSince  *time.Time
	AsOf           time.Time
}

type Grant struct {
	DownloadStartID string
	Method          string
	Location        string
	ExpiresAt       time.Time
}

type Client struct {
	baseURL    string
	httpClient *http.Client
	signer     *serviceauth.Signer
	now        func() time.Time
}

func NewClient(baseURL, clientID, clientSecret, keyID string) (*Client, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || strings.TrimSpace(clientID) == "" || len(clientSecret) < 32 || strings.TrimSpace(keyID) == "" {
		return nil, errors.New("invalid Library download client configuration")
	}
	return &Client{
		baseURL: strings.TrimRight(parsed.String(), "/"),
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		signer: serviceauth.NewSigner(clientID, clientSecret, keyID),
		now:    time.Now,
	}, nil
}

func (c *Client) Start(ctx context.Context, materialID, requestID string) (Grant, error) {
	if c == nil || c.signer == nil || c.httpClient == nil || !materialIDPattern.MatchString(materialID) || strings.TrimSpace(requestID) == "" {
		return Grant{}, ErrBadRequest
	}
	path := "/api/v1/public-materials/" + materialID + "/download-starts"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, nil)
	if err != nil {
		return Grant{}, ErrUnavailable
	}
	request.Header.Set("X-Request-Id", requestID)
	if err := c.signer.Sign(request); err != nil {
		return Grant{}, ErrUnavailable
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return Grant{}, ErrUnavailable
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case http.StatusCreated:
	case http.StatusBadRequest:
		return Grant{}, ErrBadRequest
	case http.StatusNotFound, http.StatusGone:
		return Grant{}, ErrNotFound
	case http.StatusConflict:
		return Grant{}, ErrUnavailable
	default:
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
		return Grant{}, fmt.Errorf("Library download owner status %d: %w", response.StatusCode, ErrUnavailable)
	}

	var envelope struct {
		Data struct {
			DownloadStartID string `json:"download_start_id"`
			Method          string `json:"method"`
			Location        string `json:"location"`
			ExpiresAt       string `json:"expires_at"`
		} `json:"data"`
		RequestID string `json:"request_id"`
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, (64<<10)+1))
	if err != nil || len(body) > 64<<10 {
		return Grant{}, ErrInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil || strings.TrimSpace(envelope.RequestID) == "" || strings.TrimSpace(envelope.Data.DownloadStartID) == "" || envelope.Data.Method != http.MethodGet {
		return Grant{}, ErrInvalid
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return Grant{}, ErrInvalid
	}
	expiresAt, err := time.Parse(time.RFC3339, envelope.Data.ExpiresAt)
	if err != nil {
		return Grant{}, ErrInvalid
	}
	now := c.now().UTC()
	if !expiresAt.After(now) || expiresAt.After(now.Add(60*time.Second)) {
		return Grant{}, ErrInvalid
	}
	location, err := url.Parse(envelope.Data.Location)
	if err != nil || location.Scheme != "https" || location.Host != PublicOSSHost || location.User != nil || location.Fragment != "" || location.RawQuery == "" || location.Path == "" {
		return Grant{}, ErrInvalid
	}
	query, err := url.ParseQuery(location.RawQuery)
	if err != nil {
		return Grant{}, ErrInvalid
	}
	allowed := map[string]bool{
		"versionId": true, "response-cache-control": true, "response-content-disposition": true,
		"x-oss-credential": true, "x-oss-date": true, "x-oss-expires": true,
		"x-oss-security-token": true, "x-oss-signature": true, "x-oss-signature-version": true,
	}
	for name := range query {
		if !allowed[name] {
			return Grant{}, ErrInvalid
		}
	}
	requireSingle := func(name string) (string, bool) {
		values, ok := query[name]
		returnValue := ""
		if len(values) == 1 {
			returnValue = values[0]
		}
		return returnValue, ok && len(values) == 1 && strings.TrimSpace(returnValue) != ""
	}
	for _, name := range []string{"versionId", "x-oss-signature", "x-oss-security-token", "x-oss-credential"} {
		if _, ok := requireSingle(name); !ok {
			return Grant{}, ErrInvalid
		}
	}
	if signatureVersion, ok := requireSingle("x-oss-signature-version"); !ok || signatureVersion != "OSS4-HMAC-SHA256" {
		return Grant{}, ErrInvalid
	}
	if cacheControl, ok := requireSingle("response-cache-control"); !ok || cacheControl != "private, no-store" {
		return Grant{}, ErrInvalid
	}
	disposition, ok := requireSingle("response-content-disposition")
	if !ok || !safeAttachmentDisposition(disposition) {
		return Grant{}, ErrInvalid
	}
	expiresSecondsRaw, ok := requireSingle("x-oss-expires")
	if !ok {
		return Grant{}, ErrInvalid
	}
	expiresSeconds, err := strconv.Atoi(expiresSecondsRaw)
	if err != nil || expiresSeconds < 1 || expiresSeconds > 60 {
		return Grant{}, ErrInvalid
	}
	signedAtRaw, ok := requireSingle("x-oss-date")
	if !ok {
		return Grant{}, ErrInvalid
	}
	signedAt, err := time.Parse("20060102T150405Z", signedAtRaw)
	if err != nil {
		return Grant{}, ErrInvalid
	}
	queryExpiresAt := signedAt.Add(time.Duration(expiresSeconds) * time.Second)
	if signedAt.After(now.Add(5*time.Second)) || !queryExpiresAt.After(now) || queryExpiresAt.After(now.Add(60*time.Second)) || !queryExpiresAt.Equal(expiresAt) {
		return Grant{}, ErrInvalid
	}
	return Grant{DownloadStartID: envelope.Data.DownloadStartID, Method: envelope.Data.Method, Location: location.String(), ExpiresAt: expiresAt}, nil
}

// Catalog reads one complete active public-material snapshot and its ledger
// aggregates. It never accepts browser filters or storage coordinates.
func (c *Client) Catalog(ctx context.Context, requestID string) (Catalog, error) {
	if c == nil || c.signer == nil || c.httpClient == nil || strings.TrimSpace(requestID) == "" {
		return Catalog{}, ErrBadRequest
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v1/public-materials", nil)
	if err != nil {
		return Catalog{}, ErrUnavailable
	}
	request.Header.Set("X-Request-Id", requestID)
	if err := c.signer.Sign(request); err != nil {
		return Catalog{}, ErrUnavailable
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return Catalog{}, ErrUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
		return Catalog{}, ErrUnavailable
	}

	var envelope struct {
		Data struct {
			ReleaseID      *string          `json:"release_id"`
			Materials      []PublicMaterial `json:"materials"`
			MaterialCount  int64            `json:"material_count"`
			DownloadStarts int64            `json:"download_starts"`
			CountingSince  *string          `json:"counting_since"`
			AsOf           string           `json:"as_of"`
		} `json:"data"`
		RequestID string `json:"request_id"`
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxCatalogBodyBytes+1))
	if err != nil || len(body) > maxCatalogBodyBytes {
		return Catalog{}, ErrInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil || strings.TrimSpace(envelope.RequestID) == "" {
		return Catalog{}, ErrInvalid
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return Catalog{}, ErrInvalid
	}
	asOf, err := time.Parse(time.RFC3339, envelope.Data.AsOf)
	if err != nil || envelope.Data.MaterialCount < 0 || envelope.Data.DownloadStarts < 0 || envelope.Data.MaterialCount != int64(len(envelope.Data.Materials)) || len(envelope.Data.Materials) > 500 {
		return Catalog{}, ErrInvalid
	}
	var countingSince *time.Time
	if envelope.Data.CountingSince != nil {
		parsed, parseErr := time.Parse(time.RFC3339, *envelope.Data.CountingSince)
		if parseErr != nil || asOf.Before(parsed) {
			return Catalog{}, ErrInvalid
		}
		countingSince = &parsed
	}
	if envelope.Data.ReleaseID == nil {
		if len(envelope.Data.Materials) != 0 {
			return Catalog{}, ErrInvalid
		}
	} else if !releaseIDPattern.MatchString(*envelope.Data.ReleaseID) {
		return Catalog{}, ErrInvalid
	}
	seen := make(map[string]struct{}, len(envelope.Data.Materials))
	for _, material := range envelope.Data.Materials {
		if !validPublicMaterial(material) {
			return Catalog{}, ErrInvalid
		}
		if _, duplicate := seen[material.ID]; duplicate {
			return Catalog{}, ErrInvalid
		}
		seen[material.ID] = struct{}{}
	}
	return Catalog{
		ReleaseID: envelope.Data.ReleaseID, Materials: envelope.Data.Materials,
		MaterialCount: envelope.Data.MaterialCount, DownloadStarts: envelope.Data.DownloadStarts,
		CountingSince: countingSince, AsOf: asOf,
	}, nil
}

func validPublicMaterial(material PublicMaterial) bool {
	allowedTypes := map[string]bool{"note": true, "exam": true, "mock": true, "path": true, "lab": true, "slides": true}
	if !ownerUUIDPattern.MatchString(material.ID) || !allowedTypes[material.Type] || material.FileSize < 0 || material.Downloads < 0 || !material.DownloadAvailable {
		return false
	}
	for _, value := range []string{material.Subject, material.Title, material.Role, material.FileName} {
		if strings.TrimSpace(value) == "" || strings.TrimSpace(value) != value || len(value) > 255 {
			return false
		}
		for _, char := range value {
			if unicode.IsControl(char) {
				return false
			}
		}
	}
	return !strings.ContainsAny(material.FileName, `/\\`)
}

func safeAttachmentDisposition(value string) bool {
	mediaType, parameters, err := mime.ParseMediaType(value)
	if err != nil || !strings.EqualFold(mediaType, "attachment") {
		return false
	}
	fileName := parameters["filename"]
	if fileName == "" || len(fileName) > 255 || strings.TrimSpace(fileName) != fileName || strings.ContainsAny(fileName, `/\`) {
		return false
	}
	for _, char := range fileName {
		if unicode.IsControl(char) {
			return false
		}
	}
	return true
}
