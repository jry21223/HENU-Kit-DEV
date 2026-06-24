package smoke

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Config struct {
	BaseURL          string
	Email            string
	Code             string
	Name             string
	PackageID        string
	SkipLogin        bool
	CreateOrder      bool
	ExpectPaidDenied bool
	Timeout          time.Duration
}

type Result struct {
	BaseURL        string        `json:"baseUrl"`
	StartedAt      time.Time     `json:"startedAt"`
	DurationMillis int64         `json:"durationMillis"`
	PackageID      string        `json:"packageId,omitempty"`
	PaidMaterialID string        `json:"paidMaterialId,omitempty"`
	Checks         []CheckResult `json:"checks"`
	Passed         bool          `json:"passed"`
}

type CheckResult struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	Detail     string `json:"detail,omitempty"`
	HTTPStatus int    `json:"httpStatus,omitempty"`
}

type Runner struct {
	cfg    Config
	client *http.Client
}

type envelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
	Details json.RawMessage `json:"details"`
}

type packageListData struct {
	Packages []coursePackage `json:"packages"`
}

type packageDetailData struct {
	Package   coursePackage       `json:"package"`
	Materials []material          `json:"materials"`
	Items     []map[string]string `json:"items"`
}

type coursePackage struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Status   string `json:"status"`
	PriceFen int64  `json:"priceFen"`
}

type material struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	AccessLevel string `json:"accessLevel"`
	Status      string `json:"status"`
}

type loginData struct {
	AccessToken string `json:"accessToken"`
}

type orderData struct {
	Order        *order `json:"order"`
	AlreadyOwned bool   `json:"alreadyOwned"`
}

type order struct {
	ID          string `json:"id"`
	Status      string `json:"status"`
	AmountTotal int64  `json:"amountTotal"`
	Currency    string `json:"currency"`
}

type orderStatusData struct {
	OrderID            string `json:"orderId"`
	Status             string `json:"status"`
	EntitlementGranted bool   `json:"entitlementGranted"`
}

func NewRunner(cfg Config) (Runner, error) {
	cfg.BaseURL = strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if cfg.BaseURL == "" {
		cfg.BaseURL = "http://localhost:8080/api/v1"
	}
	if _, err := url.ParseRequestURI(cfg.BaseURL); err != nil {
		return Runner{}, fmt.Errorf("invalid base url: %w", err)
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 15 * time.Second
	}
	if cfg.Name == "" {
		cfg.Name = "Smoke Tester"
	}
	return Runner{cfg: cfg, client: &http.Client{Timeout: cfg.Timeout}}, nil
}

