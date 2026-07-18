package worker

import (
	"context"
	"errors"
	"time"

	redislib "github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const maxOutboxAttempts = 5

type eventPublisher interface {
	Publish(context.Context, string, outboxEvent) error
}

type redisEventPublisher struct{ client *redislib.Client }

func (publisher redisEventPublisher) Publish(ctx context.Context, stream string, event outboxEvent) error {
	return publisher.client.XAdd(ctx, &redislib.XAddArgs{Stream: stream, Values: map[string]any{
		"event_id": event.ID, "event_type": event.EventType, "aggregate_type": event.AggregateType,
		"aggregate_id": event.AggregateID, "payload": string(event.Payload),
	}}).Err()
}

type OutboxProcessor struct {
	db        *gorm.DB
	publisher eventPublisher
	stream    string
}

func NewOutboxProcessor(db *gorm.DB, publisher eventPublisher, stream string) OutboxProcessor {
	return OutboxProcessor{db: db, publisher: publisher, stream: stream}
}

func (processor OutboxProcessor) ProcessNext(ctx context.Context) error {
	now := time.Now().UTC()
	var event outboxEvent
	err := processor.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("(status = ? OR (status = ? AND locked_at < ?)) AND available_at <= ?", "pending", "publishing", now.Add(-5*time.Minute), now).
			Order("available_at asc, created_at asc")
		if err := query.First(&event).Error; err != nil {
			return err
		}
		return tx.Model(&event).Updates(map[string]any{"status": "publishing", "locked_at": now, "attempt_count": event.AttemptCount + 1}).Error
	})
	if err != nil {
		return err
	}
	event.AttemptCount++
	if err := processor.publisher.Publish(ctx, processor.stream, event); err != nil {
		status := "pending"
		if event.AttemptCount >= maxOutboxAttempts {
			status = "failed"
		}
		processor.db.WithContext(ctx).Model(&outboxEvent{}).Where("id = ?", event.ID).Updates(map[string]any{
			"status": status, "locked_at": nil, "available_at": now.Add(time.Duration(event.AttemptCount*event.AttemptCount) * time.Minute), "last_error_code": "event_publish_failed",
		})
		return err
	}
	publishedAt := time.Now().UTC()
	result := processor.db.WithContext(ctx).Model(&outboxEvent{}).Where("id = ? AND status = ?", event.ID, "publishing").Updates(map[string]any{"status": "published", "published_at": publishedAt, "locked_at": nil, "last_error_code": ""})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("outbox event lost publishing claim")
	}
	return nil
}
