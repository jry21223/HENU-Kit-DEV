package quizcraft

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"henukit.dev/quizcraft/internal/store"
)

var sha256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

type workshopQuestionRequest struct {
	SourceQuestionID string   `json:"source_question_id"`
	Type             string   `json:"type"`
	ChapterID        string   `json:"chapter_id"`
	Chapter          string   `json:"chapter"`
	Content          string   `json:"content"`
	Options          []string `json:"options,omitempty"`
	Answer           any      `json:"answer"`
	Analysis         string   `json:"analysis,omitempty"`
}

type createWorkshopBankRequest struct {
	BankKey string `json:"bank_key"`
	Name    string `json:"name"`
}

type createWorkshopVersionRequest struct {
	ExpectedVersion int64                     `json:"expected_version"`
	Questions       []workshopQuestionRequest `json:"questions"`
}

type importWorkshopVersionRequest struct {
	ExpectedVersion *int64                    `json:"expected_version"`
	SourceSHA256    string                    `json:"source_sha256"`
	Questions       []workshopQuestionRequest `json:"questions"`
}

type versionCommandRequest struct {
	ExpectedVersion int64  `json:"expected_version"`
	Note            string `json:"note"`
}

type rollbackCommandRequest struct {
	ExpectedVersion     int64     `json:"expected_version"`
	TargetBankVersionID uuid.UUID `json:"target_bank_version_id"`
	Note                string    `json:"note"`
}

type questionFeedbackRequest struct {
	BankID            uuid.UUID `json:"bank_id"`
	QuestionID        uuid.UUID `json:"question_id"`
	QuestionVersionID uuid.UUID `json:"question_version_id"`
	Category          string    `json:"category"`
	Detail            string    `json:"detail"`
}

type feedbackStatusProjection struct {
	FeedbackID, BankID, QuestionID, QuestionVersionID uuid.UUID
	Category, Status                                  string
	CreatedAt, UpdatedAt                              time.Time
}

type workshopHTTPError struct {
	status  int
	code    string
	message string
}

func (problem workshopHTTPError) Error() string { return problem.message }

var errWorkshopReplay = errors.New("workshop idempotent replay")

func (service *practiceHTTP) workshopActor(writer http.ResponseWriter, request *http.Request, permission string, bankID *uuid.UUID, allowResourceList bool) (practiceActor, map[uuid.UUID]bool, bool) {
	actor, status, err := service.actor(writer, request)
	if err != nil || actor.userID == nil {
		if err != nil {
			writeError(writer, status, "invalid_session", err.Error())
		} else {
			writeError(writer, http.StatusUnauthorized, "authentication_required", "sign in through Platform Core to use Workshop")
		}
		return practiceActor{}, nil, false
	}
	if service.platform != nil {
		if actor.exchangeToken == "" {
			writeError(writer, http.StatusUnauthorized, "platform_session_required", "sign in through Platform Core before using Workshop")
			return practiceActor{}, nil, false
		}
		if bankID == nil && allowResourceList {
			scope := map[string]string{"kind": "product", "product_code": "quizcraft"}
			if err := service.platform.check(request.Context(), actor.exchangeToken, permission, scope); err == nil {
				actor.platformProductScope = true
				return actor, nil, true
			} else if errors.Is(err, errPlatformUnauthorized) {
				writeError(writer, http.StatusUnauthorized, "invalid_session", "Platform Core session was revoked or expired")
				return practiceActor{}, nil, false
			} else if !errors.Is(err, errPlatformForbidden) {
				writeError(writer, http.StatusServiceUnavailable, "authorization_unavailable", "Platform Core authorization is temporarily unavailable")
				return practiceActor{}, nil, false
			}
			return actor, nil, true
		}
		scope := map[string]string{"kind": "product", "product_code": "quizcraft"}
		if bankID != nil {
			scope = map[string]string{"kind": "resource", "product_code": "quizcraft", "resource_type": "bank", "resource_id": bankID.String()}
		}
		if err := service.platform.check(request.Context(), actor.exchangeToken, permission, scope); err != nil {
			if errors.Is(err, errPlatformUnauthorized) {
				writeError(writer, http.StatusUnauthorized, "invalid_session", "Platform Core session was revoked or expired")
			} else if errors.Is(err, errPlatformForbidden) {
				writeError(writer, http.StatusForbidden, "scope_denied", "Platform Core denied the required permission and Scope")
			} else {
				writeError(writer, http.StatusServiceUnavailable, "authorization_unavailable", "Platform Core authorization is temporarily unavailable")
			}
			return practiceActor{}, nil, false
		}
		return actor, nil, true
	}
	if !service.allowTestWorkshopClaims {
		writeError(writer, http.StatusServiceUnavailable, "authorization_unavailable", "Platform Core authorization is not configured")
		return practiceActor{}, nil, false
	}
	if !actor.permissions[permission] {
		writeError(writer, http.StatusForbidden, "permission_denied", "required Workshop permission is missing")
		return practiceActor{}, nil, false
	}
	resourceIDs := make(map[uuid.UUID]bool)
	for _, scope := range actor.scopes {
		if scope.Kind == "product" && scope.ProductCode == "quizcraft" {
			return actor, nil, true
		}
		if scope.Kind == "resource" && scope.ProductCode == "quizcraft" && scope.ResourceType == "bank" {
			resourceID, parseErr := uuid.Parse(scope.ResourceID)
			if parseErr != nil {
				continue
			}
			if bankID != nil && resourceID == *bankID {
				return actor, map[uuid.UUID]bool{resourceID: true}, true
			}
			if bankID == nil && allowResourceList {
				resourceIDs[resourceID] = true
			}
		}
	}
	if len(resourceIDs) > 0 {
		return actor, resourceIDs, true
	}
	writeError(writer, http.StatusForbidden, "scope_denied", "QuizCraft product or matching bank Scope is required")
	return practiceActor{}, nil, false
}

