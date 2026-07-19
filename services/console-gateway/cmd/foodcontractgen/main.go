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
	for _, id := range []string{"getFoodWorkspace", "executeFoodCommand", "getFoodOperation"} {
		if routes[id] == "" {
			fail(fmt.Errorf("required Food operation %s is missing", id))
		}
	}
	digest := sha256.Sum256(source)
	generated := fmt.Sprintf(`// Code generated from food.yaml (SHA256 %x); DO NOT EDIT.
package food

const (
	WorkspacePath = %q
	CommandPath = %q
	OperationPath = %q
)
`, digest, routes["getFoodWorkspace"], routes["executeFoodCommand"], routes["getFoodOperation"])
	formatted, err := format.Source([]byte(generated))
	fail(err)
	fail(os.MkdirAll("internal/food", 0o755))
	fail(os.WriteFile("internal/food/contract_generated.go", formatted, 0o644))
}
func fail(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
