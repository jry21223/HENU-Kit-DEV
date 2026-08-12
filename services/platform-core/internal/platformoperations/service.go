package platformoperations

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"henukit.dev/platform-core/internal/coordination"
	"henukit.dev/platform-core/internal/securebox"
	"henukit.dev/platform-core/internal/store"
)

const membershipAccountPageSize = 20

type Service struct {
	queries         *store.Queries
	database        *pgxpool.Pool
	redis           *redis.Client
	verificationKey []byte
	allowedDomains  map[string]struct{}
	emailCodec      *securebox.Codec
}

var (
	ErrInvalid             = errors.New("invalid platform operation")
	ErrNotFound            = errors.New("platform operation resource not found")
	ErrConflict            = errors.New("platform operation state conflict")
	ErrRateLimited         = errors.New("platform operation rate limit exceeded")
	ErrDependency          = errors.New("platform operation dependency unavailable")
	ErrIdempotencyConflict = errors.New("platform operation idempotency conflict")
)

type Snapshot struct {
	Accounts     []Account    `json:"accounts"`
	Sessions     []Session    `json:"sessions"`
	Mail         MailStatus   `json:"mail"`
	InboxItems   []InboxItem  `json:"inbox_items"`
	Audit        []AuditEvent `json:"audit"`
	Dependencies Dependencies `json:"dependencies"`
	GeneratedAt  time.Time    `json:"generated_at"`
}

type Account struct {
	ID                    string        `json:"id"`
	DisplayName           *string       `json:"display_name,omitempty"`
	Email                 string        `json:"email"`
	EmailVerified         bool          `json:"email_verified"`
	Status                string        `json:"status"`
	AuthorizationRevision int64         `json:"authorization_revision"`
	CreatedAt             time.Time     `json:"created_at"`
	Grants                []AccessGrant `json:"grants"`
}

type AccessGrant struct {
	RoleCode string `json:"role_code"`
	Scope    Scope  `json:"scope"`
}

type Scope struct {
	Kind         string  `json:"kind"`
	ProductCode  *string `json:"product_code,omitempty"`
	ResourceType *string `json:"resource_type,omitempty"`
	ResourceID   *string `json:"resource_id,omitempty"`
}

type Session struct {
	ID          string     `json:"id"`
	UserID      string     `json:"user_id"`
	DisplayName *string    `json:"display_name,omitempty"`
	Email       string     `json:"email"`
	Kind        string     `json:"kind"`
	ClientID    *string    `json:"client_id,omitempty"`
	LastSeenAt  time.Time  `json:"last_seen_at"`
	ExpiresAt   time.Time  `json:"expires_at"`
	RevokedAt   *time.Time `json:"revoked_at,omitempty"`
}

type MailStatus struct {
	Pending     int64 `json:"pending"`
	Processing  int64 `json:"processing"`
	RetryDue    int64 `json:"retry_due"`
	Accepted    int64 `json:"accepted"`
	Delivered   int64 `json:"delivered"`
	Failed      int64 `json:"failed"`
	DeadLetters int64 `json:"dead_letters"`
}

