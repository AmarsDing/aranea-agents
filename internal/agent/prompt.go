package agent

import (
	"context"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
)

// Deps is a minimal bundle for prompt / runtime helpers (biz facades).
type Deps struct {
	Agents       biz.AgentRepository
	AgentUC      *biz.AgentUsecase
	ToolsCatalog biz.ToolRepo // optional; with Agents, used when AgentUC is nil to still resolve GetEffectiveTools
	// SQLiteSessionMemory reflects whether Runner memory persists session entities (SessionMemoryStore wired).
	SQLiteSessionMemory bool
}

// BuildSystemPrompt joins agent description and prompt files, filtered by system_prompt_mode.
func BuildSystemPrompt(agent biz.Agent, files []biz.AgentPromptFile, mode string) string {
	filtered := biz.FilesForMode(files, mode)
	var b strings.Builder
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
func PromptFilesForAgent(ctx context.Context, d Deps, ag biz.Agent) ([]biz.AgentPromptFile, error) {
	if len(ag.Files) > 0 {
		return ag.Files, nil
	}
	return d.Agents.ListAgentPromptFiles(ctx, ag.ID)
}

// RuntimeCapabilityCue appends ADK-style runtime directives: sub-agent switches and effective tool policy.
func RuntimeCapabilityCue(ctx context.Context, d Deps, ag biz.Agent) string {
	if ag.Settings == nil {
		return ""
	}
	st := ag.Settings
	var b strings.Builder
	b.WriteString("## Runtime capability policy (system)\n")
	b.WriteString("When tools are enabled below, this server may execute matching filesystem functions via the model's tool calls (paths are confined to the configured workspace root; override with env ARANEA_WORKSPACE_ROOT or WORKSPACE_ROOT).\n")
	b.WriteString("shell_exec runs with cwd inside that sandbox only: Windows %USERPROFILE%\\Desktop and other paths outside the workspace root will fail. Use relative paths under the workspace (e.g. mkdir notes). If stderr reports access/path errors, explain the sandbox limit once—do not loop repeating the same shell command.\n")
	b.WriteString("list_files: call at most once per directory path for the same user task; if you already have a listing, proceed with read_file on specific paths or answer—do not repeat list_files on the same path.\n")
	if st.SubagentsEnabled {
		fmt.Fprintf(&b, "- Subagents: enabled; max_concurrency=%d, max_depth=%d, max_children_per_agent=%d\n",
			st.SubagentsMaxConcurrency, st.SubagentsMaxGenerationDepth, st.SubagentsMaxChildrenPerAgent)
		b.WriteString("- To delegate to another agent, use tool `transfer_to_agent` or `spawn_subagent` with JSON argument `agent_name` (sub-agent's registered name). There is no separate spawn RPC from the chat client.\n")
	} else {
		b.WriteString("- Subagents: disabled (this process runs a single agent turn; delegate via instructions only).\n")
	}
	uc := d.AgentUC
	if uc == nil && d.Agents != nil && d.ToolsCatalog != nil {
		uc = biz.NewAgentUsecase(d.Agents, d.ToolsCatalog)
	}
	if uc != nil {
		eff, err := uc.GetEffectiveTools(ctx, ag.ID)
		if err == nil {
			if !eff.ToolsEnabled {
				b.WriteString("- Tools: disabled for this agent.\n")
			} else {
				fmt.Fprintf(&b, "- Tools: enabled; profile=%q\n", eff.Profile)
				var keys []string
				memCue := false
				mcpCue := false
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
						case "workspace_search":
							hasWorkspaceSearch = true
						}
					}
				}
				if len(keys) > 0 {
					b.WriteString("- Effective tool keys this turn: " + strings.Join(keys, ", ") + "\n")
				} else {
					b.WriteString("- Effective tool keys: (none under current profile and allow list)\n")
				}
				if hasWorkspaceSearch {
					b.WriteString("- workspace_search: use to locate symbols or string literals across the workspace before listing directories; preferred order: workspace_search → read_file (use start_line/end_line for large files) → edit/write. Avoid list_files at repo root without a narrowed path or keyword.\n")
				}
				b.WriteString("- Execution planning: state 3-7 verifiable steps before substantive edits; prefer tests or builds on affected packages when tools allow; if intent_artifact appears in session metadata, align steps with refined_goal and use search_hints for workspace_search queries.\n")
				if memCue {
					if d.SQLiteSessionMemory {
						b.WriteString("- load_memory/preload_memory: SQLite-backed session memory (memory_entities); durable across process restarts for turns that sync into the store.\n")
					} else {
						b.WriteString("- load_memory/preload_memory: in-process recall only (no SessionMemory store wired to this runner); not durable across restarts.\n")
					}
				}
				if mcpCue {
					b.WriteString("- MCP (mcp_tool_set): tools from enabled platform MCP servers (stdio/sse/streamable_http per row). Optional: include `mcp:<server_key>` in Tools allow/deny JSON to restrict which servers mount; stdio servers respect request context cancellation when the tool runner passes it through.\n")
				}
				if len(eff.Deny) > 0 {
					b.WriteString("- Deny list: " + strings.Join(eff.Deny, ", ") + "\n")
				}
			}
		}
	}
	if p := strings.TrimSpace(st.ToolsToolCallPrefix); p != "" {
		fmt.Fprintf(&b, "- Strip tool name prefix before resolution: %q\n", p)
	}
	return strings.TrimSpace(b.String())
}
