// Package notice is Portal Gateway's narrow, actor-bound client for the
// Notice Owner's public feed. It is not a generic Notice proxy.
package notice

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/net/idna"

	"henukit.dev/portal-gateway/internal/contract"
	"henukit.dev/portal-gateway/internal/serviceauth"
)

const (
	PortalFeedPath                 = "/api/v1/portal/notices"
	portalNoticeSourceURLByteLimit = 2048
)

var (
	ErrUnavailable       = errors.New("notice Owner is unavailable")
	ErrInvalid           = errors.New("notice Owner returned an invalid response")
	publicDNSIDNAProfile = idna.New(
		idna.MapForLookup(),
		idna.BidiRule(),
		idna.CheckJoiners(true),
		idna.ValidateLabels(true),
		idna.StrictDomainName(true),
		idna.VerifyDNSLength(true),
	)
)

// Client only has the fixed Portal-read capability; neither browser inputs
// nor caller-selected Scope/permission headers can alter the Owner route.
type Client struct {
	baseURL    string
	httpClient *http.Client
	signer     *serviceauth.Signer
}

// NewClient refuses a pathful or ambiguous owner address, disables ambient
// proxies, and refuses redirects so service credentials cannot leave the
// intended internal owner origin.
func NewClient(baseURL, clientID, clientSecret, keyID string) (*Client, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") || !validOwnerOrigin(parsed) || strings.TrimSpace(clientID) == "" || len(clientSecret) < 32 || strings.TrimSpace(keyID) == "" {
		return nil, errors.New("invalid Notice Owner client configuration")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return &Client{
		baseURL: strings.TrimRight(parsed.String(), "/"),
		httpClient: &http.Client{
			Timeout:   10 * time.Second,
			Transport: transport,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		signer: serviceauth.NewSigner(clientID, clientSecret, keyID),
	}, nil
}

// validOwnerOrigin deliberately admits only the compose service origin and
// its fixed local equivalent. Tests inject a dialer rather than widening this
// Basic/HMAC credential to arbitrary loopback ports.
func validOwnerOrigin(parsed *url.URL) bool {
	host := strings.ToLower(parsed.Hostname())
	if parsed.Scheme != "http" || parsed.Port() != "8094" {
		return false
	}
	return host == "notice" || host == "localhost" || host == "127.0.0.1" || host == "::1"
}

// List reads and strictly validates the bounded public contract. Owner 401,
// 403, redirects, and all other non-200 outcomes are opaque unavailability to
// the browser; only Gateway's own Platform decision can produce browser 401/3.
func (c *Client) List(ctx context.Context, actorUserID, requestID string) (contract.PortalNoticeFeed, error) {
	if c == nil || c.httpClient == nil || c.signer == nil || strings.TrimSpace(requestID) == "" {
		return contract.PortalNoticeFeed{}, ErrUnavailable
	}
	if !noticeUUIDPattern.MatchString(strings.TrimSpace(actorUserID)) {
		return contract.PortalNoticeFeed{}, ErrUnavailable
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+PortalFeedPath, nil)
	if err != nil {
		return contract.PortalNoticeFeed{}, fmt.Errorf("new Notice Owner request: %w", ErrUnavailable)
	}
	request.Header.Set("X-Request-Id", requestID)
	if err := c.signer.SignWithActor(request, actorUserID); err != nil {
		return contract.PortalNoticeFeed{}, fmt.Errorf("sign Notice Owner request: %w", ErrUnavailable)
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return contract.PortalNoticeFeed{}, fmt.Errorf("call Notice Owner: %w", ErrUnavailable)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
		return contract.PortalNoticeFeed{}, ErrUnavailable
	}
	payload, err := io.ReadAll(io.LimitReader(response.Body, (6<<20)+1))
	if err != nil || len(payload) > 6<<20 {
		return contract.PortalNoticeFeed{}, ErrInvalid
	}
	var envelope struct {
		Data      contract.PortalNoticeFeed `json:"data"`
		RequestID string                    `json:"request_id"`
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
		return contract.PortalNoticeFeed{}, ErrInvalid
	}
	if err := validateFeed(envelope.RequestID, envelope.Data); err != nil {
		return contract.PortalNoticeFeed{}, ErrInvalid
	}
	return envelope.Data, nil
}

func validateFeed(requestID string, feed contract.PortalNoticeFeed) error {
	if !noticeRequestIDPattern.MatchString(requestID) || feed.Notices == nil || len(feed.Notices) > 50 {
		return errors.New("invalid Notice Owner envelope")
	}
	for _, item := range feed.Notices {
		if !noticeUUIDPattern.MatchString(item.ID) || strings.TrimSpace(item.Title) == "" || utf8.RuneCountInString(item.Title) > 200 || strings.TrimSpace(item.Body) == "" || utf8.RuneCountInString(item.Body) > 100000 || strings.TrimSpace(item.Source.Name) == "" || utf8.RuneCountInString(item.Source.Name) > 120 || item.CreatedAt.IsZero() {
			return errors.New("invalid Notice Owner item")
		}
		if !validPublicSourceURL(item.Source.URL) {
			return errors.New("invalid Notice Owner source URL")
		}
	}
	return nil
}

// validPublicSourceURL independently enforces the public-source response
// contract. Notice owns the whitelist, while Gateway must still reject a
// malformed Owner response rather than pass a private or unbounded link to a
// student browser.
func validPublicSourceURL(raw string) bool {
	if raw == "" || len(raw) > portalNoticeSourceURLByteLimit || raw != strings.TrimSpace(raw) {
		return false
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return false
	}
	// Check the original hostname before lowercasing. Compatibility case
	// folding could otherwise turn a non-ASCII hostname such as Kelvin sign
	// into ASCII and bypass the Owner/Gateway public DNS boundary.
	rawHost := strings.TrimSuffix(parsed.Hostname(), ".")
	if !isASCIIHostname(rawHost) {
		return false
	}
	host := strings.ToLower(rawHost)
	if !isPublicDNSHost(host) {
		return false
	}
	return validHTTPSPort(parsed)
}

func isPublicDNSHost(host string) bool {
	if host == "" || host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".internal") || host == "home.arpa" || strings.HasSuffix(host, ".home.arpa") || !isASCIIHostname(host) || !isValidASCIIDNALabelHost(host) {
		return false
	}
	if _, err := netip.ParseAddr(host); err == nil || isNumericIPv4Host(host) || hasNumericIPv4FinalLabel(host) {
		return false
	}
	return strings.Contains(host, ".")
}

