package service

import (
	"context"

	v1 "aranea-agents/api/kratos/team/v1"
	"aranea-agents/internal/biz/decision"
	"aranea-agents/internal/sandbox"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

func (s *TeamService) ResumeTeamRunExecution(ctx context.Context, req *v1.ResumeTeamRunExecutionRequest) (*v1.ResumeTeamRunExecutionResponse, error) {
	if err := s.assertRunTeamMutateAccess(ctx, req.GetRunId()); err != nil { // N5: IDOR
		return nil, err
	}
	run, err := s.uc.GetRun(ctx, req.GetRunId())
	if err != nil {
		return nil, mapTeamErr(err)
	}
	execID := run.GraphExecutionID
	if execID == "" {
		return nil, apierror.BadRequest("TEAM", "team run has no graph_execution_id; resume requires Graph runtime")
	}
	if s.graphUC == nil {
		return nil, apierror.Internal("TEAM", "graph runtime unavailable")
	}
	var resume map[string]any
	if req.GetResumeValue() != nil {
		resume = req.GetResumeValue().AsMap()
	}
	// 79-runtime-governance（2026-08-27 三轮审查根修）：resume 用 handler ctx
	// 重建 runtime，首次启动 runCtx 上的注入值随原执行终结而丢失，必须重注——
	// 否则恢复后成员闸事件 run 归属回落一次性 invocation uuid（RunGateStats
	// 恒漏计）、loop guard 隔离键跨执行清零、沙箱创建预算脱离 run 口径。
	// context.WithoutCancel（graph_execution_usecase ResumeExecution）保留
	// values，注入可穿透到成员节点执行。
	ctx = decision.WithGateRunID(ctx, run.ID)
	// T5（2026-08-27 四轮审查根修）：会话归属同坐标重注——resume 后成员闸
	// 事件经 GateSessionIDFromContext 取回，否则 SessionGateStats 对恢复执行
	// 段的闸事件恒漏计（team 图谱成员 invocation.Session 不保证 chat 归属）。
	ctx = decision.WithGateSessionID(ctx, run.SessionID)
	ctx = sandbox.WithRunID(ctx, run.ID)
	exec, err := s.graphUC.ResumeExecution(ctx, execID, resume)
	if err != nil {
		return nil, mapTeamErr(err)
	}

	// Delegate team status transition to biz layer.
	if run.TeamID != "" {
		if transErr := s.uc.ResumeTeamIfInterrupted(ctx, run.TeamID); transErr != nil {
			s.lg.Warn("恢复团队状态转换失败（graph 已恢复，不影响执行）",
				loggateway.StepID("team.resume.status_transition"),
				loggateway.Str("team_id", run.TeamID),
				loggateway.Err(transErr),
			)
		}
	}

	return &v1.ResumeTeamRunExecutionResponse{
		RunId:            run.ID,
		GraphExecutionId: execID,
		Status:           exec.Status,
	}, nil
}
