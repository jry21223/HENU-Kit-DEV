package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealthcheckRequiresTheRelayedUpstreamHealthResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/healthz" {
			t.Fatalf("path = %q", request.URL.Path)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"ok":true,"upstream":"RyaoVen/getWork@2c7800d"}`))
	}))
	t.Cleanup(server.Close)

	address := strings.TrimPrefix(server.URL, "http://")
	if err := check(context.Background(), server.Client(), address); err != nil {
		t.Fatal(err)
	}
}

func TestHealthcheckRejectsAnUnrelatedHTTP200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	address := strings.TrimPrefix(server.URL, "http://")
	if err := check(context.Background(), server.Client(), address); err == nil {
		t.Fatal("unrelated HTTP 200 passed healthcheck")
	}
}

func TestHealthcheckFailsWhenTheRelayedUpstreamIsUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(server.Close)

	address := strings.TrimPrefix(server.URL, "http://")
	if err := check(context.Background(), server.Client(), address); err == nil {
		t.Fatal("unavailable relayed upstream passed healthcheck")
	}
}
