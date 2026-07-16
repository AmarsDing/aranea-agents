package biz

import (
	"context"
	"strings"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// Stability:stable
type TeamReader interface {
	ListTeams(ctx context.Context) ([]Team, error)
	ListTeamsByStatus(ctx context.Context, status string) ([]Team, error)
	GetTeamByID(ctx context.Context, id string) (Team, error)
	GetTeamByKey(ctx context.Context, teamKey string) (Team, error)
	ListBySpiritSessionID(ctx context.Context, spiritSessionID string) ([]Team, error)
	ListTeamsByDepartmentID(ctx context.Context, deptID string) ([]Team, error)
	// ListTeamsByWorkspace returns teams visible to the given workspace (P2-B).
	// empty workspaceID = system caller (see all); non-empty = tenant caller
	// (see shared + own).
	// Stability:stable
	ListTeamsByWorkspace(ctx context.Context, workspaceID string) ([]Team, error)
	// CountTeamsByWorkspace returns the count of teams visible to the workspace
	// (same visibility rules as ListTeamsByWorkspace).
	// Stability:stable
	CountTeamsByWorkspace(ctx context.Context, workspaceID string) (int, error)
}

// Stability:stable
type TeamWriter interface {
	CreateTeam(ctx context.Context, t Team) (Team, error)
	UpdateTeam(ctx context.Context, t Team) (Team, error)
	DeleteTeam(ctx context.Context, id string) error
	BatchArchiveTeams(ctx context.Context, ids []string) (int, error)
	// UpdateTeamWhereStatus performs a Compare-And-Swap update on the status
	// field: the row is updated only if its current status equals
	// expectedCurrentStatus. Returns true if the row was updated, false if the
	// current status did not match (concurrent modification).
	// Stability:stable
	UpdateTeamWhereStatus(ctx context.Context, id, newStatus, expectedCurrentStatus string) (bool, error)
}

// Stability:stable
type TeamRunReader interface {
	ListTeamRuns(ctx context.Context, teamID string, limit int) ([]TeamRunRecord, error)
	ListTeamRunsByTeamIDs(ctx context.Context, teamIDs []string, limit int) (map[string][]TeamRunRecord, error)
	HasActiveTeamRun(ctx context.Context, teamID string) (bool, error)
	GetTeamRunByID(ctx context.Context, id string) (TeamRunRecord, error)
	ListTeamRunSteps(ctx context.Context, runID string) ([]TeamRunStep, error)
}

// Stability:stable
type TeamRunWriter interface {
	CreateTeamRun(ctx context.Context, r TeamRunRecord) (TeamRunRecord, error)
	UpdateTeamRun(ctx context.Context, r TeamRunRecord) error
	UpdateTeamRunGraphExecutionID(ctx context.Context, runID, graphExecutionID string) error
	UpdateTeamRunTraceID(ctx context.Context, runID, traceID string) error
	UpdateTeamRunSummaryJSON(ctx context.Context, runID, summaryJSON string) error
	CreateTeamRunStep(ctx context.Context, s TeamRunStep) (TeamRunStep, error)
	// UpdateTeamRunWhereStatus performs a Compare-And-Swap update on the status
	// field: the row is updated only if its current status equals
	// expectedCurrentStatus. Returns true if the row was updated, false if the
	// current status did not match (concurrent modification). Terminal statuses
	// also set finished_at.
	// Stability:stable
	UpdateTeamRunWhereStatus(ctx context.Context, runID, newStatus, expectedCurrentStatus string) (bool, error)
}

// Stability:evolving
type OrchestrationStepRepo interface {
	BatchCreateOrchestrationSteps(ctx context.Context, steps []OrchestrationStep) error
	ListOrchestrationSteps(ctx context.Context, teamRunID, nodeID string, limit int) ([]OrchestrationStep, error)
}

// Stability:evolving
type TaskDeadLetterRepo interface {
	CreateTaskDeadLetter(ctx context.Context, dl TaskDeadLetter) error
	ListTaskDeadLetters(ctx context.Context, filter TaskDeadLetterListFilter) ([]TaskDeadLetter, error)
	ResolveTaskDeadLetter(ctx context.Context, id string) (TaskDeadLetter, error)
}

// Stability:stable
type TeamRunRepo interface {
	TeamRunReader
	TeamRunWriter
}

type TeamUsecase struct {
	reader       TeamReader
	writer       TeamWriter
	runReader    TeamRunReader
	runWriter    TeamRunWriter
	stepRepo     OrchestrationStepRepo
	deadLetter   TaskDeadLetterRepo
	agentChecker AgentIDExistenceChecker
	deptLeadMgr  *DeptLeadManager
	graphReader  GraphReader
	graphWriter  GraphWriter
	lg           loggateway.Logger
}

// TeamUsecaseOpts groups all dependencies for NewTeamUsecase.
// Using an options struct keeps the constructor signature stable as
// new dependencies are added (CS-B7).
type TeamUsecaseOpts struct {
	Reader       TeamReader
	Writer       TeamWriter
	RunReader    TeamRunReader
	RunWriter    TeamRunWriter
	StepRepo     OrchestrationStepRepo
	DeadLetter   TaskDeadLetterRepo
	AgentChecker AgentIDExistenceChecker
	DeptLeadMgr  *DeptLeadManager
	GraphReader  GraphReader
	GraphWriter  GraphWriter
	Lg           loggateway.Logger
}

func NewTeamUsecase(opts TeamUsecaseOpts) *TeamUsecase {
	return &TeamUsecase{
		reader:       opts.Reader,
		writer:       opts.Writer,
		runReader:    opts.RunReader,
		runWriter:    opts.RunWriter,
		stepRepo:     opts.StepRepo,
		deadLetter:   opts.DeadLetter,
		agentChecker: opts.AgentChecker,
		deptLeadMgr:  opts.DeptLeadMgr,
		graphReader:  opts.GraphReader,
		graphWriter:  opts.GraphWriter,
		lg:           opts.Lg,
	}
}

func defaultTeamDefinitionJSON() string {
	out, _ := OrchestrationSpecToDefinitionJSON(DefaultOrchestrationSpec())
	if out == "" {
		return `{"version":2,"mode":"sequential","runtime_engine":"graph","team_graph_runtime":true,"members":[],"max_concurrency":2,"timeout_seconds":600}`
	}
	return out
}

func validateTeamDefinition(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	spec, err := ParseOrchestrationSpec(raw)
	if err != nil {
		return apierror.BadRequest("TEAM", "definition_json must be valid JSON")
	}
	mode := firstNonEmpty(spec.Mode, TeamModeSequential)
	switch mode {
	case TeamModeSequential, TeamModeParallel, TeamModeCoordinator, TeamModeCriticLoop, TeamModeSwarm, TeamModeAdaptive:
	default:
		return apierror.BadRequest("TEAM", "unsupported team orchestration mode")
	}
	if len(spec.Members) == 0 {
		return nil
	}
	enabledCount := 0
	hasSynthesizer := false
	hasGenerator := false
	hasCritic := false
	hasCoordinator := false
	for _, member := range spec.Members {
		if strings.TrimSpace(member.AgentID) == "" {
			return apierror.BadRequest("TEAM", "team member agent_id is required")
		}
		if member.Enabled() {
			enabledCount++
		}
		switch member.Role {
		case RoleSynthesizer:
			hasSynthesizer = true
		case RoleGenerator:
			hasGenerator = true
		case RoleCritic:
			hasCritic = true
		case RoleCoordinator:
			hasCoordinator = true
		}
	}
	// Validate role compatibility with mode.
	validRoles := validRolesForMode(mode)
	for _, member := range spec.Members {
		role := strings.TrimSpace(member.Role)
		if role == "" {
			continue
		}
		if validRoles != nil && !validRoles[role] {
			return apierror.BadRequest("TEAM", "role %s is not compatible with mode %s", role, mode)
		}
	}
	if enabledCount == 0 {
		return apierror.BadRequest("TEAM", "team must have at least one enabled member")
	}
	if mode == TeamModeParallel && !hasSynthesizer && strings.TrimSpace(spec.SynthesizerAgentID) == "" && enabledCount > 1 {
		return apierror.BadRequest("TEAM", "parallel mode requires a synthesizer member or synthesizer_agent_id")
	}
	if mode == TeamModeCoordinator && !hasSynthesizer && !hasCoordinator && strings.TrimSpace(spec.SynthesizerAgentID) == "" {
		return apierror.BadRequest("TEAM", "coordinator mode requires a synthesizer or coordinator member, or synthesizer_agent_id")
	}
	if mode == TeamModeCriticLoop && (!hasGenerator || !hasCritic) {
		return apierror.BadRequest("TEAM", "critic_loop mode requires generator and critic members")
	}
	return nil
}

// validRolesForMode returns the set of roles allowed for a given orchestration mode.
// Empty role (default member) is always allowed.
func validRolesForMode(mode string) map[string]bool {
	switch mode {
	case TeamModeCriticLoop:
		return map[string]bool{RoleGenerator: true, RoleCritic: true, RoleSynthesizer: true}
	case TeamModeParallel:
		return map[string]bool{RoleSynthesizer: true, RoleWorker: true}
	case TeamModeCoordinator:
		return map[string]bool{RoleCoordinator: true, RoleWorker: true, RoleSynthesizer: true}
	case TeamModeSequential:
		return map[string]bool{RoleWorker: true}
	case TeamModeSwarm, TeamModeAdaptive:
		// These modes accept any role; no restriction.
		return nil
	default:
		return map[string]bool{}
	}
}

// validateTeamMembersExist checks that all agent members in the team definition
// exist and are active. Validation runs at create/update time only; it does not
// retroactively enforce active status for existing teams whose members are later
// deactivated.
func (u *TeamUsecase) validateTeamMembersExist(ctx context.Context, raw string) error {
	if u.agentChecker == nil || strings.TrimSpace(raw) == "" {
		return nil
	}
	spec, err := ParseOrchestrationSpec(raw)
	if err != nil {
		return apierror.BadRequest("TEAM", "invalid team definition JSON: %s", err.Error())
	}
	for _, member := range spec.Members {
		aid := strings.TrimSpace(member.AgentID)
		if aid == "" {
			continue
		}
		if !u.agentChecker.AgentExistsByID(ctx, aid) {
			return apierror.BadRequest("TEAM", "team member agent %s does not exist", aid)
		}
		if !u.agentChecker.AgentIsActiveByID(ctx, aid) {
			return apierror.BadRequest("TEAM", "team member agent %s is not active", aid)
		}
	}
	return nil
}

// RecoverOrphanedRunningTeams transitions all running teams to interrupted
// and their running runs to failed. Called on server startup to clean up
// stale state from a previous crash.
func (u *TeamUsecase) RecoverOrphanedRunningTeams(ctx context.Context) ([]Team, error) {
	teams, err := u.ListTeamsByStatus(ctx, TeamStatusRunning)
	if err != nil {
		return nil, err
	}
	if len(teams) == 0 {
		return nil, nil
	}
	var recovered []Team
	for i := range teams {
		team, err := u.TransitionStatusWithReason(ctx, teams[i].ID, TeamStatusInterrupted, "服务器重启")
		if err != nil {
			u.lg.Warn("recover orphaned teams: failed to transition team to interrupted",
				loggateway.Str("team_id", teams[i].ID),
				loggateway.Err(err),
			)
			continue
		}
		recovered = append(recovered, team)
		orphanRecoveryMaxRuns := 10
		runs, err := u.ListRuns(ctx, teams[i].ID, orphanRecoveryMaxRuns)
		if err != nil {
			u.lg.Warn("recover orphaned teams: failed to list team runs",
				loggateway.Str("team_id", teams[i].ID),
				loggateway.Err(err),
			)
			continue
		}
		for _, run := range runs {
			if _, tErr := u.TransitionRunStatus(ctx, run.ID, TeamRunStatusFailed); tErr != nil {
				u.lg.Warn("recover orphaned teams: failed to transition team run to failed",
					loggateway.Str("team_run_id", run.ID),
					loggateway.Err(tErr),
				)
			}
		}
	}
	return recovered, nil
}

func (u *TeamUsecase) List(ctx context.Context) ([]Team, error) {
	return u.reader.ListTeams(ctx)
}

func (u *TeamUsecase) ListTeamsByStatus(ctx context.Context, status string) ([]Team, error) {
	return u.reader.ListTeamsByStatus(ctx, status)
}

func (u *TeamUsecase) ListBySpiritSessionID(ctx context.Context, spiritSessionID string) ([]Team, error) {
	return u.reader.ListBySpiritSessionID(ctx, spiritSessionID)
}

// ListByWorkspace returns teams visible to the given workspace (P2-B).
// empty workspaceID = system caller (see all); non-empty = tenant caller.
func (u *TeamUsecase) ListByWorkspace(ctx context.Context, workspaceID string) ([]Team, error) {
	return u.reader.ListTeamsByWorkspace(ctx, workspaceID)
}

// CountByWorkspace returns how many teams are visible to the workspace (P2-B).
func (u *TeamUsecase) CountByWorkspace(ctx context.Context, workspaceID string) (int, error) {
	return u.reader.CountTeamsByWorkspace(ctx, workspaceID)
}

func (u *TeamUsecase) Get(ctx context.Context, id string) (Team, error) {
	id, err := requireNonEmpty(id, "TEAM", "id")
	if err != nil {
		return Team{}, err
	}
	return u.reader.GetTeamByID(ctx, id)
}

// EnabledMemberAgentIDs returns the agent IDs of all enabled team members.
// This satisfies the usage.TeamQuotaReader interface.
func (u *TeamUsecase) EnabledMemberAgentIDs(ctx context.Context, teamID string) ([]string, error) {
	t, err := u.Get(ctx, teamID)
	if err != nil {
		return nil, err
	}
	spec, err := ParseOrchestrationSpec(t.DefinitionJSON)
	if err != nil {
		return nil, apierror.BadRequest("TEAM", "invalid definition_json: %s", err.Error())
	}
	ids := make([]string, 0, len(spec.Members))
	for _, m := range spec.Members {
		if m.Enabled() && strings.TrimSpace(m.AgentID) != "" {
			ids = append(ids, strings.TrimSpace(m.AgentID))
		}
	}
	return ids, nil
}

func (u *TeamUsecase) Create(ctx context.Context, in Team) (Team, error) {
	in.TeamKey = strings.TrimSpace(in.TeamKey)
	in.DisplayName = strings.TrimSpace(in.DisplayName)
	if in.TeamKey == "" || in.DisplayName == "" {
		return Team{}, apierror.BadRequest("TEAM", "team_key and display_name are required")
	}
	if in.ID == "" {
		in.ID = newAgentCatalogID()
	}
	if in.Status == "" {
		in.Status = TeamStatusPending
	}
	if in.DefinitionJSON == "" {
		in.DefinitionJSON = defaultTeamDefinitionJSON()
	} else {
		in.DefinitionJSON = EnsureGraphRuntimeDefault(in.DefinitionJSON)
	}
	if err := validateTeamDefinition(in.DefinitionJSON); err != nil {
		return Team{}, err
	}
	if err := u.validateTeamMembersExist(ctx, in.DefinitionJSON); err != nil {
		return Team{}, err
	}
	// Auto-inherit dept lead agent ID from department
	if in.DepartmentID != "" && in.DeptLeadAgentID == "" && u.deptLeadMgr != nil {
		lead, dlErr := u.deptLeadMgr.GetDeptLeadForTeam(ctx, in.DepartmentID)
		if dlErr == nil && lead != nil {
			in.DeptLeadAgentID = lead.ID
		}
	}
	// Validate borrow ratio: cross-dept members must not exceed 50% of total
	if in.DepartmentID != "" && u.deptLeadMgr != nil {
		memberIDs := u.extractEnabledMemberIDs(in.DefinitionJSON)
		if len(memberIDs) > 0 {
			if ratioErr := u.deptLeadMgr.ValidateBorrowRatio(ctx, in.DepartmentID, memberIDs); ratioErr != nil {
				return Team{}, ratioErr
			}
		}
	}
	return u.writer.CreateTeam(ctx, in)
}

func (u *TeamUsecase) GetRunObservatory(ctx context.Context, runID string) (TeamRunObservatory, error) {
	run, err := u.GetRun(ctx, runID)
	if err != nil {
		return TeamRunObservatory{}, err
	}
	steps, err := u.ListRunSteps(ctx, run.ID)
	if err != nil {
		return TeamRunObservatory{}, err
	}
	definitionJSON := strings.TrimSpace(run.DefinitionSnapshotJSON)
	if definitionJSON == "" {
		teamRow, terr := u.Get(ctx, run.TeamID)
		if terr != nil {
			return TeamRunObservatory{}, terr
		}
		definitionJSON = teamRow.DefinitionJSON
	}
	return BuildTeamRunObservatory(run, steps, definitionJSON), nil
}

func (u *TeamUsecase) HasActiveRun(ctx context.Context, teamID string) (bool, error) {
	teamID, err := requireNonEmpty(teamID, "TEAM", "team_id")
	if err != nil {
		return false, err
	}
	return u.runReader.HasActiveTeamRun(ctx, teamID)
}

func (u *TeamUsecase) Update(ctx context.Context, id string, patch Team) (Team, error) {
	id, err := requireNonEmpty(id, "TEAM", "id")
	if err != nil {
		return Team{}, err
	}
	active, err := u.HasActiveRun(ctx, id)
	if err != nil {
		return Team{}, err
	}
	if active {
		return Team{}, apierror.Conflict("TEAM", "team has an active run; orchestration is read-only until the run finishes")
	}
	current, err := u.reader.GetTeamByID(ctx, id)
	if err != nil {
		return Team{}, err
	}
	current.TeamKey = strings.TrimSpace(firstNonEmpty(patch.TeamKey, current.TeamKey))
	current.DisplayName = strings.TrimSpace(firstNonEmpty(patch.DisplayName, current.DisplayName))
	// Status changes must go through TransitionStatus/TransitionStatusWithReason
	// which enforce the state machine. UpdateTeam only accepts the current status
	// (no-op) or rejects the change. Direct status modification via UpdateTeam
	// is forbidden to prevent invalid state transitions.
	if patchStatus := strings.TrimSpace(patch.Status); patchStatus != "" && patchStatus != current.Status {
		return Team{}, apierror.BadRequest("TEAM", "status changes must use TransitionStatus, not UpdateTeam; current=%s patch=%s", current.Status, patchStatus)
	}
	current.DefinitionJSON = firstNonEmpty(patch.DefinitionJSON, current.DefinitionJSON)
	current.ADKAppName = patch.ADKAppName
	if patch.ADKAppName == "" {
		current.ADKAppName = current.TeamKey
	}
	current.DepartmentID = firstNonEmpty(patch.DepartmentID, current.DepartmentID)
	current.LinkedGraphID = firstNonEmpty(patch.LinkedGraphID, current.LinkedGraphID)
	if err := validateTeamDefinition(current.DefinitionJSON); err != nil {
		return Team{}, err
	}
	if err := u.validateTeamMembersExist(ctx, current.DefinitionJSON); err != nil {
		return Team{}, err
	}

	// ORG-11b: Sync Graph.team_id when Team.linked_graph_id changes
	updated, updateErr := u.writer.UpdateTeam(ctx, current)
	if updateErr != nil {
		return Team{}, updateErr
	}
	u.syncGraphTeamID(ctx, current.LinkedGraphID, updated.LinkedGraphID, updated.ID)
	return updated, nil
}

// TransitionStatus validates and applies a team status transition.
// It checks the transition is allowed by the state machine and updates
// only the status field, bypassing the HasActiveRun guard (status
// transitions are part of the team lifecycle and must work during runs).
//
// CAS (Compare-And-Swap) is used to prevent TOCTOU race conditions: the
// status is only updated if the current DB status still matches what we
// read. If a concurrent modification occurred, Conflict is returned.
func (u *TeamUsecase) TransitionStatus(ctx context.Context, id string, newStatus string) (Team, error) {
	id, err := requireNonEmpty(id, "TEAM", "id")
	if err != nil {
		return Team{}, err
	}
	current, err := u.reader.GetTeamByID(ctx, id)
	if err != nil {
		return Team{}, err
	}
	sm := NewTeamStateMachine()
	if !sm.CanTransition(TeamState(current.Status), TeamState(newStatus)) {
		return Team{}, apierror.BadRequest("TEAM", "invalid team status transition: %s → %s", current.Status, newStatus)
	}
	// CAS: only update if current status hasn't changed (prevents TOCTOU race)
	updated, err := u.writer.UpdateTeamWhereStatus(ctx, id, newStatus, current.Status)
	if err != nil {
		return Team{}, err
	}
	if !updated {
		return Team{}, apierror.Conflict("TEAM", "team status changed concurrently; please retry")
	}
	return u.reader.GetTeamByID(ctx, id)
}

// TransitionStatusWithReason transitions the team status and sets an interrupt reason
// when transitioning to interrupted status.
//
// CAS is used for the status field; the interrupt reason is set in a
// separate best-effort update after CAS succeeds. This means a crash between
// the two updates could leave the status changed but the reason empty —
// acceptable because the reason is informational, not a consistency invariant.
func (u *TeamUsecase) TransitionStatusWithReason(ctx context.Context, id string, newStatus string, reason string) (Team, error) {
	id, err := requireNonEmpty(id, "TEAM", "id")
	if err != nil {
		return Team{}, err
	}
	current, err := u.reader.GetTeamByID(ctx, id)
	if err != nil {
		return Team{}, err
	}
	sm := NewTeamStateMachine()
	if !sm.CanTransition(TeamState(current.Status), TeamState(newStatus)) {
		return Team{}, apierror.BadRequest("TEAM", "invalid team status transition: %s → %s", current.Status, newStatus)
	}
	// CAS: only update if current status hasn't changed (prevents TOCTOU race)
	updated, err := u.writer.UpdateTeamWhereStatus(ctx, id, newStatus, current.Status)
	if err != nil {
		return Team{}, err
	}
	if !updated {
		return Team{}, apierror.Conflict("TEAM", "team status changed concurrently; please retry")
	}
	// Best-effort: set interrupt reason after CAS succeeds.
	if newStatus == TeamStatusInterrupted && reason != "" {
		current.Status = newStatus
		current.InterruptReason = reason
		if _, rErr := u.writer.UpdateTeam(ctx, current); rErr != nil {
			u.lg.Warn("best-effort interrupt reason update failed",
				loggateway.Str("team_id", id),
				loggateway.Err(rErr))
		}
	}
	return u.reader.GetTeamByID(ctx, id)
}

// RetryTeam resets a failed or cancelled team to pending so it can be re-started.
// Only failed/cancelled teams are eligible; other states return BadRequest.
func (u *TeamUsecase) RetryTeam(ctx context.Context, id string) (Team, error) {
	id, err := requireNonEmpty(id, "TEAM", "id")
	if err != nil {
		return Team{}, err
	}
	return u.TransitionStatus(ctx, id, TeamStatusPending)
}

// ResumeTeamIfInterrupted transitions an interrupted team to running.
// This is a no-op if the team is not in interrupted status, allowing
// safe call from service layer after graph execution resume.
func (u *TeamUsecase) ResumeTeamIfInterrupted(ctx context.Context, id string) error {
	id, err := requireNonEmpty(id, "TEAM", "id")
	if err != nil {
		return err
	}
	current, err := u.reader.GetTeamByID(ctx, id)
	if err != nil {
		return err
	}
	if current.Status != TeamStatusInterrupted {
		return nil // not interrupted, no transition needed
	}
	_, err = u.TransitionStatus(ctx, id, TeamStatusRunning)
	return err
}

// BatchArchiveTeams archives multiple teams in a single DB operation.
// It validates each team's current status allows archiving before proceeding.
func (u *TeamUsecase) BatchArchiveTeams(ctx context.Context, ids []string) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	return u.writer.BatchArchiveTeams(ctx, ids)
}

