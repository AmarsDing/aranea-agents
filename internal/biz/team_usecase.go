package biz

import (
	"context"
	"encoding/json"
	"strings"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

type TeamRepository interface {
	ListTeams(ctx context.Context) ([]Team, error)
	GetTeamByID(ctx context.Context, id string) (Team, error)
	CreateTeam(ctx context.Context, t Team) (Team, error)
	UpdateTeam(ctx context.Context, t Team) (Team, error)
	DeleteTeam(ctx context.Context, id string) error
	ListTeamRuns(ctx context.Context, teamID string, limit int) ([]TeamRun, error)
	GetTeamRunByID(ctx context.Context, id string) (TeamRun, error)
	ListTeamRunSteps(ctx context.Context, runID string) ([]TeamRunStep, error)
	CreateTeamRun(ctx context.Context, r TeamRun) (TeamRun, error)
	UpdateTeamRun(ctx context.Context, r TeamRun) error
	UpdateTeamRunGraphExecutionID(ctx context.Context, runID, graphExecutionID string) error
	UpdateTeamRunTraceID(ctx context.Context, runID, traceID string) error
	UpdateTeamRunSummaryJSON(ctx context.Context, runID, summaryJSON string) error
	CreateTeamRunStep(ctx context.Context, s TeamRunStep) (TeamRunStep, error)
	BatchCreateOrchestrationSteps(ctx context.Context, steps []OrchestrationStep) error
	ListOrchestrationSteps(ctx context.Context, teamRunID, nodeID string, limit int) ([]OrchestrationStep, error)
	CreateTaskDeadLetter(ctx context.Context, dl TaskDeadLetter) error
	ListTaskDeadLetters(ctx context.Context, filter TaskDeadLetterListFilter) ([]TaskDeadLetter, error)
	ResolveTaskDeadLetter(ctx context.Context, id string) (TaskDeadLetter, error)
}

type TeamUsecase struct {
	repo TeamRepository
}

func NewTeamUsecase(repo TeamRepository) *TeamUsecase {
	return &TeamUsecase{repo: repo}
}

func firstNonEmptyTeam(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return strings.TrimSpace(a)
	}
	return b
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
	mode := firstNonEmptyTeam(body.Mode, "sequential")
	switch mode {
	case "sequential", "parallel", "coordinator", "critic_loop", "swarm", "adaptive":
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
	for _, member := range body.Members {
		if strings.TrimSpace(member.AgentID) == "" {
			return kerrors.BadRequest("TEAM", "team member agent_id is required")
		}
		if member.Enabled == nil || *member.Enabled {
			enabledCount++
		}
		switch member.Role {
		case "synthesizer":
			hasSynthesizer = true
		case "generator":
			hasGenerator = true
		case "critic":
			hasCritic = true
		}
	}
	if enabledCount == 0 {
		return kerrors.BadRequest("TEAM", "team must have at least one enabled member")
	}
	if mode == "parallel" && !hasSynthesizer && strings.TrimSpace(body.SynthesizerAgent) == "" && enabledCount > 1 {
		return kerrors.BadRequest("TEAM", "parallel mode requires a synthesizer member or synthesizer_agent_id")
	}
	if mode == "critic_loop" && (!hasGenerator || !hasCritic) {
		return kerrors.BadRequest("TEAM", "critic_loop mode requires generator and critic members")
	}
	return nil
}

func (u *TeamUsecase) List(ctx context.Context) ([]Team, error) {
	return u.repo.ListTeams(ctx)
}

func (u *TeamUsecase) Get(ctx context.Context, id string) (Team, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Team{}, kerrors.BadRequest("TEAM", "id is required")
	}
	return u.repo.GetTeamByID(ctx, id)
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
		in.Status = "active"
	}
	if in.DefinitionJSON == "" {
		in.DefinitionJSON = defaultTeamDefinitionJSON()
	} else {
		in.DefinitionJSON = EnsureGraphRuntimeDefault(in.DefinitionJSON)
	}
	if err := validateTeamDefinition(in.DefinitionJSON); err != nil {
		return Team{}, err
	}
	return u.repo.CreateTeam(ctx, in)
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
	teamID = strings.TrimSpace(teamID)
	if teamID == "" {
		return false, kerrors.BadRequest("TEAM", "team_id is required")
	}
	runs, err := u.repo.ListTeamRuns(ctx, teamID, 50)
	if err != nil {
		return false, err
	}
	return HasActiveTeamRun(runs), nil
}

func (u *TeamUsecase) Update(ctx context.Context, id string, patch Team) (Team, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Team{}, kerrors.BadRequest("TEAM", "id is required")
	}
	active, err := u.HasActiveRun(ctx, id)
	if err != nil {
		return Team{}, err
	}
	if active {
		return Team{}, kerrors.Conflict("TEAM", "team has an active run; orchestration is read-only until the run finishes")
	}
	current, err := u.repo.GetTeamByID(ctx, id)
	if err != nil {
		return Team{}, err
	}
	current.TeamKey = strings.TrimSpace(firstNonEmptyTeam(patch.TeamKey, current.TeamKey))
	current.DisplayName = strings.TrimSpace(firstNonEmptyTeam(patch.DisplayName, current.DisplayName))
	current.Status = firstNonEmptyTeam(patch.Status, current.Status)
	current.DefinitionJSON = firstNonEmptyTeam(patch.DefinitionJSON, current.DefinitionJSON)
	current.ADKAppName = patch.ADKAppName
	if patch.ADKAppName == "" {
		current.ADKAppName = current.TeamKey
	}
	if err := validateTeamDefinition(current.DefinitionJSON); err != nil {
		return Team{}, err
	}
	return u.repo.UpdateTeam(ctx, current)
}

