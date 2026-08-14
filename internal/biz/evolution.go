package biz

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// Evolution suggestion status constants.
const (
	EvolutionStatusPending    = "pending"
	EvolutionStatusApplied    = "applied"
	EvolutionStatusRejected   = "rejected"
	EvolutionStatusRolledBack = "rolled_back"
)

type EvolutionMetrics struct {
	AgentID                string
	TimeRange              string
	ToolSuccessRate        float64
	RetrievalQuality       float64
	TotalEpisodes          int
	NegativeFeedback       int
	ToolSuccessSeries      []MetricDataPoint
	RetrievalQualitySeries []MetricDataPoint
	// S-05 fix: indicate when metrics data is incomplete due to partial query failures.
	Partial bool
	// S-08 fix: record which sub-queries failed for observability.
	PartialErrors []string
}

type MetricDataPoint struct {
	Date  string
	Value float64
}

// EvolutionSuggestion is the L3 (agent persona/prompt/skill) suggestion view
// exposed to the service layer. After the A6 convergence the physical storage
// is unified_evolution_suggestions; legacy-only fields (type/title/diff_preview/
// pre_apply_snapshot) live in the unified row's metadata JSON and are
// reconstructed by evolutionViewFromUnified.
type EvolutionSuggestion struct {
	ID               string
	AgentID          string
	Type             string
	Title            string
	Content          string
	Status           string
	DiffPreview      string
	PreApplySnapshot string // JSON-encoded map[filename]content of files before apply
	// ApplyPayload is the substantive content written into prompt files on
	// apply (EvoMetaApplyPayload). Notification-only suggestions leave it
	// empty and are rejected by ApplySuggestion.
	ApplyPayload string
	CreatedAt    string
	AppliedAt    string
}

// Applicable reports whether the suggestion can be applied: only persona/prompt
// suggestions carrying a non-empty apply payload write into prompt files.
// Notification-only suggestions (skill / orchestration_optimization / payload-less
// persona/prompt) are not applicable; the frontend hides the apply button and
// ApplySuggestion rejects them as a defense-in-depth guard.
func (s EvolutionSuggestion) Applicable() bool {
	if s.Type != "persona" && s.Type != "prompt" {
		return false
	}
	return strings.TrimSpace(s.ApplyPayload) != ""
}

// evolutionViewFromUnified reconstructs the legacy L3 view from a unified row.
func evolutionViewFromUnified(s *UnifiedEvolutionSuggestion) EvolutionSuggestion {
	if s == nil {
		return EvolutionSuggestion{}
	}
	appliedAt := ""
	if s.AppliedAt != nil {
		appliedAt = s.AppliedAt.UTC().Format(time.RFC3339)
	}
	return EvolutionSuggestion{
		ID:               s.ID,
		AgentID:          s.TargetID,
		Type:             s.MetaString(EvoMetaLegacyType),
		Title:            s.MetaString(EvoMetaTitle),
		Content:          s.DraftBody,
		Status:           s.Status,
		DiffPreview:      s.MetaString(EvoMetaDiffPreview),
		PreApplySnapshot: s.MetaString(EvoMetaPreApplySnapshot),
		ApplyPayload:     s.MetaString(EvoMetaApplyPayload),
		CreatedAt:        s.CreatedAt.UTC().Format(time.RFC3339),
		AppliedAt:        appliedAt,
	}
}

