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
	mixedBankID := uuid.MustParse(report.BankID)
	invalidReport := importPracticeBank(t, pool, "release-probe-invalid-"+uuid.NewString())
	invalidBankID := uuid.MustParse(invalidReport.BankID)

	type storedBankName struct {
		id   uuid.UUID
		name string
	}
	rows, err := pool.Query(context.Background(), `SELECT id,name FROM quizcraft_banks`)
	if err != nil {
		t.Fatal(err)
	}
	var storedNames []storedBankName
	for rows.Next() {
		var bank storedBankName
		if err := rows.Scan(&bank.id, &bank.name); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		storedNames = append(storedNames, bank)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		t.Fatal(err)
	}
	rows.Close()
	t.Cleanup(func() {
		for _, bank := range storedNames {
			_, _ = pool.Exec(context.Background(), `UPDATE quizcraft_banks SET name=$1 WHERE id=$2`, bank.name, bank.id)
		}
		_, _ = pool.Exec(context.Background(), `UPDATE quizcraft_banks SET active_version_id=NULL WHERE id=$1 OR id=$2`, invalidBankID, mixedBankID)
	})
	for _, bank := range storedNames {
		name := "zzzz-release-probe-" + bank.id.String()
		if bank.id == invalidBankID {
			name = "0-release-probe-invalid"
		} else if bank.id == mixedBankID {
			name = "1-release-probe-mixed"
		}
		if _, err := pool.Exec(context.Background(), `UPDATE quizcraft_banks SET name=$1 WHERE id=$2`, name, bank.id); err != nil {
			t.Fatal(err)
		}
	}
	questionVersionsTriggerEnabled := false
	t.Cleanup(func() {
		if !questionVersionsTriggerEnabled {
			_, _ = pool.Exec(context.Background(), `ALTER TABLE quizcraft_question_versions ENABLE TRIGGER quizcraft_question_versions_immutable`)
		}
	})
	if _, err := pool.Exec(context.Background(), `ALTER TABLE quizcraft_question_versions DISABLE TRIGGER quizcraft_question_versions_immutable`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), `UPDATE quizcraft_question_versions SET answer='[]'::jsonb WHERE bank_id=$1`, invalidBankID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), `UPDATE quizcraft_question_versions SET answer='null'::jsonb WHERE bank_id=$1`, mixedBankID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), `UPDATE quizcraft_question_versions SET answer='["main","package main"]'::jsonb WHERE id=$1`, report.Questions[3].QuestionVersionID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), `ALTER TABLE quizcraft_question_versions ENABLE TRIGGER quizcraft_question_versions_immutable`); err != nil {
		t.Fatal(err)
	}
	questionVersionsTriggerEnabled = true
	var firstPublishedBankID uuid.UUID
	if err := pool.QueryRow(context.Background(), `SELECT b.id
		FROM quizcraft_banks b
		JOIN quizcraft_bank_versions bv ON bv.id=b.active_version_id AND bv.bank_id=b.id AND bv.sealed_at IS NOT NULL
		ORDER BY b.name,b.id LIMIT 1`).Scan(&firstPublishedBankID); err != nil {
		t.Fatal(err)
	}
	if firstPublishedBankID != invalidBankID {
		t.Fatalf("invalid bank is not the first probe candidate: got %s want %s", firstPublishedBankID, invalidBankID)
	}

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
	if got.BankID != mixedBankID {
		t.Fatalf("probe bank = %s, want mixed bank %s after skipping unscorable bank %s", got.BankID, mixedBankID, invalidBankID)
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
