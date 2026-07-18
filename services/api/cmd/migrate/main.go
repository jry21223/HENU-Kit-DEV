package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"final-review-platform/services/api/migrations"
	"final-review-platform/services/api/pkg/config"
	"final-review-platform/services/api/pkg/database"
)

func main() {
	action := flag.String("action", "up", "migration action: up or down")
	flag.Parse()
	cfg := config.Load()
	db, err := database.Connect(cfg.DatabaseURL)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	runner := migrations.NewRunner(db)
	if *action == "up" {
		err = runner.Up(context.Background())
	} else if *action == "down" {
		err = runner.Down(context.Background())
	} else {
		err = fmt.Errorf("unsupported action %q", *action)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
