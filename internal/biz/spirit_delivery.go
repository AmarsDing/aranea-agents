package biz

import (
	"aranea-agents/internal/biz/session"
	"aranea-agents/internal/biz/shared"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/ctxuser"
	"aranea-agents/pkg/loggateway"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"sort"
	"strings"
	"time"
	"unicode/utf8"
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

// WriteDeliverablesToSession persists the team's REAL deliverable — the graph
// final-state "deliverable" map written via set_deliverable — as a P2
// DeliverableRef envelope in the Team's DeliverablesOutput field (JSON object
// keyed by dag_node_id) so downstream teams can consume it via
// InjectUpstreamDeliverables and retrieve the full text on demand via
// read_upstream_deliverable.
//
// 2026-07-25 Fix 1: the ONLY source is the graph state deliverable. Reply
// text is never consulted — a team that produced no state deliverable gets
// ErrNoRealDeliverable and no envelope write (the service-layer gate flips
// such teams to failed before this is normally called; RecordTeamCompletion
// keeps a quiet second line of defense).
//
// Domain: Delivery — write upstream team deliverables to team record for downstream consumption.
func (d *SpiritDelivery) WriteDeliverablesToSession(ctx context.Context, teamID string) error {
	t, err := d.teamUC.Get(ctx, teamID)
	if err != nil {
		return err
	}
	if t.DagNodeID == "" {
		return nil
	}

	anchor, ok := d.stateDeliverableChannel(t)
	if !ok {
		return ErrNoRealDeliverable
	}
	teamSessionID, err := d.resolveTeamMainSessionID(ctx, t.ID)
	if err != nil {
		return err
	}
	if teamSessionID == "" {
		return ErrNoRealDeliverable
	}
	stateDeliv, err := d.graphDelivReader.ReadGraphDeliverable(ctx, anchor, ctxuser.TRPCUserKey(ctx), teamSessionID)
	if err != nil {
		d.lg.Warn("graph state deliverable 读取失败，按无真实交付物处理",
			loggateway.StepID("spirit.write_deliverables"),
			loggateway.Str("team_id", teamID),
			loggateway.Err(err),
		)
		return ErrNoRealDeliverable
	}

	seed, serr := d.UpstreamDeliverableSeed(ctx, t)
	if serr != nil {
		d.lg.Warn("上游种子重算失败，按无真实交付物处理",
			loggateway.StepID("spirit.write_deliverables"),
			loggateway.Str("team_id", teamID),
			loggateway.Err(serr),
		)
		return ErrNoRealDeliverable
	}
	stateDeliv = subtractUpstreamSeed(stateDeliv, seed)
	if len(stateDeliv) == 0 {
		return ErrNoRealDeliverable
	}

	d.bridgeDeliverableDocs(ctx, t, stateDeliv)

	summarySource, _ := stateDeliv[deliverableReservedKeySummary].(string)
	summarySource = strings.TrimSpace(summarySource)
	artifacts := buildDeliverableArtifacts(stateDeliv, t.Deliverables)
	structuredJSON := marshalEnvelopeStructuredJSON(stateDeliv, artifacts)
	if summarySource == "" {
		summarySource = aggregateTopicSummaries(stateDeliv)
	}
	if summarySource == "" {
		summarySource = structuredJSON
	}
	cognition := extractStateCognition(stateDeliv[deliverableReservedKeyCognition])

	if missing := requiredTopicsMissingFromState(t, stateDeliv); len(missing) > 0 {
		d.lg.Warn("成员交付物契约 required topic 未产出",
			loggateway.StepID("spirit.write_deliverables"),
			loggateway.Str("team_id", teamID),
			loggateway.Str("missing_topics", strings.Join(missing, ",")),
		)
	}

	sizeChars := utf8.RuneCountInString(summarySource)
	ref := DeliverableRef{
		Summary:        TruncateRunes(summarySource, MaxSummaryLen),
		KeyFindings:    extractKeyFindings(summarySource),
		TeamID:         t.ID,
		TeamSessionID:  teamSessionID,
		SizeChars:      sizeChars,
		Truncated:      sizeChars > MaxSummaryLen,
		StructuredJSON: structuredJSON,
		Cognition:      cognition,
		DerivedFrom:    t.DependsOn,
		Artifacts:      artifacts,
	}
	refJSON, marshalErr := json.Marshal(ref)
	if marshalErr != nil {
		return marshalErr
	}

	outputs := make(map[string]json.RawMessage)
	if t.DeliverablesOutput != "" && t.DeliverablesOutput != "{}" {

		if uerr := json.Unmarshal([]byte(t.DeliverablesOutput), &outputs); uerr != nil {
			d.lg.Warn("交付物输出缓存 JSON 损坏，重建缓存",
				loggateway.StepID("spirit.write_deliverables"),
				loggateway.Str("team_id", teamID),
				loggateway.Err(uerr),
			)
			outputs = make(map[string]json.RawMessage)
		}
	}
	outputs[t.DagNodeID] = refJSON
	updatedJSON, marshalErr := json.Marshal(outputs)
	if marshalErr != nil {
		return marshalErr
	}

	_, err = d.teamUC.Update(ctx, t.ID, Team{DeliverablesOutput: string(updatedJSON)})
	if err != nil {
		d.lg.Warn("持久化交付物输出失败",
			loggateway.StepID("spirit.write_deliverables"),
			loggateway.Str("team_id", teamID),
			loggateway.Err(err),
		)
		return err
	}

	if d.gateExecutor != nil {
		d.gateExecutor.RecordScopeWrite(t.ID, "deliverables_output")
	}
	d.lg.Info("团队交付物已落库，可供下游团队消费",
		loggateway.StepID("spirit.write_deliverables"),
		loggateway.Str("team_id", teamID),
		loggateway.Str("dag_node_id", t.DagNodeID),
		loggateway.Int("size_chars", sizeChars),
		loggateway.Bool("truncated", ref.Truncated),
	)
	return nil
}

// HasRealDeliverable reports whether the team produced a REAL deliverable —
// a non-empty graph-state "deliverable" map written via set_deliverable.
// Reply text does not count. (false, nil) covers every "no deliverable"
// shape (non-DAG team, channel disabled, no session, empty state);
// (false, err) is reserved for infra failures so the caller can distinguish
// "did not produce" from "could not verify" (2026-07-25 Fix 1 gate).
func (d *SpiritDelivery) HasRealDeliverable(ctx context.Context, team Team) (bool, error) {
	if team.DagNodeID == "" {
		return false, nil
	}
	anchor, ok := d.stateDeliverableChannel(team)
	if !ok {
		return false, nil
	}
	teamSessionID, err := d.resolveTeamMainSessionID(ctx, team.ID)
	if err != nil {
		return false, err
	}
	if teamSessionID == "" {
		return false, nil
	}
	stateDeliv, err := d.graphDelivReader.ReadGraphDeliverable(ctx, anchor, ctxuser.TRPCUserKey(ctx), teamSessionID)
	if err != nil {
		return false, err
	}

	seed, serr := d.UpstreamDeliverableSeed(ctx, team)
	if serr != nil {
		return false, serr
	}
	return len(subtractUpstreamSeed(stateDeliv, seed)) > 0, nil
}

// MemberExecutionEvidence implements F10 (Phase 11) outcome-oriented member
// status: inspects a member session's execution evidence and reports whether
// the member FAILED — regardless of what the team-level callback claims.
// Member status must follow the execution RESULT, not the message lifecycle
// (12:33: members returning text were shown as successful even when the
// underlying work failed).
//
// Evidence sources (first hit wins):
//  1. Session interrupted → failed (StatusReason explains why)
//  2. Any failed/cancelled step → failed (first such step summarized)
//
// Read failures count as "no evidence" (conservative): an infra read error
// must never flip a member to failed — systemic failures are already carried
// by the team-level status. Returns (false, "") when no failure evidence.
//
// P7（2026-08-21 统一修复）：交付事实优先。任一证据源（成员会话 steps ∪
// 团队会话回扫）确认该成员存在 completed 的 set_deliverable action ⇒
// 交付物已落库（工具 completed 即 graph 写入成功；契约违规会被工具拒绝 →
// action failed，不构成事实），其过程性异常（session interrupted、
// reply/error/action 的 failed|cancelled）一律不作为成员失败证据。
// 交付物内容是否达标由质量门 J2/J3 独立判定，与成员过程健康解耦。
// 事实无法确认时保持原有保守语义：interrupted / fatal step 仍判失败。
func (d *SpiritDelivery) MemberExecutionEvidence(ctx context.Context, sessionID string) (bool, string) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return false, ""
	}
	sess, err := d.sessionUC.Get(ctx, sessionID)
	if err != nil {
		d.lg.Warn("成员执行证据：读取成员 session 失败，按无证据处理",
			loggateway.StepID("spirit.member_evidence.session_err"),
			loggateway.Str("session_id", sessionID),
			loggateway.Err(err),
		)
		return false, ""
	}
	interrupted := sess.Status == string(session.SessionStatusInterrupted)
	interruptReason := strings.TrimSpace(sess.StatusReason)
	if interruptReason == "" {
		interruptReason = string(session.SessionStatusInterrupted)
	}
	if d.stepReader == nil {

		if interrupted {
			return true, "session interrupted: " + interruptReason
		}
		return false, ""
	}

	steps, err := d.stepReader.ListStepsBySessionID(ctx, sessionID)
	if err != nil {
		d.lg.Warn("成员执行证据：读取成员 steps 失败，按无证据处理",
			loggateway.StepID("spirit.member_evidence.steps_err"),
			loggateway.Str("session_id", sessionID),
			loggateway.Err(err),
		)

		if interrupted {
			return true, "session interrupted: " + interruptReason
		}
		return false, ""
	}

	parentID := strings.TrimSpace(sess.ParentSessionID)
	memberKey := strings.TrimSpace(sess.MemberAgentKey)
	backscan := parentID != "" && parentID != sessionID && memberKey != ""
	var teamSteps []Step
	if backscan {
		var terr error
		teamSteps, terr = d.stepReader.ListStepsBySessionID(ctx, parentID)
		if terr != nil {

			d.lg.Warn("成员执行证据：读取团队会话 steps 失败，团队会话证据按空处理",
				loggateway.StepID("spirit.member_evidence.team_steps_err"),
				loggateway.Str("team_session_id", parentID),
				loggateway.Err(terr),
			)
			teamSteps = nil
		}
	}

	delivered := false
	for _, st := range steps {
		if st.Kind == StepKindAction && st.Status == StepStatusCompleted && st.ToolName == "set_deliverable" {
			delivered = true
			break
		}
	}
	if !delivered {
		for _, st := range teamSteps {
			if strings.TrimSpace(st.AuthorAgentKey) != memberKey {
				continue
			}
			if st.Kind == StepKindAction && st.Status == StepStatusCompleted && st.ToolName == "set_deliverable" {
				delivered = true
				break
			}
		}
	}
	if delivered {

		return false, ""
	}

	if interrupted {
		return true, "session interrupted: " + interruptReason
	}
	for _, st := range steps {
		if st.Status != StepStatusFailed && st.Status != StepStatusCancelled {
			continue
		}
		summary := strings.TrimSpace(st.Content)
		if summary == "" {
			summary = strings.TrimSpace(st.ToolErrorCode)
		}
		if summary == "" {
			summary = string(st.Kind)
		}
		return true, fmt.Sprintf("step %s: %s", st.Status, truncateRunes(summary, memberEvidenceSummaryMaxRunes))
	}
	for _, st := range teamSteps {
		if strings.TrimSpace(st.AuthorAgentKey) != memberKey {
			continue
		}
		fatal := st.Status == StepStatusFailed || st.Status == StepStatusCancelled || st.Kind == StepKindError
		if !fatal {
			continue
		}
		summary := strings.TrimSpace(st.Content)
		if summary == "" {
			summary = strings.TrimSpace(st.ToolErrorCode)
		}
		if summary == "" {
			summary = string(st.Kind)
		}
		return true, fmt.Sprintf("team step %s/%s: %s", st.Kind, st.Status, truncateRunes(summary, memberEvidenceSummaryMaxRunes))
	}
	return false, ""
}

