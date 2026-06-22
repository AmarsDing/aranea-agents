package biz

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	stderrors "errors"
	"regexp"
	"strings"
	"sync/atomic"

	"aranea-agents/internal/biz/shared"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
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

// Stability:stable
type AgentReader interface {
	SearchAgents(ctx context.Context, q AgentListQuery) (AgentListResult, error)
	GetAgentByID(ctx context.Context, id string) (Agent, error)
	GetAgentByAgentKey(ctx context.Context, agentKey string) (Agent, error)
	ListExtrasForAgents(ctx context.Context, agentIDs []string) (map[string]AgentListExtras, error)
}

// Stability:stable
type AgentWriter interface {
	CreateAgent(ctx context.Context, a Agent) (Agent, error)
	UpdateAgent(ctx context.Context, a Agent) (Agent, error)
	DeleteAgent(ctx context.Context, id string) error
	ToggleFavorite(ctx context.Context, id string) (Agent, error)
}

// Stability:stable
type AgentRuntimeSettingsRepo interface {
	GetAgentRuntimeSettings(ctx context.Context, agentID string) (AgentRuntimeSettings, error)
	UpsertAgentRuntimeSettings(ctx context.Context, v AgentRuntimeSettings) (AgentRuntimeSettings, error)
}

// Stability:stable
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
// Stability:evolving
type AgentAtomicWriter interface {
	CreateAgentAtomic(ctx context.Context, a Agent, files []AgentPromptFile, settings AgentRuntimeSettings) (Agent, error)
	UpdateAgentAtomic(ctx context.Context, a Agent, files []AgentPromptFile, settings *AgentRuntimeSettings) (Agent, error)
}

// AgentPositionRepo manages agent ordering and position within departments.
// Stability:stable
type AgentPositionRepo interface {
	ListAgentCreators(ctx context.Context) ([]AgentCreator, error)
	ReorderAgents(ctx context.Context, ids []string) error
	ClearPositionByDepartment(ctx context.Context, deptID string) (int, error)
}

