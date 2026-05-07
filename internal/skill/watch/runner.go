package watch

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/pkg/skillstorage"
	"aranea-agents/internal/skill/importer"

	"github.com/fsnotify/fsnotify"
	"github.com/go-kratos/kratos/v2/log"
)

const debounceWindow = 2 * time.Second

// Runner watches the skill root and upserts DB rows from on-disk skill packages.
type Runner struct {
	uc  *biz.SkillUsecase
	sys biz.SystemSettingRepo
	log log.Logger

	mu           sync.Mutex
	timer        *time.Timer
	pending      map[string]struct{}
	childWatches []string
}

// NewRunner returns a filesystem watcher. Pass nil logger to disable structured logs.
func NewRunner(uc *biz.SkillUsecase, sys biz.SystemSettingRepo, logger log.Logger) *Runner {
	return &Runner{uc: uc, sys: sys, log: logger, pending: map[string]struct{}{}}
}

func (r *Runner) resolveRoot(ctx context.Context) string {
	if r.sys != nil {
		if st, err := r.sys.Get(ctx); err == nil {
			return skillstorage.ResolveRootWithPlatform(st.RootDirectory)
		}
	}
	return skillstorage.ResolveRoot()
}

// Start runs until ctx is cancelled.
func (r *Runner) Start(ctx context.Context) {
	if r == nil || r.uc == nil {
		return
	}
	root := r.resolveRoot(ctx)
	if err := os.MkdirAll(root, 0o755); err != nil {
		r.logf(log.LevelError, "skill.fs.error", "msg", "skill watch: mkdir skill root", "path", root, "err", err)
		return
	}
	w, err := fsnotify.NewWatcher()
	if err != nil {
		r.logf(log.LevelError, "skill.fs.error", "msg", "skill watch: fsnotify", "err", err)
		return
	}
	defer func() { _ = w.Close() }()

	if err := w.Add(root); err != nil {
		r.logf(log.LevelError, "skill.fs.error", "msg", "skill watch: add root", "path", root, "err", err)
		return
	}
	r.refreshChildWatches(w, root)

	r.logf(log.LevelInfo, "skill.fs.scan", "msg", "skill watch: startup scan", "path", root)
	r.scanAll(ctx, root, biz.SkillInvocationSourceFilesystemScan)

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
				r.logf(log.LevelWarn, "skill.fs.error", "msg", "skill watch: watcher error", "err", err)
			}
		}
	}
}

func (r *Runner) logf(level log.Level, event string, kvs ...interface{}) {
	if r.log == nil {
		return
	}
	kvs = append([]interface{}{"event", event}, kvs...)
	_ = r.log.Log(level, kvs...)
}

func (r *Runner) refreshChildWatches(w *fsnotify.Watcher, root string) {
	for _, p := range r.childWatches {
		_ = w.Remove(p)
	}
	r.childWatches = nil
	ents, err := os.ReadDir(root)
	if err != nil {
		return
	}
	for _, e := range ents {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		p := filepath.Join(root, e.Name())
		if err := w.Add(p); err == nil {
			r.childWatches = append(r.childWatches, p)
		}
	}
}

func (r *Runner) onEvent(ctx context.Context, w *fsnotify.Watcher, root string, ev fsnotify.Event) {
	slug := slugFromEvent(root, ev.Name)
	if slug == "" {
		return
	}
	if ev.Op&fsnotify.Create == fsnotify.Create {
		if fi, err := os.Stat(ev.Name); err == nil && fi.IsDir() {
			rel, _ := filepath.Rel(root, ev.Name)
			if rel == slug && !strings.Contains(slug, string(filepath.Separator)) {
				_ = w.Add(ev.Name)
				r.childWatches = append(r.childWatches, ev.Name)
			}
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
	r.timer = time.AfterFunc(debounceWindow, func() { r.flushPending(ctx, root) })
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
		r.logf(log.LevelError, "skill.fs.error", "msg", "skill watch: readdir", "path", root, "err", err)
		return
	}
	for _, e := range ents {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		r.syncSlug(ctx, root, e.Name(), source)
	}
	r.logf(log.LevelInfo, "skill.fs.scan", "msg", "skill watch: scan done", "path", root, "slugs", len(ents))
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
		_ = r.uc.MarkFilesystemMissing(ctx, slug, true)
		dur := int(time.Since(t0).Milliseconds())
		_ = r.uc.RecordInvocation(ctx, biz.SkillInvocationWrite{
			Status:       "failure",
			DurationMS:   dur,
			InputPreview: preview(slug + " missing"),
			ErrorCode:    "filesystem_missing",
			ErrorMessage: errMsg,
			Source:       source,
		})
		r.logf(log.LevelWarn, sourceTag, "slug", slug, "status", "missing", "err", errMsg)
		return
	}
	_ = r.uc.MarkFilesystemMissing(ctx, slug, false)

	files, err := importer.ReadSkillDirFiles(dir)
	if err != nil {
		r.recordFailure(ctx, slug, source, t0, "read_dir", err)
		r.logf(log.LevelWarn, "skill.fs.error", "slug", slug, "err", err)
		return
	}
	candidate, tags := importer.ValidateSkillPackage(files, slug, nil, true)
	if candidate.ValidationStatus != "pass" || len(candidate.Blocks) > 0 {
		msg := "validation failed"
		if len(candidate.Blocks) > 0 {
			msg = candidate.Blocks[0].Message
		}
		r.recordFailure(ctx, slug, source, t0, "validate", errors.New(msg))
		r.logf(log.LevelWarn, "skill.fs.error", "slug", slug, "msg", msg)
		return
	}
	body := string(files["SKILL.md"])
	if body == "" {
		body = string(files["skill.md"])
	}
	sk, err := r.uc.UpsertSkillFromDisk(ctx, biz.SkillDiskSyncInput{
		Name:        candidate.Name,
		Slug:        candidate.Slug,
		Description: candidate.Description,
		Body:        body,
		Tags:        tags,
		StorageDir:  dir,
	})
	dur := int(time.Since(t0).Milliseconds())
	if err != nil {
		r.recordFailure(ctx, slug, source, t0, "upsert", err)
		r.logf(log.LevelError, "skill.fs.error", "slug", slug, "err", err)
		return
	}
	ver := ""
	if sk.CurrentVersion != nil {
		ver = sk.CurrentVersion.Version
	}
	_ = r.uc.RecordInvocation(ctx, biz.SkillInvocationWrite{
		SkillID:       sk.ID,
		SkillName:     sk.Name,
		SkillVersion:  ver,
		Status:        "success",
		DurationMS:    dur,
		InputPreview:  preview(slug + " sync"),
		OutputPreview: preview(sk.Name + " @" + sk.Slug),
		Source:        source,
	})
	r.logf(log.LevelInfo, sourceTag, "slug", slug, "skill_id", sk.ID, "duration_ms", dur)
}

func (r *Runner) recordFailure(ctx context.Context, slug, source string, t0 time.Time, code string, err error) {
	dur := int(time.Since(t0).Milliseconds())
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	_ = r.uc.RecordInvocation(ctx, biz.SkillInvocationWrite{
		Status:       "failure",
		DurationMS:   dur,
		InputPreview: preview(slug + " sync"),
		ErrorCode:    code,
		ErrorMessage: msg,
		Source:       source,
	})
}

func preview(s string) string {
	s = strings.TrimSpace(s)
	if len([]rune(s)) <= 512 {
		return s
	}
	r := []rune(s)
	return string(r[:512]) + "..."
}
