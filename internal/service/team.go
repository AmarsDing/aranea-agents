package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	v1 "aranea-agents/api/kratos/team/v1"
	"aranea-agents/internal/biz"
	rt "aranea-agents/internal/runtime"
	"aranea-agents/internal/team"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

type TeamService struct {
	v1.UnimplementedTeamServiceServer

	uc              *biz.TeamUsecase
	graphUC         *biz.GraphUsecase
	agents          *biz.AgentUsecase
	sessions        *biz.SessionUsecase
	teamRunner      biz.TeamTurnRunnerPort
	runs            biz.RunRegistryPort
	eventBus        biz.EventBus // Phase 3b-D Task 9: migrated from v1 ActivityEventBus to v2 EventBus
	lg              loggateway.Logger
	synthesis       *SpiritSynthesisService
	teamStageReader biz.TeamStageV2Reader
	stepReader      biz.StepV2Reader
	stepWriter      biz.StepV2Writer
	teamRunV2       biz.TeamRunV2Repo
	memberSessionV2 biz.MemberSessionV2Repo
	v2Seq           rt.EventPublisher
}

func NewTeamService(
	uc *biz.TeamUsecase,
	graphUC *biz.GraphUsecase,
	agents *biz.AgentUsecase,
	sessions *biz.SessionUsecase,
	teamRunner biz.TeamTurnRunnerPort,
	runs biz.RunRegistryPort,
	eventBus biz.EventBus,
	lg loggateway.Logger,
	synthesis *SpiritSynthesisService,
	teamStageReader biz.TeamStageV2Reader,
	stepReader biz.StepV2Reader,
	stepWriter biz.StepV2Writer,
	teamRunV2 biz.TeamRunV2Repo,
	memberSessionV2 biz.MemberSessionV2Repo,
	v2Seq rt.EventPublisher,
) *TeamService {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &TeamService{
		uc: uc, graphUC: graphUC, agents: agents, sessions: sessions,
		teamRunner: teamRunner, runs: runs, eventBus: eventBus, lg: lg,
		synthesis:       synthesis,
		teamStageReader: teamStageReader,
		stepReader:      stepReader,
		stepWriter:      stepWriter,
		teamRunV2:       teamRunV2,
		memberSessionV2: memberSessionV2,
		v2Seq:           v2Seq,
	}
}

// assertTeamAccess 验证 caller 是否可读取目标 team（P2-B IDOR 防护）。
// 共享 team（workspace_id=""）对所有租户可读；变更须用 assertTeamMutateAccess。
func (s *TeamService) assertTeamAccess(ctx context.Context, teamID string) error {
	return s.checkTeamAccess(ctx, teamID, false)
}

// assertTeamMutateAccess 验证 caller 是否可变更目标 team。
// 共享 team（workspace_id=""）对租户只读（fail-closed）。
func (s *TeamService) assertTeamMutateAccess(ctx context.Context, teamID string) error {
	return s.checkTeamAccess(ctx, teamID, true)
}

func (s *TeamService) checkTeamAccess(ctx context.Context, teamID string, mutate bool) error {
	if teamID == "" {
		return nil
	}
	t, err := s.uc.Get(ctx, teamID)
	if err != nil {
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return apierror.NotFound("TEAM", "team not found")
		}
		return err
	}
	callerWS := workspace.IDFromContext(ctx)
	if mutate {
		err = workspace.AssertWorkspaceMutate(callerWS, t.WorkspaceID)
	} else {
		err = workspace.AssertWorkspaceOrShared(callerWS, t.WorkspaceID)
	}
	if err != nil {
		s.lg.Warn("team access denied: workspace mismatch",
			loggateway.StepID("team.idor"),
			loggateway.Str("team_id", teamID),
			loggateway.Str("caller_ws", callerWS))
		return apierror.NotFound("TEAM", "team not found")
	}
	return nil
}

// assertRunTeamAccess 验证 caller 是否可读取目标 team run（P2-B IDOR 防护）。
// 通过 run.team_id 查找归属 team，再校验 team 的 workspace。
func (s *TeamService) assertRunTeamAccess(ctx context.Context, runID string) error {
	return s.checkRunTeamAccess(ctx, runID, false)
}

