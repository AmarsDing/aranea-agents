package team

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/internal/metrics"
	"aranea-agents/pkg/loggateway"

	"github.com/google/uuid"
	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
)

func (r *Runner) compileTeamRuntime(
	ctx context.Context,
	sess biz.Session,
	teamRow biz.Team,
	def Definition,
	mode string,
	teamDeps TRPCTeamBuilderDeps,
	teamEmitter *event.TraceEmitter,
	runID string,
) (
	root trpcagent.Agent,
	memberLookup map[string]trpcagent.Agent,
	graphExecID string,
	compiledTeam *biz.CompiledTeam,
	err error,
) {
	if r.cfg.GraphRoot == nil {
		err = graphRuntimeDiagnosticError("", "", mode, false)
		return
	}

	if !SupportsTeamGraphRuntimeMode(mode) {
		err = graphRuntimeDiagnosticError("", "", mode, true)
		return
	}

	graphExecID = uuid.NewString()
	ct, cerr := CompileToGraphRuntimeConfigFromJSON(ctx, def, teamRow.DefinitionJSON, func(agentID string) string {
		ag, gerr := r.lookupAgent(ctx, agentID)
		if gerr != nil {
			return ""
		}
		return strings.TrimSpace(ag.AgentKey)
	}, r.cfg.GraphLoader, r.lg)
	if cerr != nil {
		r.lg.Warn("Graph 编译失败", loggateway.StepID("team.graph_runtime.compile"), loggateway.Err(cerr))
		metrics.TeamGraphRuntimeTotal.WithLabelValues("graph", "compile_error").Inc()
		err = graphRuntimeDiagnosticError(cerr.Error(), "", mode, true)
		return
	}

	compiledTeam = ct
	if ct.GraphBuildConfig.CircuitBreaker != nil && strings.TrimSpace(teamRow.ID) != "" {
		ct.GraphBuildConfig = biz.WithCircuitBreakerScope(ct.GraphBuildConfig, "team:"+teamRow.ID)
	}
	ct.GraphBuildConfig = applyCrossRequestEntryOverride(ct.GraphBuildConfig, def, readSwarmActiveAgent(sess))
	groot, gerr := r.cfg.GraphRoot.BuildTeamGraphRoot(ctx, ct.GraphBuildConfig)
	if gerr != nil {
		r.lg.Warn("GraphAgent 构建失败", loggateway.StepID("team.graph_runtime.build"), loggateway.Err(gerr))
		metrics.TeamGraphRuntimeTotal.WithLabelValues("graph", "build_error").Inc()
		err = graphRuntimeDiagnosticError("", gerr.Error(), mode, true)
		return
	}

	root = groot
	_, memberLookup, err = BuildTeamMemberAgents(ctx, def, teamDeps, r.lookupAgent, r.lg)
	if err != nil {
		return
	}
	if r.mediator != nil {
		spiritSessionID := deriveSpiritSessionID(sess)
		// C1 全量物化（B9）：graph_id 优先用 team 的 linked_graph_id（真实图资产），
		// 列值为空时回退 definition_json 中的 linked_graph_id，仍为空由下游保留 team: 合成 ID 兜底。
		linkedGraphID := ResolveLinkedGraphID(teamRow.LinkedGraphID, teamRow.DefinitionJSON)
		if regErr := r.mediator.RegisterTeamGraphExecution(ctx, graphExecID, sess.ID, spiritSessionID, teamRow.ID, runID, linkedGraphID, compiledTeam); regErr != nil {
			r.lg.Warn("graph execution 注册失败", loggateway.StepID("team.graph_runtime.register"), loggateway.Err(regErr))
		}
	}
	if teamEmitter != nil {
		teamEmitter.LogDone("team.run.graph", "Team GraphAgent 已构建", event.P("graph_execution_id", graphExecID))
	}
	metrics.TeamGraphRuntimeTotal.WithLabelValues("graph", "success").Inc()
	return
}
