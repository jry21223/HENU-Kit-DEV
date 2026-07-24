package main

import (
	"log"
	"net/http"
	"os"

	"henukit.dev/portal-api/internal/httpapi"
)

func main() {
	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = ":8085"
	}

	handler, cleanup := httpapi.NewRouter()
	defer cleanup()

	log.Printf("portal-api listening on %s", addr)

	srv := &http.Server{Addr: addr, Handler: handler}
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server: %v", err)
		os.Exit(1)
	}
}
