package main

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestRelayCommandLoadsItsPrivateListenAndLoopbackTunnelEndpoints(t *testing.T) {
	values := map[string]string{
		"GETWORK_RELAY_ADDR":         "172.17.0.1:18101",
		"GETWORK_RELAY_UPSTREAM_URL": "http://127.0.0.1:18100",
	}
	config, err := loadConfig(func(key string) string { return values[key] }, func() (string, error) {
		return "172.17.0.1", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if config.address != "172.17.0.1:18101" || config.upstream != "http://127.0.0.1:18100" {
		t.Fatalf("config = %+v", config)
	}
}

func TestRelayCommandRejectsPublicWildcardAndPrivilegedListeners(t *testing.T) {
	for _, address := range []string{"", ":18101", "0.0.0.0:18101", "192.0.2.10:18101", "172.17.0.1:443"} {
		values := map[string]string{
			"GETWORK_RELAY_ADDR":         address,
			"GETWORK_RELAY_UPSTREAM_URL": "http://127.0.0.1:18100",
		}
		if _, err := loadConfig(func(key string) string { return values[key] }, func() (string, error) {
			return "172.17.0.1", nil
		}); err == nil {
			t.Fatalf("listen address %q was accepted", address)
		}
	}
}

func TestRelayCommandRejectsAPrivateAddressThatIsNotTheDockerBridge(t *testing.T) {
	values := map[string]string{
		"GETWORK_RELAY_ADDR":         "192.168.1.20:18101",
		"GETWORK_RELAY_UPSTREAM_URL": "http://127.0.0.1:18100",
	}
	if _, err := loadConfig(func(key string) string { return values[key] }, func() (string, error) {
		return "172.17.0.1", nil
	}); err == nil {
		t.Fatal("LAN listener was accepted")
	}
}

func TestRelayCommandRejectsANonLoopbackTunnelUpstream(t *testing.T) {
	config := relayConfig{address: "172.17.0.1:18101", upstream: "http://192.0.2.10:18100"}
	if _, err := newServer(config); err == nil {
		t.Fatal("public tunnel upstream was accepted")
	}
}

func TestRelayCommandWaitsForGracefulShutdownBeforeReturning(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	serveDone := make(chan struct{})
	allowShutdown := make(chan struct{})
	runDone := make(chan error, 1)
	go func() {
		runDone <- serveUntilCancelled(
			ctx,
			time.Second,
			func() error {
				<-serveDone
				return http.ErrServerClosed
			},
			func(context.Context) error {
				<-allowShutdown
				close(serveDone)
				return nil
			},
			func() error { return nil },
		)
	}()

	cancel()
	select {
	case err := <-runDone:
		t.Fatalf("server returned before graceful shutdown completed: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(allowShutdown)
	if err := <-runDone; err != nil {
		t.Fatal(err)
	}
}

func TestRelayCommandForceClosesAfterGracefulShutdownFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	closed := make(chan struct{})
	err := serveUntilCancelled(
		ctx,
		time.Millisecond,
		func() error {
			<-closed
			return http.ErrServerClosed
		},
		func(context.Context) error { return errors.New("shutdown failed") },
		func() error { close(closed); return nil },
	)
	if err == nil || err.Error() != "shutdown failed" {
		t.Fatalf("err = %v", err)
	}
}
