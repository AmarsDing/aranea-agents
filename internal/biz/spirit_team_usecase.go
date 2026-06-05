package biz

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"aranea-agents/internal/biz/session"
	"aranea-agents/pkg/loggateway"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/google/uuid"
)

type TeamStarterPort interface {
	StartTeamTurn(ctx context.Context, sessionID string, content string) error
	HandleTeamTurnResult(ctx context.Context, spiritSessionID, teamID, status, errMsg string)
}

type SpiritTeamParams struct {
	SpiritSessionID    string
	TaskDescription    string
	AgentKeys          []string
	Mode               string
	DagNodeID          string
	TeamName           string
	TaskSummary        string
	DependsOn          []string
	ParallelConfigJSON string
	TopologyReason     string
	AutoStart          bool
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

type SpiritTeamUsecase struct {
	teamUC     *TeamUsecase
	sessionUC  *SessionUsecase
	agentUC    *AgentUsecase
	transactor SpiritTransactor
	lg         loggateway.Logger
}

func NewSpiritTeamUsecase(teamUC *TeamUsecase, sessionUC *SessionUsecase, agentUC *AgentUsecase, transactor SpiritTransactor, lg loggateway.Logger) *SpiritTeamUsecase {
	return &SpiritTeamUsecase{teamUC: teamUC, sessionUC: sessionUC, agentUC: agentUC, transactor: transactor, lg: lg}
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
		mode = "coordinator"
	}

	defJSON := buildSpiritTeamDefinitionJSON(mode, params.AgentKeys, u.lg, params.ParallelConfigJSON)

	// All teams start as pending regardless of depends_on.
	// AutoStart=true teams transition to running when StartTeamTurn is called.
	// AutoStart=false DAG root nodes stay pending until manually started or
	// scheduled by scheduleDependentTeams.
	initialStatus := TeamStatusPending

	var result SpiritTeamResult
	err := u.transactor.ExecInTx(ctx, func(txCtx context.Context) error {
		team, err := u.teamUC.Create(txCtx, Team{
			TeamKey:           fmt.Sprintf("spirit_%s_%s", spiritSessionID, uuid.New().String()[:8]),
			DisplayName:       TruncateRunes(taskDesc, 64),
			Status:            initialStatus,
			SpiritSessionID:   spiritSessionID,
			TaskDescription:   taskDesc,
			AutoCreated:       true,
			DefinitionJSON:    defJSON,
			DagNodeID:         params.DagNodeID,
			DependsOn:         params.DependsOn,
			ParallelConfigJSON: params.ParallelConfigJSON,
			Topology:          mode,
		})
		if err != nil {
			return kerrors.InternalServer("SPIRIT", "create team: "+err.Error())
		}

		teamSession, err := u.sessionUC.Create(txCtx, Session{
			OwnerType:       "team",
			TeamID:          team.ID,
			ParentSessionID: spiritSessionID,
			RootSessionID:   spiritSessionID,
			AgentDepth:      1,
			Title:           TruncateRunes(taskDesc, 128),
		})
		if err != nil {
			return kerrors.InternalServer("SPIRIT", "create team session: "+err.Error())
		}

		result = SpiritTeamResult{Team: team, Session: teamSession}
		return nil
	})
	if err != nil {
		return SpiritTeamResult{}, err
	}
	return result, nil
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
					Status:   "blocked",
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
	agents, err := u.agentUC.List(ctx, AgentListQuery{Keyword: SpiritAgentKey, Limit: 1})
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
	return ParseParallelConfig(ag.MetadataJSON, u.lg)
}

func (u *SpiritTeamUsecase) CancelTeam(ctx context.Context, teamID string) error {
	if strings.TrimSpace(teamID) == "" {
		return kerrors.BadRequest("SPIRIT", "team_id is required")
	}
	_, err := u.teamUC.TransitionStatus(ctx, teamID, TeamStatusCancelled)
	if err != nil {
		return err
	}
	return nil
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
	messages, msgErr := u.sessionUC.ListMessagesRecent(ctx, teamSession.ID, 10)
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
			summary = TruncateRunes(content, 500)
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
		if (isBullet || isNumbered) && !isQuote && len(findings) < 5 {
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
		runs, runErr := u.teamUC.ListRuns(ctx, teams[i].ID, 10)
		if runErr == nil && len(runs) > 0 {
			latestRun := runs[0]
			tp.DurationMs = int64(latestRun.DurationMS)
			if IsTeamStatusActive(tp.Status) {
				completedRuns := 0
				for _, r := range runs {
					if r.Status == "success" {
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

func buildSpiritTeamDefinitionJSON(mode string, agentKeys []string, lg loggateway.Logger, parallelCfgJSON ...string) string {
	type member struct {
		AgentKey string `json:"agent_key"`
		Role     string `json:"role"`
		Enabled  *bool  `json:"enabled"`
	}
	members := make([]member, 0, len(agentKeys))
	for i, key := range agentKeys {
		role := "worker"
		enabled := true
		if i == 0 && mode == "coordinator" {
			role = "synthesizer"
		}
		members = append(members, member{
			AgentKey: strings.TrimSpace(key),
			Role:     role,
			Enabled:  &enabled,
		})
	}
	maxConcurrency := 2
	if len(parallelCfgJSON) > 0 && parallelCfgJSON[0] != "" {
		cfg := ParseParallelConfig(parallelCfgJSON[0], lg)
		if cfg.MaxConcurrentTeams > 0 {
			maxConcurrency = cfg.MaxConcurrentTeams
		}
	}
	def := map[string]any{
		"version":            2,
		"mode":               mode,
		"runtime_engine":     "graph",
		"team_graph_runtime": true,
		"members":            members,
		"max_concurrency":    maxConcurrency,
		"timeout_seconds":    600,
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
			return TruncateRunes(messages[i].ContentMarkdown, 500)
		}
	}
	return ""
}
