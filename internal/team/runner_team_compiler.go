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

func (r *Runner) tryNativeFallback(
	ctx context.Context,
	def Definition,
	teamDeps TRPCTeamBuilderDeps,
	metricLabel string,
) (
	root trpcagent.Agent,
	memberLookup map[string]trpcagent.Agent,
	ok bool,
	err error,
) {
	if !envTeamNativeForced() {
		return nil, nil, false, nil
	}
	root, memberLookup, err = BuildTRPCTeam(ctx, def, teamDeps, r.catalogAgent, r.lg)
	if err != nil {
		return nil, nil, false, err
	}
	metrics.TeamGraphRuntimeTotal.WithLabelValues("native", metricLabel).Inc()
	return root, memberLookup, true, nil
}

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
	if r.graphRoot == nil {
		if nRoot, nLookup, nOk, nErr := r.tryNativeFallback(ctx, def, teamDeps, "native_emergency"); nOk {
			return nRoot, nLookup, "", nil, nil
		} else if nErr != nil {
			return nil, nil, "", nil, nErr
		}
		err = DecideNativeFallback(def, teamRow.ID, false, "", "", mode, false).Error()
		return
	}

	if !SupportsTeamGraphRuntimeMode(mode) {
		err = DecideNativeFallback(def, teamRow.ID, false, "", "", mode, true).Error()
		return
	}

	graphExecID = uuid.NewString()
	ct, cerr := CompileToGraphRuntimeConfigFromJSON(ctx, def, teamRow.DefinitionJSON, func(agentID string) string {
		ag, gerr := r.catalogAgent(ctx, agentID)
		if gerr != nil {
			return ""
		}
		return strings.TrimSpace(ag.AgentKey)
	}, r.graphLoader, r.lg)
	if cerr != nil {
		r.lg.Warn("Graph 编译失败", loggateway.StepID("team.graph_runtime.compile"), loggateway.Err(cerr))
		metrics.TeamGraphRuntimeTotal.WithLabelValues("graph", "compile_error").Inc()
		if nRoot, nLookup, nOk, nErr := r.tryNativeFallback(ctx, def, teamDeps, "native_fallback"); nOk {
			return nRoot, nLookup, "", nil, nil
		} else if nErr != nil {
			return nil, nil, "", nil, nErr
		}
		err = DecideNativeFallback(def, teamRow.ID, true, cerr.Error(), "", mode, true).Error()
		return
	}

	compiledTeam = ct
	groot, gerr := r.graphRoot.BuildTeamGraphRoot(ctx, ct.GraphBuildConfig)
	if gerr != nil {
		r.lg.Warn("GraphAgent 构建失败", loggateway.StepID("team.graph_runtime.build"), loggateway.Err(gerr))
		metrics.TeamGraphRuntimeTotal.WithLabelValues("graph", "build_error").Inc()
		if nRoot, nLookup, nOk, nErr := r.tryNativeFallback(ctx, def, teamDeps, "native_fallback"); nOk {
			return nRoot, nLookup, "", nil, nil
		} else if nErr != nil {
			return nil, nil, "", nil, nErr
		}
		err = DecideNativeFallback(def, teamRow.ID, true, "", gerr.Error(), mode, true).Error()
		return
	}

	root = groot
	_, memberLookup, err = BuildTeamMemberAgents(ctx, def, teamDeps, r.catalogAgent, r.lg)
	if err != nil {
		return
	}
	if r.teamGraphCoord != nil {
		if regErr := r.teamGraphCoord.RegisterTeamGraphExecution(ctx, graphExecID, sess.ID, teamRow.ID, runID, compiledTeam); regErr != nil {
			r.lg.Warn("graph execution 注册失败", loggateway.StepID("team.graph_runtime.register"), loggateway.Err(regErr))
		}
	}
	if teamEmitter != nil {
		teamEmitter.LogDone("team.run.graph", "Team GraphAgent 已构建", event.P("graph_execution_id", graphExecID))
	}
	metrics.TeamGraphRuntimeTotal.WithLabelValues("graph", "success").Inc()
	return
}
