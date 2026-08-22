package biz

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"aranea-agents/internal/biz/session"
	"aranea-agents/internal/biz/shared"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/ctxuser"
	"aranea-agents/pkg/loggateway"
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
	inboxFS           TeamInboxFS
	lg                loggateway.Logger
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

	// Primary source (production): the team session's final completed reply
	// step, read with exact session_id semantics. Legacy ChatMessage storage
	// is only a fallback when no step reader is wired or no reply step exists.
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
	stats := d.ListTeamRunStats(ctx, teamIDs) // nil-safe: reader not wired → nil map
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
					digest.Artifacts = append(digest.Artifacts, artifactDigestLine(art))
				}
			}
			digest.DeliverableSummary = strings.Join(summaries, "\n")
		}
		digests = append(digests, digest)
	}
	return digests
}

// ---------------------------------------------------------------------------
// XC-03: Cross-Department Collaboration — Contract Validation & Gate Injection
// ---------------------------------------------------------------------------

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

	// Build a map of dag_node_id → team for dependency resolution
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

	// Get verification gates from the team's linked graph
	gates, err := d.resolveVerificationGates(ctx, t)
	if err != nil || len(gates) == 0 {
		return true, nil, nil
	}

	// Resolve truncate chars from the Spirit agent's runtime settings.
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
		approved, reason, gateErr := d.gateExecutor.ExecuteGate(ctx, gate, teamOutput, truncateChars)
		if gateErr != nil {
			return false, allWarnings, gateErr
		}
		if !approved {
			allWarnings = append(allWarnings, fmt.Sprintf("gate %s rejected: %s", gate.GateType, reason))
			return false, allWarnings, nil
		}
		allWarnings = append(allWarnings, fmt.Sprintf("gate %s approved: %s", gate.GateType, reason))
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
		return nil // not a DAG node — nothing to key the output by
	}

	anchor, ok := d.stateDeliverableChannel(t)
	if !ok {
		return ErrNoRealDeliverable // channel disabled → no real deliverable can exist
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
	// 2026-08-08 问题3：减去上游播种——信封只承载本团队自有产出；种子
	// 回流混入信封会把上游产出当本团队产出再向下游传播（污染链）。
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

	// The envelope is 100% state-sourced: the reserved "summary" key becomes
	// the summary; when absent the summary aggregates each topic payload's
	// title/summary (long-article paradigm puts the body under contract
	// topics), degrading to the business keys' JSON only when no readable
	// text exists (never from the reply). Non-reserved keys land in
	// StructuredJSON and are described in Artifacts.
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

	// MDC completion-time advisory: required contract topics that were never
	// written (producer bypassed set_deliverable) surface as a Warn — the run
	// still completes and the envelope is persisted unchanged.
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

	// Merge into the dedicated deliverables_output_json column. Existing
	// values are preserved as raw messages so legacy plain-string entries
	// (pre-P2) coexist with P2 envelopes.
	outputs := make(map[string]json.RawMessage)
	if t.DeliverablesOutput != "" && t.DeliverablesOutput != "{}" {
		// Tolerate malformed JSON by starting fresh rather than failing the
		// write — the cache is rebuilt below; a corrupt row must not block
		// deliverable persistence for downstream teams.
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
		return false, nil // non-DAG teams carry no deliverable obligation
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
	// 2026-08-08 问题3：下游 graph state 可能含上游播种（turn 启动时经
	// RuntimeState 注入）。种子回流不是本团队产出——减去种子后再判空，
	// 否则「成员什么都没写」也会因种子非空误判为真实交付物。
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
		// 无 step 证据源 → 无法核实交付事实，保持原语义。
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
		// 读失败 = 事实无法确认：interrupted 仍判失败，其余按无证据。
		if interrupted {
			return true, "session interrupted: " + interruptReason
		}
		return false, ""
	}

	// 2026-08-08 修复（stream_error 误报完成）：coordinator 模式下成员的 turn
	// 级错误 step（kind=error，如 stream_error/context deadline）落在**团队会话**
	// 而非成员会话（成员会话 0 steps）→ 需回扫父（团队）会话中归属于该成员
	// （AuthorAgentKey == MemberAgentKey）的 step。可恢复的普通工具错误是
	// kind=action + ToolErrorCode，不在此列。
	parentID := strings.TrimSpace(sess.ParentSessionID)
	memberKey := strings.TrimSpace(sess.MemberAgentKey)
	backscan := parentID != "" && parentID != sessionID && memberKey != ""
	var teamSteps []Step
	if backscan {
		var terr error
		teamSteps, terr = d.stepReader.ListStepsBySessionID(ctx, parentID)
		if terr != nil {
			// 读失败 = 该证据源不贡献任何内容（事实无法确认 → 保守），
			// 成员会话自身的 fatal step 证据不受影响，继续走后续扫描。
			d.lg.Warn("成员执行证据：读取团队会话 steps 失败，团队会话证据按空处理",
				loggateway.StepID("spirit.member_evidence.team_steps_err"),
				loggateway.Str("team_session_id", parentID),
				loggateway.Err(terr),
			)
			teamSteps = nil
		}
	}

	// P7：交付事实 = 任一证据源中该成员 completed 的 set_deliverable action。
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
		// 交付事实成立 → 过程性异常不再作为成员失败证据（见函数头注释）。
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
	// coordinator 模式回退：成员 step 落在团队会话（与 MemberExecutionEvidence
	// 同一归属模型），按 AuthorAgentKey 过滤出本成员的 step。
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

	// Build a map of dag_node_id → team
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

		// Read the persisted deliverable envelope. 2026-07-25 Fix 1: the reply
		// extraction fallback is REMOVED — after the deliverable gate a
		// completed team always has an envelope, so a missing/empty one means
		// the write failed; degrade to "no injection" rather than fabricating
		// downstream input from reply text.
		ref, refOK := d.readDeliverableRef(upstream)
		if !refOK || ref.Summary == "" {
			continue
		}
		// Legacy plain-string values carry no team_id; the caller always knows it.
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

// renderArtifactSections renders the upstream envelope's payload artifacts:
// small payloads (≤ InlineUpstreamPayloadMaxChars) inline in full so the
// downstream team consumes them with zero tool calls; larger payloads render
// as a pointer plus a keyed read_upstream_deliverable instruction. Inline
// rendering is bounded by InlineUpstreamArtifactMaxCount and
// InlineUpstreamPayloadTotalMaxChars — once either budget is spent, remaining
// payloads degrade to pointers so a multi-topic deliverable cannot bloat the
// downstream first-turn input. Empty payloads (SizeChars == 0) render as an
// honest note WITHOUT a retrieval instruction: a keyed read of them can only
// return "内容为空". When the artifact key hits the downstream team's
// InputContract, the retrieval instruction escalates from an optional hint to
// a mandatory pre-task step — fetching the full text no longer depends on LLM
// discretion.
func renderArtifactSections(ref DeliverableRef, downstreamTeam Team) string {
	if len(ref.Artifacts) == 0 {
		return ""
	}
	inputNames := make(map[string]bool)
	if contracts, err := ParseDeliverableContracts(downstreamTeam.InputContract); err == nil {
		for _, c := range contracts {
			if name := strings.TrimSpace(c.Name); name != "" {
				inputNames[name] = true
			}
		}
	}
	// structured parses lazily on the first inline decision.
	var structured map[string]any
	structuredParsed := false
	parseStructured := func() map[string]any {
		if !structuredParsed {
			structuredParsed = true
			if ref.StructuredJSON != "" {
				_ = json.Unmarshal([]byte(ref.StructuredJSON), &structured)
			}
		}
		return structured
	}

	var sb strings.Builder
	inlinedCount, inlinedChars := 0, 0
	for _, art := range ref.Artifacts {
		label := art.Key
		if art.Title != "" {
			label += "《" + art.Title + "》"
		}
		format := art.Format
		if format == "" {
			format = "text"
		}
		if art.IsBulkPointer() {
			dest := art.RelPath
			if dest == "" {
				dest = art.Key
			}
			sizeHint := ""
			if art.SizeBytes > 0 {
				sizeHint = fmt.Sprintf("，%d 字节", art.SizeBytes)
			} else if art.SizeChars > 0 {
				sizeHint = fmt.Sprintf("，%d 字", art.SizeChars)
			}
			sb.WriteString(fmt.Sprintf("\n- 附件 %s（%s%s）已物化到 inbox/%s/%s，请用文件工具按需读取，禁止把文件正文贴进回复。",
				label, format, sizeHint, ref.TeamID, dest))
			continue
		}
		if art.SizeChars == 0 {
			// 空载荷：keyed 读取只会得到「内容为空」的 NotFound，不给读取指令。
			sb.WriteString(fmt.Sprintf("\n- 交付物 %s（%s）：载荷内容为空（上游写入但无正文）。", label, format))
			continue
		}
		withinInlineBudget := inlinedCount < InlineUpstreamArtifactMaxCount &&
			inlinedChars+art.SizeChars <= InlineUpstreamPayloadTotalMaxChars
		if art.SizeChars <= InlineUpstreamPayloadMaxChars && withinInlineBudget {
			content := extractArtifactContent(parseStructured(), art.Key)
			if content == "" {
				// Envelope lost the payload body (legacy writer) — degrade to
				// a pointer rather than dropping the artifact silently.
				sb.WriteString(fmt.Sprintf("\n- 交付物 %s（%s，共 %d 字）。需要全文时调用 read_upstream_deliverable(team_id=\"%s\", key=\"%s\") 获取。",
					label, format, art.SizeChars, ref.TeamID, art.Key))
				continue
			}
			inlinedCount++
			inlinedChars += art.SizeChars
			sb.WriteString(fmt.Sprintf("\n### 交付物 %s（%s，全文 %d 字）\n%s", label, format, art.SizeChars, content))
			continue
		}
		// Payloads beyond the tool's default budget need an explicit max_chars —
		// otherwise the instructed call silently returns truncated content.
		call := fmt.Sprintf("read_upstream_deliverable(team_id=\"%s\", key=\"%s\")", ref.TeamID, art.Key)
		if art.SizeChars > DefaultUpstreamDeliverableMaxChars {
			budget := art.SizeChars
			if budget > MaxUpstreamDeliverableChars {
				budget = MaxUpstreamDeliverableChars
			}
			call = fmt.Sprintf("read_upstream_deliverable(team_id=\"%s\", key=\"%s\", max_chars=%d)", ref.TeamID, art.Key, budget)
		}
		sb.WriteString(fmt.Sprintf("\n- 交付物 %s（%s，共 %d 字，全文未内联）", label, format, art.SizeChars))
		if inputNames[art.Key] {
			sb.WriteString(fmt.Sprintf("\n  你的输入契约包含 %s：开始任务前必须先调用 %s 获取全文，再基于全文执行。",
				art.Key, call))
		} else {
			sb.WriteString(fmt.Sprintf("\n  需要全文时调用 %s 获取。", call))
		}
	}
	return sb.String()
}

// extractArtifactContent pulls one payload's body out of the envelope's
// StructuredJSON: document-paradigm maps contribute their "content" field,
// plain strings pass through, other shapes serialize as indented JSON.
func extractArtifactContent(structured map[string]any, key string) string {
	if structured == nil {
		return ""
	}
	v, ok := structured[key]
	if !ok {
		return ""
	}
	switch payload := v.(type) {
	case map[string]any:
		if content, ok := payload["content"].(string); ok {
			return strings.TrimSpace(content)
		}
		b, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return ""
		}
		return string(b)
	case string:
		return strings.TrimSpace(payload)
	default:
		b, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return ""
		}
		return string(b)
	}
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
	// Knowledge/memory stay on-demand (tools). 6KB is a ceiling on Brief+protocol only.
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
	// C2: content-level schema validation runs AFTER full-text extraction (it
	// needs the content), unlike the name/type/format check above which fails
	// fast before the expensive read.
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

