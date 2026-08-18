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
	IsReady(agentMap map[string]any, platform *WebResearchPlatformFields) bool
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

// Runtime status values (see 23 tools.design.md).
const (
	RuntimeStatusAvailable      = "available"
	RuntimeStatusRegisteredOnly = "registered_only"
	RuntimeStatusDisabled       = "disabled"
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
	"read_lints": {}, "delete_file": {},
	"shell_exec": {}, "web_fetch": {}, ToolKeyWebResearch: {}, "duckduckgo_search": {}, "gemini_web_fetch": {},
	"google_search": {}, "arxiv_search": {}, "wikipedia_search": {},
	"send_email": {}, "todo_write": {}, "await_user_reply": {},
	"claude_code": {},
	// workspace_exec 不在此列：运行时尚未实现，装配路径
	// （trpc.PruneUnconfiguredToolFlags）对其一律强制关闭并 Warn，
	// catalog 必须报 registered_only 与运行时行为保持一致（批次 4 D5）。
}

// spiritOrchestrationToolKeys are orchestration tools injected via CustomTools path
// (internal/service/cli_admin_tools.go spiritCustomTools). They have runtime implementations
// but bypass the Registry factory path, so they are not in registryBackedToolKeys.
// Available only to Spirit Agent (or agents with explicit ToolsAllowJSON grant).
var spiritOrchestrationToolKeys = map[string]struct{}{
	"plan_and_execute": {}, "cancel_orchestration": {},
	"synthesize_results": {}, "build_orchestration_graph": {},
	// check_progress removed: the system-push pattern (checkAllTeamsCompleted
	// → SynthesizeResults → ExecuteTurn) replaces LLM polling. The Spirit
	// LLM receives an injected synthesis message when teams complete, so
	// it no longer needs to poll.
}

// sessionBoundToolKeys need a live agent/session to execute even when globally enabled.
var sessionBoundToolKeys = map[string]struct{}{
	ToolKeyKnowledgeSearch: {}, ToolKeyKnowledgeReflect: {}, ToolKeyKnowledgeWrite: {}, ToolKeyCallAgent: {},
	"mcp_tool_set": {}, ToolKeyMCPBroker: {},
	"memory_search": {}, "memory_get": {},
	"skill_search": {}, "use_skill": {},
	// subagents_* require SubAgentService wired at runtime (cfg.SubAgent && cfg.SubAgentService != nil).
	"subagents_spawn": {}, "subagents_list": {}, "subagents_get": {}, "subagents_cancel": {},
}

// WebResearchReadinessFunc is the function signature for checking web_research tool readiness.
type WebResearchReadinessFunc func(agentMap map[string]any, platform *WebResearchPlatformFields) bool

// CheckerToReadinessFunc adapts a WebResearchReadinessFunc from a WebResearchReadinessChecker interface.
func CheckerToReadinessFunc(c WebResearchReadinessChecker) WebResearchReadinessFunc {
	if c == nil {
		return nil
	}
	return c.IsReady
}

// EnrichToolRuntimeFieldsWithPlatform applies tool runtime fields using optional platform web research settings.
func EnrichToolRuntimeFieldsWithPlatform(t *Tool, platform *WebResearchSetting, readiness WebResearchReadinessFunc) {
	if t == nil {
		return
	}
	t.RuntimeKind = toolRuntimeKind(*t)
	t.RuntimeStatus = toolRuntimeStatus(*t, platform, readiness)
}

func toolRuntimeKind(t Tool) string {
	if t.RequiresConfirmation {
		return RuntimeKindApproval
	}
	if t.SupportsStreaming {
		return RuntimeKindStreaming
	}
	return RuntimeKindFunction
}

func toolRuntimeStatus(t Tool, platform *WebResearchSetting, readiness WebResearchReadinessFunc) string {
	if !t.Enabled {
		return RuntimeStatusDisabled
	}
	if _, sessionOnly := sessionBoundToolKeys[t.Key]; sessionOnly {
		return RuntimeStatusRegisteredOnly
	}
	src := strings.ToLower(strings.TrimSpace(t.Source))
	if src == "mcp" {
		return RuntimeStatusRegisteredOnly
	}
	if src == "external" || src == "custom" {
		if !hasOpenAPIMetadata(t.MetadataJSON) {
			return RuntimeStatusRegisteredOnly
		}
		return RuntimeStatusAvailable
	}
	if !toolConfigReady(t, platform, readiness) {
		return RuntimeStatusRegisteredOnly
	}
	if _, ok := spiritOrchestrationToolKeys[t.Key]; ok {
		return RuntimeStatusAvailable
	}
	if _, ok := registryBackedToolKeys[t.Key]; ok {
		return RuntimeStatusAvailable
	}
	if strings.HasPrefix(t.Key, "working_memory.") {
		return RuntimeStatusAvailable
	}
	return RuntimeStatusRegisteredOnly
}

func toolConfigReady(t Tool, platform *WebResearchSetting, readiness WebResearchReadinessFunc) bool {
	// fallback 垫底、用户配置优先（BUG-2：此前参数顺序颠倒，默认值覆盖用户配置）。
	cfg := MergeToolConfigMaps(t.DefaultConfigJSON, t.ConfigJSON)
	switch t.Key {
	case "google_search":
		return configString(cfg, "api_key", "google_api_key") != "" &&
			configString(cfg, "cx", "engine_id", "google_cx", "search_engine_id") != ""
	case "gemini_web_fetch":
		return configString(cfg, "model", "gemini_model") != ""
	case ToolKeyWebResearch:
		pf := webResearchPlatformFieldsPtr(platform)
		return webResearchToolReady(cfg, pf, readiness)
	default:
		return true
	}
}

// webResearchToolReady delegates to the injected checker when available,
// otherwise falls back to a simple key-presence check.
func webResearchToolReady(agentMap map[string]any, platform *WebResearchPlatformFields, readiness WebResearchReadinessFunc) bool {
	if readiness != nil {
		return readiness(agentMap, platform)
	}
	// Fallback: simple key-presence check
	if platform != nil && (platform.HasAPIKey || strings.TrimSpace(platform.APIKey) != "") {
		return true
	}
	if v, ok := agentMap["api_key"].(string); ok && strings.TrimSpace(v) != "" {
		return true
	}
	return false
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

// MergeToolConfigMaps merges two JSON config maps. Keys from overlayJSON
// overwrite keys from baseJSON when overlapping.
func MergeToolConfigMaps(baseJSON, overlayJSON string) map[string]any {
	out := map[string]any{}
	MergeJSONMapInto(out, baseJSON)
	MergeJSONMapInto(out, overlayJSON)
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

func enrichToolList(items []Tool, platform *WebResearchSetting, readiness WebResearchReadinessFunc) []Tool {
	for i := range items {
		EnrichToolRuntimeFieldsWithPlatform(&items[i], platform, readiness)
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
