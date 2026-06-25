package moment

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"final-review-platform/services/api/internal/auth"
	"final-review-platform/services/api/internal/platform/model"
	"final-review-platform/services/api/pkg/response"
)

const (
	visibilityPublic        = "public"
	visibilityMutualFriends = "mutual_friends"
	relationFollow          = "follow"
	relationBlock           = "block"
	relationMomentLike      = "moment_like"
	maxMomentImageBytes     = 5 * 1024 * 1024
	mediaUsageMomentImage   = "moment_image"
	mediaStatusUploaded     = "uploaded"
	mediaStatusAttached     = "attached"
)

type Handler struct {
	db        *gorm.DB
	uploadDir string
}

func NewHandler(db *gorm.DB, uploadDirs ...string) Handler {
	uploadDir := "uploads"
	if len(uploadDirs) > 0 && strings.TrimSpace(uploadDirs[0]) != "" {
		uploadDir = strings.TrimSpace(uploadDirs[0])
	}
	return Handler{db: db, uploadDir: uploadDir}
}

type createMomentRequest struct {
	Content    string   `json:"content"`
	Images     []string `json:"images"`
	Visibility string   `json:"visibility"`
}

type createCommentRequest struct {
	Content string `json:"content"`
}

type momentDTO struct {
	model.Moment
	Author      userSummary        `json:"author"`
	Images      []string           `json:"images"`
	LikedByMe   bool               `json:"likedByMe"`
	RecentReply []momentCommentDTO `json:"recentComments,omitempty"`
}

type momentCommentDTO struct {
	model.MomentComment
	Author userSummary `json:"author"`
}

type momentImageDTO struct {
	ID          string `json:"id"`
	URL         string `json:"url"`
	FileName    string `json:"fileName"`
	FileSize    int64  `json:"fileSize"`
	ContentType string `json:"contentType"`
}

type userSummary struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Role string `json:"role"`
}

func (h Handler) List(ctx *gin.Context) {
	currentUser, _ := auth.CurrentUser(ctx)
	limit, err := parseLimit(ctx.Query("limit"), 50, 100)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, err.Error(), nil)
		return
	}
	var moments []model.Moment
	if err := h.db.Where("status = ?", model.StatusPublished).Order("created_at desc").Limit(limit).Find(&moments).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	visible := make([]model.Moment, 0, len(moments))
	for _, item := range moments {
		if h.canViewMoment(currentUser, item) {
			visible = append(visible, item)
		}
	}
	response.OK(ctx, gin.H{"moments": h.momentDTOs(currentUser, visible)})
}

func (h Handler) Create(ctx *gin.Context) {
	user, ok := auth.CurrentUser(ctx)
	if !ok {
		response.Error(ctx, http.StatusUnauthorized, response.CodeUnauthorized, "unauthorized", nil)
		return
	}
	var req createMomentRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_request", nil)
		return
	}
	content := strings.TrimSpace(req.Content)
	if len([]rune(content)) == 0 {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "content_required", nil)
		return
	}
	if len([]rune(content)) > 500 {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "content_too_long", nil)
		return
	}
	visibility := strings.TrimSpace(req.Visibility)
	if visibility == "" {
		visibility = visibilityPublic
	}
	if visibility != visibilityPublic && visibility != visibilityMutualFriends {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_visibility", nil)
		return
	}
	if len(req.Images) > 9 {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "too_many_images", nil)
		return
	}
	imageAssets, images, err := h.validateImages(req.Images, user.ID)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, err.Error(), nil)
		return
	}
	moment := model.Moment{
		AuthorID: user.ID,
		Content:  content,
		Images:   images,
		Status:   model.StatusPublished,
	}
	moment.Visibility = visibility
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&moment).Error; err != nil {
			return err
		}
		for _, asset := range imageAssets {
			update := tx.Model(&model.MediaAsset{}).
				Where("id = ? AND owner_id = ? AND usage = ? AND status = ? AND moment_id IS NULL", asset.ID, user.ID, mediaUsageMomentImage, mediaStatusUploaded).
				Updates(map[string]interface{}{"moment_id": moment.ID, "status": mediaStatusAttached})
			if update.Error != nil {
				return update.Error
			}
			if update.RowsAffected != 1 {
				return errors.New("image_already_used")
			}
		}
		return nil
	}); err != nil {
		if err.Error() == "image_already_used" {
			response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, err.Error(), nil)
			return
		}
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "create_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"moment": h.momentDTO(user, moment)})
}

