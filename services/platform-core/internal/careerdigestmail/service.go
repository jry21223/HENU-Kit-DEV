// Package careerdigestmail owns the #397 mail boundary between Career
// Opportunities and the encrypted mail Outbox: it resolves the recipient from
// the Platform user's verified Email Identity, seals a browser-safe digest
// summary, and enqueues a career_digest Outbox row. The caller never sees the
// email or any ciphertext, and a replayed enqueue for the same search is an
// idempotent no-op (the UNIQUE dedupe key wins).
package careerdigestmail

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"henukit.dev/platform-core/internal/securebox"
	"henukit.dev/platform-core/internal/store"
	"henukit.dev/platform-core/internal/verificationmail"
)

// ErrNoVerifiedEmail is the fail-closed outcome when the user has no verified
// Email Identity; no mail is ever enqueued for an unverified recipient.
var ErrNoVerifiedEmail = errors.New("user has no verified email identity")

// EnqueueInput is the bounded, browser-safe digest summary of one completed
// Career search. The recipient is never part of the input: Platform Core
// resolves it from the user's verified Email Identity.
type EnqueueInput struct {
	UserID       uuid.UUID
	RequestID    string
	SearchID     string
	CompletedAt  string
	SourceCount  int
	JobCount     int
	MatchedCount int
	Summary      string
	CareerURL    string
	TopJobs      []verificationmail.Job
}

// EnqueueResult reports a fresh enqueue or an idempotent replay.
type EnqueueResult struct {
	OutboxID string
	Replayed bool
}

// Service resolves, seals, and enqueues Career digests.
type Service struct {
	queries     *store.Queries
	emailCodec  *securebox.Codec
	digestCodec *verificationmail.DigestCodec
}

// New builds the digest mail service under the platform-core verification key,
// the same key that seals Email Identities and the digest envelope.
func New(queries *store.Queries, verificationKey []byte) (*Service, error) {
	if queries == nil || len(verificationKey) != 32 {
		return nil, errors.New("career digest mail service requires queries and a 32-byte verification key")
	}
	emailCodec, err := securebox.New(verificationKey, "email-identity")
	if err != nil {
		return nil, err
	}
	digestCodec, err := verificationmail.NewDigestCodec(verificationKey)
	if err != nil {
		return nil, err
	}
	return &Service{queries: queries, emailCodec: emailCodec, digestCodec: digestCodec}, nil
}

// Enqueue resolves the user's verified email, seals the digest under the
// dedicated digest labels, and inserts one career_digest Outbox row keyed by
// career_search_completed:{search_id}. A UNIQUE dedupe conflict is treated as
// an idempotent replay, never as a duplicate row.
func (s *Service) Enqueue(ctx context.Context, input EnqueueInput) (EnqueueResult, error) {
	if input.UserID == uuid.Nil || input.SearchID == "" || input.RequestID == "" {
		return EnqueueResult{}, errors.New("career digest enqueue input is incomplete")
	}
	identity, err := s.queries.GetVerifiedEmailByUserID(ctx, pgtype.UUID{Bytes: input.UserID, Valid: true})
	if errors.Is(err, pgx.ErrNoRows) {
		return EnqueueResult{}, ErrNoVerifiedEmail
	}
	if err != nil {
		return EnqueueResult{}, err
	}
	if !identity.EmailVerified || len(identity.EmailCiphertext) == 0 {
		return EnqueueResult{}, ErrNoVerifiedEmail
	}
	email, err := s.emailCodec.Open(identity.EmailCiphertext)
	if err != nil {
		return EnqueueResult{}, err
	}
	digest := verificationmail.CareerDigest{
		SearchID: input.SearchID, CompletedAt: input.CompletedAt,
		SourceCount: input.SourceCount, JobCount: input.JobCount,
		MatchedCount: input.MatchedCount, Summary: input.Summary,
		CareerURL: input.CareerURL, TopJobs: input.TopJobs,
	}
	recipientCiphertext, payloadCiphertext, err := s.digestCodec.Encode(string(email), digest)
	if err != nil {
		return EnqueueResult{}, err
	}
	id, err := s.queries.CreateCareerDigestMailOutbox(ctx, store.CreateCareerDigestMailOutboxParams{
		DedupeKey:           "career_search_completed:" + input.SearchID,
		RequestID:           input.RequestID,
		RecipientUserID:     pgtype.UUID{Bytes: input.UserID, Valid: true},
		RecipientCiphertext: recipientCiphertext,
		PayloadCiphertext:   payloadCiphertext,
	})
	if err != nil {
		var postgresError *pgconn.PgError
		if errors.As(err, &postgresError) && postgresError.Code == "23505" {
			return EnqueueResult{Replayed: true}, nil
		}
		return EnqueueResult{}, err
	}
	return EnqueueResult{OutboxID: id.String()}, nil
}
