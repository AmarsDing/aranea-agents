package biz

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"aranea-agents/pkg/apierror"
)

type ObservationReadWriter interface {
	ObservationReader
	ObservationWriter
}

type PatternReadWriter interface {
	PatternReader
	PatternWriter
}

type ProposalReadWriter interface {
	ProposalReader
	ProposalWriter
}

const (
	ApprovedBySystem = "system"
	defaultObservationLookbackDays = 30
)

func FormatApprovedByUser(userID int64) string {
	return fmt.Sprintf("user:%d", userID)
}

type LearningLoopUsecase struct {
	obs       ObservationReadWriter
	patterns  PatternReadWriter
	proposals ProposalReadWriter
	agents    AgentRepository
	evolution *EvolutionUsecase
}

func NewLearningLoopUsecase(
	obs ObservationReadWriter,
	pat PatternReadWriter,
	prop ProposalReadWriter,
	agents AgentRepository,
	evolution *EvolutionUsecase,
) *LearningLoopUsecase {
	return &LearningLoopUsecase{
		obs:       obs,
		patterns:  pat,
		proposals: prop,
		agents:    agents,
		evolution: evolution,
	}
}

func (uc *LearningLoopUsecase) CollectObservations(ctx context.Context, agentID string, since time.Time) ([]Observation, error) {
	agentID, err := requireNonEmpty(agentID, "LEARNING", "agent_id")
	if err != nil {
		return nil, err
	}
	return uc.obs.ListByAgent(ctx, agentID, since)
}

func (uc *LearningLoopUsecase) ListObservations(ctx context.Context, agentID string, sinceRFC3339 string) ([]Observation, error) {
	agentID, err := requireNonEmpty(agentID, "LEARNING", "agent_id")
	if err != nil {
		return nil, err
	}
	since := time.Now().UTC().AddDate(0, 0, -defaultObservationLookbackDays)
	if sinceRFC3339 != "" {
		if t, pErr := time.Parse(time.RFC3339, sinceRFC3339); pErr == nil {
			since = t
		}
	}
	return uc.obs.ListByAgent(ctx, agentID, since)
}

func (uc *LearningLoopUsecase) RecordObservation(ctx context.Context, obs Observation) (Observation, error) {
	if obs.ID == "" {
		obs.ID = newAgentCatalogID()
	}
	if obs.ObservedAt.IsZero() {
		obs.ObservedAt = time.Now().UTC()
	}
	return uc.obs.Create(ctx, obs)
}

func (uc *LearningLoopUsecase) RecordObservations(ctx context.Context, obs []Observation) error {
	for i := range obs {
		if obs[i].ID == "" {
			obs[i].ID = newAgentCatalogID()
		}
		if obs[i].ObservedAt.IsZero() {
			obs[i].ObservedAt = time.Now().UTC()
		}
	}
	return uc.obs.BatchCreate(ctx, obs)
}

func (uc *LearningLoopUsecase) DetectPatterns(ctx context.Context, agentID string) ([]Pattern, error) {
	agentID, err := requireNonEmpty(agentID, "LEARNING", "agent_id")
	if err != nil {
		return nil, err
	}
	since := time.Now().UTC().AddDate(0, 0, -30)
	observations, err := uc.obs.ListByAgent(ctx, agentID, since)
	if err != nil {
		return nil, err
	}
	if len(observations) < 3 {
		return nil, nil
	}
	buckets := make(map[string][]Observation)
	for _, o := range observations {
		key := patternBucketKey(o)
		buckets[key] = append(buckets[key], o)
	}
	var patterns []Pattern
	for key, obs := range buckets {
		if len(obs) < 3 {
			continue
		}
		confidence := float64(len(obs)) / float64(len(observations))
		if confidence < 0.1 {
			continue
		}
		existing, exErr := uc.patterns.ListByAgent(ctx, agentID, string(PatternStatusDetected))
		if exErr != nil {
			continue
		}
		dup := false
		for _, ep := range existing {
			if ep.Kind == key && ep.Description == describeBucket(key, obs) {
				dup = true
				break
			}
		}
		if dup {
			continue
		}
		evidence := make([]string, 0, len(obs))
		for _, o := range obs {
			evidence = append(evidence, o.ID)
		}
		evidenceJSON, merr := json.Marshal(evidence)
		if merr != nil {
			return nil, apierror.Internal("LEARNING", "marshal evidence: %s", merr)
		}
		p := Pattern{
			ID:          newAgentCatalogID(),
			AgentID:     agentID,
			Kind:        key,
			Description: describeBucket(key, obs),
			Frequency:   len(obs),
			Confidence:  confidence,
			Evidence:    string(evidenceJSON),
			Status:      PatternStatusDetected,
			DetectedAt:  time.Now().UTC(),
		}
		created, err := uc.patterns.Create(ctx, p)
		if err != nil {
			return nil, err
		}
		patterns = append(patterns, created)
	}
	return patterns, nil
}