// renderPayloadContent renders one payload value as standalone full text:
// document-paradigm maps prepend 《title》 to their content, plain strings
// pass through, other shapes serialize as indented JSON.
func renderPayloadContent(v any) string {
	switch payload := v.(type) {
	case map[string]any:
		content, _ := payload["content"].(string)
		title, _ := payload["title"].(string)
		content, title = strings.TrimSpace(content), strings.TrimSpace(title)
		if content != "" {
			if title != "" {
				return "《" + title + "》\n\n" + content
			}
			return content
		}
	case string:
		return strings.TrimSpace(payload)
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return ""
	}
	return string(b)
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
		// Unresolvable reader, or a team reading its own deliverable: no check.
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
		return nil // non-JSON content cannot be schema-validated — advisory skip
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
			continue // name/format gaps are reported by the fast-fail check
		}
		verr := shared.ValidateDocumentAgainstSchema("SPIRIT", entry.SchemaJSON, content)
		if verr == nil {
			continue
		}
		if !apierror.IsCode(verr, apierror.CodeBadRequest) {
			continue // schema execution error (e.g. invalid schema) — advisory skip
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

// teamFullOutput is the untruncated team output plus its source session —
// the P2 DeliverableRef envelope needs both (SizeChars / TeamSessionID).
type teamFullOutput struct {
	Content   string // full, untruncated deliverable content
	SessionID string // team main session the content was read from
}

func extractKeyFindings(content string) string {
	var findings []string
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		isBullet := strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ")
		isNumbered := isNumberedListItem(trimmed)
		isQuote := strings.HasPrefix(trimmed, "> ")
		if (isBullet || isNumbered) && !isQuote && len(findings) < MaxKeyFindingsCount {
			findings = append(findings, trimmed)
		}
	}
	return strings.Join(findings, "\n")
}

