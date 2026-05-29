package watch

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/safego"
)

const defaultReconcileInterval = 5 * time.Minute

type AlertEvaluator interface {
	EvaluateAlerts(ctx context.Context)
}

func SetAlertEvaluator(r *Runner, eval AlertEvaluator) {
	if r == nil {
		return
	}
	r.alertEval = eval
}

func reconcileIntervalFromEnv() time.Duration {
	raw := strings.TrimSpace(os.Getenv("SKILL_FS_RECONCILE_INTERVAL"))
	if raw == "" {
		return defaultReconcileInterval
	}
	if raw == "0" || strings.EqualFold(raw, "off") || strings.EqualFold(raw, "false") {
		return 0
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return defaultReconcileInterval
	}
	return d
}

func (r *Runner) startReconcileLoop(ctx context.Context) {
	interval := reconcileIntervalFromEnv()
	if interval <= 0 {
		return
	}
	ticker := time.NewTicker(interval)
	safego.Go(ctx, "skill.watch.reconcile_loop", func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				r.reconcile(ctx)
			}
		}
	})
	event.SysLogInfo("skill.fs.reconcile", "skill reconcile ticker started", event.P("interval", interval.String()))
}

func (r *Runner) reconcile(ctx context.Context) {
	root := r.resolveRoot(ctx)
	event.SysLogInfo("skill.fs.reconcile", "skill reconcile scan", event.P("path", root))
	r.scanAll(ctx, root, biz.SkillInvocationSourceFilesystemReconcile)
	slugs, err := r.reader.ListRegisteredSlugs(ctx)
	if err != nil {
		event.SysLogWarn("skill.fs.error", "skill reconcile: list slugs", event.P("err", err))
		return
	}
	for _, slug := range slugs {
		dir := filepath.Join(root, slug)
		st, statErr := os.Stat(dir)
		if statErr != nil || !st.IsDir() {
			r.syncSlug(ctx, root, slug, biz.SkillInvocationSourceFilesystemReconcile)
		}
	}
	if r.alertEval != nil {
		r.alertEval.EvaluateAlerts(ctx)
	}
}
