package tests

import (
	"bytes"
	"encoding/base64"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type pointLedgerEntryResponse struct {
	ID        string `json:"id"`
	Amount    int64  `json:"amount"`
	Reason    string `json:"reason"`
	CreatedAt string `json:"created_at"`
}

type pointsResponse struct {
	Data struct {
		Balance    int64                      `json:"balance"`
		Entries    []pointLedgerEntryResponse `json:"entries"`
		NextCursor *string                    `json:"next_cursor"`
	} `json:"data"`
}

func TestConsolePointAdjustmentIsAuditableIdempotentAndVisibleToItsOwner(t *testing.T) {
	server, pool := newAccountPortfolioServerWithConsole(t)
	defer server.Close()
	defer pool.Close()

	const ownerID = "13131313-1313-4131-8131-131313131313"
	const operatorID = "34343434-3434-4434-8434-343434343434"
	const body = `{"user_id":"13131313-1313-4131-8131-131313131313","amount":120,"reason":"Manual correction after verified support case."}`

	portalAttempt := sendOwnerJSON(t, server.URL, http.MethodPost, ownerID, "/api/v1/console/points/adjustments", "nonce-points-portal-denied", "idem_points_portal_denied", body)
	if portalAttempt.StatusCode != http.StatusForbidden {
		t.Fatalf("Portal point adjustment status = %d, want 403: %s", portalAttempt.StatusCode, responseText(t, portalAttempt))
	}
	_ = responseText(t, portalAttempt)

	grant := sendConsoleJSON(t, server.URL, http.MethodPost, operatorID, "/api/v1/console/points/adjustments", "nonce-points-grant", "idem_points_grant", body)
	if grant.StatusCode != http.StatusOK {
		t.Fatalf("Console point grant status = %d: %s", grant.StatusCode, responseText(t, grant))
	}
	var granted struct {
		Data struct {
			Balance int64                    `json:"balance"`
			Entry   pointLedgerEntryResponse `json:"entry"`
		} `json:"data"`
	}
	decodeResponse(t, grant, &granted)
	if granted.Data.Balance != 120 || granted.Data.Entry.Amount != 120 || granted.Data.Entry.Reason != "Manual correction after verified support case." || granted.Data.Entry.ID == "" || granted.Data.Entry.CreatedAt == "" {
		t.Fatalf("Console point grant result = %+v, want real 120-point immutable entry", granted.Data)
	}

	retry := sendConsoleJSON(t, server.URL, http.MethodPost, operatorID, "/api/v1/console/points/adjustments", "nonce-points-grant-retry", "idem_points_grant", body)
	if retry.StatusCode != http.StatusOK {
		t.Fatalf("idempotent point grant retry status = %d: %s", retry.StatusCode, responseText(t, retry))
	}
	var retried struct {
		Data struct {
			Balance int64                    `json:"balance"`
			Entry   pointLedgerEntryResponse `json:"entry"`
		} `json:"data"`
	}
	decodeResponse(t, retry, &retried)
	if retried.Data.Balance != granted.Data.Balance || retried.Data.Entry.ID != granted.Data.Entry.ID {
		t.Fatalf("idempotent point grant retry = %+v, want original %+v", retried.Data, granted.Data)
	}

	points := sendOwnerJSON(t, server.URL, http.MethodGet, ownerID, "/api/v1/account/points?limit=1", "nonce-points-owner-read", "", "")
	if points.StatusCode != http.StatusOK {
		t.Fatalf("owner points status = %d: %s", points.StatusCode, responseText(t, points))
	}
	var ownerPoints pointsResponse
	decodeResponse(t, points, &ownerPoints)
	if ownerPoints.Data.Balance != 120 || len(ownerPoints.Data.Entries) != 1 || ownerPoints.Data.Entries[0].ID != granted.Data.Entry.ID || ownerPoints.Data.Entries[0].Amount != 120 {
		t.Fatalf("owner points = %+v, want durable balance and one ledger entry", ownerPoints.Data)
	}

	notifications := sendOwnerJSON(t, server.URL, http.MethodGet, ownerID, "/api/v1/account/notifications", "nonce-points-notifications", "", "")
	if notifications.StatusCode != http.StatusOK {
		t.Fatalf("owner points notifications status = %d: %s", notifications.StatusCode, responseText(t, notifications))
	}
	var notificationBody struct {
		Data struct {
			Notifications []struct {
				Kind string `json:"kind"`
			} `json:"notifications"`
		} `json:"data"`
	}
	decodeResponse(t, notifications, &notificationBody)
	if len(notificationBody.Data.Notifications) != 1 || notificationBody.Data.Notifications[0].Kind != "points_adjusted" {
		t.Fatalf("point adjustment notifications = %+v, want one durable points notification", notificationBody.Data.Notifications)
	}

	var ledgerEntries, auditEntries int
	var ledgerAmount int64
	var ledgerReason, auditOperator, auditTarget, auditKey, auditReason string
	var ledgerLinkedToAudit, notificationLinkedToLedger bool
	if err := pool.QueryRow(t.Context(), `
		SELECT
			(SELECT count(*) FROM account_portfolio_point_ledger WHERE user_id=$1),
			(SELECT count(*) FROM account_portfolio_point_adjustment_audits WHERE target_user_id=$1),
			l.amount,
			l.reason,
			a.operator_user_id::text,
			a.target_user_id::text,
			a.idempotency_key,
			a.reason,
			l.audit_id IS NOT NULL,
			(SELECT point_ledger_id IS NOT NULL FROM account_portfolio_notifications WHERE user_id=$1 AND kind='points_adjusted')
		FROM account_portfolio_point_ledger l
		JOIN account_portfolio_point_adjustment_audits a ON a.id=l.audit_id
		WHERE l.id=$2
	`, ownerID, granted.Data.Entry.ID).Scan(&ledgerEntries, &auditEntries, &ledgerAmount, &ledgerReason, &auditOperator, &auditTarget, &auditKey, &auditReason, &ledgerLinkedToAudit, &notificationLinkedToLedger); err != nil {
		t.Fatal(err)
	}
	if ledgerEntries != 1 || auditEntries != 1 || ledgerAmount != 120 || ledgerReason != "Manual correction after verified support case." || auditOperator != operatorID || auditTarget != ownerID || auditKey != "idem_points_grant" || auditReason != ledgerReason || !ledgerLinkedToAudit || !notificationLinkedToLedger {
		t.Fatalf("point adjustment durable facts = ledger/audit=%d/%d amount=%d reason=%q operator=%q target=%q key=%q audit_reason=%q links=%t/%t, want one operator-attributed immutable fact and notification", ledgerEntries, auditEntries, ledgerAmount, ledgerReason, auditOperator, auditTarget, auditKey, auditReason, ledgerLinkedToAudit, notificationLinkedToLedger)
	}
}

func TestConsolePointAdjustmentsPreserveJavaScriptSafeIntegerBoundaries(t *testing.T) {
	server, pool := newAccountPortfolioServerWithConsole(t)
	defer server.Close()
	defer pool.Close()

	const ownerID = "24242424-2424-4242-8242-242424242424"
	const operatorID = "35353535-3535-4353-8353-353535353535"
	const maxSafe = int64(9_007_199_254_740_991)
	for _, adjustment := range []struct {
		name, nonce, key, body  string
		wantBalance, wantAmount int64
	}{
		{
			name:        "maximum safe credit",
			nonce:       "nonce-points-max-safe-credit",
			key:         "idem_points_max_safe_credit",
			body:        `{"user_id":"24242424-2424-4242-8242-242424242424","amount":9007199254740991,"reason":"Maximum JavaScript-safe manual credit."}`,
			wantBalance: maxSafe,
			wantAmount:  maxSafe,
		},
		{
			name:        "minimum safe debit",
			nonce:       "nonce-points-min-safe-debit",
			key:         "idem_points_min_safe_debit",
			body:        `{"user_id":"24242424-2424-4242-8242-242424242424","amount":-9007199254740991,"reason":"Maximum JavaScript-safe manual debit."}`,
			wantBalance: 0,
			wantAmount:  -maxSafe,
		},
	} {
		t.Run(adjustment.name, func(t *testing.T) {
			response := sendConsoleJSON(t, server.URL, http.MethodPost, operatorID, "/api/v1/console/points/adjustments", adjustment.nonce, adjustment.key, adjustment.body)
			if response.StatusCode != http.StatusOK {
				t.Fatalf("%s status = %d: %s", adjustment.name, response.StatusCode, responseText(t, response))
			}
			var payload struct {
				Data struct {
					Balance int64                    `json:"balance"`
					Entry   pointLedgerEntryResponse `json:"entry"`
				} `json:"data"`
			}
			decodeResponse(t, response, &payload)
			if payload.Data.Balance != adjustment.wantBalance || payload.Data.Entry.Amount != adjustment.wantAmount {
				t.Fatalf("%s data balance/amount = %d/%d, want %d/%d", adjustment.name, payload.Data.Balance, payload.Data.Entry.Amount, adjustment.wantBalance, adjustment.wantAmount)
			}
		})
	}

	for _, rejected := range []struct {
		name, nonce, key, body string
	}{
		{
			name:  "positive value beyond JavaScript-safe range",
			nonce: "nonce-points-over-safe-credit",
			key:   "idem_points_over_safe_credit",
			body:  `{"user_id":"24242424-2424-4242-8242-242424242424","amount":9007199254740992,"reason":"Must not round through a browser."}`,
		},
		{
			name:  "negative value beyond JavaScript-safe range",
			nonce: "nonce-points-over-safe-debit",
			key:   "idem_points_over_safe_debit",
			body:  `{"user_id":"24242424-2424-4242-8242-242424242424","amount":-9007199254740992,"reason":"Must not round through a browser."}`,
		},
	} {
		t.Run(rejected.name, func(t *testing.T) {
			response := sendConsoleJSON(t, server.URL, http.MethodPost, operatorID, "/api/v1/console/points/adjustments", rejected.nonce, rejected.key, rejected.body)
			if response.StatusCode != http.StatusBadRequest {
				t.Fatalf("%s status = %d, want 400: %s", rejected.name, response.StatusCode, responseText(t, response))
			}
			_ = responseText(t, response)
		})
	}

	var balance int64
	var ledgers, audits, notifications int
	if err := pool.QueryRow(t.Context(), `
		SELECT
			(SELECT balance FROM account_portfolio_points WHERE user_id=$1),
			(SELECT count(*) FROM account_portfolio_point_ledger WHERE user_id=$1),
			(SELECT count(*) FROM account_portfolio_point_adjustment_audits WHERE target_user_id=$1),
			(SELECT count(*) FROM account_portfolio_notifications WHERE user_id=$1 AND kind='points_adjusted')
	`, ownerID).Scan(&balance, &ledgers, &audits, &notifications); err != nil {
		t.Fatal(err)
	}
	if balance != 0 || ledgers != 2 || audits != 2 || notifications != 2 {
		t.Fatalf("safe-range rejections left balance/ledger/audit/notification = %d/%d/%d/%d, want 0/2/2/2", balance, ledgers, audits, notifications)
	}
}

func TestConsolePointDebitRejectsInsufficientBalanceWithoutPartialFact(t *testing.T) {
	server, pool := newAccountPortfolioServerWithConsole(t)
	defer server.Close()
	defer pool.Close()

	const ownerID = "56565656-5656-4565-8565-565656565656"
	const operatorID = "78787878-7878-4787-8787-787878787878"
	grant := sendConsoleJSON(t, server.URL, http.MethodPost, operatorID, "/api/v1/console/points/adjustments", "nonce-points-debit-grant", "idem_points_debit_grant", `{"user_id":"56565656-5656-4565-8565-565656565656","amount":25,"reason":"Initial manual credit."}`)
	if grant.StatusCode != http.StatusOK {
		t.Fatalf("point debit setup grant status = %d: %s", grant.StatusCode, responseText(t, grant))
	}
	_ = responseText(t, grant)

	debit := sendConsoleJSON(t, server.URL, http.MethodPost, operatorID, "/api/v1/console/points/adjustments", "nonce-points-insufficient-debit", "idem_points_insufficient_debit", `{"user_id":"56565656-5656-4565-8565-565656565656","amount":-26,"reason":"This debit exceeds the available balance."}`)
	if debit.StatusCode != http.StatusConflict {
		t.Fatalf("insufficient point debit status = %d, want 409: %s", debit.StatusCode, responseText(t, debit))
	}
	_ = responseText(t, debit)

	var balance int64
	var ledgerEntries, auditEntries, notifications int
	if err := pool.QueryRow(t.Context(), `
		SELECT
			(SELECT balance FROM account_portfolio_points WHERE user_id=$1),
			(SELECT count(*) FROM account_portfolio_point_ledger WHERE user_id=$1),
			(SELECT count(*) FROM account_portfolio_point_adjustment_audits WHERE target_user_id=$1),
			(SELECT count(*) FROM account_portfolio_notifications WHERE user_id=$1 AND kind='points_adjusted')
	`, ownerID).Scan(&balance, &ledgerEntries, &auditEntries, &notifications); err != nil {
		t.Fatal(err)
	}
	if balance != 25 || ledgerEntries != 1 || auditEntries != 1 || notifications != 1 {
		t.Fatalf("insufficient debit durable state balance/ledger/audit/notifications = %d/%d/%d/%d, want 25/1/1/1", balance, ledgerEntries, auditEntries, notifications)
	}
}

func TestConsolePointDebitAgainstUninitializedTargetRollsBackAccountCreation(t *testing.T) {
	server, pool := newAccountPortfolioServerWithConsole(t)
	defer server.Close()
	defer pool.Close()

	const targetID = "67676767-6767-4767-8767-676767676767"
	const operatorID = "89898989-8989-4989-8989-898989898989"
	debit := sendConsoleJSON(t, server.URL, http.MethodPost, operatorID, "/api/v1/console/points/adjustments", "nonce-points-new-target-debit", "idem_points_new_target_debit", `{"user_id":"67676767-6767-4767-8767-676767676767","amount":-1,"reason":"Debit without available points."}`)
	if debit.StatusCode != http.StatusConflict {
		t.Fatalf("uninitialized target debit status = %d, want 409: %s", debit.StatusCode, responseText(t, debit))
	}
	_ = responseText(t, debit)

	var accounts, projections, ledgers, audits, notifications int
	if err := pool.QueryRow(t.Context(), `
		SELECT
			(SELECT count(*) FROM account_portfolio_accounts WHERE user_id=$1),
			(SELECT count(*) FROM account_portfolio_points WHERE user_id=$1),
			(SELECT count(*) FROM account_portfolio_point_ledger WHERE user_id=$1),
			(SELECT count(*) FROM account_portfolio_point_adjustment_audits WHERE target_user_id=$1),
			(SELECT count(*) FROM account_portfolio_notifications WHERE user_id=$1)
	`, targetID).Scan(&accounts, &projections, &ledgers, &audits, &notifications); err != nil {
		t.Fatal(err)
	}
	if accounts != 0 || projections != 0 || ledgers != 0 || audits != 0 || notifications != 0 {
		t.Fatalf("uninitialized insufficient debit left account/projection/ledger/audit/notification = %d/%d/%d/%d/%d, want all zero", accounts, projections, ledgers, audits, notifications)
	}
}

func TestPointLedgerPaginationIsSourceOfTruthAndFactsCannotBeMutated(t *testing.T) {
	server, pool := newAccountPortfolioServerWithConsole(t)
	defer server.Close()
	defer pool.Close()

	const ownerID = "90909090-9090-4090-8090-909090909090"
	const operatorID = "abababab-abab-4bab-8bab-abababababab"
	for index := 0; index < 3; index++ {
		response := sendConsoleJSON(t, server.URL, http.MethodPost, operatorID, "/api/v1/console/points/adjustments", "nonce-points-page-"+strconv.Itoa(index), "idem_points_page_"+strconv.Itoa(index), `{"user_id":"90909090-9090-4090-8090-909090909090","amount":1,"reason":"Paginated ledger fixture."}`)
		if response.StatusCode != http.StatusOK {
			t.Fatalf("pagination grant %d status = %d: %s", index, response.StatusCode, responseText(t, response))
		}
		_ = responseText(t, response)
	}

	first := sendOwnerJSON(t, server.URL, http.MethodGet, ownerID, "/api/v1/account/points?limit=2", "nonce-points-page-first", "", "")
	if first.StatusCode != http.StatusOK {
		t.Fatalf("first points page status = %d: %s", first.StatusCode, responseText(t, first))
	}
	var firstPage pointsResponse
	decodeResponse(t, first, &firstPage)
	if firstPage.Data.Balance != 3 || len(firstPage.Data.Entries) != 2 || firstPage.Data.NextCursor == nil || *firstPage.Data.NextCursor == "" {
		t.Fatalf("first points page = %+v, want balance 3, two entries, and a continuation cursor", firstPage.Data)
	}

	second := sendOwnerJSON(t, server.URL, http.MethodGet, ownerID, "/api/v1/account/points?limit=2&cursor="+*firstPage.Data.NextCursor, "nonce-points-page-second", "", "")
	if second.StatusCode != http.StatusOK {
		t.Fatalf("second points page status = %d: %s", second.StatusCode, responseText(t, second))
	}
	var secondPage pointsResponse
	decodeResponse(t, second, &secondPage)
	if len(secondPage.Data.Entries) != 1 || secondPage.Data.Entries[0].ID == firstPage.Data.Entries[0].ID || secondPage.Data.Entries[0].ID == firstPage.Data.Entries[1].ID || secondPage.Data.NextCursor != nil {
		t.Fatalf("second points page = %+v, want one non-duplicated final entry", secondPage.Data)
	}

	if _, err := pool.Exec(t.Context(), `UPDATE account_portfolio_points SET balance=999 WHERE user_id=$1`, ownerID); err != nil {
		t.Fatal(err)
	}
	truth := sendOwnerJSON(t, server.URL, http.MethodGet, ownerID, "/api/v1/account/points", "nonce-points-ledger-truth", "", "")
	if truth.StatusCode != http.StatusOK {
		t.Fatalf("ledger source-of-truth status = %d: %s", truth.StatusCode, responseText(t, truth))
	}
	var truthPage pointsResponse
	decodeResponse(t, truth, &truthPage)
	if truthPage.Data.Balance != 3 {
		t.Fatalf("ledger-derived balance = %d, want 3 rather than mutable projection", truthPage.Data.Balance)
	}

	if _, err := pool.Exec(t.Context(), `UPDATE account_portfolio_point_ledger SET reason='rewritten' WHERE id=$1`, firstPage.Data.Entries[0].ID); err == nil {
		t.Fatal("immutable point ledger accepted an UPDATE")
	}
	if _, err := pool.Exec(t.Context(), `DELETE FROM account_portfolio_point_adjustment_audits WHERE target_user_id=$1`, ownerID); err == nil {
		t.Fatal("immutable point adjustment audit accepted a DELETE")
	}
}

func TestPointLedgerCursorIsOpaqueBoundToItsOwnerAndExpires(t *testing.T) {
	clock := time.Date(2026, time.July, 28, 12, 0, 0, 0, time.UTC)
	server, pool := newAccountPortfolioServerWithConsoleAt(t, func() time.Time { return clock })
	defer server.Close()
	defer pool.Close()

	const ownerID = "e1e1e1e1-e1e1-41e1-81e1-e1e1e1e1e1e1"
	const otherOwnerID = "f2f2f2f2-f2f2-42f2-82f2-f2f2f2f2f2f2"
	const operatorID = "a3a3a3a3-a3a3-43a3-83a3-a3a3a3a3a3a3"
	for index := 0; index < 3; index++ {
		response := sendConsoleJSONAt(t, server.URL, http.MethodPost, operatorID, "/api/v1/console/points/adjustments", "nonce-opaque-cursor-"+strconv.Itoa(index), "idem_opaque_cursor_"+strconv.Itoa(index), `{"user_id":"e1e1e1e1-e1e1-41e1-81e1-e1e1e1e1e1e1","amount":1,"reason":"Opaque cursor fixture."}`, clock)
		if response.StatusCode != http.StatusOK {
			t.Fatalf("cursor fixture adjustment %d status = %d: %s", index, response.StatusCode, responseText(t, response))
		}
		_ = responseText(t, response)
	}

	first := sendOwnerJSONAt(t, server.URL, http.MethodGet, ownerID, "/api/v1/account/points?limit=2", "nonce-opaque-cursor-first", "", "", clock)
	if first.StatusCode != http.StatusOK {
		t.Fatalf("first opaque-cursor page status = %d: %s", first.StatusCode, responseText(t, first))
	}
	var firstPage pointsResponse
	decodeResponse(t, first, &firstPage)
	if len(firstPage.Data.Entries) != 2 || firstPage.Data.NextCursor == nil || *firstPage.Data.NextCursor == "" {
		t.Fatalf("first opaque-cursor page = %+v, want two facts and a continuation", firstPage.Data)
	}
	cursor := *firstPage.Data.NextCursor
	if !strings.HasPrefix(cursor, "plc1.") {
		t.Fatalf("point cursor = %q, want versioned opaque plc1 token", cursor)
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(cursor, "plc1."))
	if err != nil {
		t.Fatalf("opaque point cursor was not base64url ciphertext: %v", err)
	}
	if bytes.Contains(raw, []byte(firstPage.Data.Entries[1].ID)) || bytes.Contains(raw, []byte(firstPage.Data.Entries[1].CreatedAt)) || bytes.Contains(raw, []byte(ownerID)) {
		t.Fatalf("opaque point cursor leaks owner or sort boundary: %q", raw)
	}

	second := sendOwnerJSONAt(t, server.URL, http.MethodGet, ownerID, "/api/v1/account/points?limit=2&cursor="+cursor, "nonce-opaque-cursor-second", "", "", clock)
	if second.StatusCode != http.StatusOK {
		t.Fatalf("legitimate opaque cursor status = %d, want 200: %s", second.StatusCode, responseText(t, second))
	}
	var secondPage pointsResponse
	decodeResponse(t, second, &secondPage)
	if len(secondPage.Data.Entries) != 1 || secondPage.Data.NextCursor != nil {
		t.Fatalf("legitimate opaque cursor page = %+v, want final one-fact page", secondPage.Data)
	}

	tamperedCharacter := "A"
	if cursor[len("plc1.")] == 'A' {
		tamperedCharacter = "B"
	}
	tampered := cursor[:len("plc1.")] + tamperedCharacter + cursor[len("plc1.")+1:]
	for _, test := range []struct {
		name, actorID, candidate string
	}{
		{name: "cross-user", actorID: otherOwnerID, candidate: cursor},
		{name: "tampered", actorID: ownerID, candidate: tampered},
		{name: "legacy", actorID: ownerID, candidate: base64.RawURLEncoding.EncodeToString([]byte(`{"created_at":"2026-07-28T12:00:00Z","id":"11111111-1111-4111-8111-111111111111"}`))},
	} {
		response := sendOwnerJSONAt(t, server.URL, http.MethodGet, test.actorID, "/api/v1/account/points?limit=2&cursor="+test.candidate, "nonce-opaque-cursor-"+test.name, "", "", clock)
		body := responseText(t, response)
		if response.StatusCode != http.StatusBadRequest || !strings.Contains(body, `"code":"INVALID_REQUEST"`) {
			t.Fatalf("%s point cursor status/body = %d/%s, want uniform INVALID_REQUEST 400", test.name, response.StatusCode, body)
		}
	}

	clock = clock.Add(15 * time.Minute)
	expired := sendOwnerJSONAt(t, server.URL, http.MethodGet, ownerID, "/api/v1/account/points?limit=2&cursor="+cursor, "nonce-opaque-cursor-expired", "", "", clock)
	body := responseText(t, expired)
	if expired.StatusCode != http.StatusBadRequest || !strings.Contains(body, `"code":"INVALID_REQUEST"`) {
		t.Fatalf("expired point cursor status/body = %d/%s, want uniform INVALID_REQUEST 400", expired.StatusCode, body)
	}
}

func TestPointLedgerRejectsInvalidPaginationQueries(t *testing.T) {
	server, pool := newAccountPortfolioServerWithConsole(t)
	defer server.Close()
	defer pool.Close()

	const ownerID = "bcbcbcbc-bcbc-4cbc-8cbc-bcbcbcbcbcbc"
	for index, path := range []string{
		"/api/v1/account/points?limit=0",
		"/api/v1/account/points?limit=2&limit=3",
		"/api/v1/account/points?cursor=",
		"/api/v1/account/points?unknown=1",
	} {
		response := sendOwnerJSON(t, server.URL, http.MethodGet, ownerID, path, "nonce-points-invalid-page-"+strconv.Itoa(index), "", "")
		if response.StatusCode != http.StatusBadRequest {
			t.Fatalf("invalid point page %q status = %d, want 400: %s", path, response.StatusCode, responseText(t, response))
		}
		_ = responseText(t, response)
	}
}

func TestConcurrentPointAdjustmentsKeepBalanceAndFactCountsConsistent(t *testing.T) {
	server, pool := newAccountPortfolioServerWithConsole(t)
	defer server.Close()
	defer pool.Close()

	const ownerID = "cdcdcdcd-cdcd-4dcd-8dcd-cdcdcdcdcdcd"
	const operatorID = "efefefef-efef-4fef-8fef-efefefefefef"
	const count = 8
	statuses := make(chan int, count)
	var group sync.WaitGroup
	for index := 0; index < count; index++ {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			suffix := strconv.Itoa(index)
			response := sendConsoleJSON(t, server.URL, http.MethodPost, operatorID, "/api/v1/console/points/adjustments", "nonce-points-race-"+suffix, "idem_points_race_"+suffix, `{"user_id":"cdcdcdcd-cdcd-4dcd-8dcd-cdcdcdcdcdcd","amount":1,"reason":"Concurrent operator credit."}`)
			statuses <- response.StatusCode
			_ = responseText(t, response)
		}(index)
	}
	group.Wait()
	close(statuses)
	for status := range statuses {
		if status != http.StatusOK {
			t.Fatalf("concurrent point adjustment status = %d, want 200", status)
		}
	}

	var balance int64
	var ledgerEntries, auditEntries, notifications int
	if err := pool.QueryRow(t.Context(), `
		SELECT
			(SELECT balance FROM account_portfolio_points WHERE user_id=$1),
			(SELECT count(*) FROM account_portfolio_point_ledger WHERE user_id=$1),
			(SELECT count(*) FROM account_portfolio_point_adjustment_audits WHERE target_user_id=$1),
			(SELECT count(*) FROM account_portfolio_notifications WHERE user_id=$1 AND kind='points_adjusted')
	`, ownerID).Scan(&balance, &ledgerEntries, &auditEntries, &notifications); err != nil {
		t.Fatal(err)
	}
	if balance != count || ledgerEntries != count || auditEntries != count || notifications != count {
		t.Fatalf("concurrent point facts balance/ledger/audit/notifications = %d/%d/%d/%d, want %d/%d/%d/%d", balance, ledgerEntries, auditEntries, notifications, count, count, count, count)
	}
}

