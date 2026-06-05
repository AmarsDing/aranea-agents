package team

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
)

// observerSetup configures and starts all runtime observers for a team turn:
// orchestration status projector, graph task bridge, execution tracker,
// and graph step watcher. Returns stop functions for each observer.
type observerSetup struct {
	stopObsProjector   context.CancelFunc
	stopTaskBridge     context.CancelFunc
	stopExecTracker    context.CancelFunc
	stopGraphStepWatch context.CancelFunc
	activityFlusher    *ActivityStepFlusher
}

// startObservers initializes all team run observers and returns a setup
// struct with stop functions. The caller must defer-stop all observers.
func (r *Runner) startObservers(
	ctx context.Context,
	sess biz.Session,
	teamRow biz.Team,
	def Definition,
	run biz.TeamRun,
	graphExecID string,
	compiledTeam *biz.CompiledTeam,
) observerSetup {
	var setup observerSetup
	if r.td.Pipeline.Bus == nil {
		return setup
	}

	obsReg := BuildOrchestrationRegistry(def,
		func(agentID string) string {
			ag, cerr := r.catalogAgent(ctx, agentID)
			if cerr != nil {
				return ""
			}
			return strings.TrimSpace(ag.AgentKey)
		},
		func(agentID string) string {
			ag, cerr := r.catalogAgent(ctx, agentID)
			if cerr != nil {
				return ""
			}
			return strings.TrimSpace(ag.DisplayName)
		},
	)
	setup.activityFlusher = NewActivityStepFlusher(r.teams, run.ID, graphExecID, r.lg)
	failureOnError := ""
	if def.FailurePolicy != nil {
		failureOnError = def.FailurePolicy.OnError
	}
	setup.stopObsProjector = StartOrchestrationStatusProjector(ctx, r.td.Pipeline.Bus, OrchestrationProjectorConfig{
		RunID:            run.ID,
		TeamID:           teamRow.ID,
		SessionID:        sess.ID,
		Registry:         obsReg,
		GraphExecutionID: graphExecID,
		ActivityFlusher:  setup.activityFlusher,
		FailureOnError:   failureOnError,
	})
	if r.cfg.TeamGraphTasks != nil && graphExecID != "" {
		taskNodes := TaskNodesFromBuildConfig(compiledTeam.GraphBuildConfig)
		if len(taskNodes) > 0 {
			setup.stopTaskBridge = StartTeamGraphTaskBridge(ctx, r.td.Pipeline.Bus, TeamGraphTaskBridgeConfig{
				SessionID:        sess.ID,
				GraphExecutionID: graphExecID,
				Nodes:            taskNodes,
				Creator:          r.cfg.TeamGraphTasks,
			}, r.lg)
		}
	}
	if r.mediator != nil && graphExecID != "" {
		setup.stopExecTracker = StartTeamGraphExecutionTracker(ctx, r.td.Pipeline.Bus, TeamGraphExecutionTrackerConfig{
			SessionID:        sess.ID,
			GraphExecutionID: graphExecID,
			Registry:         r.mediator,
		}, r.lg)
		setup.stopGraphStepWatch = r.mediator.StartGraphStepWatch(ctx, graphExecID)
	}
	return setup
}

// stopAll stops all observers in reverse order.
func (s observerSetup) stopAll() {
	if s.stopGraphStepWatch != nil {
		s.stopGraphStepWatch()
	}
	if s.stopExecTracker != nil {
		s.stopExecTracker()
	}
	if s.stopTaskBridge != nil {
		s.stopTaskBridge()
	}
	if s.stopObsProjector != nil {
		s.stopObsProjector()
	}
	if s.activityFlusher != nil {
		s.activityFlusher.Stop()
	}
}
