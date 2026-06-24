package search

import (
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

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

type Result struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	URL         string `json:"url"`
	Meta        string `json:"meta,omitempty"`
	UpdatedAt   string `json:"updatedAt,omitempty"`
}

type groupedResults struct {
	Courses   []Result `json:"courses"`
	Materials []Result `json:"materials"`
	Packages  []Result `json:"packages"`
	Wiki      []Result `json:"wiki"`
	Blog      []Result `json:"blog"`
	Forum     []Result `json:"forum"`
}

func (h Handler) Query(ctx *gin.Context) {
	query := strings.TrimSpace(ctx.Query("q"))
	limit, ok := parseLimit(ctx.Query("limit"), 8, 30)
	if !ok {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_limit", nil)
		return
	}
	if query == "" {
		response.OK(ctx, gin.H{"query": query, "results": groupedResults{}, "total": 0})
		return
	}
	if utf8.RuneCountInString(query) > 80 {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "query_too_long", nil)
		return
	}

	pattern := "%" + escapeLike(strings.ToLower(query)) + "%"
	results := groupedResults{}

	var err error
	if results.Courses, err = h.searchCourses(pattern, limit); err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	if results.Materials, err = h.searchMaterials(pattern, limit); err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	if results.Packages, err = h.searchPackages(pattern, limit); err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	if results.Wiki, err = h.searchWiki(pattern, limit); err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	if results.Blog, err = h.searchBlog(pattern, limit); err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	if results.Forum, err = h.searchForum(pattern, limit); err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}

	total := len(results.Courses) + len(results.Materials) + len(results.Packages) + len(results.Wiki) + len(results.Blog) + len(results.Forum)
	response.OK(ctx, gin.H{"query": query, "results": results, "total": total})
}

func (h Handler) searchCourses(pattern string, limit int) ([]Result, error) {
	var courses []model.Course
	err := h.db.Where("status = ? AND (LOWER(name) LIKE ? ESCAPE '\\' OR LOWER(description) LIKE ? ESCAPE '\\' OR LOWER(exam_scope) LIKE ? ESCAPE '\\')", model.StatusPublished, pattern, pattern, pattern).
		Order("updated_at desc").
		Limit(limit).
		Find(&courses).Error
	if err != nil {
		return nil, err
	}
	results := make([]Result, 0, len(courses))
	for _, course := range courses {
		results = append(results, Result{
			ID:          course.ID,
			Type:        "course",
			Title:       course.Name,
			Description: firstNonEmpty(course.Description, course.ExamScope),
			URL:         "/courses/" + course.ID,
			Meta:        course.Grade,
			UpdatedAt:   course.UpdatedAt.Format(timeFormat),
		})
	}
	return results, nil
}

func (h Handler) searchMaterials(pattern string, limit int) ([]Result, error) {
	var materials []model.Material
	err := h.db.Where("status = ? AND (LOWER(title) LIKE ? ESCAPE '\\' OR LOWER(description) LIKE ? ESCAPE '\\' OR LOWER(preview_content) LIKE ? ESCAPE '\\')", model.StatusPublished, pattern, pattern, pattern).
		Order("updated_at desc").
		Limit(limit).
		Find(&materials).Error
	if err != nil {
		return nil, err
	}
	results := make([]Result, 0, len(materials))
	for _, material := range materials {
		results = append(results, Result{
			ID:          material.ID,
			Type:        "material",
			Title:       material.Title,
			Description: firstNonEmpty(material.Description, material.PreviewContent),
			URL:         "/materials/" + material.ID,
			Meta:        material.Type + " / " + material.AccessLevel,
			UpdatedAt:   material.UpdatedAt.Format(timeFormat),
		})
	}
	return results, nil
}

func (h Handler) searchPackages(pattern string, limit int) ([]Result, error) {
	var packages []model.CoursePackage
	err := h.db.Where("status = ? AND (LOWER(title) LIKE ? ESCAPE '\\' OR LOWER(description) LIKE ? ESCAPE '\\' OR LOWER(slug) LIKE ? ESCAPE '\\')", model.StatusPublished, pattern, pattern, pattern).
		Order("updated_at desc").
		Limit(limit).
		Find(&packages).Error
	if err != nil {
		return nil, err
	}
	results := make([]Result, 0, len(packages))
	for _, coursePackage := range packages {
		results = append(results, Result{
			ID:          coursePackage.ID,
			Type:        "package",
			Title:       coursePackage.Title,
			Description: coursePackage.Description,
			URL:         "/packages/" + coursePackage.ID,
			Meta:        coursePackage.Grade,
			UpdatedAt:   coursePackage.UpdatedAt.Format(timeFormat),
		})
	}
	return results, nil
}

