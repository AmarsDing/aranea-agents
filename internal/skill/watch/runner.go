package watch

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event/contract"
	"aranea-agents/internal/skill/importer"
	"aranea-agents/internal/skill/storage"
	"aranea-agents/pkg/appctx"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	"github.com/fsnotify/fsnotify"
)

const debounceWindow = 2 * time.Second

// SkillReader provides read-only skill lookups needed by the filesystem watcher.
type SkillReader interface {
	GetBySlug(ctx context.Context, slug string) (biz.Skill, error)
	ListSimilaritySources(ctx context.Context) ([]biz.SkillSimilaritySource, error)
	ListRegisteredSlugs(ctx context.Context) ([]string, error)
}

// SkillWriter provides write operations needed by the filesystem watcher.
type SkillWriter interface {
	MarkFilesystemMissing(ctx context.Context, slug string, missing bool) error
	RecordInvocation(ctx context.Context, inv biz.SkillInvocationWrite) error
	UpsertSkillFromDisk(ctx context.Context, input biz.SkillDiskSyncInput) (biz.Skill, biz.SkillDiskSyncOutcome, error)
}

type Runner struct {
	reader     SkillReader
	writer     SkillWriter
	sys        biz.SystemSettingRepo
	monitorBus contract.MonitorBus
	reporter   SyncReporter
	alertEval  AlertEvaluator
	lg         loggateway.Logger

	mu           sync.Mutex
	timer        *time.Timer
	pending      map[string]struct{}
	childWatches []string
}

func SetSyncReporter(r *Runner, reporter SyncReporter) {
	if r == nil {
		return
	}
	r.reporter = reporter
}

func NewRunner(reader SkillReader, writer SkillWriter, sys biz.SystemSettingRepo, lg loggateway.Logger) *Runner {
	return &Runner{reader: reader, writer: writer, sys: sys, lg: lg, pending: map[string]struct{}{}}
}

func NewRunnerWithBus(reader SkillReader, writer SkillWriter, sys biz.SystemSettingRepo, monitorBus contract.MonitorBus, lg loggateway.Logger) *Runner {
	return &Runner{reader: reader, writer: writer, sys: sys, monitorBus: monitorBus, lg: lg, pending: map[string]struct{}{}}
}

func (r *Runner) resolveRoot(ctx context.Context) string {
	if r.sys != nil {
		if st, err := r.sys.Get(ctx); err == nil {
			return storage.ResolveRootWithPlatform(st.RootDirectory)
		}
	}
	return storage.ResolveRoot()
}

func (r *Runner) Start(ctx context.Context) {
	if r == nil || r.reader == nil {
		return
	}
	root := r.resolveRoot(ctx)
	if err := os.MkdirAll(root, 0o755); err != nil {
		r.lg.Error("skill watch: mkdir skill root", loggateway.StepID("skill.fs.error"), loggateway.Str("path", root), loggateway.Err(err))
		return
	}
	w, err := fsnotify.NewWatcher()
	if err != nil {
		r.lg.Error("skill watch: fsnotify", loggateway.StepID("skill.fs.error"), loggateway.Err(err))
		return
	}
	defer func() { _ = w.Close() }()

	if err := w.Add(root); err != nil {
		r.lg.Error("skill watch: add root", loggateway.StepID("skill.fs.error"), loggateway.Str("path", root), loggateway.Err(err))
		return
	}
	r.refreshChildWatches(w, root)

	r.lg.Info("skill watch: startup scan", loggateway.StepID("skill.fs.scan"), loggateway.Str("path", root))
	r.scanAll(ctx, root, biz.SkillInvocationSourceFilesystemScan)
	r.startReconcileLoop(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-w.Events:
			if !ok {
				return
			}
			r.onEvent(ctx, w, root, ev)
		case err, ok := <-w.Errors:
			if !ok {
				return
			}
			if err != nil {
				r.lg.Warn("skill watch: watcher error", loggateway.StepID("skill.fs.error"), loggateway.Err(err))
			}
		}
	}
}

