package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	quizcraft "henukit.dev/quizcraft"
)

func main() {
	if err := run(context.Background(), os.Args[1:]); errors.Is(err, flag.ErrHelp) {
		return
	} else if err != nil {
		fmt.Fprintln(os.Stderr, "QuizCraft V2 schema migration failed")
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("migrate", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	directory := flags.String("directory", "db/migrations", "directory containing ordered .up.sql migrations")
	if err := flags.Parse(args); err != nil {
		return err
	}
	databaseURL := os.Getenv("QUIZCRAFT_V2_DATABASE_URL")
	if err := quizcraft.RequireQuizcraftV2DatabaseURL(databaseURL); err != nil {
		return err
	}
	database, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return err
	}
	defer database.Close()
	if err := quizcraft.RequireQuizcraftV2Target(ctx, database); err != nil {
		return err
	}
	report, err := quizcraft.ApplyVersionedMigrations(ctx, database, *directory)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(report)
}
