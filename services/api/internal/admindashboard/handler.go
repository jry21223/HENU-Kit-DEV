package admindashboard

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	redislib "github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"final-review-platform/services/api/internal/platform/model"
)

const (
	metricDefinitionVersion = "admin_dashboard_v1"
	foodPolicyVersion       = "food_calibration_v1"
)

type Handler struct {
	db          *gorm.DB
	cache       *redislib.Client
	enabled     bool
	environment string
	version     string
}

func NewHandler(db *gorm.DB, cache *redislib.Client, enabled bool, environment string, version string) Handler {
	return Handler{db: db, cache: cache, enabled: enabled, environment: environment, version: version}
}

type successEnvelope struct {
	Data      any    `json:"data"`
	RequestID string `json:"request_id"`
}

type metric struct {
	Code              string   `json:"code"`
	Label             string   `json:"label"`
	Value             *float64 `json:"value"`
	PreviousValue     *float64 `json:"previous_value"`
	ChangeRate        *float64 `json:"change_rate"`
	DefinitionVersion string   `json:"definition_version"`
	AsOf              string   `json:"as_of"`
}

type domainCard struct {
	Domain        string   `json:"domain"`
	Title         string   `json:"title"`
	Status        string   `json:"status"`
	PrimaryMetric metric   `json:"primary_metric"`
	Metrics       []metric `json:"metrics"`
	AsOf          string   `json:"as_of"`
	LastSuccessAt *string  `json:"last_success_at"`
	ActionPath    *string  `json:"action_path"`
	Message       string   `json:"message"`
}

type partialFailure struct {
	ServiceID     string  `json:"service_id"`
	Code          string  `json:"code"`
	LastSuccessAt *string `json:"last_success_at"`
}

type dashboardSnapshot struct {
	Status          string           `json:"status"`
	AsOf            string           `json:"as_of"`
	Cards           []domainCard     `json:"cards"`
	PartialFailures []partialFailure `json:"partial_failures"`
}

type actionItem struct {
	ID            string `json:"id"`
	Domain        string `json:"domain"`
	Urgency       string `json:"urgency"`
	Summary       string `json:"summary"`
	SourceService string `json:"source_service"`
	SourceType    string `json:"source_type"`
	SourceID      string `json:"source_id"`
	CreatedAt     string `json:"created_at"`
	DueAt         string `json:"due_at"`
	ActionPath    string `json:"action_path"`
}

func (h Handler) UIConfig(ctx *gin.Context) {
	respond(ctx, gin.H{
		"shell_version":        map[bool]string{true: "v2", false: "legacy"}[h.enabled],
		"dashboard_v2_enabled": h.enabled,
		"environment":          h.environment,
		"capabilities": gin.H{
			"global_search":       false,
			"notification_center": false,
			"legacy_shell":        true,
		},
	})
}

func (h Handler) LatestSnapshot(ctx *gin.Context) {
	now := time.Now().UTC()
	cards := []domainCard{
		h.userCard(now),
		h.noticeCard(now),
		h.mailCard(now),
		h.feedbackCard(now),
		h.foodCard(now),
		h.systemCard(ctx.Request.Context(), now),
	}
	status := model.IntegrationOK
	failures := make([]partialFailure, 0)
	for _, card := range cards {
		if card.Status != model.IntegrationOK {
			status = model.IntegrationPartial
			failures = append(failures, partialFailure{
				ServiceID:     card.Domain,
				Code:          integrationFailureCode(card.Status),
				LastSuccessAt: card.LastSuccessAt,
			})
		}
	}
	respond(ctx, dashboardSnapshot{Status: status, AsOf: now.Format(time.RFC3339), Cards: cards, PartialFailures: failures})
}

