package notice

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"henukit.dev/portal-gateway/internal/contract"
)

const (
	clientTestActor  = "11111111-1111-4111-8111-111111111111"
	clientTestSecret = "portal-notice-owner-secret-at-least-32-bytes"
)

func TestNewClientRejectsAmbiguousOwnerOrigins(t *testing.T) {
	for _, origin := range []string{
		"",
		"ftp://notice.internal",
		"https://notice.internal?",
		"https://notice.internal/?query=1",
		"https://user:password@notice.internal",
		"https://notice.internal/owner-prefix",
		"https://notice.internal/#fragment",
		"https://collector.local:8094",
		"http://notice:8093",
		"http://notice:8094?",
		"https://notice:8094",
		"http://localhost:8093",
		"http://127.0.0.1:9000",
		"http://[::1]:8095",
	} {
		t.Run(origin, func(t *testing.T) {
			if client, err := NewClient(origin, "portal-gateway-notice-read", clientTestSecret, "portal-key"); err == nil || client != nil {
				t.Fatalf("NewClient(%q) = %#v, %v; want configuration rejection", origin, client, err)
			}
		})
	}
	for _, origin := range []string{"http://notice:8094", "http://localhost:8094", "http://127.0.0.1:8094", "http://[::1]:8094"} {
		if client, err := NewClient(origin, "portal-gateway-notice-read", clientTestSecret, "portal-key"); err != nil || client == nil {
			t.Fatalf("allowed Notice origin %q rejected: %#v, %v", origin, client, err)
		}
	}
}

func TestListUsesFixedActorBoundOwnerRouteAndRejectsRedirects(t *testing.T) {
	var redirectTargetCalls atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { redirectTargetCalls.Add(1) }))
	defer target.Close()
	owner := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != PortalFeedPath || request.URL.RawQuery != "" || request.Header.Get("X-Actor-User-Id") != clientTestActor {
			t.Fatalf("owner request = %s?%s actor=%q", request.URL.Path, request.URL.RawQuery, request.Header.Get("X-Actor-User-Id"))
		}
		writer.Header().Set("Location", target.URL+"/capture")
		writer.WriteHeader(http.StatusFound)
	}))
	defer owner.Close()

	client := testOwnerClient(t, owner)
	transport, ok := client.httpClient.Transport.(*http.Transport)
	if !ok || transport.Proxy != nil || client.httpClient.CheckRedirect == nil {
		t.Fatalf("Owner client transport must disable proxies and redirects: %#v", client.httpClient)
	}
	feed, err := client.List(context.Background(), clientTestActor, "req_notice_client")
	if !errors.Is(err, ErrUnavailable) || len(feed.Notices) != 0 || redirectTargetCalls.Load() != 0 {
		t.Fatalf("redirect List = %#v, %v target=%d; want opaque unavailable with no redirect", feed, err, redirectTargetCalls.Load())
	}
}

func TestListUsesDedicatedActorBoundOwnerCredential(t *testing.T) {
	var ownerCalls atomic.Int32
	owner := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		ownerCalls.Add(1)
		if request.Method != http.MethodGet || request.URL.Path != PortalFeedPath || request.URL.RawQuery != "" || request.Header.Get("X-Actor-User-Id") != clientTestActor || request.Header.Get("X-Permission-Code") != "" || request.Header.Get("X-Scope-Kind") != "" || request.Header.Get("X-Product-Code") != "" {
			t.Fatalf("owner request = method:%s path:%s query:%q actor:%q permission:%q scope:%q product:%q", request.Method, request.URL.Path, request.URL.RawQuery, request.Header.Get("X-Actor-User-Id"), request.Header.Get("X-Permission-Code"), request.Header.Get("X-Scope-Kind"), request.Header.Get("X-Product-Code"))
		}
		assertActorBoundOwnerSignature(t, request)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"data":{"notices":[{"id":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa","title":"开学安排","body":"请按时返校。","source":{"name":"学校办公室","url":"https://example.edu/notices/1"},"created_at":"2026-08-01T00:00:00Z"}]},"request_id":"req_owner"}`))
	}))
	defer owner.Close()

	client := testOwnerClient(t, owner)
	feed, err := client.List(context.Background(), clientTestActor, "req_notice_client")
	if err != nil || ownerCalls.Load() != 1 || len(feed.Notices) != 1 || feed.Notices[0].Title != "开学安排" {
		t.Fatalf("dedicated Owner List = %#v, %v calls=%d", feed, err, ownerCalls.Load())
	}
}

