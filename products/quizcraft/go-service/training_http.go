package quizcraft

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"henukit.dev/quizcraft/internal/store"
	tc "henukit.dev/quizcraft/internal/trainingcontract"
)

const trainingPrefix = "/api/v1/portal/practice/training"

type trainingWriter struct {
	http.ResponseWriter
	status    int
	requestID string
}

func (w *trainingWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}
func (w *trainingWriter) Write(raw []byte) (int, error) {
	if w.status >= 400 {
		var envelope struct {
			RequestID string `json:"request_id"`
			Error     struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(raw, &envelope) == nil {
			code := strings.ToUpper(envelope.Error.Code)
			if !tc.TrainingErrorCode(code).Valid() {
				switch w.status {
				case 400, 422:
					code = "INVALID_REQUEST"
				case 401:
					code = "UNAUTHENTICATED"
				case 403:
					code = "FORBIDDEN"
				case 404:
					code = "NOT_FOUND"
				case 503:
					code = "DEPENDENCY_UNAVAILABLE"
				default:
					code = "INVALID_REQUEST"
				}
			}
			envelope.Error.Code = code
			envelope.RequestID = w.requestID
			raw, _ = json.Marshal(envelope)
		}
	}
	return w.ResponseWriter.Write(raw)
}
func trainingBoundary(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := serviceRequestIDFromHeader(r)
		r.Header.Set("X-Request-Id", id)
		w.Header().Set("X-Request-Id", id)
		w.Header().Set("Cache-Control", "private, no-store")
		w.Header().Set("Content-Type", "application/json; charset=utf-8")

		if len(r.Header.Values("X-Actor-User-Id")) > 0 {
			if _, err := portalActorUserID(r); err != nil {
				writeError(&trainingWriter{ResponseWriter: w, requestID: id}, 401, "UNAUTHENTICATED", "Training identity is invalid")
				return
			}
		}
		if r.Method == http.MethodPost {
			value, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if err != nil || value != "application/json" {
				writeError(&trainingWriter{ResponseWriter: w, requestID: id}, 400, "INVALID_REQUEST", "Training commands require JSON")
				return
			}
		}
		next.ServeHTTP(&trainingWriter{ResponseWriter: w, requestID: id}, r)
	})
}
func (service *practiceHTTP) registerTraining(router chi.Router) {
	router.Get("/practice-media/{digest}.mp3", serveTrainingMedia)
	router.Route(trainingPrefix, func(r chi.Router) {
		r.Use(trainingBoundary)
		r.With(service.authenticatePortalRead).Get("/catalog", service.trainingCatalog)
		private := r.With(func(next http.Handler) http.Handler {
			return service.authenticateSignedGET(serviceAuthRequirement{clientID: service.catalogClientID, keys: service.catalogKeys, requiredPermission: "portal.practice.read", optionalActor: true, label: "training", unavailableCode: "TRAINING_DISABLED"}, next)
		})
		private.Get("/sessions/{session_id}", service.recoverTraining)
		private.Get("/sessions/{session_id}/review", service.reviewTraining)
		private.Get("/review-items", service.trainingReviewItems)
		commands := r.With(service.trainingWriteGate, service.authenticatePortalCommand)
		commands.Post("/sessions", service.createTraining)
		commands.Post("/sessions/{session_id}/answers", service.answerTraining)
		commands.Post("/sessions/{session_id}/hints", service.hintTraining)
		commands.Post("/sessions/{session_id}/reveals", service.revealTraining)
	})
}
func (service *practiceHTTP) trainingWriteGate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !service.portalCommandsEnabled || service.writesDisabled || service.portalCommandClientID == "" {
			writeError(w, 503, "TRAINING_DISABLED", "Training saves are unavailable; try again later")
			return
		}
		next.ServeHTTP(w, r)
	})
}
func noTrainingQuery(w http.ResponseWriter, r *http.Request) bool {
	if r.URL.RawQuery != "" || r.URL.ForceQuery {
		writeError(w, 400, "INVALID_REQUEST", "This training request does not accept query parameters")
		return false
	}
	return true
}
func trainingUnavailable(w http.ResponseWriter) {
	writeError(w, 503, "DEPENDENCY_UNAVAILABLE", "Training is temporarily unavailable; try again")
}
func trainingNotFound(w http.ResponseWriter) {
	writeError(w, 404, "NOT_FOUND", "Training session was not found")
}

func (service *practiceHTTP) trainingCatalog(w http.ResponseWriter, r *http.Request) {
	if !noTrainingQuery(w, r) {
		return
	}
	if r.Header.Get("X-Actor-User-Id") != "" {
		writeError(w, 400, "INVALID_REQUEST", "Catalog reads do not accept an actor")
		return
	}
	rows, err := service.database.Query(r.Context(), `SELECT t.document FROM quizcraft_training_unit_versions t JOIN quizcraft_banks b ON b.id=t.bank_id AND b.active_version_id=t.bank_version_id ORDER BY b.name,t.id`)
	if err != nil {
		trainingUnavailable(w)
		return
	}
	defer rows.Close()
	catalog := tc.TrainingCatalog{Units: []tc.TrainingCatalogItem{}}
	for rows.Next() {
		var raw []byte
		var version trainingVersion
		if rows.Scan(&raw) != nil || func() bool { var err error; version, err = decodeTrainingVersion(raw); return err != nil }() {
			trainingUnavailable(w)
			return
		}
		u := version.Unit
		catalog.Units = append(catalog.Units, tc.TrainingCatalogItem{UnitId: u.UnitId, UnitVersionId: u.UnitVersionId, BankId: u.BankId, BankVersionId: u.BankVersionId, Kind: tc.TrainingKind(u.Kind), Title: u.Title, QuestionCount: len(u.Questions), ContentSha256: u.ContentSha256, Attributions: u.Attributions})
	}
	if rows.Err() != nil {
		trainingUnavailable(w)
		return
	}
	writeJSON(w, 200, responseEnvelope{RequestID: serviceRequestIDFromHeader(r), Data: catalog})
}

