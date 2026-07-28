package tests

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestSupportTicketCreationIsDurableOwnerScopedAndIdempotent(t *testing.T) {
	server, pool := newAccountPortfolioServer(t)
	defer server.Close()
	defer pool.Close()

	const ownerID = "77777777-7777-4777-8777-777777777777"
	const otherUserID = "88888888-8888-4888-8888-888888888888"
	const createBody = `{"title":"Cannot see my account history","category":"account","body":"Please help me find the missing history."}`

	first := sendOwnerJSON(t, server.URL, http.MethodPost, ownerID, "/api/v1/account/tickets", "nonce-ticket-create-1", "idem_ticket_create_1", createBody)
	if first.StatusCode != http.StatusCreated {
		t.Fatalf("create ticket status = %d: %s", first.StatusCode, responseText(t, first))
	}
	var created struct {
		Data struct {
			Ticket struct {
				ID        string `json:"id"`
				Reference string `json:"reference"`
				Status    string `json:"status"`
				Version   int    `json:"version"`
			} `json:"ticket"`
		} `json:"data"`
	}
	decodeResponse(t, first, &created)
	if created.Data.Ticket.ID == "" || created.Data.Ticket.Reference != "HKT-"+created.Data.Ticket.ID || created.Data.Ticket.Status != "open" || created.Data.Ticket.Version != 1 {
		t.Fatalf("created ticket = %+v, want stable open ticket", created.Data.Ticket)
	}

	retry := sendOwnerJSON(t, server.URL, http.MethodPost, ownerID, "/api/v1/account/tickets", "nonce-ticket-create-2", "idem_ticket_create_1", createBody)
	if retry.StatusCode != http.StatusCreated {
		t.Fatalf("idempotent ticket retry status = %d: %s", retry.StatusCode, responseText(t, retry))
	}
	var retried struct {
		Data struct {
			Ticket struct {
				ID string `json:"id"`
			} `json:"ticket"`
		} `json:"data"`
	}
	decodeResponse(t, retry, &retried)
	if retried.Data.Ticket.ID != created.Data.Ticket.ID {
		t.Fatalf("idempotent ticket retry id = %q, want %q", retried.Data.Ticket.ID, created.Data.Ticket.ID)
	}

	conflict := sendOwnerJSON(t, server.URL, http.MethodPost, ownerID, "/api/v1/account/tickets", "nonce-ticket-create-3", "idem_ticket_create_1", `{"title":"Different request","category":"account","body":"This must not create another ticket."}`)
	if conflict.StatusCode != http.StatusConflict {
		t.Fatalf("reused ticket idempotency key status = %d: %s", conflict.StatusCode, responseText(t, conflict))
	}

	detail := sendOwnerJSON(t, server.URL, http.MethodGet, ownerID, "/api/v1/account/tickets/"+created.Data.Ticket.ID, "nonce-ticket-detail", "", "")
	if detail.StatusCode != http.StatusOK {
		t.Fatalf("owner ticket detail status = %d: %s", detail.StatusCode, responseText(t, detail))
	}
	var details struct {
		Data struct {
			Ticket struct {
				Reference string `json:"reference"`
			} `json:"ticket"`
			Messages []struct {
				AuthorKind string `json:"author_kind"`
				Body       string `json:"body"`
			} `json:"messages"`
		} `json:"data"`
	}
	decodeResponse(t, detail, &details)
	if details.Data.Ticket.Reference != created.Data.Ticket.Reference || len(details.Data.Messages) != 1 || details.Data.Messages[0].AuthorKind != "user" || details.Data.Messages[0].Body != "Please help me find the missing history." {
		t.Fatalf("owner ticket detail = %+v, want persisted initial message", details.Data)
	}

	otherDetail := sendOwnerJSON(t, server.URL, http.MethodGet, otherUserID, "/api/v1/account/tickets/"+created.Data.Ticket.ID, "nonce-ticket-other-user", "", "")
	if otherDetail.StatusCode != http.StatusNotFound {
		t.Fatalf("other user ticket detail status = %d, want 404: %s", otherDetail.StatusCode, responseText(t, otherDetail))
	}
}

