package operationsinbox

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"math"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"henukit.dev/platform-core/internal/store"
)

var (
	ErrInvalid             = errors.New("invalid operations inbox request")
	ErrNotFound            = errors.New("operations inbox item not found")
	ErrConflict            = errors.New("operations inbox state conflict")
	ErrIdempotencyConflict = errors.New("idempotency key conflicts with another request")
	productPattern         = regexp.MustCompile(`^[a-z][a-z0-9-]{1,63}$`)
	resourcePattern        = regexp.MustCompile(`^[a-z][a-z0-9_-]{1,63}$`)
)

type Service struct {
	queries  *store.Queries
	database *pgxpool.Pool
}

type Item struct {
	ID                 string     `json:"id"`
	SourceProductCode  string     `json:"source_product_code"`
	SourceResourceType string     `json:"source_resource_type"`
	SourceResourceID   string     `json:"source_resource_id"`
	SourceResourceURL  *string    `json:"source_resource_url,omitempty"`
	OwnerUserID        *string    `json:"owner_user_id,omitempty"`
	Priority           string     `json:"priority"`
	SLADueAt           *time.Time `json:"sla_due_at,omitempty"`
	Status             string     `json:"status"`
	Version            int64      `json:"version"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

type CreateInput struct {
	ServiceID          string
	ActorUserID        string
	RequestID          string
	IdempotencyKey     string
	RequestHash        []byte
	SourceProductCode  string
	SourceResourceType string
	SourceResourceID   string
	SourceResourceURL  *string
	OwnerUserID        *string
	Priority           string
	SLADueAt           *time.Time
	Status             string
}

type UpdateInput struct {
	ServiceID       string
	ItemID          string
	ActorUserID     string
	RequestID       string
	IdempotencyKey  string
	RequestHash     []byte
	ExpectedVersion int64
	OwnerUserID     *string
	ClearOwner      bool
	Priority        *string
	SLADueAt        *time.Time
	ClearSLA        bool
	Status          *string
}

func New(queries *store.Queries, database *pgxpool.Pool) *Service {
	return &Service{queries: queries, database: database}
}

func (s *Service) Get(ctx context.Context, itemID string) (Item, error) {
	id, err := parseUUID(itemID)
	if err != nil {
		return Item{}, ErrInvalid
	}
	row, err := s.queries.GetOperationsInboxItem(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Item{}, ErrNotFound
	}
	return itemFromRow(row), err
}

func (s *Service) List(ctx context.Context, productCode, status string, page, pageSize int) ([]Item, bool, error) {
	if !productPattern.MatchString(productCode) || status != "" && !validStatus(status) || page < 1 || pageSize < 1 || pageSize > 100 {
		return nil, false, ErrInvalid
	}
	if page-1 > math.MaxInt32/pageSize {
		return nil, false, ErrInvalid
	}
	offset := (page - 1) * pageSize
	rows, err := s.queries.ListOperationsInboxItems(ctx, store.ListOperationsInboxItemsParams{
		SourceProductCode: productCode, Status: nullableText(status),
		PageOffset: int32(offset), PageSize: int32(pageSize + 1),
	})
	if err != nil {
		return nil, false, err
	}
	hasMore := len(rows) > pageSize
	if hasMore {
		rows = rows[:pageSize]
	}
	items := make([]Item, 0, len(rows))
	for _, row := range rows {
		items = append(items, itemFromRow(row))
	}
	return items, hasMore, nil
}

func (s *Service) OperationStatus(ctx context.Context, serviceID, actorUserID, operation, idempotencyKey string) (Item, bool, error) {
	actorID, err := parseUUID(actorUserID)
	if err != nil || serviceID == "" || operation != "create" && operation != "update" || len(idempotencyKey) < 8 || len(idempotencyKey) > 200 {
		return Item{}, false, ErrInvalid
	}
	payload, err := s.queries.GetOperationsInboxOperationStatus(ctx, store.GetOperationsInboxOperationStatusParams{
		ServiceID: serviceID, ActorUserID: actorID, Operation: operation, IdempotencyKey: idempotencyKey,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Item{}, false, nil
	}
	if err != nil {
		return Item{}, false, err
	}
	var item Item
	if err := json.Unmarshal(payload, &item); err != nil {
		return Item{}, false, err
	}
	return item, true, nil
}

func (s *Service) Create(ctx context.Context, input CreateInput) (Item, error) {
	actorID, err := parseUUID(input.ActorUserID)
	if err != nil || input.ServiceID == "" || !validWriteEnvelope(input.RequestID, input.IdempotencyKey, input.RequestHash) || !validReference(input.SourceProductCode, input.SourceResourceType, input.SourceResourceID, input.SourceResourceURL) || !validPriority(input.Priority) || input.Status != "" && !validCreateStatus(input.Status) {
		return Item{}, ErrInvalid
	}
	ownerID, err := optionalUUID(input.OwnerUserID)
	if err != nil {
		return Item{}, ErrInvalid
	}
	if input.Status == "" {
		input.Status = "open"
	}
	return s.write(ctx, input.ServiceID, actorID, "create", input.IdempotencyKey, input.RequestHash, func(queries *store.Queries) (store.OperationsInboxItem, error) {
		row, createErr := queries.CreateOperationsInboxItem(ctx, store.CreateOperationsInboxItemParams{
			SourceProductCode: input.SourceProductCode, SourceResourceType: input.SourceResourceType,
			SourceResourceID: input.SourceResourceID, SourceResourceUrl: nullableString(input.SourceResourceURL),
			OwnerUserID: ownerID, Priority: input.Priority, SlaDueAt: nullableTime(input.SLADueAt),
			Status: input.Status, ActorUserID: actorID,
		})
		if createErr != nil {
			return store.OperationsInboxItem{}, classifyWriteError(createErr)
		}
		snapshot, snapshotErr := json.Marshal(itemFromRow(row))
		if snapshotErr != nil {
			return store.OperationsInboxItem{}, snapshotErr
		}
		if auditErr := queries.CreateOperationsInboxAudit(ctx, store.CreateOperationsInboxAuditParams{
			ItemID: row.ID, ActorUserID: actorID, RequestID: input.RequestID,
			Action: "created", ToVersion: 1, ItemSnapshot: snapshot,
		}); auditErr != nil {
			return store.OperationsInboxItem{}, auditErr
		}
		return row, nil
	})
}

func (s *Service) Update(ctx context.Context, input UpdateInput) (Item, error) {
	actorID, err := parseUUID(input.ActorUserID)
	if err != nil || input.ServiceID == "" || !validWriteEnvelope(input.RequestID, input.IdempotencyKey, input.RequestHash) || input.ExpectedVersion < 1 || input.ClearOwner && input.OwnerUserID != nil || input.ClearSLA && input.SLADueAt != nil || input.OwnerUserID == nil && !input.ClearOwner && input.Priority == nil && input.SLADueAt == nil && !input.ClearSLA && input.Status == nil {
		return Item{}, ErrInvalid
	}
	itemID, err := parseUUID(input.ItemID)
	if err != nil {
		return Item{}, ErrInvalid
	}
	ownerID, err := optionalUUID(input.OwnerUserID)
	if err != nil || input.Priority != nil && !validPriority(*input.Priority) || input.Status != nil && !validStatus(*input.Status) {
		return Item{}, ErrInvalid
	}
	return s.write(ctx, input.ServiceID, actorID, "update", input.IdempotencyKey, input.RequestHash, func(queries *store.Queries) (store.OperationsInboxItem, error) {
		row, updateErr := queries.UpdateOperationsInboxItem(ctx, store.UpdateOperationsInboxItemParams{
			SetOwner: input.ClearOwner || input.OwnerUserID != nil, OwnerUserID: ownerID,
			Priority: nullableString(input.Priority), SetSla: input.ClearSLA || input.SLADueAt != nil,
			SlaDueAt: nullableTime(input.SLADueAt), Status: nullableString(input.Status),
			ActorUserID: actorID, ID: itemID, ExpectedVersion: input.ExpectedVersion,
		})
		if errors.Is(updateErr, pgx.ErrNoRows) {
			if _, getErr := queries.GetOperationsInboxItem(ctx, itemID); errors.Is(getErr, pgx.ErrNoRows) {
				return store.OperationsInboxItem{}, ErrNotFound
			} else if getErr != nil {
				return store.OperationsInboxItem{}, getErr
			}
			return store.OperationsInboxItem{}, ErrConflict
		}
		if updateErr != nil {
			return store.OperationsInboxItem{}, classifyWriteError(updateErr)
		}
		snapshot, snapshotErr := json.Marshal(itemFromRow(row))
		if snapshotErr != nil {
			return store.OperationsInboxItem{}, snapshotErr
		}
		if auditErr := queries.CreateOperationsInboxAudit(ctx, store.CreateOperationsInboxAuditParams{
			ItemID: row.ID, ActorUserID: actorID, RequestID: input.RequestID,
			Action: "updated", FromVersion: pgtype.Int8{Int64: input.ExpectedVersion, Valid: true}, ToVersion: row.Version, ItemSnapshot: snapshot,
		}); auditErr != nil {
			return store.OperationsInboxItem{}, auditErr
		}
		return row, nil
	})
}

func (s *Service) write(ctx context.Context, serviceID string, actorID pgtype.UUID, operation, key string, requestHash []byte, mutate func(*store.Queries) (store.OperationsInboxItem, error)) (Item, error) {
	tx, err := s.database.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Item{}, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	queries := s.queries.WithTx(tx)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, serviceID+"\n"+uuidString(actorID)+"\n"+operation+"\n"+key); err != nil {
		return Item{}, err
	}
	cached, err := queries.GetOperationsInboxIdempotency(ctx, store.GetOperationsInboxIdempotencyParams{ServiceID: serviceID, ActorUserID: actorID, Operation: operation, IdempotencyKey: key})
	if err == nil {
		if !bytes.Equal(cached.RequestHash, requestHash) {
			return Item{}, ErrIdempotencyConflict
		}
		var item Item
		if unmarshalErr := json.Unmarshal(cached.ResponsePayload, &item); unmarshalErr != nil {
			return Item{}, unmarshalErr
		}
		if commitErr := tx.Commit(ctx); commitErr != nil {
			return Item{}, commitErr
		}
		return item, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Item{}, err
	}
	row, err := mutate(queries)
	if err != nil {
		return Item{}, err
	}
	item := itemFromRow(row)
	payload, err := json.Marshal(item)
	if err != nil {
		return Item{}, err
	}
	if err := queries.CreateOperationsInboxIdempotency(ctx, store.CreateOperationsInboxIdempotencyParams{
		ServiceID: serviceID, ActorUserID: actorID, Operation: operation, IdempotencyKey: key, RequestHash: requestHash,
		ItemID: row.ID, ResponseVersion: row.Version, ResponsePayload: payload,
	}); err != nil {
		return Item{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Item{}, err
	}
	return item, nil
}

func itemFromRow(row store.OperationsInboxItem) Item {
	item := Item{
		ID: uuidString(row.ID), SourceProductCode: row.SourceProductCode,
		SourceResourceType: row.SourceResourceType, SourceResourceID: row.SourceResourceID,
		Priority: row.Priority, Status: row.Status, Version: row.Version,
		CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
	if row.SourceResourceUrl.Valid {
		item.SourceResourceURL = &row.SourceResourceUrl.String
	}
	if row.OwnerUserID.Valid {
		value := uuidString(row.OwnerUserID)
		item.OwnerUserID = &value
	}
	if row.SlaDueAt.Valid {
		value := row.SlaDueAt.Time
		item.SLADueAt = &value
	}
	return item
}

func validWriteEnvelope(requestID, key string, hash []byte) bool {
	return strings.HasPrefix(requestID, "req_") && len(key) >= 8 && len(key) <= 200 && len(hash) == sha256.Size
}

func validReference(product, resourceType, resourceID string, resourceURL *string) bool {
	if !productPattern.MatchString(product) || !resourcePattern.MatchString(resourceType) || len(resourceID) < 1 || len(resourceID) > 200 {
		return false
	}
	if resourceURL == nil {
		return true
	}
	parsed, err := url.ParseRequestURI(*resourceURL)
	return err == nil && (parsed.Scheme == "https" || parsed.Scheme == "http") && parsed.Host != "" && len(*resourceURL) <= 1000
}

func validPriority(value string) bool {
	return value == "low" || value == "normal" || value == "high" || value == "urgent"
}
func validCreateStatus(value string) bool {
	return value == "open" || value == "in_progress" || value == "blocked"
}
func validStatus(value string) bool {
	return validCreateStatus(value) || value == "resolved" || value == "cancelled"
}

func parseUUID(value string) (pgtype.UUID, error) {
	parsed, err := uuid.Parse(value)
	return pgtype.UUID{Bytes: parsed, Valid: err == nil}, err
}

func optionalUUID(value *string) (pgtype.UUID, error) {
	if value == nil {
		return pgtype.UUID{}, nil
	}
	return parseUUID(*value)
}

func nullableString(value *string) pgtype.Text {
	if value == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *value, Valid: true}
}

func nullableText(value string) pgtype.Text { return pgtype.Text{String: value, Valid: value != ""} }

func nullableTime(value *time.Time) pgtype.Timestamptz {
	if value == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: value.UTC(), Valid: true}
}

func classifyWriteError(err error) error {
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) {
		switch postgresError.Code {
		case "23503", "23514", "22P02":
			return ErrInvalid
		case "23505":
			return ErrConflict
		}
	}
	return err
}

func uuidString(value pgtype.UUID) string {
	if !value.Valid {
		return ""
	}
	return uuid.UUID(value.Bytes).String()
}
