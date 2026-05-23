package tool

import (
	"encoding/json"
	"strings"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

var (
	validToolRiskLevels = map[string]struct{}{
		"low": {}, "medium": {}, "high": {}, "critical": {},
	}
	validToolSources = map[string]struct{}{
		"builtin": {}, "mcp": {}, "system": {}, "external": {}, "custom": {},
	}
)

func validateToolUpsert(in ToolUpsertInput) error {
	key := strings.TrimSpace(in.Key)
	if key == "" {
		return kerrors.BadRequest("TOOL", "tool key is required")
	}
	if strings.TrimSpace(in.DisplayName) == "" {
		return kerrors.BadRequest("TOOL", "display name is required")
	}
	if in.RiskLevel != "" {
		if _, ok := validToolRiskLevels[strings.ToLower(in.RiskLevel)]; !ok {
			return kerrors.BadRequest("TOOL", "invalid risk_level")
		}
	}
	if in.Source != "" {
		if _, ok := validToolSources[strings.ToLower(in.Source)]; !ok {
			return kerrors.BadRequest("TOOL", "invalid source")
		}
	}
	for _, pair := range []struct {
		name, raw string
	}{
		{"parameters_schema_json", in.ParametersSchemaJSON},
		{"result_schema_json", in.ResultSchemaJSON},
		{"config_schema_json", in.ConfigSchemaJSON},
		{"config_json", in.ConfigJSON},
		{"default_config_json", in.DefaultConfigJSON},
		{"metadata_json", in.MetadataJSON},
	} {
		if err := requireJSONObject(pair.name, pair.raw); err != nil {
			return err
		}
	}
	return validateToolConfigFields(in)
}

func requireJSONObject(field, raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return kerrors.BadRequest("TOOL", field+" must be valid JSON")
	}
	if _, ok := v.(map[string]any); !ok && raw != "[]" {
		return kerrors.BadRequest("TOOL", field+" must be a JSON object")
	}
	return nil
}

func assertToolMutable(existing Tool, in ToolUpsertInput) error {
	if !existing.Readonly {
		return nil
	}
	if strings.TrimSpace(in.Key) != existing.Key {
		return kerrors.BadRequest("TOOL", "readonly tool key cannot change")
	}
	if in.Source != "" && !strings.EqualFold(in.Source, existing.Source) {
		return kerrors.BadRequest("TOOL", "readonly tool source cannot change")
	}
	return nil
}

func assertToolDeletable(t Tool) error {
	if t.Readonly {
		return kerrors.BadRequest("TOOL", "readonly tool cannot be deleted")
	}
	return nil
}
