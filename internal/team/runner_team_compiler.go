package team

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/internal/metrics"

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
	compiledGraphCfg biz.GraphBuildConfig,
	err error,
) {
	if r.graphRoot == nil {
		if envTeamNativeForced() {
			root, memberLookup, err = BuildTRPCTeam(ctx, def, teamDeps, r.catalogAgent)
			if err != nil {
				return
			}
			metrics.TeamGraphRuntimeTotal.WithLabelValues("native", "native_emergency").Inc()
			return
		}
		err = DecideNativeFallback(def, teamRow.ID, false, "", "", mode, false).Error()
		return
	}

	if !SupportsTeamGraphRuntimeMode(mode) {
		err = DecideNativeFallback(def, teamRow.ID, false, "", "", mode, true).Error()
		return
	}

	graphExecID = uuid.NewString()
	cfg, cerr := CompileToGraphRuntimeConfigFromJSON(ctx, def, teamRow.DefinitionJSON, func(agentID string) string {
		ag, gerr := r.catalogAgent(ctx, agentID)
		if gerr != nil {
			return ""
		}
		return strings.TrimSpace(ag.AgentKey)
	}, r.graphLoader)
	if cerr != nil {
		event.CtxFlowLogWarn(ctx, "team.graph_runtime.compile", "Graph 编译失败", event.P("error", cerr.Error()))
		metrics.TeamGraphRuntimeTotal.WithLabelValues("graph", "compile_error").Inc()
		if envTeamNativeForced() {
			root, memberLookup, err = BuildTRPCTeam(ctx, def, teamDeps, r.catalogAgent)
			if err != nil {
				return
			}
			metrics.TeamGraphRuntimeTotal.WithLabelValues("native", "native_fallback").Inc()
			return
		}
		err = DecideNativeFallback(def, teamRow.ID, true, cerr.Error(), "", mode, true).Error()
		return
	}

	compiledGraphCfg = cfg
	groot, gerr := r.graphRoot.BuildTeamGraphRoot(ctx, cfg)
	if gerr != nil {
		event.CtxFlowLogWarn(ctx, "team.graph_runtime.build", "GraphAgent 构建失败", event.P("error", gerr.Error()))
		metrics.TeamGraphRuntimeTotal.WithLabelValues("graph", "build_error").Inc()
		if envTeamNativeForced() {
			root, memberLookup, err = BuildTRPCTeam(ctx, def, teamDeps, r.catalogAgent)
			if err != nil {
				return
			}
			metrics.TeamGraphRuntimeTotal.WithLabelValues("native", "native_fallback").Inc()
			return
		}
		err = DecideNativeFallback(def, teamRow.ID, true, "", gerr.Error(), mode, true).Error()
		return
	}

	root = groot
	_, memberLookup, err = BuildTeamMemberAgents(ctx, def, teamDeps, r.catalogAgent)
	if err != nil {
		return
	}
	if r.teamGraphCoord != nil {
		if regErr := r.teamGraphCoord.RegisterTeamGraphExecution(ctx, graphExecID, sess.ID, teamRow.ID, runID, compiledGraphCfg); regErr != nil {
			event.CtxFlowLogWarn(ctx, "team.graph_runtime.register", "graph execution 注册失败", event.P("error", regErr.Error()))
		}
	}
	if teamEmitter != nil {
		teamEmitter.LogDone("team.run.graph", "Team GraphAgent 已构建", event.P("graph_execution_id", graphExecID))
	}
	metrics.TeamGraphRuntimeTotal.WithLabelValues("graph", "success").Inc()
	return
}
