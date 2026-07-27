package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	quizcraft "henukit.dev/quizcraft"
)

func main() {
	if err := run(context.Background(), os.Args[1:]); errors.Is(err, flag.ErrHelp) {
		return
	} else if err != nil {
		fmt.Fprintln(os.Stderr, "QuizCraft V2 backup/restore drill failed")
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("backuprestore", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	backupDirectory := flags.String("backup-directory", "", "directory for the retained custom-format backup")
	databaseNameHint := flags.String("database-name-hint", "quizcraft_v2", "safe prefix for the temporary restore database")
	requiredTables := flags.String("required-tables", "quizcraft_banks,quizcraft_questions,quizcraft_migration_runs", "comma-separated required restored table names")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *backupDirectory == "" {
		return errors.New("backup-directory is required")
	}
	databaseURL := os.Getenv("QUIZCRAFT_V2_DATABASE_URL")
	restoreAdminURL := os.Getenv("QUIZCRAFT_RESTORE_ADMIN_URL")
	if err := quizcraft.RequireQuizcraftV2DatabaseURL(databaseURL); err != nil {
		return err
	}
	if restoreAdminURL == "" {
		return errors.New("QUIZCRAFT_RESTORE_ADMIN_URL is required")
	}
	tables := make([]string, 0)
	for _, table := range strings.Split(*requiredTables, ",") {
		if trimmed := strings.TrimSpace(table); trimmed != "" {
			tables = append(tables, trimmed)
		}
	}
	report, err := quizcraft.RunBackupRestoreDrill(ctx, quizcraft.BackupRestoreDrillOptions{
		DatabaseURL:      databaseURL,
		RestoreAdminURL:  restoreAdminURL,
		BackupDirectory:  *backupDirectory,
		RequiredTables:   tables,
		DatabaseNameHint: *databaseNameHint,
	})
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(report)
}