func (h Handler) ActionItems(ctx *gin.Context) {
	now := time.Now().UTC()
	items := make([]actionItem, 0)

	var cases []model.OperationCase
	h.db.Where("status <> ?", "resolved").Order("due_at asc").Limit(100).Find(&cases)
	for _, row := range cases {
		items = append(items, actionItem{
			ID: row.ID, Domain: domainForSource(row.SourceService), Urgency: row.Urgency,
			Summary: row.Summary, SourceService: row.SourceService, SourceType: row.SourceType,
			SourceID: row.SourceID, CreatedAt: row.CreatedAt.UTC().Format(time.RFC3339),
			DueAt: row.DueAt.UTC().Format(time.RFC3339), ActionPath: row.ActionPath,
		})
	}

	var pendingNotices []model.CampusNotice
	h.db.Where("status = ?", "review_pending").Order("created_at asc").Limit(20).Find(&pendingNotices)
	for _, row := range pendingNotices {
		items = append(items, derivedAction("notice", row.ID, "school_notice", "校园通知待审核："+row.Title, row.CreatedAt, row.CreatedAt.Add(72*time.Hour), "/notices?status=review_pending"))
	}

	var deadLetters []model.MailDeadLetter
	h.db.Where("status = ?", "open").Order("created_at asc").Limit(20).Find(&deadLetters)
	for _, row := range deadLetters {
		items = append(items, derivedUrgentAction("mail", row.ID, "mail_dead_letter", "邮件死信需要处理", row.CreatedAt, "/mail/dead-letters?status=open"))
	}

	var foodSubmissions []model.FoodSubmission
	h.db.Where("status = ?", model.StatusPending).Order("created_at asc").Limit(20).Find(&foodSubmissions)
	for _, row := range foodSubmissions {
		items = append(items, derivedAction("food", row.ID, "food_submission", "美食投稿待审核："+row.Name, row.CreatedAt, row.CreatedAt.Add(72*time.Hour), "/food/submissions?status=pending"))
	}

	var anomalies []model.FoodVoteAnomaly
	h.db.Where("status = ? AND blocking = ?", "open", true).Order("created_at asc").Limit(20).Find(&anomalies)
	for _, row := range anomalies {
		items = append(items, derivedUrgentAction("food", row.ID, "food_vote_anomaly", "阻断性校准异常待处理", row.CreatedAt, "/food/anomalies?status=open&blocking=true"))
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].Urgency != items[j].Urgency {
			return items[i].Urgency == model.UrgencyUrgent
		}
		return items[i].DueAt < items[j].DueAt
	})
	respond(ctx, gin.H{"items": items, "as_of": now.Format(time.RFC3339)})
}

func (h Handler) MetricSeries(ctx *gin.Context) {
	metricCode := ctx.DefaultQuery("metric", "registered_users")
	days := 14
	start := time.Now().UTC().AddDate(0, 0, -days+1).Truncate(24 * time.Hour)
	type point struct {
		Time  string  `json:"time"`
		Value float64 `json:"value"`
	}
	points := make([]point, 0, days)
	for offset := 0; offset < days; offset++ {
		dayStart := start.AddDate(0, 0, offset)
		dayEnd := dayStart.AddDate(0, 0, 1)
		var value int64
		switch metricCode {
		case "new_users":
			h.db.Model(&model.User{}).Where("created_at >= ? AND created_at < ?", dayStart, dayEnd).Count(&value)
		case "notice_imports":
			h.db.Model(&model.CampusNotice{}).Where("created_at >= ? AND created_at < ?", dayStart, dayEnd).Count(&value)
		default:
			h.db.Model(&model.User{}).Where("created_at < ?", dayEnd).Count(&value)
		}
		points = append(points, point{Time: dayStart.Format("2006-01-02"), Value: float64(value)})
	}
	respond(ctx, gin.H{"metric": metricCode, "definition_version": metricDefinitionVersion, "points": points})
}

func (h Handler) userCard(now time.Time) domainCard {
	var total, verified, newToday, profileComplete int64
	h.db.Model(&model.User{}).Count(&total)
	h.db.Model(&model.User{}).Where("email_verified = ?", true).Count(&verified)
	h.db.Model(&model.User{}).Where("created_at >= ?", beginningOfDay(now)).Count(&newToday)
	h.db.Model(&model.User{}).Where("email_verified = ? AND school_id IS NOT NULL AND major_id IS NOT NULL AND grade <> ''", true).Count(&profileComplete)
	completion := ratio(profileComplete, verified)
	return card("users", "用户", model.IntegrationOK,
		m("verified_users", "已验证用户", number(verified), now),
		[]metric{
			m("new_users_today", "今日新增", number(newToday), now),
			m("dau", "DAU", nil, now),
			m("academic_profile_completion_rate", "学院资料完成率", completion, now),
		}, "/users", "用户域已接入；尚未冻结口径的 DAU 诚实显示为 —。", now)
}

func (h Handler) noticeCard(now time.Time) domainCard {
	var imported, pending, failedImports, pendingDistribution int64
	h.db.Model(&model.CampusNotice{}).Where("created_at >= ?", beginningOfDay(now)).Count(&imported)
	h.db.Model(&model.CampusNotice{}).Where("status = ?", "review_pending").Count(&pending)
	h.db.Model(&model.NoticeImportJob{}).Where("status = ?", "failed").Count(&failedImports)
	h.db.Model(&model.CampusNotice{}).Where("status = ? AND distribution_status IN ?", "approved", []string{"not_scheduled", "pending"}).Count(&pendingDistribution)
	return card("notice", "校园通知", model.IntegrationOK,
		m("notice_imported_today", "今日导入", number(imported), now),
		[]metric{
			m("notice_pending_review", "待审核", number(pending), now),
			m("notice_failed_imports", "失败导入", number(failedImports), now),
			m("notice_pending_distribution", "待分发", number(pendingDistribution), now),
		}, "/notices", "人工表单与 JSONL 导入已纳入统一口径。", now)
}

