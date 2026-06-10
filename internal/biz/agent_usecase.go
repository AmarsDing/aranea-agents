package biz

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	stderrors "errors"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"aranea-agents/internal/biz/shared"
	"aranea-agents/pkg/loggateway"

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

type AgentReader interface {
	SearchAgents(ctx context.Context, q AgentListQuery) (AgentListResult, error)
	GetAgentByID(ctx context.Context, id string) (Agent, error)
	GetAgentByAgentKey(ctx context.Context, agentKey string) (Agent, error)
	ListExtrasForAgents(ctx context.Context, agentIDs []string) (map[string]AgentListExtras, error)
}

type AgentWriter interface {
	CreateAgent(ctx context.Context, a Agent) (Agent, error)
	UpdateAgent(ctx context.Context, a Agent) (Agent, error)
	DeleteAgent(ctx context.Context, id string) error
}

type AgentRuntimeSettingsRepo interface {
	GetAgentRuntimeSettings(ctx context.Context, agentID string) (AgentRuntimeSettings, error)
	UpsertAgentRuntimeSettings(ctx context.Context, v AgentRuntimeSettings) (AgentRuntimeSettings, error)
}

type AgentPromptFileRepo interface {
	ListAgentPromptFiles(ctx context.Context, agentID string) ([]AgentPromptFile, error)
	ReplaceAgentPromptFiles(ctx context.Context, agentID string, files []AgentPromptFile) ([]AgentPromptFile, error)
	CreateAgentPromptFile(ctx context.Context, f AgentPromptFile) (AgentPromptFile, error)
	UpdateAgentPromptFile(ctx context.Context, f AgentPromptFile) (AgentPromptFile, error)
	DeleteAgentPromptFile(ctx context.Context, agentID, id string) error
}

// AgentAtomicWriter 提供跨方法的事务化写入，保证 "agent + prompt files +
// runtime settings" 三步原子化。Pack 导入场景必须使用这两个方法以避免
// partial failure 导致数据半新半旧；其他 Usecase 可继续走单步 API。
type AgentAtomicWriter interface {
	CreateAgentAtomic(ctx context.Context, a Agent, files []AgentPromptFile, settings AgentRuntimeSettings) (Agent, error)
	UpdateAgentAtomic(ctx context.Context, a Agent, files []AgentPromptFile, settings *AgentRuntimeSettings) (Agent, error)
}

// TODO(debt): AgentRepository has 14+ methods and should be split into narrow sub-interfaces
// per consumer need (e.g., AgentPositionClearer, AgentCreatorLister, AgentReorderRepo).
// Current consumers should depend on the minimal sub-interface they need.
type AgentRepository interface {
	AgentReader
	AgentWriter
	AgentAtomicWriter
	AgentRuntimeSettingsRepo
	AgentPromptFileRepo
	AgentReferenceChecker
	ListAgentCreators(ctx context.Context) ([]AgentCreator, error)
	ReorderAgents(ctx context.Context, ids []string) error
	ExecInTx(ctx context.Context, fn func(ctx context.Context) error) error
	ClearPositionByDepartment(ctx context.Context, deptID string) (int, error)
}

// ProviderModelPairValidator validates that a provider+model pair exists in the catalog.
type ProviderModelPairValidator interface {
	ValidatePair(ctx context.Context, provider, model string) (bool, string, error)
}

// AgentUsecase is catalog agent CRUD + prompt preview.
type AgentUsecase struct {
	repo               AgentRepository
	tools              ToolRegistryReader
	sys                SystemSettingRepo
	webResearchChecker WebResearchReadinessChecker
	providerValidator  ProviderModelPairValidator
	lg                 loggateway.Logger
}

