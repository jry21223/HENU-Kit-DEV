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
	var bankID, bankVersionID, sessionID uuid.UUID
	var selected store.ListPracticeQuestionsRow
	var question store.GetSessionQuestionRow
	var submittedAnswer []byte
	foundScorable := false
	for _, bank := range banks {
		if !bank.ActiveVersionID.Valid || bank.QuestionCount <= 0 {
			continue
		}
		candidateBankID := bank.ID
		candidateBankVersionID := bank.ActiveVersionID.UUID
		if _, err := queries.IsPublishedBankVersion(ctx, store.IsPublishedBankVersionParams{ID: candidateBankID, ID_2: candidateBankVersionID}); err != nil {
			return PracticeProbeReport{}, fmt.Errorf("verify published bank version: %w", err)
		}

		candidateSessionID := uuid.New()
		selectionSeed := uuid.NewSHA1(uuid.NameSpaceURL, []byte("henukit-practice-probe:"+candidateBankVersionID.String()))
		questionLimit := bank.QuestionCount
		if questionLimit > 100 {
			questionLimit = 100
		}
		questions, err := queries.ListPracticeQuestions(ctx, store.ListPracticeQuestionsParams{
			BankVersionID: candidateBankVersionID,
			SessionID:     selectionSeed,
			QuestionCount: int32(questionLimit),
		})
		if err != nil {
			return PracticeProbeReport{}, fmt.Errorf("select practice question: %w", err)
		}
		if len(questions) == 0 {
			continue
		}
		if err := queries.CreatePracticeSession(ctx, store.CreatePracticeSessionParams{
			ID:            candidateSessionID,
			BankID:        candidateBankID,
			BankVersionID: candidateBankVersionID,
			UserID:        uuid.NullUUID{},
			ActorKey:      "guest:" + candidateSessionID.String(),
			Mode:          "random",
			ChapterID:     pgtype.Text{},
		}); err != nil {
			return PracticeProbeReport{}, fmt.Errorf("create practice probe session: %w", err)
		}
		for position, candidate := range questions {
			if err := queries.AddPracticeSessionQuestion(ctx, store.AddPracticeSessionQuestionParams{
				SessionID:         candidateSessionID,
				BankID:            candidateBankID,
				BankVersionID:     candidateBankVersionID,
				QuestionID:        candidate.QuestionID,
				QuestionVersionID: candidate.QuestionVersionID,
				Position:          int32(position + 1),
			}); err != nil {
				return PracticeProbeReport{}, fmt.Errorf("attach practice probe question: %w", err)
			}
			loaded, err := queries.GetSessionQuestion(ctx, store.GetSessionQuestionParams{
				ID:                candidateSessionID,
				QuestionID:        candidate.QuestionID,
				QuestionVersionID: candidate.QuestionVersionID,
			})
			if err != nil {
				return PracticeProbeReport{}, fmt.Errorf("load practice probe question: %w", err)
			}
			var expected any
			if err := json.Unmarshal(loaded.Answer, &expected); err != nil {
				return PracticeProbeReport{}, fmt.Errorf("decode stored practice answer: %w", err)
			}
			submitted, ok := practiceProbeSubmission(loaded.Type, expected)
			if !ok {
				continue
			}
			var options []any
			if (loaded.Type == "single" || loaded.Type == "multi") && json.Unmarshal(loaded.Options, &options) != nil {
				continue
			}
			if !scoreAnswer(loaded.Type, submitted, expected, options) {
				continue
			}
			encodedSubmitted, err := json.Marshal(submitted)
			if err != nil {
				return PracticeProbeReport{}, fmt.Errorf("encode practice probe submission: %w", err)
			}
			bankID, bankVersionID, sessionID = candidateBankID, candidateBankVersionID, candidateSessionID
			selected, question = candidate, loaded
			submittedAnswer = encodedSubmitted
			foundScorable = true
			break
		}
		if foundScorable {
			break
		}
	}
	if !foundScorable {
		return PracticeProbeReport{}, errors.New("no published QuizCraft bank has a scorable practice question")
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
		SubmittedAnswer:   submittedAnswer,
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

func practiceProbeSubmission(kind string, expected any) (any, bool) {
	if kind == "multi" {
		answers, ok := expected.([]any)
		return expected, ok && len(answers) > 0
	}
	if kind != "blank" {
		return expected, expected != nil
	}
	if normalizeBlank(expected) != "" {
		return expected, true
	}
	candidates, ok := expected.([]any)
	if !ok {
		return nil, false
	}
	for _, candidate := range candidates {
		if normalizeBlank(candidate) != "" {
			return candidate, true
		}
	}
	return nil, false
}
