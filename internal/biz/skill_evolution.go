package biz

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"time"

	kerrors "github.com/go-kratos/kratos/v2/errors"

	"aranea-agents/pkg/loggateway"
)

type SkillAutoCreator interface {
	GenerateSKILLMD(ctx context.Context, patternDesc string, toolHistory []ToolCallRecord) (name string, content string, err error)
}

type SkillRegistrationPort interface {
	RegisterSkill(ctx context.Context, agentID string, name string, skillMD string) error
	SkillExists(ctx context.Context, agentID string, name string) (bool, error)
}

type SkillEvolutionUsecase struct {
	repo      SkillProposalReadWriter
	patterns  PatternReader
	agents    AgentRepository
	creator   SkillAutoCreator
	registrar SkillRegistrationPort
	lg        loggateway.Logger
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

func (uc *SkillEvolutionUsecase) DetectAndPropose(ctx context.Context, agentID string) ([]SkillProposal, error) {
	agentID, err := requireNonEmpty(agentID, "SKILL_EVO", "agent_id")
	if err != nil {
		return nil, err
	}
	if uc.creator == nil {
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
		toolHistory := uc.extractToolHistory(p)
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
		return SkillProposal{}, kerrors.BadRequest("SKILL_EVO", "only pending proposals can be approved")
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
		return SkillProposal{}, kerrors.BadRequest("SKILL_EVO", "only pending proposals can be rejected")
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
		return SkillProposal{}, kerrors.BadRequest("SKILL_EVO", "only approved proposals can be registered")
	}
	if uc.registrar == nil {
		return SkillProposal{}, kerrors.InternalServer("SKILL_EVO", "skill registrar is not configured")
	}
	exists, exErr := uc.registrar.SkillExists(ctx, p.AgentID, p.SkillName)
	if exErr != nil {
		return SkillProposal{}, exErr
	}
	if exists {
		return SkillProposal{}, kerrors.Conflict("SKILL_EVO", fmt.Sprintf("skill %q already exists for agent %s", p.SkillName, p.AgentID))
	}
	if regErr := uc.registrar.RegisterSkill(ctx, p.AgentID, p.SkillName, p.SkillMD); regErr != nil {
		return SkillProposal{}, regErr
	}
	return uc.repo.UpdateStatus(ctx, id, SkillProposalStatusRegistered, "")
}

func (uc *SkillEvolutionUsecase) ListProposals(ctx context.Context, agentID string, status string) ([]SkillProposal, error) {
	agentID, err := requireNonEmpty(agentID, "SKILL_EVO", "agent_id")
	if err != nil {
		return nil, err
	}
	return uc.repo.ListByAgent(ctx, agentID, status)
}

func (uc *SkillEvolutionUsecase) ScanAndProposeAll(ctx context.Context) error {
	if uc.agents == nil {
		return nil
	}
	page, err := uc.agents.SearchAgents(ctx, AgentListQuery{Limit: 500, Offset: 0, Status: "active"})
	if err != nil {
		return err
	}
	var errs []error
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
	if len(errs) > 0 {
		return kerrors.InternalServer("SKILL_EVO", fmt.Sprintf("skill evolution: %d agents failed", len(errs)))
	}
	return nil
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
		if p.Kind == string(ObservationKindToolCall) && p.Confidence >= 0.15 {
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
			Success:  true,
			CalledAt: p.DetectedAt,
		})
	}
	return records
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
