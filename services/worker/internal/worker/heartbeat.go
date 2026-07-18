package worker

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"final-review-platform/services/worker/pkg/config"
)

func startHeartbeat(ctx context.Context, db *gorm.DB, cfg config.Config) {
	write := func() {
		now := time.Now().UTC()
		var outboxPending int64
		var outboxFailed int64
		var deadLetters int64
		db.WithContext(ctx).Model(&outboxEvent{}).Where("status IN ?", []string{"pending", "publishing"}).Count(&outboxPending)
		db.WithContext(ctx).Model(&outboxEvent{}).Where("status = ?", "failed").Count(&outboxFailed)
		db.WithContext(ctx).Model(&mailDeadLetter{}).Where("status = ?", "open").Count(&deadLetters)
		anomalies := outboxFailed + deadLetters
		status := "ok"
		if anomalies > 0 {
			status = "partial"
		}
		heartbeat := serviceHeartbeat{ServiceID: "worker", Status: status, Version: cfg.Version, CommitSHA: cfg.CommitSHA, LastReadyAt: now, OutboxPending: outboxPending, WorkerAnomalies: anomalies}
		_ = db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "service_id"}}, DoUpdates: clause.Assignments(map[string]any{"status": status, "service_version": cfg.Version, "commit_sha": cfg.CommitSHA, "last_ready_at": now, "outbox_pending": outboxPending, "worker_anomalies": anomalies, "updated_at": now})}).Create(&heartbeat).Error
	}
	write()
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				write()
			}
		}
	}()
}