func NewAgentUsecase(repo AgentRepository, tools ToolRegistryReader, sys SystemSettingRepo, checker WebResearchReadinessChecker, validator ProviderModelPairValidator, lg loggateway.Logger) *AgentUsecase {
	return &AgentUsecase{repo: repo, tools: tools, sys: sys, webResearchChecker: checker, providerValidator: validator, lg: lg}
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
	if !stderrors.Is(err, shared.ErrNotFound) && !stderrors.Is(err, sql.ErrNoRows) {
		return false, "", err
	}
	return true, "available", nil
}

// Get returns one agent with settings and prompt files hydrated.
func (u *AgentUsecase) Get(ctx context.Context, id string) (Agent, error) {
	id, err := requireNonEmpty(id, "AGENT", "id")
	if err != nil {
		return Agent{}, err
	}
	a, err := u.repo.GetAgentByID(ctx, id)
	if err != nil {
		return Agent{}, err
	}
	return u.hydrate(ctx, a)
}

func (u *AgentUsecase) GetByAgentKey(ctx context.Context, agentKey string) (Agent, error) {
	agentKey, err := requireNonEmpty(agentKey, "AGENT", "agent_key")
	if err != nil {
		return Agent{}, err
	}
	a, err := u.repo.GetAgentByAgentKey(ctx, agentKey)
	if err != nil {
		return Agent{}, err
	}
	return u.hydrate(ctx, a)
}

func (u *AgentUsecase) GetAgentRuntimeSettings(ctx context.Context, agentID string) (AgentRuntimeSettings, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return AgentRuntimeSettings{}, kerrors.BadRequest("AGENT", "agent id is required")
	}
	settings, err := u.repo.GetAgentRuntimeSettings(ctx, agentID)
	if err != nil {
		if stderrors.Is(err, shared.ErrNotFound) || stderrors.Is(err, sql.ErrNoRows) {
			return DefaultAgentRuntimeSettings(), nil
		}
		return AgentRuntimeSettings{}, err
	}
	return settings, nil
}

func (u *AgentUsecase) hydrate(ctx context.Context, agent Agent) (Agent, error) {
	settings, err := u.repo.GetAgentRuntimeSettings(ctx, agent.ID)
	if err != nil {
		if !stderrors.Is(err, shared.ErrNotFound) && !stderrors.Is(err, sql.ErrNoRows) {
			return Agent{}, err
		}
		u.lg.Warn("agent runtime settings not found, migrating from legacy config_json", loggateway.StepID("agent.db_resolve"), loggateway.Str("agent_id", agent.ID))
		settings = u.migrateLegacySettings(ctx, agent)
	}
	files, err := u.repo.ListAgentPromptFiles(ctx, agent.ID)
	if err != nil {
		return Agent{}, err
	}
	if len(files) == 0 {
		u.lg.Warn("agent prompt files not found, migrating from legacy config_json", loggateway.StepID("agent.skill_build"), loggateway.Str("agent_id", agent.ID))
		files = u.migrateLegacyFiles(ctx, agent)
	}
	agent.Settings = &settings
	agent.Files = files
	HydrateAgentKind(&agent)
	computed, err := configJSONFromSettings(withSettingDefaults(settings), files)
	if err != nil {
		return Agent{}, err
	}
	computed = EmbedAgentKindInConfigJSON(computed, agent.AgentKind, agent.A2AProxy, u.lg)
	agent.ConfigJSON = mergeEvaluationFromLegacy(computed, agent.ConfigJSON, u.lg)
	if extras, err := u.repo.ListExtrasForAgents(ctx, []string{agent.ID}); err == nil {
		if ex, ok := extras[agent.ID]; ok {
			agent.LastRunStatus = ex.LastRunStatus
			agent.LastRunAt = ex.LastRunAt
			agent.PendingEvolutionCount = ex.PendingEvolutionCount
		}
	}
	return agent, nil
}

func (u *AgentUsecase) migrateLegacySettings(ctx context.Context, agent Agent) AgentRuntimeSettings {
	settings := withSettingDefaults(settingsFromLegacyConfig(agent.ConfigJSON))
	settings.AgentID = agent.ID
	migrated, err := u.repo.UpsertAgentRuntimeSettings(ctx, settings)
	if err != nil {
		return settings
	}
	return migrated
}

