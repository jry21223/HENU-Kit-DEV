package tests

import (
	"context"
	"testing"

	"github.com/google/uuid"
	quizcraft "henukit.dev/quizcraft"
)

func TestVerifyPracticeFlowRollsBackEveryBusinessFact(t *testing.T) {
	pool := practicePool(t)
	report := importPracticeBank(t, pool, "release-probe-"+uuid.NewString())

	var sessionsBefore, attemptsBefore, statsBefore int64
	if err := pool.QueryRow(context.Background(), `SELECT
		(SELECT count(*) FROM quizcraft_practice_sessions),
		(SELECT count(*) FROM quizcraft_practice_attempts),
		(SELECT COALESCE(sum(attempt_count),0) FROM quizcraft_question_stats)`).Scan(&sessionsBefore, &attemptsBefore, &statsBefore); err != nil {
		t.Fatal(err)
	}
	got, err := quizcraft.VerifyPracticeFlow(context.Background(), pool)
	if err != nil {
		t.Fatal(err)
	}
	if got.BankID == uuid.Nil || got.QuestionID == uuid.Nil {
		t.Fatalf("probe report = %+v", got)
	}
	// The probe may select any published bank in the shared test database; the
	// newly imported bank proves that at least one eligible target exists.
	if report.BankID == "" {
		t.Fatal("test bank was not imported")
	}

	var sessionsAfter, attemptsAfter, statsAfter int64
	if err := pool.QueryRow(context.Background(), `SELECT
		(SELECT count(*) FROM quizcraft_practice_sessions),
		(SELECT count(*) FROM quizcraft_practice_attempts),
		(SELECT COALESCE(sum(attempt_count),0) FROM quizcraft_question_stats)`).Scan(&sessionsAfter, &attemptsAfter, &statsAfter); err != nil {
		t.Fatal(err)
	}
	if sessionsAfter != sessionsBefore || attemptsAfter != attemptsBefore || statsAfter != statsBefore {
		t.Fatalf("probe persisted facts: before sessions/attempts/stats=%d/%d/%d after=%d/%d/%d", sessionsBefore, attemptsBefore, statsBefore, sessionsAfter, attemptsAfter, statsAfter)
	}
}
