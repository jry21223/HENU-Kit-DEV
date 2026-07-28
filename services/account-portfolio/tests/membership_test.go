package tests

import (
	"net/http"
	"strconv"
	"sync"
	"testing"
)

func TestConsoleGrantMakesLifetimeMembershipAuditableAndVisibleToOwner(t *testing.T) {
	server, pool := newAccountPortfolioServerWithConsole(t)
	defer server.Close()
	defer pool.Close()

	const ownerID = "12121212-1212-4121-8121-121212121212"
	const operatorID = "34343434-3434-4434-8434-343434343434"

	seed := sendOwnerJSON(t, server.URL, http.MethodGet, ownerID, "/api/v1/account/summary", "nonce-membership-seed", "", "")
	if seed.StatusCode != http.StatusOK {
		t.Fatalf("seed Account Portfolio membership status = %d: %s", seed.StatusCode, responseText(t, seed))
	}
	_ = responseText(t, seed)

	grant := sendConsoleJSON(t, server.URL, http.MethodPost, operatorID, "/api/v1/console/memberships/"+ownerID+"/grants", "nonce-membership-grant", "idem_membership_grant", `{"reason":"Manual lifetime entitlement for verified support resolution.","expected_version":1}`)
	if grant.StatusCode != http.StatusOK {
		t.Fatalf("grant lifetime membership status = %d: %s", grant.StatusCode, responseText(t, grant))
	}
	var granted struct {
		Data struct {
			Membership struct {
				Plan     string `json:"plan"`
				Lifetime bool   `json:"lifetime"`
				Version  int    `json:"version"`
			} `json:"membership"`
		} `json:"data"`
	}
	decodeResponse(t, grant, &granted)
	if granted.Data.Membership.Plan != "lifetime" || !granted.Data.Membership.Lifetime || granted.Data.Membership.Version != 2 {
		t.Fatalf("granted membership = %+v, want durable lifetime at version 2", granted.Data.Membership)
	}

	retry := sendConsoleJSON(t, server.URL, http.MethodPost, operatorID, "/api/v1/console/memberships/"+ownerID+"/grants", "nonce-membership-grant-retry", "idem_membership_grant", `{"reason":"Manual lifetime entitlement for verified support resolution.","expected_version":1}`)
	if retry.StatusCode != http.StatusOK {
		t.Fatalf("idempotent membership grant retry status = %d: %s", retry.StatusCode, responseText(t, retry))
	}
	_ = responseText(t, retry)

	ownerMembership := sendOwnerJSON(t, server.URL, http.MethodGet, ownerID, "/api/v1/account/membership", "nonce-membership-owner-read", "", "")
	if ownerMembership.StatusCode != http.StatusOK {
		t.Fatalf("owner membership status = %d: %s", ownerMembership.StatusCode, responseText(t, ownerMembership))
	}
	var ownerMembershipBody struct {
		Data struct {
			Plan     string `json:"plan"`
			Lifetime bool   `json:"lifetime"`
		} `json:"data"`
	}
	decodeResponse(t, ownerMembership, &ownerMembershipBody)
	if ownerMembershipBody.Data.Plan != "lifetime" || !ownerMembershipBody.Data.Lifetime {
		t.Fatalf("owner membership = %+v, want real lifetime entitlement", ownerMembershipBody.Data)
	}

	notifications := sendOwnerJSON(t, server.URL, http.MethodGet, ownerID, "/api/v1/account/notifications", "nonce-membership-notifications", "", "")
	if notifications.StatusCode != http.StatusOK {
		t.Fatalf("owner membership notifications status = %d: %s", notifications.StatusCode, responseText(t, notifications))
	}
	var notificationBody struct {
		Data struct {
			Notifications []struct {
				Kind string `json:"kind"`
			} `json:"notifications"`
		} `json:"data"`
	}
	decodeResponse(t, notifications, &notificationBody)
	if len(notificationBody.Data.Notifications) != 1 || notificationBody.Data.Notifications[0].Kind != "membership_lifetime_granted" {
		t.Fatalf("membership notifications = %+v, want one durable grant notification", notificationBody.Data.Notifications)
	}

	var events int
	var kind, fromPlan, toPlan, source, actorUserID, reason string
	if err := pool.QueryRow(t.Context(), `
		SELECT
			count(*),
			coalesce(min(kind), ''),
			coalesce(min(from_plan), ''),
			coalesce(min(to_plan), ''),
			coalesce(min(source), ''),
			coalesce(min(actor_user_id::text), ''),
			coalesce(min(reason), '')
		FROM account_portfolio_membership_events
		WHERE user_id=$1
	`, ownerID).Scan(&events, &kind, &fromPlan, &toPlan, &source, &actorUserID, &reason); err != nil {
		t.Fatal(err)
	}
	if events != 1 {
		t.Fatalf("membership audit events = %d, want 1 after idempotent retry", events)
	}
	if kind != "grant" || fromPlan != "free" || toPlan != "lifetime" || source != "operator" || actorUserID != operatorID || reason != "Manual lifetime entitlement for verified support resolution." {
		t.Fatalf("membership audit event = kind=%q transition=%q->%q source=%q actor=%q reason=%q, want operator-attributed grant fact", kind, fromPlan, toPlan, source, actorUserID, reason)
	}
}