func isValidASCIIDNALabelHost(host string) bool {
	decoded, err := publicDNSIDNAProfile.ToUnicode(host)
	if err != nil {
		return false
	}
	normalized, err := publicDNSIDNAProfile.ToASCII(decoded)
	return err == nil && normalized == host
}

func isASCIIHostname(host string) bool {
	if host == "" || len(host) > 253 {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || !isASCII(label) || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, character := range label {
			if !(character >= 'a' && character <= 'z') && !(character >= 'A' && character <= 'Z') && !(character >= '0' && character <= '9') && character != '-' {
				return false
			}
		}
	}
	return true
}

func validHTTPSPort(parsed *url.URL) bool {
	rawPort := parsed.Port()
	if rawPort == "" {
		return parsed.Host == parsed.Hostname()
	}
	port, err := strconv.ParseUint(rawPort, 10, 16)
	return err == nil && port > 0 && port <= 65535
}

func isASCII(value string) bool {
	for _, character := range value {
		if character > 0x7f {
			return false
		}
	}
	return true
}

func isNumericIPv4Host(host string) bool {
	components := strings.Split(host, ".")
	if len(components) == 0 {
		return false
	}
	for _, component := range components {
		if !isNumericIPv4Component(component) {
			return false
		}
	}
	return true
}

func hasNumericIPv4FinalLabel(host string) bool {
	labels := strings.Split(host, ".")
	if len(labels) == 0 {
		return false
	}
	finalLabel := labels[len(labels)-1]
	// WHATWG treats a bare hexadecimal prefix as an invalid IPv4 candidate;
	// keep it out of Portal's public DNS link contract while allowing ordinary
	// DNS names such as example.0xg.
	return finalLabel == "0x" || isNumericIPv4Component(finalLabel)
}

func isNumericIPv4Component(component string) bool {
	if component == "" {
		return false
	}
	if strings.HasPrefix(component, "0x") {
		if len(component) == len("0x") {
			return false
		}
		for _, character := range component[len("0x"):] {
			if !(character >= '0' && character <= '9') && !(character >= 'a' && character <= 'f') {
				return false
			}
		}
		return true
	}
	for _, character := range component {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

var (
	noticeUUIDPattern      = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	noticeRequestIDPattern = regexp.MustCompile(`^req_[A-Za-z0-9_-]{1,116}$`)
)