// assertRunTeamMutateAccess 验证 caller 是否可变更目标 team run。
func (s *TeamService) assertRunTeamMutateAccess(ctx context.Context, runID string) error {
	return s.checkRunTeamAccess(ctx, runID, true)
}

func (s *TeamService) checkRunTeamAccess(ctx context.Context, runID string, mutate bool) error {
	if runID == "" {
		return nil
	}
	run, err := s.uc.GetRun(ctx, runID)
	if err != nil {
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return apierror.NotFound("TEAM", "team run not found")
		}
		return err
	}
	return s.checkTeamAccess(ctx, run.TeamID, mutate)
}

func toProtoTeam(t biz.Team) *v1.Team {
	return &v1.Team{
		Id:                 t.ID,
		TeamKey:            t.TeamKey,
		DisplayName:        t.DisplayName,
		Status:             t.Status,
		IsDefault:          t.IsDefault,
		DefinitionJson:     t.DefinitionJSON,
		OrchestrationSpec:  toProtoOrchestrationSpec(t.DefinitionJSON),
		AdkAppName:         t.ADKAppName,
		CreatedAt:          t.CreatedAt,
		UpdatedAt:          t.UpdatedAt,
		DeletedAt:          t.DeletedAt,
		LinkedGraphId:      team.LinkedGraphIDFromDefinition(t.DefinitionJSON),
		DepartmentId:       t.DepartmentID,
		SpiritSessionId:    t.SpiritSessionID,
		TaskDescription:    t.TaskDescription,
		AutoCreated:        t.AutoCreated,
		DagNodeId:          t.DagNodeID,
		DependsOn:          t.DependsOn,
		ParallelConfigJson: t.ParallelConfigJSON,
		Readonly:           t.Readonly,
		Source:             t.Source,
		Kind:               t.Kind,
		Deliverables:       t.Deliverables,
		InputContract:      t.InputContract,
		DeptLeadAgentId:    t.DeptLeadAgentID,
		CrossDeptMemberIds: t.CrossDeptMemberIDs,
	}
}

func toProtoTeamRun(r biz.TeamRunRecord) *v1.TeamRun {
	return &v1.TeamRun{
		Id:                     r.ID,
		TeamId:                 r.TeamID,
		SessionId:              r.SessionID,
		MessageId:              r.MessageID,
		Mode:                   r.Mode,
		Status:                 r.Status,
		InputPreview:           r.InputPreview,
		OutputPreview:          r.OutputPreview,
		TokenIn:                int32(r.TokenIn),
		TokenOut:               int32(r.TokenOut),
		CostMicroUsd:           r.CostMicroUSD,
		DurationMs:             int32(r.DurationMS),
		ErrorMessage:           r.ErrorMessage,
		TopologyJson:           r.TopologyJSON,
		GraphExecutionId:       r.GraphExecutionID,
		DefinitionSnapshotJson: r.DefinitionSnapshotJSON,
		TraceId:                r.TraceID,
		StartedAt:              r.StartedAt,
		FinishedAt:             r.FinishedAt,
		CreatedAt:              r.CreatedAt,
		UpdatedAt:              r.UpdatedAt,
	}
}

func toProtoTeamRunSummary(data biz.TeamRunSummaryData) *v1.TeamRunSummary {
	out := &v1.TeamRunSummary{
		RunId:         data.RunID,
		TeamId:        data.TeamID,
		SessionId:     data.SessionID,
		Mode:          data.Mode,
		Status:        data.Status,
		DurationMs:    int32(data.DurationMS),
		TokenIn:       int32(data.TokenIn),
		TokenOut:      int32(data.TokenOut),
		CostMicroUsd:  data.CostMicroUSD,
		MemberCount:   int32(data.MemberCount),
		ToolCallCount: int32(data.ToolCallCount),
		OutputPreview: data.OutputPreview,
		ErrorMessage:  data.ErrorMessage,
	}
	for _, m := range data.Members {
		out.Members = append(out.Members, &v1.TeamRunMemberSummary{
			AgentId:       m.AgentID,
			AgentKey:      m.AgentKey,
			AgentName:     m.AgentName,
			Role:          m.Role,
			SortOrder:     int32(m.SortOrder),
			Status:        m.Status,
			TokenIn:       int32(m.TokenIn),
			TokenOut:      int32(m.TokenOut),
			DurationMs:    int32(m.DurationMS),
			CostMicroUsd:  m.CostMicroUSD,
			OutputPreview: m.OutputPreview,
			ToolCallCount: int32(m.ToolCallCount),
		})
	}
	return out
}

