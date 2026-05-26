package watch

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"aranea-agents/internal/biz"

	"github.com/go-kratos/kratos/v2/log"
)

const defaultReconcileInterval = 5 * time.Minute

// AlertEvaluator runs monitor alert rules (optional).
type AlertEvaluator interface {
	EvaluateAlerts(ctx context.Context)
}

// SetAlertEvaluator configures optional alert evaluation after reconcile.
func (r *Runner) SetAlertEvaluator(eval AlertEvaluator) {
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
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				r.reconcile(ctx)
			}
		}
	}()
	r.logf(log.LevelInfo, "skill.fs.reconcile", "msg", "skill reconcile ticker started", "interval", interval.String())
}

func (r *Runner) reconcile(ctx context.Context) {
	root := r.resolveRoot(ctx)
	r.logf(log.LevelInfo, "skill.fs.reconcile", "msg", "skill reconcile scan", "path", root)
	r.scanAll(ctx, root, biz.SkillInvocationSourceFilesystemReconcile)
	slugs, err := r.uc.ListRegisteredSlugs(ctx)
	if err != nil {
		r.logf(log.LevelWarn, "skill.fs.error", "msg", "skill reconcile: list slugs", "err", err)
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
