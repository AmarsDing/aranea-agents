package tool

import (
	"encoding/json"
	"strings"

	"aranea-agents/internal/biz/shared"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/outboundguard"
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

func validateToolConfigAgainstSchema(source, schemaJSON, configJSON string) error {
	if err := shared.ValidateDocumentAgainstSchema("TOOL", schemaJSON, configJSON); err != nil {
		return err
	}
	if strings.EqualFold(strings.TrimSpace(source), "mcp") {
		return validateMCPServerConfigJSON(configJSON)
	}
	return nil
}

// ToolConfigMask replaces secret values in API responses. Write paths treat an
// incoming mask as "keep the stored secret" (see MergeMaskedToolConfig).
const ToolConfigMask = "********"

// sensitiveConfigKey reports whether a config property holds a secret: the
// config schema marks it format:"password", or the key name matches the
// well-known secret vocabulary (same set as RedactToolPreview).
func sensitiveConfigKey(key string, passwordProps map[string]bool) bool {
	if passwordProps[key] {
		return true
	}
	k := strings.ToLower(key)
	for _, marker := range []string{"api_key", "apikey", "api-key", "secret", "token", "password", "authorization"} {
		if strings.Contains(k, marker) {
			return true
		}
	}
	return false
}

// passwordFormatProps parses a JSON Schema document and returns the set of
// property names declared with format:"password".
func passwordFormatProps(schemaJSON string) map[string]bool {
	var schema struct {
		Properties map[string]struct {
			Format string `json:"format"`
		} `json:"properties"`
	}
	if err := json.Unmarshal([]byte(schemaJSON), &schema); err != nil || len(schema.Properties) == 0 {
		return nil
	}
	out := make(map[string]bool, len(schema.Properties))
	for name, prop := range schema.Properties {
		if strings.EqualFold(strings.TrimSpace(prop.Format), "password") {
			out[name] = true
		}
	}
	return out
}

// RedactToolConfigJSON masks sensitive values in configJSON for API responses
// so secrets never leave the process. Non-string or empty values are left
// untouched (the mask implies a non-empty secret). Unparseable input is
// returned unchanged — validation upstream already guarantees shape.
func RedactToolConfigJSON(configJSON, schemaJSON string) string {
	raw := strings.TrimSpace(configJSON)
	if raw == "" || raw == "{}" {
		return configJSON
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return configJSON
	}
	props := passwordFormatProps(schemaJSON)
	changed := false
	for k, v := range cfg {
		s, ok := v.(string)
		if !ok || s == "" || s == ToolConfigMask {
			continue
		}
		if sensitiveConfigKey(k, props) {
			cfg[k] = ToolConfigMask
			changed = true
		}
	}
	if !changed {
		return configJSON
	}
	b, err := json.Marshal(cfg)
	if err != nil {
		return configJSON
	}
	return string(b)
}

// MergeMaskedToolConfig restores stored secret values where the incoming
// config carries the ToolConfigMask emitted by RedactToolConfigJSON, so a
// client round-tripping a redacted response cannot clobber secrets. Any other
// value (including empty string) is treated as an intentional overwrite.
func MergeMaskedToolConfig(existingJSON, incomingJSON, schemaJSON string) string {
	existing := strings.TrimSpace(existingJSON)
	incoming := strings.TrimSpace(incomingJSON)
	if existing == "" || existing == "{}" || incoming == "" || incoming == "{}" {
		return incomingJSON
	}
	var oldCfg, newCfg map[string]any
	if err := json.Unmarshal(existing, &oldCfg); err != nil {
		return incomingJSON
	}
	if err := json.Unmarshal(incoming, &newCfg); err != nil {
		return incomingJSON
	}
	props := passwordFormatProps(schemaJSON)
	changed := false
	for k, v := range newCfg {
		if v != ToolConfigMask {
			continue
		}
		if oldVal, ok := oldCfg[k]; ok && sensitiveConfigKey(k, props) {
			newCfg[k] = oldVal
			changed = true
		}
	}
	if !changed {
		return incomingJSON
	}
	b, err := json.Marshal(newCfg)
	if err != nil {
		return incomingJSON
	}
	return string(b)
}

func validateMCPServerConfigJSON(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "{}" {
		return nil
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return apierror.BadRequest("TOOL", "mcp config_json must be a JSON object")
	}
	transport, _ := cfg["transport"].(string)
	if transport == "" {
		transport = "stdio"
	}
	switch strings.ToLower(strings.TrimSpace(transport)) {
	case "stdio":
		cmd, _ := cfg["command"].(string)
		if strings.TrimSpace(cmd) == "" {
			return apierror.BadRequest("TOOL", "mcp stdio transport requires command")
		}
	case "sse", "streamable", "streamable_http":
		url, _ := cfg["url"].(string)
		if strings.TrimSpace(url) == "" {
			return apierror.BadRequest("TOOL", "mcp "+transport+" transport requires url")
		}
		if err := outboundguard.ValidateURL(url); err != nil {
			return apierror.BadRequest("TOOL", "mcp url failed SSRF check: "+err.Error())
		}
	default:
		return apierror.BadRequest("TOOL", "mcp transport must be stdio, sse, or streamable_http")
	}
	return nil
}
