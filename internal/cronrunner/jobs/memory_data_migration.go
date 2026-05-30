package jobs

import (
	"context"
	"os"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/safego"

	"github.com/go-kratos/kratos/v2/log"
)

const memoryDataMigrationTimeout = 30 * time.Second

type MemoryDataMigrationWorker struct {
	migrator biz.MemoryLegacyMigrator
	log      *log.Helper
}

func NewMemoryDataMigrationWorker(migrator biz.MemoryLegacyMigrator, logger log.Logger) *MemoryDataMigrationWorker {
	if migrator == nil {
		return nil
	}
	return &MemoryDataMigrationWorker{migrator: migrator, log: log.NewHelper(logger)}
}

func MemoryDataMigrationDisabled() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv("MEMORY_DATA_MIGRATION_DISABLED")))
	return v == "1" || v == "true" || v == "yes"
}

func (w *MemoryDataMigrationWorker) Start(ctx context.Context) {
	if w == nil || w.migrator == nil {
		return
	}
	safego.Go(ctx, "memory.data_migration", func() {
		runCtx, cancel := context.WithTimeout(ctx, memoryDataMigrationTimeout)
		defer cancel()
		migrated, skipped, err := w.migrator.RunLegacyMigration(runCtx)
		if err != nil {
			event.SysLogWarn("memory.data_migration", "legacy trpc memory migration failed", event.P("error", err))
			if w.log != nil {
				w.log.Warnf("memory data migration failed: %v", err)
			}
			return
		}
		if skipped {
			if w.log != nil {
				w.log.Infof("memory data migration skipped (version %d applied)", w.migrator.LegacyMigrationVersion())
			}
			return
		}
		if migrated > 0 && w.log != nil {
			w.log.Infof("memory data migration: migrated=%d legacy trpc_memory entities", migrated)
		}
	})
}