func toProtoTeamRunStep(s biz.TeamRunStep) *v1.TeamRunStep {
	return &v1.TeamRunStep{
		Id:            s.ID,
		RunId:         s.RunID,
		TeamId:        s.TeamID,
		AgentId:       s.AgentID,
		AgentKey:      s.AgentKey,
		AgentName:     s.AgentName,
		Role:          s.Role,
		SortOrder:     int32(s.SortOrder),
		Status:        s.Status,
		InputPreview:  s.InputPreview,
		OutputPreview: s.OutputPreview,
		TokenIn:       int32(s.TokenIn),
		TokenOut:      int32(s.TokenOut),
		CostMicroUsd:  s.CostMicroUSD,
		DurationMs:    int32(s.DurationMS),
		ErrorMessage:  s.ErrorMessage,
		StartedAt:     s.StartedAt,
		FinishedAt:    s.FinishedAt,
		CreatedAt:     s.CreatedAt,
		ToolCallCount: int32(s.ToolCallCount),
	}
}

func teamFromProto(pb *v1.Team) biz.Team {
	if pb == nil {
		return biz.Team{}
	}
	return biz.Team{
		ID:                 pb.GetId(),
		TeamKey:            pb.GetTeamKey(),
		DisplayName:        pb.GetDisplayName(),
		Status:             pb.GetStatus(),
		IsDefault:          pb.GetIsDefault(),
		DefinitionJSON:     pb.GetDefinitionJson(),
		ADKAppName:         pb.GetAdkAppName(),
		DepartmentID:       pb.GetDepartmentId(),
		Deliverables:       pb.GetDeliverables(),
		InputContract:      pb.GetInputContract(),
		DeptLeadAgentID:    pb.GetDeptLeadAgentId(),
		CrossDeptMemberIDs: pb.GetCrossDeptMemberIds(),
		SpiritSessionID:    pb.GetSpiritSessionId(),
		TaskDescription:    pb.GetTaskDescription(),
		AutoCreated:        pb.GetAutoCreated(),
		CreatedAt:          pb.GetCreatedAt(),
		UpdatedAt:          pb.GetUpdatedAt(),
		DeletedAt:          pb.GetDeletedAt(),
	}
}

func mapTeamErr(err error) error {
	if err == nil {
		return nil
	}
	if apierror.IsCode(err, apierror.CodeNotFound) {
		return apierror.NotFound("TEAM", "team not found")
	}
	if apierror.IsCode(err, apierror.CodeConflict) {
		return apierror.Conflict("TEAM", "team conflict")
	}
	return err
}

func (s *TeamService) ListTeams(ctx context.Context, req *v1.ListTeamsRequest) (*v1.ListTeamsResponse, error) {
	// P2-B: 系统 caller（workspace_id="")看全部；租户 caller 看 shared + own。
	callerWS := workspace.IDFromContext(ctx)
	if workspace.IsSystem(ctx) {
		callerWS = ""
	}
	if req.GetCountOnly() {
		n, err := s.uc.CountByWorkspace(ctx, callerWS)
		if err != nil {
			return nil, err
		}
		return &v1.ListTeamsResponse{Items: []*v1.Team{}, Total: int32(n)}, nil
	}
	items, err := s.uc.ListByWorkspace(ctx, callerWS)
	if err != nil {
		return nil, err
	}
	total := int32(len(items))
	if n, cerr := s.uc.CountByWorkspace(ctx, callerWS); cerr == nil {
		total = int32(n)
	}
	out := &v1.ListTeamsResponse{Items: make([]*v1.Team, 0, len(items)), Total: total}
	ids := make([]string, 0, len(items))
	for i := range items {
		ids = append(ids, items[i].ID)
	}
	// Batch active-run lookup (one query); fall back to per-team checks on error.
	activeByID, aerr := s.uc.ListActiveRunTeamIDs(ctx, ids)
	for i := range items {
		pb := toProtoTeam(items[i])
		if aerr == nil {
			pb.HasActiveRun = activeByID[items[i].ID]
		} else if active, serr := s.uc.HasActiveRun(ctx, items[i].ID); serr == nil {
			pb.HasActiveRun = active
		}
		out.Items = append(out.Items, pb)
	}
	return out, nil
}

