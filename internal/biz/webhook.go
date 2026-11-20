package biz

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/webhookurl"

	"github.com/google/uuid"
)

const (
	WebhookEventRunCompleted    = "run.completed"
	WebhookEventRunFailed       = "run.failed"
	WebhookEventRunCancelled    = "run.cancelled"
	WebhookEventGraphTaskStatus = "graph.task.status"
	// WebhookEventTest is the synthetic event sent by the manual "test send"
	// action (TestWebhook RPC); it is never produced by run lifecycle.
	WebhookEventTest = "webhook.test"
	// MVP commerce → NovaTrader intel only (never trading).
	WebhookEventOpportunityCreated = "opportunity.created"
	WebhookEventThemeDigest        = "theme.digest"
)

// WebhookConfig is one outbound callback target for run lifecycle events.
type WebhookConfig struct {
	ID string
	// WorkspaceID 归属工作区；空串 = 存量行，按 default 工作区对待。
	WorkspaceID    string
	Name           string
	URL            string
	EventTypesJSON string
	Secret         string
	Headers        map[string]string
	Enabled        bool
	CreatedAt      string
	UpdatedAt      string
}

// effectiveWebhookWorkspace：存量行没有工作区，与 workspace.AssertWorkspace 一样按 default 算。
func effectiveWebhookWorkspace(ws string) string {
	if strings.TrimSpace(ws) == "" {
		return workspace.DefaultWorkspaceID
	}
	return ws
}

// webhookCallerWorkspace 返回调用方只能看到的工作区；system 调用方（后台任务）返回 "" = 不过滤。
func webhookCallerWorkspace(ctx context.Context) string {
	if workspace.IsSystem(ctx) {
		return ""
	}
	return workspace.IDFromContext(ctx)
}

