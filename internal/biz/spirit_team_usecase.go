package biz

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"aranea-agents/internal/biz/session"
	"aranea-agents/internal/biz/shared"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/ctxuser"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	"github.com/google/uuid"
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
	// HasRealDeliverable backs the service-layer deliverable gate
	// (2026-07-25 Fix 1): a completed-callback DAG team without a graph
	// state deliverable is flipped to failed BEFORE any status transition,
	// so "no deliverable" can never masquerade as success.
	HasRealDeliverable(ctx context.Context, team Team) (bool, error)
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
	return func(u *SpiritTeamUsecase) { u.transactor = t }
}

func WithOrchestrationCache(c *OrchestrationCache) SpiritTeamUsecaseOption {
	return func(u *SpiritTeamUsecase) { u.orchCache = c }
}

func WithEvolutionSuggestionRepo(r EvolutionSuggestionRepo) SpiritTeamUsecaseOption {
	return func(u *SpiritTeamUsecase) { u.evolutionSugg = r }
}

func WithVerificationGateExecutor(e *VerificationGateExecutor) SpiritTeamUsecaseOption {
	return func(u *SpiritTeamUsecase) { u.gateExecutor = e }
}

func WithDeptLeadMgr(m *DeptLeadManager) SpiritTeamUsecaseOption {
	return func(u *SpiritTeamUsecase) { u.deptLeadMgr = m }
}

func WithSpiritStepReader(r SpiritStepReader) SpiritTeamUsecaseOption {
	return func(u *SpiritTeamUsecase) { u.stepReader = r }
}

// WithGraphDeliverableReader injects the graph final-state deliverable reader
// for the B.10.15.4 bridge. Nil disables the bridge (reply extraction only).
func WithGraphDeliverableReader(r SpiritGraphDeliverableReader) SpiritTeamUsecaseOption {
	return func(u *SpiritTeamUsecase) { u.graphDelivReader = r }
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
	return func(u *SpiritTeamUsecase) { u.runStatsReader = r }
}

// ListTeamRunStats returns per-team latest-run stats for report enrichment.
// Returns nil when the stats reader is not wired (v1-only deployments).
func (u *SpiritTeamUsecase) ListTeamRunStats(ctx context.Context, teamIDs []string) map[string]SpiritTeamRunStats {
	if u.runStatsReader == nil || len(teamIDs) == 0 {
		return nil
	}
	stats, err := u.runStatsReader.ListLatestRunStatsByTeams(ctx, teamIDs)
	if err != nil {
		u.lg.Warn("查询团队运行统计失败，省略 per-unit 耗时/错误字段",
			loggateway.StepID("spirit.teams.run_stats_err"),
			loggateway.Err(err),
		)
		return nil
	}
	return stats
}

// SpiritTeamUsecase manages Spirit team lifecycle.
// TECH-DEBT(COG): file_lines=1431, limit=500; sync_maps=1, limit=0; needs decomposition into sub-Usecases
// TODO(debt): DEV-09 — Split into three sub-Usecases:
//   - SpiritTeamAssemblyUsecase: assembly + creation (AssembleTeam, InjectDeptLeadIntoTeam, UpdateTeamDefinitionJSON, etc.)
//   - SpiritTeamOrchestrationUsecase: DAG scheduling + timeout + completion (ScheduleDependentTeams, registerTeamTimeout, RecordTeamCompletion, etc.)
//   - SpiritTeamDeliveryUsecase: deliverable passing + verification gate (WriteDeliverablesToSession, ExecuteVerificationGates, etc.)
//
// Current plan: Define interfaces first, then gradually move methods to sub-Usecases
// while keeping SpiritTeamUsecase as a facade during migration.
type SpiritTeamUsecase struct {
	_                 SpiritTeamController // interface assertion
	teamUC            SpiritTeamAssembler
	sessionUC         SpiritSessionAccessor
	agentUC           SpiritAgentResolver
	transactor        SpiritTransactor
	orchCache         *OrchestrationCache
	evolutionSugg     EvolutionSuggestionRepo
	timeoutHandler    TimeoutHandler
	contractValidator *DeliverableContractValidator
	gateExecutor      *VerificationGateExecutor
	deptLeadMgr       *DeptLeadManager
	stepReader        SpiritStepReader
	graphDelivReader  SpiritGraphDeliverableReader
	runStatsReader    SpiritTeamRunStatsReader
	lg                loggateway.Logger

	timeoutOnce sync.Once

	// timeoutTimers tracks pending timeout callbacks so they can be cancelled
	// when a team completes normally before the timeout fires.
	timeoutTimers sync.Map // map[string]*time.Timer

	// completionNotifier is called by the background poller when all teams
	// for a spirit session are done. Set via SetAllTeamsCompletedNotifier.
	completionNotifier AllTeamsCompletedNotifier

	// pollCtx and pollCancel manage the background polling goroutine lifecycle.
	pollCtx    context.Context
	pollCancel context.CancelFunc
}

func NewSpiritTeamUsecase(teamUC SpiritTeamAssembler, sessionUC SpiritSessionAccessor, agentUC SpiritAgentResolver, lg loggateway.Logger, opts ...SpiritTeamUsecaseOption) *SpiritTeamUsecase {
	u := &SpiritTeamUsecase{teamUC: teamUC, sessionUC: sessionUC, agentUC: agentUC, lg: lg}
	for _, opt := range opts {
		opt(u)
	}
	return u
}

// SetTimeoutHandler injects the service-layer timeout handler.
// Called after construction to break the circular dependency:
// SpiritTeamUsecase → TimeoutHandler → TeamStarter → SpiritTeamController → SpiritTeamUsecase.
// This is a justified exception like L4GraphUsecase.SetCascade.
// Uses sync.Once to ensure the handler is set exactly once.
func (u *SpiritTeamUsecase) SetTimeoutHandler(h TimeoutHandler) {
	u.timeoutOnce.Do(func() {
		u.timeoutHandler = h
	})
}

// SetAllTeamsCompletedNotifier injects the service-layer completion notifier.
// Called by the background poller when all teams for a spirit session reach
// terminal state. This is the "active notification" path.
func (u *SpiritTeamUsecase) SetAllTeamsCompletedNotifier(n AllTeamsCompletedNotifier) {
	u.completionNotifier = n
}

// StartBackgroundPolling starts a background goroutine that periodically
// checks all active spirit sessions for team completion. This supplements
// the event-driven path (HandleTeamTurnResult) with a moderate-frequency
// backup to catch cases where completion events are missed.
//
// Default interval: 30 seconds. This is backend logic and does not generate
// frontend-visible activity events.
func (u *SpiritTeamUsecase) StartBackgroundPolling(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	u.pollCtx, u.pollCancel = context.WithCancel(ctx)
	safego.Go(u.pollCtx, "spirit-team-completion-poller", func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-u.pollCtx.Done():
				return
			case <-ticker.C:
				u.pollTeamCompletions(u.pollCtx)
			}
		}
	})
	u.lg.Info("spirit team completion poller started",
		loggateway.StepID("spirit.poller.started"),
		loggateway.Str("interval", interval.String()),
	)
}

// pollTeamCompletions scans all running sessions and checks if all their
// teams have reached terminal state. When all done, it notifies the service
// layer via completionNotifier.
//
// This is a moderate-frequency backup for the event-driven path
// (HandleTeamTurnResult → checkAllTeamsCompleted). The polling itself does
// not generate frontend-visible activity events.
func (u *SpiritTeamUsecase) pollTeamCompletions(ctx context.Context) {
	sessions, err := u.sessionUC.Search(ctx, SessionSearchQuery{
		Status: string(session.SessionStatusRunning),
		Limit:  100,
	})
	if err != nil {
		u.lg.Warn("spirit poller: failed to search running sessions",
			loggateway.StepID("spirit.poller.search_err"),
			loggateway.Err(err),
		)
		return
	}
	for _, sess := range sessions.Items {
		if sess.ID == "" {
			continue
		}
		result := u.CheckAllTeamsCompleted(ctx, sess.ID)
		if !result.AllDone {
			continue
		}
		u.lg.Info("spirit poller: all teams completed for session",
			loggateway.StepID("spirit.poller.all_done"),
			loggateway.Str("spirit_session_id", sess.ID),
			loggateway.Int("total_teams", result.TotalTeams),
		)
		if u.completionNotifier != nil {
			u.completionNotifier.NotifyAllTeamsCompleted(ctx, sess.ID)
		}
	}
}

