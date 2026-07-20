package quizcraft

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	quizcraftNamespace = uuid.MustParse("c4d37db2-25ce-5f26-a0a8-9c67f9757f4d")
	bankKeyPattern     = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{1,78}[a-z0-9]$`)
)

type Config struct {
	Database                     *pgxpool.Pool
	AllowTestBootstrapActivation bool
}

type Service struct {
	database                     *pgxpool.Pool
	allowTestBootstrapActivation bool
}

func New(config Config) (*Service, error) {
	if config.Database == nil {
		return nil, errors.New("quizcraft database is required")
	}
	return &Service{database: config.Database, allowTestBootstrapActivation: config.AllowTestBootstrapActivation}, nil
}

type ValidationError struct {
	Path    string `json:"path"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ImportedQuestion struct {
	SourceQuestionID  string `json:"source_question_id"`
	QuestionID        string `json:"question_id"`
	QuestionVersionID string `json:"question_version_id"`
	ContentSHA256     string `json:"content_sha256"`
	AnswerSHA256      string `json:"answer_sha256"`
}

type ImportReport struct {
	Accepted        bool               `json:"accepted"`
	BankID          string             `json:"bank_id,omitempty"`
	BankVersionID   string             `json:"bank_version_id,omitempty"`
	SourceSHA256    string             `json:"source_sha256"`
	ContentSHA256   string             `json:"content_sha256,omitempty"`
	QuestionCount   int                `json:"question_count"`
	AnsweredCount   int                `json:"answered_count"`
	UnansweredCount int                `json:"unanswered_count"`
	TypeCounts      map[string]int     `json:"type_counts"`
	ChapterCounts   map[string]int     `json:"chapter_counts"`
	Questions       []ImportedQuestion `json:"questions"`
	Errors          []ValidationError  `json:"errors"`
}

type ImportValidationError struct{ Count int }

func (e ImportValidationError) Error() string {
	return fmt.Sprintf("quizcraft import has %d validation errors", e.Count)
}

type bankDocument struct {
	Meta      bankMeta        `json:"meta"`
	Questions []questionInput `json:"questions"`
	Chunks    json.RawMessage `json:"chunks"`
}

type bankMeta struct {
	Name          string          `json:"name"`
	Version       string          `json:"version"`
	Chapters      json.RawMessage `json:"chapters"`
	Color         string          `json:"color"`
	Total         int             `json:"total"`
	Key           string          `json:"key"`
	Subject       string          `json:"subject"`
	SourceFile    string          `json:"source_file"`
	SourceFiles   json.RawMessage `json:"source_files"`
	SourceSeasons json.RawMessage `json:"source_seasons"`
	CreatedAt     string          `json:"created_at"`
	ChapterCount  int             `json:"chapter_count"`
	ChunkCount    int             `json:"chunk_count"`
	ReviewRemoved int             `json:"review_removed"`
}

type questionInput struct {
	ID           string          `json:"id"`
	Type         string          `json:"type"`
	ChapterID    string          `json:"chapter_id"`
	Chapter      string          `json:"chapter"`
	Content      string          `json:"content"`
	Options      json.RawMessage `json:"options"`
	Answer       json.RawMessage `json:"answer"`
	Analysis     string          `json:"analysis"`
	Number       json.RawMessage `json:"number"`
	Stats        json.RawMessage `json:"stats"`
	RAGContext   json.RawMessage `json:"rag_context"`
	RAGRefs      json.RawMessage `json:"rag_refs"`
	Source       json.RawMessage `json:"source"`
	SourceSeason json.RawMessage `json:"source_season"`
}

type normalizedQuestion struct {
	SourceID, Type, ChapterID, ChapterName, Content, Analysis string
	Options, Answer                                           any
	ContentHash                                               string
	QuestionID, VersionID                                     uuid.UUID
}

func (s *Service) ImportJSON(ctx context.Context, bankKey string, source []byte) (ImportReport, error) {
	if !s.allowTestBootstrapActivation {
		return ImportReport{}, errors.New("direct bank activation is disabled; use the authenticated Workshop lifecycle")
	}
	return s.importJSON(ctx, bankKey, source, importOptions{activate: true})
}

type importOptions struct {
	activate       bool
	sourceSHA256   string
	expectedBankID uuid.UUID
	beforeCommit   func(pgx.Tx, ImportReport) error
}

