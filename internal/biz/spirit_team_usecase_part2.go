package biz

import (
	"aranea-agents/internal/biz/session"
	"context"
	"time"
)

func (u *SpiritTeamUsecase) GetParallelConfig(ctx context.Context, spiritSessionID string) ParallelConfig {
	return u.orchestration.GetParallelConfig(ctx, spiritSessionID)
}

// Domain: Orchestration — cancel team and its timeout timer.
// reason 是 P2-6 取消原因（空 = user_cancel，保持向后兼容）。
func (u *SpiritTeamUsecase) CancelTeam(ctx context.Context, teamID string, reason CancelReason) error {
	return u.orchestration.CancelTeam(ctx, teamID, reason)
}

// Domain: Orchestration — auto-archive completed/failed teams past threshold.
func (u *SpiritTeamUsecase) AutoArchiveCompletedTeams(ctx context.Context, spiritSessionID string) {
	u.orchestration.AutoArchiveCompletedTeams(ctx, spiritSessionID)
}

// CancelTimeoutTimer stops the timeout timer for a team if one is pending.
// Should be called when a team reaches a terminal state (completed/failed/cancelled)
// to prevent the timeout callback from firing unnecessarily.
func (u *SpiritTeamUsecase) CancelTimeoutTimer(teamID string) {
	u.orchestration.CancelTimeoutTimer(teamID)
}

func (u *SpiritTeamUsecase) RegisterTeamTimeout(ctx context.Context, team Team) {
	u.orchestration.RegisterTeamTimeout(ctx, team)
}

// Stop cancels all pending timeout timers and the background polling goroutine.
// Call during application shutdown to prevent callbacks from firing after the
// server has stopped.
func (u *SpiritTeamUsecase) Stop() {
	u.orchestration.Stop()
}

func (u *SpiritTeamUsecase) ExtractTeamOutput(ctx context.Context, teamID string) (summary string, keyFindings string, err error) {
	return u.delivery.ExtractTeamOutput(ctx, teamID)
}

func (u *SpiritTeamUsecase) CheckTeamProgress(ctx context.Context, spiritSessionID string) ([]TeamProgress, error) {
	return u.orchestration.CheckTeamProgress(ctx, spiritSessionID)
}

// resolveAgentKeyToIDMap maps agentKeys (e.g. "__spirit__") to agent IDs (e.g.
// "agent___spirit__" or UUID). Pages through ALL active agents (the repo clamps
// a single page to ≤500) and builds a lookup map. Unresolvable keys return an
// error naming them — the former silent "agent_"+key fallback only ever worked
// for system agents whose IDs follow that naming convention, and began
// misfiring once active agents exceeded the old single-page Limit of 200.
func (u *SpiritTeamUsecase) resolveAgentKeyToIDMap(ctx context.Context, agentKeys []string) (map[string]string, error) {
	return u.assembly.resolveAgentKeyToIDMap(ctx, agentKeys)
}

func (u *SpiritTeamUsecase) SearchSessions(ctx context.Context, q session.SessionSearchQuery) (session.SessionListResult, error) {
	return u.assembly.SearchSessions(ctx, q)
}

func (u *SpiritTeamUsecase) GetSpiritQuery(ctx context.Context, spiritSessionID string) string {
	return u.assembly.GetSpiritQuery(ctx, spiritSessionID)
}

// UpdateTeamDefinitionJSON replaces the team's DefinitionJSON with the provided value
// and persists the change. Used by TaskOrchestrator to write DAG-compiled definitions.
// Domain: Assembly — update team definition JSON (used by TaskOrchestrator for DAG-compiled definitions).
func (u *SpiritTeamUsecase) UpdateTeamDefinitionJSON(ctx context.Context, teamID string, definitionJSON string) error {
	return u.assembly.UpdateTeamDefinitionJSON(ctx, teamID, definitionJSON)
}

// RecordTeamCompletion records DQ Score, infers topology, and creates evolution suggestions
// for a completed team. Returns the computed DQ Score and inferred topology.
// Domain: Orchestration — record DQ score and create evolution suggestions on team completion.
func (u *SpiritTeamUsecase) RecordTeamCompletion(ctx context.Context, team Team, durationMs int64) (dqScore float64, topology TopologyType) {
	return u.orchestration.RecordTeamCompletion(ctx, team, durationMs)
}

// ScheduleDependentTeams resolves DAG dependencies after a team completes.
// It returns a list of actions to take (activate or fail dependent teams).
// The caller (Service layer) is responsible for executing the actions
// (starting runners, publishing events, etc.).
// Domain: Orchestration — DAG dependency resolution and scheduling.
func (u *SpiritTeamUsecase) ScheduleDependentTeams(ctx context.Context, spiritSessionID string, completedTeam Team) []DependentTeamAction {
	return u.orchestration.ScheduleDependentTeams(ctx, spiritSessionID, completedTeam)
}

