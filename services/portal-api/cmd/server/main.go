package main

import (
	"log"
	"net/http"
	"os"

	"henukit.dev/portal-api/internal/db"
	"henukit.dev/portal-api/internal/httpapi"
)

func main() {
	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = ":8085"
	}

	mode := db.Mode()
	handler, err := httpapi.NewRouter()
	if err != nil {
		log.Fatalf("portal-api startup failed (mode=%s): %v", mode, err)
	}

	log.Printf("portal-api listening on %s mode=%s", addr, mode)

	srv := &http.Server{Addr: addr, Handler: handler}
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server: %v", err)
	}
}
