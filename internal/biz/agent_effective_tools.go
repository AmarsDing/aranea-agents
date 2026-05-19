package biz

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	kerrors "github.com/go-kratos/kratos/v2/errors"
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

func jsonStringList(raw string) []string {
	var result []string
	if json.Unmarshal([]byte(strings.TrimSpace(raw)), &result) != nil {
		return []string{}
	}
	return result
}

var toolGroupsFilesystem = []string{"read_file", "read_multiple_files", "save_file", "list_file", "search_file", "search_content", "replace_content"}
var toolGroupsWeb = []string{"duckduckgo_search", "web_fetch", "gemini_web_fetch", "google_search", "arxiv_search", "wikipedia_search"}
var toolGroupsMemory = []string{"memory_search", "memory_get"}
var toolGroupsSkill = []string{"skill_search", "use_skill"}
var toolGroupsMedia = []string{"read_image", "read_document", "create_image", "tts"}
var toolGroupsRuntime = []string{"shell_exec", "claude_code", "workspace_exec"}
var toolGroupsMessaging = []string{"send_email"}
var toolGroupsSession = []string{"await_user_reply", "todo_write"}

// syntheticShellExecCatalogTool matches internal/data builtin seeds when the tools table has no shell_exec row.
func syntheticShellExecCatalogTool() Tool {
	return Tool{
		Key:                  "shell_exec",
		DisplayName:          "Shell 命令",
		Description:          "执行本地 shell 命令。",
		Category:             "runtime",
		Source:               "builtin",
		RiskLevel:            "critical",
		Enabled:              false,
		RequiresConfirmation: true,
		ParametersSchemaJSON: `{"type":"object","properties":{"command":{"type":"string"},"working_dir":{"type":"string"}},"required":["command"]}`,
	}
}

func syntheticWebSearchCatalogTool() Tool {
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

func syntheticWebFetchCatalogTool() Tool {
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

func cliAdminKeysFromCatalog(catalog []Tool) []string {
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
	case "cli_admin":
		return cliAdminKeysFromCatalog(catalog)
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

var toolProfiles = map[string][]string{
	"chat_only": {},
	"read_only": {"datetime", "read_file", "read_multiple_files", "list_file", "search_file", "search_content", "todo_write"},
	"coding":    {"group:filesystem", "group:web", "group:skill", "group:session", "datetime"},
	"research":  {"duckduckgo_search", "web_fetch", "gemini_web_fetch", "google_search", "arxiv_search", "wikipedia_search", "read_file", "read_multiple_files", "list_file", "search_file", "search_content", "skill_search", "memory_search", "todo_write", "datetime"},
	"full":      {"group:filesystem", "group:web", "group:skill", "group:memory", "group:media", "group:runtime", "group:messaging", "group:session", "group:cli_admin", "datetime"},

	"minimal":      {},
	"safe":         {"datetime", "read_file", "read_multiple_files", "list_file", "search_file", "search_content", "todo_write"},
	"system_admin": {"group:cli_admin", "web_fetch", "datetime"},
}

func canonicalToolProfile(profile string) string {
	switch strings.ToLower(strings.TrimSpace(profile)) {
	case "":
		return ""
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
// Keys are normalized ([normalizeToolPolicyKey]) so UI aliases match platform rows uniformly.
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
		out[normalizeToolPolicyKey(key)] = true
	}
	propagateAllowAliases(out)
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
		out[normalizeToolPolicyKey(key)] = true
	}
	propagateDenyAliases(out)
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
	state = "denied"
	reason = "global_disabled"
	catalogOpenByDefault := tool.Enabled
	policyNamesKey := allowed[tool.Key]
	baseEnabled := settings.ToolsEnabled && (catalogOpenByDefault || policyNamesKey)
	if baseEnabled && (prof == "" || prof == "full" || allowed[tool.Key]) {
		state = "allowed"
		reason = "profile:" + settings.ToolsProfile
	}
	if deny[tool.Key] {
		state = "denied"
		reason = "agent_deny"
	}
	if !settings.ToolsEnabled {
		reason = "agent_tools_disabled"
	}
	enabled = baseEnabled && state == "allowed"
	return state, reason, enabled
}

func buildAgentEffectiveTools(settings AgentRuntimeSettings, catalog []Tool) AgentEffectiveTools {
	allow := jsonStringList(settings.ToolsAllowJSON)
	deny := jsonStringList(settings.ToolsDenyJSON)

	prof := strings.TrimSpace(settings.ToolsProfile)
	allowedSet := computePolicyAllowedSet(prof, allow, catalog)
	denySet := computePolicyDenySet(deny, catalog)

	catalogKeys := make(map[string]bool, len(catalog))
	for _, tool := range catalog {
		catalogKeys[tool.Key] = true
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
	if !catalogKeys[shellExecKey] && allowedSet[shellExecKey] {
		syn := syntheticShellExecCatalogTool()
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
	if !catalogKeys[webSearchKey] && allowedSet[webSearchKey] {
		syn := syntheticWebSearchCatalogTool()
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
	if !catalogKeys[webFetchKey] && allowedSet[webFetchKey] {
		syn := syntheticWebFetchCatalogTool()
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
		return AgentEffectiveTools{}, kerrors.BadRequest("AGENT", "agent id is required")
	}
	if _, err := u.repo.GetAgentByID(ctx, agentID); err != nil {
		return AgentEffectiveTools{}, err
	}
	settings, err := u.runtimeSettingsForEffective(ctx, agentID)
	if err != nil {
		return AgentEffectiveTools{}, err
	}
	all, err := u.tools.SearchTools(ctx, ToolListQuery{Limit: 1000, Offset: 0})
	if err != nil {
		return AgentEffectiveTools{}, err
	}
	eff := buildAgentEffectiveTools(settings, all.Items)
	if overrides, oerr := u.tools.ListToolAgentOverridesByAgent(ctx, agentID); oerr == nil {
		ApplyAgentToolOverrides(&eff, all.Items, overrides)
	}
	return eff, nil
}

func (u *AgentUsecase) runtimeSettingsForEffective(ctx context.Context, agentID string) (AgentRuntimeSettings, error) {
	settings, err := u.repo.GetAgentRuntimeSettings(ctx, agentID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
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
		return AgentEffectiveTools{}, kerrors.BadRequest("AGENT", "agent id is required")
	}
	if _, err := u.repo.GetAgentByID(ctx, agentID); err != nil {
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
	if _, err := u.repo.UpsertAgentRuntimeSettings(ctx, settings); err != nil {
		return AgentEffectiveTools{}, err
	}
	all, err := u.tools.SearchTools(ctx, ToolListQuery{Limit: 1000, Offset: 0})
	if err != nil {
		return AgentEffectiveTools{}, err
	}
	settings, err = u.repo.GetAgentRuntimeSettings(ctx, agentID)
	if err != nil {
		return AgentEffectiveTools{}, err
	}
	settings = withSettingDefaults(settings)

	eff := buildAgentEffectiveTools(settings, all.Items)
	if overrides, oerr := u.tools.ListToolAgentOverridesByAgent(ctx, agentID); oerr == nil {
		ApplyAgentToolOverrides(&eff, all.Items, overrides)
	}
	return eff, nil
}