func TestConsoleRevocationRequiresLifetimeStateAndDoesNotRepeatSideEffects(t *testing.T) {
	server, pool := newAccountPortfolioServerWithConsole(t)
	defer server.Close()
	defer pool.Close()

	const ownerID = "56565656-5656-4565-8565-565656565656"
	const operatorID = "78787878-7878-4787-8787-787878787878"
	seed := sendOwnerJSON(t, server.URL, http.MethodGet, ownerID, "/api/v1/account/summary", "nonce-membership-revoke-seed", "", "")
	if seed.StatusCode != http.StatusOK {
		t.Fatalf("seed membership for revoke status = %d: %s", seed.StatusCode, responseText(t, seed))
	}
	_ = responseText(t, seed)

	grant := sendConsoleJSON(t, server.URL, http.MethodPost, operatorID, "/api/v1/console/memberships/"+ownerID+"/grants", "nonce-membership-revoke-grant", "idem_membership_revoke_grant", `{"reason":"Manual grant before revocation test.","expected_version":1}`)
	if grant.StatusCode != http.StatusOK {
		t.Fatalf("grant before revoke status = %d: %s", grant.StatusCode, responseText(t, grant))
	}
	_ = responseText(t, grant)

	revokeBody := `{"reason":"Manual revocation after entitlement review.","expected_version":2}`
	revoke := sendConsoleJSON(t, server.URL, http.MethodPost, operatorID, "/api/v1/console/memberships/"+ownerID+"/revocations", "nonce-membership-revoke", "idem_membership_revoke", revokeBody)
	if revoke.StatusCode != http.StatusOK {
		t.Fatalf("revoke lifetime membership status = %d: %s", revoke.StatusCode, responseText(t, revoke))
	}
	var revoked struct {
		Data struct {
			Membership struct {
				Plan     string `json:"plan"`
				Lifetime bool   `json:"lifetime"`
				Version  int    `json:"version"`
			} `json:"membership"`
		} `json:"data"`
	}
	decodeResponse(t, revoke, &revoked)
	if revoked.Data.Membership.Plan != "free" || revoked.Data.Membership.Lifetime || revoked.Data.Membership.Version != 3 {
		t.Fatalf("revoked membership = %+v, want free version 3", revoked.Data.Membership)
	}

	retry := sendConsoleJSON(t, server.URL, http.MethodPost, operatorID, "/api/v1/console/memberships/"+ownerID+"/revocations", "nonce-membership-revoke-retry", "idem_membership_revoke", revokeBody)
	if retry.StatusCode != http.StatusOK {
		t.Fatalf("idempotent membership revocation retry status = %d: %s", retry.StatusCode, responseText(t, retry))
	}
	_ = responseText(t, retry)

	again := sendConsoleJSON(t, server.URL, http.MethodPost, operatorID, "/api/v1/console/memberships/"+ownerID+"/revocations", "nonce-membership-revoke-again", "idem_membership_revoke_again", `{"reason":"This duplicate must not create another event.","expected_version":3}`)
	if again.StatusCode != http.StatusConflict {
		t.Fatalf("second revocation status = %d, want 409: %s", again.StatusCode, responseText(t, again))
	}
	_ = responseText(t, again)

	var plan string
	var version, events, notifications int
	if err := pool.QueryRow(t.Context(), `
		SELECT
			(SELECT plan FROM account_portfolio_memberships WHERE user_id=$1),
			(SELECT version FROM account_portfolio_memberships WHERE user_id=$1),
			(SELECT count(*) FROM account_portfolio_membership_events WHERE user_id=$1),
			(SELECT count(*) FROM account_portfolio_notifications WHERE user_id=$1)
	`, ownerID).Scan(&plan, &version, &events, &notifications); err != nil {
		t.Fatal(err)
	}
	if plan != "free" || version != 3 || events != 2 || notifications != 2 {
		t.Fatalf("revocation facts plan/version/events/notifications = %s/%d/%d/%d, want free/3/2/2", plan, version, events, notifications)
	}

	var kind, fromPlan, toPlan, source, actorUserID, reason, notificationKind string
	var notificationReferencesEvent bool
	if err := pool.QueryRow(t.Context(), `
		SELECT kind, from_plan, to_plan, source, actor_user_id::text, reason
		FROM account_portfolio_membership_events
		WHERE user_id=$1 AND kind='revoke'
	`, ownerID).Scan(&kind, &fromPlan, &toPlan, &source, &actorUserID, &reason); err != nil {
		t.Fatal(err)
	}
	if kind != "revoke" || fromPlan != "lifetime" || toPlan != "free" || source != "operator" || actorUserID != operatorID || reason != "Manual revocation after entitlement review." {
		t.Fatalf("membership revocation audit event = kind=%q transition=%q->%q source=%q actor=%q reason=%q, want operator-attributed revoke fact", kind, fromPlan, toPlan, source, actorUserID, reason)
	}
	if err := pool.QueryRow(t.Context(), `
		SELECT kind, membership_event_id IS NOT NULL
		FROM account_portfolio_notifications
		WHERE user_id=$1 AND kind='membership_lifetime_revoked'
	`, ownerID).Scan(&notificationKind, &notificationReferencesEvent); err != nil {
		t.Fatal(err)
	}
	if notificationKind != "membership_lifetime_revoked" || !notificationReferencesEvent {
		t.Fatalf("membership revocation notification = kind=%q references_event=%t, want durable revoke notification linked to audit", notificationKind, notificationReferencesEvent)
	}
}