func (u *TeamUsecase) Delete(ctx context.Context, id string) error {
	id, err := requireNonEmpty(id, "TEAM", "id")
	if err != nil {
		return err
	}
	team, err := u.reader.GetTeamByID(ctx, id)
	if err != nil {
		return err
	}
	if team.Kind == "system_builtin" {
		return apierror.Forbidden("TEAM", "cannot delete system_builtin team")
	}
	// ecosystem_preset teams must be deleted via industry unload to keep ecosystem_loaded status consistent.
	if team.Kind == "ecosystem_preset" {
		return apierror.Forbidden("TEAM", "cannot delete ecosystem_preset team directly; use industry unload instead")
	}
	if team.IsDefault {
		return apierror.Conflict("TEAM", "default team cannot be deleted")
	}
	if team.Readonly {
		return apierror.Forbidden("TEAM", "cannot delete a readonly team")
	}
	active, err := u.HasActiveRun(ctx, id)
	if err != nil {
		return err
	}
	if active {
		return apierror.Conflict("TEAM", "team has an active run; delete is not allowed until the run finishes")
	}

	// ORG-11c: Clean up Graph association on team deletion
	if team.LinkedGraphID != "" && u.graphReader != nil && u.graphWriter != nil {
		graphDef, graphErr := u.graphReader.GetDefinition(ctx, team.LinkedGraphID)
		if graphErr == nil && graphDef != nil {
			if graphDef.IsTemplate {
				// Template Graph: only clear team_id reference
				graphDef.TeamID = ""
				if _, updateErr := u.graphWriter.UpdateDefinition(ctx, graphDef); updateErr != nil {
					u.lg.Warn("best-effort update failed", loggateway.Err(updateErr))
				}
			} else {
				// Exclusive Graph: delete along with the team
				if deleteErr := u.graphWriter.DeleteDefinition(ctx, team.LinkedGraphID); deleteErr != nil {
					u.lg.Warn("best-effort delete failed", loggateway.Err(deleteErr))
				}
			}
		}
	}

	return u.writer.DeleteTeam(ctx, id)
}