func (service *practiceHTTP) listWorkshopBanks(writer http.ResponseWriter, request *http.Request) {
	actor, resourceFilter, ok := service.workshopActor(writer, request, "quizcraft.workshop.read", nil, true)
	if !ok {
		return
	}
	rows, err := service.queries.ListPublishedBanks(request.Context())
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft Workshop is temporarily unavailable")
		return
	}
	banks := make([]map[string]any, 0)
	for _, row := range rows {
		if service.platform != nil && actor.exchangeToken != "" && !actor.platformProductScope {
			scope := map[string]string{"kind": "resource", "product_code": "quizcraft", "resource_type": "bank", "resource_id": row.ID.String()}
			if err := service.platform.check(request.Context(), actor.exchangeToken, "quizcraft.workshop.read", scope); errors.Is(err, errPlatformForbidden) {
				continue
			} else if errors.Is(err, errPlatformUnauthorized) {
				writeError(writer, http.StatusUnauthorized, "invalid_session", "Platform Core session was revoked or expired")
				return
			} else if err != nil {
				writeError(writer, http.StatusServiceUnavailable, "authorization_unavailable", "Platform Core authorization is temporarily unavailable")
				return
			}
		}
		if !row.ActiveVersionID.Valid || (resourceFilter != nil && !resourceFilter[row.ID]) {
			continue
		}
		chaptersJSON, _ := json.Marshal(row.Chapters)
		var chapters []map[string]string
		if err := json.Unmarshal(chaptersJSON, &chapters); err != nil {
			writeError(writer, http.StatusServiceUnavailable, "invalid_bank", "QuizCraft bank chapters are invalid")
			return
		}
		banks = append(banks, map[string]any{"bank_id": row.ID, "bank_version_id": row.ActiveVersionID.UUID, "bank_key": row.BankKey, "name": row.Name, "content_sha256": row.ContentSha256, "question_count": row.QuestionCount, "chapters": chapters})
	}
	writeJSON(writer, http.StatusOK, responseEnvelope{RequestID: requestID(), Data: banks})
}

func (service *practiceHTTP) listWorkshopCatalog(writer http.ResponseWriter, request *http.Request) {
	actor, resourceFilter, ok := service.workshopActor(writer, request, "quizcraft.workshop.read", nil, true)
	if !ok {
		return
	}
	query := `SELECT id,bank_key,name,lifecycle_version,active_version_id FROM quizcraft_banks`
	args := []any{}
	if resourceFilter != nil {
		ids := make([]uuid.UUID, 0, len(resourceFilter))
		for id := range resourceFilter {
			ids = append(ids, id)
		}
		query += ` WHERE id=ANY($1)`
		args = append(args, ids)
	}
	query += ` ORDER BY bank_key`
	rows, err := service.database.Query(request.Context(), query, args...)
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft Workshop is temporarily unavailable")
		return
	}
	defer rows.Close()
	banks := make([]map[string]any, 0)
	for rows.Next() {
		var bankID uuid.UUID
		var bankKey, name string
		var lifecycleVersion int64
		var activeVersionID uuid.NullUUID
		if err := rows.Scan(&bankID, &bankKey, &name, &lifecycleVersion, &activeVersionID); err != nil {
			writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft Workshop is temporarily unavailable")
			return
		}
		if service.platform != nil && actor.exchangeToken != "" && !actor.platformProductScope {
			scope := map[string]string{"kind": "resource", "product_code": "quizcraft", "resource_type": "bank", "resource_id": bankID.String()}
			if err := service.platform.check(request.Context(), actor.exchangeToken, "quizcraft.workshop.read", scope); errors.Is(err, errPlatformForbidden) {
				continue
			} else if errors.Is(err, errPlatformUnauthorized) {
				writeError(writer, http.StatusUnauthorized, "invalid_session", "Platform Core session was revoked or expired")
				return
			} else if err != nil {
				writeError(writer, http.StatusServiceUnavailable, "authorization_unavailable", "Platform Core authorization is temporarily unavailable")
				return
			}
		}
		versionRows, queryErr := service.database.Query(request.Context(), `SELECT v.id,v.content_sha256,count(m.question_id)::bigint,coalesce(s.state,'legacy'),s.validated_at FROM quizcraft_bank_versions v LEFT JOIN quizcraft_bank_version_questions m ON m.bank_version_id=v.id LEFT JOIN quizcraft_workshop_version_states s ON s.bank_version_id=v.id WHERE v.bank_id=$1 GROUP BY v.id,s.state,s.validated_at ORDER BY v.imported_at DESC,v.id`, bankID)
		if queryErr != nil {
			writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft Workshop is temporarily unavailable")
			return
		}
		versions := make([]map[string]any, 0)
		for versionRows.Next() {
			var versionID uuid.UUID
			var contentSHA, state string
			var questionCount int64
			var validatedAt any
			if scanErr := versionRows.Scan(&versionID, &contentSHA, &questionCount, &state, &validatedAt); scanErr != nil {
				versionRows.Close()
				writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft Workshop is temporarily unavailable")
				return
			}
			item := map[string]any{"bank_version_id": versionID, "content_sha256": contentSHA, "question_count": questionCount, "state": state, "active": activeVersionID.Valid && activeVersionID.UUID == versionID}
			if validatedAt != nil {
				item["validated_at"] = validatedAt
			}
			versions = append(versions, item)
		}
		versionRows.Close()
		bank := map[string]any{"bank_id": bankID, "bank_key": bankKey, "name": name, "lifecycle_version": lifecycleVersion, "versions": versions}
		if activeVersionID.Valid {
			bank["active_version_id"] = activeVersionID.UUID
		}
		banks = append(banks, bank)
	}
	writeJSON(writer, http.StatusOK, responseEnvelope{RequestID: requestID(), Data: banks})
}

