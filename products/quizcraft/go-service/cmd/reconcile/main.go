package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	quizcraft "henukit.dev/quizcraft"
)

type options struct {
	mode, targetURL, legacyURL, sourceName, runID, snapshotFile string
	eventLimit, maxBatches                                      int
	minimumSamples                                              int64
	mismatchThreshold                                           float64
	windowStart, windowEnd                                      string
}

func main() {
	if err := run(context.Background(), os.Args[1:]); errors.Is(err, flag.ErrHelp) {
		return
	} else if err != nil {
		fmt.Fprintln(os.Stderr, "QuizCraft reconciliation gate failed")
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("reconcile", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	value := options{}
	flags.StringVar(&value.mode, "mode", "full", "snapshot, full, resume, catch-up, or shadow-gate")
	flags.StringVar(&value.sourceName, "source-name", "quizcraft-legacy-production", "stable source identifier")
	flags.StringVar(&value.runID, "run-id", "", "full migration run UUID for resume or catch-up")
	flags.StringVar(&value.snapshotFile, "snapshot-file", "", "immutable legacy snapshot artifact for full/resume, or create path for snapshot")
	flags.IntVar(&value.eventLimit, "event-limit", 1000, "events per catch-up batch")
	flags.IntVar(&value.maxBatches, "max-batches", 100, "maximum catch-up batches before blocking")
	flags.Int64Var(&value.minimumSamples, "minimum-samples", 1000, "minimum shadow comparisons required")
	flags.Float64Var(&value.mismatchThreshold, "mismatch-threshold", 0.001, "maximum mismatch plus legacy-error ratio")
	flags.StringVar(&value.windowStart, "window-start", "", "shadow window start in RFC3339")
	flags.StringVar(&value.windowEnd, "window-end", "", "shadow window end in RFC3339")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if value.eventLimit < 1 || value.eventLimit > 10000 || value.maxBatches < 1 || value.maxBatches > 10000 {
		return errors.New("event-limit and max-batches must be between 1 and 10000")
	}
	value.targetURL = os.Getenv("QUIZCRAFT_V2_DATABASE_URL")
	value.legacyURL = os.Getenv("LEGACY_DATABASE_URL")
	if value.mode == "snapshot" {
		if value.snapshotFile == "" {
			return errors.New("snapshot mode requires snapshot-file")
		}
		legacy, closeLegacy, err := openLegacy(ctx, value.legacyURL)
		if err != nil {
			return err
		}
		defer closeLegacy()
		snapshot, err := legacy.Snapshot(ctx, value.sourceName)
		if err != nil {
			return err
		}
		identity, err := legacy.DatabaseIdentity(ctx)
		if err != nil {
			return err
		}
		artifact, err := quizcraft.WriteLegacySnapshotArtifact(value.snapshotFile, identity, snapshot)
		if err != nil {
			return err
		}
		return printJSON(os.Stdout, artifact)
	}
	if err := quizcraft.RequireQuizcraftV2DatabaseURL(value.targetURL); err != nil {
		return err
	}
	target, err := pgxpool.New(ctx, value.targetURL)
	if err != nil {
		return err
	}
	defer target.Close()
	if err := quizcraft.RequireQuizcraftV2Target(ctx, target); err != nil {
		return err
	}
	service, err := quizcraft.New(quizcraft.Config{Database: target})
	if err != nil {
		return err
	}
	switch value.mode {
	case "full":
		artifact, err := readSnapshotForTarget(ctx, target, value.snapshotFile)
		if err != nil {
			return err
		}
		report, err := service.RunFullMigration(ctx, artifact.Snapshot)
		if report.RunID != uuid.Nil {
			if printErr := printJSON(os.Stdout, report); printErr != nil {
				return printErr
			}
		}
		if err != nil {
			return err
		}
		if report.State != "passed" {
			return errors.New("full reconciliation blocked")
		}
		return nil
	case "resume":
		artifact, err := readSnapshotForTarget(ctx, target, value.snapshotFile)
		if err != nil {
			return err
		}
		parsedRunID, err := uuid.Parse(value.runID)
		if err != nil {
			return errors.New("resume requires a migration run UUID")
		}
		report, err := service.ResumeFullMigration(ctx, parsedRunID, artifact.Snapshot)
		if report.RunID != uuid.Nil {
			if printErr := printJSON(os.Stdout, report); printErr != nil {
				return printErr
			}
		}
		if err != nil {
			return err
		}
		if report.State != "passed" {
			return errors.New("resumed reconciliation blocked")
		}
		return nil
	case "catch-up":
		legacy, closeLegacy, err := openLegacy(ctx, value.legacyURL)
		if err != nil {
			return err
		}
		defer closeLegacy()
		if err := requireSeparateDatabases(ctx, target, legacy); err != nil {
			return err
		}
		parsedRunID, err := uuid.Parse(value.runID)
		if err != nil {
			return errors.New("catch-up requires a migration run UUID")
		}
		var final quizcraft.CatchUpReport
		for batch := 0; batch < value.maxBatches; batch++ {
			var cursor int64
			if err := target.QueryRow(ctx, `SELECT caught_up_event_id FROM quizcraft_migration_runs WHERE id=$1`, parsedRunID).Scan(&cursor); err != nil {
				return err
			}
			events, head, err := legacy.EventsAfter(ctx, cursor, value.eventLimit)
			if err != nil {
				return err
			}
			final, err = service.ApplyIncrementalEvents(ctx, parsedRunID, head, events)
			if err != nil {
				return err
			}
			if final.CaughtUp {
				if err := printJSON(os.Stdout, final); err != nil {
					return err
				}
				if final.Ready {
					return nil
				}
				return errors.New("incremental catch-up has unresolved migration exceptions")
			}
		}
		if err := printJSON(os.Stdout, final); err != nil {
			return err
		}
		return errors.New("incremental catch-up batch limit reached")
	case "shadow-gate":
		start, err := time.Parse(time.RFC3339, value.windowStart)
		if err != nil {
			return errors.New("shadow gate requires RFC3339 window-start")
		}
		end, err := time.Parse(time.RFC3339, value.windowEnd)
		if err != nil {
			return errors.New("shadow gate requires RFC3339 window-end")
		}
		report, err := service.EvaluateShadowGate(ctx, start, end, value.minimumSamples, value.mismatchThreshold)
		if err != nil {
			return err
		}
		if err := printJSON(os.Stdout, report); err != nil {
			return err
		}
		if report.Decision != "pass" {
			return errors.New("shadow comparison gate blocked")
		}
		return nil
	default:
		return errors.New("mode must be snapshot, full, resume, catch-up, or shadow-gate")
	}
}

func readSnapshotForTarget(ctx context.Context, target *pgxpool.Pool, path string) (quizcraft.LegacySnapshotArtifact, error) {
	if path == "" {
		return quizcraft.LegacySnapshotArtifact{}, errors.New("full and resume modes require snapshot-file")
	}
	artifact, err := quizcraft.ReadLegacySnapshotArtifact(path)
	if err != nil {
		return quizcraft.LegacySnapshotArtifact{}, err
	}
	targetIdentity, err := quizcraft.DatabaseIdentity(ctx, target)
	if err != nil {
		return quizcraft.LegacySnapshotArtifact{}, err
	}
	if targetIdentity == artifact.SourceDatabaseIdentity {
		return quizcraft.LegacySnapshotArtifact{}, errors.New("legacy snapshot source and quizcraft_v2 target databases must be physically distinct")
	}
	return artifact, nil
}

func requireSeparateDatabases(ctx context.Context, target *pgxpool.Pool, legacy *quizcraft.LegacyPostgresSource) error {
	targetIdentity, err := quizcraft.DatabaseIdentity(ctx, target)
	if err != nil {
		return err
	}
	legacyIdentity, err := legacy.DatabaseIdentity(ctx)
	if err != nil {
		return err
	}
	if targetIdentity == legacyIdentity {
		return errors.New("legacy source and temporary target databases must be physically distinct")
	}
	return nil
}

func openLegacy(ctx context.Context, databaseURL string) (*quizcraft.LegacyPostgresSource, func(), error) {
	if databaseURL == "" {
		return nil, func() {}, errors.New("legacy database URL is required")
	}
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, func() {}, err
	}
	source, err := quizcraft.NewLegacyPostgresSource(pool)
	if err != nil {
		pool.Close()
		return nil, func() {}, err
	}
	return source, pool.Close, nil
}

func printJSON(writer io.Writer, value any) error {
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}
