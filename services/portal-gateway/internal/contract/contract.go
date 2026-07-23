// Package contract defines Portal Gateway API types.
// Only types actually used by the gateway handler are defined here.
// Product data is proxied to portal-api unchanged (io.Copy).
package contract

import "time"

// PortalSession is the authenticated user context returned to the browser.
type PortalSession struct {
	UserID    string    `json:"user_id"`
	ExpiresAt time.Time `json:"expires_at"`
}

// ErrorEnvelope is the standard error response.
type ErrorEnvelope struct {
	Error     string `json:"error"`
	Detail    string `json:"detail,omitempty"`
	RequestID string `json:"request_id"`
}
