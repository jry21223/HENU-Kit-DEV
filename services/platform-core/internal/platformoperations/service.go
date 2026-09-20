package platformoperations

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
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

const (
	membershipAccountPageSize  = 20
	platformOperationsPageSize = 20
)

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
	Pagination   Pagination   `json:"pagination"`
	Dependencies Dependencies `json:"dependencies"`
	GeneratedAt  time.Time    `json:"generated_at"`
}

type PageRequest struct {
	Accounts, Sessions, InboxItems, Audit int
	SnapshotAt                            time.Time
	AccountsCursor, SessionsCursor        string
	InboxItemsCursor, AuditCursor         string
}

func DefaultPageRequest() PageRequest {
	return PageRequest{Accounts: 1, Sessions: 1, InboxItems: 1, Audit: 1}
}

func (request PageRequest) Valid() bool {
	needsSnapshot := false
	for _, page := range []int{request.Accounts, request.Sessions, request.InboxItems, request.Audit} {
		if page < 1 {
			return false
		}
		needsSnapshot = needsSnapshot || page > 1
	}
	for _, cursor := range []string{request.AccountsCursor, request.SessionsCursor, request.InboxItemsCursor, request.AuditCursor} {
		if len(cursor) > 512 {
			return false
		}
		needsSnapshot = needsSnapshot || cursor != ""
	}
	if (request.Accounts > 1) != (request.AccountsCursor != "") || (request.Sessions > 1) != (request.SessionsCursor != "") || (request.InboxItems > 1) != (request.InboxItemsCursor != "") || (request.Audit > 1) != (request.AuditCursor != "") {
		return false
	}
	if needsSnapshot && request.SnapshotAt.IsZero() {
		return false
	}
	return request.SnapshotAt.IsZero() || !request.SnapshotAt.After(time.Now().UTC().Add(time.Minute))
}

type Pagination struct {
	Accounts   PageState `json:"accounts"`
	Sessions   PageState `json:"sessions"`
	InboxItems PageState `json:"inbox_items"`
	Audit      PageState `json:"audit"`
}