func (service *practiceHTTP) createWorkshopBank(writer http.ResponseWriter, request *http.Request) {
	actor, _, ok := service.workshopActor(writer, request, "quizcraft.workshop.write", nil, false)
	if !ok {
		return
	}
	var input createWorkshopBankRequest
	raw, err := decodeBody(request, &input)
	input.BankKey, input.Name = strings.TrimSpace(input.BankKey), strings.TrimSpace(input.Name)
	if err != nil || !bankKeyPattern.MatchString(input.BankKey) || len([]rune(input.Name)) < 1 || len([]rune(input.Name)) > 160 {
		writeError(writer, http.StatusBadRequest, "invalid_workshop_bank", "bank key or name is invalid")
		return
	}
	bankID := stableID(quizcraftNamespace, "bank:"+input.BankKey)
	service.runWorkshopMutation(writer, request, actor, "create_workshop_bank", http.StatusCreated, raw, bankID, func(ctx context.Context, tx pgx.Tx, requestID string) error {
		result, err := tx.Exec(ctx, `INSERT INTO quizcraft_banks(id,bank_key,name,lifecycle_version) VALUES($1,$2,$3,1) ON CONFLICT DO NOTHING`, bankID, input.BankKey, input.Name)
		if err != nil || result.RowsAffected() != 1 {
			return workshopHTTPError{http.StatusConflict, "bank_conflict", "bank key already exists"}
		}
		return insertWorkshopAudit(ctx, tx, *actor.userID, "quizcraft.workshop.write", "create_bank", bankID, uuid.Nil, 0, requestID, "")
	})
}

func (service *practiceHTTP) getWorkshopVersion(writer http.ResponseWriter, request *http.Request) {
	bankID, versionID, ok := workshopIDs(writer, request, true)
	if !ok {
		return
	}
	if _, _, allowed := service.workshopActor(writer, request, "quizcraft.workshop.read", &bankID, false); !allowed {
		return
	}
	var state, contentSHA string
	if err := service.database.QueryRow(request.Context(), `SELECT s.state,v.content_sha256 FROM quizcraft_bank_versions v JOIN quizcraft_workshop_version_states s ON s.bank_id=v.bank_id AND s.bank_version_id=v.id WHERE v.bank_id=$1 AND v.id=$2`, bankID, versionID).Scan(&state, &contentSHA); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(writer, http.StatusNotFound, "version_not_found", "Workshop version was not found")
		} else {
			writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft Workshop is temporarily unavailable")
		}
		return
	}
	rows, err := service.database.Query(request.Context(), `SELECT q.id,qv.id,q.source_question_id,qv.type,qv.chapter_id,qv.chapter_name,qv.content,qv.options,qv.answer,qv.analysis,m.position FROM quizcraft_bank_version_questions m JOIN quizcraft_questions q ON q.bank_id=m.bank_id AND q.id=m.question_id JOIN quizcraft_question_versions qv ON qv.bank_id=m.bank_id AND qv.question_id=m.question_id AND qv.id=m.question_version_id WHERE m.bank_id=$1 AND m.bank_version_id=$2 ORDER BY m.position`, bankID, versionID)
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft Workshop is temporarily unavailable")
		return
	}
	defer rows.Close()
	questions := make([]map[string]any, 0)
	for rows.Next() {
		var questionID, questionVersionID uuid.UUID
		var sourceID, questionType, chapterID, chapter, content, analysis string
		var optionsJSON, answerJSON []byte
		var position int
		if err := rows.Scan(&questionID, &questionVersionID, &sourceID, &questionType, &chapterID, &chapter, &content, &optionsJSON, &answerJSON, &analysis, &position); err != nil {
			writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft Workshop is temporarily unavailable")
			return
		}
		var options, answer any
		_ = json.Unmarshal(optionsJSON, &options)
		_ = json.Unmarshal(answerJSON, &answer)
		questions = append(questions, map[string]any{"question_id": questionID, "question_version_id": questionVersionID, "source_question_id": sourceID, "type": questionType, "chapter_id": chapterID, "chapter": chapter, "content": content, "options": options, "answer": answer, "analysis": analysis, "position": position})
	}
	writeJSON(writer, http.StatusOK, responseEnvelope{RequestID: requestID(), Data: map[string]any{"bank_id": bankID, "bank_version_id": versionID, "state": state, "content_sha256": contentSHA, "questions": questions}})
}

func (service *practiceHTTP) getWorkshopFeedback(writer http.ResponseWriter, request *http.Request) {
	feedbackID, err := uuid.Parse(chi.URLParam(request, "feedback_id"))
	if err != nil {
		writeError(writer, http.StatusBadRequest, "invalid_feedback_id", "feedback_id must be a UUID")
		return
	}
	var bankID, questionID, questionVersionID uuid.UUID
	var category, detail string
	var createdAt time.Time
	if err := service.database.QueryRow(request.Context(), `SELECT bank_id,question_id,question_version_id,category,detail,created_at FROM quizcraft_feedbacks WHERE id=$1`, feedbackID).Scan(&bankID, &questionID, &questionVersionID, &category, &detail, &createdAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(writer, http.StatusNotFound, "feedback_not_found", "feedback was not found")
		} else {
			writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft feedback is temporarily unavailable")
		}
		return
	}
	if _, _, allowed := service.workshopActor(writer, request, "quizcraft.workshop.read", &bankID, false); !allowed {
		return
	}
	writeJSON(writer, http.StatusOK, responseEnvelope{RequestID: requestID(), Data: map[string]any{"feedback_id": feedbackID, "bank_id": bankID, "question_id": questionID, "question_version_id": questionVersionID, "category": category, "detail": detail, "created_at": createdAt.UTC()}})
}