func (uc *LearningLoopUsecase) GenerateProposals(ctx context.Context, agentID string, patterns []Pattern) ([]KnowledgeProposal, error) {
	agentID, err := requireNonEmpty(agentID, "LEARNING", "agent_id")
	if err != nil {
		return nil, err
	}
	if len(patterns) == 0 {
		return nil, nil
	}
	var proposals []KnowledgeProposal
	for _, p := range patterns {
		if p.Confidence < 0.15 {
			continue
		}
		title := proposalTitle(p)
		content := proposalContent(p)
		kind := proposalKind(p.Kind)
		now := time.Now().UTC()
		prop := KnowledgeProposal{
			ID:        newAgentCatalogID(),
			AgentID:   agentID,
			PatternID: p.ID,
			Title:     title,
			Content:   content,
			Kind:      kind,
			Status:    ProposalStatusDraft,
			CreatedAt: now,
			UpdatedAt: now,
		}
		created, err := uc.proposals.Create(ctx, prop)
		if err != nil {
			return nil, err
		}
		proposals = append(proposals, created)
	}
	return proposals, nil
}

func (uc *LearningLoopUsecase) ValidateProposal(ctx context.Context, proposalID string) (KnowledgeProposal, error) {
	proposalID, err := requireNonEmpty(proposalID, "LEARNING", "proposal_id")
	if err != nil {
		return KnowledgeProposal{}, err
	}
	p, err := uc.proposals.GetByID(ctx, proposalID)
	if err != nil {
		return KnowledgeProposal{}, err
	}
	if p.Status != ProposalStatusDraft {
		return KnowledgeProposal{}, apierror.BadRequest("LEARNING", "only draft proposals can be validated")
	}
	existing, err := uc.proposals.ListByAgent(ctx, p.AgentID, string(ProposalStatusApproved))
	if err != nil {
		return KnowledgeProposal{}, err
	}
	for _, e := range existing {
		if e.Kind == p.Kind && strings.EqualFold(strings.TrimSpace(e.Title), strings.TrimSpace(p.Title)) {
			_, err := uc.proposals.UpdateStatus(ctx, proposalID, ProposalStatusConflict, "")
			if err != nil {
				return KnowledgeProposal{}, err
			}
			return uc.proposals.GetByID(ctx, proposalID)
		}
	}
	validated, err := uc.proposals.UpdateStatus(ctx, proposalID, ProposalStatusValidated, "")
	if err != nil {
		return KnowledgeProposal{}, err
	}
	return validated, nil
}

func (uc *LearningLoopUsecase) RegisterKnowledge(ctx context.Context, proposalID string, approvedBy string) (KnowledgeProposal, error) {
	proposalID, err := requireNonEmpty(proposalID, "LEARNING", "proposal_id")
	if err != nil {
		return KnowledgeProposal{}, err
	}
	p, err := uc.proposals.GetByID(ctx, proposalID)
	if err != nil {
		return KnowledgeProposal{}, err
	}
	if p.Status != ProposalStatusValidated && p.Status != ProposalStatusApproved {
		return KnowledgeProposal{}, apierror.BadRequest("LEARNING", "proposal must be validated or approved before registration")
	}
	if uc.evolution != nil && p.Kind == "prompt" {
		suggestions, sErr := uc.evolution.GetEvolutionSuggestions(ctx, p.AgentID, "pending")
		if sErr != nil {
			suggestions = nil
		}
		dup := false
		for _, s := range suggestions {
			if strings.TrimSpace(s.Title) == strings.TrimSpace(p.Title) {
				dup = true
				break
			}
		}
		if !dup {
			uc.evolution.ensurePendingSuggestion(ctx, p.AgentID, p.Kind, p.Title, p.Content)
		}
	}
	applied, err := uc.proposals.UpdateStatus(ctx, proposalID, ProposalStatusApplied, approvedBy)
	if err != nil {
		return KnowledgeProposal{}, err
	}
	return applied, nil
}

func (uc *LearningLoopUsecase) ApproveProposal(ctx context.Context, proposalID string, approvedBy string) (KnowledgeProposal, error) {
	proposalID, err := requireNonEmpty(proposalID, "LEARNING", "proposal_id")
	if err != nil {
		return KnowledgeProposal{}, err
	}
	p, err := uc.proposals.GetByID(ctx, proposalID)
	if err != nil {
		return KnowledgeProposal{}, err
	}
	if p.Status != ProposalStatusValidated {
		return KnowledgeProposal{}, apierror.BadRequest("LEARNING", "only validated proposals can be approved")
	}
	return uc.proposals.UpdateStatus(ctx, proposalID, ProposalStatusApproved, approvedBy)
}

