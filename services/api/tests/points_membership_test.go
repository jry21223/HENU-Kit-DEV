package tests

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"gorm.io/datatypes"

	"final-review-platform/services/api/internal/platform/model"
	"final-review-platform/services/api/internal/server"
	applogger "final-review-platform/services/api/pkg/logger"
)

func TestPointsEndpointsAreUserScopedAndAdminManaged(t *testing.T) {
	db := newTestDB(t)
	router := server.NewRouter(testConfig(), applogger.New("test"), db, nil)
	user := createTestUser(t, db, "points-user@stu.henu.edu.cn", model.RoleUser)
	other := createTestUser(t, db, "points-other@stu.henu.edu.cn", model.RoleUser)
	admin := createTestUser(t, db, "points-admin@stu.henu.edu.cn", model.RoleAdmin)
	if err := db.Model(&model.User{}).Where("id = ?", user.ID).Update("points_balance", 120).Error; err != nil {
		t.Fatal(err)
	}
	logs := []model.PointsLog{
		{UserID: user.ID, Delta: 120, BalanceAfter: 120, Reason: "seed_bonus", ReferenceType: "seed", ReferenceID: "points-user", IdempotencyKey: "points:user:bonus"},
		{UserID: other.ID, Delta: 50, BalanceAfter: 50, Reason: "seed_bonus", ReferenceType: "seed", ReferenceID: "points-other", IdempotencyKey: "points:other:bonus"},
	}
	if err := db.Create(&logs).Error; err != nil {
		t.Fatal(err)
	}

	unauthorized := performJSON(router, http.MethodGet, "/api/v1/me/points", "", "")
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthenticated points 401, got %d: %s", unauthorized.Code, unauthorized.Body.String())
	}

	userToken := loginTestUser(t, router, user.Email)
	me := performJSON(router, http.MethodGet, "/api/v1/me/points", "", userToken)
	if me.Code != http.StatusOK || !strings.Contains(me.Body.String(), `"balance":120`) {
		t.Fatalf("expected own point balance, got %d: %s", me.Code, me.Body.String())
	}
	myLogs := performJSON(router, http.MethodGet, "/api/v1/me/points/logs", "", userToken)
	if myLogs.Code != http.StatusOK || !strings.Contains(myLogs.Body.String(), logs[0].ID) || strings.Contains(myLogs.Body.String(), logs[1].ID) {
		t.Fatalf("expected only own points logs, got %d: %s", myLogs.Code, myLogs.Body.String())
	}
	forbiddenAdminLogs := performJSON(router, http.MethodGet, "/api/v1/admin/points/logs", "", userToken)
	if forbiddenAdminLogs.Code != http.StatusForbidden {
		t.Fatalf("expected student admin points logs 403, got %d: %s", forbiddenAdminLogs.Code, forbiddenAdminLogs.Body.String())
	}

	adminToken := loginTestUser(t, router, admin.Email)
	adminLogs := performJSON(router, http.MethodGet, "/api/v1/admin/points/logs?userId="+user.ID, "", adminToken)
	if adminLogs.Code != http.StatusOK || !strings.Contains(adminLogs.Body.String(), logs[0].ID) || strings.Contains(adminLogs.Body.String(), logs[1].ID) {
		t.Fatalf("expected admin filtered points logs, got %d: %s", adminLogs.Code, adminLogs.Body.String())
	}

	createRule := performJSON(router, http.MethodPost, "/api/v1/admin/points/rules", `{"code":"wiki_approved","description":"Wiki approved","delta":20,"enabled":true}`, adminToken)
	if createRule.Code != http.StatusOK {
		t.Fatalf("expected create points rule 200, got %d: %s", createRule.Code, createRule.Body.String())
	}
	var created struct {
		Data struct {
			Rule model.PointsRule `json:"rule"`
		} `json:"data"`
	}
	if err := json.Unmarshal(createRule.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Data.Rule.Code != "wiki_approved" || created.Data.Rule.Delta != 20 || !created.Data.Rule.Enabled {
		t.Fatalf("unexpected created rule: %#v", created.Data.Rule)
	}
	updateRule := performJSON(router, http.MethodPatch, "/api/v1/admin/points/rules/"+created.Data.Rule.ID, `{"enabled":false,"delta":25}`, adminToken)
	if updateRule.Code != http.StatusOK || !strings.Contains(updateRule.Body.String(), `"delta":25`) || !strings.Contains(updateRule.Body.String(), `"enabled":false`) {
		t.Fatalf("expected updated points rule, got %d: %s", updateRule.Code, updateRule.Body.String())
	}
	if countOperationLogs(t, db, "points_rule.created", "points_rule", created.Data.Rule.ID, admin.ID) != 1 {
		t.Fatal("expected points rule creation audit log")
	}
	if countOperationLogs(t, db, "points_rule.updated", "points_rule", created.Data.Rule.ID, admin.ID) != 1 {
		t.Fatal("expected points rule update audit log")
	}
}