func (r *Runner) refreshChildWatches(w *fsnotify.Watcher, root string) {
	r.mu.Lock()
	old := r.childWatches
	r.childWatches = nil
	r.mu.Unlock()
	for _, p := range old {
		_ = w.Remove(p)
	}
	var added []string
	// Walk recursively to add all subdirectories, not just direct children.
	filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if path == root {
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		// Skip hidden directories.
		if strings.HasPrefix(d.Name(), ".") {
			return fs.SkipDir
		}
		if addErr := w.Add(path); addErr != nil {
			r.lg.Warn("watch: failed to add subdirectory",
				loggateway.StepID("skill.watch"),
				loggateway.Str("path", path),
				loggateway.Err(addErr))
		} else {
			added = append(added, path)
		}
		return nil
	})
	r.mu.Lock()
	r.childWatches = added
	r.mu.Unlock()
}

func (r *Runner) onEvent(ctx context.Context, w *fsnotify.Watcher, root string, ev fsnotify.Event) {
	slug := slugFromEvent(root, ev.Name)
	if slug == "" {
		return
	}
	if ev.Op&fsnotify.Create == fsnotify.Create {
		if fi, err := os.Stat(ev.Name); err == nil && fi.IsDir() {
			// Recursively add watches for the new directory and all its subdirectories.
			filepath.WalkDir(ev.Name, func(path string, d fs.DirEntry, err error) error {
				if err != nil || !d.IsDir() {
					return nil
				}
				// Skip hidden directories.
				if d.Name() != filepath.Base(ev.Name) && strings.HasPrefix(d.Name(), ".") {
					return fs.SkipDir
				}
				if addErr := w.Add(path); addErr != nil {
					r.lg.Warn("watch: failed to add new directory",
						loggateway.StepID("skill.watch"),
						loggateway.Str("path", path),
						loggateway.Err(addErr))
				} else {
					r.mu.Lock()
					r.childWatches = append(r.childWatches, path)
					r.mu.Unlock()
				}
				return nil
			})
		}
	}
	r.scheduleSlug(ctx, root, slug)
}

func (r *Runner) scheduleSlug(ctx context.Context, root, slug string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pending[slug] = struct{}{}
	if r.timer != nil {
		r.timer.Stop()
	}
	r.timer = time.AfterFunc(debounceWindow, func() {
		safego.Go(ctx, "skill.watch.flush_pending", func() {
			r.flushPending(ctx, root)
		})
	})
}

func (r *Runner) flushPending(ctx context.Context, root string) {
	r.mu.Lock()
	slugs := make([]string, 0, len(r.pending))
	for s := range r.pending {
		slugs = append(slugs, s)
	}
	r.pending = map[string]struct{}{}
	r.mu.Unlock()
	for _, slug := range slugs {
		r.syncSlug(ctx, root, slug, biz.SkillInvocationSourceFilesystemWatch)
	}
}

func slugFromEvent(root, fullPath string) string {
	root = filepath.Clean(root)
	fullPath = filepath.Clean(fullPath)
	rel, err := filepath.Rel(root, fullPath)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return ""
	}
	parts := strings.Split(rel, string(filepath.Separator))
	if len(parts) == 0 {
		return ""
	}
	slug := parts[0]
	if slug == "" || strings.HasPrefix(slug, ".") {
		return ""
	}
	return slug
}

func (r *Runner) scanAll(ctx context.Context, root string, source string) {
	ents, err := os.ReadDir(root)
	if err != nil {
		r.lg.Error("skill watch: readdir", loggateway.StepID("skill.fs.error"), loggateway.Str("path", root), loggateway.Err(err))
		return
	}
	for _, e := range ents {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		r.syncSlug(ctx, root, e.Name(), source)
	}
	r.lg.Info("skill watch: scan done", loggateway.StepID("skill.fs.scan"), loggateway.Str("path", root), loggateway.Int("slugs", len(ents)))
}

