package team

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
)

// observerSetup configures and starts all runtime observers for a team turn:
// orchestration status projector and graph step watcher.
// Returns stop functions for each observer.
type observerSetup struct {
	stopObsProjector   context.CancelFunc
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
	run biz.TeamRunRecord,
	graphExecID string,
	compiledTeam *biz.CompiledTeam,
) observerSetup {
	var setup observerSetup
	if r.td.Pipeline.ActivityBus == nil {
		return setup
	}

	obsReg := BuildOrchestrationRegistry(def,
		func(agentID string) string {
			ag, cerr := r.lookupAgent(ctx, agentID)
			if cerr != nil {
				return ""
			}
			return strings.TrimSpace(ag.AgentKey)
		},
		func(agentID string) string {
			ag, cerr := r.lookupAgent(ctx, agentID)
			if cerr != nil {
				return ""
			}
			return strings.TrimSpace(ag.DisplayName)
		},
	)
	setup.activityFlusher = NewActivityStepFlusher(nil, r.stepRepo, run.ID, graphExecID, r.lg)
	failureOnError := ""
	if def.FailurePolicy != nil {
		failureOnError = def.FailurePolicy.OnError
	}
	setup.stopObsProjector = StartOrchestrationStatusProjector(ctx, OrchestrationProjectorConfig{
		RunID:            run.ID,
		TeamID:           teamRow.ID,
		SessionID:        sess.ID,
		SpiritSessionID:  deriveSpiritSessionID(sess),
		Registry:         obsReg,
		GraphExecutionID: graphExecID,
		ActivityFlusher:  setup.activityFlusher,
		FailureOnError:   failureOnError,
		ActivityBus:      r.td.Pipeline.ActivityBus,
	})
	if r.mediator != nil && graphExecID != "" {
		setup.stopGraphStepWatch = r.mediator.StartGraphStepWatch(ctx, graphExecID)
	}
	return setup
}

// stopAll stops all observers in reverse order.
func (s observerSetup) stopAll() {
	if s.stopGraphStepWatch != nil {
		s.stopGraphStepWatch()
	}
	if s.stopObsProjector != nil {
		s.stopObsProjector()
	}
	if s.activityFlusher != nil {
		s.activityFlusher.Stop()
	}
}