// Domain: Assembly — team creation and composition.
func (u *SpiritTeamUsecase) AssembleTeam(ctx context.Context, params SpiritTeamParams) (SpiritTeamResult, error) {
	spiritSessionID := strings.TrimSpace(params.SpiritSessionID)
	if spiritSessionID == "" {
		return SpiritTeamResult{}, apierror.BadRequest("SPIRIT", "spirit_session_id is required")
	}
	taskDesc := strings.TrimSpace(params.TaskDescription)
	if taskDesc == "" {
		return SpiritTeamResult{}, apierror.BadRequest("SPIRIT", "task_description is required")
	}
	teamName := strings.TrimSpace(params.TeamName)
	if teamName == "" {
		teamName = taskDesc
	}
	mode := strings.TrimSpace(params.Mode)
	if mode == "" {
		mode = TeamModeCoordinator
	}

	// Team/Agent reuse (Issue 5): before creating a new team, check if there's
	// an existing reusable team for the same spirit session + task description.
	// A team is reusable when:
	//   - Same spirit_session_id + same task_description (case-insensitive)
	//     OR same dag_node_id (when non-empty, for DAG-based orchestration)
	//   - Status is pending or running (not terminal)
	//   - Not deleted
	// This prevents duplicate team/agent creation when the same task is
	// re-executed (e.g. user retries, orchestrator re-runs), which was causing
	// management bloat.
	if reusable, ok := u.findReusableTeam(ctx, spiritSessionID, taskDesc, params.DagNodeID); ok {
		u.lg.Info("AssembleTeam 命中可复用团队，跳过创建",
			loggateway.StepID("spirit.assemble.reuse"),
			loggateway.Str("team_id", reusable.Team.ID),
			loggateway.Str("task_description", taskDesc),
		)
		return reusable, nil
	}

	// Resolve agentKeys → agent IDs. The team definition JSON's member.agent_id
	// field must hold the real agent ID (e.g. "agent___spirit__" or UUID), not
	// the agentKey (e.g. "__spirit__"), because validateTeamMembersExist uses
	// AgentExistsByID to verify each member.
	keyToID, resolveErr := u.resolveAgentKeyToIDMap(ctx, params.AgentKeys)
	if resolveErr != nil {
		return SpiritTeamResult{}, apierror.Wrap(resolveErr, apierror.CodeInternal, "SPIRIT")
	}

	agentIDs := make([]string, 0, len(params.AgentKeys))
	for _, key := range params.AgentKeys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if id, ok := keyToID[key]; ok {
			agentIDs = append(agentIDs, id)
		}
	}

	defJSON := buildSpiritTeamDefinitionJSON(mode, agentIDs, u.lg, params.ParallelConfigJSON)

	// Check session tree depth limit before creating team (P1-4: extracted
	// to session.ValidateDepth for reuse across all child-session creators).
	cfg := u.resolveParallelConfig(ctx, spiritSessionID)
	parentSession, err := u.sessionUC.Get(ctx, spiritSessionID)
	if err != nil {
		return SpiritTeamResult{}, apierror.Wrap(err, apierror.CodeInternal, "SPIRIT")
	}
	childDepth := parentSession.AgentDepth + 1
	if verr := ValidateDepth(parentSession, childDepth, DepthValidationConfig{
		SpiritMaxDepth: cfg.MaxSessionDepth,
		// Spirit creating a team — no agent-level relative depth applies
		// because spirit is not an agent session (MemberAgentKey empty).
	}); verr != nil {
		return SpiritTeamResult{}, apierror.BadRequest("SPIRIT", verr.Error())
	}

	// All teams start as pending regardless of depends_on.
	// AutoStart=true teams transition to running when StartTeamTurn is called.
	// AutoStart=false DAG root nodes stay pending until manually started or
	// scheduled by scheduleDependentTeams.
	initialStatus := TeamStatusPending

	var result SpiritTeamResult
	err = u.transactor.ExecInTx(ctx, func(txCtx context.Context) error {
		team, err := u.teamUC.Create(txCtx, Team{
			TeamKey:            fmt.Sprintf("spirit_%s_%s", spiritSessionID, uuid.New().String()[:8]),
			DisplayName:        TruncateRunes(teamName, MaxTeamDisplayNameLen),
			Status:             initialStatus,
			SpiritSessionID:    spiritSessionID,
			TaskDescription:    taskDesc,
			AutoCreated:        true,
			DefinitionJSON:     defJSON,
			DagNodeID:          params.DagNodeID,
			DependsOn:          params.DependsOn,
			ParallelConfigJSON: params.ParallelConfigJSON,
			Topology:           mode,
			DepartmentID:       params.DepartmentID,
			Deliverables:       DeliverableContractsToJSON(params.Deliverables),
			InputContract:      DeliverableContractsToJSON(params.InputContract),
		})
		if err != nil {
			return apierror.Wrap(err, apierror.CodeInternal, "SPIRIT")
		}

		teamSession, err := u.sessionUC.Create(txCtx, Session{
			OwnerType:       "team",
			SessionType:     string(SessionTypeTeam),
			TeamID:          team.ID,
			ParentSessionID: spiritSessionID,
			RootSessionID:   spiritSessionID,
			AgentDepth:      childDepth,
			Title:           TruncateRunes(teamName, MaxTeamTitleLen),
		})
		if err != nil {
			return apierror.Wrap(err, apierror.CodeInternal, "SPIRIT")
		}

		// Phase 2 (P0-4): create one agent session per team member.
		// Each member's activity records are attributed to its agent session,
		// enabling session-tree isolation and per-member activity streams.
		// Depth: spirit(0) → team(1) → agent(2). Guard against MaxSessionDepth
		// via ValidateDepth (P1-4) to avoid creating agent sessions deeper
		// than the spirit config allows.
		agentDepth := childDepth + 1
		if verr := ValidateDepth(teamSession, agentDepth, DepthValidationConfig{
			SpiritMaxDepth: cfg.MaxSessionDepth,
		}); verr != nil {
			u.lg.Warn("跳过成员 agent session 创建：深度超限",
				loggateway.StepID("spirit.assemble.agent_session.depth"),
				loggateway.Str("team_id", team.ID),
				loggateway.Int("agent_depth", agentDepth),
				loggateway.Int("spirit_max_depth", cfg.MaxSessionDepth))
		} else {
			memberSessions := make(map[string]string, len(params.AgentKeys))
			for _, agentKey := range params.AgentKeys {
				agentKey = strings.TrimSpace(agentKey)
				if agentKey == "" {
					continue
				}
				if IsSystemAgentKey(agentKey) {
					u.lg.Warn("AssembleTeam 跳过系统 Agent",
						loggateway.StepID("spirit.assemble.system_agent_skip"),
						loggateway.Str("team_id", team.ID),
						loggateway.Str("agent_key", agentKey))
					continue
				}
				agentID, ok := keyToID[agentKey]
				if !ok {
					u.lg.Warn("AssembleTeam 跳过成员：未解析到 agentID",
						loggateway.StepID("spirit.assemble.agent_session.no_id"),
						loggateway.Str("team_id", team.ID),
						loggateway.Str("agent_key", agentKey))
					continue
				}
				agentSession, aerr := u.sessionUC.Create(txCtx, Session{
					OwnerType:       "agent",
					AgentID:         agentID,
					SessionType:     string(SessionTypeAgent),
					TeamID:          team.ID,
					ParentSessionID: teamSession.ID,
					RootSessionID:   spiritSessionID,
					AgentDepth:      agentDepth,
					MemberAgentKey:  agentKey,
					Title:           TruncateRunes(teamName, MaxTeamTitleLen),
				})
				if aerr != nil {
					u.lg.Warn("创建成员 agent session 失败",
						loggateway.StepID("spirit.assemble.agent_session"),
						loggateway.Str("team_id", team.ID),
						loggateway.Str("agent_key", agentKey),
						loggateway.Err(aerr))
					// Continue rather than fail: team can still run without
					// per-member sessions; activities will fall back to team session.
				} else {
					memberSessions[agentKey] = agentSession.ID
				}
			}
			result = SpiritTeamResult{Team: team, Session: teamSession, MemberSessions: memberSessions}
		}

		if result.MemberSessions == nil {
			result = SpiritTeamResult{Team: team, Session: teamSession}
		}
		return nil
	})
	if err != nil {
		return SpiritTeamResult{}, err
	}

	// Register team timeout callback if configured.
	u.registerTeamTimeout(ctx, cfg, result.Team.ID)

	// Submit borrow requests for cross-department members (DL-09).
	// These are processed outside the transaction to avoid long-held locks.
	if u.deptLeadMgr != nil && len(params.CrossDeptMemberAgentIDs) > 0 && params.DepartmentID != "" {
		u.submitBorrowRequests(ctx, result.Team.ID, params.DepartmentID, params.CrossDeptMemberAgentIDs)
	}

	// Inject dept lead into team (DL-06): explicit injection point in the
	// Spirit orchestration flow. team_usecase.go auto-inherits DeptLeadAgentID
	// during Create, but this serves as the explicit step per design Phase 3.
	if params.DepartmentID != "" {
		if injectErr := u.InjectDeptLeadIntoTeam(ctx, result.Team.ID); injectErr != nil {
			u.lg.Warn("注入部门主管失败",
				loggateway.StepID("spirit.assemble.inject_dept_lead"),
				loggateway.Str("team_id", result.Team.ID),
				loggateway.Str("dept_id", params.DepartmentID),
				loggateway.Err(injectErr),
			)
		}
	}

	return result, nil
}

