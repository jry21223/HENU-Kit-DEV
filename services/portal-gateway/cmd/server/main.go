package main

import (
	"log"
	"net/http"
	"os"

	portalgateway "henukit.dev/portal-gateway"
	"henukit.dev/portal-gateway/internal/config"
)

func main() {
	cfg, err := config.FromEnv()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	handler, err := portalgateway.New(cfg)
	if err != nil {
		log.Fatalf("gateway: %v", err)
	}

	addr := cfg.ListenAddr
	log.Printf("portal-gateway listening on %s", addr)

	srv := &http.Server{
		Addr:    addr,
		Handler: handler,
	}

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server: %v", err)
		os.Exit(1)
	}
}