func webhookVisibleTo(w WebhookConfig, callerWS string) bool {
	return callerWS == "" || effectiveWebhookWorkspace(w.WorkspaceID) == callerWS
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

// WebhookListQuery is the pagination/filter input for admin webhook lists.
type WebhookListQuery struct {
	// WorkspaceID 非空时只列这个工作区（default 含存量空串行）；空 = 不过滤（system）。
	WorkspaceID string
	Search      string
	Limit       int
	Offset      int
}

// WebhookListResult is a page of webhooks plus the filter-scoped total.
type WebhookListResult struct {
	Items  []WebhookConfig
	Total  int
	Limit  int
	Offset int
}

// WebhookReader provides read-only access to webhook configs.
type WebhookReader interface {
	Get(ctx context.Context, id string) (WebhookConfig, error)
	List(ctx context.Context) ([]WebhookConfig, error)
	ListPaged(ctx context.Context, q WebhookListQuery) (WebhookListResult, error)
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
		return WebhookConfig{}, apierror.Internal("GATEWAY", "webhook repository not configured")
	}
	if err := validateWebhookConfig(w); err != nil {
		return WebhookConfig{}, err
	}
	if err := validateEventTypesNonEmpty(w.EventTypesJSON); err != nil {
		return WebhookConfig{}, err
	}
	if err := uc.ensureNameUnique(ctx, w.Name, ""); err != nil {
		return WebhookConfig{}, err
	}
	w.Headers = sanitizeWebhookHeaders(w.Headers)
	if strings.TrimSpace(w.ID) == "" {
		w.ID = uuid.NewString()
	}
	if strings.TrimSpace(w.EventTypesJSON) == "" {
		w.EventTypesJSON = defaultWebhookEventTypesJSON()
	}
	if w.Headers == nil {
		w.Headers = map[string]string{}
	}
	if !workspace.IsSystem(ctx) {
		w.WorkspaceID = workspace.IDFromContext(ctx)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	w.CreatedAt = now
	w.UpdatedAt = now
	return uc.writer.Create(ctx, w)
}

func (uc *WebhookUsecase) Get(ctx context.Context, id string) (WebhookConfig, error) {
	if strings.TrimSpace(id) == "" {
		return WebhookConfig{}, apierror.BadRequest("GATEWAY", "id is required")
	}
	w, err := uc.reader.Get(ctx, id)
	if err != nil {
		return WebhookConfig{}, err
	}
	if !webhookVisibleTo(w, webhookCallerWorkspace(ctx)) {
		// 别的工作区的推送一律当不存在，不暴露 id 是否有效
		return WebhookConfig{}, apierror.NotFound("GATEWAY", "webhook not found")
	}
	return w, nil
}

func (uc *WebhookUsecase) List(ctx context.Context) ([]WebhookConfig, error) {
	if uc == nil || uc.reader == nil {
		return nil, apierror.Internal("GATEWAY", "webhook repository not configured")
	}
	all, err := uc.reader.List(ctx)
	if err != nil {
		return nil, err
	}
	callerWS := webhookCallerWorkspace(ctx)
	out := make([]WebhookConfig, 0, len(all))
	for _, w := range all {
		if webhookVisibleTo(w, callerWS) {
			out = append(out, w)
		}
	}
	return out, nil
}

// ListPaged returns a page of webhooks for the admin registry UI.
func (uc *WebhookUsecase) ListPaged(ctx context.Context, q WebhookListQuery) (WebhookListResult, error) {
	if uc == nil || uc.reader == nil {
		return WebhookListResult{}, apierror.Internal("GATEWAY", "webhook repository not configured")
	}
	if q.Limit <= 0 {
		q.Limit = 20
	}
	if q.Limit > 100 {
		q.Limit = 100
	}
	if q.Offset < 0 {
		q.Offset = 0
	}
	q.WorkspaceID = webhookCallerWorkspace(ctx)
	return uc.reader.ListPaged(ctx, q)
}

func (uc *WebhookUsecase) Update(ctx context.Context, patch WebhookUpdatePatch) (WebhookConfig, error) {
	if uc == nil || uc.writer == nil {
		return WebhookConfig{}, apierror.Internal("GATEWAY", "webhook repository not configured")
	}
	if strings.TrimSpace(patch.ID) == "" {
		return WebhookConfig{}, apierror.BadRequest("GATEWAY", "id is required")
	}
	cur, err := uc.Get(ctx, patch.ID)
	if err != nil {
		return WebhookConfig{}, err
	}
	// 仅拦截本次 patch 显式写入的空数组；存量 "[]" 记录更新其他字段不受影响
	if err := validateEventTypesNonEmpty(patch.EventTypesJSON); err != nil {
		return WebhookConfig{}, err
	}
	merged := mergeWebhookPatch(cur, patch)
	if err := validateWebhookConfig(merged); err != nil {
		return WebhookConfig{}, err
	}
	if !strings.EqualFold(merged.Name, cur.Name) {
		if err := uc.ensureNameUnique(ctx, merged.Name, cur.ID); err != nil {
			return WebhookConfig{}, err
		}
	}
	merged.Headers = sanitizeWebhookHeaders(merged.Headers)
	merged.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return uc.writer.Update(ctx, merged)
}

func (uc *WebhookUsecase) Delete(ctx context.Context, id string) error {
	if strings.TrimSpace(id) == "" {
		return apierror.BadRequest("GATEWAY", "id is required")
	}
	if _, err := uc.Get(ctx, id); err != nil {
		return err
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
		return apierror.BadRequest("GATEWAY", "name is required")
	}
	rawURL := strings.TrimSpace(w.URL)
	if rawURL == "" {
		return apierror.BadRequest("GATEWAY", "url is required")
	}
	if err := webhookurl.ValidateNotifyURL(rawURL); err != nil {
		return apierror.BadRequest("GATEWAY",
			"回调 URL 不可用：%v。须为 HTTPS 或公网 HTTP 地址；localhost/内网地址需在服务端环境变量 ARANEA_OUTBOUND_ALLOW_HOSTS 中显式放行", err)
	}
	// S-08 fix: removed unused requireSecret parameter; secret is optional.
	if v := strings.TrimSpace(w.EventTypesJSON); v != "" {
		var types []string
		if err := json.Unmarshal([]byte(v), &types); err != nil {
			return apierror.BadRequest("GATEWAY", "event_types_json must be a JSON string array")
		}
	}
	return nil
}

// validateEventTypesNonEmpty rejects an explicitly provided empty JSON array
// ("[]"): WebhookSubscribes treats an empty list as "subscribe to everything",
// which is the opposite of the caller's intent. Empty string is allowed —
// Create maps it to the default run-event set, Update treats it as "unchanged".
func validateEventTypesNonEmpty(raw string) error {
	v := strings.TrimSpace(raw)
	if v == "" {
		return nil
	}
	var types []string
	if err := json.Unmarshal([]byte(v), &types); err != nil {
		return nil // 格式错误由 validateWebhookConfig 统一报告
	}
	if len(types) == 0 {
		return apierror.BadRequest("GATEWAY", "请至少选择一个事件类型")
	}
	return nil
}

// ensureNameUnique rejects a webhook name already used by another config
// (case-insensitive). excludeID skips the record being updated.
func (uc *WebhookUsecase) ensureNameUnique(ctx context.Context, name, excludeID string) error {
	name = strings.TrimSpace(name)
	if name == "" || uc.reader == nil {
		return nil
	}
	// 名称只在本工作区内唯一
	items, err := uc.List(ctx)
	if err != nil {
		return err
	}
	for i := range items {
		if items[i].ID == excludeID {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(items[i].Name), name) {
			return apierror.Conflict("GATEWAY", "webhook 名称 %q 已存在，请换一个名称", name)
		}
	}
	return nil
}

// sanitizeWebhookHeaders drops entries with blank keys or blank values so
// that incomplete UI rows never persist (and never ship as empty headers).
func sanitizeWebhookHeaders(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if k == "" || v == "" {
			continue
		}
		out[k] = v
	}
	return out
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
	case "cancelled", "canceled": // 运行状态两种拼写都有（event_bus_callback_consumer 按 canceled 判终态）
		return WebhookEventRunCancelled
	default:
		return ""
	}
}
