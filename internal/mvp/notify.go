package mvp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/biz/review"
	"aranea-agents/pkg/webhookurl"
)

var (
	channelEnqueueMu sync.Mutex
	channelEnqueue   ChannelEnqueueFn
)

// ChannelEnqueueFn enqueues native IM outbound text (Channel ≠ Publish).
// Implementations typically call ChannelUsecase.EnqueueOutboundDelivery →
// delivery worker → platform SendText.
type ChannelEnqueueFn func(ctx context.Context, channelID, platform, recipient, text, idempotencyKey string) error

// ConfigureChannelEnqueue wires native IM pending-review notify delivery.
// Noop-safe: when unset, channel env vars are ignored (HTTP URL path still works).
func ConfigureChannelEnqueue(fn ChannelEnqueueFn) {
	channelEnqueueMu.Lock()
	defer channelEnqueueMu.Unlock()
	channelEnqueue = fn
}

func currentChannelEnqueue() ChannelEnqueueFn {
	channelEnqueueMu.Lock()
	defer channelEnqueueMu.Unlock()
	return channelEnqueue
}

func resetChannelEnqueueForTest() {
	channelEnqueueMu.Lock()
	defer channelEnqueueMu.Unlock()
	channelEnqueue = nil
}

// ComposeReviewNotify runs multiple notify hooks (nil entries skipped).
func ComposeReviewNotify(fns ...review.NotifyFn) review.NotifyFn {
	var active []review.NotifyFn
	for _, fn := range fns {
		if fn != nil {
			active = append(active, fn)
		}
	}
	if len(active) == 0 {
		return nil
	}
	return func(task review.Task) {
		for _, fn := range active {
			fn(task)
		}
	}
}

// DefaultReviewNotifyFromEnv returns pending-review notify when HTTP URL and/or
// native IM channel env is set; otherwise nil (Enqueue stays noop-safe).
//
// Env:
//   - ARANEA_MVP_REVIEW_NOTIFY_URL — HTTP POST (ops webhook)
//   - ARANEA_MVP_REVIEW_NOTIFY_CHANNEL_ID + ARANEA_MVP_REVIEW_NOTIFY_RECIPIENT —
//     native IM via ConfigureChannelEnqueue (platform optional:
//     ARANEA_MVP_REVIEW_NOTIFY_PLATFORM; else resolved by enqueue impl)
//
// Channel ≠ Publish: IM/ops notify only, never a platform content publisher.
func DefaultReviewNotifyFromEnv() review.NotifyFn {
	httpFn := httpReviewNotifyFromEnv()
	channelFn := channelReviewNotifyFromEnv()
	return ComposeReviewNotify(httpFn, channelFn)
}

func httpReviewNotifyFromEnv() review.NotifyFn {
	raw := strings.TrimSpace(os.Getenv("ARANEA_MVP_REVIEW_NOTIFY_URL"))
	if raw == "" {
		return nil
	}
	if err := webhookurl.ValidateNotifyURL(raw); err != nil {
		// Misconfigured URL must not crash boot; leave notify unset.
		return nil
	}
	client := webhookurl.NewOutboundHTTPClient(8 * time.Second)
	url := raw
	return func(task review.Task) {
		payload := map[string]any{
			"event":           "review.pending",
			"schema_version":  "1.0",
			"review_task_id":  task.ID,
			"content_item_id": task.ContentItemID,
			"draft_title":     task.DraftTitle,
			"machine_flags":   task.MachineFlags,
			"graph_execution": task.GraphExecutionID,
			"disclaimer":      "Pending human review — not published. Channel≠Publish.",
		}
		body, err := json.Marshal(payload)
		if err != nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			return
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "aranea-mvp-review-notify/1.0")
		resp, err := client.Do(req)
		if err != nil {
			return
		}
		_ = resp.Body.Close()
	}
}

func channelReviewNotifyFromEnv() review.NotifyFn {
	channelID := strings.TrimSpace(os.Getenv("ARANEA_MVP_REVIEW_NOTIFY_CHANNEL_ID"))
	recipient := strings.TrimSpace(os.Getenv("ARANEA_MVP_REVIEW_NOTIFY_RECIPIENT"))
	platform := strings.TrimSpace(os.Getenv("ARANEA_MVP_REVIEW_NOTIFY_PLATFORM"))
	if channelID == "" || recipient == "" {
		return nil
	}
	return func(task review.Task) {
		fn := currentChannelEnqueue()
		if fn == nil {
			// Channel env set but enqueue not wired yet — noop (safe).
			return
		}
		text := NotifyPendingReviewMessage(task)
		idem := "mvp-review-pending:" + task.ID
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		_ = fn(ctx, channelID, platform, recipient, text, idem)
	}
}

// NotifyPendingReviewMessage is a small helper for tests / Channel adapters.
func NotifyPendingReviewMessage(task review.Task) string {
	title := strings.TrimSpace(task.DraftTitle)
	if title == "" {
		title = task.ID
	}
	return fmt.Sprintf("[Aranea 待审] %s (task=%s) — 请到审核台过审；未过审零外发", title, task.ID)
}
