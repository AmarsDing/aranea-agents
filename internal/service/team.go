package service

import (
	"context"
	"database/sql"
	stderrors "errors"
	"fmt"
	"strings"
	"time"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	v1 "aranea-agents/api/kratos/team/v1"
	"aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	rt "aranea-agents/internal/runtime"
	"aranea-agents/internal/team"

	kerrors "github.com/go-kratos/kratos/v2/errors"

	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

type TeamService struct {
	v1.UnimplementedTeamServiceServer

	uc         *biz.TeamUsecase
	sessions   *biz.SessionUsecase
	teamRunner *team.Runner
	runs       *rt.RunRegistry
	eventBus   event.Bus
}

func NewTeamService(
	uc *biz.TeamUsecase,
	sessions *biz.SessionUsecase,
	teamRunner *team.Runner,
	runs *rt.RunRegistry,
	eventBus event.Bus,
) *TeamService {
	return &TeamService{uc: uc, sessions: sessions, teamRunner: teamRunner, runs: runs, eventBus: eventBus}
}

func toProtoTeam(t biz.Team) *v1.Team {
	return &v1.Team{
		Id:             t.ID,
		TeamKey:        t.TeamKey,
		DisplayName:    t.DisplayName,
		Status:         t.Status,
		IsDefault:      t.IsDefault,
		DefinitionJson: t.DefinitionJSON,
		AdkAppName:     t.ADKAppName,
		CreatedAt:      t.CreatedAt,
		UpdatedAt:      t.UpdatedAt,
		DeletedAt:      t.DeletedAt,
	}
}

func toProtoTeamRun(r biz.TeamRun) *v1.TeamRun {
	return &v1.TeamRun{
		Id:            r.ID,
		TeamId:        r.TeamID,
		SessionId:     r.SessionID,
		MessageId:     r.MessageID,
		Mode:          r.Mode,
		Status:        r.Status,
		InputPreview:  r.InputPreview,
		OutputPreview: r.OutputPreview,
		TokenIn:       int32(r.TokenIn),
		TokenOut:      int32(r.TokenOut),
		CostMicroUsd:  r.CostMicroUSD,
		DurationMs:    int32(r.DurationMS),
		ErrorMessage:  r.ErrorMessage,
		TopologyJson:  r.TopologyJSON,
		StartedAt:     r.StartedAt,
		FinishedAt:    r.FinishedAt,
		CreatedAt:     r.CreatedAt,
		UpdatedAt:     r.UpdatedAt,
	}
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
	}
}

func teamFromProto(pb *v1.Team) biz.Team {
	if pb == nil {
		return biz.Team{}
	}
	return biz.Team{
		ID:             pb.GetId(),
		TeamKey:        pb.GetTeamKey(),
		DisplayName:    pb.GetDisplayName(),
		Status:         pb.GetStatus(),
		IsDefault:      pb.GetIsDefault(),
		DefinitionJSON: pb.GetDefinitionJson(),
		ADKAppName:     pb.GetAdkAppName(),
		CreatedAt:      pb.GetCreatedAt(),
		UpdatedAt:      pb.GetUpdatedAt(),
		DeletedAt:      pb.GetDeletedAt(),
	}
}

func mapTeamErr(err error) error {
	if err == nil {
		return nil
	}
	if stderrors.Is(err, sql.ErrNoRows) {
		return kerrors.NotFound("TEAM", "team not found")
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
		out.Items = append(out.Items, toProtoTeam(items[i]))
	}
	return out, nil
}

func (s *TeamService) CreateTeam(ctx context.Context, req *v1.CreateTeamRequest) (*v1.Team, error) {
	in := biz.Team{
		TeamKey:        req.GetTeamKey(),
		DisplayName:    req.GetDisplayName(),
		Status:         req.GetStatus(),
		DefinitionJSON: req.GetDefinitionJson(),
		ADKAppName:     req.GetAdkAppName(),
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
	return toProtoTeam(t), nil
}

func (s *TeamService) UpdateTeam(ctx context.Context, req *v1.UpdateTeamRequest) (*v1.Team, error) {
	if req.GetTeam() == nil {
		return nil, kerrors.BadRequest("TEAM", "team body is required")
	}
	patch := teamFromProto(req.GetTeam())
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
	r, err := s.uc.GetRun(ctx, req.GetId())
	if err != nil {
		return nil, mapTeamErr(err)
	}
	if r.Status != "running" && r.Status != "pending" {
		return nil, kerrors.BadRequest("TEAM", "only running or pending team runs can be cancelled")
	}
	if s.runs != nil && strings.TrimSpace(r.SessionID) != "" {
		_ = s.runs.Cancel(r.SessionID)
		runID := strings.TrimSpace(r.ID)
		if entry, ok := s.runs.GetStatus(r.SessionID); ok && strings.TrimSpace(entry.RunID) != "" {
			runID = entry.RunID
		}
		CancelSessionRunSideEffects(ctx, s.eventBus, s.sessions, r.SessionID, runID)
	}
	now := agent.RFC3339Now()
	r.Status = "cancelled"
	r.FinishedAt = now
	r.UpdatedAt = now
	if err := s.uc.UpdateRun(ctx, r); err != nil {
		return nil, err
	}
	return toProtoTeamRun(r), nil
}

func (s *TeamService) RunTeamTest(ctx context.Context, req *v1.RunTeamTestRequest) (*v1.RunTeamTestResponse, error) {
	teamID := strings.TrimSpace(req.GetId())
	if teamID == "" {
		return nil, kerrors.BadRequest("TEAM", "team id is required")
	}
	if s.teamRunner == nil || s.sessions == nil {
		return nil, kerrors.InternalServer("TEAM", "team test runtime is not configured")
	}
	if _, err := s.uc.Get(ctx, teamID); err != nil {
		return nil, mapTeamErr(err)
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
	defer func() { _ = s.sessions.Delete(ctx, sess.ID) }()

	testReq := &chatv1.SendChatMessageRequest{
		SessionId: sess.ID,
		Content:   content,
		TeamId:    &teamID,
	}
	_, assistant, err := s.teamRunner.RunTurn(ctx, sess, testReq)
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
		run = biz.TeamRun{TeamID: teamID, SessionID: sess.ID, Status: "success"}
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
	snap, err := s.uc.ExportStructure(ctx, req.GetTeamId())
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
