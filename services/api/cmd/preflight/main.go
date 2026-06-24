package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"final-review-platform/services/api/internal/preflight"
	"final-review-platform/services/api/pkg/config"
)

type output struct {
	Passed bool              `json:"passed"`
	Checks []preflight.Check `json:"checks"`
}

func main() {
	var envFile string
	var checkFiles bool
	var jsonOutput bool
	flag.StringVar(&envFile, "env-file", env("APP_ENV_FILE", ""), "optional .env file to load before running checks")
	flag.BoolVar(&checkFiles, "check-files", boolEnv("PREFLIGHT_CHECK_FILES", true), "verify mounted key/cert/upload paths exist")
	flag.BoolVar(&jsonOutput, "json", boolEnv("PREFLIGHT_JSON", false), "print machine-readable JSON")
	flag.Parse()

	if strings.TrimSpace(envFile) != "" {
		if err := preflight.LoadEnvFile(envFile); err != nil {
			fmt.Fprintf(os.Stderr, "preflight env load failed: %v\n", err)
			os.Exit(2)
		}
	}

	checks := preflight.Run(config.Load(), preflight.Options{CheckFiles: checkFiles})
	passed := preflight.Passed(checks)
	if jsonOutput {
		encoded, err := json.MarshalIndent(output{Passed: passed, Checks: checks}, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "preflight output failed: %v\n", err)
			os.Exit(2)
		}
		fmt.Println(string(encoded))
	} else {
		for _, item := range checks {
			status := "PASS"
			if !item.Passed {
				status = "FAIL"
			}
			fmt.Printf("%s %s: %s\n", status, item.Name, item.Message)
		}
		if passed {
			fmt.Println("production preflight passed")
		} else {
			fmt.Printf("production preflight failed: %d check(s) failed\n", len(preflight.FailedChecks(checks)))
		}
	}
	if !passed {
		os.Exit(1)
	}
}

func env(key string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func boolEnv(key string, fallback bool) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}