func (h Handler) UploadImage(ctx *gin.Context) {
	user, ok := auth.CurrentUser(ctx)
	if !ok {
		response.Error(ctx, http.StatusUnauthorized, response.CodeUnauthorized, "unauthorized", nil)
		return
	}
	ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, maxMomentImageBytes+1024)
	file, header, err := ctx.Request.FormFile("file")
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "too large") {
			response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "file_too_large", nil)
			return
		}
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "missing_file", nil)
		return
	}
	defer file.Close()

	if header.Size <= 0 || header.Size > maxMomentImageBytes {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "file_too_large", nil)
		return
	}
	originalName, ext, err := safeMomentImageFileName(header)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, err.Error(), nil)
		return
	}
	contentType, err := validateMomentImageContent(file, ext)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, err.Error(), nil)
		return
	}

	storageKey := filepath.ToSlash(filepath.Join("moments", user.ID, uuid.NewString()+ext))
	targetPath, err := safeUploadPath(h.uploadDir, storageKey)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "unsafe_image_path", nil)
		return
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "upload_failed", nil)
		return
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "upload_failed", nil)
		return
	}
	target, err := os.OpenFile(targetPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "upload_failed", nil)
		return
	}
	written, err := io.Copy(target, io.LimitReader(file, maxMomentImageBytes+1))
	closeErr := target.Close()
	if err != nil || closeErr != nil || written > maxMomentImageBytes {
		_ = os.Remove(targetPath)
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "upload_failed", nil)
		return
	}

	asset := model.MediaAsset{
		OwnerID:     user.ID,
		Usage:       mediaUsageMomentImage,
		StorageKey:  storageKey,
		FileName:    originalName,
		FileSize:    written,
		ContentType: contentType,
		Status:      mediaStatusUploaded,
	}
	if err := h.db.Create(&asset).Error; err != nil {
		_ = os.Remove(targetPath)
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "upload_failed", nil)
		return
	}

	response.OK(ctx, gin.H{"image": momentImageDTO{
		ID:          asset.ID,
		URL:         momentImageURL(asset.ID),
		FileName:    originalName,
		FileSize:    written,
		ContentType: contentType,
	}})
}

func (h Handler) ServeImage(ctx *gin.Context) {
	currentUser, _ := auth.CurrentUser(ctx)
	var asset model.MediaAsset
	if err := h.db.First(&asset, "id = ? AND usage = ?", ctx.Param("id"), mediaUsageMomentImage).Error; err != nil {
		response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "image_not_found", nil)
		return
	}
	if !h.canViewMediaAsset(currentUser, asset) {
		response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "image_not_found", nil)
		return
	}
	targetPath, err := safeUploadPath(h.uploadDir, asset.StorageKey)
	if err != nil {
		response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "image_not_found", nil)
		return
	}
	info, err := os.Stat(targetPath)
	if err != nil || info.IsDir() {
		response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "image_not_found", nil)
		return
	}
	ctx.Header("Cache-Control", "private, max-age=60")
	if asset.ContentType != "" {
		ctx.Header("Content-Type", asset.ContentType)
	}
	ctx.File(targetPath)
}

func (h Handler) Delete(ctx *gin.Context) {
	user, ok := auth.CurrentUser(ctx)
	if !ok {
		response.Error(ctx, http.StatusUnauthorized, response.CodeUnauthorized, "unauthorized", nil)
		return
	}
	var item model.Moment
	if err := h.db.First(&item, "id = ?", ctx.Param("id")).Error; err != nil {
		response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "moment_not_found", nil)
		return
	}
	if item.AuthorID != user.ID && user.Role != model.RoleAdmin && user.Role != model.RoleSuperAdmin {
		response.Error(ctx, http.StatusForbidden, response.CodeForbidden, "forbidden", nil)
		return
	}
	if err := h.db.Delete(&item).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "delete_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"deleted": true})
}

