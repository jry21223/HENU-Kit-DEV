package accountportfolio

import (
	"encoding/json"
	"strings"
	"time"
)

const maxPublicPointValue int64 = 9_007_199_254_740_991

// validateData is the Gateway's runtime contract guard. The owner is a
// separate deployment, so a 200 with a truncated or incompatible payload must
// never become a successful browser account response.
func validateData(path string, raw json.RawMessage) error {
	value, ok := requiredObject(raw)
	if !ok {
		return ErrInvalid
	}
	switch {
	case path == SummaryPath:
		if !validSummary(value) {
			return ErrInvalid
		}
	case path == PointsPath:
		if !validPoints(value) {
			return ErrInvalid
		}
	case path == MembershipPath:
		if !validMembership(value) {
			return ErrInvalid
		}
	case path == NotificationsPath:
		if !validNotifications(value) {
			return ErrInvalid
		}
	case path == TicketsPath:
		if !validTickets(value) {
			return ErrInvalid
		}
	case strings.HasPrefix(path, TicketsPath+"/"):
		if !validTicketDetail(value) {
			return ErrInvalid
		}
	case path == MembershipOrdersPath:
		if !validMembershipOrders(value) {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	return nil
}

func validateCommandData(path string, raw json.RawMessage) error {
	value, ok := requiredObject(raw)
	if !ok {
		return ErrInvalid
	}
	switch {
	case path == TicketsPath || strings.HasSuffix(path, "/follow-ups"):
		ticket, ticketOK := requiredObject(value["ticket"])
		if !ticketOK || !validTicket(ticket) {
			return ErrInvalid
		}
	case strings.HasPrefix(path, NotificationsPath+"/") && strings.HasSuffix(path, "/read"):
		notification, notificationOK := requiredObject(value["notification"])
		if !notificationOK || !validNotification(notification) {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	return nil
}

func validSummary(value map[string]json.RawMessage) bool {
	points, pointsOK := requiredInt(value, "points_balance")
	plan, planOK := requiredString(value, "plan")
	lifetime, lifetimeOK := requiredBool(value, "lifetime")
	unread, unreadOK := requiredInt(value, "unread_notification_count")
	open, openOK := requiredInt(value, "open_ticket_count")
	return pointsOK && points >= 0 && points <= maxPublicPointValue && planOK && validPlan(plan) && lifetimeOK && lifetime == (plan == "lifetime") && unreadOK && unread >= 0 && openOK && open >= 0
}

func validPoints(value map[string]json.RawMessage) bool {
	balance, balanceOK := requiredInt(value, "balance")
	entries, entriesOK := requiredArray(value, "entries")
	if !balanceOK || balance < 0 || balance > maxPublicPointValue || !entriesOK || !onlyKeys(value, "balance", "entries", "next_cursor") || !validPointCursor(value) {
		return false
	}
	for _, raw := range entries {
		entry, ok := requiredObject(raw)
		if !ok {
			return false
		}
		id, idOK := requiredString(entry, "id")
		amount, amountOK := requiredInt(entry, "amount")
		_, reasonOK := requiredString(entry, "reason")
		createdOK := requiredTimestamp(entry, "created_at")
		if !idOK || !validUUID(id) || !amountOK || amount < -maxPublicPointValue || amount > maxPublicPointValue || amount == 0 || !reasonOK || !createdOK || !onlyKeys(entry, "id", "amount", "reason", "created_at") {
			return false
		}
	}
	return true
}

func validPointCursor(value map[string]json.RawMessage) bool {
	raw, exists := value["next_cursor"]
	if !exists || isNull(raw) {
		return exists
	}
	cursor, ok := requiredString(value, "next_cursor")
	return ok && len(cursor) > 0 && len(cursor) <= 512
}

func validMembership(value map[string]json.RawMessage) bool {
	plan, planOK := requiredString(value, "plan")
	lifetime, lifetimeOK := requiredBool(value, "lifetime")
	return planOK && validPlan(plan) && lifetimeOK && lifetime == (plan == "lifetime")
}

func validNotifications(value map[string]json.RawMessage) bool {
	items, ok := requiredArray(value, "notifications")
	if !ok {
		return false
	}
	for _, raw := range items {
		item, itemOK := requiredObject(raw)
		if !itemOK || !validNotification(item) {
			return false
		}
	}
	return true
}

func validTickets(value map[string]json.RawMessage) bool {
	items, ok := requiredArray(value, "tickets")
	if !ok {
		return false
	}
	for _, raw := range items {
		item, itemOK := requiredObject(raw)
		if !itemOK || !validTicket(item) {
			return false
		}
	}
	return true
}

func validNotification(item map[string]json.RawMessage) bool {
	id, idOK := requiredString(item, "id")
	_, titleOK := requiredString(item, "title")
	_, bodyOK := requiredString(item, "body")
	_, kindOK := requiredString(item, "kind")
	createdOK := requiredTimestamp(item, "created_at")
	return idOK && validUUID(id) && titleOK && bodyOK && kindOK && createdOK && optionalTimestamp(item, "read_at") && validOptionalTicketReference(item)
}

func validOptionalTicketReference(item map[string]json.RawMessage) bool {
	ticketIDRaw, hasTicketID := item["ticket_id"]
	referenceRaw, hasReference := item["ticket_reference"]
	if (!hasTicketID || isNull(ticketIDRaw)) && (!hasReference || isNull(referenceRaw)) {
		return true
	}
	if !hasTicketID || !hasReference || isNull(ticketIDRaw) || isNull(referenceRaw) {
		return false
	}
	ticketID, ticketIDOK := requiredString(item, "ticket_id")
	reference, referenceOK := requiredString(item, "ticket_reference")
	return ticketIDOK && referenceOK && validUUID(ticketID) && reference == "HKT-"+strings.ToLower(ticketID)
}

func validTicket(item map[string]json.RawMessage) bool {
	id, idOK := requiredString(item, "id")
	reference, referenceOK := requiredString(item, "reference")
	_, titleOK := requiredString(item, "title")
	_, categoryOK := requiredString(item, "category")
	status, statusOK := requiredString(item, "status")
	version, versionOK := requiredInt(item, "version")
	createdOK := requiredTimestamp(item, "created_at")
	updatedOK := requiredTimestamp(item, "updated_at")
	return idOK && validUUID(id) && referenceOK && reference == "HKT-"+strings.ToLower(id) && titleOK && categoryOK && statusOK && validTicketStatus(status) && versionOK && version >= 1 && createdOK && updatedOK
}

func validTicketDetail(value map[string]json.RawMessage) bool {
	ticket, ticketOK := requiredObject(value["ticket"])
	messages, messagesOK := requiredArray(value, "messages")
	events, eventsOK := requiredArray(value, "events")
	if !ticketOK || !validTicket(ticket) || !messagesOK || !eventsOK {
		return false
	}
	for _, raw := range messages {
		message, ok := requiredObject(raw)
		if !ok || !validTicketMessage(message) {
			return false
		}
	}
	for _, raw := range events {
		event, ok := requiredObject(raw)
		if !ok || !validTicketEvent(event) {
			return false
		}
	}
	return true
}

func validTicketMessage(value map[string]json.RawMessage) bool {
	id, idOK := requiredString(value, "id")
	authorKind, authorKindOK := requiredString(value, "author_kind")
	_, bodyOK := requiredString(value, "body")
	return idOK && validUUID(id) && authorKindOK && (authorKind == "user" || authorKind == "operator") && bodyOK && requiredTimestamp(value, "created_at")
}

func validTicketEvent(value map[string]json.RawMessage) bool {
	id, idOK := requiredString(value, "id")
	kind, kindOK := requiredString(value, "kind")
	from, fromOK := requiredString(value, "from_status")
	to, toOK := requiredString(value, "to_status")
	return idOK && validUUID(id) && kindOK && (kind == "operator_reply" || kind == "status_transition" || kind == "reopened") && fromOK && validTicketStatus(from) && toOK && validTicketStatus(to) && requiredTimestamp(value, "created_at")
}

func validMembershipOrders(value map[string]json.RawMessage) bool {
	items, ok := requiredArray(value, "orders")
	if !ok {
		return false
	}
	for _, raw := range items {
		item, itemOK := requiredObject(raw)
		id, idOK := requiredString(item, "id")
		plan, planOK := requiredString(item, "plan")
		amount, amountOK := requiredInt(item, "amount_cents")
		status, statusOK := requiredString(item, "status")
		version, versionOK := requiredInt(item, "version")
		createdOK := requiredTimestamp(item, "created_at")
		updatedOK := requiredTimestamp(item, "updated_at")
		if !itemOK || !idOK || !validUUID(id) || !planOK || plan != "lifetime" || !amountOK || amount != 990 || !statusOK || !validOrderStatus(status) || !versionOK || version < 1 || !createdOK || !updatedOK {
			return false
		}
	}
	return true
}

func requiredObject(raw json.RawMessage) (map[string]json.RawMessage, bool) {
	if isNull(raw) {
		return nil, false
	}
	var value map[string]json.RawMessage
	if err := json.Unmarshal(raw, &value); err != nil || value == nil {
		return nil, false
	}
	return value, true
}

func onlyKeys(value map[string]json.RawMessage, allowed ...string) bool {
	for key := range value {
		valid := false
		for _, candidate := range allowed {
			if key == candidate {
				valid = true
				break
			}
		}
		if !valid {
			return false
		}
	}
	return true
}

func requiredArray(value map[string]json.RawMessage, key string) ([]json.RawMessage, bool) {
	raw, ok := value[key]
	if !ok || isNull(raw) {
		return nil, false
	}
	var result []json.RawMessage
	if err := json.Unmarshal(raw, &result); err != nil || result == nil {
		return nil, false
	}
	return result, true
}

func requiredString(value map[string]json.RawMessage, key string) (string, bool) {
	raw, ok := value[key]
	if !ok || isNull(raw) {
		return "", false
	}
	var result string
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", false
	}
	return result, true
}

func requiredInt(value map[string]json.RawMessage, key string) (int64, bool) {
	raw, ok := value[key]
	if !ok || isNull(raw) {
		return 0, false
	}
	var result int64
	if err := json.Unmarshal(raw, &result); err != nil {
		return 0, false
	}
	return result, true
}

func requiredBool(value map[string]json.RawMessage, key string) (bool, bool) {
	raw, ok := value[key]
	if !ok || isNull(raw) {
		return false, false
	}
	var result bool
	if err := json.Unmarshal(raw, &result); err != nil {
		return false, false
	}
	return result, true
}

func requiredTimestamp(value map[string]json.RawMessage, key string) bool {
	text, ok := requiredString(value, key)
	if !ok {
		return false
	}
	_, err := time.Parse(time.RFC3339, text)
	return err == nil
}

func optionalTimestamp(value map[string]json.RawMessage, key string) bool {
	raw, exists := value[key]
	if !exists || isNull(raw) {
		return true
	}
	return requiredTimestamp(value, key)
}

func isNull(raw json.RawMessage) bool {
	return strings.TrimSpace(string(raw)) == "null"
}

func validPlan(value string) bool {
	return value == "free" || value == "lifetime"
}

func validTicketStatus(value string) bool {
	return value == "open" || value == "in_progress" || value == "resolved"
}

func validOrderStatus(value string) bool {
	switch value {
	case "created", "pending_payment", "paid", "closed", "failed", "refunded":
		return true
	default:
		return false
	}
}

func validUUID(value string) bool {
	if len(value) != 36 {
		return false
	}
	for index, char := range value {
		if index == 8 || index == 13 || index == 18 || index == 23 {
			if char != '-' {
				return false
			}
			continue
		}
		if !(char >= '0' && char <= '9') && !(char >= 'a' && char <= 'f') && !(char >= 'A' && char <= 'F') {
			return false
		}
	}
	return true
}

func validIdempotencyKey(value string) bool {
	if len(value) < 8 || len(value) > 200 {
		return false
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '.' || char == '_' || char == ':' || char == '-' {
			continue
		}
		return false
	}
	return true
}
