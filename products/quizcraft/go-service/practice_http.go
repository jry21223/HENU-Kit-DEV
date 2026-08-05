package quizcraft

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/text/unicode/norm"
	"henukit.dev/quizcraft/internal/store"
)

type PracticeHTTPConfig struct {
	Database            *pgxpool.Pool
	AuthHMACSecret      []byte
	LegacyBaseURL       string
	LegacyCompareSecret string
	HTTPClient          *http.Client
	Now                 func() time.Time
	SummaryClientID     string
	SummaryKeys         map[string]string
	CatalogClientID     string
	CatalogKeys         map[string]string
	// Portal command credentials are a deliberately separate, write-capable
	// service identity. They must never share the read-only catalog key ring.
	PortalCommandClientID   string
	PortalCommandKeys       map[string]string
	PortalCommandsEnabled   bool
	PlatformCoreURL         string
	PlatformClientID        string
	PlatformClientSecret    string
	PlatformKeyID           string
	PublicURL               string
	SessionEncryptionKey    []byte
	InboxExchangeToken      string
	WorkerContext           context.Context
	AllowTestWorkshopClaims bool
	WritesDisabled          bool
	ReleaseSHA              string
	CutoverEvidenceSecret   []byte
}

type practiceHTTP struct {
	database                *pgxpool.Pool
	queries                 *store.Queries
	authHMACSecret          []byte
	legacyBaseURL           string
	legacyCompareSecret     string
	httpClient              *http.Client
	now                     func() time.Time
	summaryClientID         string
	summaryKeys             map[string]string
	catalogClientID         string
	catalogKeys             map[string]string
	portalCommandClientID   string
	portalCommandKeys       map[string]string
	portalCommandsEnabled   bool
	platform                *platformClient
	sessionCodec            *quizcraftSessionCodec
	publicURL               string
	inboxExchangeToken      string
	inboxDispatchWake       chan struct{}
	allowTestWorkshopClaims bool
	writesDisabled          bool
	releaseSHA              string
	cutoverEvidenceSecret   []byte
}

type practiceActor struct {
	userID               *uuid.UUID
	key                  string
	permissions          map[string]bool
	scopes               []workshopScope
	exchangeToken        string
	platformProductScope bool
}

type workshopScope struct {
	Kind         string
	ProductCode  string
	ResourceType string
	ResourceID   string
}

type createSessionRequest struct {
	BankID        uuid.UUID `json:"bank_id"`
	BankVersionID uuid.UUID `json:"bank_version_id"`
	Mode          string    `json:"mode"`
	ChapterID     string    `json:"chapter_id"`
	QuestionCount int       `json:"question_count"`
}

type answerSubmissionRequest struct {
	QuestionID        uuid.UUID `json:"question_id"`
	QuestionVersionID uuid.UUID `json:"question_version_id"`
	Answer            any       `json:"answer"`
}

type rankingProfileRequest struct {
	Visible      bool   `json:"visible"`
	Nickname     string `json:"nickname"`
	SystemAvatar string `json:"system_avatar"`
}

type practiceQuestion struct {
	QuestionID        uuid.UUID `json:"question_id"`
	QuestionVersionID uuid.UUID `json:"question_version_id"`
	Type              string    `json:"type"`
	ChapterID         string    `json:"chapter_id"`
	Chapter           string    `json:"chapter"`
	Content           string    `json:"content"`
	Options           []string  `json:"options,omitempty"`
}

type practiceSession struct {
	SessionID                uuid.UUID          `json:"session_id"`
	BankID                   uuid.UUID          `json:"bank_id"`
	BankVersionID            uuid.UUID          `json:"bank_version_id"`
	Mode                     string             `json:"mode"`
	ExcludedUnavailableCount int                `json:"excluded_unavailable_count"`
	Questions                []practiceQuestion `json:"questions"`
}

type answerResult struct {
	QuestionID        uuid.UUID `json:"question_id"`
	QuestionVersionID uuid.UUID `json:"question_version_id"`
	Correct           bool      `json:"correct"`
	Replayed          bool      `json:"replayed"`
	ExpectedAnswer    any       `json:"expected_answer"`
	Analysis          string    `json:"analysis"`
}

type masterySubject struct {
	BankID           uuid.UUID `json:"bank_id"`
	Label            string    `json:"label"`
	Value            int       `json:"value"`
	TotalQuestions   int64     `json:"total_questions"`
	CorrectQuestions int64     `json:"correct_questions"`
}

type personalPracticeStats struct {
	TotalAnswers   int64            `json:"total_answers"`
	CorrectAnswers int64            `json:"correct_answers"`
	Accuracy       int              `json:"accuracy"`
	StreakDays     int              `json:"streak_days"`
	Mastery        []masterySubject `json:"mastery"`
}

type responseEnvelope struct {
	RequestID string `json:"request_id"`
	Data      any    `json:"data"`
}