func TestMembershipGrantRevokeAndVisibility(t *testing.T) {
	db := newTestDB(t)
	router := server.NewRouter(testConfig(), applogger.New("test"), db, nil)
	user := createTestUser(t, db, "member-user@stu.henu.edu.cn", model.RoleUser)
	admin := createTestUser(t, db, "member-admin@stu.henu.edu.cn", model.RoleAdmin)
	plans := []model.MembershipPlan{
		{Code: "tier1", Name: "Tier 1", PriceFen: 990, PointsCost: 300, DurationDays: 30, Benefits: datatypes.JSON([]byte(`{"aiDiscount":true}`)), Status: model.StatusPublished},
		{Code: "tier2", Name: "Tier 2", PriceFen: 1990, PointsCost: 600, DurationDays: 30, Benefits: datatypes.JSON([]byte(`{"aiFree":true}`)), Status: model.StatusDraft},
	}
	if err := db.Create(&plans).Error; err != nil {
		t.Fatal(err)
	}

	publicPlans := performJSON(router, http.MethodGet, "/api/v1/membership/plans", "", "")
	if publicPlans.Code != http.StatusOK || !strings.Contains(publicPlans.Body.String(), `"code":"tier1"`) || strings.Contains(publicPlans.Body.String(), `"code":"tier2"`) {
		t.Fatalf("expected only published membership plans, got %d: %s", publicPlans.Code, publicPlans.Body.String())
	}

	userToken := loginTestUser(t, router, user.Email)
	emptyMe := performJSON(router, http.MethodGet, "/api/v1/me/membership", "", userToken)
	if emptyMe.Code != http.StatusOK || !strings.Contains(emptyMe.Body.String(), `"memberships":[]`) {
		t.Fatalf("expected empty membership list, got %d: %s", emptyMe.Code, emptyMe.Body.String())
	}
	forbiddenGrant := performJSON(router, http.MethodPost, "/api/v1/admin/memberships/grant", `{"userId":"`+user.ID+`","planCode":"tier1"}`, userToken)
	if forbiddenGrant.Code != http.StatusForbidden {
		t.Fatalf("expected student membership grant 403, got %d: %s", forbiddenGrant.Code, forbiddenGrant.Body.String())
	}

	adminToken := loginTestUser(t, router, admin.Email)
	expireAt := time.Now().Add(30 * 24 * time.Hour).UTC().Format(time.RFC3339)
	grant := performJSON(router, http.MethodPost, "/api/v1/admin/memberships/grant", `{"userId":"`+user.ID+`","planCode":"tier1","expiresAt":"`+expireAt+`","note":"internal test grant"}`, adminToken)
	if grant.Code != http.StatusOK {
		t.Fatalf("expected membership grant 200, got %d: %s", grant.Code, grant.Body.String())
	}
	var granted struct {
		Data struct {
			Membership model.Membership `json:"membership"`
			Created    bool             `json:"created"`
		} `json:"data"`
	}
	if err := json.Unmarshal(grant.Body.Bytes(), &granted); err != nil {
		t.Fatal(err)
	}
	if !granted.Data.Created || granted.Data.Membership.UserID != user.ID || granted.Data.Membership.PlanCode != "tier1" || granted.Data.Membership.Status != "active" {
		t.Fatalf("unexpected granted membership: %#v", granted.Data)
	}

	afterGrantMe := performJSON(router, http.MethodGet, "/api/v1/me/membership", "", userToken)
	if afterGrantMe.Code != http.StatusOK || !strings.Contains(afterGrantMe.Body.String(), granted.Data.Membership.ID) || !strings.Contains(afterGrantMe.Body.String(), `"current"`) {
		t.Fatalf("expected active membership in user view, got %d: %s", afterGrantMe.Code, afterGrantMe.Body.String())
	}
	adminList := performJSON(router, http.MethodGet, "/api/v1/admin/memberships?userId="+user.ID, "", adminToken)
	if adminList.Code != http.StatusOK || !strings.Contains(adminList.Body.String(), granted.Data.Membership.ID) || !strings.Contains(adminList.Body.String(), user.Email) {
		t.Fatalf("expected admin membership list with user, got %d: %s", adminList.Code, adminList.Body.String())
	}

	duplicateGrant := performJSON(router, http.MethodPost, "/api/v1/admin/memberships/grant", `{"userId":"`+user.ID+`","planCode":"tier1","expiresAt":"`+time.Now().Add(60*24*time.Hour).UTC().Format(time.RFC3339)+`"}`, adminToken)
	if duplicateGrant.Code != http.StatusOK || !strings.Contains(duplicateGrant.Body.String(), `"created":false`) {
		t.Fatalf("expected duplicate active grant to update existing membership, got %d: %s", duplicateGrant.Code, duplicateGrant.Body.String())
	}

	revoke := performJSON(router, http.MethodPost, "/api/v1/admin/memberships/"+granted.Data.Membership.ID+"/revoke", `{"reason":"manual revoke"}`, adminToken)
	if revoke.Code != http.StatusOK || !strings.Contains(revoke.Body.String(), `"status":"revoked"`) {
		t.Fatalf("expected membership revoke, got %d: %s", revoke.Code, revoke.Body.String())
	}
	afterRevokeMe := performJSON(router, http.MethodGet, "/api/v1/me/membership", "", userToken)
	if afterRevokeMe.Code != http.StatusOK || strings.Contains(afterRevokeMe.Body.String(), granted.Data.Membership.ID) {
		t.Fatalf("expected revoked membership hidden from active user view, got %d: %s", afterRevokeMe.Code, afterRevokeMe.Body.String())
	}
	if countOperationLogs(t, db, "membership.granted", "membership", granted.Data.Membership.ID, admin.ID) < 1 {
		t.Fatal("expected membership grant audit log")
	}
	if countOperationLogs(t, db, "membership.revoked", "membership", granted.Data.Membership.ID, admin.ID) != 1 {
		t.Fatal("expected membership revoke audit log")
	}

	pastGrant := performJSON(router, http.MethodPost, "/api/v1/admin/memberships/grant", `{"userId":"`+user.ID+`","planCode":"tier1","expiresAt":"`+time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)+`"}`, adminToken)
	if pastGrant.Code != http.StatusBadRequest || !strings.Contains(pastGrant.Body.String(), "expires_at_in_past") {
		t.Fatalf("expected past grant rejection, got %d: %s", pastGrant.Code, pastGrant.Body.String())
	}
	unpublishedPlan := performJSON(router, http.MethodPost, "/api/v1/admin/memberships/grant", `{"userId":"`+user.ID+`","planCode":"tier2"}`, adminToken)
	if unpublishedPlan.Code != http.StatusBadRequest || !strings.Contains(unpublishedPlan.Body.String(), "plan_not_found") {
		t.Fatalf("expected unpublished plan rejection, got %d: %s", unpublishedPlan.Code, unpublishedPlan.Body.String())
	}
}