func (r Runner) Run(ctx context.Context) Result {
	started := time.Now().UTC()
	result := Result{BaseURL: r.cfg.BaseURL, StartedAt: started, Passed: true}
	fail := func(name string, status int, err error) Result {
		result.add(name, "failed", status, err.Error())
		result.Passed = false
		result.DurationMillis = time.Since(started).Milliseconds()
		return result
	}

	if status, _, err := r.request(ctx, http.MethodGet, "/readyz", nil, ""); err != nil || status != http.StatusOK {
		if err == nil {
			err = fmt.Errorf("unexpected readiness status %d", status)
		}
		return fail("api readiness", status, err)
	}
	result.add("api readiness", "passed", http.StatusOK, "/readyz returned ready")

	if status, _, err := r.request(ctx, http.MethodGet, "/schools", nil, ""); err != nil || status != http.StatusOK {
		if err == nil {
			err = fmt.Errorf("unexpected schools status %d", status)
		}
		return fail("public schools", status, err)
	}
	result.add("public schools", "passed", http.StatusOK, "schools endpoint returned ok")

	status, packagesRaw, err := r.request(ctx, http.MethodGet, "/packages", nil, "")
	if err != nil || status != http.StatusOK {
		if err == nil {
			err = fmt.Errorf("unexpected packages status %d", status)
		}
		return fail("public packages", status, err)
	}
	var packages packageListData
	if err := json.Unmarshal(packagesRaw.Data, &packages); err != nil {
		return fail("public packages", status, err)
	}
	if len(packages.Packages) == 0 {
		return fail("public packages", status, errors.New("no published course packages returned"))
	}
	result.add("public packages", "passed", status, fmt.Sprintf("%d published package(s)", len(packages.Packages)))

	packageID := strings.TrimSpace(r.cfg.PackageID)
	if packageID == "" {
		packageID = packages.Packages[0].ID
	}
	result.PackageID = packageID
	status, detailRaw, err := r.request(ctx, http.MethodGet, "/packages/"+packageID, nil, "")
	if err != nil || status != http.StatusOK {
		if err == nil {
			err = fmt.Errorf("unexpected package detail status %d", status)
		}
		return fail("package detail", status, err)
	}
	if bytes.Contains(detailRaw.Data, []byte("storageKey")) {
		return fail("package detail hides storage keys", status, errors.New("package detail leaked storageKey"))
	}
	var detail packageDetailData
	if err := json.Unmarshal(detailRaw.Data, &detail); err != nil {
		return fail("package detail", status, err)
	}
	result.add("package detail", "passed", status, fmt.Sprintf("%d published material(s)", len(detail.Materials)))
	result.add("package detail hides storage keys", "passed", status, "storageKey absent from public package detail")

	paidMaterialID := firstPaidMaterialID(detail.Materials)
	if paidMaterialID == "" {
		return fail("paid material presence", status, errors.New("selected package has no paid or member_only material"))
	}
	result.PaidMaterialID = paidMaterialID
	result.add("paid material presence", "passed", status, paidMaterialID)

	token := ""
	if !r.cfg.SkipLogin {
		token, err = r.login(ctx)
		if err != nil {
			return fail("email login", 0, err)
		}
		result.add("email login", "passed", http.StatusOK, r.cfg.Email)
		status, _, err = r.request(ctx, http.MethodGet, "/auth/me", nil, token)
		if err != nil || status != http.StatusOK {
			if err == nil {
				err = fmt.Errorf("unexpected auth/me status %d", status)
			}
			return fail("auth me", status, err)
		}
		result.add("auth me", "passed", status, "current user returned")
	}

	if r.cfg.ExpectPaidDenied && token != "" {
		status, raw, err := r.request(ctx, http.MethodGet, "/materials/"+paidMaterialID+"/download", nil, token)
		if err != nil && status == 0 {
			return fail("paid download denied before entitlement", status, err)
		}
		if status != http.StatusForbidden {
			detail := fmt.Sprintf("expected 403 entitlement_required, got %d", status)
			if raw.Message != "" {
				detail += ": " + raw.Message
			}
			return fail("paid download denied before entitlement", status, errors.New(detail))
		}
		result.add("paid download denied before entitlement", "passed", status, "paid asset denied without entitlement")
	}

	if r.cfg.CreateOrder {
		if token == "" {
			return fail("create order", 0, errors.New("create-order requires login"))
		}
		orderID, status, err := r.createOrder(ctx, packageID, token)
		if err != nil {
			return fail("create order", status, err)
		}
		result.add("create order", "passed", status, orderID)
		status, orderStatus, err := r.orderStatus(ctx, orderID, token)
		if err != nil {
			return fail("order status", status, err)
		}
		result.add("order status", "passed", status, orderStatus)
	}

	result.DurationMillis = time.Since(started).Milliseconds()
	return result
}