func TestTicketFollowUpRequiresCurrentVersionAndReopensResolvedTicket(t *testing.T) {
	server, pool := newAccountPortfolioServer(t)
	defer server.Close()
	defer pool.Close()

	const ownerID = "99999999-9999-4999-8999-999999999999"
	created := sendOwnerJSON(t, server.URL, http.MethodPost, ownerID, "/api/v1/account/tickets", "nonce-follow-up-create", "idem_ticket_followup_create", `{"title":"Need a follow-up","category":"account","body":"Initial message."}`)
	if created.StatusCode != http.StatusCreated {
		t.Fatalf("create ticket status = %d: %s", created.StatusCode, responseText(t, created))
	}
	var createdBody struct {
		Data struct {
			Ticket struct {
				ID string `json:"id"`
			} `json:"ticket"`
		} `json:"data"`
	}
	decodeResponse(t, created, &createdBody)
	if _, err := pool.Exec(t.Context(), `UPDATE account_portfolio_tickets SET status='resolved', version=2 WHERE id=$1`, createdBody.Data.Ticket.ID); err != nil {
		t.Fatal(err)
	}

	stale := sendOwnerJSON(t, server.URL, http.MethodPost, ownerID, "/api/v1/account/tickets/"+createdBody.Data.Ticket.ID+"/follow-ups", "nonce-follow-up-stale", "idem_ticket_followup_stale", `{"body":"Please reopen this.","expected_version":1}`)
	if stale.StatusCode != http.StatusConflict {
		t.Fatalf("stale follow-up status = %d, want 409: %s", stale.StatusCode, responseText(t, stale))
	}

	updated := sendOwnerJSON(t, server.URL, http.MethodPost, ownerID, "/api/v1/account/tickets/"+createdBody.Data.Ticket.ID+"/follow-ups", "nonce-follow-up-current", "idem_ticket_followup_current", `{"body":"Please reopen this.","expected_version":2}`)
	if updated.StatusCode != http.StatusOK {
		t.Fatalf("current follow-up status = %d: %s", updated.StatusCode, responseText(t, updated))
	}
	var updatedBody struct {
		Data struct {
			Ticket struct {
				Status  string `json:"status"`
				Version int    `json:"version"`
			} `json:"ticket"`
		} `json:"data"`
	}
	decodeResponse(t, updated, &updatedBody)
	if updatedBody.Data.Ticket.Status != "open" || updatedBody.Data.Ticket.Version != 3 {
		t.Fatalf("reopened ticket = %+v, want open version 3", updatedBody.Data.Ticket)
	}

	retry := sendOwnerJSON(t, server.URL, http.MethodPost, ownerID, "/api/v1/account/tickets/"+createdBody.Data.Ticket.ID+"/follow-ups", "nonce-follow-up-retry", "idem_ticket_followup_current", `{"body":"Please reopen this.","expected_version":2}`)
	if retry.StatusCode != http.StatusOK {
		t.Fatalf("idempotent follow-up retry status = %d: %s", retry.StatusCode, responseText(t, retry))
	}
	var reopenedEvents, notifications int
	if err := pool.QueryRow(t.Context(), `
		SELECT
			(SELECT count(*) FROM account_portfolio_ticket_events WHERE ticket_id=$1 AND kind='reopened'),
			(SELECT count(*) FROM account_portfolio_notifications WHERE ticket_id=$1)
	`, createdBody.Data.Ticket.ID).Scan(&reopenedEvents, &notifications); err != nil {
		t.Fatal(err)
	}
	if reopenedEvents != 1 || notifications != 0 {
		t.Fatalf("reopen events/notifications = %d/%d, want 1/0", reopenedEvents, notifications)
	}
}