func (r *Runner) syncSlug(ctx context.Context, root, slug, source string) {
	t0 := time.Now()
	sourceTag := "skill.fs.sync"
	dir := filepath.Join(root, slug)
	st, statErr := os.Stat(dir)
	if statErr != nil || !st.IsDir() {
		errMsg := "not a directory"
		if statErr != nil {
			errMsg = statErr.Error()
		}
		if err := r.writer.MarkFilesystemMissing(ctx, slug, true); err != nil {
			r.lg.Warn("failed to mark filesystem missing", loggateway.Err(err), loggateway.Str("slug", slug))
		}
		dur := int(time.Since(t0).Milliseconds())
		if err := r.writer.RecordInvocation(ctx, biz.SkillInvocationWrite{
			Status:       "failure",
			DurationMS:   dur,
			InputPreview: preview(slug + " missing"),
			ErrorCode:    "filesystem_missing",
			ErrorMessage: errMsg,
			Source:       source,
		}); err != nil {
			r.lg.Warn("failed to record skill invocation", loggateway.Err(err), loggateway.Str("slug", slug))
		}
		r.lg.Warn("skill filesystem missing", loggateway.StepID(sourceTag), loggateway.Str("slug", slug), loggateway.Str("err", errMsg))
		r.reportSync(ctx, "skill.filesystem.missing", slug, "Skill 磁盘目录缺失: "+slug, "warn")
		return
	}
	if err := r.writer.MarkFilesystemMissing(ctx, slug, false); err != nil {
		r.lg.Warn("failed to mark filesystem missing", loggateway.Err(err), loggateway.Str("slug", slug))
	}

	files, err := importer.ReadSkillDirFiles(dir)
	if err != nil {
		r.recordFailure(ctx, slug, source, t0, "read_dir", err)
		r.lg.Warn("skill read dir failed", loggateway.StepID("skill.fs.error"), loggateway.Str("slug", slug), loggateway.Err(err))
		return
	}
	candidate, tags, triggers := importer.ValidateSkillPackage(files, slug, nil, true)
	if mismatch := importer.DirectorySlugMismatch(slug, candidate.Slug); mismatch != nil {
		r.recordFailure(ctx, slug, source, t0, mismatch.Type, errors.New(mismatch.Message))
		r.lg.Warn("skill slug mismatch", loggateway.StepID("skill.fs.error"), loggateway.Str("slug", slug), loggateway.Str("msg", mismatch.Message))
		r.reportSync(ctx, "skill.filesystem.rejected", slug, mismatch.Message, "warn")
		return
	}
	if candidate.ValidationStatus != "pass" || len(candidate.Blocks) > 0 {
		msg := "validation failed"
		if len(candidate.Blocks) > 0 {
			msg = candidate.Blocks[0].Message
		}
		r.recordFailure(ctx, slug, source, t0, "validate", errors.New(msg))
		r.lg.Warn("skill validation failed", loggateway.StepID("skill.fs.error"), loggateway.Str("slug", slug), loggateway.Str("msg", msg))
		r.reportSync(ctx, "skill.filesystem.rejected", slug, msg, "warn")
		return
	}
	body := string(files["SKILL.md"])
	if body == "" {
		body = string(files["skill.md"])
	}
	if strings.TrimSpace(body) == "" {
		// 防御性检查：ValidateSkillPackage 正常会拒绝空 body，此处处理边缘情况。
		// 必须报告失败而非静默返回，否则前端/运维无法感知异常。
		r.recordFailure(ctx, slug, source, t0, "empty_body", errors.New("SKILL.md content is empty"))
		r.lg.Warn("syncSlug: SKILL.md is empty or missing",
			loggateway.StepID("skill.watch"),
			loggateway.Str("slug", slug),
			loggateway.Str("dir", dir))
		r.reportSync(ctx, "skill.filesystem.rejected", slug, "SKILL.md 内容为空", "warn")
		return
	}
	wasMissing := false
	isNew := true
	if existing, err := r.reader.GetBySlug(ctx, slug); err == nil {
		isNew = false
		wasMissing = existing.FilesystemMissing
	}
	sk, outcome, err := r.writer.UpsertSkillFromDisk(ctx, biz.SkillDiskSyncInput{
		Name:        candidate.Name,
		Slug:        candidate.Slug,
		Description: candidate.Description,
		Body:        body,
		Tags:        tags,
		Triggers:    triggers,
		StorageDir:  dir,
	})
	dur := int(time.Since(t0).Milliseconds())
	if err != nil {
		r.recordFailure(ctx, slug, source, t0, "upsert", err)
		r.lg.Error("skill upsert failed", loggateway.StepID("skill.fs.error"), loggateway.Str("slug", slug), loggateway.Err(err))
		return
	}
	ver := ""
	if sk.CurrentVersion != nil {
		ver = sk.CurrentVersion.Version
	}
	if err := r.writer.RecordInvocation(ctx, biz.SkillInvocationWrite{
		SkillID:       sk.ID,
		SkillVersion:  ver,
		Status:        "success",
		DurationMS:    dur,
		InputPreview:  preview(slug + " sync"),
		OutputPreview: preview(sk.Name + " @" + sk.Slug),
		Source:        source,
	}); err != nil {
		r.lg.Warn("failed to record skill invocation", loggateway.Err(err), loggateway.Str("slug", slug))
	}
	r.lg.Info("skill sync success", loggateway.StepID(sourceTag), loggateway.Str("slug", slug), loggateway.Str("skill_id", sk.ID), loggateway.Int("duration_ms", dur))
	eventKey := "skill.filesystem.updated"
	severity := "info"
	message := "Skill 磁盘同步: " + sk.Name
	switch {
	case outcome.RevertedToDraft:
		eventKey = "skill.filesystem.updated"
		severity = "warn"
		message = "磁盘内容已变更，Skill 已回退为草稿并停用: " + sk.Name
	case wasMissing:
		eventKey = "skill.filesystem.recovered"
		message = "Skill 磁盘目录已恢复: " + sk.Name
	case isNew:
		eventKey = "skill.filesystem.imported"
		message = "检测到新磁盘 Skill（待发布）: " + sk.Name
	}
	r.reportSync(ctx, eventKey, slug, message, severity)
	if isNew {
		r.checkSimilarityAsync(slug, candidate.Name)
	}
	if r.monitorBus != nil {
		ev := contract.NewMonitorEvent(contract.MonitorEventTypeSkillReload, "skill.watch")
		ev.Metadata = map[string]any{"slug": slug}
		r.monitorBus.Publish(ctx, ev)
	}
}