func TestConcurrentPointDebitsCannotOverspendTheLedger(t *testing.T) {
	server, pool := newAccountPortfolioServerWithConsole(t)
	defer server.Close()
	defer pool.Close()

	const ownerID = "dededede-dede-4ede-8ede-dededededede"
	const operatorID = "fefefefe-fefe-4efe-8efe-fefefefefefe"
	credit := sendConsoleJSON(t, server.URL, http.MethodPost, operatorID, "/api/v1/console/points/adjustments", "nonce-points-debit-race-credit", "idem_points_debit_race_credit", `{"user_id":"dededede-dede-4ede-8ede-dededededede","amount":10,"reason":"Concurrent debit fixture credit."}`)
	if credit.StatusCode != http.StatusOK {
		t.Fatalf("concurrent debit setup credit status = %d: %s", credit.StatusCode, responseText(t, credit))
	}
	_ = responseText(t, credit)

	statuses := make(chan int, 2)
	var group sync.WaitGroup
	for index := 0; index < 2; index++ {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			suffix := strconv.Itoa(index)
			response := sendConsoleJSON(t, server.URL, http.MethodPost, operatorID, "/api/v1/console/points/adjustments", "nonce-points-debit-race-"+suffix, "idem_points_debit_race_"+suffix, `{"user_id":"dededede-dede-4ede-8ede-dededededede","amount":-7,"reason":"Concurrent debit fixture."}`)
			statuses <- response.StatusCode
			_ = responseText(t, response)
		}(index)
	}
	group.Wait()
	close(statuses)
	var succeeded, conflicted int
	for status := range statuses {
		switch status {
		case http.StatusOK:
			succeeded++
		case http.StatusConflict:
			conflicted++
		default:
			t.Fatalf("concurrent point debit status = %d, want 200 or 409", status)
		}
	}
	if succeeded != 1 || conflicted != 1 {
		t.Fatalf("concurrent point debit outcomes success/conflict = %d/%d, want 1/1", succeeded, conflicted)
	}

	var balance int64
	var ledgers, audits, notifications int
	if err := pool.QueryRow(t.Context(), `
		SELECT
			(SELECT balance FROM account_portfolio_points WHERE user_id=$1),
			(SELECT count(*) FROM account_portfolio_point_ledger WHERE user_id=$1),
			(SELECT count(*) FROM account_portfolio_point_adjustment_audits WHERE target_user_id=$1),
			(SELECT count(*) FROM account_portfolio_notifications WHERE user_id=$1 AND kind='points_adjusted')
	`, ownerID).Scan(&balance, &ledgers, &audits, &notifications); err != nil {
		t.Fatal(err)
	}
	if balance != 3 || ledgers != 2 || audits != 2 || notifications != 2 {
		t.Fatalf("concurrent debit state balance/ledger/audit/notifications = %d/%d/%d/%d, want 3/2/2/2", balance, ledgers, audits, notifications)
	}
}
