package biz

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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
	// ListAgentsByIDs returns agents matching the given IDs (order not guaranteed).
	// Missing IDs are silently skipped — callers should check len(result) vs len(ids).
	ListAgentsByIDs(ctx context.Context, ids []string) ([]Agent, error)
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
	// ListAgentRuntimeSettings returns settings for all agents keyed by agent ID.
	// Agents without a settings row are absent from the map (callers apply defaults).
	ListAgentRuntimeSettings(ctx context.Context) (map[string]AgentRuntimeSettings, error)
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
	// It is no longer written to the database; hydrate generates it on-demand
	// for read paths. The in.ConfigJSON field is intentionally left empty so
	// the data layer stores no stale snapshot.
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
