package main

import (
	"crypto/sha256"
	"fmt"
	"go/format"
	"os"
	"sort"

	"gopkg.in/yaml.v3"
)

type document struct {
	Paths map[string]map[string]struct {
		OperationID string   `yaml:"operationId"`
		Tags        []string `yaml:"tags"`
	} `yaml:"paths"`
}

func main() {
	source, err := os.ReadFile("../../../packages/api-contracts/openapi/quizcraft.yaml")
	fail(err)
	var spec document
	fail(yaml.Unmarshal(source, &spec))
	routes := map[string]string{}
	domains := map[string]bool{}
	for path, methods := range spec.Paths {
		for method, operation := range methods {
			if operation.OperationID == "" {
				fail(fmt.Errorf("%s %s has no operationId", method, path))
			}
			if routes[operation.OperationID] != "" {
				fail(fmt.Errorf("duplicate operationId %s", operation.OperationID))
			}
			routes[operation.OperationID] = path
			for _, tag := range operation.Tags {
				domains[tag] = true
			}
		}
	}
	for _, domain := range []string{"Practice", "Favorites", "Ranking", "Feedback", "Workshop"} {
		if !domains[domain] {
			fail(fmt.Errorf("required QuizCraft domain %s is missing", domain))
		}
	}
	required := map[string]string{
		"ListBanksRoute":            "listPracticeBanks",
		"PersonalStatsRoute":        "getPersonalPracticeStats",
		"CreateSessionRoute":        "createPracticeSession",
		"SubmitAnswerRoute":         "submitPracticeAnswer",
		"CreatePortalSessionRoute":  "createPortalPracticeSession",
		"SubmitPortalAnswerRoute":   "submitPortalPracticeAnswer",
		"ListFavoritesRoute":        "listFavoriteQuestions",
		"OverallRankingRoute":       "getOverallRanking",
		"CreateFeedbackRoute":       "createQuestionFeedback",
		"ListFeedbackStatusesRoute": "listQuestionFeedbackStatuses",
		"FeedbackStatusRoute":       "getQuestionFeedbackStatus",
		"WorkshopImportRoute":       "importWorkshopBank",
	}
	keys := make([]string, 0, len(required))
	for constant, operationID := range required {
		if routes[operationID] == "" {
			fail(fmt.Errorf("required QuizCraft operation %s is missing", operationID))
		}
		keys = append(keys, constant)
	}
	sort.Strings(keys)
	digest := sha256.Sum256(source)
	generated := fmt.Sprintf("// Code generated from quizcraft.yaml (SHA256 %x); DO NOT EDIT.\npackage contract\n\nconst (\n", digest)
	for _, constant := range keys {
		generated += fmt.Sprintf("\t%s = %q\n", constant, routes[required[constant]])
	}
	generated += ")\n"
	formatted, err := format.Source([]byte(generated))
	fail(err)
	fail(os.MkdirAll("internal/contract", 0o755))
	fail(os.WriteFile("internal/contract/generated.go", formatted, 0o644))
}

func fail(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