func (uc *LearningLoopUsecase) RejectProposal(ctx context.Context, proposalID string) (KnowledgeProposal, error) {
	proposalID, err := requireNonEmpty(proposalID, "LEARNING", "proposal_id")
	if err != nil {
		return KnowledgeProposal{}, err
	}
	p, err := uc.proposals.GetByID(ctx, proposalID)
	if err != nil {
		return KnowledgeProposal{}, err
	}
	if p.Status != ProposalStatusDraft && p.Status != ProposalStatusValidated {
		return KnowledgeProposal{}, apierror.BadRequest("LEARNING", "only draft or validated proposals can be rejected")
	}
	return uc.proposals.UpdateStatus(ctx, proposalID, ProposalStatusRejected, "")
}

func (uc *LearningLoopUsecase) ListProposals(ctx context.Context, agentID string, status string) ([]KnowledgeProposal, error) {
	agentID, err := requireNonEmpty(agentID, "LEARNING", "agent_id")
	if err != nil {
		return nil, err
	}
	return uc.proposals.ListByAgent(ctx, agentID, status)
}

func (uc *LearningLoopUsecase) ListPatterns(ctx context.Context, agentID string, status string) ([]Pattern, error) {
	agentID, err := requireNonEmpty(agentID, "LEARNING", "agent_id")
	if err != nil {
		return nil, err
	}
	return uc.patterns.ListByAgent(ctx, agentID, status)
}

func (uc *LearningLoopUsecase) RunLoop(ctx context.Context, agentID string) error {
	agentID, err := requireNonEmpty(agentID, "LEARNING", "agent_id")
	if err != nil {
		return err
	}
	patterns, err := uc.DetectPatterns(ctx, agentID)
	if err != nil {
		return apierror.Internal("LEARNING", "detect patterns: %s", err)
	}
	if len(patterns) == 0 {
		return nil
	}
	proposals, err := uc.GenerateProposals(ctx, agentID, patterns)
	if err != nil {
		return apierror.Internal("LEARNING", "generate proposals: %s", err)
	}
	for _, prop := range proposals {
		if _, err := uc.ValidateProposal(ctx, prop.ID); err != nil {
			continue
		}
		settings, serr := uc.agents.GetAgentRuntimeSettings(ctx, agentID)
		if serr != nil {
			continue
		}
		if settings.EvoAutoApply {
			if _, err := uc.RegisterKnowledge(ctx, prop.ID, "auto"); err != nil {
				continue
			}
		}
	}
	return nil
}

func (uc *LearningLoopUsecase) RunLoopAll(ctx context.Context) error {
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
		if !settings.EvoEnabled {
			continue
		}
		if err := uc.RunLoop(ctx, a.ID); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return apierror.Internal("LEARNING", "learning loop: %d agents failed", len(errs))
	}
	return nil
}

func patternBucketKey(o Observation) string {
	return string(o.Kind)
}

func describeBucket(key string, obs []Observation) string {
	switch key {
	case string(ObservationKindToolCall):
		toolNames := make(map[string]int)
		for _, o := range obs {
			var m map[string]any
			if json.Unmarshal([]byte(o.Metadata), &m) == nil {
				if name, ok := m["tool_name"].(string); ok {
					toolNames[name]++
				}
			}
		}
		var parts []string
		for name, count := range toolNames {
			parts = append(parts, fmt.Sprintf("%s(%d)", name, count))
		}
		return fmt.Sprintf("高频工具调用模式: %s", strings.Join(parts, ", "))
	case string(ObservationKindFeedback):
		neg := 0
		for _, o := range obs {
			if strings.Contains(strings.ToLower(o.Content), "negative") || strings.Contains(strings.ToLower(o.Content), "bad") {
				neg++
			}
		}
		return fmt.Sprintf("用户反馈模式: %d 条中 %d 条负面", len(obs), neg)
	case string(ObservationKindMemoryHit):
		return fmt.Sprintf("高频记忆命中: %d 次", len(obs))
	case string(ObservationKindMemoryMiss):
		return fmt.Sprintf("频繁记忆未命中: %d 次", len(obs))
	default:
		return fmt.Sprintf("重复模式: %s ×%d", key, len(obs))
	}
}

func proposalTitle(p Pattern) string {
	return fmt.Sprintf("学习闭环: %s (置信度 %.0f%%)", p.Description, p.Confidence*100)
}

func proposalContent(p Pattern) string {
	return fmt.Sprintf("检测到重复模式（频率 %d，置信度 %.1f%%）。\n\n模式描述: %s\n\n建议: 基于此模式优化 Agent 行为策略。", p.Frequency, p.Confidence*100, p.Description)
}

func proposalKind(patternKind string) string {
	switch patternKind {
	case string(ObservationKindToolCall):
		return "prompt"
	case string(ObservationKindFeedback):
		return "persona"
	case string(ObservationKindMemoryHit), string(ObservationKindMemoryMiss):
		return "skill"
	default:
		return "prompt"
	}
}