// MemberExecutionWindow implements the member-duration step-stream aggregation
// (2026-08-08 问题4): a member's real execution window is aggregated from the
// steps it owns — start = earliest StartedAt, end = latest activity evidence
// (CompletedAt when set, StartedAt otherwise). Lookup mirrors
// MemberExecutionEvidence — member session steps first; when the member
// session has no steps (coordinator mode lands member steps on the TEAM
// session), fall back to team-session steps filtered by
// AuthorAgentKey == MemberAgentKey.
//
// Read failures and empty results both yield ok=false (conservative): the
// caller then falls back to the publish timestamp. Steps with a zero
// StartedAt are ignored for the start so a malformed row cannot pull the
// window to the zero time; likewise end only considers non-zero evidence.
func (d *SpiritDelivery) MemberExecutionWindow(ctx context.Context, sessionID string) (time.Time, time.Time, bool) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || d.stepReader == nil {
		return time.Time{}, time.Time{}, false
	}
	sess, err := d.sessionUC.Get(ctx, sessionID)
	if err != nil {
		d.lg.Warn("成员执行窗口：读取成员 session 失败，按无窗口处理",
			loggateway.StepID("spirit.member_window.session_err"),
			loggateway.Str("session_id", sessionID),
			loggateway.Err(err),
		)
		return time.Time{}, time.Time{}, false
	}
	if steps, err := d.stepReader.ListStepsBySessionID(ctx, sessionID); err != nil {
		d.lg.Warn("成员执行窗口：读取成员 steps 失败，按无窗口处理",
			loggateway.StepID("spirit.member_window.steps_err"),
			loggateway.Str("session_id", sessionID),
			loggateway.Err(err),
		)
		return time.Time{}, time.Time{}, false
	} else if start, end, ok := memberStepWindow(steps, ""); ok {
		return start, end, true
	}

	parentID := strings.TrimSpace(sess.ParentSessionID)
	memberKey := strings.TrimSpace(sess.MemberAgentKey)
	if parentID == "" || parentID == sessionID || memberKey == "" {
		return time.Time{}, time.Time{}, false
	}
	teamSteps, terr := d.stepReader.ListStepsBySessionID(ctx, parentID)
	if terr != nil {
		d.lg.Warn("成员执行窗口：读取团队会话 steps 失败，按无窗口处理",
			loggateway.StepID("spirit.member_window.team_steps_err"),
			loggateway.Str("team_session_id", parentID),
			loggateway.Err(terr),
		)
		return time.Time{}, time.Time{}, false
	}
	return memberStepWindow(teamSteps, memberKey)
}