func (r *Runner) recordFailure(ctx context.Context, slug, source string, t0 time.Time, code string, err error) {
	dur := int(time.Since(t0).Milliseconds())
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	if err := r.writer.RecordInvocation(ctx, biz.SkillInvocationWrite{
		Status:       "failure",
		DurationMS:   dur,
		InputPreview: preview(slug + " sync"),
		ErrorCode:    code,
		ErrorMessage: msg,
		Source:       source,
	}); err != nil {
		r.lg.Warn("failed to record skill invocation", loggateway.Err(err), loggateway.Str("slug", slug))
	}
}

func preview(s string) string {
	s = strings.TrimSpace(s)
	if len([]rune(s)) <= 512 {
		return s
	}
	r := []rune(s)
	return string(r[:512]) + "..."
}

func (r *Runner) checkSimilarityAsync(slug, name string) {
	if r == nil || r.reader == nil {
		return
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	slug = strings.TrimSpace(slug)
	safego.Go(appctx.Ctx(), "skill.fs.similarity", func() {
		ctx := context.Background()
		sources, err := r.reader.ListSimilaritySources(ctx)
		if err != nil {
			return
		}
		for _, item := range sources {
			if strings.EqualFold(strings.TrimSpace(item.Slug), slug) {
				continue
			}
			if strings.EqualFold(strings.TrimSpace(item.Name), name) {
				r.reportSync(ctx, "skill.filesystem.similarity_warn", slug,
					"磁盘 Skill "+name+" 与已有 Skill "+item.Name+" 名称相同，建议 review", "warn")
				return
			}
		}
	})
}

func (r *Runner) reportSync(ctx context.Context, eventKey, slug, message, severity string) {
	if r == nil || r.reporter == nil {
		return
	}
	r.reporter.ReportFilesystemSync(ctx, SyncReport{
		EventKey: eventKey,
		Slug:     slug,
		Message:  message,
		Severity: severity,
		// 常规磁盘同步成功（updated/info）高频低价值：只发 Bus 不落库，
		// 避免淹没 monitor_events（治理向事件仍落库：imported/recovered/rejected/missing）。
		SkipPersist: eventKey == "skill.filesystem.updated" && severity == "info",
	})
}
