package service

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	v1 "aranea-agents/api/kratos/session/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
)

// SessionV2Service implements the v2 entity read RPCs (Task/Turn/Step + 7
// child entities) by delegating to the v2 repo readers. It replaces the v1
// ListActivities path.
//
// 2026-07-04 问题 6 修复：补齐 7 个 child entity 的读取 RPC，让前端
// fetchSessionHistory 刷新后能重建完整活动树（Plan/Graph/Team/Member）。
type SessionV2Service struct {
	v1.UnimplementedSessionV2ServiceServer

	taskReader          biz.TaskV2Reader
	turnReader          biz.TurnV2Reader
	stepReader          biz.StepV2Reader
	teamStageReader     biz.TeamStageV2Reader
	teamRunReader       biz.TeamRunV2Reader
	memberSessionReader biz.MemberSessionV2Reader
	planBoardReader     biz.PlanBoardV2Reader
	planStepReader      biz.PlanStepV2Reader
	graphStageReader    biz.GraphStageV2Reader
	graphNodeReader     biz.GraphNodeV2Reader
}

// NewSessionV2Service constructs a SessionV2Service from v2 repo readers.
func NewSessionV2Service(
	taskReader biz.TaskV2Reader,
	turnReader biz.TurnV2Reader,
	stepReader biz.StepV2Reader,
	teamStageReader biz.TeamStageV2Reader,
	teamRunReader biz.TeamRunV2Reader,
	memberSessionReader biz.MemberSessionV2Reader,
	planBoardReader biz.PlanBoardV2Reader,
	planStepReader biz.PlanStepV2Reader,
	graphStageReader biz.GraphStageV2Reader,
	graphNodeReader biz.GraphNodeV2Reader,
) *SessionV2Service {
	return &SessionV2Service{
		taskReader:          taskReader,
		turnReader:          turnReader,
		stepReader:          stepReader,
		teamStageReader:     teamStageReader,
		teamRunReader:       teamRunReader,
		memberSessionReader: memberSessionReader,
		planBoardReader:     planBoardReader,
		planStepReader:      planStepReader,
		graphStageReader:    graphStageReader,
		graphNodeReader:     graphNodeReader,
	}
}

// ListTasks returns all tasks for a session.
func (s *SessionV2Service) ListTasks(ctx context.Context, req *v1.ListTasksV2Request) (*v1.ListTasksV2Response, error) {
	sessionID := strings.TrimSpace(req.GetSessionId())
	if sessionID == "" {
		return nil, apierror.BadRequest(apierror.DomainShared, "session_id is required")
	}
	tasks, err := s.taskReader.ListTasksBySession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	out := make([]*v1.TaskV2, 0, len(tasks))
	for _, t := range tasks {
		out = append(out, bizTaskToProto(t))
	}
	return &v1.ListTasksV2Response{Tasks: out}, nil
}

// ListTurns returns all turns for a task.
func (s *SessionV2Service) ListTurns(ctx context.Context, req *v1.ListTurnsV2Request) (*v1.ListTurnsV2Response, error) {
	taskID := strings.TrimSpace(req.GetTaskId())
	if taskID == "" {
		return nil, apierror.BadRequest(apierror.DomainShared, "task_id is required")
	}
	turns, err := s.turnReader.ListTurnsByTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	out := make([]*v1.TurnV2, 0, len(turns))
	for _, t := range turns {
		out = append(out, bizTurnToProto(t))
	}
	return &v1.ListTurnsV2Response{Turns: out}, nil
}

// listStepsMaxLimit caps client-supplied page sizes to protect the DB.
const listStepsMaxLimit = 500