func TestListAllowsUTF8SourceURLWithinByteBound(t *testing.T) {
	owner := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"data":{"notices":[{"id":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa","title":"开学安排","body":"请按时返校。","source":{"name":"学校办公室","url":"https://xn--bcher-kva.example/通知/详情?栏目=教务"},"created_at":"2026-08-01T00:00:00Z"},{"id":"bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb","title":"可导航来源","body":"普通 DNS 尾部仍可使用。","source":{"name":"学校办公室","url":"https://example.0xg/notices/2"},"created_at":"2026-08-01T00:01:00Z"}]},"request_id":"req_owner_utf8_url"}`))
	}))
	defer owner.Close()

	feed, err := testOwnerClient(t, owner).List(context.Background(), clientTestActor, "req_notice_client")
	if err != nil || len(feed.Notices) != 2 || feed.Notices[0].Source.URL != "https://xn--bcher-kva.example/通知/详情?栏目=教务" || feed.Notices[1].Source.URL != "https://example.0xg/notices/2" {
		t.Fatalf("UTF-8 source URL List = %#v, %v", feed, err)
	}
}

func TestListConsumesLargeBoundedOwnerEnvelope(t *testing.T) {
	const itemCount = 17
	items := make([]contract.PortalNotice, 0, itemCount)
	for index := 0; index < itemCount; index++ {
		items = append(items, contract.PortalNotice{
			ID:        fmt.Sprintf("aaaaaaaa-aaaa-4aaa-8aaa-%012x", index+1),
			Title:     fmt.Sprintf("大正文 %d", index),
			Body:      strings.Repeat("学", 100000),
			Source:    contract.PortalNoticeSource{Name: "学校办公室", URL: fmt.Sprintf("https://example.edu/notices/%d", index)},
			CreatedAt: time.Date(2026, time.August, 1, 0, index, 0, 0, time.UTC),
		})
	}
	payload, err := json.Marshal(struct {
		Data      contract.PortalNoticeFeed `json:"data"`
		RequestID string                    `json:"request_id"`
	}{
		Data:      contract.PortalNoticeFeed{Notices: items},
		RequestID: "req_owner_large",
	})
	if err != nil {
		t.Fatal(err)
	}
	payload = append(payload, '\n')
	if len(payload) > 5<<20 || len(payload) > 6<<20 {
		t.Fatalf("large valid Owner envelope = %d bytes, want within Owner and Gateway bounds", len(payload))
	}
	owner := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write(payload)
	}))
	defer owner.Close()

	feed, err := testOwnerClient(t, owner).List(context.Background(), clientTestActor, "req_notice_client")
	if err != nil || len(feed.Notices) != itemCount || feed.Notices[itemCount-1].Body != items[itemCount-1].Body {
		t.Fatalf("large bounded Owner envelope = %#v, %v", feed, err)
	}
}