func (u *TeamUsecase) Duplicate(ctx context.Context, id string) (Team, error) {
	id, err := requireNonEmpty(id, "TEAM", "id")
	if err != nil {
		return Team{}, err
	}
	current, err := u.reader.GetTeamByID(ctx, id)
	if err != nil {
		return Team{}, err
	}
	if current.Readonly {
		return Team{}, apierror.BadRequest("TEAM", "readonly teams cannot be duplicated")
	}
	dup := Team{
		TeamKey:            current.TeamKey + "-copy-" + newAgentCatalogID(),
		DisplayName:        current.DisplayName + " Copy",
		DefinitionJSON:     current.DefinitionJSON,
		ADKAppName:         current.TeamKey + "-copy-" + newAgentCatalogID(),
		DepartmentID:       current.DepartmentID,
		Deliverables:       current.Deliverables,
		InputContract:      current.InputContract,
		CrossDeptMemberIDs: current.CrossDeptMemberIDs,
		Kind:               "user",
		Source:             "user",
		TaskDescription:    current.TaskDescription,
		ParallelConfigJSON: current.ParallelConfigJSON,
		Topology:           current.Topology,
		WorkspaceID:        current.WorkspaceID, // P2-B: inherit source workspace
	}
	// Delegate to Create which validates definition_json and member existence.
	return u.Create(ctx, dup)
}