type PageState struct {
	Page       int     `json:"page"`
	NextPage   *int    `json:"next_page"`
	NextCursor *string `json:"next_cursor"`
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

type AccountPage struct {
	Accounts   []Account `json:"accounts"`
	NextPage   *int      `json:"next_page"`
	NextCursor *string   `json:"next_cursor"`
}

type pageCursor struct {
	Kind              string `json:"k"`
	SnapshotUnixNano  int64  `json:"s"`
	CreatedAtUnixNano int64  `json:"c"`
	ID                string `json:"i"`
	RequestID         string `json:"r,omitempty"`
	Source            int16  `json:"o,omitempty"`
	FilterHash        string `json:"f,omitempty"`
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

func (s *Service) Snapshot(ctx context.Context, serviceID, actorUserID string, pages PageRequest) (Snapshot, error) {
	if !pages.Valid() {
		return Snapshot{}, ErrInvalid
	}
	limited, err := s.readRateLimited(ctx, serviceID, actorUserID)
	if err != nil {
		return Snapshot{}, err
	}
	if limited {
		return Snapshot{}, ErrRateLimited
	}
	snapshotAt := pages.SnapshotAt.UTC()
	if snapshotAt.IsZero() {
		if s.database == nil || s.database.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&snapshotAt) != nil {
			return Snapshot{}, ErrDependency
		}
		snapshotAt = snapshotAt.UTC()
	}
	accountCursor, err := s.decodePageCursor(pages.AccountsCursor, "accounts", snapshotAt)
	if err != nil {
		return Snapshot{}, err
	}
	sessionCursor, err := s.decodePageCursor(pages.SessionsCursor, "sessions", snapshotAt)
	if err != nil {
		return Snapshot{}, err
	}
	inboxCursor, err := s.decodePageCursor(pages.InboxItemsCursor, "inbox", snapshotAt)
	if err != nil {
		return Snapshot{}, err
	}
	auditCursor, err := s.decodePageCursor(pages.AuditCursor, "audit", snapshotAt)
	if err != nil {
		return Snapshot{}, err
	}
	accountRows, err := s.queries.ListPlatformOperationAccounts(ctx, pageParams(snapshotAt, accountCursor))
	if err != nil {
		return Snapshot{}, err
	}
	accountHasMore := len(accountRows) > platformOperationsPageSize
	if len(accountRows) > platformOperationsPageSize {
		accountRows = accountRows[:platformOperationsPageSize]
	}
	accountState, err := s.pageState(pages.Accounts, accountHasMore, cursorFromAccount(snapshotAt, accountRows))
	if err != nil {
		return Snapshot{}, err
	}
	accountIDs := make([]pgtype.UUID, 0, len(accountRows))
	for _, row := range accountRows {
		accountIDs = append(accountIDs, row.ID)
	}
	grantRows, err := s.queries.ListPlatformOperationAccountGrants(ctx, accountIDs)
	if err != nil {
		return Snapshot{}, err
	}
	sessionRows, err := s.queries.ListPlatformOperationSessions(ctx, sessionPageParams(snapshotAt, sessionCursor))
	if err != nil {
		return Snapshot{}, err
	}
	sessionHasMore := len(sessionRows) > platformOperationsPageSize
	if len(sessionRows) > platformOperationsPageSize {
		sessionRows = sessionRows[:platformOperationsPageSize]
	}
	sessionState, err := s.pageState(pages.Sessions, sessionHasMore, cursorFromSession(snapshotAt, sessionRows))
	if err != nil {
		return Snapshot{}, err
	}
	mail, err := s.queries.CountPlatformOperationMailStatuses(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	inboxRows, err := s.queries.ListPlatformOperationInboxItems(ctx, inboxPageParams(snapshotAt, inboxCursor))
	if err != nil {
		return Snapshot{}, err
	}
	inboxHasMore := len(inboxRows) > platformOperationsPageSize
	if len(inboxRows) > platformOperationsPageSize {
		inboxRows = inboxRows[:platformOperationsPageSize]
	}
	inboxState, err := s.pageState(pages.InboxItems, inboxHasMore, cursorFromInbox(snapshotAt, inboxRows))
	if err != nil {
		return Snapshot{}, err
	}
	auditRows, err := s.queries.ListPlatformOperationAuditEvents(ctx, auditPageParams(snapshotAt, auditCursor))
	if err != nil {
		return Snapshot{}, err
	}
	auditHasMore := len(auditRows) > platformOperationsPageSize
	if len(auditRows) > platformOperationsPageSize {
		auditRows = auditRows[:platformOperationsPageSize]
	}
	auditState, err := s.pageState(pages.Audit, auditHasMore, cursorFromAudit(snapshotAt, auditRows))
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
		Mail: MailStatus{Pending: mail.Pending, Processing: mail.Processing, RetryDue: mail.RetryDue, Accepted: mail.Accepted, Delivered: mail.Delivered, Failed: mail.Failed, DeadLetters: mail.DeadLetters},
		Pagination: Pagination{
			Accounts: accountState, Sessions: sessionState, InboxItems: inboxState, Audit: auditState,
		},
		Dependencies: Dependencies{Postgres: "ready", Redis: redisStatus}, GeneratedAt: snapshotAt,
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

func pageParams(snapshotAt time.Time, cursor pageCursor) store.ListPlatformOperationAccountsParams {
	return store.ListPlatformOperationAccountsParams{SnapshotAt: timestampValue(snapshotAt), CursorCreatedAt: cursorTimestamp(cursor), CursorID: cursorUUID(cursor), PageLimit: platformOperationsPageSize + 1}
}

func sessionPageParams(snapshotAt time.Time, cursor pageCursor) store.ListPlatformOperationSessionsParams {
	return store.ListPlatformOperationSessionsParams{SnapshotAt: timestampValue(snapshotAt), CursorCreatedAt: cursorTimestamp(cursor), CursorID: cursorUUID(cursor), PageLimit: platformOperationsPageSize + 1}
}

func inboxPageParams(snapshotAt time.Time, cursor pageCursor) store.ListPlatformOperationInboxItemsParams {
	return store.ListPlatformOperationInboxItemsParams{SnapshotAt: timestampValue(snapshotAt), CursorCreatedAt: cursorTimestamp(cursor), CursorID: cursorUUID(cursor), PageLimit: platformOperationsPageSize + 1}
}

func auditPageParams(snapshotAt time.Time, cursor pageCursor) store.ListPlatformOperationAuditEventsParams {
	return store.ListPlatformOperationAuditEventsParams{SnapshotAt: timestampValue(snapshotAt), CursorCreatedAt: cursorTimestamp(cursor), CursorRequestID: pgtype.Text{String: cursor.RequestID, Valid: cursor.RequestID != ""}, CursorSource: pgtype.Int2{Int16: cursor.Source, Valid: cursor.ID != ""}, CursorID: cursorUUID(cursor), PageLimit: platformOperationsPageSize + 1}
}

func timestampValue(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value.UTC(), Valid: !value.IsZero()}
}

func cursorTimestamp(cursor pageCursor) pgtype.Timestamptz {
	if cursor.ID == "" {
		return pgtype.Timestamptz{}
	}
	return timestampValue(time.Unix(0, cursor.CreatedAtUnixNano))
}

func cursorUUID(cursor pageCursor) pgtype.UUID {
	if cursor.ID == "" {
		return pgtype.UUID{}
	}
	id, _ := uuid.Parse(cursor.ID)
	return pgtype.UUID{Bytes: id, Valid: true}
}

func cursorFromAccount(snapshotAt time.Time, rows []store.ListPlatformOperationAccountsRow) pageCursor {
	if len(rows) == 0 {
		return pageCursor{}
	}
	last := rows[len(rows)-1]
	return pageCursor{Kind: "accounts", SnapshotUnixNano: snapshotAt.UnixNano(), CreatedAtUnixNano: last.CreatedAt.Time.UnixNano(), ID: uuidString(last.ID)}
}

func cursorFromSearch(snapshotAt time.Time, query string, rows []store.SearchPlatformOperationAccountsRow) pageCursor {
	if len(rows) == 0 {
		return pageCursor{}
	}
	last := rows[len(rows)-1]
	return pageCursor{Kind: "accounts-search", SnapshotUnixNano: snapshotAt.UnixNano(), CreatedAtUnixNano: last.CreatedAt.Time.UnixNano(), ID: uuidString(last.ID), FilterHash: searchCursorFilter(query)}
}

func searchCursorFilter(query string) string {
	hash := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(query))))
	return base64.RawURLEncoding.EncodeToString(hash[:])
}