func TestListRejectsUnknownOrOversizedOwnerPayloads(t *testing.T) {
	valid := `{"data":{"notices":[{"id":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa","title":"开学安排","body":"请按时返校。","source":{"name":"学校办公室","url":"https://example.edu/notices/1"},"created_at":"2026-08-01T00:00:00Z"}]},"request_id":"req_owner"}`
	for _, testCase := range []struct {
		name    string
		payload string
	}{
		{name: "required notices omitted", payload: `{"data":{},"request_id":"req_owner"}`},
		{name: "browser compatibility facade is not part of Owner response", payload: strings.Replace(valid, `,"request_id"`, `,"notices":[],"request_id"`, 1)},
		{name: "unknown field", payload: strings.Replace(valid, `"created_at":"2026-08-01T00:00:00Z"`, `"created_at":"2026-08-01T00:00:00Z","state":"distributed"`, 1)},
		{name: "request ID outside contract", payload: strings.Replace(valid, `"request_id":"req_owner"`, `"request_id":"owner"`, 1)},
		{name: "title exceeds contract maximum", payload: strings.Replace(valid, "开学安排", strings.Repeat("题", 201), 1)},
		{name: "body exceeds contract maximum", payload: strings.Replace(valid, "请按时返校。", strings.Repeat("正", 100001), 1)},
		{name: "source name exceeds contract maximum", payload: strings.Replace(valid, "学校办公室", strings.Repeat("源", 121), 1)},
		{name: "source URL exceeds contract maximum", payload: strings.Replace(valid, "https://example.edu/notices/1", "https://example.edu/"+strings.Repeat("u", 2048), 1)},
		{name: "source URL has Unicode hostname", payload: strings.Replace(valid, "https://example.edu/notices/1", "https://学校.example.edu/notices/1", 1)},
		{name: "source URL has Kelvin sign hostname", payload: strings.Replace(valid, "https://example.edu/notices/1", "https://\u212A.example.edu/notices/1", 1)},
		{name: "source URL targets numeric loopback host", payload: strings.Replace(valid, "https://example.edu/notices/1", "https://127.1/notices/1", 1)},
		{name: "source URL targets home.arpa", payload: strings.Replace(valid, "https://example.edu/notices/1", "https://router.home.arpa/admin", 1)},
		{name: "source URL targets a home.arpa subdomain", payload: strings.Replace(valid, "https://example.edu/notices/1", "https://status.router.home.arpa/admin", 1)},
		{name: "source URL has DNS underscore", payload: strings.Replace(valid, "https://example.edu/notices/1", "https://foo_.example.edu/notices/1", 1)},
		{name: "source URL has DNS punctuation", payload: strings.Replace(valid, "https://example.edu/notices/1", "https://foo!.example.edu/notices/1", 1)},
		{name: "source URL has DNS label over 63 characters", payload: strings.Replace(valid, "https://example.edu/notices/1", "https://"+strings.Repeat("a", 64)+".example.edu/notices/1", 1)},
		{name: "source URL has decimal IPv4-like final DNS label", payload: strings.Replace(valid, "https://example.edu/notices/1", "https://example.127/notices/1", 1)},
		{name: "source URL has hexadecimal IPv4-like final DNS label", payload: strings.Replace(valid, "https://example.edu/notices/1", "https://foo.0x7f/notices/1", 1)},
		{name: "source URL has bare hexadecimal IPv4-like final DNS label", payload: strings.Replace(valid, "https://example.edu/notices/1", "https://example.0x/notices/1", 1)},
		{name: "source URL has invalid punycode A-label", payload: strings.Replace(valid, "https://example.edu/notices/1", "https://xn--a.example/notices/1", 1)},
		{name: "source URL has invalid short punycode A-label", payload: strings.Replace(valid, "https://example.edu/notices/1", "https://xn--0.example/notices/1", 1)},
		{name: "response exceeds hard byte limit", payload: strings.Repeat(" ", 6<<20) + valid},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			owner := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				_, _ = writer.Write([]byte(testCase.payload))
			}))
			defer owner.Close()
			client := testOwnerClient(t, owner)
			feed, err := client.List(context.Background(), clientTestActor, "req_notice_client")
			if !errors.Is(err, ErrInvalid) || len(feed.Notices) != 0 {
				t.Fatalf("invalid Owner payload List = %#v, %v", feed, err)
			}
		})
	}
}

func testOwnerClient(t *testing.T, owner *httptest.Server) *Client {
	t.Helper()
	client, err := NewClient("http://notice:8094", "portal-gateway-notice-read", clientTestSecret, "portal-key")
	if err != nil {
		t.Fatal(err)
	}
	transport, ok := client.httpClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("Owner transport = %T", client.httpClient.Transport)
	}
	directTransport := transport.Clone()
	ownerAddress := owner.Listener.Addr().String()
	directTransport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, ownerAddress)
	}
	client.httpClient.Transport = directTransport
	return client
}

func assertActorBoundOwnerSignature(t *testing.T, request *http.Request) {
	t.Helper()
	user, password, ok := request.BasicAuth()
	if !ok || user != "portal-gateway-notice-read" || password != clientTestSecret || request.Header.Get("X-Service-Id") != user || request.Header.Get("X-Key-Id") != "portal-key" {
		t.Fatal("Notice Owner request omitted the dedicated Portal credential")
	}
	body, err := io.ReadAll(request.Body)
	if err != nil {
		t.Fatal(err)
	}
	request.Body = io.NopCloser(strings.NewReader(string(body)))
	digest := sha256.Sum256(body)
	canonical := strings.Join([]string{request.Method, request.URL.RequestURI(), request.Header.Get("X-Timestamp"), request.Header.Get("X-Nonce"), hex.EncodeToString(digest[:]), request.Header.Get("X-Actor-User-Id")}, "\n")
	mac := hmac.New(sha256.New, []byte(clientTestSecret))
	_, _ = mac.Write([]byte(canonical))
	if request.Header.Get("X-Signature") != base64.RawURLEncoding.EncodeToString(mac.Sum(nil)) {
		t.Fatal("Notice Owner request did not actor-bind its signature")
	}
}
