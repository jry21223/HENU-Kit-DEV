package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"henukit.dev/portal-gateway/internal/config"
	"henukit.dev/portal-gateway/internal/httpapi"
)

const (
	coreAddress    = "127.0.0.1:3211"
	gatewayAddress = "127.0.0.1:3210"
	portalOrigin   = "http://127.0.0.1:3111"
	clientID       = "portal-gateway"
	clientSecret   = "portal-e2e-client-secret-with-enough-entropy"
	keyID          = "primary"
	redirectURI    = portalOrigin + "/api/v1/auth/callback"
	testEmail      = "student@henu.edu.cn"
	testPassword   = "correct horse battery staple"
)

type continuation struct {
	state, challenge, redirectURI, device string
}

type coreFixture struct {
	mu            sync.Mutex
	continuations map[string]continuation
	codes         map[string]continuation
}

func main() {
	redisServer, err := miniredis.Run()
	if err != nil {
		log.Fatal(err)
	}
	defer redisServer.Close()
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	defer func() { _ = redisClient.Close() }()

	core := &coreFixture{continuations: map[string]continuation{}, codes: map[string]continuation{}}
	go func() {
		log.Printf("OAuth E2E Platform fixture listening on %s", coreAddress)
		if err := http.ListenAndServe(coreAddress, core); err != nil {
			log.Fatal(err)
		}
	}()

	handler, err := httpapi.New(config.Config{
		PlatformCoreURL:        "http://" + coreAddress,
		PlatformCorePublicURL:  "http://" + coreAddress,
		PlatformClientID:       clientID,
		PlatformSecret:         clientSecret,
		PlatformKeyID:          keyID,
		PortalRedirectURI:      redirectURI,
		PortalOrigin:           portalOrigin,
		SessionKey:             []byte("0123456789abcdef0123456789abcdef"),
		LocalOAuthCookieName:   "henukit_portal_oauth_e2e",
		LocalSessionCookieName: "henukit_portal_session_e2e",
	}, redisClient)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("OAuth E2E Portal Gateway listening on %s", gatewayAddress)
	log.Fatal(http.ListenAndServe(gatewayAddress, handler.Router()))
}

func (f *coreFixture) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	switch request.URL.Path {
	case "/api/v1/oauth/authorize":
		f.authorize(writer, request)
	case "/account/bootstrap":
		f.bootstrap(writer, request)
	case "/login/password":
		f.passwordLogin(writer, request)
	case "/account/continuation/resume":
		f.resume(writer, request)
	case "/api/v1/oauth/token":
		f.exchange(writer, request)
	default:
		http.NotFound(writer, request)
	}
}

func (f *coreFixture) authorize(writer http.ResponseWriter, request *http.Request) {
	query := request.URL.Query()
	if query.Get("response_type") != "code" || query.Get("client_id") != clientID ||
		query.Get("redirect_uri") != redirectURI || query.Get("code_challenge_method") != "S256" ||
		len(query.Get("state")) < 8 || len(query.Get("code_challenge")) != 43 {
		http.Error(writer, "invalid authorize", http.StatusBadRequest)
		return
	}
	device := cookieValue(request, "henukit_core_device_e2e")
	if device == "" {
		device = randomToken(16)
		http.SetCookie(writer, browserCookie("henukit_core_device_e2e", device, 1800))
	}
	handle := randomToken(32)
	f.mu.Lock()
	f.continuations[handle] = continuation{
		state: query.Get("state"), challenge: query.Get("code_challenge"),
		redirectURI: query.Get("redirect_uri"), device: device,
	}
	f.mu.Unlock()
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Referrer-Policy", "no-referrer")
	http.Redirect(writer, request, portalOrigin+"/account/login?continuation="+url.QueryEscape(handle), http.StatusFound)
}

func (f *coreFixture) bootstrap(writer http.ResponseWriter, request *http.Request) {
	handle := request.URL.Query().Get("continuation")
	device := cookieValue(request, "henukit_core_device_e2e")
	f.mu.Lock()
	stored, ok := f.continuations[handle]
	f.mu.Unlock()
	if !ok || stored.device != device || request.URL.Query().Get("flow") != "login" {
		writeJSON(writer, http.StatusGone, map[string]any{
			"error":      map[string]string{"code": "OAUTH_CONTINUATION_UNAVAILABLE", "message": "unavailable"},
			"request_id": "req_e2e_continuation_unavailable",
		})
		return
	}
	csrf := randomToken(32)
	http.SetCookie(writer, browserCookie("henukit_core_csrf_e2e", csrf, 600))
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Referrer-Policy", "no-referrer")
	writeJSON(writer, http.StatusOK, map[string]any{
		"data": map[string]any{
			"flow": "login", "csrf_token": csrf,
			"continuation": map[string]any{"available": true, "product_name": "HENU Kit"},
		},
		"request_id": "req_e2e_continuation_bootstrap",
	})
}