func (u *TeamUsecase) ListRuns(ctx context.Context, teamID string, limit int) ([]TeamRunRecord, error) {
	return u.runReader.ListTeamRuns(ctx, teamID, limit)
}

func (u *TeamUsecase) ListRunsByTeamIDs(ctx context.Context, teamIDs []string, limit int) (map[string][]TeamRunRecord, error) {
	if len(teamIDs) == 0 {
		return nil, nil
	}
	return u.runReader.ListTeamRunsByTeamIDs(ctx, teamIDs, limit)
}

func (u *TeamUsecase) GetRun(ctx context.Context, id string) (TeamRunRecord, error) {
	id, err := requireNonEmpty(id, "TEAM", "id")
	if err != nil {
		return TeamRunRecord{}, err
	}
	return u.runReader.GetTeamRunByID(ctx, id)
}

func (u *TeamUsecase) UpdateRun(ctx context.Context, r TeamRunRecord) error {
	if strings.TrimSpace(r.ID) == "" {
		return apierror.BadRequest("TEAM", "run id is required")
	}
	// Status changes must go through TransitionRunStatus/CancelRun, which
	// enforce the TeamRunStateMachine and CAS. UpdateRun only persists
	// non-status field changes; any status mismatch is rejected to prevent
	// callers from bypassing the state machine.
	current, err := u.runReader.GetTeamRunByID(ctx, r.ID)
	if err != nil {
		return err
	}
	newStatus := strings.TrimSpace(r.Status)
	if newStatus == "" {
		r.Status = current.Status
	} else if newStatus != current.Status {
		return apierror.BadRequest("TEAM", "team run status changes must use TransitionRunStatus, not UpdateRun; current=%s requested=%s", current.Status, newStatus)
	}
	return u.runWriter.UpdateTeamRun(ctx, r)
}

