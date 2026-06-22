package tests

import (
	"net/http"
	"strings"
	"testing"

	"gorm.io/gorm"

	"final-review-platform/services/api/internal/platform/model"
	"final-review-platform/services/api/internal/server"
	applogger "final-review-platform/services/api/pkg/logger"
)

func TestQuestionListDoesNotExposeAnswers(t *testing.T) {
	db := newTestDB(t)
	router := server.NewRouter(testConfig(), applogger.New("test"), db, nil)
	course := createTestCourse(t, db)
	question := createTestQuestion(t, db, course.ID, "single_choice", "A")
	createTestOptions(t, db, question.ID)

	listResponse := performJSON(router, http.MethodGet, "/api/v1/courses/"+course.ID+"/questions", "", "")
	if listResponse.Code != http.StatusOK {
		t.Fatalf("expected question list 200, got %d: %s", listResponse.Code, listResponse.Body.String())
	}
	if strings.Contains(listResponse.Body.String(), `"answer"`) {
		t.Fatal("question list exposed answer field")
	}

	detailResponse := performJSON(router, http.MethodGet, "/api/v1/questions/"+question.ID, "", "")
	if detailResponse.Code != http.StatusOK {
		t.Fatalf("expected question detail 200, got %d: %s", detailResponse.Code, detailResponse.Body.String())
	}
	if strings.Contains(detailResponse.Body.String(), `"answer"`) {
		t.Fatal("question detail exposed answer field")
	}
}

func TestQuestionSubmitAndWrongQuestionIsolation(t *testing.T) {
	db := newTestDB(t)
	router := server.NewRouter(testConfig(), applogger.New("test"), db, nil)
	course := createTestCourse(t, db)
	question := createTestQuestion(t, db, course.ID, "single_choice", "A")
	createTestOptions(t, db, question.ID)

	correctResponse := performJSON(router, http.MethodPost, "/api/v1/questions/"+question.ID+"/submit", `{"answer":"A"}`, "")
	if correctResponse.Code != http.StatusOK {
		t.Fatalf("expected correct submit 200, got %d: %s", correctResponse.Code, correctResponse.Body.String())
	}
	if !strings.Contains(correctResponse.Body.String(), `"isCorrect":true`) {
		t.Fatalf("expected correct response, got %s", correctResponse.Body.String())
	}

	anonymousWrong := performJSON(router, http.MethodPost, "/api/v1/questions/"+question.ID+"/submit", `{"answer":"B"}`, "")
	if anonymousWrong.Code != http.StatusOK {
		t.Fatalf("expected anonymous wrong submit 200, got %d: %s", anonymousWrong.Code, anonymousWrong.Body.String())
	}
	var wrongCount int64
	if err := db.Model(&model.WrongQuestion{}).Count(&wrongCount).Error; err != nil {
		t.Fatal(err)
	}
	if wrongCount != 0 {
		t.Fatalf("expected anonymous wrong answer not to persist, got %d records", wrongCount)
	}

	studentToken := loginTestUser(t, router, "student@stu.henu.edu.cn")
	wrongResponse := performJSON(router, http.MethodPost, "/api/v1/questions/"+question.ID+"/submit", `{"answer":"B"}`, studentToken)
	if wrongResponse.Code != http.StatusOK {
		t.Fatalf("expected authenticated wrong submit 200, got %d: %s", wrongResponse.Code, wrongResponse.Body.String())
	}

	studentWrong := performJSON(router, http.MethodGet, "/api/v1/me/wrong-questions", "", studentToken)
	if studentWrong.Code != http.StatusOK {
		t.Fatalf("expected wrong question list 200, got %d: %s", studentWrong.Code, studentWrong.Body.String())
	}
	if !strings.Contains(studentWrong.Body.String(), question.ID) {
		t.Fatalf("expected student's wrong question in list, got %s", studentWrong.Body.String())
	}

	otherToken := loginTestUser(t, router, "other@stu.henu.edu.cn")
	otherWrong := performJSON(router, http.MethodGet, "/api/v1/me/wrong-questions", "", otherToken)
	if otherWrong.Code != http.StatusOK {
		t.Fatalf("expected other wrong question list 200, got %d: %s", otherWrong.Code, otherWrong.Body.String())
	}
	if strings.Contains(otherWrong.Body.String(), question.ID) {
		t.Fatalf("other user saw student's wrong question: %s", otherWrong.Body.String())
	}
}

