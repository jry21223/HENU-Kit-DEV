package course

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"final-review-platform/services/api/internal/platform/model"
	"final-review-platform/services/api/pkg/response"
)

type Handler struct {
	db *gorm.DB
}

func NewHandler(db *gorm.DB) Handler {
	return Handler{db: db}
}

func (h Handler) CourseMaterials(ctx *gin.Context) {
	var materials []model.Material
	if err := h.db.Where("course_id = ? AND status = ?", ctx.Param("id"), model.StatusPublished).
		Order("created_at asc").
		Find(&materials).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"materials": materials})
}