func (u *AgentUsecase) migrateLegacyFiles(ctx context.Context, agent Agent) []AgentPromptFile {
	files := filesFromLegacyConfig(agent.ConfigJSON)
	if len(files) == 0 {
		files = defaultPromptFiles()
	}
	for i := range files {
		files[i].AgentID = agent.ID
	}
	migrated, err := u.repo.ReplaceAgentPromptFiles(ctx, agent.ID, withFileDefaults(files))
	if err != nil {
		return withFileDefaults(files)
	}
	return migrated
}

// Create inserts an agent and persists settings + prompt files.
func (u *AgentUsecase) Create(ctx context.Context, in Agent) (Agent, error) {
	in.AgentKey = strings.TrimSpace(in.AgentKey)
	in.DisplayName = strings.TrimSpace(in.DisplayName)
	in.Provider = strings.TrimSpace(in.Provider)
	in.Model = strings.TrimSpace(in.Model)
	in.AgentKind = NormalizeAgentKind(in.AgentKind)
	HydrateAgentKind(&in)

	if in.AgentKey == "" || in.DisplayName == "" {
		return Agent{}, kerrors.BadRequest("AGENT", "agent_key and display_name are required")
	}
	switch in.AgentKind {
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
		in.AgentKind = AgentKindLLM
	}
	// Validate that the provider+model pair exists in the catalog (non-A2A agents only).
	if u.providerValidator != nil && in.AgentKind != AgentKindA2AProxy {
		ok, msg, valErr := u.providerValidator.ValidatePair(ctx, in.Provider, in.Model)
		if valErr != nil {
			u.lg.Warn("provider model validation failed, proceeding with create",
				loggateway.StepID("agent.create.provider_validate"),
				loggateway.Str("provider", in.Provider),
				loggateway.Str("model", in.Model),
				loggateway.Err(valErr))
		} else if !ok {
			return Agent{}, kerrors.BadRequest("AGENT", "provider/model is not enabled: "+msg)
		}
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
	if in.AgentKind == AgentKindA2AProxy {
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
	// TODO(debt): DEV-10 — ConfigJSON should be a read-only projection of Settings + Files.
	// Currently it participates in the write path, creating a risk of data inconsistency.
	// Plan: After all consumers are migrated to read from Settings/Files directly,
	// remove ConfigJSON from the write path and generate it on-demand for read-only use.
	if strings.TrimSpace(in.ConfigJSON) == "" {
		configJSON, configErr := configJSONFromSettings(settings, files)
		if configErr != nil {
			return Agent{}, configErr
		}
		in.ConfigJSON = configJSON
	}
	in.ConfigJSON = EmbedAgentKindInConfigJSON(in.ConfigJSON, in.AgentKind, in.A2AProxy, u.lg)
	in.Status = strings.TrimSpace(in.Status)
	if in.Status == "" {
		in.Status = "active"
	}
	if strings.TrimSpace(in.CreatedBy) == "" {
		in.CreatedBy = AgentCreatedByFromContext(ctx)
	}
	if err := u.repo.ExecInTx(ctx, func(txCtx context.Context) error {
		if _, err := u.repo.CreateAgent(txCtx, in); err != nil {
			if isAgentKeyDuplicate(err) {
				return kerrors.BadRequest("AGENT_KEY_CONFLICT", "agent_key already in use")
			}
			return err
		}
		if _, err := u.repo.UpsertAgentRuntimeSettings(txCtx, settings); err != nil {
			return err
		}
		if _, err := u.repo.ReplaceAgentPromptFiles(txCtx, in.ID, files); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return Agent{}, err
	}
	readCtx := ctx
	if ctx.Err() != nil {
		readCtx = context.Background()
	}
	return u.Get(readCtx, in.ID)
}

// Update merges patch into the stored agent, then rewrites settings, files, and config_json.
func (u *AgentUsecase) Update(ctx context.Context, id string, patch Agent) (Agent, error) {
	// #region debug-point agent.update.trace
	// DEBUG ONLY: timing trace to identify the SQL call that hangs in AgentUsecase.Update.
	// Remove this block once root-cause is fixed.
	// NOTE: uses Info (not Debug) because smoke.yaml logging.level=info filters Debug.
	traceStart := time.Now()
	traceTrace := u.lg.With(loggateway.StepID("agent.update.trace"), loggateway.Str("agent_id", id))
	traceTrace.Info("enter Update", loggateway.Duration(time.Since(traceStart).Milliseconds()))
	defer func() {
		traceTrace.Info("exit Update", loggateway.Duration(time.Since(traceStart).Milliseconds()))
	}()
	// #endregion debug-point
	id, err := requireNonEmpty(id, "AGENT", "id")
	if err != nil {
		return Agent{}, err
	}
	current, err := u.Get(ctx, id)
	// #region debug-point agent.update.trace
	traceTrace.Info("after Get#1 (hydrate)", loggateway.Duration(time.Since(traceStart).Milliseconds()))
	// #endregion debug-point
	if err != nil {
		return Agent{}, err
	}
	HydrateAgentKind(&patch)
	HydrateAgentKind(&current)
	if strings.TrimSpace(patch.AgentKey) != "" && strings.TrimSpace(patch.AgentKey) != strings.TrimSpace(current.AgentKey) {
		return Agent{}, kerrors.BadRequest("AGENT", "agent_key is immutable")
	}
	if strings.TrimSpace(patch.AgentKind) != "" && NormalizeAgentKind(patch.AgentKind) != NormalizeAgentKind(current.AgentKind) {
		return Agent{}, kerrors.BadRequest("AGENT", "agent_kind is immutable")
	}
	merged := mergeAgentCatalog(current, patch)
	merged.AgentKey = current.AgentKey
	merged.AgentKind = current.AgentKind
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
	merged.ConfigJSON = EmbedAgentKindInConfigJSON(merged.ConfigJSON, merged.AgentKind, merged.A2AProxy, u.lg)
	// #region debug-point agent.update.trace
	traceTrace.Info("before ExecInTx", loggateway.Duration(time.Since(traceStart).Milliseconds()), loggateway.Int("files_len", len(files)))
	// #endregion debug-point
	if err := u.repo.ExecInTx(ctx, func(txCtx context.Context) error {
		// #region debug-point agent.update.trace
		traceTrace.Info("tx-body: start", loggateway.Duration(time.Since(traceStart).Milliseconds()))
		// #endregion debug-point
		// #region debug-point agent.update.trace
		s1 := time.Now()
		traceTrace.Info("tx-body: before UpdateAgent", loggateway.Duration(time.Since(traceStart).Milliseconds()))
		// #endregion debug-point
		if _, err := u.repo.UpdateAgent(txCtx, merged); err != nil {
			return err
		}
		// #region debug-point agent.update.trace
		traceTrace.Info("tx-body: after UpdateAgent", loggateway.Duration(time.Since(traceStart).Milliseconds()), loggateway.Duration(time.Since(s1).Milliseconds()))
		// #endregion debug-point
		// #region debug-point agent.update.trace
		s2 := time.Now()
		traceTrace.Info("tx-body: before UpsertAgentRuntimeSettings", loggateway.Duration(time.Since(traceStart).Milliseconds()))
		// #endregion debug-point
		if _, err := u.repo.UpsertAgentRuntimeSettings(txCtx, settings); err != nil {
			return err
		}
		// #region debug-point agent.update.trace
		traceTrace.Info("tx-body: after UpsertAgentRuntimeSettings", loggateway.Duration(time.Since(traceStart).Milliseconds()), loggateway.Duration(time.Since(s2).Milliseconds()))
		// #endregion debug-point
		// #region debug-point agent.update.trace
		s3 := time.Now()
		traceTrace.Info("tx-body: before ReplaceAgentPromptFiles", loggateway.Duration(time.Since(traceStart).Milliseconds()))
		// #endregion debug-point
		if _, err := u.repo.ReplaceAgentPromptFiles(txCtx, id, files); err != nil {
			return err
		}
		// #region debug-point agent.update.trace
		traceTrace.Info("tx-body: after ReplaceAgentPromptFiles", loggateway.Duration(time.Since(traceStart).Milliseconds()), loggateway.Duration(time.Since(s3).Milliseconds()))
		// #endregion debug-point
		return nil
	}); err != nil {
		return Agent{}, err
	}
	// #region debug-point agent.update.trace
	traceTrace.Info("after ExecInTx", loggateway.Duration(time.Since(traceStart).Milliseconds()))
	// #endregion debug-point
	// Use a detached context for the final read: the transaction has already
	// committed, but the HTTP request context may have been cancelled while
	// waiting for a SQLite write lock.  The caller still needs the updated
	// agent data to return to the frontend.
	readCtx := ctx
	if ctx.Err() != nil {
		readCtx = context.Background()
	}
	// #region debug-point agent.update.trace
	s4 := time.Now()
	traceTrace.Info("before Get#2 (final hydrate)", loggateway.Duration(time.Since(traceStart).Milliseconds()))
	// #endregion debug-point
	out, err := u.Get(readCtx, id)
	// #region debug-point agent.update.trace
	traceTrace.Info("after Get#2 (final hydrate)", loggateway.Duration(time.Since(traceStart).Milliseconds()), loggateway.Duration(time.Since(s4).Milliseconds()))
	// #endregion debug-point
	return out, err
}

// Delete soft-deletes the agent.
func (u *AgentUsecase) Delete(ctx context.Context, id string) error {
	id, err := requireNonEmpty(id, "AGENT", "id")
	if err != nil {
		return err
	}
	current, err := u.Get(ctx, id)
	if err != nil {
		return err
	}
	// Kind is the ownership classification (user | system_builtin | ecosystem_preset | ...).
	if current.Kind == "system_builtin" {
		if strings.HasPrefix(current.AgentKey, "__dept_lead_") {
			return kerrors.Forbidden("AGENT", "cannot delete department lead agent; delete the department instead")
		}
		return kerrors.Forbidden("AGENT", "cannot delete system_builtin agent")
	}
	if current.Readonly {
		return kerrors.Forbidden("AGENT", "cannot delete a readonly agent")
	}
	return u.repo.DeleteAgent(ctx, id)
}

// ForceDelete soft-deletes the agent bypassing kind/readonly permission checks.
// Only for internal system operations (e.g., cleaning up dept lead agents).
func (u *AgentUsecase) ForceDelete(ctx context.Context, id string) error {
	id, err := requireNonEmpty(id, "AGENT", "id")
	if err != nil {
		return err
	}
	return u.repo.DeleteAgent(ctx, id)
}

// ToggleFavorite flips the is_favorite flag on an agent.
func (u *AgentUsecase) ToggleFavorite(ctx context.Context, id string) (Agent, error) {
	id, err := requireNonEmpty(id, "AGENT", "id")
	if err != nil {
		return Agent{}, err
	}
	a, err := u.repo.GetAgentByID(ctx, id)
	if err != nil {
		if stderrors.Is(err, shared.ErrNotFound) || stderrors.Is(err, sql.ErrNoRows) {
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
	return nil
}

func (u *AgentUsecase) ReorderAgents(ctx context.Context, ids []string) error {
	return u.repo.ReorderAgents(ctx, ids)
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

func (u *AgentUsecase) computeConfigJSON(ctx context.Context, id string) (string, error) {
	a, err := u.repo.GetAgentByID(ctx, id)
	if err != nil {
		return "", err
	}
	settings, err := u.repo.GetAgentRuntimeSettings(ctx, id)
	if err != nil {
		if !stderrors.Is(err, shared.ErrNotFound) && !stderrors.Is(err, sql.ErrNoRows) {
			return "", err
		}
		settings = withSettingDefaults(settingsFromLegacyConfig(a.ConfigJSON))
		settings.AgentID = id
	}
	files, err := u.repo.ListAgentPromptFiles(ctx, id)
	if err != nil {
		return "", err
	}
	cj, err := configJSONFromSettings(withSettingDefaults(settings), files)
	if err != nil {
		return "", err
	}
	HydrateAgentKind(&a)
	cj = EmbedAgentKindInConfigJSON(cj, a.AgentKind, a.A2AProxy, u.lg)
	cj = mergeEvaluationFromLegacy(cj, a.ConfigJSON, u.lg)
	return cj, nil
}

func mergeEvaluationFromLegacy(computed, legacy string, lg loggateway.Logger) string {
	if strings.TrimSpace(legacy) == "" {
		return computed
	}
	var leg map[string]json.RawMessage
	if err := json.Unmarshal([]byte(legacy), &leg); err != nil {
		lg.Warn("解析 legacy config_json 失败", loggateway.StepID("agent.merge_eval"), loggateway.Err(err))
		return computed
	}
	evalRaw, ok := leg["evaluation"]
	if !ok {
		return computed
	}
	var comp map[string]any
	if err := json.Unmarshal([]byte(computed), &comp); err != nil {
		lg.Warn("解析 computed config_json 失败", loggateway.StepID("agent.merge_eval"), loggateway.Err(err))
		return computed
	}
	var eval any
	if err := json.Unmarshal(evalRaw, &eval); err != nil {
		lg.Warn("解析 evaluation 字段失败", loggateway.StepID("agent.merge_eval"), loggateway.Err(err))
		return computed
	}
	comp["evaluation"] = eval
	out, err := json.Marshal(comp)
	if err != nil {
		return computed
	}
	return string(out)
}

func mergeAgentCatalog(current, patch Agent) Agent {
	out := current
	out.AgentKey = firstNonEmpty(patch.AgentKey, current.AgentKey)
	out.DisplayName = firstNonEmpty(patch.DisplayName, current.DisplayName)
	out.Provider = firstNonEmpty(patch.Provider, current.Provider)
	out.Model = firstNonEmpty(patch.Model, current.Model)
	out.Status = firstNonEmpty(patch.Status, current.Status)
	out.IsDefault = patch.IsDefault
	out.IsFavorite = patch.IsFavorite
	out.Icon = patch.Icon
	out.AgentDescription = patch.AgentDescription
	out.PositionID = patch.PositionID
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

// firstNonEmpty returns the first argument after TrimSpace if non-empty, otherwise the second.
func firstNonEmpty(a, b string) string {
	a = strings.TrimSpace(a)
	if a != "" {
		return a
	}
	return b
}

// UpsertByKey 按 agent_key 幂等创建或更新 Agent。
// 如果 agent_key 已存在则更新，否则创建。返回最终的 Agent（含水合）。
func (u *AgentUsecase) UpsertByKey(ctx context.Context, agent Agent) (Agent, error) {
	agent.AgentKey = strings.TrimSpace(agent.AgentKey)
	if agent.AgentKey == "" {
		return Agent{}, kerrors.BadRequest("AGENT", "agent_key is required for upsert")
	}
	existing, err := u.repo.GetAgentByAgentKey(ctx, agent.AgentKey)
	if err == nil {
		// 已存在 → 更新
		agent.ID = existing.ID
		return u.Update(ctx, existing.ID, agent)
	}
	if !stderrors.Is(err, shared.ErrNotFound) && !stderrors.Is(err, sql.ErrNoRows) {
		return Agent{}, err
	}
	// 不存在 → 创建
	return u.Create(ctx, agent)
}

// CreateWithFilesAndSettings 在事务中创建 Agent 并同时写入 Files 和 RuntimeSettings。
// 适用于 Pack 导入等需要精确控制写入内容的场景。
func (u *AgentUsecase) CreateWithFilesAndSettings(ctx context.Context, agent Agent, files []AgentPromptFile, settings *AgentRuntimeSettings) (Agent, error) {
	agent.AgentKey = strings.TrimSpace(agent.AgentKey)
	agent.DisplayName = strings.TrimSpace(agent.DisplayName)
	agent.AgentKind = NormalizeAgentKind(agent.AgentKind)
	HydrateAgentKind(&agent)

	if agent.AgentKey == "" || agent.DisplayName == "" {
		return Agent{}, kerrors.BadRequest("AGENT", "agent_key and display_name are required")
	}
	if agent.ID == "" {
		agent.ID = newAgentCatalogID()
	}
	if agent.Status == "" {
		agent.Status = "active"
	}

	// Settings
	var s AgentRuntimeSettings
	if settings != nil {
		s = *settings
	} else {
		s = withSettingDefaults(settingsFromAgentInput(agent))
	}
	s.AgentID = agent.ID

	// Files
	for i := range files {
		files[i].AgentID = agent.ID
	}
	files = withFileDefaults(files)

	// ConfigJSON
	if strings.TrimSpace(agent.ConfigJSON) == "" {
		cj, err := configJSONFromSettings(s, files)
		if err != nil {
			return Agent{}, err
		}
		agent.ConfigJSON = cj
	}
	agent.ConfigJSON = EmbedAgentKindInConfigJSON(agent.ConfigJSON, agent.AgentKind, agent.A2AProxy, u.lg)

	if err := u.repo.ExecInTx(ctx, func(txCtx context.Context) error {
		if _, err := u.repo.CreateAgent(txCtx, agent); err != nil {
			if isAgentKeyDuplicate(err) {
				return kerrors.BadRequest("AGENT_KEY_CONFLICT", "agent_key already in use")
			}
			return err
		}
		if _, err := u.repo.UpsertAgentRuntimeSettings(txCtx, s); err != nil {
			return err
		}
		if len(files) > 0 {
			if _, err := u.repo.ReplaceAgentPromptFiles(txCtx, agent.ID, files); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return Agent{}, err
	}
	readCtx := ctx
	if ctx.Err() != nil {
		readCtx = context.Background()
	}
	return u.Get(readCtx, agent.ID)
}

// AgentBatchUpdateInput is LIST-04 bulk enable/disable/delete.
type AgentBatchUpdateInput struct {
	IDs    []string
	Status string // optional: active | inactive
	Delete bool
}

// BatchUpdateAgents applies status changes or deletes for many agents inside a transaction.
func (u *AgentUsecase) BatchUpdateAgents(ctx context.Context, in AgentBatchUpdateInput) (int, error) {
	if u == nil || u.repo == nil {
		return 0, kerrors.InternalServer("AGENT", "agent repository not configured")
	}
	var n int
	err := u.repo.ExecInTx(ctx, func(txCtx context.Context) error {
		for _, id := range in.IDs {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			if in.Delete {
				// Permission check: same as single Delete
				a, err := u.repo.GetAgentByID(txCtx, id)
				if err != nil {
					return err
				}
				if a.Kind == "system_builtin" {
					return kerrors.Forbidden("AGENT", "cannot delete system_builtin agent")
				}
				if a.Kind == "ecosystem_preset" {
					return kerrors.Forbidden("AGENT", "cannot delete ecosystem_preset agent directly; use industry unload instead")
				}
				if a.Readonly {
					return kerrors.Forbidden("AGENT", "cannot delete a readonly agent")
				}
				if err := u.repo.DeleteAgent(txCtx, id); err != nil {
					return err
				}
				n++
				continue
			}
			if st := strings.TrimSpace(in.Status); st != "" {
				a, err := u.repo.GetAgentByID(txCtx, id)
				if err != nil {
					return err
				}
				a.Status = st
				if _, err := u.repo.UpdateAgent(txCtx, a); err != nil {
					return err
				}
				n++
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return n, nil
}
