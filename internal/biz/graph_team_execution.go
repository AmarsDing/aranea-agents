package biz

import (
	"context"
	"strings"
	"time"

	"aranea-agents/pkg/loggateway"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

// RegisterTeamGraphExecution indexes a team GraphAgent run for task/resume coordination (M53 Phase 7).
// Build config is kept in-memory; graph_id uses the team: prefix (not a persisted graph asset).
func (uc *GraphUsecase) RegisterTeamGraphExecution(ctx context.Context, execID, sessionID, teamID, teamRunID string, ct *CompiledTeam) error {
	if uc == nil {
		return nil
	}
	execID = strings.TrimSpace(execID)
	if execID == "" {
		return kerrors.BadRequest("GRAPH", "graph execution id required")
	}
	teamID = strings.TrimSpace(teamID)
	if teamID == "" {
		return kerrors.BadRequest("GRAPH", "team id required")
	}
	graphID := "team:" + teamID
	if teamRunID != "" {
		graphID = graphID + ":" + strings.TrimSpace(teamRunID)
	}
	exec := &GraphExecution{
		ID:        execID,
		GraphID:   graphID,
		SessionID: strings.TrimSpace(sessionID),
		Status:    TeamRunStatusRunning,
		StartedAt: time.Now(),
	}
	uc.mu.Lock()
	if uc.teamBuildConfigs == nil {
		uc.teamBuildConfigs = make(map[string]*CompiledTeam)
	}
	uc.mu.Unlock()

	if uc.runRepo != nil {
		if err := uc.runRepo.SaveRun(ctx, exec); err != nil {
			return err
		}
	}

	if uc.compiledTeamRepo != nil {
		if err := uc.compiledTeamRepo.Save(ctx, teamID, graphID, ct); err != nil {
			uc.lg.Warn("persist compiled team failed", loggateway.StepID("graph.register_team"), loggateway.Err(err))
		}
	}

	uc.mu.Lock()
	uc.teamBuildConfigs[execID] = ct
	uc.evictIfNeeded()
	uc.executions[execID] = exec
	uc.mu.Unlock()
	return nil
}

// MarkTeamGraphInterrupt records HITL/checkpoint pause for a team graph execution.
func (uc *GraphUsecase) MarkTeamGraphInterrupt(ctx context.Context, execID, nodeID, lineageID string) error {
	if uc == nil {
		return nil
	}
	exec, err := uc.loadExecution(ctx, strings.TrimSpace(execID))
	if err != nil {
		return err
	}
	nodeID = strings.TrimSpace(nodeID)
	lineageID = strings.TrimSpace(lineageID)
	exec.interruptMu.Lock()
	exec.interrupted = true
	exec.InterruptNode = nodeID
	exec.interruptMu.Unlock()
	uc.mu.Lock()
	exec.Status = TeamRunStatusWaitingHuman
	exec.CurrentNode = nodeID
	if lineageID != "" {
		exec.LineageID = lineageID
	}
	uc.mu.Unlock()
	if uc.runRepo == nil {
		return nil
	}
	return uc.runRepo.UpdateRun(ctx, exec)
}

func (uc *GraphUsecase) teamBuildConfig(execID string) (*CompiledTeam, bool) {
	if uc == nil {
		return nil, false
	}
	uc.mu.RLock()
	defer uc.mu.RUnlock()
	ct, ok := uc.teamBuildConfigs[strings.TrimSpace(execID)]
	return ct, ok
}