func TestConsoleMembershipRejectsNonConsoleAndNeverInitializesAnUnknownTarget(t *testing.T) {
	server, pool := newAccountPortfolioServerWithConsole(t)
	defer server.Close()
	defer pool.Close()

	const unknownUserID = "90909090-9090-4090-8090-909090909090"
	const operatorID = "abababab-abab-4bab-8bab-abababababab"

	portalAttempt := sendOwnerJSON(t, server.URL, http.MethodPost, unknownUserID, "/api/v1/console/memberships/"+unknownUserID+"/grants", "nonce-membership-portal-denied", "idem_membership_portal_denied", `{"reason":"A Portal caller must not grant membership.","expected_version":1}`)
	if portalAttempt.StatusCode != http.StatusForbidden {
		t.Fatalf("Portal membership mutation status = %d, want 403: %s", portalAttempt.StatusCode, responseText(t, portalAttempt))
	}
	_ = responseText(t, portalAttempt)

	lookup := sendConsoleJSON(t, server.URL, http.MethodGet, operatorID, "/api/v1/console/memberships/"+unknownUserID, "nonce-membership-unknown-lookup", "", "")
	if lookup.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown target membership lookup status = %d, want 404: %s", lookup.StatusCode, responseText(t, lookup))
	}
	_ = responseText(t, lookup)

	var accounts, memberships int
	if err := pool.QueryRow(t.Context(), `
		SELECT
			(SELECT count(*) FROM account_portfolio_accounts WHERE user_id=$1),
			(SELECT count(*) FROM account_portfolio_memberships WHERE user_id=$1)
	`, unknownUserID).Scan(&accounts, &memberships); err != nil {
		t.Fatal(err)
	}
	if accounts != 0 || memberships != 0 {
		t.Fatalf("unknown target created Account Portfolio state accounts/memberships = %d/%d, want 0/0", accounts, memberships)
	}
}

func TestConcurrentMembershipGrantCreatesOneEventAndOneNotification(t *testing.T) {
	server, pool := newAccountPortfolioServerWithConsole(t)
	defer server.Close()
	defer pool.Close()

	const ownerID = "cdcdcdcd-cdcd-4dcd-8dcd-cdcdcdcdcdcd"
	const operatorID = "efefefef-efef-4fef-8fef-efefefefefef"
	seed := sendOwnerJSON(t, server.URL, http.MethodGet, ownerID, "/api/v1/account/summary", "nonce-membership-race-seed", "", "")
	if seed.StatusCode != http.StatusOK {
		t.Fatalf("seed membership race status = %d: %s", seed.StatusCode, responseText(t, seed))
	}
	_ = responseText(t, seed)

	statuses := make(chan int, 2)
	var group sync.WaitGroup
	for index := 0; index < cap(statuses); index++ {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			suffix := strconv.Itoa(index)
			response := sendConsoleJSON(t, server.URL, http.MethodPost, operatorID, "/api/v1/console/memberships/"+ownerID+"/grants", "nonce-membership-race-"+suffix, "idem_membership_race_"+suffix, `{"reason":"Concurrent grant test.","expected_version":1}`)
			statuses <- response.StatusCode
			_ = responseText(t, response)
		}(index)
	}
	group.Wait()
	close(statuses)

	var succeeded, conflicted int
	for status := range statuses {
		if status == http.StatusOK {
			succeeded++
		}
		if status == http.StatusConflict {
			conflicted++
		}
	}
	if succeeded != 1 || conflicted != 1 {
		t.Fatalf("concurrent membership grant statuses = success/conflict %d/%d, want 1/1", succeeded, conflicted)
	}

	var plan string
	var version, events, notifications int
	if err := pool.QueryRow(t.Context(), `
		SELECT
			(SELECT plan FROM account_portfolio_memberships WHERE user_id=$1),
			(SELECT version FROM account_portfolio_memberships WHERE user_id=$1),
			(SELECT count(*) FROM account_portfolio_membership_events WHERE user_id=$1),
			(SELECT count(*) FROM account_portfolio_notifications WHERE user_id=$1)
	`, ownerID).Scan(&plan, &version, &events, &notifications); err != nil {
		t.Fatal(err)
	}
	if plan != "lifetime" || version != 2 || events != 1 || notifications != 1 {
		t.Fatalf("concurrent membership facts plan/version/events/notifications = %s/%d/%d/%d, want lifetime/2/1/1", plan, version, events, notifications)
	}
}
