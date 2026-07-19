package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	quizcraft "henukit.dev/quizcraft"
)

func main() {
	atText := flag.String("at", "", "RFC3339 instant used to select the previous complete UTC week")
	flag.Parse()
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		fmt.Fprintln(os.Stderr, "DATABASE_URL is required")
		os.Exit(2)
	}
	at := time.Now()
	if *atText != "" {
		parsed, err := time.Parse(time.RFC3339, *atText)
		if err != nil {
			fmt.Fprintln(os.Stderr, "-at must be RFC3339")
			os.Exit(2)
		}
		at = parsed
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		fmt.Fprintln(os.Stderr, "connect to QuizCraft PostgreSQL failed")
		os.Exit(1)
	}
	defer pool.Close()
	service, err := quizcraft.New(quizcraft.Config{Database: pool})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	count, err := service.SettlePreviousUTCWeek(ctx, at)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ranking settlement failed")
		os.Exit(1)
	}
	fmt.Printf("recorded %d reward-free ranking settlement events\n", count)
}