func TestTicketFollowUpIdempotencyKeyCannotCrossTicketResources(t *testing.T) {
	server, pool := newAccountPortfolioServer(t)
	defer server.Close()
	defer pool.Close()

	const ownerID = "abababab-abab-4bab-8bab-abababababab"
	first := sendOwnerJSON(t, server.URL, http.MethodPost, ownerID, "/api/v1/account/tickets", "nonce-cross-resource-create-one", "idem_cross_resource_create_one", `{"title":"First ticket","category":"account","body":"First initial message."}`)
	second := sendOwnerJSON(t, server.URL, http.MethodPost, ownerID, "/api/v1/account/tickets", "nonce-cross-resource-create-two", "idem_cross_resource_create_two", `{"title":"Second ticket","category":"account","body":"Second initial message."}`)
	if first.StatusCode != http.StatusCreated || second.StatusCode != http.StatusCreated {
		t.Fatalf("ticket creation statuses = %d/%d", first.StatusCode, second.StatusCode)
	}
	var firstBody, secondBody struct {
		Data struct {
			Ticket struct {
				ID string `json:"id"`
			} `json:"ticket"`
		} `json:"data"`
	}
	decodeResponse(t, first, &firstBody)
	decodeResponse(t, second, &secondBody)

	const followUp = `{"body":"Shared idempotency key must not cross tickets.","expected_version":1}`
	firstFollowUp := sendOwnerJSON(t, server.URL, http.MethodPost, ownerID, "/api/v1/account/tickets/"+firstBody.Data.Ticket.ID+"/follow-ups", "nonce-cross-resource-follow-one", "idem_cross_resource_followup", followUp)
	if firstFollowUp.StatusCode != http.StatusOK {
		t.Fatalf("first follow-up status = %d: %s", firstFollowUp.StatusCode, responseText(t, firstFollowUp))
	}
	_ = responseText(t, firstFollowUp)

	secondFollowUp := sendOwnerJSON(t, server.URL, http.MethodPost, ownerID, "/api/v1/account/tickets/"+secondBody.Data.Ticket.ID+"/follow-ups", "nonce-cross-resource-follow-two", "idem_cross_resource_followup", followUp)
	if secondFollowUp.StatusCode != http.StatusConflict {
		t.Fatalf("cross-ticket idempotency reuse status = %d, want 409: %s", secondFollowUp.StatusCode, responseText(t, secondFollowUp))
	}
	_ = responseText(t, secondFollowUp)

	var secondVersion, secondMessages int
	if err := pool.QueryRow(t.Context(), `
		SELECT version, (SELECT count(*) FROM account_portfolio_ticket_messages WHERE ticket_id=$1)
		FROM account_portfolio_tickets
		WHERE id=$1
	`, secondBody.Data.Ticket.ID).Scan(&secondVersion, &secondMessages); err != nil {
		t.Fatal(err)
	}
	if secondVersion != 1 || secondMessages != 1 {
		t.Fatalf("cross-ticket replay changed second ticket version/messages = %d/%d, want 1/1", secondVersion, secondMessages)
	}
}

func TestNotificationReadIsOwnerScopedAndIdempotent(t *testing.T) {
	server, pool := newAccountPortfolioServer(t)
	defer server.Close()
	defer pool.Close()

	const ownerID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	const otherUserID = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	seed := sendOwnerJSON(t, server.URL, http.MethodGet, ownerID, "/api/v1/account/summary", "nonce-notification-seed", "", "")
	if seed.StatusCode != http.StatusOK {
		t.Fatalf("seed account status = %d: %s", seed.StatusCode, responseText(t, seed))
	}
	_ = responseText(t, seed)
	const notificationID = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
	if _, err := pool.Exec(t.Context(), `
		INSERT INTO account_portfolio_notifications(id, user_id, title, body, kind)
		VALUES($1, $2, 'Ticket update', 'An operator replied.', 'ticket_operator_reply')
	`, notificationID, ownerID); err != nil {
		t.Fatal(err)
	}

	read := sendOwnerJSON(t, server.URL, http.MethodPost, ownerID, "/api/v1/account/notifications/"+notificationID+"/read", "nonce-notification-read", "idem_notification_read", "")
	if read.StatusCode != http.StatusOK {
		t.Fatalf("mark notification read status = %d: %s", read.StatusCode, responseText(t, read))
	}
	var result struct {
		Data struct {
			Notification struct {
				ReadAt *time.Time `json:"read_at"`
			} `json:"notification"`
		} `json:"data"`
	}
	decodeResponse(t, read, &result)
	if result.Data.Notification.ReadAt == nil {
		t.Fatal("marked notification omitted read_at")
	}

	retry := sendOwnerJSON(t, server.URL, http.MethodPost, ownerID, "/api/v1/account/notifications/"+notificationID+"/read", "nonce-notification-read-retry", "idem_notification_read", "")
	if retry.StatusCode != http.StatusOK {
		t.Fatalf("idempotent notification read retry status = %d: %s", retry.StatusCode, responseText(t, retry))
	}
	_ = responseText(t, retry)
	other := sendOwnerJSON(t, server.URL, http.MethodPost, otherUserID, "/api/v1/account/notifications/"+notificationID+"/read", "nonce-notification-other", "idem_notification_read_other", "")
	if other.StatusCode != http.StatusNotFound {
		t.Fatalf("other user mark-read status = %d, want 404: %s", other.StatusCode, responseText(t, other))
	}
}