// findReusableTeam searches for an existing team that can be reused instead of
// creating a new one. A team is reusable when:
//   - Same spirit_session_id
//   - Same task_description (case-insensitive trimmed match) OR same dag_node_id
//     (when both caller and candidate have a non-empty dag_node_id)
//   - Status is pending or running (not terminal: completed/failed/cancelled/archived)
//
// Returns the team with its session and member sessions if found.
// This is a best-effort lookup: any query failure returns (zero, false) and the
// caller proceeds to create a new team.
func (u *SpiritTeamUsecase) findReusableTeam(
	ctx context.Context,
	spiritSessionID, taskDesc, dagNodeID string,
) (SpiritTeamResult, bool) {
	teams, err := u.teamUC.ListBySpiritSessionID(ctx, spiritSessionID)
	if err != nil {
		u.lg.Warn("findReusableTeam 查询团队列表失败，跳过复用",
			loggateway.StepID("spirit.assemble.reuse_list"),
			loggateway.Err(err),
		)
		return SpiritTeamResult{}, false
	}

	normalizedDesc := strings.ToLower(strings.TrimSpace(taskDesc))
	var matched *Team
	for i := range teams {
		t := &teams[i]
		if t.Status != TeamStatusPending && t.Status != TeamStatusRunning {
			continue
		}
		// DAG-based: match by dag_node_id when both are non-empty
		if dagNodeID != "" && t.DagNodeID != "" {
			if t.DagNodeID == dagNodeID {
				matched = t
				break
			}
			continue
		}
		// Non-DAG: match by task_description (case-insensitive)
		if normalizedDesc != "" && strings.ToLower(strings.TrimSpace(t.TaskDescription)) == normalizedDesc {
			matched = t
			break
		}
	}
	if matched == nil {
		return SpiritTeamResult{}, false
	}

	// Find the team session (OwnerType="team", TeamID=matched.ID) via child sessions.
	childSessions, err := u.sessionUC.ListChildSessions(ctx, spiritSessionID)
	if err != nil {
		u.lg.Warn("findReusableTeam 查询子 session 失败，跳过复用",
			loggateway.StepID("spirit.assemble.reuse_session"),
			loggateway.Str("team_id", matched.ID),
			loggateway.Err(err),
		)
		return SpiritTeamResult{}, false
	}

	var teamSession *Session
	for i := range childSessions {
		s := &childSessions[i]
		if s.TeamID == matched.ID && s.OwnerType == "team" {
			teamSession = s
			break
		}
	}
	if teamSession == nil {
		u.lg.Warn("findReusableTeam 未找到团队 session，跳过复用",
			loggateway.StepID("spirit.assemble.reuse_no_session"),
			loggateway.Str("team_id", matched.ID),
		)
		return SpiritTeamResult{}, false
	}

	// Find member agent sessions (OwnerType="agent", TeamID=matched.ID) via
	// child sessions of the team session.
	memberSessions := make(map[string]string)
	memberChildren, err := u.sessionUC.ListChildSessions(ctx, teamSession.ID)
	if err != nil {
		u.lg.Warn("findReusableTeam 查询成员 session 失败，跳过成员映射",
			loggateway.StepID("spirit.assemble.reuse_members"),
			loggateway.Str("team_id", matched.ID),
			loggateway.Err(err),
		)
		// Continue without member sessions — team can still be reused.
	} else {
		for i := range memberChildren {
			s := &memberChildren[i]
			if s.OwnerType == "agent" && s.MemberAgentKey != "" {
				memberSessions[s.MemberAgentKey] = s.ID
			}
		}
	}

	return SpiritTeamResult{
		Team:           *matched,
		Session:        *teamSession,
		MemberSessions: memberSessions,
	}, true
}

// submitBorrowRequests creates borrow requests for cross-department members.
// This is a best-effort operation: failures are logged but do not fail team creation.
// Domain: Assembly — cross-department borrow request submission.
func (u *SpiritTeamUsecase) submitBorrowRequests(ctx context.Context, teamID, homeDeptID string, crossDeptAgentIDs []string) {
	for _, agentID := range crossDeptAgentIDs {
		fromDeptID, err := u.deptLeadMgr.agentDepartment(ctx, agentID)
		if err != nil || fromDeptID == "" || fromDeptID == homeDeptID {
			continue // skip agents without a department or already in home dept
		}
		_, err = u.deptLeadMgr.SubmitBorrowRequest(ctx, BorrowRequest{
			TeamID:     teamID,
			AgentID:    agentID,
			FromDeptID: fromDeptID,
			ToDeptID:   homeDeptID,
			Reason:     "cross-department member for spirit team",
		})
		if err != nil {
			u.lg.Warn("failed to submit borrow request",
				loggateway.StepID("spirit.borrow.submit"),
				loggateway.Str("team_id", teamID),
				loggateway.Str("agent_id", agentID),
				loggateway.Err(err),
			)
		}
	}
}

// Domain: Orchestration — timeout registration for team execution.
func (u *SpiritTeamUsecase) registerTeamTimeout(ctx context.Context, cfg ParallelConfig, teamID string) {
	if cfg.TeamTimeoutSeconds <= 0 {
		return
	}
	// Use WithoutCancel to preserve trace/log context while detaching from request lifecycle.
	bgCtx := context.WithoutCancel(ctx)
	timer := time.AfterFunc(cfg.TeamTimeout(), func() {
		// If CancelTimeoutTimer already removed this entry, the team completed
		// normally and we should not interfere.
		if _, loaded := u.timeoutTimers.LoadAndDelete(teamID); !loaded {
			return
		}
		safego.Go(bgCtx, "spirit-team-timeout", func() {
			timeoutCtx, timeoutCancel := context.WithTimeout(bgCtx, cfg.TimeoutHandlerDBTimeout())
			defer timeoutCancel()
			team, err := u.teamUC.Get(timeoutCtx, teamID)
			if err != nil {
				return
			}
			if team.Status == TeamStatusCompleted || team.Status == TeamStatusFailed || team.Status == TeamStatusCancelled {
				return
			}
			u.lg.Warn("团队执行超时",
				loggateway.StepID("spirit.team.timeout"),
				loggateway.Str("team_id", teamID),
			)
			if _, err := u.teamUC.TransitionStatus(timeoutCtx, teamID, TeamStatusFailed); err != nil {
				u.lg.Warn("超时后转换团队状态失败",
					loggateway.StepID("spirit.team.timeout_transition_err"),
					loggateway.Str("team_id", teamID),
					loggateway.Err(err),
				)
				return
			}
			// Notify service layer to handle dependency scheduling, event
			// publishing, and AllDone checks — same lifecycle as a normal
			// team failure.
			if u.timeoutHandler != nil && team.SpiritSessionID != "" {
				u.timeoutHandler.HandleTeamTimeout(timeoutCtx, team.SpiritSessionID, teamID)
			}
		})
	})
	u.timeoutTimers.Store(teamID, timer)
}

func (u *SpiritTeamUsecase) GetTeam(ctx context.Context, teamID string) (Team, error) {
	return u.teamUC.Get(ctx, teamID)
}

func (u *SpiritTeamUsecase) ListActiveTeams(ctx context.Context, spiritSessionID string) ([]Team, error) {
	spiritSessionID = strings.TrimSpace(spiritSessionID)
	if spiritSessionID == "" {
		return nil, apierror.BadRequest("SPIRIT", "spirit_session_id is required")
	}
	teams, err := u.teamUC.ListBySpiritSessionID(ctx, spiritSessionID)
	if err != nil {
		return nil, err
	}
	var active []Team
	for i := range teams {
		s := teams[i].Status
		if IsTeamStatusActive(s) {
			active = append(active, teams[i])
		}
	}
	return active, nil
}

func (u *SpiritTeamUsecase) ListAllTeams(ctx context.Context, spiritSessionID string) ([]Team, error) {
	spiritSessionID = strings.TrimSpace(spiritSessionID)
	if spiritSessionID == "" {
		return nil, apierror.BadRequest("SPIRIT", "spirit_session_id is required")
	}
	return u.teamUC.ListBySpiritSessionID(ctx, spiritSessionID)
}

func (u *SpiritTeamUsecase) ListCompletedAndFailedTeams(ctx context.Context, spiritSessionID string) ([]Team, error) {
	spiritSessionID = strings.TrimSpace(spiritSessionID)
	if spiritSessionID == "" {
		return nil, apierror.BadRequest("SPIRIT", "spirit_session_id is required")
	}
	teams, err := u.teamUC.ListBySpiritSessionID(ctx, spiritSessionID)
	if err != nil {
		return nil, err
	}
	var out []Team
	for i := range teams {
		if teams[i].Status == TeamStatusCompleted || teams[i].Status == TeamStatusFailed {
			out = append(out, teams[i])
		}
	}
	return out, nil
}

