package accountportfolio

import (
	"encoding/json"
	"strings"
	"time"
)

// validateData is the Gateway's runtime contract guard. The owner is a
// separate deployment, so a 200 with a truncated or incompatible payload must
// never become a successful browser account response.
func validateData(path string, raw json.RawMessage) error {
	value, ok := requiredObject(raw)
	if !ok {
		return ErrInvalid
	}
	switch path {
	case SummaryPath:
		if !validSummary(value) {
			return ErrInvalid
		}
	case PointsPath:
		if !validPoints(value) {
			return ErrInvalid
		}
	case MembershipPath:
		if !validMembership(value) {
			return ErrInvalid
		}
	case NotificationsPath:
		if !validNotifications(value) {
			return ErrInvalid
		}
	case TicketsPath:
		if !validTickets(value) {
			return ErrInvalid
		}
	case MembershipOrdersPath:
		if !validMembershipOrders(value) {
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
	return pointsOK && points >= 0 && planOK && validPlan(plan) && lifetimeOK && lifetime == (plan == "lifetime") && unreadOK && unread >= 0 && openOK && open >= 0
}

func validPoints(value map[string]json.RawMessage) bool {
	balance, balanceOK := requiredInt(value, "balance")
	entries, entriesOK := requiredArray(value, "entries")
	if !balanceOK || balance < 0 || !entriesOK {
		return false
	}
	for _, raw := range entries {
		entry, ok := requiredObject(raw)
		if !ok {
			return false
		}
		id, idOK := requiredString(entry, "id")
		_, amountOK := requiredInt(entry, "amount")
		_, reasonOK := requiredString(entry, "reason")
		createdOK := requiredTimestamp(entry, "created_at")
		if !idOK || !validUUID(id) || !amountOK || !reasonOK || !createdOK {
			return false
		}
	}
	return true
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
		id, idOK := requiredString(item, "id")
		_, titleOK := requiredString(item, "title")
		_, bodyOK := requiredString(item, "body")
		_, kindOK := requiredString(item, "kind")
		createdOK := requiredTimestamp(item, "created_at")
		if !itemOK || !idOK || !validUUID(id) || !titleOK || !bodyOK || !kindOK || !createdOK || !optionalTimestamp(item, "read_at") {
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
		id, idOK := requiredString(item, "id")
		_, titleOK := requiredString(item, "title")
		_, categoryOK := requiredString(item, "category")
		status, statusOK := requiredString(item, "status")
		updatedOK := requiredTimestamp(item, "updated_at")
		if !itemOK || !idOK || !validUUID(id) || !titleOK || !categoryOK || !statusOK || !validTicketStatus(status) || !updatedOK {
			return false
		}
	}
	return true
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
		createdOK := requiredTimestamp(item, "created_at")
		if !itemOK || !idOK || !validUUID(id) || !planOK || plan != "lifetime" || !amountOK || amount != 990 || !statusOK || !validOrderStatus(status) || !createdOK {
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
