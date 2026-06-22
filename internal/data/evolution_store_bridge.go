package data

import (
	"context"
	"encoding/json"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// evolutionStoreBridge implements biz.EvolutionStoreBridge by composing
// UnifiedEvolutionRepo and SkillEvolutionSuggestionRepo.
type evolutionStoreBridge struct {
	unified *UnifiedEvolutionRepo
	legacy  *SkillEvolutionSuggestionRepo
}

var _ biz.EvolutionStoreBridge = (*evolutionStoreBridge)(nil)

// NewEvolutionStoreBridge creates a new EvolutionStoreBridge.
func NewEvolutionStoreBridge(unified *UnifiedEvolutionRepo, legacy *SkillEvolutionSuggestionRepo, _ loggateway.Logger) biz.EvolutionStoreBridge {
	return &evolutionStoreBridge{unified: unified, legacy: legacy}
}

// ── UnifiedEvolutionCheckReader ──────────────────────────────────────────────

func (b *evolutionStoreBridge) HasPendingForTarget(ctx context.Context, targetType string, targetID string) (bool, error) {
	return b.unified.HasPendingForTarget(ctx, targetType, targetID)
}

func (b *evolutionStoreBridge) GetLatestByTarget(ctx context.Context, targetType string, targetID string) (*biz.UnifiedEvolutionSuggestion, error) {
	return b.unified.GetLatestByTarget(ctx, targetType, targetID)
}

func (b *evolutionStoreBridge) GetLatestByTargetAndAction(ctx context.Context, targetType string, targetID string, actionType string) (*biz.UnifiedEvolutionSuggestion, error) {
	return b.unified.GetLatestByTargetAndAction(ctx, targetType, targetID, actionType)
}

// ── UnifiedEvolutionQueryReader ──────────────────────────────────────────────

func (b *evolutionStoreBridge) GetByID(ctx context.Context, id string) (*biz.UnifiedEvolutionSuggestion, error) {
	return b.unified.GetByID(ctx, id)
}

func (b *evolutionStoreBridge) ListByTarget(ctx context.Context, targetType string, targetID string, status string, limit, offset int) ([]biz.UnifiedEvolutionSuggestion, error) {
	return b.unified.ListByTarget(ctx, targetType, targetID, status, limit, offset)
}

func (b *evolutionStoreBridge) CountByTarget(ctx context.Context, targetType string, targetID string, status string) (int, error) {
	return b.unified.CountByTarget(ctx, targetType, targetID, status)
}

// ── UnifiedEvolutionMutationWriter ───────────────────────────────────────────

func (b *evolutionStoreBridge) Create(ctx context.Context, suggestion biz.UnifiedEvolutionSuggestion) error {
	return b.unified.Create(ctx, suggestion)
}

func (b *evolutionStoreBridge) UpdateStatus(ctx context.Context, id string, status string, actor string, reason string) error {
	return b.unified.UpdateStatus(ctx, id, status, actor, reason)
}

func (b *evolutionStoreBridge) UpdateDraftBody(ctx context.Context, id string, draftBody string) error {
	return b.unified.UpdateDraftBody(ctx, id, draftBody)
}

func (b *evolutionStoreBridge) UpdateLifecycleStatus(ctx context.Context, id string, lifecycleStatus string) error {
	return b.unified.UpdateLifecycleStatus(ctx, id, lifecycleStatus)
}

func (b *evolutionStoreBridge) UpdateSandboxResult(ctx context.Context, id string, passed bool, result json.RawMessage) error {
	return b.unified.UpdateSandboxResult(ctx, id, passed, result)
}

// ── UnifiedEvolutionExpirationWriter ─────────────────────────────────────────

func (b *evolutionStoreBridge) ExpireOlderThan(ctx context.Context, cutoff time.Time) (int, error) {
	return b.unified.ExpireOlderThan(ctx, cutoff)
}

// ── Legacy access ────────────────────────────────────────────────────────────

func (b *evolutionStoreBridge) GetEvolutionSuggestion(ctx context.Context, id string) (*biz.SkillEvolutionSuggestion, error) {
	return b.legacy.GetByID(ctx, id)
}

func (b *evolutionStoreBridge) ListEvolutionSuggestions(ctx context.Context, skillID string, status biz.EvolutionSuggestionStatus, limit, offset int) ([]biz.SkillEvolutionSuggestion, error) {
	return b.legacy.ListBySkill(ctx, skillID, status, limit, offset)
}

func (b *evolutionStoreBridge) CountEvolutionSuggestions(ctx context.Context, skillID string, status biz.EvolutionSuggestionStatus) (int, error) {
	return b.legacy.CountBySkill(ctx, skillID, status)
}

func (b *evolutionStoreBridge) CreateSuggestion(ctx context.Context, s biz.SkillEvolutionSuggestion) error {
	return b.legacy.Create(ctx, s)
}

func (b *evolutionStoreBridge) UpdateSuggestionStatus(ctx context.Context, id string, status biz.EvolutionSuggestionStatus, resolvedBy string, reason string) error {
	return b.legacy.UpdateStatus(ctx, id, status, resolvedBy, reason)
}

func (b *evolutionStoreBridge) UpdateSuggestionDraftBody(ctx context.Context, id string, draftBody string) error {
	return b.legacy.UpdateDraftBody(ctx, id, draftBody)
}

func (b *evolutionStoreBridge) UpdateSuggestionLifecycleStatus(ctx context.Context, id string, lifecycleStatus biz.EvolutionLifecycleStatus) error {
	return b.legacy.UpdateLifecycleStatus(ctx, id, lifecycleStatus)
}

func (b *evolutionStoreBridge) UpdateSuggestionSandboxResult(ctx context.Context, id string, passed bool, result json.RawMessage) error {
	return b.legacy.UpdateSandboxResult(ctx, id, passed, result)
}

func (b *evolutionStoreBridge) ListPendingSuggestions(ctx context.Context, limit, offset int) ([]biz.SkillEvolutionSuggestion, error) {
	return b.legacy.ListPending(ctx, limit, offset)
}

func (b *evolutionStoreBridge) GetLatestSuggestionBySkill(ctx context.Context, skillID string) (*biz.SkillEvolutionSuggestion, error) {
	return b.legacy.GetLatestBySkill(ctx, skillID)
}