// CancelRun cancels a team run if it is in running or pending status.
// Returns the updated run or an error if the run cannot be cancelled.
// Uses TeamRunStateMachine to validate the transition.
func (u *TeamUsecase) CancelRun(ctx context.Context, runID string) (TeamRunRecord, error) {
	return u.TransitionRunStatus(ctx, runID, TeamRunStatusCancelled)
}

// TransitionRunStatus validates and applies a team run status transition
// using the TeamRunStateMachine. This is the single authoritative path
// for all team run status changes — no caller should modify run.Status
// directly.
//
// CAS (Compare-And-Swap) is used to prevent TOCTOU race conditions: the
// status is only updated if the current DB status still matches what we
// read. If a concurrent modification occurred, Conflict is returned.
func (u *TeamUsecase) TransitionRunStatus(ctx context.Context, runID string, newStatus string) (TeamRunRecord, error) {
	runID, err := requireNonEmpty(runID, "TEAM", "run_id")
	if err != nil {
		return TeamRunRecord{}, err
	}
	r, err := u.runReader.GetTeamRunByID(ctx, runID)
	if err != nil {
		return TeamRunRecord{}, err
	}
	sm := NewTeamRunStateMachine()
	if !sm.CanTransition(TeamRunState(r.Status), TeamRunState(newStatus)) {
		return TeamRunRecord{}, apierror.BadRequest("TEAM", "invalid team run status transition: %s → %s", r.Status, newStatus)
	}
	// CAS: only update if current status hasn't changed (prevents TOCTOU race).
	// UpdateTeamRunWhereStatus also sets finished_at for terminal statuses.
	updated, err := u.runWriter.UpdateTeamRunWhereStatus(ctx, runID, newStatus, r.Status)
	if err != nil {
		return TeamRunRecord{}, err
	}
	if !updated {
		return TeamRunRecord{}, apierror.Conflict("TEAM", "team run status changed concurrently; please retry")
	}
	return u.runReader.GetTeamRunByID(ctx, runID)
}

