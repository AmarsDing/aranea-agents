package biz

import (
	"aranea-agents/pkg/loggateway"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"sort"
	"strings"
)

// SpiritDelivery owns deliverable extraction, upstream injection, verification
// gates, and member-execution evidence for Spirit (DEV-09).
type SpiritDelivery struct {
	teamUC            SpiritTeamAssembler
	sessionUC         SpiritSessionAccessor
	agentUC           SpiritAgentResolver
	contractValidator *DeliverableContractValidator
	gateExecutor      *VerificationGateExecutor
	stepReader        SpiritStepReader
	graphDelivReader  SpiritGraphDeliverableReader
	runStatsReader    SpiritTeamRunStatsReader
	// heartbeatReader 是 P2-1 持久化心跳读取端口（idle 探测信号源之一）。
	// nil = 探测回退 steps.started_at 旧语义（向后兼容）。
	heartbeatReader    SpiritTeamRunHeartbeatReader
	inboxFS            TeamInboxFS
	artifactSaver      deliverableArtifactSaver
	artifactPublicBase string
	lg                 loggateway.Logger
}

// deliverableArtifactSaver 是 biz/artifact.Saver 的结构等价窄接口（biz 不能
// 反向 import biz/artifact，循环依赖）。nil = 桥接关闭（向后兼容）。
type deliverableArtifactSaver interface {
	Save(ctx context.Context, sessionID, name, mimeType string, data []byte) (Artifact, error)
}

// ListTeamRunStats returns per-team latest-run stats for report enrichment.
// Returns nil when the stats reader is not wired (v1-only deployments).
func (d *SpiritDelivery) ListTeamRunStats(ctx context.Context, teamIDs []string) map[string]SpiritTeamRunStats {
	if d.runStatsReader == nil || len(teamIDs) == 0 {
		return nil
	}
	stats, err := d.runStatsReader.ListLatestRunStatsByTeams(ctx, teamIDs)
	if err != nil {
		d.lg.Warn("查询团队运行统计失败，省略 per-unit 耗时/错误字段",
			loggateway.StepID("spirit.teams.run_stats_err"),
			loggateway.Err(err),
		)
		return nil
	}
	return stats
}

func (d *SpiritDelivery) ExtractTeamOutput(ctx context.Context, teamID string) (summary string, keyFindings string, err error) {
	full, err := d.extractTeamFullOutput(ctx, teamID)
	if err != nil || full.Content == "" {
		return "", "", err
	}
	return TruncateRunes(full.Content, MaxSummaryLen), extractKeyFindings(full.Content), nil
}

// resolveTeamMainSessionID returns the team's main session ID — the session
// whose ID keys the trpc graph state. Member agent sessions share the same
// team_id and Search ordering is not guaranteed, so the main session is
// identified by SessionType; when no typed session exists (legacy rows) the
// first hit is used. "" (nil error) means the team has no session at all.
func (d *SpiritDelivery) resolveTeamMainSessionID(ctx context.Context, teamID string) (string, error) {
	result, err := d.sessionUC.Search(ctx, SessionSearchQuery{TeamID: teamID, Limit: 10})
	if err != nil {
		return "", err
	}
	if len(result.Items) == 0 {
		return "", nil
	}
	for _, s := range result.Items {
		if s.SessionType == string(SessionTypeTeam) {
			return s.ID, nil
		}
	}
	return result.Items[0].ID, nil
}