func isNumberedListItem(s string) bool {
	if len(s) < 3 {
		return false
	}
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == 0 || i >= len(s) {
		return false
	}
	return (s[i] == '.' || s[i] == ')') && i+1 < len(s) && s[i+1] == ' '
}

// ---------------------------------------------------------------------------
// XC-03b: Deliverable Passing Mechanism
// ---------------------------------------------------------------------------

// ErrNoRealDeliverable marks a DAG team that has no graph-state deliverable —
// set_deliverable was never called (or the state is unreadable). Reply text
// must NOT be dressed up as a deliverable: a team that only asked clarifying
// questions produced nothing, and persisting its reply as a "deliverable" is
// the root of the 19:29 false-success chain (2026-07-25 Fix 1).
var ErrNoRealDeliverable = errors.New("no real deliverable: set_deliverable was never called")

// stateDeliverableProbe is the minimal subset of the team DefinitionJSON the
// graph-state bridge needs. biz cannot import internal/team (dependency
// direction), so the relevant fields are probed directly.
type stateDeliverableProbe struct {
	EnableStateDeliverable bool   `json:"enable_state_deliverable"`
	IntentAnchorAgentID    string `json:"intent_anchor_agent_id"`
	Members                []struct {
		AgentID string `json:"agent_id"`
	} `json:"members"`
}

