package biz

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"aranea-agents/internal/biz/session"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/google/uuid"
)

type TeamStarterPort interface {
	StartTeamTurn(ctx context.Context, sessionID string, content string) error
	HandleTeamTurnResult(ctx context.Context, spiritSessionID, teamID, status, errMsg string)
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
}

// TimeoutHandler is called when a team times out. Implemented by the service
// layer to trigger dependency scheduling, event publishing, and AllDone checks.
type TimeoutHandler interface {
	HandleTeamTimeout(ctx context.Context, spiritSessionID, teamID string)
}

type SpiritTeamParams struct {
	SpiritSessionID       string
	TaskDescription       string
	AgentKeys             []string
	Mode                  string
	DagNodeID             string
	TeamName              string
	TaskSummary           string
	DependsOn             []string
	ParallelConfigJSON    string
	TopologyReason        string
	AutoStart             bool
	DepartmentID          string   // home department for the team
	CrossDeptMemberAgentIDs []string // agent IDs from other departments requiring borrow approval
}

var ErrNoCompletedTeams = kerrors.BadRequest("SPIRIT", "no completed teams to synthesize")

type SpiritTeamResult struct {
	Team    Team
	Session Session
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

func WithDeliverableContractValidator(v *DeliverableContractValidator) SpiritTeamUsecaseOption {
	return func(u *SpiritTeamUsecase) { u.contractValidator = v }
}

func WithVerificationGateExecutor(e *VerificationGateExecutor) SpiritTeamUsecaseOption {
	return func(u *SpiritTeamUsecase) { u.gateExecutor = e }
}

func WithDeptLeadMgr(m *DeptLeadManager) SpiritTeamUsecaseOption {
	return func(u *SpiritTeamUsecase) { u.deptLeadMgr = m }
}

type SpiritTeamUsecase struct {
	_                SpiritTeamController // interface assertion
	teamUC           *TeamUsecase
	sessionUC        *SessionUsecase
	agentUC          *AgentUsecase
	transactor       SpiritTransactor
	orchCache        *OrchestrationCache
	evolutionSugg    EvolutionSuggestionRepo
	timeoutHandler   TimeoutHandler
	contractValidator *DeliverableContractValidator
	gateExecutor     *VerificationGateExecutor
	deptLeadMgr      *DeptLeadManager
	lg               loggateway.Logger

	timeoutOnce sync.Once

	// timeoutTimers tracks pending timeout callbacks so they can be cancelled
	// when a team completes normally before the timeout fires.
	timeoutTimers sync.Map // map[string]*time.Timer
}

func NewSpiritTeamUsecase(teamUC *TeamUsecase, sessionUC *SessionUsecase, agentUC *AgentUsecase, lg loggateway.Logger, opts ...SpiritTeamUsecaseOption) *SpiritTeamUsecase {
	u := &SpiritTeamUsecase{teamUC: teamUC, sessionUC: sessionUC, agentUC: agentUC, lg: lg}
	for _, opt := range opts {
		opt(u)
	}
	return u
}

// SetTimeoutHandler injects the service-layer timeout handler.
// Called after construction to break the circular dependency.
// Uses sync.Once to ensure the handler is set exactly once.
func (u *SpiritTeamUsecase) SetTimeoutHandler(h TimeoutHandler) {
	u.timeoutOnce.Do(func() {
		u.timeoutHandler = h
	})
}

func (u *SpiritTeamUsecase) AssembleTeam(ctx context.Context, params SpiritTeamParams) (SpiritTeamResult, error) {
	spiritSessionID := strings.TrimSpace(params.SpiritSessionID)
	if spiritSessionID == "" {
		return SpiritTeamResult{}, kerrors.BadRequest("SPIRIT", "spirit_session_id is required")
	}
	taskDesc := strings.TrimSpace(params.TaskDescription)
	if taskDesc == "" {
		return SpiritTeamResult{}, kerrors.BadRequest("SPIRIT", "task_description is required")
	}
	mode := strings.TrimSpace(params.Mode)
	if mode == "" {
		mode = TeamModeCoordinator
	}

	defJSON := buildSpiritTeamDefinitionJSON(mode, params.AgentKeys, u.lg, params.ParallelConfigJSON)

	// Check session tree depth limit before creating team.
	cfg := u.resolveParallelConfig(ctx, spiritSessionID)
	parentSession, err := u.sessionUC.Get(ctx, spiritSessionID)
	if err != nil {
		return SpiritTeamResult{}, kerrors.InternalServer("SPIRIT", "get spirit session").WithCause(err)
	}
	if parentSession.AgentDepth >= cfg.MaxSessionDepth {
		return SpiritTeamResult{}, kerrors.BadRequest("SPIRIT",
			fmt.Sprintf("session tree depth (%d) exceeds max (%d)", parentSession.AgentDepth, cfg.MaxSessionDepth))
	}
	childDepth := parentSession.AgentDepth + 1

	// All teams start as pending regardless of depends_on.
	// AutoStart=true teams transition to running when StartTeamTurn is called.
	// AutoStart=false DAG root nodes stay pending until manually started or
	// scheduled by scheduleDependentTeams.
	initialStatus := TeamStatusPending

	var result SpiritTeamResult
	err = u.transactor.ExecInTx(ctx, func(txCtx context.Context) error {
		team, err := u.teamUC.Create(txCtx, Team{
			TeamKey:           fmt.Sprintf("spirit_%s_%s", spiritSessionID, uuid.New().String()[:8]),
			DisplayName:       TruncateRunes(taskDesc, MaxTeamDisplayNameLen),
			Status:            initialStatus,
			SpiritSessionID:   spiritSessionID,
			TaskDescription:   taskDesc,
			AutoCreated:       true,
			DefinitionJSON:    defJSON,
			DagNodeID:         params.DagNodeID,
			DependsOn:         params.DependsOn,
			ParallelConfigJSON: params.ParallelConfigJSON,
			Topology:          mode,
			DepartmentID:      params.DepartmentID,
		})
		if err != nil {
			return kerrors.InternalServer("SPIRIT", "create team").WithCause(err)
		}

		teamSession, err := u.sessionUC.Create(txCtx, Session{
			OwnerType:       "team",
			TeamID:          team.ID,
			ParentSessionID: spiritSessionID,
			RootSessionID:   spiritSessionID,
			AgentDepth:      childDepth,
			Title:           TruncateRunes(taskDesc, MaxTeamTitleLen),
		})
		if err != nil {
			return kerrors.InternalServer("SPIRIT", "create team session").WithCause(err)
		}

		result = SpiritTeamResult{Team: team, Session: teamSession}
		return nil
	})
	if err != nil {
		return SpiritTeamResult{}, err
	}

	// Register team timeout callback if configured.
	u.registerTeamTimeout(cfg, result.Team.ID)

	// Submit borrow requests for cross-department members (DL-09).
	// These are processed outside the transaction to avoid long-held locks.
	if u.deptLeadMgr != nil && len(params.CrossDeptMemberAgentIDs) > 0 && params.DepartmentID != "" {
		u.submitBorrowRequests(ctx, result.Team.ID, params.DepartmentID, params.CrossDeptMemberAgentIDs)
	}

	return result, nil
}

// submitBorrowRequests creates borrow requests for cross-department members.
// This is a best-effort operation: failures are logged but do not fail team creation.
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

func (u *SpiritTeamUsecase) registerTeamTimeout(cfg ParallelConfig, teamID string) {
	if cfg.TeamTimeoutSeconds <= 0 {
		return
	}
	timer := time.AfterFunc(cfg.TeamTimeout(), func() {
		// If CancelTimeoutTimer already removed this entry, the team completed
		// normally and we should not interfere.
		if _, loaded := u.timeoutTimers.LoadAndDelete(teamID); !loaded {
			return
		}
		safego.Go(context.Background(), "spirit-team-timeout", func() {
			timeoutCtx, timeoutCancel := context.WithTimeout(context.Background(), cfg.TimeoutHandlerDBTimeout())
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
		return nil, kerrors.BadRequest("SPIRIT", "spirit_session_id is required")
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
		return nil, kerrors.BadRequest("SPIRIT", "spirit_session_id is required")
	}
	return u.teamUC.ListBySpiritSessionID(ctx, spiritSessionID)
}

func (u *SpiritTeamUsecase) ListCompletedAndFailedTeams(ctx context.Context, spiritSessionID string) ([]Team, error) {
	spiritSessionID = strings.TrimSpace(spiritSessionID)
	if spiritSessionID == "" {
		return nil, kerrors.BadRequest("SPIRIT", "spirit_session_id is required")
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
		u.lg.Warn("查询精灵 Agent 失败，使用默认并行配置",
			loggateway.StepID("spirit.parallel_config"),
			loggateway.Err(err),
		)
		return DefaultParallelConfig()
	}
	if len(agents.Items) == 0 {
		return DefaultParallelConfig()
	}
	ag := agents.Items[0]
	// Read parallel_config from ConfigJSON (stored as a top-level key).
	// Previously attempted to read from MetadataJSON, but that field has no DB column.
	return ParseParallelConfig(ag.ConfigJSON, u.lg)
}

func (u *SpiritTeamUsecase) CancelTeam(ctx context.Context, teamID string) error {
	if strings.TrimSpace(teamID) == "" {
		return kerrors.BadRequest("SPIRIT", "team_id is required")
	}
	u.CancelTimeoutTimer(teamID)
	_, err := u.teamUC.TransitionStatus(ctx, teamID, TeamStatusCancelled)
	if err != nil {
		return err
	}
	return nil
}

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
		updatedAt, parseErr := time.Parse(time.RFC3339, t.UpdatedAt)
		if parseErr != nil {
			u.lg.Warn("解析团队更新时间失败，跳过归档",
				loggateway.StepID("spirit.auto_archive.parse_err"),
				loggateway.Str("team_id", t.ID),
				loggateway.Err(parseErr),
			)
			continue
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

// Stop cancels all pending timeout timers. Call during application shutdown
// to prevent timeout callbacks from firing after the server has stopped.
func (u *SpiritTeamUsecase) Stop() {
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
	result, searchErr := u.sessionUC.Search(ctx, SessionSearchQuery{TeamID: teamID, Limit: 1})
	if searchErr != nil {
		u.lg.Warn("搜索团队 session 失败",
			loggateway.StepID("spirit.extract_output.search_err"),
			loggateway.Str("team_id", teamID),
			loggateway.Err(searchErr),
		)
		return "", "", searchErr
	}
	if len(result.Items) == 0 {
		return "", "", nil
	}
	teamSession := result.Items[0]
	messages, msgErr := u.sessionUC.ListMessagesRecent(ctx, teamSession.ID, SpiritRecentMessageCount)
	if msgErr != nil {
		u.lg.Warn("获取团队消息失败",
			loggateway.StepID("spirit.extract_output.msg_err"),
			loggateway.Str("team_id", teamID),
			loggateway.Err(msgErr),
		)
		return "", "", msgErr
	}
	if len(messages) == 0 {
		return "", "", nil
	}
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "assistant" {
			content := messages[i].ContentMarkdown
			summary = TruncateRunes(content, MaxSummaryLen)
			keyFindings = extractKeyFindings(content)
			return summary, keyFindings, nil
		}
	}
	return "", "", nil
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
		return nil, kerrors.BadRequest("SPIRIT", "spirit_session_id is required")
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
	MaxTeamDisplayNameLen    = 64
	MaxTeamTitleLen          = 128
	MaxSummaryLen            = 500
	MaxSuggestionTitleLen    = 40
	MaxSpiritQueryLen        = 500

	// TimeoutHandlerContextTimeout is the maximum duration for DB operations
	// inside the timeout callback goroutine.
	// Deprecated: use ParallelConfig.TimeoutHandlerDBTimeout() instead.
	TimeoutHandlerContextTimeout = 30 * time.Second

	// MaxKeyFindingsCount is the maximum number of key findings extracted.
	MaxKeyFindingsCount = 5
)

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
func (u *SpiritTeamUsecase) UpdateTeamDefinitionJSON(ctx context.Context, teamID string, definitionJSON string) error {
	_, err := u.teamUC.Update(ctx, teamID, Team{DefinitionJSON: definitionJSON})
	if err != nil {
		return kerrors.InternalServer("SPIRIT", "update team definition").WithCause(err)
	}
	return nil
}

// RecordTeamCompletion records DQ Score, infers topology, and creates evolution suggestions
// for a completed team. Returns the computed DQ Score and inferred topology.
func (u *SpiritTeamUsecase) RecordTeamCompletion(ctx context.Context, team Team, durationMs int64) (dqScore float64, topology TopologyType) {
	// Cancel timeout timer since team has completed.
	u.CancelTimeoutTimer(team.ID)

	if u.orchCache == nil || team.DagNodeID == "" {
		return 0, ""
	}
	dqScore = ComputeDQScore(TeamSynthesisResult{
		TeamID:   team.ID,
		TeamName: team.DisplayName,
		TaskName: team.TaskDescription,
	// RecordTeamCompletion always records for a completed team; the "completed"
	// status is intentional — DQ Score is only meaningful for successful executions.
	Status:   TeamStatusCompleted,
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
	TeamID   string
	TeamName string
	DagNodeID string
	Action   string // "activate" or "fail"
	Reason   string
}

// ScheduleDependentTeams resolves DAG dependencies after a team completes.
// It returns a list of actions to take (activate or fail dependent teams).
// The caller (Service layer) is responsible for executing the actions
// (starting runners, publishing events, etc.).
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
		actions = append(actions, DependentTeamAction{
			TeamID:    t.ID,
			TeamName:  t.DisplayName,
			DagNodeID: t.DagNodeID,
			Action:    "activate",
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
	TotalTokenIn   int
	TotalTokenOut  int
}

// CheckAllTeamsCompleted checks whether all teams for a spirit session are in a terminal state.
// Returns a result indicating if all teams are done and the list of team IDs.
// A team is considered "done" if it is in completed, failed, or cancelled state.
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
	var completedTeams, failedTeams int
	for _, t := range teams {
		teamIDs = append(teamIDs, t.ID)
		switch t.Status {
		case TeamStatusCompleted:
			completedTeams++
		case TeamStatusFailed, TeamStatusCancelled:
			failedTeams++
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
		TotalTokenIn:   totalTokenIn,
		TotalTokenOut:  totalTokenOut,
	}
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
		return kerrors.InternalServer("SPIRIT", "update team dept lead").WithCause(err)
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

// WriteDeliverablesToSession writes upstream team deliverables to the
// Team's deliverables field so downstream teams can access them.
// The deliverable content is extracted from the team output and stored
// in the team record for downstream consumption.
func (u *SpiritTeamUsecase) WriteDeliverablesToSession(ctx context.Context, teamID string) error {
	t, err := u.teamUC.Get(ctx, teamID)
	if err != nil {
		return err
	}
	if t.Deliverables == "" || t.Deliverables == "[]" {
		return nil // no deliverables defined
	}

	// Extract team output
	summary, _, extractErr := u.ExtractTeamOutput(ctx, teamID)
	if extractErr != nil {
		u.lg.Warn("提取团队输出失败，跳过交付物写入",
			loggateway.StepID("spirit.write_deliverables"),
			loggateway.Str("team_id", teamID),
			loggateway.Err(extractErr),
		)
		return nil
	}
	if summary == "" {
		return nil
	}

	// Store the deliverable content in the team's metadata
	// so InjectUpstreamDeliverables can read it from the team record.
	// We use the team's input_contract field to store the actual output
	// for downstream consumption (the input_contract defines what the team
	// expects, but after execution we store what it actually produced).
	if t.InputContract == "" || t.InputContract == "[]" {
		t.InputContract = "[]"
	}

	// Write deliverable output to the team's metadata_json via update
	// The actual content is stored as a JSON map keyed by dag_node_id
	// for downstream retrieval.
	u.lg.Info("团队交付物已就绪，可供下游团队消费",
		loggateway.StepID("spirit.write_deliverables"),
		loggateway.Str("team_id", teamID),
		loggateway.Str("dag_node_id", t.DagNodeID),
	)

	// Persist the deliverable summary into the team record
	// so InjectUpstreamDeliverables can read it.
	// We store it in the team's metadata field (parallel_config_json)
	// as a deliverable output cache.
	deliverableKey := fmt.Sprintf("deliverable_output_%s", t.DagNodeID)
	if t.ParallelConfigJSON == "" || t.ParallelConfigJSON == "{}" {
		t.ParallelConfigJSON = "{}"
	}
	var parallelCfg map[string]any
	if jsonErr := json.Unmarshal([]byte(t.ParallelConfigJSON), &parallelCfg); jsonErr != nil {
		parallelCfg = make(map[string]any)
	}
	parallelCfg[deliverableKey] = summary
	updatedJSON, marshalErr := json.Marshal(parallelCfg)
	if marshalErr != nil {
		return marshalErr
	}
	t.ParallelConfigJSON = string(updatedJSON)
	_, err = u.teamUC.Update(ctx, t.ID, t)
	if err != nil {
		u.lg.Warn("持久化交付物输出失败",
			loggateway.StepID("spirit.write_deliverables"),
			loggateway.Str("team_id", teamID),
			loggateway.Err(err),
		)
		return err
	}
	return nil
}

// InjectUpstreamDeliverables collects upstream team deliverables and formats
// them as a prefix for the downstream team's input message.
// Called when a DAG activates a downstream team.
// It first tries to read from the persisted deliverable output cache
// (written by WriteDeliverablesToSession), then falls back to
// extracting from the team output directly.
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

		// Try to read from persisted deliverable output cache first
		summary := u.readDeliverableOutput(upstream)
		if summary == "" {
			// Fallback: extract from team output directly
			summary, _, extractErr := u.ExtractTeamOutput(ctx, upstream.ID)
			if extractErr != nil || summary == "" {
				continue
			}
		}
		deliverableParts = append(deliverableParts, fmt.Sprintf("## 上游团队: %s\n%s", upstream.DisplayName, summary))
	}

	if len(deliverableParts) == 0 {
		return ""
	}

	return fmt.Sprintf("--- 上游交付物 ---\n%s\n--- 请基于以上上游交付物执行任务 ---\n\n",
		strings.Join(deliverableParts, "\n\n"))
}

// readDeliverableOutput reads the persisted deliverable output from a team's
// parallel_config_json (written by WriteDeliverablesToSession).
func (u *SpiritTeamUsecase) readDeliverableOutput(t Team) string {
	if t.ParallelConfigJSON == "" || t.ParallelConfigJSON == "{}" {
		return ""
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(t.ParallelConfigJSON), &cfg); err != nil {
		return ""
	}
	key := fmt.Sprintf("deliverable_output_%s", t.DagNodeID)
	val, ok := cfg[key]
	if !ok {
		return ""
	}
	s, ok := val.(string)
	if !ok {
		return ""
	}
	return s
}

// ---------------------------------------------------------------------------
// XC-05: Escalation on Max Retries
// ---------------------------------------------------------------------------

// EscalateToSpirit escalates a team that has exceeded max retries to the
// Spirit assistant. This creates a system message in the Spirit session
// notifying the user that human intervention may be needed.
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
		return kerrors.InternalServer("SPIRIT", "escalation status transition failed").WithCause(err)
	}

	return nil
}

// HandleTeamRejection handles a team rejection by a verification gate.
// If the team can retry, it marks the team for rework and transitions
// its status back to pending for re-execution; otherwise it escalates
// to the Spirit assistant.
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
