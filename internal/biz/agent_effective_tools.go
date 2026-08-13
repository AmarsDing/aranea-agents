package biz

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"aranea-agents/internal/biz/shared"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// EffectiveAgentTool is one row in the agent effective tools matrix (legacy JSON shape).
type EffectiveAgentTool struct {
	ToolKey        string
	DisplayName    string
	Category       string
	Source         string
	Enabled        bool
	EffectiveState string
	Reason         string
}

// AgentEffectiveTools matches pkg/backend domain.AgentEffectiveTools JSON for API compatibility.
type AgentEffectiveTools struct {
	ToolsEnabled bool
	Profile      string
	Allow        []string
	Deny         []string
	Items        []EffectiveAgentTool
}

// AgentToolPolicyInput is the writable subset for PUT .../tools/policy.
type AgentToolPolicyInput struct {
	ToolsEnabled bool
	Profile      string
	Allow        []string
	Deny         []string
}

// searchToolsAllLimit is the page size used when fetching all tools for effective-tools computation.
// Must be large enough to cover the full tool catalog in a single page.
const searchToolsAllLimit = 5000

func jsonStringList(raw string, lg loggateway.Logger) []string {
	list, err := JSONStringList(raw)
	if err != nil {
		lg.Warn("json string list parse failed", loggateway.StepID("agent.tools"), loggateway.Err(err))
		return nil
	}
	return list
}

var toolGroupsFilesystem = []string{"read_file", "read_multiple_files", "save_file", "list_file", "search_file", "search_content", "replace_content", "diff_edit", "patch_file"}
var toolGroupsWeb = []string{ToolKeyWebResearch, "web_fetch", "duckduckgo_search", "gemini_web_fetch", "google_search", "arxiv_search", "wikipedia_search"}
var toolGroupsMemory = []string{"memory_search", "memory_get"}
var toolGroupsSkill = []string{"skill_search", "use_skill"}
var toolGroupsMedia = []string{"read_image", "read_document", "read_spreadsheet", "create_image", "tts"}
var toolGroupsRuntime = []string{"shell_exec", "claude_code", "workspace_exec"}
var toolGroupsMessaging = []string{"send_email"}
var toolGroupsSession = []string{"await_user_reply", "todo_write"}
var toolGroupsIntegration = []string{"call_agent", "knowledge_search", "mcp_tool_set", "mcp_broker"}
var toolGroupsSubagent = []string{"subagents_spawn", "subagents_list", "subagents_get", "subagents_cancel"}
var toolGroupsBrowser = []string{"browser"}

var toolGroupsComputerUse = []string{
	"computer_use_observe", "computer_use_screenshot",
	"computer_use_act", "computer_use_launch", "computer_use_session",
}

// syntheticShellExecTool matches internal/data builtin seeds when the tools table has no shell_exec row.
func syntheticShellExecTool() Tool {
	return Tool{
		Key:                  "shell_exec",
		DisplayName:          "Shell 命令",
		Description:          "执行本地 shell 命令。",
		Category:             "runtime",
		Source:               "builtin",
		RiskLevel:            "critical",
		Enabled:              false,
		RequiresConfirmation: true,
		ParametersSchemaJSON: `{"type":"object","properties":{"command":{"type":"string"},"workdir":{"type":"string"}},"required":["command"]}`,
	}
}

func syntheticWebSearchTool() Tool {
	return Tool{
		Key:                  "duckduckgo_search",
		DisplayName:          "DuckDuckGo 搜索",
		Description:          "使用 DuckDuckGo 搜索实时网络信息，返回标题、链接和摘要。",
		Category:             "web",
		Source:               "builtin",
		RiskLevel:            "medium",
		Enabled:              true,
		Readonly:             true,
		ParametersSchemaJSON: `{"type":"object","properties":{"query":{"type":"string","description":"搜索关键词"}},"required":["query"]}`,
	}
}

func syntheticWebFetchTool() Tool {
	return Tool{
		Key:                  "web_fetch",
		DisplayName:          "Web 抓取",
		Description:          "抓取 URL 并提取页面文本或 Markdown。",
		Category:             "web",
		Source:               "builtin",
		RiskLevel:            "medium",
		Enabled:              true,
		Readonly:             true,
		ParametersSchemaJSON: `{"type":"object","properties":{"url":{"type":"string"},"extract_mode":{"type":"string","enum":["markdown","text","json"]}},"required":["url"]}`,
	}
}

