package tool

import (
	"encoding/json"
	"strings"

	"aranea-agents/internal/biz/shared"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

func validateToolConfigFields(in ToolUpsertInput) error {
	schema := strings.TrimSpace(in.ConfigSchemaJSON)
	if schema == "" || schema == "{}" {
		if strings.EqualFold(strings.TrimSpace(in.Source), "mcp") {
			if err := validateMCPServerConfigJSON(in.ConfigJSON); err != nil {
				return err
			}
			if err := validateMCPServerConfigJSON(in.DefaultConfigJSON); err != nil {
				return err
			}
		}
		return nil
	}
	for _, pair := range []struct {
		field, raw string
	}{
		{"config_json", in.ConfigJSON},
		{"default_config_json", in.DefaultConfigJSON},
	} {
		raw := strings.TrimSpace(pair.raw)
		if raw == "" || raw == "{}" {
			if pair.field == "default_config_json" {
				continue
			}
		}
		if err := shared.ValidateDocumentAgainstSchema("TOOL", schema, pair.raw); err != nil {
			return err
		}
	}
	if strings.EqualFold(strings.TrimSpace(in.Source), "mcp") {
		if err := validateMCPServerConfigJSON(in.ConfigJSON); err != nil {
			return err
		}
	}
	return nil
}

func validateToolConfigAgainstSchema(schemaJSON, configJSON string) error {
	if err := shared.ValidateDocumentAgainstSchema("TOOL", schemaJSON, configJSON); err != nil {
		return err
	}
	return validateMCPServerConfigJSON(configJSON)
}

func validateMCPServerConfigJSON(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "{}" {
		return nil
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return kerrors.BadRequest("TOOL", "mcp config_json must be a JSON object")
	}
	transport, _ := cfg["transport"].(string)
	if transport == "" {
		transport = "stdio"
	}
	switch strings.ToLower(strings.TrimSpace(transport)) {
	case "stdio":
		cmd, _ := cfg["command"].(string)
		if strings.TrimSpace(cmd) == "" {
			return kerrors.BadRequest("TOOL", "mcp stdio transport requires command")
		}
	case "sse", "streamable", "streamable_http":
		url, _ := cfg["url"].(string)
		if strings.TrimSpace(url) == "" {
			return kerrors.BadRequest("TOOL", "mcp "+transport+" transport requires url")
		}
	default:
		return kerrors.BadRequest("TOOL", "mcp transport must be stdio, sse, or streamable_http")
	}
	return nil
}