func (h Handler) mailCard(now time.Time) domainCard {
	var ended, accepted, queued, deadLetters int64
	h.db.Model(&model.MailDelivery{}).Where("category = ? AND status IN ?", "critical", []string{"accepted", "delivered", "bounced", "failed"}).Count(&ended)
	h.db.Model(&model.MailDelivery{}).Where("category = ? AND status IN ?", "critical", []string{"accepted", "delivered"}).Count(&accepted)
	h.db.Model(&model.MailDelivery{}).Where("status IN ?", []string{"queued", "sending", "retry_due"}).Count(&queued)
	h.db.Model(&model.MailDeadLetter{}).Where("status = ?", "open").Count(&deadLetters)
	var oldest model.MailDelivery
	oldestMinutes := (*float64)(nil)
	if err := h.db.Where("status IN ?", []string{"queued", "sending", "retry_due"}).Order("queued_at asc").First(&oldest).Error; err == nil {
		v := time.Since(oldest.QueuedAt).Minutes()
		oldestMinutes = &v
	}
	return card("mail", "邮件", model.IntegrationOK,
		m("critical_accepted_rate", "Critical 接受率", ratio(accepted, ended), now),
		[]metric{
			m("mail_queued", "排队数", number(queued), now),
			m("mail_oldest_age_minutes", "最老任务（分钟）", oldestMinutes, now),
			m("mail_dead_letters", "死信", number(deadLetters), now),
		}, "/mail", "accepted 与 delivered 分开统计。", now)
}

func (h Handler) feedbackCard(now time.Time) domainCard {
	var platformOpen, questionOpen, overdue, resolvedToday int64
	h.db.Model(&model.PlatformFeedback{}).Where("status NOT IN ?", []string{"resolved", "closed", "rejected"}).Count(&platformOpen)
	h.db.Model(&model.Report{}).Where("status = ?", model.StatusPending).Count(&questionOpen)
	h.db.Model(&model.PlatformFeedback{}).Where("status NOT IN ? AND due_at < ?", []string{"resolved", "closed", "rejected"}, now).Count(&overdue)
	h.db.Model(&model.PlatformFeedback{}).Where("resolved_at >= ?", beginningOfDay(now)).Count(&resolvedToday)
	return card("feedback", "反馈", model.IntegrationOK,
		m("platform_feedback_open", "平台反馈", number(platformOpen), now),
		[]metric{
			m("question_feedback_open", "题目反馈", number(questionOpen), now),
			m("feedback_overdue", "超时", number(overdue), now),
			m("feedback_resolved_today", "今日解决", number(resolvedToday), now),
		}, "/feedback", "平台反馈与 QuizCraft Report Adapter 均已接入。", now)
}

func (h Handler) foodCard(now time.Time) domainCard {
	var pending, anomalies int64
	h.db.Model(&model.FoodSubmission{}).Where("status = ?", model.StatusPending).Count(&pending)
	h.db.Model(&model.FoodVoteAnomaly{}).Where("status = ?", "open").Count(&anomalies)
	promote, demote := h.foodCandidates(now)
	return card("food", "美食", model.IntegrationOK,
		m("food_pending_submissions", "待审核投稿", number(pending), now),
		[]metric{
			m("food_promote_candidates", "升档候选", number(promote), now),
			m("food_demote_candidates", "降档候选", number(demote), now),
			m("food_vote_anomalies", "异常票", number(anomalies), now),
		}, "/food", "Policy v1：10 人、70%、7 天冷却。", now)
}

func (h Handler) systemCard(requestContext context.Context, now time.Time) domainCard {
	status := model.IntegrationOK
	message := "API、PostgreSQL 与 Redis 正常。"
	serviceCount := int64(1)
	degraded := int64(0)
	workerAnomalies := int64(0)
	outboxPending := int64(0)
	if sqlDB, err := h.db.DB(); err != nil || sqlDB.PingContext(requestContext) != nil {
		status = model.IntegrationUnavailable
		degraded++
		message = "PostgreSQL 探针失败。"
	}
	if h.cache != nil {
		if err := h.cache.Ping(requestContext).Err(); err != nil {
			status = model.IntegrationPartial
			degraded++
			message = "Redis 探针失败，数据库与 API 仍可用。"
		}
	}
	var heartbeats []model.ServiceHeartbeat
	if err := h.db.Find(&heartbeats).Error; err == nil {
		serviceCount += int64(len(heartbeats))
		for _, heartbeat := range heartbeats {
			outboxPending += heartbeat.OutboxPending
			workerAnomalies += heartbeat.WorkerAnomalies
			if (heartbeat.Status != "ready" && heartbeat.Status != "ok") || now.Sub(heartbeat.LastReadyAt) > 5*time.Minute {
				degraded++
				status = model.IntegrationPartial
			}
		}
	}
	return card("system", "系统", status,
		m("services_total", "服务总数", number(serviceCount), now),
		[]metric{
			m("services_degraded", "降级服务", number(degraded), now),
			m("outbox_pending", "Outbox 积压", number(outboxPending), now),
			m("worker_anomalies", "Worker 异常", number(workerAnomalies), now),
		}, "/system", message, now)
}

