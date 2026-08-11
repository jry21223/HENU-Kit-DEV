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
	source, err := os.ReadFile("../../packages/api-contracts/openapi/library.yaml")
	fail(err)
	var spec document
	fail(yaml.Unmarshal(source, &spec))
	routes := map[string]string{}
	for path, methods := range spec.Paths {
		for _, operation := range methods {
			routes[operation.OperationID] = path
		}
	}
	required := map[string]string{"HealthRoute": "getLibraryHealth", "SummaryRoute": "getLibraryConsoleSummary", "WorkspaceRoute": "getLibraryWorkspace", "CommandRoute": "executeLibraryCommand", "OperationRoute": "getLibraryOperation", "PublicMaterialCatalogRoute": "getPublicMaterialCatalog", "DownloadStartRoute": "createPublicMaterialDownloadStart", "GlobalDownloadAggregateRoute": "getGlobalDownloadStartAggregate", "MaterialDownloadAggregateRoute": "getMaterialDownloadStartAggregate"}
	for _, operationID := range required {
		if routes[operationID] == "" {
			fail(fmt.Errorf("required Library operation %s is missing", operationID))
		}
	}
	digest := sha256.Sum256(source)
	generated := fmt.Sprintf(`// Code generated from library.yaml (SHA256 %x); DO NOT EDIT.
package contract

const (
	HealthRoute = %q
	SummaryRoute = %q
	WorkspaceRoute = %q
	CommandRoute = %q
	OperationRoute = %q
	PublicMaterialCatalogRoute = %q
	DownloadStartRoute = %q
	GlobalDownloadAggregateRoute = %q
	MaterialDownloadAggregateRoute = %q
)
`, digest, routes["getLibraryHealth"], routes["getLibraryConsoleSummary"], routes["getLibraryWorkspace"], routes["executeLibraryCommand"], routes["getLibraryOperation"], routes["getPublicMaterialCatalog"], routes["createPublicMaterialDownloadStart"], routes["getGlobalDownloadStartAggregate"], routes["getMaterialDownloadStartAggregate"])
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
