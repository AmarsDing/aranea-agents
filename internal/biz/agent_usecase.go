package biz

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	stderrors "errors"
	"strings"
	"sync/atomic"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

var agentIDRand uint64

func newAgentCatalogID() string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		n := atomic.AddUint64(&agentIDRand, 1)
		return hex.EncodeToString([]byte{
			byte(n >> 56), byte(n >> 48), byte(n >> 40), byte(n >> 32),
			byte(n >> 24), byte(n >> 16), byte(n >> 8), byte(n),
		})
	}
	return hex.EncodeToString(buf)
}

// AgentRepository persists agents, runtime settings, and prompt files.
type AgentRepository interface {
	SearchAgents(ctx context.Context, q AgentListQuery) (AgentListResult, error)
	GetAgentByID(ctx context.Context, id string) (Agent, error)
	GetAgentByAgentKey(ctx context.Context, agentKey string) (Agent, error)
	CreateAgent(ctx context.Context, a Agent) (Agent, error)
	UpdateAgent(ctx context.Context, a Agent) (Agent, error)
	DeleteAgent(ctx context.Context, id string) error
	GetAgentRuntimeSettings(ctx context.Context, agentID string) (AgentRuntimeSettings, error)
	UpsertAgentRuntimeSettings(ctx context.Context, v AgentRuntimeSettings) (AgentRuntimeSettings, error)
	ListAgentPromptFiles(ctx context.Context, agentID string) ([]AgentPromptFile, error)
	ReplaceAgentPromptFiles(ctx context.Context, agentID string, files []AgentPromptFile) ([]AgentPromptFile, error)
	CreateAgentPromptFile(ctx context.Context, f AgentPromptFile) (AgentPromptFile, error)
	UpdateAgentPromptFile(ctx context.Context, f AgentPromptFile) (AgentPromptFile, error)
	DeleteAgentPromptFile(ctx context.Context, agentID, id string) error
}

// AgentUsecase is catalog agent CRUD + prompt preview.
type AgentUsecase struct {
	repo  AgentRepository
	tools ToolRepo
}

func NewAgentUsecase(repo AgentRepository, tools ToolRepo) *AgentUsecase {
	return &AgentUsecase{repo: repo, tools: tools}
}

// List returns a page of agents without per-row hydration (settings/files).
func (u *AgentUsecase) List(ctx context.Context, q AgentListQuery) (AgentListResult, error) {
	return u.repo.SearchAgents(ctx, q)
}

// Get returns one agent with settings and prompt files hydrated.
func (u *AgentUsecase) Get(ctx context.Context, id string) (Agent, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Agent{}, kerrors.BadRequest("AGENT", "id is required")
	}
	a, err := u.repo.GetAgentByID(ctx, id)
	if err != nil {
		return Agent{}, err
	}
	return u.hydrate(ctx, a)
}

func (u *AgentUsecase) hydrate(ctx context.Context, agent Agent) (Agent, error) {
	settings, err := u.repo.GetAgentRuntimeSettings(ctx, agent.ID)
	if err != nil {
		if !stderrors.Is(err, sql.ErrNoRows) {
			return Agent{}, err
		}
		settings = withSettingDefaults(settingsFromLegacyConfig(agent.ConfigJSON))
		settings.AgentID = agent.ID
		if settings, err = u.repo.UpsertAgentRuntimeSettings(ctx, settings); err != nil {
			return Agent{}, err
		}
	}
	files, err := u.repo.ListAgentPromptFiles(ctx, agent.ID)
	if err != nil {
		return Agent{}, err
	}
	if len(files) == 0 {
		files = filesFromLegacyConfig(agent.ConfigJSON)
		if len(files) == 0 {
			files = defaultPromptFiles()
		}
		for i := range files {
			files[i].AgentID = agent.ID
		}
		files, err = u.repo.ReplaceAgentPromptFiles(ctx, agent.ID, withFileDefaults(files))
		if err != nil {
			return Agent{}, err
		}
	}
	agent.Settings = &settings
	agent.Files = files
	return agent, nil
}