func (u *SpiritTeamUsecase) BuildCascadeBlockedResults(ctx context.Context, teams []Team) []TeamSynthesisResult {
	var results []TeamSynthesisResult
	for i := range teams {
		t := teams[i]
		summary, keyFindings, extractErr := u.ExtractTeamOutput(ctx, t.ID)
		if extractErr != nil {
			u.lg.Warn("提取团队输出失败",
				loggateway.StepID("spirit.extract_output_err"),
				loggateway.Str("team_id", t.ID),
				loggateway.Err(extractErr),
			)
		}
		result := TeamSynthesisResult{
			TeamID:      t.ID,
			TeamName:    t.DisplayName,
			TaskName:    t.TaskDescription,
			Status:      t.Status,
			Summary:     summary,
			KeyFindings: keyFindings,
		}
		if t.Status == TeamStatusFailed {
			result.Summary = "[执行失败] " + result.Summary
		}
		results = append(results, result)
	}
	failedDagNodes := make(map[string]string)
	for i := range teams {
		if teams[i].Status == TeamStatusFailed && teams[i].DagNodeID != "" {
			failedDagNodes[teams[i].DagNodeID] = teams[i].DisplayName
		}
	}
	for i := range teams {
		if teams[i].Status != TeamStatusPending {
			continue
		}
		for _, depID := range teams[i].DependsOn {
			if failedName, ok := failedDagNodes[depID]; ok {
				results = append(results, TeamSynthesisResult{
					TeamID:   teams[i].ID,
					TeamName: teams[i].DisplayName,
					TaskName: teams[i].TaskDescription,
					Status:   TeamStatusBlocked,
					Summary:  fmt.Sprintf("被失败团队 %s 阻塞", failedName),
				})
				break
			}
		}
	}
	return results
}

func (u *SpiritTeamUsecase) GetMaxParallelTeams(ctx context.Context, spiritSessionID string) int {
	cfg := u.resolveParallelConfig(ctx, spiritSessionID)
	return cfg.MaxConcurrentTeams
}

func (u *SpiritTeamUsecase) GetParallelConfig(ctx context.Context, spiritSessionID string) ParallelConfig {
	return u.resolveParallelConfig(ctx, spiritSessionID)
}

func (u *SpiritTeamUsecase) resolveParallelConfig(ctx context.Context, spiritSessionID string) ParallelConfig {
	if u.agentUC == nil {
		return DefaultParallelConfig()
	}
	agents, err := u.agentUC.List(ctx, AgentListQuery{Keyword: SpiritAgentKey, Limit: SpiritAgentQueryLimit})
	if err != nil {
		u.lg.Error("查询精灵 Agent 失败，使用默认并行配置（用户自定义配置将失效）",
			loggateway.StepID("spirit.parallel_config"),
			loggateway.Str("spirit_session_id", spiritSessionID),
			loggateway.Err(err),
		)
		return DefaultParallelConfig()
	}
	if len(agents.Items) == 0 {
		u.lg.Error("精灵 Agent 不存在，使用默认并行配置（用户自定义配置将失效）",
			loggateway.StepID("spirit.parallel_config"),
			loggateway.Str("spirit_session_id", spiritSessionID),
		)
		return DefaultParallelConfig()
	}
	ag := agents.Items[0]
	// Read parallel_config from ConfigJSON (stored as a top-level key).
	// Previously attempted to read from MetadataJSON, but that field has no DB column.
	return ParseParallelConfig(ag.ConfigJSON, u.lg)
}

// Domain: Orchestration — cancel team and its timeout timer.
func (u *SpiritTeamUsecase) CancelTeam(ctx context.Context, teamID string) error {
	if strings.TrimSpace(teamID) == "" {
		return apierror.BadRequest("SPIRIT", "team_id is required")
	}
	u.CancelTimeoutTimer(teamID)
	_, err := u.teamUC.TransitionStatus(ctx, teamID, TeamStatusCancelled)
	if err != nil {
		return err
	}
	return nil
}

// Domain: Orchestration — auto-archive completed/failed teams past threshold.
func (u *SpiritTeamUsecase) AutoArchiveCompletedTeams(ctx context.Context, spiritSessionID string) {
	cfg := u.resolveParallelConfig(ctx, spiritSessionID)
	if cfg.AutoArchiveSeconds <= 0 {
		return
	}
	teams, err := u.teamUC.ListBySpiritSessionID(ctx, spiritSessionID)
	if err != nil {
		u.lg.Warn("查询精灵团队列表失败，跳过自动归档",
			loggateway.StepID("spirit.auto_archive.list_err"),
			loggateway.Str("spirit_session_id", spiritSessionID),
			loggateway.Err(err),
		)
		return
	}
	threshold := time.Now().Add(-cfg.AutoArchiveAfter())
	var archiveIDs []string
	for _, t := range teams {
		if t.Status != TeamStatusCompleted && t.Status != TeamStatusFailed && t.Status != TeamStatusCancelled {
			continue
		}
		updatedAt, parseErr := parseTimeFlexible(t.UpdatedAt)
		if parseErr != nil {
			u.lg.Warn("解析团队更新时间失败，使用兜底策略（视为可归档）",
				loggateway.StepID("spirit.auto_archive.parse_err"),
				loggateway.Str("team_id", t.ID),
				loggateway.Str("updated_at", t.UpdatedAt),
				loggateway.Err(parseErr),
			)
			// Fallback: if we can't parse the time, treat the team as eligible
			// for archiving rather than silently skipping it forever.
			updatedAt = time.Now().Add(-cfg.AutoArchiveAfter())
		}
		if updatedAt.Before(threshold) {
			archiveIDs = append(archiveIDs, t.ID)
		}
	}
	if len(archiveIDs) == 0 {
		return
	}
	archived, archiveErr := u.teamUC.BatchArchiveTeams(ctx, archiveIDs)
	if archiveErr != nil {
		u.lg.Warn("批量归档团队失败",
			loggateway.StepID("spirit.auto_archive.batch_err"),
			loggateway.Str("spirit_session_id", spiritSessionID),
			loggateway.Err(archiveErr),
		)
		return
	}
	if archived > 0 {
		u.lg.Info("批量归档团队完成",
			loggateway.StepID("spirit.auto_archive.batch_done"),
			loggateway.Str("spirit_session_id", spiritSessionID),
			loggateway.Int("archived_count", archived),
		)
	}
}

// CancelTimeoutTimer stops the timeout timer for a team if one is pending.
// Should be called when a team reaches a terminal state (completed/failed/cancelled)
// to prevent the timeout callback from firing unnecessarily.
func (u *SpiritTeamUsecase) CancelTimeoutTimer(teamID string) {
	if v, ok := u.timeoutTimers.LoadAndDelete(teamID); ok {
		if t, ok := v.(*time.Timer); ok {
			t.Stop()
		}
	}
}

// Stop cancels all pending timeout timers and the background polling goroutine.
// Call during application shutdown to prevent callbacks from firing after the
// server has stopped.
func (u *SpiritTeamUsecase) Stop() {
	// Cancel background polling goroutine.
	if u.pollCancel != nil {
		u.pollCancel()
	}
	u.timeoutTimers.Range(func(key, value any) bool {
		u.timeoutTimers.Delete(key)
		if t, ok := value.(*time.Timer); ok {
			t.Stop()
		}
		return true
	})
}

type TeamProgress struct {
	TeamID      string  `json:"team_id"`
	TeamName    string  `json:"team_name"`
	Status      string  `json:"status"`
	ProgressPct float64 `json:"progress_pct"`
	CurrentStep string  `json:"current_step"`
	DurationMs  int64   `json:"duration_ms"`
}

func (u *SpiritTeamUsecase) ExtractTeamOutput(ctx context.Context, teamID string) (summary string, keyFindings string, err error) {
	full, err := u.extractTeamFullOutput(ctx, teamID)
	if err != nil || full.Content == "" {
		return "", "", err
	}
	return TruncateRunes(full.Content, MaxSummaryLen), extractKeyFindings(full.Content), nil
}

// teamFullOutput is the untruncated team output plus its source session —
// the P2 DeliverableRef envelope needs both (SizeChars / TeamSessionID).
type teamFullOutput struct {
	Content   string // full, untruncated deliverable content
	SessionID string // team main session the content was read from
}

