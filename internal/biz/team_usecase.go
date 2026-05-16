package biz

import (
	"context"
	"encoding/json"
	"strings"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

// TeamRepository persists teams and reads team run telemetry.
type TeamRepository interface {
	ListTeams(ctx context.Context) ([]Team, error)
	GetTeamByID(ctx context.Context, id string) (Team, error)
	CreateTeam(ctx context.Context, t Team) (Team, error)
	UpdateTeam(ctx context.Context, t Team) (Team, error)
	DeleteTeam(ctx context.Context, id string) error
	ListTeamRuns(ctx context.Context, teamID string, limit int) ([]TeamRun, error)
	ListTeamRunSteps(ctx context.Context, runID string) ([]TeamRunStep, error)
	CreateTeamRun(ctx context.Context, r TeamRun) (TeamRun, error)
	UpdateTeamRun(ctx context.Context, r TeamRun) error
	CreateTeamRunStep(ctx context.Context, s TeamRunStep) (TeamRunStep, error)
}

// TeamUsecase implements team catalog + run listing (writes to runs still happen in legacy chat stack).
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
	// Align with server.http.timeout in configs (long coordinator / tool runs); user may set 0 to skip runner deadline.
	return `{"version":1,"mode":"sequential","members":[],"max_concurrency":2,"timeout_seconds":600}`
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
	case "sequential", "parallel", "coordinator", "critic_loop", "adaptive":
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

// List returns non-deleted teams (default first, then newest).
func (u *TeamUsecase) List(ctx context.Context) ([]Team, error) {
	return u.repo.ListTeams(ctx)
}

// Get returns one team.
func (u *TeamUsecase) Get(ctx context.Context, id string) (Team, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Team{}, kerrors.BadRequest("TEAM", "id is required")
	}
	return u.repo.GetTeamByID(ctx, id)
}

// Create validates and inserts a team.
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
	}
	if err := validateTeamDefinition(in.DefinitionJSON); err != nil {
		return Team{}, err
	}
	return u.repo.CreateTeam(ctx, in)
}

// Update merges changes into an existing team.
func (u *TeamUsecase) Update(ctx context.Context, id string, patch Team) (Team, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Team{}, kerrors.BadRequest("TEAM", "id is required")
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

// Delete soft-deletes a non-default team.
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

// Duplicate clones a team with a new id and key.
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

// ListRuns returns recent team runs, optionally filtered by team_id.
func (u *TeamUsecase) ListRuns(ctx context.Context, teamID string, limit int) ([]TeamRun, error) {
	return u.repo.ListTeamRuns(ctx, teamID, limit)
}

// ListRunSteps returns steps for one run.
func (u *TeamUsecase) ListRunSteps(ctx context.Context, runID string) ([]TeamRunStep, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return nil, kerrors.BadRequest("TEAM", "run_id is required")
	}
	return u.repo.ListTeamRunSteps(ctx, runID)
}
