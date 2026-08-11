package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestPortalGatewayCatalogContractMatchesGeneratedTypes(t *testing.T) {
	spec := readPortalGatewayContract(t)
	if err := validateQuizCraftCatalog(spec.Components.Schemas); err != nil {
		t.Fatal(err)
	}
}

func TestPortalGatewayCatalogGeneratorRejectsHardCodedTypeDrift(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*document)
	}{
		{
			name: "scalar type drift",
			mutate: func(spec *document) {
				catalog := spec.Components.Schemas["QuizCraftCatalogResponse"]
				bank := catalog.Properties["banks"].Items
				name := bank.Properties["name"]
				name.Type = schemaType{"integer"}
				bank.Properties["name"] = name
			},
		},
		{
			name: "UUID format drift",
			mutate: func(spec *document) {
				catalog := spec.Components.Schemas["QuizCraftCatalogResponse"]
				bank := catalog.Properties["banks"].Items
				bankID := bank.Properties["bank_id"]
				bankID.Format = ""
				bank.Properties["bank_id"] = bankID
			},
		},
		{
			name: "new required property",
			mutate: func(spec *document) {
				catalog := spec.Components.Schemas["QuizCraftCatalogResponse"]
				bank := catalog.Properties["banks"].Items
				bank.Properties["future_field"] = schema{Type: schemaType{"string"}}
				bank.Required = append(bank.Required, "future_field")
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			spec := readPortalGatewayContract(t)
			test.mutate(&spec)
			source, err := yaml.Marshal(spec)
			if err != nil {
				t.Fatal(err)
			}
			temp := t.TempDir()
			contractPath := filepath.Join(temp, "portal-gateway.yaml")
			if err := os.WriteFile(contractPath, source, 0o600); err != nil {
				t.Fatal(err)
			}
			command := exec.Command(
				"go", "run", ".",
				"-contract", contractPath,
				"-go-output", filepath.Join(temp, "generated.go"),
				"-ts-output", filepath.Join(temp, "generated.ts"),
			)
			if output, err := command.CombinedOutput(); err == nil {
				t.Fatalf("generator accepted %s:\n%s", test.name, output)
			}
		})
	}
}

func readPortalGatewayContract(t *testing.T) document {
	t.Helper()
	source, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "packages", "api-contracts", "openapi", "portal-gateway.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var spec document
	if err := yaml.Unmarshal(source, &spec); err != nil {
		t.Fatal(err)
	}
	return spec
}