// Create inserts an agent and persists settings + prompt files.
func (u *AgentUsecase) Create(ctx context.Context, in Agent) (Agent, error) {
	in.AgentKey = strings.TrimSpace(in.AgentKey)
	in.DisplayName = strings.TrimSpace(in.DisplayName)
	in.Provider = strings.TrimSpace(in.Provider)
	in.Model = strings.TrimSpace(in.Model)
	if in.AgentKey == "" || in.DisplayName == "" || in.Provider == "" || in.Model == "" {
		return Agent{}, kerrors.BadRequest("AGENT", "agent_key, display_name, provider, and model are required")
	}
	if in.ID == "" {
		in.ID = newAgentCatalogID()
	}
	settings := withSettingDefaults(settingsFromAgentInput(in))
	settings.AgentID = in.ID
	files := filesFromAgentInput(in)
	for i := range files {
		files[i].AgentID = in.ID
	}
	files = withFileDefaults(files)
	if strings.TrimSpace(in.ConfigJSON) == "" {
		in.ConfigJSON = configJSONFromSettings(settings, files)
	}
	in.Status = strings.TrimSpace(in.Status)
	if in.Status == "" {
		in.Status = "active"
	}
	if _, err := u.repo.CreateAgent(ctx, in); err != nil {
		return Agent{}, err
	}
	if _, err := u.repo.UpsertAgentRuntimeSettings(ctx, settings); err != nil {
		return Agent{}, err
	}
	if _, err := u.repo.ReplaceAgentPromptFiles(ctx, in.ID, files); err != nil {
		return Agent{}, err
	}
	if _, err := u.syncConfigJSON(ctx, in.ID); err != nil {
		return Agent{}, err
	}
	return u.Get(ctx, in.ID)
}

// Update merges patch into the stored agent, then rewrites settings, files, and config_json.
func (u *AgentUsecase) Update(ctx context.Context, id string, patch Agent) (Agent, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Agent{}, kerrors.BadRequest("AGENT", "id is required")
	}
	current, err := u.Get(ctx, id)
	if err != nil {
		return Agent{}, err
	}
	merged := mergeAgentCatalog(current, patch)
	settings := withSettingDefaults(settingsFromAgentInput(merged))
	settings.AgentID = id
	files := merged.Files
	if len(files) == 0 {
		files = current.Files
	} else {
		files = withFileDefaults(files)
		for i := range files {
			files[i].AgentID = id
		}
	}
	merged.Settings = &settings
	merged.Files = files
	if _, err := u.repo.UpdateAgent(ctx, merged); err != nil {
		return Agent{}, err
	}
	if _, err := u.repo.UpsertAgentRuntimeSettings(ctx, settings); err != nil {
		return Agent{}, err
	}
	if _, err := u.repo.ReplaceAgentPromptFiles(ctx, id, files); err != nil {
		return Agent{}, err
	}
	if _, err := u.syncConfigJSON(ctx, id); err != nil {
		return Agent{}, err
	}
	return u.Get(ctx, id)
}

// Delete soft-deletes the agent.
func (u *AgentUsecase) Delete(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return kerrors.BadRequest("AGENT", "id is required")
	}
	return u.repo.DeleteAgent(ctx, id)
}

// ToggleFavorite flips the is_favorite flag on an agent.
func (u *AgentUsecase) ToggleFavorite(ctx context.Context, id string) (Agent, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Agent{}, kerrors.BadRequest("AGENT", "id is required")
	}
	a, err := u.repo.GetAgentByID(ctx, id)
	if err != nil {
		if stderrors.Is(err, sql.ErrNoRows) {
			return Agent{}, kerrors.NotFound("AGENT", "agent not found")
		}
		return Agent{}, err
	}
	a.IsFavorite = !a.IsFavorite
	updated, err := u.repo.UpdateAgent(ctx, a)
	if err != nil {
		return Agent{}, err
	}
	return u.hydrate(ctx, updated)
}

// PromptPreview returns the composed system prompt text for an agent (after hydration).
func (u *AgentUsecase) PromptPreview(ctx context.Context, id, mode string) (string, error) {
	a, err := u.Get(ctx, id)
	if err != nil {
		return "", err
	}
	mode = strings.TrimSpace(mode)
	if mode == "" {
		mode = "complete"
	}
	return composePromptPreview(a, mode), nil
}

// CreatePromptFile adds a single prompt file to an agent.
func (u *AgentUsecase) CreatePromptFile(ctx context.Context, f AgentPromptFile) (AgentPromptFile, error) {
	f.AgentID = strings.TrimSpace(f.AgentID)
	f.Name = strings.TrimSpace(f.Name)
	if f.AgentID == "" || f.Name == "" {
		return AgentPromptFile{}, kerrors.BadRequest("AGENT_FILE", "agent_id and name are required")
	}
	if _, err := u.Get(ctx, f.AgentID); err != nil {
		return AgentPromptFile{}, err
	}
	created, err := u.repo.CreateAgentPromptFile(ctx, f)
	if err != nil {
		return AgentPromptFile{}, err
	}
	if _, err := u.syncConfigJSON(ctx, f.AgentID); err != nil {
		return AgentPromptFile{}, err
	}
	return created, nil
}