func cliAdminKeysFromRegistry(catalog []Tool) []string {
	var keys []string
	for _, t := range catalog {
		if strings.HasPrefix(t.Key, "cli_admin_") {
			keys = append(keys, t.Key)
		}
	}
	return keys
}

func expandToolGroup(name string, catalog []Tool) []string {
	switch strings.TrimSpace(name) {
	case "filesystem":
		return append([]string{}, toolGroupsFilesystem...)
	case "web":
		return append([]string{}, toolGroupsWeb...)
	case "memory":
		return append([]string{}, toolGroupsMemory...)
	case "skill":
		return append([]string{}, toolGroupsSkill...)
	case "media":
		return append([]string{}, toolGroupsMedia...)
	case "runtime":
		return append([]string{}, toolGroupsRuntime...)
	case "messaging":
		return append([]string{}, toolGroupsMessaging...)
	case "session":
		return append([]string{}, toolGroupsSession...)
	case "integration":
		return append([]string{}, toolGroupsIntegration...)
	case "subagent":
		return append([]string{}, toolGroupsSubagent...)
	case "browser":
		return append([]string{}, toolGroupsBrowser...)
	case "computeruse":
		return append([]string{}, toolGroupsComputerUse...)
	case "cli_admin":
		return cliAdminKeysFromRegistry(catalog)
	default:
		return nil
	}
}

func profileAllowSet(profile string, catalog []Tool) map[string]bool {
	result := map[string]bool{}
	for _, key := range toolProfiles[strings.TrimSpace(profile)] {
		if strings.HasPrefix(key, "group:") {
			gn := strings.TrimPrefix(key, "group:")
			for _, member := range expandToolGroup(gn, catalog) {
				result[member] = true
			}
			continue
		}
		result[key] = true
	}
	return result
}

// registryOptInOnlyKeys matches platform seeds with enabled=false: catalog row off still allows
// profile/allow JSON to opt in (e.g. shell_exec on "full"). Default-enabled tools (gemini_web_fetch)
// administratively disabled in Tools UI are forced into denySet so profiles cannot re-enable them.
//
// Note: "model_registry_sync" is cron-only (invoked via cronrunner.RegistrySyncAgent.RunSync),
// has no tool factory, and is not mapped in ToolsetConfigFromEffectiveKeys. It's listed here so
// the catalog row can be enabled for UI display without being auto-denied, but it will never be
// assembled as a regular agent tool.
var registryOptInOnlyKeys = map[string]bool{
	"shell_exec":          true,
	"send_email":          true,
	"claude_code":         true,
	"workspace_exec":      true,
	"create_image":        true,
	"tts":                 true,
	"generate_image":      true,
	"generate_video":      true,
	"image_to_video":      true,
	"subagents_spawn":     true,
	"subagents_list":      true,
	"subagents_get":       true,
	"subagents_cancel":    true,
	"browser":             true,
	"message":             true,
	"model_registry_sync": true,
	"mcp_tool_set":        true,
	"mcp_broker":          true,
	// cli_admin_* are seeded with enabled=false (seed_system_admin.go) as
	// opt-in-only admin tools. Without these entries, applyRegistryAdminDenials
	// hard-denied them for every agent — including __system_admin__ whose
	// system_admin profile explicitly names group:cli_admin — so the tools
	// could never be assembled (member agents hallucinated installs instead).
	"cli_admin_skill_list":             true,
	"cli_admin_skill_get":              true,
	"cli_admin_skill_install_from_url": true,
	"cli_admin_skill_import_status":    true,
	"cli_admin_skill_import_apply":     true,
	"cli_admin_agent_list":             true,
	"cli_admin_agent_get":              true,
	"cli_admin_pkg_install_from_url":   true,
	// computer_use_* are seeded with enabled=false (builtin_tools_seed.go) as
	// opt-in-only desktop automation tools: catalog row stays off, but an agent
	// whose profile/allow JSON names group:computeruse (e.g. spirit) can enable them.
	"computer_use_observe":    true,
	"computer_use_screenshot": true,
	"computer_use_act":        true,
	"computer_use_launch":     true,
	"computer_use_session":    true,
}

