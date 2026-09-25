package platformcore

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// QQBinding exposes only the Portal's three account-owned operations. Identity
// comes from the exchange Session; no browser-owned QQ or user ID is forwarded.
func (c *Client) QQBinding(ctx context.Context, action, sessionToken, token string) (int, json.RawMessage, error) {
	if action != "authorize" && action != "status" && action != "unlink" {
		return 0, nil, fmt.Errorf("invalid binding operation")
	}
	body, _ := json.Marshal(map[string]string{"session_token": sessionToken, "token": token})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/qq-bindings/"+action, bytes.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if err = c.signer.Sign(req); err != nil {
		return 0, nil, err
	}
	// No credential forwarding on redirects, including to another Core path.
	client := *c.httpClient
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	res, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 8193))
	if err != nil || len(raw) > 8192 || !json.Valid(raw) {
		return 0, nil, fmt.Errorf("invalid binding response")
	}
	if res.StatusCode != 200 && res.StatusCode != 400 && res.StatusCode != 403 && res.StatusCode != 404 && res.StatusCode != 409 && res.StatusCode != 410 && res.StatusCode != 429 {
		return 0, nil, fmt.Errorf("binding dependency unavailable")
	}
	return res.StatusCode, raw, nil
}
