package biz

import (
	"encoding/json"
	"strings"
)

// Catalog runtime_status values (see 23 tools.design.md).
const (
	RuntimeStatusAvailable   = "available"
	RuntimeStatusCatalogOnly = "catalog_only"
	RuntimeStatusDisabled    = "disabled"
)

// Catalog runtime_kind values.
const (
	RuntimeKindFunction  = "function"
	RuntimeKindStreaming = "streaming"
	RuntimeKindApproval  = "approval"
)

// registryBackedToolKeys are catalog rows with a direct Registry / ToolSet factory path.
var registryBackedToolKeys = map[string]struct{}{
	"read_file": {}, "read_multiple_files": {}, "save_file": {}, "list_file": {},
	"search_file": {}, "search_content": {}, "replace_content": {},
	"shell_exec": {}, "web_fetch": {}, "duckduckgo_search": {}, "gemini_web_fetch": {},
	"google_search": {}, "arxiv_search": {}, "wikipedia_search": {},
	"send_email": {}, "todo_write": {}, "await_user_reply": {},
	"claude_code": {}, "workspace_exec": {},
}

// sessionBoundToolKeys need a live agent/session to execute even when globally enabled.
var sessionBoundToolKeys = map[string]struct{}{
	ToolKeyKnowledgeSearch: {}, ToolKeyCallAgent: {},
	"mcp_tool_set": {}, ToolKeyMCPBroker: {},
	"memory_search": {}, "memory_get": {},
	"skill_search": {}, "use_skill": {},
}

// EnrichToolCatalogRuntime fills RuntimeStatus and RuntimeKind for API responses.
func EnrichToolCatalogRuntime(t *Tool) {
	if t == nil {
		return
	}
	t.RuntimeKind = catalogRuntimeKind(*t)
	t.RuntimeStatus = catalogRuntimeStatus(*t)
}

func catalogRuntimeKind(t Tool) string {
	if t.RequiresConfirmation {
		return RuntimeKindApproval
	}
	if t.SupportsStreaming {
		return RuntimeKindStreaming
	}
	return RuntimeKindFunction
}

func catalogRuntimeStatus(t Tool) string {
	if !t.Enabled {
		return RuntimeStatusDisabled
	}
	if _, sessionOnly := sessionBoundToolKeys[t.Key]; sessionOnly {
		return RuntimeStatusCatalogOnly
	}
	src := strings.ToLower(strings.TrimSpace(t.Source))
	if src == "mcp" {
		return RuntimeStatusCatalogOnly
	}
	if src == "external" || src == "custom" {
		if !hasOpenAPIMetadata(t.MetadataJSON) {
			return RuntimeStatusCatalogOnly
		}
		return RuntimeStatusAvailable
	}
	if !catalogConfigReady(t) {
		return RuntimeStatusCatalogOnly
	}
	if _, ok := registryBackedToolKeys[t.Key]; ok {
		return RuntimeStatusAvailable
	}
	if strings.HasPrefix(t.Key, "working_memory.") {
		return RuntimeStatusAvailable
	}
	return RuntimeStatusCatalogOnly
}

func catalogConfigReady(t Tool) bool {
	cfg := mergeToolConfigMaps(t.ConfigJSON, t.DefaultConfigJSON)
	switch t.Key {
	case "google_search":
		return configString(cfg, "api_key", "google_api_key") != "" &&
			configString(cfg, "cx", "engine_id", "google_cx", "search_engine_id") != ""
	case "gemini_web_fetch":
		return configString(cfg, "model", "gemini_model") != ""
	default:
		return true
	}
}

func hasOpenAPIMetadata(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "{}" {
		return false
	}
	var m map[string]any
	if json.Unmarshal([]byte(raw), &m) != nil {
		return false
	}
	for _, k := range []string{"openapi_spec_url", "openapi_spec_data", "spec_url", "spec_data"} {
		if v, ok := m[k].(string); ok && strings.TrimSpace(v) != "" {
			return true
		}
	}
	return false
}

func mergeToolConfigMaps(baseJSON, defaultJSON string) map[string]any {
	out := map[string]any{}
	mergeJSONMapInto(out, baseJSON)
	mergeJSONMapInto(out, defaultJSON)
	return out
}

func mergeJSONMapInto(dst map[string]any, raw string) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "{}" {
		return
	}
	var patch map[string]any
	if json.Unmarshal([]byte(raw), &patch) != nil {
		return
	}
	for k, v := range patch {
		dst[k] = v
	}
}

func configString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}

func enrichToolList(items []Tool) []Tool {
	for i := range items {
		EnrichToolCatalogRuntime(&items[i])
	}
	return items
}