func applyRegistryAdminDenials(catalog []Tool, deny map[string]bool) {
	if deny == nil {
		return
	}
	for _, tool := range catalog {
		if tool.Enabled || registryOptInOnlyKeys[tool.Key] {
			continue
		}
		deny[tool.Key] = true
	}
}

var toolProfiles = map[string][]string{
	"chat_only": {},
	"read_only": {"datetime", "read_file", "read_multiple_files", "list_file", "search_file", "search_content", "todo_write"},
	"coding":    {"group:filesystem", "group:web", "group:skill", "group:session", "datetime"},
	"research":  {ToolKeyWebResearch, "web_fetch", "arxiv_search", "wikipedia_search", "read_file", "read_multiple_files", "list_file", "search_file", "search_content", "skill_search", "memory_search", "todo_write", "datetime"},
	"full":      {"group:filesystem", "group:web", "group:skill", "group:memory", "group:media", "group:runtime", "group:messaging", "group:session", "group:integration", "group:subagent", "group:browser", "group:cli_admin", "datetime"},

	"minimal":      {},
	"safe":         {"datetime", "read_file", "read_multiple_files", "list_file", "search_file", "search_content", "todo_write"},
	"system_admin": {"group:cli_admin", "web_fetch", "datetime"},
	"spirit":       {"plan_and_execute", "cancel_orchestration", "synthesize_results", "get_team_deliverable", "build_orchestration_graph", "memory_search", "group:subagent", "shell_exec", "datetime", "group:computeruse"},
}

func canonicalToolProfile(profile string) string {
	switch strings.ToLower(strings.TrimSpace(profile)) {
	case "":
		// Empty profile falls back to "coding" (the default in DefaultAgentRuntimeSettings).
		// Treating empty as "full" would bypass profile restrictions entirely.
		return "coding"
	case "chat_only", "minimal":
		return "chat_only"
	case "read_only", "safe":
		return "read_only"
	case "coding":
		return "coding"
	case "research":
		return "research"
	case "system_admin", "full":
		return "full"
	default:
		return profile
	}
}

// computePolicyAllowedSet merges built-in profile + ToolsAllowJSON into one set of catalog tool_key values.
// Keys are normalized ([NormalizeToolPolicyKey]) so UI aliases match platform rows uniformly.
func computePolicyAllowedSet(profile string, allowExtra []string, catalog []Tool) map[string]bool {
	prof := strings.TrimSpace(profile)
	out := profileAllowSet(prof, catalog)
	for _, key := range allowExtra {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if strings.HasPrefix(key, "group:") {
			for _, member := range expandToolGroup(strings.TrimPrefix(key, "group:"), catalog) {
				out[member] = true
			}
			continue
		}
		out[NormalizeToolPolicyKey(key)] = true
	}
	PropagateAllowAliases(out)
	return out
}

// computePolicyDenySet merges ToolsDenyJSON + optional group: entries into denied tool_key set (normalized).
func computePolicyDenySet(denyList []string, catalog []Tool) map[string]bool {
	out := map[string]bool{}
	for _, key := range denyList {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if strings.HasPrefix(key, "group:") {
			for _, member := range expandToolGroup(strings.TrimPrefix(key, "group:"), catalog) {
				out[member] = true
			}
			continue
		}
		out[NormalizeToolPolicyKey(key)] = true
	}
	PropagateDenyAliases(out)
	return out
}

// computeEffectiveToolState applies one catalog row against agent policy.
//
// Semantics (all tools, same rules):
//   - tools.enabled on the catalog row means "open by default": the tool may be used if the active
//     profile gate passes and the key is not denied.
//   - tools.enabled == false means "opt-in only": the tool is usable only if the expanded policy set
//     (profile presets + group:* + allow JSON) explicitly names this tool_key. This supports high
//     risk defaults (e.g. shell_exec) without per-tool code branches in callers.
//   - Deny list and ToolsEnabled global switch override everything else.
func computeEffectiveToolState(settings AgentRuntimeSettings, tool Tool, prof string, allowed, deny map[string]bool) (state, reason string, enabled bool) {
	// 1. Deny always wins — check first regardless of profile or allow set.
	if deny[tool.Key] {
		if !settings.ToolsEnabled {
			reason = "agent_tools_disabled"
		} else {
			reason = "agent_deny"
		}
		return "denied", reason, false
	}

	// 2. Global switch off.
	if !settings.ToolsEnabled {
		return "denied", "agent_tools_disabled", false
	}

	// 3. Allow: tool is open-by-default or explicitly named in policy, AND
	//    either the profile is "full" or the tool key is in the allowed set.
	toolOpenByDefault := tool.Enabled
	policyNamesKey := allowed[tool.Key]
	if (toolOpenByDefault || policyNamesKey) && (prof == "full" || allowed[tool.Key]) {
		return "allowed", "profile:" + settings.ToolsProfile, true
	}

	// 4. Not covered by any allow rule.
	return "denied", "global_disabled", false
}