func (s *TeamService) CreateTeam(ctx context.Context, req *v1.CreateTeamRequest) (*v1.Team, error) {
	defJSON := req.GetDefinitionJson()
	if strings.TrimSpace(defJSON) == "" {
		defJSON = biz.EnsureGraphRuntimeDefault("")
	} else {
		defJSON = biz.EnsureGraphRuntimeDefault(defJSON)
	}
	in := biz.Team{
		TeamKey:        req.GetTeamKey(),
		DisplayName:    req.GetDisplayName(),
		Status:         req.GetStatus(),
		DefinitionJSON: defJSON,
		ADKAppName:     req.GetAdkAppName(),
		DepartmentID:   req.GetDepartmentId(),
	}
	// P2-B: 租户 caller 创建的 team 绑定到 caller workspace；系统 caller 创建共享 team。
	if !workspace.IsSystem(ctx) {
		in.WorkspaceID = workspace.IDFromContext(ctx)
	}
	created, err := s.uc.Create(ctx, in)
	if err != nil {
		return nil, err
	}
	return toProtoTeam(created), nil
}

func (s *TeamService) GetTeam(ctx context.Context, req *v1.GetTeamRequest) (*v1.Team, error) {
	if err := s.assertTeamAccess(ctx, req.GetId()); err != nil {
		return nil, err
	}
	t, err := s.uc.Get(ctx, req.GetId())
	if err != nil {
		return nil, mapTeamErr(err)
	}
	out := toProtoTeam(t)
	if active, aerr := s.uc.HasActiveRun(ctx, t.ID); aerr == nil {
		out.HasActiveRun = active
	}
	return out, nil
}

func (s *TeamService) UpdateTeam(ctx context.Context, req *v1.UpdateTeamRequest) (*v1.Team, error) {
	if req.GetTeam() == nil {
		return nil, apierror.BadRequest("TEAM", "team body is required")
	}
	if err := s.assertTeamMutateAccess(ctx, req.GetId()); err != nil {
		return nil, err
	}
	patch := teamFromProto(req.GetTeam())
	patch.WorkspaceID = "" // P2-B: workspace_id immutable on update
	if pb := req.GetTeam(); pb != nil {
		base := patch.DefinitionJSON
		if strings.TrimSpace(base) == "" {
			current, err := s.uc.Get(ctx, req.GetId())
			if err != nil {
				return nil, mapTeamErr(err)
			}
			base = current.DefinitionJSON
		}
		if merged, err := mergeTeamDefinitionFromRequest(base, pb); err != nil {
			return nil, apierror.BadRequest("TEAM", "invalid orchestration_spec: "+err.Error())
		} else {
			patch.DefinitionJSON = merged
		}
	}
	t, err := s.uc.Update(ctx, req.GetId(), patch)
	if err != nil {
		return nil, mapTeamErr(err)
	}
	return toProtoTeam(t), nil
}

func (s *TeamService) DeleteTeam(ctx context.Context, req *v1.DeleteTeamRequest) (*emptypb.Empty, error) {
	if err := s.assertTeamMutateAccess(ctx, req.GetId()); err != nil {
		return nil, err
	}
	if err := s.uc.Delete(ctx, req.GetId()); err != nil {
		return nil, mapTeamErr(err)
	}
	return &emptypb.Empty{}, nil
}