func (h Handler) foodCandidates(now time.Time) (int64, int64) {
	var rounds []model.FoodCalibrationRound
	h.db.Where("status = ?", "open").Find(&rounds)
	var promote, demote int64
	for _, round := range rounds {
		var entry model.FoodEntry
		if err := h.db.First(&entry, "id = ?", round.EntryID).Error; err != nil {
			continue
		}
		if entry.LastAdjustedAt != nil && now.Sub(*entry.LastAdjustedAt) < 7*24*time.Hour {
			continue
		}
		var blocking int64
		h.db.Model(&model.FoodVoteAnomaly{}).Where("round_id = ? AND status = ? AND blocking = ?", round.ID, "open", true).Count(&blocking)
		if blocking > 0 {
			continue
		}
		type voteCount struct {
			Position string
			Count    int64
		}
		var counts []voteCount
		h.db.Model(&model.FoodCalibrationVote{}).Select("position, count(*) as count").Where("round_id = ? AND status = ?", round.ID, "valid").Group("position").Scan(&counts)
		var total, underrated, overrated int64
		for _, count := range counts {
			total += count.Count
			if count.Position == "underrated" {
				underrated = count.Count
			}
			if count.Position == "overrated" {
				overrated = count.Count
			}
		}
		if total < 10 {
			continue
		}
		if float64(underrated)/float64(total) >= .70 {
			promote++
		}
		if float64(overrated)/float64(total) >= .70 {
			demote++
		}
	}
	return promote, demote
}

func respond(ctx *gin.Context, data any) {
	requestID := ctx.GetHeader("X-Request-Id")
	if requestID == "" {
		requestID = "req_" + uuid.NewString()
	}
	ctx.Header("X-Request-Id", requestID)
	ctx.JSON(http.StatusOK, successEnvelope{Data: data, RequestID: requestID})
}

func card(domain, title, status string, primary metric, metrics []metric, actionPath, message string, now time.Time) domainCard {
	asOf := now.Format(time.RFC3339)
	return domainCard{Domain: domain, Title: title, Status: status, PrimaryMetric: primary, Metrics: metrics, AsOf: asOf, LastSuccessAt: &asOf, ActionPath: &actionPath, Message: message}
}

func m(code, label string, value *float64, now time.Time) metric {
	return metric{Code: code, Label: label, Value: value, PreviousValue: nil, ChangeRate: nil, DefinitionVersion: metricDefinitionVersion, AsOf: now.Format(time.RFC3339)}
}

func number(value int64) *float64 { v := float64(value); return &v }
func ratio(numerator, denominator int64) *float64 {
	if denominator == 0 {
		return nil
	}
	v := float64(numerator) / float64(denominator)
	return &v
}
func beginningOfDay(now time.Time) time.Time {
	y, m, d := now.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}
func integrationFailureCode(status string) string { return fmt.Sprintf("SUMMARY_%s", status) }
func domainForSource(source string) string {
	if source == "quiz" {
		return "feedback"
	}
	return source
}

func derivedAction(domain, sourceID, sourceType, summary string, createdAt, dueAt time.Time, path string) actionItem {
	urgency := model.UrgencyNormal
	if dueAt.Sub(createdAt) <= 24*time.Hour {
		urgency = model.UrgencyUrgent
	}
	return actionItem{ID: "derived:" + sourceType + ":" + sourceID, Domain: domain, Urgency: urgency, Summary: summary, SourceService: domain, SourceType: sourceType, SourceID: sourceID, CreatedAt: createdAt.UTC().Format(time.RFC3339), DueAt: dueAt.UTC().Format(time.RFC3339), ActionPath: path}
}

func derivedUrgentAction(domain, sourceID, sourceType, summary string, createdAt time.Time, path string) actionItem {
	return derivedAction(domain, sourceID, sourceType, summary, createdAt, createdAt.Add(24*time.Hour), path)
}

var _ = foodPolicyVersion
