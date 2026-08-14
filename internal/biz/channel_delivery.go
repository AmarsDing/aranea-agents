package biz

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	"github.com/google/uuid"
)

const (
	ChannelDeliveryStatusPending   = "pending"
	ChannelDeliveryStatusRetry     = "retry"
	ChannelDeliveryStatusSending   = "sending"
	ChannelDeliveryStatusDelivered = "delivered"
	ChannelDeliveryStatusError     = "error"
	ChannelOutboundTextKind        = "outbound_text"
	ChannelOutboundCardKind        = "outbound_card"
	MaxOutboundAttempts            = 3
	outboundRetryBaseDelay         = 5 * time.Second
	outboundRetryMaxDelay          = 5 * time.Minute
	// OutboundDeliveryLease is how long a sending claim is exclusive. Crashed
	// workers' rows become reclaimable after this window so they cannot stick.
	OutboundDeliveryLease = 5 * time.Minute
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
// Returns (delivery, inserted, error). When inserted=false, the idempotency_key
// already existed and the existing delivery row is returned instead.
func (u *ChannelUsecase) EnqueueOutboundDelivery(ctx context.Context, channelID string, payload ChannelOutboundPayload) (ChannelDelivery, bool, error) {
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return ChannelDelivery{}, false, apierror.BadRequest("CHANNEL", "channel id is required")
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
		return ChannelDelivery{}, false, apierror.BadRequest("CHANNEL", "platform and recipient are required")
	}
	switch payload.Kind {
	case ChannelOutboundCardKind:
		if payload.CardJSON == "" && payload.Text == "" {
			return ChannelDelivery{}, false, apierror.BadRequest("CHANNEL", "card_json or text is required")
		}
	default:
		if payload.Text == "" {
			return ChannelDelivery{}, false, apierror.BadRequest("CHANNEL", "text is required")
		}
		payload.Kind = ChannelOutboundTextKind
	}
	if payload.Extra == nil {
		payload.Extra = map[string]string{}
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return ChannelDelivery{}, false, err
	}
	delivery := ChannelDelivery{
		ID:             uuid.NewString(),
		ChannelID:      channelID,
		IdempotencyKey: payload.IdempotencyKey,
		Status:         ChannelDeliveryStatusPending,
		PayloadJSON:    string(raw),
	}
	// Atomic upsert: when idempotency_key is set, use AddDeliveryIfNotExists
	// to prevent duplicate deliveries under concurrent requests.
	if strings.TrimSpace(payload.IdempotencyKey) != "" {
		result, inserted, err := u.deliveries.AddDeliveryIfNotExists(ctx, delivery)
		if err != nil {
			return ChannelDelivery{}, false, err
		}
		if !inserted {
			u.lg.Debug("投递幂等命中，跳过重复入队",
				loggateway.Str("channel_id", channelID),
				loggateway.Str("idempotency_key", payload.IdempotencyKey),
			)
		}
		return result, inserted, nil
	}
	result, err := u.deliveries.AddDelivery(ctx, delivery)
	return result, true, err
}

// ListPendingOutboundDeliveries returns queued outbound rows across channels.
func (u *ChannelUsecase) ListPendingOutboundDeliveries(ctx context.Context, limit int) ([]ChannelDelivery, error) {
	return u.deliveries.ListPendingDeliveries(ctx, limit)
}

// ClaimPendingOutboundDeliveries atomically claims due outbound rows so only
// one admin replica will send each delivery. See ChannelDeliveryRepo.ClaimPendingDeliveries.
func (u *ChannelUsecase) ClaimPendingOutboundDeliveries(ctx context.Context, limit int) ([]ChannelDelivery, error) {
	if u == nil || u.deliveries == nil {
		return nil, nil
	}
	return u.deliveries.ClaimPendingDeliveries(ctx, limit)
}

// MarkOutboundAttempt records send result and schedules retry when needed.
func (u *ChannelUsecase) MarkOutboundAttempt(ctx context.Context, row ChannelDelivery, sendErr error) (deadLetter bool, err error) {
	var payload ChannelOutboundPayload
	if err := json.Unmarshal([]byte(defaultJSON(row.PayloadJSON)), &payload); err != nil {
		u.lg.Warn("解析 outbound payload 失败，标记为 dead letter",
			loggateway.StepID("channel.delivery"),
			loggateway.Err(err),
			loggateway.Str("delivery_id", row.ID),
		)
		row.Status = ChannelDeliveryStatusError
		row.ErrorMessage = "payload parse failed: " + err.Error()
		if updateErr := u.deliveries.UpdateDelivery(ctx, row); updateErr != nil {
			return false, updateErr
		}
		return true, nil
	}
	payload.Attempts++
	if row.PayloadJSON, err = marshalOutboundPayload(payload); err != nil {
		return false, err
	}
	row.ErrorMessage = ""
	if sendErr == nil {
		row.Status = ChannelDeliveryStatusDelivered
		payload.NextRetryAt = ""
		if row.PayloadJSON, err = marshalOutboundPayload(payload); err != nil {
			return false, err
		}
		return false, u.deliveries.UpdateDelivery(ctx, row)
	}
	row.ErrorMessage = sendErr.Error()
	if payload.Attempts >= MaxOutboundAttempts {
		row.Status = ChannelDeliveryStatusError
		payload.NextRetryAt = ""
		if row.PayloadJSON, err = marshalOutboundPayload(payload); err != nil {
			return false, err
		}
		return true, u.deliveries.UpdateDelivery(ctx, row)
	}
	row.Status = ChannelDeliveryStatusRetry
	payload.NextRetryAt = time.Now().UTC().Add(outboundRetryDelay(payload.Attempts)).Format(time.RFC3339)
	if row.PayloadJSON, err = marshalOutboundPayload(payload); err != nil {
		return false, err
	}
	return false, u.deliveries.UpdateDelivery(ctx, row)
}

// IsOutboundDeliveryReady reports whether a pending/retry row may be attempted now.
// Returns false for rows with corrupt payloads — they should be marked as error by the caller.
func (u *ChannelUsecase) IsOutboundDeliveryReady(row ChannelDelivery) bool {
	if row.Status == ChannelDeliveryStatusPending || row.Status == ChannelDeliveryStatusSending {
		return true
	}
	var payload ChannelOutboundPayload
	if json.Unmarshal([]byte(defaultJSON(row.PayloadJSON)), &payload) != nil {
		return false
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
	// Exponential backoff: base * 2^(attempts-1), capped at max.
	shift := uint(attempts - 1)
	if shift > 30 { // prevent overflow
		shift = 30
	}
	delay := outboundRetryBaseDelay << shift
	if delay <= 0 || delay > outboundRetryMaxDelay {
		return outboundRetryMaxDelay
	}
	return delay
}

func marshalOutboundPayload(payload ChannelOutboundPayload) (string, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", apierror.Internal("CHANNEL", "failed to marshal outbound payload: %s", err.Error())
	}
	return string(raw), nil
}
