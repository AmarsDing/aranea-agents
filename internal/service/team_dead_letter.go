package service

import (
	"context"
	"strings"

	v1 "aranea-agents/api/kratos/team/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/team"

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
	if teamID != "" {
		if err := s.assertTeamAccess(ctx, teamID); err != nil { // N5: IDOR
			return nil, err
		}
	} else if sessionID != "" {
		if err := s.assertSpiritSessionAccess(ctx, sessionID); err != nil { // N5: IDOR
			return nil, err
		}
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
	if err := s.assertDeadLetterAccess(ctx, id); err != nil { // N5: IDOR
		return nil, err
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
	if err := s.assertSpiritSessionAccess(ctx, spiritSessionID); err != nil { // N5: IDOR
		return nil, err
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
		// Members come from v2 TeamStage (typed []MemberInfo). Legacy
		// activities-table fallback was removed with ActivityBridge/dual-write.
		// S-3 后 stage 按 (teamID, rootTaskID) 每轮一行，此 API ctx 无
		// RootTaskActivityID——查团队最新行取成员，无行则 members 为空。
		var members []*v1.SpiritMemberView
		if s.teamStageReader != nil {
			if ts, err := s.teamStageReader.GetLatestTeamStageByTeam(ctx, teams[i].ID); err == nil {
				members = memberInfosToSpiritViews(ts.Members)
			}
		}
		// R3（2026-09-01）：团队尚未运行（无 TeamStage 行）时 members 为空，
		// 前端团队列表/graph 看不到成员——用户期望「团队分配完成即确认成员」。
		// 回退解析 definition_json.members（创建时静态定义），状态一律 pending；
		// 运行后 TeamStage 成员（携带真实执行状态）自然覆盖本回退。
		if len(members) == 0 {
			members = s.definitionMemberViews(ctx, teams[i].DefinitionJSON)
		}
		if len(members) > 0 {
			view.Members = members
			view.MemberAvatars = make([]string, 0, len(members))
			for _, m := range members {
				if m.AvatarUrl != "" {
					view.MemberAvatars = append(view.MemberAvatars, m.AvatarUrl)
				}
			}
		}
		out = append(out, view)
	}
	return &v1.ListSpiritTeamsResponse{Teams: out}, nil
}

// definitionMemberViews 回退解析团队定义（DefinitionJSON）中的成员——团队尚未
// 运行（无 TeamStage 行）时成员列表仍应可见（R3，2026-09-01）：创建即确认成员
// 构成，而非等到运行时才出现。状态统一 pending（尚无执行事实）；AgentKey /
// DisplayName / AvatarURL 尽量经 agents 解析对齐运行时口径，解析失败降级为
// agent_id 占位（显示重于精确，infra 错误不得拖垮列表接口）。
func (s *TeamService) definitionMemberViews(ctx context.Context, definitionJSON string) []*v1.SpiritMemberView {
	raw := strings.TrimSpace(definitionJSON)
	if raw == "" {
		return nil
	}
	def, err := team.ParseDefinition(raw)
	if err != nil || len(def.Members) == 0 {
		return nil
	}
	out := make([]*v1.SpiritMemberView, 0, len(def.Members))
	for _, m := range def.Members {
		if m.Enabled != nil && !*m.Enabled {
			continue
		}
		agentID := strings.TrimSpace(m.AgentID)
		if agentID == "" {
			continue
		}
		mv := &v1.SpiritMemberView{
			AgentKey:    agentID,
			DisplayName: strings.TrimSpace(m.Name),
			Status:      "pending",
		}
		if s.agents != nil {
			if ag, aerr := s.agents.Get(ctx, agentID); aerr == nil {
				if key := strings.TrimSpace(ag.AgentKey); key != "" {
					mv.AgentKey = key
				}
				if mv.DisplayName == "" {
					mv.DisplayName = strings.TrimSpace(ag.DisplayName)
				}
				mv.AvatarUrl = ag.Icon
			}
		}
		if mv.DisplayName == "" {
			mv.DisplayName = agentID
		}
		out = append(out, mv)
	}
	return out
}

// memberInfosToSpiritViews converts typed v2 TeamStage.Members ([]biz.MemberInfo)
// to the proto SpiritMemberView shape used by the frontend sidebar.
func memberInfosToSpiritViews(in []biz.MemberInfo) []*v1.SpiritMemberView {
	if len(in) == 0 {
		return nil
	}
	out := make([]*v1.SpiritMemberView, 0, len(in))
	for _, m := range in {
		out = append(out, &v1.SpiritMemberView{
			AgentKey:    m.AgentKey,
			DisplayName: m.AgentName,
			AvatarUrl:   m.AvatarURL,
			Status:      m.Status,
		})
	}
	return out
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

// SynthesizeResults merges results from multiple teams (B-4).
func (s *TeamService) SynthesizeResults(ctx context.Context, req *v1.SynthesizeResultsRequest) (*v1.SynthesizeResultsResponse, error) {
	if s == nil {
		return nil, apierror.Internal("TEAM", "synthesis service not configured")
	}
	spiritSessionID := strings.TrimSpace(req.GetSpiritSessionId())
	if spiritSessionID == "" {
		return nil, apierror.BadRequest("TEAM", "spirit_session_id is required")
	}
	if err := s.assertSpiritSessionAccess(ctx, spiritSessionID); err != nil { // N5: IDOR
		return nil, err
	}
	if s.synthesis == nil {
		return nil, apierror.Internal("TEAM", "synthesis service not configured")
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
	if err := s.assertTeamAccess(ctx, teamID); err != nil { // N5: IDOR
		return nil, err
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
	if err := s.assertTeamAccess(ctx, teamID); err != nil { // N5: IDOR
		return nil, err
	}
	team, err := s.uc.RetryTeam(ctx, teamID)
	if err != nil {
		return nil, err
	}
	return &v1.RetryTeamResponse{TeamId: team.ID, Status: team.Status}, nil
}
