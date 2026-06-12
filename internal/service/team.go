package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	v1 "aranea-agents/api/kratos/team/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/internal/team"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

type TeamService struct {
	v1.UnimplementedTeamServiceServer

	uc          *biz.TeamUsecase
	graphUC     *biz.GraphUsecase
	agents      *biz.AgentUsecase
	sessions    *biz.SessionUsecase
	teamRunner  biz.TeamTurnRunnerPort
	runs        biz.RunRegistryPort
	eventBus    event.Bus
	lg          loggateway.Logger
	synthesis   *SpiritSynthesisService
}

func NewTeamService(
	uc *biz.TeamUsecase,
	graphUC *biz.GraphUsecase,
	agents *biz.AgentUsecase,
	sessions *biz.SessionUsecase,
	teamRunner biz.TeamTurnRunnerPort,
	runs biz.RunRegistryPort,
	eventBus event.Bus,
	lg loggateway.Logger,
	synthesis *SpiritSynthesisService,
) *TeamService {
	return &TeamService{
		uc: uc, graphUC: graphUC, agents: agents, sessions: sessions,
		teamRunner: teamRunner, runs: runs, eventBus: eventBus, lg: lg,
		synthesis: synthesis,
	}
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

func toProtoTeamRun(r biz.TeamRun) *v1.TeamRun {
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
	return err
}

func (s *TeamService) ListTeams(ctx context.Context, _ *v1.ListTeamsRequest) (*v1.ListTeamsResponse, error) {
	items, err := s.uc.List(ctx)
	if err != nil {
		return nil, err
	}
	out := &v1.ListTeamsResponse{Items: make([]*v1.Team, 0, len(items))}
	for i := range items {
		pb := toProtoTeam(items[i])
		if active, aerr := s.uc.HasActiveRun(ctx, items[i].ID); aerr == nil {
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
	created, err := s.uc.Create(ctx, in)
	if err != nil {
		return nil, err
	}
	return toProtoTeam(created), nil
}

func (s *TeamService) GetTeam(ctx context.Context, req *v1.GetTeamRequest) (*v1.Team, error) {
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
	patch := teamFromProto(req.GetTeam())
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
	if err := s.uc.Delete(ctx, req.GetId()); err != nil {
		return nil, mapTeamErr(err)
	}
	return &emptypb.Empty{}, nil
}

func (s *TeamService) DuplicateTeam(ctx context.Context, req *v1.DuplicateTeamRequest) (*v1.Team, error) {
	t, err := s.uc.Duplicate(ctx, req.GetId())
	if err != nil {
		return nil, mapTeamErr(err)
	}
	return toProtoTeam(t), nil
}

func (s *TeamService) ListTeamRuns(ctx context.Context, req *v1.ListTeamRunsRequest) (*v1.ListTeamRunsResponse, error) {
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
	r, err := s.uc.GetRun(ctx, req.GetId())
	if err != nil {
		return nil, mapTeamErr(err)
	}
	return toProtoTeamRun(r), nil
}

func (s *TeamService) CancelTeamRun(ctx context.Context, req *v1.CancelTeamRunRequest) (*v1.TeamRun, error) {
	r, err := s.uc.CancelRun(ctx, req.GetId())
	if err != nil {
		return nil, mapTeamErr(err)
	}
	if s.runs != nil && strings.TrimSpace(r.SessionID) != "" {
		if cancelled, reason := s.runs.Cancel(r.SessionID); !cancelled && reason != "" {
			s.lg.Warn("cancel team run session failed", loggateway.Str("session_id", r.SessionID), loggateway.Str("reason", reason))
		}
		runID := strings.TrimSpace(r.ID)
		if entry, ok := s.runs.GetStatus(r.SessionID); ok && strings.TrimSpace(entry.RunID) != "" {
			runID = entry.RunID
		}
		CancelSessionRunSideEffects(ctx, s.eventBus, s.sessions, r.SessionID, runID, s.lg)
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

	var run biz.TeamRun
	runs, listErr := s.uc.ListRuns(ctx, teamID, 10)
	if listErr == nil {
		for _, candidate := range runs {
			if candidate.SessionID == sess.ID {
				run = candidate
				break
			}
		}
		if run.ID == "" && len(runs) > 0 {
			run = runs[0]
		}
	}
	if run.ID == "" {
		run = biz.TeamRun{TeamID: teamID, SessionID: sess.ID, Status: biz.TeamRunStatusSuccess}
	}

	return &v1.RunTeamTestResponse{
		Run:   toProtoTeamRun(run),
		Reply: strings.TrimSpace(assistant.ContentMarkdown),
	}, nil
}

func (s *TeamService) ListTeamRunSteps(ctx context.Context, req *v1.ListTeamRunStepsRequest) (*v1.ListTeamRunStepsResponse, error) {
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
	updated, err := s.uc.UpdateSwarmMembers(ctx, req.GetTeamId(), req.GetAddAgentIds(), req.GetRemoveAgentIds())
	if err != nil {
		return nil, mapTeamErr(err)
	}
	return &v1.UpdateSwarmMembersResponse{Updated: updated}, nil
}

func (s *TeamService) ExportTeamStructure(ctx context.Context, req *v1.ExportTeamStructureRequest) (*v1.ExportTeamStructureResponse, error) {
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
	data, err := s.uc.GetRunSummary(ctx, req.GetId())
	if err != nil {
		return nil, mapTeamErr(err)
	}
	return &v1.GetTeamRunSummaryResponse{Summary: toProtoTeamRunSummary(data)}, nil
}
