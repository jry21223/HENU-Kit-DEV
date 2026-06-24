package tests

import (
	"net/http"
	"strings"
	"testing"

	"final-review-platform/services/api/internal/platform/model"
	"final-review-platform/services/api/internal/server"
	applogger "final-review-platform/services/api/pkg/logger"
)

func TestMyWikiEntriesAndProposals(t *testing.T) {
	db := newTestDB(t)
	router := server.NewRouter(testConfig(), applogger.New("test"), db, nil)

	unauthorizedEntries := performJSON(router, http.MethodGet, "/api/v1/me/wiki-entries", "", "")
	if unauthorizedEntries.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthenticated wiki entries 401, got %d: %s", unauthorizedEntries.Code, unauthorizedEntries.Body.String())
	}
	unauthorizedProposals := performJSON(router, http.MethodGet, "/api/v1/me/wiki-proposals", "", "")
	if unauthorizedProposals.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthenticated wiki proposals 401, got %d: %s", unauthorizedProposals.Code, unauthorizedProposals.Body.String())
	}

	course := createTestCourse(t, db)
	owner := createTestUser(t, db, "wiki-me-owner@stu.henu.edu.cn", model.RoleCreator)
	other := createTestUser(t, db, "wiki-me-other@stu.henu.edu.cn", model.RoleCreator)
	reviewer := createTestUser(t, db, "wiki-me-reviewer@stu.henu.edu.cn", model.RoleReviewer)
	ownerToken := loginTestUser(t, router, owner.Email)
	otherToken := loginTestUser(t, router, other.Email)
	reviewerToken := loginTestUser(t, router, reviewer.Email)

	createOwnerEntry := performJSON(router, http.MethodPost, "/api/v1/wiki/entries", `{"courseId":"`+course.ID+`","title":"Owner Pending Wiki","slug":"owner-pending-wiki","content":"owner content","summary":"initial"}`, ownerToken)
	if createOwnerEntry.Code != http.StatusOK {
		t.Fatalf("expected owner wiki create 200, got %d: %s", createOwnerEntry.Code, createOwnerEntry.Body.String())
	}
	var ownerEntry model.WikiEntry
	if err := db.First(&ownerEntry, "slug = ?", "owner-pending-wiki").Error; err != nil {
		t.Fatal(err)
	}
	rejectEntry := performJSON(router, http.MethodPost, "/api/v1/admin/wiki/entries/"+ownerEntry.ID+"/reject", `{"reviewReason":"needs clearer structure"}`, reviewerToken)
	if rejectEntry.Code != http.StatusOK {
		t.Fatalf("expected owner wiki reject 200, got %d: %s", rejectEntry.Code, rejectEntry.Body.String())
	}

	createOtherEntry := performJSON(router, http.MethodPost, "/api/v1/wiki/entries", `{"title":"Other Wiki","slug":"other-wiki","content":"other content"}`, otherToken)
	if createOtherEntry.Code != http.StatusOK {
		t.Fatalf("expected other wiki create 200, got %d: %s", createOtherEntry.Code, createOtherEntry.Body.String())
	}
	var otherEntry model.WikiEntry
	if err := db.First(&otherEntry, "slug = ?", "other-wiki").Error; err != nil {
		t.Fatal(err)
	}

	ownerEntries := performJSON(router, http.MethodGet, "/api/v1/me/wiki-entries", "", ownerToken)
	if ownerEntries.Code != http.StatusOK ||
		!strings.Contains(ownerEntries.Body.String(), ownerEntry.ID) ||
		!strings.Contains(ownerEntries.Body.String(), "needs clearer structure") ||
		strings.Contains(ownerEntries.Body.String(), otherEntry.ID) ||
		strings.Contains(ownerEntries.Body.String(), "reviewerId") {
		t.Fatalf("expected owner wiki entries to be scoped and redacted, got %d: %s", ownerEntries.Code, ownerEntries.Body.String())
	}
	otherEntries := performJSON(router, http.MethodGet, "/api/v1/me/wiki-entries", "", otherToken)
	if otherEntries.Code != http.StatusOK || strings.Contains(otherEntries.Body.String(), ownerEntry.ID) || !strings.Contains(otherEntries.Body.String(), otherEntry.ID) {
		t.Fatalf("expected other wiki entries to be scoped, got %d: %s", otherEntries.Code, otherEntries.Body.String())
	}

	publicEntry := model.WikiEntry{
		ReviewFields: model.ReviewFields{Status: model.StatusPublished},
		ContentStats: model.ContentStats{Visibility: "public"},
		AuthorID:     owner.ID,
		CourseID:     &course.ID,
		Title:        "Published Wiki Target",
		Slug:         "published-wiki-target",
		Content:      "published content",
		Version:      1,
	}
	if err := db.Create(&publicEntry).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.WikiEditHistory{EntryID: publicEntry.ID, EditorID: owner.ID, Version: 1, Content: publicEntry.Content, Summary: "initial"}).Error; err != nil {
		t.Fatal(err)
	}

	createProposal := performJSON(router, http.MethodPost, "/api/v1/wiki/entries/"+publicEntry.ID+"/proposals", `{"title":"Owner Proposal","content":"owner proposed content","summary":"clarify"}`, ownerToken)
	if createProposal.Code != http.StatusOK {
		t.Fatalf("expected owner proposal create 200, got %d: %s", createProposal.Code, createProposal.Body.String())
	}
	var proposal model.WikiEditProposal
	if err := db.First(&proposal, "entry_id = ? AND proposed_title = ?", publicEntry.ID, "Owner Proposal").Error; err != nil {
		t.Fatal(err)
	}
	rejectProposal := performJSON(router, http.MethodPost, "/api/v1/admin/wiki/proposals/"+proposal.ID+"/reject", `{"reviewReason":"keep original wording"}`, reviewerToken)
	if rejectProposal.Code != http.StatusOK {
		t.Fatalf("expected owner proposal reject 200, got %d: %s", rejectProposal.Code, rejectProposal.Body.String())
	}

	ownerProposals := performJSON(router, http.MethodGet, "/api/v1/me/wiki-proposals", "", ownerToken)
	if ownerProposals.Code != http.StatusOK ||
		!strings.Contains(ownerProposals.Body.String(), proposal.ID) ||
		!strings.Contains(ownerProposals.Body.String(), "Published Wiki Target") ||
		!strings.Contains(ownerProposals.Body.String(), "keep original wording") ||
		strings.Contains(ownerProposals.Body.String(), "reviewerId") {
		t.Fatalf("expected owner wiki proposals to include own review status without reviewer id, got %d: %s", ownerProposals.Code, ownerProposals.Body.String())
	}
	otherProposals := performJSON(router, http.MethodGet, "/api/v1/me/wiki-proposals", "", otherToken)
	if otherProposals.Code != http.StatusOK || strings.Contains(otherProposals.Body.String(), proposal.ID) {
		t.Fatalf("expected other wiki proposals not to include owner proposal, got %d: %s", otherProposals.Code, otherProposals.Body.String())
	}
}