// anchorAgentID mirrors the runner's anchor resolution
// (resolveAnchorAndAttachments): intent_anchor_agent_id wins when it names a
// member; an intent anchor naming no member is ignored (the runner warns and
// falls back the same way) and the first member's agent ID is used. The
// anchor agent ID is the AppName the team run persisted its graph state
// under — a wrong pick makes the bridge read a session that never existed.
func (p stateDeliverableProbe) anchorAgentID() string {
	first := ""
	for _, m := range p.Members {
		if id := strings.TrimSpace(m.AgentID); id != "" {
			first = id
			break
		}
	}
	if want := strings.TrimSpace(p.IntentAnchorAgentID); want != "" {
		for _, m := range p.Members {
			if strings.TrimSpace(m.AgentID) == want {
				return want
			}
		}
	}
	return first
}

// memberEvidenceSummaryMaxRunes caps the failed-step summary carried in a
// MemberExecutionEvidence reason (log + projection diagnostics only).
const memberEvidenceSummaryMaxRunes = 120

// memberStepWindow aggregates the execution window across steps: start is the
// earliest non-zero StartedAt; end is the latest activity evidence, preferring
// CompletedAt over StartedAt per step. authorFilter non-empty restricts to
// steps authored by that agent key. ok=false means no step carried a non-zero
// StartedAt.
//
// notice steps are excluded (2026-08-08 回归发现): context_usage 等被动通知以
// 成员 author 持续落到 run 结束，会把 end 撑大到远超成员真实工作完成时刻
// （真实 reply 04:54:42，notice 拖到 04:59:15），且 publish 查询与 notice 落库
// 存在时序 race，窗口结果不稳定。notice 不产生任何执行工作量，不是活动证据。
func memberStepWindow(steps []Step, authorFilter string) (start, end time.Time, ok bool) {
	for _, st := range steps {
		if authorFilter != "" && strings.TrimSpace(st.AuthorAgentKey) != authorFilter {
			continue
		}
		if st.Kind == StepKindNotice {
			continue
		}
		if !st.StartedAt.IsZero() {
			if start.IsZero() || st.StartedAt.Before(start) {
				start = st.StartedAt
			}
		}
		// 每步的「最后活动证据」：CompletedAt 优先，缺失退 StartedAt。
		candidate := st.StartedAt
		if st.CompletedAt != nil && !st.CompletedAt.IsZero() {
			candidate = *st.CompletedAt
		}
		if !candidate.IsZero() && (end.IsZero() || candidate.After(end)) {
			end = candidate
		}
	}
	return start, end, !start.IsZero()
}

