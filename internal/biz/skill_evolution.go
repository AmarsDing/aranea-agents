package biz

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"sync"
	"time"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

const (
	defaultScanAgentLimit   = 500
	skillPatternMinConfidence = 0.15
)

type SkillAutoCreator interface {
	GenerateSKILLMD(ctx context.Context, patternDesc string, toolHistory []ToolCallRecord) (name string, content string, err error)
}

type SkillRegistrationPort interface {
	RegisterSkill(ctx context.Context, agentID string, name string, skillMD string) error
	SkillExists(ctx context.Context, agentID string, name string) (bool, error)
}

type SkillEvolutionUsecase struct {
	repo          SkillProposalReadWriter
	patterns      PatternReader
	agents        AgentRepository
	creator       SkillAutoCreator
	registrar     SkillRegistrationPort
	coordinator   *EvolutionCoordinator
	coordinatorOnce sync.Once
	lg            loggateway.Logger
}

func NewSkillEvolutionUsecase(
	repo SkillProposalReadWriter,
	patterns PatternReader,
	agents AgentRepository,
	creator SkillAutoCreator,
	registrar SkillRegistrationPort,
	lg loggateway.Logger,
) *SkillEvolutionUsecase {
	return &SkillEvolutionUsecase{
		repo:      repo,
		patterns:  patterns,
		agents:    agents,
		creator:   creator,
		registrar: registrar,
		lg:        lg,
	}
}

// SetCoordinator sets the evolution coordinator for cross-pipeline dedup.
// Must only be called once during initialization. Panics on repeated calls.
func (uc *SkillEvolutionUsecase) SetCoordinator(c *EvolutionCoordinator) {
	uc.coordinatorOnce.Do(func() {
		uc.coordinator = c
	})
	if uc.coordinator != c {
		panic("SkillEvolutionUsecase: SetCoordinator called more than once")
	}
}

