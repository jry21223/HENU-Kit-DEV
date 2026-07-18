package worker

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"final-review-platform/services/worker/internal/mailprovider"
)

const maxMailAttempts = 3

type MailProcessor struct {
	db       *gorm.DB
	provider mailprovider.Provider
	workerID string
}

func NewMailProcessor(db *gorm.DB, provider mailprovider.Provider) MailProcessor {
	return MailProcessor{db: db, provider: provider, workerID: "mail-" + uuid.NewString()}
}

func (p MailProcessor) ProcessNext(ctx context.Context) error {
	if p.provider == nil {
		return gorm.ErrRecordNotFound
	}
	now := time.Now().UTC()
	var delivery mailDelivery
	err := p.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		cutoff := now.Add(-5 * time.Minute)
		if err := tx.Model(&mailDelivery{}).Where("status = ? AND locked_at < ?", "sending", cutoff).Updates(map[string]any{"status": "queued", "locked_at": nil, "locked_by": ""}).Error; err != nil {
			return err
		}
		query := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("status IN ? AND (next_retry_at IS NULL OR next_retry_at <= ?)", []string{"queued", "retry"}, now).
			Order("CASE category WHEN 'critical' THEN 0 WHEN 'transactional' THEN 1 WHEN 'digest' THEN 2 ELSE 3 END").
			Order("queued_at asc")
		if err := query.First(&delivery).Error; err != nil {
			return err
		}
		return tx.Model(&delivery).Updates(map[string]any{"status": "sending", "locked_at": now, "locked_by": p.workerID, "version": delivery.Version + 1}).Error
	})
	if err != nil {
		return err
	}

	var suppression mailSuppression
	suppressed := p.db.WithContext(ctx).Where("recipient_hash = ? AND (expires_at IS NULL OR expires_at > ?)", delivery.RecipientHash, now).First(&suppression).Error == nil
	if suppressed {
		return p.finishSuppressed(ctx, delivery, now, "recipient_suppressed")
	}
	if delivery.TemplateCode == "campus_notice" && delivery.RecipientUserID != nil {
		var subscription noticeEmailSubscription
		subscribed := p.db.WithContext(ctx).Where("user_id = ? AND enabled = ?", *delivery.RecipientUserID, true).First(&subscription).Error == nil
		if !subscribed {
			return p.finishSuppressed(ctx, delivery, now, "recipient_unsubscribed")
		}
	}
	startedAt := time.Now().UTC()
	result, sendErr := p.provider.Send(ctx, mailprovider.Message{To: delivery.Recipient, Subject: delivery.Subject, Text: delivery.Body})
	endedAt := time.Now().UTC()
	if sendErr != nil {
		return p.finishFailure(ctx, delivery, startedAt, endedAt)
	}
	if result.Status != mailprovider.StatusAccepted {
		return errors.New("mail provider returned unsupported status")
	}
	return p.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		attempt := mailAttempt{DeliveryID: delivery.ID, Attempt: delivery.AttemptCount + 1, Status: "accepted", StartedAt: startedAt, EndedAt: &endedAt}
		if err := tx.Create(&attempt).Error; err != nil {
			return err
		}
		return tx.Model(&mailDelivery{}).Where("id = ? AND locked_by = ?", delivery.ID, p.workerID).Updates(map[string]any{
			"status": "accepted", "accepted_at": result.AcceptedAt, "attempt_count": delivery.AttemptCount + 1,
			"next_retry_at": nil, "locked_at": nil, "locked_by": "", "last_error_code": "", "version": gorm.Expr("version + 1"),
		}).Error
	})
}

func (p MailProcessor) finishSuppressed(ctx context.Context, delivery mailDelivery, now time.Time, reasonCode string) error {
	return p.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		attempt := mailAttempt{DeliveryID: delivery.ID, Attempt: delivery.AttemptCount + 1, Status: "suppressed", ErrorCode: reasonCode, StartedAt: now, EndedAt: &now}
		if err := tx.Create(&attempt).Error; err != nil {
			return err
		}
		return tx.Model(&mailDelivery{}).Where("id = ? AND locked_by = ?", delivery.ID, p.workerID).Updates(map[string]any{"status": "suppressed", "attempt_count": delivery.AttemptCount + 1, "locked_at": nil, "locked_by": "", "last_error_code": reasonCode, "version": gorm.Expr("version + 1")}).Error
	})
}

func (p MailProcessor) finishFailure(ctx context.Context, delivery mailDelivery, startedAt, endedAt time.Time) error {
	attemptNumber := delivery.AttemptCount + 1
	return p.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		attempt := mailAttempt{DeliveryID: delivery.ID, Attempt: attemptNumber, Status: "failed", ErrorCode: "provider_error", StartedAt: startedAt, EndedAt: &endedAt}
		if err := tx.Create(&attempt).Error; err != nil {
			return err
		}
		if attemptNumber >= maxMailAttempts {
			deadLetter := mailDeadLetter{DeliveryID: delivery.ID, Status: "open", ReasonCode: "provider_error"}
			if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "delivery_id"}}, DoUpdates: clause.Assignments(map[string]any{"status": "open", "reason_code": "provider_error", "resolved_at": nil})}).Create(&deadLetter).Error; err != nil {
				return err
			}
			return tx.Model(&mailDelivery{}).Where("id = ? AND locked_by = ?", delivery.ID, p.workerID).Updates(map[string]any{"status": "failed", "attempt_count": attemptNumber, "locked_at": nil, "locked_by": "", "last_error_code": "provider_error", "version": gorm.Expr("version + 1")}).Error
		}
		nextRetry := endedAt.Add(time.Duration(attemptNumber*attemptNumber) * time.Minute)
		return tx.Model(&mailDelivery{}).Where("id = ? AND locked_by = ?", delivery.ID, p.workerID).Updates(map[string]any{"status": "retry", "attempt_count": attemptNumber, "next_retry_at": nextRetry, "locked_at": nil, "locked_by": "", "last_error_code": "provider_error", "version": gorm.Expr("version + 1")}).Error
	})
}