type errorEnvelope struct {
	RequestID string `json:"request_id"`
	Error     struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func NewPracticeHTTP(config PracticeHTTPConfig) (http.Handler, error) {
	if config.Database == nil {
		return nil, errors.New("practice database is required")
	}
	if len(config.AuthHMACSecret) < 32 {
		return nil, errors.New("practice auth HMAC secret must be at least 32 bytes")
	}
	if (config.SummaryClientID == "") != (len(config.SummaryKeys) == 0) {
		return nil, errors.New("QuizCraft summary client and key ring must be configured together")
	}
	if (config.CatalogClientID == "") != (len(config.CatalogKeys) == 0) {
		return nil, errors.New("QuizCraft catalog client and key ring must be configured together")
	}
	if (config.PortalCommandClientID == "") != (len(config.PortalCommandKeys) == 0) {
		return nil, errors.New("QuizCraft Portal command client and key ring must be configured together")
	}
	if config.PortalCommandsEnabled && (config.PortalCommandClientID == "" || len(config.PortalCommandKeys) == 0) {
		return nil, errors.New("QuizCraft Portal command credentials are required when Portal commands are enabled")
	}
	if config.PortalCommandClientID != "" && config.PortalCommandClientID == config.CatalogClientID {
		return nil, errors.New("QuizCraft Portal command client must differ from the catalog client")
	}
	platformValues := []bool{config.PlatformCoreURL != "", config.PlatformClientID != "", config.PlatformClientSecret != "", config.PlatformKeyID != "", config.PublicURL != "", len(config.SessionEncryptionKey) != 0}
	platformCount := 0
	for _, configured := range platformValues {
		if configured {
			platformCount++
		}
	}
	if platformCount != 0 && platformCount != len(platformValues) {
		return nil, errors.New("platform Core OAuth configuration must be complete")
	}
	if config.InboxExchangeToken != "" && (len(config.InboxExchangeToken) < 32 || platformCount != len(platformValues)) {
		return nil, errors.New("operations Inbox exchange token requires complete Platform Core configuration")
	}
	if len(config.CutoverEvidenceSecret) != 0 && len(config.CutoverEvidenceSecret) < 32 {
		return nil, errors.New("cutover evidence secret must be at least 32 bytes")
	}
	for keyID, secret := range config.SummaryKeys {
		if keyID == "" || len(secret) < 32 {
			return nil, errors.New("quizCraft summary key ring is invalid")
		}
	}
	for keyID, secret := range config.CatalogKeys {
		if keyID == "" || len(secret) < 32 {
			return nil, errors.New("QuizCraft catalog key ring is invalid")
		}
	}
	for keyID, secret := range config.PortalCommandKeys {
		if keyID == "" || len(secret) < 32 {
			return nil, errors.New("QuizCraft Portal command key ring is invalid")
		}
		for _, catalogSecret := range config.CatalogKeys {
			if subtle.ConstantTimeCompare([]byte(secret), []byte(catalogSecret)) == 1 {
				return nil, errors.New("QuizCraft Portal command keys must differ from catalog keys")
			}
		}
	}
	legacyBaseURL := strings.TrimRight(strings.TrimSpace(config.LegacyBaseURL), "/")
	if legacyBaseURL != "" {
		parsed, err := url.Parse(legacyBaseURL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return nil, errors.New("legacy base URL must be an absolute HTTP(S) URL")
		}
		if len(config.LegacyCompareSecret) < 32 {
			return nil, errors.New("legacy compare secret must be at least 32 bytes")
		}
	}
	client := config.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 750 * time.Millisecond}
	}
	now := config.Now
	if now == nil {
		now = time.Now
	}
	releaseSHA := strings.TrimSpace(config.ReleaseSHA)
	if releaseSHA == "" {
		releaseSHA = "development"
	}
	service := &practiceHTTP{database: config.Database, queries: store.New(config.Database), authHMACSecret: config.AuthHMACSecret, legacyBaseURL: legacyBaseURL, legacyCompareSecret: config.LegacyCompareSecret, httpClient: client, now: now, summaryClientID: config.SummaryClientID, summaryKeys: config.SummaryKeys, catalogClientID: config.CatalogClientID, catalogKeys: config.CatalogKeys, portalCommandClientID: config.PortalCommandClientID, portalCommandKeys: config.PortalCommandKeys, portalCommandsEnabled: config.PortalCommandsEnabled, allowTestWorkshopClaims: config.AllowTestWorkshopClaims, writesDisabled: config.WritesDisabled, releaseSHA: releaseSHA, cutoverEvidenceSecret: config.CutoverEvidenceSecret}
	if platformCount == len(platformValues) {
		platform, err := newPlatformClient(config.PlatformCoreURL, config.PlatformClientID, config.PlatformClientSecret, config.PlatformKeyID, client)
		if err != nil {
			return nil, err
		}
		codec, err := newQuizcraftSessionCodec(config.SessionEncryptionKey)
		if err != nil {
			return nil, errors.New("invalid QuizCraft session encryption key")
		}
		publicURL := strings.TrimRight(config.PublicURL, "/")
		parsed, err := url.Parse(publicURL)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
			return nil, errors.New("QuizCraft public URL must be an HTTPS origin")
		}
		service.platform, service.sessionCodec, service.publicURL = platform, codec, publicURL
		service.inboxExchangeToken = config.InboxExchangeToken
		if config.InboxExchangeToken != "" {
			service.inboxDispatchWake = make(chan struct{}, 1)
		}
	}
	router := chi.NewRouter()
	router.Get("/healthz", service.health)
	router.Get("/readyz", service.readiness)
	if len(service.cutoverEvidenceSecret) != 0 {
		router.With(service.authenticateCutoverEvidence).Get("/api/v1/cutover-evidence", service.cutoverEvidence)
	}
	router.Get("/auth/login", service.startPlatformLogin)
	router.Get("/auth/callback", service.finishPlatformLogin)
	router.With(service.authenticateConsoleSummary).Get("/api/v1/console-summary", service.consoleSummary)
	if service.catalogClientID != "" {
		portalRead := router.With(service.authenticatePortalRead)
		portalRead.Get("/api/v1/banks", service.listBanks)
		portalRead.Get("/api/v1/rankings/overall", service.overallRanking)
		portalRead.Get("/api/v1/banks/{bank_id}/rankings", service.bankRanking)
		// This is the one read endpoint whose signed identity is an account
		// subject. authenticatePortalPersonalStats validates the six-part HMAC;
		// catalog and rankings remain on their established five-part contract.
		router.With(service.authenticatePortalPersonalStats).Get("/api/v1/stats", service.personalStats)
		router.With(service.authenticatePortalPersonalStats).Get("/api/v1/portal/practice/feedback/{feedback_id}/status", service.portalFeedbackStatus)
	}
	writes := router.With(service.requireWritesEnabled)
	writes.Get("/api/v1/feedback", service.listFeedbackStatuses)
	writes.Post("/api/v1/feedback", service.createFeedback)
	writes.Get("/api/v1/feedback/{feedback_id}/status", service.getFeedbackStatus)
	router.Get("/api/v1/workshop/banks", service.listWorkshopBanks)
	router.Get("/api/v1/workshop/catalog", service.listWorkshopCatalog)
	writes.Post("/api/v1/workshop/banks", service.createWorkshopBank)
	writes.Post("/api/v1/workshop/banks/{bank_id}/versions", service.createWorkshopVersion)
	router.Get("/api/v1/workshop/banks/{bank_id}/versions/{bank_version_id}", service.getWorkshopVersion)
	writes.Post("/api/v1/workshop/banks/{bank_id}/imports", service.importWorkshopVersion)
	writes.Post("/api/v1/workshop/banks/{bank_id}/versions/{bank_version_id}/validate", service.validateWorkshopVersion)
	writes.Post("/api/v1/workshop/banks/{bank_id}/versions/{bank_version_id}/publish", service.publishWorkshopVersion)
	writes.Post("/api/v1/workshop/banks/{bank_id}/versions/{bank_version_id}/unpublish", service.unpublishWorkshopVersion)
	writes.Post("/api/v1/workshop/banks/{bank_id}/rollback", service.rollbackWorkshopBank)
	router.Get("/api/v1/workshop/feedback/{feedback_id}", service.getWorkshopFeedback)
	router.Get("/api/v1/favorites", service.listFavoriteFolders)
	router.Get("/api/v1/banks/{bank_id}/favorites", service.listFavoriteQuestions)
	writes.Put("/api/v1/banks/{bank_id}/favorites/{question_id}", service.favoriteQuestion)
	writes.Delete("/api/v1/banks/{bank_id}/favorites/{question_id}", service.unfavoriteQuestion)
	writes.Post("/api/v1/banks/{bank_id}/favorites/practice-sessions", service.createFavoritesSession)
	router.Get("/api/v1/rankings/legacy", service.legacyRanking)
	writes.Patch("/api/v1/ranking-profile", service.updateRankingProfile)
	writes.Post("/api/v1/practice/sessions", service.createSession)
	writes.Post("/api/v1/practice/sessions/{session_id}/answers", service.submitAnswer)
	// Portal is the sole browser-facing caller for these narrow command routes.
	// Keeping their registration default-off makes a deployment without an
	// intentional cutover flag indistinguishable from a route that does not
	// exist; it also prevents a generic Core write proxy from growing here.
	if service.portalCommandsEnabled && service.portalCommandClientID != "" {
		portalCommands := router.With(service.authenticatePortalCommand).With(service.requireWritesEnabled)
		portalCommands.Post("/api/v1/portal/practice/sessions", service.createSession)
		portalCommands.Post("/api/v1/portal/practice/sessions/{session_id}/answers", service.submitAnswer)
		portalCommands.Post("/api/v1/portal/practice/feedback", service.createFeedback)
	}
	router.Get("/api/v1/operations/{operation_kind}", service.operationStatus)
	router.Get("/api/v1/learning-state", service.learningState)
	if service.inboxExchangeToken != "" {
		workerContext := config.WorkerContext
		if workerContext == nil {
			workerContext = context.Background()
		}
		go service.runInboxDispatcher(workerContext)
	}
	return router, nil
}

func (service *practiceHTTP) operationStatus(writer http.ResponseWriter, request *http.Request) {
	actor, status, err := service.actor(writer, request)
	if err != nil {
		writeError(writer, status, "invalid_session", err.Error())
		return
	}
	idempotencyKey, ok := requiredIdempotencyKey(writer, request)
	if !ok {
		return
	}
	kind := chi.URLParam(request, "operation_kind")
	if kind != "create_practice_session" && kind != "submit_practice_answer" && kind != "favorite_question" && kind != "unfavorite_question" && kind != "create_favorites_session" && kind != "update_ranking_profile" && kind != "create_feedback" && kind != "create_workshop_bank" && kind != "create_bank_version" && kind != "import_bank" && kind != "validate_version" && kind != "publish_version" && kind != "unpublish_version" && kind != "rollback_bank" {
		writeError(writer, http.StatusNotFound, "operation_unknown", "operation is not implemented by Practice Core")
		return
	}
	result, err := service.queries.GetOperationResult(request.Context(), store.GetOperationResultParams{ActorKey: actor.key, OperationKind: kind, IdempotencyKey: idempotencyKey})
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(writer, http.StatusNotFound, "operation_unknown", "no result exists for this actor and idempotency key")
		return
	}
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft is temporarily unavailable")
		return
	}
	if !result.ResourceID.Valid {
		writeError(writer, http.StatusServiceUnavailable, "invalid_operation", "stored operation has no resource")
		return
	}
	var original struct {
		RequestID string `json:"request_id"`
		Data      struct {
			OperationID uuid.UUID `json:"operation_id"`
		} `json:"data"`
	}
	_ = json.Unmarshal([]byte(result.ResponseBody), &original)
	if original.Data.OperationID == uuid.Nil {
		original.Data.OperationID = uuid.NewSHA1(result.ResourceID.UUID, []byte(kind+":"+idempotencyKey))
	}
	writeJSON(writer, http.StatusOK, responseEnvelope{RequestID: requestID(), Data: map[string]any{
		"operation_id": original.Data.OperationID,
		"state":        "succeeded", "idempotency_key": idempotencyKey,
		"request_id": original.RequestID, "resource_id": result.ResourceID.UUID,
	}})
}

func (service *practiceHTTP) overallRanking(writer http.ResponseWriter, request *http.Request) {
	service.writeRanking(writer, request, uuid.Nil)
}

func (service *practiceHTTP) bankRanking(writer http.ResponseWriter, request *http.Request) {
	bankID, err := uuid.Parse(chi.URLParam(request, "bank_id"))
	if err != nil {
		writeError(writer, http.StatusBadRequest, "invalid_bank_id", "bank_id must be a UUID")
		return
	}
	service.writeRanking(writer, request, bankID)
}

