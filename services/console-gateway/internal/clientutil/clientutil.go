package clientutil

import (
	"net"
	"net/url"
	"strings"
)

// IsTrustedLoopbackHTTP reports whether parsed is an http:// URL whose host is
// on the trusted loopback allowlist: localhost, the in-cluster service names
// (platform-core, portal-api), any *.local host, or a loopback IP. Product
// clients require https:// in production; only these hosts may be reached
// without TLS. Keep this allowlist in sync with the deployments that set
// http:// service URLs for local development.
func IsTrustedLoopbackHTTP(parsed *url.URL) bool {
	host := parsed.Hostname()
	ip := net.ParseIP(host)
	return parsed.Scheme == "http" && (host == "localhost" || host == "platform-core" || host == "portal-api" || strings.HasSuffix(host, ".local") || (ip != nil && ip.IsLoopback()))
}
