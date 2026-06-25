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

type publicSchoolDTO struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type publicCollegeDTO struct {
	ID       string `json:"id"`
	SchoolID string `json:"schoolId"`
	Name     string `json:"name"`
}

type publicMajorDTO struct {
	ID        string `json:"id"`
	SchoolID  string `json:"schoolId"`
	CollegeID string `json:"collegeId"`
	Name      string `json:"name"`
	Slug      string `json:"slug"`
}

type publicCourseDTO struct {
	ID          string `json:"id"`
	SchoolID    string `json:"schoolId"`
	CollegeID   string `json:"collegeId"`
	MajorID     string `json:"majorId"`
	Grade       string `json:"grade"`
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description"`
	ExamScope   string `json:"examScope"`
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
	response.OK(ctx, gin.H{"schools": publicSchools(schools)})
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
	response.OK(ctx, gin.H{"colleges": publicColleges(colleges)})
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
	response.OK(ctx, gin.H{"majors": publicMajors(majors)})
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
	response.OK(ctx, gin.H{"courses": publicCourses(courses)})
}

func (h Handler) Course(ctx *gin.Context) {
	var course model.Course
	if err := h.db.First(&course, "id = ? AND status = ?", ctx.Param("id"), model.StatusPublished).Error; err != nil {
		response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "course_not_found", nil)
		return
	}
	response.OK(ctx, gin.H{"course": publicCourse(course)})
}

func publicSchools(schools []model.School) []publicSchoolDTO {
	result := make([]publicSchoolDTO, 0, len(schools))
	for _, school := range schools {
		result = append(result, publicSchoolDTO{ID: school.ID, Name: school.Name, Slug: school.Slug})
	}
	return result
}

func publicColleges(colleges []model.College) []publicCollegeDTO {
	result := make([]publicCollegeDTO, 0, len(colleges))
	for _, college := range colleges {
		result = append(result, publicCollegeDTO{
			ID:       college.ID,
			SchoolID: college.SchoolID,
			Name:     college.Name,
		})
	}
	return result
}

func publicMajors(majors []model.Major) []publicMajorDTO {
	result := make([]publicMajorDTO, 0, len(majors))
	for _, major := range majors {
		result = append(result, publicMajorDTO{
			ID:        major.ID,
			SchoolID:  major.SchoolID,
			CollegeID: major.CollegeID,
			Name:      major.Name,
			Slug:      major.Slug,
		})
	}
	return result
}

func publicCourses(courses []model.Course) []publicCourseDTO {
	result := make([]publicCourseDTO, 0, len(courses))
	for _, course := range courses {
		result = append(result, publicCourse(course))
	}
	return result
}

func publicCourse(course model.Course) publicCourseDTO {
	return publicCourseDTO{
		ID:          course.ID,
		SchoolID:    course.SchoolID,
		CollegeID:   course.CollegeID,
		MajorID:     course.MajorID,
		Grade:       course.Grade,
		Name:        course.Name,
		Slug:        course.Slug,
		Description: course.Description,
		ExamScope:   course.ExamScope,
	}
}