// CheckAllTeamsCompleted checks whether all teams for a spirit session are in a terminal state.
// Returns a result indicating if all teams are done and the list of team IDs.
// A team is considered "done" if it is in completed, failed, or cancelled state.
// Domain: Orchestration — check if all teams reached terminal state.
func (u *SpiritTeamUsecase) CheckAllTeamsCompleted(ctx context.Context, spiritSessionID string) AllTeamsCompletedResult {
	return u.orchestration.CheckAllTeamsCompleted(ctx, spiritSessionID)
}

// ListFailedTeamBriefs collects honest failure briefs for the synthesis
// summary trigger (2026-07-25 Fix 3). Only genuinely failed teams are
// included — cancelled teams are a user abort and skip the report entirely
// (checkAllTeamsCompleted returns early when CancelledTeams > 0). Reason
// comes from latest-run stats; LastReply carries the team's final reply,
// which is where its unresolved questions live. Best-effort: missing pieces
// degrade to fallback text rather than failing the whole collection.
func (u *SpiritTeamUsecase) ListFailedTeamBriefs(ctx context.Context, spiritSessionID string) []TeamFailureBrief {
	return u.delivery.ListFailedTeamBriefs(ctx, spiritSessionID)
}

// ListTeamDeliverableDigests collects per-terminal-team deliverable summaries
// for the synthesis trigger (F7, Phase 11). Completed AND failed teams are
// included (failed teams render with an empty summary → 「无交付物」), so the
// Spirit LLM composes the final report from real structured outputs instead
// of excavating session history. Non-terminal teams and read failures are
// skipped (best effort — the trigger must never fail on digest collection).
// Domain: Delivery — per-team deliverable digest assembly.
func (u *SpiritTeamUsecase) ListTeamDeliverableDigests(ctx context.Context, spiritSessionID string) []TeamDeliverableDigest {
	return u.delivery.ListTeamDeliverableDigests(ctx, spiritSessionID)
}

// ---------------------------------------------------------------------------
// XC-03: Cross-Department Collaboration — Contract Validation & Gate Injection
// ---------------------------------------------------------------------------
// ValidateDeliverableContracts validates deliverable contracts between
// upstream and downstream teams in the DAG. Returns a list of warnings
// for contract mismatches. Called after Team DAG is built.
// Domain: Delivery — validate deliverable contracts between upstream and downstream teams.
func (u *SpiritTeamUsecase) ValidateDeliverableContracts(ctx context.Context, spiritSessionID string) []string {
	return u.delivery.ValidateDeliverableContracts(ctx, spiritSessionID)
}

// InjectDeptLeadIntoTeam adds the department lead agent to a team's definition.
// Called during team assembly for cross-department collaboration.
// Domain: Assembly — dept lead injection into team definition.
func (u *SpiritTeamUsecase) InjectDeptLeadIntoTeam(ctx context.Context, teamID string) error {
	return u.assembly.InjectDeptLeadIntoTeam(ctx, teamID)
}

