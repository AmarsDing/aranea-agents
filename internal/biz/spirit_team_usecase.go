package biz

import (
	"context"
	"time"
	"unicode/utf8"

	"aranea-agents/internal/biz/session"
	"aranea-agents/pkg/loggateway"
)

type TeamStarterPort interface {
	StartTeamTurn(ctx context.Context, sessionID string, content string) error
	HandleTeamTurnResult(ctx context.Context, spiritSessionID, teamID, status, errMsg string, chatSessionID string)
}

// SpiritTeamAssembler provides the team CRUD operations needed by SpiritTeamUsecase.
// Narrow interface over TeamUsecase to avoid injecting the full god object (O-4 fix).
// Stability:evolving
type SpiritTeamAssembler interface {
	Create(ctx context.Context, in Team) (Team, error)
	Get(ctx context.Context, id string) (Team, error)
	Update(ctx context.Context, id string, patch Team) (Team, error)
	TransitionStatus(ctx context.Context, id string, newStatus string) (Team, error)
	ListBySpiritSessionID(ctx context.Context, spiritSessionID string) ([]Team, error)
	BatchArchiveTeams(ctx context.Context, ids []string) (int, error)
	ListRuns(ctx context.Context, teamID string, limit int) ([]TeamRunRecord, error)
}

// SpiritSessionAccessor provides the session operations needed by SpiritTeamUsecase.
// Narrow interface over SessionUsecase to avoid injecting the full god object (O-4 fix).
// Stability:evolving
type SpiritSessionAccessor interface {
	Get(ctx context.Context, id string) (Session, error)
	Create(ctx context.Context, in Session) (Session, error)
	Search(ctx context.Context, q SessionSearchQuery) (SessionListResult, error)
	ListMessagesRecent(ctx context.Context, sessionID string, limit int) ([]ChatMessage, error)
	ListChildSessions(ctx context.Context, parentSessionID string) ([]Session, error)
}

// SpiritAgentResolver provides the agent query operations needed by SpiritTeamUsecase.
// Narrow interface over AgentUsecase to avoid injecting the full god object (O-4 fix).
// Stability:evolving
type SpiritAgentResolver interface {
	List(ctx context.Context, q AgentListQuery) (AgentListResult, error)
}

// SpiritStepReader provides the step queries needed by SpiritTeamUsecase for
// deliverable extraction. Narrow subset of StepV2Reader with exact session_id
// semantics (O-4 fix). Implemented by data.StepV2Repo.
// Stability:evolving
type SpiritStepReader interface {
	ListStepsBySessionID(ctx context.Context, sessionID string) ([]Step, error)
}

// SpiritGraphDeliverableReader reads the graph final-state "deliverable" map
// of a completed team's trpc session (B.10.15.4 Graph StateFields bridge).
// appName/userID/sessionID form the trpc session key: appName is the team's
// anchor (manager) agent ID, NOT the default app scope.
// Returns (nil, nil) when the session or the deliverable key is absent.
// Stability:evolving
type SpiritGraphDeliverableReader interface {
	ReadGraphDeliverable(ctx context.Context, appName, userID, sessionID string) (map[string]any, error)
}