// Private reads require existing ownership; this function never creates a cookie.
func (service *practiceHTTP) trainingActor(r *http.Request) (practiceActor, bool) {
	if actor := r.Header.Get("X-Actor-User-Id"); actor != "" {
		id, err := uuid.Parse(actor)
		return practiceActor{key: "user:" + id.String(), userID: &id}, err == nil
	}
	if cookie, err := r.Cookie("quizcraft_anonymous"); err == nil {
		if id, err := service.parseSubject(cookie.Value, "quizcraft-anonymous"); err == nil {
			return practiceActor{key: "guest:" + id.String()}, true
		}
	}
	return practiceActor{}, false
}
func (service *practiceHTTP) trainingOwner(ctx context.Context, tx pgx.Tx, r *http.Request, session uuid.UUID, actor practiceActor, claim bool) (bool, error) {
	queries := store.New(tx)
	owner, err := queries.GetPracticeSessionActor(ctx, session)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if owner == actor.key {
		return true, nil
	}
	if actor.userID == nil || !strings.HasPrefix(owner, "guest:") {
		return false, nil
	}
	cookie, err := r.Cookie("quizcraft_anonymous")
	if err != nil {
		return false, nil
	}
	id, err := service.parseSubject(cookie.Value, "quizcraft-anonymous")
	if err != nil || owner != "guest:"+id.String() {
		return false, nil
	}
	if !claim {
		return true, nil
	}
	count, err := queries.ClaimGuestPracticeSession(ctx, store.ClaimGuestPracticeSessionParams{ID: session, GuestActorKey: owner, UserID: *actor.userID, UserActorKey: actor.key})
	return count == 1, err
}
func loadTrainingVersion(ctx context.Context, tx pgx.Tx, version uuid.UUID) (trainingVersion, error) {
	var raw []byte
	if err := tx.QueryRow(ctx, `SELECT document FROM quizcraft_training_unit_versions WHERE id=$1`, version).Scan(&raw); err != nil {
		return trainingVersion{}, err
	}
	return decodeTrainingVersion(raw)
}
func trainingPhase(saved, total int) tc.TrainingPhase {
	if saved == 0 {
		return "pending"
	}
	if saved == total {
		return "completed"
	}
	return "partial"
}
func trainingSnapshot(ctx context.Context, tx pgx.Tx, session uuid.UUID, actor practiceActor) (tc.TrainingSession, trainingVersion, error) {
	var versionID uuid.UUID
	err := tx.QueryRow(ctx, `SELECT unit_version_id FROM quizcraft_training_sessions WHERE session_id=$1`, session).Scan(&versionID)
	if err != nil {
		return tc.TrainingSession{}, trainingVersion{}, err
	}
	version, err := loadTrainingVersion(ctx, tx, versionID)
	if err != nil {
		return tc.TrainingSession{}, version, err
	}
	result := tc.TrainingSession{SessionId: session, TotalCount: len(version.Unit.Questions), HintDisclosures: []tc.TrainingHintDisclosure{}, Receipts: []tc.TrainingAnswerReceipt{}, RevealedLayers: []tc.TrainingSessionRevealedLayers{}, RecoveryScope: "browser"}
	owner, ownerErr := store.New(tx).GetPracticeSessionActor(ctx, session)
	if ownerErr != nil {
		return result, version, ownerErr
	}
	if strings.HasPrefix(owner, "user:") {
		result.RecoveryScope = "account"
	}
	if version.Cloze != nil {
		_ = result.Unit.FromTrainingClozeUnit(version.Cloze.Unit)
	} else {
		_ = result.Unit.FromTrainingListeningUnit(version.Listening.Unit)
	}
	rows, err := tx.Query(ctx, `SELECT a.question_id,a.question_version_id,a.submitted_answer,a.correct,a.submitted_at,o.assisted,o.outcome,o.review_required FROM quizcraft_practice_attempts a JOIN quizcraft_training_outcomes o ON o.attempt_id=a.id JOIN quizcraft_practice_session_questions q ON q.session_id=a.session_id AND q.question_id=a.question_id WHERE a.session_id=$1 ORDER BY q.position`, session)
	if err != nil {
		return result, version, err
	}
	defer rows.Close()
	for rows.Next() {
		receipt := tc.TrainingAnswerReceipt{SessionId: session, UnitVersionId: versionID, TotalCount: result.TotalCount, Replayed: true}
		var answer []byte
		if err = rows.Scan(&receipt.QuestionId, &receipt.QuestionVersionId, &answer, &receipt.Correct, &receipt.SubmittedAt, &receipt.Assisted, &receipt.Outcome, &receipt.ReviewRequired); err != nil {
			return result, version, err
		}
		receipt.SubmittedAt = receipt.SubmittedAt.UTC()
		if err = json.Unmarshal(answer, &receipt.Answer); err != nil {
			return result, version, err
		}
		result.Receipts = append(result.Receipts, receipt)
	}
	if err = rows.Err(); err != nil {
		return result, version, err
	}
	result.SavedCount = len(result.Receipts)
	result.Phase = trainingPhase(result.SavedCount, result.TotalCount)
	for i := range result.Receipts {
		result.Receipts[i].SavedCount = result.SavedCount
		result.Receipts[i].Phase = result.Phase
	}
	layers, err := trainingLayers(ctx, tx, session, version)
	if err != nil {
		return result, version, err
	}
	for _, disclosure := range layers {
		language, err := trainingLayerName(disclosure)
		if err != nil {
			return result, version, err
		}
		result.RevealedLayers = append(result.RevealedLayers, tc.TrainingSessionRevealedLayers(language))
	}
	if version.Cloze != nil {
		result.HintDisclosures, err = trainingHints(ctx, tx, session, *version.Cloze)
		if err != nil {
			return result, version, err
		}
	}
	return result, version, nil
}

