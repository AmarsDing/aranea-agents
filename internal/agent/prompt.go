package agent

import (
	"context"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// Deps is a minimal bundle for prompt / runtime helpers (biz facades).
type Deps struct {
	Agents                 biz.AgentRepository
	AgentUC                biz.TeamAgentLookup
	ToolRegistry           biz.ToolRegistryReader
	SessionMemoryAvailable bool
	Organization           *biz.OrganizationUsecase
	LG                     loggateway.Logger
	// CustomToolKeys carries the names of dynamically injected CustomTools
	// (e.g. plan_and_execute, cancel_orchestration) that are NOT in the Registry.
	// The Runtime Cue uses this to produce accurate tool availability hints.
	CustomToolKeys []string
	// CachedEffectiveTools 携带 BUILD 期预取的 effective-tools 结果
	// （2026-08-21 全链路审查 B3）。DynamicRuntimeCapabilityCue 在工具循环
	// 每轮 BeforeModel 都会执行；无缓存时 GetEffectiveTools 每轮 ≈4 次 DB
	// 查询（agent + settings + SearchTools(all) + overrides）。非空时优先
	// 复用，跳过 DB。
	CachedEffectiveTools *biz.AgentEffectiveTools
}

// BuildSystemPrompt joins agent description and prompt files, filtered by system_prompt_mode.
//
// Order: <role_responsibility> → <industry_context> → description →
// <working_contract> (coding / computer-use face) →
// <team_execution_contract> (spirit orchestration face, LBG-1) →
// <permission_state> → <internal_config> files → optional memory self-marking.
//
// PGO-1-AGENT-01: a new optional categoryResponsibility parameter has been added.
// When non-empty and PGO_CATEGORY_RESPONSIBILITY_INJECT is enabled, it is prepended
// as a <role_responsibility source="category"> block BEFORE agent_description.
// Pass "" to preserve the original behaviour (backward-compatible).
func BuildSystemPrompt(agent biz.Agent, files []biz.AgentPromptFile, mode string, categoryResponsibility ...string) string {
	filtered := biz.FilesForMode(files, mode)
	var b strings.Builder

	if len(categoryResponsibility) > 0 {
		if cr := strings.TrimSpace(categoryResponsibility[0]); cr != "" {
			b.WriteString("<role_responsibility source=\"category\">\n")
			b.WriteString(cr)
			b.WriteString("\n</role_responsibility>\n\n")
		}
	}

	if pk := strings.TrimSpace(agent.PositionKey); pk != "" {
		if vd := strings.TrimSpace(agent.VariantDescription); vd != "" {
			b.WriteString("<industry_context>\n")
			fmt.Fprintf(&b, "## 当前定位\n你是本岗位的 %s 方向专家。\n%s\n", agent.AgentVariant, vd)
			b.WriteString("</industry_context>\n\n")
		} else if av := strings.TrimSpace(agent.AgentVariant); av != "" && av != "general" {
			b.WriteString("<industry_context>\n")
			fmt.Fprintf(&b, "## 当前定位\n你是本岗位的 %s 方向专家。\n", av)
			b.WriteString("</industry_context>\n\n")
		}
	}
	if d := strings.TrimSpace(agent.AgentDescription); d != "" {
		b.WriteString(d)
		b.WriteString("\n\n")
	}

	// A1/A2: coding-bridge working contract + session permission sit after
	// role/industry (stable prefix) and before <internal_config> files so
	// DeepSeek prefix cache stays aligned across turns.
	if block := WorkingContractBlock(agent); block != "" {
		b.WriteString(block)
		b.WriteString("\n\n")
	}
	// LBG-1: team/spirit 编排面执行契约，与 working_contract 适用域互斥，
	// 同处稳定前缀区（纯静态文本，保持 prefix cache 对齐）。
	if block := TeamExecutionContractBlock(agent); block != "" {
		b.WriteString(block)
		b.WriteString("\n\n")
	}
	if block := PermissionStateBlock(agent); block != "" {
		b.WriteString(block)
		b.WriteString("\n\n")
	}

	for _, f := range filtered {
		if body := strings.TrimSpace(f.Body); body != "" {
			b.WriteString(fmt.Sprintf("<internal_config name=%q>\n", f.Name))
			b.WriteString(body)
			b.WriteString("\n</internal_config>\n\n")
		}
	}

	// Memory Self-Marking Instructions: inject only when memory is enabled
	// AND the L3 fact layer is enabled. The <fact> tags are persisted to
	// memory_fact (L3) — with L3 off the instructions are pure system-prompt
	// overhead (2026-08-20 token 成本审查 方案E).
	if agent.Settings != nil && agent.Settings.MemoryEnabled && agent.Settings.L3Enabled {
		b.WriteString(memorySelfMarkingInstructions())
		b.WriteString("\n\n")
	}

	return strings.TrimSpace(b.String())
}

// memorySelfMarkingInstructions returns the standard Memory Self-Marking Instructions
// that teach the agent to mark user-stated facts using <fact> XML tags.
// These tags are parsed by the backend and persisted immediately to memory_fact,
// bridging the async gap between conversation and Sleep-time consolidation.
func memorySelfMarkingInstructions() string {
	return `<memory_self_marking>
## Memory Self-Marking Instructions

When the user explicitly tells you information that should be remembered long-term, you MUST mark it using XML tags in your response. This ensures the information is immediately saved to persistent memory.

### When to Mark
Mark information when the user:
1. Tells you their name or preferences ("My name is...", "I like...", "Call me...")
2. Instructs you to change your behavior or identity ("Your name is...", "You should...", "Remember that...")
3. Shares important personal facts ("I work at...", "My project is...", "I have a meeting tomorrow")

### How to Mark
Use the following XML format in your response:

<fact type="CATEGORY" confidence="LEVEL">
The factual statement in third person, one sentence.
</fact>

Categories:
- "identity": User's name, preferences, personal attributes
- "instruction": Instructions about how you should behave or identify
- "domain_knowledge": Facts about the user's work, projects, or domain

Confidence:
- "high": User explicitly stated this
- "medium": Strongly implied by context

### Rules
1. Place the <fact> tag at the END of your response, after your normal reply
2. Do NOT mention the tag to the user unless they ask
3. If multiple facts, use multiple <fact> tags
4. Do NOT mark general conversation content
5. If no facts to remember, do NOT use the tag

### Example
User: "My name is Alice and I prefer morning meetings"
Your response: "Nice to meet you, Alice! I'll remember you prefer morning meetings.
<fact type="identity" confidence="high">The user's name is Alice</fact>
<fact type="preference" confidence="high">The user prefers morning meetings</fact>"
</memory_self_marking>`
}

// PromptFilesForAgent returns hydrated in-memory files when present, otherwise loads from persistence.
func BuildIndustryContext(ctx context.Context, d Deps, ag biz.Agent) string {
	if strings.TrimSpace(ag.PositionKey) == "" {
		return ""
	}
	if d.Organization == nil {
		return ""
	}
	lg := d.LG
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	posNode, err := d.Organization.GetByKey(ctx, ag.PositionKey)
	if err != nil {
		lg.Error("行业上下文构建失败：无法获取岗位节点", loggateway.StepID("agent.industry_context"), loggateway.Str("position_key", ag.PositionKey), loggateway.Err(err))
		return ""
	}
	anc, err := d.Organization.GetAncestors(ctx, posNode.ID)
	if err != nil {
		lg.Error("行业上下文构建失败：无法获取岗位祖先链", loggateway.StepID("agent.industry_context"), loggateway.Str("position_key", ag.PositionKey), loggateway.Err(err))
		return ""
	}
	var b strings.Builder
	if anc.Company.Key != "" {
		fmt.Fprintf(&b, "## 行业\n%s： %s\n", anc.Company.Name, anc.Company.Description)
	}
	fmt.Fprintf(&b, "## 部门\n%s： %s\n", anc.Department.Name, anc.Department.Description)
	fmt.Fprintf(&b, "## 岗位\n%s： %s\n", anc.Position.Name, anc.Position.Description)
	return b.String()
}

func PromptFilesForAgent(ctx context.Context, d Deps, ag biz.Agent) ([]biz.AgentPromptFile, error) {
	if len(ag.Files) > 0 {
		return ag.Files, nil
	}
	return d.Agents.ListAgentPromptFiles(ctx, ag.ID)
}

// StaticRuntimeCapabilityCue generates the static portion of runtime capability
// directives: sub-agent switches, tool enablement, workspace root, and profile.
// This content does not change within a session lifecycle.
// Verbosity scales with system_prompt_mode (complete > task > minimized > none).
func StaticRuntimeCapabilityCue(ctx context.Context, d Deps, ag biz.Agent) string {
	if ag.Settings == nil {
		return ""
	}
	level := capabilityCueLevelForMode(ag.SystemPromptMode)
	if level == cueLevelMinimal && !ag.Settings.ToolsEnabled && !ag.Settings.SubagentsEnabled {
		return ""
	}
	st := ag.Settings
	var b strings.Builder
	b.WriteString("## Runtime capability policy (system)\n")
	if level >= cueLevelFull {
		b.WriteString("When tools are enabled below, filesystem tools operate under a shared workspace root (override with env ARANEA_WORKSPACE_ROOT or WORKSPACE_ROOT).\n")
		b.WriteString("exec_command (shell_exec) uses that workspace as default cwd; optional JSON field workdir sets a subdirectory or absolute path allowed by the OS user. Prefer relative paths under the workspace (e.g. mkdir notes). If stderr reports access errors, explain once—do not loop repeating the same shell command.\n")
		b.WriteString("list_files: call at most once per directory path for the same user task; if you already have a listing, proceed with read_file on specific paths or answer—do not repeat list_files on the same path.\n")
	} else if level >= cueLevelStandard && st.ToolsEnabled {
		b.WriteString("Filesystem tools use the shared workspace root (ARANEA_WORKSPACE_ROOT / WORKSPACE_ROOT). Prefer search_content before broad list_file; avoid repeating the same directory listing.\n")
	}
	if st.SubagentsEnabled {
		fmt.Fprintf(&b, "- Subagents: enabled; max_concurrency=%d, max_depth=%d, max_children_per_agent=%d\n",
			st.SubagentsMaxConcurrency, st.SubagentsMaxGenerationDepth, st.SubagentsMaxChildrenPerAgent)
		if level >= cueLevelStandard {
			b.WriteString("- To delegate to another agent, use tool `transfer_to_agent` or `subagents_spawn` with JSON argument `agent_name` (sub-agent's registered name). There is no separate spawn RPC from the chat client.\n")
		}
	} else if level >= cueLevelCompact {
		b.WriteString("- Subagents: disabled (this process runs a single agent turn; delegate via instructions only).\n")
	}
	uc := d.AgentUC
	if uc == nil && d.Agents != nil && d.ToolRegistry != nil {
		uc = biz.NewAgentUsecase(biz.AgentUsecaseDeps{Reader: d.Agents, Writer: d.Agents, Settings: d.Agents, Files: d.Agents, Position: d.Agents, Tx: d.Agents, Tools: d.ToolRegistry})
	}
	// B3：BUILD 期缓存优先——static hook 每轮 BeforeModel 都先算 cue 再查重，
	// 无缓存时 GetEffectiveTools 每轮 ≈4 次 DB 查询纯属浪费。
	if d.CachedEffectiveTools != nil {
		eff := *d.CachedEffectiveTools
		if !eff.ToolsEnabled {
			if level >= cueLevelCompact {
				b.WriteString("- Tools: disabled for this agent.\n")
			}
		} else {
			fmt.Fprintf(&b, "- Tools: enabled; profile=%q\n", eff.Profile)
		}
	} else if uc != nil {
		eff, err := uc.GetEffectiveTools(ctx, ag.ID)
		if err == nil {
			if !eff.ToolsEnabled {
				if level >= cueLevelCompact {
					b.WriteString("- Tools: disabled for this agent.\n")
				}
			} else {
				fmt.Fprintf(&b, "- Tools: enabled; profile=%q\n", eff.Profile)
			}
		}
	}
	if p := strings.TrimSpace(st.ToolsToolCallPrefix); p != "" && level >= cueLevelStandard {
		fmt.Fprintf(&b, "- Strip tool name prefix before resolution: %q\n", p)
	}
	files := ag.Files
	if len(files) == 0 && d.Agents != nil {
		if loaded, err := PromptFilesForAgent(ctx, d, ag); err == nil {
			files = loaded
		}
	}
	skipToolCue := HasFilteredPromptFile(files, ag.SystemPromptMode, "CAPABILITIES.md")
	if uc == nil && skipToolCue && level >= cueLevelCompact {
		b.WriteString("- Tools: see CAPABILITIES.md in instruction; effective keys resolved at runtime.\n")
	}
	return strings.TrimSpace(b.String())
}

// DynamicRuntimeCapabilityCue generates the dynamic portion of runtime capability
// directives: effective tool keys, deny list, memory/MCP cues, and detailed tool guidance.
// This content may change on task/turn switches.
// Verbosity scales with system_prompt_mode (complete > task > minimized > none).
func DynamicRuntimeCapabilityCue(ctx context.Context, d Deps, ag biz.Agent) string {
	if ag.Settings == nil {
		return ""
	}
	level := capabilityCueLevelForMode(ag.SystemPromptMode)
	if level == cueLevelMinimal && !ag.Settings.ToolsEnabled && !ag.Settings.SubagentsEnabled {
		return ""
	}
	uc := d.AgentUC
	if uc == nil && d.Agents != nil && d.ToolRegistry != nil {
		uc = biz.NewAgentUsecase(biz.AgentUsecaseDeps{Reader: d.Agents, Writer: d.Agents, Settings: d.Agents, Files: d.Agents, Position: d.Agents, Tx: d.Agents, Tools: d.ToolRegistry})
	}
	var eff biz.AgentEffectiveTools
	if d.CachedEffectiveTools != nil {
		// B3：BUILD 期已预取（chat/team/graph 三条路径均填充），工具循环
		// 每轮复用，避免每轮 BeforeModel ≈4 次 DB 查询。
		eff = *d.CachedEffectiveTools
	} else {
		if uc == nil {
			return ""
		}
		var err error
		eff, err = uc.GetEffectiveTools(ctx, ag.ID)
		if err != nil || !eff.ToolsEnabled {
			return ""
		}
	}
	if !eff.ToolsEnabled {
		return ""
	}
	files := ag.Files
	if len(files) == 0 && d.Agents != nil {
		if loaded, err := PromptFilesForAgent(ctx, d, ag); err == nil {
			files = loaded
		}
	}
	skipToolCue := HasFilteredPromptFile(files, ag.SystemPromptMode, "CAPABILITIES.md")
	var b strings.Builder
	var keys []string
	memCue := false
	mcpCue := false
	mcpBrokerCue := false
	hasWorkspaceSearch := false
	hasFragmentEdit := false
	hasFileWrite := false
	hasReadLints := false
	hasSubagents := false
	hasBrowser := false
	hasShell := false
	for _, it := range eff.Items {
		if it.Enabled {
			keys = append(keys, it.ToolKey)
			tk := strings.ToLower(strings.TrimSpace(it.ToolKey))
			switch tk {
			case "load_memory", "preload_memory":
				memCue = true
			case biz.ToolKeyMCPToolSet:
				mcpCue = true
			case biz.ToolKeyMCPBroker:
				mcpBrokerCue = true
			case "search_content":
				hasWorkspaceSearch = true
			case "diff_edit", "patch_file":
				hasFragmentEdit = true
			case "replace_content", "save_file", "delete_file":
				hasFileWrite = true
			case "read_lints":
				hasReadLints = true
			case "subagents_spawn":
				hasSubagents = true
			case "browser":
				hasBrowser = true
			case "shell_exec":
				hasShell = true
			}
		}
	}
	// Merge CustomTool keys (e.g. plan_and_execute) into the effective key list
	// so the LLM sees the complete set of available tools.
	// Use existingKeys to deduplicate against eff.Items.
	existingKeys := make(map[string]bool, len(keys))
	for _, k := range keys {
		existingKeys[k] = true
	}
	for _, ck := range d.CustomToolKeys {
		ck = strings.TrimSpace(ck)
		if ck != "" && !existingKeys[ck] {
			existingKeys[ck] = true
			keys = append(keys, ck)
		}
	}
	if len(keys) > 0 && !skipToolCue {
		b.WriteString("- Effective tool keys this turn: " + strings.Join(keys, ", ") + "\n")
		if level >= cueLevelStandard {
			b.WriteString("- Call tools by their runtime function names: save_file (not write_file), exec_command (not shell_exec), list_file (not list_files).\n")
		}
	} else if len(keys) == 0 && level >= cueLevelCompact {
		b.WriteString("- Effective tool keys: (none under current profile and allow list)\n")
	} else if skipToolCue && len(keys) > 0 && level >= cueLevelCompact {
		b.WriteString("- Tools: see CAPABILITIES.md in instruction; effective keys resolved at runtime.\n")
	}
	// Spirit orchestration fallback: when plan_and_execute is referenced in
	// prompt files but NOT available as a CustomTool, inject explicit fallback
	// guidance so the LLM does not waste turns trying to call a missing tool.
	if ag.AgentKey == biz.SpiritAgentKey && !existingKeys["plan_and_execute"] && level >= cueLevelStandard {
		b.WriteString("- IMPORTANT: plan_and_execute is NOT available this session. For multi-step tasks, use subagents_spawn to delegate to sub-agents instead. Do NOT attempt to call plan_and_execute.\n")
	}
	if hasWorkspaceSearch && level >= cueLevelFull && !skipToolCue {
		editStep := "replace_content for targeted edits (or save_file for new files)"
		if hasFragmentEdit {
			editStep = "diff_edit (or patch_file when you have unified diff); use save_file only for new files; use replace_content for simple single replacements"
		} else if hasFileWrite {
			editStep = "replace_content for targeted edits; use save_file only for new files"
		}
		b.WriteString("- search_content: use to locate symbols or string literals across the workspace before listing directories; optional after/before/context, type, head_limit/offset. Preferred order: search_content → read_file (use start_line/end_line for large files) → " + editStep + ". Avoid list_file at repo root without a narrowed path or keyword.\n")
	}
	if hasReadLints && level >= cueLevelFull && !skipToolCue {
		b.WriteString("- After save_file / diff_edit / replace_content / patch_file / delete_file, call read_lints (omit path to lint recently edited files) before guessing compile status or running tests.\n")
	}
	if hasShell && level >= cueLevelFull && !skipToolCue {
		b.WriteString("- exec_command: results include exit_code and duration_ms; long jobs also return session_id, output_file, running_for_ms; hung=true means output stalled. write_stdin can wait with notify_pattern / block_until_ms.\n")
	}
	if hasBrowser && level >= cueLevelStandard && !skipToolCue {
		b.WriteString("- Browser: after navigate/click/type, call browser_snapshot before the next interaction (mutating tools stamp next_tool). Do not parallelize browser tools.\n")
	}
	if hasSubagents && level >= cueLevelStandard && !skipToolCue {
		b.WriteString("- subagents_spawn: set kind=explore for codebase search (avoid writes) or kind=verify for tests/builds. subagents_get accepts block_until_ms to wait for completion.\n")
	}
	if level >= cueLevelStandard && !skipToolCue {
		b.WriteString("- todo_declare_blocker is last resort: only after you already told the user a concrete next step (routing advice or labeled gap). Do not use it as the first response to FORBIDDEN/not-found or empty search.\n")
	}
	if level >= cueLevelFull {
		b.WriteString("- Execution planning: state 3-7 verifiable steps before substantive edits; prefer tests or builds on affected packages when tools allow; if intent_artifact appears in session metadata, align steps with refined_goal and use search_hints for search_content queries.\n")
	}
	if memCue && level >= cueLevelStandard && !skipToolCue {
		if d.SessionMemoryAvailable {
			b.WriteString("- load_memory/preload_memory: persistent session memory (memory_entities); durable across process restarts for turns that sync into the store.\n")
		} else {
			b.WriteString("- load_memory/preload_memory: in-process recall only (no SessionMemory store wired to this runner); not durable across restarts.\n")
		}
	}
	// B2（2026-08-21）：broker 与直连同开时装配层只挂 broker（shard_plan
	// broker 优先），直连 cue 同步抑制，避免引导模型去找未挂载的直连工具。
	if mcpCue && !mcpBrokerCue && level >= cueLevelStandard && !skipToolCue {
		b.WriteString("- MCP (mcp_tool_set): tools from enabled platform MCP servers (stdio/sse/streamable_http per row). Optional: include `mcp:<server_key>` in Tools allow/deny JSON to restrict which servers mount; stdio servers respect request context cancellation when the tool runner passes it through.\n")
	}
	if mcpBrokerCue && level >= cueLevelStandard && !skipToolCue {
		b.WriteString("- MCP Broker (mcp_broker): runtime MCP discovery tools (mcp_list_servers, mcp_list_tools, mcp_inspect_tools, mcp_call). Use these to dynamically discover and invoke MCP servers at runtime instead of having tools pre-mounted.\n")
	}
	if len(eff.Deny) > 0 && level >= cueLevelStandard {
		b.WriteString("- Deny list: " + strings.Join(eff.Deny, ", ") + "\n")
	}
	return strings.TrimSpace(b.String())
}

// RuntimeCapabilityCue returns the combined static + dynamic runtime capability
// cue. Verbosity scales with system_prompt_mode (complete > task > minimized > none).
//
// Use this wrapper in test/preview contexts that need the combined output.
// Production bakes the static portion into WithInstruction (trpc_build.go).
// The dynamic portion is injected per LLM call as a trailing user-role cue
// (runtime_cue_inject.go). Use this wrapper in test/preview contexts that
// need the combined output.
func RuntimeCapabilityCue(ctx context.Context, d Deps, ag biz.Agent) string {
	static := StaticRuntimeCapabilityCue(ctx, d, ag)
	dynamic := DynamicRuntimeCapabilityCue(ctx, d, ag)
	if static == "" {
		return dynamic
	}
	if dynamic == "" {
		return static
	}
	return static + "\n" + dynamic
}
