package agent

import (
	"context"

	"aranea-agents/internal/agent/agentsmd"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// loadProjectAgentsMD reads the AGENTS.md chain from the agent's trusted
// workspace (tenant root). Untrusted host paths are skipped. Only coding /
// spirit / computer-use faces get the block (same gate as working_contract).
func loadProjectAgentsMD(ctx context.Context, ag biz.Agent, deps TRPCBuilderDeps) string {
	if !ShouldAttachWorkingContract(ag) {
		return ""
	}
	lg := deps.Logger()
	cwd, err := resolveToolWorkspaceRoot(ctx, ag, deps, "")
	if err != nil || cwd == "" {
		return ""
	}
	loaded := agentsmd.Load(cwd, []string{cwd}, agentsmd.DefaultMaxBytes)
	if loaded.Truncated {
		lg.Info("AGENTS.md chain truncated",
			loggateway.StepID("agent.agents_md"),
			loggateway.Str("agent_id", ag.ID),
			loggateway.Str("cwd", cwd),
			loggateway.Bool("agents_md_truncated", true),
			loggateway.Int("files", len(loaded.Files)),
		)
	}
	return agentsmd.FormatBlock(loaded)
}
