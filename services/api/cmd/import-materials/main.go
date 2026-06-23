package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"final-review-platform/services/api/internal/materialimport"
	"final-review-platform/services/api/pkg/config"
	"final-review-platform/services/api/pkg/database"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: go run ./cmd/import-materials <manifest.json>")
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
	result, err := materialimport.New(db, cfg.LocalUploadDir).ImportFile(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}
	output, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(output))
}
