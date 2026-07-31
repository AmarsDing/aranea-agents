package modelregistry

import (
	"context"
	"fmt"
	"strings"
	"time"

	"aranea-agents/pkg/loggateway"
)

type SyncInput struct {
	DryRun bool
}

type SyncOutput struct {
	Log         SyncLogEntry
	Status      string
	Message     string
	Meta        Meta
	Policy      Policy
	Apply       ApplyResult
	ApplyFailed bool
}

// syncFlowStepID is the flow log (流程日志) step ID for catalog sync,
// registered in internal/event/flow_log.go ("模型目录同步").
const syncFlowStepID = "provider.catalog.sync"

// FlowPair is a key-value pair for flow log extra metadata. It mirrors
// event.Pair / biz.LogPair without importing internal/event: that would
// create an import cycle (modelregistry → event → biz → modelregistry).
type FlowPair struct {
	Key   string
	Value any
}

// FP is a shorthand for creating a FlowPair.
func FP(key string, value any) FlowPair {
	return FlowPair{Key: key, Value: value}
}

// SyncFlowLogger is the narrow port for emitting user-visible flow logs
// (流程日志) from the catalog sync pipeline. The production adapter is wired
// in higher layers (biz/service) on top of internal/event TraceEmitter;
// nil disables emission.
type SyncFlowLogger interface {
	LogFlowStart(ctx context.Context, stepID, message string, pairs ...FlowPair)
	LogFlowDone(ctx context.Context, stepID, message string, pairs ...FlowPair)
	LogFlowError(ctx context.Context, stepID, message string, pairs ...FlowPair)
}

// SyncerOption customizes a Syncer (functional options, nil-safe).
type SyncerOption func(*Syncer)

// WithSyncFlowLogger injects the flow log port used to emit
// provider.catalog.sync start/done/error events.
func WithSyncFlowLogger(fl SyncFlowLogger) SyncerOption {
	return func(s *Syncer) { s.flow = fl }
}

type Syncer struct {
	store *Store
	now   func() time.Time
	lg    loggateway.Logger
	flow  SyncFlowLogger
}

func NewSyncer(store *Store, lg loggateway.Logger, opts ...SyncerOption) *Syncer {
	s := &Syncer{store: store, now: time.Now, lg: lg}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *Syncer) flowStart(ctx context.Context, message string, pairs ...FlowPair) {
	if s == nil || s.flow == nil {
		return
	}
	s.flow.LogFlowStart(ctx, syncFlowStepID, message, pairs...)
}

func (s *Syncer) flowDone(ctx context.Context, message string, pairs ...FlowPair) {
	if s == nil || s.flow == nil {
		return
	}
	s.flow.LogFlowDone(ctx, syncFlowStepID, message, pairs...)
}

func (s *Syncer) flowError(ctx context.Context, message string, err error, pairs ...FlowPair) {
	if s == nil || s.flow == nil {
		return
	}
	s.flow.LogFlowError(ctx, syncFlowStepID, message, append(pairs, FP("error", err.Error()))...)
}