func (r Runner) login(ctx context.Context) (string, error) {
	email := strings.TrimSpace(r.cfg.Email)
	if email == "" {
		return "", errors.New("email is required unless -skip-login is set")
	}
	sendBody := map[string]string{"email": email}
	status, sendRaw, err := r.request(ctx, http.MethodPost, "/auth/send-code", sendBody, "")
	if err != nil || status != http.StatusOK {
		if err == nil {
			err = fmt.Errorf("unexpected send-code status %d: %s", status, sendRaw.Message)
		}
		return "", err
	}
	code := strings.TrimSpace(r.cfg.Code)
	if code == "" {
		var sendData struct {
			DevCode string `json:"devCode"`
		}
		_ = json.Unmarshal(sendRaw.Data, &sendData)
		code = strings.TrimSpace(sendData.DevCode)
	}
	if code == "" {
		return "", errors.New("verification code is required; pass -code for non-development environments")
	}
	loginBody := map[string]string{"email": email, "code": code, "name": r.cfg.Name}
	status, loginRaw, err := r.request(ctx, http.MethodPost, "/auth/login", loginBody, "")
	if err != nil || status != http.StatusOK {
		if err == nil {
			err = fmt.Errorf("unexpected login status %d: %s", status, loginRaw.Message)
		}
		return "", err
	}
	var login loginData
	if err := json.Unmarshal(loginRaw.Data, &login); err != nil {
		return "", err
	}
	if strings.TrimSpace(login.AccessToken) == "" {
		return "", errors.New("login response did not include accessToken")
	}
	return login.AccessToken, nil
}

func (r Runner) createOrder(ctx context.Context, packageID string, token string) (string, int, error) {
	body := map[string]string{"packageId": packageID}
	status, raw, err := r.request(ctx, http.MethodPost, "/orders", body, token)
	if err != nil || status != http.StatusOK {
		if err == nil {
			err = fmt.Errorf("unexpected create order status %d: %s", status, raw.Message)
		}
		return "", status, err
	}
	var data orderData
	if err := json.Unmarshal(raw.Data, &data); err != nil {
		return "", status, err
	}
	if data.AlreadyOwned {
		return "", status, errors.New("user already owns selected package; use a fresh smoke email")
	}
	if data.Order == nil || data.Order.ID == "" {
		return "", status, errors.New("create order response did not include order.id")
	}
	if data.Order.AmountTotal < 0 {
		return "", status, errors.New("order amount was negative")
	}
	return data.Order.ID, status, nil
}

func (r Runner) orderStatus(ctx context.Context, orderID string, token string) (int, string, error) {
	status, raw, err := r.request(ctx, http.MethodGet, "/orders/"+orderID+"/status", nil, token)
	if err != nil || status != http.StatusOK {
		if err == nil {
			err = fmt.Errorf("unexpected order status code %d: %s", status, raw.Message)
		}
		return status, "", err
	}
	var data orderStatusData
	if err := json.Unmarshal(raw.Data, &data); err != nil {
		return status, "", err
	}
	if data.OrderID != orderID {
		return status, "", errors.New("order status returned different order id")
	}
	if data.Status == "" {
		return status, "", errors.New("order status response missing status")
	}
	return status, data.Status, nil
}

func (r Runner) request(ctx context.Context, method string, path string, body interface{}, token string) (int, envelope, error) {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return 0, envelope{}, err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, r.cfg.BaseURL+path, reader)
	if err != nil {
		return 0, envelope{}, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := r.client.Do(req)
	if err != nil {
		return 0, envelope{}, err
	}
	defer res.Body.Close()
	rawBody, err := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	if err != nil {
		return res.StatusCode, envelope{}, err
	}
	var raw envelope
	if len(rawBody) > 0 {
		if err := json.Unmarshal(rawBody, &raw); err != nil {
			if res.StatusCode >= 400 {
				raw.Message = strings.TrimSpace(string(rawBody))
			} else {
				return res.StatusCode, raw, err
			}
		}
	}
	if res.StatusCode >= 200 && res.StatusCode < 300 && raw.Code != 0 {
		return res.StatusCode, raw, fmt.Errorf("unexpected envelope code %d: %s", raw.Code, raw.Message)
	}
	return res.StatusCode, raw, nil
}

func (r *Result) add(name string, status string, httpStatus int, detail string) {
	r.Checks = append(r.Checks, CheckResult{Name: name, Status: status, HTTPStatus: httpStatus, Detail: detail})
}

func firstPaidMaterialID(materials []material) string {
	for _, material := range materials {
		switch material.AccessLevel {
		case "paid", "member_only":
			return material.ID
		}
	}
	return ""
}
