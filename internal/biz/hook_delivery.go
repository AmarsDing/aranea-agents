package biz

import (
	"context"
	"strings"
)

// HookDeliveryStatus is hook_deliveries.status.
type HookDeliveryStatus string

const (
	HookDeliveryPending HookDeliveryStatus = "pending"
	HookDeliverySuccess HookDeliveryStatus = "success"
	HookDeliveryFailed  HookDeliveryStatus = "failed"
)

// HookDelivery is one queued Hook notify webhook attempt.
type HookDelivery struct {
	ID           string
	HookKey      string
	HookID       string
	WebhookURL   string
	PayloadJSON  string
	Status       HookDeliveryStatus
	AttemptCount int
	MaxAttempts  int
	LastError    string
	CreatedAt    string
	UpdatedAt    string
}

// HookDeliveryQuery filters hook_deliveries list API.
type HookDeliveryQuery struct {
	HookKey  string
	Status   string
	From     string
	To       string
	Limit    int32
	Offset   int32
}

// HookDeliveryListResult is a paginated delivery list.
type HookDeliveryListResult struct {
	Items  []HookDelivery
	Total  int32
	Limit  int32
	Offset int32
}

// HookDeliveryRepo persists notify delivery rows.
type HookDeliveryRepo interface {
	Insert(ctx context.Context, d HookDelivery) error
	UpdateResult(ctx context.Context, id string, status HookDeliveryStatus, attemptCount int, lastError string) error
	List(ctx context.Context, q HookDeliveryQuery) (HookDeliveryListResult, error)
}

// HookNotifyOptions from action.notify_* fields in hook config_json.
type HookNotifyOptions struct {
	MaxAttempts int
	TimeoutSec  int
}

// ParseHookNotifyOptions reads optional notify retry settings from HookAction.
func ParseHookNotifyOptions(action HookAction) HookNotifyOptions {
	opts := HookNotifyOptions{MaxAttempts: 3, TimeoutSec: 8}
	if action.NotifyMaxRetries > 0 {
		opts.MaxAttempts = action.NotifyMaxRetries
	}
	if action.NotifyTimeoutSec > 0 {
		opts.TimeoutSec = action.NotifyTimeoutSec
	}
	return opts
}

// NormalizeHookDeliveryStatus canonicalizes status strings.
func NormalizeHookDeliveryStatus(s string) HookDeliveryStatus {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "success", "ok":
		return HookDeliverySuccess
	case "failed", "error":
		return HookDeliveryFailed
	default:
		return HookDeliveryPending
	}
}
