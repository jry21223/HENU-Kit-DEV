// Code generated from account-portfolio.yaml (SHA256 a5729e5c4a1003748f3e0014db9690e951346b6aa462d3d23f943d06ba80bff1); DO NOT EDIT.
package contract

const (
	HealthRoute                       = "/healthz"
	SummaryRoute                      = "/api/v1/account/summary"
	PointsRoute                       = "/api/v1/account/points"
	MembershipRoute                   = "/api/v1/account/membership"
	NotificationsRoute                = "/api/v1/account/notifications"
	NotificationReadRoute             = "/api/v1/account/notifications/{notification_id}/read"
	TicketsRoute                      = "/api/v1/account/tickets"
	TicketRoute                       = "/api/v1/account/tickets/{ticket_id}"
	TicketFollowUpsRoute              = "/api/v1/account/tickets/{ticket_id}/follow-ups"
	MembershipOrdersRoute             = "/api/v1/account/membership-orders"
	MembershipOrderCreateRoute        = "/api/v1/account/membership-orders"
	PaymentProviderNotificationRoute  = "/api/v1/payment-providers/{provider}/notifications"
	ConsoleMembershipRoute            = "/api/v1/console/memberships/{user_id}"
	ConsoleMembershipGrantsRoute      = "/api/v1/console/memberships/{user_id}/grants"
	ConsoleMembershipRevocationsRoute = "/api/v1/console/memberships/{user_id}/revocations"
	ConsoleTicketsRoute               = "/api/v1/console/tickets"
	ConsoleTicketRoute                = "/api/v1/console/tickets/{ticket_id}"
	ConsoleTicketRepliesRoute         = "/api/v1/console/tickets/{ticket_id}/replies"
	ConsoleTicketTransitionsRoute     = "/api/v1/console/tickets/{ticket_id}/transitions"
)