func buildAgentEffectiveTools(settings AgentRuntimeSettings, catalog []Tool, lg loggateway.Logger) AgentEffectiveTools {
	allow := jsonStringList(settings.ToolsAllowJSON, lg)
	deny := jsonStringList(settings.ToolsDenyJSON, lg)

	prof := strings.TrimSpace(settings.ToolsProfile)
	allowedSet := computePolicyAllowedSet(prof, allow, catalog)
	denySet := computePolicyDenySet(deny, catalog)
	applyRegistryAdminDenials(catalog, denySet)

	registryKeys := make(map[string]bool, len(catalog))
	for _, tool := range catalog {
		registryKeys[tool.Key] = true
	}

	items := make([]EffectiveAgentTool, 0, len(catalog)+3)
	for _, tool := range catalog {
		st, rsn, en := computeEffectiveToolState(settings, tool, prof, allowedSet, denySet)
		items = append(items, EffectiveAgentTool{
			ToolKey:        tool.Key,
			DisplayName:    tool.DisplayName,
			Category:       tool.Category,
			Source:         tool.Source,
			Enabled:        en,
			EffectiveState: st,
			Reason:         rsn,
		})
	}

	const shellExecKey = "shell_exec"
	if !registryKeys[shellExecKey] && allowedSet[shellExecKey] {
		syn := syntheticShellExecTool()
		st, rsn, en := computeEffectiveToolState(settings, syn, prof, allowedSet, denySet)
		items = append(items, EffectiveAgentTool{
			ToolKey:        shellExecKey,
			DisplayName:    syn.DisplayName,
			Category:       syn.Category,
			Source:         syn.Source,
			Enabled:        en,
			EffectiveState: st,
			Reason:         rsn,
		})
	}

	const webSearchKey = "duckduckgo_search"
	if !registryKeys[webSearchKey] && allowedSet[webSearchKey] {
		syn := syntheticWebSearchTool()
		st, rsn, en := computeEffectiveToolState(settings, syn, prof, allowedSet, denySet)
		items = append(items, EffectiveAgentTool{
			ToolKey:        webSearchKey,
			DisplayName:    syn.DisplayName,
			Category:       syn.Category,
			Source:         syn.Source,
			Enabled:        en,
			EffectiveState: st,
			Reason:         rsn,
		})
	}
	const webFetchKey = "web_fetch"
	if !registryKeys[webFetchKey] && allowedSet[webFetchKey] {
		syn := syntheticWebFetchTool()
		st, rsn, en := computeEffectiveToolState(settings, syn, prof, allowedSet, denySet)
		items = append(items, EffectiveAgentTool{
			ToolKey:        webFetchKey,
			DisplayName:    syn.DisplayName,
			Category:       syn.Category,
			Source:         syn.Source,
			Enabled:        en,
			EffectiveState: st,
			Reason:         rsn,
		})
	}

	return AgentEffectiveTools{
		ToolsEnabled: settings.ToolsEnabled,
		Profile:      canonicalToolProfile(settings.ToolsProfile),
		Allow:        allow,
		Deny:         deny,
		Items:        items,
	}
}