func (s *Service) importJSON(ctx context.Context, bankKey string, source []byte, options importOptions) (ImportReport, error) {
	sourceSHA256 := hash(source)
	if options.sourceSHA256 != "" {
		sourceSHA256 = options.sourceSHA256
	}
	report := ImportReport{
		SourceSHA256:  sourceSHA256,
		TypeCounts:    map[string]int{},
		ChapterCounts: map[string]int{},
		Questions:     []ImportedQuestion{},
		Errors:        []ValidationError{},
	}
	bankKey = strings.TrimSpace(bankKey)
	if !bankKeyPattern.MatchString(bankKey) {
		report.Errors = append(report.Errors, validation("bank_key", "invalid_bank_key", "bank key must be a stable lowercase key"))
	}

	var document bankDocument
	decoder := json.NewDecoder(bytes.NewReader(source))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		report.Errors = append(report.Errors, validation("$", "invalid_json", err.Error()))
		return report, ImportValidationError{len(report.Errors)}
	}
	if err := ensureJSONEOF(decoder); err != nil {
		report.Errors = append(report.Errors, validation("$", "trailing_json", err.Error()))
		return report, ImportValidationError{len(report.Errors)}
	}
	if strings.TrimSpace(document.Meta.Name) == "" {
		report.Errors = append(report.Errors, validation("meta.name", "required", "bank name is required"))
	} else if len([]rune(strings.TrimSpace(document.Meta.Name))) > 160 {
		report.Errors = append(report.Errors, validation("meta.name", "too_long", "bank name must not exceed 160 characters"))
	}
	if len(document.Questions) == 0 {
		report.Errors = append(report.Errors, validation("questions", "required", "at least one question is required"))
	}

	seen := map[string]bool{}
	normalized := make([]normalizedQuestion, 0, len(document.Questions))
	bankID := stableID(quizcraftNamespace, "bank:"+bankKey)
	if options.expectedBankID != uuid.Nil && options.expectedBankID != bankID {
		report.Errors = append(report.Errors, validation("bank_id", "bank_identity_conflict", "bank_id does not match the stable bank key"))
	}
	for index, input := range document.Questions {
		path := fmt.Sprintf("questions[%d]", index)
		sourceID := strings.TrimSpace(input.ID)
		kind := strings.TrimSpace(input.Type)
		chapterID := strings.TrimSpace(input.ChapterID)
		chapterName := strings.TrimSpace(input.Chapter)
		content := strings.TrimSpace(input.Content)

		if sourceID == "" {
			report.Errors = append(report.Errors, validation(path+".id", "required", "stable source question id is required"))
		} else if seen[sourceID] {
			report.Errors = append(report.Errors, validation(path+".id", "duplicate", "source question id must be unique"))
		} else if len([]rune(sourceID)) > 160 {
			report.Errors = append(report.Errors, validation(path+".id", "too_long", "source question id must not exceed 160 characters"))
		}
		seen[sourceID] = true
		if !validQuestionType(kind) {
			report.Errors = append(report.Errors, validation(path+".type", "invalid_type", "type must be single, multi, judge, or blank"))
		} else {
			report.TypeCounts[kind]++
		}
		if chapterID == "" && chapterName == "" {
			report.Errors = append(report.Errors, validation(path+".chapter", "required", "chapter id or name is required"))
		} else if chapterID == "" {
			chapterID = "chapter-" + hash([]byte(chapterName))[:12]
		}
		if chapterName == "" {
			chapterName = chapterID
		}
		if chapterID != "" {
			report.ChapterCounts[chapterID]++
		}
		if len([]rune(chapterID)) > 160 {
			report.Errors = append(report.Errors, validation(path+".chapter_id", "too_long", "chapter id must not exceed 160 characters"))
		}
		if len([]rune(chapterName)) > 240 {
			report.Errors = append(report.Errors, validation(path+".chapter", "too_long", "chapter name must not exceed 240 characters"))
		}
		if content == "" {
			report.Errors = append(report.Errors, validation(path+".content", "required", "question content is required"))
		} else if len([]rune(content)) > 10000 {
			report.Errors = append(report.Errors, validation(path+".content", "too_long", "question content must not exceed 10000 characters"))
		}

		answer, answered := decodeRequiredJSON(input.Answer)
		if !answered {
			report.UnansweredCount++
			report.Errors = append(report.Errors, validation(path+".answer", "required", "question answer is required"))
		} else {
			report.AnsweredCount++
		}
		options, _ := decodeOptionalJSON(input.Options)
		report.Errors = append(report.Errors, validateQuestionShape(path, kind, options, answer)...)

		semantic := map[string]any{
			"type": kind, "chapter_id": chapterID, "chapter": chapterName,
			"content": content, "options": options, "answer": answer, "analysis": input.Analysis,
		}
		canonical, _ := json.Marshal(semantic)
		contentHash := hash(canonical)
		answerCanonical, _ := json.Marshal(answer)
		answerHash := hash(answerCanonical)
		questionID := stableID(bankID, "question:"+sourceID)
		versionID := stableID(questionID, "version:"+contentHash)
		normalized = append(normalized, normalizedQuestion{
			SourceID: sourceID, Type: kind, ChapterID: chapterID, ChapterName: chapterName,
			Content: content, Analysis: input.Analysis, Options: options, Answer: answer,
			ContentHash: contentHash, QuestionID: questionID, VersionID: versionID,
		})
		report.Questions = append(report.Questions, ImportedQuestion{
			SourceQuestionID: sourceID, QuestionID: questionID.String(),
			QuestionVersionID: versionID.String(), ContentSHA256: contentHash, AnswerSHA256: answerHash,
		})
	}

	report.QuestionCount = len(document.Questions)
	report.BankID = bankID.String()
	bankSemantic := map[string]any{
		"bank_key": bankKey, "name": strings.TrimSpace(document.Meta.Name),
		"source_version": document.Meta.Version, "questions": report.Questions,
	}
	canonical, _ := json.Marshal(bankSemantic)
	report.ContentSHA256 = hash(canonical)
	bankVersionID := stableID(bankID, "version:"+report.ContentSHA256)
	report.BankVersionID = bankVersionID.String()
	if len(report.Errors) > 0 {
		return report, ImportValidationError{len(report.Errors)}
	}

	storedReport := report
	storedReport.Accepted = true
	encodedReport, _ := json.Marshal(storedReport)
	tx, err := s.database.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return report, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, bankKey); err != nil {
		return report, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO quizcraft_banks(id,bank_key,name) VALUES($1,$2,$3) ON CONFLICT(bank_key) DO UPDATE SET name=EXCLUDED.name,updated_at=now()`, bankID, bankKey, strings.TrimSpace(document.Meta.Name)); err != nil {
		return report, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO quizcraft_bank_versions(id,bank_id,name,source_version,source_sha256,content_sha256,import_report) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(id) DO NOTHING`, bankVersionID, bankID, strings.TrimSpace(document.Meta.Name), document.Meta.Version, report.SourceSHA256, report.ContentSHA256, encodedReport); err != nil {
		return report, err
	}
	for position, item := range normalized {
		if _, err = tx.Exec(ctx, `INSERT INTO quizcraft_questions(id,bank_id,source_question_id) VALUES($1,$2,$3) ON CONFLICT(bank_id,source_question_id) DO NOTHING`, item.QuestionID, bankID, item.SourceID); err != nil {
			return report, err
		}
		optionsJSON, _ := json.Marshal(item.Options)
		answerJSON, _ := json.Marshal(item.Answer)
		if _, err = tx.Exec(ctx, `INSERT INTO quizcraft_question_versions(id,bank_id,question_id,type,chapter_id,chapter_name,content,options,answer,analysis,content_sha256) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) ON CONFLICT(id) DO NOTHING`, item.VersionID, bankID, item.QuestionID, item.Type, item.ChapterID, item.ChapterName, item.Content, string(optionsJSON), string(answerJSON), item.Analysis, item.ContentHash); err != nil {
			return report, err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO quizcraft_bank_version_questions(bank_id,bank_version_id,question_id,question_version_id,position) VALUES($1,$2,$3,$4,$5) ON CONFLICT(bank_version_id,question_id) DO NOTHING`, bankID, bankVersionID, item.QuestionID, item.VersionID, position+1); err != nil {
			return report, err
		}
		var storedBankID, storedVersionID uuid.UUID
		var storedPosition int
		if err = tx.QueryRow(ctx, `SELECT bank_id,question_version_id,position FROM quizcraft_bank_version_questions WHERE bank_version_id=$1 AND question_id=$2`, bankVersionID, item.QuestionID).Scan(&storedBankID, &storedVersionID, &storedPosition); err != nil {
			return report, err
		}
		if storedBankID != bankID || storedVersionID != item.VersionID || storedPosition != position+1 {
			return report, fmt.Errorf("immutable bank version membership conflict for question %s", item.SourceID)
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE quizcraft_bank_versions SET sealed_at=now() WHERE id=$1 AND sealed_at IS NULL`, bankVersionID); err != nil {
		return report, err
	}
	if options.activate {
		if _, err = tx.Exec(ctx, `UPDATE quizcraft_banks SET active_version_id=$2,updated_at=now() WHERE id=$1`, bankID, bankVersionID); err != nil {
			return report, err
		}
	}
	report.Accepted = true
	if options.beforeCommit != nil {
		if err = options.beforeCommit(tx, report); err != nil {
			return report, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return report, err
	}
	return report, nil
}

func stableID(namespace uuid.UUID, value string) uuid.UUID {
	return uuid.NewSHA1(namespace, []byte(value))
}

func hash(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

func validation(path, code, message string) ValidationError {
	return ValidationError{Path: path, Code: code, Message: message}
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); errors.Is(err, io.EOF) {
		return nil
	} else if err != nil {
		return err
	}
	return errors.New("only one JSON document is allowed")
}

func validQuestionType(kind string) bool {
	return kind == "single" || kind == "multi" || kind == "judge" || kind == "blank"
}

func validateQuestionShape(path, kind string, options, answer any) []ValidationError {
	errorsFound := []ValidationError{}
	optionList, hasOptions := options.([]any)
	if options != nil && !hasOptions {
		errorsFound = append(errorsFound, validation(path+".options", "invalid_options", "options must be an array of non-empty text"))
	}
	if hasOptions {
		for index, option := range optionList {
			text, ok := option.(string)
			if !ok || strings.TrimSpace(text) == "" {
				errorsFound = append(errorsFound, validation(fmt.Sprintf("%s.options[%d]", path, index), "invalid_option", "options must contain non-empty text"))
			}
		}
	}
	switch kind {
	case "single":
		if !hasOptions || len(optionList) < 2 {
			errorsFound = append(errorsFound, validation(path+".options", "invalid_options", "single choice requires at least two options"))
		}
		if number, numeric := answer.(float64); numeric {
			if number < 0 || number != float64(int(number)) || int(number) >= len(optionList) {
				errorsFound = append(errorsFound, validation(path+".answer", "invalid_answer", "single choice index must identify an option"))
			}
		} else if text, textual := answer.(string); !textual || !containsOption(optionList, text) {
			errorsFound = append(errorsFound, validation(path+".answer", "invalid_answer", "single choice answer must be an option index or value"))
		}
	case "multi":
		if !hasOptions || len(optionList) < 2 {
			errorsFound = append(errorsFound, validation(path+".options", "invalid_options", "multiple choice requires at least two options"))
		}
		if answers, ok := answer.([]any); !ok || len(answers) == 0 {
			errorsFound = append(errorsFound, validation(path+".answer", "invalid_answer", "multiple choice answer must be a non-empty array"))
		} else {
			for _, selected := range answers {
				if !validSelectedOption(optionList, selected) {
					errorsFound = append(errorsFound, validation(path+".answer", "invalid_answer", "every multiple choice answer must identify an option"))
					break
				}
			}
		}
	case "judge":
		if _, ok := answer.(bool); !ok {
			errorsFound = append(errorsFound, validation(path+".answer", "invalid_answer", "judge answer must be a boolean"))
		}
	case "blank":
		if text, ok := answer.(string); ok {
			if strings.TrimSpace(text) == "" {
				errorsFound = append(errorsFound, validation(path+".answer", "invalid_answer", "blank answer must not be empty"))
			}
		} else if answers, list := answer.([]any); !list || !nonEmptyTextList(answers) {
			errorsFound = append(errorsFound, validation(path+".answer", "invalid_answer", "blank answer must be text or a non-empty array"))
		}
	}
	return errorsFound
}

func containsOption(options []any, selected string) bool {
	for _, option := range options {
		if text, ok := option.(string); ok && text == selected {
			return true
		}
	}
	return false
}

func validSelectedOption(options []any, selected any) bool {
	if number, ok := selected.(float64); ok {
		return number >= 0 && number == float64(int(number)) && int(number) < len(options)
	}
	if text, ok := selected.(string); ok {
		return containsOption(options, text)
	}
	return false
}

func nonEmptyTextList(values []any) bool {
	if len(values) == 0 {
		return false
	}
	for _, value := range values {
		text, ok := value.(string)
		if !ok || strings.TrimSpace(text) == "" {
			return false
		}
	}
	return true
}

func decodeRequiredJSON(raw json.RawMessage) (any, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, false
	}
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return nil, false
	}
	switch item := value.(type) {
	case string:
		return item, item != ""
	case []any:
		return item, len(item) > 0
	default:
		return value, true
	}
}

func decodeOptionalJSON(raw json.RawMessage) (any, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, false
	}
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return nil, false
	}
	return value, true
}
