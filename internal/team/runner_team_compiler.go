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

// compileTeamRuntime attempts to compile the team definition into a GraphAgent
// runtime. If graph runtime is not available or not enabled, it falls back to
// native team runtime based on the fallback policy.
//
// Returns the root agent, member lookup map, graph execution ID, and any error.
func (r *Runner) compileTeamRuntime(
	ctx context.Context,
	sess biz.Session,
	teamRow biz.Team,
	def Definition,
	mode string,
	teamDeps TRPCTeamBuilderDeps,
	teamEmitter *event.TraceEmitter,
) (
	root trpcagent.Agent,
	memberLookup map[string]trpcagent.Agent,
	graphExecID string,
	compiledGraphCfg biz.GraphBuildConfig,
	err error,
) {
	graphAttempted := false
	graphCompileErr := ""
	graphBuildErr := ""
	useGraph := r.graphRoot != nil && TeamGraphRuntimeEnabledForTeam(def, teamRow.ID) && SupportsTeamGraphRuntimeMode(mode)

	if useGraph {
		graphAttempted = true
		graphExecID = uuid.NewString()
		cfg, cerr := CompileToGraphRuntimeConfigFromJSON(ctx, def, teamRow.DefinitionJSON, func(agentID string) string {
			ag, gerr := r.catalogAgent(ctx, agentID)
			if gerr != nil {
				return ""
			}
			return strings.TrimSpace(ag.AgentKey)
		}, r.graphLoader)
		if cerr != nil {
			graphCompileErr = cerr.Error()
			event.CtxFlowLogWarn(ctx, "team.graph_runtime.compile", "Graph 编译失败", event.P("error", cerr.Error()))
			metrics.TeamGraphRuntimeTotal.WithLabelValues("graph", "compile_error").Inc()
		} else {
			compiledGraphCfg = cfg
			groot, gerr := r.graphRoot.BuildTeamGraphRoot(ctx, cfg)
			if gerr != nil {
				graphBuildErr = gerr.Error()
				event.CtxFlowLogWarn(ctx, "team.graph_runtime.build", "GraphAgent 构建失败", event.P("error", gerr.Error()))
				metrics.TeamGraphRuntimeTotal.WithLabelValues("graph", "build_error").Inc()
			} else {
				root = groot
				_, memberLookup, err = BuildTeamMemberAgents(ctx, def, teamDeps, r.catalogAgent)
				if err != nil {
					return
				}
				if r.teamGraphCoord != nil {
					if regErr := r.teamGraphCoord.RegisterTeamGraphExecution(ctx, graphExecID, sess.ID, teamRow.ID, "", compiledGraphCfg); regErr != nil {
						event.CtxFlowLogWarn(ctx, "team.graph_runtime.register", "graph execution 注册失败", event.P("error", regErr.Error()))
					}
				}
				if teamEmitter != nil {
					teamEmitter.LogDone("team.run.graph", "Team GraphAgent 已构建", event.P("graph_execution_id", graphExecID))
				}
				metrics.TeamGraphRuntimeTotal.WithLabelValues("graph", "success").Inc()
				return
			}
		}
	}

	// Fallback to native runtime
	if root == nil {
		decision := DecideNativeFallback(def, teamRow.ID, graphAttempted, graphCompileErr, graphBuildErr, mode, r.graphRoot != nil)
		if decision.UseNative {
			root, memberLookup, err = BuildTRPCTeam(ctx, def, teamDeps, r.catalogAgent)
			if err != nil {
				return
			}
			metrics.TeamGraphRuntimeTotal.WithLabelValues("native", decision.MetricLabel).Inc()
			if teamEmitter != nil {
				teamEmitter.LogDone("team.run.build", "团队 Native 应急路径已构建", event.P("mode", mode), event.P("graph_attempted", graphAttempted))
			}
			return
		}

		// No fallback available — produce a clear error
		err = decision.Error()
		return
	}
	return
}
