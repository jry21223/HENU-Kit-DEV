package main

import "testing"

func TestValidateNoticeURLContractsRequiresBoundedIRIs(t *testing.T) {
	validURL := schema{Type: "string", Format: "iri", Pattern: "^https://", MaxLength: 2048, Description: "Bounded public HTTPS IRI."}
	validSchemas := map[string]schema{
		"CreateNoticeSourceRequest":  {Properties: map[string]schema{"canonical_url": validURL}},
		"CreateNoticeVersionRequest": {Properties: map[string]schema{"source_url": validURL}},
		"NoticeVersion":              {Properties: map[string]schema{"source_url": validURL}},
	}
	if err := validateNoticeURLContracts(validSchemas); err != nil {
		t.Fatalf("valid Notice URL contracts rejected: %v", err)
	}

	for name, mutate := range map[string]func(map[string]schema){
		"canonical URL accepts RFC 3986 uri only": func(schemas map[string]schema) {
			schema := schemas["CreateNoticeSourceRequest"]
			property := schema.Properties["canonical_url"]
			property.Format = "uri"
			schema.Properties["canonical_url"] = property
			schemas["CreateNoticeSourceRequest"] = schema
		},
		"version URL has no byte-compatible bound": func(schemas map[string]schema) {
			schema := schemas["CreateNoticeVersionRequest"]
			property := schema.Properties["source_url"]
			property.MaxLength = 0
			schema.Properties["source_url"] = property
			schemas["CreateNoticeVersionRequest"] = schema
		},
		"snapshot URL has no description": func(schemas map[string]schema) {
			schema := schemas["NoticeVersion"]
			property := schema.Properties["source_url"]
			property.Description = ""
			schema.Properties["source_url"] = property
			schemas["NoticeVersion"] = schema
		},
	} {
		t.Run(name, func(t *testing.T) {
			schemas := cloneNoticeURLSchemas(validSchemas)
			mutate(schemas)
			if err := validateNoticeURLContracts(schemas); err == nil {
				t.Fatal("invalid Notice URL contract accepted")
			}
		})
	}
}

func cloneNoticeURLSchemas(source map[string]schema) map[string]schema {
	copy := make(map[string]schema, len(source))
	for name, value := range source {
		properties := make(map[string]schema, len(value.Properties))
		for propertyName, property := range value.Properties {
			properties[propertyName] = property
		}
		value.Properties = properties
		copy[name] = value
	}
	return copy
}