// resolveTeamMainSessionID returns the team's main session ID — the session
// whose ID keys the trpc graph state. Member agent sessions share the same
// team_id and Search ordering is not guaranteed, so the main session is
// identified by SessionType; when no typed session exists (legacy rows) the
// first hit is used. "" (nil error) means the team has no session at all.
func (u *SpiritTeamUsecase) resolveTeamMainSessionID(ctx context.Context, teamID string) (string, error) {
	result, err := u.sessionUC.Search(ctx, SessionSearchQuery{TeamID: teamID, Limit: 10})
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
func (u *SpiritTeamUsecase) extractTeamFullOutput(ctx context.Context, teamID string) (teamFullOutput, error) {
	teamSessionID, searchErr := u.resolveTeamMainSessionID(ctx, teamID)
	if searchErr != nil {
		u.lg.Warn("搜索团队 session 失败",
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
	if u.stepReader != nil {
		steps, stepErr := u.stepReader.ListStepsBySessionID(ctx, teamSessionID)
		if stepErr != nil {
			u.lg.Warn("获取团队步骤失败",
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

	messages, msgErr := u.sessionUC.ListMessagesRecent(ctx, teamSessionID, SpiritRecentMessageCount)
	if msgErr != nil {
		u.lg.Warn("获取团队消息失败",
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

func (u *SpiritTeamUsecase) CheckTeamProgress(ctx context.Context, spiritSessionID string) ([]TeamProgress, error) {
	spiritSessionID = strings.TrimSpace(spiritSessionID)
	if spiritSessionID == "" {
		return nil, apierror.BadRequest("SPIRIT", "spirit_session_id is required")
	}
	teams, err := u.teamUC.ListBySpiritSessionID(ctx, spiritSessionID)
	if err != nil {
		return nil, err
	}
	out := make([]TeamProgress, 0, len(teams))
	for i := range teams {
		tp := TeamProgress{
			TeamID:   teams[i].ID,
			TeamName: teams[i].DisplayName,
			Status:   teams[i].Status,
		}
		switch teams[i].Status {
		case TeamStatusCompleted:
			tp.ProgressPct = 100
			tp.CurrentStep = "已完成"
		case TeamStatusFailed:
			tp.ProgressPct = 0
			tp.CurrentStep = "执行失败"
		case TeamStatusCancelled:
			tp.ProgressPct = 0
			tp.CurrentStep = "已取消"
		case TeamStatusPending:
			tp.ProgressPct = 0
			tp.CurrentStep = "等待执行"
		default:
		}
		runs, runErr := u.teamUC.ListRuns(ctx, teams[i].ID, SpiritRecentRunCount)
		if runErr == nil && len(runs) > 0 {
			latestRun := runs[0]
			tp.DurationMs = int64(latestRun.DurationMS)
			if IsTeamStatusActive(tp.Status) {
				completedRuns := 0
				for _, r := range runs {
					if r.Status == TeamRunStatusSuccess {
						completedRuns++
					}
				}
				if len(runs) > 0 {
					tp.ProgressPct = float64(completedRuns) / float64(len(runs)) * 100
				}
				if tp.ProgressPct >= 100 {
					tp.ProgressPct = 99
				}
				tp.CurrentStep = fmt.Sprintf("执行中 (已完成 %d/%d 轮)", completedRuns, len(runs))
			}
		}
		out = append(out, tp)
	}
	return out, nil
}

// Spirit team definition constants.
// SpiritTeamDefVersion is the current version of spirit team definition JSON.
const (
	SpiritTeamDefVersion     = 2
	SpiritTeamDefaultTimeout = 600
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

	// DefaultUpstreamDeliverableMaxChars is the default full-text budget for
	// ReadUpstreamDeliverable when maxChars is unset/invalid.
	DefaultUpstreamDeliverableMaxChars = 50000
	// MaxUpstreamDeliverableChars is the hard cap for a single full-text read
	// (defense against runaway context consumption).
	MaxUpstreamDeliverableChars = 200000
)

// resolveAgentKeyToIDMap maps agentKeys (e.g. "__spirit__") to agent IDs (e.g.
// "agent___spirit__" or UUID). Uses SpiritAgentResolver.List to fetch all
// active agents once and builds a lookup map.
func (u *SpiritTeamUsecase) resolveAgentKeyToIDMap(ctx context.Context, agentKeys []string) (map[string]string, error) {
	if len(agentKeys) == 0 {
		return nil, nil
	}
	result, err := u.agentUC.List(ctx, AgentListQuery{
		Status: "active",
		Limit:  200,
	})
	if err != nil {
		return nil, err
	}
	keyToID := make(map[string]string, len(result.Items))
	for _, a := range result.Items {
		keyToID[a.AgentKey] = a.ID
	}
	out := make(map[string]string, len(agentKeys))
	for _, key := range agentKeys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, dup := out[key]; dup {
			continue
		}
		id, ok := keyToID[key]
		if !ok {
			id = "agent_" + key
		}
		out[key] = id
	}
	return out, nil
}

func buildSpiritTeamDefinitionJSON(mode string, agentKeys []string, lg loggateway.Logger, parallelCfgJSON ...string) string {
	type member struct {
		AgentKey string `json:"agent_id"`
		Role     string `json:"role"`
		Enabled  *bool  `json:"enabled"`
	}
	members := make([]member, 0, len(agentKeys))
	for i, key := range agentKeys {
		role := RoleWorker
		enabled := true
		if i == 0 && mode == TeamModeCoordinator {
			role = RoleSynthesizer
		}
		members = append(members, member{
			AgentKey: strings.TrimSpace(key),
			Role:     role,
			Enabled:  &enabled,
		})
	}
	maxConcurrency := SpiritTeamDefaultMaxConc
	if len(parallelCfgJSON) > 0 && parallelCfgJSON[0] != "" {
		cfg := ParseParallelConfig(parallelCfgJSON[0], lg)
		if cfg.MaxTeamConcurrency > 0 {
			maxConcurrency = cfg.MaxTeamConcurrency
		}
	}
	def := map[string]any{
		"version":            SpiritTeamDefVersion,
		"mode":               mode,
		"runtime_engine":     RuntimeEngineGraph,
		"team_graph_runtime": true,
		"members":            members,
		"max_concurrency":    maxConcurrency,
		"timeout_seconds":    SpiritTeamDefaultTimeout,
	}
	// C1/C3: multi-member teams get the deliverable state channel so members
	// can pass structured output via set_deliverable/get_deliverable tools.
	if len(agentKeys) > 1 {
		def["enable_state_deliverable"] = true
	}
	out, err := json.Marshal(def)
	if err != nil {
		return "{}"
	}
	return string(out)
}

func TruncateRunes(s string, maxLen int) string {
	if utf8.RuneCountInString(s) <= maxLen {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxLen-3]) + "..."
}

func (u *SpiritTeamUsecase) SearchSessions(ctx context.Context, q session.SessionSearchQuery) (session.SessionListResult, error) {
	return u.sessionUC.Search(ctx, q)
}

func (u *SpiritTeamUsecase) GetSpiritQuery(ctx context.Context, spiritSessionID string) string {
	messages, err := u.sessionUC.ListMessagesRecent(ctx, spiritSessionID, 5)
	if err != nil || len(messages) == 0 {
		return ""
	}
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			return TruncateRunes(messages[i].ContentMarkdown, MaxSpiritQueryLen)
		}
	}
	return ""
}

// UpdateTeamDefinitionJSON replaces the team's DefinitionJSON with the provided value
// and persists the change. Used by TaskOrchestrator to write DAG-compiled definitions.
// Domain: Assembly — update team definition JSON (used by TaskOrchestrator for DAG-compiled definitions).
func (u *SpiritTeamUsecase) UpdateTeamDefinitionJSON(ctx context.Context, teamID string, definitionJSON string) error {
	_, err := u.teamUC.Update(ctx, teamID, Team{DefinitionJSON: definitionJSON})
	if err != nil {
		return apierror.Wrap(err, apierror.CodeInternal, "SPIRIT")
	}
	return nil
}

// RecordTeamCompletion records DQ Score, infers topology, and creates evolution suggestions
// for a completed team. Returns the computed DQ Score and inferred topology.
// Domain: Orchestration — record DQ score and create evolution suggestions on team completion.
func (u *SpiritTeamUsecase) RecordTeamCompletion(ctx context.Context, team Team, durationMs int64) (dqScore float64, topology TopologyType) {
	// Cancel timeout timer since team has completed.
	u.CancelTimeoutTimer(team.ID)

	// P0-②: persist deliverable output BEFORE any downstream scheduling reads
	// it. Callers guarantee RecordTeamCompletion runs before
	// ScheduleDependentTeams / PlanExecutor.NotifyTeamCompletion.
	if team.DagNodeID != "" {
		if werr := u.WriteDeliverablesToSession(ctx, team.ID); werr != nil {
			if errors.Is(werr, ErrNoRealDeliverable) {
				// 2026-07-25 Fix 1 双保险：service 闸门已把无交付物团队翻转
				// 为 failed（正常不会走到这里）；静默跳过，不产生 Warn 噪音。
				u.lg.Info("团队无真实交付物，跳过交付物落库",
					loggateway.StepID("spirit.team.completion.deliverables"),
					loggateway.Str("team_id", team.ID),
				)
			} else {
				u.lg.Warn("交付物落库失败（下游团队将无注入输入）",
					loggateway.StepID("spirit.team.completion.deliverables"),
					loggateway.Str("team_id", team.ID),
					loggateway.Err(werr),
				)
			}
		}
	}

	if u.orchCache == nil || team.DagNodeID == "" {
		return 0, ""
	}
	dqScore = ComputeDQScore(TeamSynthesisResult{
		TeamID:   team.ID,
		TeamName: team.DisplayName,
		TaskName: team.TaskDescription,
		// RecordTeamCompletion always records for a completed team; the "completed"
		// status is intentional — DQ Score is only meaningful for successful executions.
		Status: TeamStatusCompleted,
	}, durationMs)
	taskPattern := ExtractTaskPattern(team.TaskDescription)
	topology = InferTopologyFromTeam(team, u.lg)
	u.orchCache.RecordCompletion(ctx, taskPattern, topology, dqScore, 1, durationMs)
	u.lg.Info("精灵团队完成，记录 DQ Score",
		loggateway.StepID("spirit.team.completion"),
		loggateway.Str("team_id", team.ID),
		loggateway.Str("task_pattern", taskPattern),
		loggateway.Float64("dq_score", dqScore),
	)

	if dqScore < DQEvolutionThreshold && u.evolutionSugg != nil && team.SpiritSessionID != "" {
		altTopology, altFound := u.orchCache.SuggestBestAlternativeTopology(team.TaskDescription, topology)
		content := fmt.Sprintf("团队 %q 的 DQ Score 为 %.2f（低于阈值 %.1f），当前拓扑 %s 执行效果不佳。", team.DisplayName, dqScore, DQEvolutionThreshold, topology)
		if altFound {
			content += fmt.Sprintf("建议尝试 %s 拓扑。", altTopology)
		} else {
			content += "暂无历史数据推荐替代拓扑，建议调整任务描述或减少团队数量。"
		}
		_, suggErr := u.evolutionSugg.Create(ctx, EvolutionSuggestion{
			AgentID: team.SpiritSessionID,
			Type:    "orchestration_optimization",
			Title:   fmt.Sprintf("编排优化建议: %s", TruncateRunes(team.TaskDescription, MaxSuggestionTitleLen)),
			Content: content,
			Status:  "pending",
		})
		if suggErr != nil {
			u.lg.Warn("创建编排优化建议失败",
				loggateway.StepID("spirit.evolution_suggestion_err"),
				loggateway.Str("team_id", team.ID),
				loggateway.Err(suggErr),
			)
		}
	}
	return dqScore, topology
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

// ScheduleDependentTeams resolves DAG dependencies after a team completes.
// It returns a list of actions to take (activate or fail dependent teams).
// The caller (Service layer) is responsible for executing the actions
// (starting runners, publishing events, etc.).
// Domain: Orchestration — DAG dependency resolution and scheduling.
func (u *SpiritTeamUsecase) ScheduleDependentTeams(ctx context.Context, spiritSessionID string, completedTeam Team) []DependentTeamAction {
	if completedTeam.DagNodeID == "" {
		return nil
	}
	allTeams, err := u.teamUC.ListBySpiritSessionID(ctx, spiritSessionID)
	if err != nil {
		u.lg.Warn("查询精灵团队列表失败，跳过依赖调度",
			loggateway.StepID("spirit.schedule_deps.list_err"),
			loggateway.Err(err),
		)
		return nil
	}

	var actions []DependentTeamAction
	for i := range allTeams {
		t := &allTeams[i]
		if t.Status != TeamStatusPending {
			continue
		}
		if !containsString(t.DependsOn, completedTeam.DagNodeID) {
			continue
		}
		allDepsMet := true
		anyDepFailed := false
		for _, depID := range t.DependsOn {
			found := false
			for j := range allTeams {
				if allTeams[j].DagNodeID == depID {
					if allTeams[j].Status == TeamStatusCompleted {
						found = true
					} else if allTeams[j].Status == TeamStatusFailed || allTeams[j].Status == TeamStatusCancelled {
						anyDepFailed = true
					}
					break
				}
			}
			if !found && !anyDepFailed {
				allDepsMet = false
				break
			}
		}
		if anyDepFailed {
			actions = append(actions, DependentTeamAction{
				TeamID:    t.ID,
				TeamName:  t.DisplayName,
				DagNodeID: t.DagNodeID,
				Action:    "fail",
				Reason:    "前置依赖团队执行失败",
			})
			continue
		}
		if !allDepsMet {
			continue
		}
		// Re-check current status to avoid stale data.
		current, getErr := u.teamUC.Get(ctx, t.ID)
		if getErr != nil || current.Status != TeamStatusPending {
			u.lg.Info("依赖调度：团队状态已变更，跳过激活",
				loggateway.StepID("spirit.schedule_deps.stale"),
				loggateway.Str("team_id", t.ID),
				loggateway.Str("current_status", current.Status),
			)
			continue
		}
		// P0-③b: build the downstream team's first-turn input as
		// upstream-deliverable prefix + its own task description, so the
		// activated team receives structured upstream outputs.
		taskDesc := t.TaskDescription
		if prefix := u.InjectUpstreamDeliverables(ctx, *t); prefix != "" {
			taskDesc = prefix + taskDesc
		}
		actions = append(actions, DependentTeamAction{
			TeamID:          t.ID,
			TeamName:        t.DisplayName,
			DagNodeID:       t.DagNodeID,
			Action:          "activate",
			TaskDescription: taskDesc,
		})
	}
	return actions
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

// CheckAllTeamsCompleted checks whether all teams for a spirit session are in a terminal state.
// Returns a result indicating if all teams are done and the list of team IDs.
// A team is considered "done" if it is in completed, failed, or cancelled state.
// Domain: Orchestration — check if all teams reached terminal state.
func (u *SpiritTeamUsecase) CheckAllTeamsCompleted(ctx context.Context, spiritSessionID string) AllTeamsCompletedResult {
	teams, err := u.teamUC.ListBySpiritSessionID(ctx, spiritSessionID)
	if err != nil {
		u.lg.Warn("查询精灵会话团队列表失败，跳过全完成检查",
			loggateway.StepID("spirit.teams.check_all"),
			loggateway.Str("spirit_session_id", spiritSessionID),
			loggateway.Err(err),
		)
		return AllTeamsCompletedResult{}
	}
	if len(teams) == 0 {
		return AllTeamsCompletedResult{}
	}
	for _, t := range teams {
		switch t.Status {
		case TeamStatusPending, TeamStatusRunning, TeamStatusInterrupted:
			return AllTeamsCompletedResult{}
		}
	}
	// All teams are in a terminal state (completed, failed, cancelled, or archived).
	var teamIDs []string
	var completedTeams, failedTeams, cancelledTeams int
	for _, t := range teams {
		teamIDs = append(teamIDs, t.ID)
		switch t.Status {
		case TeamStatusCompleted:
			completedTeams++
		case TeamStatusFailed:
			failedTeams++
		case TeamStatusCancelled:
			failedTeams++
			cancelledTeams++
		}
	}
	// Aggregate token usage from child sessions of the spirit session.
	var totalTokenIn, totalTokenOut int
	childSessions, sessErr := u.sessionUC.ListChildSessions(ctx, spiritSessionID)
	if sessErr != nil {
		u.lg.Warn("查询精灵会话子 session 失败，跳过 token 聚合",
			loggateway.StepID("spirit.teams.token_agg_err"),
			loggateway.Str("spirit_session_id", spiritSessionID),
			loggateway.Err(sessErr),
		)
	} else {
		teamIDSet := make(map[string]struct{}, len(teamIDs))
		for _, id := range teamIDs {
			teamIDSet[id] = struct{}{}
		}
		for _, s := range childSessions {
			if _, ok := teamIDSet[s.TeamID]; ok {
				totalTokenIn += s.InputTokens
				totalTokenOut += s.OutputTokens
			}
		}
	}
	return AllTeamsCompletedResult{
		AllDone:        true,
		TeamIDs:        teamIDs,
		TotalTeams:     len(teams),
		CompletedTeams: completedTeams,
		FailedTeams:    failedTeams,
		CancelledTeams: cancelledTeams,
		TotalTokenIn:   totalTokenIn,
		TotalTokenOut:  totalTokenOut,
	}
}

// parseTimeFlexible tries multiple time formats to parse a timestamp string.
// This handles the case where Ent may output timestamps in formats other than
// strict RFC3339 (e.g., "2026-06-08 12:34:56.789+08:00").
func parseTimeFlexible(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, apierror.BadRequest("SPIRIT", "empty timestamp")
	}
	formats := []string{
		time.RFC3339,
		time.RFC3339Nano,
		"2006-01-02 15:04:05.999-07:00",
		"2006-01-02 15:04:05.999 -0700",
		"2006-01-02 15:04:05-07:00",
		"2006-01-02T15:04:05-07:00",
		"2006-01-02 15:04:05Z07:00",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, apierror.BadRequest("SPIRIT", "unable to parse timestamp: %s", s)
}

// containsString checks if a string slice contains a given string.
func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// XC-03: Cross-Department Collaboration — Contract Validation & Gate Injection
// ---------------------------------------------------------------------------

// ValidateDeliverableContracts validates deliverable contracts between
// upstream and downstream teams in the DAG. Returns a list of warnings
// for contract mismatches. Called after Team DAG is built.
// Domain: Delivery — validate deliverable contracts between upstream and downstream teams.
func (u *SpiritTeamUsecase) ValidateDeliverableContracts(ctx context.Context, spiritSessionID string) []string {
	if u.contractValidator == nil {
		return nil
	}
	teams, err := u.teamUC.ListBySpiritSessionID(ctx, spiritSessionID)
	if err != nil {
		u.lg.Warn("查询团队列表失败，跳过交付物合约校验",
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
		warnings := u.contractValidator.ValidateContractMatch(upstreamContracts, downstreamContracts)
		if len(warnings) > 0 {
			u.lg.Info("交付物合约校验发现不匹配",
				loggateway.StepID("spirit.contract_validate.mismatch"),
				loggateway.Str("team_id", t.ID),
				loggateway.Int("warning_count", len(warnings)),
			)
			allWarnings = append(allWarnings, warnings...)
		}
	}
	return allWarnings
}

// InjectDeptLeadIntoTeam adds the department lead agent to a team's definition.
// Called during team assembly for cross-department collaboration.
// Domain: Assembly — dept lead injection into team definition.
func (u *SpiritTeamUsecase) InjectDeptLeadIntoTeam(ctx context.Context, teamID string) error {
	if u.deptLeadMgr == nil {
		return nil
	}
	t, err := u.teamUC.Get(ctx, teamID)
	if err != nil {
		return err
	}
	if t.DepartmentID == "" {
		return nil
	}
	lead, err := u.deptLeadMgr.GetDeptLeadForTeam(ctx, t.DepartmentID)
	if err != nil {
		u.lg.Warn("获取部门主管失败，跳过注入",
			loggateway.StepID("spirit.inject_dept_lead"),
			loggateway.Str("team_id", teamID),
			loggateway.Str("dept_id", t.DepartmentID),
			loggateway.Err(err),
		)
		return nil
	}

	// Update the team's DeptLeadAgentID
	_, err = u.teamUC.Update(ctx, teamID, Team{DeptLeadAgentID: lead.ID})
	if err != nil {
		return apierror.Wrap(err, apierror.CodeInternal, "SPIRIT")
	}

	u.lg.Info("注入部门主管到团队",
		loggateway.StepID("spirit.inject_dept_lead"),
		loggateway.Str("team_id", teamID),
		loggateway.Str("dept_lead_agent_id", lead.ID),
	)
	return nil
}

// ExecuteVerificationGates runs all verification gates for a team's output.
// Returns (approved bool, warnings []string, err error).
// If any gate rejects, the whole verification fails.
// Domain: Delivery — execute verification gates on team output.
func (u *SpiritTeamUsecase) ExecuteVerificationGates(ctx context.Context, teamID string, teamOutput string) (bool, []string, error) {
	if u.gateExecutor == nil {
		return true, nil, nil
	}
	t, err := u.teamUC.Get(ctx, teamID)
	if err != nil {
		return false, nil, err
	}

	// Get verification gates from the team's linked graph
	gates, err := u.resolveVerificationGates(ctx, t)
	if err != nil || len(gates) == 0 {
		return true, nil, nil
	}

	// Resolve truncate chars from the Spirit agent's runtime settings.
	truncateChars := 0
	if u.agentUC != nil {
		agents, listErr := u.agentUC.List(ctx, AgentListQuery{Keyword: SpiritAgentKey, Limit: SpiritAgentQueryLimit})
		if listErr == nil && len(agents.Items) > 0 && agents.Items[0].Settings != nil {
			truncateChars = agents.Items[0].Settings.VerificationTruncateChars
		}
	}

	var allWarnings []string
	for _, gate := range gates {
		if gate.MaxRetries <= 0 {
			gate.MaxRetries = 3
		}
		approved, reason, gateErr := u.gateExecutor.ExecuteGate(ctx, gate, teamOutput, truncateChars)
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
func (u *SpiritTeamUsecase) resolveVerificationGates(ctx context.Context, t Team) ([]VerificationGate, error) {
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

// ---------------------------------------------------------------------------
// XC-03b: Deliverable Passing Mechanism
// ---------------------------------------------------------------------------

// ErrNoRealDeliverable marks a DAG team that has no graph-state deliverable —
// set_deliverable was never called (or the state is unreadable). Reply text
// must NOT be dressed up as a deliverable: a team that only asked clarifying
// questions produced nothing, and persisting its reply as a "deliverable" is
// the root of the 19:29 false-success chain (2026-07-25 Fix 1).
var ErrNoRealDeliverable = errors.New("no real deliverable: set_deliverable was never called")

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
	t, err := u.teamUC.Get(ctx, teamID)
	if err != nil {
		return err
	}
	if t.DagNodeID == "" {
		return nil // not a DAG node — nothing to key the output by
	}

	anchor, ok := u.stateDeliverableChannel(t)
	if !ok {
		return ErrNoRealDeliverable // channel disabled → no real deliverable can exist
	}
	teamSessionID, err := u.resolveTeamMainSessionID(ctx, t.ID)
	if err != nil {
		return err
	}
	if teamSessionID == "" {
		return ErrNoRealDeliverable
	}
	stateDeliv, err := u.graphDelivReader.ReadGraphDeliverable(ctx, anchor, ctxuser.TRPCUserKey(ctx), teamSessionID)
	if err != nil {
		u.lg.Warn("graph state deliverable 读取失败，按无真实交付物处理",
			loggateway.StepID("spirit.write_deliverables"),
			loggateway.Str("team_id", teamID),
			loggateway.Err(err),
		)
		return ErrNoRealDeliverable
	}
	if len(stateDeliv) == 0 {
		return ErrNoRealDeliverable
	}

	// The envelope is 100% state-sourced: the reserved "summary" key becomes
	// the summary; when absent the summary derives from the business keys'
	// JSON (never from the reply). Non-reserved keys land in StructuredJSON.
	summarySource, _ := stateDeliv[deliverableReservedKeySummary].(string)
	summarySource = strings.TrimSpace(summarySource)
	structuredJSON := marshalNonReservedStateKeys(stateDeliv)
	if summarySource == "" {
		summarySource = structuredJSON
	}
	cognition := extractStateCognition(stateDeliv[deliverableReservedKeyCognition])

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
			u.lg.Warn("交付物输出缓存 JSON 损坏，重建缓存",
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

	_, err = u.teamUC.Update(ctx, t.ID, Team{DeliverablesOutput: string(updatedJSON)})
	if err != nil {
		u.lg.Warn("持久化交付物输出失败",
			loggateway.StepID("spirit.write_deliverables"),
			loggateway.Str("team_id", teamID),
			loggateway.Err(err),
		)
		return err
	}
	u.lg.Info("团队交付物已落库，可供下游团队消费",
		loggateway.StepID("spirit.write_deliverables"),
		loggateway.Str("team_id", teamID),
		loggateway.Str("dag_node_id", t.DagNodeID),
		loggateway.Int("size_chars", sizeChars),
		loggateway.Bool("truncated", ref.Truncated),
	)
	return nil
}

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

// HasRealDeliverable reports whether the team produced a REAL deliverable —
// a non-empty graph-state "deliverable" map written via set_deliverable.
// Reply text does not count. (false, nil) covers every "no deliverable"
// shape (non-DAG team, channel disabled, no session, empty state);
// (false, err) is reserved for infra failures so the caller can distinguish
// "did not produce" from "could not verify" (2026-07-25 Fix 1 gate).
func (u *SpiritTeamUsecase) HasRealDeliverable(ctx context.Context, team Team) (bool, error) {
	if team.DagNodeID == "" {
		return false, nil // non-DAG teams carry no deliverable obligation
	}
	anchor, ok := u.stateDeliverableChannel(team)
	if !ok {
		return false, nil
	}
	teamSessionID, err := u.resolveTeamMainSessionID(ctx, team.ID)
	if err != nil {
		return false, err
	}
	if teamSessionID == "" {
		return false, nil
	}
	stateDeliv, err := u.graphDelivReader.ReadGraphDeliverable(ctx, anchor, ctxuser.TRPCUserKey(ctx), teamSessionID)
	if err != nil {
		return false, err
	}
	return len(stateDeliv) > 0, nil
}

// stateDeliverableChannel resolves the graph-state deliverable channel
// coordinates for a team. ok=false means no real deliverable can exist:
// no reader wired, no/undecodable DefinitionJSON, enable_state_deliverable
// off, or no anchor agent. The anchor mirrors the runner's AppName decision
// (stateDeliverableProbe.anchorAgentID) so the state is read under the same
// session key the run persisted to.
func (u *SpiritTeamUsecase) stateDeliverableChannel(t Team) (anchor string, ok bool) {
	if u.graphDelivReader == nil || strings.TrimSpace(t.DefinitionJSON) == "" {
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

// InjectUpstreamDeliverables collects upstream team deliverables and formats
// them as a prefix for the downstream team's input message.
// Called when a DAG activates a downstream team.
// It first tries to read from the persisted deliverable output cache
// (written by WriteDeliverablesToSession), then falls back to
// extracting from the team output directly.
// Domain: Delivery — collect and format upstream deliverables for downstream team input.
func (u *SpiritTeamUsecase) InjectUpstreamDeliverables(ctx context.Context, downstreamTeam Team) string {
	if len(downstreamTeam.DependsOn) == 0 {
		return ""
	}
	teams, err := u.teamUC.ListBySpiritSessionID(ctx, downstreamTeam.SpiritSessionID)
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
		ref, refOK := u.readDeliverableRef(upstream)
		if !refOK || ref.Summary == "" {
			continue
		}
		// Legacy plain-string values carry no team_id; the caller always knows it.
		if ref.TeamID == "" {
			ref.TeamID = upstream.ID
		}
		part := fmt.Sprintf("## 上游团队: %s\n%s%s", upstream.DisplayName, contractDeclarationLines(upstream.Deliverables), ref.Summary)
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

// readDeliverableRef reads the persisted DeliverableRef for the team's own
// dag_node_id. Dual-mode: P2 envelopes parse with full metadata; legacy
// plain-string values yield a summary-only ref. ok=false when absent or
// unparseable.
// Domain: Delivery — read persisted deliverable envelope from team's cache.
func (u *SpiritTeamUsecase) readDeliverableRef(t Team) (DeliverableRef, bool) {
	if t.DagNodeID == "" {
		return DeliverableRef{}, false
	}
	ref, ok := ParseDeliverableRefs(t.DeliverablesOutput)[t.DagNodeID]
	return ref, ok
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

// ReadUpstreamDeliverable returns the full deliverable text of a COMPLETED
// upstream team, truncated to maxChars (default DefaultUpstreamDeliverableMaxChars,
// hard cap MaxUpstreamDeliverableChars). Backs the read_upstream_deliverable
// tool: the injection prefix only carries a truncated summary, and downstream
// team members call the tool when they genuinely need the full text.
//
// Phase B (runtime contract validation): when readerSessionID resolves to a
// reader team with a declared InputContract, that contract is checked against
// the upstream team's declared Deliverables BEFORE the (expensive) full-text
// extraction; a mismatch returns a structured *ContractMismatchError so the
// calling agent can auto-correct and retry.
// Domain: Delivery — full-text retrieval of an upstream team's deliverable.
func (u *SpiritTeamUsecase) ReadUpstreamDeliverable(ctx context.Context, readerSessionID, teamID string, maxChars int) (UpstreamDeliverableContent, error) {
	teamID = strings.TrimSpace(teamID)
	if teamID == "" {
		return UpstreamDeliverableContent{}, apierror.BadRequest("SPIRIT", "team_id is required")
	}
	if maxChars <= 0 || maxChars > MaxUpstreamDeliverableChars {
		maxChars = DefaultUpstreamDeliverableMaxChars
	}
	t, err := u.teamUC.Get(ctx, teamID)
	if err != nil {
		return UpstreamDeliverableContent{}, err
	}
	if t.Status != TeamStatusCompleted {
		return UpstreamDeliverableContent{}, apierror.BadRequest("SPIRIT", "upstream team %s has not completed yet (status=%s)", teamID, t.Status)
	}
	if err := u.validateUpstreamContract(ctx, readerSessionID, t); err != nil {
		return UpstreamDeliverableContent{}, err
	}
	full, err := u.extractTeamFullOutput(ctx, teamID)
	if err != nil {
		return UpstreamDeliverableContent{}, err
	}
	if full.Content == "" {
		return UpstreamDeliverableContent{}, apierror.NotFound("SPIRIT", "no deliverable content found for team %s", teamID)
	}
	// C2: content-level schema validation runs AFTER full-text extraction (it
	// needs the content), unlike the name/type/format check above which fails
	// fast before the expensive read.
	if err := u.validateUpstreamContractSchema(ctx, readerSessionID, t, full.Content); err != nil {
		return UpstreamDeliverableContent{}, err
	}

	sizeChars := utf8.RuneCountInString(full.Content)
	content := full.Content
	truncated := sizeChars > maxChars
	if truncated {
		content = TruncateRunes(content, maxChars) + fmt.Sprintf("\n...[已截断，全文共 %d 字符]", sizeChars)
	}
	u.lg.Info("上游交付物全文读取",
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

// validateUpstreamContract performs the tool-call level contract check for
// ReadUpstreamDeliverable: reader team (resolved from readerSessionID) → its
// InputContract vs the upstream team's declared Deliverables. A mismatch
// returns *ContractMismatchError; the check is skipped (nil) when the reader
// side is unresolvable or either side declares no contract — declarations
// stay advisory for legacy teams without contracts.
// Domain: Delivery — runtime contract guard for upstream deliverable reads.
func (u *SpiritTeamUsecase) validateUpstreamContract(ctx context.Context, readerSessionID string, upstream Team) error {
	reader, inputContracts, ok := u.resolveReaderContracts(ctx, readerSessionID, upstream.ID)
	if !ok {
		return nil
	}
	upstreamContracts, err := ParseDeliverableContracts(upstream.Deliverables)
	if err != nil || len(upstreamContracts) == 0 {
		return nil
	}
	validator := u.contractValidator
	if validator == nil {
		validator = NewDeliverableContractValidator()
	}
	mismatches := validator.ValidateContractMatchDetailed(upstreamContracts, inputContracts)
	if len(mismatches) == 0 {
		return nil
	}
	u.lg.Warn("上游交付物契约校验不匹配",
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
func (u *SpiritTeamUsecase) resolveReaderContracts(ctx context.Context, readerSessionID, upstreamTeamID string) (Team, []DeliverableContract, bool) {
	readerSessionID = strings.TrimSpace(readerSessionID)
	if readerSessionID == "" {
		return Team{}, nil, false
	}
	sess, err := u.sessionUC.Get(ctx, readerSessionID)
	if err != nil || sess.TeamID == "" || sess.TeamID == upstreamTeamID {
		// Unresolvable reader, or a team reading its own deliverable: no check.
		return Team{}, nil, false
	}
	reader, err := u.teamUC.Get(ctx, sess.TeamID)
	if err != nil {
		return Team{}, nil, false
	}
	inputContracts, err := ParseDeliverableContracts(reader.InputContract)
	if err != nil || len(inputContracts) == 0 {
		return Team{}, nil, false
	}
	return reader, inputContracts, true
}

// schemaViolationPrefix is the message prefix produced by
// shared.ValidateDocumentAgainstSchema on a document/schema mismatch; the
// mismatch record keeps only the violation detail.
const schemaViolationPrefix = "config does not match schema: "

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
func (u *SpiritTeamUsecase) validateUpstreamContractSchema(ctx context.Context, readerSessionID string, upstream Team, content string) error {
	reader, inputContracts, ok := u.resolveReaderContracts(ctx, readerSessionID, upstream.ID)
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
	u.lg.Warn("上游交付物内容 schema 校验不匹配",
		loggateway.StepID("spirit.read_upstream_deliverable.schema_mismatch"),
		loggateway.Str("reader_team_id", reader.ID),
		loggateway.Str("upstream_team_id", upstream.ID),
		loggateway.Int("mismatch_count", len(mismatches)),
	)
	return &ContractMismatchError{ReaderTeamID: reader.ID, UpstreamTeamID: upstream.ID, Mismatches: mismatches}
}

// ---------------------------------------------------------------------------
// XC-05: Escalation on Max Retries
// ---------------------------------------------------------------------------

// EscalateToSpirit escalates a team that has exceeded max retries to the
// Spirit assistant. This creates a system message in the Spirit session
// notifying the user that human intervention may be needed.
// Domain: Orchestration — escalation to Spirit assistant on max retries.
func (u *SpiritTeamUsecase) EscalateToSpirit(ctx context.Context, teamID string, tracker ReworkTracker) error {
	t, err := u.teamUC.Get(ctx, teamID)
	if err != nil {
		return err
	}

	u.lg.Warn("团队达到最大重试次数，升级到 Spirit 助手",
		loggateway.StepID("spirit.escalate"),
		loggateway.Str("team_id", teamID),
		loggateway.Str("team_name", t.DisplayName),
		loggateway.Int("attempts", tracker.Attempt),
		loggateway.Str("last_reason", tracker.LastReason),
	)

	// Transition team to failed status with escalation reason
	_, err = u.teamUC.TransitionStatus(ctx, teamID, TeamStatusFailed)
	if err != nil {
		return apierror.Wrap(err, apierror.CodeInternal, "SPIRIT")
	}

	return nil
}

// HandleTeamRejection handles a team rejection by a verification gate.
// If the team can retry, it marks the team for rework and transitions
// its status back to pending for re-execution; otherwise it escalates
// to the Spirit assistant.
// Domain: Orchestration — handle verification gate rejection with retry/escalation logic.
// Note: The Running → Pending transition (TeamEventRework) was added in B-02 fix
// to support the rework flow. Before the fix, this transition was illegal and
// would silently fail.
func (u *SpiritTeamUsecase) HandleTeamRejection(ctx context.Context, teamID string, tracker ReworkTracker, reason string) (*ReworkTracker, error) {
	tracker.LastReason = reason

	if !tracker.CanRetry() {
		if err := u.EscalateToSpirit(ctx, teamID, tracker); err != nil {
			return nil, err
		}
		return &tracker, nil
	}

	tracker.IncrementAttempt()

	// Mark team for rework: transition back to pending status
	// so the DAG scheduler can re-execute it.
	_, transitionErr := u.teamUC.TransitionStatus(ctx, teamID, TeamStatusPending)
	if transitionErr != nil {
		u.lg.Warn("返工状态转换失败",
			loggateway.StepID("spirit.rework"),
			loggateway.Str("team_id", teamID),
			loggateway.Err(transitionErr),
		)
	}

	u.lg.Info("团队被拒绝，准备重试",
		loggateway.StepID("spirit.rework"),
		loggateway.Str("team_id", teamID),
		loggateway.Int("attempt", tracker.Attempt),
		loggateway.Int("max_retries", tracker.MaxRetries),
		loggateway.Str("reason", reason),
	)
	return &tracker, nil
}