// Reserved keys of the graph deliverable state map. Both are extracted into
// first-class DeliverableRef fields by WriteDeliverablesToSession and are
// therefore excluded from StructuredJSON.
const (
	deliverableReservedKeySummary   = "summary"
	deliverableReservedKeyCognition = "cognition"
)

// extractStateCognition converts the reserved "cognition" state-map entry
// into a DeliverableCognition. The value arrives as a map[string]any (JSON
// round-trip from graph state), so it is re-marshalled and unmarshalled into
// the typed struct. Tolerant: nil on absence, wrong shape, or corrupt data —
// a malformed cognition must never block the deliverable write.
func extractStateCognition(v any) *DeliverableCognition {
	if v == nil {
		return nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var c DeliverableCognition
	if err := json.Unmarshal(b, &c); err != nil {
		return nil
	}
	if len(c.Decisions) == 0 && len(c.Rejected) == 0 && len(c.Assumptions) == 0 && len(c.OpenQuestions) == 0 {
		return nil
	}
	return &c
}

// marshalNonReservedStateKeys serializes every deliverable state key except
// the reserved ones ("summary", "cognition") into the envelope's
// StructuredJSON field. "" when nothing remains.
func marshalNonReservedStateKeys(stateDeliv map[string]any) string {
	rest := make(map[string]any, len(stateDeliv))
	for k, v := range stateDeliv {
		if k == deliverableReservedKeySummary || k == deliverableReservedKeyCognition {
			continue
		}
		// ack/<topic> keys are intra-team delivery acknowledgments (MDC) —
		// advisory signals for the coordinator, never inter-team content, so
		// they must not leak into the DeliverableRef envelope.
		if strings.HasPrefix(k, deliverableAckKeyPrefix) {
			continue
		}
		rest[k] = v
	}
	if len(rest) == 0 {
		return ""
	}
	b, err := json.Marshal(rest)
	if err != nil {
		return ""
	}
	return string(b)
}

// payloadStateKeys returns the non-reserved deliverable state keys (sorted
// for deterministic output): the reserved summary/cognition keys and the
// intra-team ack/ keys are excluded — everything else is a payload topic.
func payloadStateKeys(stateDeliv map[string]any) []string {
	keys := make([]string, 0, len(stateDeliv))
	for k := range stateDeliv {
		if k == deliverableReservedKeySummary || k == deliverableReservedKeyCognition {
			continue
		}
		if strings.HasPrefix(k, deliverableAckKeyPrefix) {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// buildDeliverableArtifacts describes every payload key of the state
// deliverable so downstream injection can decide inline vs pointer.
// Contract entries (matched by name == key) supply type/format; document-
// paradigm payloads {"title","format","content"} contribute title and a
// content-based size; other shapes size by JSON serialization.
func buildDeliverableArtifacts(stateDeliv map[string]any, deliverablesJSON string) []DeliverableArtifact {
	keys := payloadStateKeys(stateDeliv)
	if len(keys) == 0 {
		return nil
	}
	contracts, _ := ParseDeliverableContracts(deliverablesJSON)
	byName := make(map[string]DeliverableContract, len(contracts))
	for _, c := range contracts {
		byName[strings.TrimSpace(c.Name)] = c
	}
	out := make([]DeliverableArtifact, 0, len(keys))
	for _, k := range keys {
		art := DeliverableArtifact{Key: k}
		if c, ok := byName[k]; ok {
			art.Type = strings.TrimSpace(c.Type)
			art.Format = strings.TrimSpace(c.Format)
		}
		switch v := stateDeliv[k].(type) {
		case map[string]any:
			fillArtifactFromPayload(&art, v)
		case string:
			art.SizeChars = utf8.RuneCountInString(v)
		default:
			art.SizeChars = jsonValueRuneLen(v)
		}
		out = append(out, art)
	}
	return out
}

func fillArtifactFromPayload(art *DeliverableArtifact, v map[string]any) {
	if art == nil || v == nil {
		return
	}
	if title, ok := v["title"].(string); ok {
		art.Title = strings.TrimSpace(title)
	}
	if art.Format == "" {
		if f, ok := v["format"].(string); ok {
			art.Format = strings.TrimSpace(f)
		}
	}
	if mime, ok := v["mime_type"].(string); ok {
		art.MimeType = strings.TrimSpace(mime)
	}
	if sha, ok := v["sha256"].(string); ok {
		art.SHA256 = strings.TrimSpace(sha)
	}
	if id, ok := v["artifact_id"].(string); ok {
		art.ArtifactID = strings.TrimSpace(id)
	}
	if rel, ok := v["rel_path"].(string); ok {
		art.RelPath = sanitizeInboxRelPath(rel)
	} else if rel, ok := v["path"].(string); ok {
		art.RelPath = sanitizeInboxRelPath(rel)
	}
	switch n := v["size_bytes"].(type) {
	case float64:
		art.SizeBytes = int64(n)
	case int:
		art.SizeBytes = int64(n)
	case int64:
		art.SizeBytes = n
	}
	if content, ok := v["content"].(string); ok && strings.TrimSpace(content) != "" {
		art.SizeChars = utf8.RuneCountInString(content)
	} else if art.SizeChars == 0 {
		art.SizeChars = jsonValueRuneLen(v)
	}
	if strings.TrimSpace(art.ArtifactID) != "" {
		art.Kind = DeliverableArtifactKindArtifact
	} else if strings.TrimSpace(art.RelPath) != "" {
		art.Kind = DeliverableArtifactKindWorkspaceRel
	}
}

// marshalEnvelopeStructuredJSON copies non-reserved keys into the envelope.
// Bulk pointers and oversize text omit the full content (M78 ORGFAST-16).
func marshalEnvelopeStructuredJSON(stateDeliv map[string]any, artifacts []DeliverableArtifact) string {
	byKey := make(map[string]DeliverableArtifact, len(artifacts))
	for _, a := range artifacts {
		byKey[a.Key] = a
	}
	rest := make(map[string]any, len(stateDeliv))
	for k, v := range stateDeliv {
		if k == deliverableReservedKeySummary || k == deliverableReservedKeyCognition {
			continue
		}
		if strings.HasPrefix(k, deliverableAckKeyPrefix) {
			continue
		}
		art := byKey[k]
		if art.IsBulkPointer() {
			rest[k] = bulkPointerStub(v, art)
			continue
		}
		if art.SizeChars > MaxEnvelopeStructuredPayloadChars {
			rest[k] = oversizedTextStub(v, art)
			continue
		}
		rest[k] = v
	}
	if len(rest) == 0 {
		return ""
	}
	b, err := json.Marshal(rest)
	if err != nil {
		return ""
	}
	return string(b)
}

func bulkPointerStub(v any, art DeliverableArtifact) map[string]any {
	stub := map[string]any{
		"content_omitted": true,
		"kind":            art.ResolvedKind(),
	}
	if art.Title != "" {
		stub["title"] = art.Title
	}
	if art.Format != "" {
		stub["format"] = art.Format
	}
	if art.RelPath != "" {
		stub["rel_path"] = art.RelPath
	}
	if art.ArtifactID != "" {
		stub["artifact_id"] = art.ArtifactID
	}
	if art.SizeBytes > 0 {
		stub["size_bytes"] = art.SizeBytes
	}
	if m, ok := v.(map[string]any); ok {
		if t, ok := m["title"].(string); ok && stub["title"] == nil {
			stub["title"] = t
		}
	}
	return stub
}

func oversizedTextStub(v any, art DeliverableArtifact) map[string]any {
	stub := map[string]any{
		"content_omitted": true,
		"size_chars":      art.SizeChars,
	}
	if m, ok := v.(map[string]any); ok {
		if t, ok := m["title"].(string); ok {
			stub["title"] = t
		}
		if f, ok := m["format"].(string); ok {
			stub["format"] = f
		}
	}
	if art.Title != "" {
		stub["title"] = art.Title
	}
	if art.Format != "" {
		stub["format"] = art.Format
	}
	return stub
}

// aggregateTopicSummaries builds a readable summary from topic payloads when
// the producer did not write a top-level "summary" key (the long-article
// paradigm puts the body under contract topics). Only map-shaped topics
// contribute (title/summary/note); flat scalar keys keep the legacy
// structured-JSON fallback so no key context is lost. Returns "" when no
// topic contributes human-readable text.
func aggregateTopicSummaries(stateDeliv map[string]any) string {
	keys := payloadStateKeys(stateDeliv)
	lines := make([]string, 0, len(keys))
	for _, k := range keys {
		v, ok := stateDeliv[k].(map[string]any)
		if !ok {
			continue
		}
		title, _ := v["title"].(string)
		summ, _ := v["summary"].(string)
		if strings.TrimSpace(summ) == "" {
			summ, _ = v["note"].(string)
		}
		title, summ = strings.TrimSpace(title), strings.TrimSpace(summ)
		switch {
		case title != "" && summ != "":
			lines = append(lines, "《"+title+"》"+summ)
		case title != "":
			lines = append(lines, "《"+title+"》")
		case summ != "":
			lines = append(lines, summ)
		}
	}
	// Cap the aggregate — the envelope truncates to MaxSummaryLen anyway, so
	// this only bounds pathological multi-topic maps.
	return TruncateRunes(strings.Join(lines, "\n"), MaxSummaryLen*4)
}

// jsonValueRuneLen sizes a non-string payload by its indented JSON
// serialization — the same rendering extractArtifactContent/renderPayloadContent
// inline or serve, so the inline-vs-pointer threshold
// (InlineUpstreamPayloadMaxChars) measures exactly what gets injected.
func jsonValueRuneLen(v any) int {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return 0
	}
	return utf8.RuneCountInString(string(b))
}

// artifactDigestLine renders one artifact as a compact pointer-level digest
// line, e.g. article《云计算十年》（markdown，8234字）.
func artifactDigestLine(art DeliverableArtifact) string {
	label := art.Key
	if art.Title != "" {
		label += "《" + art.Title + "》"
	}
	format := art.Format
	if format == "" {
		format = "text"
	}
	return fmt.Sprintf("%s（%s，%d字）", label, format, art.SizeChars)
}

// deliverableAckKeyPrefix mirrors deliverabletools.AckKeyPrefix. biz cannot
// import internal/tools (dependency direction), so the prefix is duplicated
// and pinned by TestMarshalNonReservedStateKeys_ExcludesAckKeys.
const deliverableAckKeyPrefix = "ack/"

// requiredTopicsMissingFromState is the completion-time advisory check for
// the member-level deliverable contract (MDC): topics declared required in
// the team's deliverable_contract but absent from the final deliverable map
// (covers the "producer never called set_deliverable" bypass). Advisory only
// — the caller logs a Warn and never blocks the run.
func requiredTopicsMissingFromState(t Team, stateDeliv map[string]any) []string {
	if strings.TrimSpace(t.DefinitionJSON) == "" {
		return nil
	}
	var probe struct {
		DeliverableContract *MemberDeliverableContract `json:"deliverable_contract"`
	}
	if err := json.Unmarshal([]byte(t.DefinitionJSON), &probe); err != nil {
		return nil
	}
	return probe.DeliverableContract.RequiredTopicsMissing(stateDeliv)
}

// subtractUpstreamSeed returns the entries of stateDeliv that did NOT come
// from the seed. A key counts as seeded only when its current value
// deep-equals the seed value: a member overwrite of a seeded topic changes
// the value and therefore counts as the team's OWN output. nil/empty seed
// (or empty state) returns the state unchanged.
func subtractUpstreamSeed(stateDeliv, seed map[string]any) map[string]any {
	if len(seed) == 0 || len(stateDeliv) == 0 {
		return stateDeliv
	}
	own := make(map[string]any, len(stateDeliv))
	for k, v := range stateDeliv {
		if sv, ok := seed[k]; ok && reflect.DeepEqual(sv, v) {
			continue
		}
		own[k] = v
	}
	return own
}

// contractSubmissionLines renders the F5 per-contract submission instruction
// for the delivery-protocol suffix, one entry per contract:
//
//	契约: xlsx_install_result (data/json) — 安装结果
//	  提交方式：set_deliverable(topic="xlsx_install_result", data={...})
//
// The explicit topic removes the member's freedom to invent topic names —
// the 12:33 root cause where the spirit guessed pdf_install_result while the
// member wrote xlsx_install_result. Returns "" when no parseable contract.
func contractSubmissionLines(deliverablesJSON string) string {
	contracts, err := ParseDeliverableContracts(deliverablesJSON)
	if err != nil || len(contracts) == 0 {
		return ""
	}
	lines := make([]string, 0, len(contracts)*2)
	for _, c := range contracts {
		name := strings.TrimSpace(c.Name)
		if name == "" {
			continue
		}
		line := fmt.Sprintf("契约: %s (%s/%s)", name, c.Type, c.Format)
		if c.Description != "" {
			line += " — " + c.Description
		}
		lines = append(lines, line)
		lines = append(lines, fmt.Sprintf("  提交方式：set_deliverable(topic=%q, data={...})", name))
	}
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

// contractDeclarationLines renders the P1 contract declaration for the
// injection prefix, one line per declared contract, e.g.
// "契约: research_report (document/markdown) — 调研结论报告\n".
// Returns "" when the upstream team has no parseable contract, so the
// prefix keeps its legacy shape (name + summary only).
func contractDeclarationLines(deliverablesJSON string) string {
	contracts, err := ParseDeliverableContracts(deliverablesJSON)
	if err != nil || len(contracts) == 0 {
		return ""
	}
	lines := make([]string, 0, len(contracts))
	for _, c := range contracts {
		line := fmt.Sprintf("契约: %s (%s/%s)", c.Name, c.Type, c.Format)
		if c.Description != "" {
			line += " — " + c.Description
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n") + "\n"
}

// cognitionItemMaxRunes bounds each rendered cognition item so a verbose
// upstream record cannot blow up the injection prefix.
const cognitionItemMaxRunes = 200

// renderCognitionLines renders the C1 cognition block of an upstream
// deliverable ref for the injection prefix, one line per aspect:
//
//	[上游决策] 选择 A 方案（理由: …，置信度 0.8）；否决 B 方案（原因: …）
//	[上游假设] 数据源 Q3 已封板
//	[上游遗留问题] 样本偏差未校正
//
// Each item is truncated to cognitionItemMaxRunes. "" when cognition is nil
// or renders to nothing — legacy envelopes keep the prefix unchanged.
func renderCognitionLines(c *DeliverableCognition) string {
	if c == nil {
		return ""
	}
	var lines []string
	var decisions []string
	for _, d := range c.Decisions {
		item := fmt.Sprintf("选择 %s（理由: %s", d.Choice, d.Rationale)
		if d.Confidence > 0 {
			item += fmt.Sprintf("，置信度 %g", d.Confidence)
		}
		item += "）"
		decisions = append(decisions, TruncateRunes(item, cognitionItemMaxRunes))
	}
	for _, r := range c.Rejected {
		decisions = append(decisions, TruncateRunes(fmt.Sprintf("否决 %s（原因: %s）", r.Option, r.Reason), cognitionItemMaxRunes))
	}
	if len(decisions) > 0 {
		lines = append(lines, "[上游决策] "+strings.Join(decisions, "；"))
	}
	if len(c.Assumptions) > 0 {
		items := make([]string, 0, len(c.Assumptions))
		for _, a := range c.Assumptions {
			items = append(items, TruncateRunes(a, cognitionItemMaxRunes))
		}
		lines = append(lines, "[上游假设] "+strings.Join(items, "；"))
	}
	if len(c.OpenQuestions) > 0 {
		items := make([]string, 0, len(c.OpenQuestions))
		for _, q := range c.OpenQuestions {
			items = append(items, TruncateRunes(q, cognitionItemMaxRunes))
		}
		lines = append(lines, "[上游遗留问题] "+strings.Join(items, "；"))
	}
	return strings.Join(lines, "\n")
}

// schemaViolationPrefix is the message prefix produced by
// shared.ValidateDocumentAgainstSchema on a document/schema mismatch; the
// mismatch record keeps only the violation detail.
const schemaViolationPrefix = "config does not match schema: "