func TestNotificationReadIdempotencyKeyCannotCrossNotificationResources(t *testing.T) {
	server, pool := newAccountPortfolioServer(t)
	defer server.Close()
	defer pool.Close()

	const ownerID = "acacacac-acac-4cac-8cac-acacacacacac"
	seed := sendOwnerJSON(t, server.URL, http.MethodGet, ownerID, "/api/v1/account/summary", "nonce-cross-notification-seed", "", "")
	if seed.StatusCode != http.StatusOK {
		t.Fatalf("seed account status = %d: %s", seed.StatusCode, responseText(t, seed))
	}
	_ = responseText(t, seed)
	const firstNotificationID = "cdcdcdcd-cdcd-4dcd-8dcd-cdcdcdcdcdcd"
	const secondNotificationID = "dededede-dede-4ede-8ede-dededededede"
	if _, err := pool.Exec(t.Context(), `
		INSERT INTO account_portfolio_notifications(id, user_id, title, body, kind)
		VALUES($1, $3, 'First', 'First notification.', 'ticket_status'),
		       ($2, $3, 'Second', 'Second notification.', 'ticket_status')
	`, firstNotificationID, secondNotificationID, ownerID); err != nil {
		t.Fatal(err)
	}

	firstRead := sendOwnerJSON(t, server.URL, http.MethodPost, ownerID, "/api/v1/account/notifications/"+firstNotificationID+"/read", "nonce-cross-notification-first", "idem_cross_notification_read", "")
	if firstRead.StatusCode != http.StatusOK {
		t.Fatalf("first notification read status = %d: %s", firstRead.StatusCode, responseText(t, firstRead))
	}
	_ = responseText(t, firstRead)
	secondRead := sendOwnerJSON(t, server.URL, http.MethodPost, ownerID, "/api/v1/account/notifications/"+secondNotificationID+"/read", "nonce-cross-notification-second", "idem_cross_notification_read", "")
	if secondRead.StatusCode != http.StatusConflict {
		t.Fatalf("cross-notification idempotency reuse status = %d, want 409: %s", secondRead.StatusCode, responseText(t, secondRead))
	}
	_ = responseText(t, secondRead)

	var secondReadAt *time.Time
	if err := pool.QueryRow(t.Context(), `SELECT read_at FROM account_portfolio_notifications WHERE id=$1`, secondNotificationID).Scan(&secondReadAt); err != nil {
		t.Fatal(err)
	}
	if secondReadAt != nil {
		t.Fatalf("cross-notification replay marked second notification read at %s", secondReadAt)
	}
}