// ListSteps returns steps filtered by turn_id, task_id, or session_id.
func (s *SessionV2Service) ListSteps(ctx context.Context, req *v1.ListStepsV2Request) (*v1.ListStepsV2Response, error) {
	if req.GetLimit() < 0 || req.GetBeforeSeq() < 0 {
		return nil, apierror.BadRequest(apierror.DomainShared, "limit and before_seq must be >= 0")
	}
	var steps []biz.Step
	var err error
	var hasMore bool
	switch {
	case strings.TrimSpace(req.GetTurnId()) != "":
		steps, err = s.stepReader.ListStepsByTurn(ctx, req.GetTurnId())
	case strings.TrimSpace(req.GetTaskId()) != "":
		steps, err = s.stepReader.ListStepsByTask(ctx, req.GetTaskId())
	default:
		sessionID := strings.TrimSpace(req.GetSessionId())
		if sessionID == "" {
			return nil, apierror.BadRequest(apierror.DomainShared, "session_id is required when turn_id and task_id are empty")
		}
		if req.GetLimit() > 0 {
			limit := int(req.GetLimit())
			if limit > listStepsMaxLimit {
				limit = listStepsMaxLimit
			}
			steps, hasMore, err = s.stepReader.ListStepsBySessionPaged(ctx, sessionID, biz.StepListOptions{
				Limit:     limit,
				BeforeSeq: req.GetBeforeSeq(),
			})
		} else {
			steps, err = s.stepReader.ListStepsBySession(ctx, sessionID)
		}
	}
	if err != nil {
		return nil, err
	}
	out := make([]*v1.StepV2, 0, len(steps))
	for _, st := range steps {
		out = append(out, bizStepToProto(st))
	}
	return &v1.ListStepsV2Response{Steps: out, HasMore: hasMore}, nil
}

// GetStep returns a single step by ID.
func (s *SessionV2Service) GetStep(ctx context.Context, req *v1.GetStepV2Request) (*v1.GetStepV2Response, error) {
	stepID := strings.TrimSpace(req.GetStepId())
	if stepID == "" {
		return nil, apierror.BadRequest(apierror.DomainShared, "step_id is required")
	}
	step, err := s.stepReader.GetStep(ctx, stepID)
	if err != nil {
		return nil, err
	}
	return &v1.GetStepV2Response{Step: bizStepToProto(step)}, nil
}

// === 2026-07-04 问题 6 修复：7 个 child entity 的 List RPC ===
// 每个 handler 委托给对应的 v2 repo reader，并将 biz struct 转换为 proto 消息。
// 错误处理遵循 v2 约定：apierror.BadRequest 用于参数校验，repo 错误透传。

// ListTeamStages returns all team_stages for a task.
func (s *SessionV2Service) ListTeamStages(ctx context.Context, req *v1.ListTeamStagesV2Request) (*v1.ListTeamStagesV2Response, error) {
	taskID := strings.TrimSpace(req.GetTaskId())
	if taskID == "" {
		return nil, apierror.BadRequest(apierror.DomainShared, "task_id is required")
	}
	stages, err := s.teamStageReader.ListTeamStagesByTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	out := make([]*v1.TeamStageV2, 0, len(stages))
	for _, ts := range stages {
		out = append(out, bizTeamStageToProto(ts))
	}
	return &v1.ListTeamStagesV2Response{TeamStages: out}, nil
}

// ListTeamRuns returns all team_runs for a team_stage.
func (s *SessionV2Service) ListTeamRuns(ctx context.Context, req *v1.ListTeamRunsV2Request) (*v1.ListTeamRunsV2Response, error) {
	stageID := strings.TrimSpace(req.GetStageId())
	if stageID == "" {
		return nil, apierror.BadRequest(apierror.DomainShared, "stage_id is required")
	}
	runs, err := s.teamRunReader.ListTeamRunsByStage(ctx, stageID)
	if err != nil {
		return nil, err
	}
	out := make([]*v1.TeamRunV2, 0, len(runs))
	for _, tr := range runs {
		out = append(out, bizTeamRunToProto(tr))
	}
	return &v1.ListTeamRunsV2Response{TeamRuns: out}, nil
}