// SpiritTeamController exposes the methods needed by the service layer's
// TeamOrchestrationDeps for team lifecycle orchestration (timeout, completion,
// dependency scheduling, and completion checks).
type SpiritTeamController interface {
	CancelTimeoutTimer(teamID string)
	// RegisterTeamTimeout starts the team-level AfterFunc clock. Must be
	// called when the team actually starts running (StartTeamTurn), not at
	// AssembleTeam — pending DAG dependents must not time out while waiting
	// on upstream teams.
	RegisterTeamTimeout(ctx context.Context, team Team)
	RecordTeamCompletion(ctx context.Context, team Team, durationMs int64) (dqScore float64, topology TopologyType)
	ScheduleDependentTeams(ctx context.Context, spiritSessionID string, completedTeam Team) []DependentTeamAction
	CheckAllTeamsCompleted(ctx context.Context, spiritSessionID string) AllTeamsCompletedResult
	GetParallelConfig(ctx context.Context, spiritSessionID string) ParallelConfig
	AutoArchiveCompletedTeams(ctx context.Context, spiritSessionID string)
	// ReadUpstreamDeliverable backs the read_upstream_deliverable tool (P2):
	// full-text retrieval of a completed upstream team's deliverable.
	// readerSessionID identifies the calling (downstream) team's main session;
	// when resolvable, the reader team's InputContract is validated against the
	// upstream team's declared Deliverables before the read (Phase B).
	ReadUpstreamDeliverable(ctx context.Context, readerSessionID, teamID string, maxChars int) (UpstreamDeliverableContent, error)
	// ReadUpstreamDeliverableKey backs the key parameter of the
	// read_upstream_deliverable tool (envelope v2): single-payload retrieval
	// for long-form deliveries (e.g. an article the downstream must publish).
	ReadUpstreamDeliverableKey(ctx context.Context, readerSessionID, teamID, key string, maxChars int) (UpstreamDeliverableContent, error)
	// HasRealDeliverable backs the service-layer deliverable gate
	// (2026-07-25 Fix 1): a completed-callback DAG team without a graph
	// state deliverable is flipped to failed BEFORE any status transition,
	// so "no deliverable" can never masquerade as success.
	HasRealDeliverable(ctx context.Context, team Team) (bool, error)
	// EvaluateDeliverableQuality backs the runner-side quality gate
	// (G3/ADR-G): rule-based content-quality verdict (pass/revise/fail) for
	// a DAG team's own deliverable, consulted after the binary gate passes.
	EvaluateDeliverableQuality(ctx context.Context, team Team) (QualityGateResult, error)
	// ListFailedTeamBriefs backs the honest synthesis trigger (2026-07-25
	// Fix 3): briefs for genuinely failed teams (cancelled excluded),
	// feeding the summary report with failure reasons and the teams'
	// unresolved questions (their last replies).
	ListFailedTeamBriefs(ctx context.Context, spiritSessionID string) []TeamFailureBrief
	// ListTeamDeliverableDigests backs the F7 (Phase 11) structured
	// synthesis trigger: per-terminal-team deliverable summaries inlined
	// into the system-push message.
	ListTeamDeliverableDigests(ctx context.Context, spiritSessionID string) []TeamDeliverableDigest
	// MemberExecutionEvidence backs F10 (Phase 11) outcome-oriented member
	// status: per-member failure evidence (interrupted session / failed or
	// cancelled step) overrides the team-level completed status in the
	// member projection — status follows the execution RESULT, not the
	// message lifecycle.
	MemberExecutionEvidence(ctx context.Context, sessionID string) (failed bool, reason string)
	// MemberExecutionWindow backs the member-duration fix (2026-08-08 问题4):
	// a member's real execution window aggregates its step stream — start is
	// the earliest StartedAt, end the latest activity evidence (CompletedAt
	// preferred, StartedAt fallback) — member session first, team session
	// (coordinator mode, AuthorAgentKey-attributed) as fallback. The terminal
	// MemberSession event must carry this window instead of the publish
	// timestamp.
	// ok=false means no step evidence (caller falls back to the publish
	// timestamp). end may be zero when steps carry no timestamps at all.
	MemberExecutionWindow(ctx context.Context, sessionID string) (start, end time.Time, ok bool)
	// UpstreamDeliverableSeed backs the runner-side cross-team deliverable
	// seeding (2026-08-08 问题3c): merged business topics of completed upstream
	// dependencies' graph deliverables, injected into the downstream DAG team's
	// initial graph state at turn start. See SpiritTeamUsecase.UpstreamDeliverableSeed.
	UpstreamDeliverableSeed(ctx context.Context, downstreamTeam Team) (map[string]any, error)
	// ExecuteVerificationGates backs the F9 (Phase 11) verification gate in
	// the service-layer outcome pass (2026-07-28): runs the team's definition
	// verification_gates (current automatic source: skill-install
	// tool_assertion gate) after the deliverable gate passes. Rejection or
	// infra error is fail-closed → the team is flipped to failed, because
	// "installed but not usable" must never masquerade as success.
	ExecuteVerificationGates(ctx context.Context, teamID string, teamOutput string) (bool, []string, error)
}

