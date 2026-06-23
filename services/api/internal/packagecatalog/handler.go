package packagecatalog

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"final-review-platform/services/api/internal/platform/model"
	"final-review-platform/services/api/pkg/response"
)

const resourceTypeMaterial = "material"

type Handler struct {
	db *gorm.DB
}

func NewHandler(db *gorm.DB) Handler {
	return Handler{db: db}
}

func (h Handler) List(ctx *gin.Context) {
	query := h.db.Where("status = ?", model.StatusPublished)
	if schoolID := ctx.Query("schoolId"); schoolID != "" {
		query = query.Where("school_id = ?", schoolID)
	}
	if majorID := ctx.Query("majorId"); majorID != "" {
		query = query.Where("major_id = ?", majorID)
	}
	if courseID := ctx.Query("courseId"); courseID != "" {
		query = query.Where("course_id = ?", courseID)
	}
	if grade := ctx.Query("grade"); grade != "" {
		query = query.Where("grade = ?", grade)
	}

	var packages []model.CoursePackage
	if err := query.Order("created_at asc").Find(&packages).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"packages": packages})
}

func (h Handler) CoursePackages(ctx *gin.Context) {
	var packages []model.CoursePackage
	if err := h.db.Where("course_id = ? AND status = ?", ctx.Param("id"), model.StatusPublished).
		Order("created_at asc").
		Find(&packages).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"packages": packages})
}

func (h Handler) Detail(ctx *gin.Context) {
	var coursePackage model.CoursePackage
	if err := h.db.First(&coursePackage, "id = ? AND status = ?", ctx.Param("id"), model.StatusPublished).Error; err != nil {
		response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "package_not_found", nil)
		return
	}

	var items []model.CoursePackageItem
	if err := h.db.Where("package_id = ?", coursePackage.ID).
		Order("sort_order asc, created_at asc").
		Find(&items).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}

	materialIDs := make([]string, 0, len(items))
	for _, item := range items {
		if item.ResourceType == resourceTypeMaterial {
			materialIDs = append(materialIDs, item.ResourceID)
		}
	}
	materials := []model.Material{}
	publishedItems := []model.CoursePackageItem{}
	if len(materialIDs) > 0 {
		if err := h.db.Where("id IN ? AND status = ?", materialIDs, model.StatusPublished).
			Order("created_at asc").
			Find(&materials).Error; err != nil {
			response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
			return
		}
		publishedMaterialIDs := map[string]bool{}
		for _, material := range materials {
			publishedMaterialIDs[material.ID] = true
		}
		for _, item := range items {
			if item.ResourceType == resourceTypeMaterial && publishedMaterialIDs[item.ResourceID] {
				publishedItems = append(publishedItems, item)
			}
		}
	}

	response.OK(ctx, gin.H{
		"package":   coursePackage,
		"items":     publishedItems,
		"materials": materials,
	})
}
