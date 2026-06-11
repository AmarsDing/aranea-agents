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
	Agents              biz.AgentRepository
	AgentUC             biz.TeamAgentLookup
	ToolRegistry        biz.ToolRegistryReader
	SQLiteSessionMemory bool
	Organization        *biz.OrganizationUsecase
	LG                  loggateway.Logger
}

// BuildSystemPrompt joins agent description and prompt files, filtered by system_prompt_mode.
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

	for _, f := range filtered {
		if body := strings.TrimSpace(f.Body); body != "" {
			b.WriteString(fmt.Sprintf("<internal_config name=%q>\n", f.Name))
			b.WriteString(body)
			b.WriteString("\n</internal_config>\n\n")
		}
	}
	return strings.TrimSpace(b.String())
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
	if uc != nil {
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
	if uc == nil {
		return ""
	}
	eff, err := uc.GetEffectiveTools(ctx, ag.ID)
	if err != nil || !eff.ToolsEnabled {
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
			}
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
	if hasWorkspaceSearch && level >= cueLevelFull && !skipToolCue {
		b.WriteString("- search_content: use to locate symbols or string literals across the workspace before listing directories; preferred order: search_content → read_file (use start_line/end_line for large files) → diff_edit (or patch_file when you have unified diff). Use save_file only for new files or small full rewrites; use replace_content for simple single replacements. Avoid list_file at repo root without a narrowed path or keyword.\n")
	}
	if level >= cueLevelFull {
		b.WriteString("- Execution planning: state 3-7 verifiable steps before substantive edits; prefer tests or builds on affected packages when tools allow; if intent_artifact appears in session metadata, align steps with refined_goal and use search_hints for search_content queries.\n")
	}
	if memCue && level >= cueLevelStandard && !skipToolCue {
		if d.SQLiteSessionMemory {
			b.WriteString("- load_memory/preload_memory: SQLite-backed session memory (memory_entities); durable across process restarts for turns that sync into the store.\n")
		} else {
			b.WriteString("- load_memory/preload_memory: in-process recall only (no SessionMemory store wired to this runner); not durable across restarts.\n")
		}
	}
	if mcpCue && level >= cueLevelStandard && !skipToolCue {
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

// RuntimeCapabilityCue appends ADK-style runtime directives: sub-agent switches and effective tool policy.
// Verbosity scales with system_prompt_mode (complete > task > minimized > none).
//
// Deprecated: use StaticRuntimeCapabilityCue + DynamicRuntimeCapabilityCue instead.
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