func (s *Syncer) Sync(ctx context.Context, in SyncInput) (SyncOutput, error) {
	policy, err := s.store.LoadPolicy()
	if err != nil {
		s.lg.Error("Model registry load policy failed", loggateway.StepID("model_registry.sync.policy_fail"), loggateway.Err(err))
		s.flowError(ctx, "模型目录同步失败：加载同步策略失败", err, FP("phase", "load_policy"))
		return SyncOutput{}, err
	}
	s.flowStart(ctx, "模型目录同步开始",
		FP("source_url", policy.SourceURL),
		FP("dry_run", in.DryRun),
		FP("auto_apply", policy.AutoApply))
	started := s.now().UTC()
	logID := fmt.Sprintf("sync-%s", started.Format("20060102T150405Z"))
	entry := SyncLogEntry{
		ID:        logID,
		StartedAt: started.Format(time.RFC3339),
		SourceURL: policy.SourceURL,
		DryRun:    in.DryRun,
		Status:    "running",
	}

	s.lg.Info("Model registry sync started", loggateway.StepID("model_registry.sync"), loggateway.Str("log_id", logID), loggateway.Str("source_url", policy.SourceURL), loggateway.Str("dry_run", fmt.Sprintf("%v", in.DryRun)))

	prevMeta, _ := s.store.LoadMeta()
	fetch, err := FetchDirectory(ctx, policy.SourceURL, prevMeta.ETag, s.lg)
	if err != nil {
		entry.Status = "failed"
		entry.Message = err.Error()
		entry.FinishedAt = s.now().UTC().Format(time.RFC3339)
		if err := AppendSyncLog(s.store, entry); err != nil {
			s.lg.Error("append sync log failed", loggateway.StepID("model_registry.sync.log_fail"), loggateway.Str("log_id", logID), loggateway.Err(err))
		}
		s.lg.Error("Model registry fetch directory failed", loggateway.StepID("model_registry.sync.fetch_fail"), loggateway.Str("source_url", policy.SourceURL), loggateway.Err(err))
		s.flowError(ctx, "模型目录同步失败：拉取目录失败", err, FP("phase", "fetch"), FP("source_url", policy.SourceURL))
		return SyncOutput{Log: entry, Status: "failed", Message: err.Error(), Policy: policy}, err
	}
	if fetch.NotModified {
		entry.Status = "ok"
		entry.Message = "not modified (304)"
		entry.ETag = fetch.ETag
		if entry.ETag == "" {
			entry.ETag = prevMeta.ETag
		}
		entry.FinishedAt = s.now().UTC().Format(time.RFC3339)
		if err := AppendSyncLog(s.store, entry); err != nil {
			s.lg.Error("append sync log failed", loggateway.StepID("model_registry.sync.log_fail"), loggateway.Str("log_id", logID), loggateway.Err(err))
		}
		s.flowDone(ctx, "模型目录同步完成（目录未变更 304）",
			FP("log_id", logID),
			FP("etag", entry.ETag))
		return SyncOutput{
			Log:     entry,
			Status:  "ok",
			Message: entry.Message,
			Meta:    prevMeta,
			Policy:  policy,
		}, nil
	}

	cat, err := ParseDirectory(fetch.Body, s.lg)
	if err != nil {
		entry.Status = "failed"
		entry.Message = err.Error()
		entry.FinishedAt = s.now().UTC().Format(time.RFC3339)
		if err := AppendSyncLog(s.store, entry); err != nil {
			s.lg.Error("append sync log failed", loggateway.StepID("model_registry.sync.log_fail"), loggateway.Str("log_id", logID), loggateway.Err(err))
		}
		s.lg.Error("Model registry parse directory failed", loggateway.StepID("model_registry.sync.parse_fail"), loggateway.Err(err))
		s.flowError(ctx, "模型目录同步失败：目录解析失败", err, FP("phase", "parse"))
		return SyncOutput{Log: entry, Status: "failed", Message: err.Error(), Policy: policy}, err
	}

	pCount, mCount := CountDirectory(cat)
	entry.Stats = SyncStats{Providers: pCount, Models: mCount}
	entry.ETag = fetch.ETag

	meta := Meta{
		SyncedAt:      s.now().UTC().Format(time.RFC3339),
		ETag:          fetch.ETag,
		SHA256:        SHA256Hex(fetch.Body),
		SourceURL:     policy.SourceURL,
		ProviderCount: pCount,
		ModelCount:    mCount,
	}

	if in.DryRun {
		entry.Status = "ok"
		entry.Message = "dry run: catalog validated, not written"
		entry.FinishedAt = s.now().UTC().Format(time.RFC3339)
		if err := AppendSyncLog(s.store, entry); err != nil {
			s.lg.Error("append sync log failed", loggateway.StepID("model_registry.sync.log_fail"), loggateway.Str("log_id", logID), loggateway.Err(err))
		}
		s.flowDone(ctx, "模型目录同步完成（dry run，未写入）",
			FP("log_id", logID),
			FP("providers", pCount),
			FP("models", mCount))
		return SyncOutput{
			Log:     entry,
			Status:  "ok",
			Message: entry.Message,
			Meta:    meta,
			Policy:  policy,
		}, nil
	}

	if err := s.store.SaveDirectory(cat, meta); err != nil {
		entry.Status = "failed"
		entry.Message = err.Error()
		entry.FinishedAt = s.now().UTC().Format(time.RFC3339)
		if err := AppendSyncLog(s.store, entry); err != nil {
			s.lg.Error("append sync log failed", loggateway.StepID("model_registry.sync.log_fail"), loggateway.Str("log_id", logID), loggateway.Err(err))
		}
		s.lg.Error("Model registry save directory failed", loggateway.StepID("model_registry.sync.save_fail"), loggateway.Err(err))
		s.flowError(ctx, "模型目录同步失败：目录落库失败", err, FP("phase", "save"))
		return SyncOutput{Log: entry, Status: "failed", Message: err.Error(), Policy: policy}, err
	}

	entry.Status = "ok"
	entry.Message = fmt.Sprintf("synced %d providers, %d models", pCount, mCount)
	entry.FinishedAt = s.now().UTC().Format(time.RFC3339)
	if err := AppendSyncLog(s.store, entry); err != nil {
		s.lg.Error("append sync log failed", loggateway.StepID("model_registry.sync.log_fail"), loggateway.Str("log_id", logID), loggateway.Err(err))
	}

	s.lg.Info("Model registry sync completed", loggateway.StepID("model_registry.sync"), loggateway.Int("providers", pCount), loggateway.Int("models", mCount))
	s.flowDone(ctx, fmt.Sprintf("模型目录同步完成：%d 个 provider、%d 个模型", pCount, mCount),
		FP("log_id", logID),
		FP("providers", pCount),
		FP("models", mCount),
		FP("etag", fetch.ETag))

	return SyncOutput{
		Log:     entry,
		Status:  "ok",
		Message: entry.Message,
		Meta:    meta,
		Policy:  policy,
	}, nil
}

func (s *Syncer) NeedsScheduledSync() (bool, Policy, error) {
	policy, err := s.store.LoadPolicy()
	if err != nil {
		s.lg.Warn("Model registry load policy for scheduled sync failed", loggateway.StepID("model_registry.scheduled_sync.policy_fail"), loggateway.Err(err))
		return false, policy, err
	}
	if strings.ToLower(strings.TrimSpace(policy.SyncPolicy)) != "scheduled" {
		return false, policy, nil
	}
	meta, err := s.store.LoadMeta()
	if err != nil || meta.SyncedAt == "" {
		return true, policy, nil
	}
	t, err := time.Parse(time.RFC3339, meta.SyncedAt)
	if err != nil {
		return true, policy, nil
	}
	interval := time.Duration(policy.SyncIntervalHours) * time.Hour
	if interval <= 0 {
		interval = 24 * time.Hour
	}
	return s.now().Sub(t) >= interval, policy, nil
}