// ListMemberSessions returns all member_sessions for a team_run.
func (s *SessionV2Service) ListMemberSessions(ctx context.Context, req *v1.ListMemberSessionsV2Request) (*v1.ListMemberSessionsV2Response, error) {
	runID := strings.TrimSpace(req.GetRunId())
	if runID == "" {
		return nil, apierror.BadRequest(apierror.DomainShared, "run_id is required")
	}
	sessions, err := s.memberSessionReader.ListMemberSessionsByRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	out := make([]*v1.MemberSessionV2, 0, len(sessions))
	for _, ms := range sessions {
		out = append(out, bizMemberSessionToProto(ms))
	}
	return &v1.ListMemberSessionsV2Response{MemberSessions: out}, nil
}

// ListOrphanMemberSessions returns Mode B member sessions (empty TeamRunID)
// for a spirit root session — restores orphan agent-cards after refresh.
func (s *SessionV2Service) ListOrphanMemberSessions(ctx context.Context, req *v1.ListOrphanMemberSessionsV2Request) (*v1.ListMemberSessionsV2Response, error) {
	sessionID := strings.TrimSpace(req.GetSessionId())
	if sessionID == "" {
		return nil, apierror.BadRequest(apierror.DomainShared, "session_id is required")
	}
	sessions, err := s.memberSessionReader.ListOrphanMemberSessionsBySpiritSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	out := make([]*v1.MemberSessionV2, 0, len(sessions))
	for _, ms := range sessions {
		out = append(out, bizMemberSessionToProto(ms))
	}
	return &v1.ListMemberSessionsV2Response{MemberSessions: out}, nil
}

// ListPlanBoards returns all plan_boards for a task.
func (s *SessionV2Service) ListPlanBoards(ctx context.Context, req *v1.ListPlanBoardsV2Request) (*v1.ListPlanBoardsV2Response, error) {
	taskID := strings.TrimSpace(req.GetTaskId())
	if taskID == "" {
		return nil, apierror.BadRequest(apierror.DomainShared, "task_id is required")
	}
	boards, err := s.planBoardReader.ListPlanBoardsByTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	out := make([]*v1.PlanBoardV2, 0, len(boards))
	for _, pb := range boards {
		out = append(out, bizPlanBoardToProto(pb))
	}
	return &v1.ListPlanBoardsV2Response{PlanBoards: out}, nil
}

// ListPlanSteps returns all plan_steps for a task.
func (s *SessionV2Service) ListPlanSteps(ctx context.Context, req *v1.ListPlanStepsV2Request) (*v1.ListPlanStepsV2Response, error) {
	taskID := strings.TrimSpace(req.GetTaskId())
	if taskID == "" {
		return nil, apierror.BadRequest(apierror.DomainShared, "task_id is required")
	}
	steps, err := s.planStepReader.ListPlanStepsByTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	out := make([]*v1.PlanStepV2, 0, len(steps))
	for _, ps := range steps {
		out = append(out, bizPlanStepToProto(ps))
	}
	return &v1.ListPlanStepsV2Response{PlanSteps: out}, nil
}

// ListGraphStages returns all graph_stages for a task.
func (s *SessionV2Service) ListGraphStages(ctx context.Context, req *v1.ListGraphStagesV2Request) (*v1.ListGraphStagesV2Response, error) {
	taskID := strings.TrimSpace(req.GetTaskId())
	if taskID == "" {
		return nil, apierror.BadRequest(apierror.DomainShared, "task_id is required")
	}
	stages, err := s.graphStageReader.ListGraphStagesByTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	out := make([]*v1.GraphStageV2, 0, len(stages))
	for _, gs := range stages {
		out = append(out, bizGraphStageToProto(gs))
	}
	return &v1.ListGraphStagesV2Response{GraphStages: out}, nil
}

