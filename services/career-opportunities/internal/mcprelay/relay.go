package mcprelay

import (
	"errors"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

const reviewedUpstreamOrigin = "http://127.0.0.1:18100"

// New returns the private HTTP relay that bridges the production host to the
// loopback endpoint created by the restricted SSH reverse tunnel.
func New(rawUpstream string) (http.Handler, error) {
	return newWithTransport(rawUpstream, http.DefaultTransport)
}

// NewWithTransport preserves the exact reviewed origin while allowing tests to
// replace only the network transport.
func NewWithTransport(rawUpstream string, transport http.RoundTripper) (http.Handler, error) {
	return newWithTransport(rawUpstream, transport)
}

func newWithTransport(rawUpstream string, transport http.RoundTripper) (http.Handler, error) {
	if strings.TrimSpace(rawUpstream) != reviewedUpstreamOrigin || transport == nil {
		return nil, errors.New("job source tunnel upstream must be http://127.0.0.1:18100")
	}
	upstream, err := url.Parse(reviewedUpstreamOrigin)
	if err != nil {
		return nil, errors.New("job source tunnel upstream is invalid")
	}
	upstream.Path = ""
	proxy := httputil.NewSingleHostReverseProxy(upstream)
	proxy.Transport = transport
	proxy.ErrorHandler = func(writer http.ResponseWriter, _ *http.Request, _ error) {
		http.Error(writer, "job source tunnel unavailable", http.StatusServiceUnavailable)
	}
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/mcp", "/healthz":
			proxy.ServeHTTP(writer, request)
		default:
			http.NotFound(writer, request)
		}
	}), nil
}