// AgentTxRepo provides transactional execution for multi-step agent operations.
// Stability:stable
type AgentTxRepo interface {
	ExecInTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// AgentRepository is the composite interface aggregating all narrow agent sub-interfaces.
// Stability:stable
type AgentRepository interface {
	AgentReader
	AgentWriter
	AgentAtomicWriter
	AgentRuntimeSettingsRepo
	AgentPromptFileRepo
	AgentReferenceChecker
	AgentPositionRepo
	AgentTxRepo
}

// ProviderModelPairValidator validates that a provider+model pair exists in the catalog.
type ProviderModelPairValidator interface {
	ValidatePair(ctx context.Context, provider, model string) (bool, string, error)
}

// AgentUsecaseDeps groups the narrow interface dependencies for AgentUsecase.
// Using a deps struct avoids long parameter lists (CS-B7) while keeping
// each dependency as a narrow interface for testability.
type AgentUsecaseDeps struct {
	Reader             AgentReader
	Writer             AgentWriter
	Settings           AgentRuntimeSettingsRepo
	Files              AgentPromptFileRepo
	Position           AgentPositionRepo
	Tx                 AgentTxRepo
	Tools              ToolRegistryReader
	Sys                SystemSettingRepo
	WebResearchChecker WebResearchReadinessChecker
	ProviderValidator  ProviderModelPairValidator
	Lg                 loggateway.Logger
}

// AgentUsecase is catalog agent CRUD + prompt preview.
type AgentUsecase struct {
	reader             AgentReader
	writer             AgentWriter
	settings           AgentRuntimeSettingsRepo
	files              AgentPromptFileRepo
	position           AgentPositionRepo
	tx                 AgentTxRepo
	tools              ToolRegistryReader
	sys                SystemSettingRepo
	webResearchChecker WebResearchReadinessChecker
	providerValidator  ProviderModelPairValidator
	lg                 loggateway.Logger
	agentSM            *AgentStateMachine
}

func NewAgentUsecase(d AgentUsecaseDeps) *AgentUsecase {
	return &AgentUsecase{
		reader: d.Reader, writer: d.Writer, settings: d.Settings, files: d.Files,
		position: d.Position, tx: d.Tx, tools: d.Tools, sys: d.Sys,
		webResearchChecker: d.WebResearchChecker, providerValidator: d.ProviderValidator, lg: d.Lg,
		agentSM: NewAgentStateMachine(),
	}
}

// ListAgentCreators returns distinct creators for list filter options.
func (u *AgentUsecase) ListAgentCreators(ctx context.Context) ([]AgentCreator, error) {
	if u == nil || u.position == nil {
		return nil, nil
	}
	return u.position.ListAgentCreators(ctx)
}

// List returns a page of agents without per-row hydration (settings/files).
func (u *AgentUsecase) List(ctx context.Context, q AgentListQuery) (AgentListResult, error) {
	page, err := u.reader.SearchAgents(ctx, q)
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
	extras, err := u.reader.ListExtrasForAgents(ctx, ids)
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
		return false, "agent_key is required", apierror.BadRequest("AGENT", "agent_key is required")
	}
	if !agentKeyPattern.MatchString(agentKey) {
		return false, "invalid agent_key format", apierror.BadRequest("AGENT_KEY_INVALID", "agent_key must be lowercase letters, digits, and hyphens")
	}
	_, err = u.reader.GetAgentByAgentKey(ctx, agentKey)
	if err == nil {
		return false, "agent_key already in use", nil
	}
	if !stderrors.Is(err, shared.ErrNotFound) {
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
	a, err := u.reader.GetAgentByID(ctx, id)
	if err != nil {
		if stderrors.Is(err, shared.ErrNotFound) {
			return Agent{}, shared.ErrNotFound
		}
		return Agent{}, err
	}
	return u.hydrate(ctx, a)
}

func (u *AgentUsecase) GetByAgentKey(ctx context.Context, agentKey string) (Agent, error) {
	agentKey, err := requireNonEmpty(agentKey, "AGENT", "agent_key")
	if err != nil {
		return Agent{}, err
	}
	a, err := u.reader.GetAgentByAgentKey(ctx, agentKey)
	if err != nil {
		if stderrors.Is(err, shared.ErrNotFound) {
			return Agent{}, shared.ErrNotFound
		}
		return Agent{}, err
	}
	return u.hydrate(ctx, a)
}

func (u *AgentUsecase) GetAgentRuntimeSettings(ctx context.Context, agentID string) (AgentRuntimeSettings, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return AgentRuntimeSettings{}, apierror.BadRequest("AGENT", "agent id is required")
	}
	settings, err := u.settings.GetAgentRuntimeSettings(ctx, agentID)
	if err != nil {
		if stderrors.Is(err, shared.ErrNotFound) {
			return DefaultAgentRuntimeSettings(), nil
		}
		return AgentRuntimeSettings{}, err
	}
	return settings, nil
}

func (u *AgentUsecase) hydrate(ctx context.Context, agent Agent) (Agent, error) {
	return u.hydrateWithExtras(ctx, agent, false)
}

// hydrateWithExtras hydrates an agent with settings, files, and optionally extras.
// When skipExtras is true, the ListExtrasForAgents query is skipped (suitable for
// orchestration/build paths that only need settings + files, not list-display fields).
func (u *AgentUsecase) hydrateWithExtras(ctx context.Context, agent Agent, skipExtras bool) (Agent, error) {
	settings, err := u.settings.GetAgentRuntimeSettings(ctx, agent.ID)
	if err != nil {
		if !stderrors.Is(err, shared.ErrNotFound) {
			return Agent{}, err
		}
		u.lg.Warn("agent runtime settings not found, migrating from legacy config_json", loggateway.StepID("agent.db_resolve"), loggateway.Str("agent_id", agent.ID))
		settings = u.migrateLegacySettings(ctx, agent)
	}
	files, err := u.files.ListAgentPromptFiles(ctx, agent.ID)
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
	if !skipExtras {
		if extras, err := u.reader.ListExtrasForAgents(ctx, []string{agent.ID}); err != nil {
			u.lg.Warn("agent extras query failed, skipping enrichment",
				loggateway.StepID("agent.hydrate_extras"),
				loggateway.Str("agent_id", agent.ID),
				loggateway.Err(err))
		} else if ex, ok := extras[agent.ID]; ok {
			agent.LastRunStatus = ex.LastRunStatus
			agent.LastRunAt = ex.LastRunAt
			agent.PendingEvolutionCount = ex.PendingEvolutionCount
		}
	}
	return agent, nil
}