func TestJudgeMultipleChoiceAndFillBlank(t *testing.T) {
	db := newTestDB(t)
	router := server.NewRouter(testConfig(), applogger.New("test"), db, nil)
	course := createTestCourse(t, db)
	multiple := createTestQuestion(t, db, course.ID, "multiple_choice", "A,C")
	fillBlank := createTestQuestion(t, db, course.ID, "fill_blank", "Go Lang")

	multipleResponse := performJSON(router, http.MethodPost, "/api/v1/questions/"+multiple.ID+"/submit", `{"answer":"C A"}`, "")
	if multipleResponse.Code != http.StatusOK || !strings.Contains(multipleResponse.Body.String(), `"isCorrect":true`) {
		t.Fatalf("expected multiple choice normalized correct, got %d: %s", multipleResponse.Code, multipleResponse.Body.String())
	}

	fillResponse := performJSON(router, http.MethodPost, "/api/v1/questions/"+fillBlank.ID+"/submit", `{"answer":"golang"}`, "")
	if fillResponse.Code != http.StatusOK || !strings.Contains(fillResponse.Body.String(), `"isCorrect":true`) {
		t.Fatalf("expected fill blank normalized correct, got %d: %s", fillResponse.Code, fillResponse.Body.String())
	}
}

func TestQuizAttemptsRequireAuthAndAreUserScoped(t *testing.T) {
	db := newTestDB(t)
	router := server.NewRouter(testConfig(), applogger.New("test"), db, nil)
	course := createTestCourse(t, db)

	unauthorized := performJSON(router, http.MethodPost, "/api/v1/quiz/attempts", `{"courseId":"`+course.ID+`"}`, "")
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthenticated attempt create 401, got %d: %s", unauthorized.Code, unauthorized.Body.String())
	}

	studentToken := loginTestUser(t, router, "student@stu.henu.edu.cn")
	createResponse := performJSON(router, http.MethodPost, "/api/v1/quiz/attempts", `{"courseId":"`+course.ID+`","mode":"practice"}`, studentToken)
	if createResponse.Code != http.StatusOK {
		t.Fatalf("expected attempt create 200, got %d: %s", createResponse.Code, createResponse.Body.String())
	}
	if !strings.Contains(createResponse.Body.String(), `"courseId":"`+course.ID+`"`) {
		t.Fatalf("expected created attempt course id, got %s", createResponse.Body.String())
	}

	listResponse := performJSON(router, http.MethodGet, "/api/v1/me/quiz-attempts?courseId="+course.ID, "", studentToken)
	if listResponse.Code != http.StatusOK {
		t.Fatalf("expected attempt list 200, got %d: %s", listResponse.Code, listResponse.Body.String())
	}
	if !strings.Contains(listResponse.Body.String(), course.ID) {
		t.Fatalf("expected attempt in own list, got %s", listResponse.Body.String())
	}

	otherToken := loginTestUser(t, router, "other@stu.henu.edu.cn")
	otherList := performJSON(router, http.MethodGet, "/api/v1/me/quiz-attempts", "", otherToken)
	if otherList.Code != http.StatusOK {
		t.Fatalf("expected other attempt list 200, got %d: %s", otherList.Code, otherList.Body.String())
	}
	if strings.Contains(otherList.Body.String(), course.ID) {
		t.Fatalf("other user saw student's attempt: %s", otherList.Body.String())
	}
}

func createTestQuestion(t *testing.T, db *gorm.DB, courseID string, questionType string, answer string) model.QuizQuestion {
	t.Helper()
	question := model.QuizQuestion{
		CourseID:    courseID,
		Type:        questionType,
		Stem:        "Test stem " + questionType + " " + answer,
		Answer:      answer,
		Explanation: "Explanation",
		Difficulty:  1,
		Status:      model.StatusPublished,
	}
	if err := db.Create(&question).Error; err != nil {
		t.Fatal(err)
	}
	return question
}

func createTestOptions(t *testing.T, db *gorm.DB, questionID string) {
	t.Helper()
	options := []model.QuizOption{
		{QuestionID: questionID, Label: "A", Content: "Option A", SortOrder: 1},
		{QuestionID: questionID, Label: "B", Content: "Option B", SortOrder: 2},
		{QuestionID: questionID, Label: "C", Content: "Option C", SortOrder: 3},
	}
	for index := range options {
		if err := db.Create(&options[index]).Error; err != nil {
			t.Fatal(err)
		}
	}
}
