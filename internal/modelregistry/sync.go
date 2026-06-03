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

type Syncer struct {
	store *Store
	now   func() time.Time
	lg    loggateway.Logger
}

func NewSyncer(store *Store, lg loggateway.Logger) *Syncer {
	return &Syncer{store: store, now: time.Now, lg: lg}
}

func (s *Syncer) Sync(ctx context.Context, in SyncInput) (SyncOutput, error) {
	policy, err := s.store.LoadPolicy()
	if err != nil {
		s.lg.Error("Model registry load policy failed", loggateway.StepID("model_registry.sync.policy_fail"), loggateway.Err(err))
		return SyncOutput{}, err
	}
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
		return SyncOutput{Log: entry, Status: "failed", Message: err.Error(), Policy: policy}, err
	}

	entry.Status = "ok"
	entry.Message = fmt.Sprintf("synced %d providers, %d models", pCount, mCount)
	entry.FinishedAt = s.now().UTC().Format(time.RFC3339)
	if err := AppendSyncLog(s.store, entry); err != nil {
		s.lg.Error("append sync log failed", loggateway.StepID("model_registry.sync.log_fail"), loggateway.Str("log_id", logID), loggateway.Err(err))
	}

	s.lg.Info("Model registry sync completed", loggateway.StepID("model_registry.sync"), loggateway.Int("providers", pCount), loggateway.Int("models", mCount))

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
