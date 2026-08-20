package quizcraft

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

const (
	inboxBatchSize        = 20
	inboxMaxBatchesPerRun = 10
)

func (service *practiceHTTP) runInboxDispatcher(ctx context.Context) {
	service.drainInbox(ctx)
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-service.inboxDispatchWake:
			service.drainInbox(ctx)
		case <-ticker.C:
			service.drainInbox(ctx)
		}
	}
}

func (service *practiceHTTP) drainInbox(ctx context.Context) {
	for batch := 0; batch < inboxMaxBatchesPerRun; batch++ {
		if service.dispatchInboxBatch(ctx) < inboxBatchSize {
			return
		}
	}
	if ctx.Err() == nil {
		select {
		case service.inboxDispatchWake <- struct{}{}:
		default:
		}
	}
}

func (service *practiceHTTP) dispatchInboxBatch(parent context.Context) int {
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	// No row lock is taken here on purpose. The previous FOR UPDATE OF d SKIP
	// LOCKED was decorative: this Query runs on the pool rather than inside a
	// transaction, so the locks were released by rows.Close() below — before any
	// delivery was attempted. Holding a real transaction open across the outbound
	// HTTP calls would keep DB locks for the length of a network round trip, so
	// the idempotency key passed to createInboxItem is, and remains, the actual
	// guard against duplicate delivery.
	rows, err := service.database.Query(ctx, `SELECT o.id,o.source_product_code,o.source_resource_type,o.source_resource_id,o.source_resource_url,o.priority FROM quizcraft_feedback_inbox_outbox o JOIN quizcraft_feedback_inbox_deliveries d ON d.outbox_id=o.id WHERE d.delivered_at IS NULL AND d.next_attempt_at<=now() ORDER BY o.created_at LIMIT 20`)
	if err != nil {
		return 0
	}
	type item struct {
		id                                                       uuid.UUID
		product, resourceType, resourceID, resourceURL, priority string
	}
	items := make([]item, 0)
	for rows.Next() {
		var value item
		if scanErr := rows.Scan(&value.id, &value.product, &value.resourceType, &value.resourceID, &value.resourceURL, &value.priority); scanErr != nil {
			// A row that will not scan used to be skipped silently. Its delivery
			// row keeps delivered_at IS NULL, so it was re-selected and dropped
			// again on every tick — a permanently stuck item with no error
			// surface and no backoff, since attempts was never incremented.
			// Abandon the batch instead: returning 0 stops the drain loop rather
			// than reporting progress that did not happen.
			rows.Close()
			return 0
		}
		items = append(items, value)
	}
	// Iteration that stops early leaves a short batch that is indistinguishable
	// from an exhausted queue, and the caller uses the returned count to decide
	// whether to keep draining.
	if rows.Err() != nil {
		rows.Close()
		return 0
	}
	rows.Close()
	for _, value := range items {
		body, _ := json.Marshal(map[string]string{"source_product_code": value.product, "source_resource_type": value.resourceType, "source_resource_id": value.resourceID, "source_resource_url": value.resourceURL, "priority": value.priority, "status": "open"})
		platformID, deliveryErr := service.platform.createInboxItem(ctx, service.inboxExchangeToken, "idem_quizcraft_inbox_"+value.id.String(), body)
		if deliveryErr == nil {
			_, _ = service.database.Exec(ctx, `UPDATE quizcraft_feedback_inbox_deliveries SET platform_item_id=$2,attempts=attempts+1,delivered_at=now(),last_error='' WHERE outbox_id=$1 AND delivered_at IS NULL`, value.id, platformID)
			continue
		}
		message := deliveryErr.Error()
		if len(message) > 500 {
			message = message[:500]
		}
		_, _ = service.database.Exec(ctx, `UPDATE quizcraft_feedback_inbox_deliveries SET attempts=attempts+1,last_error=$2,next_attempt_at=now()+least(interval '5 minutes',interval '5 seconds'*power(2,least(attempts,6))) WHERE outbox_id=$1 AND delivered_at IS NULL`, value.id, message)
	}
	return len(items)
}
