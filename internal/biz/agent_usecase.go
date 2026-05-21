package biz

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	stderrors "errors"
	"regexp"
	"strings"
	"sync/atomic"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

var (
	agentIDRand     uint64
	agentKeyPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
)

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
	ListExtrasForAgents(ctx context.Context, agentIDs []string) (map[string]AgentListExtras, error)
	ListAgentCreators(ctx context.Context) ([]AgentCreator, error)
}

// AgentUsecase is catalog agent CRUD + prompt preview.
type AgentUsecase struct {
	repo  AgentRepository
	tools ToolRepo
}

func NewAgentUsecase(repo AgentRepository, tools ToolRepo) *AgentUsecase {
	return &AgentUsecase{repo: repo, tools: tools}
}

// ListAgentCreators returns distinct creators for list filter options.
func (u *AgentUsecase) ListAgentCreators(ctx context.Context) ([]AgentCreator, error) {
	if u == nil || u.repo == nil {
		return nil, nil
	}
	return u.repo.ListAgentCreators(ctx)
}

// List returns a page of agents without per-row hydration (settings/files).
func (u *AgentUsecase) List(ctx context.Context, q AgentListQuery) (AgentListResult, error) {
	page, err := u.repo.SearchAgents(ctx, q)
	if err != nil {
		return AgentListResult{}, err
	}
	if len(page.Items) == 0 {
		return page, nil
	}
	ids := make([]string, 0, len(page.Items))
	for i := range page.Items {
		ids = append(ids, page.Items[i].ID)
	}
	extras, err := u.repo.ListExtrasForAgents(ctx, ids)
	if err != nil {
		return AgentListResult{}, err
	}
	for i := range page.Items {
		if ex, ok := extras[page.Items[i].ID]; ok {
			page.Items[i].LastRunStatus = ex.LastRunStatus
			page.Items[i].LastRunAt = ex.LastRunAt
			page.Items[i].PendingEvolutionCount = ex.PendingEvolutionCount
		}
	}
	return page, nil
}

// CheckAgentKeyAvailability reports whether agent_key is free for a new catalog agent.
func (u *AgentUsecase) CheckAgentKeyAvailability(ctx context.Context, agentKey string) (available bool, message string, err error) {
	agentKey = strings.TrimSpace(agentKey)
	if agentKey == "" {
		return false, "agent_key is required", kerrors.BadRequest("AGENT", "agent_key is required")
	}
	if !agentKeyPattern.MatchString(agentKey) {
		return false, "invalid agent_key format", kerrors.BadRequest("AGENT_KEY_INVALID", "agent_key must be lowercase letters, digits, and hyphens")
	}
	_, err = u.repo.GetAgentByAgentKey(ctx, agentKey)
	if err == nil {
		return false, "agent_key already in use", nil
	}
	if !stderrors.Is(err, sql.ErrNoRows) {
		return false, "", err
	}
	return true, "available", nil
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
	HydrateAgentKind(&agent)
	if extras, err := u.repo.ListExtrasForAgents(ctx, []string{agent.ID}); err == nil {
		if ex, ok := extras[agent.ID]; ok {
			agent.LastRunStatus = ex.LastRunStatus
			agent.LastRunAt = ex.LastRunAt
			agent.PendingEvolutionCount = ex.PendingEvolutionCount
		}
	}
	return agent, nil
}