// ExecuteVerificationGates runs all verification gates for a team's output.
// Returns (approved bool, warnings []string, err error).
// If any gate rejects, the whole verification fails.
// Domain: Delivery — execute verification gates on team output.
func (u *SpiritTeamUsecase) ExecuteVerificationGates(ctx context.Context, teamID string, teamOutput string) (bool, []string, error) {
	return u.delivery.ExecuteVerificationGates(ctx, teamID, teamOutput)
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
func (u *SpiritTeamUsecase) WriteDeliverablesToSession(ctx context.Context, teamID string) error {
	return u.delivery.WriteDeliverablesToSession(ctx, teamID)
}

// HasRealDeliverable reports whether the team produced a REAL deliverable —
// a non-empty graph-state "deliverable" map written via set_deliverable.
// Reply text does not count. (false, nil) covers every "no deliverable"
// shape (non-DAG team, channel disabled, no session, empty state);
// (false, err) is reserved for infra failures so the caller can distinguish
// "did not produce" from "could not verify" (2026-07-25 Fix 1 gate).
func (u *SpiritTeamUsecase) HasRealDeliverable(ctx context.Context, team Team) (bool, error) {
	return u.delivery.HasRealDeliverable(ctx, team)
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
func (u *SpiritTeamUsecase) MemberExecutionEvidence(ctx context.Context, sessionID string) (bool, string) {
	return u.delivery.MemberExecutionEvidence(ctx, sessionID)
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
func (u *SpiritTeamUsecase) MemberExecutionWindow(ctx context.Context, sessionID string) (time.Time, time.Time, bool) {
	return u.delivery.MemberExecutionWindow(ctx, sessionID)
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
func (u *SpiritTeamUsecase) UpstreamDeliverableSeed(ctx context.Context, downstreamTeam Team) (map[string]any, error) {
	return u.delivery.UpstreamDeliverableSeed(ctx, downstreamTeam)
}

// InjectUpstreamDeliverables collects upstream team deliverables and formats
// them as a prefix for the downstream team's input message.
// Called when a DAG activates a downstream team.
// It first tries to read from the persisted deliverable output cache
// (written by WriteDeliverablesToSession), then falls back to
// extracting from the team output directly.
// Domain: Delivery — collect and format upstream deliverables for downstream team input.
func (u *SpiritTeamUsecase) InjectUpstreamDeliverables(ctx context.Context, downstreamTeam Team) string {
	return u.delivery.InjectUpstreamDeliverables(ctx, downstreamTeam)
}

// DeliverableProtocolSuffix renders the mandatory delivery-protocol block
// appended to a DAG team's first-turn input (2026-07-25 Fix 2b). Without it a
// team has no way of knowing that "reply text is not a deliverable": the
// real-deliverable gate (Fix 1) flips completed-without-set_deliverable teams
// to failed, so the obligation must be declared up front. Non-DAG teams carry
// no deliverable obligation and get no suffix.
func (u *SpiritTeamUsecase) DeliverableProtocolSuffix(t Team) string {
	return u.delivery.DeliverableProtocolSuffix(t)
}

// BuildTeamTurnInput composes a DAG team's first-turn input:
// upstream deliverable prefix + task description + delivery protocol suffix.
// The stored Team.TaskDescription stays pure — injection happens only on the
// turn input (both the orchestrator dispatch path and the lazy DAG activation
// path compose through this single function so the two cannot drift).
func (u *SpiritTeamUsecase) BuildTeamTurnInput(ctx context.Context, t Team) string {
	return u.delivery.BuildTeamTurnInput(ctx, t)
}

// readDeliverableRef reads the persisted DeliverableRef for the team's own
// dag_node_id. Dual-mode: P2 envelopes parse with full metadata; legacy
// plain-string values yield a summary-only ref. ok=false when absent or
// unparseable.
// Domain: Delivery — read persisted deliverable envelope from team's cache.
func (u *SpiritTeamUsecase) readDeliverableRef(t Team) (DeliverableRef, bool) {
	return u.delivery.readDeliverableRef(t)
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
func (u *SpiritTeamUsecase) ReadUpstreamDeliverable(ctx context.Context, readerSessionID, teamID string, maxChars int) (UpstreamDeliverableContent, error) {
	return u.delivery.ReadUpstreamDeliverable(ctx, readerSessionID, teamID, maxChars)
}

// ReadUpstreamDeliverableKey returns ONE payload entry of a completed upstream
// team's deliverable (envelope v2 artifacts). Backs the key parameter of the
// read_upstream_deliverable tool: for long-form deliveries (e.g. an article
// the downstream team must publish), the injection prefix instructs the agent
// to fetch only the contracted payload key instead of the full concatenation.
// Domain: Delivery — keyed payload retrieval of an upstream team's deliverable.
func (u *SpiritTeamUsecase) ReadUpstreamDeliverableKey(ctx context.Context, readerSessionID, teamID, key string, maxChars int) (UpstreamDeliverableContent, error) {
	return u.delivery.ReadUpstreamDeliverableKey(ctx, readerSessionID, teamID, key, maxChars)
}

// ---------------------------------------------------------------------------
// XC-05: Escalation on Max Retries
// ---------------------------------------------------------------------------
// EscalateToSpirit marks a max-retry team failed (fail-closed). It does not
// inject a Spirit-session message; see SpiritOrchestration.EscalateToSpirit.
func (u *SpiritTeamUsecase) EscalateToSpirit(ctx context.Context, teamID string, tracker ReworkTracker) error {
	return u.orchestration.EscalateToSpirit(ctx, teamID, tracker)
}

// HandleTeamRejection is leftover (Running→Pending). Production dagRun must
// not call this — use EvaluateDeliverableQuality + revision enqueue.
func (u *SpiritTeamUsecase) HandleTeamRejection(ctx context.Context, teamID string, tracker ReworkTracker, reason string) (*ReworkTracker, error) {
	return u.orchestration.HandleTeamRejection(ctx, teamID, tracker, reason)
}

func (u *SpiritTeamUsecase) EvaluateDeliverableQuality(ctx context.Context, team Team) (QualityGateResult, error) {
	return u.delivery.EvaluateDeliverableQuality(ctx, team)
}