func (service *practiceHTTP) writeRanking(writer http.ResponseWriter, request *http.Request, bankID uuid.UUID) {
	period := request.URL.Query().Get("period")
	if period == "" {
		period = "weekly"
	}
	if period != "weekly" && period != "lifetime" {
		writeError(writer, http.StatusBadRequest, "invalid_period", "period must be weekly or lifetime")
		return
	}
	start := time.Unix(0, 0).UTC()
	if period == "weekly" {
		now := service.now().UTC()
		start = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -(int(now.Weekday())+6)%7)
	}
	entries := make([]map[string]any, 0)
	if bankID == uuid.Nil {
		rows, err := service.queries.ListOverallRanking(request.Context(), pgtype.Timestamptz{Time: start, Valid: true})
		if err != nil {
			writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft ranking is temporarily unavailable")
			return
		}
		for _, row := range rows {
			entries = append(entries, map[string]any{"rank": row.Rank, "nickname": row.Nickname, "system_avatar": row.SystemAvatar, "correct_answer_count": row.CorrectAnswerCount})
		}
	} else {
		rows, err := service.queries.ListBankRanking(request.Context(), store.ListBankRankingParams{BankID: bankID, SubmittedAt: pgtype.Timestamptz{Time: start, Valid: true}})
		if err != nil {
			writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft ranking is temporarily unavailable")
			return
		}
		for _, row := range rows {
			entries = append(entries, map[string]any{"rank": row.Rank, "nickname": row.Nickname, "system_avatar": row.SystemAvatar, "correct_answer_count": row.CorrectAnswerCount})
		}
	}
	data := map[string]any{"scope": "overall", "period": period, "metric": "correct_answer_count", "entries": entries}
	if bankID != uuid.Nil {
		data["scope"] = "bank"
		data["bank_id"] = bankID
	}
	writeJSON(writer, http.StatusOK, responseEnvelope{RequestID: requestID(), Data: data})
}

func (service *practiceHTTP) updateRankingProfile(writer http.ResponseWriter, request *http.Request) {
	actor, ok := service.requireUser(writer, request)
	if !ok {
		return
	}
	idempotencyKey, ok := requiredIdempotencyKey(writer, request)
	if !ok {
		return
	}
	var input rankingProfileRequest
	raw, err := decodeBody(request, &input)
	nickname, validNickname := normalizeRankingNickname(input.Nickname)
	if err != nil || !jsonStringFieldPresent(raw, "nickname") || !validNickname || !validSystemAvatar(input.SystemAvatar) {
		writeError(writer, http.StatusBadRequest, "invalid_ranking_profile", "nickname or system avatar is not allowed")
		return
	}
	requestHash := hashCanonical(raw)
	tx, err := service.database.BeginTx(request.Context(), pgx.TxOptions{})
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft ranking is temporarily unavailable")
		return
	}
	defer func() { _ = tx.Rollback(request.Context()) }()
	queries := store.New(tx)
	if err := queries.LockRankingProfileMutation(request.Context(), actor.userID.String()); err != nil {
		writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft ranking is temporarily unavailable")
		return
	}
	if err := lockIdempotency(request.Context(), queries, actor.key, "update_ranking_profile", idempotencyKey); err != nil {
		writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft ranking is temporarily unavailable")
		return
	}
	if storedStatus, storedBody, found, conflict, err := loadIdempotency(request.Context(), queries, actor.key, "update_ranking_profile", idempotencyKey, requestHash); err != nil {
		writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft ranking is temporarily unavailable")
		return
	} else if conflict {
		writeError(writer, http.StatusConflict, "idempotency_conflict", "idempotency key was already used with another request")
		return
	} else if found {
		writeRawJSON(writer, storedStatus, storedBody)
		return
	}
	if err := queries.UpsertRankingProfile(request.Context(), store.UpsertRankingProfileParams{UserID: *actor.userID, Nickname: nickname, SystemAvatar: input.SystemAvatar, Visible: input.Visible}); err != nil {
		writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft ranking is temporarily unavailable")
		return
	}
	outerRequestID := requestID()
	response := responseEnvelope{RequestID: outerRequestID, Data: map[string]any{"operation_id": uuid.NewSHA1(*actor.userID, []byte("update_ranking_profile:"+idempotencyKey)), "state": "succeeded", "idempotency_key": idempotencyKey, "request_id": outerRequestID, "resource_id": *actor.userID}}
	encoded, _ := json.Marshal(response)
	if err := storeIdempotency(request.Context(), queries, actor.key, "update_ranking_profile", idempotencyKey, requestHash, http.StatusOK, encoded, *actor.userID); err != nil {
		writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft ranking is temporarily unavailable")
		return
	}
	if err := tx.Commit(request.Context()); err != nil {
		writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft ranking is temporarily unavailable")
		return
	}
	writeRawJSON(writer, http.StatusOK, encoded)
}

func normalizeRankingNickname(value string) (string, bool) {
	value = strings.TrimSpace(norm.NFKC.String(value))
	if value == "" {
		return "匿名学习者", true
	}
	if looksLikeRankingIdentifier(value) {
		return "", false
	}
	length := len([]rune(value))
	if length < 1 || length > 32 {
		return "", false
	}
	var skeleton strings.Builder
	for _, r := range value {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || (unicode.Is(unicode.Latin, r) && r > unicode.MaxASCII) || (!unicode.Is(unicode.Han, r) && !unicode.Is(unicode.Latin, r) && !unicode.IsDigit(r) && !strings.ContainsRune(" _-.", r)) {
			return "", false
		}
		if !strings.ContainsRune(" _-.", r) {
			skeleton.WriteRune(unicode.ToLower(r))
		}
	}
	lower := skeleton.String()
	for _, forbidden := range []string{"admin", "administrator", "henukit", "quizcraft", "官方", "管理员", "管理員", "官网", "官網"} {
		if strings.Contains(lower, forbidden) {
			return "", false
		}
	}
	return value, true
}

func looksLikeRankingIdentifier(value string) bool {
	if strings.Contains(value, "@") {
		return true
	}
	compact := strings.Map(func(r rune) rune {
		if strings.ContainsRune(" _-.", r) {
			return -1
		}
		return r
	}, value)
	if len(compact) != 32 {
		return false
	}
	for _, r := range compact {
		if !('0' <= r && r <= '9') && !('a' <= r && r <= 'f') && !('A' <= r && r <= 'F') {
			return false
		}
	}
	return true
}

func validSystemAvatar(value string) bool {
	return value == "scholar-blue" || value == "coder-green" || value == "reader-amber" || value == "owl-purple"
}

func (service *practiceHTTP) requireUser(writer http.ResponseWriter, request *http.Request) (practiceActor, bool) {
	actor, status, err := service.actor(writer, request)
	if err != nil {
		writeError(writer, status, "invalid_session", err.Error())
		return practiceActor{}, false
	}
	if actor.userID == nil {
		writeError(writer, http.StatusUnauthorized, "authentication_required", "sign in to use favorites")
		return practiceActor{}, false
	}
	return actor, true
}

func parseFavoritePath(writer http.ResponseWriter, request *http.Request, withQuestion bool) (uuid.UUID, uuid.UUID, bool) {
	bankID, err := uuid.Parse(chi.URLParam(request, "bank_id"))
	if err != nil {
		writeError(writer, http.StatusBadRequest, "invalid_bank_id", "bank_id must be a UUID")
		return uuid.Nil, uuid.Nil, false
	}
	if !withQuestion {
		return bankID, uuid.Nil, true
	}
	questionID, err := uuid.Parse(chi.URLParam(request, "question_id"))
	if err != nil {
		writeError(writer, http.StatusBadRequest, "invalid_question_id", "question_id must be a UUID")
		return uuid.Nil, uuid.Nil, false
	}
	return bankID, questionID, true
}

func (service *practiceHTTP) listFavoriteFolders(writer http.ResponseWriter, request *http.Request) {
	actor, ok := service.requireUser(writer, request)
	if !ok {
		return
	}
	rows, err := service.queries.ListFavoriteFolders(request.Context(), *actor.userID)
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft favorites are temporarily unavailable")
		return
	}
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		items = append(items, map[string]any{"bank_id": row.BankID, "bank_name": row.BankName, "available_count": row.AvailableCount, "unavailable_count": row.UnavailableCount})
	}
	writeJSON(writer, http.StatusOK, responseEnvelope{RequestID: requestID(), Data: items})
}

func (service *practiceHTTP) listFavoriteQuestions(writer http.ResponseWriter, request *http.Request) {
	actor, ok := service.requireUser(writer, request)
	if !ok {
		return
	}
	bankID, _, ok := parseFavoritePath(writer, request, false)
	if !ok {
		return
	}
	rows, err := service.queries.ListFavoriteQuestions(request.Context(), store.ListFavoriteQuestionsParams{UserID: *actor.userID, BankID: bankID})
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft favorites are temporarily unavailable")
		return
	}
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		item := map[string]any{"bank_id": row.BankID, "question_id": row.QuestionID, "available": row.QuestionVersionID.Valid}
		if row.QuestionVersionID.Valid {
			item["question_version_id"] = row.QuestionVersionID.UUID
		}
		items = append(items, item)
	}
	writeJSON(writer, http.StatusOK, responseEnvelope{RequestID: requestID(), Data: items})
}