// TimeoutHandler is called when a team times out. Implemented by the service
// layer to trigger dependency scheduling, event publishing, and AllDone checks.
type TimeoutHandler interface {
	HandleTeamTimeout(ctx context.Context, spiritSessionID, teamID string)
}

// AllTeamsCompletedNotifier is called by the background poller when all teams
// for a spirit session have reached a terminal state. This provides the
// "active notification" path for team completion, supplementing the
// event-driven path (HandleTeamTurnResult → checkAllTeamsCompleted).
// Implemented by the service layer to publish events and trigger synthesis.
type AllTeamsCompletedNotifier interface {
	NotifyAllTeamsCompleted(ctx context.Context, spiritSessionID string)
}

type SpiritTeamParams struct {
	SpiritSessionID         string
	TaskDescription         string
	AgentKeys               []string
	Mode                    string
	DagNodeID               string
	TeamName                string
	TaskSummary             string
	DependsOn               []string
	ParallelConfigJSON      string
	TopologyReason          string
	AutoStart               bool
	DepartmentID            string   // home department for the team
	CrossDeptMemberAgentIDs []string // agent IDs from other departments requiring borrow approval
	// P1 形式契约（B.10.15.2）：dagRun 派发时从 PlanStep 透传；
	// AssembleTeam 序列化落库到 Team 记录，供契约验证与下游注入读取。
	Deliverables  []DeliverableContract
	InputContract []DeliverableContract
	// GraphTemplateID optionally routes this team through an existing M53
	// template (playbook stage). Empty = ordinary Team Turn.
	GraphTemplateID string
	// CollectionIDs scope member knowledge tools for this stage.
	CollectionIDs []string
}

type SpiritTeamResult struct {
	Team           Team
	Session        Session
	MemberSessions map[string]string // agentKey → sessionID, for frontend lazy-loading member execution process
}