// stateDeliverableChannel resolves the graph-state deliverable channel
// coordinates for a team. ok=false means no real deliverable can exist:
// no reader wired, no/undecodable DefinitionJSON, enable_state_deliverable
// off, or no anchor agent. The anchor mirrors the runner's AppName decision
// (stateDeliverableProbe.anchorAgentID) so the state is read under the same
// session key the run persisted to.
func (d *SpiritDelivery) stateDeliverableChannel(t Team) (anchor string, ok bool) {
	if d.graphDelivReader == nil || strings.TrimSpace(t.DefinitionJSON) == "" {
		return "", false
	}
	var probe stateDeliverableProbe
	if err := json.Unmarshal([]byte(t.DefinitionJSON), &probe); err != nil || !probe.EnableStateDeliverable {
		return "", false
	}
	if anchor = probe.anchorAgentID(); anchor == "" {
		return "", false
	}
	return anchor, true
}

// UpstreamDeliverableSeed returns the cross-team deliverable seed for a
// downstream DAG team (2026-08-08 问题3 修复): the business topics of every
// COMPLETED upstream dependency's graph deliverable map, merged in DependsOn
// order (later deps win on topic collision, Warn-logged). Reserved keys
// (summary/cognition) and intra-team ack/* keys are never seeded — the
// downstream team's own summary must come from its own members, and ack
// signals are intra-team coordination, not content.
//
// The seed is installed into the downstream graph's initial deliverable
// state at turn start (Runner RuntimeState injection), so members can
// get_deliverable(topic=...) the upstream contract topics directly — the
// per-execution graph state is otherwise isolated, which was the 01:54 断链
// root cause (downstream get_deliverable always returned found=false).
//
// HasRealDeliverable / WriteDeliverablesToSession recompute and subtract
// this same seed (subtractUpstreamSeed): seed round-trip alone can never
// satisfy the real-deliverable gate, and upstream topics never leak into
// this team's own envelope.
//
// Conservative on infra failure: an unreadable upstream state skips that
// dependency (Warn) — the text injection prefix + read_upstream_deliverable
// tool remain as fallback. Domain: Delivery — cross-team state handoff.
func (d *SpiritDelivery) UpstreamDeliverableSeed(ctx context.Context, downstreamTeam Team) (map[string]any, error) {
	if len(downstreamTeam.DependsOn) == 0 || d.graphDelivReader == nil {
		return nil, nil
	}
	teams, err := d.teamUC.ListBySpiritSessionID(ctx, downstreamTeam.SpiritSessionID)
	if err != nil {
		return nil, err
	}
	teamByDagNode := make(map[string]Team, len(teams))
	for _, t := range teams {
		if t.DagNodeID != "" {
			teamByDagNode[t.DagNodeID] = t
		}
	}
	var seed map[string]any
	for _, depID := range downstreamTeam.DependsOn {
		upstream, ok := teamByDagNode[depID]
		if !ok || upstream.Status != TeamStatusCompleted {
			continue
		}
		anchor, ok := d.stateDeliverableChannel(upstream)
		if !ok {
			continue
		}
		sessID, serr := d.resolveTeamMainSessionID(ctx, upstream.ID)
		if serr != nil {
			d.lg.Warn("上游交付物种子：解析上游团队 session 失败，跳过该依赖",
				loggateway.StepID("spirit.upstream_seed"),
				loggateway.Str("upstream_team_id", upstream.ID),
				loggateway.Err(serr),
			)
			continue
		}
		if sessID == "" {
			continue
		}
		m, rerr := d.graphDelivReader.ReadGraphDeliverable(ctx, anchor, ctxuser.TRPCUserKey(ctx), sessID)
		if rerr != nil {
			d.lg.Warn("上游交付物种子：读取上游 graph state 失败，跳过该依赖",
				loggateway.StepID("spirit.upstream_seed"),
				loggateway.Str("upstream_team_id", upstream.ID),
				loggateway.Err(rerr),
			)
			continue
		}
		for k, v := range m {
			if k == deliverableReservedKeySummary || k == deliverableReservedKeyCognition || strings.HasPrefix(k, deliverableAckKeyPrefix) {
				continue
			}
			if seed == nil {
				seed = make(map[string]any, len(m))
			}
			if _, dup := seed[k]; dup {
				d.lg.Warn("上游交付物 topic 冲突，按 DependsOn 顺序后者覆盖",
					loggateway.StepID("spirit.upstream_seed"),
					loggateway.Str("topic", k),
					loggateway.Str("upstream_team_id", upstream.ID),
				)
			}
			seed[k] = v
		}
	}
	return seed, nil
}

