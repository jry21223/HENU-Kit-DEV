package main

import (
	"crypto/sha256"
	"fmt"
	"go/format"
	"os"

	"gopkg.in/yaml.v3"
)

type document struct {
	Paths map[string]map[string]struct {
		OperationID string `yaml:"operationId"`
	} `yaml:"paths"`
}

func main() {
	source, err := os.ReadFile("../../packages/api-contracts/openapi/account-portfolio.yaml")
	fail(err)
	var spec document
	fail(yaml.Unmarshal(source, &spec))
	routes := map[string]string{}
	for path, methods := range spec.Paths {
		for _, operation := range methods {
			routes[operation.OperationID] = path
		}
	}
	required := map[string]string{
		"HealthRoute":                       "getAccountPortfolioHealth",
		"SummaryRoute":                      "getAccountSummary",
		"PointsRoute":                       "getAccountPoints",
		"MembershipRoute":                   "getAccountMembership",
		"NotificationsRoute":                "getAccountNotifications",
		"NotificationReadRoute":             "markAccountNotificationRead",
		"TicketsRoute":                      "getAccountTickets",
		"TicketRoute":                       "getAccountTicket",
		"TicketFollowUpsRoute":              "createAccountTicketFollowUp",
		"MembershipOrdersRoute":             "getAccountMembershipOrders",
		"MembershipOrderCreateRoute":        "createAccountMembershipOrder",
		"PaymentProviderNotificationRoute":  "recordAccountPaymentProviderNotification",
		"ConsoleMembershipRoute":            "getConsoleAccountMembership",
		"ConsoleMembershipGrantsRoute":      "grantConsoleAccountMembership",
		"ConsoleMembershipRevocationsRoute": "revokeConsoleAccountMembership",
		"ConsolePointAdjustmentsRoute":      "adjustConsoleAccountPoints",
		"ConsoleTicketsRoute":               "getConsoleAccountTickets",
		"ConsoleTicketRoute":                "getConsoleAccountTicket",
		"ConsoleTicketRepliesRoute":         "replyConsoleAccountTicket",
		"ConsoleTicketTransitionsRoute":     "transitionConsoleAccountTicket",
	}
	for name, operationID := range required {
		if routes[operationID] == "" {
			fail(fmt.Errorf("required Account Portfolio operation %s is missing", name))
		}
	}
	digest := sha256.Sum256(source)
	generated := fmt.Sprintf(`// Code generated from account-portfolio.yaml (SHA256 %x); DO NOT EDIT.
package contract

const (
	HealthRoute = %q
	SummaryRoute = %q
	PointsRoute = %q
	MembershipRoute = %q
	NotificationsRoute = %q
	NotificationReadRoute = %q
	TicketsRoute = %q
	TicketRoute = %q
	TicketFollowUpsRoute = %q
	MembershipOrdersRoute = %q
	MembershipOrderCreateRoute = %q
	PaymentProviderNotificationRoute = %q
	ConsoleMembershipRoute = %q
	ConsoleMembershipGrantsRoute = %q
	ConsoleMembershipRevocationsRoute = %q
	ConsolePointAdjustmentsRoute = %q
	ConsoleTicketsRoute = %q
	ConsoleTicketRoute = %q
	ConsoleTicketRepliesRoute = %q
	ConsoleTicketTransitionsRoute = %q
)
	`, digest, routes["getAccountPortfolioHealth"], routes["getAccountSummary"], routes["getAccountPoints"], routes["getAccountMembership"], routes["getAccountNotifications"], routes["markAccountNotificationRead"], routes["getAccountTickets"], routes["getAccountTicket"], routes["createAccountTicketFollowUp"], routes["getAccountMembershipOrders"], routes["createAccountMembershipOrder"], routes["recordAccountPaymentProviderNotification"], routes["getConsoleAccountMembership"], routes["grantConsoleAccountMembership"], routes["revokeConsoleAccountMembership"], routes["adjustConsoleAccountPoints"], routes["getConsoleAccountTickets"], routes["getConsoleAccountTicket"], routes["replyConsoleAccountTicket"], routes["transitionConsoleAccountTicket"])
	formatted, err := format.Source([]byte(generated))
	fail(err)
	fail(os.MkdirAll("internal/contract", 0o755))
	fail(os.WriteFile("internal/contract/generated.go", formatted, 0o644))
}

func fail(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