// SpiritTransactor executes a function within a single database transaction.
// Defined in biz to avoid direct data-layer dependency; implemented in data.
type SpiritTransactor interface {
	ExecInTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// SpiritTeamUsecaseOption configures a SpiritTeamUsecase during construction.
type SpiritTeamUsecaseOption func(*SpiritTeamUsecase)

func WithSpiritTransactor(t SpiritTransactor) SpiritTeamUsecaseOption {
	return func(u *SpiritTeamUsecase) { u.assembly.transactor = t }
}

func WithOrchestrationCache(c *OrchestrationCache) SpiritTeamUsecaseOption {
	return func(u *SpiritTeamUsecase) { u.orchestration.orchCache = c }
}

func WithEvolutionSuggestionCreator(c EvolutionSuggestionCreator) SpiritTeamUsecaseOption {
	return func(u *SpiritTeamUsecase) { u.orchestration.evolutionSugg = c }
}

func WithVerificationGateExecutor(e *VerificationGateExecutor) SpiritTeamUsecaseOption {
	return func(u *SpiritTeamUsecase) { u.delivery.gateExecutor = e }
}

func WithDeptLeadMgr(m *DeptLeadManager) SpiritTeamUsecaseOption {
	return func(u *SpiritTeamUsecase) { u.assembly.deptLeadMgr = m }
}

func WithSpiritStepReader(r SpiritStepReader) SpiritTeamUsecaseOption {
	return func(u *SpiritTeamUsecase) { u.delivery.stepReader = r }
}

// WithGraphDeliverableReader injects the graph final-state deliverable reader
// for the B.10.15.4 bridge. Nil disables the bridge (reply extraction only).
func WithGraphDeliverableReader(r SpiritGraphDeliverableReader) SpiritTeamUsecaseOption {
	return func(u *SpiritTeamUsecase) { u.delivery.graphDelivReader = r }
}

// TeamInboxFS copies declared Bulk files into downstream member workspaces.
// Nil disables materialization (tests / v1-only). Stability:internal
type TeamInboxFS interface {
	MaterializeFile(ctx context.Context, spec InboxCopySpec) error
}

// InboxCopySpec describes one declared bulk file to copy.
type InboxCopySpec struct {
	SrcAgentKeys   []string
	DestAgentKeys  []string
	UpstreamTeamID string
	RelPath        string
	DestName       string
}

func WithTeamInboxFS(fs TeamInboxFS) SpiritTeamUsecaseOption {
	return func(u *SpiritTeamUsecase) {
		if u != nil && u.delivery != nil {
			u.delivery.inboxFS = fs
		}
	}
}

// WithSpiritDeptMailbox wires the dept mailbox for borrow negotiation
// notifications (P2). Nil = skip notification (backward compatible).
func WithSpiritDeptMailbox(mb *DeptMailboxUsecase) SpiritTeamUsecaseOption {
	return func(u *SpiritTeamUsecase) {
		if u != nil && u.assembly != nil {
			u.assembly.deptMailbox = mb
		}
	}
}

// SpiritTeamRunStats is the latest-run statistics for one team, used by the
// execution report (B.10.17) to enrich per-unit duration and error reason.
type SpiritTeamRunStats struct {
	TeamID       string
	DurationMs   int64
	ErrorMessage string
}

// SpiritTeamRunStatsReader reads latest team-run statistics in batch
// (team_runs_v2 ⋈ team_stages_v2, latest run per team).
//
// Stability:evolving
type SpiritTeamRunStatsReader interface {
	ListLatestRunStatsByTeams(ctx context.Context, teamIDs []string) (map[string]SpiritTeamRunStats, error)
}

// WithSpiritTeamRunStatsReader injects the per-team run stats reader used by
// the execution report (B.10.17). Nil omits per-unit duration/error fields.
func WithSpiritTeamRunStatsReader(r SpiritTeamRunStatsReader) SpiritTeamUsecaseOption {
	return func(u *SpiritTeamUsecase) { u.delivery.runStatsReader = r }
}

// SpiritTeamUsecase is the public facade for Spirit team lifecycle (DEV-09).
// Implementation is split across SpiritAssembly / SpiritOrchestration /
// SpiritDelivery; service and Wire keep constructing this type.
type SpiritTeamUsecase struct {
	assembly      *SpiritAssembly
	orchestration *SpiritOrchestration
	delivery      *SpiritDelivery
}

var _ SpiritTeamController = (*SpiritTeamUsecase)(nil)

func NewSpiritTeamUsecase(teamUC SpiritTeamAssembler, sessionUC SpiritSessionAccessor, agentUC SpiritAgentResolver, lg loggateway.Logger, opts ...SpiritTeamUsecaseOption) *SpiritTeamUsecase {
	delivery := &SpiritDelivery{teamUC: teamUC, sessionUC: sessionUC, agentUC: agentUC, lg: lg}
	orch := &SpiritOrchestration{
		teamUC:    teamUC,
		sessionUC: sessionUC,
		agentUC:   agentUC,
		timeouts:  &teamTimeoutRegistry{},
		delivery:  delivery,
		lg:        lg,
	}
	assembly := &SpiritAssembly{
		teamUC:    teamUC,
		sessionUC: sessionUC,
		agentUC:   agentUC,
		orch:      orch,
		lg:        lg,
	}
	u := &SpiritTeamUsecase{assembly: assembly, orchestration: orch, delivery: delivery}
	for _, opt := range opts {
		opt(u)
	}
	return u
}

type TeamProgress struct {
	TeamID      string  `json:"team_id"`
	TeamName    string  `json:"team_name"`
	Status      string  `json:"status"`
	ProgressPct float64 `json:"progress_pct"`
	CurrentStep string  `json:"current_step"`
	DurationMs  int64   `json:"duration_ms"`
}

// Spirit team definition constants.
// SpiritTeamDefVersion is the current version of spirit team definition JSON.
const (
	SpiritTeamDefVersion = 2
	// SpiritTeamDefaultTimeout: 600s proved too short for serial coordinator
	// teams (eval0831-s06-r3 produce team needed 626s and was killed mid-run).
	// 1800s covers media-production DAG stages; teamTurnMaxSeconds=7200 still caps.
	SpiritTeamDefaultTimeout = 1800
	SpiritTeamDefaultMaxConc = 2

	// Truncation limits for display strings.
	MaxTeamDisplayNameLen = 64
	MaxTeamTitleLen       = 128
	MaxSummaryLen         = 500
	MaxSuggestionTitleLen = 40
	MaxSpiritQueryLen     = 500

	// TimeoutHandlerContextTimeout is the maximum duration for DB operations
	// inside the timeout callback goroutine.
	// Deprecated: use ParallelConfig.TimeoutHandlerDBTimeout() instead.
	TimeoutHandlerContextTimeout = 30 * time.Second

	// MaxKeyFindingsCount is the maximum number of key findings extracted.
	MaxKeyFindingsCount = 5

	// InlineUpstreamPayloadMaxChars is the inline-vs-pointer threshold for
	// upstream artifact payloads in the DAG injection prefix: payloads at or
	// below this size are inlined in full (zero tool calls for the downstream
	// team); larger payloads render as a pointer plus a keyed
	// read_upstream_deliverable retrieval instruction.
	InlineUpstreamPayloadMaxChars = 2000

	// MaxEnvelopeStructuredPayloadChars is the max rune count of a topic's
	// content that may be copied into DeliverableRef.StructuredJSON (M78).
	// Larger text stays in graph state and is fetched via
	// read_upstream_deliverable; file/binary payloads never land here.
	MaxEnvelopeStructuredPayloadChars = InlineUpstreamPayloadMaxChars

	// InlineUpstreamArtifactMaxCount caps how many payloads of ONE upstream
	// team may render inline in the injection prefix; the rest degrade to
	// pointers. Bounds the "payload count × per-payload size" product the
	// per-payload threshold alone cannot (2026-08-15 review).
	InlineUpstreamArtifactMaxCount = 5
	// InlineUpstreamPayloadTotalMaxChars caps the combined inline size of one
	// upstream team's payloads in the injection prefix; once the budget is
	// spent, remaining small payloads degrade to pointers. Guards the
	// downstream first-turn input against multi-topic prefix bloat.
	InlineUpstreamPayloadTotalMaxChars = 8000

	// DefaultUpstreamDeliverableMaxChars is the default full-text budget for
	// ReadUpstreamDeliverable when maxChars is unset/invalid.
	DefaultUpstreamDeliverableMaxChars = 50000
	// MaxUpstreamDeliverableChars is the hard cap for a single full-text read
	// (defense against runaway context consumption).
	MaxUpstreamDeliverableChars = 200000
)

func TruncateRunes(s string, maxLen int) string {
	if utf8.RuneCountInString(s) <= maxLen {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxLen-3]) + "..."
}

