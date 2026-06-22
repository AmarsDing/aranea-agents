package planner

import (
	"encoding/json"
	"strings"
)

type builtinConfigJSON struct {
	ReasoningEffort *string `json:"reasoning_effort"`
	ThinkingEnabled *bool   `json:"thinking_enabled"`
	ThinkingTokens  *int    `json:"thinking_tokens"`
}

type A2UIResult struct {
	PlannerKind string
}

type a2uiConfigJSON struct {
	Instruction                           string `json:"instruction"`
	ServerToClientWithStandardCatalogJSON string `json:"server_to_client_with_standard_catalog_schema_json"`
	ClientToServerSchemaJSON              string `json:"client_to_server_schema_json"`
	ClientCapabilitiesSchemaJSON          string `json:"client_capabilities_schema_json"`
	ServerToClientOnlySchemaJSON          string `json:"server_to_client_only_schema_json"`
	StandardCatalogDefinitionJSON         string `json:"standard_catalog_definition_json"`
	CatalogDescriptionSchemaJSON          string `json:"catalog_description_schema_json"`
}

func parseBuiltinConfigJSON(raw string) builtinConfigJSON {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "{}" {
		return builtinConfigJSON{}
	}
	var cfg builtinConfigJSON
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return builtinConfigJSON{}
	}
	return cfg
}

func parseA2UIConfigJSON(raw string) a2uiConfigJSON {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "{}" {
		return a2uiConfigJSON{}
	}
	var cfg a2uiConfigJSON
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return a2uiConfigJSON{}
	}
	return cfg
}