func (service *practiceHTTP) favoriteQuestion(writer http.ResponseWriter, request *http.Request) {
	service.changeFavorite(writer, request, true)
}

func (service *practiceHTTP) unfavoriteQuestion(writer http.ResponseWriter, request *http.Request) {
	service.changeFavorite(writer, request, false)
}

func (service *practiceHTTP) changeFavorite(writer http.ResponseWriter, request *http.Request, add bool) {
	actor, ok := service.requireUser(writer, request)
	if !ok {
		return
	}
	bankID, questionID, ok := parseFavoritePath(writer, request, true)
	if !ok {
		return
	}
	idempotencyKey, ok := requiredIdempotencyKey(writer, request)
	if !ok {
		return
	}
	kind := "unfavorite_question"
	if add {
		kind = "favorite_question"
	}
	requestHash := hashCanonical([]byte(request.Method + ":" + bankID.String() + ":" + questionID.String()))
	tx, err := service.database.BeginTx(request.Context(), pgx.TxOptions{})
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft favorites are temporarily unavailable")
		return
	}
	defer func() { _ = tx.Rollback(request.Context()) }()
	queries := store.New(tx)
	if err := lockIdempotency(request.Context(), queries, actor.key, kind, idempotencyKey); err != nil {
		writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft favorites are temporarily unavailable")
		return
	}
	if storedStatus, storedBody, found, conflict, err := loadIdempotency(request.Context(), queries, actor.key, kind, idempotencyKey, requestHash); err != nil {
		writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft favorites are temporarily unavailable")
		return
	} else if conflict {
		writeError(writer, http.StatusConflict, "idempotency_conflict", "idempotency key was already used with another request")
		return
	} else if found {
		writeRawJSON(writer, storedStatus, storedBody)
		return
	}
	if err := queries.LockFavorite(request.Context(), actor.key+":"+bankID.String()); err != nil {
		writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft favorites are temporarily unavailable")
		return
	}
	if add {
		if _, err := queries.IsQuestionInBank(request.Context(), store.IsQuestionInBankParams{BankID: bankID, ID: questionID}); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeError(writer, http.StatusNotFound, "question_unknown", "question does not belong to this bank")
			} else {
				writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft favorites are temporarily unavailable")
			}
			return
		}
		err = queries.AddFavorite(request.Context(), store.AddFavoriteParams{UserID: *actor.userID, BankID: bankID, QuestionID: questionID})
	} else {
		err = queries.RemoveFavorite(request.Context(), store.RemoveFavoriteParams{UserID: *actor.userID, BankID: bankID, QuestionID: questionID})
	}
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft favorites are temporarily unavailable")
		return
	}
	outerRequestID := requestID()
	response := responseEnvelope{RequestID: outerRequestID, Data: map[string]any{"operation_id": uuid.NewSHA1(questionID, []byte(kind+":"+idempotencyKey)), "state": "succeeded", "idempotency_key": idempotencyKey, "request_id": outerRequestID, "resource_id": questionID}}
	encoded, _ := json.Marshal(response)
	if err := storeIdempotency(request.Context(), queries, actor.key, kind, idempotencyKey, requestHash, http.StatusOK, encoded, questionID); err != nil {
		writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft favorites are temporarily unavailable")
		return
	}
	if err := tx.Commit(request.Context()); err != nil {
		writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft favorites are temporarily unavailable")
		return
	}
	writeRawJSON(writer, http.StatusOK, encoded)
}

func (service *practiceHTTP) createFavoritesSession(writer http.ResponseWriter, request *http.Request) {
	actor, ok := service.requireUser(writer, request)
	if !ok {
		return
	}
	bankID, _, ok := parseFavoritePath(writer, request, false)
	if !ok {
		return
	}
	idempotencyKey, ok := requiredIdempotencyKey(writer, request)
	if !ok {
		return
	}
	requestHash := hashCanonical([]byte(bankID.String()))
	tx, err := service.database.BeginTx(request.Context(), pgx.TxOptions{})
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft favorites are temporarily unavailable")
		return
	}
	defer func() { _ = tx.Rollback(request.Context()) }()
	queries := store.New(tx)
	if err := lockIdempotency(request.Context(), queries, actor.key, "create_favorites_session", idempotencyKey); err != nil {
		writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft favorites are temporarily unavailable")
		return
	}
	if storedStatus, storedBody, found, conflict, err := loadIdempotency(request.Context(), queries, actor.key, "create_favorites_session", idempotencyKey, requestHash); err != nil {
		writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft favorites are temporarily unavailable")
		return
	} else if conflict {
		writeError(writer, http.StatusConflict, "idempotency_conflict", "idempotency key was already used with another request")
		return
	} else if found {
		writeRawJSON(writer, storedStatus, storedBody)
		return
	}
	if err := queries.LockFavorite(request.Context(), actor.key+":"+bankID.String()); err != nil {
		writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft favorites are temporarily unavailable")
		return
	}
	bankVersionID, err := queries.GetActiveBankVersion(request.Context(), bankID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(writer, http.StatusNotFound, "bank_unavailable", "bank has no published active version")
		} else {
			writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft favorites are temporarily unavailable")
		}
		return
	}
	rows, err := queries.ListFavoritePracticeQuestions(request.Context(), store.ListFavoritePracticeQuestionsParams{BankVersionID: bankVersionID, UserID: *actor.userID, BankID: bankID})
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft favorites are temporarily unavailable")
		return
	}
	total, err := queries.CountFavoritesForBank(request.Context(), store.CountFavoritesForBankParams{UserID: *actor.userID, BankID: bankID})
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft favorites are temporarily unavailable")
		return
	}
	if len(rows) == 0 {
		writeError(writer, http.StatusConflict, "empty_favorites", "this bank has no available favorite questions")
		return
	}
	sessionID := uuid.New()
	if err := queries.CreatePracticeSession(request.Context(), store.CreatePracticeSessionParams{ID: sessionID, BankID: bankID, BankVersionID: bankVersionID, UserID: nullableUUID(actor.userID), ActorKey: actor.key, Mode: "favorites"}); err != nil {
		writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft favorites are temporarily unavailable")
		return
	}
	questions := make([]practiceQuestion, 0, len(rows))
	for index, row := range rows {
		question := practiceQuestion{QuestionID: row.QuestionID, QuestionVersionID: row.QuestionVersionID, Type: row.Type, ChapterID: row.ChapterID, Chapter: row.ChapterName, Content: row.Content}
		if question.Type == "single" || question.Type == "multi" {
			if err := json.Unmarshal(row.Options, &question.Options); err != nil {
				writeError(writer, http.StatusServiceUnavailable, "invalid_question", "stored question options are invalid")
				return
			}
		}
		questions = append(questions, question)
		if err := queries.AddPracticeSessionQuestion(request.Context(), store.AddPracticeSessionQuestionParams{SessionID: sessionID, BankID: bankID, BankVersionID: bankVersionID, QuestionID: row.QuestionID, QuestionVersionID: row.QuestionVersionID, Position: int32(index + 1)}); err != nil {
			writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft favorites are temporarily unavailable")
			return
		}
	}
	response := responseEnvelope{RequestID: requestID(), Data: practiceSession{SessionID: sessionID, BankID: bankID, BankVersionID: bankVersionID, Mode: "favorites", ExcludedUnavailableCount: int(total) - len(questions), Questions: questions}}
	encoded, _ := json.Marshal(response)
	if err := storeIdempotency(request.Context(), queries, actor.key, "create_favorites_session", idempotencyKey, requestHash, http.StatusCreated, encoded, sessionID); err != nil {
		writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft favorites are temporarily unavailable")
		return
	}
	if err := tx.Commit(request.Context()); err != nil {
		writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft favorites are temporarily unavailable")
		return
	}
	writeRawJSON(writer, http.StatusCreated, encoded)
}

func (service *practiceHTTP) health(writer http.ResponseWriter, _ *http.Request) {
	writeJSON(writer, http.StatusOK, responseEnvelope{RequestID: requestID(), Data: map[string]string{"status": "ok"}})
}

func (service *practiceHTTP) readiness(writer http.ResponseWriter, request *http.Request) {
	if err := service.database.Ping(request.Context()); err != nil {
		writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft database is not ready")
		return
	}
	writeJSON(writer, http.StatusOK, responseEnvelope{RequestID: requestID(), Data: map[string]string{"status": "ok"}})
}