// BatchHydrateForBuild hydrates multiple agents in bulk for orchestration/build paths.
// Unlike calling Get() in a loop, this method:
//  1. Skips the ListExtrasForAgents query (not needed for runtime builds).
//  2. Batches the extras query when needed (future optimization point).
//
// This avoids the N+1 query pattern that would result from calling Get() per agent.
func (u *AgentUsecase) BatchHydrateForBuild(ctx context.Context, agents []Agent) ([]Agent, error) {
	out := make([]Agent, len(agents))
	for i, a := range agents {
		h, err := u.hydrateWithExtras(ctx, a, true)
		if err != nil {
			return nil, err
		}
		out[i] = h
	}
	return out, nil
}

func (u *AgentUsecase) migrateLegacySettings(ctx context.Context, agent Agent) AgentRuntimeSettings {
	settings := withSettingDefaults(settingsFromLegacyConfig(agent.ConfigJSON))
	settings.AgentID = agent.ID
	migrated, err := u.settings.UpsertAgentRuntimeSettings(ctx, settings)
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
	migrated, err := u.files.ReplaceAgentPromptFiles(ctx, agent.ID, withFileDefaults(files))
	if err != nil {
		return withFileDefaults(files)
	}
	return migrated
}

// validateAgentCreate performs common validation for agent creation.
// It mutates settings for A2A Proxy agents (force-disables certain features).
func validateAgentCreate(ctx context.Context, u *AgentUsecase, agent *Agent, settings *AgentRuntimeSettings, skipProviderValidate bool) error {
	switch agent.AgentKind {
	case AgentKindA2AProxy:
		if agent.A2AProxy == nil || strings.TrimSpace(agent.A2AProxy.RemoteURL) == "" {
			return apierror.BadRequest("AGENT", "a2a_proxy remote_url is required")
		}
		if agent.Provider == "" {
			agent.Provider = "a2a"
		}
		if agent.Model == "" {
			agent.Model = "proxy"
		}
	default:
		// Allow empty provider/model — the agent will inherit the model
		// from the chat interface at runtime (resolved via WithModel RunOption).
		if (agent.Provider != "" && agent.Model == "") || (agent.Provider == "" && agent.Model != "") {
			return apierror.BadRequest("AGENT", "provider and model must both be set or both be empty")
		}
	}
	return validateAgentSettings(ctx, u, agent, settings, "agent.create.provider_validate", skipProviderValidate)
}

// validateAgentUpdate performs validation for agent updates.
// Unlike create, provider/model are already set on the merged agent;
// this validates the pair and all settings fields.
func validateAgentUpdate(ctx context.Context, u *AgentUsecase, agent *Agent, settings *AgentRuntimeSettings) error {
	if IsA2AProxyAgent(*agent) {
		if agent.A2AProxy == nil || strings.TrimSpace(agent.A2AProxy.RemoteURL) == "" {
			return apierror.BadRequest("AGENT", "a2a_proxy remote_url is required")
		}
	}
	return validateAgentSettings(ctx, u, agent, settings, "agent.update.provider_validate", false)
}

