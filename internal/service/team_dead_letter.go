package service

import (
	"context"
	"strings"

	v1 "aranea-agents/api/kratos/team/v1"
	"aranea-agents/internal/biz"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

func taskDeadLetterToProto(dl biz.TaskDeadLetter) *v1.TaskDeadLetter {
	return &v1.TaskDeadLetter{
		Id:               dl.ID,
		SourceType:       dl.SourceType,
		SourceId:         dl.SourceID,
		TeamId:           dl.TeamID,
		TeamRunId:        dl.TeamRunID,
		SessionId:        dl.SessionID,
		GraphExecutionId: dl.GraphExecutionID,
		ErrorMessage:     dl.ErrorMessage,
		PayloadJson:      dl.PayloadJSON,
		Status:           dl.Status,
		CreatedAt:        dl.CreatedAt,
		ResolvedAt:       dl.ResolvedAt,
	}
}

// ListTaskDeadLetters lists halted orchestration dead letters for admin review (FP-04).
func (s *TeamService) ListTaskDeadLetters(ctx context.Context, req *v1.ListTaskDeadLettersRequest) (*v1.ListTaskDeadLettersResponse, error) {
	if s == nil || s.uc == nil {
		return &v1.ListTaskDeadLettersResponse{}, nil
	}
	if req == nil {
		req = &v1.ListTaskDeadLettersRequest{}
	}
	sessionID := strings.TrimSpace(req.GetSessionId())
	teamID := strings.TrimSpace(req.GetTeamId())
	if sessionID == "" && teamID == "" {
		return nil, kerrors.BadRequest("TEAM", "session_id or team_id is required")
	}
	items, err := s.uc.ListTaskDeadLetters(ctx, biz.TaskDeadLetterListFilter{
		SessionID: sessionID,
		TeamID:    teamID,
		Status:    strings.TrimSpace(req.GetStatus()),
		Limit:     int(req.GetLimit()),
	})
	if err != nil {
		return nil, err
	}
	out := make([]*v1.TaskDeadLetter, 0, len(items))
	for _, item := range items {
		cp := item
		out = append(out, taskDeadLetterToProto(cp))
	}
	return &v1.ListTaskDeadLettersResponse{Items: out}, nil
}

// ResolveTaskDeadLetter marks a dead letter as resolved (FP-04).
func (s *TeamService) ResolveTaskDeadLetter(ctx context.Context, req *v1.ResolveTaskDeadLetterRequest) (*v1.ResolveTaskDeadLetterResponse, error) {
	if s == nil || s.uc == nil {
		return nil, kerrors.InternalServer("TEAM", "team service not configured")
	}
	id := strings.TrimSpace(req.GetId())
	if id == "" {
		return nil, kerrors.BadRequest("TEAM", "id is required")
	}
	item, err := s.uc.ResolveTaskDeadLetter(ctx, id)
	if err != nil {
		return nil, err
	}
	return &v1.ResolveTaskDeadLetterResponse{Item: taskDeadLetterToProto(item)}, nil
}

// ListSpiritTeams returns teams belonging to a spirit session (B-1).
func (s *TeamService) ListSpiritTeams(ctx context.Context, req *v1.ListSpiritTeamsRequest) (*v1.ListSpiritTeamsResponse, error) {
	if s == nil || s.uc == nil {
		return nil, kerrors.InternalServer("TEAM", "team service not configured")
	}
	spiritSessionID := strings.TrimSpace(req.GetSpiritSessionId())
	if spiritSessionID == "" {
		return nil, kerrors.BadRequest("TEAM", "spirit_session_id is required")
	}
	teams, err := s.uc.ListBySpiritSessionID(ctx, spiritSessionID)
	if err != nil {
		return nil, err
	}
	out := make([]*v1.SpiritTeamView, 0, len(teams))
	for i := range teams {
		out = append(out, toProtoSpiritTeamView(&teams[i]))
	}
	return &v1.ListSpiritTeamsResponse{Teams: out}, nil
}

// toProtoSpiritTeamView converts a biz.Team to SpiritTeamView proto.
// Fields not available on biz.Team (duration, tokens, steps, members) are left as zero values
// and should be populated from TeamRun data when available.
func toProtoSpiritTeamView(t *biz.Team) *v1.SpiritTeamView {
	return &v1.SpiritTeamView{
		Id:              t.ID,
		TeamName:        t.DisplayName,
		TaskSummary:     t.TaskDescription,
		Status:          t.Status,
		Mode:            t.Topology,
		SpiritSessionId: t.SpiritSessionID,
		DagNodeId:       t.DagNodeID,
		DependsOn:       t.DependsOn,
		InterruptReason: t.InterruptReason,
	}
}

// SynthesizeResults merges results from multiple teams (B-4).
func (s *TeamService) SynthesizeResults(ctx context.Context, req *v1.SynthesizeResultsRequest) (*v1.SynthesizeResultsResponse, error) {
	if s == nil || s.synthesis == nil {
		return nil, kerrors.InternalServer("TEAM", "synthesis service not configured")
	}
	spiritSessionID := strings.TrimSpace(req.GetSpiritSessionId())
	if spiritSessionID == "" {
		return nil, kerrors.BadRequest("TEAM", "spirit_session_id is required")
	}
	output, err := s.synthesis.SynthesizeResults(ctx, spiritSessionID, req.GetStrategy())
	if err != nil {
		return nil, err
	}
	resp := &v1.SynthesizeResultsResponse{
		Strategy:       string(output.Strategy),
		UnifiedSummary: output.Content,
	}
	for _, tr := range output.TeamResults {
		findings := []string{}
		if tr.KeyFindings != "" {
			findings = append(findings, tr.KeyFindings)
		}
		resp.TeamResults = append(resp.TeamResults, &v1.SynthesisTeamResult{
			TeamId:        tr.TeamID,
			TeamName:      tr.TeamName,
			Status:        tr.Status,
			ResultSummary: tr.Summary,
			KeyFindings:   findings,
		})
	}
	return resp, nil
}

// ArchiveTeam archives a completed/failed/cancelled team (SP-BE-25).
func (s *TeamService) ArchiveTeam(ctx context.Context, req *v1.ArchiveTeamRequest) (*v1.ArchiveTeamResponse, error) {
	if s == nil || s.uc == nil {
		return nil, kerrors.InternalServer("TEAM", "team service not configured")
	}
	teamID := strings.TrimSpace(req.GetTeamId())
	if teamID == "" {
		return nil, kerrors.BadRequest("TEAM", "team_id is required")
	}
	team, err := s.uc.TransitionStatus(ctx, teamID, biz.TeamStatusArchived)
	if err != nil {
		return nil, err
	}
	return &v1.ArchiveTeamResponse{TeamId: team.ID, Status: team.Status}, nil
}

// RetryTeam retries a failed/cancelled team by resetting its status and re-starting (SP-BE-26).
func (s *TeamService) RetryTeam(ctx context.Context, req *v1.RetryTeamRequest) (*v1.RetryTeamResponse, error) {
	if s == nil || s.uc == nil {
		return nil, kerrors.InternalServer("TEAM", "team service not configured")
	}
	teamID := strings.TrimSpace(req.GetTeamId())
	if teamID == "" {
		return nil, kerrors.BadRequest("TEAM", "team_id is required")
	}
	team, err := s.uc.RetryTeam(ctx, teamID)
	if err != nil {
		return nil, err
	}
	return &v1.RetryTeamResponse{TeamId: team.ID, Status: team.Status}, nil
}