// extractTeamFullOutput resolves the team main session and returns its final
// deliverable content WITHOUT truncation. Shared by ExtractTeamOutput
// (summary view) and ReadUpstreamDeliverable (full-text tool).
func (d *SpiritDelivery) extractTeamFullOutput(ctx context.Context, teamID string) (teamFullOutput, error) {
	teamSessionID, searchErr := d.resolveTeamMainSessionID(ctx, teamID)
	if searchErr != nil {
		d.lg.Warn("搜索团队 session 失败",
			loggateway.StepID("spirit.extract_output.search_err"),
			loggateway.Str("team_id", teamID),
			loggateway.Err(searchErr),
		)
		return teamFullOutput{}, searchErr
	}
	if teamSessionID == "" {
		return teamFullOutput{}, nil
	}

	if d.stepReader != nil {
		steps, stepErr := d.stepReader.ListStepsBySessionID(ctx, teamSessionID)
		if stepErr != nil {
			d.lg.Warn("获取团队步骤失败",
				loggateway.StepID("spirit.extract_output.step_err"),
				loggateway.Str("team_id", teamID),
				loggateway.Err(stepErr),
			)
			return teamFullOutput{}, stepErr
		}
		for i := len(steps) - 1; i >= 0; i-- {
			st := steps[i]
			if st.Kind != StepKindReply || st.Status != StepStatusCompleted {
				continue
			}
			content := strings.TrimSpace(st.Content)
			if content == "" {
				continue
			}
			return teamFullOutput{Content: content, SessionID: teamSessionID}, nil
		}
	}

	messages, msgErr := d.sessionUC.ListMessagesRecent(ctx, teamSessionID, SpiritRecentMessageCount)
	if msgErr != nil {
		d.lg.Warn("获取团队消息失败",
			loggateway.StepID("spirit.extract_output.msg_err"),
			loggateway.Str("team_id", teamID),
			loggateway.Err(msgErr),
		)
		return teamFullOutput{}, msgErr
	}
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "assistant" {
			return teamFullOutput{Content: messages[i].ContentMarkdown, SessionID: teamSessionID}, nil
		}
	}
	return teamFullOutput{}, nil
}

// ListFailedTeamBriefs collects honest failure briefs for the synthesis
// summary trigger (2026-07-25 Fix 3). Only genuinely failed teams are
// included — cancelled teams are a user abort and skip the report entirely
// (checkAllTeamsCompleted returns early when CancelledTeams > 0). Reason
// comes from latest-run stats; LastReply carries the team's final reply,
// which is where its unresolved questions live. Best-effort: missing pieces
// degrade to fallback text rather than failing the whole collection.
func (d *SpiritDelivery) ListFailedTeamBriefs(ctx context.Context, spiritSessionID string) []TeamFailureBrief {
	teams, err := d.teamUC.ListBySpiritSessionID(ctx, spiritSessionID)
	if err != nil {
		d.lg.Warn("查询精灵会话团队列表失败，跳过失败简报收集",
			loggateway.StepID("spirit.teams.failure_briefs_err"),
			loggateway.Str("spirit_session_id", spiritSessionID),
			loggateway.Err(err),
		)
		return nil
	}
	var failed []Team
	for _, t := range teams {
		if t.Status == TeamStatusFailed {
			failed = append(failed, t)
		}
	}
	if len(failed) == 0 {
		return nil
	}
	teamIDs := make([]string, 0, len(failed))
	for _, t := range failed {
		teamIDs = append(teamIDs, t.ID)
	}
	stats := d.ListTeamRunStats(ctx, teamIDs)
	briefs := make([]TeamFailureBrief, 0, len(failed))
	for _, t := range failed {
		brief := TeamFailureBrief{TeamName: t.DisplayName, TaskName: t.TaskDescription}
		if s, ok := stats[t.ID]; ok {
			brief.Reason = s.ErrorMessage
		}
		if brief.Reason == "" {
			brief.Reason = "任务失败（无详细错误记录）"
		}
		if summary, _, extErr := d.ExtractTeamOutput(ctx, t.ID); extErr == nil {
			brief.LastReply = summary
		}
		briefs = append(briefs, brief)
	}
	return briefs
}