func (service *practiceHTTP) authenticateCutoverEvidence(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		provided := request.Header.Get("X-QuizCraft-Cutover-Secret")
		if len(provided) != len(service.cutoverEvidenceSecret) || subtle.ConstantTimeCompare([]byte(provided), service.cutoverEvidenceSecret) != 1 {
			writeError(writer, http.StatusUnauthorized, "cutover_evidence_unauthorized", "cutover evidence authentication failed")
			return
		}
		next.ServeHTTP(writer, request)
	})
}

func (service *practiceHTTP) cutoverEvidence(writer http.ResponseWriter, request *http.Request) {
	runID, err := uuid.Parse(request.URL.Query().Get("run_id"))
	if err != nil {
		writeError(writer, http.StatusBadRequest, "invalid_cutover_evidence", "run_id must be a UUID")
		return
	}
	expectedHead, err := strconv.ParseInt(request.URL.Query().Get("source_head"), 10, 64)
	if err != nil || expectedHead < 0 {
		writeError(writer, http.StatusBadRequest, "invalid_cutover_evidence", "source_head must be a non-negative integer")
		return
	}
	var cursor int64
	if err := service.database.QueryRow(request.Context(), `SELECT r.caught_up_event_id FROM quizcraft_migration_runs r WHERE r.id=$1 AND r.state='passed' AND COALESCE((r.report->>'content_reconciled')::boolean,false) AND NOT EXISTS(SELECT 1 FROM quizcraft_migration_exceptions e LEFT JOIN quizcraft_migration_exception_resolutions x ON x.exception_id=e.id WHERE e.run_id=r.id AND x.exception_id IS NULL)`, runID).Scan(&cursor); err != nil || cursor != expectedHead {
		writeError(writer, http.StatusServiceUnavailable, "migration_evidence_missing", "a passed QuizCraft migration run is required")
		return
	}
	writeJSON(writer, http.StatusOK, responseEnvelope{RequestID: requestID(), Data: map[string]any{
		"database":         "ready",
		"writes_enabled":   !service.writesDisabled,
		"release_sha":      service.releaseSHA,
		"migration_run_id": runID,
		"migration_cursor": cursor,
	}})
}

func (service *practiceHTTP) requireWritesEnabled(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if service.writesDisabled {
			writer.Header().Set("Retry-After", "60")
			writeError(writer, http.StatusServiceUnavailable, "writes_disabled", "QuizCraft write cutover is not enabled")
			return
		}
		next.ServeHTTP(writer, request)
	})
}

func (service *practiceHTTP) listBanks(writer http.ResponseWriter, request *http.Request) {
	rows, err := service.queries.ListPublishedBanks(request.Context())
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft banks are temporarily unavailable")
		return
	}
	banks := make([]map[string]any, 0)
	for _, row := range rows {
		if !row.ActiveVersionID.Valid {
			continue
		}
		chaptersJSON, marshalErr := json.Marshal(row.Chapters)
		if marshalErr != nil {
			writeError(writer, http.StatusServiceUnavailable, "invalid_bank", "QuizCraft bank chapters are invalid")
			return
		}
		var chapters []map[string]string
		if err := json.Unmarshal(chaptersJSON, &chapters); err != nil {
			writeError(writer, http.StatusServiceUnavailable, "invalid_bank", "QuizCraft bank chapters are invalid")
			return
		}
		banks = append(banks, map[string]any{"bank_id": row.ID, "bank_version_id": row.ActiveVersionID.UUID, "bank_key": row.BankKey, "name": row.Name, "content_sha256": row.ContentSha256, "question_count": row.QuestionCount, "chapters": chapters})
	}
	writeJSON(writer, http.StatusOK, responseEnvelope{RequestID: requestID(), Data: banks})
}

func (service *practiceHTTP) createSession(writer http.ResponseWriter, request *http.Request) {
	actor, status, err := service.actor(writer, request)
	if err != nil {
		writeError(writer, status, "invalid_session", err.Error())
		return
	}
	idempotencyKey, ok := requiredIdempotencyKey(writer, request)
	if !ok {
		return
	}
	var input createSessionRequest
	raw, err := decodeBody(request, &input)
	if err != nil {
		writeError(writer, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if input.BankID == uuid.Nil || input.BankVersionID == uuid.Nil || !validPracticeMode(input.Mode) || input.Mode == "favorites" {
		writeError(writer, http.StatusBadRequest, "invalid_request", "bank IDs and random, difficult, or chapter mode are required")
		return
	}
	if input.Mode == "chapter" && strings.TrimSpace(input.ChapterID) == "" {
		writeError(writer, http.StatusBadRequest, "invalid_request", "chapter mode requires chapter_id")
		return
	}
	if input.QuestionCount == 0 {
		input.QuestionCount = 20
	}
	if input.QuestionCount < 1 || input.QuestionCount > 500 {
		writeError(writer, http.StatusBadRequest, "invalid_request", "question_count must be between 1 and 500")
		return
	}
	requestHash := hashCanonical(raw)
	tx, err := service.database.BeginTx(request.Context(), pgx.TxOptions{})
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft is temporarily unavailable")
		return
	}
	defer func() { _ = tx.Rollback(request.Context()) }()
	queries := store.New(tx)
	if err := lockIdempotency(request.Context(), queries, actor.key, "create_practice_session", idempotencyKey); err != nil {
		writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft is temporarily unavailable")
		return
	}
	if storedStatus, storedBody, found, conflict, err := loadIdempotency(request.Context(), queries, actor.key, "create_practice_session", idempotencyKey, requestHash); err != nil {
		writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft is temporarily unavailable")
		return
	} else if conflict {
		writeError(writer, http.StatusConflict, "idempotency_conflict", "idempotency key was already used with another request")
		return
	} else if found {
		writeRawJSON(writer, storedStatus, storedBody)
		return
	}

	if _, err := queries.IsPublishedBankVersion(request.Context(), store.IsPublishedBankVersionParams{ID: input.BankID, ID_2: input.BankVersionID}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(writer, http.StatusBadRequest, "bank_version_unavailable", "bank version is not published")
		} else {
			writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft is temporarily unavailable")
		}
		return
	}
	sessionID := uuid.New()
	questions, err := selectPracticeQuestions(request.Context(), queries, input, sessionID)
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft is temporarily unavailable")
		return
	}
	if len(questions) == 0 {
		writeError(writer, http.StatusBadRequest, "empty_selection", "no questions match this practice selection")
		return
	}
	if err = queries.CreatePracticeSession(request.Context(), store.CreatePracticeSessionParams{ID: sessionID, BankID: input.BankID, BankVersionID: input.BankVersionID, UserID: nullableUUID(actor.userID), ActorKey: actor.key, Mode: input.Mode, ChapterID: nullableString(input.ChapterID)}); err != nil {
		writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft is temporarily unavailable")
		return
	}
	for position, question := range questions {
		if err = queries.AddPracticeSessionQuestion(request.Context(), store.AddPracticeSessionQuestionParams{SessionID: sessionID, BankID: input.BankID, BankVersionID: input.BankVersionID, QuestionID: question.QuestionID, QuestionVersionID: question.QuestionVersionID, Position: int32(position + 1)}); err != nil {
			writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft is temporarily unavailable")
			return
		}
	}
	response := responseEnvelope{RequestID: requestID(), Data: practiceSession{SessionID: sessionID, BankID: input.BankID, BankVersionID: input.BankVersionID, Mode: input.Mode, ExcludedUnavailableCount: 0, Questions: questions}}
	encoded, _ := json.Marshal(response)
	if err := storeIdempotency(request.Context(), queries, actor.key, "create_practice_session", idempotencyKey, requestHash, http.StatusCreated, encoded, sessionID); err != nil {
		writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft is temporarily unavailable")
		return
	}
	if err := tx.Commit(request.Context()); err != nil {
		writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft is temporarily unavailable")
		return
	}
	writeRawJSON(writer, http.StatusCreated, encoded)
}

func selectPracticeQuestions(ctx context.Context, queries *store.Queries, input createSessionRequest, sessionID uuid.UUID) ([]practiceQuestion, error) {
	rows, err := queries.ListPracticeQuestions(ctx, store.ListPracticeQuestionsParams{BankVersionID: input.BankVersionID, ChapterID: strings.TrimSpace(input.ChapterID), Difficult: input.Mode == "difficult", SessionID: sessionID, QuestionCount: int32(input.QuestionCount)})
	if err != nil {
		return nil, err
	}
	questions := make([]practiceQuestion, 0, input.QuestionCount)
	for _, row := range rows {
		question := practiceQuestion{QuestionID: row.QuestionID, QuestionVersionID: row.QuestionVersionID, Type: row.Type, ChapterID: row.ChapterID, Chapter: row.ChapterName, Content: row.Content}
		if question.Type == "single" || question.Type == "multi" {
			if err := json.Unmarshal(row.Options, &question.Options); err != nil {
				return nil, err
			}
		}
		questions = append(questions, question)
	}
	return questions, nil
}

