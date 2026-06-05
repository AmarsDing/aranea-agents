package tool

import (
	"context"
	"encoding/json"
	"strings"
)

// WebResearchPlatformFields holds platform-level defaults for web_research.
type WebResearchPlatformFields struct {
	HasAPIKey   bool
	APIKey      string
	Provider    string
	MaxResults  int
	FetchTop    int
	SearchDepth string
	TimeoutSec  int
	HTTPProxy   string
}

// WebResearchReadinessChecker abstracts web_research readiness resolution.
type WebResearchReadinessChecker interface {
	ResolveReady(agentMap map[string]any, platform *WebResearchPlatformFields) bool
	CatalogReady(agentMap map[string]any, platform *WebResearchPlatformFields) bool
}

// webResearchPlatformFields converts WebResearchSetting to WebResearchPlatformFields.
func webResearchPlatformFields(s WebResearchSetting) *WebResearchPlatformFields {
	if !s.HasAPIKey && strings.TrimSpace(s.APIKey) == "" && strings.TrimSpace(s.Provider) == "" {
		return nil
	}
	return &WebResearchPlatformFields{
		HasAPIKey:   s.HasAPIKey,
		APIKey:      s.APIKey,
		Provider:    s.Provider,
		MaxResults:  s.MaxResults,
		FetchTop:    s.FetchTop,
		SearchDepth: s.SearchDepth,
		TimeoutSec:  s.TimeoutSec,
		HTTPProxy:   s.HTTPProxy,
	}
}

func webResearchPlatformFieldsPtr(s *WebResearchSetting) *WebResearchPlatformFields {
	if s == nil {
		return nil
	}
	return webResearchPlatformFields(*s)
}

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
	"search_file": {}, "search_content": {}, "replace_content": {}, "diff_edit": {}, "patch_file": {},
	"shell_exec": {}, "web_fetch": {}, ToolKeyWebResearch: {}, "duckduckgo_search": {}, "gemini_web_fetch": {},
	"google_search": {}, "arxiv_search": {}, "wikipedia_search": {},
	"send_email": {}, "todo_write": {}, "await_user_reply": {},
	"claude_code": {}, "workspace_exec": {},
}

// sessionBoundToolKeys need a live agent/session to execute even when globally enabled.
var sessionBoundToolKeys = map[string]struct{}{
	ToolKeyKnowledgeSearch: {}, ToolKeyKnowledgeReflect: {}, ToolKeyCallAgent: {},
	"mcp_tool_set": {}, ToolKeyMCPBroker: {},
	"memory_search": {}, "memory_get": {},
	"skill_search": {}, "use_skill": {},
}

// WebResearchCatalogReadyFunc is the function signature for checking web_research catalog readiness.
type WebResearchCatalogReadyFunc func(agentMap map[string]any, platform *WebResearchPlatformFields) bool

// WebResearchReadinessChecker adapts a WebResearchCatalogReadyFunc from a WebResearchReadinessChecker interface.
func CheckerToCatalogReadyFunc(c WebResearchReadinessChecker) WebResearchCatalogReadyFunc {
	if c == nil {
		return nil
	}
	return c.CatalogReady
}

// EnrichToolCatalogRuntime fills RuntimeStatus and RuntimeKind for API responses.
func EnrichToolCatalogRuntime(t *Tool) {
	EnrichToolCatalogRuntimeWithPlatform(t, nil, nil)
}

// EnrichToolCatalogRuntimeWithPlatform applies catalog runtime fields using optional platform web research settings.
func EnrichToolCatalogRuntimeWithPlatform(t *Tool, platform *WebResearchSetting, catalogReady WebResearchCatalogReadyFunc) {
	if t == nil {
		return
	}
	t.RuntimeKind = catalogRuntimeKind(*t)
	t.RuntimeStatus = catalogRuntimeStatus(*t, platform, catalogReady)
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

func catalogRuntimeStatus(t Tool, platform *WebResearchSetting, catalogReady WebResearchCatalogReadyFunc) string {
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
	if !catalogConfigReady(t, platform, catalogReady) {
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

func catalogConfigReady(t Tool, platform *WebResearchSetting, catalogReady WebResearchCatalogReadyFunc) bool {
	cfg := MergeToolConfigMaps(t.ConfigJSON, t.DefaultConfigJSON)
	switch t.Key {
	case "google_search":
		return configString(cfg, "api_key", "google_api_key") != "" &&
			configString(cfg, "cx", "engine_id", "google_cx", "search_engine_id") != ""
	case "gemini_web_fetch":
		return configString(cfg, "model", "gemini_model") != ""
	case ToolKeyWebResearch:
		pf := webResearchPlatformFieldsPtr(platform)
		return webResearchCatalogReady(cfg, pf, catalogReady)
	default:
		return true
	}
}

// webResearchCatalogReady delegates to the injected checker when available,
// otherwise falls back to a simple key-presence check.
func webResearchCatalogReady(agentMap map[string]any, platform *WebResearchPlatformFields, catalogReady WebResearchCatalogReadyFunc) bool {
	if catalogReady != nil {
		return catalogReady(agentMap, platform)
	}
	// Fallback: simple key-presence check
	if platform != nil && (platform.HasAPIKey || strings.TrimSpace(platform.APIKey) != "") {
		return true
	}
	if v, ok := agentMap["api_key"].(string); ok && strings.TrimSpace(v) != "" {
		return true
	}
	return platform != nil && platform.HasAPIKey
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

func MergeToolConfigMaps(baseJSON, defaultJSON string) map[string]any {
	out := map[string]any{}
	MergeJSONMapInto(out, baseJSON)
	MergeJSONMapInto(out, defaultJSON)
	return out
}

func MergeJSONMapInto(dst map[string]any, raw string) {
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

func enrichToolList(items []Tool, platform *WebResearchSetting, catalogReady WebResearchCatalogReadyFunc) []Tool {
	for i := range items {
		EnrichToolCatalogRuntimeWithPlatform(&items[i], platform, catalogReady)
	}
	return items
}

func LoadWebResearchPlatform(ctx context.Context, sys SettingRepo) *WebResearchSetting {
	if sys == nil {
		return nil
	}
	s, err := sys.GetWebResearch(ctx)
	if err != nil {
		return nil
	}
	return &s
}