// DependentTeamAction represents an action to take on a dependent team.
type DependentTeamAction struct {
	TeamID    string
	TeamName  string
	DagNodeID string
	Action    string // "activate" or "fail"
	Reason    string
	// TaskDescription is the full input for the activated team's first turn:
	// upstream deliverable prefix (if any) + the team's own task description.
	// Empty for "fail" actions. (P0-③b)
	TaskDescription string
}

// AllTeamsCompletedResult holds the result of checking if all teams are completed.
type AllTeamsCompletedResult struct {
	AllDone        bool
	TeamIDs        []string
	TotalTeams     int
	CompletedTeams int
	FailedTeams    int
	// CancelledTeams counts teams in cancelled state (subset of FailedTeams).
	// B.10.17: >0 skips the execution report and the synthesis summary turn —
	// user-initiated interruption must not produce a report.
	CancelledTeams int
	TotalTokenIn   int
	TotalTokenOut  int
}

// UpstreamDeliverableContent is the full-text result of ReadUpstreamDeliverable
// (the read_upstream_deliverable tool's biz-layer response).
type UpstreamDeliverableContent struct {
	Content   string // deliverable text, truncated to the requested budget
	SizeChars int    // FULL content size in runes (before truncation)
	Truncated bool   // true when Content was cut to the budget
	TeamID    string
	SessionID string // team main session the content was read from
}

