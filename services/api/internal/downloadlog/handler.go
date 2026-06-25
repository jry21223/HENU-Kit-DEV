package downloadlog

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"final-review-platform/services/api/internal/auth"
	materialview "final-review-platform/services/api/internal/material"
	"final-review-platform/services/api/internal/platform/model"
	"final-review-platform/services/api/pkg/response"
)

type Handler struct {
	db *gorm.DB
}

func NewHandler(db *gorm.DB) Handler {
	return Handler{db: db}
}

type downloadRecord struct {
	ID           string                       `json:"id"`
	UserID       *string                      `json:"userId,omitempty"`
	MaterialID   string                       `json:"materialId"`
	AccessLevel  string                       `json:"accessLevel"`
	IP           string                       `json:"ip,omitempty"`
	UserAgent    string                       `json:"userAgent,omitempty"`
	DownloadedAt time.Time                    `json:"downloadedAt"`
	Material     *materialview.PublicMaterial `json:"material,omitempty"`
}

func (h Handler) MyDownloads(ctx *gin.Context) {
	user, ok := auth.CurrentUser(ctx)
	if !ok {
		response.Error(ctx, http.StatusUnauthorized, response.CodeUnauthorized, "unauthorized", nil)
		return
	}

	var logs []model.MaterialDownloadLog
	if err := h.db.Where("user_id = ?", user.ID).
		Order("downloaded_at desc").
		Limit(100).
		Find(&logs).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}

	records, err := h.withMaterials(logs, false)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"downloads": records})
}

func (h Handler) AdminDownloads(ctx *gin.Context) {
	query := h.db.Model(&model.MaterialDownloadLog{})
	if materialID := ctx.Query("materialId"); materialID != "" {
		query = query.Where("material_id = ?", materialID)
	}
	if userID := ctx.Query("userId"); userID != "" {
		query = query.Where("user_id = ?", userID)
	}

	var logs []model.MaterialDownloadLog
	if err := query.Order("downloaded_at desc").Limit(200).Find(&logs).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}

	records, err := h.withMaterials(logs, true)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"downloads": records})
}

func (h Handler) withMaterials(logs []model.MaterialDownloadLog, includeRequestMeta bool) ([]downloadRecord, error) {
	materialIDs := make([]string, 0, len(logs))
	for _, log := range logs {
		materialIDs = append(materialIDs, log.MaterialID)
	}

	materialsByID := map[string]model.Material{}
	if len(materialIDs) > 0 {
		var materials []model.Material
		if err := h.db.Where("id IN ?", materialIDs).Find(&materials).Error; err != nil {
			return nil, err
		}
		for _, material := range materials {
			materialsByID[material.ID] = material
		}
	}

	records := make([]downloadRecord, 0, len(logs))
	for _, log := range logs {
		record := downloadRecord{
			ID:           log.ID,
			UserID:       log.UserID,
			MaterialID:   log.MaterialID,
			AccessLevel:  log.AccessLevel,
			DownloadedAt: log.DownloadedAt,
		}
		if includeRequestMeta {
			record.IP = log.IP
			record.UserAgent = log.UserAgent
		}
		if material, ok := materialsByID[log.MaterialID]; ok {
			publicMaterial := materialview.ToPublic(material)
			record.Material = &publicMaterial
		}
		records = append(records, record)
	}
	return records, nil
}
