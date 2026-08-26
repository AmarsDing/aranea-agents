package server

import (
	"encoding/json"
	"fmt"

	"aranea-agents/internal/biz"
)

// ws_v2_wire_convert.go — v2 领域事件 → Wire 类型的转换。
// Wire 类型定义与字节兼容约束见 ws_v2_wire.go。

func taskToWire(t biz.Task) taskWire {
	return taskWire{
		ID:          t.ID,
		SessionID:   t.SessionID,
		UserMessage: t.UserMessage,
		Status:      t.Status,
		Seq:         t.Seq,
		Version:     t.Version,
		WorkspaceID: t.WorkspaceID,
		CreatedAt:   t.CreatedAt,
		UpdatedAt:   t.UpdatedAt,
		CompletedAt: t.CompletedAt,
	}
}

func turnToWire(t biz.Turn) turnWire {
	return turnWire{
		ID:              t.ID,
		TaskID:          t.TaskID,
		SessionID:       t.SessionID,
		SpiritSessionID: t.SpiritSessionID,
		ParentTurnID:    t.ParentTurnID,
		AgentKey:        t.AgentKey,
		TeamID:          t.TeamID,
		TeamStageID:     t.TeamStageID,
		Seq:             t.Seq,
		Version:         t.Version,
		Status:          t.Status,
		StartedAt:       t.StartedAt,
		CompletedAt:     t.CompletedAt,
	}
}

func stepToWire(s biz.Step) stepWire {
	return stepWire{
		ID:              s.ID,
		TurnID:          s.TurnID,
		TaskID:          s.TaskID,
		SessionID:       s.SessionID,
		SpiritSessionID: s.SpiritSessionID,
		Kind:            s.Kind,
		AuthorAgentKey:  s.AuthorAgentKey,
		Seq:             s.Seq,
		Version:         s.Version,
		Content:         s.Content,
		Reasoning:       s.Reasoning,
		ToolName:        s.ToolName,
		ToolCallID:      s.ToolCallID,
		ToolArgs:        sanitizeRawJSON(s.ToolArgs),
		ToolResult:      sanitizeRawJSON(s.ToolResult),
		ToolDurationMs:  s.ToolDurationMs,
		ToolErrorCode:   s.ToolErrorCode,
		NoticeType:      s.NoticeType,
		Danger:          s.Danger,
		Status:          s.Status,
		IsFinal:         s.IsFinal,
		StartedAt:       s.StartedAt,
		CompletedAt:     s.CompletedAt,
	}
}

// sanitizeRawJSON returns nil for invalid JSON so json.Marshal emits null
// instead of failing with "error calling MarshalJSON for type json.RawMessage".
func sanitizeRawJSON(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	if !json.Valid(raw) {
		return nil
	}
	return raw
}

func memberInfoToWire(m biz.MemberInfo) memberInfoWire {
	return memberInfoWire{
		AgentKey:       m.AgentKey,
		AgentName:      m.AgentName,
		AvatarURL:      m.AvatarURL,
		ChildSessionID: m.ChildSessionID,
		Status:         m.Status,
	}
}

func teamStageToWire(ts biz.TeamStage) teamStageWire {
	return teamStageWire{
		ID:          ts.ID,
		TaskID:      ts.TaskID,
		TurnID:      ts.TurnID,
		SessionID:   ts.SessionID,
		TeamID:      ts.TeamID,
		TeamName:    ts.TeamName,
		DagNodeID:   ts.DagNodeID,
		DependsOn:   ts.DependsOn,
		Status:      ts.Status,
		Stage:       ts.Stage,
		Members:     mapSlice(ts.Members, memberInfoToWire),
		Strategy:    ts.Strategy,
		StartedAt:   ts.StartedAt,
		CompletedAt: ts.CompletedAt,
		Seq:         ts.Seq,
		Version:     ts.Version,
	}
}

