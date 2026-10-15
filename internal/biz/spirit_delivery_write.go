package biz

import (
	"aranea-agents/internal/biz/session"
	"aranea-agents/pkg/ctxuser"
	"aranea-agents/pkg/loggateway"
	"context"
	"encoding/json"
	"fmt"

	"strings"
	"time"
	"unicode/utf8"
)

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