func TestMembershipRedeemWithPoints(t *testing.T) {
	db := newTestDB(t)
	router := server.NewRouter(testConfig(), applogger.New("test"), db, nil)
	user := createTestUser(t, db, "member-redeem@stu.henu.edu.cn", model.RoleUser)
	poorUser := createTestUser(t, db, "member-redeem-poor@stu.henu.edu.cn", model.RoleUser)
	frozenUser := createTestUser(t, db, "member-redeem-frozen@stu.henu.edu.cn", model.RoleUser)
	plans := []model.MembershipPlan{
		{Code: "tier1", Name: "Tier 1", PriceFen: 990, PointsCost: 100, DurationDays: 30, Benefits: datatypes.JSON([]byte(`{"aiDiscount":true}`)), Status: model.StatusPublished},
		{Code: "cash_only", Name: "Cash Only", PriceFen: 1990, PointsCost: 0, DurationDays: 30, Benefits: datatypes.JSON([]byte(`{"aiFree":true}`)), Status: model.StatusPublished},
		{Code: "draft_plan", Name: "Draft", PriceFen: 1990, PointsCost: 50, DurationDays: 30, Status: model.StatusDraft},
	}
	if err := db.Create(&plans).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&model.User{}).Where("id = ?", user.ID).Update("points_balance", 150).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&model.User{}).Where("id = ?", poorUser.ID).Update("points_balance", 20).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&model.User{}).Where("id = ?", frozenUser.ID).Updates(map[string]interface{}{"points_balance": 200, "status": "frozen"}).Error; err != nil {
		t.Fatal(err)
	}

	unauthorized := performJSON(router, http.MethodPost, "/api/v1/membership/redeem", `{"planCode":"tier1","requestId":"redeem-1"}`, "")
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthenticated redeem 401, got %d: %s", unauthorized.Code, unauthorized.Body.String())
	}

	userToken := loginTestUser(t, router, user.Email)
	missingRequestID := performJSON(router, http.MethodPost, "/api/v1/membership/redeem", `{"planCode":"tier1"}`, userToken)
	if missingRequestID.Code != http.StatusBadRequest || !strings.Contains(missingRequestID.Body.String(), "request_id_required") {
		t.Fatalf("expected request id required, got %d: %s", missingRequestID.Code, missingRequestID.Body.String())
	}
	cashOnly := performJSON(router, http.MethodPost, "/api/v1/membership/redeem", `{"planCode":"cash_only","requestId":"cash-only"}`, userToken)
	if cashOnly.Code != http.StatusBadRequest || !strings.Contains(cashOnly.Body.String(), "plan_not_redeemable") {
		t.Fatalf("expected cash-only plan not redeemable, got %d: %s", cashOnly.Code, cashOnly.Body.String())
	}
	draftPlan := performJSON(router, http.MethodPost, "/api/v1/membership/redeem", `{"planCode":"draft_plan","requestId":"draft"}`, userToken)
	if draftPlan.Code != http.StatusBadRequest || !strings.Contains(draftPlan.Body.String(), "plan_not_found") {
		t.Fatalf("expected draft plan not found, got %d: %s", draftPlan.Code, draftPlan.Body.String())
	}

	poorToken := loginTestUser(t, router, poorUser.Email)
	insufficient := performJSON(router, http.MethodPost, "/api/v1/membership/redeem", `{"planCode":"tier1","requestId":"poor"}`, poorToken)
	if insufficient.Code != http.StatusBadRequest || !strings.Contains(insufficient.Body.String(), "insufficient_points") {
		t.Fatalf("expected insufficient points, got %d: %s", insufficient.Code, insufficient.Body.String())
	}

	frozenToken := loginTestUser(t, router, frozenUser.Email)
	frozen := performJSON(router, http.MethodPost, "/api/v1/membership/redeem", `{"planCode":"tier1","requestId":"frozen"}`, frozenToken)
	if frozen.Code != http.StatusForbidden || !strings.Contains(frozen.Body.String(), "user_frozen") {
		t.Fatalf("expected frozen user redeem 403, got %d: %s", frozen.Code, frozen.Body.String())
	}

	redeem := performJSON(router, http.MethodPost, "/api/v1/membership/redeem", `{"planCode":"tier1","requestId":"redeem-1"}`, userToken)
	if redeem.Code != http.StatusOK {
		t.Fatalf("expected points redeem 200, got %d: %s", redeem.Code, redeem.Body.String())
	}
	var payload struct {
		Data struct {
			Membership      model.Membership     `json:"membership"`
			Plan            model.MembershipPlan `json:"plan"`
			PointsBalance   int64                `json:"pointsBalance"`
			AlreadyRedeemed bool                 `json:"alreadyRedeemed"`
		} `json:"data"`
	}
	if err := json.Unmarshal(redeem.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Data.AlreadyRedeemed || payload.Data.PointsBalance != 50 || payload.Data.Membership.UserID != user.ID || payload.Data.Membership.PlanCode != "tier1" || payload.Data.Membership.Source != "points_redeem" || payload.Data.Membership.ExpiresAt == nil {
		t.Fatalf("unexpected redeem payload: %#v", payload.Data)
	}
	var refreshed model.User
	if err := db.First(&refreshed, "id = ?", user.ID).Error; err != nil {
		t.Fatal(err)
	}
	if refreshed.PointsBalance != 50 {
		t.Fatalf("expected points balance 50 after redeem, got %d", refreshed.PointsBalance)
	}
	var logCount int64
	if err := db.Model(&model.PointsLog{}).Where("user_id = ? AND delta = ? AND balance_after = ? AND reason = ? AND reference_type = ? AND reference_id = ?", user.ID, -100, 50, "membership_redeem", "membership", payload.Data.Membership.ID).Count(&logCount).Error; err != nil {
		t.Fatal(err)
	}
	if logCount != 1 {
		t.Fatalf("expected one membership redeem points log, got %d", logCount)
	}
	if countOperationLogs(t, db, "membership.redeemed", "membership", payload.Data.Membership.ID, user.ID) != 1 {
		t.Fatal("expected membership redeem audit log")
	}
	me := performJSON(router, http.MethodGet, "/api/v1/me/membership", "", userToken)
	if me.Code != http.StatusOK || !strings.Contains(me.Body.String(), payload.Data.Membership.ID) || !strings.Contains(me.Body.String(), `"current"`) {
		t.Fatalf("expected redeemed membership in user view, got %d: %s", me.Code, me.Body.String())
	}

	duplicate := performJSON(router, http.MethodPost, "/api/v1/membership/redeem", `{"planCode":"tier1","requestId":"redeem-1"}`, userToken)
	if duplicate.Code != http.StatusOK || !strings.Contains(duplicate.Body.String(), `"alreadyRedeemed":true`) || !strings.Contains(duplicate.Body.String(), `"pointsBalance":50`) {
		t.Fatalf("expected duplicate request id to be idempotent, got %d: %s", duplicate.Code, duplicate.Body.String())
	}
	var duplicateLogCount int64
	if err := db.Model(&model.PointsLog{}).Where("user_id = ? AND reason = ? AND reference_id = ?", user.ID, "membership_redeem", payload.Data.Membership.ID).Count(&duplicateLogCount).Error; err != nil {
		t.Fatal(err)
	}
	if duplicateLogCount != 1 {
		t.Fatalf("expected duplicate redeem not to create another points log, got %d", duplicateLogCount)
	}

	firstExpiresAt := payload.Data.Membership.ExpiresAt
	if firstExpiresAt == nil {
		t.Fatal("expected first redeemed membership to have an expiry")
	}
	if err := db.Model(&model.User{}).Where("id = ?", user.ID).Update("points_balance", 150).Error; err != nil {
		t.Fatal(err)
	}
	secondRedeem := performJSON(router, http.MethodPost, "/api/v1/membership/redeem", `{"planCode":"tier1","requestId":"redeem-2"}`, userToken)
	if secondRedeem.Code != http.StatusOK {
		t.Fatalf("expected second redeem to extend membership, got %d: %s", secondRedeem.Code, secondRedeem.Body.String())
	}
	var secondPayload struct {
		Data struct {
			Membership    model.Membership `json:"membership"`
			PointsBalance int64            `json:"pointsBalance"`
		} `json:"data"`
	}
	if err := json.Unmarshal(secondRedeem.Body.Bytes(), &secondPayload); err != nil {
		t.Fatal(err)
	}
	if secondPayload.Data.Membership.ID != payload.Data.Membership.ID || secondPayload.Data.PointsBalance != 50 || secondPayload.Data.Membership.ExpiresAt == nil || !secondPayload.Data.Membership.ExpiresAt.After(*firstExpiresAt) {
		t.Fatalf("expected second redeem to extend same membership and deduct points, got %#v", secondPayload.Data)
	}
	var finalLogCount int64
	if err := db.Model(&model.PointsLog{}).Where("user_id = ? AND reason = ? AND reference_id = ?", user.ID, "membership_redeem", payload.Data.Membership.ID).Count(&finalLogCount).Error; err != nil {
		t.Fatal(err)
	}
	if finalLogCount != 2 {
		t.Fatalf("expected second redeem to create one more points log, got %d", finalLogCount)
	}
}
