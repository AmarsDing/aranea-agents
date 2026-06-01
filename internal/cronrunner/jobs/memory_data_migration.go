package jobs

import (
	"context"
	"os"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

const memoryDataMigrationTimeout = 30 * time.Second

type MemoryDataMigrationWorker struct {
	migrator biz.MemoryLegacyMigrator
	lg       loggateway.Logger
}

func NewMemoryDataMigrationWorker(migrator biz.MemoryLegacyMigrator, lg loggateway.Logger) *MemoryDataMigrationWorker {
	if migrator == nil {
		return nil
	}
	return &MemoryDataMigrationWorker{migrator: migrator, lg: lg}
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
			w.lg.Warn("legacy trpc memory migration failed", loggateway.Err(err))
			return
		}
		if skipped {
			w.lg.Info("memory data migration skipped", loggateway.Int("version", w.migrator.LegacyMigrationVersion()))
			return
		}
		if migrated > 0 {
			w.lg.Info("memory data migration completed", loggateway.Int("migrated", migrated))
		}
	})
}
