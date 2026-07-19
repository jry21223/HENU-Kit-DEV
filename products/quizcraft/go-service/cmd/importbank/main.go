package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	quizcraft "henukit.dev/quizcraft"

	"github.com/jackc/pgx/v5/pgxpool"
)

const maxBankSize = 32 << 20

func main() {
	databaseURL := flag.String("database-url", "", "PostgreSQL connection URL override (prefer DATABASE_URL to avoid process-list exposure)")
	bankKey := flag.String("bank-key", "", "stable lowercase bank key")
	filePath := flag.String("file", "", "JSON bank file to import explicitly")
	flag.Parse()
	if *databaseURL == "" {
		*databaseURL = os.Getenv("DATABASE_URL")
	}
	if *databaseURL == "" || *bankKey == "" || *filePath == "" {
		fail(fmt.Errorf("DATABASE_URL (or --database-url), --bank-key, and --file are required"))
	}

	file, err := os.Open(*filePath)
	if err != nil {
		fail(err)
	}
	defer file.Close()
	source, err := io.ReadAll(io.LimitReader(file, maxBankSize+1))
	if err != nil {
		fail(err)
	}
	if len(source) > maxBankSize {
		fail(fmt.Errorf("bank file exceeds %d bytes", maxBankSize))
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, *databaseURL)
	if err != nil {
		fail(err)
	}
	defer pool.Close()
	service, err := quizcraft.New(quizcraft.Config{Database: pool})
	if err != nil {
		fail(err)
	}
	report, importErr := service.ImportJSON(ctx, *bankKey, source)
	if err := json.NewEncoder(os.Stdout).Encode(report); err != nil {
		fail(err)
	}
	if importErr != nil {
		fmt.Fprintln(os.Stderr, importErr)
		os.Exit(1)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
