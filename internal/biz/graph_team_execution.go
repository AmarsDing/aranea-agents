package biz

import (
	"context"
	"strings"
	"time"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

// RegisterTeamGraphExecution indexes a team GraphAgent run for task/resume coordination (M53 Phase 7).
// Build config is kept in-memory; graph_id uses the team: prefix (not a persisted graph asset).
func (uc *GraphUsecase) RegisterTeamGraphExecution(ctx context.Context, execID, sessionID, teamID, teamRunID string, cfg GraphBuildConfig) error {
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
	cfg = FinalizeGraphFailurePolicy(cfg)
	exec := &GraphExecution{
		ID:        execID,
		GraphID:   graphID,
		SessionID: strings.TrimSpace(sessionID),
		Status:    "running",
		StartedAt: time.Now(),
	}
	uc.mu.Lock()
	if uc.teamBuildConfigs == nil {
		uc.teamBuildConfigs = make(map[string]GraphBuildConfig)
	}
	uc.mu.Unlock()

	if uc.runRepo != nil {
		if err := uc.runRepo.SaveRun(ctx, exec); err != nil {
			return err
		}
	}

	uc.mu.Lock()
	uc.teamBuildConfigs[execID] = cfg
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
	uc.mu.Lock()
	exec.Status = "waiting_human"
	exec.InterruptNode = nodeID
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

func (uc *GraphUsecase) teamBuildConfig(execID string) (GraphBuildConfig, bool) {
	if uc == nil {
		return GraphBuildConfig{}, false
	}
	uc.mu.RLock()
	defer uc.mu.RUnlock()
	cfg, ok := uc.teamBuildConfigs[strings.TrimSpace(execID)]
	return cfg, ok
}