// ListGraphNodes returns all graph_nodes for a graph_stage.
//
// 2026-07-05 修复：GraphNode.DependsOn 是 in-memory 字段（不持久化到 graph_nodes_v2 表），
// 需在读取时从 PlanStep.DependsOn 派生。
// 派生路径：graphStageID → GraphStage.PlanBoardID → ListPlanStepsByPlan →
// 用 gn.DagNodeID == PlanStep.ID 关联，填充 DependsOn。
// 设计依据：internal/biz/graph_stage.go 注释「DependsOn 取自 PlanStep.DependsOn（派生，不持久化）」。
func (s *SessionV2Service) ListGraphNodes(ctx context.Context, req *v1.ListGraphNodesV2Request) (*v1.ListGraphNodesV2Response, error) {
	stageID := strings.TrimSpace(req.GetStageId())
	if stageID == "" {
		return nil, apierror.BadRequest(apierror.DomainShared, "stage_id is required")
	}
	nodes, err := s.graphNodeReader.ListGraphNodesByStage(ctx, stageID)
	if err != nil {
		return nil, err
	}
	// 派生 DependsOn：从 PlanStep 关联数据填充
	depMap, err := s.buildGraphNodeDependsOnMap(ctx, stageID)
	if err != nil {
		// 派生失败不阻断主流程（前端有 fallback），仅记录日志后继续
		// 注：service 层无 logger，按现有代码风格保持静默降级
	} else {
		for i := range nodes {
			if deps, ok := depMap[nodes[i].DagNodeID]; ok {
				nodes[i].DependsOn = deps
			}
		}
	}
	out := make([]*v1.GraphNodeV2, 0, len(nodes))
	for _, gn := range nodes {
		out = append(out, bizGraphNodeToProto(gn))
	}
	return &v1.ListGraphNodesV2Response{GraphNodes: out}, nil
}

// buildGraphNodeDependsOnMap 构造 dagNodeID → PlanStep.DependsOn 的映射。
// 用于在 ListGraphNodes 时派生 GraphNode.DependsOn 字段。
func (s *SessionV2Service) buildGraphNodeDependsOnMap(ctx context.Context, graphStageID string) (map[string][]string, error) {
	gs, err := s.graphStageReader.GetGraphStage(ctx, graphStageID)
	if err != nil {
		return nil, err
	}
	if gs.PlanBoardID == "" {
		return nil, nil
	}
	steps, err := s.planStepReader.ListPlanStepsByPlan(ctx, gs.PlanBoardID)
	if err != nil {
		return nil, err
	}
	out := make(map[string][]string, len(steps))
	for _, ps := range steps {
		out[ps.ID] = ps.DependsOn
	}
	return out, nil
}

// === Conversion helpers (biz → proto) ===

func bizTaskToProto(t biz.Task) *v1.TaskV2 {
	return &v1.TaskV2{
		Id:          t.ID,
		SessionId:   t.SessionID,
		UserMessage: t.UserMessage,
		Status:      string(t.Status),
		Seq:         t.Seq,
		Version:     t.Version,
		CreatedAt:   t.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   t.UpdatedAt.Format(time.RFC3339),
		CompletedAt: formatTimePtr(t.CompletedAt),
	}
}

func bizTurnToProto(t biz.Turn) *v1.TurnV2 {
	return &v1.TurnV2{
		Id:              t.ID,
		TaskId:          t.TaskID,
		SessionId:       t.SessionID,
		SpiritSessionId: t.SpiritSessionID,
		ParentTurnId:    t.ParentTurnID,
		AgentKey:        t.AgentKey,
		TeamId:          t.TeamID,
		TeamStageId:     t.TeamStageID,
		Seq:             t.Seq,
		Version:         t.Version,
		Status:          string(t.Status),
		StartedAt:       t.StartedAt.Format(time.RFC3339),
		CompletedAt:     formatTimePtr(t.CompletedAt),
	}
}

