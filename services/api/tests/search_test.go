package tests

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"final-review-platform/services/api/internal/platform/model"
	"final-review-platform/services/api/internal/server"
	applogger "final-review-platform/services/api/pkg/logger"
)

func TestSearchReturnsOnlyPublicPublishedContent(t *testing.T) {
	db := newTestDB(t)
	router := server.NewRouter(testConfig(), applogger.New("test"), db, nil)
	course := createTestCourse(t, db)
	course.Name = "Graph Theory Review"
	course.Description = "Graph review course with trees and paths"
	if err := db.Save(&course).Error; err != nil {
		t.Fatal(err)
	}
	hiddenCourse := model.Course{
		SchoolID:    course.SchoolID,
		CollegeID:   course.CollegeID,
		MajorID:     course.MajorID,
		Grade:       "2023",
		Name:        "Hidden Graph Course",
		Slug:        "hidden-graph-course",
		Description: "This archived graph course must not appear",
		Status:      model.StatusArchived,
	}
	if err := db.Create(&hiddenCourse).Error; err != nil {
		t.Fatal(err)
	}

	material := createTestMaterial(t, db, course.ID, "Graph Mock Paper", model.MaterialAccessPaid, "materials/graph/mock.pdf")
	hiddenMaterial := createTestMaterial(t, db, course.ID, "Hidden Graph Answers", model.MaterialAccessPaid, "materials/graph/hidden.pdf")
	if err := db.Model(&hiddenMaterial).Update("status", model.StatusDraft).Error; err != nil {
		t.Fatal(err)
	}

	pkg := model.CoursePackage{
		SchoolID:    course.SchoolID,
		CollegeID:   course.CollegeID,
		MajorID:     course.MajorID,
		CourseID:    &course.ID,
		Grade:       course.Grade,
		Title:       "Graph Final Package",
		Slug:        "graph-final-package",
		Description: "Graph theory paid review package",
		PriceFen:    1990,
		Currency:    "CNY",
		Status:      model.StatusPublished,
	}
	if err := db.Create(&pkg).Error; err != nil {
		t.Fatal(err)
	}

	wikiEntry := model.WikiEntry{
		ReviewFields: model.ReviewFields{Status: model.StatusPublished},
		ContentStats: model.ContentStats{Visibility: "public"},
		AuthorID:     createTestUser(t, db, "wiki-author@stu.henu.edu.cn", model.RoleCreator).ID,
		CourseID:     &course.ID,
		Title:        "Graph Trees Wiki",
		Slug:         "graph-trees-wiki",
		Content:      "Graph trees and connectivity notes",
	}
	if err := db.Create(&wikiEntry).Error; err != nil {
		t.Fatal(err)
	}
	hiddenWiki := wikiEntry
	hiddenWiki.BaseModel = model.BaseModel{}
	hiddenWiki.Title = "Hidden Graph Wiki"
	hiddenWiki.Slug = "hidden-graph-wiki"
	hiddenWiki.Status = model.StatusPending
	if err := db.Create(&hiddenWiki).Error; err != nil {
		t.Fatal(err)
	}

	blogPost := model.BlogPost{
		ReviewFields: model.ReviewFields{Status: model.StatusPublished},
		ContentStats: model.ContentStats{Visibility: "public"},
		AuthorID:     createTestUser(t, db, "blog-author@stu.henu.edu.cn", model.RoleUser).ID,
		Title:        "Graph Review Blog",
		Slug:         "graph-review-blog",
		Content:      "How I reviewed graph proof questions",
	}
	if err := db.Create(&blogPost).Error; err != nil {
		t.Fatal(err)
	}
	privateBlog := blogPost
	privateBlog.BaseModel = model.BaseModel{}
	privateBlog.Title = "Private Graph Blog"
	privateBlog.Slug = "private-graph-blog"
	privateBlog.Visibility = "private"
	if err := db.Create(&privateBlog).Error; err != nil {
		t.Fatal(err)
	}

	board := model.ForumBoard{Name: "Graph Help", Slug: "graph-help", Status: model.StatusPublished}
	if err := db.Create(&board).Error; err != nil {
		t.Fatal(err)
	}
	forumPost := model.ForumPost{
		ContentStats: model.ContentStats{Visibility: "public"},
		AuthorID:     createTestUser(t, db, "forum-search@stu.henu.edu.cn", model.RoleUser).ID,
		BoardID:      board.ID,
		Title:        "Graph proof discussion",
		Content:      "How to prove a graph is connected",
		Type:         "question",
		Status:       model.StatusPublished,
	}
	if err := db.Create(&forumPost).Error; err != nil {
		t.Fatal(err)
	}
	pendingForum := forumPost
	pendingForum.BaseModel = model.BaseModel{}
	pendingForum.Title = "Pending Graph Forum"
	pendingForum.Status = model.StatusPending
	if err := db.Create(&pendingForum).Error; err != nil {
		t.Fatal(err)
	}

	response := performJSON(router, http.MethodGet, "/api/v1/search?q=graph&limit=5", "", "")
	if response.Code != http.StatusOK {
		t.Fatalf("expected search 200, got %d: %s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	for _, expected := range []string{course.ID, material.ID, pkg.ID, wikiEntry.ID, blogPost.ID, forumPost.ID} {
		if !strings.Contains(body, expected) {
			t.Fatalf("expected search response to include %s, got %s", expected, body)
		}
	}
	for _, forbidden := range []string{hiddenCourse.ID, hiddenMaterial.ID, hiddenWiki.ID, privateBlog.ID, pendingForum.ID, "storageKey", "materials/graph/mock.pdf"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("search response leaked hidden/internal value %q: %s", forbidden, body)
		}
	}

	var payload struct {
		Data struct {
			Total   int `json:"total"`
			Results struct {
				Courses   []searchResult `json:"courses"`
				Materials []searchResult `json:"materials"`
				Packages  []searchResult `json:"packages"`
				Wiki      []searchResult `json:"wiki"`
				Blog      []searchResult `json:"blog"`
				Forum     []searchResult `json:"forum"`
			} `json:"results"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Data.Total != 6 {
		t.Fatalf("expected 6 grouped search results, got %d", payload.Data.Total)
	}
	if len(payload.Data.Results.Courses) != 1 || len(payload.Data.Results.Materials) != 1 || len(payload.Data.Results.Packages) != 1 || len(payload.Data.Results.Wiki) != 1 || len(payload.Data.Results.Blog) != 1 || len(payload.Data.Results.Forum) != 1 {
		t.Fatalf("unexpected grouped result counts: %#v", payload.Data.Results)
	}
}

func TestSearchValidation(t *testing.T) {
	db := newTestDB(t)
	router := server.NewRouter(testConfig(), applogger.New("test"), db, nil)

	empty := performJSON(router, http.MethodGet, "/api/v1/search", "", "")
	if empty.Code != http.StatusOK || !strings.Contains(empty.Body.String(), `"total":0`) {
		t.Fatalf("expected empty search to return zero results, got %d: %s", empty.Code, empty.Body.String())
	}

	invalidLimit := performJSON(router, http.MethodGet, "/api/v1/search?q=graph&limit=1000", "", "")
	if invalidLimit.Code != http.StatusBadRequest || !strings.Contains(invalidLimit.Body.String(), "invalid_limit") {
		t.Fatalf("expected invalid limit rejection, got %d: %s", invalidLimit.Code, invalidLimit.Body.String())
	}

	longQuery := strings.Repeat("x", 81)
	tooLong := performJSON(router, http.MethodGet, "/api/v1/search?q="+longQuery, "", "")
	if tooLong.Code != http.StatusBadRequest || !strings.Contains(tooLong.Body.String(), "query_too_long") {
		t.Fatalf("expected long query rejection, got %d: %s", tooLong.Code, tooLong.Body.String())
	}
}

type searchResult struct {
	ID    string `json:"id"`
	Type  string `json:"type"`
	Title string `json:"title"`
	URL   string `json:"url"`
}
