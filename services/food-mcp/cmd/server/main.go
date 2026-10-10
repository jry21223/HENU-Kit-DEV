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

	"henukit.dev/food-mcp/internal/foodclient"
	mcp "henukit.dev/food-mcp/internal/mcp"
	"henukit.dev/food-mcp/internal/sessionclient"
)

func main() {
	accessToken := strings.TrimSpace(os.Getenv("FOOD_MCP_ACCESS_TOKEN"))
	if accessToken == "" {
		log.Fatal("FOOD_MCP_ACCESS_TOKEN is required (fail closed)")
	}
	client, err := foodclient.NewClient(
		os.Getenv("FOOD_POSTS_URL"),
		os.Getenv("FOOD_POST_CREATE_CLIENT_ID"), os.Getenv("FOOD_POST_CREATE_SECRET"), os.Getenv("FOOD_POST_CREATE_KEY_ID"),
		os.Getenv("FOOD_POST_READ_CLIENT_ID"), os.Getenv("FOOD_POST_READ_SECRET"), os.Getenv("FOOD_POST_READ_KEY_ID"),
	)
	if err != nil {
		log.Fatal(err)
	}
	kitSession, err := sessionclient.NewClient(os.Getenv("FOOD_MCP_KIT_SESSION_URL"), os.Getenv("FOOD_MCP_KIT_SESSION_COOKIE_NAME"))
	if err != nil {
		log.Fatal(err)
	}
	requireActor := strings.TrimSpace(os.Getenv("FOOD_MCP_REQUIRE_ACTOR_CONTEXT"))
	if requireActor != "" && requireActor != "true" && requireActor != "false" && requireActor != "1" && requireActor != "0" {
		log.Fatal("invalid FOOD_MCP_REQUIRE_ACTOR_CONTEXT")
	}
	handler, err := mcp.NewHandler(mcp.Options{Client: client, AccessToken: accessToken, KitSession: kitSession, ActorSecret: strings.TrimSpace(os.Getenv("FOOD_MCP_ACTOR_SECRET")), RequireActorContext: requireActor == "true" || requireActor == "1"})
	if err != nil {
		log.Fatal(err)
	}
	address := strings.TrimSpace(os.Getenv("FOOD_MCP_ADDR"))
	if address == "" {
		address = ":8098"
	}
	server := &http.Server{
		Addr: address, Handler: handler.Handler(),
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 60 * time.Second, IdleTimeout: 60 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	log.Printf("Food Post MCP server listening on %s", address)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