func bizStepToProto(s biz.Step) *v1.StepV2 {
	return &v1.StepV2{
		Id:              s.ID,
		TurnId:          s.TurnID,
		TaskId:          s.TaskID,
		SessionId:       s.SessionID,
		SpiritSessionId: s.SpiritSessionID,
		Kind:            string(s.Kind),
		AuthorAgentKey:  s.AuthorAgentKey,
		Seq:             s.Seq,
		Version:         s.Version,
		Content:         s.Content,
		Reasoning:       s.Reasoning,
		ToolName:        s.ToolName,
		ToolCallId:      s.ToolCallID,
		ToolArgs:        []byte(s.ToolArgs),
		ToolResult:      []byte(s.ToolResult),
		ToolDurationMs:  s.ToolDurationMs,
		ToolErrorCode:   s.ToolErrorCode,
		NoticeType:      s.NoticeType,
		Danger:          s.Danger,
		Status:          string(s.Status),
		IsFinal:         s.IsFinal,
		StartedAt:       s.StartedAt.Format(time.RFC3339),
		CompletedAt:     formatTimePtr(s.CompletedAt),
	}
}

// formatTimePtr formats a *time.Time as RFC3339, returning "" for nil.
func formatTimePtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.RFC3339)
}

// === 2026-07-04 问题 6 修复：7 个 child entity 的 biz→proto 转换函数 ===

func bizTeamStageToProto(ts biz.TeamStage) *v1.TeamStageV2 {
	members := make([]*v1.MemberInfoV2, 0, len(ts.Members))
	for _, m := range ts.Members {
		members = append(members, &v1.MemberInfoV2{
			AgentKey:       m.AgentKey,
			AgentName:      m.AgentName,
			AvatarUrl:      m.AvatarURL,
			ChildSessionId: m.ChildSessionID,
			Status:         m.Status,
		})
	}
	return &v1.TeamStageV2{
		Id:          ts.ID,
		TaskId:      ts.TaskID,
		TurnId:      ts.TurnID,
		SessionId:   ts.SessionID,
		TeamId:      ts.TeamID,
		TeamName:    ts.TeamName,
		DagNodeId:   ts.DagNodeID,
		DependsOn:   ts.DependsOn,
		Status:      string(ts.Status),
		Stage:       string(ts.Stage),
		Members:     members,
		Strategy:    ts.Strategy,
		StartedAt:   ts.StartedAt.Format(time.RFC3339),
		CompletedAt: formatTimePtr(ts.CompletedAt),
		Seq:         ts.Seq,
		Version:     ts.Version,
	}
}

func bizTeamRunToProto(tr biz.TeamRun) *v1.TeamRunV2 {
	return &v1.TeamRunV2{
		Id:              tr.ID,
		TeamStageId:     tr.TeamStageID,
		TaskId:          tr.TaskID,
		SessionId:       tr.SessionID,
		SpiritSessionId: tr.SpiritSessionID,
		DagNodeId:       tr.DagNodeID,
		DependsOn:       tr.DependsOn,
		Status:          string(tr.Status),
		StartedAt:       tr.StartedAt.Format(time.RFC3339),
		CompletedAt:     formatTimePtr(tr.CompletedAt),
		Seq:             tr.Seq,
		Version:         tr.Version,
		Error:           tr.Error,
	}
}

func bizMemberSessionToProto(ms biz.MemberSession) *v1.MemberSessionV2 {
	return &v1.MemberSessionV2{
		Id:              ms.ID,
		TeamRunId:       ms.TeamRunID,
		TeamStageId:     ms.TeamStageID,
		TaskId:          ms.TaskID,
		SessionId:       ms.SessionID,
		SpiritSessionId: ms.SpiritSessionID,
		AgentKey:        ms.AgentKey,
		AgentName:       ms.AgentName,
		AvatarUrl:       ms.AvatarURL,
		Status:          string(ms.Status),
		Seq:             ms.Seq,
		Version:         ms.Version,
		StartedAt:       ms.StartedAt.Format(time.RFC3339),
		FinishedAt:      formatTimePtr(ms.FinishedAt),
		Error:           ms.Error,
	}
}

