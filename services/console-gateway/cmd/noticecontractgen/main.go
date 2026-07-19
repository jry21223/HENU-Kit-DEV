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
	contractPath := "../../packages/api-contracts/openapi/notice.yaml"
	source, err := os.ReadFile(contractPath)
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
		"SnapshotPath": "listConsoleNotices", "SourcePath": "createNoticeSource", "VersionPath": "createNoticeVersion",
		"ReviewPath": "reviewNoticeVersion", "DistributionPath": "distributeNoticeVersion", "OperationPath": "getNoticeOperation",
	}
	for _, operationID := range required {
		if routes[operationID] == "" {
			fail(fmt.Errorf("required notice owner operation %s is missing", operationID))
		}
	}
	digest := sha256.Sum256(source)
	generated := fmt.Sprintf(`// Code generated from notice.yaml (SHA256 %x); DO NOT EDIT.
package notice

const (
	SnapshotPath = %q
	SourcePath = %q
	VersionPath = %q
	ReviewPath = %q
	DistributionPath = %q
	OperationPath = %q
)
`, digest, routes["listConsoleNotices"], routes["createNoticeSource"], routes["createNoticeVersion"], routes["reviewNoticeVersion"], routes["distributeNoticeVersion"], routes["getNoticeOperation"])
	formatted, err := format.Source([]byte(generated))
	fail(err)
	fail(os.WriteFile("internal/notice/contract_generated.go", formatted, 0o644))
}

func fail(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
