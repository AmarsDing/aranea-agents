package biz

import (
	"strings"

	"aranea-agents/pkg/loggateway"
)

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

// toolStaticGroups 是静态工具组表单源（组名 → 成员 tool_key）。动态组
// cli_admin 按目录实时展开（expandToolGroup 特判），不在此表。
// 成员增删必须与目录种子（internal/data builtinPlatformToolSeeds）同步——
// 一致性由 internal/data 的工具元数据 fitness 测试强制。
var toolStaticGroups = map[string][]string{
	"filesystem":  {"read_file", "read_multiple_files", "save_file", "list_file", "search_file", "search_content", "replace_content", "diff_edit", "patch_file", "read_lints", "delete_file"},
	"web":         {ToolKeyWebResearch, "web_fetch", "duckduckgo_search", "gemini_web_fetch", "google_search", "arxiv_search", "wikipedia_search"},
	"memory":      {"memory_search", "memory_get"},
	"skill":       {"skill_search", "use_skill"},
	"media":       {"read_document", "read_spreadsheet", "create_image", "tts"},
	"runtime":     {"shell_exec", "claude_code"},
	"messaging":   {"send_email"},
	"session":     {"await_user_reply", "todo_write"},
	"integration": {"call_agent", "knowledge_search", "mcp_tool_set", "mcp_broker"},
	"subagent":    {"subagents_spawn", "subagents_list", "subagents_get", "subagents_cancel"},
	"browser":     {"browser"},
	"computeruse": {"computer_use_observe", "computer_use_screenshot", "computer_use_act", "computer_use_launch", "computer_use_session"},
	// sandbox（M82 P1-2）：会话沙箱文件工具族。无 profile 默认引用——沙箱是
	// 显式 opt-in 能力，agent allow JSON 命名 group:sandbox 或单键授予。
	"sandbox": {"sandbox_fs_write", "sandbox_fs_read"},
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
	name = strings.TrimSpace(name)
	if name == "cli_admin" {
		return cliAdminKeysFromRegistry(catalog)
	}
	if members, ok := toolStaticGroups[name]; ok {
		return append([]string{}, members...)
	}
	return nil
}

// ToolPolicyStaticTables 是工具策略静态元数据快照：静态组表（不含动态组
// cli_admin）、profile 条目表、registry opt-in 表。供跨包一致性 fitness 测试
// （internal/data）与诊断使用。
type ToolPolicyStaticTables struct {
	Groups    map[string][]string
	Profiles  map[string][]string
	OptInOnly map[string]bool
}

// ExportToolPolicyStaticTables 返回策略静态元数据的拷贝（调用方修改不影响策略计算）。
func ExportToolPolicyStaticTables() ToolPolicyStaticTables {
	groups := make(map[string][]string, len(toolStaticGroups))
	for name, members := range toolStaticGroups {
		groups[name] = append([]string{}, members...)
	}
	profiles := make(map[string][]string, len(toolProfiles))
	for name, entries := range toolProfiles {
		profiles[name] = append([]string{}, entries...)
	}
	optIn := make(map[string]bool, len(registryOptInOnlyKeys))
	for key := range registryOptInOnlyKeys {
		optIn[key] = true
	}
	return ToolPolicyStaticTables{Groups: groups, Profiles: profiles, OptInOnly: optIn}
}

