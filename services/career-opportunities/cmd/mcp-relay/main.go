package main

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"henukit.dev/career/internal/mcprelay"
)

type relayConfig struct {
	address  string
	upstream string
}

func loadConfig(getenv func(string) string, bridgeAddress func() (string, error)) (relayConfig, error) {
	config := relayConfig{
		address:  strings.TrimSpace(getenv("GETWORK_RELAY_ADDR")),
		upstream: strings.TrimSpace(getenv("GETWORK_RELAY_UPSTREAM_URL")),
	}
	host, rawPort, err := net.SplitHostPort(config.address)
	if err != nil {
		return relayConfig{}, errors.New("GETWORK_RELAY_ADDR must be a private IP and unprivileged port")
	}
	address := net.ParseIP(host)
	port, portErr := strconv.Atoi(rawPort)
	bridge, bridgeErr := bridgeAddress()
	if address == nil || portErr != nil || port != 18101 || bridgeErr != nil || host != bridge {
		return relayConfig{}, errors.New("GETWORK_RELAY_ADDR must be the Docker bridge IPv4 address on port 18101")
	}
	return config, nil
}

func dockerBridgeIPv4() (string, error) {
	bridge, err := net.InterfaceByName("docker0")
	if err != nil {
		return "", errors.New("docker0 interface is unavailable")
	}
	addresses, err := bridge.Addrs()
	if err != nil {
		return "", errors.New("docker0 addresses are unavailable")
	}
	for _, raw := range addresses {
		address, _, parseErr := net.ParseCIDR(raw.String())
		if parseErr == nil && address.To4() != nil {
			return address.String(), nil
		}
	}
	return "", errors.New("docker0 has no IPv4 address")
}

func newServer(config relayConfig) (*http.Server, error) {
	handler, err := mcprelay.New(config.upstream)
	if err != nil {
		return nil, err
	}
	return &http.Server{
		Addr:              config.address,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      7 * time.Minute,
		IdleTimeout:       65 * time.Second,
	}, nil
}

func serveUntilCancelled(
	ctx context.Context,
	shutdownTimeout time.Duration,
	serve func() error,
	shutdown func(context.Context) error,
	closeServer func() error,
) error {
	serveResult := make(chan error, 1)
	go func() {
		serveResult <- serve()
	}()
	select {
	case err := <-serveResult:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
	}

	shutdownContext, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	shutdownErr := shutdown(shutdownContext)
	if shutdownErr != nil {
		_ = closeServer()
	}
	serveErr := <-serveResult
	if shutdownErr != nil {
		return shutdownErr
	}
	if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
		return serveErr
	}
	return nil
}

func main() {
	config, err := loadConfig(os.Getenv, dockerBridgeIPv4)
	if err != nil {
		log.Fatal(err)
	}
	server, err := newServer(config)
	if err != nil {
		log.Fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	log.Printf("Job Source MCP relay listening on %s", config.address)
	if err := serveUntilCancelled(
		ctx, 10*time.Second, server.ListenAndServe, server.Shutdown, server.Close,
	); err != nil {
		log.Fatal(err)
	}
}
