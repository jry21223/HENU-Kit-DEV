package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	quizcraft "henukit.dev/quizcraft"
)

func TestDirectBootstrapActivationIsDisabledByDefault(t *testing.T) {
	pool := practicePool(t)
	service, err := quizcraft.New(quizcraft.Config{Database: pool})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ImportJSON(context.Background(), "blocked-bootstrap", []byte(validBank)); err == nil || !strings.Contains(err.Error(), "Workshop") {
		t.Fatalf("direct activation was not blocked: %v", err)
	}
}

const validBank = `{"meta":{"name":"程序设计","version":"legacy-v1","color":"#2563eb","total":4,"source_files":["legacy.md"],"chapters":[{"id":"ch01","name":"基础"},{"id":"ch02","name":"进阶"}]},"questions":[{"id":"q0001","number":"1","type":"single","chapter_id":"ch01","chapter":"基础","content":"1+1=?","options":["1","2"],"answer":1,"analysis":"","stats":{"total":0,"correct":0}},{"id":"q0002","type":"multi","chapter_id":"ch02","chapter":"进阶","content":"选择偶数","options":["1","2","4"],"answer":[1,2],"analysis":""},{"id":"q0003","type":"judge","chapter_id":"ch01","chapter":"基础","content":"Go 是编译型语言","answer":true,"analysis":""},{"id":"q0004","type":"blank","chapter_id":"ch01","chapter":"基础","content":"Go 入口包是____","answer":"main","analysis":""}]}`

func TestExplicitImportIsStableVersionedAndReported(t *testing.T) {
	pool, err := pgxpool.New(context.Background(), os.Getenv("QUIZCRAFT_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	service, err := quizcraft.New(quizcraft.Config{Database: pool, AllowTestBootstrapActivation: true})
	if err != nil {
		t.Fatal(err)
	}
	first, err := service.ImportJSON(context.Background(), "programming-basics", []byte(validBank))
	if err != nil {
		t.Fatal(err)
	}
	if !first.Accepted || first.QuestionCount != 4 || first.AnsweredCount != 4 || first.TypeCounts["single"] != 1 || first.TypeCounts["multi"] != 1 || first.TypeCounts["judge"] != 1 || first.TypeCounts["blank"] != 1 || first.ChapterCounts["ch01"] != 3 || first.ChapterCounts["ch02"] != 1 {
		t.Fatalf("report = %+v", first)
	}
	if len(first.SourceSHA256) != 64 || len(first.ContentSHA256) != 64 || len(first.Questions) != 4 {
		t.Fatalf("hash/ids missing: %+v", first)
	}
	if len(first.Questions[0].AnswerSHA256) != 64 {
		t.Fatalf("answer hash missing: %+v", first.Questions[0])
	}
	second, err := service.ImportJSON(context.Background(), "programming-basics", []byte(validBank))
	if err != nil {
		t.Fatal(err)
	}
	if first.BankID != second.BankID || first.BankVersionID != second.BankVersionID || first.Questions[0] != second.Questions[0] {
		t.Fatalf("repeat changed stable ids: %+v / %+v", first, second)
	}
	whitespace, err := service.ImportJSON(context.Background(), "programming-basics", []byte(validBank+"\n"))
	if err != nil || whitespace.BankVersionID != first.BankVersionID || whitespace.SourceSHA256 == first.SourceSHA256 {
		t.Fatalf("semantic reimport identity = %+v / %v", whitespace, err)
	}
	var storedSourceSHA string
	if err := pool.QueryRow(context.Background(), `SELECT source_sha256 FROM quizcraft_bank_versions WHERE id=$1`, first.BankVersionID).Scan(&storedSourceSHA); err != nil {
		t.Fatal(err)
	}
	if storedSourceSHA != first.SourceSHA256 {
		t.Fatalf("immutable provenance changed from %s to %s", first.SourceSHA256, storedSourceSHA)
	}
	var banks, questions, versions int
	if err := pool.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM quizcraft_banks WHERE id=$1),(SELECT count(*) FROM quizcraft_questions WHERE bank_id=$1),(SELECT count(*) FROM quizcraft_question_versions WHERE bank_id=$1)`, first.BankID).Scan(&banks, &questions, &versions); err != nil {
		t.Fatal(err)
	}
	if banks != 1 || questions != 4 || versions != 4 {
		t.Fatalf("stored counts = %d/%d/%d", banks, questions, versions)
	}

	var edited map[string]any
	if err := json.Unmarshal([]byte(validBank), &edited); err != nil {
		t.Fatal(err)
	}
	items := edited["questions"].([]any)
	items[0].(map[string]any)["content"] = "2+2=?"
	changed, _ := json.Marshal(edited)
	third, err := service.ImportJSON(context.Background(), "programming-basics", changed)
	if err != nil {
		t.Fatal(err)
	}
	if third.BankID != first.BankID || third.BankVersionID == first.BankVersionID || third.Questions[0].QuestionID != first.Questions[0].QuestionID || third.Questions[0].QuestionVersionID == first.Questions[0].QuestionVersionID {
		t.Fatalf("versioning unstable: %+v / %+v", first.Questions[0], third.Questions[0])
	}
	var memberships int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM quizcraft_bank_version_questions WHERE bank_version_id=$1`, third.BankVersionID).Scan(&memberships); err != nil {
		t.Fatal(err)
	}
	if memberships != 4 {
		t.Fatalf("changed bank version memberships = %d, want 4", memberships)
	}

	items[0].(map[string]any)["content"] = "1+1=?"
	items[0].(map[string]any)["answer"] = float64(0)
	answerChanged, _ := json.Marshal(edited)
	fourth, err := service.ImportJSON(context.Background(), "programming-basics", answerChanged)
	if err != nil {
		t.Fatal(err)
	}
	if fourth.Questions[0].QuestionID != first.Questions[0].QuestionID || fourth.Questions[0].AnswerSHA256 == first.Questions[0].AnswerSHA256 || fourth.Questions[0].QuestionVersionID == first.Questions[0].QuestionVersionID {
		t.Fatalf("answer-only reconciliation failed: %+v / %+v", first.Questions[0], fourth.Questions[0])
	}
}

