package team

import (
	"context"
	"errors"

	"aranea-agents/internal/biz"
)

// HITL resume 路径共享 stub（从 team_graph_run_coordinator_test.go 拆出，
// AS-COG-01 行数纪律）。同包测试文件共享标识符，使用点保留在原测试文件。

type failingResumeBackend struct {
	inner TeamGraphExecutionBackend
}

func (b *failingResumeBackend) RegisterTeamGraphExecution(ctx context.Context, execID, sessionID, spiritSessionID, teamID, teamRunID, linkedGraphID string, ct *biz.CompiledTeam) error {
	return b.inner.RegisterTeamGraphExecution(ctx, execID, sessionID, spiritSessionID, teamID, teamRunID, linkedGraphID, ct)
}
func (b *failingResumeBackend) MarkTeamGraphInterrupt(ctx context.Context, execID, nodeID, lineageID string) error {
	return b.inner.MarkTeamGraphInterrupt(ctx, execID, nodeID, lineageID)
}
func (b *failingResumeBackend) ResumeExecution(context.Context, string, map[string]any) (*biz.GraphExecution, error) {
	return nil, errors.New("resume failed")
}
func (b *failingResumeBackend) RecoverOrphanedExecution(context.Context, string) (*biz.GraphExecution, error) {
	return nil, errors.New("resume failed")
}
func (b *failingResumeBackend) RecordTeamGraphNodeEnd(ctx context.Context, execID, nodeID string, stepIndex int, status, errMsg string) error {
	return b.inner.RecordTeamGraphNodeEnd(ctx, execID, nodeID, stepIndex, status, errMsg)
}
func (b *failingResumeBackend) FinalizeTeamGraphExecution(ctx context.Context, execID string, failed bool, errMsg string) error {
	return b.inner.FinalizeTeamGraphExecution(ctx, execID, failed, errMsg)
}
func (b *failingResumeBackend) GetExecution(ctx context.Context, executionID string) (*biz.GraphExecution, error) {
	return b.inner.GetExecution(ctx, executionID)
}

type succeedingResumeBackend struct {
	inner TeamGraphExecutionBackend
}

func (b *succeedingResumeBackend) RegisterTeamGraphExecution(ctx context.Context, execID, sessionID, spiritSessionID, teamID, teamRunID, linkedGraphID string, ct *biz.CompiledTeam) error {
	return b.inner.RegisterTeamGraphExecution(ctx, execID, sessionID, spiritSessionID, teamID, teamRunID, linkedGraphID, ct)
}
func (b *succeedingResumeBackend) MarkTeamGraphInterrupt(ctx context.Context, execID, nodeID, lineageID string) error {
	return b.inner.MarkTeamGraphInterrupt(ctx, execID, nodeID, lineageID)
}
func (b *succeedingResumeBackend) RecordTeamGraphNodeEnd(ctx context.Context, execID, nodeID string, stepIndex int, status, errMsg string) error {
	return b.inner.RecordTeamGraphNodeEnd(ctx, execID, nodeID, stepIndex, status, errMsg)
}
func (b *succeedingResumeBackend) FinalizeTeamGraphExecution(ctx context.Context, execID string, failed bool, errMsg string) error {
	return b.inner.FinalizeTeamGraphExecution(ctx, execID, failed, errMsg)
}
func (b *succeedingResumeBackend) ResumeExecution(ctx context.Context, executionID string, resumeValue map[string]any) (*biz.GraphExecution, error) {
	exec, err := b.inner.GetExecution(ctx, executionID)
	if err != nil {
		return nil, err
	}
	exec.Status = biz.TeamRunStatusRunning
	return exec, nil
}
func (b *succeedingResumeBackend) RecoverOrphanedExecution(ctx context.Context, executionID string) (*biz.GraphExecution, error) {
	return b.inner.RecoverOrphanedExecution(ctx, executionID)
}
func (b *succeedingResumeBackend) GetExecution(ctx context.Context, executionID string) (*biz.GraphExecution, error) {
	return b.inner.GetExecution(ctx, executionID)
}