func (h Handler) Like(ctx *gin.Context) {
	user, ok := auth.CurrentUser(ctx)
	if !ok {
		response.Error(ctx, http.StatusUnauthorized, response.CodeUnauthorized, "unauthorized", nil)
		return
	}
	var item model.Moment
	if err := h.db.First(&item, "id = ?", ctx.Param("id")).Error; err != nil || !h.canViewMoment(user, item) {
		response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "moment_not_found", nil)
		return
	}
	created := false
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		var existing model.UserRelation
		err := tx.Where("user_id = ? AND target_id = ? AND type = ?", user.ID, item.ID, relationMomentLike).First(&existing).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if err := tx.Create(&model.UserRelation{UserID: user.ID, TargetID: item.ID, Type: relationMomentLike}).Error; err != nil {
				return err
			}
			if err := tx.Model(&model.Moment{}).Where("id = ?", item.ID).UpdateColumn("like_count", gorm.Expr("like_count + ?", 1)).Error; err != nil {
				return err
			}
			created = true
		}
		return tx.First(&item, "id = ?", item.ID).Error
	}); err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "like_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"liked": true, "created": created, "moment": h.momentDTO(user, item)})
}

func (h Handler) CreateComment(ctx *gin.Context) {
	user, ok := auth.CurrentUser(ctx)
	if !ok {
		response.Error(ctx, http.StatusUnauthorized, response.CodeUnauthorized, "unauthorized", nil)
		return
	}
	var item model.Moment
	if err := h.db.First(&item, "id = ?", ctx.Param("id")).Error; err != nil || !h.canViewMoment(user, item) {
		response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "moment_not_found", nil)
		return
	}
	var req createCommentRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_request", nil)
		return
	}
	content := strings.TrimSpace(req.Content)
	if len([]rune(content)) == 0 {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "content_required", nil)
		return
	}
	if len([]rune(content)) > 500 {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "content_too_long", nil)
		return
	}
	comment := model.MomentComment{AuthorID: user.ID, MomentID: item.ID, Content: content, Status: model.StatusPublished}
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&comment).Error; err != nil {
			return err
		}
		return tx.Model(&model.Moment{}).Where("id = ?", item.ID).UpdateColumn("comment_count", gorm.Expr("comment_count + ?", 1)).Error
	}); err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "comment_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"comment": h.commentDTO(comment)})
}

func (h Handler) DeleteComment(ctx *gin.Context) {
	user, ok := auth.CurrentUser(ctx)
	if !ok {
		response.Error(ctx, http.StatusUnauthorized, response.CodeUnauthorized, "unauthorized", nil)
		return
	}
	var comment model.MomentComment
	if err := h.db.First(&comment, "id = ?", ctx.Param("id")).Error; err != nil {
		response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "comment_not_found", nil)
		return
	}
	var item model.Moment
	if err := h.db.First(&item, "id = ?", comment.MomentID).Error; err != nil {
		response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "moment_not_found", nil)
		return
	}
	if comment.AuthorID != user.ID && item.AuthorID != user.ID && user.Role != model.RoleAdmin && user.Role != model.RoleSuperAdmin {
		response.Error(ctx, http.StatusForbidden, response.CodeForbidden, "forbidden", nil)
		return
	}
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&comment).Error; err != nil {
			return err
		}
		return tx.Model(&model.Moment{}).Where("id = ? AND comment_count > 0", item.ID).UpdateColumn("comment_count", gorm.Expr("comment_count - ?", 1)).Error
	}); err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "delete_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"deleted": true})
}

func (h Handler) momentDTOs(currentUser *model.User, moments []model.Moment) []momentDTO {
	dtos := make([]momentDTO, 0, len(moments))
	for _, item := range moments {
		dtos = append(dtos, h.momentDTO(currentUser, item))
	}
	return dtos
}

func (h Handler) momentDTO(currentUser *model.User, item model.Moment) momentDTO {
	return momentDTO{
		Moment:      item,
		Author:      h.userSummary(item.AuthorID),
		Images:      decodeImages(item.Images),
		LikedByMe:   currentUser != nil && h.hasRelation(currentUser.ID, item.ID, relationMomentLike),
		RecentReply: h.recentComments(item.ID),
	}
}