func (service *practiceHTTP) createWorkshopVersion(writer http.ResponseWriter, request *http.Request) {
	var input createWorkshopVersionRequest
	raw, err := decodeBody(request, &input)
	if err != nil {
		writeError(writer, http.StatusBadRequest, "invalid_request", "Workshop version body is invalid")
		return
	}
	service.importWorkshopDraft(writer, request, raw, input.ExpectedVersion, "create_bank_version", "create_version", "", input.Questions)
}

func (service *practiceHTTP) importWorkshopVersion(writer http.ResponseWriter, request *http.Request) {
	var input importWorkshopVersionRequest
	raw, err := decodeBody(request, &input)
	if err != nil || !sha256Pattern.MatchString(input.SourceSHA256) {
		writeError(writer, http.StatusBadRequest, "invalid_request", "Workshop import body is invalid")
		return
	}
	expectedVersion := int64(-1)
	if input.ExpectedVersion != nil {
		expectedVersion = *input.ExpectedVersion
	}
	service.importWorkshopDraft(writer, request, raw, expectedVersion, "import_bank", "import_version", input.SourceSHA256, input.Questions)
}

func (service *practiceHTTP) importWorkshopDraft(writer http.ResponseWriter, request *http.Request, raw []byte, expectedVersion int64, operationKind, action, sourceSHA string, questions []workshopQuestionRequest) {
	bankID, err := uuid.Parse(chi.URLParam(request, "bank_id"))
	if err != nil {
		writeError(writer, http.StatusBadRequest, "invalid_bank_id", "bank_id must be a UUID")
		return
	}
	actor, _, ok := service.workshopActor(writer, request, "quizcraft.workshop.write", &bankID, false)
	if !ok {
		return
	}
	idempotencyKey, ok := requiredIdempotencyKey(writer, request)
	if !ok {
		return
	}
	if expectedVersion < -1 || (expectedVersion == -1 && action != "import_version") || len(questions) == 0 {
		writeError(writer, http.StatusBadRequest, "invalid_request", "expected_version and questions are required")
		return
	}
	var bankKey, name string
	if err := service.database.QueryRow(request.Context(), `SELECT bank_key,name FROM quizcraft_banks WHERE id=$1`, bankID).Scan(&bankKey, &name); err != nil {
		writeError(writer, http.StatusNotFound, "bank_not_found", "Workshop bank was not found")
		return
	}
	documentQuestions := make([]map[string]any, 0, len(questions))
	for _, question := range questions {
		documentQuestions = append(documentQuestions, map[string]any{"id": question.SourceQuestionID, "type": question.Type, "chapter_id": question.ChapterID, "chapter": question.Chapter, "content": question.Content, "options": question.Options, "answer": question.Answer, "analysis": question.Analysis})
	}
	source, _ := json.Marshal(map[string]any{"meta": map[string]any{"name": name, "version": "workshop"}, "questions": documentQuestions})
	requestHash := hashCanonical(raw)
	var encoded []byte
	core := &Service{database: service.database}
	report, importErr := core.importJSON(request.Context(), bankKey, source, importOptions{activate: false, sourceSHA256: sourceSHA, expectedBankID: bankID, beforeCommit: func(tx pgx.Tx, report ImportReport) error {
		queries := store.New(tx)
		if err := lockIdempotency(request.Context(), queries, actor.key, operationKind, idempotencyKey); err != nil {
			return err
		}
		if status, body, found, conflict, err := loadIdempotency(request.Context(), queries, actor.key, operationKind, idempotencyKey, requestHash); err != nil {
			return err
		} else if conflict {
			return workshopHTTPError{http.StatusConflict, "idempotency_conflict", "idempotency key was already used with another request"}
		} else if found {
			encoded = body
			_ = status
			return errWorkshopReplay
		}
		var current int64
		if err := tx.QueryRow(request.Context(), `SELECT lifecycle_version FROM quizcraft_banks WHERE id=$1 FOR UPDATE`, bankID).Scan(&current); err != nil {
			return workshopHTTPError{http.StatusConflict, "bank_not_found", "Workshop bank was not found"}
		}
		effectiveExpected := expectedVersion
		if action == "import_version" && effectiveExpected == -1 {
			effectiveExpected = current
		}
		if current != effectiveExpected {
			return workshopHTTPError{http.StatusConflict, "version_conflict", "Workshop bank version changed; refresh before retrying"}
		}
		versionID := uuid.MustParse(report.BankVersionID)
		result, err := tx.Exec(request.Context(), `INSERT INTO quizcraft_workshop_version_states(bank_id,bank_version_id,state,created_by) VALUES($1,$2,'draft',$3) ON CONFLICT DO NOTHING`, bankID, versionID, *actor.userID)
		if err != nil {
			return err
		}
		if result.RowsAffected() != 1 {
			return workshopHTTPError{http.StatusConflict, "version_conflict", "identical immutable version already exists"}
		}
		if _, err := tx.Exec(request.Context(), `UPDATE quizcraft_banks SET lifecycle_version=lifecycle_version+1,updated_at=now() WHERE id=$1`, bankID); err != nil {
			return err
		}
		outerRequestID := requestID()
		if err := insertWorkshopAudit(request.Context(), tx, *actor.userID, "quizcraft.workshop.write", action, bankID, versionID, effectiveExpected, outerRequestID, ""); err != nil {
			return err
		}
		if action == "import_version" {
			encoded, _ = json.Marshal(responseEnvelope{RequestID: outerRequestID, Data: report})
		} else {
			encoded, _ = json.Marshal(responseEnvelope{RequestID: outerRequestID, Data: map[string]any{"operation_id": uuid.NewSHA1(versionID, []byte(operationKind+":"+idempotencyKey)), "state": "succeeded", "idempotency_key": idempotencyKey, "request_id": outerRequestID, "resource_id": versionID}})
		}
		return storeIdempotency(request.Context(), queries, actor.key, operationKind, idempotencyKey, requestHash, http.StatusCreated, encoded, versionID)
	}})
	if errors.Is(importErr, errWorkshopReplay) {
		writeRawJSON(writer, http.StatusCreated, encoded)
		return
	}
	if importErr != nil {
		var validationError ImportValidationError
		if errors.As(importErr, &validationError) {
			writeJSON(writer, http.StatusUnprocessableEntity, responseEnvelope{RequestID: requestID(), Data: report})
			return
		}
		var problem workshopHTTPError
		if errors.As(importErr, &problem) {
			writeError(writer, problem.status, problem.code, problem.message)
			return
		}
		writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft Workshop is temporarily unavailable")
		return
	}
	writeRawJSON(writer, http.StatusCreated, encoded)
}