// Disclosure identity and time are immutable facts; content is always recovered
// from this session's pinned version, never the currently published unit.
func trainingLayers(ctx context.Context, tx pgx.Tx, session uuid.UUID, version trainingVersion) ([]tc.TrainingLayerDisclosure, error) {
	layers := []tc.TrainingLayerDisclosure{}
	rows, err := tx.Query(ctx, `SELECT id,kind,disclosed_at FROM quizcraft_training_disclosures WHERE session_id=$1 AND kind IN ('english','chinese') ORDER BY CASE kind WHEN 'english' THEN 0 ELSE 1 END`, session)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var disclosure tc.TrainingLayerDisclosure
		var language string
		if err := rows.Scan(&disclosure.DisclosureId, &language, &disclosure.DisclosedAt); err != nil {
			return nil, err
		}
		if (len(layers) == 0 && language != "english") || len(layers) >= 2 {
			return nil, errors.New("invalid training language disclosure order")
		}
		if version.Cloze != nil {
			found := false
			for _, content := range version.Cloze.Layers {
				if string(content.Layer) == language {
					_ = disclosure.Content.FromTrainingClozeLayer(content)
					found = true
					break
				}
			}
			if !found {
				return nil, errors.New("disclosed cloze layer is missing")
			}
			disclosure.DisclosedAt = disclosure.DisclosedAt.UTC()
			layers = append(layers, disclosure)
			continue
		}
		var content *tc.TrainingListeningLayer
		for i := range version.Listening.Layers {
			if string(version.Listening.Layers[i].Layer) == language {
				content = &version.Listening.Layers[i]
				break
			}
		}
		if content == nil {
			return nil, errors.New("disclosed training layer is missing from pinned version")
		}
		disclosure.DisclosedAt = disclosure.DisclosedAt.UTC()
		if err := disclosure.Content.FromTrainingListeningLayer(*content); err != nil {
			return nil, err
		}
		layers = append(layers, disclosure)
	}
	return layers, rows.Err()
}
func (service *practiceHTTP) createTraining(w http.ResponseWriter, r *http.Request) {
	if !noTrainingQuery(w, r) {
		return
	}
	var input tc.TrainingCreateSession
	raw, err := decodeBody(r, &input)
	if err != nil || tc.Validate("TrainingCreateSession", raw) != nil {
		writeError(w, 400, "INVALID_REQUEST", "Choose a published training unit")
		return
	}
	key := r.Header.Get("Idempotency-Key")
	if len(key) < 16 || len(key) > 160 {
		writeError(w, 400, "INVALID_REQUEST", "A valid save key is required")
		return
	}
	actor, _, err := service.actor(w, r)
	if err != nil {
		writeError(w, 401, "UNAUTHENTICATED", "Training identity is invalid")
		return
	}
	tx, err := service.database.BeginTx(r.Context(), pgx.TxOptions{})
	if err != nil {
		trainingUnavailable(w)
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()
	queries := store.New(tx)
	kind := "POST " + trainingPrefix + "/sessions"
	if lockIdempotency(r.Context(), queries, actor.key, kind, key) != nil {
		trainingUnavailable(w)
		return
	}
	var sessionID uuid.UUID
	requestHash := hash(raw)
	status, body, found, conflict, err := loadIdempotency(r.Context(), queries, actor.key, kind, key, requestHash)
	_ = status
	if err != nil {
		trainingUnavailable(w)
		return
	}
	if conflict {
		writeError(w, 409, "IDEMPOTENCY_CONFLICT", "This save key was used for a different request")
		return
	}
	if found {
		var prior tc.TrainingSessionEnvelope
		if json.Unmarshal(body, &prior) != nil {
			trainingUnavailable(w)
			return
		}
		sessionID = prior.Data.SessionId
	} else {
		var published bool
		err = tx.QueryRow(r.Context(), `SELECT true FROM quizcraft_training_unit_versions t JOIN quizcraft_banks b ON b.id=t.bank_id AND b.active_version_id=t.bank_version_id WHERE t.id=$1 AND t.unit_id=$2 FOR SHARE OF b`, input.UnitVersionId, input.UnitId).Scan(&published)
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, 409, "VERSION_UNAVAILABLE", "This training version is no longer available; choose a unit again")
			return
		}
		if err != nil {
			trainingUnavailable(w)
			return
		}
		version, err := loadTrainingVersion(r.Context(), tx, input.UnitVersionId)
		if err != nil {
			trainingUnavailable(w)
			return
		}
		sessionID = uuid.New()
		u := version.Unit
		if _, err = tx.Exec(r.Context(), `INSERT INTO quizcraft_practice_sessions(id,bank_id,bank_version_id,user_id,actor_key,mode,purpose) VALUES($1,$2,$3,$4,$5,'random','training')`, sessionID, u.BankId, u.BankVersionId, actor.userID, actor.key); err != nil {
			trainingUnavailable(w)
			return
		}
		if _, err = tx.Exec(r.Context(), `INSERT INTO quizcraft_training_sessions(session_id,unit_version_id,bank_id,bank_version_id) VALUES($1,$2,$3,$4)`, sessionID, u.UnitVersionId, u.BankId, u.BankVersionId); err != nil {
			trainingUnavailable(w)
			return
		}
		for i, q := range u.Questions {
			if err = queries.AddPracticeSessionQuestion(r.Context(), store.AddPracticeSessionQuestionParams{SessionID: sessionID, BankID: u.BankId, BankVersionID: u.BankVersionId, QuestionID: q.QuestionId, QuestionVersionID: q.QuestionVersionId, Position: int32(i + 1)}); err != nil {
				trainingUnavailable(w)
				return
			}
		}
	}

	if queries.LockSubmission(r.Context(), sessionID.String()) != nil {
		trainingUnavailable(w)
		return
	}
	owns, err := service.trainingOwner(r.Context(), tx, r, sessionID, actor, false)
	if err != nil {
		trainingUnavailable(w)
		return
	}
	if !owns {
		trainingNotFound(w)
		return
	}
	snapshot, _, err := trainingSnapshot(r.Context(), tx, sessionID, actor)
	if err != nil {
		trainingUnavailable(w)
		return
	}
	encoded, _ := json.Marshal(responseEnvelope{RequestID: serviceRequestIDFromHeader(r), Data: snapshot})
	if !found {
		if storeIdempotency(r.Context(), queries, actor.key, kind, key, requestHash, 201, encoded, sessionID) != nil {
			trainingUnavailable(w)
			return
		}
	}
	if tx.Commit(r.Context()) != nil {
		trainingUnavailable(w)
		return
	}
	writeRawJSON(w, 201, encoded)
}
func (service *practiceHTTP) readTraining(w http.ResponseWriter, r *http.Request, review bool) {
	if !noTrainingQuery(w, r) {
		return
	}
	actor, ok := service.trainingActor(r)
	session, err := uuid.Parse(chi.URLParam(r, "session_id"))
	if !ok || err != nil {
		trainingNotFound(w)
		return
	}
	tx, err := service.database.BeginTx(r.Context(), pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		trainingUnavailable(w)
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()
	owns, err := service.trainingOwner(r.Context(), tx, r, session, actor, false)
	if err != nil {
		trainingUnavailable(w)
		return
	}
	if !owns {
		trainingNotFound(w)
		return
	}
	snapshot, version, err := trainingSnapshot(r.Context(), tx, session, actor)
	if errors.Is(err, pgx.ErrNoRows) {
		trainingNotFound(w)
		return
	}
	if err != nil {
		trainingUnavailable(w)
		return
	}
	if review {
		if snapshot.Phase != "completed" {
			writeError(w, 409, "UNIT_INCOMPLETE", "Save every answer before opening the unit review")
			return
		}
		layers, err := trainingLayers(r.Context(), tx, session, version)
		if err != nil {
			trainingUnavailable(w)
			return
		}
		if version.Cloze != nil {
			writeJSON(w, 200, responseEnvelope{RequestID: serviceRequestIDFromHeader(r), Data: tc.TrainingClozeReview{Kind: "cloze", SessionId: session, UnitVersionId: version.Unit.UnitVersionId, Questions: version.ReviewQuestions, Layers: layers}})
			return
		}
		writeJSON(w, 200, responseEnvelope{RequestID: serviceRequestIDFromHeader(r), Data: tc.TrainingListeningReview{Kind: "listening", SessionId: session, UnitVersionId: version.Unit.UnitVersionId, Questions: version.ReviewQuestions, Segments: version.Listening.Segments, Layers: layers}})
		return
	}
	writeJSON(w, 200, responseEnvelope{RequestID: serviceRequestIDFromHeader(r), Data: snapshot})
}
func (service *practiceHTTP) recoverTraining(w http.ResponseWriter, r *http.Request) {
	service.readTraining(w, r, false)
}
func (service *practiceHTTP) reviewTraining(w http.ResponseWriter, r *http.Request) {
	service.readTraining(w, r, true)
}

func (service *practiceHTTP) revealTraining(w http.ResponseWriter, r *http.Request) {
	if !noTrainingQuery(w, r) {
		return
	}
	var input tc.TrainingRevealRequest
	raw, err := decodeBody(r, &input)
	session, parseErr := uuid.Parse(chi.URLParam(r, "session_id"))
	if err != nil || parseErr != nil || tc.Validate("TrainingRevealRequest", raw) != nil {
		writeError(w, 400, "INVALID_REQUEST", "Choose an available text layer for this session")
		return
	}
	key := r.Header.Get("Idempotency-Key")
	if len(key) < 16 || len(key) > 160 {
		writeError(w, 400, "INVALID_REQUEST", "A valid save key is required")
		return
	}
	actor, ok := service.trainingActor(r)
	if !ok {
		trainingNotFound(w)
		return
	}
	tx, err := service.database.BeginTx(r.Context(), pgx.TxOptions{})
	if err != nil {
		trainingUnavailable(w)
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()
	queries := store.New(tx)
	if queries.LockSubmission(r.Context(), session.String()) != nil {
		trainingUnavailable(w)
		return
	}
	// Claims must serialize with account learning refreshes in answer commands.
	if actor.userID != nil {
		var bank uuid.UUID
		err := tx.QueryRow(r.Context(), `SELECT bank_id FROM quizcraft_training_sessions WHERE session_id=$1`, session).Scan(&bank)
		if errors.Is(err, pgx.ErrNoRows) {
			trainingNotFound(w)
			return
		}
		if err != nil || lockIdempotency(r.Context(), queries, actor.key, "training-learning", bank.String()) != nil {
			trainingUnavailable(w)
			return
		}
	}
	// Ownership precedes every idempotency lookup, including stale guest replays
	// after a session has been claimed by an account.
	owns, err := service.trainingOwner(r.Context(), tx, r, session, actor, true)
	if err != nil {
		trainingUnavailable(w)
		return
	}
	if !owns {
		trainingNotFound(w)
		return
	}
	snapshot, version, err := trainingSnapshot(r.Context(), tx, session, actor)
	if errors.Is(err, pgx.ErrNoRows) {
		trainingNotFound(w)
		return
	}
	if err != nil {
		trainingUnavailable(w)
		return
	}
	if input.UnitVersionId != version.Unit.UnitVersionId {
		writeError(w, 400, "INVALID_REQUEST", "The training version does not match; reopen this session")
		return
	}
	if snapshot.Phase != "completed" {
		writeError(w, 409, "UNIT_INCOMPLETE", "Save every answer before opening the text")
		return
	}
	kind := "POST " + r.URL.Path
	requestHash := hash(raw)
	_, _, found, conflict, err := loadIdempotency(r.Context(), queries, actor.key, kind, key, requestHash)
	if err != nil {
		trainingUnavailable(w)
		return
	}
	if conflict {
		writeError(w, 409, "IDEMPOTENCY_CONFLICT", "This save key was used for a different text layer")
		return
	}
	layers, err := trainingLayers(r.Context(), tx, session, version)
	if err != nil {
		trainingUnavailable(w)
		return
	}
	var disclosure *tc.TrainingLayerDisclosure
	for i := range layers {
		language, err := trainingLayerName(layers[i])
		if err != nil {
			trainingUnavailable(w)
			return
		}
		if language == string(input.Layer) {
			disclosure = &layers[i]
		}
	}
	if disclosure == nil {
		if input.Layer == "chinese" && len(layers) == 0 {
			writeError(w, 409, "LAYER_LOCKED", "Open the English text before the Chinese meaning")
			return
		}
		// now() is the transaction start, which can precede another command that
		// acquired this session lock first. Timestamp the serialized disclosure.
		if _, err = tx.Exec(r.Context(), `INSERT INTO quizcraft_training_disclosures(id,session_id,kind,disclosed_at) VALUES($1,$2,$3,clock_timestamp())`, uuid.New(), session, input.Layer); err != nil {
			trainingUnavailable(w)
			return
		}
		layers, err = trainingLayers(r.Context(), tx, session, version)
		if err != nil || len(layers) == 0 {
			trainingUnavailable(w)
			return
		}
		disclosure = &layers[len(layers)-1]
	}
	if actor.userID != nil && refreshTrainingLearning(r.Context(), tx, *actor.userID, version.Unit.BankId) != nil {
		trainingUnavailable(w)
		return
	}
	encoded, _ := json.Marshal(responseEnvelope{RequestID: serviceRequestIDFromHeader(r), Data: disclosure})
	if !found && storeIdempotency(r.Context(), queries, actor.key, kind, key, requestHash, 200, encoded, session) != nil {
		trainingUnavailable(w)
		return
	}
	if tx.Commit(r.Context()) != nil {
		trainingUnavailable(w)
		return
	}
	writeRawJSON(w, 200, encoded)
}

func (service *practiceHTTP) answerTraining(w http.ResponseWriter, r *http.Request) {
	if !noTrainingQuery(w, r) {
		return
	}
	var input tc.TrainingAnswerSubmission
	raw, err := decodeBody(r, &input)
	session, parseErr := uuid.Parse(chi.URLParam(r, "session_id"))
	if err != nil || parseErr != nil || tc.Validate("TrainingAnswerSubmission", raw) != nil {
		writeError(w, 400, "INVALID_REQUEST", "The answer does not match this training session")
		return
	}
	key := r.Header.Get("Idempotency-Key")
	if len(key) < 16 || len(key) > 160 {
		writeError(w, 400, "INVALID_REQUEST", "A valid save key is required")
		return
	}
	actor, ok := service.trainingActor(r)
	if !ok {
		trainingNotFound(w)
		return
	}
	tx, err := service.database.BeginTx(r.Context(), pgx.TxOptions{})
	if err != nil {
		trainingUnavailable(w)
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()
	queries := store.New(tx)
	if queries.LockSubmission(r.Context(), session.String()) != nil {
		trainingUnavailable(w)
		return
	}

	if actor.userID != nil {
		var bank uuid.UUID
		if err := tx.QueryRow(r.Context(), `SELECT bank_id FROM quizcraft_training_sessions WHERE session_id=$1`, session).Scan(&bank); errors.Is(err, pgx.ErrNoRows) {
			trainingNotFound(w)
			return
		} else if err != nil {
			trainingUnavailable(w)
			return
		}
		if lockIdempotency(r.Context(), queries, actor.key, "training-learning", bank.String()) != nil {
			trainingUnavailable(w)
			return
		}
	}
	owns, err := service.trainingOwner(r.Context(), tx, r, session, actor, true)
	if err != nil {
		trainingUnavailable(w)
		return
	}
	if !owns {
		trainingNotFound(w)
		return
	}
	snapshot, version, err := trainingSnapshot(r.Context(), tx, session, actor)
	if errors.Is(err, pgx.ErrNoRows) {
		trainingNotFound(w)
		return
	}
	if err != nil {
		trainingUnavailable(w)
		return
	}
	if version.Unit.UnitVersionId != input.UnitVersionId {
		writeError(w, 400, "INVALID_REQUEST", "The training version does not match")
		return
	}
	var question *tc.TrainingQuestion
	var review *tc.TrainingQuestionReview
	for i := range version.Unit.Questions {
		q := &version.Unit.Questions[i]
		if q.QuestionId == input.QuestionId && q.QuestionVersionId == input.QuestionVersionId {
			question = q
			review = &version.ReviewQuestions[i]
			break
		}
	}
	if question == nil || input.Answer >= len(question.Options) {
		writeError(w, 400, "INVALID_REQUEST", "Choose an option from this question")
		return
	}
	kind := "POST " + r.URL.Path
	requestHash := hash(raw)
	_, _, found, conflict, err := loadIdempotency(r.Context(), queries, actor.key, kind, key, requestHash)
	if err != nil {
		trainingUnavailable(w)
		return
	}
	if conflict {
		writeError(w, 409, "IDEMPOTENCY_CONFLICT", "This save key was used for a different answer")
		return
	}
	var receipt *tc.TrainingAnswerReceipt
	for i := range snapshot.Receipts {
		if snapshot.Receipts[i].QuestionId == input.QuestionId {
			receipt = &snapshot.Receipts[i]
			break
		}
	}
	if receipt != nil && receipt.Answer != input.Answer {
		writeError(w, 409, "FIRST_ANSWER_IMMUTABLE", "The first saved answer cannot be changed; start a fresh practice to try again")
		return
	}
	if receipt == nil {
		options := make([]any, len(question.Options))
		for i, option := range question.Options {
			options[i] = option
		}
		correct := scoreAnswer("single", float64(input.Answer), float64(review.ExpectedAnswer), options)
		assisted := false
		for _, hint := range snapshot.HintDisclosures {
			for _, id := range hint.AffectedQuestionIds {
				if id == input.QuestionId {
					assisted = true
				}
			}
		}
		outcome := tc.TrainingOutcome("review_required")
		if correct {
			if assisted {
				outcome = "assisted_completion"
			} else {
				outcome = "independent_correct"
			}
		}
		result := tc.TrainingAnswerReceipt{SessionId: session, UnitVersionId: input.UnitVersionId, QuestionId: input.QuestionId, QuestionVersionId: input.QuestionVersionId, Answer: input.Answer, Correct: correct, Assisted: assisted, Outcome: outcome, ReviewRequired: !correct || assisted, TotalCount: snapshot.TotalCount, SavedCount: snapshot.SavedCount + 1, Phase: trainingPhase(snapshot.SavedCount+1, snapshot.TotalCount)}
		var submittedAt time.Time
		if tx.QueryRow(r.Context(), `SELECT clock_timestamp()`).Scan(&submittedAt) != nil {
			trainingUnavailable(w)
			return
		}
		result.SubmittedAt = submittedAt
		encoded, _ := json.Marshal(responseEnvelope{RequestID: serviceRequestIDFromHeader(r), Data: result})
		answerJSON, _ := json.Marshal(input.Answer)
		expected, _ := json.Marshal(review.ExpectedAnswer)
		attempt := uuid.New()
		var timestamp time.Time
		err := tx.QueryRow(r.Context(), `INSERT INTO quizcraft_practice_attempts(id,session_id,bank_id,bank_version_id,question_id,question_version_id,user_id,submitted_answer,correct,expected_answer,analysis,response_body,submitted_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,clock_timestamp()) RETURNING submitted_at`, attempt, session, version.Unit.BankId, version.Unit.BankVersionId, input.QuestionId, input.QuestionVersionId, nullableUUID(actor.userID), answerJSON, correct, expected, review.Analysis, string(encoded)).Scan(&timestamp)
		if err != nil {
			trainingUnavailable(w)
			return
		}
		result.SubmittedAt = timestamp.UTC()
		if _, err = tx.Exec(r.Context(), `INSERT INTO quizcraft_training_outcomes(attempt_id,assisted,outcome,review_required) VALUES($1,$2,$3,$4)`, attempt, assisted, outcome, !correct || assisted); err != nil {
			trainingUnavailable(w)
			return
		}

		receipt = &result
	}

	if actor.userID != nil {
		if refreshTrainingLearning(r.Context(), tx, *actor.userID, version.Unit.BankId) != nil {
			trainingUnavailable(w)
			return
		}
	}
	encoded, _ := json.Marshal(responseEnvelope{RequestID: serviceRequestIDFromHeader(r), Data: receipt})
	if !found {
		if storeIdempotency(r.Context(), queries, actor.key, kind, key, requestHash, 200, encoded, session) != nil {
			trainingUnavailable(w)
			return
		}
	}
	if tx.Commit(r.Context()) != nil {
		trainingUnavailable(w)
		return
	}
	writeRawJSON(w, 200, encoded)
}

type trainingCursor struct {
	Actor   string    `json:"a"`
	At      time.Time `json:"t"`
	Version uuid.UUID `json:"v"`
}

func (service *practiceHTTP) trainingCursor(actor string, at time.Time, version uuid.UUID) string {
	raw, _ := json.Marshal(trainingCursor{Actor: hash([]byte(actor)), At: at, Version: version})
	mac := hmac.New(sha256.New, service.authHMACSecret)
	_, _ = mac.Write(raw)
	return base64.RawURLEncoding.EncodeToString(append(raw, mac.Sum(nil)...))
}
func (service *practiceHTTP) trainingReviewItems(w http.ResponseWriter, r *http.Request) {
	actor, ok := service.trainingActor(r)
	if !ok {
		trainingNotFound(w)
		return
	}
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		writeError(w, 400, "INVALID_REQUEST", "Review filters are invalid")
		return
	}
	size := 20
	var cursor trainingCursor
	for name, items := range values {
		if len(items) != 1 || (name != "cursor" && name != "page_size") {
			writeError(w, 400, "INVALID_REQUEST", "Review filters are invalid")
			return
		}
	}
	if raw, exists := values["page_size"]; exists {
		size, err = strconv.Atoi(raw[0])
		if err != nil || size < 1 || size > 50 {
			writeError(w, 400, "INVALID_REQUEST", "Review page size must be between 1 and 50")
			return
		}
	}
	if raw, exists := values["cursor"]; exists {
		data, err := base64.RawURLEncoding.DecodeString(raw[0])
		if err != nil || len(raw[0]) > 512 || len(data) <= 32 {
			writeError(w, 400, "INVALID_REQUEST", "Review continuation is invalid; restart the list")
			return
		}
		payload, signature := data[:len(data)-32], data[len(data)-32:]
		mac := hmac.New(sha256.New, service.authHMACSecret)
		_, _ = mac.Write(payload)
		if !hmac.Equal(signature, mac.Sum(nil)) || json.Unmarshal(payload, &cursor) != nil || cursor.Actor != hash([]byte(actor.key)) {
			writeError(w, 400, "INVALID_REQUEST", "Review continuation is invalid; restart the list")
			return
		}
	}
	canonical := values.Encode()
	if canonical != r.URL.RawQuery {
		writeError(w, 400, "INVALID_REQUEST", "Review query is not canonical")
		return
	}
	rows, err := service.database.Query(r.Context(), `WITH latest AS (
 SELECT DISTINCT ON(a.question_id) a.*,t.unit_version_id,o.review_required
 FROM quizcraft_practice_attempts a JOIN quizcraft_training_sessions t ON t.session_id=a.session_id
 JOIN quizcraft_training_outcomes o ON o.attempt_id=a.id
 JOIN quizcraft_practice_sessions s ON s.id=a.session_id LEFT JOIN quizcraft_practice_session_claims c ON c.session_id=s.id
 WHERE coalesce(c.user_actor_key,s.actor_key)=$1
 ORDER BY a.question_id,a.submitted_at DESC,a.id DESC
 ), groups AS(SELECT unit_version_id,count(*)::int AS review_count,max(submitted_at) AS attempted_at FROM latest WHERE review_required GROUP BY unit_version_id)
 SELECT v.document,g.review_count,g.attempted_at,(SELECT l.session_id FROM latest l WHERE l.unit_version_id=g.unit_version_id AND l.review_required ORDER BY l.submitted_at DESC,l.id DESC LIMIT 1)
 FROM groups g JOIN quizcraft_training_unit_versions v ON v.id=g.unit_version_id
 WHERE $2::timestamptz IS NULL OR (g.attempted_at,g.unit_version_id)<($2,$3::uuid)
 ORDER BY g.attempted_at DESC,g.unit_version_id DESC LIMIT $4`, actor.key, func() any {
		if cursor.At.IsZero() {
			return nil
		}
		return cursor.At
	}(), cursor.Version, size+1)
	if err != nil {
		trainingUnavailable(w)
		return
	}
	result := tc.TrainingReviewItems{Items: []tc.TrainingReviewItem{}}
	for rows.Next() {
		var raw []byte
		var item tc.TrainingReviewItem
		var version trainingVersion
		if rows.Scan(&raw, &item.ReviewRequiredCount, &item.LastAttemptAt, &item.SessionId) != nil || func() bool { var err error; version, err = decodeTrainingVersion(raw); return err != nil }() {
			rows.Close()
			trainingUnavailable(w)
			return
		}
		item.LastAttemptAt = item.LastAttemptAt.UTC()
		item.UnitId = version.Unit.UnitId
		item.UnitVersionId = version.Unit.UnitVersionId
		item.Title = version.Unit.Title
		item.Kind = tc.TrainingKind(version.Unit.Kind)
		result.Items = append(result.Items, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		trainingUnavailable(w)
		return
	}
	if len(result.Items) > size {
		last := result.Items[size-1]
		result.NextCursor = service.trainingCursor(actor.key, last.LastAttemptAt, last.UnitVersionId)
		result.Items = result.Items[:size]
	}
	for i := range result.Items {
		var saved, total int
		if service.database.QueryRow(r.Context(), `SELECT (SELECT count(*) FROM quizcraft_practice_attempts WHERE session_id=$1),(SELECT count(*) FROM quizcraft_practice_session_questions WHERE session_id=$1)`, result.Items[i].SessionId).Scan(&saved, &total) != nil {
			trainingUnavailable(w)
			return
		}
		result.Items[i].Phase = trainingPhase(saved, total)
	}
	writeJSON(w, 200, responseEnvelope{RequestID: serviceRequestIDFromHeader(r), Data: result})
}

func refreshTrainingLearning(ctx context.Context, tx pgx.Tx, user, bank uuid.UUID) error {
	_, err := tx.Exec(ctx, `INSERT INTO quizcraft_learning_state(user_id,bank_id,question_id,question_version_id,wrong,attempt_count,correct_count,latest_attempt_id,updated_at)
 SELECT user_id,bank_id,question_id,question_version_id,wrong,attempt_count,correct_count,latest_attempt_id,updated_at FROM quizcraft_expected_learning_state WHERE user_id=$1 AND bank_id=$2
 ON CONFLICT(user_id,bank_id,question_id) DO UPDATE SET question_version_id=EXCLUDED.question_version_id,wrong=EXCLUDED.wrong,attempt_count=EXCLUDED.attempt_count,correct_count=EXCLUDED.correct_count,latest_attempt_id=EXCLUDED.latest_attempt_id,updated_at=EXCLUDED.updated_at`, user, bank)
	return err
}

func (service *practiceHTTP) hintTraining(w http.ResponseWriter, r *http.Request) {
	if !noTrainingQuery(w, r) {
		return
	}
	var input tc.TrainingHintRequest
	raw, err := decodeBody(r, &input)
	session, parseErr := uuid.Parse(chi.URLParam(r, "session_id"))
	if err != nil || parseErr != nil || tc.Validate("TrainingHintRequest", raw) != nil {
		writeError(w, 400, "INVALID_REQUEST", "Choose an available hint for this session")
		return
	}
	key := r.Header.Get("Idempotency-Key")
	if len(key) < 16 || len(key) > 160 {
		writeError(w, 400, "INVALID_REQUEST", "A valid save key is required")
		return
	}
	actor, ok := service.trainingActor(r)
	if !ok {
		trainingNotFound(w)
		return
	}
	tx, err := service.database.BeginTx(r.Context(), pgx.TxOptions{})
	if err != nil {
		trainingUnavailable(w)
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()
	queries := store.New(tx)
	if queries.LockSubmission(r.Context(), session.String()) != nil {
		trainingUnavailable(w)
		return
	}
	// Claims must serialize with account learning refreshes in answer commands.
	if actor.userID != nil {
		var bank uuid.UUID
		err := tx.QueryRow(r.Context(), `SELECT bank_id FROM quizcraft_training_sessions WHERE session_id=$1`, session).Scan(&bank)
		if errors.Is(err, pgx.ErrNoRows) {
			trainingNotFound(w)
			return
		}
		if err != nil || lockIdempotency(r.Context(), queries, actor.key, "training-learning", bank.String()) != nil {
			trainingUnavailable(w)
			return
		}
	}
	// Ownership precedes every idempotency lookup, including stale guest replays
	// after a session has been claimed by an account.
	owns, err := service.trainingOwner(r.Context(), tx, r, session, actor, true)
	if err != nil {
		trainingUnavailable(w)
		return
	}
	if !owns {
		trainingNotFound(w)
		return
	}
	snapshot, version, err := trainingSnapshot(r.Context(), tx, session, actor)
	if errors.Is(err, pgx.ErrNoRows) {
		trainingNotFound(w)
		return
	}
	if err != nil {
		trainingUnavailable(w)
		return
	}
	if input.UnitVersionId != version.Unit.UnitVersionId {
		writeError(w, 400, "INVALID_REQUEST", "The training version does not match; reopen this session")
		return
	}
	if version.Cloze == nil {
		writeError(w, 400, "INVALID_REQUEST", "Hints are available only for cloze training")
		return
	}
	var authored *tc.TrainingAuthoredHint
	for i := range version.Cloze.Hints {
		if version.Cloze.Hints[i].HintId == input.HintId {
			authored = &version.Cloze.Hints[i]
			break
		}
	}
	if authored == nil || input.Level > len(authored.Levels) {
		writeError(w, 400, "INVALID_REQUEST", "Choose an available hint level")
		return
	}
	kind := "POST " + r.URL.Path
	requestHash := hash(raw)
	_, _, found, conflict, err := loadIdempotency(r.Context(), queries, actor.key, kind, key, requestHash)
	if err != nil {
		trainingUnavailable(w)
		return
	}
	if conflict {
		writeError(w, 409, "IDEMPOTENCY_CONFLICT", "This save key was used for a different hint")
		return
	}
	var disclosure *tc.TrainingHintDisclosure
	nextLevel := 1
	for i := range snapshot.HintDisclosures {
		hint := &snapshot.HintDisclosures[i]
		if hint.HintId == input.HintId {
			if hint.Level >= nextLevel {
				nextLevel = hint.Level + 1
			}
			if hint.Level == input.Level {
				disclosure = hint
			}
		}
	}
	if disclosure == nil {
		if input.Level != nextLevel {
			writeError(w, 409, "LAYER_LOCKED", "Open the previous hint level first")
			return
		}
		saved := map[uuid.UUID]bool{}
		for _, receipt := range snapshot.Receipts {
			saved[receipt.QuestionId] = true
		}
		affected := []uuid.UUID{}
		for _, question := range version.Unit.Questions {
			if !saved[question.QuestionId] {
				affected = append(affected, question.QuestionId)
			}
		}
		value := tc.TrainingHintDisclosure{DisclosureId: uuid.New(), HintId: input.HintId, Level: input.Level, Text: authored.Levels[input.Level-1], AffectedQuestionIds: affected}
		if err := tx.QueryRow(r.Context(), `INSERT INTO quizcraft_training_disclosures(id,session_id,kind,target_id,level,affected_question_ids,disclosed_at) VALUES($1,$2,'hint',$3,$4,$5,clock_timestamp()) RETURNING disclosed_at`, value.DisclosureId, session, input.HintId, input.Level, affected).Scan(&value.DisclosedAt); err != nil {
			trainingUnavailable(w)
			return
		}
		value.DisclosedAt = value.DisclosedAt.UTC()
		disclosure = &value
	}
	if actor.userID != nil && refreshTrainingLearning(r.Context(), tx, *actor.userID, version.Unit.BankId) != nil {
		trainingUnavailable(w)
		return
	}
	encoded, _ := json.Marshal(responseEnvelope{RequestID: serviceRequestIDFromHeader(r), Data: disclosure})
	if !found && storeIdempotency(r.Context(), queries, actor.key, kind, key, requestHash, 200, encoded, session) != nil {
		trainingUnavailable(w)
		return
	}
	if tx.Commit(r.Context()) != nil {
		trainingUnavailable(w)
		return
	}
	writeRawJSON(w, 200, encoded)
}
