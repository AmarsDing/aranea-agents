package jobs

import (
	"context"
	"os"
	"strings"
	"time"

	"aranea-agents/internal/data"
	"aranea-agents/internal/data/sessionmemory"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/safego"

	"github.com/go-kratos/kratos/v2/log"
)

const memoryDataMigrationTimeout = 30 * time.Second

// MemoryDataMigrationWorker runs one-time data migrations after HTTP is listening.
type MemoryDataMigrationWorker struct {
	store *sessionmemory.Store
	log   *log.Helper
}

func NewMemoryDataMigrationWorker(store *sessionmemory.Store, logger log.Logger) *MemoryDataMigrationWorker {
	if store == nil {
		return nil
	}
	return &MemoryDataMigrationWorker{store: store, log: log.NewHelper(logger)}
}

func MemoryDataMigrationDisabled() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv("MEMORY_DATA_MIGRATION_DISABLED")))
	return v == "1" || v == "true" || v == "yes"
}

// Start runs pending migrations once in the background (post HTTP listen via kratos AfterStart).
func (w *MemoryDataMigrationWorker) Start(ctx context.Context) {
	if w == nil || w.store == nil {
		return
	}
	safego.Go(ctx, "memory.data_migration", func() {
		runCtx, cancel := context.WithTimeout(ctx, memoryDataMigrationTimeout)
		defer cancel()
		migrated, skipped, err := data.RunLegacyTRPCMemoryMigration(runCtx, w.store)
		if err != nil {
			event.SysLogWarn("memory.data_migration", "legacy trpc memory migration failed", event.P("error", err))
			if w.log != nil {
				w.log.Warnf("memory data migration failed: %v", err)
			}
			return
		}
		if skipped {
			if w.log != nil {
				w.log.Infof("memory data migration skipped (version %d applied)", data.MigrationLegacyTRPCMemoryFacts)
			}
			return
		}
		if migrated > 0 && w.log != nil {
			w.log.Infof("memory data migration: migrated=%d legacy trpc_memory entities", migrated)
		}
	})
}
