package biz

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"aranea-agents/pkg/webhookurl"

	"github.com/go-kratos/kratos/v2/errors"
	"github.com/google/uuid"
)

const (
	WebhookEventRunCompleted    = "run.completed"
	WebhookEventRunFailed       = "run.failed"
	WebhookEventRunCancelled    = "run.cancelled"
	WebhookEventGraphTaskStatus = "graph.task.status"
)

// WebhookConfig is one outbound callback target for run lifecycle events.
type WebhookConfig struct {
	ID             string
	Name           string
	URL            string
	EventTypesJSON string
	Secret         string
	Headers        map[string]string
	Enabled        bool
	CreatedAt      string
	UpdatedAt      string
}

// WebhookUpdatePatch carries partial webhook fields; nil Enabled preserves the current value.
type WebhookUpdatePatch struct {
	ID             string
	Name           string
	URL            string
	EventTypesJSON string
	Secret         string
	Headers        map[string]string
	Enabled        *bool
}

// WebhookReader provides read-only access to webhook configs.
type WebhookReader interface {
	Get(ctx context.Context, id string) (WebhookConfig, error)
	List(ctx context.Context) ([]WebhookConfig, error)
	ListEnabled(ctx context.Context) ([]WebhookConfig, error)
}

// WebhookWriter provides write access to webhook configs.
type WebhookWriter interface {
	Create(ctx context.Context, w WebhookConfig) (WebhookConfig, error)
	Update(ctx context.Context, w WebhookConfig) (WebhookConfig, error)
	Delete(ctx context.Context, id string) error
}

// WebhookRepository combines read and write access.
// Deprecated: use WebhookReader or WebhookWriter for narrower dependency.
type WebhookRepository interface {
	WebhookReader
	WebhookWriter
}

// WebhookUsecase manages webhook configs.
// W-2 fix: depend on narrow interfaces instead of composite WebhookRepository.
type WebhookUsecase struct {
	reader WebhookReader
	writer WebhookWriter
}

// S-2 fix: accept narrow interfaces instead of composite WebhookRepository.
func NewWebhookUsecase(reader WebhookReader, writer WebhookWriter) *WebhookUsecase {
	return &WebhookUsecase{reader: reader, writer: writer}
}

func (uc *WebhookUsecase) Create(ctx context.Context, w WebhookConfig) (WebhookConfig, error) {
	if uc == nil || uc.writer == nil {
		return WebhookConfig{}, errors.InternalServer("GATEWAY", "webhook repository not configured")
	}
	if err := validateWebhookConfig(w); err != nil {
		return WebhookConfig{}, err
	}
	if strings.TrimSpace(w.ID) == "" {
		w.ID = uuid.NewString()
	}
	if strings.TrimSpace(w.EventTypesJSON) == "" {
		w.EventTypesJSON = defaultWebhookEventTypesJSON()
	}
	if w.Headers == nil {
		w.Headers = map[string]string{}
	}
	now := time.Now().UTC().Format(time.RFC3339)
	w.CreatedAt = now
	w.UpdatedAt = now
	return uc.writer.Create(ctx, w)
}

func (uc *WebhookUsecase) Get(ctx context.Context, id string) (WebhookConfig, error) {
	if strings.TrimSpace(id) == "" {
		return WebhookConfig{}, errors.BadRequest("GATEWAY", "id is required")
	}
	return uc.reader.Get(ctx, id)
}

func (uc *WebhookUsecase) List(ctx context.Context) ([]WebhookConfig, error) {
	if uc == nil || uc.reader == nil {
		return nil, errors.InternalServer("GATEWAY", "webhook repository not configured")
	}
	return uc.reader.List(ctx)
}

func (uc *WebhookUsecase) Update(ctx context.Context, patch WebhookUpdatePatch) (WebhookConfig, error) {
	if uc == nil || uc.writer == nil {
		return WebhookConfig{}, errors.InternalServer("GATEWAY", "webhook repository not configured")
	}
	if strings.TrimSpace(patch.ID) == "" {
		return WebhookConfig{}, errors.BadRequest("GATEWAY", "id is required")
	}
	cur, err := uc.reader.Get(ctx, patch.ID)
	if err != nil {
		return WebhookConfig{}, err
	}
	merged := mergeWebhookPatch(cur, patch)
	if err := validateWebhookConfig(merged); err != nil {
		return WebhookConfig{}, err
	}
	merged.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return uc.writer.Update(ctx, merged)
}

func (uc *WebhookUsecase) Delete(ctx context.Context, id string) error {
	if strings.TrimSpace(id) == "" {
		return errors.BadRequest("GATEWAY", "id is required")
	}
	return uc.writer.Delete(ctx, id)
}

func mergeWebhookPatch(cur WebhookConfig, patch WebhookUpdatePatch) WebhookConfig {
	out := cur
	if v := strings.TrimSpace(patch.Name); v != "" {
		out.Name = v
	}
	if v := strings.TrimSpace(patch.URL); v != "" {
		out.URL = v
	}
	if v := strings.TrimSpace(patch.EventTypesJSON); v != "" {
		out.EventTypesJSON = v
	}
	if patch.Secret != "" {
		out.Secret = patch.Secret
	}
	if patch.Headers != nil {
		out.Headers = patch.Headers
	}
	if patch.Enabled != nil {
		out.Enabled = *patch.Enabled
	}
	return out
}

func validateWebhookConfig(w WebhookConfig) error {
	if strings.TrimSpace(w.Name) == "" {
		return errors.BadRequest("GATEWAY", "name is required")
	}
	rawURL := strings.TrimSpace(w.URL)
	if rawURL == "" {
		return errors.BadRequest("GATEWAY", "url is required")
	}
	if err := webhookurl.ValidateNotifyURL(rawURL); err != nil {
		return errors.BadRequest("GATEWAY", err.Error())
	}
	// S-08 fix: removed unused requireSecret parameter; secret is optional.
	if v := strings.TrimSpace(w.EventTypesJSON); v != "" {
		var types []string
		if err := json.Unmarshal([]byte(v), &types); err != nil {
			return errors.BadRequest("GATEWAY", "event_types_json must be a JSON string array")
		}
	}
	return nil
}

func defaultWebhookEventTypesJSON() string {
	b, _ := json.Marshal([]string{
		WebhookEventRunCompleted,
		WebhookEventRunFailed,
		WebhookEventRunCancelled,
	})
	return string(b)
}

// WebhookSubscribes returns true when the config listens for eventType.
func WebhookSubscribes(eventTypesJSON, eventType string) bool {
	eventType = strings.TrimSpace(eventType)
	if eventType == "" {
		return false
	}
	var types []string
	if err := json.Unmarshal([]byte(strings.TrimSpace(eventTypesJSON)), &types); err != nil {
		return false
	}
	if len(types) == 0 {
		return true
	}
	for _, t := range types {
		if strings.TrimSpace(t) == eventType {
			return true
		}
	}
	return false
}

// RunStatusToWebhookEvent maps terminal run statuses to webhook event types.
func RunStatusToWebhookEvent(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed":
		return WebhookEventRunCompleted
	case "failed":
		return WebhookEventRunFailed
	case "cancelled":
		return WebhookEventRunCancelled
	default:
		return ""
	}
}
