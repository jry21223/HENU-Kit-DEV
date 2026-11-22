package accountportfolio

import (
	"bytes"
	"context"
	"encoding/json"
	"net/url"
	"time"
)

func withQuery(path string, query url.Values) string {
	if encoded := query.Encode(); encoded != "" {
		return path + "?" + encoded
	}
	return path
}

func (c *Client) Points(ctx context.Context, actor, userID string, query url.Values) (json.RawMessage, error) {
	if !validUUID(userID) {
		return nil, ErrInvalid
	}
	return c.get(ctx, withQuery("/api/v1/console/points/"+url.PathEscape(userID), query), actor, validateConsolePoints)
}

func (c *Client) MembershipOrders(ctx context.Context, actor string, query url.Values) (json.RawMessage, error) {
	return c.get(ctx, withQuery("/api/v1/console/membership-orders", query), actor, validateConsoleOrders)
}

func validateConsolePoints(raw json.RawMessage) error {
	var data struct {
		Balance *int64 `json:"balance"`
		Entries []struct {
			ID        string    `json:"id"`
			Amount    int64     `json:"amount"`
			Reason    string    `json:"reason"`
			CreatedAt time.Time `json:"created_at"`
		} `json:"entries"`
		NextCursor *string `json:"next_cursor,omitempty"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&data); err != nil || data.Balance == nil || data.Entries == nil {
		return ErrInvalid
	}
	return nil
}

func validateConsoleOrders(raw json.RawMessage) error {
	var data struct {
		Orders []struct {
			ID string `json:"id"`
			UserID string `json:"user_id"`
			Plan string `json:"plan"`
			AmountCents int64 `json:"amount_cents"`
			Status string `json:"status"`
			Version int `json:"version"`
			CreatedAt time.Time `json:"created_at"`
			UpdatedAt time.Time `json:"updated_at"`
			PaidAt *time.Time `json:"paid_at"`
			ClosedAt *time.Time `json:"closed_at"`
			RefundedAt *time.Time `json:"refunded_at"`
			LatestRefund *struct {
				ID string `json:"id"`
				Status string `json:"status"`
				AmountCents int64 `json:"amount_cents"`
			} `json:"latest_refund"`
		} `json:"orders"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&data); err != nil || data.Orders == nil {
		return ErrInvalid
	}
	return nil
}
