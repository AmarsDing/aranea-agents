package mvp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"aranea-agents/internal/biz/review"
	"aranea-agents/pkg/webhookurl"
)

// DefaultReviewNotifyFromEnv returns an HTTP pending-review notifier when
// ARANEA_MVP_REVIEW_NOTIFY_URL is set; otherwise nil (Enqueue stays noop-safe).
// Channel ≠ Publish: this is IM/ops notify only, never a platform publisher.
func DefaultReviewNotifyFromEnv() review.NotifyFn {
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

// NotifyPendingReviewMessage is a small helper for tests / Channel adapters.
func NotifyPendingReviewMessage(task review.Task) string {
	title := strings.TrimSpace(task.DraftTitle)
	if title == "" {
		title = task.ID
	}
	return fmt.Sprintf("[Aranea 待审] %s (task=%s) — 请到审核台过审；未过审零外发", title, task.ID)
}
