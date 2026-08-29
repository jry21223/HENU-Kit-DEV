package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

func check(ctx context.Context, client *http.Client, address string) error {
	host, port, err := net.SplitHostPort(strings.TrimSpace(address))
	if err != nil || host == "" || port == "" {
		return errors.New("relay address is invalid")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+net.JoinHostPort(host, port)+"/healthz", nil)
	if err != nil {
		return err
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return errors.New("relayed upstream is unhealthy")
	}
	var health struct {
		OK       bool   `json:"ok"`
		Upstream string `json:"upstream"`
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 4096))
	if err := decoder.Decode(&health); err != nil || !health.OK || health.Upstream != "RyaoVen/getWork@2c7800d" {
		return errors.New("relayed upstream identity is invalid")
	}
	return nil
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	address := os.Getenv("GETWORK_RELAY_HEALTH_ADDR")
	if strings.TrimSpace(address) == "" {
		address = os.Getenv("GETWORK_RELAY_ADDR")
	}
	if err := check(ctx, &http.Client{}, address); err != nil {
		os.Exit(1)
	}
}