// UpdatePromptFile modifies a single prompt file.
func (u *AgentUsecase) UpdatePromptFile(ctx context.Context, f AgentPromptFile) (AgentPromptFile, error) {
	f.AgentID = strings.TrimSpace(f.AgentID)
	f.ID = strings.TrimSpace(f.ID)
	if f.AgentID == "" || f.ID == "" {
		return AgentPromptFile{}, kerrors.BadRequest("AGENT_FILE", "agent_id and id are required")
	}
	if _, err := u.Get(ctx, f.AgentID); err != nil {
		return AgentPromptFile{}, err
	}
	updated, err := u.repo.UpdateAgentPromptFile(ctx, f)
	if err != nil {
		return AgentPromptFile{}, err
	}
	if _, err := u.syncConfigJSON(ctx, f.AgentID); err != nil {
		return AgentPromptFile{}, err
	}
	return updated, nil
}

// DeletePromptFile removes a single prompt file.
func (u *AgentUsecase) DeletePromptFile(ctx context.Context, agentID, id string) error {
	agentID = strings.TrimSpace(agentID)
	id = strings.TrimSpace(id)
	if agentID == "" || id == "" {
		return kerrors.BadRequest("AGENT_FILE", "agent_id and id are required")
	}
	if _, err := u.Get(ctx, agentID); err != nil {
		return err
	}
	if err := u.repo.DeleteAgentPromptFile(ctx, agentID, id); err != nil {
		return err
	}
	_, err := u.syncConfigJSON(ctx, agentID)
	return err
}

// EstimateTokens returns an approximate token count for all prompt files of an agent.
func (u *AgentUsecase) EstimateTokens(ctx context.Context, agentID string) (FileTokenEstimates, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return FileTokenEstimates{}, kerrors.BadRequest("AGENT_FILE", "agent_id is required")
	}
	a, err := u.Get(ctx, agentID)
	if err != nil {
		return FileTokenEstimates{}, err
	}
	return estimateTokensForFiles(a.Files), nil
}

func (u *AgentUsecase) syncConfigJSON(ctx context.Context, id string) (Agent, error) {
	a, err := u.repo.GetAgentByID(ctx, id)
	if err != nil {
		return Agent{}, err
	}
	settings, err := u.repo.GetAgentRuntimeSettings(ctx, id)
	if err != nil {
		if !stderrors.Is(err, sql.ErrNoRows) {
			return Agent{}, err
		}
		settings = withSettingDefaults(settingsFromLegacyConfig(a.ConfigJSON))
		settings.AgentID = id
	}
	files, err := u.repo.ListAgentPromptFiles(ctx, id)
	if err != nil {
		return Agent{}, err
	}
	a.ConfigJSON = configJSONFromSettings(withSettingDefaults(settings), files)
	return u.repo.UpdateAgent(ctx, a)
}

func mergeAgentCatalog(current, patch Agent) Agent {
	out := current
	if strings.TrimSpace(patch.AgentKey) != "" {
		out.AgentKey = strings.TrimSpace(patch.AgentKey)
	}
	if strings.TrimSpace(patch.DisplayName) != "" {
		out.DisplayName = strings.TrimSpace(patch.DisplayName)
	}
	if strings.TrimSpace(patch.Provider) != "" {
		out.Provider = strings.TrimSpace(patch.Provider)
	}
	if strings.TrimSpace(patch.Model) != "" {
		out.Model = strings.TrimSpace(patch.Model)
	}
	if strings.TrimSpace(patch.Status) != "" {
		out.Status = strings.TrimSpace(patch.Status)
	}
	out.IsDefault = patch.IsDefault
	out.IsFavorite = patch.IsFavorite
	out.Icon = patch.Icon
	out.AgentDescription = patch.AgentDescription
	out.CategoryPositionID = patch.CategoryPositionID
	out.SystemPromptMode = patch.SystemPromptMode
	out.ContextWindow = patch.ContextWindow
	out.BudgetMonthlyCents = patch.BudgetMonthlyCents
	if strings.TrimSpace(patch.ConfigJSON) != "" {
		out.ConfigJSON = patch.ConfigJSON
	}
	if patch.Settings != nil {
		out.Settings = patch.Settings
	}
	if len(patch.Files) > 0 {
		out.Files = patch.Files
	}
	return out
}