// ---------------------------------------------------------------------------
// Facade delegates (public API unchanged)
// ---------------------------------------------------------------------------

// ListTeamRunStats returns per-team latest-run stats for report enrichment.
// Returns nil when the stats reader is not wired (v1-only deployments).
func (u *SpiritTeamUsecase) ListTeamRunStats(ctx context.Context, teamIDs []string) map[string]SpiritTeamRunStats {
	return u.delivery.ListTeamRunStats(ctx, teamIDs)
}

// SetTimeoutHandler injects the service-layer timeout handler.
// Called after construction to break the circular dependency:
// SpiritTeamUsecase → TimeoutHandler → TeamStarter → SpiritTeamController → SpiritTeamUsecase.
// This is a justified exception like L4GraphUsecase.SetCascade.
// Uses sync.Once to ensure the handler is set exactly once.
func (u *SpiritTeamUsecase) SetTimeoutHandler(h TimeoutHandler) {
	u.orchestration.SetTimeoutHandler(h)
}

// SetAllTeamsCompletedNotifier injects the service-layer completion notifier.
// Called by the background poller when all teams for a spirit session reach
// terminal state. This is the "active notification" path.
func (u *SpiritTeamUsecase) SetAllTeamsCompletedNotifier(n AllTeamsCompletedNotifier) {
	u.orchestration.SetAllTeamsCompletedNotifier(n)
}

// StartBackgroundPolling starts a background goroutine that periodically
// checks all active spirit sessions for team completion. This supplements
// the event-driven path (HandleTeamTurnResult) with a moderate-frequency
// backup to catch cases where completion events are missed.
//
// Default interval: 30 seconds. This is backend logic and does not generate
// frontend-visible activity events.
func (u *SpiritTeamUsecase) StartBackgroundPolling(ctx context.Context, interval time.Duration) {
	u.orchestration.StartBackgroundPolling(ctx, interval)
}

// Domain: Assembly — team creation and composition.
func (u *SpiritTeamUsecase) AssembleTeam(ctx context.Context, params SpiritTeamParams) (SpiritTeamResult, error) {
	return u.assembly.AssembleTeam(ctx, params)
}

func (u *SpiritTeamUsecase) GetTeam(ctx context.Context, teamID string) (Team, error) {
	return u.assembly.GetTeam(ctx, teamID)
}

func (u *SpiritTeamUsecase) ListActiveTeams(ctx context.Context, spiritSessionID string) ([]Team, error) {
	return u.assembly.ListActiveTeams(ctx, spiritSessionID)
}

func (u *SpiritTeamUsecase) ListAllTeams(ctx context.Context, spiritSessionID string) ([]Team, error) {
	return u.assembly.ListAllTeams(ctx, spiritSessionID)
}

func (u *SpiritTeamUsecase) ListCompletedAndFailedTeams(ctx context.Context, spiritSessionID string) ([]Team, error) {
	return u.assembly.ListCompletedAndFailedTeams(ctx, spiritSessionID)
}

func (u *SpiritTeamUsecase) BuildCascadeBlockedResults(ctx context.Context, teams []Team) []TeamSynthesisResult {
	return u.orchestration.BuildCascadeBlockedResults(ctx, teams)
}

func (u *SpiritTeamUsecase) GetMaxParallelTeams(ctx context.Context, spiritSessionID string) int {
	return u.orchestration.GetMaxParallelTeams(ctx, spiritSessionID)
}

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
