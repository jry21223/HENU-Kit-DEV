package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	quizcraft "henukit.dev/quizcraft"
)

func main() {
	if err := run(os.Args[1:]); errors.Is(err, flag.ErrHelp) {
		return
	} else if err != nil {
		fmt.Fprintln(os.Stderr, "QuizCraft release state could not be verified; retain maintenance and check target/schema evidence through the original release owner")
		os.Exit(1)
	}
}
func run(args []string) error {
	flags := flag.NewFlagSet("release-state", flag.ContinueOnError)
	directory := flags.String("directory", "db/migrations", "exact verified migration directory")
	snapshot := flags.String("snapshot", "", "existing exported read-only snapshot")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected arguments")
	}
	url := os.Getenv("QUIZCRAFT_V2_DATABASE_URL")
	if err := quizcraft.RequireQuizcraftV2DatabaseURL(url); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err = quizcraft.RequireQuizcraftV2Target(ctx, pool); err != nil {
		return err
	}
	report, err := quizcraft.ReadReleaseState(ctx, pool, *directory, *snapshot)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(report)
}