func bizPlanBoardToProto(pb biz.PlanBoard) *v1.PlanBoardV2 {
	steps := make([]*v1.PlanStepV2, 0, len(pb.Steps))
	for _, ps := range pb.Steps {
		steps = append(steps, bizPlanStepToProto(ps))
	}
	return &v1.PlanBoardV2{
		Id:          pb.ID,
		TaskId:      pb.TaskID,
		TurnId:      pb.TurnID,
		SessionId:   pb.SessionID,
		Strategy:    string(pb.Strategy),
		Status:      string(pb.Status),
		Steps:       steps,
		StartedAt:   pb.StartedAt.Format(time.RFC3339),
		CompletedAt: formatTimePtr(pb.CompletedAt),
		Seq:         pb.Seq,
		Version:     pb.Version,
	}
}

func bizPlanStepToProto(ps biz.PlanStep) *v1.PlanStepV2 {
	return &v1.PlanStepV2{
		Id:                ps.ID,
		PlanId:            ps.PlanID,
		TaskId:            ps.TaskID,
		Label:             ps.Label,
		Description:       ps.Description,
		DependsOn:         ps.DependsOn,
		MappedTeamStageId: ps.MappedTeamStageID,
		Status:            string(ps.Status),
		AutoSynthesis:     ps.AutoSynthesis,
		StartedAt:         ps.StartedAt.Format(time.RFC3339),
		CompletedAt:       formatTimePtr(ps.CompletedAt),
		Seq:               ps.Seq,
		Version:           ps.Version,
		Result:            bizStepResultToProto(ps.Result),
		Error:             bizStepErrorToProto(ps.Error),
	}
}

func bizStepResultToProto(r *biz.StepResult) *v1.StepResultV2 {
	if r == nil {
		return nil
	}
	reports := make([]*v1.MemberReportV2, 0, len(r.MemberReports))
	for _, mr := range r.MemberReports {
		reports = append(reports, bizMemberReportToProto(mr))
	}
	return &v1.StepResultV2{
		Output:        r.Output,
		MemberReports: reports,
		TokensUsed:    bizTokenUsageToProto(r.TokensUsed),
		DurationMs:    r.DurationMs,
	}
}

func bizStepErrorToProto(e *biz.StepError) *v1.StepErrorV2 {
	if e == nil {
		return nil
	}
	var failedMember *v1.MemberReportV2
	if e.FailedMember != nil {
		failedMember = bizMemberReportToProto(*e.FailedMember)
	}
	return &v1.StepErrorV2{
		Code:         e.Code,
		Message:      e.Message,
		Retryable:    e.Retryable,
		FailedMember: failedMember,
	}
}

func bizMemberReportToProto(mr biz.MemberReport) *v1.MemberReportV2 {
	return &v1.MemberReportV2{
		AgentKey:   mr.AgentKey,
		AgentName:  mr.AgentName,
		Output:     mr.Output,
		TokensUsed: bizTokenUsageToProto(mr.TokensUsed),
		DurationMs: mr.DurationMs,
		Error:      mr.Error,
	}
}

func bizTokenUsageToProto(t biz.TokenUsage) *v1.TokenUsageV2 {
	return &v1.TokenUsageV2{
		PromptTokens:     t.PromptTokens,
		CompletionTokens: t.CompletionTokens,
		TotalTokens:      t.TotalTokens,
	}
}

func bizGraphStageToProto(gs biz.GraphStage) *v1.GraphStageV2 {
	nodes := make([]*v1.GraphNodeV2, 0, len(gs.Nodes))
	for _, gn := range gs.Nodes {
		nodes = append(nodes, bizGraphNodeToProto(gn))
	}
	return &v1.GraphStageV2{
		Id:          gs.ID,
		TaskId:      gs.TaskID,
		TurnId:      gs.TurnID,
		SessionId:   gs.SessionID,
		PlanBoardId: gs.PlanBoardID,
		Nodes:       nodes,
		Status:      string(gs.Status),
		StartedAt:   gs.StartedAt.Format(time.RFC3339),
		CompletedAt: formatTimePtr(gs.CompletedAt),
		Seq:         gs.Seq,
		Version:     gs.Version,
	}
}

