package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"henukit.dev/career-mcp/internal/careerclient"
	mcp "henukit.dev/career-mcp/internal/mcp"
)

func main() {
	accessToken := strings.TrimSpace(os.Getenv("CAREER_MCP_ACCESS_TOKEN"))
	if accessToken == "" {
		log.Fatal("CAREER_MCP_ACCESS_TOKEN is required (fail closed)")
	}
	client, err := careerclient.NewClient(
		os.Getenv("CAREER_URL"),
		os.Getenv("CAREER_CLIENT_ID"), os.Getenv("CAREER_CLIENT_SECRET"), os.Getenv("CAREER_KEY_ID"),
	)
	if err != nil {
		log.Fatal(err)
	}
	handler, err := mcp.NewHandler(mcp.Options{Client: client, AccessToken: accessToken})
	if err != nil {
		log.Fatal(err)
	}
	address := strings.TrimSpace(os.Getenv("CAREER_MCP_ADDR"))
	if address == "" {
		address = ":8099"
	}
	server := &http.Server{
		Addr: address, Handler: handler.Handler(),
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 60 * time.Second, WriteTimeout: 120 * time.Second, IdleTimeout: 60 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	log.Printf("Career Resume MCP server listening on %s", address)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