func cursorFromSession(snapshotAt time.Time, rows []store.ListPlatformOperationSessionsRow) pageCursor {
	if len(rows) == 0 {
		return pageCursor{}
	}
	last := rows[len(rows)-1]
	return pageCursor{Kind: "sessions", SnapshotUnixNano: snapshotAt.UnixNano(), CreatedAtUnixNano: last.CreatedAt.Time.UnixNano(), ID: uuidString(last.ID)}
}

func cursorFromInbox(snapshotAt time.Time, rows []store.ListPlatformOperationInboxItemsRow) pageCursor {
	if len(rows) == 0 {
		return pageCursor{}
	}
	last := rows[len(rows)-1]
	return pageCursor{Kind: "inbox", SnapshotUnixNano: snapshotAt.UnixNano(), CreatedAtUnixNano: last.CreatedAt.Time.UnixNano(), ID: uuidString(last.ID)}
}

func cursorFromAudit(snapshotAt time.Time, rows []store.ListPlatformOperationAuditEventsRow) pageCursor {
	if len(rows) == 0 {
		return pageCursor{}
	}
	last := rows[len(rows)-1]
	return pageCursor{Kind: "audit", SnapshotUnixNano: snapshotAt.UnixNano(), CreatedAtUnixNano: last.CreatedAt.Time.UnixNano(), ID: uuidString(last.EventID), RequestID: last.RequestID, Source: last.EventSource}
}

func (s *Service) pageState(page int, hasMore bool, cursor pageCursor) (PageState, error) {
	state := PageState{Page: page}
	if !hasMore {
		return state, nil
	}
	if cursor.ID == "" || page == int(^uint(0)>>1) {
		return PageState{}, ErrInvalid
	}
	token, err := s.encodePageCursor(cursor)
	if err != nil {
		return PageState{}, err
	}
	next := page + 1
	state.NextPage = &next
	state.NextCursor = &token
	return state, nil
}

func (s *Service) encodePageCursor(cursor pageCursor) (string, error) {
	if len(s.verificationKey) == 0 {
		return "", ErrDependency
	}
	payload, err := json.Marshal(cursor)
	if err != nil {
		return "", ErrDependency
	}
	mac := hmac.New(sha256.New, s.verificationKey)
	_, _ = mac.Write([]byte("henukit-platform-operations:page-cursor\x00"))
	_, _ = mac.Write(payload)
	return base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func (s *Service) decodePageCursor(token, kind string, snapshotAt time.Time) (pageCursor, error) {
	if token == "" {
		return pageCursor{}, nil
	}
	if len(token) > 512 || len(s.verificationKey) == 0 {
		return pageCursor{}, ErrInvalid
	}
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return pageCursor{}, ErrInvalid
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return pageCursor{}, ErrInvalid
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return pageCursor{}, ErrInvalid
	}
	mac := hmac.New(sha256.New, s.verificationKey)
	_, _ = mac.Write([]byte("henukit-platform-operations:page-cursor\x00"))
	_, _ = mac.Write(payload)
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return pageCursor{}, ErrInvalid
	}
	var cursor pageCursor
	if err := json.Unmarshal(payload, &cursor); err != nil || cursor.Kind != kind || cursor.SnapshotUnixNano != snapshotAt.UnixNano() || cursor.CreatedAtUnixNano == 0 || cursor.ID == "" {
		return pageCursor{}, ErrInvalid
	}
	if _, err := uuid.Parse(cursor.ID); err != nil || (kind == "audit" && cursor.RequestID == "") {
		return pageCursor{}, ErrInvalid
	}
	return cursor, nil
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