func profileAllowSet(profile string, catalog []Tool) map[string]bool {
	result := map[string]bool{}
	key := strings.TrimSpace(profile)
	if _, ok := toolProfiles[key]; !ok {
		// 无独立条目的别名/空 profile 回退 canonical 归一（general→coding、
		// ""→coding），与 CanonicalToolProfile 的展示语义对齐——否则别名
		// profile 会静默得到空工具面而 API 展示 canonical 名。有独立条目
		// （system_admin/system_memory 等）时优先原始条目，小工具面不被放大。
		key = CanonicalToolProfile(key)
	}
	for _, k := range toolProfiles[key] {
		if strings.HasPrefix(k, "group:") {
			gn := strings.TrimPrefix(k, "group:")
			for _, member := range expandToolGroup(gn, catalog) {
				result[member] = true
			}
			continue
		}
		result[k] = true
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
	// duckduckgo_search is seeded enabled=false (Instant Answer / fallback
	// web search). Spirit names it so weather/facts work without Tavily.
	"duckduckgo_search": true,
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
	// twin_*/gns3_*（方案10 TwinOps 工具集，internal/tools/twinops）种子为
	// enabled=false 的 opt-in-only 工具：白名单岗位经 allow JSON 显式启用；
	// 不入本表将被 applyRegistryAdminDenials 对全员硬 deny，授权永远不生效。
	"twin_alarm_query":        true,
	"twin_alarm_get":          true,
	"twin_alarm_ack":          true,
	"twin_alarm_rule_get":     true,
	"twin_line_status":        true,
	"twin_line_events":        true,
	"twin_line_probe":         true,
	"twin_device_get":         true,
	"twin_device_search":      true,
	"twin_device_metrics":     true,
	"twin_collector_status":   true,
	"twin_remediation_status": true,
	"twin_inspection_query":   true,
	"gns3_health_check":       true,
	"gns3_exec":               true,
	"gns3_fault_inject":       true,
	"gns3_fault_clear":        true,
	// twin_config_*（Phase B 配置自动化三工具，同 twinops 族）漏登记事故
	// （2026-08-24 P1-2② fitness 测试捕获）：种子 enabled=false 且本表无条目时
	// applyRegistryAdminDenials 对全员硬 deny，ops 岗位 allow JSON 授权静默无效。
	"twin_config_diff":     true,
	"twin_config_push":     true,
	"twin_config_rollback": true,
	// officecli_* 有完整工具实现（internal/tools/officecli），种子 enabled=false
	//（依赖 OfficeCLI 二进制环境配置）。入表使其成为 opt-in-only：agent allow
	// JSON 显式命名即可启用，不入表则被 applyRegistryAdminDenials 全员硬 deny、
	// 永远不可装配（2026-08-24 审计 P5）。read_image 无 factory 实现，已从
	// toolGroupsMedia/种子/存量库清除，不在此列。
	"officecli_read":   true,
	"officecli_write":  true,
	"officecli_render": true,
	// sandbox_fs_*（M82 P1-2 会话沙箱文件工具族）种子 enabled=false：沙箱是
	// 显式 opt-in 能力，allow JSON 命名 group:sandbox / 单键授予；不入本表将被
	// applyRegistryAdminDenials 全员硬 deny。沙箱子系统不可用时装配层另行裁剪。
	"sandbox_fs_write": true,
	"sandbox_fs_read":  true,
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

func applySpiritReservedDenials(profile string, allowSet, denySet map[string]bool) {
	if denySet == nil {
		return
	}
	if CanonicalToolProfile(profile) == "spirit" {
		return
	}
	for _, k := range SpiritReservedToolKeys() {
		denySet[k] = true
	}
}

var toolProfiles = map[string][]string{
	"chat_only": {},
	"read_only": {"datetime", "read_file", "read_multiple_files", "list_file", "search_file", "search_content", "todo_write"},
	// 2026-08-21 全链路审查 B1：框架 WithKnowledge 自动面已砍，knowledge_search
	// 唯一来源是目录工具（effective tools 门禁）。coding/research 显式命名，
	// 保证默认 profile 下知识检索工具仍可达（种子 enabled=true + 此处命名 =
	// allowed）；full 经 group:integration 覆盖。其余 profile 用 allow JSON 按需
	// opt-in。
	// 2026-08-21 调用契约 7.4：mcp_broker 进 coding 默认面——「启用 MCP」的默认
	// 形态从直连全量 schema 改为 broker 元工具（schema 按需拉取）。mcp_broker 是
	// registryOptInOnlyKeys 成员（种子 enabled=false），此处命名即 allowed，无需
	// seed 翻转与存量库迁移。无 MCP 服务器时 cfg.MCPBroker=nil 不挂任何工具，
	// 零上下文成本；mcp_tool_set 保持显式 opt-in（小工具面场景）。
	"coding":   {"group:filesystem", "group:web", "group:skill", "group:session", "datetime", "shell_exec", "knowledge_search", "mcp_broker"},
	"research": {ToolKeyWebResearch, "web_fetch", "arxiv_search", "wikipedia_search", "read_file", "read_multiple_files", "list_file", "search_file", "search_content", "skill_search", "memory_search", "knowledge_search", "todo_write", "datetime"},
	"full":     {"group:filesystem", "group:web", "group:skill", "group:memory", "group:media", "group:runtime", "group:messaging", "group:session", "group:integration", "group:subagent", "group:browser", "group:cli_admin", "datetime"},

	"minimal":      {},
	"safe":         {"datetime", "read_file", "read_multiple_files", "list_file", "search_file", "search_content", "todo_write"},
	"system_admin": {"group:cli_admin", "web_fetch", "datetime"},
	// system_memory / system_skills：__memory__ / __skills__ 专属 profile。
	// 核心 memory_butler_* / skills_butler_* 工具经 CustomTools 按 agent_key 条件注入
	// （service/cli_admin_tools.go memoryButlerTools/skillsButlerTools），绕行目录门禁，
	// 不在 tools 表、无需此处命名。此 profile 仅提供目录侧最小面（datetime），
	// 避免未知 profile 导致 effective-tools 全 denied 的误导性展示。
	"system_memory": {"datetime"},
	"system_skills": {"datetime"},
	// Spirit 编排者：只编排与沟通，不直接执行 shell / 桌面自动化（与 CAPABILITIES.md 契约一致）。
	"spirit": {"plan_and_execute", "cancel_orchestration", "synthesize_results", "get_team_deliverable", "build_orchestration_graph", "memory_search", "group:subagent", "datetime", "web_research", "duckduckgo_search", "web_fetch"},
}

// CanonicalToolProfile 归一化 profile 别名/空值到策略桶（general→coding、
// safe→read_only、system_admin→full 等）。策略计算与装配层（核心/延迟分离）
// 必须使用同一归一化语义，避免出现「展示 canonical、门禁按原始串」的双轨。
func CanonicalToolProfile(profile string) string {
	switch strings.ToLower(strings.TrimSpace(profile)) {
	case "":
		// Empty profile falls back to "coding" (the default in DefaultAgentRuntimeSettings).
		// Treating empty as "full" would bypass profile restrictions entirely.
		return "coding"
	case "chat_only", "minimal":
		return "chat_only"
	case "read_only", "safe":
		return "read_only"
	case "coding", "general":
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
	applySpiritReservedDenials(prof, allowedSet, denySet)

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
		Profile:      CanonicalToolProfile(settings.ToolsProfile),
		Allow:        allow,
		Deny:         deny,
		Items:        items,
	}
}
