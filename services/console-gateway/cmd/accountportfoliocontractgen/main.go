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
		"MembershipPath":              "getConsoleAccountMembership",
		"MembershipGrantsPath":        "grantConsoleAccountMembership",
		"MembershipRevocationsPath":   "revokeConsoleAccountMembership",
		"PointAdjustmentsPath":        "adjustConsoleAccountPoints",
		"TicketsPath":                 "getConsoleAccountTickets",
		"TicketPath":                  "getConsoleAccountTicket",
		"TicketRepliesPath":           "replyConsoleAccountTicket",
		"TicketTransitionsPath":       "transitionConsoleAccountTicket",
		"MembershipOrderClosuresPath": "closeConsoleMembershipOrder",
		"MembershipOrderRefundsPath":  "refundConsoleMembershipOrder",
		"MembershipOrderRefundPath":   "getConsoleMembershipOrderRefund",
	}
	for _, operationID := range required {
		if routes[operationID] == "" {
			fail(fmt.Errorf("required Account Portfolio Console operation %s is missing", operationID))
		}
	}
	digest := sha256.Sum256(source)
	generated := fmt.Sprintf(`// Code generated from account-portfolio.yaml (SHA256 %x); DO NOT EDIT.
package accountportfolio

const (
	MembershipPathTemplate = %q
	MembershipGrantsPathTemplate = %q
	MembershipRevocationsPathTemplate = %q
	PointAdjustmentsPath = %q
	TicketsPath = %q
	TicketPathTemplate = %q
	TicketRepliesPathTemplate = %q
	TicketTransitionsPathTemplate = %q
	MembershipOrderClosuresPathTemplate = %q
	MembershipOrderRefundsPathTemplate = %q
	MembershipOrderRefundPathTemplate = %q
)
`, digest, routes["getConsoleAccountMembership"], routes["grantConsoleAccountMembership"], routes["revokeConsoleAccountMembership"], routes["adjustConsoleAccountPoints"], routes["getConsoleAccountTickets"], routes["getConsoleAccountTicket"], routes["replyConsoleAccountTicket"], routes["transitionConsoleAccountTicket"], routes["closeConsoleMembershipOrder"], routes["refundConsoleMembershipOrder"], routes["getConsoleMembershipOrderRefund"])
	formatted, err := format.Source([]byte(generated))
	fail(err)
	fail(os.MkdirAll("internal/accountportfolio", 0o755))
	fail(os.WriteFile("internal/accountportfolio/contract_generated.go", formatted, 0o644))
}

func fail(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