func (s *TeamService) DuplicateTeam(ctx context.Context, req *v1.DuplicateTeamRequest) (*v1.Team, error) {
	if err := s.assertTeamAccess(ctx, req.GetId()); err != nil {
		return nil, err
	}
	t, err := s.uc.Duplicate(ctx, req.GetId())
	if err != nil {
		return nil, mapTeamErr(err)
	}
	return toProtoTeam(t), nil
}

func (s *TeamService) ListTeamRuns(ctx context.Context, req *v1.ListTeamRunsRequest) (*v1.ListTeamRunsResponse, error) {
	if err := s.assertTeamAccess(ctx, req.GetTeamId()); err != nil {
		return nil, err
	}
	items, err := s.uc.ListRuns(ctx, req.GetTeamId(), int(req.GetLimit()))
	if err != nil {
		return nil, err
	}
	out := &v1.ListTeamRunsResponse{Items: make([]*v1.TeamRun, 0, len(items))}
	for i := range items {
		out.Items = append(out.Items, toProtoTeamRun(items[i]))
	}
	return out, nil
}

func (s *TeamService) GetTeamRun(ctx context.Context, req *v1.GetTeamRunRequest) (*v1.TeamRun, error) {
	if err := s.assertRunTeamAccess(ctx, req.GetId()); err != nil {
		return nil, err
	}
	r, err := s.uc.GetRun(ctx, req.GetId())
	if err != nil {
		return nil, mapTeamErr(err)
	}
	return toProtoTeamRun(r), nil
}

func (s *TeamService) CancelTeamRun(ctx context.Context, req *v1.CancelTeamRunRequest) (*v1.TeamRun, error) {
	if err := s.assertRunTeamMutateAccess(ctx, req.GetId()); err != nil {
		return nil, err
	}
	r, err := s.uc.CancelRun(ctx, req.GetId())
	if err != nil {
		return nil, mapTeamErr(err)
	}
	if s.runs != nil && strings.TrimSpace(r.SessionID) != "" {
		if cancelled, reason := s.runs.Cancel(r.SessionID, "team_cancel"); !cancelled && reason != "" {
			s.lg.Warn("cancel team run session failed", loggateway.Str("session_id", r.SessionID), loggateway.Str("reason", reason))
		}
		runID := strings.TrimSpace(r.ID)
		if entry, ok := s.runs.GetStatus(r.SessionID); ok && strings.TrimSpace(entry.RunID) != "" {
			runID = entry.RunID
		}
		CancelSessionRunSideEffects(ctx, s.eventBus, s.stepReader, s.stepWriter, r.SessionID, runID, s.lg)
	}
	return toProtoTeamRun(r), nil
}

func (s *TeamService) RunTeamTest(ctx context.Context, req *v1.RunTeamTestRequest) (*v1.RunTeamTestResponse, error) {
	teamID := strings.TrimSpace(req.GetId())
	if teamID == "" {
		return nil, apierror.BadRequest("TEAM", "team id is required")
	}
	if s.teamRunner == nil || s.sessions == nil {
		return nil, apierror.Internal("TEAM", "team test runtime is not configured")
	}
	if err := s.assertTeamMutateAccess(ctx, teamID); err != nil {
		return nil, err
	}
	if _, err := s.uc.Get(ctx, teamID); err != nil {
		return nil, mapTeamErr(err)
	}
	active, err := s.uc.HasActiveRun(ctx, teamID)
	if err != nil {
		return nil, mapTeamErr(err)
	}
	if active {
		return nil, apierror.Conflict("TEAM", "team has an active run; test is not allowed until the run finishes")
	}

	content := strings.TrimSpace(req.GetContent())
	if content == "" {
		content = "Hello — please introduce your team briefly and respond in one short paragraph."
	}

	sess, err := s.sessions.Create(ctx, biz.Session{
		OwnerType: "team",
		TeamID:    teamID,
		Title:     fmt.Sprintf("Team test %s", time.Now().UTC().Format(time.RFC3339)),
		Status:    "active",
	})
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := s.sessions.Delete(ctx, sess.ID); err != nil {
			s.lg.Warn("delete team test session failed", loggateway.Err(err), loggateway.Str("session_id", sess.ID))
		}
	}()

	testInput := biz.TurnInput{
		SessionID: sess.ID,
		Content:   content,
		TeamID:    teamID,
	}
	_, assistant, err := s.teamRunner.RunTurnFromInput(ctx, sess, testInput)
	if err != nil {
		return nil, err
	}

	var run biz.TeamRunRecord
	runs, listErr := s.uc.ListRuns(ctx, teamID, 10)
	if listErr == nil {
		for _, candidate := range runs {
			if candidate.SessionID == sess.ID {
				run = candidate
				break
			}
		}
	}
	if run.ID == "" {
		// No persisted run found for this test session; synthesize a
		// lightweight response so the caller still gets a reply.
		run = biz.TeamRunRecord{TeamID: teamID, SessionID: sess.ID, Status: biz.TeamRunStatusSuccess}
	}

	return &v1.RunTeamTestResponse{
		Run:   toProtoTeamRun(run),
		Reply: strings.TrimSpace(assistant.ContentMarkdown),
	}, nil
}

