package mcprelay

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func handlerForTestUpstream(t *testing.T, rawTarget string) http.Handler {
	t.Helper()
	target, err := url.Parse(rawTarget)
	if err != nil {
		t.Fatal(err)
	}
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		cloned := request.Clone(request.Context())
		cloned.URL.Scheme = target.Scheme
		cloned.URL.Host = target.Host
		return http.DefaultTransport.RoundTrip(cloned)
	})
	handler, err := newWithTransport(reviewedUpstreamOrigin, transport)
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

func TestRelayForwardsMCPRequestsToTheLoopbackTunnel(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/mcp" {
			t.Fatalf("upstream path = %q", request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer deployment-owned-token" {
			t.Fatalf("authorization = %q", request.Header.Get("Authorization"))
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		if string(body) != `{"jsonrpc":"2.0","method":"initialize","id":1}` {
			t.Fatalf("body = %q", body)
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusAccepted)
		_, _ = writer.Write([]byte(`{"jsonrpc":"2.0","result":{},"id":1}`))
	}))
	t.Cleanup(upstream.Close)

	handler := handlerForTestUpstream(t, upstream.URL)
	relay := httptest.NewServer(handler)
	t.Cleanup(relay.Close)

	request, err := http.NewRequest(http.MethodPost, relay.URL+"/mcp", strings.NewReader(`{"jsonrpc":"2.0","method":"initialize","id":1}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer deployment-owned-token")
	response, err := relay.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d", response.StatusCode)
	}
}

func TestRelayRejectsAnythingExceptALoopbackHTTPOrigin(t *testing.T) {
	for _, upstream := range []string{
		"https://127.0.0.1:18100",
		"http://192.0.2.10:18100",
		"http://user@127.0.0.1:18100",
		"http://127.0.0.1:18100/mcp",
		"http://127.0.0.1:18100?target=other",
		"http://127.0.0.1",
		"http://127.0.0.1:22",
		"http://localhost:18100",
		"http://[::1]:18100",
	} {
		if _, err := New(upstream); err == nil {
			t.Fatalf("upstream %q was accepted", upstream)
		}
	}
}

func TestRelayExposesOnlyTheMCPAndHealthInterfaces(t *testing.T) {
	var upstreamRequests atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		upstreamRequests.Add(1)
	}))
	t.Cleanup(upstream.Close)
	handler := handlerForTestUpstream(t, upstream.URL)
	relay := httptest.NewServer(handler)
	t.Cleanup(relay.Close)

	response, err := relay.Client().Get(relay.URL + "/debug")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusNotFound || upstreamRequests.Load() != 0 {
		t.Fatalf("status = %d, upstream requests = %d", response.StatusCode, upstreamRequests.Load())
	}
}

func TestRelayReportsAnUnavailableTunnelWithoutLeakingTheDialError(t *testing.T) {
	handler, err := newWithTransport(reviewedUpstreamOrigin, roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("fixture dial failure")
	}))
	if err != nil {
		t.Fatal(err)
	}
	relay := httptest.NewServer(handler)
	t.Cleanup(relay.Close)

	response, err := relay.Client().Get(relay.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}

	if response.StatusCode != http.StatusServiceUnavailable || strings.TrimSpace(string(body)) != "job source tunnel unavailable" {
		t.Fatalf("status = %d, body = %q", response.StatusCode, body)
	}
}