func bizGraphNodeToProto(gn biz.GraphNode) *v1.GraphNodeV2 {
	return &v1.GraphNodeV2{
		Id:           gn.ID,
		GraphStageId: gn.GraphStageID,
		Label:        gn.Label,
		DagNodeId:    gn.DagNodeID,
		TeamStageId:  gn.TeamStageID,
		Status:       string(gn.Status),
		DependsOn:    gn.DependsOn,
	}
}

// stepV2ToActivityV1 converts a v2 StepV2 proto back to the v1 Activity proto
// shape. This is used by SessionService.ListActivities (Task 4 of Phase 3b-D)
// to keep the v1 ListActivitiesResponse backward compatible while reads are
// migrated to the v2 steps_v2 table. The frontend will switch to the v2
// endpoint directly, after which this adapter can be removed (Phase 3b-E).
//
// Field mapping notes:
//   - v2 StepV2 has no ParentActivityId/ChildBoardId/TeamId/DagNodeId/
//     DependsOn/AgentName/Label/Collapsed/PromptTokens/CompletionTokens;
//     these v1-only fields are left as zero values.
//   - MetaJson: populated with kind-specific metadata that v1 carried in
//     Activity.Meta (is_final, notice_type, author_agent_key).
//   - DurationMs: computed from StartedAt + CompletedAt when both are present
//     (v2 StepV2 stores them as RFC3339 strings).
func stepV2ToActivityV1(s *v1.StepV2) *v1.Activity {
	if s == nil {
		return nil
	}
	meta := make(map[string]any, 3)
	if s.IsFinal {
		meta["is_final"] = true
	}
	if s.NoticeType != "" {
		meta["notice_type"] = s.NoticeType
	}
	if s.AuthorAgentKey != "" {
		meta["agent_key"] = s.AuthorAgentKey
	}
	metaJSON := ""
	if len(meta) > 0 {
		if b, err := json.Marshal(meta); err == nil {
			metaJSON = string(b)
		}
	}
	return &v1.Activity{
		Id:              s.GetId(),
		Kind:            s.GetKind(),
		Status:          s.GetStatus(),
		SessionId:       s.GetSessionId(),
		TurnId:          s.GetTurnId(),
		Timestamp:       s.GetStartedAt(),
		DurationMs:      computeStepDurationMs(s.GetStartedAt(), s.GetCompletedAt()),
		Seq:             s.GetSeq(),
		Content:         s.GetContent(),
		Reasoning:       s.GetReasoning(),
		ToolName:        s.GetToolName(),
		ToolCallId:      s.GetToolCallId(),
		ToolArguments:   string(s.GetToolArgs()),
		ToolResult:      string(s.GetToolResult()),
		ToolDurationMs:  s.GetToolDurationMs(),
		ToolErrorCode:   s.GetToolErrorCode(),
		SpiritSessionId: s.GetSpiritSessionId(),
		AgentKey:        s.GetAuthorAgentKey(),
		MetaJson:        metaJSON,
	}
}

// computeStepDurationMs returns the duration in milliseconds between startedAt
// and completedAt (RFC3339 strings). Returns 0 if either value is empty or
// cannot be parsed.
func computeStepDurationMs(startedAt, completedAt string) int64 {
	if startedAt == "" || completedAt == "" {
		return 0
	}
	start, err := time.Parse(time.RFC3339, startedAt)
	if err != nil {
		return 0
	}
	end, err := time.Parse(time.RFC3339, completedAt)
	if err != nil {
		return 0
	}
	if end.Before(start) {
		return 0
	}
	return end.Sub(start).Milliseconds()
}
