package library

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

type legacyAdapter struct {
	baseURL string
	token   string
	client  *http.Client
}

type upstreamError struct {
	status int
}

func (e upstreamError) Error() string { return fmt.Sprintf("legacy Study API returned %d", e.status) }

func newLegacyAdapter(baseURL, token string, client *http.Client) (*legacyAdapter, error) {
	parsed, err := url.Parse(baseURL)
	loopback := err == nil && parsed.Scheme == "http" && (parsed.Hostname() == "localhost" || net.ParseIP(parsed.Hostname()).IsLoopback())
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && !loopback) || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || strings.TrimSpace(token) == "" {
		return nil, errors.New("invalid Study Legacy API configuration")
	}
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	return &legacyAdapter{baseURL: strings.TrimRight(baseURL, "/"), token: token, client: client}, nil
}

func (a *legacyAdapter) workspace(ctx context.Context) Workspace {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	result := Workspace{Status: "ok", StatusMessage: "Library compatibility data is current.", Courses: []Course{}, Materials: []Material{}, Downloads: []Download{}, Submissions: []Material{}, Corrections: []Correction{}, GeneratedAt: time.Now().UTC()}
	type fetchResult struct {
		kind string
		data json.RawMessage
		err  error
	}
	requests := []struct{ kind, path string }{
		{"courses", "/api/v1/admin/courses"},
		{"materials", "/api/v1/admin/materials"},
		{"downloads", "/api/v1/admin/downloads"},
		{"submissions", "/api/v1/admin/material-reviews"},
		{"corrections", "/api/v1/admin/reports?status=all"},
	}
	results := make(chan fetchResult, len(requests))
	var wait sync.WaitGroup
	for _, item := range requests {
		wait.Add(1)
		go func() {
			defer wait.Done()
			data, err := a.get(ctx, item.path)
			results <- fetchResult{kind: item.kind, data: data, err: err}
		}()
	}
	wait.Wait()
	close(results)
	failures := 0
	for item := range results {
		if item.err != nil {
			failures++
			continue
		}
		if err := decodeWorkspacePart(item.kind, item.data, &result); err != nil {
			failures++
		}
	}
	if failures > 0 {
		result.Degraded = true
		result.Status = "partial"
		result.StatusMessage = "部分旧资料能力暂不可用；已返回可确认的数据。"
	}
	if failures == len(requests) {
		result.Status = "unavailable"
		result.StatusMessage = "Study Legacy API 暂不可用。"
	}
	return result
}

func decodeWorkspacePart(kind string, payload json.RawMessage, workspace *Workspace) error {
	switch kind {
	case "courses":
		var value struct {
			Courses []legacyCourse `json:"courses"`
		}
		if err := json.Unmarshal(payload, &value); err != nil {
			return err
		}
		for _, item := range value.Courses {
			workspace.Courses = append(workspace.Courses, Course(item))
		}
	case "materials", "submissions":
		var value struct {
			Materials []legacyMaterial `json:"materials"`
		}
		if err := json.Unmarshal(payload, &value); err != nil {
			return err
		}
		target := &workspace.Materials
		if kind == "submissions" {
			target = &workspace.Submissions
		}
		for _, item := range value.Materials {
			*target = append(*target, Material{ID: item.ID, CourseID: item.CourseID, Title: item.Title, Type: item.Type, FileName: item.FileName, FileSize: item.FileSize, AccessLevel: libraryAccessLevel(item.AccessLevel), Status: item.Status, UpdatedAt: item.UpdatedAt})
		}
	case "downloads":
		var value struct {
			Downloads []legacyDownload `json:"downloads"`
		}
		if err := json.Unmarshal(payload, &value); err != nil {
			return err
		}
		for _, item := range value.Downloads {
			title := ""
			if item.Material != nil {
				title = item.Material.Title
			}
			workspace.Downloads = append(workspace.Downloads, Download{ID: item.ID, MaterialID: item.MaterialID, MaterialTitle: title, AccessLevel: libraryAccessLevel(item.AccessLevel), DownloadedAt: item.DownloadedAt})
		}
	case "corrections":
		var value struct {
			Reports []legacyCorrection `json:"reports"`
		}
		if err := json.Unmarshal(payload, &value); err != nil {
			return err
		}
		for _, item := range value.Reports {
			if item.TargetType == "material" || item.TargetType == "course" {
				workspace.Corrections = append(workspace.Corrections, Correction(item))
			}
		}
	default:
		return errors.New("unsupported workspace part")
	}
	return nil
}