// validateAgentSettings is the shared validation logic for both Create and Update paths.
// It validates provider/model pairs, settings fields, and force-disables A2A-incompatible features.
func validateAgentSettings(ctx context.Context, u *AgentUsecase, agent *Agent, settings *AgentRuntimeSettings, logStepID string, skipProviderValidate bool) error {
	// Validate that the provider+model pair exists in the catalog (non-A2A agents only).
	// Skip validation when provider/model is empty — the agent will inherit
	// the model from the chat interface at runtime (resolved via WithModel RunOption).
	// Duplicate may also skip validation so a clone of an existing agent can be created
	// even when its original provider/model is no longer enabled in the catalog.
	if !skipProviderValidate && u.providerValidator != nil && !IsA2AProxyAgent(*agent) {
		prov := strings.TrimSpace(agent.Provider)
		mod := strings.TrimSpace(agent.Model)
		if prov != "" || mod != "" {
			ok, msg, valErr := u.providerValidator.ValidatePair(ctx, prov, mod)
			if valErr != nil {
				u.lg.Warn("provider model validation failed, proceeding",
					loggateway.StepID(logStepID),
					loggateway.Str("provider", prov),
					loggateway.Str("model", mod),
					loggateway.Err(valErr))
			} else if !ok {
				return apierror.BadRequest("AGENT", "provider/model is not enabled: "+msg)
			}
		}
	}
	if err := ValidateCodeExecutorType(settings.CodeExecutorType); err != nil {
		return err
	}
	if err := ValidatePlannerKind(settings.PlannerKind); err != nil {
		return err
	}
	if err := ValidatePlannerConfigJSON(settings.PlannerKind, settings.PlannerConfigJSON); err != nil {
		return err
	}
	if err := ValidateRalphLoopSettings(settings); err != nil {
		return err
	}
	// 校验 ToolsAllowJSON / ToolsDenyJSON 是有效的 JSON 字符串数组格式。
	// 注意：allow/deny 列表允许包含 group:* 前缀和未注册的 tool key（设计如此，
	// 不存在的 key 会被运行时忽略），因此只校验 JSON 格式，不校验 key 存在性。
	if err := validateToolsPolicyJSON(settings.ToolsAllowJSON, "tools_allow"); err != nil {
		return err
	}
	if err := validateToolsPolicyJSON(settings.ToolsDenyJSON, "tools_deny"); err != nil {
		return err
	}
	if IsA2AProxyAgent(*agent) {
		settings.IntentPassEnabled = false
		settings.ToolsEnabled = false
		settings.MemoryEnabled = false
		settings.SelfEvolve = false
	}
	return nil
}

// validateToolsPolicyJSON 校验 ToolsAllowJSON / ToolsDenyJSON 是有效的 JSON 字符串数组格式。
// 空字符串和 "[]" 视为合法（表示无策略）。允许包含 "group:*" 前缀的组 key。
func validateToolsPolicyJSON(raw, field string) error {
	if _, err := shared.JSONStringList(raw); err != nil {
		return apierror.BadRequest("AGENT", field+" json parse: "+err.Error())
	}
	return nil
}

// Create inserts an agent and persists settings + prompt files.
func (u *AgentUsecase) Create(ctx context.Context, in Agent) (Agent, error) {
	return u.create(ctx, in, false)
}