type InboxItem struct {
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

type AuditEvent struct {
	RequestID          string    `json:"request_id"`
	ActorUserID        string    `json:"actor_user_id"`
	DisplayName        *string   `json:"display_name,omitempty"`
	Email              string    `json:"email"`
	PermissionCode     string    `json:"permission_code"`
	TargetKind         string    `json:"target_kind"`
	TargetProductCode  *string   `json:"target_product_code,omitempty"`
	TargetResourceType *string   `json:"target_resource_type,omitempty"`
	TargetResourceID   *string   `json:"target_resource_id,omitempty"`
	Decision           string    `json:"decision"`
	ReasonCode         string    `json:"reason_code"`
	CreatedAt          time.Time `json:"created_at"`
}

type Dependencies struct {
	Postgres string `json:"postgres"`
	Redis    string `json:"redis"`
}

type AccountLookup struct {
	Account *AccountLookupAccount `json:"account"`
}

type AccountLookupAccount struct {
	ID          string  `json:"id"`
	DisplayName *string `json:"display_name,omitempty"`
	Email       string  `json:"email"`
	Status      string  `json:"status"`
}

type IdentityResolution struct {
	Identities []AccountLookupAccount `json:"identities"`
}

type MembershipAccountPage struct {
	Accounts []MembershipAccount `json:"accounts"`
	NextPage *int                `json:"next_page"`
}

type MembershipAccount struct {
	ID          string  `json:"id"`
	DisplayName *string `json:"display_name,omitempty"`
	Email       string  `json:"email"`
	Status      string  `json:"status"`
}

type OperationResult struct {
	Operation       string `json:"operation"`
	Status          string `json:"status"`
	ResourceID      string `json:"resource_id,omitempty"`
	ResourceVersion int64  `json:"resource_version,omitempty"`
}

type WriteInput struct {
	ServiceID, ActorUserID, RequestID, IdempotencyKey string
	RequestHash                                       []byte
	ResourceID                                        string
}

type AccessUpdateInput struct {
	WriteInput
	ExpectedRevision int64
	Status           string
	Grants           []GrantInput
}

type GrantInput struct {
	RoleCode string
	Scope    ScopeInput
}

type ScopeInput struct {
	Kind, ProductCode, ResourceType, ResourceID string
}

func New(queries *store.Queries, database *pgxpool.Pool, redisClient *redis.Client, verificationKey []byte, allowedDomains []string) *Service {
	domains := make(map[string]struct{}, len(allowedDomains))
	for _, domain := range allowedDomains {
		domains[strings.ToLower(strings.TrimSpace(domain))] = struct{}{}
	}
	emailCodec, _ := securebox.New(verificationKey, "email-identity")
	return &Service{queries: queries, database: database, redis: redisClient, verificationKey: append([]byte(nil), verificationKey...), allowedDomains: domains, emailCodec: emailCodec}
}

func (s *Service) Snapshot(ctx context.Context) (Snapshot, error) {
	accountRows, err := s.queries.ListPlatformOperationAccounts(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	grantRows, err := s.queries.ListPlatformOperationAccountGrants(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	sessionRows, err := s.queries.ListPlatformOperationSessions(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	mail, err := s.queries.CountPlatformOperationMailStatuses(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	inboxRows, err := s.queries.ListPlatformOperationInboxItems(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	auditRows, err := s.queries.ListPlatformOperationAuditEvents(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	redisStatus := "ready"
	if err := s.redis.Ping(ctx).Err(); err != nil {
		redisStatus = "unavailable"
	}

	result := Snapshot{
		Accounts: make([]Account, 0, len(accountRows)), Sessions: make([]Session, 0, len(sessionRows)),
		InboxItems: make([]InboxItem, 0, len(inboxRows)), Audit: make([]AuditEvent, 0, len(auditRows)),
		Mail:         MailStatus{Pending: mail.Pending, Processing: mail.Processing, RetryDue: mail.RetryDue, Accepted: mail.Accepted, Delivered: mail.Delivered, Failed: mail.Failed, DeadLetters: mail.DeadLetters},
		Dependencies: Dependencies{Postgres: "ready", Redis: redisStatus}, GeneratedAt: time.Now().UTC(),
	}
	for _, row := range accountRows {
		email, err := s.openEmail(row.EmailCiphertext)
		if err != nil {
			return Snapshot{}, err
		}
		result.Accounts = append(result.Accounts, Account{ID: uuidString(row.ID), DisplayName: textPointer(row.DisplayName), Email: email, EmailVerified: row.EmailVerified, Status: row.Status, AuthorizationRevision: row.AuthorizationRevision, CreatedAt: row.CreatedAt.Time, Grants: []AccessGrant{}})
	}
	accountIndexes := make(map[string]int, len(result.Accounts))
	for index := range result.Accounts {
		accountIndexes[result.Accounts[index].ID] = index
	}
	for _, row := range grantRows {
		index, ok := accountIndexes[uuidString(row.UserID)]
		if !ok {
			continue
		}
		result.Accounts[index].Grants = append(result.Accounts[index].Grants, AccessGrant{RoleCode: row.RoleCode, Scope: Scope{Kind: row.ScopeKind, ProductCode: textPointer(row.ProductCode), ResourceType: textPointer(row.ResourceType), ResourceID: textPointer(row.ResourceID)}})
	}
	for _, row := range sessionRows {
		email, err := s.openOptionalEmail(row.EmailCiphertext)
		if err != nil {
			return Snapshot{}, err
		}
		result.Sessions = append(result.Sessions, Session{ID: uuidString(row.ID), UserID: uuidString(row.UserID), DisplayName: textPointer(row.DisplayName), Email: email, Kind: row.Kind, ClientID: textPointer(row.ClientID), LastSeenAt: row.LastSeenAt.Time, ExpiresAt: row.ExpiresAt.Time, RevokedAt: timePointer(row.RevokedAt)})
	}
	for _, row := range inboxRows {
		result.InboxItems = append(result.InboxItems, InboxItem{ID: uuidString(row.ID), SourceProductCode: row.SourceProductCode, SourceResourceType: row.SourceResourceType, SourceResourceID: row.SourceResourceID, SourceResourceURL: textPointer(row.SourceResourceUrl), OwnerUserID: uuidPointer(row.OwnerUserID), Priority: row.Priority, SLADueAt: timePointer(row.SlaDueAt), Status: row.Status, Version: row.Version, CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time})
	}
	for _, row := range auditRows {
		email, err := s.openOptionalEmail(row.EmailCiphertext)
		if err != nil {
			return Snapshot{}, err
		}
		result.Audit = append(result.Audit, AuditEvent{RequestID: row.RequestID, ActorUserID: uuidString(row.ActorUserID), DisplayName: textPointer(row.DisplayName), Email: email, PermissionCode: row.PermissionCode, TargetKind: row.TargetKind, TargetProductCode: textPointer(row.TargetProductCode), TargetResourceType: textPointer(row.TargetResourceType), TargetResourceID: textPointer(row.TargetResourceID), Decision: row.Decision, ReasonCode: row.ReasonCode, CreatedAt: row.CreatedAt.Time})
	}
	return result, nil
}

func (s *Service) LookupAccount(ctx context.Context, serviceID, email string) (AccountLookup, error) {
	if serviceID == "" || s.verificationKey == nil {
		return AccountLookup{}, ErrDependency
	}
	normalized, err := s.normalizeEmail(email)
	if err != nil {
		return AccountLookup{}, ErrInvalid
	}
	limited, err := s.rateLimited(ctx, serviceID)
	if err != nil {
		return AccountLookup{}, err
	}
	if limited {
		return AccountLookup{}, ErrRateLimited
	}
	hash := emailLookupHash(s.verificationKey, normalized)
	row, err := s.queries.GetPlatformOperationAccountByEmailLookupHash(ctx, hash)
	if errors.Is(err, pgx.ErrNoRows) {
		// A miss must cost a comparable index probe so the response time
		// cannot be used to tell whether an account exists (#242).
		var dummy [32]byte
		_, _ = rand.Read(dummy[:])
		_, _ = s.queries.GetPlatformOperationAccountByEmailLookupHash(ctx, dummy[:])
		return AccountLookup{Account: nil}, nil
	}
	if err != nil {
		return AccountLookup{}, err
	}
	resolvedEmail, err := s.openEmail(row.EmailCiphertext)
	if err != nil {
		return AccountLookup{}, err
	}
	return AccountLookup{Account: &AccountLookupAccount{ID: uuidString(row.ID), DisplayName: textPointer(row.DisplayName), Email: resolvedEmail, Status: row.Status}}, nil
}

func (s *Service) ResolveIdentities(ctx context.Context, userIDs []string) (IdentityResolution, error) {
	if len(userIDs) == 0 || len(userIDs) > 100 {
		return IdentityResolution{}, ErrInvalid
	}
	ids := make([]pgtype.UUID, 0, len(userIDs))
	seen := make(map[uuid.UUID]struct{}, len(userIDs))
	for _, rawID := range userIDs {
		id, err := uuid.Parse(rawID)
		if err != nil {
			return IdentityResolution{}, ErrInvalid
		}
		if _, exists := seen[id]; exists {
			return IdentityResolution{}, ErrInvalid
		}
		seen[id] = struct{}{}
		ids = append(ids, pgtype.UUID{Bytes: id, Valid: true})
	}
	rows, err := s.queries.ListConsoleUserIdentities(ctx, ids)
	if err != nil {
		return IdentityResolution{}, err
	}
	result := IdentityResolution{Identities: make([]AccountLookupAccount, 0, len(rows))}
	for _, row := range rows {
		email, err := s.openEmail(row.EmailCiphertext)
		if err != nil {
			return IdentityResolution{}, err
		}
		result.Identities = append(result.Identities, AccountLookupAccount{ID: uuidString(row.ID), DisplayName: textPointer(row.DisplayName), Email: email, Status: row.Status})
	}
	return result, nil
}

func (s *Service) openEmail(ciphertext []byte) (string, error) {
	if s.emailCodec == nil {
		return "", ErrDependency
	}
	plaintext, err := s.emailCodec.Open(ciphertext)
	if err != nil || len(plaintext) == 0 {
		return "", ErrDependency
	}
	return string(plaintext), nil
}

func (s *Service) openOptionalEmail(ciphertext []byte) (string, error) {
	if len(ciphertext) == 0 {
		return "", nil
	}
	return s.openEmail(ciphertext)
}

func (s *Service) ListMembershipAccounts(ctx context.Context, query string, page int) (MembershipAccountPage, error) {
	query = strings.TrimSpace(query)
	if len([]rune(query)) > 100 || page < 1 || page > 10_000 {
		return MembershipAccountPage{}, ErrInvalid
	}
	result := MembershipAccountPage{Accounts: []MembershipAccount{}}
	var exactEmailHash []byte
	if normalized, normalizeErr := s.normalizeEmail(query); normalizeErr == nil {
		exactEmailHash = emailLookupHash(s.verificationKey, normalized)
	}
	rows, err := s.queries.ListPlatformOperationMembershipAccounts(ctx, store.ListPlatformOperationMembershipAccountsParams{
		Search: query, EmailLookupHash: exactEmailHash, PageLimit: membershipAccountPageSize + 1, PageOffset: int32((page - 1) * membershipAccountPageSize),
	})
	if err != nil {
		return MembershipAccountPage{}, err
	}
	if len(rows) > membershipAccountPageSize {
		next := page + 1
		result.NextPage = &next
		rows = rows[:membershipAccountPageSize]
	}
	for _, row := range rows {
		email, openErr := s.openEmail(row.EmailCiphertext)
		if openErr != nil {
			return MembershipAccountPage{}, openErr
		}
		account := MembershipAccount{ID: uuidString(row.ID), DisplayName: textPointer(row.DisplayName), Email: email, Status: row.Status}
		result.Accounts = append(result.Accounts, account)
	}
	return result, nil
}

func (s *Service) rateLimited(ctx context.Context, serviceID string) (bool, error) {
	coordinator := coordination.NewRedis(s.redis)
	dimensions := []struct {
		key    string
		limit  int64
		window time.Duration
	}{
		{key: "account-lookup-minute:" + serviceID, limit: 30, window: time.Minute},
		{key: "account-lookup-hour:" + serviceID, limit: 300, window: time.Hour},
	}
	limited := false
	for _, dimension := range dimensions {
		allowed, err := coordinator.Allow(ctx, "platform-core:"+dimension.key, dimension.limit, dimension.window)
		if err != nil {
			return false, ErrDependency
		}
		limited = limited || !allowed
	}
	return limited, nil
}

func (s *Service) normalizeEmail(value string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	parsed, err := mail.ParseAddress(normalized)
	if err != nil || parsed.Address != normalized || strings.Count(normalized, "@") != 1 {
		return "", ErrInvalid
	}
	domain := normalized[strings.LastIndexByte(normalized, '@')+1:]
	if _, allowed := s.allowedDomains[domain]; !allowed {
		return "", ErrInvalid
	}
	return normalized, nil
}

func emailLookupHash(key []byte, email string) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte("henukit-verification:email"))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(email))
	return mac.Sum(nil)
}

func (s *Service) RevokeSession(ctx context.Context, input WriteInput) (OperationResult, error) {
	if !validWriteInput(input) {
		return OperationResult{}, ErrInvalid
	}
	resourceID, err := uuid.Parse(input.ResourceID)
	if err != nil {
		return OperationResult{}, ErrInvalid
	}
	resourceUUID := pgtype.UUID{Bytes: resourceID, Valid: true}
	return s.write(ctx, input, "session_revoke", "session", resourceUUID, func(queries *store.Queries) (int64, error) {
		_, err := queries.RevokePlatformOperationSession(ctx, resourceUUID)
		if errors.Is(err, pgx.ErrNoRows) {
			session, getErr := queries.GetPlatformOperationSession(ctx, resourceUUID)
			if errors.Is(getErr, pgx.ErrNoRows) {
				return 0, ErrNotFound
			}
			if getErr != nil {
				return 0, getErr
			}
			if session.RevokedAt.Valid || session.ExpiresAt.Time.Before(time.Now()) {
				return 0, ErrConflict
			}
			return 0, ErrConflict
		}
		return 0, err
	})
}

func (s *Service) UpdateAccess(ctx context.Context, input AccessUpdateInput) (OperationResult, error) {
	if !validWriteInput(input.WriteInput) || input.ExpectedRevision < 1 || !validUserStatus(input.Status) || !validGrants(input.Grants) {
		return OperationResult{}, ErrInvalid
	}
	resourceID, err := uuid.Parse(input.ResourceID)
	if err != nil {
		return OperationResult{}, ErrInvalid
	}
	resourceUUID := pgtype.UUID{Bytes: resourceID, Valid: true}
	return s.write(ctx, input.WriteInput, "access_update", "user", resourceUUID, func(queries *store.Queries) (int64, error) {
		if _, err := queries.GetPlatformOperationUser(ctx, resourceUUID); errors.Is(err, pgx.ErrNoRows) {
			return 0, ErrNotFound
		} else if err != nil {
			return 0, err
		}
		revision, err := queries.UpdatePlatformOperationUser(ctx, store.UpdatePlatformOperationUserParams{Status: input.Status, UserID: resourceUUID, ExpectedRevision: input.ExpectedRevision})
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, ErrConflict
		}
		if err != nil {
			return 0, err
		}
		if err := queries.RevokePlatformOperationUserGrants(ctx, resourceUUID); err != nil {
			return 0, err
		}
		for _, grant := range input.Grants {
			roleID, err := queries.GetPlatformOperationRoleByCode(ctx, grant.RoleCode)
			if errors.Is(err, pgx.ErrNoRows) {
				return 0, ErrInvalid
			}
			if err != nil {
				return 0, err
			}
			if err := queries.CreatePlatformOperationUserGrant(ctx, store.CreatePlatformOperationUserGrantParams{
				UserID: resourceUUID, RoleID: roleID, ScopeKind: grant.Scope.Kind,
				ProductCode: nullableText(grant.Scope.ProductCode), ResourceType: nullableText(grant.Scope.ResourceType), ResourceID: nullableText(grant.Scope.ResourceID),
			}); err != nil {
				return 0, err
			}
		}
		return revision, nil
	})
}

func (s *Service) OperationStatus(ctx context.Context, serviceID, actorUserID, operation, idempotencyKey string) (OperationResult, error) {
	actorID, err := uuid.Parse(actorUserID)
	if err != nil || serviceID == "" || operation != "session_revoke" && operation != "access_update" || len(idempotencyKey) < 8 || len(idempotencyKey) > 200 {
		return OperationResult{}, ErrInvalid
	}
	actorUUID := pgtype.UUID{Bytes: actorID, Valid: true}
	row, err := s.queries.GetPlatformOperationIdempotency(ctx, store.GetPlatformOperationIdempotencyParams{ServiceID: serviceID, ActorUserID: actorUUID, Operation: operation, IdempotencyKey: idempotencyKey})
	if errors.Is(err, pgx.ErrNoRows) {
		return OperationResult{Operation: operation, Status: "unknown"}, nil
	}
	if err != nil {
		return OperationResult{}, err
	}
	var result OperationResult
	if err := json.Unmarshal(row.ResponsePayload, &result); err != nil {
		return OperationResult{}, err
	}
	return result, nil
}

func (s *Service) write(ctx context.Context, input WriteInput, operation, resourceKind string, resourceID pgtype.UUID, mutate func(*store.Queries) (int64, error)) (OperationResult, error) {
	parsedActorID, _ := uuid.Parse(input.ActorUserID)
	actorID := pgtype.UUID{Bytes: parsedActorID, Valid: true}
	tx, err := s.database.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return OperationResult{}, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	queries := s.queries.WithTx(tx)
	lockKey := input.ServiceID + "\n" + input.ActorUserID + "\n" + operation + "\n" + input.IdempotencyKey
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, lockKey); err != nil {
		return OperationResult{}, err
	}
	cached, err := queries.GetPlatformOperationIdempotency(ctx, store.GetPlatformOperationIdempotencyParams{ServiceID: input.ServiceID, ActorUserID: actorID, Operation: operation, IdempotencyKey: input.IdempotencyKey})
	if err == nil {
		if !bytes.Equal(cached.RequestHash, input.RequestHash) {
			return OperationResult{}, ErrIdempotencyConflict
		}
		var result OperationResult
		if err := json.Unmarshal(cached.ResponsePayload, &result); err != nil {
			return OperationResult{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return OperationResult{}, err
		}
		return result, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return OperationResult{}, err
	}
	resourceVersion, err := mutate(queries)
	if err != nil {
		return OperationResult{}, err
	}
	result := OperationResult{Operation: operation, Status: "succeeded", ResourceID: input.ResourceID, ResourceVersion: resourceVersion}
	payload, err := json.Marshal(result)
	if err != nil {
		return OperationResult{}, err
	}
	if err := queries.CreatePlatformOperationAudit(ctx, store.CreatePlatformOperationAuditParams{ActorUserID: actorID, RequestID: input.RequestID, Operation: operation, ResourceKind: resourceKind, ResourceID: resourceID, ResultPayload: payload}); err != nil {
		return OperationResult{}, err
	}
	if err := queries.CreatePlatformOperationIdempotency(ctx, store.CreatePlatformOperationIdempotencyParams{ServiceID: input.ServiceID, ActorUserID: actorID, Operation: operation, IdempotencyKey: input.IdempotencyKey, RequestHash: input.RequestHash, ResponsePayload: payload}); err != nil {
		return OperationResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return OperationResult{}, err
	}
	return result, nil
}

func validWriteInput(input WriteInput) bool {
	if input.ServiceID == "" || len(input.IdempotencyKey) < 8 || len(input.IdempotencyKey) > 200 || len(input.RequestHash) != sha256Size || input.ResourceID == "" {
		return false
	}
	if _, err := uuid.Parse(input.ActorUserID); err != nil {
		return false
	}
	if len(input.RequestID) < 8 || len(input.RequestID) > 100 || input.RequestID[:4] != "req_" {
		return false
	}
	return true
}

const sha256Size = 32

func validUserStatus(status string) bool {
	return status == "active" || status == "suspended" || status == "deleted"
}

func validGrants(grants []GrantInput) bool {
	seen := make(map[string]struct{}, len(grants))
	for _, grant := range grants {
		if len(grant.RoleCode) < 2 || len(grant.RoleCode) > 64 {
			return false
		}
		scope := grant.Scope
		validScope := scope.Kind == "platform" && scope.ProductCode == "" && scope.ResourceType == "" && scope.ResourceID == "" ||
			scope.Kind == "product" && scope.ProductCode != "" && scope.ResourceType == "" && scope.ResourceID == "" ||
			scope.Kind == "resource" && scope.ProductCode != "" && scope.ResourceType != "" && scope.ResourceID != ""
		if !validScope {
			return false
		}
		key := grant.RoleCode + "\x00" + scope.Kind + "\x00" + scope.ProductCode + "\x00" + scope.ResourceType + "\x00" + scope.ResourceID
		if _, exists := seen[key]; exists {
			return false
		}
		seen[key] = struct{}{}
	}
	return true
}

func nullableText(value string) pgtype.Text {
	return pgtype.Text{String: value, Valid: value != ""}
}

func uuidString(value pgtype.UUID) string {
	if !value.Valid {
		return ""
	}
	return value.String()
}

func uuidPointer(value pgtype.UUID) *string {
	if !value.Valid {
		return nil
	}
	text := value.String()
	return &text
}

func textPointer(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}

func timePointer(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	return &value.Time
}