func (h Handler) searchWiki(pattern string, limit int) ([]Result, error) {
	var entries []model.WikiEntry
	err := h.db.Where("status = ? AND visibility = ? AND (LOWER(title) LIKE ? ESCAPE '\\' OR LOWER(content) LIKE ? ESCAPE '\\')", model.StatusPublished, "public", pattern, pattern).
		Order("updated_at desc").
		Limit(limit).
		Find(&entries).Error
	if err != nil {
		return nil, err
	}
	results := make([]Result, 0, len(entries))
	for _, entry := range entries {
		results = append(results, Result{
			ID:          entry.ID,
			Type:        "wiki",
			Title:       entry.Title,
			Description: truncate(entry.Content, 180),
			URL:         "/wiki/" + entry.ID,
			Meta:        "v" + strconv.Itoa(entry.Version),
			UpdatedAt:   entry.UpdatedAt.Format(timeFormat),
		})
	}
	return results, nil
}

func (h Handler) searchBlog(pattern string, limit int) ([]Result, error) {
	var posts []model.BlogPost
	err := h.db.Where("status = ? AND visibility = ? AND (LOWER(title) LIKE ? ESCAPE '\\' OR LOWER(content) LIKE ? ESCAPE '\\')", model.StatusPublished, "public", pattern, pattern).
		Order("updated_at desc").
		Limit(limit).
		Find(&posts).Error
	if err != nil {
		return nil, err
	}
	results := make([]Result, 0, len(posts))
	for _, post := range posts {
		results = append(results, Result{
			ID:          post.ID,
			Type:        "blog",
			Title:       post.Title,
			Description: truncate(post.Content, 180),
			URL:         "/blog/" + post.ID,
			Meta:        "Blog",
			UpdatedAt:   post.UpdatedAt.Format(timeFormat),
		})
	}
	return results, nil
}

func (h Handler) searchForum(pattern string, limit int) ([]Result, error) {
	var posts []model.ForumPost
	err := h.db.Model(&model.ForumPost{}).
		Joins("JOIN forum_boards ON forum_boards.id = forum_posts.board_id").
		Where("forum_posts.status = ? AND forum_posts.visibility = ? AND forum_boards.status = ? AND (LOWER(forum_posts.title) LIKE ? ESCAPE '\\' OR LOWER(forum_posts.content) LIKE ? ESCAPE '\\')", model.StatusPublished, "public", model.StatusPublished, pattern, pattern).
		Order("forum_posts.updated_at desc").
		Limit(limit).
		Find(&posts).Error
	if err != nil {
		return nil, err
	}
	results := make([]Result, 0, len(posts))
	for _, post := range posts {
		results = append(results, Result{
			ID:          post.ID,
			Type:        "forum",
			Title:       post.Title,
			Description: truncate(post.Content, 180),
			URL:         "/forum/" + post.ID,
			Meta:        post.Type,
			UpdatedAt:   post.UpdatedAt.Format(timeFormat),
		})
	}
	return results, nil
}

const timeFormat = "2006-01-02T15:04:05Z07:00"

func parseLimit(value string, fallback int, max int) (int, bool) {
	if strings.TrimSpace(value) == "" {
		return fallback, true
	}
	limit, err := strconv.Atoi(value)
	if err != nil || limit <= 0 || limit > max {
		return 0, false
	}
	return limit, true
}

func escapeLike(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return replacer.Replace(value)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return truncate(value, 180)
		}
	}
	return ""
}

func truncate(value string, maxRunes int) string {
	value = strings.TrimSpace(value)
	if utf8.RuneCountInString(value) <= maxRunes {
		return value
	}
	runes := []rune(value)
	return strings.TrimSpace(string(runes[:maxRunes])) + "..."
}
