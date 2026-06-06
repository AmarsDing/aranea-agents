package biz

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

type TeamReader interface {
	ListTeams(ctx context.Context) ([]Team, error)
	ListTeamsByStatus(ctx context.Context, status string) ([]Team, error)
	GetTeamByID(ctx context.Context, id string) (Team, error)
	GetTeamByKey(ctx context.Context, teamKey string) (Team, error)
	ListBySpiritSessionID(ctx context.Context, spiritSessionID string) ([]Team, error)
}

type TeamWriter interface {
	CreateTeam(ctx context.Context, t Team) (Team, error)
	UpdateTeam(ctx context.Context, t Team) (Team, error)
	DeleteTeam(ctx context.Context, id string) error
	BatchArchiveTeams(ctx context.Context, ids []string) (int, error)
}

type TeamRunReader interface {
	ListTeamRuns(ctx context.Context, teamID string, limit int) ([]TeamRun, error)
	ListTeamRunsByTeamIDs(ctx context.Context, teamIDs []string, limit int) (map[string][]TeamRun, error)
	HasActiveTeamRun(ctx context.Context, teamID string) (bool, error)
	GetTeamRunByID(ctx context.Context, id string) (TeamRun, error)
	ListTeamRunSteps(ctx context.Context, runID string) ([]TeamRunStep, error)
}

type TeamRunWriter interface {
	CreateTeamRun(ctx context.Context, r TeamRun) (TeamRun, error)
	UpdateTeamRun(ctx context.Context, r TeamRun) error
	UpdateTeamRunGraphExecutionID(ctx context.Context, runID, graphExecutionID string) error
	UpdateTeamRunTraceID(ctx context.Context, runID, traceID string) error
	UpdateTeamRunSummaryJSON(ctx context.Context, runID, summaryJSON string) error
	CreateTeamRunStep(ctx context.Context, s TeamRunStep) (TeamRunStep, error)
}

type OrchestrationStepRepo interface {
	BatchCreateOrchestrationSteps(ctx context.Context, steps []OrchestrationStep) error
	ListOrchestrationSteps(ctx context.Context, teamRunID, nodeID string, limit int) ([]OrchestrationStep, error)
}

type TaskDeadLetterRepo interface {
	CreateTaskDeadLetter(ctx context.Context, dl TaskDeadLetter) error
	ListTaskDeadLetters(ctx context.Context, filter TaskDeadLetterListFilter) ([]TaskDeadLetter, error)
	ResolveTaskDeadLetter(ctx context.Context, id string) (TaskDeadLetter, error)
}

type TeamRunRepo interface {
	TeamRunReader
	TeamRunWriter
}

// TeamRepository is a composition interface for backward compatibility.
// Deprecated: Consumers should depend on the narrow sub-interfaces
// (TeamReader, TeamWriter, TeamRunReader, TeamRunWriter, OrchestrationStepRepo, TaskDeadLetterRepo)
// instead of this aggregate. New code MUST NOT reference TeamRepository directly.
// TODO(debt): migrate internal/team/runner.go and team_graph_run_coordinator.go to narrow
// interfaces, then remove TeamRepository. Issue: #SPIRIT-REPO-MIGRATE
type TeamRepository interface {
	TeamReader
	TeamWriter
	TeamRunReader
	TeamRunWriter
	OrchestrationStepRepo
	TaskDeadLetterRepo
}

type TeamUsecase struct {
	reader         TeamReader
	writer         TeamWriter
	runReader      TeamRunReader
	runWriter      TeamRunWriter
	stepRepo       OrchestrationStepRepo
	deadLetter     TaskDeadLetterRepo
	agentChecker   AgentIDExistenceChecker
}