func (s *Service) SearchAccounts(ctx context.Context, serviceID, actorUserID, query string, page int, snapshotAt time.Time, cursorToken string) (AccountPage, error) {
	query = strings.TrimSpace(query)
	if len([]rune(query)) < 2 || len([]rune(query)) > 100 || page < 1 || snapshotAt.IsZero() || snapshotAt.After(time.Now().UTC().Add(time.Minute)) || len(cursorToken) > 512 || (page > 1) != (cursorToken != "") {
		return AccountPage{}, ErrInvalid
	}
	limited, err := s.readRateLimited(ctx, serviceID, actorUserID)
	if err != nil {
		return AccountPage{}, err
	}
	if limited {
		return AccountPage{}, ErrRateLimited
	}
	var exactEmailHash []byte
	if normalized, normalizeErr := s.normalizeEmail(query); normalizeErr == nil {
		exactEmailHash = emailLookupHash(s.verificationKey, normalized)
	}
	cursor, err := s.decodePageCursor(cursorToken, "accounts-search", snapshotAt.UTC())
	if err != nil {
		return AccountPage{}, err
	}
	if cursorToken != "" && cursor.FilterHash != searchCursorFilter(query) {
		return AccountPage{}, ErrInvalid
	}
	rows, err := s.queries.SearchPlatformOperationAccounts(ctx, store.SearchPlatformOperationAccountsParams{
		SnapshotAt: timestampValue(snapshotAt), Search: query, EmailLookupHash: exactEmailHash, CursorCreatedAt: cursorTimestamp(cursor), CursorID: cursorUUID(cursor), PageLimit: platformOperationsPageSize + 1,
	})
	if err != nil {
		return AccountPage{}, err
	}
	hasMore := len(rows) > platformOperationsPageSize
	if hasMore {
		rows = rows[:platformOperationsPageSize]
	}
	state, err := s.pageState(page, hasMore, cursorFromSearch(snapshotAt.UTC(), query, rows))
	if err != nil {
		return AccountPage{}, err
	}
	result := AccountPage{Accounts: []Account{}, NextPage: state.NextPage, NextCursor: state.NextCursor}
	ids := make([]pgtype.UUID, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	grantRows, err := s.queries.ListPlatformOperationAccountGrants(ctx, ids)
	if err != nil {
		return AccountPage{}, err
	}
	for _, row := range rows {
		email, openErr := s.openEmail(row.EmailCiphertext)
		if openErr != nil {
			return AccountPage{}, openErr
		}
		result.Accounts = append(result.Accounts, Account{ID: uuidString(row.ID), DisplayName: textPointer(row.DisplayName), Email: email, EmailVerified: row.EmailVerified, Status: row.Status, AuthorizationRevision: row.AuthorizationRevision, CreatedAt: row.CreatedAt.Time, Grants: []AccessGrant{}})
	}
	indexes := make(map[string]int, len(result.Accounts))
	for index := range result.Accounts {
		indexes[result.Accounts[index].ID] = index
	}
	for _, row := range grantRows {
		index, exists := indexes[uuidString(row.UserID)]
		if !exists {
			continue
		}
		result.Accounts[index].Grants = append(result.Accounts[index].Grants, AccessGrant{RoleCode: row.RoleCode, Scope: Scope{Kind: row.ScopeKind, ProductCode: textPointer(row.ProductCode), ResourceType: textPointer(row.ResourceType), ResourceID: textPointer(row.ResourceID)}})
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

func (s *Service) readRateLimited(ctx context.Context, serviceID, actorUserID string) (bool, error) {
	if serviceID == "" || actorUserID == "" || s.redis == nil {
		return false, ErrDependency
	}
	coordinator := coordination.NewRedis(s.redis)
	for _, dimension := range []struct {
		key    string
		limit  int64
		window time.Duration
	}{
		{key: "platform-read-minute:" + serviceID + ":" + actorUserID, limit: 120, window: time.Minute},
		{key: "platform-read-hour:" + serviceID + ":" + actorUserID, limit: 1_000, window: time.Hour},
	} {
		allowed, err := coordinator.Allow(ctx, "platform-core:"+dimension.key, dimension.limit, dimension.window)
		if err != nil {
			return false, ErrDependency
		}
		if !allowed {
			return true, nil
		}
	}
	return false, nil
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