func (h Handler) recentComments(momentID string) []momentCommentDTO {
	var comments []model.MomentComment
	if err := h.db.Where("moment_id = ? AND status = ?", momentID, model.StatusPublished).Order("created_at asc").Limit(5).Find(&comments).Error; err != nil {
		return []momentCommentDTO{}
	}
	dtos := make([]momentCommentDTO, 0, len(comments))
	for _, comment := range comments {
		dtos = append(dtos, h.commentDTO(comment))
	}
	return dtos
}

func (h Handler) commentDTO(comment model.MomentComment) momentCommentDTO {
	return momentCommentDTO{MomentComment: comment, Author: h.userSummary(comment.AuthorID)}
}

func (h Handler) userSummary(userID string) userSummary {
	var user model.User
	if err := h.db.First(&user, "id = ?", userID).Error; err != nil {
		return userSummary{ID: userID}
	}
	return userSummary{ID: user.ID, Name: user.Name, Role: user.Role}
}

func (h Handler) canViewMoment(currentUser *model.User, item model.Moment) bool {
	if item.Status != model.StatusPublished {
		return false
	}
	if item.Visibility == visibilityPublic || item.Visibility == "" {
		if currentUser == nil {
			return true
		}
		return !h.hasBlockEitherDirection(currentUser.ID, item.AuthorID)
	}
	if item.Visibility != visibilityMutualFriends || currentUser == nil {
		return false
	}
	if currentUser.ID == item.AuthorID {
		return true
	}
	return !h.hasBlockEitherDirection(currentUser.ID, item.AuthorID) &&
		h.hasRelation(currentUser.ID, item.AuthorID, relationFollow) &&
		h.hasRelation(item.AuthorID, currentUser.ID, relationFollow)
}

func (h Handler) hasRelation(userID string, targetID string, relationType string) bool {
	var count int64
	if err := h.db.Model(&model.UserRelation{}).Where("user_id = ? AND target_id = ? AND type = ?", userID, targetID, relationType).Count(&count).Error; err != nil {
		return false
	}
	return count > 0
}

func (h Handler) hasBlockEitherDirection(leftID string, rightID string) bool {
	var count int64
	if err := h.db.Model(&model.UserRelation{}).
		Where("type = ? AND ((user_id = ? AND target_id = ?) OR (user_id = ? AND target_id = ?))", relationBlock, leftID, rightID, rightID, leftID).
		Count(&count).Error; err != nil {
		return false
	}
	return count > 0
}

func (h Handler) canViewMediaAsset(currentUser *model.User, asset model.MediaAsset) bool {
	if currentUser != nil && currentUser.ID == asset.OwnerID {
		return true
	}
	if asset.Status != mediaStatusAttached || asset.MomentID == nil || *asset.MomentID == "" {
		return false
	}
	var item model.Moment
	if err := h.db.First(&item, "id = ?", *asset.MomentID).Error; err != nil {
		return false
	}
	return h.canViewMoment(currentUser, item)
}

func (h Handler) validateImages(values []string, userID string) ([]model.MediaAsset, datatypes.JSON, error) {
	cleaned := make([]string, 0, len(values))
	assets := make([]model.MediaAsset, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if len([]rune(trimmed)) > 500 {
			return nil, nil, errors.New("image_url_too_long")
		}
		mediaID, ok := mediaIDFromURL(trimmed)
		if !ok {
			return nil, nil, errors.New("invalid_image_url")
		}
		if _, ok := seen[mediaID]; ok {
			return nil, nil, errors.New("duplicate_image")
		}
		seen[mediaID] = struct{}{}
		var asset model.MediaAsset
		if err := h.db.First(&asset, "id = ? AND owner_id = ? AND usage = ? AND status = ? AND moment_id IS NULL", mediaID, userID, mediaUsageMomentImage, mediaStatusUploaded).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, nil, errors.New("image_not_found")
			}
			return nil, nil, err
		}
		path, err := safeUploadPath(h.uploadDir, asset.StorageKey)
		if err != nil {
			return nil, nil, errors.New("invalid_image_url")
		}
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			return nil, nil, errors.New("image_not_found")
		}
		assets = append(assets, asset)
		cleaned = append(cleaned, momentImageURL(asset.ID))
	}
	raw, err := json.Marshal(cleaned)
	if err != nil {
		return nil, nil, err
	}
	return assets, datatypes.JSON(raw), nil
}