func (s *TeamService) ListTeamRunSteps(ctx context.Context, req *v1.ListTeamRunStepsRequest) (*v1.ListTeamRunStepsResponse, error) {
	if err := s.assertRunTeamAccess(ctx, req.GetRunId()); err != nil {
		return nil, err
	}
	items, err := s.uc.ListRunSteps(ctx, req.GetRunId())
	if err != nil {
		return nil, err
	}
	out := &v1.ListTeamRunStepsResponse{Items: make([]*v1.TeamRunStep, 0, len(items))}
	for i := range items {
		out.Items = append(out.Items, toProtoTeamRunStep(items[i]))
	}
	return out, nil
}

func (s *TeamService) UpdateSwarmMembers(ctx context.Context, req *v1.UpdateSwarmMembersRequest) (*v1.UpdateSwarmMembersResponse, error) {
	if err := s.assertTeamMutateAccess(ctx, req.GetTeamId()); err != nil {
		return nil, err
	}
	updated, err := s.uc.UpdateSwarmMembers(ctx, req.GetTeamId(), req.GetAddAgentIds(), req.GetRemoveAgentIds())
	if err != nil {
		return nil, mapTeamErr(err)
	}
	return &v1.UpdateSwarmMembersResponse{Updated: updated}, nil
}

func (s *TeamService) ExportTeamStructure(ctx context.Context, req *v1.ExportTeamStructureRequest) (*v1.ExportTeamStructureResponse, error) {
	if err := s.assertTeamAccess(ctx, req.GetTeamId()); err != nil {
		return nil, err
	}
	snap, err := s.exportStructureViaCompiler(ctx, req.GetTeamId())
	if err != nil {
		return nil, mapTeamErr(err)
	}
	resp := &v1.ExportTeamStructureResponse{
		EntryNodeId: snap.EntryNodeID,
	}
	for _, n := range snap.Nodes {
		resp.Nodes = append(resp.Nodes, &v1.StructureNode{NodeId: n.NodeID, Kind: n.Kind, Name: n.Name})
	}
	for _, e := range snap.Edges {
		resp.Edges = append(resp.Edges, &v1.StructureEdge{FromNodeId: e.FromNodeID, ToNodeId: e.ToNodeID})
	}
	for _, sf := range snap.Surfaces {
		resp.Surfaces = append(resp.Surfaces, &v1.StructureSurface{NodeId: sf.NodeID, Name: sf.Name})
	}
	return resp, nil
}

func (s *TeamService) GetTeamRunSummary(ctx context.Context, req *v1.GetTeamRunSummaryRequest) (*v1.GetTeamRunSummaryResponse, error) {
	if err := s.assertRunTeamAccess(ctx, req.GetId()); err != nil {
		return nil, err
	}
	data, err := s.uc.GetRunSummary(ctx, req.GetId())
	if err != nil {
		return nil, mapTeamErr(err)
	}
	return &v1.GetTeamRunSummaryResponse{Summary: toProtoTeamRunSummary(data)}, nil
}
