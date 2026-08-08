package clientutil

import (
	"net"
	"net/url"
	"strings"
)

// IsTrustedLoopbackHTTP reports whether parsed is an http:// URL whose host is
// on the trusted loopback allowlist: localhost, any *.local host, or a
// loopback IP, plus the in-cluster service names each caller passes in
// extraHosts (e.g. "platform-core", "portal-api", "account-portfolio").
// Product clients require https:// in production; only these hosts may be
// reached without TLS. Keep the host lists in sync with the deployments that
// set http:// service URLs for local development.
func IsTrustedLoopbackHTTP(parsed *url.URL, extraHosts ...string) bool {
	host := parsed.Hostname()
	ip := net.ParseIP(host)
	if parsed.Scheme != "http" {
		return false
	}
	if host == "localhost" || strings.HasSuffix(host, ".local") || (ip != nil && ip.IsLoopback()) {
		return true
	}
	for _, extra := range extraHosts {
		if host == extra {
			return true
		}
	}
	return false
}
