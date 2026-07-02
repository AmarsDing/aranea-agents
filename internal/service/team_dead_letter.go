package service

import (
	"context"
	"strings"

	v1 "aranea-agents/api/kratos/team/v1"
	"aranea-agents/internal/agent"
	"aranea-agents/internal/biz"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
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
		return nil, apierror.BadRequest("TEAM", "session_id or team_id is required")
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
		return nil, apierror.Internal("TEAM", "team service not configured")
	}
	id := strings.TrimSpace(req.GetId())
	if id == "" {
		return nil, apierror.BadRequest("TEAM", "id is required")
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
		return nil, apierror.Internal("TEAM", "team service not configured")
	}
	spiritSessionID := strings.TrimSpace(req.GetSpiritSessionId())
	if spiritSessionID == "" {
		return nil, apierror.BadRequest("TEAM", "spirit_session_id is required")
	}
	teams, err := s.uc.ListBySpiritSessionID(ctx, spiritSessionID)
	if err != nil {
		return nil, err
	}
	// Batch-fetch latest TeamRun per team to populate run-derived fields
	// (duration, tokens, session, graph execution id).
	teamIDs := make([]string, 0, len(teams))
	for i := range teams {
		teamIDs = append(teamIDs, teams[i].ID)
	}
	runsByTeam, err := s.uc.ListRunsByTeamIDs(ctx, teamIDs, 1)
	if err != nil {
		s.lg.Warn("failed to batch-fetch team runs for spirit view", loggateway.Err(err))
		runsByTeam = nil
	}
	out := make([]*v1.SpiritTeamView, 0, len(teams))
	for i := range teams {
		var run *biz.TeamRunRecord
		if runs := runsByTeam[teams[i].ID]; len(runs) > 0 {
			run = &runs[0]
		}
		view := toProtoSpiritTeamView(&teams[i], run)
		if s.activityRepo != nil {
			if act, err := s.activityRepo.GetActivity(ctx, agent.TeamStageActivityID(teams[i].ID)); err == nil {
				view.Members = membersFromTeamStageActivity(act.Meta)
				if len(view.Members) > 0 {
					view.MemberAvatars = make([]string, 0, len(view.Members))
					for _, m := range view.Members {
						if m.AvatarUrl != "" {
							view.MemberAvatars = append(view.MemberAvatars, m.AvatarUrl)
						}
					}
				}
			}
		}
		out = append(out, view)
	}
	return &v1.ListSpiritTeamsResponse{Teams: out}, nil
}

// toProtoSpiritTeamView converts a biz.Team and optional latest TeamRun to
// SpiritTeamView proto. Fields not available on biz.Team or TeamRun (members,
// member_avatars, completed_steps, total_steps, shared_agent_ids,
// topology_reason) are left as zero values.
func toProtoSpiritTeamView(t *biz.Team, run *biz.TeamRunRecord) *v1.SpiritTeamView {
	view := &v1.SpiritTeamView{
		Id:              t.ID,
		TeamName:        t.DisplayName,
		TaskSummary:     biz.TruncateRunes(t.TaskDescription, 200),
		Status:          t.Status,
		Mode:            t.Topology,
		SpiritSessionId: t.SpiritSessionID,
		DagNodeId:       t.DagNodeID,
		DependsOn:       t.DependsOn,
		InterruptReason: t.InterruptReason,
	}
	if run != nil {
		view.DurationMs = int64(run.DurationMS)
		view.TokenIn = int64(run.TokenIn)
		view.TokenOut = int64(run.TokenOut)
		view.TeamSessionId = run.SessionID
		view.GraphExecutionId = run.GraphExecutionID
	}
	return view
}

// membersFromTeamStageActivity extracts the member list stored in a
// team_stage activity's meta (written by publishSpiritTeamAssembled). This
// provides the authoritative agent keys, display names, avatars and session
// IDs for the frontend sidebar without duplicating the assembly logic.
func membersFromTeamStageActivity(meta map[string]any) []*v1.SpiritMemberView {
	if meta == nil {
		return nil
	}
	raw, ok := meta["members"].([]any)
	if !ok || len(raw) == 0 {
		return nil
	}
	out := make([]*v1.SpiritMemberView, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, &v1.SpiritMemberView{
			AgentKey:    stringField(m, "agent_key"),
			DisplayName: stringField(m, "agent_name"),
			AvatarUrl:   stringField(m, "avatar_url"),
			Status:      stringField(m, "status"),
		})
	}
	return out
}

func stringField(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

// SynthesizeResults merges results from multiple teams (B-4).
func (s *TeamService) SynthesizeResults(ctx context.Context, req *v1.SynthesizeResultsRequest) (*v1.SynthesizeResultsResponse, error) {
	if s == nil || s.synthesis == nil {
		return nil, apierror.Internal("TEAM", "synthesis service not configured")
	}
	spiritSessionID := strings.TrimSpace(req.GetSpiritSessionId())
	if spiritSessionID == "" {
		return nil, apierror.BadRequest("TEAM", "spirit_session_id is required")
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
		return nil, apierror.Internal("TEAM", "team service not configured")
	}
	teamID := strings.TrimSpace(req.GetTeamId())
	if teamID == "" {
		return nil, apierror.BadRequest("TEAM", "team_id is required")
	}
	team, err := s.uc.TransitionStatus(ctx, teamID, biz.TeamStatusArchived)
	if err != nil {
		return nil, err
	}
	return &v1.ArchiveTeamResponse{TeamId: team.ID, Status: team.Status}, nil
}

// RetryTeam resets a failed/cancelled team to pending status so it can be
// re-started by the caller or scheduler (SP-BE-26). Note: this only resets
// the status; actual execution re-trigger must be initiated separately via
// the team run lifecycle (e.g. StartTeamTurn).
func (s *TeamService) RetryTeam(ctx context.Context, req *v1.RetryTeamRequest) (*v1.RetryTeamResponse, error) {
	if s == nil || s.uc == nil {
		return nil, apierror.Internal("TEAM", "team service not configured")
	}
	teamID := strings.TrimSpace(req.GetTeamId())
	if teamID == "" {
		return nil, apierror.BadRequest("TEAM", "team_id is required")
	}
	team, err := s.uc.RetryTeam(ctx, teamID)
	if err != nil {
		return nil, err
	}
	return &v1.RetryTeamResponse{TeamId: team.ID, Status: team.Status}, nil
}
