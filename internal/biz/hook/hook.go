// Package hook implements hook CRUD, config parsing, validation, delivery, and resolution.
package hook

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"aranea-agents/pkg/apierror"

	"aranea-agents/internal/biz/shared"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/webhookurl"
)

var hookIDRand uint64

func newHookID() string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		n := atomic.AddUint64(&hookIDRand, 1)
		return hex.EncodeToString([]byte{byte(n >> 56), byte(n >> 48), byte(n >> 40), byte(n >> 32), byte(n >> 24), byte(n >> 16), byte(n >> 8), byte(n)})
	}
	return hex.EncodeToString(buf)
}

// ── Hook model ────────────────────────────────────────────────────────────────

// HookPatch is a partial update for a hook; nil fields are left unchanged.
type HookPatch struct {
	Key          *string
	Name         *string
	Description  *string
	Status       *string
	Enabled      *bool
	SortOrder    *int
	ConfigJSON   *string
	MetadataJSON *string
}

// StrPtr returns a pointer to the given string.
func StrPtr(s string) *string { return &s }

// BoolPtr returns a pointer to the given bool.
func BoolPtr(b bool) *bool { return &b }

// IntPtr returns a pointer to the given int.
func IntPtr(i int) *int { return &i }

// Hook is one row of hooks (legacy "hooks" platform resource).
type Hook struct {
	ID           string
	Key          string
	Name         string
	Description  string
	Status       string
	Enabled      bool
	SortOrder    int
	ConfigJSON   string
	MetadataJSON string
	CreatedAt    string
	UpdatedAt    string
	DeletedAt    string
}

// ListQuery is the pagination/filter input for admin hook lists.
type ListQuery struct {
	Search        string
	CallbackPoint string
	Limit         int
	Offset        int
}

// ListResult is a page of hooks plus the filter-scoped total.
type ListResult struct {
	Items  []Hook
	Total  int
	Limit  int
	Offset int
}

// Repo abstracts hook persistence.
type Repo interface {
	ListHooks(ctx context.Context) ([]Hook, error)
	ListHooksPaged(ctx context.Context, q ListQuery) (ListResult, error)
	GetHook(ctx context.Context, id string) (Hook, error)
	CreateHook(ctx context.Context, h Hook) (Hook, error)
	UpdateHook(ctx context.Context, h Hook) (Hook, error)
	DeleteHook(ctx context.Context, id string) error
}

// Usecase implements hook CRUD workflows.
type Usecase struct {
	repo Repo
	lg   loggateway.Logger
}

// NewUsecase constructs a HookUsecase.
func NewUsecase(repo Repo, lg loggateway.Logger) *Usecase {
	return &Usecase{repo: repo, lg: lg}
}

// List returns all hooks.
func (u *Usecase) List(ctx context.Context) ([]Hook, error) {
	return u.repo.ListHooks(ctx)
}

// ListPaged returns a page of hooks for the admin registry UI.
func (u *Usecase) ListPaged(ctx context.Context, q ListQuery) (ListResult, error) {
	if q.Limit <= 0 {
		q.Limit = 20
	}
	if q.Limit > 100 {
		q.Limit = 100
	}
	if q.Offset < 0 {
		q.Offset = 0
	}
	return u.repo.ListHooksPaged(ctx, q)
}

// Get returns one hook by ID.
func (u *Usecase) Get(ctx context.Context, id string) (Hook, error) {
	if strings.TrimSpace(id) == "" {
		return Hook{}, apierror.BadRequest("HOOK", "id is required")
	}
	return u.repo.GetHook(ctx, id)
}

// Create validates and stores a new hook.
func (u *Usecase) Create(ctx context.Context, in Hook) (Hook, error) {
	in.Key = strings.TrimSpace(in.Key)
	in.Name = strings.TrimSpace(in.Name)
	if in.Key == "" || in.Name == "" {
		return Hook{}, apierror.BadRequest("HOOK", "key and name are required")
	}
	if in.ID == "" {
		in.ID = newHookID()
	}
	if in.Status == "" {
		in.Status = "active"
	}
	if err := ValidateConfigForSave(in.ConfigJSON, u.lg); err != nil {
		return Hook{}, err
	}
	return u.repo.CreateHook(ctx, in)
}

