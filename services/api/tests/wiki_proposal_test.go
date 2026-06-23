package tests

import (
	"net/http"
	"strings"
	"testing"

	"final-review-platform/services/api/internal/platform/model"
	"final-review-platform/services/api/internal/server"
	applogger "final-review-platform/services/api/pkg/logger"
)

func TestWikiEditProposalWorkflow(t *testing.T) {
	db := newTestDB(t)
	router := server.NewRouter(testConfig(), applogger.New("test"), db, nil)
	course := createTestCourse(t, db)
	creator := createTestUser(t, db, "wiki-proposal-creator@stu.henu.edu.cn", model.RoleCreator)
	entry := model.WikiEntry{
		ReviewFields: model.ReviewFields{Status: model.StatusPublished},
		ContentStats: model.ContentStats{Visibility: "public"},
		AuthorID:     creator.ID,
		CourseID:     &course.ID,
		Title:        "Original Logic",
		Slug:         "original-logic",
		Content:      "Original content",
		Version:      1,
	}
	if err := db.Create(&entry).Error; err != nil {
		t.Fatal(err)
	}
	initialHistory := model.WikiEditHistory{EntryID: entry.ID, EditorID: creator.ID, Version: 1, Content: entry.Content, Summary: "initial"}
	if err := db.Create(&initialHistory).Error; err != nil {
		t.Fatal(err)
	}
	pendingEntry := model.WikiEntry{
		ReviewFields: model.ReviewFields{Status: model.StatusPending},
		ContentStats: model.ContentStats{Visibility: "public"},
		AuthorID:     creator.ID,
		Title:        "Hidden Entry",
		Slug:         "hidden-entry",
		Content:      "hidden",
		Version:      1,
	}
	if err := db.Create(&pendingEntry).Error; err != nil {
		t.Fatal(err)
	}

	unauthorizedCreate := performJSON(router, http.MethodPost, "/api/v1/wiki/entries/"+entry.ID+"/proposals", `{"title":"No auth","content":"body"}`, "")
	if unauthorizedCreate.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthenticated proposal create 401, got %d: %s", unauthorizedCreate.Code, unauthorizedCreate.Body.String())
	}
	studentToken := loginTestUser(t, router, "wiki-proposal-student@stu.henu.edu.cn")
	studentCreate := performJSON(router, http.MethodPost, "/api/v1/wiki/entries/"+entry.ID+"/proposals", `{"title":"Student edit","content":"body"}`, studentToken)
	if studentCreate.Code != http.StatusForbidden {
		t.Fatalf("expected non-creator proposal create 403, got %d: %s", studentCreate.Code, studentCreate.Body.String())
	}

	creatorToken := loginTestUser(t, router, "wiki-proposal-creator@stu.henu.edu.cn")
	unchanged := performJSON(router, http.MethodPost, "/api/v1/wiki/entries/"+entry.ID+"/proposals", `{"title":"Original Logic","content":"Original content"}`, creatorToken)
	if unchanged.Code != http.StatusBadRequest || !strings.Contains(unchanged.Body.String(), "proposal_unchanged") {
		t.Fatalf("expected unchanged proposal rejection, got %d: %s", unchanged.Code, unchanged.Body.String())
	}
	hiddenTarget := performJSON(router, http.MethodPost, "/api/v1/wiki/entries/"+pendingEntry.ID+"/proposals", `{"title":"Edit hidden","content":"body"}`, creatorToken)
	if hiddenTarget.Code != http.StatusNotFound || !strings.Contains(hiddenTarget.Body.String(), "entry_not_found") {
		t.Fatalf("expected hidden-entry proposal rejection, got %d: %s", hiddenTarget.Code, hiddenTarget.Body.String())
	}

	createProposal := performJSON(router, http.MethodPost, "/api/v1/wiki/entries/"+entry.ID+"/proposals", `{"title":"Updated Logic","content":"Updated content","summary":"clarify definitions"}`, creatorToken)
	if createProposal.Code != http.StatusOK {
		t.Fatalf("expected proposal create 200, got %d: %s", createProposal.Code, createProposal.Body.String())
	}
	var proposal model.WikiEditProposal
	if err := db.First(&proposal, "entry_id = ? AND proposed_title = ?", entry.ID, "Updated Logic").Error; err != nil {
		t.Fatal(err)
	}
	if proposal.Status != model.StatusPending || proposal.EditorID != creator.ID || proposal.BaseVersion != 1 {
		t.Fatalf("expected pending proposal at base version 1, got %#v", proposal)
	}
	publicBeforeReview := performJSON(router, http.MethodGet, "/api/v1/wiki/entries/"+entry.ID, "", "")
	if publicBeforeReview.Code != http.StatusOK || !strings.Contains(publicBeforeReview.Body.String(), "Original content") || strings.Contains(publicBeforeReview.Body.String(), "Updated content") {
		t.Fatalf("expected proposal not to change public entry before review, got %d: %s", publicBeforeReview.Code, publicBeforeReview.Body.String())
	}

	forbiddenList := performJSON(router, http.MethodGet, "/api/v1/admin/wiki/proposals", "", studentToken)
	if forbiddenList.Code != http.StatusForbidden {
		t.Fatalf("expected student wiki proposal list 403, got %d: %s", forbiddenList.Code, forbiddenList.Body.String())
	}
	forbiddenApprove := performJSON(router, http.MethodPost, "/api/v1/admin/wiki/proposals/"+proposal.ID+"/approve", `{"reviewReason":"ok"}`, studentToken)
	if forbiddenApprove.Code != http.StatusForbidden {
		t.Fatalf("expected student wiki proposal approve 403, got %d: %s", forbiddenApprove.Code, forbiddenApprove.Body.String())
	}

	reviewer := createTestUser(t, db, "wiki-proposal-reviewer@stu.henu.edu.cn", model.RoleReviewer)
	reviewerToken := loginTestUser(t, router, "wiki-proposal-reviewer@stu.henu.edu.cn")
	reviewList := performJSON(router, http.MethodGet, "/api/v1/admin/wiki/proposals", "", reviewerToken)
	if reviewList.Code != http.StatusOK || !strings.Contains(reviewList.Body.String(), proposal.ID) {
		t.Fatalf("expected reviewer proposal list to include pending proposal, got %d: %s", reviewList.Code, reviewList.Body.String())
	}
	rejectWithoutReason := performJSON(router, http.MethodPost, "/api/v1/admin/wiki/proposals/"+proposal.ID+"/reject", "", reviewerToken)
	if rejectWithoutReason.Code != http.StatusBadRequest || !strings.Contains(rejectWithoutReason.Body.String(), "review_reason_required") {
		t.Fatalf("expected proposal reject reason required, got %d: %s", rejectWithoutReason.Code, rejectWithoutReason.Body.String())
	}

	approve := performJSON(router, http.MethodPost, "/api/v1/admin/wiki/proposals/"+proposal.ID+"/approve", `{"reviewReason":"accurate revision"}`, reviewerToken)
	if approve.Code != http.StatusOK {
		t.Fatalf("expected reviewer proposal approve 200, got %d: %s", approve.Code, approve.Body.String())
	}
	var approvedProposal model.WikiEditProposal
	if err := db.First(&approvedProposal, "id = ?", proposal.ID).Error; err != nil {
		t.Fatal(err)
	}
	if approvedProposal.Status != model.StatusPublished || approvedProposal.ReviewerID == nil || *approvedProposal.ReviewerID != reviewer.ID || approvedProposal.ReviewedAt == nil || approvedProposal.ReviewReason != "accurate revision" {
		t.Fatalf("expected approved proposal metadata, got %#v", approvedProposal)
	}
	var updatedEntry model.WikiEntry
	if err := db.First(&updatedEntry, "id = ?", entry.ID).Error; err != nil {
		t.Fatal(err)
	}
	if updatedEntry.Title != "Updated Logic" || updatedEntry.Content != "Updated content" || updatedEntry.Version != 2 {
		t.Fatalf("expected approved proposal to update live entry version 2, got %#v", updatedEntry)
	}
	var historyCount int64
	if err := db.Model(&model.WikiEditHistory{}).Where("entry_id = ? AND version = ? AND content = ?", entry.ID, 2, "Updated content").Count(&historyCount).Error; err != nil {
		t.Fatal(err)
	}
	if historyCount != 1 {
		t.Fatalf("expected one version-2 wiki edit history row, got %d", historyCount)
	}
	if countOperationLogs(t, db, "wiki_proposal.published", "wiki_edit_proposal", proposal.ID, reviewer.ID) != 1 {
		t.Fatal("expected wiki proposal approval operation log")
	}
	publicAfterApprove := performJSON(router, http.MethodGet, "/api/v1/wiki/entries/"+entry.ID, "", "")
	if publicAfterApprove.Code != http.StatusOK || !strings.Contains(publicAfterApprove.Body.String(), "Updated content") {
		t.Fatalf("expected approved proposal to change public entry, got %d: %s", publicAfterApprove.Code, publicAfterApprove.Body.String())
	}

	reviewAgain := performJSON(router, http.MethodPost, "/api/v1/admin/wiki/proposals/"+proposal.ID+"/reject", `{"reviewReason":"overwrite"}`, reviewerToken)
	if reviewAgain.Code != http.StatusConflict || !strings.Contains(reviewAgain.Body.String(), "proposal_not_reviewable") {
		t.Fatalf("expected reviewed proposal conflict, got %d: %s", reviewAgain.Code, reviewAgain.Body.String())
	}
	if countOperationLogs(t, db, "wiki_proposal.published", "wiki_edit_proposal", proposal.ID, reviewer.ID) != 1 {
		t.Fatal("expected repeated proposal review to avoid duplicate logs")
	}
	if err := db.Model(&model.WikiEditHistory{}).Where("entry_id = ? AND version = ?", entry.ID, 2).Count(&historyCount).Error; err != nil {
		t.Fatal(err)
	}
	if historyCount != 1 {
		t.Fatalf("expected repeated proposal review to avoid duplicate history, got %d", historyCount)
	}

	staleResponse := performJSON(router, http.MethodPost, "/api/v1/wiki/entries/"+entry.ID+"/proposals", `{"title":"Stale Edit","content":"Stale content","summary":"stale"}`, creatorToken)
	if staleResponse.Code != http.StatusOK {
		t.Fatalf("expected stale-target proposal create 200, got %d: %s", staleResponse.Code, staleResponse.Body.String())
	}
	var staleProposal model.WikiEditProposal
	if err := db.First(&staleProposal, "entry_id = ? AND proposed_title = ?", entry.ID, "Stale Edit").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&model.WikiEntry{}).Where("id = ?", entry.ID).Updates(map[string]interface{}{"version": 3, "content": "Externally changed"}).Error; err != nil {
		t.Fatal(err)
	}
	staleApprove := performJSON(router, http.MethodPost, "/api/v1/admin/wiki/proposals/"+staleProposal.ID+"/approve", `{"reviewReason":"too late"}`, reviewerToken)
	if staleApprove.Code != http.StatusConflict || !strings.Contains(staleApprove.Body.String(), "proposal_stale") {
		t.Fatalf("expected stale proposal conflict, got %d: %s", staleApprove.Code, staleApprove.Body.String())
	}
	var staleAfter model.WikiEditProposal
	if err := db.First(&staleAfter, "id = ?", staleProposal.ID).Error; err != nil {
		t.Fatal(err)
	}
	if staleAfter.Status != model.StatusPending {
		t.Fatalf("expected stale proposal review rollback to keep status pending, got %#v", staleAfter)
	}
	if countOperationLogs(t, db, "wiki_proposal.published", "wiki_edit_proposal", staleProposal.ID, reviewer.ID) != 0 {
		t.Fatal("expected stale proposal to avoid approval log")
	}

	rejectResponse := performJSON(router, http.MethodPost, "/api/v1/wiki/entries/"+entry.ID+"/proposals", `{"title":"Rejected Edit","content":"Rejected content"}`, creatorToken)
	if rejectResponse.Code != http.StatusOK {
		t.Fatalf("expected reject-target proposal create 200, got %d: %s", rejectResponse.Code, rejectResponse.Body.String())
	}
	var rejectedProposal model.WikiEditProposal
	if err := db.First(&rejectedProposal, "entry_id = ? AND proposed_title = ?", entry.ID, "Rejected Edit").Error; err != nil {
		t.Fatal(err)
	}
	admin := createTestUser(t, db, "wiki-proposal-admin@stu.henu.edu.cn", model.RoleAdmin)
	adminToken := loginTestUser(t, router, "wiki-proposal-admin@stu.henu.edu.cn")
	reject := performJSON(router, http.MethodPost, "/api/v1/admin/wiki/proposals/"+rejectedProposal.ID+"/reject", `{"reviewReason":"not enough context"}`, adminToken)
	if reject.Code != http.StatusOK {
		t.Fatalf("expected admin proposal reject 200, got %d: %s", reject.Code, reject.Body.String())
	}
	var rejected model.WikiEditProposal
	if err := db.First(&rejected, "id = ?", rejectedProposal.ID).Error; err != nil {
		t.Fatal(err)
	}
	if rejected.Status != model.StatusRejected || rejected.ReviewerID == nil || *rejected.ReviewerID != admin.ID || rejected.ReviewReason != "not enough context" {
		t.Fatalf("expected rejected proposal metadata, got %#v", rejected)
	}
	if countOperationLogs(t, db, "wiki_proposal.rejected", "wiki_edit_proposal", rejectedProposal.ID, admin.ID) != 1 {
		t.Fatal("expected wiki proposal rejection operation log")
	}

	frozenProposalResponse := performJSON(router, http.MethodPost, "/api/v1/wiki/entries/"+entry.ID+"/proposals", `{"title":"Frozen Review Edit","content":"Frozen review content"}`, creatorToken)
	if frozenProposalResponse.Code != http.StatusOK {
		t.Fatalf("expected frozen-review proposal create 200, got %d: %s", frozenProposalResponse.Code, frozenProposalResponse.Body.String())
	}
	var frozenProposal model.WikiEditProposal
	if err := db.First(&frozenProposal, "entry_id = ? AND proposed_title = ?", entry.ID, "Frozen Review Edit").Error; err != nil {
		t.Fatal(err)
	}
	frozenReviewer := createTestUser(t, db, "frozen-wiki-proposal-reviewer@stu.henu.edu.cn", model.RoleReviewer)
	frozenReviewerToken := loginTestUser(t, router, "frozen-wiki-proposal-reviewer@stu.henu.edu.cn")
	if err := db.Model(&model.User{}).Where("id = ?", frozenReviewer.ID).Update("status", "frozen").Error; err != nil {
		t.Fatal(err)
	}
	frozenReview := performJSON(router, http.MethodPost, "/api/v1/admin/wiki/proposals/"+frozenProposal.ID+"/approve", `{"reviewReason":"ok"}`, frozenReviewerToken)
	if frozenReview.Code != http.StatusForbidden || !strings.Contains(frozenReview.Body.String(), "user_frozen") {
		t.Fatalf("expected frozen reviewer proposal approve 403, got %d: %s", frozenReview.Code, frozenReview.Body.String())
	}
	if countOperationLogs(t, db, "wiki_proposal.published", "wiki_edit_proposal", frozenProposal.ID, frozenReviewer.ID) != 0 {
		t.Fatal("expected frozen reviewer to avoid wiki proposal approval logs")
	}
}
