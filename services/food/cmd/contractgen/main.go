package main

import (
	"crypto/sha256"
	"fmt"
	"go/format"
	"os"

	"gopkg.in/yaml.v3"
)

type document struct {
	Paths map[string]map[string]struct {
		OperationID string `yaml:"operationId"`
	} `yaml:"paths"`
}

func main() {
	source, err := os.ReadFile("../../packages/api-contracts/openapi/food.yaml")
	fail(err)
	var spec document
	fail(yaml.Unmarshal(source, &spec))
	routes := map[string]string{}
	for path, methods := range spec.Paths {
		for _, operation := range methods {
			routes[operation.OperationID] = path
		}
	}
	required := map[string]string{
		"HealthRoute":     "getFoodHealth",
		"SummaryRoute":    "getFoodConsoleSummary",
		"WorkspaceRoute":  "getFoodWorkspace",
		"CommandRoute":    "executeFoodCommand",
		"OperationRoute":  "getFoodOperation",
		"CreatePostRoute": "createFoodPost",
		"ListPostsRoute":  "listFoodPosts",
		"MyPostsRoute":    "listMyFoodPosts",
		"PostRoute":       "getFoodPost",
		"PostImageRoute":  "getFoodPostImage",
		"VenuesRoute":     "listFoodVenues",
	}
	for _, operationID := range required {
		if routes[operationID] == "" {
			fail(fmt.Errorf("required Food operation %s is missing", operationID))
		}
	}
	digest := sha256.Sum256(source)
	generated := fmt.Sprintf(`// Code generated from food.yaml (SHA256 %x); DO NOT EDIT.
package contract

const (
	HealthRoute = %q
	SummaryRoute = %q
	WorkspaceRoute = %q
	CommandRoute = %q
	OperationRoute = %q
	CreatePostRoute = %q
	ListPostsRoute = %q
	MyPostsRoute = %q
	PostRoute = %q
	PostImageRoute = %q
	VenuesRoute = %q
)
`, digest, routes["getFoodHealth"], routes["getFoodConsoleSummary"], routes["getFoodWorkspace"], routes["executeFoodCommand"], routes["getFoodOperation"], routes["createFoodPost"], routes["listFoodPosts"], routes["listMyFoodPosts"], routes["getFoodPost"], routes["getFoodPostImage"], routes["listFoodVenues"])
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