func teamRunToWire(tr biz.TeamRun) teamRunWire {
	return teamRunWire{
		ID:              tr.ID,
		TeamStageID:     tr.TeamStageID,
		TaskID:          tr.TaskID,
		SessionID:       tr.SessionID,
		SpiritSessionID: tr.SpiritSessionID,
		DagNodeID:       tr.DagNodeID,
		DependsOn:       tr.DependsOn,
		Status:          tr.Status,
		StartedAt:       tr.StartedAt,
		CompletedAt:     tr.CompletedAt,
		Seq:             tr.Seq,
		Version:         tr.Version,
		Error:           tr.Error,
	}
}

func memberSessionToWire(ms biz.MemberSession) memberSessionWire {
	return memberSessionWire{
		ID:              ms.ID,
		TeamRunID:       ms.TeamRunID,
		TeamStageID:     ms.TeamStageID,
		TaskID:          ms.TaskID,
		SessionID:       ms.SessionID,
		SpiritSessionID: ms.SpiritSessionID,
		AgentKey:        ms.AgentKey,
		AgentName:       ms.AgentName,
		AvatarURL:       ms.AvatarURL,
		Status:          ms.Status,
		Seq:             ms.Seq,
		Version:         ms.Version,
		StartedAt:       ms.StartedAt,
		FinishedAt:      ms.FinishedAt,
		Error:           ms.Error,
	}
}

func tokenUsageToWire(u biz.TokenUsage) tokenUsageWire {
	return tokenUsageWire{
		PromptTokens:     u.PromptTokens,
		CompletionTokens: u.CompletionTokens,
		TotalTokens:      u.TotalTokens,
	}
}

func memberReportToWire(r biz.MemberReport) memberReportWire {
	return memberReportWire{
		AgentKey:   r.AgentKey,
		AgentName:  r.AgentName,
		Output:     r.Output,
		TokensUsed: tokenUsageToWire(r.TokensUsed),
		DurationMs: r.DurationMs,
		Error:      r.Error,
	}
}

func stepResultToWire(r *biz.StepResult) *stepResultWire {
	if r == nil {
		return nil
	}
	return &stepResultWire{
		Output:        r.Output,
		MemberReports: mapSlice(r.MemberReports, memberReportToWire),
		TokensUsed:    tokenUsageToWire(r.TokensUsed),
		DurationMs:    r.DurationMs,
	}
}

func stepErrorToWire(e *biz.StepError) *stepErrorWire {
	if e == nil {
		return nil
	}
	var failedMember *memberReportWire
	if e.FailedMember != nil {
		w := memberReportToWire(*e.FailedMember)
		failedMember = &w
	}
	return &stepErrorWire{
		Code:         e.Code,
		Message:      e.Message,
		Retryable:    e.Retryable,
		FailedMember: failedMember,
	}
}

func planStepToWire(ps biz.PlanStep) planStepWire {
	return planStepWire{
		ID:                  ps.ID,
		PlanID:              ps.PlanID,
		TaskID:              ps.TaskID,
		Label:               ps.Label,
		Description:         ps.Description,
		DependsOn:           ps.DependsOn,
		MappedTeamStageID:   ps.MappedTeamStageID,
		Status:              ps.Status,
		AutoSynthesis:       ps.AutoSynthesis,
		StartedAt:           ps.StartedAt,
		CompletedAt:         ps.CompletedAt,
		Seq:                 ps.Seq,
		Version:             ps.Version,
		Result:              stepResultToWire(ps.Result),
		Error:               stepErrorToWire(ps.Error),
		AgentKeys:           ps.AgentKeys,
		Mode:                ps.Mode,
		Deliverables:        ps.Deliverables,
		InputContract:       ps.InputContract,
		DepartmentID:        ps.DepartmentID,
		CrossDeptMemberKeys: ps.CrossDeptMemberKeys,
		DomainPath:          ps.DomainPath,
		AssignedName:        ps.AssignedName,
		MatchLayer:          ps.MatchLayer,
		MatchReason:         ps.MatchReason,
		GraphTemplateID:     ps.GraphTemplateID,
		ConfirmBefore:       ps.ConfirmBefore,
		CollectionIDs:       ps.CollectionIDs,
	}
}