func (service *practiceHTTP) submitAnswer(writer http.ResponseWriter, request *http.Request) {
	actor, status, err := service.actor(writer, request)
	if err != nil {
		writeError(writer, status, "invalid_session", err.Error())
		return
	}
	idempotencyKey, ok := requiredIdempotencyKey(writer, request)
	if !ok {
		return
	}
	sessionID, err := uuid.Parse(chi.URLParam(request, "session_id"))
	if err != nil {
		writeError(writer, http.StatusBadRequest, "invalid_session_id", "session_id must be a UUID")
		return
	}
	var input answerSubmissionRequest
	raw, err := decodeBody(request, &input)
	if err != nil || input.QuestionID == uuid.Nil || input.QuestionVersionID == uuid.Nil || !jsonFieldPresent(raw, "answer") {
		writeError(writer, http.StatusBadRequest, "invalid_request", "question IDs and answer are required")
		return
	}
	requestHash := hashCanonical(append([]byte(sessionID.String()+":"), raw...))
	tx, err := service.database.BeginTx(request.Context(), pgx.TxOptions{})
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft is temporarily unavailable")
		return
	}
	defer func() { _ = tx.Rollback(request.Context()) }()
	queries := store.New(tx)
	if err = queries.LockSubmission(request.Context(), sessionID.String()); err != nil {
		writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft is temporarily unavailable")
		return
	}
	sessionActorKey, err := queries.GetPracticeSessionActor(request.Context(), sessionID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(writer, http.StatusNotFound, "session_not_found", "practice session does not exist")
		} else {
			writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft is temporarily unavailable")
		}
		return
	}
	ownerMatches := sessionActorKey == actor.key
	if !ownerMatches && actor.userID != nil {
		if anonymousCookie, cookieErr := request.Cookie("quizcraft_anonymous"); cookieErr == nil {
			if anonymousSubject, parseErr := service.parseSubject(anonymousCookie.Value, "quizcraft-anonymous"); parseErr == nil {
				guestActorKey := "guest:" + anonymousSubject.String()
				if guestActorKey == sessionActorKey {
					claimed, claimErr := queries.ClaimGuestPracticeSession(request.Context(), store.ClaimGuestPracticeSessionParams{ID: sessionID, GuestActorKey: guestActorKey, UserID: *actor.userID, UserActorKey: actor.key})
					if claimErr != nil {
						writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft is temporarily unavailable")
						return
					}
					ownerMatches = claimed == 1
				}
			}
		}
	}
	if !ownerMatches {
		writeError(writer, http.StatusForbidden, "session_owner_mismatch", "practice session belongs to another actor")
		return
	}
	question, err := queries.GetSessionQuestion(request.Context(), store.GetSessionQuestionParams{ID: sessionID, QuestionID: input.QuestionID, QuestionVersionID: input.QuestionVersionID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(writer, http.StatusBadRequest, "question_not_in_session", "question version is not part of this session")
		} else {
			writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft is temporarily unavailable")
		}
		return
	}
	if storedStatus, storedBody, found, conflict, err := loadIdempotency(request.Context(), queries, actor.key, "submit_practice_answer", idempotencyKey, requestHash); err != nil {
		writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft is temporarily unavailable")
		return
	} else if conflict {
		writeError(writer, http.StatusConflict, "idempotency_conflict", "idempotency key was already used with another request")
		return
	} else if found {
		writeRawJSON(writer, storedStatus, storedBody)
		return
	}
	var expected any
	if err := json.Unmarshal(question.Answer, &expected); err != nil {
		writeError(writer, http.StatusServiceUnavailable, "invalid_question", "stored question answer is invalid")
		return
	}
	var options []any
	if (question.Type == "single" || question.Type == "multi") && json.Unmarshal(question.Options, &options) != nil {
		writeError(writer, http.StatusServiceUnavailable, "invalid_question", "stored question options are invalid")
		return
	}
	var existingBody []byte
	if storedBody, attemptErr := queries.GetPracticeAttemptResponse(request.Context(), store.GetPracticeAttemptResponseParams{SessionID: sessionID, QuestionID: input.QuestionID}); attemptErr == nil {
		existingBody = []byte(storedBody)
		body := markReplayed(existingBody)
		if err := storeIdempotency(request.Context(), queries, actor.key, "submit_practice_answer", idempotencyKey, requestHash, http.StatusOK, body, sessionID); err != nil {
			writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft is temporarily unavailable")
			return
		}
		if err := tx.Commit(request.Context()); err != nil {
			writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft is temporarily unavailable")
			return
		}
		writeRawJSON(writer, http.StatusOK, body)
		return
	} else if !errors.Is(attemptErr, pgx.ErrNoRows) {
		writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft is temporarily unavailable")
		return
	}

	correct := scoreAnswer(question.Type, input.Answer, expected, options)
	result := answerResult{QuestionID: input.QuestionID, QuestionVersionID: input.QuestionVersionID, Correct: correct, Replayed: false, ExpectedAnswer: expected, Analysis: question.Analysis}
	response := responseEnvelope{RequestID: requestID(), Data: result}
	encoded, _ := json.Marshal(response)
	answerJSON, _ := json.Marshal(input.Answer)
	attemptID := uuid.New()
	attemptSubmittedAt, err := queries.CreatePracticeAttempt(request.Context(), store.CreatePracticeAttemptParams{ID: attemptID, SessionID: sessionID, BankID: question.BankID, BankVersionID: question.BankVersionID, QuestionID: input.QuestionID, QuestionVersionID: input.QuestionVersionID, UserID: nullableUUID(actor.userID), SubmittedAnswer: answerJSON, Correct: correct, ExpectedAnswer: question.Answer, Analysis: question.Analysis, ResponseBody: string(encoded)})
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft is temporarily unavailable")
		return
	}
	if err = queries.UpdateQuestionStats(request.Context(), store.UpdateQuestionStatsParams{QuestionID: input.QuestionID, CorrectCount: boolInt(correct)}); err != nil {
		writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft is temporarily unavailable")
		return
	}
	if actor.userID != nil {
		if err = queries.UpdateLearningState(request.Context(), store.UpdateLearningStateParams{UserID: *actor.userID, BankID: question.BankID, QuestionID: input.QuestionID, QuestionVersionID: input.QuestionVersionID, Wrong: !correct, CorrectCount: boolInt(correct), LatestAttemptID: attemptID, UpdatedAt: attemptSubmittedAt}); err != nil {
			writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft is temporarily unavailable")
			return
		}
	}
	if err := storeIdempotency(request.Context(), queries, actor.key, "submit_practice_answer", idempotencyKey, requestHash, http.StatusOK, encoded, sessionID); err != nil {
		writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft is temporarily unavailable")
		return
	}
	if err := tx.Commit(request.Context()); err != nil {
		writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft is temporarily unavailable")
		return
	}
	writeRawJSON(writer, http.StatusOK, encoded)
	go service.compareLegacy(context.Background(), sessionID, input.QuestionID, question.BankKey, question.SourceQuestionID, input.Answer, result)
}

func (service *practiceHTTP) learningState(writer http.ResponseWriter, request *http.Request) {
	actor, status, err := service.actor(writer, request)
	if err != nil {
		writeError(writer, status, "invalid_session", err.Error())
		return
	}
	if actor.userID == nil {
		writeError(writer, http.StatusUnauthorized, "authentication_required", "sign in to read persistent learning state")
		return
	}
	rows, err := service.queries.ListLearningState(request.Context(), *actor.userID)
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft is temporarily unavailable")
		return
	}
	items := make([]map[string]any, 0)
	for _, row := range rows {
		if !row.UpdatedAt.Valid {
			writeError(writer, http.StatusServiceUnavailable, "invalid_learning_state", "stored learning state timestamp is invalid")
			return
		}
		items = append(items, map[string]any{"bank_id": row.BankID, "question_id": row.QuestionID, "question_version_id": row.QuestionVersionID, "wrong": row.Wrong, "attempt_count": row.AttemptCount, "correct_count": row.CorrectCount, "updated_at": row.UpdatedAt.Time.UTC().Format(time.RFC3339Nano)})
	}
	writeJSON(writer, http.StatusOK, responseEnvelope{RequestID: requestID(), Data: items})
}

