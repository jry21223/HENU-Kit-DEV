package mailworker

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"henukit.dev/platform-core/internal/store"
	"henukit.dev/platform-core/internal/verificationmail"
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
	queries           *store.Queries
	sender            Sender
	workerID          string
	mailCodec         *verificationmail.Codec
	leaseTimeout      time.Duration
	sendTimeout       time.Duration
	now               func() time.Time
	nextReconcile     time.Time
	reconcileInterval time.Duration
}

type Outcome struct {
	Processed    bool
	OutboxID     string
	RequestID    string
	Result       string
	ErrorCode    string
	AttemptCount int32
	Duration     time.Duration
}

func New(queries *store.Queries, sender Sender, workerID string, masterKey []byte, leaseTimeout, sendTimeout time.Duration) (*Worker, error) {
	if queries == nil || sender == nil || workerID == "" || len(masterKey) != 32 || leaseTimeout <= 0 || sendTimeout <= 0 {
		return nil, errors.New("mail worker dependencies and positive timeouts are required")
	}
	mailCodec, err := verificationmail.NewCodec(masterKey)
	if err != nil {
		return nil, err
	}
	return &Worker{
		queries: queries, sender: sender, workerID: workerID,
		mailCodec:    mailCodec,
		leaseTimeout: leaseTimeout, sendTimeout: sendTimeout, now: func() time.Time { return time.Now().UTC() },
		reconcileInterval: 5 * time.Second,
	}, nil
}

func (w *Worker) ProcessOne(ctx context.Context) (outcome Outcome, err error) {
	startedAt := w.now()
	defer func() { outcome.Duration = w.now().Sub(startedAt) }()
	if !w.now().Before(w.nextReconcile) {
		if err := w.queries.ApplyAllPendingMailDeliveryReceipts(ctx); err != nil {
			return outcome, err
		}
		w.nextReconcile = w.now().Add(w.reconcileInterval)
	}
	reclaimBefore := w.now().Add(-w.leaseTimeout)
	if err := w.queries.FailExhaustedOutboxLeases(ctx, pgtype.Timestamptz{Time: reclaimBefore, Valid: true}); err != nil {
		return outcome, err
	}
	job, err := w.queries.ClaimMailOutbox(ctx, store.ClaimMailOutboxParams{
		WorkerID:      pgtype.Text{String: w.workerID, Valid: true},
		ReclaimBefore: pgtype.Timestamptz{Time: reclaimBefore, Valid: true},
	})
	if errors.Is(err, pgx.ErrNoRows) {
		outcome.Result = "no_work"
		return outcome, nil
	}
	if err != nil {
		return outcome, err
	}
	outcome.Processed = true
	outcome.OutboxID = uuidString(job.ID)
	outcome.RequestID = job.RequestID
	outcome.AttemptCount = job.AttemptCount
	recipient, content, err := w.mailCodec.Decode(job.RecipientCiphertext, job.PayloadCiphertext)
	if err != nil {
		outcome.Result, outcome.ErrorCode = "failed", "PAYLOAD_INVALID"
		return outcome, w.fail(ctx, job.ID, outcome.ErrorCode)
	}
	if !w.now().Before(content.ExpiresAt) {
		outcome.Result, outcome.ErrorCode = "failed", "VERIFICATION_EXPIRED"
		return outcome, w.fail(ctx, job.ID, outcome.ErrorCode)
	}
	sendContext, cancel := context.WithTimeout(ctx, w.sendTimeout)
	providerMessageID, sendErr := w.sender.Send(sendContext, Message{
		IdempotencyKey: job.DedupeKey,
		Recipient:      recipient, Code: content.Code, Purpose: content.Purpose,
		ExpiresAt: content.ExpiresAt, RequestID: job.RequestID,
	})
	cancel()
	if sendErr == nil && providerMessageID != "" {
		rows, err := w.queries.AcceptMailOutbox(ctx, store.AcceptMailOutboxParams{
			OutboxID: job.ID, WorkerID: w.workerID,
			ProviderMessageID: pgtype.Text{String: providerMessageID, Valid: true},
		})
		if err != nil {
			return outcome, err
		}
		if rows != 1 {
			return outcome, errors.New("mail outbox lease was lost before acceptance")
		}
		if _, err := w.queries.ApplyPendingMailDeliveryReceipt(ctx, pgtype.Text{String: providerMessageID, Valid: true}); err != nil {
			return outcome, err
		}
		outcome.Result = "accepted"
		return outcome, nil
	}
	code, permanent := "PROVIDER_UNAVAILABLE", false
	var classified *SendError
	if errors.As(sendErr, &classified) {
		code, permanent = safeErrorCode(classified.Code), classified.Permanent
	} else if errors.Is(sendErr, context.DeadlineExceeded) || errors.Is(sendContext.Err(), context.DeadlineExceeded) {
		code = "SEND_TIMEOUT"
	}
	if permanent || job.AttemptCount >= job.MaxAttempts {
		outcome.Result, outcome.ErrorCode = "failed", code
		return outcome, w.fail(ctx, job.ID, code)
	}
	retryAt := w.now().Add(retryDelay(job.AttemptCount))
	rows, err := w.queries.RetryMailOutbox(ctx, store.RetryMailOutboxParams{
		OutboxID: job.ID, WorkerID: w.workerID,
		AvailableAt:   pgtype.Timestamptz{Time: retryAt, Valid: true},
		LastErrorCode: pgtype.Text{String: code, Valid: true},
	})
	if err != nil {
		return outcome, err
	}
	if rows != 1 {
		return outcome, errors.New("mail outbox lease was lost before retry")
	}
	outcome.Result, outcome.ErrorCode = "retry_due", code
	return outcome, nil
}

func (w *Worker) MarkDelivered(ctx context.Context, providerMessageID, requestID, actorID string) (bool, error) {
	if providerMessageID == "" || requestID == "" || actorID == "" {
		return false, errors.New("provider message id, request id, and actor id are required")
	}
	rows, err := w.queries.MarkMailOutboxDelivered(ctx, store.MarkMailOutboxDeliveredParams{
		ProviderMessageID: pgtype.Text{String: providerMessageID, Valid: true},
		RequestID:         requestID,
		ActorID:           actorID,
	})
	return rows == 1, err
}

func (w *Worker) Requeue(ctx context.Context, outboxID, requestID, actorID, reason string) error {
	return Requeue(ctx, w.queries, outboxID, requestID, actorID, reason)
}

func Requeue(ctx context.Context, queries *store.Queries, outboxID, requestID, actorID, reason string) error {
	parsedID, err := uuid.Parse(outboxID)
	if queries == nil || err != nil || requestID == "" || actorID == "" || reason == "" {
		return errors.New("valid outbox id, request id, actor id, and reason are required")
	}
	_, err = queries.RequeueMailOutbox(ctx, store.RequeueMailOutboxParams{
		OutboxID:  pgtype.UUID{Bytes: parsedID, Valid: true},
		RequestID: requestID,
		ActorID:   pgtype.Text{String: actorID, Valid: true},
		Reason:    pgtype.Text{String: reason, Valid: true},
	})
	return err
}

func (w *Worker) fail(ctx context.Context, id pgtype.UUID, code string) error {
	rows, err := w.queries.FailMailOutbox(ctx, store.FailMailOutboxParams{
		OutboxID: id, WorkerID: w.workerID,
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

func uuidString(value pgtype.UUID) string {
	if !value.Valid {
		return ""
	}
	return uuid.UUID(value.Bytes).String()
}