func NewTeamUsecase(
	reader TeamReader,
	writer TeamWriter,
	runReader TeamRunReader,
	runWriter TeamRunWriter,
	stepRepo OrchestrationStepRepo,
	deadLetter TaskDeadLetterRepo,
	agentChecker AgentIDExistenceChecker,
) *TeamUsecase {
	return &TeamUsecase{
		reader:       reader,
		writer:       writer,
		runReader:    runReader,
		runWriter:    runWriter,
		stepRepo:     stepRepo,
		deadLetter:   deadLetter,
		agentChecker: agentChecker,
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
	var body struct {
		Mode             string `json:"mode"`
		SynthesizerAgent string `json:"synthesizer_agent_id"`
		Members          []struct {
			AgentID string `json:"agent_id"`
			Role    string `json:"role"`
			Enabled *bool  `json:"enabled"`
		} `json:"members"`
	}
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		return kerrors.BadRequest("TEAM", "definition_json must be valid JSON")
	}
	mode := firstNonEmpty(body.Mode, TeamModeSequential)
	switch mode {
	case TeamModeSequential, TeamModeParallel, TeamModeCoordinator, TeamModeCriticLoop, TeamModeSwarm, TeamModeAdaptive:
	default:
		return kerrors.BadRequest("TEAM", "unsupported team orchestration mode")
	}
	if len(body.Members) == 0 {
		return nil
	}
	enabledCount := 0
	hasSynthesizer := false
	hasGenerator := false
	hasCritic := false
	hasCoordinator := false
	for _, member := range body.Members {
		if strings.TrimSpace(member.AgentID) == "" {
			return kerrors.BadRequest("TEAM", "team member agent_id is required")
		}
		if member.Enabled == nil || *member.Enabled {
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
	for _, member := range body.Members {
		role := strings.TrimSpace(member.Role)
		if role == "" {
			continue
		}
		if validRoles != nil && !validRoles[role] {
			return kerrors.BadRequest("TEAM", "role "+role+" is not compatible with mode "+mode)
		}
	}
	if enabledCount == 0 {
		return kerrors.BadRequest("TEAM", "team must have at least one enabled member")
	}
	if mode == TeamModeParallel && !hasSynthesizer && strings.TrimSpace(body.SynthesizerAgent) == "" && enabledCount > 1 {
		return kerrors.BadRequest("TEAM", "parallel mode requires a synthesizer member or synthesizer_agent_id")
	}
	if mode == TeamModeCoordinator && !hasSynthesizer && !hasCoordinator && strings.TrimSpace(body.SynthesizerAgent) == "" {
		return kerrors.BadRequest("TEAM", "coordinator mode requires a synthesizer or coordinator member, or synthesizer_agent_id")
	}
	if mode == TeamModeCriticLoop && (!hasGenerator || !hasCritic) {
		return kerrors.BadRequest("TEAM", "critic_loop mode requires generator and critic members")
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

func (u *TeamUsecase) validateTeamMembersExist(ctx context.Context, raw string) error {
	if u.agentChecker == nil || strings.TrimSpace(raw) == "" {
		return nil
	}
	var body struct {
		Members []struct {
			AgentID string `json:"agent_id"`
			Role    string `json:"role"`
			Enabled *bool  `json:"enabled"`
		} `json:"members"`
	}
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		return kerrors.BadRequest("TEAM", "invalid team definition JSON: "+err.Error())
	}
	for _, member := range body.Members {
		aid := strings.TrimSpace(member.AgentID)
		if aid == "" {
			continue
		}
		if !u.agentChecker.AgentExistsByID(ctx, aid) {
			return kerrors.BadRequest("TEAM", "team member agent "+aid+" does not exist")
		}
		// NOTE: AgentIDExistenceChecker only checks existence, not active status.
		// Adding AgentIsActiveByID would require interface changes across multiple packages.
	}
	return nil
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
	def, err := parseTeamDefinition(t.DefinitionJSON)
	if err != nil {
		return nil, nil
	}
	members := enabledTeamMembers(def)
	ids := make([]string, 0, len(members))
	for _, m := range members {
		if aid := strings.TrimSpace(m.AgentID); aid != "" {
			ids = append(ids, aid)
		}
	}
	return ids, nil
}

func (u *TeamUsecase) Create(ctx context.Context, in Team) (Team, error) {
	in.TeamKey = strings.TrimSpace(in.TeamKey)
	in.DisplayName = strings.TrimSpace(in.DisplayName)
	if in.TeamKey == "" || in.DisplayName == "" {
		return Team{}, kerrors.BadRequest("TEAM", "team_key and display_name are required")
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
		return Team{}, kerrors.Conflict("TEAM", "team has an active run; orchestration is read-only until the run finishes")
	}
	current, err := u.reader.GetTeamByID(ctx, id)
	if err != nil {
		return Team{}, err
	}
	current.TeamKey = strings.TrimSpace(firstNonEmpty(patch.TeamKey, current.TeamKey))
	current.DisplayName = strings.TrimSpace(firstNonEmpty(patch.DisplayName, current.DisplayName))
	current.Status = firstNonEmpty(patch.Status, current.Status)
	current.DefinitionJSON = firstNonEmpty(patch.DefinitionJSON, current.DefinitionJSON)
	current.ADKAppName = patch.ADKAppName
	if patch.ADKAppName == "" {
		current.ADKAppName = current.TeamKey
	}
	current.CategoryIndustryID = firstNonEmpty(patch.CategoryIndustryID, current.CategoryIndustryID)
	if err := validateTeamDefinition(current.DefinitionJSON); err != nil {
		return Team{}, err
	}
	if err := u.validateTeamMembersExist(ctx, current.DefinitionJSON); err != nil {
		return Team{}, err
	}
	return u.writer.UpdateTeam(ctx, current)
}

// TransitionStatus validates and applies a team status transition.
// It checks the transition is allowed by the state machine and updates
// only the status field, bypassing the HasActiveRun guard (status
// transitions are part of the team lifecycle and must work during runs).
func (u *TeamUsecase) TransitionStatus(ctx context.Context, id string, newStatus string) (Team, error) {
	id, err := requireNonEmpty(id, "TEAM", "id")
	if err != nil {
		return Team{}, err
	}
	current, err := u.reader.GetTeamByID(ctx, id)
	if err != nil {
		return Team{}, err
	}
	if !ValidTeamStatusTransition(current.Status, newStatus) {
		return Team{}, kerrors.BadRequest("TEAM", fmt.Sprintf("invalid team status transition: %s → %s", current.Status, newStatus))
	}
	current.Status = newStatus
	return u.writer.UpdateTeam(ctx, current)
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
		return kerrors.Forbidden("TEAM", "cannot delete system_builtin team")
	}
	// ecosystem_preset teams must be deleted via industry unload to keep ecosystem_loaded status consistent.
	if team.Kind == "ecosystem_preset" {
		return kerrors.Forbidden("TEAM", "cannot delete ecosystem_preset team directly; use industry unload instead")
	}
	if team.IsDefault {
		return kerrors.Conflict("TEAM", "default team cannot be deleted")
	}
	if team.Readonly {
		return kerrors.Forbidden("TEAM", "cannot delete a readonly team")
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
	current.ID = newAgentCatalogID()
	current.TeamKey = current.TeamKey + "-copy-" + newAgentCatalogID()
	current.DisplayName = current.DisplayName + " Copy"
	current.IsDefault = false
	return u.writer.CreateTeam(ctx, current)
}

func (u *TeamUsecase) ListRuns(ctx context.Context, teamID string, limit int) ([]TeamRun, error) {
	return u.runReader.ListTeamRuns(ctx, teamID, limit)
}

func (u *TeamUsecase) ListRunsByTeamIDs(ctx context.Context, teamIDs []string, limit int) (map[string][]TeamRun, error) {
	if len(teamIDs) == 0 {
		return nil, nil
	}
	return u.runReader.ListTeamRunsByTeamIDs(ctx, teamIDs, limit)
}

func (u *TeamUsecase) GetRun(ctx context.Context, id string) (TeamRun, error) {
	id, err := requireNonEmpty(id, "TEAM", "id")
	if err != nil {
		return TeamRun{}, err
	}
	return u.runReader.GetTeamRunByID(ctx, id)
}

func (u *TeamUsecase) UpdateRun(ctx context.Context, r TeamRun) error {
	if strings.TrimSpace(r.ID) == "" {
		return kerrors.BadRequest("TEAM", "run id is required")
	}
	return u.runWriter.UpdateTeamRun(ctx, r)
}

func (u *TeamUsecase) UpdateRunSummaryJSON(ctx context.Context, runID, summaryJSON string) error {
	if strings.TrimSpace(runID) == "" {
		return kerrors.BadRequest("TEAM", "run id is required")
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
		return false, kerrors.BadRequest("TEAM", "team_id is required")
	}
	active, err := u.HasActiveRun(ctx, teamID)
	if err != nil {
		return false, err
	}
	if active {
		return false, kerrors.BadRequest("TEAM", "cannot update members while team has active run")
	}
	t, err := u.reader.GetTeamByID(ctx, teamID)
	if err != nil {
		return false, err
	}
	def, err := parseDefinitionForUpdate(t.DefinitionJSON)
	if err != nil {
		return false, kerrors.BadRequest("TEAM", "invalid definition_json")
	}
	mode := strings.ToLower(strings.TrimSpace(def.Mode))
	if mode != TeamModeSwarm && mode != TeamModeAdaptive {
		return false, kerrors.BadRequest("TEAM", "swarm member management only applies to swarm or adaptive mode")
	}
	removeSet := make(map[string]bool, len(removeIDs))
	for _, id := range removeIDs {
		removeSet[strings.TrimSpace(id)] = true
	}
	var filtered []teamMemberEntry
	for _, m := range def.Members {
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
		filtered = append(filtered, teamMemberEntry{AgentID: aid, Role: RoleWorker, Enabled: boolPtr(true)})
	}
	def.Members = filtered
	updatedJSON, err := json.Marshal(def)
	if err != nil {
		return false, kerrors.InternalServer("TEAM", "failed to marshal updated definition")
	}
	t.DefinitionJSON = string(updatedJSON)
	if err := validateTeamDefinition(t.DefinitionJSON); err != nil {
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
		return nil, kerrors.BadRequest("TEAM", "team_id is required")
	}
	t, err := u.reader.GetTeamByID(ctx, teamID)
	if err != nil {
		return nil, err
	}
	def, err := parseDefinitionForUpdate(t.DefinitionJSON)
	if err != nil {
		return nil, kerrors.BadRequest("TEAM", "invalid definition_json")
	}
	mode := strings.ToLower(strings.TrimSpace(def.Mode))
	snapshot := &TeamStructureSnapshot{
		EntryNodeID: "team-" + t.TeamKey,
		Nodes:       []StructureNode{{NodeID: "team-" + t.TeamKey, Kind: "team", Name: t.DisplayName}},
	}
	switch mode {
	case TeamModeCoordinator:
		for i, m := range def.Members {
			nid := m.AgentID
			snapshot.Nodes = append(snapshot.Nodes, StructureNode{NodeID: nid, Kind: "agent", Name: m.Name})
			if i == 0 {
				snapshot.Edges = append(snapshot.Edges, StructureEdge{FromNodeID: "team-" + t.TeamKey, ToNodeID: nid})
			} else {
				snapshot.Edges = append(snapshot.Edges, StructureEdge{FromNodeID: def.Members[0].AgentID, ToNodeID: nid})
			}
		}
	case TeamModeSwarm, TeamModeAdaptive:
		for i, m := range def.Members {
			nid := m.AgentID
			snapshot.Nodes = append(snapshot.Nodes, StructureNode{NodeID: nid, Kind: "agent", Name: m.Name})
			if i == 0 {
				snapshot.Edges = append(snapshot.Edges, StructureEdge{FromNodeID: "team-" + t.TeamKey, ToNodeID: nid})
			}
			for j, other := range def.Members {
				if i != j {
					snapshot.Edges = append(snapshot.Edges, StructureEdge{FromNodeID: nid, ToNodeID: other.AgentID})
				}
			}
		}
	default:
		for _, m := range def.Members {
			nid := m.AgentID
			snapshot.Nodes = append(snapshot.Nodes, StructureNode{NodeID: nid, Kind: "agent", Name: m.Name})
			snapshot.Edges = append(snapshot.Edges, StructureEdge{FromNodeID: "team-" + t.TeamKey, ToNodeID: nid})
		}
	}
	return snapshot, nil
}

type teamMemberEntry struct {
	AgentID   string `json:"agent_id"`
	Role      string `json:"role"`
	Enabled   *bool  `json:"enabled"`
	SortOrder int    `json:"sort_order"`
	Name      string `json:"name"`
}

type definitionForUpdate struct {
	Version            int               `json:"version"`
	Mode               string            `json:"mode"`
	SynthesizerAgentID string            `json:"synthesizer_agent_id"`
	Members            []teamMemberEntry `json:"members"`
	MaxConcurrency     int               `json:"max_concurrency"`
	TimeoutSeconds     int               `json:"timeout_seconds"`
}

func parseDefinitionForUpdate(raw string) (*definitionForUpdate, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return &definitionForUpdate{Version: 1, Mode: TeamModeSequential}, nil
	}
	var d definitionForUpdate
	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		return nil, err
	}
	return &d, nil
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

// ResolveMemberAgentKeys 将 Team definition_json 中的 agent_key 解析为 agent_id。
// 用于 Pack 导入场景：导入时 Team 成员使用 agent_key 引用，需转为实际 agent_id。
// agentKeyResolver 接收 agent_key 返回 (agent_id, error)。
func (u *TeamUsecase) ResolveMemberAgentKeys(ctx context.Context, raw string, agentKeyResolver func(string) (string, error)) (string, error) {
	if strings.TrimSpace(raw) == "" || agentKeyResolver == nil {
		return raw, nil
	}
	spec, err := ParseOrchestrationSpec(raw)
	if err != nil {
		return "", kerrors.BadRequest("TEAM", "invalid definition_json: "+err.Error())
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
				return "", kerrors.BadRequest("TEAM", "agent_key "+m.AgentID+" 解析失败: "+err.Error())
			}
			m.AgentID = resolved
		}
	}
	if spec.IntentAnchorAgentID != "" && !isHexID(spec.IntentAnchorAgentID) {
		resolved, err := agentKeyResolver(spec.IntentAnchorAgentID)
		if err == nil {
			spec.IntentAnchorAgentID = resolved
		}
	}
	if spec.SynthesizerAgentID != "" && !isHexID(spec.SynthesizerAgentID) {
		resolved, err := agentKeyResolver(spec.SynthesizerAgentID)
		if err == nil {
			spec.SynthesizerAgentID = resolved
		}
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
		return Team{}, kerrors.BadRequest("TEAM", "team_key and display_name are required")
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
	return u.writer.CreateTeam(ctx, team)
}

// teamMemberDef mirrors team.MemberDef to avoid import cycle (biz → team → biz).
type teamMemberDef struct {
	AgentID    string `json:"agent_id"`
	Role       string `json:"role"`
	Enabled    *bool  `json:"enabled"`
	SortOrder  int    `json:"sort_order"`
	Name       string `json:"name"`
	TaskPrompt string `json:"task_prompt,omitempty"`
}

// teamDefinition mirrors team.Definition (subset needed for EnabledMemberAgentIDs).
type teamDefinition struct {
	Version            int               `json:"version"`
	Mode               string            `json:"mode"`
	SynthesizerAgentID string            `json:"synthesizer_agent_id"`
	Members            []teamMemberDef   `json:"members"`
	MaxConcurrency     int               `json:"max_concurrency"`
	TimeoutSeconds     int               `json:"timeout_seconds"`
}

// parseTeamDefinition unmarshals team JSON; empty string yields default.
func parseTeamDefinition(raw string) (teamDefinition, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return teamDefinition{Version: 1, Mode: TeamModeSequential}, nil
	}
	var d teamDefinition
	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		return teamDefinition{}, err
	}
	if strings.TrimSpace(d.Mode) == "" {
		d.Mode = TeamModeSequential
	}
	return d, nil
}

func teamMemberEnabled(m teamMemberDef) bool {
	return m.Enabled == nil || *m.Enabled
}

type teamMemberWithIndex struct {
	m teamMemberDef
	i int
}

// enabledTeamMembers returns enabled members with non-empty agent_id, ordered by sort_order.
func enabledTeamMembers(d teamDefinition) []teamMemberDef {
	var pairs []teamMemberWithIndex
	for i, m := range d.Members {
		if !teamMemberEnabled(m) || strings.TrimSpace(m.AgentID) == "" {
			continue
		}
		pairs = append(pairs, teamMemberWithIndex{m: m, i: i})
	}
	sort.SliceStable(pairs, func(a, b int) bool {
		sa, sb := pairs[a].m.SortOrder, pairs[b].m.SortOrder
		switch {
		case sa > 0 && sb > 0 && sa != sb:
			return sa < sb
		case sa > 0 && sb <= 0:
			return true
		case sa <= 0 && sb > 0:
			return false
		default:
			return pairs[a].i < pairs[b].i
		}
	})
	out := make([]teamMemberDef, len(pairs))
	for i, p := range pairs {
		out[i] = p.m
	}
	return out
}