func (a *legacyAdapter) get(ctx context.Context, path string) (json.RawMessage, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, a.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+a.token)
	response, err := a.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
		return nil, upstreamError{status: response.StatusCode}
	}
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&envelope); err != nil || len(envelope.Data) == 0 {
		return nil, errors.New("invalid Study Legacy API response")
	}
	return envelope.Data, nil
}

func (a *legacyAdapter) command(ctx context.Context, command commandInput) (string, error) {
	method, path, err := legacyCommandRoute(command)
	if err != nil {
		return "", err
	}
	body, err := filteredPayload(command.Kind, command.Payload)
	if err != nil {
		return "", err
	}
	request, err := http.NewRequestWithContext(ctx, method, a.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	request.Header.Set("Authorization", "Bearer "+a.token)
	request.Header.Set("Content-Type", "application/json")
	response, err := a.client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 256<<10))
		return "", upstreamError{status: response.StatusCode}
	}
	if command.Kind != "course_create" && command.Kind != "material_create" {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 256<<10))
		return command.ResourceID, nil
	}
	var envelope struct {
		Data struct {
			Course *struct {
				ID string `json:"id"`
			} `json:"course"`
			Material *struct {
				ID string `json:"id"`
			} `json:"material"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&envelope); err != nil {
		return "", errors.New("invalid Study Legacy API create response")
	}
	resourceID := ""
	if envelope.Data.Course != nil {
		resourceID = envelope.Data.Course.ID
	}
	if envelope.Data.Material != nil {
		resourceID = envelope.Data.Material.ID
	}
	if _, err := uuid.Parse(resourceID); err != nil {
		return "", errors.New("invalid Study Legacy API create resource id")
	}
	return resourceID, nil
}

func legacyCommandRoute(command commandInput) (string, string, error) {
	id := url.PathEscape(command.ResourceID)
	switch command.Kind {
	case "course_create":
		return http.MethodPost, "/api/v1/admin/courses", nil
	case "course_update":
		return http.MethodPatch, "/api/v1/admin/courses/" + id, nil
	case "course_archive":
		return http.MethodDelete, "/api/v1/admin/courses/" + id, nil
	case "material_create":
		return http.MethodPost, "/api/v1/admin/materials", nil
	case "material_update":
		return http.MethodPatch, "/api/v1/admin/materials/" + id, nil
	case "material_archive":
		return http.MethodDelete, "/api/v1/admin/materials/" + id, nil
	case "submission_approve":
		return http.MethodPost, "/api/v1/admin/materials/" + id + "/approve", nil
	case "submission_reject":
		return http.MethodPost, "/api/v1/admin/materials/" + id + "/reject", nil
	case "correction_resolve":
		return http.MethodPost, "/api/v1/admin/reports/" + id + "/resolve", nil
	case "correction_reject":
		return http.MethodPost, "/api/v1/admin/reports/" + id + "/reject", nil
	default:
		return "", "", errors.New("command is outside the Library boundary")
	}
}

func filteredPayload(kind string, payload json.RawMessage) ([]byte, error) {
	type rule struct {
		required bool
		kind     string
		max      int
		enum     map[string]bool
	}
	stringRule := func(max int) rule { return rule{kind: "string", max: max} }
	uuidRule := rule{kind: "uuid"}
	courseFields := map[string]rule{
		"schoolId": uuidRule, "collegeId": uuidRule, "majorId": uuidRule,
		"grade": stringRule(32), "name": stringRule(160), "slug": stringRule(160),
		"description": stringRule(4000), "examScope": stringRule(2000),
		"status": {kind: "string", enum: map[string]bool{"draft": true, "published": true, "archived": true}},
	}
	materialType := map[string]bool{"knowledge_note": true, "mock_paper": true, "answer": true, "quick_review": true, "past_exam": true, "other": true}
	materialStatus := map[string]bool{"draft": true, "pending": true, "published": true, "rejected": true, "archived": true}
	materialCreateFields := map[string]rule{
		"courseId": uuidRule, "title": stringRule(200), "type": {kind: "string", enum: materialType},
		"description": stringRule(4000), "storageKey": stringRule(512), "fileName": stringRule(255),
		"fileSize": {kind: "integer"}, "previewContent": stringRule(20000),
		"accessLevel": {kind: "string", enum: map[string]bool{"free": true, "login_required": true}},
		"status":      {kind: "string", enum: materialStatus},
	}
	materialUpdateFields := map[string]rule{
		"courseId": uuidRule, "title": stringRule(200), "type": {kind: "string", enum: materialType},
		"description": stringRule(4000), "previewContent": stringRule(20000),
		"accessLevel": {kind: "string", enum: map[string]bool{"free": true, "login_required": true}},
		"status":      {kind: "string", enum: materialStatus},
	}
	reviewFields := map[string]rule{"reviewReason": stringRule(1000)}
	rules := map[string]map[string]rule{
		"course_create": courseFields, "course_update": courseFields, "course_archive": {},
		"material_create": materialCreateFields, "material_update": materialUpdateFields, "material_archive": {},
		"submission_approve": reviewFields, "submission_reject": reviewFields,
		"correction_resolve": reviewFields, "correction_reject": reviewFields,
	}
	fields, ok := rules[kind]
	if !ok {
		return nil, errors.New("command is outside the Library boundary")
	}
	if len(payload) == 0 {
		payload = json.RawMessage(`{}`)
	}
	var value map[string]json.RawMessage
	if err := json.Unmarshal(payload, &value); err != nil || value == nil {
		return nil, errors.New("invalid command payload")
	}
	if kind == "course_create" {
		for _, key := range []string{"schoolId", "collegeId", "majorId", "grade", "name", "slug"} {
			field := fields[key]
			field.required = true
			fields[key] = field
		}
	}
	if kind == "material_create" {
		for _, key := range []string{"courseId", "title", "storageKey"} {
			field := fields[key]
			field.required = true
			fields[key] = field
		}
	}
	for key, field := range fields {
		raw, exists := value[key]
		if field.required && !exists {
			return nil, fmt.Errorf("field %s is required", key)
		}
		if !exists {
			continue
		}
		switch field.kind {
		case "string", "uuid":
			var text string
			if json.Unmarshal(raw, &text) != nil || (field.max > 0 && len([]rune(text)) > field.max) {
				return nil, fmt.Errorf("field %s is invalid", key)
			}
			if field.kind == "uuid" {
				if _, err := uuid.Parse(text); err != nil {
					return nil, fmt.Errorf("field %s is invalid", key)
				}
			}
			if field.enum != nil && !field.enum[text] {
				return nil, fmt.Errorf("field %s is invalid", key)
			}
		case "integer":
			var number int64
			if json.Unmarshal(raw, &number) != nil || number < 0 {
				return nil, fmt.Errorf("field %s is invalid", key)
			}
		}
	}
	for key := range value {
		if _, exists := fields[key]; !exists {
			return nil, fmt.Errorf("field %s is outside the Library boundary", key)
		}
	}
	return json.Marshal(value)
}

func libraryAccessLevel(legacy string) string {
	switch legacy {
	case "free":
		return "public"
	case "login_required":
		return "authenticated"
	default:
		return "restricted"
	}
}
