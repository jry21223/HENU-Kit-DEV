package leaderboard

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"final-review-platform/services/api/pkg/response"
)

type Handler struct {
	db *gorm.DB
}

func NewHandler(db *gorm.DB) Handler {
	return Handler{db: db}
}

type Entry struct {
	Rank    int            `json:"rank"`
	UserID  string         `json:"userId"`
	Name    string         `json:"name"`
	Role    string         `json:"role"`
	Score   int64          `json:"score"`
	Metrics map[string]any `json:"metrics"`
}

type row struct {
	UserID       string
	Name         string
	Role         string
	Score        int64
	WikiCount    int64
	Engagement   int64
	AnswerCount  int64
	CorrectCount int64
	Points       int64
}

func (h Handler) Wiki(ctx *gin.Context) {
	limit, ok := parseLimit(ctx.Query("limit"), 10, 50)
	if !ok {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_limit", nil)
		return
	}

	var rows []row
	err := h.db.Raw(`
		SELECT
			users.id AS user_id,
			users.name AS name,
			users.role AS role,
			(COUNT(wiki_entries.id) * 10 + COALESCE(SUM(wiki_entries.like_count + wiki_entries.comment_count + wiki_entries.collect_count), 0)) AS score,
			COUNT(wiki_entries.id) AS wiki_count,
			COALESCE(SUM(wiki_entries.like_count + wiki_entries.comment_count + wiki_entries.collect_count), 0) AS engagement
		FROM users
		JOIN wiki_entries ON wiki_entries.author_id = users.id
		WHERE users.status = ?
			AND wiki_entries.status = ?
			AND wiki_entries.visibility = ?
			AND wiki_entries.deleted_at IS NULL
			AND users.deleted_at IS NULL
		GROUP BY users.id, users.name, users.role
		HAVING COUNT(wiki_entries.id) > 0
		ORDER BY score DESC, wiki_count DESC, users.name ASC
		LIMIT ?
	`, statusActive, statusPublished, visibilityPublic, limit).Scan(&rows).Error
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"type": "wiki", "period": "all_time", "generatedAt": time.Now().UTC(), "entries": wikiEntries(rows)})
}

func (h Handler) Quiz(ctx *gin.Context) {
	limit, ok := parseLimit(ctx.Query("limit"), 10, 50)
	if !ok {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_limit", nil)
		return
	}

	var rows []row
	err := h.db.Raw(`
		SELECT
			users.id AS user_id,
			users.name AS name,
			users.role AS role,
			CAST(COALESCE(SUM(quiz_answers.score), 0) * 100 AS INTEGER) AS score,
			COUNT(quiz_answers.id) AS answer_count,
			COALESCE(SUM(CASE WHEN quiz_answers.is_correct THEN 1 ELSE 0 END), 0) AS correct_count
		FROM users
		JOIN quiz_answers ON quiz_answers.user_id = users.id
		WHERE users.status = ?
			AND users.deleted_at IS NULL
			AND quiz_answers.deleted_at IS NULL
		GROUP BY users.id, users.name, users.role
		HAVING COUNT(quiz_answers.id) > 0
		ORDER BY score DESC, correct_count DESC, answer_count DESC, users.name ASC
		LIMIT ?
	`, statusActive, limit).Scan(&rows).Error
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"type": "quiz", "period": "all_time", "generatedAt": time.Now().UTC(), "entries": quizEntries(rows)})
}

func (h Handler) Overall(ctx *gin.Context) {
	limit, ok := parseLimit(ctx.Query("limit"), 10, 50)
	if !ok {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_limit", nil)
		return
	}

	var rows []row
	err := h.db.Raw(`
		SELECT
			users.id AS user_id,
			users.name AS name,
			users.role AS role,
			(COALESCE(users.points_balance, 0) + COALESCE(wiki_stats.wiki_count, 0) * 10 + COALESCE(quiz_stats.correct_count, 0) * 5) AS score,
			COALESCE(users.points_balance, 0) AS points,
			COALESCE(wiki_stats.wiki_count, 0) AS wiki_count,
			COALESCE(quiz_stats.correct_count, 0) AS correct_count
		FROM users
		LEFT JOIN (
			SELECT author_id, COUNT(id) AS wiki_count
			FROM wiki_entries
			WHERE status = ? AND visibility = ? AND deleted_at IS NULL
			GROUP BY author_id
		) wiki_stats ON wiki_stats.author_id = users.id
		LEFT JOIN (
			SELECT user_id, COALESCE(SUM(CASE WHEN is_correct THEN 1 ELSE 0 END), 0) AS correct_count
			FROM quiz_answers
			WHERE deleted_at IS NULL
			GROUP BY user_id
		) quiz_stats ON quiz_stats.user_id = users.id
		WHERE users.status = ?
			AND users.deleted_at IS NULL
			AND (COALESCE(users.points_balance, 0) + COALESCE(wiki_stats.wiki_count, 0) * 10 + COALESCE(quiz_stats.correct_count, 0) * 5) > 0
		ORDER BY score DESC, points DESC, wiki_count DESC, correct_count DESC, users.name ASC
		LIMIT ?
	`, statusPublished, visibilityPublic, statusActive, limit).Scan(&rows).Error
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"type": "overall", "period": "all_time", "generatedAt": time.Now().UTC(), "entries": overallEntries(rows)})
}

func wikiEntries(rows []row) []Entry {
	entries := make([]Entry, 0, len(rows))
	for index, item := range rows {
		entries = append(entries, Entry{
			Rank:   index + 1,
			UserID: item.UserID,
			Name:   item.Name,
			Role:   item.Role,
			Score:  item.Score,
			Metrics: map[string]any{
				"wikiCount":  item.WikiCount,
				"engagement": item.Engagement,
			},
		})
	}
	return entries
}

func quizEntries(rows []row) []Entry {
	entries := make([]Entry, 0, len(rows))
	for index, item := range rows {
		entries = append(entries, Entry{
			Rank:   index + 1,
			UserID: item.UserID,
			Name:   item.Name,
			Role:   item.Role,
			Score:  item.Score,
			Metrics: map[string]any{
				"answerCount":  item.AnswerCount,
				"correctCount": item.CorrectCount,
			},
		})
	}
	return entries
}

func overallEntries(rows []row) []Entry {
	entries := make([]Entry, 0, len(rows))
	for index, item := range rows {
		entries = append(entries, Entry{
			Rank:   index + 1,
			UserID: item.UserID,
			Name:   item.Name,
			Role:   item.Role,
			Score:  item.Score,
			Metrics: map[string]any{
				"points":       item.Points,
				"wikiCount":    item.WikiCount,
				"correctCount": item.CorrectCount,
			},
		})
	}
	return entries
}

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

const (
	statusActive     = "active"
	statusPublished  = "published"
	visibilityPublic = "public"
)
