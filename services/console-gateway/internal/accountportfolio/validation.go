package accountportfolio

import (
	"encoding/json"
	"strings"
	"time"
)

// Runtime owner validation prevents a syntactically successful but incomplete
// downstream response from becoming a Console success response.
func validateTicketQueue(raw json.RawMessage) error {
	value, ok := requiredObject(raw)
	if !ok {
		return ErrInvalid
	}
	tickets, ok := requiredArray(value, "tickets")
	if !ok {
		return ErrInvalid
	}
	for _, item := range tickets {
		ticket, ok := requiredObject(item)
		if !ok || !validTicket(ticket) {
			return ErrInvalid
		}
	}
	return nil
}

func validateTicketDetail(raw json.RawMessage) error {
	value, ok := requiredObject(raw)
	if !ok {
		return ErrInvalid
	}
	ticket, ticketOK := requiredObject(value["ticket"])
	messages, messagesOK := requiredArray(value, "messages")
	events, eventsOK := requiredArray(value, "events")
	if !ticketOK || !validTicket(ticket) || !messagesOK || !eventsOK {
		return ErrInvalid
	}
	for _, item := range messages {
		message, ok := requiredObject(item)
		if !ok || !validMessage(message) {
			return ErrInvalid
		}
	}
	for _, item := range events {
		event, ok := requiredObject(item)
		if !ok || !validEvent(event) {
			return ErrInvalid
		}
	}
	return nil
}

func validateTicketCommand(raw json.RawMessage) error {
	value, ok := requiredObject(raw)
	if !ok {
		return ErrInvalid
	}
	ticket, ok := requiredObject(value["ticket"])
	if !ok || !validTicket(ticket) {
		return ErrInvalid
	}
	return nil
}

func validTicket(value map[string]json.RawMessage) bool {
	id, idOK := requiredString(value, "id")
	reference, referenceOK := requiredString(value, "reference")
	_, titleOK := requiredString(value, "title")
	_, categoryOK := requiredString(value, "category")
	status, statusOK := requiredString(value, "status")
	version, versionOK := requiredInt(value, "version")
	return idOK && validUUID(id) && referenceOK && reference == "HKT-"+strings.ToLower(id) && titleOK && categoryOK && statusOK && validStatus(status) && versionOK && version >= 1 && requiredTimestamp(value, "created_at") && requiredTimestamp(value, "updated_at") && !hasInternalIdentity(value)
}

func validMessage(value map[string]json.RawMessage) bool {
	id, idOK := requiredString(value, "id")
	authorKind, authorKindOK := requiredString(value, "author_kind")
	_, bodyOK := requiredString(value, "body")
	return idOK && validUUID(id) && authorKindOK && (authorKind == "user" || authorKind == "operator") && bodyOK && requiredTimestamp(value, "created_at") && !hasInternalIdentity(value)
}

func validEvent(value map[string]json.RawMessage) bool {
	id, idOK := requiredString(value, "id")
	kind, kindOK := requiredString(value, "kind")
	from, fromOK := requiredString(value, "from_status")
	to, toOK := requiredString(value, "to_status")
	return idOK && validUUID(id) && kindOK && (kind == "operator_reply" || kind == "status_transition" || kind == "reopened") && fromOK && validStatus(from) && toOK && validStatus(to) && requiredTimestamp(value, "created_at") && !hasInternalIdentity(value)
}

func hasInternalIdentity(value map[string]json.RawMessage) bool {
	_, hasOwner := value["user_id"]
	_, hasOperator := value["operator_user_id"]
	return hasOwner || hasOperator
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

func requiredTimestamp(value map[string]json.RawMessage, key string) bool {
	text, ok := requiredString(value, key)
	if !ok {
		return false
	}
	_, err := time.Parse(time.RFC3339, text)
	return err == nil
}

func isNull(raw json.RawMessage) bool { return strings.TrimSpace(string(raw)) == "null" }

func validStatus(value string) bool {
	return value == "open" || value == "in_progress" || value == "resolved"
}
