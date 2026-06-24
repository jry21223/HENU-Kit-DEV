package tests

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"final-review-platform/services/api/internal/platform/model"
	"final-review-platform/services/api/internal/server"
	applogger "final-review-platform/services/api/pkg/logger"
)

func TestLeaderboardsExposePublicAggregatesOnly(t *testing.T) {
	db := newTestDB(t)
	router := server.NewRouter(testConfig(), applogger.New("test"), db, nil)
	course := createTestCourse(t, db)
	alice := createTestUser(t, db, "alice-board@stu.henu.edu.cn", model.RoleCreator)
	alice.Name = "Alice"
	alice.PointsBalance = 80
	if err := db.Save(&alice).Error; err != nil {
		t.Fatal(err)
	}
	bob := createTestUser(t, db, "bob-board@stu.henu.edu.cn", model.RoleUser)
	bob.Name = "Bob"
	bob.PointsBalance = 10
	if err := db.Save(&bob).Error; err != nil {
		t.Fatal(err)
	}
	frozen := createTestUser(t, db, "frozen-board@stu.henu.edu.cn", model.RoleCreator)
	frozen.Name = "Frozen"
	frozen.Status = "frozen"
	if err := db.Save(&frozen).Error; err != nil {
		t.Fatal(err)
	}

	createWikiEntry(t, db, alice.ID, course.ID, "Alice Wiki One", model.StatusPublished, "public", 3, 2, 1)
	createWikiEntry(t, db, alice.ID, course.ID, "Alice Wiki Two", model.StatusPublished, "public", 1, 0, 0)
	createWikiEntry(t, db, bob.ID, course.ID, "Bob Wiki", model.StatusPublished, "public", 0, 0, 0)
	createWikiEntry(t, db, bob.ID, course.ID, "Bob Pending Wiki", model.StatusPending, "public", 100, 100, 100)
	createWikiEntry(t, db, bob.ID, course.ID, "Bob Private Wiki", model.StatusPublished, "private", 100, 100, 100)
	createWikiEntry(t, db, frozen.ID, course.ID, "Frozen Wiki", model.StatusPublished, "public", 100, 100, 100)

	question := createTestQuestion(t, db, course.ID, "single_choice", "A")
	createQuizAnswer(t, db, alice.ID, question.ID, 1, true)
	createQuizAnswer(t, db, alice.ID, question.ID, 1, true)
	createQuizAnswer(t, db, bob.ID, question.ID, 0, false)
	createQuizAnswer(t, db, bob.ID, question.ID, 1, true)
	createQuizAnswer(t, db, frozen.ID, question.ID, 1, true)

	wiki := performJSON(router, http.MethodGet, "/api/v1/leaderboards/wiki?limit=5", "", "")
	if wiki.Code != http.StatusOK {
		t.Fatalf("expected wiki leaderboard 200, got %d: %s", wiki.Code, wiki.Body.String())
	}
	assertLeaderboardPublic(t, wiki.Body.String(), alice, bob, frozen)
	wikiData := decodeLeaderboard(t, wiki.Body.Bytes())
	if len(wikiData.Data.Entries) != 2 || wikiData.Data.Entries[0].UserID != alice.ID || wikiData.Data.Entries[0].Metrics.WikiCount != 2 {
		t.Fatalf("unexpected wiki leaderboard entries: %#v", wikiData.Data.Entries)
	}

	quiz := performJSON(router, http.MethodGet, "/api/v1/leaderboards/quiz?limit=5", "", "")
	if quiz.Code != http.StatusOK {
		t.Fatalf("expected quiz leaderboard 200, got %d: %s", quiz.Code, quiz.Body.String())
	}
	assertLeaderboardPublic(t, quiz.Body.String(), alice, bob, frozen)
	quizData := decodeLeaderboard(t, quiz.Body.Bytes())
	if len(quizData.Data.Entries) != 2 || quizData.Data.Entries[0].UserID != alice.ID || quizData.Data.Entries[0].Metrics.CorrectCount != 2 {
		t.Fatalf("unexpected quiz leaderboard entries: %#v", quizData.Data.Entries)
	}

	overall := performJSON(router, http.MethodGet, "/api/v1/leaderboards/overall?limit=5", "", "")
	if overall.Code != http.StatusOK {
		t.Fatalf("expected overall leaderboard 200, got %d: %s", overall.Code, overall.Body.String())
	}
	assertLeaderboardPublic(t, overall.Body.String(), alice, bob, frozen)
	overallData := decodeLeaderboard(t, overall.Body.Bytes())
	if len(overallData.Data.Entries) != 2 || overallData.Data.Entries[0].UserID != alice.ID || overallData.Data.Entries[0].Metrics.Points != 80 {
		t.Fatalf("unexpected overall leaderboard entries: %#v", overallData.Data.Entries)
	}
}

