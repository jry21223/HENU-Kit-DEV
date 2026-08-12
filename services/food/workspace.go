package food

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const workspaceCacheKey = "food:workspace:last-success"

type submission struct {
	ID          string    `json:"id"`
	VenueName   string    `json:"venue_name"`
	ItemName    string    `json:"item_name"`
	Description string    `json:"description"`
	Status      string    `json:"status"`
	Version     int       `json:"version"`
	SubmittedAt time.Time `json:"submitted_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
type anomalyTicket struct {
	ID        string    `json:"id"`
	VenueName string    `json:"venue_name"`
	Kind      string    `json:"kind"`
	Details   string    `json:"details"`
	Severity  string    `json:"severity"`
	Status    string    `json:"status"`
	Version   int       `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
type tierAdjustment struct {
	ID           string    `json:"id"`
	VenueName    string    `json:"venue_name"`
	CurrentTier  string    `json:"current_tier"`
	ProposedTier string    `json:"proposed_tier"`
	Reason       string    `json:"reason"`
	Status       string    `json:"status"`
	Version      int       `json:"version"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}
type workspaceData struct {
	Status             string           `json:"status"`
	StatusMessage      string           `json:"status_message"`
	Stale              bool             `json:"stale"`
	AsOf               time.Time        `json:"as_of"`
	Submissions        []submission     `json:"submissions"`
	AnomalyTickets     []anomalyTicket  `json:"anomaly_tickets"`
	TierAdjustments    []tierAdjustment `json:"tier_adjustments"`
	PendingSubmissions int              `json:"-"`
	OpenAnomalies      int              `json:"-"`
	PendingTiers       int              `json:"-"`
}
type cachedWorkspace struct {
	Workspace          workspaceData `json:"workspace"`
	CachedAt           time.Time     `json:"cached_at"`
	PendingSubmissions int           `json:"pending_submissions"`
	OpenAnomalies      int           `json:"open_anomalies"`
	PendingTiers       int           `json:"pending_tiers"`
}

func (h *service) loadWorkspace(r *http.Request) (workspaceData, error) {
	result := workspaceData{Submissions: []submission{}, AnomalyTickets: []anomalyTicket{}, TierAdjustments: []tierAdjustment{}}
	rows, err := h.database.Query(r.Context(), `SELECT id,venue_name,item_name,description,status,version,submitted_at,updated_at FROM food_submissions WHERE status='pending' ORDER BY submitted_at DESC LIMIT 200`)
	if err != nil {
		return h.staleWorkspace(r, err)
	}
	for rows.Next() {
		var item submission
		if err := rows.Scan(&item.ID, &item.VenueName, &item.ItemName, &item.Description, &item.Status, &item.Version, &item.SubmittedAt, &item.UpdatedAt); err != nil {
			rows.Close()
			return h.staleWorkspace(r, err)
		}
		result.Submissions = append(result.Submissions, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return h.staleWorkspace(r, err)
	}
	rows.Close()
	rows, err = h.database.Query(r.Context(), `SELECT id,venue_name,kind,details,severity,status,version,created_at,updated_at FROM food_anomaly_tickets WHERE status='open' ORDER BY created_at DESC LIMIT 200`)
	if err != nil {
		return h.staleWorkspace(r, err)
	}
	for rows.Next() {
		var item anomalyTicket
		if err := rows.Scan(&item.ID, &item.VenueName, &item.Kind, &item.Details, &item.Severity, &item.Status, &item.Version, &item.CreatedAt, &item.UpdatedAt); err != nil {
			rows.Close()
			return h.staleWorkspace(r, err)
		}
		result.AnomalyTickets = append(result.AnomalyTickets, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return h.staleWorkspace(r, err)
	}
	rows.Close()
	rows, err = h.database.Query(r.Context(), `SELECT id,venue_name,current_tier,proposed_tier,reason,status,version,created_at,updated_at FROM food_tier_adjustments WHERE status='pending' ORDER BY created_at DESC LIMIT 200`)
	if err != nil {
		return h.staleWorkspace(r, err)
	}
	for rows.Next() {
		var item tierAdjustment
		if err := rows.Scan(&item.ID, &item.VenueName, &item.CurrentTier, &item.ProposedTier, &item.Reason, &item.Status, &item.Version, &item.CreatedAt, &item.UpdatedAt); err != nil {
			rows.Close()
			return h.staleWorkspace(r, err)
		}
		result.TierAdjustments = append(result.TierAdjustments, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return h.staleWorkspace(r, err)
	}
	rows.Close()
	if err := h.database.QueryRow(r.Context(), `SELECT (SELECT count(*) FROM food_submissions WHERE status='pending'),(SELECT count(*) FROM food_anomaly_tickets WHERE status='open'),(SELECT count(*) FROM food_tier_adjustments WHERE status='pending')`).Scan(&result.PendingSubmissions, &result.OpenAnomalies, &result.PendingTiers); err != nil {
		return h.staleWorkspace(r, err)
	}
	result.AsOf = h.now().UTC()
	if len(result.Submissions)+len(result.AnomalyTickets)+len(result.TierAdjustments) == 0 {
		result.Status, result.StatusMessage = "empty", "暂无 Food 待处理事项"
	} else {
		result.Status, result.StatusMessage = "ok", "Food 数据正常"
	}
	encoded, _ := json.Marshal(cachedWorkspace{Workspace: result, CachedAt: result.AsOf, PendingSubmissions: result.PendingSubmissions, OpenAnomalies: result.OpenAnomalies, PendingTiers: result.PendingTiers})
	_ = h.redis.Set(r.Context(), workspaceCacheKey, encoded, 24*time.Hour).Err()
	return result, nil
}

func (h *service) staleWorkspace(r *http.Request, cause error) (workspaceData, error) {
	encoded, err := h.redis.Get(r.Context(), workspaceCacheKey).Bytes()
	if err != nil {
		return workspaceData{}, cause
	}
	var cached cachedWorkspace
	if json.Unmarshal(encoded, &cached) != nil || cached.CachedAt.IsZero() {
		return workspaceData{}, cause
	}
	cached.Workspace.Status = "stale"
	cached.Workspace.StatusMessage = "Food 实时数据暂不可用，展示最近成功快照"
	cached.Workspace.Stale = true
	cached.Workspace.AsOf = cached.CachedAt
	cached.Workspace.PendingSubmissions = cached.PendingSubmissions
	cached.Workspace.OpenAnomalies = cached.OpenAnomalies
	cached.Workspace.PendingTiers = cached.PendingTiers
	return cached.Workspace, nil
}

func (h *service) workspace(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.require(w, r, "food.read"); !ok {
		return
	}
	data, err := h.loadWorkspace(r)
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Food workspace is unavailable")
		return
	}
	writeData(w, r, http.StatusOK, data)
}
func (h *service) consoleSummary(w http.ResponseWriter, r *http.Request) {
	data, err := h.loadWorkspace(r)
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Food summary is unavailable")
		return
	}
	writeData(w, r, http.StatusOK, map[string]any{"status": data.Status, "status_message": data.StatusMessage, "as_of": data.AsOf, "last_success_at": data.AsOf, "metrics": []map[string]string{{"label": "待审核投稿", "value": fmt.Sprint(data.PendingSubmissions)}, {"label": "异常票", "value": fmt.Sprint(data.OpenAnomalies)}, {"label": "待确认调档", "value": fmt.Sprint(data.PendingTiers)}}})
}