// GetEffectiveTools returns merged tool catalog + agent runtime policy (legacy semantics).
func (u *AgentUsecase) GetEffectiveTools(ctx context.Context, agentID string) (AgentEffectiveTools, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return AgentEffectiveTools{}, apierror.BadRequest("AGENT", "agent id is required")
	}
	if _, err := u.reader.GetAgentByID(ctx, agentID); err != nil {
		return AgentEffectiveTools{}, err
	}
	settings, err := u.runtimeSettingsForEffective(ctx, agentID)
	if err != nil {
		return AgentEffectiveTools{}, err
	}
	all, err := u.tools.SearchTools(ctx, ToolListQuery{Limit: searchToolsAllLimit, Offset: 0})
	if err != nil {
		return AgentEffectiveTools{}, err
	}
	platform := loadWebResearchPlatformFromSys(ctx, u.sys)
	for i := range all.Items {
		EnrichToolRuntimeFieldsWithPlatform(&all.Items[i], platform, checkerToReadinessFunc(u.webResearchChecker))
	}
	eff := buildAgentEffectiveTools(settings, all.Items, u.lg)
	var overrides []ToolAgentOverride
	if o, oerr := u.tools.ListToolAgentOverridesByAgent(ctx, agentID); oerr == nil {
		overrides = o
		ApplyAgentToolOverrides(&eff, all.Items, overrides)
	}
	applyWebResearchEffectiveGate(u.webResearchChecker, &eff, all.Items, platform, overrides)
	return eff, nil
}

func (u *AgentUsecase) runtimeSettingsForEffective(ctx context.Context, agentID string) (AgentRuntimeSettings, error) {
	settings, err := u.settings.GetAgentRuntimeSettings(ctx, agentID)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			s := DefaultAgentRuntimeSettings()
			s.AgentID = agentID
			return withSettingDefaults(s), nil
		}
		return AgentRuntimeSettings{}, err
	}
	return withSettingDefaults(settings), nil
}

// UpdateAgentToolPolicy updates agent_runtime_settings tool columns and returns recomputed effective tools.
func (u *AgentUsecase) UpdateAgentToolPolicy(ctx context.Context, agentID string, in AgentToolPolicyInput) (AgentEffectiveTools, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return AgentEffectiveTools{}, apierror.BadRequest("AGENT", "agent id is required")
	}
	if _, err := u.reader.GetAgentByID(ctx, agentID); err != nil {
		return AgentEffectiveTools{}, err
	}
	settings, err := u.runtimeSettingsForEffective(ctx, agentID)
	if err != nil {
		return AgentEffectiveTools{}, err
	}
	settings.ToolsEnabled = in.ToolsEnabled
	if strings.TrimSpace(in.Profile) != "" {
		settings.ToolsProfile = strings.TrimSpace(in.Profile)
	}
	allowJSON, _ := json.Marshal(in.Allow)
	denyJSON, _ := json.Marshal(in.Deny)
	settings.ToolsAllowJSON = string(allowJSON)
	settings.ToolsDenyJSON = string(denyJSON)
	if _, err := u.settings.UpsertAgentRuntimeSettings(ctx, settings); err != nil {
		return AgentEffectiveTools{}, err
	}
	all, err := u.tools.SearchTools(ctx, ToolListQuery{Limit: searchToolsAllLimit, Offset: 0})
	if err != nil {
		return AgentEffectiveTools{}, err
	}
	settings, err = u.settings.GetAgentRuntimeSettings(ctx, agentID)
	if err != nil {
		return AgentEffectiveTools{}, err
	}
	settings = withSettingDefaults(settings)

	platform := loadWebResearchPlatformFromSys(ctx, u.sys)
	for i := range all.Items {
		EnrichToolRuntimeFieldsWithPlatform(&all.Items[i], platform, checkerToReadinessFunc(u.webResearchChecker))
	}
	eff := buildAgentEffectiveTools(settings, all.Items, u.lg)
	var overrides []ToolAgentOverride
	if o, oerr := u.tools.ListToolAgentOverridesByAgent(ctx, agentID); oerr == nil {
		overrides = o
		ApplyAgentToolOverrides(&eff, all.Items, overrides)
	}
	applyWebResearchEffectiveGate(u.webResearchChecker, &eff, all.Items, platform, overrides)
	return eff, nil
}

// ToolKeyInAllowJSON checks whether a specific tool key is present in a JSON allow list string.
// Used by CustomTool injection logic (e.g. build_orchestration_graph) to determine
// whether a non-Spirit agent should receive the tool.
func ToolKeyInAllowJSON(allowJSON, key string) bool {
	list, err := shared.JSONStringList(strings.TrimSpace(allowJSON))
	if err != nil {
		return false
	}
	for _, k := range list {
		if k == key {
			return true
		}
	}
	return false
}