func TestLeaderboardLimitValidation(t *testing.T) {
	db := newTestDB(t)
	router := server.NewRouter(testConfig(), applogger.New("test"), db, nil)

	response := performJSON(router, http.MethodGet, "/api/v1/leaderboards/wiki?limit=1000", "", "")
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "invalid_limit") {
		t.Fatalf("expected invalid limit rejection, got %d: %s", response.Code, response.Body.String())
	}
}

func createWikiEntry(t *testing.T, db *gorm.DB, authorID string, courseID string, title string, status string, visibility string, likes int64, comments int64, collects int64) {
	t.Helper()
	entry := model.WikiEntry{
		ReviewFields: model.ReviewFields{Status: status},
		ContentStats: model.ContentStats{Visibility: visibility, LikeCount: likes, CommentCount: comments, CollectCount: collects},
		AuthorID:     authorID,
		CourseID:     &courseID,
		Title:        title,
		Slug:         strings.ToLower(strings.ReplaceAll(title, " ", "-")) + "-" + strconv.FormatInt(time.Now().UnixNano(), 10),
		Content:      title + " content",
	}
	if err := db.Create(&entry).Error; err != nil {
		t.Fatal(err)
	}
}

func createQuizAnswer(t *testing.T, db *gorm.DB, userID string, questionID string, score float64, correct bool) {
	t.Helper()
	answer := model.QuizAnswer{
		AttemptID:  "00000000-0000-0000-0000-000000000001",
		QuestionID: questionID,
		UserID:     userID,
		Answer:     "A",
		IsCorrect:  correct,
		Score:      score,
	}
	if err := db.Create(&answer).Error; err != nil {
		t.Fatal(err)
	}
}

func assertLeaderboardPublic(t *testing.T, body string, includedA model.User, includedB model.User, excluded model.User) {
	t.Helper()
	if !strings.Contains(body, includedA.ID) || !strings.Contains(body, includedB.ID) {
		t.Fatalf("expected active ranked users in leaderboard, got %s", body)
	}
	for _, forbidden := range []string{includedA.Email, includedB.Email, excluded.ID, excluded.Email, "@stu.henu.edu.cn", "Pending Wiki", "Private Wiki", "Frozen Wiki"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("leaderboard leaked forbidden value %q: %s", forbidden, body)
		}
	}
}

type leaderboardPayload struct {
	Data struct {
		Type    string `json:"type"`
		Entries []struct {
			Rank    int    `json:"rank"`
			UserID  string `json:"userId"`
			Name    string `json:"name"`
			Role    string `json:"role"`
			Score   int64  `json:"score"`
			Metrics struct {
				WikiCount    int64 `json:"wikiCount"`
				Engagement   int64 `json:"engagement"`
				AnswerCount  int64 `json:"answerCount"`
				CorrectCount int64 `json:"correctCount"`
				Points       int64 `json:"points"`
			} `json:"metrics"`
		} `json:"entries"`
	} `json:"data"`
}

func decodeLeaderboard(t *testing.T, body []byte) leaderboardPayload {
	t.Helper()
	var payload leaderboardPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	return payload
}