// InjectUpstreamDeliverables collects upstream team deliverables and formats
// them as a prefix for the downstream team's input message.
// Called when a DAG activates a downstream team.
// It first tries to read from the persisted deliverable output cache
// (written by WriteDeliverablesToSession), then falls back to
// extracting from the team output directly.
// Domain: Delivery — collect and format upstream deliverables for downstream team input.
func (d *SpiritDelivery) InjectUpstreamDeliverables(ctx context.Context, downstreamTeam Team) string {
	if len(downstreamTeam.DependsOn) == 0 {
		return ""
	}
	teams, err := d.teamUC.ListBySpiritSessionID(ctx, downstreamTeam.SpiritSessionID)
	if err != nil {
		return ""
	}

	teamByDagNode := make(map[string]Team, len(teams))
	for _, t := range teams {
		if t.DagNodeID != "" {
			teamByDagNode[t.DagNodeID] = t
		}
	}

	var deliverableParts []string
	for _, depID := range downstreamTeam.DependsOn {
		upstream, ok := teamByDagNode[depID]
		if !ok || upstream.Status != TeamStatusCompleted {
			continue
		}

		ref, refOK := d.readDeliverableRef(upstream)
		if !refOK || ref.Summary == "" {
			continue
		}

		if ref.TeamID == "" {
			ref.TeamID = upstream.ID
		}
		part := fmt.Sprintf("## 上游团队: %s\n%s%s", upstream.DisplayName, contractDeclarationLines(upstream.Deliverables), ref.Summary)
		part += renderArtifactSections(ref, downstreamTeam)
		if cog := renderCognitionLines(ref.Cognition); cog != "" {
			part += "\n" + cog
		}
		if ref.Truncated {
			part += fmt.Sprintf("\n[交付物全文共 %d 字符，以上为截断摘要。需要完整内容时调用 read_upstream_deliverable(team_id=\"%s\") 获取]",
				ref.SizeChars, ref.TeamID)
		}
		deliverableParts = append(deliverableParts, part)
	}

	if len(deliverableParts) == 0 {
		return ""
	}

	return fmt.Sprintf("--- 上游交付物 ---\n%s\n--- 请基于以上上游交付物执行任务 ---\n\n",
		strings.Join(deliverableParts, "\n\n"))
}