func (service *practiceHTTP) personalStats(writer http.ResponseWriter, request *http.Request) {
	userID, err := portalActorUserID(request)
	if err != nil {
		writeError(writer, http.StatusUnauthorized, "authentication_required", "sign in to read persistent Practice statistics")
		return
	}
	requestID := serviceRequestIDFromHeader(request)
	writer.Header().Set("X-Request-Id", requestID)

	tx, err := service.database.BeginTx(request.Context(), pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft statistics are temporarily unavailable")
		return
	}
	defer func() { _ = tx.Rollback(request.Context()) }()

	queries := store.New(tx)
	actor := uuid.NullUUID{UUID: userID, Valid: true}
	totals, err := queries.GetPersonalPracticeTotals(request.Context(), actor)
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft statistics are temporarily unavailable")
		return
	}
	rows, err := queries.ListPersonalMasteryFacts(request.Context(), actor)
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft statistics are temporarily unavailable")
		return
	}
	days, err := queries.ListPersonalPracticeDays(request.Context(), actor)
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft statistics are temporarily unavailable")
		return
	}
	if err := tx.Commit(request.Context()); err != nil {
		writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft statistics are temporarily unavailable")
		return
	}

	streakDays, err := consecutivePracticeDays(days, service.now())
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, "invalid_practice_statistics", "stored Practice activity day is invalid")
		return
	}
	stats := personalPracticeStats{
		TotalAnswers:   totals.TotalAnswers,
		CorrectAnswers: totals.CorrectAnswers,
		Accuracy:       roundedPercent(totals.CorrectAnswers, totals.TotalAnswers),
		StreakDays:     streakDays,
		Mastery:        make([]masterySubject, 0, len(rows)),
	}
	for _, row := range rows {
		label := strings.TrimSpace(row.Label)
		if label == "" || row.TotalQuestions < 0 || row.CorrectQuestions < 0 {
			writeError(writer, http.StatusServiceUnavailable, "invalid_practice_statistics", "stored Practice mastery is invalid")
			return
		}
		stats.Mastery = append(stats.Mastery, masterySubject{
			BankID:           row.BankID,
			Label:            label,
			Value:            roundedPercent(row.CorrectQuestions, row.TotalQuestions),
			TotalQuestions:   row.TotalQuestions,
			CorrectQuestions: row.CorrectQuestions,
		})
	}
	writeJSON(writer, http.StatusOK, responseEnvelope{RequestID: requestID, Data: stats})
}

// portalActorUserID is called only after authenticatePortalPersonalStats has
// verified a six-part signature over this exact header. It intentionally
// refuses all guest, nil, duplicate, and malformed values before stats query
// execution.
func portalActorUserID(request *http.Request) (uuid.UUID, error) {
	values := request.Header.Values("X-Actor-User-Id")
	if len(values) != 1 || values[0] == "" || strings.TrimSpace(values[0]) != values[0] || values[0] == "anonymous" {
		return uuid.Nil, errors.New("portal actor is not signed in")
	}
	userID, err := uuid.Parse(values[0])
	if err != nil || userID == uuid.Nil {
		return uuid.Nil, errors.New("portal actor user ID is invalid")
	}
	return userID, nil
}

var practiceStatsLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

func consecutivePracticeDays(days []string, now time.Time) (int, error) {
	if len(days) == 0 {
		return 0, nil
	}
	today := midnightInPracticeStatsLocation(now)
	expected := today
	first, err := time.ParseInLocation(time.DateOnly, days[0], practiceStatsLocation)
	if err != nil {
		return 0, err
	}
	if first.Before(today) {
		expected = today.AddDate(0, 0, -1)
	}
	streak := 0
	for _, raw := range days {
		day, err := time.ParseInLocation(time.DateOnly, raw, practiceStatsLocation)
		if err != nil {
			return 0, err
		}
		if !day.Equal(expected) {
			break
		}
		streak++
		expected = expected.AddDate(0, 0, -1)
	}
	return streak, nil
}

func midnightInPracticeStatsLocation(value time.Time) time.Time {
	local := value.In(practiceStatsLocation)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, practiceStatsLocation)
}

func roundedPercent(numerator, denominator int64) int {
	if numerator <= 0 || denominator <= 0 {
		return 0
	}
	value := (numerator*100 + denominator/2) / denominator
	if value > 100 {
		return 100
	}
	return int(value)
}

func (service *practiceHTTP) actor(writer http.ResponseWriter, request *http.Request) (practiceActor, int, error) {
	if identity, ok := portalPracticeCommandIdentityFromContext(request.Context()); ok {
		if identity.userID != nil {
			return practiceActor{userID: identity.userID, key: "user:" + identity.userID.String()}, 0, nil
		}
		// A five-part Portal command is a guest command. In particular, ignore
		// its Basic service credential and any unrelated browser session header;
		// only QuizCraft's own anonymous cookie can establish the guest actor.
		return service.anonymousActor(writer, request)
	}
	if service.sessionCodec != nil {
		if cookie, err := request.Cookie("__Host-quizcraft_session"); err == nil {
			var session localPlatformSession
			if err := service.sessionCodec.decode(cookie.Value, "quizcraft-platform-session-v1", &session); err != nil || service.now().After(session.ExpiresAt) || len(session.ExchangeToken) < 32 {
				return practiceActor{}, http.StatusUnauthorized, errors.New("platform Core session is invalid or expired")
			}
			userID, err := uuid.Parse(session.UserID)
			if err != nil {
				return practiceActor{}, http.StatusUnauthorized, errors.New("platform Core session user is invalid")
			}
			return practiceActor{userID: &userID, key: "user:" + userID.String(), exchangeToken: session.ExchangeToken}, 0, nil
		}
	}
	header := strings.TrimSpace(request.Header.Get("Authorization"))
	if header != "" {
		if !strings.HasPrefix(header, "Bearer ") {
			return practiceActor{}, http.StatusUnauthorized, errors.New("authorization must use Bearer")
		}
		return service.signedInActor(strings.TrimSpace(strings.TrimPrefix(header, "Bearer ")))
	}
	if cookie, err := request.Cookie("quizcraft_session"); err == nil {
		return service.signedInActor(cookie.Value)
	}
	return service.anonymousActor(writer, request)
}

func (service *practiceHTTP) anonymousActor(writer http.ResponseWriter, request *http.Request) (practiceActor, int, error) {
	if cookie, err := request.Cookie("quizcraft_anonymous"); err == nil {
		if subject, parseErr := service.parseSubject(cookie.Value, "quizcraft-anonymous"); parseErr == nil {
			return practiceActor{key: "guest:" + subject.String()}, 0, nil
		}
	}
	subject := uuid.New()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": subject.String(), "iss": "quizcraft-anonymous", "aud": "quizcraft",
		"exp": service.now().Add(90 * 24 * time.Hour).Unix(),
	})
	signed, err := token.SignedString(service.authHMACSecret)
	if err != nil {
		return practiceActor{}, http.StatusServiceUnavailable, errors.New("anonymous session could not be created")
	}
	http.SetCookie(writer, &http.Cookie{Name: "quizcraft_anonymous", Value: signed, Path: "/", MaxAge: 90 * 24 * 60 * 60, HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode})
	return practiceActor{key: "guest:" + subject.String()}, 0, nil
}

func (service *practiceHTTP) signedInActor(tokenText string) (practiceActor, int, error) {
	claims, userID, err := service.parseClaims(tokenText, "quizcraft-session")
	if err != nil {
		return practiceActor{}, http.StatusUnauthorized, errors.New("invalid or expired QuizCraft session")
	}
	actor := practiceActor{userID: &userID, key: "user:" + userID.String(), permissions: map[string]bool{}}
	if values, ok := claims["permissions"].([]any); ok {
		for _, value := range values {
			if permission, ok := value.(string); ok {
				actor.permissions[permission] = true
			}
		}
	}
	if values, ok := claims["scopes"].([]any); ok {
		for _, value := range values {
			item, ok := value.(map[string]any)
			if !ok {
				continue
			}
			actor.scopes = append(actor.scopes, workshopScope{Kind: stringClaim(item, "kind"), ProductCode: stringClaim(item, "product_code"), ResourceType: stringClaim(item, "resource_type"), ResourceID: stringClaim(item, "resource_id")})
		}
	}
	return actor, 0, nil
}

func (service *practiceHTTP) parseSubject(tokenText, issuer string) (uuid.UUID, error) {
	_, userID, err := service.parseClaims(tokenText, issuer)
	return userID, err
}

func (service *practiceHTTP) parseClaims(tokenText, issuer string) (jwt.MapClaims, uuid.UUID, error) {
	claims := jwt.MapClaims{}
	token, err := jwt.ParseWithClaims(tokenText, claims, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, errors.New("unexpected signing method")
		}
		return service.authHMACSecret, nil
	}, jwt.WithAudience("quizcraft"), jwt.WithIssuer(issuer), jwt.WithExpirationRequired(), jwt.WithValidMethods([]string{"HS256"}))
	if err != nil || !token.Valid {
		return nil, uuid.Nil, errors.New("invalid signed session")
	}
	subject, err := claims.GetSubject()
	if err != nil {
		return nil, uuid.Nil, errors.New("signed session has no subject")
	}
	userID, err := uuid.Parse(subject)
	if err != nil {
		return nil, uuid.Nil, errors.New("signed session subject must be a UUID")
	}
	return claims, userID, nil
}

func stringClaim(values map[string]any, key string) string {
	value, _ := values[key].(string)
	return value
}