func (u *TeamUsecase) UpdateRunSummaryJSON(ctx context.Context, runID, summaryJSON string) error {
	if strings.TrimSpace(runID) == "" {
		return apierror.BadRequest("TEAM", "run id is required")
	}
	return u.runWriter.UpdateTeamRunSummaryJSON(ctx, runID, summaryJSON)
}

func (u *TeamUsecase) UpdateRunTraceID(ctx context.Context, runID, traceID string) error {
	runID = strings.TrimSpace(runID)
	traceID = strings.TrimSpace(traceID)
	if runID == "" || traceID == "" {
		return nil
	}
	return u.runWriter.UpdateTeamRunTraceID(ctx, runID, traceID)
}

func (u *TeamUsecase) ListRunSteps(ctx context.Context, runID string) ([]TeamRunStep, error) {
	runID, err := requireNonEmpty(runID, "TEAM", "run_id")
	if err != nil {
		return nil, err
	}
	return u.runReader.ListTeamRunSteps(ctx, runID)
}

func (u *TeamUsecase) ListRunObservatoryTimeline(ctx context.Context, runID, nodeID string, limit int) ([]OrchestrationStep, error) {
	runID, err := requireNonEmpty(runID, "TEAM", "run_id")
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	return u.stepRepo.ListOrchestrationSteps(ctx, runID, strings.TrimSpace(nodeID), limit)
}

func (u *TeamUsecase) GetRunSummary(ctx context.Context, runID string) (TeamRunSummaryData, error) {
	run, err := u.GetRun(ctx, runID)
	if err != nil {
		return TeamRunSummaryData{}, err
	}
	steps, err := u.ListRunSteps(ctx, run.ID)
	if err != nil {
		return TeamRunSummaryData{}, err
	}
	return BuildTeamRunSummaryData(run, steps), nil
}