func TestConsoleReplyAndTransitionUseSeparateCallerAndCreateOneNotificationEach(t *testing.T) {
	server, pool := newAccountPortfolioServerWithConsole(t)
	defer server.Close()
	defer pool.Close()

	const ownerID = "dddddddd-dddd-4ddd-8ddd-dddddddddddd"
	const operatorID = "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee"
	created := sendOwnerJSON(t, server.URL, http.MethodPost, ownerID, "/api/v1/account/tickets", "nonce-console-create", "idem_console_create", `{"title":"Console workflow","category":"account","body":"I need an answer."}`)
	if created.StatusCode != http.StatusCreated {
		t.Fatalf("create ticket status = %d: %s", created.StatusCode, responseText(t, created))
	}
	var createdBody struct {
		Data struct {
			Ticket struct {
				ID string `json:"id"`
			} `json:"ticket"`
		} `json:"data"`
	}
	decodeResponse(t, created, &createdBody)
	queue := sendConsoleJSON(t, server.URL, http.MethodGet, operatorID, "/api/v1/console/tickets", "nonce-console-queue", "", "")
	if queue.StatusCode != http.StatusOK {
		t.Fatalf("console ticket queue status = %d: %s", queue.StatusCode, responseText(t, queue))
	}
	queueBody := responseText(t, queue)
	if !strings.Contains(queueBody, createdBody.Data.Ticket.ID) || strings.Contains(queueBody, ownerID) {
		t.Fatalf("console ticket queue leaked owner identity or omitted ticket: %s", queueBody)
	}

	replyBody := `{"body":"We are looking into this.","expected_version":1}`
	reply := sendConsoleJSON(t, server.URL, http.MethodPost, operatorID, "/api/v1/console/tickets/"+createdBody.Data.Ticket.ID+"/replies", "nonce-console-reply", "idem_console_reply", replyBody)
	if reply.StatusCode != http.StatusOK {
		t.Fatalf("console reply status = %d: %s", reply.StatusCode, responseText(t, reply))
	}
	var replied struct {
		Data struct {
			Ticket struct {
				Status  string `json:"status"`
				Version int    `json:"version"`
			} `json:"ticket"`
		} `json:"data"`
	}
	decodeResponse(t, reply, &replied)
	if replied.Data.Ticket.Status != "in_progress" || replied.Data.Ticket.Version != 2 {
		t.Fatalf("console reply ticket = %+v, want in_progress version 2", replied.Data.Ticket)
	}

	retry := sendConsoleJSON(t, server.URL, http.MethodPost, operatorID, "/api/v1/console/tickets/"+createdBody.Data.Ticket.ID+"/replies", "nonce-console-reply-retry", "idem_console_reply", replyBody)
	if retry.StatusCode != http.StatusOK {
		t.Fatalf("console reply retry status = %d: %s", retry.StatusCode, responseText(t, retry))
	}
	var retried struct {
		Data struct {
			Ticket struct {
				Status  string `json:"status"`
				Version int    `json:"version"`
			} `json:"ticket"`
		} `json:"data"`
	}
	decodeResponse(t, retry, &retried)
	if retried.Data.Ticket.Status != "in_progress" || retried.Data.Ticket.Version != 2 {
		t.Fatalf("idempotent console reply = %+v, want in_progress version 2", retried.Data.Ticket)
	}

	detail := sendConsoleJSON(t, server.URL, http.MethodGet, operatorID, "/api/v1/console/tickets/"+createdBody.Data.Ticket.ID, "nonce-console-detail", "", "")
	if detail.StatusCode != http.StatusOK {
		t.Fatalf("console ticket detail status = %d: %s", detail.StatusCode, responseText(t, detail))
	}
	detailBody := responseText(t, detail)
	if !strings.Contains(detailBody, "operator_reply") || strings.Contains(detailBody, operatorID) || strings.Contains(detailBody, ownerID) {
		t.Fatalf("console ticket detail leaked actor identity or omitted durable reply event: %s", detailBody)
	}
	var messages, notifications, operatorMessages int
	if err := pool.QueryRow(t.Context(), `
		SELECT
			(SELECT count(*) FROM account_portfolio_ticket_messages WHERE ticket_id=$1),
			(SELECT count(*) FROM account_portfolio_notifications WHERE ticket_id=$1),
			(SELECT count(*) FROM account_portfolio_ticket_messages WHERE ticket_id=$1 AND author_kind='operator' AND operator_user_id=$2)
	`, createdBody.Data.Ticket.ID, operatorID).Scan(&messages, &notifications, &operatorMessages); err != nil {
		t.Fatal(err)
	}
	if messages != 2 || notifications != 1 || operatorMessages != 1 {
		t.Fatalf("reply facts messages/notifications/operator_messages = %d/%d/%d, want 2/1/1", messages, notifications, operatorMessages)
	}

	portalCaller := sendOwnerJSON(t, server.URL, http.MethodPost, ownerID, "/api/v1/console/tickets/"+createdBody.Data.Ticket.ID+"/replies", "nonce-portal-console-denied", "idem_portal_console_denied", replyBody)
	if portalCaller.StatusCode != http.StatusForbidden {
		t.Fatalf("portal caller reached console command status = %d, want 403: %s", portalCaller.StatusCode, responseText(t, portalCaller))
	}

	transition := sendConsoleJSON(t, server.URL, http.MethodPost, operatorID, "/api/v1/console/tickets/"+createdBody.Data.Ticket.ID+"/transitions", "nonce-console-transition", "idem_console_transition", `{"status":"resolved","expected_version":2}`)
	if transition.StatusCode != http.StatusOK {
		t.Fatalf("console transition status = %d: %s", transition.StatusCode, responseText(t, transition))
	}
	var transitioned struct {
		Data struct {
			Ticket struct {
				Status  string `json:"status"`
				Version int    `json:"version"`
			} `json:"ticket"`
		} `json:"data"`
	}
	decodeResponse(t, transition, &transitioned)
	if transitioned.Data.Ticket.Status != "resolved" || transitioned.Data.Ticket.Version != 3 {
		t.Fatalf("transitioned ticket = %+v, want resolved version 3", transitioned.Data.Ticket)
	}
	transitionRetry := sendConsoleJSON(t, server.URL, http.MethodPost, operatorID, "/api/v1/console/tickets/"+createdBody.Data.Ticket.ID+"/transitions", "nonce-console-transition-retry", "idem_console_transition", `{"status":"resolved","expected_version":2}`)
	if transitionRetry.StatusCode != http.StatusOK {
		t.Fatalf("console transition retry status = %d: %s", transitionRetry.StatusCode, responseText(t, transitionRetry))
	}
	var transitionRetried struct {
		Data struct {
			Ticket struct {
				Status  string `json:"status"`
				Version int    `json:"version"`
			} `json:"ticket"`
		} `json:"data"`
	}
	decodeResponse(t, transitionRetry, &transitionRetried)
	if transitionRetried.Data.Ticket.Status != "resolved" || transitionRetried.Data.Ticket.Version != 3 {
		t.Fatalf("idempotent console transition = %+v, want resolved version 3", transitionRetried.Data.Ticket)
	}
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM account_portfolio_notifications WHERE ticket_id=$1`, createdBody.Data.Ticket.ID).Scan(&notifications); err != nil {
		t.Fatal(err)
	}
	if notifications != 2 {
		t.Fatalf("notifications after transition = %d, want 2", notifications)
	}
}

func TestConsoleTicketCommandsRejectStaleAndResolvedCommandsWithoutFacts(t *testing.T) {
	server, pool := newAccountPortfolioServerWithConsole(t)
	defer server.Close()
	defer pool.Close()

	const ownerID = "f1111111-1111-4111-8111-111111111111"
	const operatorID = "f2222222-2222-4222-8222-222222222222"
	created := sendOwnerJSON(t, server.URL, http.MethodPost, ownerID, "/api/v1/account/tickets", "nonce-console-state-create", "idem_console_state_create", `{"title":"Lifecycle","category":"account","body":"Initial message."}`)
	if created.StatusCode != http.StatusCreated {
		t.Fatalf("create ticket status = %d: %s", created.StatusCode, responseText(t, created))
	}
	var createdBody struct {
		Data struct {
			Ticket struct {
				ID string `json:"id"`
			} `json:"ticket"`
		} `json:"data"`
	}
	decodeResponse(t, created, &createdBody)

	stale := sendConsoleJSON(t, server.URL, http.MethodPost, operatorID, "/api/v1/console/tickets/"+createdBody.Data.Ticket.ID+"/transitions", "nonce-console-state-stale", "idem_console_state_stale", `{"status":"resolved","expected_version":2}`)
	if stale.StatusCode != http.StatusConflict {
		t.Fatalf("stale console transition status = %d, want 409: %s", stale.StatusCode, responseText(t, stale))
	}

	resolved := sendConsoleJSON(t, server.URL, http.MethodPost, operatorID, "/api/v1/console/tickets/"+createdBody.Data.Ticket.ID+"/transitions", "nonce-console-state-resolve", "idem_console_state_resolve", `{"status":"resolved","expected_version":1}`)
	if resolved.StatusCode != http.StatusOK {
		t.Fatalf("resolve console ticket status = %d: %s", resolved.StatusCode, responseText(t, resolved))
	}
	_ = responseText(t, resolved)

	replyResolved := sendConsoleJSON(t, server.URL, http.MethodPost, operatorID, "/api/v1/console/tickets/"+createdBody.Data.Ticket.ID+"/replies", "nonce-console-state-reply", "idem_console_state_reply", `{"body":"This must not be added.","expected_version":2}`)
	if replyResolved.StatusCode != http.StatusConflict {
		t.Fatalf("reply to resolved ticket status = %d, want 409: %s", replyResolved.StatusCode, responseText(t, replyResolved))
	}

	var messages, events, notifications int
	if err := pool.QueryRow(t.Context(), `
		SELECT
			(SELECT count(*) FROM account_portfolio_ticket_messages WHERE ticket_id=$1),
			(SELECT count(*) FROM account_portfolio_ticket_events WHERE ticket_id=$1),
			(SELECT count(*) FROM account_portfolio_notifications WHERE ticket_id=$1)
	`, createdBody.Data.Ticket.ID).Scan(&messages, &events, &notifications); err != nil {
		t.Fatal(err)
	}
	if messages != 1 || events != 1 || notifications != 1 {
		t.Fatalf("rejected console command wrote facts messages/events/notifications = %d/%d/%d, want 1/1/1", messages, events, notifications)
	}
}

func TestConcurrentConsoleTransitionsWithOneRevisionYieldOneSuccessAndOneConflict(t *testing.T) {
	server, pool := newAccountPortfolioServerWithConsole(t)
	defer server.Close()
	defer pool.Close()

	const ownerID = "f3333333-3333-4333-8333-333333333333"
	const operatorID = "f4444444-4444-4444-8444-444444444444"
	created := sendOwnerJSON(t, server.URL, http.MethodPost, ownerID, "/api/v1/account/tickets", "nonce-concurrent-create", "idem_concurrent_create", `{"title":"Concurrent transition","category":"account","body":"Initial message."}`)
	if created.StatusCode != http.StatusCreated {
		t.Fatalf("create ticket status = %d: %s", created.StatusCode, responseText(t, created))
	}
	var createdBody struct {
		Data struct {
			Ticket struct {
				ID string `json:"id"`
			} `json:"ticket"`
		} `json:"data"`
	}
	decodeResponse(t, created, &createdBody)

	start := make(chan struct{})
	statuses := make(chan int, 2)
	for index := range 2 {
		go func(index int) {
			<-start
			response := sendConsoleJSON(t, server.URL, http.MethodPost, operatorID, "/api/v1/console/tickets/"+createdBody.Data.Ticket.ID+"/transitions", "nonce-concurrent-transition-"+strconv.Itoa(index), "idem_concurrent_transition_"+strconv.Itoa(index), `{"status":"resolved","expected_version":1}`)
			defer response.Body.Close()
			statuses <- response.StatusCode
		}(index)
	}
	close(start)
	first, second := <-statuses, <-statuses
	if (first != http.StatusOK || second != http.StatusConflict) && (first != http.StatusConflict || second != http.StatusOK) {
		t.Fatalf("concurrent transition statuses = %d/%d, want one 200 and one 409", first, second)
	}

	var status string
	var version, events, notifications int
	if err := pool.QueryRow(t.Context(), `
		SELECT
			(SELECT status FROM account_portfolio_tickets WHERE id=$1),
			(SELECT version FROM account_portfolio_tickets WHERE id=$1),
			(SELECT count(*) FROM account_portfolio_ticket_events WHERE ticket_id=$1 AND kind='status_transition'),
			(SELECT count(*) FROM account_portfolio_notifications WHERE ticket_id=$1)
	`, createdBody.Data.Ticket.ID).Scan(&status, &version, &events, &notifications); err != nil {
		t.Fatal(err)
	}
	if status != "resolved" || version != 2 || events != 1 || notifications != 1 {
		t.Fatalf("concurrent transition facts status/version/events/notifications = %s/%d/%d/%d, want resolved/2/1/1", status, version, events, notifications)
	}
}

func sendOwnerJSON(t *testing.T, baseURL, method, actorID, route, nonce, idempotencyKey, body string) *http.Response {
	t.Helper()
	request, err := http.NewRequest(method, baseURL+route, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("X-Actor-User-Id", actorID)
	request.Header.Set("X-Request-Id", "req_ticket_test")
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if idempotencyKey != "" {
		request.Header.Set("Idempotency-Key", idempotencyKey)
	}
	signOwnerRequest(t, request, nonce, []byte(body))
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func sendConsoleJSON(t *testing.T, baseURL, method, actorID, route, nonce, idempotencyKey, body string) *http.Response {
	t.Helper()
	request, err := http.NewRequest(method, baseURL+route, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("X-Actor-User-Id", actorID)
	request.Header.Set("X-Request-Id", "req_console_ticket_test")
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if idempotencyKey != "" {
		request.Header.Set("Idempotency-Key", idempotencyKey)
	}
	signConsoleRequest(t, request, nonce, []byte(body))
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func signOwnerRequest(t *testing.T, request *http.Request, nonce string, body []byte) {
	t.Helper()
	timestamp := time.Now().Unix()
	nonceDigest := sha256.Sum256([]byte(nonce))
	nonce = base64.RawURLEncoding.EncodeToString(nonceDigest[:24])
	digest := sha256.Sum256(body)
	canonical := strings.Join([]string{
		request.Method,
		request.URL.RequestURI(),
		fmtInt(timestamp),
		nonce,
		hex.EncodeToString(digest[:]),
		request.Header.Get("X-Actor-User-Id"),
	}, "\n")
	mac := hmac.New(sha256.New, []byte(serviceSecret))
	_, _ = mac.Write([]byte(canonical))
	request.SetBasicAuth("portal-gateway", serviceSecret)
	request.Header.Set("X-Service-Id", "portal-gateway")
	request.Header.Set("X-Key-Id", "account-key")
	request.Header.Set("X-Timestamp", fmtInt(timestamp))
	request.Header.Set("X-Nonce", nonce)
	request.Header.Set("X-Signature", base64.RawURLEncoding.EncodeToString(mac.Sum(nil)))
}

func signConsoleRequest(t *testing.T, request *http.Request, nonce string, body []byte) {
	t.Helper()
	timestamp := time.Now().Unix()
	nonceDigest := sha256.Sum256([]byte(nonce))
	nonce = base64.RawURLEncoding.EncodeToString(nonceDigest[:24])
	digest := sha256.Sum256(body)
	canonical := strings.Join([]string{
		request.Method,
		request.URL.RequestURI(),
		fmtInt(timestamp),
		nonce,
		hex.EncodeToString(digest[:]),
		request.Header.Get("X-Actor-User-Id"),
	}, "\n")
	mac := hmac.New(sha256.New, []byte(consoleServiceSecret))
	_, _ = mac.Write([]byte(canonical))
	request.SetBasicAuth("console-gateway", consoleServiceSecret)
	request.Header.Set("X-Service-Id", "console-gateway")
	request.Header.Set("X-Key-Id", "console-key")
	request.Header.Set("X-Timestamp", fmtInt(timestamp))
	request.Header.Set("X-Nonce", nonce)
	request.Header.Set("X-Signature", base64.RawURLEncoding.EncodeToString(mac.Sum(nil)))
}

func decodeResponse(t *testing.T, response *http.Response, target any) {
	t.Helper()
	defer response.Body.Close()
	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		t.Fatal(err)
	}
}

func responseText(t *testing.T, response *http.Response) string {
	t.Helper()
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func fmtInt(value int64) string {
	return strconv.FormatInt(value, 10)
}
