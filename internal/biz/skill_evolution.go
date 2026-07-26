package biz

import (
	"context"
	"crypto/sha256"
	stderrors "errors"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/biz/shared"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

const skillPatternMinConfidence = 0.15

type SkillAutoCreator interface {
	GenerateSKILLMD(ctx context.Context, patternDesc string, toolHistory []ToolCallRecord) (name string, content string, err error)
}

type SkillRegistrationPort interface {
	RegisterSkill(ctx context.Context, agentID string, name string, skillMD string) error
	SkillExists(ctx context.Context, agentID string, name string) (bool, error)
}

type SkillEvolutionUsecase struct {
	store          UnifiedEvolutionStore
	patternReader  UnifiedEvolutionPatternReader
	patterns       PatternReader
	agents         AgentRepository
	creator        SkillAutoCreator
	registrar      SkillRegistrationPort
	orchestrator   *SkillEvolutionOrchestrator
	orchestratorOnce sync.Once
	proposalSM     *SkillProposalStateMachine
	lg             loggateway.Logger
}

// NewSkillEvolutionUsecase constructs the L1 skill-creation proposal usecase.
// After the A6 convergence the physical storage is unified_evolution_suggestions
// (agent + create_skill rows); SkillProposal is a view reconstructed by
// skillProposalFromUnified, with legacy-only fields in metadata.
func NewSkillEvolutionUsecase(
	store UnifiedEvolutionStore,
	patternReader UnifiedEvolutionPatternReader,
	patterns PatternReader,
	agents AgentRepository,
	creator SkillAutoCreator,
	registrar SkillRegistrationPort,
	lg loggateway.Logger,
) *SkillEvolutionUsecase {
	return &SkillEvolutionUsecase{
		store:         store,
		patternReader: patternReader,
		patterns:      patterns,
		agents:        agents,
		creator:       creator,
		registrar:     registrar,
		proposalSM:    NewSkillProposalStateMachine(),
		lg:            lg,
	}
}

// ── View conversion: SkillProposal ↔ UnifiedEvolutionSuggestion ──────────────
//
// Mirrors the 20261111 backfill mapping: target_type=agent, action_type=
// create_skill, trigger_source=pattern, pattern_hash/pattern_desc/approved_at/
// rejected_by in metadata. The legacy status 'registered' is stored verbatim
// (it is not a unified state machine state).

// skillProposalFromUnified reconstructs the legacy L1 view from a unified row.
func skillProposalFromUnified(s *UnifiedEvolutionSuggestion) SkillProposal {
	if s == nil {
		return SkillProposal{}
	}
	var approvedAt *time.Time
	if raw := s.MetaString(EvoMetaApprovedAt); raw != "" {
		if t, err := time.Parse(time.RFC3339, raw); err == nil {
			approvedAt = &t
		}
	}
	return SkillProposal{
		ID:          s.ID,
		AgentID:     s.TargetID,
		PatternHash: s.MetaString(EvoMetaPatternHash),
		PatternDesc: s.MetaString(EvoMetaPatternDesc),
		SkillName:   s.DraftName,
		SkillMD:     s.DraftBody,
		Status:      SkillProposalStatus(s.Status),
		ApprovedBy:  s.ApprovedBy,
		RejectedBy:  s.MetaString(EvoMetaRejectedBy),
		CreatedAt:   s.CreatedAt,
		ApprovedAt:  approvedAt,
	}
}

// unifiedFromSkillProposal converts a legacy L1 view into a unified row for
// creation.
func unifiedFromSkillProposal(p SkillProposal) UnifiedEvolutionSuggestion {
	approvedAt := ""
	if p.ApprovedAt != nil {
		approvedAt = p.ApprovedAt.UTC().Format(time.RFC3339)
	}
	metadata, _ := json.Marshal(map[string]string{
		EvoMetaPatternHash: p.PatternHash,
		EvoMetaPatternDesc: p.PatternDesc,
		EvoMetaApprovedAt:  approvedAt,
		EvoMetaRejectedBy:  p.RejectedBy,
	})
	return UnifiedEvolutionSuggestion{
		ID:              p.ID,
		TargetType:      EvolutionTargetAgent,
		TargetID:        p.AgentID,
		ActionType:      EvolutionActionCreate,
		TriggerSource:   "pattern",
		TriggerReason:   p.PatternDesc,
		Status:          string(p.Status),
		Priority:        1,
		DraftBody:       p.SkillMD,
		DraftName:       p.SkillName,
		LifecycleStatus: "draft",
		Metadata:        metadata,
		CreatedAt:       p.CreatedAt,
		ApprovedBy:      p.ApprovedBy,
	}
}

// getProposalView fetches a unified row by ID and converts it to the L1 view,
// returning NotFound when absent (mirrors the legacy repo semantics).
func (uc *SkillEvolutionUsecase) getProposalView(ctx context.Context, id string) (SkillProposal, error) {
	s, err := uc.store.GetByID(ctx, id)
	if err != nil {
		return SkillProposal{}, err
	}
	if s == nil {
		return SkillProposal{}, apierror.NotFound("SKILL_EVO", "proposal not found")
	}
	return skillProposalFromUnified(s), nil
}

// SetOrchestrator sets the unified evolution orchestrator for cross-pipeline dedup.
// When set, DetectAndPropose delegates to the orchestrator for pending checks.
// Protected by sync.Once to prevent concurrent initialization races.
func (uc *SkillEvolutionUsecase) SetOrchestrator(o *SkillEvolutionOrchestrator) {
	uc.orchestratorOnce.Do(func() {
		uc.orchestrator = o
	})
}

func (uc *SkillEvolutionUsecase) DetectAndPropose(ctx context.Context, agentID string) ([]SkillProposal, error) {
	agentID, err := requireNonEmpty(agentID, "SKILL_EVO", "agent_id")
	if err != nil {
		return nil, err
	}
	// Fail-closed: the AgentRepository is required to validate that the target
	// agent exists before spending LLM calls or writing proposals. A nil
	// dependency indicates misconfiguration.
	if uc.agents == nil {
		uc.lg.Error("agent repository not configured, refusing to detect skill evolution",
			loggateway.StepID("skill_evo.detect"),
			loggateway.Str("agent_id", agentID))
		return nil, apierror.Internal("SKILL_EVO", "agent repository not configured; cannot validate agent existence")
	}
	// REL-2: validate agent existence. Without this, a non-existent agentID
	// would silently produce zero proposals, masking caller errors.
	if _, err := uc.agents.GetAgentByID(ctx, agentID); err != nil {
		if stderrors.Is(err, shared.ErrNotFound) {
			return nil, apierror.BadRequest("SKILL_EVO", "agent not found: %s", agentID)
		}
		return nil, err
	}
	if uc.creator == nil {
		uc.lg.Warn("skill auto creator not configured, skill auto-creation disabled", loggateway.StepID("skill_evo.detect"))
		return nil, nil
	}

	// Cross-pipeline dedup: skip if another pipeline already has a pending
	// suggestion for this agent. Prefer orchestrator over legacy coordinator.
	if uc.orchestrator != nil {
		hasPending, err := uc.orchestrator.HasPendingForTarget(ctx, "agent", agentID)
		if err == nil && hasPending {
			uc.lg.Debug("DetectAndPropose: skipped, pending evolution already exists via orchestrator",
				loggateway.StepID("skill_evo.detect"),
				loggateway.Str("agent_id", agentID))
			return nil, nil
		}
	}

	patterns, err := uc.findSkillPatterns(ctx, agentID)
	if err != nil {
		return nil, err
	}
	if len(patterns) == 0 {
		return nil, nil
	}
	var proposals []SkillProposal
	for _, p := range patterns {
		hash := patternHash(p.Description)
		existing, exErr := uc.patternReader.GetLatestByPatternHash(ctx, agentID, hash)
		if exErr != nil {
			uc.lg.Warn("check existing proposal", loggateway.StepID("skill_evo.detect"), loggateway.Err(exErr))
			continue
		}
		if existing != nil {
			continue
		}
		// Check if a skill with the same name already exists before
		// spending an LLM call to generate SKILL.md.
		toolHistory := uc.extractToolHistory(p)
		suggestedName := uc.inferSkillName(p.Description)
		if suggestedName != "" && uc.registrar != nil {
			exists, existErr := uc.registrar.SkillExists(ctx, agentID, suggestedName)
			if existErr != nil {
				uc.lg.Warn("check skill existence before generation", loggateway.StepID("skill_evo.detect"), loggateway.Err(existErr))
			} else if exists {
				uc.lg.Info("skill already exists, skipping LLM generation",
					loggateway.StepID("skill_evo.detect"),
					loggateway.Str("agent_id", agentID),
					loggateway.Str("name", suggestedName))
				continue
			}
		}
		name, content, genErr := uc.creator.GenerateSKILLMD(ctx, p.Description, toolHistory)
		if genErr != nil {
			uc.lg.Warn("generate SKILL.md", loggateway.StepID("skill_evo.detect"), loggateway.Err(genErr))
			continue
		}
		proposal := SkillProposal{
			ID:          newAgentCatalogID(),
			AgentID:     agentID,
			PatternHash: hash,
			PatternDesc: p.Description,
			SkillName:   name,
			SkillMD:     content,
			Status:      SkillProposalStatusPending,
			CreatedAt:   time.Now().UTC(),
		}
		// DB unique index on (target, action, pattern_hash) for pending rows
		// backstops the check-then-create race; duplicates warn and skip.
		if cErr := uc.store.Create(ctx, unifiedFromSkillProposal(proposal)); cErr != nil {
			uc.lg.Warn("create skill proposal", loggateway.StepID("skill_evo.detect"), loggateway.Err(cErr))
			continue
		}
		proposals = append(proposals, proposal)
	}
	return proposals, nil
}

func (uc *SkillEvolutionUsecase) ApproveProposal(ctx context.Context, id string, approvedBy string) (SkillProposal, error) {
	id, err := requireNonEmpty(id, "SKILL_EVO", "id")
	if err != nil {
		return SkillProposal{}, err
	}
	p, err := uc.getProposalView(ctx, id)
	if err != nil {
		return SkillProposal{}, err
	}
	next, terr := uc.transitionProposal(p.Status, SkillProposalEventApprove)
	if terr != nil {
		return SkillProposal{}, apierror.BadRequest("SKILL_EVO", "only pending proposals can be approved")
	}
	// UpdateStatus merges metadata.approved_at for the view layer (A6).
	if err := uc.store.UpdateStatus(ctx, id, string(next), approvedBy, ""); err != nil {
		return SkillProposal{}, err
	}
	return uc.getProposalView(ctx, id)
}

func (uc *SkillEvolutionUsecase) RejectProposal(ctx context.Context, id string, rejectedBy string) (SkillProposal, error) {
	id, err := requireNonEmpty(id, "SKILL_EVO", "id")
	if err != nil {
		return SkillProposal{}, err
	}
	p, err := uc.getProposalView(ctx, id)
	if err != nil {
		return SkillProposal{}, err
	}
	next, terr := uc.transitionProposal(p.Status, SkillProposalEventReject)
	if terr != nil {
		return SkillProposal{}, apierror.BadRequest("SKILL_EVO", "only pending proposals can be rejected")
	}
	// UpdateStatus merges metadata.rejected_by for the view layer (A6).
	if err := uc.store.UpdateStatus(ctx, id, string(next), rejectedBy, ""); err != nil {
		return SkillProposal{}, err
	}
	return uc.getProposalView(ctx, id)
}

func (uc *SkillEvolutionUsecase) RegisterApproved(ctx context.Context, id string) (SkillProposal, error) {
	id, err := requireNonEmpty(id, "SKILL_EVO", "id")
	if err != nil {
		return SkillProposal{}, err
	}
	p, err := uc.getProposalView(ctx, id)
	if err != nil {
		return SkillProposal{}, err
	}
	next, terr := uc.transitionProposal(p.Status, SkillProposalEventRegister)
	if terr != nil {
		return SkillProposal{}, apierror.BadRequest("SKILL_EVO", "only approved proposals can be registered")
	}
	if uc.registrar == nil {
		uc.lg.Warn("skill registrar not configured, registration skipped", loggateway.StepID("skill_evo.register"))
		return SkillProposal{}, nil
	}
	if strings.TrimSpace(p.SkillMD) == "" {
		return SkillProposal{}, apierror.BadRequest("SKILL_EVO", "cannot register proposal with empty skill content")
	}
	exists, exErr := uc.registrar.SkillExists(ctx, p.AgentID, p.SkillName)
	if exErr != nil {
		return SkillProposal{}, exErr
	}
	if exists {
		return SkillProposal{}, apierror.Conflict("SKILL_EVO", "skill %q already exists for agent %s", p.SkillName, p.AgentID)
	}
	if regErr := uc.registrar.RegisterSkill(ctx, p.AgentID, p.SkillName, p.SkillMD); regErr != nil {
		return SkillProposal{}, regErr
	}
	// 'registered' is an L1 view status stored verbatim (not a unified SM state);
	// the repo performs a plain status update for it.
	if err := uc.store.UpdateStatus(ctx, id, string(next), "", ""); err != nil {
		return SkillProposal{}, err
	}
	return uc.getProposalView(ctx, id)
}

func (uc *SkillEvolutionUsecase) transitionProposal(from SkillProposalStatus, event SkillProposalEvent) (SkillProposalStatus, error) {
	sm := uc.proposalSM
	if sm == nil {
		sm = NewSkillProposalStateMachine()
	}
	return sm.Transition(from, event)
}

func (uc *SkillEvolutionUsecase) GetProposal(ctx context.Context, id string) (SkillProposal, error) {
	id, err := requireNonEmpty(id, "SKILL_EVO", "id")
	if err != nil {
		return SkillProposal{}, err
	}
	return uc.getProposalView(ctx, id)
}

func (uc *SkillEvolutionUsecase) ListProposals(ctx context.Context, agentID string, status string, limit int, offset int) ([]SkillProposal, error) {
	rows, err := uc.store.ListByTargetAndAction(ctx, string(EvolutionTargetAgent), agentID, string(EvolutionActionCreate), status, limit, offset)
	if err != nil {
		return nil, err
	}
	out := make([]SkillProposal, 0, len(rows))
	for i := range rows {
		out = append(out, skillProposalFromUnified(&rows[i]))
	}
	return out, nil
}

// CountProposals returns the total number of proposals matching the filter,
// for pagination metadata. Mirrors the ListProposals filter semantics.
func (uc *SkillEvolutionUsecase) CountProposals(ctx context.Context, agentID string, status string) (int, error) {
	return uc.store.CountByTargetAndAction(ctx, string(EvolutionTargetAgent), agentID, string(EvolutionActionCreate), status)
}

func (uc *SkillEvolutionUsecase) CreateProposal(ctx context.Context, proposal SkillProposal) (SkillProposal, error) {
	if proposal.ID == "" {
		proposal.ID = newAgentCatalogID()
	}
	if proposal.Status == "" {
		proposal.Status = SkillProposalStatusPending
	}
	if proposal.CreatedAt.IsZero() {
		proposal.CreatedAt = time.Now().UTC()
	}
	if proposal.PatternHash == "" {
		proposal.PatternHash = patternHash(proposal.PatternDesc)
	}
	if err := uc.store.Create(ctx, unifiedFromSkillProposal(proposal)); err != nil {
		return SkillProposal{}, err
	}
	return proposal, nil
}

// ── Helpers ──────────────────────────────────────────────────────────────────

func (uc *SkillEvolutionUsecase) findSkillPatterns(ctx context.Context, agentID string) ([]Pattern, error) {
	if uc.patterns == nil {
		return nil, nil
	}
	all, err := uc.patterns.ListByAgent(ctx, agentID, string(PatternStatusDetected))
	if err != nil {
		return nil, err
	}
	var skillPatterns []Pattern
	for _, p := range all {
		if p.Kind == string(ObservationKindToolCall) && p.Confidence >= skillPatternMinConfidence {
			skillPatterns = append(skillPatterns, p)
		}
	}
	return skillPatterns, nil
}

// extractToolHistory delegates to the package-level extractToolHistoryFromPattern.
func (uc *SkillEvolutionUsecase) extractToolHistory(p Pattern) []ToolCallRecord {
	return extractToolHistoryFromPattern(p)
}

// inferSkillName delegates to the package-level inferSkillNameFromDesc.
func (uc *SkillEvolutionUsecase) inferSkillName(desc string) string {
	return inferSkillNameFromDesc(desc)
}

func patternHash(desc string) string {
	h := sha256.Sum256([]byte(strings.TrimSpace(strings.ToLower(desc))))
	return fmt.Sprintf("%x", h[:8])
}

func extractToolNamesFromDesc(desc string) []string {
	var names []string
	parts := strings.Split(desc, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if idx := strings.Index(part, "("); idx > 0 {
			name := strings.TrimSpace(part[:idx])
			if name != "" {
				names = append(names, name)
			}
		}
	}
	return names
}