// unifiedFromEvolutionView converts a legacy L3 view into a unified row for
// creation. Mirrors the 20261111 backfill mapping: trigger_source=agent_config,
// trigger_reason=title, action_type=evolve_agent.
func unifiedFromEvolutionView(s EvolutionSuggestion) UnifiedEvolutionSuggestion {
	metadata, _ := json.Marshal(map[string]string{
		EvoMetaLegacyType:       s.Type,
		EvoMetaTitle:            s.Title,
		EvoMetaDiffPreview:      s.DiffPreview,
		EvoMetaPreApplySnapshot: s.PreApplySnapshot,
		EvoMetaApplyPayload:     s.ApplyPayload,
	})
	createdAt, err := time.Parse(time.RFC3339, s.CreatedAt)
	if err != nil {
		createdAt = time.Now().UTC()
	}
	return UnifiedEvolutionSuggestion{
		ID:              s.ID,
		TargetType:      EvolutionTargetAgent,
		TargetID:        s.AgentID,
		ActionType:      EvolutionActionEvolve,
		TriggerSource:   "agent_config",
		TriggerReason:   s.Title,
		Status:          s.Status,
		Priority:        1,
		DraftBody:       s.Content,
		LifecycleStatus: "draft",
		Metadata:        metadata,
		CreatedAt:       createdAt,
	}
}

// Stability:stable
type EvolutionMetricsRepo interface {
	GetToolSuccessRate(ctx context.Context, agentID string, since time.Time) (float64, []MetricDataPoint, error)
	GetRetrievalQuality(ctx context.Context, agentID string, since time.Time) (float64, []MetricDataPoint, error)
	GetEpisodeCount(ctx context.Context, agentID string, since time.Time) (int, error)
	GetNegativeFeedbackCount(ctx context.Context, agentID string, since time.Time) (int, error)
}

// EvolutionSuggestionCreator is the narrow write/list port for L3 suggestions
// used by non-evolution consumers (spirit team completion learning, task
// orchestrator DQ feedback). Implemented by EvolutionUsecase.
// Stability:evolving
type EvolutionSuggestionCreator interface {
	CreateSuggestion(ctx context.Context, s EvolutionSuggestion) (EvolutionSuggestion, error)
	GetEvolutionSuggestions(ctx context.Context, agentID string, status string) ([]EvolutionSuggestion, error)
}

type EvolutionUsecase struct {
	metricsRepo EvolutionMetricsRepo
	store       UnifiedEvolutionStore
	agents      AgentRepository
	lg          loggateway.Logger
	evolutionSM *EvolutionStateMachine
	txProvider  EvolutionTxProvider
}

// EvolutionTxProvider provides transactional execution for atomic prompt-file
// + suggestion-status writes. When set via SetTxProvider, ApplySuggestion and
// RollbackSuggestion wrap the file replacement and status update in a single
// transaction so a status-update failure rolls back the prompt files
// (red line #24).
// Stability:stable
type EvolutionTxProvider interface {
	ExecInTx(ctx context.Context, fn func(ctx context.Context) error) error
}

func NewEvolutionUsecase(
	metricsRepo EvolutionMetricsRepo,
	store UnifiedEvolutionStore,
	agents AgentRepository,
	lg loggateway.Logger,
) *EvolutionUsecase {
	return &EvolutionUsecase{
		metricsRepo: metricsRepo,
		store:       store,
		agents:      agents,
		lg:          lg,
		evolutionSM: NewEvolutionStateMachine(),
	}
}

// ProvideEvolutionUsecase is the Wire provider that constructs an
// EvolutionUsecase and injects the transaction provider so ApplySuggestion
// and RollbackSuggestion wrap file replacement + status update in a single
// transaction (red line #24). Tests call NewEvolutionUsecase directly
// (without a txProvider) to preserve legacy non-transactional behavior.
func ProvideEvolutionUsecase(
	metricsRepo EvolutionMetricsRepo,
	store UnifiedEvolutionStore,
	agents AgentRepository,
	tp EvolutionTxProvider,
	lg loggateway.Logger,
) *EvolutionUsecase {
	uc := NewEvolutionUsecase(metricsRepo, store, agents, lg)
	uc.SetTxProvider(tp)
	return uc
}

// SetTxProvider sets the transaction provider used to wrap multi-step writes
// (prompt-file replacement + suggestion status update) in a single atomic
// transaction. When not set, the writes execute non-transactionally
// (preserving legacy behavior for tests and offline tooling).
func (uc *EvolutionUsecase) SetTxProvider(tp EvolutionTxProvider) {
	uc.txProvider = tp
}