func (f *coreFixture) passwordLogin(writer http.ResponseWriter, request *http.Request) {
	if request.Header.Get("X-Henukit-Form-Response") != "status" || request.ParseForm() != nil ||
		request.FormValue("csrf_token") == "" || request.FormValue("csrf_token") != cookieValue(request, "henukit_core_csrf_e2e") ||
		request.FormValue("email") != testEmail || request.FormValue("password") != testPassword {
		writeJSON(writer, http.StatusUnauthorized, map[string]any{
			"error":      map[string]string{"code": "AUTHENTICATION_FAILED", "message": "authentication failed"},
			"request_id": "req_e2e_authentication_failed",
		})
		return
	}
	http.SetCookie(writer, browserCookie("henukit_core_session_e2e", randomToken(32), 1800))
	writer.WriteHeader(http.StatusNoContent)
}

func (f *coreFixture) resume(writer http.ResponseWriter, request *http.Request) {
	if request.ParseForm() != nil || request.FormValue("csrf_token") == "" ||
		request.FormValue("csrf_token") != cookieValue(request, "henukit_core_csrf_e2e") ||
		cookieValue(request, "henukit_core_session_e2e") == "" {
		http.Redirect(writer, request, portalOrigin+"/account/login?continuation_error=expired&request_id=req_e2e_resume_rejected", http.StatusSeeOther)
		return
	}
	handle := request.FormValue("continuation")
	device := cookieValue(request, "henukit_core_device_e2e")
	f.mu.Lock()
	stored, ok := f.continuations[handle]
	if ok && stored.device == device {
		delete(f.continuations, handle)
	}
	f.mu.Unlock()
	if !ok || stored.device != device {
		http.Redirect(writer, request, portalOrigin+"/account/login?continuation_error=expired&request_id=req_e2e_resume_unavailable", http.StatusSeeOther)
		return
	}
	code := randomToken(32)
	f.mu.Lock()
	f.codes[code] = stored
	f.mu.Unlock()
	callback, _ := url.Parse(stored.redirectURI)
	query := callback.Query()
	query.Set("code", code)
	query.Set("state", stored.state)
	callback.RawQuery = query.Encode()
	http.Redirect(writer, request, callback.String(), http.StatusSeeOther)
}

func (f *coreFixture) exchange(writer http.ResponseWriter, request *http.Request) {
	body, err := io.ReadAll(io.LimitReader(request.Body, 16<<10))
	if err != nil || !validServiceSignature(request, body) {
		http.Error(writer, "unauthorized", http.StatusUnauthorized)
		return
	}
	var payload map[string]string
	if json.Unmarshal(body, &payload) != nil || payload["grant_type"] != "authorization_code" ||
		payload["client_id"] != clientID || payload["redirect_uri"] != redirectURI {
		http.Error(writer, "invalid exchange", http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	stored, ok := f.codes[payload["code"]]
	if ok {
		delete(f.codes, payload["code"])
	}
	f.mu.Unlock()
	verifierHash := sha256.Sum256([]byte(payload["code_verifier"]))
	if !ok || base64.RawURLEncoding.EncodeToString(verifierHash[:]) != stored.challenge {
		http.Error(writer, "invalid code", http.StatusBadRequest)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"data": map[string]any{
			"user": map[string]string{
				"user_id": "171f1c6f-7b10-4c92-91a2-b39bf5af5302", "display_name": "小河同学",
			},
			"session_exchange_token": "portal_e2e_exchange_token_with_32_characters",
			"expires_at":             time.Now().UTC().Add(8 * time.Hour),
		},
		"request_id": "req_e2e_exchange",
	})
}

func validServiceSignature(request *http.Request, body []byte) bool {
	username, password, ok := request.BasicAuth()
	if !ok || username != clientID || password != clientSecret ||
		request.Header.Get("X-Service-Id") != clientID || request.Header.Get("X-Key-Id") != keyID {
		return false
	}
	digest := sha256.Sum256(body)
	canonical := strings.Join([]string{
		request.Method, request.URL.RequestURI(), request.Header.Get("X-Timestamp"),
		request.Header.Get("X-Nonce"), hex.EncodeToString(digest[:]),
	}, "\n")
	mac := hmac.New(sha256.New, []byte(clientSecret))
	_, _ = mac.Write([]byte(canonical))
	expected := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(request.Header.Get("X-Signature")))
}

func browserCookie(name, value string, maxAge int) *http.Cookie {
	return &http.Cookie{Name: name, Value: value, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: maxAge}
}

func cookieValue(request *http.Request, name string) string {
	cookie, err := request.Cookie(name)
	if err != nil {
		return ""
	}
	return cookie.Value
}

func randomToken(size int) string {
	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(value)
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	if err := json.NewEncoder(writer).Encode(value); err != nil {
		panic(fmt.Errorf("encode fixture response: %w", err))
	}
}