func (service *practiceHTTP) validateWorkshopVersion(writer http.ResponseWriter, request *http.Request) {
	service.workshopVersionCommand(writer, request, "quizcraft.workshop.write", "validate_version", "validate_version")
}

func (service *practiceHTTP) publishWorkshopVersion(writer http.ResponseWriter, request *http.Request) {
	service.workshopVersionCommand(writer, request, "quizcraft.workshop.publish", "publish_version", "publish_version")
}

func (service *practiceHTTP) unpublishWorkshopVersion(writer http.ResponseWriter, request *http.Request) {
	service.workshopVersionCommand(writer, request, "quizcraft.workshop.publish", "unpublish_version", "unpublish_version")
}

func (service *practiceHTTP) workshopVersionCommand(writer http.ResponseWriter, request *http.Request, permission, operationKind, action string) {
	bankID, versionID, ok := workshopIDs(writer, request, true)
	if !ok {
		return
	}
	actor, _, allowed := service.workshopActor(writer, request, permission, &bankID, false)
	if !allowed {
		return
	}
	var input versionCommandRequest
	raw, err := decodeBody(request, &input)
	input.Note = strings.TrimSpace(input.Note)
	if err != nil || input.ExpectedVersion < 0 || len([]rune(input.Note)) > 1000 {
		writeError(writer, http.StatusBadRequest, "invalid_request", "Workshop command body is invalid")
		return
	}
	service.runWorkshopMutation(writer, request, actor, operationKind, http.StatusOK, raw, versionID, func(ctx context.Context, tx pgx.Tx, requestID string) error {
		if err := requireLifecycleVersion(ctx, tx, bankID, input.ExpectedVersion); err != nil {
			return err
		}
		switch action {
		case "validate_version":
			result, err := tx.Exec(ctx, `UPDATE quizcraft_workshop_version_states SET state='validated',validated_by=$3,validated_at=now(),validation_note=$4 WHERE bank_id=$1 AND bank_version_id=$2 AND state='draft'`, bankID, versionID, *actor.userID, input.Note)
			if err != nil || result.RowsAffected() != 1 {
				return workshopHTTPError{http.StatusConflict, "validation_conflict", "version is not an unvalidated Workshop draft"}
			}
		case "publish_version":
			result, err := tx.Exec(ctx, `UPDATE quizcraft_banks b SET active_version_id=$2 WHERE b.id=$1 AND EXISTS(SELECT 1 FROM quizcraft_workshop_version_states s WHERE s.bank_id=$1 AND s.bank_version_id=$2 AND s.state='validated')`, bankID, versionID)
			if err != nil || result.RowsAffected() != 1 {
				return workshopHTTPError{http.StatusConflict, "validation_required", "human validation is required before publication"}
			}
		case "unpublish_version":
			result, err := tx.Exec(ctx, `UPDATE quizcraft_banks SET active_version_id=NULL WHERE id=$1 AND active_version_id=$2`, bankID, versionID)
			if err != nil || result.RowsAffected() != 1 {
				return workshopHTTPError{http.StatusConflict, "publication_conflict", "version is not currently published"}
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE quizcraft_banks SET lifecycle_version=lifecycle_version+1,updated_at=now() WHERE id=$1`, bankID); err != nil {
			return err
		}
		return insertWorkshopAudit(ctx, tx, *actor.userID, permission, action, bankID, versionID, input.ExpectedVersion, requestID, input.Note)
	})
}

func (service *practiceHTTP) rollbackWorkshopBank(writer http.ResponseWriter, request *http.Request) {
	bankID, _, ok := workshopIDs(writer, request, false)
	if !ok {
		return
	}
	actor, _, allowed := service.workshopActor(writer, request, "quizcraft.workshop.publish", &bankID, false)
	if !allowed {
		return
	}
	var input rollbackCommandRequest
	raw, err := decodeBody(request, &input)
	input.Note = strings.TrimSpace(input.Note)
	if err != nil || input.ExpectedVersion < 0 || input.TargetBankVersionID == uuid.Nil || len([]rune(input.Note)) > 1000 {
		writeError(writer, http.StatusBadRequest, "invalid_request", "rollback command is invalid")
		return
	}
	service.runWorkshopMutation(writer, request, actor, "rollback_bank", http.StatusOK, raw, input.TargetBankVersionID, func(ctx context.Context, tx pgx.Tx, requestID string) error {
		if err := requireLifecycleVersion(ctx, tx, bankID, input.ExpectedVersion); err != nil {
			return err
		}
		result, err := tx.Exec(ctx, `UPDATE quizcraft_banks b SET active_version_id=$2,lifecycle_version=lifecycle_version+1,updated_at=now() WHERE b.id=$1 AND EXISTS(SELECT 1 FROM quizcraft_workshop_version_states s WHERE s.bank_id=$1 AND s.bank_version_id=$2 AND s.state='validated')`, bankID, input.TargetBankVersionID)
		if err != nil || result.RowsAffected() != 1 {
			return workshopHTTPError{http.StatusConflict, "rollback_conflict", "rollback target is not a validated version of this bank"}
		}
		return insertWorkshopAudit(ctx, tx, *actor.userID, "quizcraft.workshop.publish", "rollback_bank", bankID, input.TargetBankVersionID, input.ExpectedVersion, requestID, input.Note)
	})
}

func (service *practiceHTTP) runWorkshopMutation(writer http.ResponseWriter, request *http.Request, actor practiceActor, kind string, status int, raw []byte, resourceID uuid.UUID, mutate func(context.Context, pgx.Tx, string) error) {
	idempotencyKey, ok := requiredIdempotencyKey(writer, request)
	if !ok {
		return
	}
	requestHash := hashCanonical(raw)
	tx, err := service.database.BeginTx(request.Context(), pgx.TxOptions{})
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft Workshop is temporarily unavailable")
		return
	}
	defer func() { _ = tx.Rollback(request.Context()) }()
	queries := store.New(tx)
	if err := lockIdempotency(request.Context(), queries, actor.key, kind, idempotencyKey); err != nil {
		writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft Workshop is temporarily unavailable")
		return
	}
	if storedStatus, storedBody, found, conflict, err := loadIdempotency(request.Context(), queries, actor.key, kind, idempotencyKey, requestHash); err != nil {
		writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft Workshop is temporarily unavailable")
		return
	} else if conflict {
		writeError(writer, http.StatusConflict, "idempotency_conflict", "idempotency key was already used with another request")
		return
	} else if found {
		writeRawJSON(writer, storedStatus, storedBody)
		return
	}
	outerRequestID := requestID()
	if err := mutate(request.Context(), tx, outerRequestID); err != nil {
		var problem workshopHTTPError
		if errors.As(err, &problem) {
			writeError(writer, problem.status, problem.code, problem.message)
		} else {
			writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft Workshop is temporarily unavailable")
		}
		return
	}
	response := responseEnvelope{RequestID: outerRequestID, Data: map[string]any{"operation_id": uuid.NewSHA1(resourceID, []byte(kind+":"+idempotencyKey)), "state": "succeeded", "idempotency_key": idempotencyKey, "request_id": outerRequestID, "resource_id": resourceID}}
	encoded, _ := json.Marshal(response)
	if err := storeIdempotency(request.Context(), queries, actor.key, kind, idempotencyKey, requestHash, status, encoded, resourceID); err != nil {
		writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft Workshop is temporarily unavailable")
		return
	}
	if err := tx.Commit(request.Context()); err != nil {
		writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft Workshop is temporarily unavailable")
		return
	}
	writeRawJSON(writer, status, encoded)
}

func requireLifecycleVersion(ctx context.Context, tx pgx.Tx, bankID uuid.UUID, expected int64) error {
	var current int64
	if err := tx.QueryRow(ctx, `SELECT lifecycle_version FROM quizcraft_banks WHERE id=$1 FOR UPDATE`, bankID).Scan(&current); err != nil {
		return workshopHTTPError{http.StatusConflict, "bank_not_found", "Workshop bank was not found"}
	}
	if current != expected {
		return workshopHTTPError{http.StatusConflict, "version_conflict", "Workshop bank version changed; refresh before retrying"}
	}
	return nil
}

func insertWorkshopAudit(ctx context.Context, tx pgx.Tx, actorID uuid.UUID, permission, action string, bankID, versionID uuid.UUID, expected int64, requestID, note string) error {
	var nullableVersion any
	if versionID != uuid.Nil {
		nullableVersion = versionID
	}
	_, err := tx.Exec(ctx, `INSERT INTO quizcraft_workshop_audit_events(id,actor_user_id,permission_code,action,bank_id,bank_version_id,expected_version,resulting_version,request_id,note) VALUES($1,$2,$3,$4,$5,$6,$7::bigint,$7::bigint+1,$8,$9)`, uuid.New(), actorID, permission, action, bankID, nullableVersion, expected, requestID, note)
	return err
}

func workshopIDs(writer http.ResponseWriter, request *http.Request, withVersion bool) (uuid.UUID, uuid.UUID, bool) {
	bankID, err := uuid.Parse(chi.URLParam(request, "bank_id"))
	if err != nil {
		writeError(writer, http.StatusBadRequest, "invalid_bank_id", "bank_id must be a UUID")
		return uuid.Nil, uuid.Nil, false
	}
	if !withVersion {
		return bankID, uuid.Nil, true
	}
	versionID, err := uuid.Parse(chi.URLParam(request, "bank_version_id"))
	if err != nil {
		writeError(writer, http.StatusBadRequest, "invalid_bank_version_id", "bank_version_id must be a UUID")
		return uuid.Nil, uuid.Nil, false
	}
	return bankID, versionID, true
}

func (service *practiceHTTP) createFeedback(writer http.ResponseWriter, request *http.Request) {
	actor, status, err := service.actor(writer, request)
	if err != nil {
		writeError(writer, status, "invalid_session", err.Error())
		return
	}
	idempotencyKey, ok := requiredIdempotencyKey(writer, request)
	if !ok {
		return
	}
	var input questionFeedbackRequest
	raw, err := decodeBody(request, &input)
	input.Detail = strings.TrimSpace(input.Detail)
	if err != nil || input.BankID == uuid.Nil || input.QuestionID == uuid.Nil || input.QuestionVersionID == uuid.Nil || !validFeedbackCategory(input.Category) || len([]rune(input.Detail)) < 1 || len([]rune(input.Detail)) > 4000 {
		writeError(writer, http.StatusBadRequest, "invalid_feedback", "feedback body or stable question reference is invalid")
		return
	}
	tx, err := service.database.BeginTx(request.Context(), pgx.TxOptions{})
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft feedback is temporarily unavailable")
		return
	}
	defer func() { _ = tx.Rollback(request.Context()) }()
	queries := store.New(tx)
	if err := lockIdempotency(request.Context(), queries, actor.key, "create_feedback", idempotencyKey); err != nil {
		writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft feedback is temporarily unavailable")
		return
	}
	requestHash := hashCanonical(raw)
	if storedStatus, storedBody, found, conflict, err := loadIdempotency(request.Context(), queries, actor.key, "create_feedback", idempotencyKey, requestHash); err != nil {
		writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft feedback is temporarily unavailable")
		return
	} else if conflict {
		writeError(writer, http.StatusConflict, "idempotency_conflict", "idempotency key was already used with another request")
		return
	} else if found {
		writeRawJSON(writer, storedStatus, storedBody)
		return
	}
	var exists bool
	if err := tx.QueryRow(request.Context(), `SELECT EXISTS(SELECT 1 FROM quizcraft_question_versions WHERE bank_id=$1 AND question_id=$2 AND id=$3)`, input.BankID, input.QuestionID, input.QuestionVersionID).Scan(&exists); err != nil || !exists {
		writeError(writer, http.StatusBadRequest, "invalid_question_reference", "feedback must reference one stored bank, question, and version")
		return
	}
	feedbackID := uuid.New()
	var actorUserID any
	if actor.userID != nil {
		actorUserID = *actor.userID
	}
	if _, err := tx.Exec(request.Context(), `INSERT INTO quizcraft_feedbacks(id,bank_id,question_id,question_version_id,actor_user_id,actor_key,category,detail) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, feedbackID, input.BankID, input.QuestionID, input.QuestionVersionID, actorUserID, actor.key, input.Category, input.Detail); err != nil {
		writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft feedback is temporarily unavailable")
		return
	}
	if _, err := tx.Exec(request.Context(), `INSERT INTO quizcraft_feedback_status_facts(id,feedback_id,status,source,source_event_id,occurred_at) VALUES($1,$2,'pending','submission','submitted',now())`, uuid.New(), feedbackID); err != nil {
		writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft feedback is temporarily unavailable")
		return
	}
	priority := "normal"
	if input.Category == "wrong_answer" {
		priority = "high"
	}
	resourceOrigin := service.publicURL
	if resourceOrigin == "" {
		resourceOrigin = "https://quizcraft.invalid"
	}
	resourceURL := fmt.Sprintf("%s/workshop/feedback/%s", resourceOrigin, feedbackID)
	outboxID := uuid.New()
	if _, err := tx.Exec(request.Context(), `INSERT INTO quizcraft_feedback_inbox_outbox(id,feedback_id,source_resource_id,source_resource_url,category,priority) VALUES($1,$2::uuid,($2::uuid)::text,$3,$4,$5)`, outboxID, feedbackID, resourceURL, input.Category, priority); err != nil {
		writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft feedback is temporarily unavailable")
		return
	}
	if _, err := tx.Exec(request.Context(), `INSERT INTO quizcraft_feedback_inbox_deliveries(outbox_id) VALUES($1)`, outboxID); err != nil {
		writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft feedback is temporarily unavailable")
		return
	}
	outerRequestID := requestID()
	response := responseEnvelope{RequestID: outerRequestID, Data: map[string]any{"operation_id": uuid.NewSHA1(feedbackID, []byte("create_feedback:"+idempotencyKey)), "state": "succeeded", "idempotency_key": idempotencyKey, "request_id": outerRequestID, "resource_id": feedbackID}}
	encoded, _ := json.Marshal(response)
	if err := storeIdempotency(request.Context(), queries, actor.key, "create_feedback", idempotencyKey, requestHash, http.StatusAccepted, encoded, feedbackID); err != nil {
		writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft feedback is temporarily unavailable")
		return
	}
	if err := tx.Commit(request.Context()); err != nil {
		writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft feedback is temporarily unavailable")
		return
	}
	if service.inboxExchangeToken != "" {
		select {
		case service.inboxDispatchWake <- struct{}{}:
		default:
		}
	}
	writeRawJSON(writer, http.StatusAccepted, encoded)
}

func (service *practiceHTTP) getFeedbackStatus(writer http.ResponseWriter, request *http.Request) {
	feedbackID, err := uuid.Parse(chi.URLParam(request, "feedback_id"))
	if err != nil {
		writeError(writer, http.StatusBadRequest, "invalid_feedback_id", "feedback_id must be a UUID")
		return
	}
	actor, status, err := service.actor(writer, request)
	if err != nil {
		writeError(writer, status, "invalid_session", err.Error())
		return
	}
	_, err = service.loadFeedbackStatus(request.Context(), feedbackID, actor.key)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Treat a foreign identifier as absent so feedback IDs cannot be enumerated.
			writeError(writer, http.StatusNotFound, "feedback_not_found", "feedback was not found")
		} else {
			writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft feedback is temporarily unavailable")
		}
		return
	}
	if err := service.syncFeedbackStatusFromInbox(request.Context(), feedbackID); err != nil {
		writeError(writer, http.StatusServiceUnavailable, "feedback_status_unavailable", "feedback was saved but its processing status is temporarily unavailable")
		return
	}
	feedback, err := service.loadFeedbackStatus(request.Context(), feedbackID, actor.key)
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft feedback is temporarily unavailable")
		return
	}
	writeJSON(writer, http.StatusOK, responseEnvelope{RequestID: requestID(), Data: feedbackStatusResponse(feedback)})
}

