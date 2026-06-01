package cronrunner

import (
	"context"

	"aranea-agents/internal/biz"
)

func SeedModelRegistrySyncTask(ctx context.Context, r *Runner) error {
	return biz.SeedModelRegistryCronTask(ctx, r.deps.Cron)
}
