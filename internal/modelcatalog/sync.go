package modelcatalog

import (
	"context"
	"fmt"
	"strings"
	"time"
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

// Syncer downloads models.dev catalog and persists it locally.
type Syncer struct {
	store *Store
	now   func() time.Time
}

func NewSyncer(store *Store) *Syncer {
	return &Syncer{store: store, now: time.Now}
}

func (s *Syncer) Sync(ctx context.Context, in SyncInput) (SyncOutput, error) {
	policy, err := s.store.LoadPolicy()
	if err != nil {
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

	prevMeta, _ := s.store.LoadMeta()
	fetch, err := FetchCatalog(ctx, policy.SourceURL, prevMeta.ETag)
	if err != nil {
		entry.Status = "failed"
		entry.Message = err.Error()
		entry.FinishedAt = s.now().UTC().Format(time.RFC3339)
		_ = AppendSyncLog(s.store, entry)
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
		_ = AppendSyncLog(s.store, entry)
		return SyncOutput{
			Log:     entry,
			Status:  "ok",
			Message: entry.Message,
			Meta:    prevMeta,
			Policy:  policy,
		}, nil
	}

	cat, err := ParseCatalog(fetch.Body)
	if err != nil {
		entry.Status = "failed"
		entry.Message = err.Error()
		entry.FinishedAt = s.now().UTC().Format(time.RFC3339)
		_ = AppendSyncLog(s.store, entry)
		return SyncOutput{Log: entry, Status: "failed", Message: err.Error(), Policy: policy}, err
	}

	pCount, mCount := CountCatalog(cat)
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
		_ = AppendSyncLog(s.store, entry)
		return SyncOutput{
			Log:     entry,
			Status:  "ok",
			Message: entry.Message,
			Meta:    meta,
			Policy:  policy,
		}, nil
	}

	if err := s.store.SaveCatalog(cat, meta); err != nil {
		entry.Status = "failed"
		entry.Message = err.Error()
		entry.FinishedAt = s.now().UTC().Format(time.RFC3339)
		_ = AppendSyncLog(s.store, entry)
		return SyncOutput{Log: entry, Status: "failed", Message: err.Error(), Policy: policy}, err
	}

	logoRes := SyncProviderLogos(ctx, s.store, cat, defaultLogosBaseURL)
	entry.Stats.LogosSynced = logoRes.Synced
	entry.Stats.LogosFailed = logoRes.Failed
	entry.Stats.LogosRemoved = logoRes.Removed
	if len(logoRes.Errors) > 0 {
		entry.Errors = append(entry.Errors, logoRes.Errors...)
	}

	entry.Status = "ok"
	msg := fmt.Sprintf("synced %d providers, %d models", pCount, mCount)
	if logoRes.Synced > 0 || logoRes.Failed > 0 {
		msg += fmt.Sprintf("; logos synced=%d failed=%d", logoRes.Synced, logoRes.Failed)
	}
	entry.Message = msg
	entry.FinishedAt = s.now().UTC().Format(time.RFC3339)
	_ = AppendSyncLog(s.store, entry)

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