// ListTeamDeliverableDigests collects per-terminal-team deliverable summaries
// for the synthesis trigger (F7, Phase 11). Completed AND failed teams are
// included (failed teams render with an empty summary → 「无交付物」), so the
// Spirit LLM composes the final report from real structured outputs instead
// of excavating session history. Non-terminal teams and read failures are
// skipped (best effort — the trigger must never fail on digest collection).
// Domain: Delivery — per-team deliverable digest assembly.
func (d *SpiritDelivery) ListTeamDeliverableDigests(ctx context.Context, spiritSessionID string) []TeamDeliverableDigest {
	teams, err := d.teamUC.ListBySpiritSessionID(ctx, spiritSessionID)
	if err != nil {
		d.lg.Warn("查询精灵会话团队列表失败，跳过交付物摘要收集",
			loggateway.StepID("spirit.teams.deliverable_digests_err"),
			loggateway.Str("spirit_session_id", spiritSessionID),
			loggateway.Err(err),
		)
		return nil
	}
	digests := make([]TeamDeliverableDigest, 0, len(teams))
	for _, t := range teams {
		if t.Status != TeamStatusCompleted && t.Status != TeamStatusFailed {
			continue
		}
		digest := TeamDeliverableDigest{
			TeamName: t.DisplayName,
			TaskName: t.TaskDescription,
			Status:   t.Status,
		}
		refs := ParseDeliverableRefs(t.DeliverablesOutput)
		if len(refs) > 0 {
			keys := make([]string, 0, len(refs))
			for k := range refs {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			summaries := make([]string, 0, len(keys))
			for _, k := range keys {
				if s := strings.TrimSpace(refs[k].Summary); s != "" {
					summaries = append(summaries, s)
				}
				for _, art := range refs[k].Artifacts {
					digest.Artifacts = append(digest.Artifacts, d.artifactDigestLine(art))
					if ref, ok := deliverableArtifactRefOf(art); ok {
						digest.ArtifactRefs = append(digest.ArtifactRefs, ref)
					}
				}
			}
			digest.DeliverableSummary = strings.Join(summaries, "\n")
		}
		digests = append(digests, digest)
	}
	return digests
}

// ValidateDeliverableContracts validates deliverable contracts between
// upstream and downstream teams in the DAG. Returns a list of warnings
// for contract mismatches. Called after Team DAG is built.
// Domain: Delivery — validate deliverable contracts between upstream and downstream teams.
func (d *SpiritDelivery) ValidateDeliverableContracts(ctx context.Context, spiritSessionID string) []string {
	if d.contractValidator == nil {
		return nil
	}
	teams, err := d.teamUC.ListBySpiritSessionID(ctx, spiritSessionID)
	if err != nil {
		d.lg.Warn("查询团队列表失败，跳过交付物合约校验",
			loggateway.StepID("spirit.contract_validate.list_err"),
			loggateway.Str("spirit_session_id", spiritSessionID),
			loggateway.Err(err),
		)
		return nil
	}

	teamByDagNode := make(map[string]Team, len(teams))
	for _, t := range teams {
		if t.DagNodeID != "" {
			teamByDagNode[t.DagNodeID] = t
		}
	}

	var allWarnings []string
	for _, t := range teams {
		if len(t.DependsOn) == 0 {
			continue
		}
		downstreamContracts, parseErr := ParseDeliverableContracts(t.InputContract)
		if parseErr != nil || len(downstreamContracts) == 0 {
			continue
		}
		// Collect upstream deliverables from all dependency teams
		var upstreamContracts []DeliverableContract
		for _, depID := range t.DependsOn {
			upstream, ok := teamByDagNode[depID]
			if !ok {
				continue
			}
			upContracts, parseErr := ParseDeliverableContracts(upstream.Deliverables)
			if parseErr != nil {
				continue
			}
			upstreamContracts = append(upstreamContracts, upContracts...)
		}
		if len(upstreamContracts) == 0 {
			continue
		}
		warnings := d.contractValidator.ValidateContractMatch(upstreamContracts, downstreamContracts)
		if len(warnings) > 0 {
			d.lg.Info("交付物合约校验发现不匹配",
				loggateway.StepID("spirit.contract_validate.mismatch"),
				loggateway.Str("team_id", t.ID),
				loggateway.Int("warning_count", len(warnings)),
			)
			allWarnings = append(allWarnings, warnings...)
		}
	}
	return allWarnings
}

// ExecuteVerificationGates runs all verification gates for a team's output.
// Returns (approved bool, warnings []string, err error).
// If any gate rejects, the whole verification fails.
// Domain: Delivery — execute verification gates on team output.
func (d *SpiritDelivery) ExecuteVerificationGates(ctx context.Context, teamID string, teamOutput string) (bool, []string, error) {
	if d.gateExecutor == nil {
		return true, nil, nil
	}
	t, err := d.teamUC.Get(ctx, teamID)
	if err != nil {
		return false, nil, err
	}

	gates, err := d.resolveVerificationGates(ctx, t)
	if err != nil || len(gates) == 0 {
		return true, nil, nil
	}

	truncateChars := 0
	if d.agentUC != nil {
		agents, listErr := d.agentUC.List(ctx, AgentListQuery{Keyword: SpiritAgentKey, Limit: SpiritAgentQueryLimit})
		if listErr == nil && len(agents.Items) > 0 && agents.Items[0].Settings != nil {
			truncateChars = agents.Items[0].Settings.VerificationTruncateChars
		}
	}

	var allWarnings []string
	for _, gate := range gates {
		if gate.MaxRetries <= 0 {
			gate.MaxRetries = 3
		}
		approved, reason, _, gateErr := d.gateExecutor.ExecuteGateScoped(ctx, teamID, gate, teamOutput, truncateChars)
		if gateErr != nil {
			return false, allWarnings, gateErr
		}
		if !approved {
			allWarnings = append(allWarnings, fmt.Sprintf("gate %s rejected: %s", gate.GateType, reason))
			return false, allWarnings, nil
		}
		allWarnings = append(allWarnings, fmt.Sprintf("gate %s approved: %s", gate.GateType, reason))
	}

	if invalidated := d.gateExecutor.InvalidatedReceipts(teamID); len(invalidated) > 0 {
		d.lg.Warn("验证门回执写后失效，terminal 前全量重验",
			loggateway.StepID("spirit.verification_gate.revalidate"),
			loggateway.Str("team_id", teamID),
			loggateway.Int("invalidated_count", len(invalidated)),
		)
		for _, gate := range gates {
			approved, reason, _, gateErr := d.gateExecutor.ExecuteGateScoped(ctx, teamID, gate, teamOutput, truncateChars)
			if gateErr != nil {
				return false, allWarnings, gateErr
			}
			if !approved {
				allWarnings = append(allWarnings, fmt.Sprintf("gate %s rejected on revalidation: %s", gate.GateType, reason))
				return false, allWarnings, nil
			}
			allWarnings = append(allWarnings, fmt.Sprintf("gate %s revalidated: %s", gate.GateType, reason))
		}
	}
	return true, allWarnings, nil
}

// resolveVerificationGates finds verification gates for a team.
// Domain: Delivery — resolve verification gates from team definition.
func (d *SpiritDelivery) resolveVerificationGates(ctx context.Context, t Team) ([]VerificationGate, error) {
	// Check if the team has verification gates in its definition JSON
	// or if the linked graph has verification gates
	// For now, parse from the team's DefinitionJSON if it contains a verification_gates field
	type defWithGates struct {
		VerificationGates []VerificationGate `json:"verification_gates"`
	}
	var def defWithGates
	if err := json.Unmarshal([]byte(t.DefinitionJSON), &def); err == nil && len(def.VerificationGates) > 0 {
		return def.VerificationGates, nil
	}
	return nil, nil
}

// ErrNoRealDeliverable marks a DAG team that has no graph-state deliverable —
// set_deliverable was never called (or the state is unreadable). Reply text
// must NOT be dressed up as a deliverable: a team that only asked clarifying
// questions produced nothing, and persisting its reply as a "deliverable" is
// the root of the 19:29 false-success chain (2026-07-25 Fix 1).
var ErrNoRealDeliverable = errors.New("no real deliverable: set_deliverable was never called")
