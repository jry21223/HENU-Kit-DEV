package mailworker

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"henukit.dev/platform-core/internal/securebox"
	"henukit.dev/platform-core/internal/store"
)

type Message struct {
	IdempotencyKey string
	Recipient      string
	Code           string
	Purpose        string
	ExpiresAt      time.Time
	RequestID      string
}

type Sender interface {
	Send(context.Context, Message) (string, error)
}

type SendError struct {
	Code      string
	Permanent bool
	Err       error
}

func (e *SendError) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	return e.Code
}

type Worker struct {
	queries        *store.Queries
	sender         Sender
	workerID       string
	recipientCodec *securebox.Codec
	payloadCodec   *securebox.Codec
	leaseTimeout   time.Duration
	sendTimeout    time.Duration
	now            func() time.Time
}

type payload struct {
	Code      string    `json:"code"`
	Purpose   string    `json:"purpose"`
	ExpiresAt time.Time `json:"expires_at"`
}

func New(queries *store.Queries, sender Sender, workerID string, masterKey []byte, leaseTimeout, sendTimeout time.Duration) (*Worker, error) {
	if queries == nil || sender == nil || workerID == "" || len(masterKey) != 32 || leaseTimeout <= 0 || sendTimeout <= 0 {
		return nil, errors.New("mail worker dependencies and positive timeouts are required")
	}
	recipientCodec, err := securebox.New(masterKey, "verification-recipient")
	if err != nil {
		return nil, err
	}
	payloadCodec, err := securebox.New(masterKey, "verification-payload")
	if err != nil {
		return nil, err
	}
	return &Worker{
		queries: queries, sender: sender, workerID: workerID,
		recipientCodec: recipientCodec, payloadCodec: payloadCodec,
		leaseTimeout: leaseTimeout, sendTimeout: sendTimeout, now: func() time.Time { return time.Now().UTC() },
	}, nil
}

func (w *Worker) ProcessOne(ctx context.Context) (bool, error) {
	reclaimBefore := w.now().Add(-w.leaseTimeout)
	if err := w.queries.FailExhaustedOutboxLeases(ctx, pgtype.Timestamptz{Time: reclaimBefore, Valid: true}); err != nil {
		return false, err
	}
	job, err := w.queries.ClaimMailOutbox(ctx, store.ClaimMailOutboxParams{
		WorkerID:      pgtype.Text{String: w.workerID, Valid: true},
		ReclaimBefore: pgtype.Timestamptz{Time: reclaimBefore, Valid: true},
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	recipient, err := w.recipientCodec.Open(job.RecipientCiphertext)
	if err != nil {
		return true, w.fail(ctx, job.ID, "PAYLOAD_INVALID")
	}
	payloadBytes, err := w.payloadCodec.Open(job.PayloadCiphertext)
	if err != nil {
		return true, w.fail(ctx, job.ID, "PAYLOAD_INVALID")
	}
	var content payload
	if err := json.Unmarshal(payloadBytes, &content); err != nil || content.Code == "" || content.Purpose == "" || content.ExpiresAt.IsZero() {
		return true, w.fail(ctx, job.ID, "PAYLOAD_INVALID")
	}
	sendContext, cancel := context.WithTimeout(ctx, w.sendTimeout)
	providerMessageID, sendErr := w.sender.Send(sendContext, Message{
		IdempotencyKey: job.DedupeKey,
		Recipient:      string(recipient), Code: content.Code, Purpose: content.Purpose,
		ExpiresAt: content.ExpiresAt, RequestID: job.RequestID,
	})
	cancel()
	if sendErr == nil && providerMessageID != "" {
		rows, err := w.queries.AcceptMailOutbox(ctx, store.AcceptMailOutboxParams{
			ID: job.ID, LockedBy: pgtype.Text{String: w.workerID, Valid: true},
			ProviderMessageID: pgtype.Text{String: providerMessageID, Valid: true},
		})
		if err != nil {
			return true, err
		}
		if rows != 1 {
			return true, errors.New("mail outbox lease was lost before acceptance")
		}
		return true, nil
	}
	code, permanent := "PROVIDER_UNAVAILABLE", false
	var classified *SendError
	if errors.As(sendErr, &classified) {
		code, permanent = safeErrorCode(classified.Code), classified.Permanent
	} else if errors.Is(sendErr, context.DeadlineExceeded) || errors.Is(sendContext.Err(), context.DeadlineExceeded) {
		code = "SEND_TIMEOUT"
	}
	if permanent || job.AttemptCount >= job.MaxAttempts {
		return true, w.fail(ctx, job.ID, code)
	}
	retryAt := w.now().Add(retryDelay(job.AttemptCount))
	rows, err := w.queries.RetryMailOutbox(ctx, store.RetryMailOutboxParams{
		ID: job.ID, LockedBy: pgtype.Text{String: w.workerID, Valid: true},
		AvailableAt:   pgtype.Timestamptz{Time: retryAt, Valid: true},
		LastErrorCode: pgtype.Text{String: code, Valid: true},
	})
	if err != nil {
		return true, err
	}
	if rows != 1 {
		return true, errors.New("mail outbox lease was lost before retry")
	}
	return true, nil
}

func (w *Worker) MarkDelivered(ctx context.Context, providerMessageID string) (bool, error) {
	if providerMessageID == "" {
		return false, errors.New("provider message id is required")
	}
	rows, err := w.queries.MarkMailOutboxDelivered(ctx, pgtype.Text{String: providerMessageID, Valid: true})
	return rows == 1, err
}

func (w *Worker) fail(ctx context.Context, id pgtype.UUID, code string) error {
	rows, err := w.queries.FailMailOutbox(ctx, store.FailMailOutboxParams{
		ID: id, LockedBy: pgtype.Text{String: w.workerID, Valid: true},
		LastErrorCode: pgtype.Text{String: safeErrorCode(code), Valid: true},
	})
	if err != nil {
		return err
	}
	if rows != 1 {
		return errors.New("mail outbox lease was lost before failure")
	}
	return nil
}

func retryDelay(attempt int32) time.Duration {
	delay := time.Minute
	for current := int32(1); current < attempt && delay < time.Hour; current++ {
		delay *= 2
	}
	return min(delay, time.Hour)
}

func safeErrorCode(value string) string {
	if value == "" || len(value) > 80 {
		return "PROVIDER_UNAVAILABLE"
	}
	for _, character := range value {
		if !(character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || character == '_') {
			return "PROVIDER_UNAVAILABLE"
		}
	}
	return value
}