func (u *TeamUsecase) Delete(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return kerrors.BadRequest("TEAM", "id is required")
	}
	team, err := u.repo.GetTeamByID(ctx, id)
	if err != nil {
		return err
	}
	if team.IsDefault {
		return kerrors.Conflict("TEAM", "default team cannot be deleted")
	}
	return u.repo.DeleteTeam(ctx, id)
}

func (u *TeamUsecase) Duplicate(ctx context.Context, id string) (Team, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Team{}, kerrors.BadRequest("TEAM", "id is required")
	}
	current, err := u.repo.GetTeamByID(ctx, id)
	if err != nil {
		return Team{}, err
	}
	current.ID = newAgentCatalogID()
	suffix := newAgentCatalogID()
	if len(suffix) > 6 {
		suffix = strings.ToLower(suffix[:6])
	}
	current.TeamKey = current.TeamKey + "-copy-" + suffix
	current.DisplayName = current.DisplayName + " Copy"
	current.IsDefault = false
	return u.repo.CreateTeam(ctx, current)
}

func (u *TeamUsecase) ListRuns(ctx context.Context, teamID string, limit int) ([]TeamRun, error) {
	return u.repo.ListTeamRuns(ctx, teamID, limit)
}

func (u *TeamUsecase) GetRun(ctx context.Context, id string) (TeamRun, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return TeamRun{}, kerrors.BadRequest("TEAM", "id is required")
	}
	return u.repo.GetTeamRunByID(ctx, id)
}

func (u *TeamUsecase) UpdateRun(ctx context.Context, r TeamRun) error {
	if strings.TrimSpace(r.ID) == "" {
		return kerrors.BadRequest("TEAM", "run id is required")
	}
	return u.repo.UpdateTeamRun(ctx, r)
}

func (u *TeamUsecase) UpdateRunSummaryJSON(ctx context.Context, runID, summaryJSON string) error {
	if strings.TrimSpace(runID) == "" {
		return kerrors.BadRequest("TEAM", "run id is required")
	}
	return u.repo.UpdateTeamRunSummaryJSON(ctx, runID, summaryJSON)
}

func (u *TeamUsecase) UpdateRunTraceID(ctx context.Context, runID, traceID string) error {
	runID = strings.TrimSpace(runID)
	traceID = strings.TrimSpace(traceID)
	if runID == "" || traceID == "" {
		return nil
	}
	return u.repo.UpdateTeamRunTraceID(ctx, runID, traceID)
}

func (u *TeamUsecase) ListRunSteps(ctx context.Context, runID string) ([]TeamRunStep, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return nil, kerrors.BadRequest("TEAM", "run_id is required")
	}
	return u.repo.ListTeamRunSteps(ctx, runID)
}

func (u *TeamUsecase) ListRunObservatoryTimeline(ctx context.Context, runID, nodeID string, limit int) ([]OrchestrationStep, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return nil, kerrors.BadRequest("TEAM", "run_id is required")
	}
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	return u.repo.ListOrchestrationSteps(ctx, runID, strings.TrimSpace(nodeID), limit)
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
	t, err := u.repo.GetTeamByID(ctx, teamID)
	if err != nil {
		return false, err
	}
	def, err := parseDefinitionForUpdate(t.DefinitionJSON)
	if err != nil {
		return false, kerrors.BadRequest("TEAM", "invalid definition_json")
	}
	mode := strings.ToLower(strings.TrimSpace(def.Mode))
	if mode != "swarm" && mode != "adaptive" {
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
		filtered = append(filtered, teamMemberEntry{AgentID: aid, Role: "worker", Enabled: boolPtr(true)})
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
	_, err = u.repo.UpdateTeam(ctx, t)
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
	t, err := u.repo.GetTeamByID(ctx, teamID)
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
	case "coordinator":
		for i, m := range def.Members {
			nid := m.AgentID
			snapshot.Nodes = append(snapshot.Nodes, StructureNode{NodeID: nid, Kind: "agent", Name: m.Name})
			if i == 0 {
				snapshot.Edges = append(snapshot.Edges, StructureEdge{FromNodeID: "team-"+t.TeamKey, ToNodeID: nid})
			} else {
				snapshot.Edges = append(snapshot.Edges, StructureEdge{FromNodeID: def.Members[0].AgentID, ToNodeID: nid})
			}
		}
	case "swarm", "adaptive":
		for i, m := range def.Members {
			nid := m.AgentID
			snapshot.Nodes = append(snapshot.Nodes, StructureNode{NodeID: nid, Kind: "agent", Name: m.Name})
			if i == 0 {
				snapshot.Edges = append(snapshot.Edges, StructureEdge{FromNodeID: "team-"+t.TeamKey, ToNodeID: nid})
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
			snapshot.Edges = append(snapshot.Edges, StructureEdge{FromNodeID: "team-"+t.TeamKey, ToNodeID: nid})
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
		return &definitionForUpdate{Version: 1, Mode: "sequential"}, nil
	}
	var d definitionForUpdate
	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		return nil, err
	}
	return &d, nil
}

func (uc *TeamUsecase) ListTaskDeadLetters(ctx context.Context, filter TaskDeadLetterListFilter) ([]TaskDeadLetter, error) {
	if uc == nil || uc.repo == nil {
		return nil, nil
	}
	return uc.repo.ListTaskDeadLetters(ctx, filter)
}

func (uc *TeamUsecase) ResolveTaskDeadLetter(ctx context.Context, id string) (TaskDeadLetter, error) {
	if uc == nil || uc.repo == nil {
		return TaskDeadLetter{}, ErrNotFound
	}
	return uc.repo.ResolveTaskDeadLetter(ctx, id)
}

func boolPtr(v bool) *bool { return &v }
