package career

import (
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type profileInput struct {
	TargetRoles       string `json:"target_roles"`
	TechStack         string `json:"tech_stack"`
	Locations         string `json:"locations"`
	JobType           string `json:"job_type"`
	GraduationYear    *int   `json:"graduation_year"`
	ResumeText        string `json:"resume_text"`
	EmailNotification *bool  `json:"email_notification_enabled"`
}

type profileWire struct {
	UserID            string `json:"user_id"`
	TargetRoles       string `json:"target_roles"`
	TechStack         string `json:"tech_stack"`
	Locations         string `json:"locations"`
	JobType           string `json:"job_type"`
	GraduationYear    *int   `json:"graduation_year"`
	ResumeText        string `json:"resume_text"`
	EmailNotification bool   `json:"email_notification_enabled"`
	UpdatedAt         string `json:"updated_at"`
}

func (h *service) getProfile(w http.ResponseWriter, r *http.Request) {
	value, ok := h.requireActor(w, r)
	if !ok {
		return
	}
	profile, found, err := h.loadProfile(r, value.userID)
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "career profile is unavailable")
		return
	}
	if !found {
		writeData(w, r, http.StatusOK, map[string]any{"profile": emptyProfile(value.userID)})
		return
	}
	writeData(w, r, http.StatusOK, map[string]any{"profile": profile})
}

func (h *service) updateProfile(w http.ResponseWriter, r *http.Request) {
	value, ok := h.requireActor(w, r)
	if !ok {
		return
	}
	var input profileInput
	if _, ok := decode(w, r, &input); !ok {
		return
	}
	input.TargetRoles = strings.TrimSpace(input.TargetRoles)
	if !validProfileInput(input) {
		writeError(w, r, http.StatusBadRequest, "INVALID_PROFILE", "career profile is invalid")
		return
	}
	enabled := true
	if input.EmailNotification != nil {
		enabled = *input.EmailNotification
	}
	profile, err := h.storeProfile(r, value.userID, input, enabled)
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "career profile is unavailable")
		return
	}
	writeData(w, r, http.StatusOK, map[string]any{"profile": profile})
}

func validProfileInput(input profileInput) bool {
	if strings.TrimSpace(input.TargetRoles) == "" {
		return false
	}
	switch input.JobType {
	case "", "daily_intern", "summer_intern", "campus_recruit":
	default:
		return false
	}
	if input.GraduationYear != nil && (*input.GraduationYear < 1900 || *input.GraduationYear > 2200) {
		return false
	}
	return true
}

func (h *service) loadProfile(r *http.Request, userID string) (profileWire, bool, error) {
	var item profileWire
	var graduationYear *int
	var updatedAt time.Time
	err := h.database.QueryRow(r.Context(), `SELECT user_id,target_roles,tech_stack,locations,job_type,graduation_year,resume_text,email_notification_enabled,updated_at FROM career_profiles WHERE user_id=$1`, userID).Scan(&item.UserID, &item.TargetRoles, &item.TechStack, &item.Locations, &item.JobType, &graduationYear, &item.ResumeText, &item.EmailNotification, &updatedAt)
	if err == pgx.ErrNoRows {
		return profileWire{}, false, nil
	}
	if err != nil {
		return profileWire{}, false, err
	}
	item.GraduationYear = graduationYear
	item.UpdatedAt = updatedAt.UTC().Format(time.RFC3339)
	return item, true, nil
}

func (h *service) storeProfile(r *http.Request, userID string, input profileInput, enabled bool) (profileWire, error) {
	graduationYear := input.GraduationYear
	_, err := h.database.Exec(r.Context(), `INSERT INTO career_profiles(user_id,target_roles,tech_stack,locations,job_type,graduation_year,resume_text,email_notification_enabled,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,now()) ON CONFLICT(user_id) DO UPDATE SET target_roles=EXCLUDED.target_roles,tech_stack=EXCLUDED.tech_stack,locations=EXCLUDED.locations,job_type=EXCLUDED.job_type,graduation_year=EXCLUDED.graduation_year,resume_text=EXCLUDED.resume_text,email_notification_enabled=EXCLUDED.email_notification_enabled,updated_at=now()`, userID, input.TargetRoles, input.TechStack, input.Locations, input.JobType, graduationYear, input.ResumeText, enabled)
	if err != nil {
		return profileWire{}, err
	}
	profile, _, err := h.loadProfile(r, userID)
	return profile, err
}

func emptyProfile(userID string) profileWire {
	return profileWire{UserID: userID, EmailNotification: true}
}
