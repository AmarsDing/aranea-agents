package tool

import "testing"

func TestValidateToolConfigFields_schemaMatch(t *testing.T) {
	err := validateToolConfigFields(ToolUpsertInput{
		Key:              "custom_api",
		DisplayName:      "Custom",
		ConfigSchemaJSON: `{"type":"object","properties":{"api_key":{"type":"string"}},"required":["api_key"]}`,
		ConfigJSON:       `{"api_key":"secret"}`,
	})
	if err != nil {
		t.Fatalf("expected valid config: %v", err)
	}
}

func TestValidateToolConfigFields_schemaReject(t *testing.T) {
	err := validateToolConfigFields(ToolUpsertInput{
		Key:              "custom_api",
		DisplayName:      "Custom",
		ConfigSchemaJSON: `{"type":"object","properties":{"api_key":{"type":"string"}},"required":["api_key"]}`,
		ConfigJSON:       `{}`,
	})
	if err == nil {
		t.Fatal("expected schema validation error")
	}
}

func TestValidateMCPServerConfigJSON_stdio(t *testing.T) {
	if err := validateMCPServerConfigJSON(`{"transport":"stdio","command":"npx"}`); err != nil {
		t.Fatal(err)
	}
	if err := validateMCPServerConfigJSON(`{"transport":"stdio"}`); err == nil {
		t.Fatal("expected missing command error")
	}
}

func TestValidateMCPServerConfigJSON_sse(t *testing.T) {
	if err := validateMCPServerConfigJSON(`{"transport":"sse","url":"https://mcp.example/sse"}`); err != nil {
		t.Fatal(err)
	}
}
