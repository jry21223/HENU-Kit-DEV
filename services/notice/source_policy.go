package notice

import (
	"errors"
	"net/netip"
	"net/url"
	"strconv"
	"strings"

	"golang.org/x/net/idna"
)

var errInvalidPublicSourceOrigin = errors.New("notice source URL is not a permitted public origin")

var publicDNSIDNAProfile = idna.New(
	idna.MapForLookup(),
	idna.BidiRule(),
	idna.CheckJoiners(true),
	idna.ValidateLabels(true),
	idna.StrictDomainName(true),
	idna.VerifyDNSLength(true),
)

// publicSourceURLByteLimit is a verified safe size below PostgreSQL's btree
// entry limit for notice_versions' unique source_url index. It applies to the
// decoded UTF-8 URL bytes; OpenAPI maxLength remains a client-side character
// bound while this owner policy is the persistence boundary.
const publicSourceURLByteLimit = 2048

// publicURLOrigin is the stable, public origin recorded by a Notice source.
// notice_sources is the owner-managed whitelist: a version may only link back
// to the same approved public origin, rather than introducing a new host.
type publicURLOrigin struct {
	host string
	port string
}

// publicURLOriginFor parses a public HTTPS URL without DNS lookup. Notice
// never fetches these URLs; a source is explicitly approved by a Notice
// manager and this policy keeps that approval bounded to a public origin.
func publicURLOriginFor(raw string) (publicURLOrigin, bool) {
	if raw == "" || len(raw) > publicSourceURLByteLimit || raw != strings.TrimSpace(raw) {
		return publicURLOrigin{}, false
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.User != nil {
		return publicURLOrigin{}, false
	}
	// Validate the parsed hostname before case folding. Unicode compatibility
	// folding maps the Kelvin sign to ASCII "k", so lowercasing first would
	// turn a non-ASCII hostname into an apparently permitted public DNS name.
	rawHost := strings.TrimSuffix(parsed.Hostname(), ".")
	if !isASCIIHostname(rawHost) {
		return publicURLOrigin{}, false
	}
	host := strings.ToLower(rawHost)
	if !isPublicSourceHost(host) {
		return publicURLOrigin{}, false
	}
	port, portOK := normalizedHTTPSPort(parsed)
	if !portOK {
		return publicURLOrigin{}, false
	}
	return publicURLOrigin{host: host, port: port}, true
}

// normalizedHTTPSPort returns a canonical decimal port for origin comparison.
// The implicit HTTPS default and any numeric spelling of 443 are equivalent;
// malformed, zero, and out-of-range ports are not public URL origins.
func normalizedHTTPSPort(parsed *url.URL) (string, bool) {
	rawPort := parsed.Port()
	if rawPort == "" {
		// url.URL.Port intentionally returns an empty string for both no port and
		// malformed host-port syntax. A DNS hostname must be exactly the raw Host
		// when no explicit port exists.
		if parsed.Host != parsed.Hostname() {
			return "", false
		}
		return "443", true
	}
	port, err := strconv.ParseUint(rawPort, 10, 16)
	if err != nil || port == 0 || port > 65535 {
		return "", false
	}
	if port == 443 {
		return "443", true
	}
	return strconv.FormatUint(port, 10), true
}

func validPublicHTTPSURL(raw string) bool {
	_, ok := publicURLOriginFor(raw)
	return ok
}

func samePublicSourceOrigin(canonicalURL, versionURL string) bool {
	canonical, canonicalOK := publicURLOriginFor(canonicalURL)
	version, versionOK := publicURLOriginFor(versionURL)
	return canonicalOK && versionOK && canonical == version
}

func isPublicSourceHost(host string) bool {
	if host == "" || host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".internal") {
		return false
	}
	// Notice keeps its owner-managed whitelist to ASCII DNS hostnames. This
	// deliberately avoids accepting a Unicode host spelling that a browser's
	// WHATWG parser could normalize into a private or loopback address.
	if !isASCIIHostname(host) || !isValidASCIIDNALabelHost(host) {
		return false
	}
	// A public source is a registered DNS hostname, not an IP literal. Besides
	// forbidding direct private and loopback addresses, this keeps the browser's
	// WHATWG IP canonicalization from turning an apparently public URL such as
	// 127.1 or 0x7f.0.0.1 into a loopback address when a student follows it.
	if _, err := netip.ParseAddr(host); err == nil || isNumericIPv4Host(host) || hasNumericIPv4FinalLabel(host) {
		return false
	}
	// A single-label name is not a public web origin. We deliberately do not
	// resolve DNS here: that would turn an owner-managed data validation rule
	// into an ambient remote dependency.
	return strings.Contains(host, ".")
}

// isValidASCIIDNALabelHost proves that any ASCII xn-- A-label is real IDNA,
// not merely a label-shaped string a browser would reject during navigation.
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

func isASCII(value string) bool {
	for _, character := range value {
		if character > 0x7f {
			return false
		}
	}
	return true
}

// isNumericIPv4Host recognizes the numeric component forms that browsers can
// interpret as IPv4 even when netip deliberately rejects their non-canonical
// spelling. Rejecting any all-numeric dotted host is intentional: public Notice
// sources must use a DNS hostname, so there is no public-IP exception to make.
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

// hasNumericIPv4FinalLabel rejects browser-invalid IPv4-candidate tails such
// as foo.0x7f, example.127, or example.0x without forbidding normal names
// like www.123.com. WHATWG treats a bare hexadecimal prefix as an invalid IP
// candidate, so it is not a browser-navigable public DNS final label either.
func hasNumericIPv4FinalLabel(host string) bool {
	labels := strings.Split(host, ".")
	if len(labels) == 0 {
		return false
	}
	finalLabel := labels[len(labels)-1]
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
