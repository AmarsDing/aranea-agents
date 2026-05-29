package modelcatalog

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

// Runner runs scheduled catalog sync in the background.
type Runner struct {
	stores   StoreProvider
	applier  *Applier
	interval time.Duration
	logger   *log.Logger
	syncMu   sync.Mutex
}

func NewRunner(stores StoreProvider, logger *log.Logger) *Runner {
	if logger == nil {
		logger = log.Default()
	}
	return &Runner{
		stores:   stores,
		interval: time.Hour,
		logger:   logger,
	}
}

func (r *Runner) SetApplier(a *Applier) {
	r.applier = a
}

func (r *Runner) Start(ctx context.Context) {
	go r.loop(ctx)
}

func (r *Runner) loop(ctx context.Context) {
	r.tick(ctx)
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.tick(ctx)
		}
	}
}

func (r *Runner) tick(ctx context.Context) {
	syncCtx, cancel := context.WithTimeout(ctx, defaultFetchTimeout+10*time.Second)
	defer cancel()
	store, err := r.stores.Store(syncCtx)
	if err != nil {
		r.logger.Printf("model-catalog: store resolve failed: %v", err)
		return
	}
	syncer := NewSyncer(store)
	need, policy, err := syncer.NeedsScheduledSync()
	if err != nil {
		r.logger.Printf("model-catalog: schedule check failed: %v", err)
		return
	}
	if !need {
		return
	}
	out, applyRes, err := r.syncAndApplyLocked(syncCtx, syncer, store, false)
	if err != nil {
		r.logger.Printf("model-catalog: scheduled sync failed: %v", err)
		return
	}
	if len(applyRes.Errors) > 0 {
		r.logger.Printf("model-catalog: scheduled sync apply failed: %v", applyRes.Errors)
	} else {
		r.logger.Printf("model-catalog: scheduled sync ok providers=%d models=%d policy=%s",
			out.Meta.ProviderCount, out.Meta.ModelCount, policy.SyncPolicy)
	}
}

// SyncNow triggers an immediate sync (manual). Apply runs in the same exclusive lock when configured.
func (r *Runner) SyncNow(ctx context.Context, dryRun bool) (SyncOutput, ApplyResult, error) {
	r.syncMu.Lock()
	defer r.syncMu.Unlock()

	syncCtx, cancel := context.WithTimeout(ctx, defaultFetchTimeout+10*time.Second)
	defer cancel()
	store, err := r.stores.Store(syncCtx)
	if err != nil {
		return SyncOutput{}, ApplyResult{}, err
	}
	return r.syncAndApplyLocked(syncCtx, NewSyncer(store), store, dryRun)
}

func (r *Runner) syncAndApplyLocked(ctx context.Context, syncer *Syncer, store *Store, dryRun bool) (SyncOutput, ApplyResult, error) {
	out, err := syncer.Sync(ctx, SyncInput{DryRun: dryRun})
	if err != nil || dryRun || r.applier == nil {
		return out, ApplyResult{}, err
	}
	if strings.EqualFold(strings.TrimSpace(out.Policy.AutoApply), "none") {
		return out, ApplyResult{}, nil
	}
	cat, _, loadErr := store.LoadCatalog()
	if loadErr != nil {
		out.ApplyFailed = true
		out.Log.Errors = append(out.Log.Errors, "load catalog after sync: "+loadErr.Error())
		return out, ApplyResult{}, fmt.Errorf("load catalog after sync: %w", loadErr)
	}
	applyRes := r.applier.ApplyWithMigration(ctx, cat, out.Policy.AutoApply)
	out.Apply = applyRes
	out.Log.Stats.LLMRowsUpdated = applyRes.LLMRowsUpdated
	out.Log.Stats.DeprecatedDisabled = applyRes.LLMRowsDisabled
	out.Log.Stats.AgentsUpdated = applyRes.Migration.Agents
	if len(applyRes.Errors) > 0 {
		out.ApplyFailed = true
		out.Log.Errors = append(out.Log.Errors, applyRes.Errors...)
	}
	if out.ApplyFailed && !dryRun {
		out.Log.Status = "partial"
		out.Log.Message += "; apply failed"
		_ = UpdateSyncLogEntry(store, out.Log)
	} else if !dryRun && len(applyRes.Errors) == 0 && (applyRes.Migration.Agents > 0 || applyRes.Migration.Sessions > 0 ||
		applyRes.Migration.Eval > 0 || applyRes.Migration.RuntimeSettings > 0 || applyRes.Migration.Skills > 0 ||
		applyRes.Migration.KnowledgeEmbed > 0 || applyRes.Migration.WebResearch > 0 || applyRes.LLMRowsUpdated > 0) {
		_ = store.SaveMigrationCheckpoint(NewMigrationCheckpoint(applyRes.Migration))
	}
	return out, applyRes, nil
}