func (u *TeamUsecase) UpdateSwarmMembers(ctx context.Context, teamID string, addIDs []string, removeIDs []string) (bool, error) {
	teamID = strings.TrimSpace(teamID)
	if teamID == "" {
		return false, apierror.BadRequest("TEAM", "team_id is required")
	}
	active, err := u.HasActiveRun(ctx, teamID)
	if err != nil {
		return false, err
	}
	if active {
		return false, apierror.BadRequest("TEAM", "cannot update members while team has active run")
	}
	t, err := u.reader.GetTeamByID(ctx, teamID)
	if err != nil {
		return false, err
	}
	spec, err := ParseOrchestrationSpec(t.DefinitionJSON)
	if err != nil {
		return false, apierror.BadRequest("TEAM", "invalid definition_json")
	}
	mode := strings.ToLower(strings.TrimSpace(spec.Mode))
	if mode != TeamModeSwarm && mode != TeamModeAdaptive {
		return false, apierror.BadRequest("TEAM", "swarm member management only applies to swarm or adaptive mode")
	}
	removeSet := make(map[string]bool, len(removeIDs))
	for _, id := range removeIDs {
		removeSet[strings.TrimSpace(id)] = true
	}
	var filtered []OrchestrationMember
	for _, m := range spec.Members {
		if removeSet[strings.TrimSpace(m.AgentID)] {
			continue
		}
		filtered = append(filtered, m)
	}
	for _, id := range addIDs {
		aid := strings.TrimSpace(id)
		if aid == "" {
			continue
		}
		filtered = append(filtered, OrchestrationMember{AgentID: aid, Role: RoleWorker, EnabledPtr: boolPtr(true)})
	}
	spec.Members = filtered
	updatedJSON, err := OrchestrationSpecToDefinitionJSON(spec)
	if err != nil {
		return false, apierror.Internal("TEAM", "failed to marshal updated definition")
	}
	t.DefinitionJSON = updatedJSON
	if err := validateTeamDefinition(t.DefinitionJSON); err != nil {
		return false, err
	}
	// Validate that all added members exist and are active (consistent with Create/Update).
	if err := u.validateTeamMembersExist(ctx, t.DefinitionJSON); err != nil {
		return false, err
	}
	_, err = u.writer.UpdateTeam(ctx, t)
	if err != nil {
		return false, err
	}
	return true, nil
}

func (u *TeamUsecase) ExportStructure(ctx context.Context, teamID string) (*TeamStructureSnapshot, error) {
	teamID = strings.TrimSpace(teamID)
	if teamID == "" {
		return nil, apierror.BadRequest("TEAM", "team_id is required")
	}
	t, err := u.reader.GetTeamByID(ctx, teamID)
	if err != nil {
		return nil, err
	}
	spec, err := ParseOrchestrationSpec(t.DefinitionJSON)
	if err != nil {
		return nil, apierror.BadRequest("TEAM", "invalid definition_json")
	}
	mode := strings.ToLower(strings.TrimSpace(spec.Mode))
	snapshot := &TeamStructureSnapshot{
		EntryNodeID: "team-" + t.TeamKey,
		Nodes:       []StructureNode{{NodeID: "team-" + t.TeamKey, Kind: "team", Name: t.DisplayName}},
	}
	switch mode {
	case TeamModeCoordinator:
		for i, m := range spec.Members {
			nid := m.AgentID
			snapshot.Nodes = append(snapshot.Nodes, StructureNode{NodeID: nid, Kind: "agent", Name: m.Name})
			if i == 0 {
				snapshot.Edges = append(snapshot.Edges, StructureEdge{FromNodeID: "team-" + t.TeamKey, ToNodeID: nid})
			} else {
				snapshot.Edges = append(snapshot.Edges, StructureEdge{FromNodeID: spec.Members[0].AgentID, ToNodeID: nid})
			}
		}
	case TeamModeSwarm, TeamModeAdaptive:
		for i, m := range spec.Members {
			nid := m.AgentID
			snapshot.Nodes = append(snapshot.Nodes, StructureNode{NodeID: nid, Kind: "agent", Name: m.Name})
			if i == 0 {
				snapshot.Edges = append(snapshot.Edges, StructureEdge{FromNodeID: "team-" + t.TeamKey, ToNodeID: nid})
			}
			for j, other := range spec.Members {
				if i != j {
					snapshot.Edges = append(snapshot.Edges, StructureEdge{FromNodeID: nid, ToNodeID: other.AgentID})
				}
			}
		}
	default:
		for _, m := range spec.Members {
			nid := m.AgentID
			snapshot.Nodes = append(snapshot.Nodes, StructureNode{NodeID: nid, Kind: "agent", Name: m.Name})
			snapshot.Edges = append(snapshot.Edges, StructureEdge{FromNodeID: "team-" + t.TeamKey, ToNodeID: nid})
		}
	}
	return snapshot, nil
}

func (uc *TeamUsecase) ListTaskDeadLetters(ctx context.Context, filter TaskDeadLetterListFilter) ([]TaskDeadLetter, error) {
	if uc == nil || uc.deadLetter == nil {
		return nil, nil
	}
	return uc.deadLetter.ListTaskDeadLetters(ctx, filter)
}

func (uc *TeamUsecase) ResolveTaskDeadLetter(ctx context.Context, id string) (TaskDeadLetter, error) {
	if uc == nil || uc.deadLetter == nil {
		return TaskDeadLetter{}, ErrNotFound
	}
	return uc.deadLetter.ResolveTaskDeadLetter(ctx, id)
}

func boolPtr(v bool) *bool { return &v }

// extractEnabledMemberIDs parses the team definition JSON and returns the
// agent IDs of all enabled members.
func (u *TeamUsecase) extractEnabledMemberIDs(definitionJSON string) []string {
	spec, err := ParseOrchestrationSpec(definitionJSON)
	if err != nil {
		return nil
	}
	ids := make([]string, 0, len(spec.Members))
	for _, m := range spec.Members {
		if m.Enabled() && strings.TrimSpace(m.AgentID) != "" {
			ids = append(ids, strings.TrimSpace(m.AgentID))
		}
	}
	return ids
}