func planBoardToWire(pb biz.PlanBoard) planBoardWire {
	return planBoardWire{
		ID:          pb.ID,
		TaskID:      pb.TaskID,
		TurnID:      pb.TurnID,
		SessionID:   pb.SessionID,
		Strategy:    pb.Strategy,
		Status:      pb.Status,
		Steps:       mapSlice(pb.Steps, planStepToWire),
		StartedAt:   pb.StartedAt,
		CompletedAt: pb.CompletedAt,
		Seq:         pb.Seq,
		Version:     pb.Version,
	}
}

func graphNodeToWire(n biz.GraphNode) graphNodeWire {
	return graphNodeWire{
		ID:           n.ID,
		GraphStageID: n.GraphStageID,
		Label:        n.Label,
		DagNodeID:    n.DagNodeID,
		TeamStageID:  n.TeamStageID,
		Status:       n.Status,
		DependsOn:    n.DependsOn,
	}
}

func skillCatalogEntryToWire(e biz.SkillCatalogEntry) skillCatalogEntryWire {
	return skillCatalogEntryWire{
		Slug:        e.Slug,
		Name:        e.Name,
		Description: e.Description,
		Tags:        e.Tags,
	}
}

func graphStageToWire(gs biz.GraphStage) graphStageWire {
	return graphStageWire{
		ID:          gs.ID,
		TaskID:      gs.TaskID,
		TurnID:      gs.TurnID,
		SessionID:   gs.SessionID,
		PlanBoardID: gs.PlanBoardID,
		Nodes:       mapSlice(gs.Nodes, graphNodeToWire),
		Status:      gs.Status,
		StartedAt:   gs.StartedAt,
		CompletedAt: gs.CompletedAt,
		Seq:         gs.Seq,
		Version:     gs.Version,
	}
}

// mapSlice maps a slice element-wise, preserving nil (nil in → nil out),
// which is required for byte-level JSON parity (nil slice marshals as null).
func mapSlice[T any, R any](in []T, fn func(T) R) []R {
	if in == nil {
		return nil
	}
	out := make([]R, len(in))
	for i, v := range in {
		out[i] = fn(v)
	}
	return out
}

