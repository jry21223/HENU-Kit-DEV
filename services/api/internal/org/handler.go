package org

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

func (h Handler) Schools(ctx *gin.Context) {
	var schools []model.School
	if err := h.db.Where("status <> ?", model.StatusArchived).Order("created_at asc").Find(&schools).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"schools": schools})
}

func (h Handler) Colleges(ctx *gin.Context) {
	query := h.db.Where("status <> ?", model.StatusArchived)
	if schoolID := ctx.Query("schoolId"); schoolID != "" {
		query = query.Where("school_id = ?", schoolID)
	}
	var colleges []model.College
	if err := query.Order("created_at asc").Find(&colleges).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"colleges": colleges})
}

func (h Handler) Majors(ctx *gin.Context) {
	query := h.db.Where("status <> ?", model.StatusArchived)
	if schoolID := ctx.Query("schoolId"); schoolID != "" {
		query = query.Where("school_id = ?", schoolID)
	}
	if collegeID := ctx.Query("collegeId"); collegeID != "" {
		query = query.Where("college_id = ?", collegeID)
	}
	var majors []model.Major
	if err := query.Order("created_at asc").Find(&majors).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"majors": majors})
}

func (h Handler) Courses(ctx *gin.Context) {
	query := h.db.Where("status = ?", model.StatusPublished)
	if schoolID := ctx.Query("schoolId"); schoolID != "" {
		query = query.Where("school_id = ?", schoolID)
	}
	if majorID := ctx.Query("majorId"); majorID != "" {
		query = query.Where("major_id = ?", majorID)
	}
	if grade := ctx.Query("grade"); grade != "" {
		query = query.Where("grade = ?", grade)
	}
	var courses []model.Course
	if err := query.Order("created_at asc").Find(&courses).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"courses": courses})
}

func (h Handler) Course(ctx *gin.Context) {
	var course model.Course
	if err := h.db.First(&course, "id = ? AND status = ?", ctx.Param("id"), model.StatusPublished).Error; err != nil {
		response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "course_not_found", nil)
		return
	}
	response.OK(ctx, gin.H{"course": course})
}
