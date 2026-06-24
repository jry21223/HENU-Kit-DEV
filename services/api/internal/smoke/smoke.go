package smoke

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
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
	BaseURL            string
	Email              string
	Code               string
	Name               string
	AdminEmail         string
	AdminCode          string
	AdminName          string
	PackageID          string
	SkipLogin          bool
	CreateOrder        bool
	MockWeChatPay      bool
	MockWeChatSecret   string
	ExpectPaidDenied   bool
	GrantPackageAccess bool
	Timeout            time.Duration
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

type userData struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Role  string `json:"role"`
}

type orderData struct {
	Order        *order `json:"order"`
	AlreadyOwned bool   `json:"alreadyOwned"`
}

type order struct {
	ID          string `json:"id"`
	OutTradeNo  string `json:"outTradeNo"`
	Status      string `json:"status"`
	AmountTotal int64  `json:"amountTotal"`
	Currency    string `json:"currency"`
}

type orderStatusData struct {
	OrderID            string `json:"orderId"`
	Status             string `json:"status"`
	EntitlementGranted bool   `json:"entitlementGranted"`
}

type nativePaymentData struct {
	OrderID     string `json:"orderId"`
	CodeURL     string `json:"codeUrl"`
	Status      string `json:"status"`
	AmountTotal int64  `json:"amountTotal"`
	Mock        bool   `json:"mock"`
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
	if cfg.AdminName == "" {
		cfg.AdminName = "Smoke Admin"
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
	userID := ""
	if !r.cfg.SkipLogin {
		token, err = r.login(ctx, r.cfg.Email, r.cfg.Code, r.cfg.Name)
		if err != nil {
			return fail("email login", 0, err)
		}
		result.add("email login", "passed", http.StatusOK, r.cfg.Email)
		user, status, err := r.currentUser(ctx, token)
		if err != nil {
			return fail("auth me", status, err)
		}
		userID = user.ID
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

	if r.cfg.CreateOrder || r.cfg.MockWeChatPay {
		if token == "" {
			return fail("create order", 0, errors.New("create-order requires login"))
		}
		createdOrder, status, err := r.createOrder(ctx, packageID, token)
		if err != nil {
			return fail("create order", status, err)
		}
		result.add("create order", "passed", status, createdOrder.ID)
		status, orderStatus, err := r.orderStatus(ctx, createdOrder.ID, token)
		if err != nil {
			return fail("order status", status, err)
		}
		result.add("order status", "passed", status, orderStatus)

		if r.cfg.MockWeChatPay {
			if strings.TrimSpace(r.cfg.MockWeChatSecret) == "" {
				return fail("mock wechat notify", 0, errors.New("mock-wechat-secret is required for -mock-wechat-pay"))
			}
			native, status, err := r.createNativePayment(ctx, createdOrder.ID, token)
			if err != nil {
				return fail("mock wechat native", status, err)
			}
			result.add("mock wechat native", "passed", status, native.CodeURL)
			status, err = r.mockWeChatNotify(ctx, createdOrder, r.cfg.MockWeChatSecret)
			if err != nil {
				return fail("mock wechat notify", status, err)
			}
			result.add("mock wechat notify", "passed", status, "signed SUCCESS callback accepted")
			status, paidStatus, err := r.orderStatusDetail(ctx, createdOrder.ID, token)
			if err != nil {
				return fail("paid order status", status, err)
			}
			if paidStatus.Status != "paid" || !paidStatus.EntitlementGranted {
				return fail("paid order status", status, fmt.Errorf("expected paid with entitlement, got status=%s entitlement=%v", paidStatus.Status, paidStatus.EntitlementGranted))
			}
			result.add("paid order status", "passed", status, "paid with entitlement")
			status, bodySize, err := r.download(ctx, paidMaterialID, token)
			if err != nil {
				return fail("paid download after mock payment", status, err)
			}
			result.add("paid download after mock payment", "passed", status, fmt.Sprintf("%d byte(s)", bodySize))
		}
	}

	if r.cfg.GrantPackageAccess {
		if token == "" || userID == "" {
			return fail("manual package grant", 0, errors.New("grant-package-access requires smoke user login"))
		}
		if strings.TrimSpace(r.cfg.AdminEmail) == "" {
			return fail("manual package grant", 0, errors.New("admin email is required for grant-package-access"))
		}
		adminToken, err := r.login(ctx, r.cfg.AdminEmail, r.cfg.AdminCode, r.cfg.AdminName)
		if err != nil {
			return fail("admin login", 0, err)
		}
		result.add("admin login", "passed", http.StatusOK, r.cfg.AdminEmail)
		status, err := r.grantPackageAccess(ctx, userID, packageID, adminToken)
		if err != nil {
			return fail("manual package grant", status, err)
		}
		result.add("manual package grant", "passed", status, "admin granted selected package to smoke user")
		status, bodySize, err := r.download(ctx, paidMaterialID, token)
		if err != nil {
			return fail("paid download after grant", status, err)
		}
		result.add("paid download after grant", "passed", status, fmt.Sprintf("%d byte(s)", bodySize))
	}

	result.DurationMillis = time.Since(started).Milliseconds()
	return result
}

func (r Runner) login(ctx context.Context, email string, code string, name string) (string, error) {
	email = strings.TrimSpace(email)
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
	code = strings.TrimSpace(code)
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
	loginBody := map[string]string{"email": email, "code": code, "name": name}
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

func (r Runner) currentUser(ctx context.Context, token string) (userData, int, error) {
	status, raw, err := r.request(ctx, http.MethodGet, "/auth/me", nil, token)
	if err != nil || status != http.StatusOK {
		if err == nil {
			err = fmt.Errorf("unexpected auth/me status %d: %s", status, raw.Message)
		}
		return userData{}, status, err
	}
	var user userData
	if err := json.Unmarshal(raw.Data, &user); err != nil {
		return userData{}, status, err
	}
	if strings.TrimSpace(user.ID) == "" {
		return userData{}, status, errors.New("auth/me response did not include user id")
	}
	return user, status, nil
}

func (r Runner) createOrder(ctx context.Context, packageID string, token string) (order, int, error) {
	body := map[string]string{"packageId": packageID}
	status, raw, err := r.request(ctx, http.MethodPost, "/orders", body, token)
	if err != nil || status != http.StatusOK {
		if err == nil {
			err = fmt.Errorf("unexpected create order status %d: %s", status, raw.Message)
		}
		return order{}, status, err
	}
	var data orderData
	if err := json.Unmarshal(raw.Data, &data); err != nil {
		return order{}, status, err
	}
	if data.AlreadyOwned {
		return order{}, status, errors.New("user already owns selected package; use a fresh smoke email")
	}
	if data.Order == nil || data.Order.ID == "" {
		return order{}, status, errors.New("create order response did not include order.id")
	}
	if data.Order.AmountTotal < 0 {
		return order{}, status, errors.New("order amount was negative")
	}
	if data.Order.OutTradeNo == "" && r.cfg.MockWeChatPay {
		return order{}, status, errors.New("mock wechat payment requires order.outTradeNo")
	}
	return *data.Order, status, nil
}

func (r Runner) orderStatus(ctx context.Context, orderID string, token string) (int, string, error) {
	status, data, err := r.orderStatusDetail(ctx, orderID, token)
	if err != nil {
		return status, "", err
	}
	return status, data.Status, nil
}

func (r Runner) orderStatusDetail(ctx context.Context, orderID string, token string) (int, orderStatusData, error) {
	status, raw, err := r.request(ctx, http.MethodGet, "/orders/"+orderID+"/status", nil, token)
	if err != nil || status != http.StatusOK {
		if err == nil {
			err = fmt.Errorf("unexpected order status code %d: %s", status, raw.Message)
		}
		return status, orderStatusData{}, err
	}
	var data orderStatusData
	if err := json.Unmarshal(raw.Data, &data); err != nil {
		return status, orderStatusData{}, err
	}
	if data.OrderID != orderID {
		return status, orderStatusData{}, errors.New("order status returned different order id")
	}
	if data.Status == "" {
		return status, orderStatusData{}, errors.New("order status response missing status")
	}
	return status, data, nil
}

func (r Runner) createNativePayment(ctx context.Context, orderID string, token string) (nativePaymentData, int, error) {
	body := map[string]string{"orderId": orderID}
	status, raw, err := r.request(ctx, http.MethodPost, "/payments/wechat/native", body, token)
	if err != nil || status != http.StatusOK {
		if err == nil {
			err = fmt.Errorf("unexpected wechat native status %d: %s", status, raw.Message)
		}
		return nativePaymentData{}, status, err
	}
	var data nativePaymentData
	if err := json.Unmarshal(raw.Data, &data); err != nil {
		return nativePaymentData{}, status, err
	}
	if data.OrderID != orderID {
		return nativePaymentData{}, status, errors.New("wechat native response returned different order id")
	}
	if data.CodeURL == "" {
		return nativePaymentData{}, status, errors.New("wechat native response missing codeUrl")
	}
	if data.Status != "paying" {
		return nativePaymentData{}, status, fmt.Errorf("expected native payment status paying, got %s", data.Status)
	}
	if !data.Mock {
		return nativePaymentData{}, status, errors.New("mock-wechat-pay expected mock native response; refuse to treat live payment as smoke")
	}
	return data, status, nil
}

func (r Runner) mockWeChatNotify(ctx context.Context, order order, secret string) (int, error) {
	transactionID := fmt.Sprintf("SMOKE_%d", time.Now().UTC().UnixNano())
	body := []byte(fmt.Sprintf(`{"outTradeNo":"%s","transactionId":"%s","tradeState":"SUCCESS","amountTotal":%d}`, order.OutTradeNo, transactionID, order.AmountTotal))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.cfg.BaseURL+"/payments/wechat/notify", bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-WeChat-Mock-Signature", mockNotifySignature(body, secret))
	res, err := r.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()
	rawBody, err := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	if err != nil {
		return res.StatusCode, err
	}
	if res.StatusCode != http.StatusOK {
		return res.StatusCode, fmt.Errorf("unexpected mock notify status %d: %s", res.StatusCode, strings.TrimSpace(string(rawBody)))
	}
	if !bytes.Contains(rawBody, []byte(`"code":"SUCCESS"`)) {
		return res.StatusCode, fmt.Errorf("mock notify response did not report SUCCESS: %s", strings.TrimSpace(string(rawBody)))
	}
	return res.StatusCode, nil
}

func (r Runner) grantPackageAccess(ctx context.Context, userID string, packageID string, adminToken string) (int, error) {
	body := map[string]string{"userId": userID, "packageId": packageID}
	status, raw, err := r.request(ctx, http.MethodPost, "/admin/access-grants", body, adminToken)
	if err != nil || status != http.StatusOK {
		if err == nil {
			err = fmt.Errorf("unexpected access grant status %d: %s", status, raw.Message)
		}
		return status, err
	}
	return status, nil
}

func (r Runner) download(ctx context.Context, materialID string, token string) (int, int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.cfg.BaseURL+"/materials/"+materialID+"/download", nil)
	if err != nil {
		return 0, 0, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := r.client.Do(req)
	if err != nil {
		return 0, 0, err
	}
	defer res.Body.Close()
	bodySize, err := io.Copy(io.Discard, io.LimitReader(res.Body, 20<<20))
	if err != nil {
		return res.StatusCode, bodySize, err
	}
	if res.StatusCode != http.StatusOK {
		return res.StatusCode, bodySize, fmt.Errorf("expected paid download 200 after grant, got %d", res.StatusCode)
	}
	if bodySize <= 0 {
		return res.StatusCode, bodySize, errors.New("download response was empty")
	}
	return res.StatusCode, bodySize, nil
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

func mockNotifySignature(body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}
