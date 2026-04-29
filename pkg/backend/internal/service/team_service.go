package service

import (
	"encoding/json"
	"strings"

	"arenea/backend/internal/domain"
	"arenea/backend/internal/repository"
)

type TeamService struct {
	repo repository.Store
}

func NewTeamService(repo repository.Store) *TeamService {
	return &TeamService{repo: repo}
}

func (s *TeamService) List() ([]domain.Team, error) {
	return s.repo.ListTeams()
}

func (s *TeamService) Get(id string) (domain.Team, error) {
	return s.repo.GetTeamByID(id)
}

func (s *TeamService) Create(in domain.Team) (domain.Team, error) {
	in.TeamKey = strings.TrimSpace(in.TeamKey)
	in.DisplayName = strings.TrimSpace(in.DisplayName)
	if in.TeamKey == "" || in.DisplayName == "" {
		return domain.Team{}, validationError("team_key and display_name are required")
	}
	if in.ID == "" {
		in.ID = newID()
	}
	if in.Status == "" {
		in.Status = "active"
	}
	if in.DefinitionJSON == "" {
		in.DefinitionJSON = defaultTeamDefinitionJSON()
	}
	if err := validateTeamDefinition(in.DefinitionJSON); err != nil {
		return domain.Team{}, err
	}
	return s.repo.CreateTeam(in)
}

func (s *TeamService) Update(id string, in domain.Team) (domain.Team, error) {
	current, err := s.repo.GetTeamByID(id)
	if err != nil {
		return domain.Team{}, err
	}
	current.TeamKey = strings.TrimSpace(firstNonEmptyString(in.TeamKey, current.TeamKey))
	current.DisplayName = strings.TrimSpace(firstNonEmptyString(in.DisplayName, current.DisplayName))
	current.Status = firstNonEmptyString(in.Status, current.Status)
	current.DefinitionJSON = firstNonEmptyString(in.DefinitionJSON, current.DefinitionJSON)
	current.ADKAppName = in.ADKAppName
	if in.ADKAppName == "" {
		current.ADKAppName = current.TeamKey
	}
	if err := validateTeamDefinition(current.DefinitionJSON); err != nil {
		return domain.Team{}, err
	}
	return s.repo.UpdateTeam(current)
}

func (s *TeamService) Delete(id string) error {
	team, err := s.repo.GetTeamByID(id)
	if err != nil {
		return err
	}
	if team.IsDefault {
		return conflictError("default team cannot be deleted")
	}
	return s.repo.DeleteTeam(id)
}

func (s *TeamService) Duplicate(id string) (domain.Team, error) {
	current, err := s.repo.GetTeamByID(id)
	if err != nil {
		return domain.Team{}, err
	}
	current.ID = newID()
	current.TeamKey = current.TeamKey + "-copy-" + strings.ToLower(newID()[:6])
	current.DisplayName = current.DisplayName + " Copy"
	current.IsDefault = false
	return s.repo.CreateTeam(current)
}

func (s *TeamService) ListRuns(teamID string, limit int) ([]domain.TeamRun, error) {
	return s.repo.ListTeamRuns(teamID, limit)
}

func (s *TeamService) ListRunSteps(runID string) ([]domain.TeamRunStep, error) {
	return s.repo.ListTeamRunSteps(runID)
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
		return validationError("definition_json must be valid JSON")
	}
	mode := firstNonEmptyString(body.Mode, "sequential")
	switch mode {
	case "sequential", "parallel", "coordinator", "critic_loop", "adaptive":
	default:
		return validationError("unsupported team orchestration mode")
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
			return validationError("team member agent_id is required")
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
		return validationError("team must have at least one enabled member")
	}
	if mode == "parallel" && !hasSynthesizer && strings.TrimSpace(body.SynthesizerAgent) == "" && enabledCount > 1 {
		return validationError("parallel mode requires a synthesizer member or synthesizer_agent_id")
	}
	if mode == "critic_loop" && (!hasGenerator || !hasCritic) {
		return validationError("critic_loop mode requires generator and critic members")
	}
	return nil
}

func defaultTeamDefinitionJSON() string {
	return `{"version":1,"mode":"sequential","members":[],"max_concurrency":2,"timeout_seconds":180}`
}
