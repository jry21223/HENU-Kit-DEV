package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	notice "henukit.dev/notice"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	pool, err := pgxpool.New(ctx, os.Getenv("NOTICE_DATABASE_URL"))
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	worker, err := notice.NewWorker(pool, notice.WebhookSender{URL: os.Getenv("NOTICE_DELIVERY_URL"), Token: os.Getenv("NOTICE_DELIVERY_TOKEN")})
	if err != nil {
		log.Fatal(err)
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		processed, runErr := worker.RunOnce(ctx)
		if runErr != nil {
			log.Printf("Notice delivery attempt: %v", runErr)
		}
		if processed {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
