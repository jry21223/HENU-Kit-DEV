package notice

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const maxDeliveryAttempts = 3

type Delivery struct {
	ID              uuid.UUID `json:"id"`
	NoticeVersionID uuid.UUID `json:"notice_version_id"`
	Channel         string    `json:"channel"`
	AudienceKind    string    `json:"audience_kind"`
	AudienceValue   *string   `json:"audience_value,omitempty"`
	Title           string    `json:"title"`
	Body            string    `json:"body"`
}

type DeliverySender interface {
	Deliver(context.Context, Delivery) error
}

type Worker struct {
	database *pgxpool.Pool
	sender   DeliverySender
	now      func() time.Time
}

func NewWorker(database *pgxpool.Pool, sender DeliverySender) (*Worker, error) {
	if database == nil || sender == nil {
		return nil, errors.New("notice worker database and sender are required")
	}
	return &Worker{database: database, sender: sender, now: time.Now}, nil
}

func (w *Worker) RunOnce(ctx context.Context) (bool, error) {
	tx, err := w.database.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var delivery Delivery
	var actorID, requestID string
	var attempts int
	err = tx.QueryRow(ctx, `
		WITH candidate AS (
			SELECT id FROM notice_distributions
			WHERE status='queued' OR (status='processing' AND claimed_at < $1)
			ORDER BY created_at
			FOR UPDATE SKIP LOCKED LIMIT 1
		)
		UPDATE notice_distributions d
		SET status='processing', claimed_at=$2, attempt_count=attempt_count+1, last_error=NULL
		FROM candidate c, notice_versions v
		WHERE d.id=c.id AND v.id=d.notice_version_id
		RETURNING d.id,d.notice_version_id,d.channel,d.audience_kind,d.audience_value,v.title,v.body,d.actor_user_id::text,d.request_id,d.attempt_count`,
		w.now().Add(-5*time.Minute), w.now()).Scan(&delivery.ID, &delivery.NoticeVersionID, &delivery.Channel, &delivery.AudienceKind, &delivery.AudienceValue, &delivery.Title, &delivery.Body, &actorID, &requestID, &attempts)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}

	deliveryErr := w.sender.Deliver(ctx, delivery)
	status, action := "delivered", "distribution.delivered"
	var lastError *string
	if deliveryErr != nil {
		message := deliveryErr.Error()
		if len(message) > 1000 {
			message = message[:1000]
		}
		lastError = &message
		status, action = "queued", "distribution.retry"
		if attempts >= maxDeliveryAttempts {
			status, action = "failed", "distribution.failed"
		}
	}
	completionTx, err := w.database.Begin(ctx)
	if err != nil {
		return true, err
	}
	defer func() { _ = completionTx.Rollback(ctx) }()
	command, err := completionTx.Exec(ctx, `UPDATE notice_distributions SET status=$2,completed_at=CASE WHEN $2::text IN ('delivered','failed') THEN $3::timestamptz ELSE NULL END,last_error=$4 WHERE id=$1 AND status='processing'`, delivery.ID, status, w.now(), lastError)
	if err != nil || command.RowsAffected() != 1 {
		if err == nil {
			err = errors.New("notice distribution claim was lost")
		}
		return true, err
	}
	actor := actor{userID: actorID, permission: "notice.distribute"}
	if err := audit(ctx, completionTx, actor, action, "notice_distribution", delivery.ID, requestID); err != nil {
		return true, err
	}
	if err := completionTx.Commit(ctx); err != nil {
		return true, err
	}
	return true, deliveryErr
}

type WebhookSender struct {
	URL    string
	Token  string
	Client *http.Client
}

func (s WebhookSender) Deliver(ctx context.Context, delivery Delivery) error {
	if s.URL == "" || s.Token == "" {
		return errors.New("notice delivery webhook is not configured")
	}
	payload, err := json.Marshal(delivery)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, s.URL, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+s.Token)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", delivery.ID.String())
	client := s.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("delivery webhook returned HTTP %d", response.StatusCode)
	}
	return nil
}
