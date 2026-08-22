package quizcraft

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"henukit.dev/quizcraft/internal/store"
)

// PracticeProbeReport identifies the published content exercised by
// VerifyPracticeFlow. It intentionally excludes the stored answer.
type PracticeProbeReport struct {
	BankID     uuid.UUID
	QuestionID uuid.UUID
}

// VerifyPracticeFlow exercises the same published-bank selection, session,
// scoring, attempt, and question-stat statements used by the HTTP handlers.
// The transaction is always rolled back and the unique probe session is
// checked afterwards, so a release check cannot affect learner history,
// difficulty statistics, or rankings.
func VerifyPracticeFlow(ctx context.Context, database *pgxpool.Pool) (PracticeProbeReport, error) {
	if database == nil {
		return PracticeProbeReport{}, errors.New("practice probe database is required")
	}
	tx, err := database.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return PracticeProbeReport{}, fmt.Errorf("begin practice probe: %w", err)
	}
	rolledBack := false
	defer func() {
		if !rolledBack {
			_ = tx.Rollback(context.WithoutCancel(ctx))
		}
	}()

	queries := store.New(tx)
	banks, err := queries.ListPublishedBanks(ctx)
	if err != nil {
		return PracticeProbeReport{}, fmt.Errorf("list published banks: %w", err)
	}
	var bankID, bankVersionID uuid.UUID
	for _, bank := range banks {
		if bank.ActiveVersionID.Valid && bank.QuestionCount > 0 {
			bankID = bank.ID
			bankVersionID = bank.ActiveVersionID.UUID
			break
		}
	}
	if bankID == uuid.Nil {
		return PracticeProbeReport{}, errors.New("no published QuizCraft bank with questions")
	}
	if _, err := queries.IsPublishedBankVersion(ctx, store.IsPublishedBankVersionParams{ID: bankID, ID_2: bankVersionID}); err != nil {
		return PracticeProbeReport{}, fmt.Errorf("verify published bank version: %w", err)
	}

	sessionID := uuid.New()
	questions, err := queries.ListPracticeQuestions(ctx, store.ListPracticeQuestionsParams{
		BankVersionID: bankVersionID,
		SessionID:     sessionID,
		QuestionCount: 1,
	})
	if err != nil {
		return PracticeProbeReport{}, fmt.Errorf("select practice question: %w", err)
	}
	if len(questions) != 1 {
		return PracticeProbeReport{}, errors.New("published QuizCraft bank returned no practice question")
	}
	selected := questions[0]
	if err := queries.CreatePracticeSession(ctx, store.CreatePracticeSessionParams{
		ID:            sessionID,
		BankID:        bankID,
		BankVersionID: bankVersionID,
		UserID:        uuid.NullUUID{},
		ActorKey:      "guest:" + sessionID.String(),
		Mode:          "random",
		ChapterID:     pgtype.Text{},
	}); err != nil {
		return PracticeProbeReport{}, fmt.Errorf("create practice probe session: %w", err)
	}
	if err := queries.AddPracticeSessionQuestion(ctx, store.AddPracticeSessionQuestionParams{
		SessionID:         sessionID,
		BankID:            bankID,
		BankVersionID:     bankVersionID,
		QuestionID:        selected.QuestionID,
		QuestionVersionID: selected.QuestionVersionID,
		Position:          1,
	}); err != nil {
		return PracticeProbeReport{}, fmt.Errorf("attach practice probe question: %w", err)
	}

	question, err := queries.GetSessionQuestion(ctx, store.GetSessionQuestionParams{
		ID:                sessionID,
		QuestionID:        selected.QuestionID,
		QuestionVersionID: selected.QuestionVersionID,
	})
	if err != nil {
		return PracticeProbeReport{}, fmt.Errorf("load practice probe question: %w", err)
	}
	var expected any
	if err := json.Unmarshal(question.Answer, &expected); err != nil {
		return PracticeProbeReport{}, fmt.Errorf("decode stored practice answer: %w", err)
	}
	var options []any
	if (question.Type == "single" || question.Type == "multi") && json.Unmarshal(question.Options, &options) != nil {
		return PracticeProbeReport{}, errors.New("decode stored practice options")
	}
	if !scoreAnswer(question.Type, expected, expected, options) {
		return PracticeProbeReport{}, errors.New("practice scoring rejected the stored answer")
	}
	attemptID := uuid.New()
	responseBody := `{"probe":true}`
	if _, err := queries.CreatePracticeAttempt(ctx, store.CreatePracticeAttemptParams{
		ID:                attemptID,
		SessionID:         sessionID,
		BankID:            bankID,
		BankVersionID:     bankVersionID,
		QuestionID:        selected.QuestionID,
		QuestionVersionID: selected.QuestionVersionID,
		UserID:            uuid.NullUUID{},
		SubmittedAnswer:   question.Answer,
		Correct:           true,
		ExpectedAnswer:    question.Answer,
		Analysis:          question.Analysis,
		ResponseBody:      responseBody,
	}); err != nil {
		return PracticeProbeReport{}, fmt.Errorf("create practice probe attempt: %w", err)
	}
	if err := queries.UpdateQuestionStats(ctx, store.UpdateQuestionStatsParams{QuestionID: selected.QuestionID, CorrectCount: 1}); err != nil {
		return PracticeProbeReport{}, fmt.Errorf("update practice probe statistics: %w", err)
	}
	stored, err := queries.GetPracticeAttemptResponse(ctx, store.GetPracticeAttemptResponseParams{SessionID: sessionID, QuestionID: selected.QuestionID})
	if err != nil || stored != responseBody {
		return PracticeProbeReport{}, errors.New("read back practice probe attempt")
	}

	if err := tx.Rollback(ctx); err != nil {
		return PracticeProbeReport{}, fmt.Errorf("roll back practice probe: %w", err)
	}
	rolledBack = true
	var residue int64
	if err := database.QueryRow(ctx, `SELECT count(*) FROM quizcraft_practice_sessions WHERE id=$1`, sessionID).Scan(&residue); err != nil {
		return PracticeProbeReport{}, fmt.Errorf("check practice probe rollback: %w", err)
	}
	if residue != 0 {
		return PracticeProbeReport{}, errors.New("practice probe left persisted session data")
	}
	return PracticeProbeReport{BankID: bankID, QuestionID: selected.QuestionID}, nil
}
