package commerce

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// WebhookDispatcher fires opportunity.created / theme.digest (intel only — no trading).
type WebhookDispatcher func(eventType string, payload map[string]any) error

const (
	EventOpportunityCreated = "opportunity.created"
	EventThemeDigest        = "theme.digest"
)

type OpportunityUsecase struct {
	repo OpportunityRepo
	hook WebhookDispatcher
}

func NewOpportunityUsecase(repo OpportunityRepo) *OpportunityUsecase {
	return &OpportunityUsecase{repo: repo}
}

func (uc *OpportunityUsecase) SetWebhook(fn WebhookDispatcher) { uc.hook = fn }

func (uc *OpportunityUsecase) Create(o Opportunity) (Opportunity, error) {
	o.Title = strings.TrimSpace(o.Title)
	if o.Title == "" {
		return Opportunity{}, fmt.Errorf("commerce: opportunity title required")
	}
	switch o.Audience {
	case AudienceNovaTradeIntel, AudienceCommerceSourcing, AudienceBoth:
	default:
		return Opportunity{}, fmt.Errorf("commerce: invalid audience %q", o.Audience)
	}
	if o.Status == "" {
		o.Status = OppOpen
	}
	now := time.Now().UTC()
	if o.ID == "" {
		o.ID = uuid.NewString()
	}
	o.CreatedAt = now
	o.UpdatedAt = now
	out, err := uc.repo.Create(o)
	if err != nil {
		return Opportunity{}, err
	}
	if uc.hook != nil {
		_ = uc.hook(EventOpportunityCreated, map[string]any{
			"schema_version": "1.0",
			"event":          EventOpportunityCreated,
			"opportunity":    out,
			"disclaimer":     "NovaTrader intel only — never triggers orders",
		})
	}
	return out, nil
}

func (uc *OpportunityUsecase) List(audience, status string, limit, offset int) ([]Opportunity, int, error) {
	if limit <= 0 {
		limit = 50
	}
	return uc.repo.List(audience, status, limit, offset)
}

func (uc *OpportunityUsecase) Acknowledge(id string) (Opportunity, error) {
	o, err := uc.repo.Get(id)
	if err != nil {
		return Opportunity{}, err
	}
	o.Status = OppAcknowledged
	o.UpdatedAt = time.Now().UTC()
	return uc.repo.Update(o)
}

func (uc *OpportunityUsecase) Get(id string) (Opportunity, error) { return uc.repo.Get(id) }

func (uc *OpportunityUsecase) EmitThemeDigest(theme string, contentIDs []string) error {
	if uc.hook == nil {
		return nil
	}
	return uc.hook(EventThemeDigest, map[string]any{
		"schema_version": "1.0",
		"event":          EventThemeDigest,
		"theme":          theme,
		"content_ids":    contentIDs,
		"disclaimer":     "NovaTrader intel only — never triggers orders",
	})
}
