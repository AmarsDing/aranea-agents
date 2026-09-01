package biz

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"aranea-agents/internal/biz/session"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	"github.com/google/uuid"
)

// SpiritAssembly owns team/member/session assembly for Spirit orchestration (DEV-09).
type SpiritAssembly struct {
	teamUC      SpiritTeamAssembler
	sessionUC   SpiritSessionAccessor
	agentUC     SpiritAgentResolver
	transactor  SpiritTransactor
	deptLeadMgr *DeptLeadManager
	orch        *SpiritOrchestration
	lg          loggateway.Logger
	deptMailbox *DeptMailboxUsecase // P2: borrow 前置协商通知
}

// Domain: Assembly — team creation and composition.
func (a *SpiritAssembly) AssembleTeam(ctx context.Context, params SpiritTeamParams) (SpiritTeamResult, error) {
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
	//   - Status is pending or running (not terminal). Running hits are
	//     reused so a duplicate Orchestrate cannot mint a second team; the
	//     caller must not StartTeamTurn again (see RealTeamOrchestrator).
	//   - Not deleted
	// This prevents duplicate team/agent creation when the same task is
	// re-executed (e.g. user retries, orchestrator re-runs), which was causing
	// management bloat.
	if reusable, ok := a.findReusableTeam(ctx, spiritSessionID, taskDesc, params.DagNodeID); ok {
		a.lg.Info("AssembleTeam 命中可复用团队，跳过创建",
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
	keyToID, resolveErr := a.resolveAgentKeyToIDMap(ctx, params.AgentKeys)
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

	// 2026-07-28 Fix: a single-member team has nothing to coordinate — normalize
	// coordinator to sequential so buildSpiritTeamDefinitionJSON builds the only
	// member as a worker (executes tools) instead of a synthesizer (coordination
	// role; hallucinated success with zero tool calls on install-style tasks).
	// Normalized here (not inside the builder) so Team.Topology stays consistent
	// with DefinitionJSON. Also satisfies validateTeamDefinition, which rejects
	// coordinator teams without a synthesizer/coordinator member.
	if mode == TeamModeCoordinator && len(agentIDs) <= 1 {
		mode = TeamModeSequential
	}
	if route := ResolveGraphTemplateRoute(mode, params.GraphTemplateID); route.Builtin && route.Mode != "" {
		mode = route.Mode
	}

	// F9 (Phase 11): a solo system-admin team with a skill-install intent gets
	// a deterministic tool_assertion gate so "installed" can never be
	// hallucinated — the gate checks the skill is actually enabled.
	var gates []VerificationGate
	if g := skillInstallAssertionGate(params.AgentKeys, params.TaskDescription); g != nil {
		gates = append(gates, *g)
	}
	defJSON := buildSpiritTeamDefinitionJSON(mode, agentIDs, a.lg, params.DagNodeID != "", params.Deliverables, gates, params.ParallelConfigJSON)
	faceKeys := append(append([]string{}, params.AgentKeys...), agentIDs...)
	defJSON = ApplyAssembleOrgFaces(defJSON, faceKeys, params.GraphTemplateID, params.CollectionIDs)

	// Check session tree depth limit before creating team (P1-4: extracted
	// to session.ValidateDepth for reuse across all child-session creators).
	cfg := a.orch.resolveParallelConfig(ctx, spiritSessionID)
	parentSession, err := a.sessionUC.Get(ctx, spiritSessionID)
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
	err = a.transactor.ExecInTx(ctx, func(txCtx context.Context) error {
		team, err := a.teamUC.Create(txCtx, Team{
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

		teamSession, err := a.sessionUC.Create(txCtx, Session{
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
			a.lg.Warn("跳过成员 agent session 创建：深度超限",
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
				// ADR-06 (12:33 修复)：不再跳过系统 Agent。凡进入团队定义的成员
				// （含 Planner 显式指定的 __system_admin__ 等系统 Agent）均创建
				// agent session，保证 MemberSession 生命周期完整（created→updated）。
				// 系统 Agent 的分配约束上移到 Allocator/Planner 层。
				agentID, ok := keyToID[agentKey]
				if !ok {
					a.lg.Warn("AssembleTeam 跳过成员：未解析到 agentID",
						loggateway.StepID("spirit.assemble.agent_session.no_id"),
						loggateway.Str("team_id", team.ID),
						loggateway.Str("agent_key", agentKey))
					continue
				}
				agentSession, aerr := a.sessionUC.Create(txCtx, Session{
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
					a.lg.Warn("创建成员 agent session 失败",
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

	// Team timeout is registered at StartTeamTurn (activation), not assembly.
	// Registering here would fail pending DAG dependents whose upstream is still running.

	// Submit borrow requests for cross-department members (DL-09).
	// These are processed outside the transaction to avoid long-held locks.
	if a.deptLeadMgr != nil && len(params.CrossDeptMemberAgentIDs) > 0 && params.DepartmentID != "" {
		a.submitBorrowRequests(ctx, result.Team.ID, params.DepartmentID, params.CrossDeptMemberAgentIDs)
	}

	// Inject dept lead into team (DL-06): explicit injection point in the
	// Spirit orchestration flow. team_usecase.go auto-inherits DeptLeadAgentID
	// during Create, but this serves as the explicit step per design Phase 3.
	if params.DepartmentID != "" {
		if injectErr := a.InjectDeptLeadIntoTeam(ctx, result.Team.ID); injectErr != nil {
			a.lg.Warn("注入部门主管失败",
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
func (a *SpiritAssembly) findReusableTeam(
	ctx context.Context,
	spiritSessionID, taskDesc, dagNodeID string,
) (SpiritTeamResult, bool) {
	teams, err := a.teamUC.ListBySpiritSessionID(ctx, spiritSessionID)
	if err != nil {
		a.lg.Warn("findReusableTeam 查询团队列表失败，跳过复用",
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
	childSessions, err := a.sessionUC.ListChildSessions(ctx, spiritSessionID)
	if err != nil {
		a.lg.Warn("findReusableTeam 查询子 session 失败，跳过复用",
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
		a.lg.Warn("findReusableTeam 未找到团队 session，跳过复用",
			loggateway.StepID("spirit.assemble.reuse_no_session"),
			loggateway.Str("team_id", matched.ID),
		)
		return SpiritTeamResult{}, false
	}

	// Find member agent sessions (OwnerType="agent", TeamID=matched.ID) via
	// child sessions of the team session.
	memberSessions := make(map[string]string)
	memberChildren, err := a.sessionUC.ListChildSessions(ctx, teamSession.ID)
	if err != nil {
		a.lg.Warn("findReusableTeam 查询成员 session 失败，跳过成员映射",
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
func (a *SpiritAssembly) submitBorrowRequests(ctx context.Context, teamID, homeDeptID string, crossDeptAgentIDs []string) {
	rs := make([]BorrowRequest, 0, len(crossDeptAgentIDs))
	for _, agentID := range crossDeptAgentIDs {
		fromDeptID, err := a.deptLeadMgr.agentDepartment(ctx, agentID)
		if err != nil || fromDeptID == "" || fromDeptID == homeDeptID {
			continue // skip agents without a department or already in home dept
		}
		rs = append(rs, BorrowRequest{
			TeamID:     teamID,
			AgentID:    agentID,
			FromDeptID: fromDeptID,
			ToDeptID:   homeDeptID,
			Reason:     "cross-department member for spirit team",
		})
	}
	if len(rs) == 0 {
		return
	}
	if _, err := a.deptLeadMgr.SubmitBorrowRequests(ctx, rs); err != nil {
		a.lg.Warn("failed to submit borrow requests",
			loggateway.StepID("spirit.borrow.submit"),
			loggateway.Str("team_id", teamID),
			loggateway.Err(err),
		)
		return
	}
	// P2: 借调请求提交后，向出借方部门主管发送 deptmail 协商通知。
	a.notifyBorrowLeads(ctx, teamID, homeDeptID, rs)
}

// notifyBorrowLeads sends deptmail notifications to lending department leads
// after borrow requests are submitted. Best-effort: failures are logged only.
func (a *SpiritAssembly) notifyBorrowLeads(ctx context.Context, teamID, homeDeptID string, requests []BorrowRequest) {
	if a.deptMailbox == nil || a.deptLeadMgr == nil {
		return
	}
	homeLead, err := a.deptLeadMgr.GetDeptLeadForTeam(ctx, homeDeptID)
	if err != nil || homeLead == nil {
		a.lg.Warn("借调协商通知: 借用方部门无主管，跳过",
			loggateway.StepID("spirit.borrow.notify"),
			loggateway.Str("dept_id", homeDeptID),
		)
		return
	}
	for _, r := range requests {
		subject := fmt.Sprintf("借调协商: %s 请求借调 %s", homeDeptID, r.AgentID)
		body := fmt.Sprintf(
			"团队 %s 需要借调贵部门成员 %s。\n\n借用方部门：%s\n出借方部门：%s\n借调原因：%s\n\n请审批借调请求，或回复本消息协商借调条件。",
			teamID, r.AgentID, r.ToDeptID, r.FromDeptID, r.Reason,
		)
		if _, sendErr := a.deptMailbox.SendMessage(ctx, homeLead.ID, r.FromDeptID, subject, body, "[]"); sendErr != nil {
			a.lg.Warn("借调协商通知发送失败",
				loggateway.StepID("spirit.borrow.notify"),
				loggateway.Str("team_id", teamID),
				loggateway.Str("from_dept", r.FromDeptID),
				loggateway.Err(sendErr),
			)
		}
	}
}

func (a *SpiritAssembly) GetTeam(ctx context.Context, teamID string) (Team, error) {
	return a.teamUC.Get(ctx, teamID)
}

func (a *SpiritAssembly) ListActiveTeams(ctx context.Context, spiritSessionID string) ([]Team, error) {
	spiritSessionID = strings.TrimSpace(spiritSessionID)
	if spiritSessionID == "" {
		return nil, apierror.BadRequest("SPIRIT", "spirit_session_id is required")
	}
	teams, err := a.teamUC.ListBySpiritSessionID(ctx, spiritSessionID)
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

func (a *SpiritAssembly) ListAllTeams(ctx context.Context, spiritSessionID string) ([]Team, error) {
	spiritSessionID = strings.TrimSpace(spiritSessionID)
	if spiritSessionID == "" {
		return nil, apierror.BadRequest("SPIRIT", "spirit_session_id is required")
	}
	return a.teamUC.ListBySpiritSessionID(ctx, spiritSessionID)
}

func (a *SpiritAssembly) ListCompletedAndFailedTeams(ctx context.Context, spiritSessionID string) ([]Team, error) {
	spiritSessionID = strings.TrimSpace(spiritSessionID)
	if spiritSessionID == "" {
		return nil, apierror.BadRequest("SPIRIT", "spirit_session_id is required")
	}
	teams, err := a.teamUC.ListBySpiritSessionID(ctx, spiritSessionID)
	if err != nil {
		return nil, err
	}
	var out []Team
	for i := range teams {
		// partial_failure 交付物已产出，与 completed 同列入综合/交付物收集范围。
		if teams[i].Status == TeamStatusCompleted || teams[i].Status == TeamStatusFailed || teams[i].Status == TeamStatusPartialFailure {
			out = append(out, teams[i])
		}
	}
	return out, nil
}

// resolveAgentKeyToIDMap maps agentKeys (e.g. "__spirit__") to agent IDs (e.g.
// "agent___spirit__" or UUID). Pages through ALL active agents (the repo clamps
// a single page to ≤500) and builds a lookup map. Unresolvable keys return an
// error naming them — the former silent "agent_"+key fallback only ever worked
// for system agents whose IDs follow that naming convention, and began
// misfiring once active agents exceeded the old single-page Limit of 200.
func (a *SpiritAssembly) resolveAgentKeyToIDMap(ctx context.Context, agentKeys []string) (map[string]string, error) {
	if len(agentKeys) == 0 {
		return nil, nil
	}
	const pageSize = 500
	keyToID := make(map[string]string)
	for offset := 0; ; offset += pageSize {
		result, err := a.agentUC.List(ctx, AgentListQuery{
			Status: "active",
			Limit:  pageSize,
			Offset: offset,
		})
		if err != nil {
			return nil, err
		}
		for _, a := range result.Items {
			keyToID[a.AgentKey] = a.ID
		}
		if len(result.Items) < pageSize || offset+len(result.Items) >= result.Total {
			break
		}
	}
	out := make(map[string]string, len(agentKeys))
	var missing []string
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
			missing = append(missing, key)
			continue
		}
		out[key] = id
	}
	if len(missing) > 0 {
		return nil, apierror.BadRequest("SPIRIT", "agent keys not found or not active: %s", strings.Join(missing, ", "))
	}
	return out, nil
}

func (a *SpiritAssembly) SearchSessions(ctx context.Context, q session.SessionSearchQuery) (session.SessionListResult, error) {
	return a.sessionUC.Search(ctx, q)
}

func (a *SpiritAssembly) GetSpiritQuery(ctx context.Context, spiritSessionID string) string {
	messages, err := a.sessionUC.ListMessagesRecent(ctx, spiritSessionID, 5)
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
func (a *SpiritAssembly) UpdateTeamDefinitionJSON(ctx context.Context, teamID string, definitionJSON string) error {
	_, err := a.teamUC.Update(ctx, teamID, Team{DefinitionJSON: definitionJSON})
	if err != nil {
		return apierror.Wrap(err, apierror.CodeInternal, "SPIRIT")
	}
	return nil
}

// InjectDeptLeadIntoTeam adds the department lead agent to a team's definition.
// Called during team assembly for cross-department collaboration.
// Domain: Assembly — dept lead injection into team definition.
func (a *SpiritAssembly) InjectDeptLeadIntoTeam(ctx context.Context, teamID string) error {
	if a.deptLeadMgr == nil {
		return nil
	}
	t, err := a.teamUC.Get(ctx, teamID)
	if err != nil {
		return err
	}
	if t.DepartmentID == "" {
		return nil
	}
	lead, err := a.deptLeadMgr.GetDeptLeadForTeam(ctx, t.DepartmentID)
	if err != nil {
		a.lg.Warn("获取部门主管失败，跳过注入",
			loggateway.StepID("spirit.inject_dept_lead"),
			loggateway.Str("team_id", teamID),
			loggateway.Str("dept_id", t.DepartmentID),
			loggateway.Err(err),
		)
		return nil
	}

	// Update the team's DeptLeadAgentID
	_, err = a.teamUC.Update(ctx, teamID, Team{DeptLeadAgentID: lead.ID})
	if err != nil {
		return apierror.Wrap(err, apierror.CodeInternal, "SPIRIT")
	}

	a.lg.Info("注入部门主管到团队",
		loggateway.StepID("spirit.inject_dept_lead"),
		loggateway.Str("team_id", teamID),
		loggateway.Str("dept_lead_agent_id", lead.ID),
	)
	return nil
}

func buildSpiritTeamDefinitionJSON(mode string, agentKeys []string, lg loggateway.Logger, requireDeliverable bool, deliverables []DeliverableContract, gates []VerificationGate, parallelCfgJSON ...string) string {
	type member struct {
		AgentKey   string `json:"agent_id"`
		Role       string `json:"role"`
		Enabled    *bool  `json:"enabled"`
		TaskPrompt string `json:"task_prompt,omitempty"`
	}
	members := make([]member, 0, len(agentKeys))
	// deliverableChannel 与下方 enable_state_deliverable 同规则：无交付通道的
	// 团队（单成员非 DAG）不得让任务书提及 set_deliverable（工具不存在会误导）。
	deliverableChannel := len(agentKeys) > 1 || requireDeliverable
	for i, key := range agentKeys {
		role := RoleWorker
		enabled := true
		if mode == TeamModeCoordinator && i == 0 {
			role = RoleSynthesizer
		}
		if mode == TeamModeParallel && i == len(agentKeys)-1 && len(agentKeys) > 1 {
			role = RoleSynthesizer
		}
		members = append(members, member{
			AgentKey:   strings.TrimSpace(key),
			Role:       role,
			Enabled:    &enabled,
			TaskPrompt: memberRoleTaskPrompt(role, deliverableChannel),
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
	if mode == TeamModeParallel && len(agentKeys) > 1 {
		def["synthesizer_agent_id"] = strings.TrimSpace(agentKeys[len(agentKeys)-1])
	}
	// C1/C3: multi-member teams get the deliverable state channel so members
	// can pass structured output via set_deliverable/get_deliverable tools.
	// 2026-07-25 Fix 2b: DAG teams (requireDeliverable, i.e. DagNodeID != "")
	// get the channel unconditionally — the real-deliverable gate (Fix 1)
	// judges DAG teams solely by set_deliverable output, so a single-member
	// DAG team without the channel could never pass.
	if len(agentKeys) > 1 || requireDeliverable {
		def["enable_state_deliverable"] = true
	}
	// F5 (Phase 11): auto-generate the member-level deliverable contract (MDC)
	// from the team's inter-team deliverable contracts. topic == contract name
	// (1:1), so the member prompt instruction and the spirit's retrieval always
	// agree on the topic. Members write via set_deliverable; the MDC is
	// enforced write-time (schema/required_keys) and advisory at completion.
	if entries := MemberEntriesFromDeliverableContracts(deliverables); len(entries) > 0 {
		def["deliverable_contract"] = MemberDeliverableContract{Entries: entries}
	}
	// F9 (Phase 11): verification gates (e.g. the deterministic tool_assertion
	// gate for skill-install teams) ride the definition JSON;
	// resolveVerificationGates parses them back at completion time.
	if len(gates) > 0 {
		def["verification_gates"] = gates
	}
	out, err := json.Marshal(def)
	if err != nil {
		return "{}"
	}
	return string(out)
}

// memberRoleTaskPrompt 按团队角色生成差异化任务书（2026-09-01 修复：此前
// 成员定义不写 task_prompt，图编译节点 instruction 为空，synthesizer 与
// worker 收到完全相同的输入，分工仅靠各自 system prompt，存在产出重叠与
// 越界风险——如调研角色产出远超设计角色）。任务书经 definition JSON →
// 图编译节点 instruction（embedded_graph.go）→ 注入成员用户消息头部
// （graph/trpc/agent_instruction.go），不触碰 system prompt（保前缀缓存）。
func memberRoleTaskPrompt(role string, deliverableChannel bool) string {
	deliver := "产出通过 set_deliverable 提交。"
	if !deliverableChannel {
		deliver = "产出直接作为你的最终回复。"
	}
	switch role {
	case RoleSynthesizer:
		return "你是本团队的合成者：基于其他成员的产出做整合、去重与冲突裁决，形成团队最终交付物；" + deliver +
			"不要重复执行成员已完成的调研/执行工作，输入中已有成员产出时在其基础上综合而非重做。"
	case RoleWorker:
		return "你是本团队的执行者：只完成与你专业职责直接相关的子任务，聚焦你的专业领域产出具体成果；" + deliver +
			"团队最终汇总由合成者负责，不要代劳；不要越界承担其他成员的职责。"
	default:
		return ""
	}
}