// Create inserts an agent and persists settings + prompt files.
func (u *AgentUsecase) Create(ctx context.Context, in Agent) (Agent, error) {
	in.AgentKey = strings.TrimSpace(in.AgentKey)
	in.DisplayName = strings.TrimSpace(in.DisplayName)
	in.Provider = strings.TrimSpace(in.Provider)
	in.Model = strings.TrimSpace(in.Model)
	in.Kind = NormalizeAgentKind(in.Kind)
	HydrateAgentKind(&in)

	if in.AgentKey == "" || in.DisplayName == "" {
		return Agent{}, kerrors.BadRequest("AGENT", "agent_key and display_name are required")
	}
	switch in.Kind {
	case AgentKindA2AProxy:
		if in.A2AProxy == nil || strings.TrimSpace(in.A2AProxy.RemoteURL) == "" {
			return Agent{}, kerrors.BadRequest("AGENT", "a2a_proxy remote_url is required")
		}
		if in.Provider == "" {
			in.Provider = "a2a"
		}
		if in.Model == "" {
			in.Model = "proxy"
		}
	default:
		if in.Provider == "" || in.Model == "" {
			return Agent{}, kerrors.BadRequest("AGENT", "provider and model are required")
		}
		in.Kind = AgentKindLLM
	}
	if in.ID == "" {
		in.ID = newAgentCatalogID()
	}
	settings := withSettingDefaults(settingsFromAgentInput(in))
	settings.AgentID = in.ID
	if err := ValidateCodeExecutorType(settings.CodeExecutorType); err != nil {
		return Agent{}, err
	}
	if err := ValidatePlannerKind(settings.PlannerKind); err != nil {
		return Agent{}, err
	}
	if err := ValidatePlannerConfigJSON(settings.PlannerKind, settings.PlannerConfigJSON); err != nil {
		return Agent{}, err
	}
	if err := ValidateRalphLoopSettings(&settings); err != nil {
		return Agent{}, err
	}
	if in.Kind == AgentKindA2AProxy {
		settings.IntentPassEnabled = false
		settings.ToolsEnabled = false
		settings.MemoryEnabled = false
		settings.SelfEvolve = false
	}
	files := filesFromAgentInput(in)
	for i := range files {
		files[i].AgentID = in.ID
	}
	files = withFileDefaults(files)
	if strings.TrimSpace(in.ConfigJSON) == "" {
		in.ConfigJSON = configJSONFromSettings(settings, files)
	}
	in.ConfigJSON = EmbedAgentKindInConfigJSON(in.ConfigJSON, in.Kind, in.A2AProxy)
	in.Status = strings.TrimSpace(in.Status)
	if in.Status == "" {
		in.Status = "active"
	}
	if strings.TrimSpace(in.CreatedBy) == "" {
		in.CreatedBy = AgentCreatedByFromContext(ctx)
	}
	if _, err := u.repo.CreateAgent(ctx, in); err != nil {
		if isAgentKeyDuplicate(err) {
			return Agent{}, kerrors.BadRequest("AGENT_KEY_CONFLICT", "agent_key already in use")
		}
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
	HydrateAgentKind(&patch)
	HydrateAgentKind(&current)
	if strings.TrimSpace(patch.Kind) != "" && NormalizeAgentKind(patch.Kind) != NormalizeAgentKind(current.Kind) {
		return Agent{}, kerrors.BadRequest("AGENT", "agent_kind is immutable")
	}
	merged := mergeAgentCatalog(current, patch)
	merged.Kind = current.Kind
	settings := withSettingDefaults(settingsFromAgentInput(merged))
	settings.AgentID = id
	if err := ValidateCodeExecutorType(settings.CodeExecutorType); err != nil {
		return Agent{}, err
	}
	if err := ValidatePlannerKind(settings.PlannerKind); err != nil {
		return Agent{}, err
	}
	if err := ValidatePlannerConfigJSON(settings.PlannerKind, settings.PlannerConfigJSON); err != nil {
		return Agent{}, err
	}
	if err := ValidateRalphLoopSettings(&settings); err != nil {
		return Agent{}, err
	}
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
	HydrateAgentKind(&merged)
	if IsA2AProxyAgent(merged) {
		if merged.A2AProxy == nil || strings.TrimSpace(merged.A2AProxy.RemoteURL) == "" {
			return Agent{}, kerrors.BadRequest("AGENT", "a2a_proxy remote_url is required")
		}
	}
	merged.ConfigJSON = EmbedAgentKindInConfigJSON(merged.ConfigJSON, merged.Kind, merged.A2AProxy)
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
	HydrateAgentKind(&a)
	a.ConfigJSON = EmbedAgentKindInConfigJSON(a.ConfigJSON, a.Kind, a.A2AProxy)
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
		out.ConfigJSON = MergeAgentConfigJSON(current.ConfigJSON, patch.ConfigJSON)
	}
	if patch.Settings != nil {
		out.Settings = patch.Settings
	}
	if len(patch.Files) > 0 {
		out.Files = patch.Files
	}
	if patch.A2AProxy != nil && IsA2AProxyAgent(out) {
		out.A2AProxy = patch.A2AProxy
	}
	return out
}

// AgentBatchUpdateInput is LIST-04 bulk enable/disable/delete.
type AgentBatchUpdateInput struct {
	IDs     []string
	Status  string // optional: active | inactive
	Delete  bool
}

// BatchUpdateAgents applies status changes or deletes for many agents.
func (u *AgentUsecase) BatchUpdateAgents(ctx context.Context, in AgentBatchUpdateInput) (int, error) {
	if u == nil || u.repo == nil {
		return 0, kerrors.InternalServer("AGENT", "agent repository not configured")
	}
	n := 0
	for _, id := range in.IDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if in.Delete {
			if err := u.repo.DeleteAgent(ctx, id); err != nil {
				return n, err
			}
			n++
			continue
		}
		if st := strings.TrimSpace(in.Status); st != "" {
			a, err := u.repo.GetAgentByID(ctx, id)
			if err != nil {
				return n, err
			}
			a.Status = st
			if _, err := u.repo.UpdateAgent(ctx, a); err != nil {
				return n, err
			}
			n++
		}
	}
	return n, nil
}