func (uc *EvolutionUsecase) GetEvolutionMetrics(ctx context.Context, agentID string, timeRange string) (EvolutionMetrics, error) {
	agentID, err := requireNonEmpty(agentID, "EVOLUTION", "agent_id")
	if err != nil {
		return EvolutionMetrics{}, err
	}
	return collectEvolutionMetrics(ctx, uc.metricsRepo, uc.lg, agentID, timeRange), nil
}

// collectEvolutionMetrics gathers the four metric families for an agent over
// timeRange. Sub-query failures degrade to partial metrics (Partial=true)
// instead of failing the whole collection. Shared by EvolutionUsecase and
// AgentConfigTrigger (A6).
func collectEvolutionMetrics(ctx context.Context, metricsRepo EvolutionMetricsRepo, lg loggateway.Logger, agentID string, timeRange string) EvolutionMetrics {
	since := timeRangeToSince(timeRange)
	var partial bool
	var partialErrors []string
	toolRate, toolSeries, err := metricsRepo.GetToolSuccessRate(ctx, agentID, since)
	if err != nil {
		partial = true
		partialErrors = append(partialErrors, "GetToolSuccessRate: "+err.Error())
		lg.Warn("GetToolSuccessRate failed", loggateway.StepID("evolution.get_tool_success_rate"), loggateway.Err(err))
	}
	retrievalRate, retrievalSeries, err := metricsRepo.GetRetrievalQuality(ctx, agentID, since)
	if err != nil {
		partial = true
		partialErrors = append(partialErrors, "GetRetrievalQuality: "+err.Error())
		lg.Warn("GetRetrievalQuality failed", loggateway.StepID("evolution.get_retrieval_quality"), loggateway.Err(err))
	}
	episodes, err := metricsRepo.GetEpisodeCount(ctx, agentID, since)
	if err != nil {
		partial = true
		partialErrors = append(partialErrors, "GetEpisodeCount: "+err.Error())
		lg.Warn("GetEpisodeCount failed", loggateway.StepID("evolution.get_episode_count"), loggateway.Err(err))
	}
	negFeedback, err := metricsRepo.GetNegativeFeedbackCount(ctx, agentID, since)
	if err != nil {
		partial = true
		partialErrors = append(partialErrors, "GetNegativeFeedbackCount: "+err.Error())
		lg.Warn("GetNegativeFeedbackCount failed", loggateway.StepID("evolution.get_negative_feedback_count"), loggateway.Err(err))
	}
	return EvolutionMetrics{
		AgentID:                agentID,
		TimeRange:              timeRange,
		ToolSuccessRate:        toolRate,
		RetrievalQuality:       retrievalRate,
		TotalEpisodes:          episodes,
		NegativeFeedback:       negFeedback,
		ToolSuccessSeries:      toolSeries,
		RetrievalQualitySeries: retrievalSeries,
		Partial:                partial,
		PartialErrors:          partialErrors,
	}
}

func (uc *EvolutionUsecase) GetEvolutionSuggestions(ctx context.Context, agentID string, status string) ([]EvolutionSuggestion, error) {
	agentID, err := requireNonEmpty(agentID, "EVOLUTION", "agent_id")
	if err != nil {
		return nil, err
	}
	rows, err := uc.store.ListByTargetAndAction(ctx, string(EvolutionTargetAgent), agentID, string(EvolutionActionEvolve), evolutionCallerWorkspace(ctx), status, 1000, 0)
	if err != nil {
		return nil, err
	}
	out := make([]EvolutionSuggestion, 0, len(rows))
	for i := range rows {
		out = append(out, evolutionViewFromUnified(&rows[i]))
	}
	return out, nil
}

