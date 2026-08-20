package heal

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// HealStatus represents the outcome of a self-heal action.
type HealStatus string

const (
	HealStatusApplied              HealStatus = "applied"
	HealStatusSkippedLowConfidence HealStatus = "skipped_low_confidence"
	HealStatusSkippedCooldown      HealStatus = "skipped_cooldown"
	HealStatusSkippedNoAction      HealStatus = "skipped_no_action"
	HealStatusFailed               HealStatus = "failed"
	HealStatusObservedHealed       HealStatus = "observed_healed"
	HealStatusObservedFailed       HealStatus = "observed_failed"
	HealStatusAlertFired           HealStatus = "alert_fired"
)

// HealRecord documents a self-heal action taken.
type HealRecord struct {
	ID                  string         `json:"id"`
	RuleID              string         `json:"rule_id"`
	TriggerType         string         `json:"trigger_type"`
	TraceID             string         `json:"trace_id"`
	SessionID           string         `json:"session_id"`
	StepID              string         `json:"step_id"`
	FixAction           FixAction      `json:"fix_action"`
	Confidence          float64        `json:"confidence"`
	Status              string         `json:"status"` // HealStatus values: applied, skipped_low_confidence, skipped_cooldown, skipped_no_action, failed, observed_healed, observed_failed, alert_fired
	RuntimeAutoHealed   bool           `json:"runtime_auto_healed"`
	RuntimeHealAttempts int            `json:"runtime_heal_attempts"`
	Reason              string         `json:"reason,omitempty"`
	CreatedAt           string         `json:"created_at"`
	Metadata            map[string]any `json:"metadata,omitempty"`
}

// HealActionHandler is the port for executing fix actions.
// Implementations are injected via Wire to decouple biz from runtime specifics.
type HealActionHandler interface {
	HandleFixAction(ctx context.Context, action FixAction, metadata map[string]any) error
}

// HealRecordRepo persists heal records to SQLite via Ent.
type HealRecordRepo interface {
	InsertHealRecord(ctx context.Context, record HealRecord) error
	ListHealRecords(ctx context.Context, query HealRecordQuery) (HealRecordListResult, error)
	DeleteHealRecordsOlderThan(ctx context.Context, olderThan time.Time) (int, error)
}

// HealRecordQuery filters heal records for listing.
type HealRecordQuery struct {
	RuleID    string
	Status    string
	SessionID string
	Limit     int
	Offset    int
}

// HealRecordListResult is a paginated heal record list.
type HealRecordListResult struct {
	Items []HealRecord
	Total int
}

// HealStats aggregates self-heal statistics.
type HealStats struct {
	TotalHeals   int             `json:"total_heals"`
	SuccessRate  float64         `json:"success_rate"`
	TopFailRules []RuleFailCount `json:"top_fail_rules,omitempty"`
}

// RuleFailCount tracks repeated failures for a single rule.
type RuleFailCount struct {
	RuleID string `json:"rule_id"`
	Count  int    `json:"count"`
}

func generateHealID() string {
	return "heal-" + uuid.NewString()
}