func (service *practiceHTTP) compareLegacy(ctx context.Context, sessionID, questionID uuid.UUID, bankKey, sourceQuestionID string, answer any, result answerResult) {
	if service.legacyBaseURL == "" {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	payload, _ := json.Marshal(map[string]any{"bank": bankKey, "question_id": sourceQuestionID, "answer": answer})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, service.legacyBaseURL+"/api/practice/shadow-compare", bytes.NewReader(payload))
	if err != nil {
		return
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-QuizCraft-Shadow-Secret", service.legacyCompareSecret)
	response, err := service.httpClient.Do(request)
	var legacy any
	outcome := "legacy_error"
	detail := ""
	if err != nil {
		detail = err.Error()
	} else {
		defer response.Body.Close()
		decoder := json.NewDecoder(io.LimitReader(response.Body, 1<<20))
		var legacyMap map[string]any
		if response.StatusCode != http.StatusOK || decoder.Decode(&legacyMap) != nil {
			detail = fmt.Sprintf("legacy status %d or invalid JSON", response.StatusCode)
		} else {
			legacy = legacyMap
			legacyCorrect, _ := legacyMap["correct"].(bool)
			if legacyCorrect == result.Correct && equalJSON(legacyMap["correct_answer"], result.ExpectedAnswer) {
				outcome = "match"
			} else {
				outcome = "mismatch"
				detail = "correct or expected answer differs"
			}
		}
	}
	newJSON, _ := json.Marshal(result)
	legacyJSON, _ := json.Marshal(legacy)
	_ = service.queries.InsertShadowComparison(ctx, store.InsertShadowComparisonParams{ID: uuid.New(), SessionID: sessionID, QuestionID: questionID, NewResponse: newJSON, LegacyResponse: nullableJSON(legacyJSON, legacy), Outcome: outcome, Detail: detail})
}

func scoreAnswer(kind string, submitted, expected any, options []any) bool {
	switch kind {
	case "judge":
		left, leftOK := normalizeJudge(submitted)
		right, rightOK := normalizeJudge(expected)
		return leftOK && rightOK && left == right
	case "blank":
		left := normalizeBlank(submitted)
		if left == "" {
			return false
		}
		if candidates, ok := expected.([]any); ok {
			for _, candidate := range candidates {
				if left == normalizeBlank(candidate) {
					return true
				}
			}
			return false
		}
		return left == normalizeBlank(expected)
	case "multi":
		left, leftOK := submitted.([]any)
		right, rightOK := expected.([]any)
		if !leftOK || !rightOK || len(left) != len(right) {
			return false
		}
		leftValues, leftOK := canonicalChoiceValues(left, options)
		rightValues, rightOK := canonicalChoiceValues(right, options)
		if !leftOK || !rightOK {
			return false
		}
		return strings.Join(leftValues, "\x00") == strings.Join(rightValues, "\x00")
	case "single":
		left, leftOK := canonicalChoiceValue(submitted, options)
		right, rightOK := canonicalChoiceValue(expected, options)
		return leftOK && rightOK && left == right
	default:
		return equalJSON(submitted, expected)
	}
}

func normalizeJudge(value any) (bool, bool) {
	switch item := value.(type) {
	case bool:
		return item, true
	case float64:
		if item == 1 {
			return true, true
		}
		if item == 0 {
			return false, true
		}
	case string:
		normalized := strings.ToLower(strings.TrimSpace(item))
		for _, candidate := range []string{"true", "t", "1", "yes", "y", "right", "对", "正确", "是", "√"} {
			if normalized == candidate {
				return true, true
			}
		}
		for _, candidate := range []string{"false", "f", "0", "no", "n", "wrong", "错", "错误", "否", "×"} {
			if normalized == candidate {
				return false, true
			}
		}
	}
	return false, false
}

func normalizeBlank(value any) string {
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.ToLower(strings.Join(strings.FieldsFunc(text, unicode.IsSpace), " "))
}

func canonicalChoiceValues(values, options []any) ([]string, bool) {
	result := make([]string, 0, len(values))
	for _, value := range values {
		canonical, ok := canonicalChoiceValue(value, options)
		if !ok {
			return nil, false
		}
		result = append(result, canonical)
	}
	sort.Strings(result)
	return result, true
}

func canonicalChoiceValue(value any, options []any) (string, bool) {
	if number, ok := value.(float64); ok && number >= 0 && number == float64(int(number)) && int(number) < len(options) {
		return fmt.Sprintf("%d", int(number)), true
	}
	if selected, ok := value.(string); ok {
		for index, option := range options {
			if text, textOK := option.(string); textOK && text == selected {
				return fmt.Sprintf("%d", index), true
			}
		}
	}
	return "", false
}

func equalJSON(left, right any) bool {
	leftJSON, _ := json.Marshal(left)
	rightJSON, _ := json.Marshal(right)
	return bytes.Equal(leftJSON, rightJSON)
}

func markReplayed(body []byte) []byte {
	var envelope map[string]any
	if json.Unmarshal(body, &envelope) != nil {
		return body
	}
	envelope["request_id"] = requestID()
	if data, ok := envelope["data"].(map[string]any); ok {
		data["replayed"] = true
	}
	encoded, _ := json.Marshal(envelope)
	return encoded
}

func requiredIdempotencyKey(writer http.ResponseWriter, request *http.Request) (string, bool) {
	key := strings.TrimSpace(request.Header.Get("Idempotency-Key"))
	if len(key) < 16 || len(key) > 160 {
		writeError(writer, http.StatusBadRequest, "invalid_idempotency_key", "Idempotency-Key must contain 16 to 160 characters")
		return "", false
	}
	return key, true
}

func lockIdempotency(ctx context.Context, queries *store.Queries, actorKey, kind, key string) error {
	return queries.LockIdempotency(ctx, actorKey+":"+kind+":"+key)
}

func loadIdempotency(ctx context.Context, queries *store.Queries, actorKey, kind, key, requestHash string) (int, []byte, bool, bool, error) {
	result, err := queries.GetIdempotencyResult(ctx, store.GetIdempotencyResultParams{ActorKey: actorKey, OperationKind: kind, IdempotencyKey: key})
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil, false, false, nil
	}
	if err != nil {
		return 0, nil, false, false, err
	}
	return int(result.ResponseStatus), []byte(result.ResponseBody), true, result.RequestSha256 != requestHash, nil
}

func storeIdempotency(ctx context.Context, queries *store.Queries, actorKey, kind, key, requestHash string, status int, body []byte, resourceID uuid.UUID) error {
	return queries.StoreIdempotencyResult(ctx, store.StoreIdempotencyResultParams{ActorKey: actorKey, OperationKind: kind, IdempotencyKey: key, RequestSha256: requestHash, ResponseStatus: int32(status), ResponseBody: string(body), ResourceID: uuid.NullUUID{UUID: resourceID, Valid: true}})
}

func decodeBody(request *http.Request, target any) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(request.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if err := decoder.Decode(target); err != nil {
		return nil, err
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return nil, err
	}
	return raw, nil
}

func jsonFieldPresent(raw []byte, field string) bool {
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) != nil {
		return false
	}
	_, present := object[field]
	return present
}

func jsonStringFieldPresent(raw []byte, field string) bool {
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) != nil {
		return false
	}
	value, present := object[field]
	if !present {
		return false
	}
	var decoded any
	if json.Unmarshal(value, &decoded) != nil {
		return false
	}
	_, isString := decoded.(string)
	return isString
}

func hashCanonical(raw []byte) string {
	var value any
	if json.Unmarshal(raw, &value) == nil {
		raw, _ = json.Marshal(value)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func validPracticeMode(mode string) bool {
	return mode == "random" || mode == "difficult" || mode == "chapter" || mode == "favorites"
}

func nullableUUID(value *uuid.UUID) uuid.NullUUID {
	if value == nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: *value, Valid: true}
}

func nullableString(value string) pgtype.Text {
	if strings.TrimSpace(value) == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: strings.TrimSpace(value), Valid: true}
}

func nullableJSON(encoded []byte, value any) []byte {
	if value == nil {
		return nil
	}
	return encoded
}

func boolInt(value bool) int64 {
	if value {
		return 1
	}
	return 0
}

func requestID() string {
	return "req_" + strings.ReplaceAll(uuid.NewString(), "-", "")
}

func serviceRequestIDFromHeader(request *http.Request) string {
	value := strings.TrimSpace(request.Header.Get("X-Request-Id"))
	if len(value) <= 120 && serviceRequestIDPattern.MatchString(value) {
		return value
	}
	return requestID()
}

func writeError(writer http.ResponseWriter, status int, code, message string) {
	response := errorEnvelope{RequestID: requestID()}
	response.Error.Code = code
	response.Error.Message = message
	writeJSON(writer, status, response)
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	encoded, _ := json.Marshal(value)
	writeRawJSON(writer, status, encoded)
}

func writeRawJSON(writer http.ResponseWriter, status int, encoded []byte) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_, _ = writer.Write(encoded)
}
