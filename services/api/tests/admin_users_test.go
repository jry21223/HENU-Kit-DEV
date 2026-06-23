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

func TestAdminUserManagementRequiresAdminAndFiltersUsers(t *testing.T) {
	db := newTestDB(t)
	router := server.NewRouter(testConfig(), applogger.New("test"), db, nil)

	student := createTestUser(t, db, "student-users@stu.henu.edu.cn", model.RoleUser)
	createTestUser(t, db, "reviewer-users@stu.henu.edu.cn", model.RoleReviewer)
	admin := createTestUser(t, db, "admin-users@stu.henu.edu.cn", model.RoleAdmin)
	studentToken := loginTestUser(t, router, student.Email)
	adminToken := loginTestUser(t, router, admin.Email)

	unauthenticated := performJSON(router, http.MethodGet, "/api/v1/admin/users", "", "")
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthenticated list 401, got %d: %s", unauthenticated.Code, unauthenticated.Body.String())
	}
	forbidden := performJSON(router, http.MethodGet, "/api/v1/admin/users", "", studentToken)
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("expected student list 403, got %d: %s", forbidden.Code, forbidden.Body.String())
	}
	invalidRole := performJSON(router, http.MethodGet, "/api/v1/admin/users?role=root", "", adminToken)
	if invalidRole.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid role 400, got %d: %s", invalidRole.Code, invalidRole.Body.String())
	}
	filtered := performJSON(router, http.MethodGet, "/api/v1/admin/users?role=reviewer&email=reviewer-users", "", adminToken)
	if filtered.Code != http.StatusOK {
		t.Fatalf("expected filtered user list 200, got %d: %s", filtered.Code, filtered.Body.String())
	}
	var payload struct {
		Data struct {
			Users []model.User `json:"users"`
		} `json:"data"`
	}
	if err := json.Unmarshal(filtered.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Data.Users) != 1 || payload.Data.Users[0].Role != model.RoleReviewer || !strings.Contains(payload.Data.Users[0].Email, "reviewer-users") {
		t.Fatalf("unexpected filtered users: %#v", payload.Data.Users)
	}
}

func TestAdminCanUpdateUserStatusAndFrozenUserIsBlocked(t *testing.T) {
	db := newTestDB(t)
	router := server.NewRouter(testConfig(), applogger.New("test"), db, nil)

	admin := createTestUser(t, db, "freeze-admin@stu.henu.edu.cn", model.RoleAdmin)
	target := createTestUser(t, db, "freeze-target@stu.henu.edu.cn", model.RoleUser)
	adminToken := loginTestUser(t, router, admin.Email)
	targetToken := loginTestUser(t, router, target.Email)

	update := performJSON(router, http.MethodPatch, "/api/v1/admin/users/"+target.ID, `{"name":"Frozen Target","status":"frozen"}`, adminToken)
	if update.Code != http.StatusOK {
		t.Fatalf("expected user update 200, got %d: %s", update.Code, update.Body.String())
	}
	var updated model.User
	if err := db.First(&updated, "id = ?", target.ID).Error; err != nil {
		t.Fatal(err)
	}
	if updated.Name != "Frozen Target" || updated.Status != "frozen" {
		t.Fatalf("unexpected updated user: %#v", updated)
	}
	if countOperationLogs(t, db, "user.update", "user", target.ID, admin.ID) != 1 {
		t.Fatal("expected user.update operation log")
	}

	frozenProfileUpdate := performJSON(router, http.MethodPatch, "/api/v1/auth/me", `{"name":"Should Fail"}`, targetToken)
	if frozenProfileUpdate.Code != http.StatusForbidden {
		t.Fatalf("expected frozen user profile update 403, got %d: %s", frozenProfileUpdate.Code, frozenProfileUpdate.Body.String())
	}
}

func TestAdminUserManagementRejectsUnsafeRoleChanges(t *testing.T) {
	db := newTestDB(t)
	router := server.NewRouter(testConfig(), applogger.New("test"), db, nil)

	admin := createTestUser(t, db, "role-admin@stu.henu.edu.cn", model.RoleAdmin)
	target := createTestUser(t, db, "role-target@stu.henu.edu.cn", model.RoleUser)
	superAdmin := createTestUser(t, db, "role-super@stu.henu.edu.cn", model.RoleSuperAdmin)
	adminToken := loginTestUser(t, router, admin.Email)
	superAdminToken := loginTestUser(t, router, superAdmin.Email)

	selfDemotion := performJSON(router, http.MethodPatch, "/api/v1/admin/users/"+admin.ID, `{"role":"user"}`, adminToken)
	if selfDemotion.Code != http.StatusForbidden {
		t.Fatalf("expected self role update 403, got %d: %s", selfDemotion.Code, selfDemotion.Body.String())
	}
	selfFreeze := performJSON(router, http.MethodPatch, "/api/v1/admin/users/"+admin.ID, `{"status":"frozen"}`, adminToken)
	if selfFreeze.Code != http.StatusForbidden {
		t.Fatalf("expected self status update 403, got %d: %s", selfFreeze.Code, selfFreeze.Body.String())
	}
	grantSuperAdmin := performJSON(router, http.MethodPatch, "/api/v1/admin/users/"+target.ID, `{"role":"super_admin"}`, adminToken)
	if grantSuperAdmin.Code != http.StatusForbidden {
		t.Fatalf("expected non-super admin grant 403, got %d: %s", grantSuperAdmin.Code, grantSuperAdmin.Body.String())
	}
	editSuperAdmin := performJSON(router, http.MethodPatch, "/api/v1/admin/users/"+superAdmin.ID, `{"name":"Edited"}`, adminToken)
	if editSuperAdmin.Code != http.StatusForbidden {
		t.Fatalf("expected non-super admin edit super admin 403, got %d: %s", editSuperAdmin.Code, editSuperAdmin.Body.String())
	}

	grantReviewer := performJSON(router, http.MethodPatch, "/api/v1/admin/users/"+target.ID, `{"role":"reviewer"}`, superAdminToken)
	if grantReviewer.Code != http.StatusOK {
		t.Fatalf("expected super admin role update 200, got %d: %s", grantReviewer.Code, grantReviewer.Body.String())
	}
	var updated model.User
	if err := db.First(&updated, "id = ?", target.ID).Error; err != nil {
		t.Fatal(err)
	}
	if updated.Role != model.RoleReviewer {
		t.Fatalf("expected reviewer role, got %s", updated.Role)
	}
	if countOperationLogs(t, db, "user.update", "user", target.ID, superAdmin.ID) != 1 {
		t.Fatal("expected super admin user.update operation log")
	}

	freezeAdmin := performJSON(router, http.MethodPatch, "/api/v1/admin/users/"+admin.ID, `{"status":"frozen"}`, superAdminToken)
	if freezeAdmin.Code != http.StatusOK {
		t.Fatalf("expected super admin freeze admin 200, got %d: %s", freezeAdmin.Code, freezeAdmin.Body.String())
	}
	frozenAdminList := performJSON(router, http.MethodGet, "/api/v1/admin/users", "", adminToken)
	if frozenAdminList.Code != http.StatusForbidden {
		t.Fatalf("expected frozen admin list 403, got %d: %s", frozenAdminList.Code, frozenAdminList.Body.String())
	}
}