func decodeImages(raw datatypes.JSON) []string {
	if len(raw) == 0 {
		return []string{}
	}
	var images []string
	if err := json.Unmarshal(raw, &images); err != nil {
		return []string{}
	}
	safeImages := make([]string, 0, len(images))
	for _, image := range images {
		trimmed := strings.TrimSpace(image)
		if _, ok := mediaIDFromURL(trimmed); ok {
			safeImages = append(safeImages, trimmed)
		}
	}
	return safeImages
}

func parseLimit(value string, fallback int, max int) (int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback, nil
	}
	limit, err := strconv.Atoi(value)
	if err != nil || limit <= 0 || limit > max {
		return 0, errors.New("invalid_limit")
	}
	return limit, nil
}

func momentImageURL(id string) string {
	return "/api/v1/moments/images/" + id
}

func mediaIDFromURL(value string) (string, bool) {
	id := strings.TrimPrefix(strings.TrimSpace(value), "/api/v1/moments/images/")
	if id == value || id == "" || strings.Contains(id, "/") || strings.Contains(id, `\`) {
		return "", false
	}
	return id, true
}

var momentImageContentTypes = map[string]string{
	".gif":  "image/gif",
	".jpeg": "image/jpeg",
	".jpg":  "image/jpeg",
	".png":  "image/png",
	".webp": "image/webp",
}

func safeMomentImageFileName(header *multipart.FileHeader) (string, string, error) {
	name := strings.TrimSpace(header.Filename)
	if name == "" || strings.ContainsAny(name, `/\`) || name != filepath.Base(name) {
		return "", "", errors.New("unsafe_file_name")
	}
	ext := strings.ToLower(filepath.Ext(name))
	if _, ok := momentImageContentTypes[ext]; !ok {
		return "", "", errors.New("unsupported_image_type")
	}
	return name, ext, nil
}

func validateMomentImageContent(file multipart.File, ext string) (string, error) {
	buffer := make([]byte, 512)
	n, err := file.Read(buffer)
	if err != nil && err != io.EOF {
		return "", errors.New("invalid_image_content")
	}
	content := buffer[:n]
	contentType := detectMomentImageContentType(content)
	if contentType == "" || contentType != momentImageContentTypes[ext] {
		return "", errors.New("invalid_image_content")
	}
	return contentType, nil
}

func detectMomentImageContentType(content []byte) string {
	switch {
	case len(content) >= 3 && content[0] == 0xff && content[1] == 0xd8 && content[2] == 0xff:
		return "image/jpeg"
	case len(content) >= 8 && bytes.Equal(content[:8], []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}):
		return "image/png"
	case len(content) >= 6 && (bytes.Equal(content[:6], []byte("GIF87a")) || bytes.Equal(content[:6], []byte("GIF89a"))):
		return "image/gif"
	case len(content) >= 12 && bytes.Equal(content[:4], []byte("RIFF")) && bytes.Equal(content[8:12], []byte("WEBP")):
		return "image/webp"
	default:
		return ""
	}
}

func safeUploadPath(uploadDir string, storageKey string) (string, error) {
	normalizedKey := filepath.ToSlash(strings.TrimSpace(storageKey))
	if normalizedKey == "" || strings.Contains(normalizedKey, `\`) {
		return "", errors.New("unsafe_path")
	}
	for _, part := range strings.Split(normalizedKey, "/") {
		if part == "" || part == "." || part == ".." {
			return "", errors.New("unsafe_path")
		}
	}
	cleanKey := filepath.Clean(filepath.FromSlash(normalizedKey))
	if cleanKey == "" || cleanKey == "." || cleanKey == ".." || filepath.IsAbs(cleanKey) || strings.HasPrefix(cleanKey, ".."+string(filepath.Separator)) {
		return "", errors.New("unsafe_path")
	}
	root, err := filepath.Abs(uploadDir)
	if err != nil {
		return "", err
	}
	target, err := filepath.Abs(filepath.Join(root, cleanKey))
	if err != nil {
		return "", err
	}
	if target != root && !strings.HasPrefix(target, root+string(filepath.Separator)) {
		return "", errors.New("unsafe_path")
	}
	return target, nil
}