// v2EventPayloadToWire converts a v2 domain Event to its wire payload (the
// value placed in wsEnvelope.Payload). Fail-closed: event types without an
// explicit mapping return an error instead of leaking domain internals via
// default marshaling.
func v2EventPayloadToWire(e biz.Event) (any, error) {
	switch ev := e.(type) {
	// Task
	case *biz.TaskCreatedEvent:
		return taskEventWire{Task: taskToWire(ev.Task)}, nil
	case *biz.TaskUpdatedEvent:
		return taskEventWire{Task: taskToWire(ev.Task)}, nil
	case *biz.TaskCompletedEvent:
		return taskEventWire{Task: taskToWire(ev.Task)}, nil
	case *biz.TaskFailedEvent:
		return taskEventWire{Task: taskToWire(ev.Task)}, nil
	// Turn
	case *biz.TurnStartedEvent:
		return turnEventWire{TurnID: ev.TurnID, Turn: turnToWire(ev.Turn)}, nil
	case *biz.TurnCompletedEvent:
		return turnEventWire{TurnID: ev.TurnID, Turn: turnToWire(ev.Turn)}, nil
	case *biz.TurnFailedEvent:
		return turnEventWire{TurnID: ev.TurnID, Turn: turnToWire(ev.Turn)}, nil
	// Step
	case *biz.StepCreatedEvent:
		return stepEventWire{Step: stepToWire(ev.Step)}, nil
	case *biz.StepStreamingEvent:
		return stepStreamingEventWire{StepID: ev.StepID, DeltaField: ev.DeltaField, DeltaChunk: ev.DeltaChunk, DeltaSeq: ev.DeltaSeq}, nil
	case *biz.StepUpdatedEvent:
		return stepEventWire{Step: stepToWire(ev.Step)}, nil
	case *biz.StepCompletedEvent:
		return stepEventWire{Step: stepToWire(ev.Step)}, nil
	case *biz.StepFailedEvent:
		return stepEventWire{Step: stepToWire(ev.Step)}, nil
	// TeamStage
	case *biz.TeamStageCreatedEvent:
		return teamStageEventWire{TeamStage: teamStageToWire(ev.TeamStage)}, nil
	case *biz.TeamStageUpdatedEvent:
		return teamStageEventWire{TeamStage: teamStageToWire(ev.TeamStage)}, nil
	case *biz.TeamStageCompletedEvent:
		return teamStageEventWire{TeamStage: teamStageToWire(ev.TeamStage)}, nil
	case *biz.TeamStageFailedEvent:
		return teamStageEventWire{TeamStage: teamStageToWire(ev.TeamStage)}, nil
	// TeamRun
	case *biz.TeamRunStartedEvent:
		return teamRunEventWire{TeamRun: teamRunToWire(ev.TeamRun)}, nil
	case *biz.TeamRunCompletedEvent:
		return teamRunEventWire{TeamRun: teamRunToWire(ev.TeamRun)}, nil
	case *biz.TeamRunFailedEvent:
		return teamRunEventWire{TeamRun: teamRunToWire(ev.TeamRun)}, nil
	// MemberSession
	case *biz.MemberSessionCreatedEvent:
		return memberSessionEventWire{MemberSession: memberSessionToWire(ev.MemberSession)}, nil
	case *biz.MemberSessionUpdatedEvent:
		return memberSessionEventWire{MemberSession: memberSessionToWire(ev.MemberSession)}, nil
	// PlanBoard
	case *biz.PlanBoardCreatedEvent:
		return planBoardEventWire{PlanBoard: planBoardToWire(ev.PlanBoard)}, nil
	case *biz.PlanBoardUpdatedEvent:
		return planBoardEventWire{PlanBoard: planBoardToWire(ev.PlanBoard)}, nil
	// PlanStep
	case *biz.PlanStepStartedEvent:
		return planStepEventWire{PlanStep: planStepToWire(ev.PlanStep)}, nil
	case *biz.PlanStepCompletedEvent:
		return planStepEventWire{PlanStep: planStepToWire(ev.PlanStep)}, nil
	case *biz.PlanStepFailedEvent:
		return planStepEventWire{PlanStep: planStepToWire(ev.PlanStep)}, nil
	case *biz.PlanStepSkippedEvent:
		return planStepSkippedEventWire{PlanStep: planStepToWire(ev.PlanStep), Reason: ev.Reason}, nil
	case *biz.PlanStepUpdatedEvent:
		return planStepEventWire{PlanStep: planStepToWire(ev.PlanStep)}, nil
	// GraphStage / GraphNode
	case *biz.GraphStageCreatedEvent:
		return graphStageEventWire{GraphStage: graphStageToWire(ev.GraphStage)}, nil
	case *biz.GraphStageUpdatedEvent:
		return graphStageEventWire{GraphStage: graphStageToWire(ev.GraphStage)}, nil
	case *biz.GraphStageCompletedEvent:
		return graphStageEventWire{GraphStage: graphStageToWire(ev.GraphStage)}, nil
	case *biz.GraphStageFailedEvent:
		return graphStageEventWire{GraphStage: graphStageToWire(ev.GraphStage)}, nil
	case *biz.GraphStageInterruptedEvent:
		return graphStageEventWire{GraphStage: graphStageToWire(ev.GraphStage)}, nil
	case *biz.GraphNodeUpdatedEvent:
		return graphNodeEventWire{GraphNode: graphNodeToWire(ev.GraphNode)}, nil
	// System domain
	case *biz.RunStatusEvent:
		return runStatusEventWire{RunID: ev.RunID, Status: ev.Status, Meta: ev.Meta}, nil
	case *biz.HeartbeatEvent:
		return heartbeatEventWire{Message: ev.Message, Meta: ev.Meta}, nil
	case *biz.SystemNoticeEvent:
		return systemNoticeEventWire{NoticeType: ev.NoticeType, Message: ev.Message, Meta: ev.Meta, Seq: ev.Seq}, nil
	case *biz.SkillCatalogEvent:
		return skillCatalogEventWire{Skills: mapSlice(ev.Skills, skillCatalogEntryToWire)}, nil
	// v1 bridge
	case *biz.ActivityBridgeEvent:
		return activityBridgeEventWire{Event: ev.Event}, nil
	default:
		return nil, fmt.Errorf("no wire mapping for v2 event kind %q (type %T)", e.EventKind(), e)
	}
}