// CreateSuggestion persists an L3 suggestion through the unified store (A6).
// Used by non-evolution consumers (spirit team learning, DQ feedback) via the
// EvolutionSuggestionCreator port.
func (uc *EvolutionUsecase) CreateSuggestion(ctx context.Context, s EvolutionSuggestion) (EvolutionSuggestion, error) {
	if strings.TrimSpace(s.ID) == "" {
		s.ID = newAgentCatalogID()
	}
	if s.Status == "" {
		s.Status = EvolutionStatusPending
	}
	if s.CreatedAt == "" {
		s.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	if err := uc.store.Create(ctx, unifiedFromEvolutionView(s)); err != nil {
		return EvolutionSuggestion{}, err
	}
	return s, nil
}

func (uc *EvolutionUsecase) GetSuggestionByID(ctx context.Context, id string) (EvolutionSuggestion, error) {
	id, err := requireNonEmpty(id, "EVOLUTION", "id")
	if err != nil {
		return EvolutionSuggestion{}, err
	}
	s, err := uc.store.GetByID(ctx, id)
	if err != nil {
		return EvolutionSuggestion{}, err
	}
	if s == nil {
		return EvolutionSuggestion{}, apierror.NotFound("EVOLUTION", "suggestion not found")
	}
	return evolutionViewFromUnified(s), nil
}

func (uc *EvolutionUsecase) ApplySuggestion(ctx context.Context, agentID string, suggestionID string) (EvolutionSuggestion, error) {
	agentID = strings.TrimSpace(agentID)
	suggestionID = strings.TrimSpace(suggestionID)
	if agentID == "" || suggestionID == "" {
		return EvolutionSuggestion{}, apierror.BadRequest("EVOLUTION", "agent_id and suggestion_id are required")
	}
	s, err := uc.GetSuggestionByID(ctx, suggestionID)
	if err != nil {
		return EvolutionSuggestion{}, err
	}
	if s.AgentID != agentID {
		return EvolutionSuggestion{}, apierror.NotFound("EVOLUTION", "suggestion not found for this agent")
	}
	if s.Status != EvolutionStatusPending {
		return EvolutionSuggestion{}, apierror.BadRequest("EVOLUTION", "only pending suggestions can be applied")
	}
	// AS-FSM-01: Validate state transition via state machine.
	if _, err := uc.evolutionSM.Transition(ParseEvolutionState(s.Status), EvolutionEventApply); err != nil {
		return EvolutionSuggestion{}, apierror.BadRequest("EVOLUTION", "invalid status transition from "+s.Status+" to applied")
	}
	// P0-2 guard (2026-08-07): persona/prompt apply writes into prompt files,
	// so it requires an explicit apply payload (EvolutionSuggestion.Applicable).
	// All current producers (AgentConfigTrigger metric notifications,
	// orchestration optimization notices) generate notification text as Content
	// and carry no payload — applying them would corrupt IDENTITY.md /
	// AGENTS*.md. Secure by default: legacy rows without the metadata key are
	// rejected, no migration needed.
	payload := strings.TrimSpace(s.ApplyPayload)
	switch s.Type {
	case "persona":
		if !s.Applicable() {
			return EvolutionSuggestion{}, apierror.BadRequest("EVOLUTION", "该建议为指标通知，不包含可应用的修改内容，无法应用")
		}
		files, err := uc.agents.ListAgentPromptFiles(ctx, agentID)
		if err != nil {
			return EvolutionSuggestion{}, err
		}
		// Save pre-apply snapshot for rollback support.
		if snapErr := uc.savePreApplySnapshot(ctx, suggestionID, files); snapErr != nil {
			uc.lg.Warn("failed to save pre-apply snapshot", loggateway.StepID("evolution.apply"), loggateway.Err(snapErr))
		}
		// PGO-1-BIZ-06: Write persona suggestions into IDENTITY.md's ## Persona
		// section instead of the now-deprecated SOUL.md. Fall back to SOUL.md
		// for legacy agents that still carry that file.
		applied := false
		for i, f := range files {
			if f.Name == "IDENTITY.md" {
				files[i].Body = replaceOrAppendPersona(f.Body, payload)
				applied = true
				break
			}
		}
		if !applied {
			for i, f := range files {
				if f.Name == "SOUL.md" {
					files[i].Body = payload
					applied = true
					break
				}
			}
		}
		// If neither file exists, append a new IDENTITY.md.
		if !applied {
			files = append(files, AgentPromptFile{
				AgentID:   agentID,
				Name:      "IDENTITY.md",
				Body:      "# IDENTITY\n\n## Persona\n\n" + payload,
				SortOrder: 30,
			})
		}
		return uc.applyAndMark(ctx, agentID, suggestionID, files)
	case "prompt":
		if !s.Applicable() {
			return EvolutionSuggestion{}, apierror.BadRequest("EVOLUTION", "该建议为指标通知，不包含可应用的修改内容，无法应用")
		}
		files, err := uc.agents.ListAgentPromptFiles(ctx, agentID)
		if err != nil {
			return EvolutionSuggestion{}, err
		}
		// Save pre-apply snapshot for rollback support.
		if snapErr := uc.savePreApplySnapshot(ctx, suggestionID, files); snapErr != nil {
			uc.lg.Warn("failed to save pre-apply snapshot", loggateway.StepID("evolution.apply"), loggateway.Err(snapErr))
		}
		applied := false
		for i, f := range files {
			name := strings.TrimSpace(f.Name)
			if name == "AGENTS_CORE.md" || name == "AGENTS_TASK.md" || strings.HasPrefix(name, "AGENTS") {
				files[i].Body = payload
				applied = true
				break
			}
		}
		if !applied && len(files) > 0 {
			// No AGENTS*.md file found — refuse to apply rather than
			// overwriting an unrelated file (SOUL.md, IDENTITY.md, etc.).
			return EvolutionSuggestion{}, apierror.BadRequest("EVOLUTION", "no AGENTS*.md prompt file found; create one before applying prompt suggestions")
		}
		return uc.applyAndMark(ctx, agentID, suggestionID, files)
	}
	// Unknown suggestion type — nothing to apply.
	return EvolutionSuggestion{}, apierror.BadRequest("EVOLUTION", "unsupported suggestion type: "+s.Type)
}

// applyAndMark replaces the agent's prompt files and marks the suggestion as
// applied. When a txProvider is configured, both writes execute in a single
// transaction so a status-update failure rolls back the prompt files
// (red line #24). Without a txProvider, the writes execute sequentially
// (legacy behavior for tests and offline tooling).
func (uc *EvolutionUsecase) applyAndMark(ctx context.Context, agentID, suggestionID string, files []AgentPromptFile) (EvolutionSuggestion, error) {
	// markApplied transitions pending→applied atomically: a concurrent reject /
	// expire tick winning the race makes the CAS miss, surfacing Conflict
	// instead of silently resurrecting the row to applied (B-1).
	markApplied := func(c context.Context) error {
		ok, err := uc.store.UpdateStatusCAS(c, suggestionID, []string{EvolutionStatusPending}, EvolutionStatusApplied, "", "")
		if err != nil {
			return err
		}
		if !ok {
			return apierror.Conflict("EVOLUTION", "suggestion %s status changed concurrently; retry", suggestionID)
		}
		return nil
	}
	if uc.txProvider != nil {
		err := uc.txProvider.ExecInTx(ctx, func(txCtx context.Context) error {
			if _, err := uc.agents.ReplaceAgentPromptFiles(txCtx, agentID, files); err != nil {
				return err
			}
			return markApplied(txCtx)
		})
		if err != nil {
			return EvolutionSuggestion{}, err
		}
		return uc.GetSuggestionByID(ctx, suggestionID)
	}
	if _, err := uc.agents.ReplaceAgentPromptFiles(ctx, agentID, files); err != nil {
		return EvolutionSuggestion{}, err
	}
	if err := markApplied(ctx); err != nil {
		return EvolutionSuggestion{}, err
	}
	return uc.GetSuggestionByID(ctx, suggestionID)
}

func (uc *EvolutionUsecase) RejectSuggestion(ctx context.Context, agentID string, suggestionID string, reason string) (EvolutionSuggestion, error) {
	agentID = strings.TrimSpace(agentID)
	suggestionID = strings.TrimSpace(suggestionID)
	reason = strings.TrimSpace(reason)
	if agentID == "" || suggestionID == "" {
		return EvolutionSuggestion{}, apierror.BadRequest("EVOLUTION", "agent_id and suggestion_id are required")
	}
	s, err := uc.GetSuggestionByID(ctx, suggestionID)
	if err != nil {
		return EvolutionSuggestion{}, err
	}
	if s.AgentID != agentID {
		return EvolutionSuggestion{}, apierror.NotFound("EVOLUTION", "suggestion not found for this agent")
	}
	if s.Status != EvolutionStatusPending {
		return EvolutionSuggestion{}, apierror.BadRequest("EVOLUTION", "only pending suggestions can be rejected")
	}
	// AS-FSM-01: Validate state transition via state machine.
	if _, err := uc.evolutionSM.Transition(ParseEvolutionState(s.Status), EvolutionEventReject); err != nil {
		return EvolutionSuggestion{}, apierror.BadRequest("EVOLUTION", "invalid status transition from "+s.Status+" to rejected")
	}
	// CAS guard (B-1): a concurrent apply/expire winning the race surfaces as
	// Conflict instead of being overwritten back to rejected.
	ok, err := uc.store.UpdateStatusCAS(ctx, suggestionID, []string{EvolutionStatusPending}, EvolutionStatusRejected, "", reason)
	if err != nil {
		return EvolutionSuggestion{}, err
	}
	if !ok {
		return EvolutionSuggestion{}, apierror.Conflict("EVOLUTION", "suggestion %s status changed concurrently; retry", suggestionID)
	}
	return uc.GetSuggestionByID(ctx, suggestionID)
}

// RollbackSuggestion restores the agent's prompt files to the state captured
// in the pre-apply snapshot and updates the suggestion status to "rolled_back".
func (uc *EvolutionUsecase) RollbackSuggestion(ctx context.Context, agentID string, suggestionID string) (EvolutionSuggestion, error) {
	agentID = strings.TrimSpace(agentID)
	suggestionID = strings.TrimSpace(suggestionID)
	if agentID == "" || suggestionID == "" {
		return EvolutionSuggestion{}, apierror.BadRequest("EVOLUTION", "agent_id and suggestion_id are required")
	}
	s, err := uc.GetSuggestionByID(ctx, suggestionID)
	if err != nil {
		return EvolutionSuggestion{}, err
	}
	if s.AgentID != agentID {
		return EvolutionSuggestion{}, apierror.NotFound("EVOLUTION", "suggestion not found for this agent")
	}
	if s.Status != EvolutionStatusApplied {
		return EvolutionSuggestion{}, apierror.BadRequest("EVOLUTION", "only applied suggestions can be rolled back")
	}
	// AS-FSM-01: Validate state transition via state machine.
	if _, err := uc.evolutionSM.Transition(ParseEvolutionState(s.Status), EvolutionEventRollback); err != nil {
		return EvolutionSuggestion{}, apierror.BadRequest("EVOLUTION", "invalid status transition from "+s.Status+" to rolled_back")
	}
	if s.PreApplySnapshot == "" {
		return EvolutionSuggestion{}, apierror.BadRequest("EVOLUTION", "no pre-apply snapshot available for rollback")
	}
	// Decode the snapshot: map[filename]content
	var snapshot map[string]string
	if err := json.Unmarshal([]byte(s.PreApplySnapshot), &snapshot); err != nil {
		return EvolutionSuggestion{}, apierror.BadRequest("EVOLUTION", "invalid pre-apply snapshot data")
	}
	// Load current files and restore from snapshot.
	files, err := uc.agents.ListAgentPromptFiles(ctx, agentID)
	if err != nil {
		return EvolutionSuggestion{}, err
	}
	for i, f := range files {
		if content, ok := snapshot[f.Name]; ok {
			files[i].Body = content
		}
	}
	// Wrap file replacement + status update in a transaction when a txProvider
	// is configured so a status-update failure rolls back the file restore
	// (red line #24). The status write carries a CAS precondition on 'applied'
	// so a concurrent retry/transition surfaces as Conflict (B-1).
	markRolledBack := func(c context.Context) error {
		ok, err := uc.store.UpdateStatusCAS(c, suggestionID, []string{EvolutionStatusApplied}, EvolutionStatusRolledBack, "", "")
		if err != nil {
			return err
		}
		if !ok {
			return apierror.Conflict("EVOLUTION", "suggestion %s status changed concurrently; retry", suggestionID)
		}
		return nil
	}
	if uc.txProvider != nil {
		err := uc.txProvider.ExecInTx(ctx, func(txCtx context.Context) error {
			if _, err := uc.agents.ReplaceAgentPromptFiles(txCtx, agentID, files); err != nil {
				return err
			}
			return markRolledBack(txCtx)
		})
		if err != nil {
			return EvolutionSuggestion{}, err
		}
		return uc.GetSuggestionByID(ctx, suggestionID)
	}
	if _, err := uc.agents.ReplaceAgentPromptFiles(ctx, agentID, files); err != nil {
		return EvolutionSuggestion{}, err
	}
	if err := markRolledBack(ctx); err != nil {
		return EvolutionSuggestion{}, err
	}
	return uc.GetSuggestionByID(ctx, suggestionID)
}

// savePreApplySnapshot captures the current content of all prompt files as a
// JSON-encoded map[filename]content and persists it into the unified row's
// metadata (EvoMetaPreApplySnapshot) for later rollback.
func (uc *EvolutionUsecase) savePreApplySnapshot(ctx context.Context, suggestionID string, files []AgentPromptFile) error {
	snapshot := make(map[string]string, len(files))
	for _, f := range files {
		snapshot[f.Name] = f.Body
	}
	data, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	return uc.store.UpdateMetadataKey(ctx, suggestionID, EvoMetaPreApplySnapshot, string(data))
}

// replaceOrAppendPersona writes personaContent into the "## Persona" section
// of an IDENTITY.md body. If the section exists it replaces everything from
// the ## Persona heading to the next same-level heading (or EOF). If the
// section does not exist it is appended. PGO-1-BIZ-06.
func replaceOrAppendPersona(body, personaContent string) string {
	const anchor = "## Persona"
	idx := strings.Index(body, anchor)
	if idx == -1 {
		// No ## Persona section — append it.
		trimmed := strings.TrimRight(body, "\n ")
		return trimmed + "\n\n" + anchor + "\n\n" + strings.TrimSpace(personaContent)
	}
	// Find the end of the ## Persona section: next "## " heading or EOF.
	after := body[idx+len(anchor):]
	nextH2 := strings.Index(after, "\n## ")
	var tail string
	if nextH2 == -1 {
		tail = ""
	} else {
		tail = after[nextH2:]
	}
	prefix := strings.TrimRight(body[:idx], "\n ")
	return prefix + "\n\n" + anchor + "\n\n" + strings.TrimSpace(personaContent) + tail
}

func timeRangeToSince(tr string) time.Time {
	now := time.Now()
	switch tr {
	case "7d":
		return now.AddDate(0, 0, -7)
	case "30d":
		return now.AddDate(0, 0, -30)
	case "90d":
		return now.AddDate(0, 0, -90)
	default:
		return now.AddDate(0, 0, -30)
	}
}