// Update patches an existing hook.
func (u *Usecase) Update(ctx context.Context, id string, patch HookPatch) (Hook, error) {
	if strings.TrimSpace(id) == "" {
		return Hook{}, apierror.BadRequest("HOOK", "id is required")
	}
	cur, err := u.repo.GetHook(ctx, id)
	if err != nil {
		return Hook{}, err
	}
	merged := cur
	if patch.Key != nil && *patch.Key != "" {
		merged.Key = *patch.Key
	}
	if patch.Name != nil && *patch.Name != "" {
		merged.Name = *patch.Name
	}
	if patch.Status != nil && *patch.Status != "" {
		merged.Status = *patch.Status
	}
	if patch.Description != nil {
		merged.Description = *patch.Description
	}
	if patch.Enabled != nil {
		merged.Enabled = *patch.Enabled
	}
	if patch.SortOrder != nil {
		merged.SortOrder = *patch.SortOrder
	}
	if patch.ConfigJSON != nil {
		merged.ConfigJSON = *patch.ConfigJSON
	}
	if patch.MetadataJSON != nil {
		merged.MetadataJSON = *patch.MetadataJSON
	}
	if err := ValidateConfigForSave(merged.ConfigJSON, u.lg); err != nil {
		return Hook{}, err
	}
	return u.repo.UpdateHook(ctx, merged)
}

// Delete removes a hook.
func (u *Usecase) Delete(ctx context.Context, id string) error {
	if strings.TrimSpace(id) == "" {
		return apierror.BadRequest("HOOK", "id is required")
	}
	return u.repo.DeleteHook(ctx, id)
}

// ── Hook config ───────────────────────────────────────────────────────────────

// Config is the JSON shape stored in hooks.config_json.
type Config struct {
	CallbackPoint string    `json:"callback_point"`
	Condition     Condition `json:"condition"`
	Action        Action    `json:"action"`
}

// Condition scopes when a hook fires.
type Condition struct {
	AgentID   string `json:"agent_id"`
	ToolName  string `json:"tool_name"`
	EventType string `json:"event_type"`
}

// Action describes what to do when the hook fires.
type Action struct {
	Type             string         `json:"type"`
	WebhookURL       string         `json:"webhook_url"`
	WebhookSecret    string         `json:"webhook_secret"`
	ModifyPatch      map[string]any `json:"modify_patch"`
	LogLevel         string         `json:"log_level"`
	Message          string         `json:"message"`
	NotifyMaxRetries int            `json:"notify_max_retries"`
	NotifyTimeoutSec int            `json:"notify_timeout_sec"`
}

// ParseConfig unmarshals ConfigJSON; empty config is valid but has no point.
func ParseConfig(configJSON string, lg loggateway.Logger) (Config, error) {
	raw := strings.TrimSpace(configJSON)
	if raw == "" {
		return Config{}, nil
	}
	var cfg Config
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		lg.Warn("解析 hook config 失败", loggateway.StepID("hook.parse_config"), loggateway.Err(err))
		return Config{}, err
	}
	cfg.CallbackPoint = NormalizeCallbackPoint(cfg.CallbackPoint)
	return cfg, nil
}

// CallbackPoint returns the normalized lifecycle point from config_json.
func (h Hook) CallbackPoint() string {
	cfg, err := ParseConfig(h.ConfigJSON, loggateway.NewNoop())
	if err != nil {
		return ""
	}
	return cfg.CallbackPoint
}

// ConditionFromConfig returns condition fields from config_json.
func (h Hook) ConditionFromConfig() Condition {
	cfg, err := ParseConfig(h.ConfigJSON, loggateway.NewNoop())
	if err != nil {
		return Condition{}
	}
	return cfg.Condition
}

// ActionFromConfig returns action fields from config_json.
func (h Hook) ActionFromConfig() Action {
	cfg, err := ParseConfig(h.ConfigJSON, loggateway.NewNoop())
	if err != nil {
		return Action{}
	}
	return cfg.Action
}

// NormalizeCallbackPoint maps aliases to canonical snake_case points.
func NormalizeCallbackPoint(point string) string {
	switch strings.ToLower(strings.TrimSpace(point)) {
	case "beforeagent", "before_agent":
		return "before_agent"
	case "afteragent", "after_agent":
		return "after_agent"
	case "beforemodel", "before_model":
		return "before_model"
	case "aftermodel", "after_model":
		return "after_model"
	case "beforetool", "before_tool":
		return "before_tool"
	case "aftertool", "after_tool":
		return "after_tool"
	case "onevent", "on_event":
		return "on_event"
	default:
		return strings.ToLower(strings.TrimSpace(point))
	}
}

// AppliesToAgent reports whether condition.agent_id matches the given ids.
func AppliesToAgent(cond Condition, agentID, agentKey string) bool {
	want := strings.TrimSpace(cond.AgentID)
	if want == "" {
		return true
	}
	if want == strings.TrimSpace(agentID) {
		return true
	}
	return want == strings.TrimSpace(agentKey)
}

