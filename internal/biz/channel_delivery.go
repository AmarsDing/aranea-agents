package biz

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"aranea-agents/internal/event"

	"github.com/go-kratos/kratos/v2/errors"
	"github.com/google/uuid"
)

const (
	ChannelDeliveryStatusPending   = "pending"
	ChannelDeliveryStatusRetry     = "retry"
	ChannelDeliveryStatusDelivered = "delivered"
	ChannelDeliveryStatusError     = "error"
	ChannelOutboundTextKind        = "outbound_text"
	ChannelOutboundCardKind        = "outbound_card"
	MaxOutboundAttempts            = 3
	outboundRetryBaseDelay         = 5 * time.Second
	outboundRetryMaxDelay          = 5 * time.Minute
)

// ChannelOutboundPayload is stored in channel_delivery.payload_json for async outbound sends.
type ChannelOutboundPayload struct {
	Kind           string            `json:"kind"`
	Platform       string            `json:"platform"`
	Recipient      string            `json:"recipient"`
	Text           string            `json:"text"`
	CardJSON       string            `json:"card_json,omitempty"`
	IdempotencyKey string            `json:"idempotency_key"`
	Attempts       int               `json:"attempts,omitempty"`
	NextRetryAt    string            `json:"next_retry_at,omitempty"`
	Extra          map[string]string `json:"extra,omitempty"`
}

// EnqueueOutboundDelivery queues an outbound message for the delivery worker.
func (u *ChannelUsecase) EnqueueOutboundDelivery(ctx context.Context, channelID string, payload ChannelOutboundPayload) (ChannelDelivery, error) {
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return ChannelDelivery{}, errors.BadRequest("CHANNEL", "channel id is required")
	}
	payload.Platform = strings.TrimSpace(payload.Platform)
	payload.Recipient = strings.TrimSpace(payload.Recipient)
	payload.Text = strings.TrimSpace(payload.Text)
	payload.CardJSON = strings.TrimSpace(payload.CardJSON)
	payload.IdempotencyKey = strings.TrimSpace(payload.IdempotencyKey)
	if payload.Kind == "" {
		if payload.CardJSON != "" {
			payload.Kind = ChannelOutboundCardKind
		} else {
			payload.Kind = ChannelOutboundTextKind
		}
	}
	if payload.Platform == "" || payload.Recipient == "" {
		return ChannelDelivery{}, errors.BadRequest("CHANNEL", "platform and recipient are required")
	}
	switch payload.Kind {
	case ChannelOutboundCardKind:
		if payload.CardJSON == "" && payload.Text == "" {
			return ChannelDelivery{}, errors.BadRequest("CHANNEL", "card_json or text is required")
		}
	default:
		if payload.Text == "" {
			return ChannelDelivery{}, errors.BadRequest("CHANNEL", "text is required")
		}
		payload.Kind = ChannelOutboundTextKind
	}
	if payload.IdempotencyKey != "" {
		if exists, err := u.hasOutboundIdempotency(ctx, channelID, payload.IdempotencyKey); err != nil {
			return ChannelDelivery{}, err
		} else if exists {
			return ChannelDelivery{}, nil
		}
	}
	if payload.Extra == nil {
		payload.Extra = map[string]string{}
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return ChannelDelivery{}, err
	}
	return u.repo.AddDelivery(ctx, ChannelDelivery{
		ID:          uuid.NewString(),
		ChannelID:   channelID,
		Status:      ChannelDeliveryStatusPending,
		PayloadJSON: string(raw),
	})
}

func (u *ChannelUsecase) hasOutboundIdempotency(ctx context.Context, channelID, key string) (bool, error) {
	items, err := u.repo.ListDeliveries(ctx, channelID, 100)
	if err != nil {
		return false, err
	}
	if len(items) >= 100 {
		event.SysLogWarn("system.monitor.alert_channel_fail", "hasOutboundIdempotency scanned max deliveries without finding match; consider DB unique index for idempotency_key", event.P("channel_id", channelID), event.P("key", key))
	}
	for _, item := range items {
		if item.Status != ChannelDeliveryStatusPending &&
			item.Status != ChannelDeliveryStatusRetry &&
			item.Status != ChannelDeliveryStatusDelivered {
			continue
		}
		var payload ChannelOutboundPayload
		if json.Unmarshal([]byte(item.PayloadJSON), &payload) != nil {
			continue
		}
		if payload.IdempotencyKey == key {
			return true, nil
		}
	}
	return false, nil
}

// ListPendingOutboundDeliveries returns queued outbound rows across channels.
func (u *ChannelUsecase) ListPendingOutboundDeliveries(ctx context.Context, limit int) ([]ChannelDelivery, error) {
	return u.repo.ListPendingDeliveries(ctx, limit)
}

// MarkOutboundAttempt records send result and schedules retry when needed.
func (u *ChannelUsecase) MarkOutboundAttempt(ctx context.Context, row ChannelDelivery, sendErr error) (deadLetter bool, err error) {
	var payload ChannelOutboundPayload
	if err := json.Unmarshal([]byte(defaultJSON(row.PayloadJSON)), &payload); err != nil {
		return false, err
	}
	payload.Attempts++
	row.PayloadJSON = mustMarshalJSON(payload)
	row.ErrorMessage = ""
	if sendErr == nil {
		row.Status = ChannelDeliveryStatusDelivered
		payload.NextRetryAt = ""
		row.PayloadJSON = mustMarshalJSON(payload)
		return false, u.repo.UpdateDelivery(ctx, row)
	}
	row.ErrorMessage = sendErr.Error()
	if payload.Attempts >= MaxOutboundAttempts {
		row.Status = ChannelDeliveryStatusError
		payload.NextRetryAt = ""
		row.PayloadJSON = mustMarshalJSON(payload)
		return true, u.repo.UpdateDelivery(ctx, row)
	}
	row.Status = ChannelDeliveryStatusRetry
	payload.NextRetryAt = time.Now().UTC().Add(outboundRetryDelay(payload.Attempts)).Format(time.RFC3339)
	row.PayloadJSON = mustMarshalJSON(payload)
	return false, u.repo.UpdateDelivery(ctx, row)
}

// IsOutboundDeliveryReady reports whether a pending/retry row may be attempted now.
func (u *ChannelUsecase) IsOutboundDeliveryReady(row ChannelDelivery) bool {
	if row.Status == ChannelDeliveryStatusPending {
		return true
	}
	var payload ChannelOutboundPayload
	if json.Unmarshal([]byte(defaultJSON(row.PayloadJSON)), &payload) != nil {
		return true
	}
	if strings.TrimSpace(payload.NextRetryAt) == "" {
		return true
	}
	t, err := time.Parse(time.RFC3339, payload.NextRetryAt)
	if err != nil {
		return true
	}
	return !time.Now().UTC().Before(t)
}

func outboundRetryDelay(attempts int) time.Duration {
	if attempts <= 0 {
		return outboundRetryBaseDelay
	}
	delay := outboundRetryBaseDelay
	for i := 1; i < attempts; i++ {
		delay *= 2
		if delay >= outboundRetryMaxDelay {
			return outboundRetryMaxDelay
		}
	}
	return delay
}

func mustMarshalJSON(v any) string {
	raw, _ := json.Marshal(v)
	return string(raw)
}
