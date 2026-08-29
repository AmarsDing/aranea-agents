package service

import (
	"context"
	"fmt"

	v1 "aranea-agents/api/kratos/team/v1"
	"aranea-agents/internal/biz/decision"
	"aranea-agents/internal/event"
	"aranea-agents/internal/sandbox"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/auth"
	"aranea-agents/pkg/loggateway"

	"github.com/google/uuid"
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

	// P1-④（2026-08-30）：HITL 恢复执行强制写 hitl_approval 决策记录 +
	// flowlog。HTTP/gRPC 恢复入口 ctx 不带 turn TraceEmitter，flowlog 走
	// service 级 emitter（SetDecisionEvidence 注入的 monitorBus）。
	s.emitResumeDecision(ctx, run.ID, run.SessionID, execID)

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

// emitResumeDecision 把「用户触发 team run 恢复执行」双写到决策层
// （P1-④）：hitl_approval 决策记录（SourceRef 带 run/session 归属）+
// flowlog（service 级 emitter，monitorBus 由 SetDecisionEvidence 注入，
// nil 时仅进程日志；collector nil 时 EmitDecision 内部记 collector_nil）。
func (s *TeamService) emitResumeDecision(ctx context.Context, runID, sessionID, execID string) {
	flow := event.NewTraceEmitterForRun(event.TraceEmitterOpts{
		Ctx:       ctx,
		SessionID: sessionID,
		RunID:     runID,
		Domain:    event.TraceDomainChat,
		LG:        s.lg,
		Infra:     event.NewInfraFromBus(s.decisionBus),
	})
	flowCtx := event.WithTraceEmitter(ctx, flow)
	userID := "unknown"
	if a, ok := auth.FromContext(ctx); ok && a != nil && a.UserID > 0 {
		userID = fmt.Sprintf("%d", a.UserID)
	}
	event.EmitDecision(flowCtx, s.decisions, decision.Record{
		DecisionKey: uuid.NewString(),
		Category:    decision.CategoryHITLApproval,
		Scenario:    "团队运行恢复执行",
		Reasoning:   "用户触发中断/挂起 team run 的恢复执行（graph runtime resume 成功）",
		Outcome:     "resumed",
		ActorType:   decision.ActorHuman,
		ActorKey:    userID,
		SourceRef:   decision.SourceRef{RunID: runID, SessionID: sessionID},
		Metadata:    map[string]any{"session_id": sessionID, "run_id": runID, "graph_execution_id": execID},
	}, "team.run.resume", "团队运行恢复执行",
		event.P("trigger", "hitl_resume"),
		event.P("outcome", "resumed"),
		event.P("run_id", runID))
}