// create is the internal creation path shared by Create and Duplicate.
// When skipProviderValidate is true, the provider/model catalog check is skipped
// (used by Duplicate so a clone can be created from an existing agent even when
// its original provider/model is no longer enabled).
func (u *AgentUsecase) create(ctx context.Context, in Agent, skipProviderValidate bool) (Agent, error) {
	in.AgentKey = strings.TrimSpace(in.AgentKey)
	in.DisplayName = strings.TrimSpace(in.DisplayName)
	in.Provider = strings.TrimSpace(in.Provider)
	in.Model = strings.TrimSpace(in.Model)
	in.AgentKind = NormalizeAgentKind(in.AgentKind)
	HydrateAgentKind(&in)

	if in.AgentKey == "" || in.DisplayName == "" {
		return Agent{}, apierror.BadRequest("AGENT", "agent_key and display_name are required")
	}
	if in.ID == "" {
		in.ID = newAgentCatalogID()
	}
	settings := withSettingDefaults(settingsFromAgentInput(in))
	settings.AgentID = in.ID
	if err := validateAgentCreate(ctx, u, &in, &settings, skipProviderValidate); err != nil {
		return Agent{}, err
	}
	if in.AgentKind != AgentKindA2AProxy {
		in.AgentKind = AgentKindLLM
	}
	files := filesFromAgentInput(in)
	for i := range files {
		files[i].AgentID = in.ID
	}
	files = withFileDefaults(files)
	// DEV-10 FIXED: ConfigJSON is now a read-only projection of Settings + Files.
	// It is no longer written to the database; computeConfigJSON / hydrate
	// generates it on-demand for read paths. The in.ConfigJSON field is
	// intentionally left empty so the data layer stores no stale snapshot.
	in.ConfigJSON = ""
	in.Status = strings.TrimSpace(in.Status)
	if in.Status == "" {
		in.Status = string(AgentStatusActive)
	} else if err := ValidateAgentStatus(in.Status); err != nil {
		return Agent{}, err
	}
	if strings.TrimSpace(in.CreatedBy) == "" {
		in.CreatedBy = AgentCreatedByFromContext(ctx)
	}
	if err := u.tx.ExecInTx(ctx, func(txCtx context.Context) error {
		if _, err := u.writer.CreateAgent(txCtx, in); err != nil {
			if isAgentKeyDuplicate(err) {
				return apierror.BadRequest("AGENT_KEY_CONFLICT", "agent_key already in use: "+err.Error())
			}
			return err
		}
		if _, err := u.settings.UpsertAgentRuntimeSettings(txCtx, settings); err != nil {
			return err
		}
		if _, err := u.files.ReplaceAgentPromptFiles(txCtx, in.ID, files); err != nil {
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
	id, err := requireNonEmpty(id, "AGENT", "id")
	if err != nil {
		return Agent{}, err
	}
	current, err := u.Get(ctx, id)
	if err != nil {
		return Agent{}, err
	}
	HydrateAgentKind(&patch)
	HydrateAgentKind(&current)
	if strings.TrimSpace(patch.AgentKey) != "" && strings.TrimSpace(patch.AgentKey) != strings.TrimSpace(current.AgentKey) {
		return Agent{}, apierror.BadRequest("AGENT", "agent_key is immutable")
	}
	if strings.TrimSpace(patch.AgentKind) != "" && NormalizeAgentKind(patch.AgentKind) != NormalizeAgentKind(current.AgentKind) {
		return Agent{}, apierror.BadRequest("AGENT", "agent_kind is immutable")
	}
	merged := mergeAgentCatalog(current, patch)
	// AS-FSM-01: Validate state transition when status changes.
	if merged.Status != current.Status && strings.TrimSpace(merged.Status) != "" {
		if _, err := u.agentSM.Transition(ParseAgentState(current.Status), agentEventForTarget(ParseAgentState(merged.Status))); err != nil {
			return Agent{}, apierror.BadRequest("AGENT", "invalid status transition from "+current.Status+" to "+merged.Status)
		}
	}
	settings := withSettingDefaults(settingsFromAgentInput(merged))
	settings.AgentID = id
	if err := validateAgentUpdate(ctx, u, &merged, &settings); err != nil {
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
	// DEV-10 FIXED: ConfigJSON is no longer written; cleared before persisting.
	merged.ConfigJSON = ""
	if err := u.tx.ExecInTx(ctx, func(txCtx context.Context) error {
		if _, err := u.writer.UpdateAgent(txCtx, merged); err != nil {
			return err
		}
		if _, err := u.settings.UpsertAgentRuntimeSettings(txCtx, settings); err != nil {
			return err
		}
		if _, err := u.files.ReplaceAgentPromptFiles(txCtx, id, files); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return Agent{}, err
	}
	// Use a detached context for the final read: the transaction has already
	// committed, but the HTTP request context may have been cancelled while
	// waiting for a SQLite write lock.  The caller still needs the updated
	// agent data to return to the frontend.
	readCtx := ctx
	if ctx.Err() != nil {
		readCtx = context.Background()
	}
	out, err := u.Get(readCtx, id)
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
	if err := canDeleteAgent(current); err != nil {
		return err
	}
	return u.writer.DeleteAgent(ctx, id)
}

// canDeleteAgent is the unified delete-permission check used by both
// single Delete and BatchUpdateAgents to ensure consistent authorization.
func canDeleteAgent(a Agent) error {
	// Kind is the ownership classification (user | system_builtin | ecosystem_preset | ...).
	if a.Kind == "system_builtin" {
		if strings.HasPrefix(a.AgentKey, "__dept_lead_") {
			return apierror.Forbidden("AGENT", "cannot delete department lead agent; delete the department instead")
		}
		return apierror.Forbidden("AGENT", "cannot delete system_builtin agent")
	}
	if a.Kind == "ecosystem_preset" {
		return apierror.Forbidden("AGENT", "cannot delete ecosystem_preset agent directly; use industry unload instead")
	}
	if a.Readonly {
		return apierror.Forbidden("AGENT", "cannot delete a readonly agent")
	}
	return nil
}

// ForceDelete soft-deletes the agent bypassing kind/readonly permission checks.
// Only for internal system operations (e.g., cleaning up dept lead agents).
func (u *AgentUsecase) ForceDelete(ctx context.Context, id string) error {
	id, err := requireNonEmpty(id, "AGENT", "id")
	if err != nil {
		return err
	}
	return u.writer.DeleteAgent(ctx, id)
}

// ToggleFavorite flips the is_favorite flag on an agent.
// Delegates to repo for atomic SQL toggle to avoid read-then-write race condition.
func (u *AgentUsecase) ToggleFavorite(ctx context.Context, id string) (Agent, error) {
	id, err := requireNonEmpty(id, "AGENT", "id")
	if err != nil {
		return Agent{}, err
	}
	updated, err := u.writer.ToggleFavorite(ctx, id)
	if err != nil {
		if stderrors.Is(err, shared.ErrNotFound) {
			return Agent{}, apierror.NotFound("AGENT", "agent not found")
		}
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
		return AgentPromptFile{}, apierror.BadRequest("AGENT_FILE", "agent_id and name are required")
	}
	if _, err := u.Get(ctx, f.AgentID); err != nil {
		return AgentPromptFile{}, err
	}
	created, err := u.files.CreateAgentPromptFile(ctx, f)
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
		return AgentPromptFile{}, apierror.BadRequest("AGENT_FILE", "agent_id and id are required")
	}
	if _, err := u.Get(ctx, f.AgentID); err != nil {
		return AgentPromptFile{}, err
	}
	updated, err := u.files.UpdateAgentPromptFile(ctx, f)
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
		return apierror.BadRequest("AGENT_FILE", "agent_id and id are required")
	}
	if _, err := u.Get(ctx, agentID); err != nil {
		return err
	}
	if err := u.files.DeleteAgentPromptFile(ctx, agentID, id); err != nil {
		return err
	}
	return nil
}

func (u *AgentUsecase) ReorderAgents(ctx context.Context, ids []string) error {
	return u.position.ReorderAgents(ctx, ids)
}

// EstimateTokens returns an approximate token count for all prompt files of an agent.
func (u *AgentUsecase) EstimateTokens(ctx context.Context, agentID string) (FileTokenEstimates, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return FileTokenEstimates{}, apierror.BadRequest("AGENT_FILE", "agent_id is required")
	}
	a, err := u.Get(ctx, agentID)
	if err != nil {
		return FileTokenEstimates{}, err
	}
	return estimateTokensForFiles(a.Files), nil
}

func (u *AgentUsecase) computeConfigJSON(ctx context.Context, id string) (string, error) {
	a, err := u.reader.GetAgentByID(ctx, id)
	if err != nil {
		return "", err
	}
	settings, err := u.settings.GetAgentRuntimeSettings(ctx, id)
	if err != nil {
		if !stderrors.Is(err, shared.ErrNotFound) {
			return "", err
		}
		settings = withSettingDefaults(settingsFromLegacyConfig(a.ConfigJSON))
		settings.AgentID = id
	}
	files, err := u.files.ListAgentPromptFiles(ctx, id)
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
	// Immutable fields: AgentKey and AgentKind are never merged from patch.
	// They are validated as immutable in Update() before this function is called.
	// Skipping them here prevents accidental overwrite if patch carries a stale value.
	// out.AgentKey remains current.AgentKey
	// out.AgentKind remains current.AgentKind
	out.DisplayName = firstNonEmpty(patch.DisplayName, current.DisplayName)
	out.Provider = firstNonEmpty(patch.Provider, current.Provider)
	out.Model = firstNonEmpty(patch.Model, current.Model)
	out.Status = firstNonEmpty(patch.Status, current.Status)
	// Boolean fields: *bool semantics — nil means "not set" (skip), non-nil
	// means "explicitly set" (overwrite). This solves the Proto3 zero-value
	// ambiguity where false and "not set" are indistinguishable.
	if patch.IsDefault != nil {
		out.IsDefault = patch.IsDefault
	}
	if patch.IsFavorite != nil {
		out.IsFavorite = patch.IsFavorite
	}
	out.Icon = firstNonEmpty(patch.Icon, current.Icon)
	out.AgentDescription = firstNonEmpty(patch.AgentDescription, current.AgentDescription)
	out.PositionID = firstNonEmpty(patch.PositionID, current.PositionID)
	out.SystemPromptMode = firstNonEmpty(patch.SystemPromptMode, current.SystemPromptMode)
	if patch.ContextWindow != 0 {
		out.ContextWindow = patch.ContextWindow
	}
	if patch.BudgetMonthlyCents != 0 {
		out.BudgetMonthlyCents = patch.BudgetMonthlyCents
	}
	// DEV-10 FIXED: ConfigJSON is no longer merged/written; it is computed on read.
	// out.ConfigJSON is intentionally left from current (will be cleared before persist).
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
		return Agent{}, apierror.BadRequest("AGENT", "agent_key is required for upsert")
	}
	existing, err := u.reader.GetAgentByAgentKey(ctx, agent.AgentKey)
	if err == nil {
		// 已存在 → 更新
		agent.ID = existing.ID
		return u.Update(ctx, existing.ID, agent)
	}
	if !stderrors.Is(err, shared.ErrNotFound) {
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
	agent.Provider = strings.TrimSpace(agent.Provider)
	agent.Model = strings.TrimSpace(agent.Model)
	agent.AgentKind = NormalizeAgentKind(agent.AgentKind)
	HydrateAgentKind(&agent)

	if agent.AgentKey == "" || agent.DisplayName == "" {
		return Agent{}, apierror.BadRequest("AGENT", "agent_key and display_name are required")
	}
	if agent.ID == "" {
		agent.ID = newAgentCatalogID()
	}
	if agent.Status == "" {
		agent.Status = string(AgentStatusActive)
	} else if err := ValidateAgentStatus(agent.Status); err != nil {
		return Agent{}, err
	}

	// Settings
	var s AgentRuntimeSettings
	if settings != nil {
		s = *settings
	} else {
		s = withSettingDefaults(settingsFromAgentInput(agent))
	}
	s.AgentID = agent.ID

	if err := validateAgentCreate(ctx, u, &agent, &s, false); err != nil {
		return Agent{}, err
	}
	if agent.AgentKind != AgentKindA2AProxy {
		agent.AgentKind = AgentKindLLM
	}

	// Files
	for i := range files {
		files[i].AgentID = agent.ID
	}
	files = withFileDefaults(files)

	// DEV-10 FIXED: ConfigJSON is no longer written; cleared before persisting.
	agent.ConfigJSON = ""

	if err := u.tx.ExecInTx(ctx, func(txCtx context.Context) error {
		if _, err := u.writer.CreateAgent(txCtx, agent); err != nil {
			if isAgentKeyDuplicate(err) {
				return apierror.BadRequest("AGENT_KEY_CONFLICT", "agent_key already in use")
			}
			return err
		}
		if _, err := u.settings.UpsertAgentRuntimeSettings(txCtx, s); err != nil {
			return err
		}
		if len(files) > 0 {
			if _, err := u.files.ReplaceAgentPromptFiles(txCtx, agent.ID, files); err != nil {
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
	if u == nil || u.tx == nil {
		return 0, apierror.Internal("AGENT", "agent repository not configured")
	}
	if st := strings.TrimSpace(in.Status); st != "" {
		if err := ValidateAgentStatus(st); err != nil {
			return 0, err
		}
	}
	var n int
	err := u.tx.ExecInTx(ctx, func(txCtx context.Context) error {
		for _, id := range in.IDs {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			if in.Delete {
				// Permission check: unified with single Delete via canDeleteAgent
				a, err := u.reader.GetAgentByID(txCtx, id)
				if err != nil {
					return err
				}
				if err := canDeleteAgent(a); err != nil {
					return err
				}
				if err := u.writer.DeleteAgent(txCtx, id); err != nil {
					return err
				}
				n++
				continue
			}
			if st := strings.TrimSpace(in.Status); st != "" {
				a, err := u.reader.GetAgentByID(txCtx, id)
				if err != nil {
					return err
				}
				// AS-FSM-01: Validate state transition.
				if a.Status != st {
					if _, err := u.agentSM.Transition(ParseAgentState(a.Status), agentEventForTarget(ParseAgentState(st))); err != nil {
						return apierror.BadRequest("AGENT", "invalid status transition from "+a.Status+" to "+st+" for agent "+id)
					}
				}
				a.Status = st
				if _, err := u.writer.UpdateAgent(txCtx, a); err != nil {
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
