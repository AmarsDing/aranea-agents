package biz

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/go-kratos/kratos/v2/errors"
	"github.com/google/uuid"
)

const (
	ChannelDeliveryStatusPending  = "pending"
	ChannelDeliveryStatusRetry    = "retry"
	ChannelDeliveryStatusDelivered = "delivered"
	ChannelDeliveryStatusError    = "error"
	channelOutboundKind           = "outbound_text"
	maxOutboundAttempts           = 3
)

// ChannelOutboundPayload is stored in channel_delivery.payload_json for async outbound sends.
type ChannelOutboundPayload struct {
	Kind           string            `json:"kind"`
	Platform       string            `json:"platform"`
	Recipient      string            `json:"recipient"`
	Text           string            `json:"text"`
	IdempotencyKey string            `json:"idempotency_key"`
	Attempts       int               `json:"attempts,omitempty"`
	Extra          map[string]string `json:"extra,omitempty"`
}

// EnqueueOutboundDelivery queues an outbound message for the delivery worker.
func (u *ChannelUsecase) EnqueueOutboundDelivery(ctx context.Context, channelID string, payload ChannelOutboundPayload) (ChannelDelivery, error) {
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return ChannelDelivery{}, errors.BadRequest("CHANNEL", "channel id is required")
	}
	payload.Kind = channelOutboundKind
	payload.Platform = strings.TrimSpace(payload.Platform)
	payload.Recipient = strings.TrimSpace(payload.Recipient)
	payload.Text = strings.TrimSpace(payload.Text)
	payload.IdempotencyKey = strings.TrimSpace(payload.IdempotencyKey)
	if payload.Platform == "" || payload.Recipient == "" || payload.Text == "" {
		return ChannelDelivery{}, errors.BadRequest("CHANNEL", "platform, recipient and text are required")
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
func (u *ChannelUsecase) MarkOutboundAttempt(ctx context.Context, row ChannelDelivery, sendErr error) error {
	var payload ChannelOutboundPayload
	if err := json.Unmarshal([]byte(defaultJSON(row.PayloadJSON)), &payload); err != nil {
		return err
	}
	payload.Attempts++
	row.PayloadJSON = mustMarshalJSON(payload)
	row.ErrorMessage = ""
	if sendErr == nil {
		row.Status = ChannelDeliveryStatusDelivered
		return u.repo.UpdateDelivery(ctx, row)
	}
	row.ErrorMessage = sendErr.Error()
	if payload.Attempts >= maxOutboundAttempts {
		row.Status = ChannelDeliveryStatusError
	} else {
		row.Status = ChannelDeliveryStatusRetry
	}
	return u.repo.UpdateDelivery(ctx, row)
}

func mustMarshalJSON(v any) string {
	raw, _ := json.Marshal(v)
	return string(raw)
}
