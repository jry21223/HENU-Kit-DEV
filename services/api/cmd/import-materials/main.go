package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	"final-review-platform/services/api/internal/materialimport"
	"final-review-platform/services/api/pkg/config"
	"final-review-platform/services/api/pkg/database"
)

func main() {
	dryRun := flag.Bool("dry-run", false, "validate and report planned changes without writing to the database")
	checkRelease := flag.Bool("check-release", false, "run internal-release acceptance checks against the import report and exit non-zero if any check fails")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: go run ./cmd/import-materials [-dry-run] [-check-release] <manifest.json>")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	if *checkRelease && !*dryRun {
		fmt.Fprintln(os.Stderr, "-check-release must be used with -dry-run so a failed release gate cannot persist partial rollout data")
		flag.Usage()
		os.Exit(2)
	}
	cfg := config.Load()
	db, err := database.Connect(cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	if err := database.EnsureExtensions(db); err != nil {
		log.Fatal(err)
	}
	if cfg.AutoMigrate {
		if err := database.AutoMigrate(db); err != nil {
			log.Fatal(err)
		}
	}
	importer := materialimport.New(db, cfg.LocalUploadDir)
	var result materialimport.Result
	if *dryRun {
		result, err = importer.ImportFileDryRun(flag.Arg(0))
	} else {
		result, err = importer.ImportFile(flag.Arg(0))
	}
	if err != nil {
		log.Fatal(err)
	}
	if *checkRelease {
		releaseCheck := materialimport.CheckReleaseReadiness(result)
		result.ReleaseCheck = &releaseCheck
	}
	output, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(output))
	if result.ReleaseCheck != nil && !result.ReleaseCheck.Passed {
		os.Exit(1)
	}
}