func (service *practiceHTTP) listFeedbackStatuses(writer http.ResponseWriter, request *http.Request) {
	actor, status, err := service.actor(writer, request)
	if err != nil {
		writeError(writer, status, "invalid_session", err.Error())
		return
	}
	rows, err := service.database.Query(request.Context(), `SELECT f.id,f.bank_id,f.question_id,f.question_version_id,f.category,s.status,f.created_at,s.recorded_at FROM quizcraft_feedbacks f JOIN LATERAL (SELECT status,recorded_at FROM quizcraft_feedback_status_facts WHERE feedback_id=f.id ORDER BY CASE WHEN source='operations_inbox' THEN 1 ELSE 0 END DESC,source_version DESC NULLS LAST,recorded_at DESC,occurred_at DESC,id DESC LIMIT 1) s ON true WHERE f.actor_key=$1 ORDER BY f.created_at DESC,f.id DESC LIMIT 100`, actor.key)
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft feedback is temporarily unavailable")
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var feedback feedbackStatusProjection
		if err := rows.Scan(&feedback.FeedbackID, &feedback.BankID, &feedback.QuestionID, &feedback.QuestionVersionID, &feedback.Category, &feedback.Status, &feedback.CreatedAt, &feedback.UpdatedAt); err != nil {
			writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft feedback is temporarily unavailable")
			return
		}
		items = append(items, feedbackStatusResponse(feedback))
	}
	if err := rows.Err(); err != nil {
		writeError(writer, http.StatusServiceUnavailable, "database_unavailable", "QuizCraft feedback is temporarily unavailable")
		return
	}
	writeJSON(writer, http.StatusOK, responseEnvelope{RequestID: requestID(), Data: map[string]any{"items": items}})
}