// ResolveMemberAgentKeys 将 Team definition_json 中的 agent_key 解析为 agent_id。
// 用于 Pack 导入场景：导入时 Team 成员使用 agent_key 引用，需转为实际 agent_id。
// agentKeyResolver 接收 agent_key 返回 (agent_id, error)。
func (u *TeamUsecase) ResolveMemberAgentKeys(ctx context.Context, raw string, agentKeyResolver func(string) (string, error)) (string, error) {
	if strings.TrimSpace(raw) == "" || agentKeyResolver == nil {
		return raw, nil
	}
	spec, err := ParseOrchestrationSpec(raw)
	if err != nil {
		return "", apierror.BadRequest("TEAM", "invalid definition_json: %s", err.Error())
	}
	for i := range spec.Members {
		m := &spec.Members[i]
		if strings.TrimSpace(m.AgentID) == "" {
			continue
		}
		// 如果 AgentID 看起来像 agent_key（非 hex 格式），尝试解析
		if !isHexID(m.AgentID) {
			resolved, err := agentKeyResolver(m.AgentID)
			if err != nil {
				return "", apierror.BadRequest("TEAM", "agent_key %s 解析失败: %s", m.AgentID, err.Error())
			}
			m.AgentID = resolved
		}
	}
	if spec.IntentAnchorAgentID != "" && !isHexID(spec.IntentAnchorAgentID) {
		resolved, err := agentKeyResolver(spec.IntentAnchorAgentID)
		if err != nil {
			return "", apierror.BadRequest("TEAM", "intent_anchor agent_key %s 解析失败: %s", spec.IntentAnchorAgentID, err.Error())
		}
		spec.IntentAnchorAgentID = resolved
	}
	if spec.SynthesizerAgentID != "" && !isHexID(spec.SynthesizerAgentID) {
		resolved, err := agentKeyResolver(spec.SynthesizerAgentID)
		if err != nil {
			return "", apierror.BadRequest("TEAM", "synthesizer agent_key %s 解析失败: %s", spec.SynthesizerAgentID, err.Error())
		}
		spec.SynthesizerAgentID = resolved
	}
	return OrchestrationSpecToDefinitionJSON(spec)
}

// isHexID 判断字符串是否为 hex 格式的数据库 ID（24 位 hex）。
func isHexID(s string) bool {
	if len(s) != 24 {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// SaveTeamWithGraph 保存 Team 并关联 Graph。
// 如果 OrchestrationSpec 包含 linked_graph_id，确保 Graph 存在后保存 Team。
// graphSaver 接收 graph_id，返回 (new_graph_id, error)，用于 ID 映射。
func (u *TeamUsecase) SaveTeamWithGraph(ctx context.Context, team Team, graphIDMapper func(string) (string, error)) (Team, error) {
	team.TeamKey = strings.TrimSpace(team.TeamKey)
	team.DisplayName = strings.TrimSpace(team.DisplayName)
	if team.TeamKey == "" || team.DisplayName == "" {
		return Team{}, apierror.BadRequest("TEAM", "team_key and display_name are required")
	}

	// 处理 linked_graph_id 映射
	if graphIDMapper != nil && strings.TrimSpace(team.DefinitionJSON) != "" {
		spec, err := ParseOrchestrationSpec(team.DefinitionJSON)
		if err == nil && spec.LinkedGraphID != "" {
			newID, err := graphIDMapper(spec.LinkedGraphID)
			if err == nil {
				spec.LinkedGraphID = newID
				updated, err := OrchestrationSpecToDefinitionJSON(spec)
				if err == nil {
					team.DefinitionJSON = updated
				}
			}
		}
	}

	if team.ID == "" {
		team.ID = newAgentCatalogID()
	}
	if team.Status == "" {
		team.Status = TeamStatusPending
	}
	if team.DefinitionJSON == "" {
		team.DefinitionJSON = defaultTeamDefinitionJSON()
	} else {
		team.DefinitionJSON = EnsureGraphRuntimeDefault(team.DefinitionJSON)
	}
	if err := validateTeamDefinition(team.DefinitionJSON); err != nil {
		return Team{}, err
	}
	if err := u.validateTeamMembersExist(ctx, team.DefinitionJSON); err != nil {
		return Team{}, err
	}
	return u.writer.CreateTeam(ctx, team)
}

// syncGraphTeamID updates Graph.team_id when Team.linked_graph_id changes.
// ORG-11b: When a team's linked_graph_id is set or changed, the Graph's
// team_id field must be updated to maintain the bidirectional reference.
// Best-effort: errors are logged but do not fail the parent operation.
func (u *TeamUsecase) syncGraphTeamID(ctx context.Context, oldGraphID, newGraphID, teamID string) {
	if u.graphReader == nil || u.graphWriter == nil {
		return
	}

	// Clear team_id on the old graph if it changed
	if oldGraphID != "" && oldGraphID != newGraphID {
		oldGraph, err := u.graphReader.GetDefinition(ctx, oldGraphID)
		if err == nil && oldGraph != nil && oldGraph.TeamID == teamID {
			oldGraph.TeamID = ""
			if _, updateErr := u.graphWriter.UpdateDefinition(ctx, oldGraph); updateErr != nil {
				u.lg.Warn("best-effort update failed", loggateway.Err(updateErr))
			}
		}
	}

	// Set team_id on the new graph
	if newGraphID != "" {
		newGraph, err := u.graphReader.GetDefinition(ctx, newGraphID)
		if err == nil && newGraph != nil {
			newGraph.TeamID = teamID
			if _, updateErr := u.graphWriter.UpdateDefinition(ctx, newGraph); updateErr != nil {
				u.lg.Warn("best-effort update failed", loggateway.Err(updateErr))
			}
		}
	}
}