// DeliverableProtocolSuffix renders the mandatory delivery-protocol block
// appended to a DAG team's first-turn input (2026-07-25 Fix 2b). Without it a
// team has no way of knowing that "reply text is not a deliverable": the
// real-deliverable gate (Fix 1) flips completed-without-set_deliverable teams
// to failed, so the obligation must be declared up front. Non-DAG teams carry
// no deliverable obligation and get no suffix.
func (d *SpiritDelivery) DeliverableProtocolSuffix(t Team) string {
	if t.DagNodeID == "" {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("\n--- 交付协议（强制） ---\n")
	sb.WriteString("本团队是 DAG 编排节点。任务完成前必须调用 set_deliverable 提交结构化交付物（只在回复文本中给出内容不算产出）。交付范式：摘要是指针，载荷是数据。\n")
	sb.WriteString("- \"summary\"（保留 key，必填）：交付物摘要，供下游团队与最终报告消费。固定写法：①一句话结论 ②3-5 条 \"- \" 要点 ③末行载荷清单（载荷：<topic>(<format>, <n>字)）。注意：摘要超过 500 字将被截断——正文严禁写进 summary，必须放入契约 topic 的 content 字段。\n")
	if contracts := contractSubmissionLines(t.Deliverables); contracts != "" {
		sb.WriteString("- 交付契约（逐项按指定 topic 提交，禁止自创 topic 名）：\n")
		sb.WriteString(contracts)
	}
	sb.WriteString("- 文档型交付物（报告/文章/复盘等长文）载荷固定结构：{\"title\": \"标题\", \"format\": \"markdown\", \"content\": \"完整正文\", \"meta\": {...}}——content 承载全文，不会被截断。\n")
	sb.WriteString("样例 1（短交付物，内容可全部写进摘要）：\n")
	sb.WriteString("  set_deliverable(data={\"summary\": \"完成服务器巡检，整体健康。\\n- CPU/内存正常\\n- 磁盘使用率 85%，建议扩容\\n载荷：无（结论即全文）\"})\n")
	sb.WriteString("样例 2（长文交付物，正文放契约 topic，摘要只写指针。注意顺序：先写摘要、后写 topic 载荷——不带 topic 的写入会整体覆盖本节点本地视图，topic 载荷写在最后可保住本地读回）：\n")
	sb.WriteString("  set_deliverable(data={\"summary\": \"《云计算十年》已完成并复核。\\n- 观点：云原生进入平台工程阶段\\n- 数据：引用 12 组行业数据\\n载荷：article(markdown, 8234字)\"})\n")
	sb.WriteString("  set_deliverable(topic=\"article\", data={\"title\": \"云计算十年\", \"format\": \"markdown\", \"content\": \"# 云计算十年\\n……（完整正文，8234 字）\"})\n")
	sb.WriteString("- Brief：summary 必填（一句话结论 + 要点）。大文件/二进制禁止写入 content 指望下游整段注入；把路径或制品列入同一 topic（rel_path / artifact_id），下游工作区 inbox/<上游团队ID>/ 仅含声明附件。\n")
	sb.WriteString("未调用 set_deliverable 的 completed 将被判定为 failed，下游团队不会收到输入；信息不足时在 summary 中如实写明阻塞或待澄清事项，禁止虚构交付物。\n")
	return sb.String()
}

// BuildTeamTurnInput composes a DAG team's first-turn input:
// upstream deliverable prefix + task description + delivery protocol suffix.
// The stored Team.TaskDescription stays pure — injection happens only on the
// turn input (both the orchestrator dispatch path and the lazy DAG activation
// path compose through this single function so the two cannot drift).
func (d *SpiritDelivery) BuildTeamTurnInput(ctx context.Context, t Team) string {
	d.materializeUpstreamInbox(ctx, t)
	taskDesc := t.TaskDescription
	prefix := d.InjectUpstreamDeliverables(ctx, t)
	protocol := d.DeliverableProtocolSuffix(t)

	trimmed := TrimPrefixBudget(PrefixBudget{Brief: prefix, Protocol: protocol})
	if trimmed.Brief != "" {
		taskDesc = trimmed.Brief + taskDesc
	}
	return taskDesc + trimmed.Protocol
}

// readDeliverableRef reads the persisted DeliverableRef for the team's own
// dag_node_id. Dual-mode: P2 envelopes parse with full metadata; legacy
// plain-string values yield a summary-only ref. ok=false when absent or
// unparseable.
// Domain: Delivery — read persisted deliverable envelope from team's cache.
func (d *SpiritDelivery) readDeliverableRef(t Team) (DeliverableRef, bool) {
	if t.DagNodeID == "" {
		return DeliverableRef{}, false
	}
	ref, ok := ParseDeliverableRefs(t.DeliverablesOutput)[t.DagNodeID]
	return ref, ok
}

// ReadUpstreamDeliverable returns the full deliverable text of a COMPLETED
// upstream team, truncated to maxChars (default DefaultUpstreamDeliverableMaxChars,
// hard cap MaxUpstreamDeliverableChars). Backs the read_upstream_deliverable
// tool: the injection prefix only carries a truncated summary, and downstream
// team members call the tool when they genuinely need the full text.
//
// 2026-07-25 Fix 7: the content source is the DELIVERABLE itself — graph
// state first, then the persisted envelope — never the reply text. After the
// Fix-1 gate "reply is not a deliverable", a reply-sourced full text would
// contradict the injection prefix (which renders the envelope summary).
//
// Phase B (runtime contract validation): when readerSessionID resolves to a
// reader team with a declared InputContract, that contract is checked against
// the upstream team's declared Deliverables BEFORE the (expensive) full-text
// extraction; a mismatch returns a structured *ContractMismatchError so the
// calling agent can auto-correct and retry.
// Domain: Delivery — full-text retrieval of an upstream team's deliverable.
func (d *SpiritDelivery) ReadUpstreamDeliverable(ctx context.Context, readerSessionID, teamID string, maxChars int) (UpstreamDeliverableContent, error) {
	teamID = strings.TrimSpace(teamID)
	if teamID == "" {
		return UpstreamDeliverableContent{}, apierror.BadRequest("SPIRIT", "team_id is required")
	}
	if maxChars <= 0 || maxChars > MaxUpstreamDeliverableChars {
		maxChars = DefaultUpstreamDeliverableMaxChars
	}
	t, err := d.teamUC.Get(ctx, teamID)
	if err != nil {
		return UpstreamDeliverableContent{}, err
	}
	if t.Status != TeamStatusCompleted {
		return UpstreamDeliverableContent{}, apierror.BadRequest("SPIRIT", "upstream team %s has not completed yet (status=%s)", teamID, t.Status)
	}
	if err := d.validateUpstreamContract(ctx, readerSessionID, t); err != nil {
		return UpstreamDeliverableContent{}, err
	}
	full, err := d.resolveDeliverableFullContent(ctx, t)
	if err != nil {
		return UpstreamDeliverableContent{}, err
	}
	if full.Content == "" {
		return UpstreamDeliverableContent{}, apierror.NotFound("SPIRIT", "no deliverable content found for team %s", teamID)
	}

	if err := d.validateUpstreamContractSchema(ctx, readerSessionID, t, full.Content); err != nil {
		return UpstreamDeliverableContent{}, err
	}

	sizeChars := utf8.RuneCountInString(full.Content)
	content := full.Content
	truncated := sizeChars > maxChars
	if truncated {
		content = TruncateRunes(content, maxChars) + fmt.Sprintf("\n...[已截断，全文共 %d 字符]", sizeChars)
	}
	d.lg.Info("上游交付物全文读取",
		loggateway.StepID("spirit.read_upstream_deliverable"),
		loggateway.Str("team_id", teamID),
		loggateway.Int("size_chars", sizeChars),
		loggateway.Bool("truncated", truncated),
	)
	return UpstreamDeliverableContent{
		Content:   content,
		SizeChars: sizeChars,
		Truncated: truncated,
		TeamID:    teamID,
		SessionID: full.SessionID,
	}, nil
}

// ReadUpstreamDeliverableKey returns the full content of ONE payload key of
// an upstream team's deliverable (document-paradigm payloads render as
// title + content; other shapes as indented JSON). Same completion/contract
// gates as ReadUpstreamDeliverable. The keyed read exists for the long-
// article paradigm: downstream teams fetch exactly the contracted artifact
// instead of the summary+everything concatenation.
// Domain: Delivery — keyed full-text retrieval of an upstream deliverable payload.
func (d *SpiritDelivery) ReadUpstreamDeliverableKey(ctx context.Context, readerSessionID, teamID, key string, maxChars int) (UpstreamDeliverableContent, error) {
	teamID = strings.TrimSpace(teamID)
	key = strings.TrimSpace(key)
	if teamID == "" {
		return UpstreamDeliverableContent{}, apierror.BadRequest("SPIRIT", "team_id is required")
	}
	if key == "" {
		return UpstreamDeliverableContent{}, apierror.BadRequest("SPIRIT", "key is required")
	}
	if key == deliverableReservedKeySummary || key == deliverableReservedKeyCognition || strings.HasPrefix(key, deliverableAckKeyPrefix) {
		return UpstreamDeliverableContent{}, apierror.BadRequest("SPIRIT", "key %q 为保留 key：摘要已在注入前缀中，cognition/ack 不支持按 key 读取", key)
	}
	if maxChars <= 0 || maxChars > MaxUpstreamDeliverableChars {
		maxChars = DefaultUpstreamDeliverableMaxChars
	}
	t, err := d.teamUC.Get(ctx, teamID)
	if err != nil {
		return UpstreamDeliverableContent{}, err
	}
	if t.Status != TeamStatusCompleted {
		return UpstreamDeliverableContent{}, apierror.BadRequest("SPIRIT", "upstream team %s has not completed yet (status=%s)", teamID, t.Status)
	}
	if err := d.validateUpstreamContract(ctx, readerSessionID, t); err != nil {
		return UpstreamDeliverableContent{}, err
	}
	full, available, found, err := d.resolveDeliverableKeyContent(ctx, t, key)
	if err != nil {
		return UpstreamDeliverableContent{}, err
	}
	if full.Content == "" {
		switch {
		case found:
			return UpstreamDeliverableContent{}, apierror.NotFound("SPIRIT", "deliverable key %q 内容为空（team %s）：载荷存在但无正文；或不带 key 调用 read_upstream_deliverable 查看交付物整体", key, teamID)
		case len(available) > 0:
			return UpstreamDeliverableContent{}, apierror.NotFound("SPIRIT", "deliverable key %q not found for team %s（可用 key：%s；或不带 key 调用 read_upstream_deliverable 读取交付物全文）", key, teamID, strings.Join(available, ", "))
		default:
			return UpstreamDeliverableContent{}, apierror.NotFound("SPIRIT", "no deliverable content found for team %s（可不带 key 调用 read_upstream_deliverable 读取交付物全文）", teamID)
		}
	}
	if err := d.validateUpstreamContractSchema(ctx, readerSessionID, t, full.Content); err != nil {
		return UpstreamDeliverableContent{}, err
	}

	sizeChars := utf8.RuneCountInString(full.Content)
	content := full.Content
	truncated := sizeChars > maxChars
	if truncated {
		content = TruncateRunes(content, maxChars) + fmt.Sprintf("\n...[已截断，全文共 %d 字符]", sizeChars)
	}
	d.lg.Info("上游交付物按 key 读取",
		loggateway.StepID("spirit.read_upstream_deliverable_key"),
		loggateway.Str("team_id", teamID),
		loggateway.Str("key", key),
		loggateway.Int("size_chars", sizeChars),
		loggateway.Bool("truncated", truncated),
	)
	return UpstreamDeliverableContent{
		Content:   content,
		SizeChars: sizeChars,
		Truncated: truncated,
		TeamID:    teamID,
		SessionID: full.SessionID,
	}, nil
}

// resolveDeliverableKeyContent resolves one payload key with the same source
// priority as resolveDeliverableFullContent: graph-state re-read first, then
// the persisted envelope. available lists the readable payload keys when a
// source was reachable but the key missing (LLM-actionable NotFound); found
// reports the key existed in a reachable source (its rendered content may
// still be empty — the caller reports that honestly instead of a misleading
// "not found").
func (d *SpiritDelivery) resolveDeliverableKeyContent(ctx context.Context, t Team, key string) (teamFullOutput, []string, bool, error) {
	if t.DagNodeID != "" {
		if out, available, found, ok := d.readGraphStateKeyContent(ctx, t, key); ok {
			return out, available, found, nil
		}
		if out, available, found, ok := d.readEnvelopeKeyContent(t, key); ok {
			return out, available, found, nil
		}
	}
	return teamFullOutput{}, nil, false, nil
}

// readGraphStateKeyContent reads one payload key from the graph final state.
// ok=false means the source is unreadable (caller degrades to the envelope);
// ok=true with found=false means the key is absent — available then lists
// the payload keys present.
func (d *SpiritDelivery) readGraphStateKeyContent(ctx context.Context, t Team, key string) (teamFullOutput, []string, bool, bool) {
	anchor, ok := d.stateDeliverableChannel(t)
	if !ok {
		return teamFullOutput{}, nil, false, false
	}
	teamSessionID, err := d.resolveTeamMainSessionID(ctx, t.ID)
	if err != nil || teamSessionID == "" {
		return teamFullOutput{}, nil, false, false
	}
	stateDeliv, err := d.graphDelivReader.ReadGraphDeliverable(ctx, anchor, ctxuser.TRPCUserKey(ctx), teamSessionID)
	if err != nil || len(stateDeliv) == 0 {
		return teamFullOutput{}, nil, false, false
	}
	v, exists := stateDeliv[key]
	if !exists {
		return teamFullOutput{}, payloadStateKeys(stateDeliv), false, true
	}
	return teamFullOutput{Content: renderPayloadContent(v), SessionID: teamSessionID}, nil, true, true
}

// readEnvelopeKeyContent reads one payload key from the persisted envelope's
// StructuredJSON (same ok/found/available semantics as the graph-state variant).
func (d *SpiritDelivery) readEnvelopeKeyContent(t Team, key string) (teamFullOutput, []string, bool, bool) {
	ref, ok := d.readDeliverableRef(t)
	if !ok || strings.TrimSpace(ref.StructuredJSON) == "" {
		return teamFullOutput{}, nil, false, false
	}
	var structured map[string]any
	if err := json.Unmarshal([]byte(ref.StructuredJSON), &structured); err != nil || len(structured) == 0 {
		return teamFullOutput{}, nil, false, false
	}
	v, exists := structured[key]
	if !exists {
		keys := make([]string, 0, len(structured))
		for k := range structured {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		return teamFullOutput{}, keys, false, true
	}
	return teamFullOutput{Content: renderPayloadContent(v), SessionID: ref.TeamSessionID}, nil, true, true
}

// resolveDeliverableFullContent returns the full deliverable content of a
// completed team, sourced from the DELIVERABLE itself — never the reply
// (2026-07-25 Fix 7). Priority:
//  1. graph-state re-read: untruncated, and the same source the envelope was
//     persisted from, so the full text can never disagree with the injected
//     summary (the envelope summary is truncated at MaxSummaryLen — only a
//     state re-read fulfills the tool's "full text" promise);
//  2. persisted DeliverableRef envelope, when the graph session is
//     unreadable (StructuredJSON is untruncated; Summary may be);
//  3. legacy reply extraction, for non-DAG teams and rows completed before
//     the Fix-1 gate (no envelope can exist for those).
func (d *SpiritDelivery) resolveDeliverableFullContent(ctx context.Context, t Team) (teamFullOutput, error) {
	if t.DagNodeID != "" {
		if full, ok := d.readGraphStateFullContent(ctx, t); ok {
			return full, nil
		}
		if full, ok := d.readEnvelopeFullContent(t); ok {
			return full, nil
		}
	}
	return d.extractTeamFullOutput(ctx, t.ID)
}

// readGraphStateFullContent renders the full deliverable from the graph final
// state: the reserved "summary" key (untruncated, unlike the envelope's) plus
// the non-reserved keys as a JSON object. ok=false on any gap (channel
// disabled, no session, read error, empty state) — the caller then degrades
// to the persisted envelope.
func (d *SpiritDelivery) readGraphStateFullContent(ctx context.Context, t Team) (teamFullOutput, bool) {
	anchor, ok := d.stateDeliverableChannel(t)
	if !ok {
		return teamFullOutput{}, false
	}
	teamSessionID, err := d.resolveTeamMainSessionID(ctx, t.ID)
	if err != nil || teamSessionID == "" {
		return teamFullOutput{}, false
	}
	stateDeliv, err := d.graphDelivReader.ReadGraphDeliverable(ctx, anchor, ctxuser.TRPCUserKey(ctx), teamSessionID)
	if err != nil || len(stateDeliv) == 0 {
		return teamFullOutput{}, false
	}
	summary, _ := stateDeliv[deliverableReservedKeySummary].(string)
	content := strings.TrimSpace(summary)
	if structured := marshalNonReservedStateKeys(stateDeliv); structured != "" {
		if content != "" {
			content += "\n\n"
		}
		content += structured
	}
	if content == "" {
		return teamFullOutput{}, false
	}
	return teamFullOutput{Content: content, SessionID: teamSessionID}, true
}

// readEnvelopeFullContent renders full content from the persisted
// DeliverableRef envelope: the untruncated StructuredJSON, preceded by the
// Summary when present. ok=false when no envelope exists for the team's node
// or the envelope carries nothing.
func (d *SpiritDelivery) readEnvelopeFullContent(t Team) (teamFullOutput, bool) {
	ref, ok := d.readDeliverableRef(t)
	if !ok {
		return teamFullOutput{}, false
	}
	content := strings.TrimSpace(ref.Summary)
	if structured := strings.TrimSpace(ref.StructuredJSON); structured != "" {
		if content != "" {
			content += "\n\n"
		}
		content += structured
	}
	if content == "" {
		return teamFullOutput{}, false
	}
	return teamFullOutput{Content: content, SessionID: ref.TeamSessionID}, true
}

// validateUpstreamContract performs the tool-call level contract check for
// ReadUpstreamDeliverable: reader team (resolved from readerSessionID) → its
// InputContract vs the upstream team's declared Deliverables. A mismatch
// returns *ContractMismatchError; the check is skipped (nil) when the reader
// side is unresolvable or either side declares no contract — declarations
// stay advisory for legacy teams without contracts.
// Domain: Delivery — runtime contract guard for upstream deliverable reads.
func (d *SpiritDelivery) validateUpstreamContract(ctx context.Context, readerSessionID string, upstream Team) error {
	reader, inputContracts, ok := d.resolveReaderContracts(ctx, readerSessionID, upstream.ID)
	if !ok {
		return nil
	}
	upstreamContracts, err := ParseDeliverableContracts(upstream.Deliverables)
	if err != nil || len(upstreamContracts) == 0 {
		return nil
	}
	validator := d.contractValidator
	if validator == nil {
		validator = NewDeliverableContractValidator()
	}
	mismatches := validator.ValidateContractMatchDetailed(upstreamContracts, inputContracts)
	if len(mismatches) == 0 {
		return nil
	}
	d.lg.Warn("上游交付物契约校验不匹配",
		loggateway.StepID("spirit.read_upstream_deliverable.contract_mismatch"),
		loggateway.Str("reader_team_id", reader.ID),
		loggateway.Str("upstream_team_id", upstream.ID),
		loggateway.Int("mismatch_count", len(mismatches)),
	)
	return &ContractMismatchError{ReaderTeamID: reader.ID, UpstreamTeamID: upstream.ID, Mismatches: mismatches}
}

// resolveReaderContracts resolves the reader team and its parsed input
// contract for the runtime contract guards. ok=false covers every
// advisory-skip case: empty session id, unresolvable session/team, a team
// reading its own deliverable, or no declared input contract.
func (d *SpiritDelivery) resolveReaderContracts(ctx context.Context, readerSessionID, upstreamTeamID string) (Team, []DeliverableContract, bool) {
	readerSessionID = strings.TrimSpace(readerSessionID)
	if readerSessionID == "" {
		return Team{}, nil, false
	}
	sess, err := d.sessionUC.Get(ctx, readerSessionID)
	if err != nil || sess.TeamID == "" || sess.TeamID == upstreamTeamID {

		return Team{}, nil, false
	}
	reader, err := d.teamUC.Get(ctx, sess.TeamID)
	if err != nil {
		return Team{}, nil, false
	}
	inputContracts, err := ParseDeliverableContracts(reader.InputContract)
	if err != nil || len(inputContracts) == 0 {
		return Team{}, nil, false
	}
	return reader, inputContracts, true
}

// validateUpstreamContractSchema is the C2 content-level contract guard for
// ReadUpstreamDeliverable. For each reader input-contract entry carrying a
// schema_json AND declaring format=="json", the upstream deliverable content
// is validated against that schema. A violation joins a
// *ContractMismatchError (LLM-actionable, auto-correct-and-retry).
//
// Advisory skips (never block legacy teams): entry without schema_json,
// entry with non-json format, upstream side not declaring the same-named
// json deliverable, content that is not valid JSON, and schema execution
// errors (e.g. an invalid schema declaration on the reader side).
// Domain: Delivery — content-level schema guard for upstream deliverable reads.
func (d *SpiritDelivery) validateUpstreamContractSchema(ctx context.Context, readerSessionID string, upstream Team, content string) error {
	reader, inputContracts, ok := d.resolveReaderContracts(ctx, readerSessionID, upstream.ID)
	if !ok {
		return nil
	}
	needsSchema := false
	for _, c := range inputContracts {
		if strings.TrimSpace(c.SchemaJSON) != "" && c.Format == "json" {
			needsSchema = true
			break
		}
	}
	if !needsSchema {
		return nil
	}
	if !json.Valid([]byte(content)) {
		return nil
	}
	upstreamContracts, err := ParseDeliverableContracts(upstream.Deliverables)
	if err != nil || len(upstreamContracts) == 0 {
		return nil
	}
	upstreamJSONDeclared := make(map[string]bool, len(upstreamContracts))
	for _, c := range upstreamContracts {
		if c.Format == "json" {
			upstreamJSONDeclared[c.Name] = true
		}
	}

	var mismatches []ContractMismatch
	for _, entry := range inputContracts {
		if strings.TrimSpace(entry.SchemaJSON) == "" || entry.Format != "json" {
			continue
		}
		if !upstreamJSONDeclared[entry.Name] {
			continue
		}
		verr := shared.ValidateDocumentAgainstSchema("SPIRIT", entry.SchemaJSON, content)
		if verr == nil {
			continue
		}
		if !apierror.IsCode(verr, apierror.CodeBadRequest) {
			continue
		}
		detail := verr.Error()
		if ae, ok := apierror.From(verr); ok {
			detail = strings.TrimPrefix(ae.Message, schemaViolationPrefix)
		}
		mismatches = append(mismatches, ContractMismatch{
			Name:     entry.Name,
			Kind:     ContractMismatchSchema,
			Expected: detail,
		})
	}
	if len(mismatches) == 0 {
		return nil
	}
	d.lg.Warn("上游交付物内容 schema 校验不匹配",
		loggateway.StepID("spirit.read_upstream_deliverable.schema_mismatch"),
		loggateway.Str("reader_team_id", reader.ID),
		loggateway.Str("upstream_team_id", upstream.ID),
		loggateway.Int("mismatch_count", len(mismatches)),
	)
	return &ContractMismatchError{ReaderTeamID: reader.ID, UpstreamTeamID: upstream.ID, Mismatches: mismatches}
}

// ErrNoRealDeliverable marks a DAG team that has no graph-state deliverable —
// set_deliverable was never called (or the state is unreadable). Reply text
// must NOT be dressed up as a deliverable: a team that only asked clarifying
// questions produced nothing, and persisting its reply as a "deliverable" is
// the root of the 19:29 false-success chain (2026-07-25 Fix 1).
var ErrNoRealDeliverable = errors.New("no real deliverable: set_deliverable was never called")