func (uc *SkillEvolutionUsecase) DetectAndPropose(ctx context.Context, agentID string) ([]SkillProposal, error) {
	agentID, err := requireNonEmpty(agentID, "SKILL_EVO", "agent_id")
	if err != nil {
		return nil, err
	}
	if uc.creator == nil {
		uc.lg.Warn("skill auto creator not configured, skill auto-creation disabled", loggateway.StepID("skill_evo.detect"))
		return nil, nil
	}

	// Cross-pipeline dedup: skip if another pipeline already has a pending
	// suggestion for this agent.
	if uc.coordinator != nil && uc.coordinator.HasPendingEvolution(ctx, EvolutionTarget{Type: "agent", ID: agentID}) {
		uc.lg.Debug("DetectAndPropose: skipped, pending evolution already exists via another pipeline",
			loggateway.StepID("skill_evo.detect"),
			loggateway.Str("agent_id", agentID))
		return nil, nil
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
		existing, exErr := uc.repo.GetByPatternHash(ctx, agentID, hash)
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
		if suggestedName != "" {
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
		created, cErr := uc.repo.Create(ctx, proposal)
		if cErr != nil {
			uc.lg.Warn("create skill proposal", loggateway.StepID("skill_evo.detect"), loggateway.Err(cErr))
			continue
		}
		proposals = append(proposals, created)
	}
	return proposals, nil
}

func (uc *SkillEvolutionUsecase) ApproveProposal(ctx context.Context, id string, approvedBy string) (SkillProposal, error) {
	id, err := requireNonEmpty(id, "SKILL_EVO", "id")
	if err != nil {
		return SkillProposal{}, err
	}
	p, err := uc.repo.GetByID(ctx, id)
	if err != nil {
		return SkillProposal{}, err
	}
	if p.Status != SkillProposalStatusPending {
		return SkillProposal{}, apierror.BadRequest("SKILL_EVO", "only pending proposals can be approved")
	}
	return uc.repo.UpdateStatus(ctx, id, SkillProposalStatusApproved, approvedBy)
}

func (uc *SkillEvolutionUsecase) RejectProposal(ctx context.Context, id string, rejectedBy string) (SkillProposal, error) {
	id, err := requireNonEmpty(id, "SKILL_EVO", "id")
	if err != nil {
		return SkillProposal{}, err
	}
	p, err := uc.repo.GetByID(ctx, id)
	if err != nil {
		return SkillProposal{}, err
	}
	if p.Status != SkillProposalStatusPending {
		return SkillProposal{}, apierror.BadRequest("SKILL_EVO", "only pending proposals can be rejected")
	}
	return uc.repo.UpdateStatus(ctx, id, SkillProposalStatusRejected, rejectedBy)
}

func (uc *SkillEvolutionUsecase) RegisterApproved(ctx context.Context, id string) (SkillProposal, error) {
	id, err := requireNonEmpty(id, "SKILL_EVO", "id")
	if err != nil {
		return SkillProposal{}, err
	}
	p, err := uc.repo.GetByID(ctx, id)
	if err != nil {
		return SkillProposal{}, err
	}
	if p.Status != SkillProposalStatusApproved {
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
	return uc.repo.UpdateStatus(ctx, id, SkillProposalStatusRegistered, "")
}

func (uc *SkillEvolutionUsecase) GetProposal(ctx context.Context, id string) (SkillProposal, error) {
	id, err := requireNonEmpty(id, "SKILL_EVO", "id")
	if err != nil {
		return SkillProposal{}, err
	}
	return uc.repo.GetByID(ctx, id)
}

func (uc *SkillEvolutionUsecase) ListProposals(ctx context.Context, agentID string, status string, limit int, offset int) ([]SkillProposal, error) {
	agentID, err := requireNonEmpty(agentID, "SKILL_EVO", "agent_id")
	if err != nil {
		return nil, err
	}
	return uc.repo.ListByAgent(ctx, agentID, status, limit, offset)
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
	return uc.repo.Create(ctx, proposal)
}

func (uc *SkillEvolutionUsecase) ScanAndProposeAll(ctx context.Context) error {
	if uc.agents == nil {
		return nil
	}
	var errs []error
	offset := 0
	for {
		select {
		case <-ctx.Done():
			if len(errs) > 0 {
				return apierror.Internal("SKILL_EVO", "skill evolution: %d agents failed (cancelled)", len(errs))
			}
			return ctx.Err()
		default:
		}
		page, err := uc.agents.SearchAgents(ctx, AgentListQuery{Limit: defaultScanAgentLimit, Offset: offset, Status: string(AgentStatusActive)})
		if err != nil {
			return err
		}
		for _, a := range page.Items {
			settings, serr := uc.agents.GetAgentRuntimeSettings(ctx, a.ID)
			if serr != nil {
				continue
			}
			if !settings.EvolutionSkillEvolve {
				continue
			}
			if _, dErr := uc.DetectAndPropose(ctx, a.ID); dErr != nil {
				uc.lg.Warn("skill evolution detect", loggateway.StepID("skill_evo.scan"), loggateway.Str("agent_id", a.ID), loggateway.Err(dErr))
				errs = append(errs, dErr)
			}
		}
		if len(page.Items) < defaultScanAgentLimit {
			break
		}
		offset += defaultScanAgentLimit
	}
	if len(errs) > 0 {
		return apierror.Internal("SKILL_EVO", "skill evolution: %d agents failed", len(errs))
	}
	return nil
}

// ── Bridge: SkillProposal → SkillEvolutionSuggestion ─────────────────────────
//
// TODO(debt): DEV-04 — Transitional bridge function. Will be removed once
// SkillProposal is deprecated in favor of SkillEvolutionSuggestion.

// SuggestionFromProposal converts a SkillProposal into a
// SkillEvolutionSuggestion for interoperability with the SkillIntelligenceUsecase pipeline.
// Fields that have no direct equivalent are left at their zero values.
func (uc *SkillEvolutionUsecase) SuggestionFromProposal(p SkillProposal) SkillEvolutionSuggestion {
	return SkillEvolutionSuggestion{
		ID:              p.ID,
		SkillID:         "",                       // no equivalent; SkillProposal is agent-scoped
		Type:            EvoSuggestionCreateSkill,  // SkillProposal is agent-scoped skill creation
		Status:          ProposalStatusToSuggestion(p.Status),
		TriggerReason:   p.PatternDesc,
		DraftSkillBody:  p.SkillMD,
		ApprovedBy:      p.ApprovedBy,
		RejectedBy:      p.RejectedBy,
		CreatedAt:       p.CreatedAt,
		ResolvedAt:      p.ApprovedAt,
	}
}

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

func (uc *SkillEvolutionUsecase) extractToolHistory(p Pattern) []ToolCallRecord {
	var records []ToolCallRecord
	toolNames := extractToolNamesFromDesc(p.Description)
	for _, name := range toolNames {
		records = append(records, ToolCallRecord{
			ToolName: name,
			Success:  p.Confidence >= skillPatternMinConfidence, // use pattern confidence as proxy
			CalledAt: p.DetectedAt,
		})
	}
	return records
}

// inferSkillName attempts to derive a concise skill name from the pattern
// description (e.g. "web_search(query)" → "web_search").
func (uc *SkillEvolutionUsecase) inferSkillName(desc string) string {
	parts := strings.Split(desc, ",")
	if len(parts) == 0 {
		return ""
	}
	first := strings.TrimSpace(parts[0])
	if idx := strings.Index(first, "("); idx > 0 {
		return strings.TrimSpace(first[:idx])
	}
	return ""
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
