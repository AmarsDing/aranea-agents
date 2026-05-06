package agent

import (
	"context"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
)

// BuildSystemPrompt joins agent description and prompt files.
func BuildSystemPrompt(agent biz.Agent, files []biz.AgentPromptFile) string {
	var b strings.Builder
	if d := strings.TrimSpace(agent.AgentDescription); d != "" {
		b.WriteString(d)
		b.WriteString("\n\n")
	}
	for _, f := range files {
		if body := strings.TrimSpace(f.Body); body != "" {
			b.WriteString(body)
			b.WriteString("\n\n")
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
	if st.SubagentsEnabled {
		fmt.Fprintf(&b, "- Subagents: enabled; max_concurrency=%d, max_depth=%d, max_children_per_agent=%d\n",
			st.SubagentsMaxConcurrency, st.SubagentsMaxGenerationDepth, st.SubagentsMaxChildrenPerAgent)
	} else {
		b.WriteString("- Subagents: disabled (this process runs a single agent turn; delegate via instructions only).\n")
	}
	if d.AgentUC != nil {
		eff, err := d.AgentUC.GetEffectiveTools(ctx, ag.ID)
		if err == nil {
			if !eff.ToolsEnabled {
				b.WriteString("- Tools: disabled for this agent.\n")
			} else {
				fmt.Fprintf(&b, "- Tools: enabled; profile=%q\n", eff.Profile)
				var keys []string
				for _, it := range eff.Items {
					if it.Enabled {
						keys = append(keys, it.ToolKey)
					}
				}
				if len(keys) > 0 {
					b.WriteString("- Effective tool keys this turn: " + strings.Join(keys, ", ") + "\n")
				} else {
					b.WriteString("- Effective tool keys: (none under current profile and allow list)\n")
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