func TestInvalidImportReportsAnswersTypesChaptersAndDoesNotWrite(t *testing.T) {
	pool, err := pgxpool.New(context.Background(), os.Getenv("QUIZCRAFT_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	service, _ := quizcraft.New(quizcraft.Config{Database: pool, AllowTestBootstrapActivation: true})
	invalid := []byte(`{"meta":{"name":"坏题库"},"questions":[{"id":"dup","type":"single","content":"缺答案","chapter":""},{"id":"dup","type":"essay","content":"非法类型","chapter":"第一章","answer":"x"}]}`)
	report, err := service.ImportJSON(context.Background(), "invalid-bank", invalid)
	if err == nil || report.Accepted || report.UnansweredCount != 1 || len(report.Errors) < 3 {
		t.Fatalf("invalid report/error = %+v / %v", report, err)
	}
	var count int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM quizcraft_banks WHERE bank_key='invalid-bank'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("invalid import wrote bank: %d %v", count, err)
	}
	trailing, err := service.ImportJSON(context.Background(), "invalid-trailing", []byte(validBank+` {}`))
	if err == nil || trailing.Accepted || len(trailing.Errors) != 1 || trailing.Errors[0].Code != "trailing_json" {
		t.Fatalf("trailing JSON report/error = %+v / %v", trailing, err)
	}
	badShape := []byte(`{"meta":{"name":"Bad shape"},"questions":[{"id":"q1","type":"single","chapter":"c","content":"x","options":["a","b"],"answer":1.5},{"id":"q2","type":"multi","chapter":"c","content":"x","options":["a","b"],"answer":[2]},{"id":"q3","type":"blank","chapter":"c","content":"x","answer":[""]}]}`)
	shapeReport, err := service.ImportJSON(context.Background(), "invalid-shape", badShape)
	if err == nil || shapeReport.Accepted || len(shapeReport.Errors) != 3 {
		t.Fatalf("invalid shape report/error = %+v / %v", shapeReport, err)
	}
	longName := strings.Repeat("x", 161)
	tooLong := []byte(fmt.Sprintf(`{"meta":{"name":%q},"questions":[{"id":"q1","type":"blank","chapter":"c","content":"x","answer":"ok"}]}`, longName))
	lengthReport, err := service.ImportJSON(context.Background(), "invalid-length", tooLong)
	if err == nil || lengthReport.Accepted || len(lengthReport.Errors) != 1 || lengthReport.Errors[0].Code != "too_long" {
		t.Fatalf("invalid length report/error = %+v / %v", lengthReport, err)
	}
}

func TestDatabaseRejectsCrossOwnerVersionMembership(t *testing.T) {
	pool, err := pgxpool.New(context.Background(), os.Getenv("QUIZCRAFT_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	service, _ := quizcraft.New(quizcraft.Config{Database: pool, AllowTestBootstrapActivation: true})
	left, err := service.ImportJSON(context.Background(), "owner-left", []byte(validBank))
	if err != nil {
		t.Fatal(err)
	}
	right, err := service.ImportJSON(context.Background(), "owner-right", []byte(validBank))
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(context.Background(), `INSERT INTO quizcraft_bank_version_questions(bank_id,bank_version_id,question_id,question_version_id,position) VALUES($1,$2,$3,$4,99)`, left.BankID, left.BankVersionID, right.Questions[0].QuestionID, right.Questions[0].QuestionVersionID)
	if err == nil {
		t.Fatal("cross-bank membership unexpectedly accepted")
	}
	_, err = pool.Exec(context.Background(), `UPDATE quizcraft_banks SET active_version_id=$1 WHERE id=$2`, right.BankVersionID, left.BankID)
	if err == nil {
		t.Fatal("cross-bank active version unexpectedly accepted")
	}
}

func TestDatabaseRejectsImmutableContentMutation(t *testing.T) {
	pool, err := pgxpool.New(context.Background(), os.Getenv("QUIZCRAFT_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	service, _ := quizcraft.New(quizcraft.Config{Database: pool, AllowTestBootstrapActivation: true})
	report, err := service.ImportJSON(context.Background(), "immutable-bank", []byte(validBank))
	if err != nil {
		t.Fatal(err)
	}
	checks := []struct {
		name string
		sql  string
		args []any
	}{
		{name: "bank version update", sql: `UPDATE quizcraft_bank_versions SET source_version='changed' WHERE id=$1`, args: []any{report.BankVersionID}},
		{name: "question version delete", sql: `DELETE FROM quizcraft_question_versions WHERE id=$1`, args: []any{report.Questions[0].QuestionVersionID}},
		{name: "membership update", sql: `UPDATE quizcraft_bank_version_questions SET position=99 WHERE bank_version_id=$1 AND question_id=$2`, args: []any{report.BankVersionID, report.Questions[0].QuestionID}},
		{name: "version truncate", sql: `TRUNCATE quizcraft_question_versions CASCADE`},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			if _, err := pool.Exec(context.Background(), check.sql, check.args...); err == nil {
				t.Fatalf("immutable mutation unexpectedly succeeded: %s", check.sql)
			}
		})
	}
	newQuestionID := uuid.New()
	newVersionID := uuid.New()
	if _, err := pool.Exec(context.Background(), `INSERT INTO quizcraft_questions(id,bank_id,source_question_id) VALUES($1,$2,$3)`, newQuestionID, report.BankID, "post-seal-question"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), `INSERT INTO quizcraft_question_versions(id,bank_id,question_id,type,chapter_id,chapter_name,content,answer,content_sha256) VALUES($1,$2,$3,'blank','ch','chapter','new question','"answer"',$4)`, newVersionID, report.BankID, newQuestionID, strings.Repeat("a", 64)); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), `INSERT INTO quizcraft_bank_version_questions(bank_id,bank_version_id,question_id,question_version_id,position) VALUES($1,$2,$3,$4,99)`, report.BankID, report.BankVersionID, newQuestionID, newVersionID); err == nil {
		t.Fatal("sealed bank version unexpectedly accepted a new membership")
	}
}

func TestExistingAnsweredBankShapeImportsExplicitly(t *testing.T) {
	pool, err := pgxpool.New(context.Background(), os.Getenv("QUIZCRAFT_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	service, err := quizcraft.New(quizcraft.Config{Database: pool, AllowTestBootstrapActivation: true})
	if err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile("../../generated/computer_fundamentals.json")
	if err != nil {
		t.Fatal(err)
	}
	report, err := service.ImportJSON(context.Background(), "computer-fundamentals", source)
	if err != nil {
		t.Fatalf("existing bank rejected: %v (%+v)", err, report.Errors)
	}
	if !report.Accepted || report.QuestionCount != 556 || report.AnsweredCount != 556 || report.TypeCounts["blank"] != 289 || report.TypeCounts["judge"] != 267 {
		t.Fatalf("existing bank report = %+v", report)
	}
}