// AppliesToTool reports whether condition.tool_name matches (empty = any).
func AppliesToTool(cond Condition, toolName string) bool {
	want := strings.TrimSpace(cond.ToolName)
	if want == "" {
		return true
	}
	return want == strings.TrimSpace(toolName)
}

// ── Hook validation ───────────────────────────────────────────────────────────

// ValidateConfigForSave checks config_json before Hook CRUD persistence.
func ValidateConfigForSave(configJSON string, lg loggateway.Logger) error {
	cfg, err := ParseConfig(configJSON, lg)
	if err != nil {
		return apierror.BadRequest("HOOK", "invalid config_json: "+err.Error())
	}
	action := strings.ToLower(strings.TrimSpace(cfg.Action.Type))
	if action != "notify" {
		return nil
	}
	url := strings.TrimSpace(cfg.Action.WebhookURL)
	if url == "" {
		return apierror.BadRequest("HOOK", "webhook_url required for notify action")
	}
	if err := webhookurl.ValidateNotifyURL(url); err != nil {
		return apierror.BadRequest("HOOK", err.Error())
	}
	return nil
}

// ── Hook delivery ─────────────────────────────────────────────────────────────

// DeliveryStatus is hook_deliveries.status.
type DeliveryStatus string

const (
	DeliveryPending DeliveryStatus = "pending"
	DeliverySuccess DeliveryStatus = "success"
	DeliveryFailed  DeliveryStatus = "failed"
)

// Delivery is one queued Hook notify webhook attempt.
type Delivery struct {
	ID             string
	HookKey        string
	HookID         string
	WebhookURL     string
	WebhookSecret  string
	PayloadJSON    string
	Status         DeliveryStatus
	AttemptCount   int
	MaxAttempts    int
	LastError      string
	IdempotencyKey string
	CreatedAt      string
	UpdatedAt      string
}

// DeliveryQuery filters hook_deliveries list API.
type DeliveryQuery struct {
	HookKey string
	Status  string
	From    string
	To      string
	Limit   int32
	Offset  int32
}

// DeliveryListResult is a paginated delivery list.
type DeliveryListResult struct {
	Items  []Delivery
	Total  int32
	Limit  int32
	Offset int32
}

// DeliveryRepo persists notify delivery rows.
type DeliveryRepo interface {
	Insert(ctx context.Context, d Delivery) error
	UpdateResult(ctx context.Context, id string, status DeliveryStatus, attemptCount int, lastError string) error
	List(ctx context.Context, q DeliveryQuery) (DeliveryListResult, error)
	// ListStalePending returns pending deliveries whose updated_at is older than
	// updatedBefore and that still have remaining attempts (OUT-02 / HK-01).
	// Used by the retry worker to rediscover deliveries after process crash.
	ListStalePending(ctx context.Context, updatedBefore time.Time, limit int) ([]Delivery, error)
	// TryClaimForRetry atomically increments attempt_count for the given delivery
	// only when its current count equals expectedAttemptCount. Returns true when
	// the claim succeeds (this worker owns the retry), false when another instance
	// already claimed it (optimistic lock, multi-pod safe). (OUT-02 / HK-01)
	TryClaimForRetry(ctx context.Context, id string, expectedAttemptCount int) (bool, error)
}

// NotifyOptions from action.notify_* fields in hook config_json.
type NotifyOptions struct {
	MaxAttempts   int
	TimeoutSec    int
	WebhookSecret string
}

// ParseNotifyOptions reads optional notify retry settings from Action.
func ParseNotifyOptions(action Action) NotifyOptions {
	opts := NotifyOptions{MaxAttempts: 3, TimeoutSec: 8}
	if action.NotifyMaxRetries > 0 {
		opts.MaxAttempts = action.NotifyMaxRetries
	}
	if action.NotifyTimeoutSec > 0 {
		opts.TimeoutSec = action.NotifyTimeoutSec
	}
	opts.WebhookSecret = strings.TrimSpace(action.WebhookSecret)
	return opts
}

// NormalizeDeliveryStatus canonicalizes status strings.
func NormalizeDeliveryStatus(s string) DeliveryStatus {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "success", "ok":
		return DeliverySuccess
	case "failed", "error":
		return DeliveryFailed
	default:
		return DeliveryPending
	}
}