func (service *practiceHTTP) loadFeedbackStatus(ctx context.Context, feedbackID uuid.UUID, actorKey string) (feedbackStatusProjection, error) {
	var feedback feedbackStatusProjection
	err := service.database.QueryRow(ctx, `SELECT f.id,f.bank_id,f.question_id,f.question_version_id,f.category,s.status,f.created_at,s.recorded_at FROM quizcraft_feedbacks f JOIN LATERAL (SELECT status,recorded_at FROM quizcraft_feedback_status_facts WHERE feedback_id=f.id ORDER BY CASE WHEN source='operations_inbox' THEN 1 ELSE 0 END DESC,source_version DESC NULLS LAST,recorded_at DESC,occurred_at DESC,id DESC LIMIT 1) s ON true WHERE f.id=$1 AND f.actor_key=$2`, feedbackID, actorKey).Scan(&feedback.FeedbackID, &feedback.BankID, &feedback.QuestionID, &feedback.QuestionVersionID, &feedback.Category, &feedback.Status, &feedback.CreatedAt, &feedback.UpdatedAt)
	return feedback, err
}

func (service *practiceHTTP) syncFeedbackStatusFromInbox(ctx context.Context, feedbackID uuid.UUID) error {
	if service.platform == nil || service.inboxExchangeToken == "" {
		return nil
	}
	var platformItemID uuid.UUID
	err := service.database.QueryRow(ctx, `SELECT d.platform_item_id FROM quizcraft_feedback_inbox_outbox o JOIN quizcraft_feedback_inbox_deliveries d ON d.outbox_id=o.id WHERE o.feedback_id=$1 AND d.platform_item_id IS NOT NULL`, feedbackID).Scan(&platformItemID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	item, err := service.platform.getInboxItem(ctx, service.inboxExchangeToken, platformItemID, "quizcraft", "feedback", feedbackID.String())
	if err != nil {
		return err
	}
	if item.SourceProductCode != "quizcraft" || item.SourceResourceType != "feedback" || item.SourceResourceID != feedbackID.String() {
		return errors.New("operations inbox returned a mismatched feedback reference")
	}
	feedbackStatus, ok := feedbackStatusFromInbox(item.Status)
	if !ok {
		return errors.New("operations inbox returned an invalid feedback status")
	}
	eventID := "operations-inbox:" + item.ID + ":" + strconv.FormatInt(item.Version, 10)
	_, err = service.database.Exec(ctx, `INSERT INTO quizcraft_feedback_status_facts(id,feedback_id,status,source,source_event_id,source_version,occurred_at) SELECT $1,$2,$3,'operations_inbox',$4,$5,$6 WHERE NOT EXISTS (SELECT 1 FROM quizcraft_feedback_status_facts WHERE feedback_id=$2 AND source='operations_inbox' AND source_version>$5) ON CONFLICT(feedback_id,source_event_id) DO NOTHING`, uuid.New(), feedbackID, feedbackStatus, eventID, item.Version, item.UpdatedAt.UTC())
	return err
}

func feedbackStatusFromInbox(value string) (string, bool) {
	switch value {
	case "open":
		return "pending", true
	case "in_progress":
		return "in_progress", true
	case "blocked":
		return "blocked", true
	case "resolved":
		return "resolved", true
	case "cancelled":
		return "archived", true
	default:
		return "", false
	}
}

func feedbackStatusResponse(feedback feedbackStatusProjection) map[string]any {
	return map[string]any{
		"feedback_id":         feedback.FeedbackID,
		"bank_id":             feedback.BankID,
		"question_id":         feedback.QuestionID,
		"question_version_id": feedback.QuestionVersionID,
		"category":            feedback.Category,
		"status":              feedback.Status,
		"created_at":          feedback.CreatedAt.UTC(),
		"updated_at":          feedback.UpdatedAt.UTC(),
	}
}

func validFeedbackCategory(value string) bool {
	return value == "wrong_answer" || value == "ambiguous" || value == "typo" || value == "outdated" || value == "other"
}
