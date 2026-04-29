package service

import (
	"context"
	"database/sql"
	stderrors "errors"

	v1 "aranea-agents/api/kratos/team/v1"
	"aranea-agents/internal/biz"

	kerrors "github.com/go-kratos/kratos/v2/errors"

	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

// TeamService implements kratos team.v1.
type TeamService struct {
	v1.UnimplementedTeamServiceServer

	uc *biz.TeamUsecase
}

// NewTeamService constructs the service.
func NewTeamService(uc *biz.TeamUsecase) *TeamService {
	return &TeamService{uc: uc}
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

// ListTeams implements GET /v1/teams.
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

// CreateTeam implements POST /v1/teams.
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

// GetTeam implements GET /v1/teams/{id}.
func (s *TeamService) GetTeam(ctx context.Context, req *v1.GetTeamRequest) (*v1.Team, error) {
	t, err := s.uc.Get(ctx, req.GetId())
	if err != nil {
		return nil, mapTeamErr(err)
	}
	return toProtoTeam(t), nil
}

// UpdateTeam implements PATCH /v1/teams/{id}.
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

// DeleteTeam implements DELETE /v1/teams/{id}.
func (s *TeamService) DeleteTeam(ctx context.Context, req *v1.DeleteTeamRequest) (*emptypb.Empty, error) {
	if err := s.uc.Delete(ctx, req.GetId()); err != nil {
		return nil, mapTeamErr(err)
	}
	return &emptypb.Empty{}, nil
}

// DuplicateTeam implements POST /v1/teams/{id}/duplicate.
func (s *TeamService) DuplicateTeam(ctx context.Context, req *v1.DuplicateTeamRequest) (*v1.Team, error) {
	t, err := s.uc.Duplicate(ctx, req.GetId())
	if err != nil {
		return nil, mapTeamErr(err)
	}
	return toProtoTeam(t), nil
}

// ListTeamRuns implements GET /v1/team-runs.
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

// ListTeamRunSteps implements GET /v1/team-runs/{run_id}/steps.
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