// DeliveryIdempotencyKey produces a deterministic key from hook ID + event identity.
// Same hook + same event = same key → duplicate triggers map to one delivery row.
func DeliveryIdempotencyKey(hookID string, payload map[string]any) string {
	h := sha256.New()
	h.Write([]byte(hookID))
	if eventType, ok := payload["event_type"]; ok {
		h.Write([]byte{0})
		h.Write([]byte(fmtString(eventType)))
	}
	if runID, ok := payload["run_id"]; ok {
		h.Write([]byte{0})
		h.Write([]byte(fmtString(runID)))
	}
	if sessionID, ok := payload["session_id"]; ok {
		h.Write([]byte{0})
		h.Write([]byte(fmtString(sessionID)))
	}
	return hex.EncodeToString(h.Sum(nil))[:32]
}

func fmtString(v any) string {
	if v == nil {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}

// ── Hook delivery usecase ─────────────────────────────────────────────────────

// DeliveryUsecase lists persisted Hook notify deliveries.
type DeliveryUsecase struct {
	repo DeliveryRepo
}

// NewDeliveryUsecase creates a delivery query usecase.
func NewDeliveryUsecase(repo DeliveryRepo) *DeliveryUsecase {
	return &DeliveryUsecase{repo: repo}
}

// List returns paginated hook_deliveries rows.
func (u *DeliveryUsecase) List(ctx context.Context, q DeliveryQuery, page, pageSize int32) (DeliveryListResult, error) {
	if u == nil || u.repo == nil {
		return DeliveryListResult{}, nil
	}
	limit, offset, _, _ := shared.PageToLimitOffset(page, pageSize)
	q.Limit = int32(limit)
	q.Offset = int32(offset)
	return u.repo.List(ctx, q)
}

// ListStalePending returns pending deliveries that have been idle longer than
// staleAfter, up to limit rows. Used by the retry worker (OUT-02 / HK-01).
func (u *DeliveryUsecase) ListStalePending(ctx context.Context, staleAfter time.Duration, limit int) ([]Delivery, error) {
	if u == nil || u.repo == nil {
		return nil, nil
	}
	return u.repo.ListStalePending(ctx, time.Now().UTC().Add(-staleAfter), limit)
}

// ── Hook resolver ─────────────────────────────────────────────────────────────

// ResolvedHook is one enabled hook rule that matches an agent scope.
type ResolvedHook struct {
	Hook Hook
	Rule Config
}

// Resolver loads hooks from the DB and matches them to agents at chain-build time.
type Resolver struct {
	uc     *Usecase
	mu     sync.RWMutex
	cache  []ResolvedHook
	loaded bool
	lg     loggateway.Logger
}

// NewResolver creates a resolver backed by HookUsecase.
func NewResolver(uc *Usecase, lg loggateway.Logger) *Resolver {
	return &Resolver{uc: uc, lg: lg}
}

// Reload refreshes the in-memory hook snapshot (enabled + active only).
func (r *Resolver) Reload(ctx context.Context) error {
	if r == nil || r.uc == nil {
		return nil
	}
	all, err := r.uc.List(ctx)
	if err != nil {
		return err
	}
	enabled := make([]ResolvedHook, 0, len(all))
	for _, h := range all {
		if !hookRuleActive(h) {
			continue
		}
		cfg, err := ParseConfig(h.ConfigJSON, r.lg)
		if err != nil || cfg.CallbackPoint == "" {
			continue
		}
		enabled = append(enabled, ResolvedHook{Hook: h, Rule: cfg})
	}
	r.mu.Lock()
	r.cache = enabled
	r.loaded = true
	r.mu.Unlock()
	return nil
}

func hookRuleActive(h Hook) bool {
	if !h.Enabled {
		return false
	}
	if strings.TrimSpace(h.DeletedAt) != "" {
		return false
	}
	st := strings.ToLower(strings.TrimSpace(h.Status))
	return st == "" || st == "active"
}

// Resolve returns hooks whose config matches the agent (tool_name checked at invoke time).
// It reads from the in-memory cache populated by Reload; if the cache is empty it
// falls back to a DB query and auto-populates the cache.
func (r *Resolver) Resolve(agentID, agentKey string) []ResolvedHook {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	cached := r.cache
	wasLoaded := r.loaded
	r.mu.RUnlock()
	if !wasLoaded {
		if err := r.Reload(context.Background()); err != nil {
			r.lg.Warn("resolver.fallback_reload_failed", loggateway.StepID("hook"), loggateway.Err(err))
			return nil
		}
		r.mu.RLock()
		cached = r.cache
		r.mu.RUnlock()
	}
	out := make([]ResolvedHook, 0, len(cached))
	for _, rh := range cached {
		if !AppliesToAgent(rh.Rule.Condition, agentID, agentKey) {
			continue
		}
		out = append(out, rh)
	}
	return out
}
