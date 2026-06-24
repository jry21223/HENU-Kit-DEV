package tests

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"final-review-platform/services/api/internal/platform/model"
	"final-review-platform/services/api/internal/server"
	applogger "final-review-platform/services/api/pkg/logger"
)

func TestAdminMediaAssetAuditAndCleanup(t *testing.T) {
	db := newTestDB(t)
	cfg := testConfig()
	cfg.LocalUploadDir = t.TempDir()
	router := server.NewRouter(cfg, applogger.New("test"), db, nil)

	admin := createTestUser(t, db, "media-admin@stu.henu.edu.cn", model.RoleAdmin)
	student := createTestUser(t, db, "media-student@stu.henu.edu.cn", model.RoleUser)
	owner := createTestUser(t, db, "media-owner@stu.henu.edu.cn", model.RoleUser)
	adminToken := loginTestUser(t, router, admin.Email)
	studentToken := loginTestUser(t, router, student.Email)

	oldTime := time.Now().Add(-72 * time.Hour)
	createAsset := func(fileName string, status string, momentID *string, createdAt time.Time) model.MediaAsset {
		t.Helper()
		asset := model.MediaAsset{
			BaseModel: model.BaseModel{
				CreatedAt: createdAt,
				UpdatedAt: createdAt,
			},
			OwnerID:     owner.ID,
			Usage:       "moment_image",
			StorageKey:  "moments/" + owner.ID + "/" + fileName,
			FileName:    fileName,
			FileSize:    int64(len(validPNGBytes())),
			ContentType: "image/png",
			Status:      status,
			MomentID:    momentID,
		}
		if err := db.Create(&asset).Error; err != nil {
			t.Fatal(err)
		}
		return asset
	}

	oldAsset := createAsset("old.png", "uploaded", nil, oldTime)
	writeUploadBytes(t, cfg.LocalUploadDir, oldAsset.StorageKey, validPNGBytes())
	freshAsset := createAsset("fresh.png", "uploaded", nil, time.Now())
	writeUploadBytes(t, cfg.LocalUploadDir, freshAsset.StorageKey, validPNGBytes())

	moment := model.Moment{
		AuthorID: owner.ID,
		Content:  "attached image",
		Status:   model.StatusPublished,
	}
	if err := db.Create(&moment).Error; err != nil {
		t.Fatal(err)
	}
	attachedAsset := createAsset("attached.png", "attached", &moment.ID, oldTime)
	writeUploadBytes(t, cfg.LocalUploadDir, attachedAsset.StorageKey, validPNGBytes())
	missingAsset := createAsset("missing.png", "uploaded", nil, oldTime)

	studentList := performJSON(router, http.MethodGet, "/api/v1/admin/media-assets", "", studentToken)
	if studentList.Code != http.StatusForbidden {
		t.Fatalf("expected student media list 403, got %d: %s", studentList.Code, studentList.Body.String())
	}
	invalidStatus := performJSON(router, http.MethodGet, "/api/v1/admin/media-assets?status=live", "", adminToken)
	if invalidStatus.Code != http.StatusBadRequest || !strings.Contains(invalidStatus.Body.String(), "invalid_status") {
		t.Fatalf("expected invalid status rejection, got %d: %s", invalidStatus.Code, invalidStatus.Body.String())
	}
	invalidUsage := performJSON(router, http.MethodGet, "/api/v1/admin/media-assets?usage=avatar", "", adminToken)
	if invalidUsage.Code != http.StatusBadRequest || !strings.Contains(invalidUsage.Body.String(), "invalid_usage") {
		t.Fatalf("expected invalid usage rejection, got %d: %s", invalidUsage.Code, invalidUsage.Body.String())
	}

	list := performJSON(router, http.MethodGet, "/api/v1/admin/media-assets?usage=moment_image&status=uploaded&ownerEmail=media-owner&limit=10", "", adminToken)
	if list.Code != http.StatusOK {
		t.Fatalf("expected media list 200, got %d: %s", list.Code, list.Body.String())
	}
	listBody := list.Body.String()
	if !strings.Contains(listBody, oldAsset.ID) || !strings.Contains(listBody, freshAsset.ID) || !strings.Contains(listBody, missingAsset.ID) {
		t.Fatalf("expected uploaded assets in admin list, got %s", listBody)
	}
	if strings.Contains(listBody, attachedAsset.ID) {
		t.Fatalf("expected attached asset to be filtered by uploaded status, got %s", listBody)
	}
	if strings.Contains(listBody, "storageKey") || strings.Contains(listBody, oldAsset.StorageKey) {
		t.Fatalf("expected admin list not to expose storage keys, got %s", listBody)
	}

	dryRun := performJSON(router, http.MethodPost, "/api/v1/admin/media-assets/cleanup", `{"olderThanHours":24,"dryRun":true,"limit":10}`, adminToken)
	if dryRun.Code != http.StatusOK {
		t.Fatalf("expected dry-run cleanup 200, got %d: %s", dryRun.Code, dryRun.Body.String())
	}
	dryRunPayload := decodeMediaCleanupPayload(t, dryRun.Body.Bytes())
	if !dryRunPayload.Data.Cleanup.DryRun || dryRunPayload.Data.Cleanup.Candidates != 2 || dryRunPayload.Data.Cleanup.DeletedFiles != 0 || dryRunPayload.Data.Cleanup.ArchivedRows != 0 {
		t.Fatalf("unexpected dry-run cleanup summary: %#v", dryRunPayload.Data.Cleanup)
	}
	if _, err := os.Stat(filepath.Join(cfg.LocalUploadDir, filepath.FromSlash(oldAsset.StorageKey))); err != nil {
		t.Fatalf("dry-run should not remove file: %v", err)
	}
	var unchanged model.MediaAsset
	if err := db.First(&unchanged, "id = ?", oldAsset.ID).Error; err != nil {
		t.Fatal(err)
	}
	if unchanged.Status != "uploaded" {
		t.Fatalf("dry-run should not archive rows, got %s", unchanged.Status)
	}

	cleanup := performJSON(router, http.MethodPost, "/api/v1/admin/media-assets/cleanup", `{"olderThanHours":24,"dryRun":false,"limit":10}`, adminToken)
	if cleanup.Code != http.StatusOK {
		t.Fatalf("expected real cleanup 200, got %d: %s", cleanup.Code, cleanup.Body.String())
	}
	cleanupPayload := decodeMediaCleanupPayload(t, cleanup.Body.Bytes())
	if cleanupPayload.Data.Cleanup.DryRun || cleanupPayload.Data.Cleanup.Candidates != 2 || cleanupPayload.Data.Cleanup.DeletedFiles != 1 || cleanupPayload.Data.Cleanup.MissingFiles != 1 || cleanupPayload.Data.Cleanup.ArchivedRows != 2 {
		t.Fatalf("unexpected real cleanup summary: %#v", cleanupPayload.Data.Cleanup)
	}
	if _, err := os.Stat(filepath.Join(cfg.LocalUploadDir, filepath.FromSlash(oldAsset.StorageKey))); !os.IsNotExist(err) {
		t.Fatalf("expected old file to be deleted, got %v", err)
	}
	for _, expectedArchived := range []model.MediaAsset{oldAsset, missingAsset} {
		var refreshed model.MediaAsset
		if err := db.First(&refreshed, "id = ?", expectedArchived.ID).Error; err != nil {
			t.Fatal(err)
		}
		if refreshed.Status != model.StatusArchived {
			t.Fatalf("expected asset %s archived, got %s", refreshed.ID, refreshed.Status)
		}
	}
	var fresh model.MediaAsset
	if err := db.First(&fresh, "id = ?", freshAsset.ID).Error; err != nil {
		t.Fatal(err)
	}
	if fresh.Status != "uploaded" {
		t.Fatalf("expected fresh asset to remain uploaded, got %s", fresh.Status)
	}
	var attached model.MediaAsset
	if err := db.First(&attached, "id = ?", attachedAsset.ID).Error; err != nil {
		t.Fatal(err)
	}
	if attached.Status != "attached" {
		t.Fatalf("expected attached asset to remain attached, got %s", attached.Status)
	}
	if countOperationLogs(t, db, "media_asset.cleanup", "media_asset", "", admin.ID) != 1 {
		t.Fatal("expected media cleanup operation log")
	}

	repeat := performJSON(router, http.MethodPost, "/api/v1/admin/media-assets/cleanup", `{"olderThanHours":24,"dryRun":false,"limit":10}`, adminToken)
	if repeat.Code != http.StatusOK {
		t.Fatalf("expected repeat cleanup 200, got %d: %s", repeat.Code, repeat.Body.String())
	}
	repeatPayload := decodeMediaCleanupPayload(t, repeat.Body.Bytes())
	if repeatPayload.Data.Cleanup.Candidates != 0 {
		t.Fatalf("expected repeat cleanup to be idempotent, got %#v", repeatPayload.Data.Cleanup)
	}
}

func decodeMediaCleanupPayload(t *testing.T, body []byte) struct {
	Data struct {
		Cleanup struct {
			DryRun         bool  `json:"dryRun"`
			Candidates     int   `json:"candidates"`
			DeletedFiles   int   `json:"deletedFiles"`
			MissingFiles   int   `json:"missingFiles"`
			ArchivedRows   int64 `json:"archivedRows"`
			OlderThanHours int   `json:"olderThanHours"`
		} `json:"cleanup"`
	} `json:"data"`
} {
	t.Helper()
	var payload struct {
		Data struct {
			Cleanup struct {
				DryRun         bool  `json:"dryRun"`
				Candidates     int   `json:"candidates"`
				DeletedFiles   int   `json:"deletedFiles"`
				MissingFiles   int   `json:"missingFiles"`
				ArchivedRows   int64 `json:"archivedRows"`
				OlderThanHours int   `json:"olderThanHours"`
			} `json:"cleanup"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	return payload
}
