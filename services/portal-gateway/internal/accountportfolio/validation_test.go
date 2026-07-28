package accountportfolio

import (
	"encoding/json"
	"testing"
)

func TestValidateDataAcceptsEveryDeclaredEmptyAccountShape(t *testing.T) {
	for _, path := range []string{
		SummaryPath,
		PointsPath,
		MembershipPath,
		NotificationsPath,
		TicketsPath,
		MembershipOrdersPath,
	} {
		raw, err := json.Marshal(validOwnerData(path))
		if err != nil {
			t.Fatal(err)
		}
		if err := validateData(path, raw); err != nil {
			t.Fatalf("%s valid data rejected: %v", path, err)
		}
	}
}

func TestValidateDataRejectsMissingAndInvalidContractValues(t *testing.T) {
	tests := []struct {
		name string
		path string
		body string
	}{
		{name: "summary missing required fields", path: SummaryPath, body: `{}`},
		{name: "summary negative balance", path: SummaryPath, body: `{"points_balance":-1,"plan":"free","lifetime":false,"unread_notification_count":0,"open_ticket_count":0}`},
		{name: "summary invalid plan", path: SummaryPath, body: `{"points_balance":0,"plan":"preview","lifetime":false,"unread_notification_count":0,"open_ticket_count":0}`},
		{name: "summary inconsistent lifetime", path: SummaryPath, body: `{"points_balance":0,"plan":"free","lifetime":true,"unread_notification_count":0,"open_ticket_count":0}`},
		{name: "points missing entries", path: PointsPath, body: `{"balance":0}`},
		{name: "membership lifetime type", path: MembershipPath, body: `{"plan":"free","lifetime":"false"}`},
		{name: "notification incomplete item", path: NotificationsPath, body: `{"notifications":[{}]}`},
		{name: "ticket invalid status", path: TicketsPath, body: `{"tickets":[{"id":"11111111-1111-4111-8111-111111111111","title":"t","category":"c","status":"preview","updated_at":"2026-07-28T00:00:00Z"}]}`},
		{name: "order invalid amount", path: MembershipOrdersPath, body: `{"orders":[{"id":"11111111-1111-4111-8111-111111111111","plan":"lifetime","amount_cents":1,"status":"paid","created_at":"2026-07-28T00:00:00Z"}]}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := validateData(test.path, json.RawMessage(test.body)); err != ErrInvalid {
				t.Fatalf("validateData(%s, %s) = %v, want ErrInvalid", test.path, test.body, err)
			}
		})
	}
}
