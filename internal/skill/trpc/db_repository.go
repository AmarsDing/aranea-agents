package trpc

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	trpcskill "trpc.group/trpc-go/trpc-agent-go/skill"
)

// dbSkillEntry is a cached skill loaded from the DB.
type dbSkillEntry struct {
	summary trpcskill.Summary
	slug    string
}

// skillRepoCaches groups the adapter's lazily-populated per-slug caches so
// they swap atomically as one unit on reload/Invalidate, and keeps the
// adapter at a single cache field (AS-COG-01: 2+ sync.Map fields must be
// extracted into a sub-manager).
type skillRepoCaches struct {
	bodies *sync.Map // slug → *trpcskill.Skill (Get)
	// dirs caches Path() outcomes (R3, 2026-08-13): the framework resolves
	// the storage dir on every skill_load/skill_run, and each uncached call
	// costs two DB queries (GetBySlug + GetStorageDir). "" means the skill
	// has no storage path (DB-only) — a cacheable fact, unlike errors which
	// are never stored so transient failures retry on the next call.
	dirs *sync.Map // slug → string
}

func newSkillRepoCaches() *skillRepoCaches {
	return &skillRepoCaches{bodies: &sync.Map{}, dirs: &sync.Map{}}
}

// dbQueryTimeout is the maximum duration for a single DB query performed by
// the adapter. The framework's skill.Repository interface does not accept a
// context, so we use context.Background() with an internal timeout to prevent
// unbounded DB queries from blocking the request goroutine.
const dbQueryTimeout = 5 * time.Second

// DBRepositoryAdapter implements trpcskill.Repository backed by the skill DB.
// Skills are loaded from biz.SkillUsecase and cached in-process with a TTL.
//
// Immutability guarantee: the entries slice and index map are replaced atomically
// on reload — they are never mutated after publication. Lazy-loaded skill bodies
// are stored in a separate *sync.Map so Get() never writes to the snapshot.
//
// The skillCache pointer is swapped under mu (write lock) during reload/Invalidate.
// Readers access the pointer via loadSkillCache(), ensuring they always see a
// complete map — never a torn sync.Map value from a mid-swap replacement.
type DBRepositoryAdapter struct {
	uc  *biz.SkillUsecase
	ttl time.Duration
	lg  loggateway.Logger

	mu        sync.RWMutex
	entries   []dbSkillEntry
	index     map[string]int // slug → entries index
	loaded    time.Time
	reloading bool // prevents concurrent redundant DB fetches
	// invalidateGen 由 Invalidate 递增。reload 完成时若代际变化（fetch 期间
	// 又有 Invalidate），装载新数据但保持 stale，下一轮再拉取确认 —— 用
	// 独立代际而非 loaded 零值表达「Invalidate during fetch」，避免冷启动
	// 首次加载后 loaded 永远零值导致 TTL 永不生效（缓存形同虚设）。
	invalidateGen uint64

	// caches holds lazily-populated per-slug data (bodies + storage dirs).
	// Pointer so the whole holder can be swapped atomically under mu.
	// Readers must call loadCaches() to get the current pointer.
	caches *skillRepoCaches
}

var _ trpcskill.Repository = (*DBRepositoryAdapter)(nil)

// NewDBRepositoryAdapter creates a DB-backed skill repository.
// ttl controls how long cached summaries are retained before a re-fetch.
func NewDBRepositoryAdapter(uc *biz.SkillUsecase, ttl time.Duration, lg loggateway.Logger) *DBRepositoryAdapter {
	if ttl <= 0 {
		ttl = 2 * time.Minute
	}
	return &DBRepositoryAdapter{
		uc:     uc,
		ttl:    ttl,
		lg:     lg,
		index:  make(map[string]int),
		caches: newSkillRepoCaches(),
	}
}

// loadCaches returns the current caches pointer.
// The pointer itself is immutable once read — only the map contents change
// via concurrent-safe sync.Map operations. Swaps happen under mu.
func (r *DBRepositoryAdapter) loadCaches() *skillRepoCaches {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.caches
}

// queryCtx returns a context with an internal timeout for DB queries.
// Used because the framework's skill.Repository interface does not accept
// a context parameter, so we cannot propagate the request context.
func (r *DBRepositoryAdapter) queryCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), dbQueryTimeout)
}

// Summaries returns all enabled+published skill summaries, refreshing from DB when stale.
func (r *DBRepositoryAdapter) Summaries() []trpcskill.Summary {
	ctx, cancel := r.queryCtx()
	defer cancel()
	r.refreshIfStale(ctx)
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]trpcskill.Summary, 0, len(r.entries))
	for _, e := range r.entries {
		out = append(out, e.summary)
	}
	return out
}

// Get returns the full skill by name (slug), loading body from DB on demand.
func (r *DBRepositoryAdapter) Get(name string) (*trpcskill.Skill, error) {
	ctx, cancel := r.queryCtx()
	defer cancel()
	r.refreshIfStale(ctx)
	key := canonicalSlug(name)

	// Fast path: check bodies cache (no lock needed, sync.Map is concurrent-safe).
	cache := r.loadCaches()
	if cached, ok := cache.bodies.Load(key); ok {
		return cached.(*trpcskill.Skill), nil
	}

	r.mu.RLock()
	idx, ok := r.index[key]
	if !ok {
		r.mu.RUnlock()
		return nil, &skillNotFoundError{name: name}
	}
	if idx < 0 || idx >= len(r.entries) {
		r.mu.RUnlock()
		return nil, &skillNotFoundError{name: name}
	}
	entry := r.entries[idx]
	r.mu.RUnlock()

	// Load body from DB (slow path).
	bodyCtx, bodyCancel := r.queryCtx()
	defer bodyCancel()
	body := r.loadBody(bodyCtx, entry)
	sk := &trpcskill.Skill{
		Summary: entry.summary,
		Body:    body,
	}

	// Cache for subsequent lookups — re-read the cache pointer in case a reload
	// swapped it while we were loading the body. Writing to a stale orphaned
	// sync.Map is safe but wasteful (the entry would be invisible to future Gets).
	if body != "" {
		r.loadCaches().bodies.Store(key, sk)
	}
	return sk, nil
}

// Path returns the on-disk storage directory for the skill (may be empty for DB-only skills).
func (r *DBRepositoryAdapter) Path(name string) (string, error) {
	ctx, cancel := r.queryCtx()
	defer cancel()
	r.refreshIfStale(ctx)
	key := canonicalSlug(name)
	r.mu.RLock()
	idx, ok := r.index[key]
	if !ok {
		r.mu.RUnlock()
		return "", nil // Skill not in cache, no storage path
	}
	slug := r.entries[idx].slug
	r.mu.RUnlock()

	// R3: per-slug dir cache — Path() runs on the skill_load/skill_run hot
	// path and each uncached call costs two DB queries. Cached outcomes are
	// dropped together with the body cache on reload/Invalidate.
	caches := r.loadCaches()
	if dir, ok := caches.dirs.Load(key); ok {
		return dir.(string), nil
	}

	// Resolve dir from DB on demand (dir is not part of the immutable snapshot).
	dirCtx, dirCancel := r.queryCtx()
	defer dirCancel()
	sk, err := r.uc.GetBySlug(dirCtx, slug)
	if err != nil {
		var notFound *skillNotFoundError
		if errors.As(err, &notFound) {
			return "", nil // DB-only skill has no storage path
		}
		r.lg.Warn("DBRepositoryAdapter.Path: GetBySlug failed",
			loggateway.StepID("skill.trpc"),
			loggateway.Str("slug", slug),
			loggateway.Err(err))
		return "", nil // Graceful degradation; error NOT cached, retried next call
	}
	if sk.ID == "" {
		r.loadCaches().dirs.Store(key, "")
		return "", nil
	}
	storageCtx, storageCancel := r.queryCtx()
	defer storageCancel()
	dir, dirErr := r.uc.GetStorageDir(storageCtx, sk.ID)
	if dirErr != nil {
		// Preserve the historical swallow-the-error behavior, but do not
		// cache the outcome — a transient failure must retry on the next call.
		return "", nil
	}
	r.loadCaches().dirs.Store(key, dir)
	return dir, nil
}

// Invalidate clears the cache, forcing a re-fetch on the next call.
func (r *DBRepositoryAdapter) Invalidate() {
	r.mu.Lock()
	r.loaded = time.Time{}
	r.invalidateGen++
	r.caches = newSkillRepoCaches()
	r.mu.Unlock()
}

// InvalidateSkillRuntimeCache implements biz skill.RuntimeCacheInvalidator（P0）。
// Skill 启用状态/可见性/正文变更后由 Usecase 主动调用，使变更立即生效，
// 而非等待 TTL（2min）兜底。
func (r *DBRepositoryAdapter) InvalidateSkillRuntimeCache() {
	r.Invalidate()
}

// reloadFailureBackoff 是后台 reload 失败后的重试退避窗口。single-flight 只能
// 挡住并发重复，挡不住串行请求各自触发后台重试；失败时把 loaded 前移使 TTL
// 在该窗口后才再次过期，避免 DB 故障期间每个 stale 请求都打一次 DB。
// 包级变量以便测试缩短窗口。
var reloadFailureBackoff = 30 * time.Second

// refreshIfStale 采用 stale-while-revalidate：
//   - 冷启动（无快照）或 Invalidate 后（loaded 零值）：同步 reload —— 主动失效
//     语义要求「变更立即生效」，首请求必须见到新数据，不能落回旧快照。
//   - TTL 自然过期且已有快照：立即用旧快照应答，后台 single-flight 刷新，
//     请求路径不再承担同步 reload 的毛刺（实测 summaries 全量拉取 ~280ms）。
func (r *DBRepositoryAdapter) refreshIfStale(ctx context.Context) {
	r.mu.RLock()
	stale := time.Since(r.loaded) > r.ttl
	syncLoad := len(r.entries) == 0 || r.loaded.IsZero()
	r.mu.RUnlock()
	if !stale {
		return
	}
	if syncLoad {
		r.reload(ctx)
		return
	}
	// 红线 #13：后台刷新走 safego（panic 恢复 + hook），进程级生命周期。
	safego.GoBackground("skill.repo_revalidate", func() {
		bgCtx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
		defer cancel()
		r.reload(bgCtx)
	})
}

func (r *DBRepositoryAdapter) reload(ctx context.Context) {
	// Double-check under write lock to avoid redundant reloads.
	// The reloading flag prevents multiple goroutines from fetching concurrently
	// when the cache is stale — only the first one proceeds; others return early.
	r.mu.Lock()
	if time.Since(r.loaded) <= r.ttl || r.reloading {
		r.mu.Unlock()
		return
	}
	r.reloading = true
	gen := r.invalidateGen
	r.mu.Unlock()

	candidates, err := r.uc.ListEnabledPublishedCandidates(ctx)
	if err != nil {
		r.lg.Warn("skill 缓存刷新失败，保留陈旧数据",
			loggateway.StepID("skill.reload_fail"),
			loggateway.Err(err))
		r.mu.Lock()
		// 失败退避：loaded 前移使 TTL 在 backoff 后才再次过期，
		// 避免 DB 故障期间每个 stale 请求都触发一次后台 reload。
		// loaded 零值（冷启动失败 / Invalidate 等待同步拉取）不退避，
		// 保持「下次访问同步重试」语义。
		if !r.loaded.IsZero() {
			r.loaded = time.Now().Add(-r.ttl + reloadFailureBackoff)
		}
		r.reloading = false
		r.mu.Unlock()
		return
	}
	entries := make([]dbSkillEntry, 0, len(candidates))
	index := make(map[string]int, len(candidates))
	for _, c := range candidates {
		slug := canonicalSlug(c.Slug)
		if slug == "" {
			continue
		}
		// TPM-P1-06: Summary.Name is the canonical handle (slug) used by trpcskill.Get
		// and by Aranea skill visibility filters (allow/deny lists are slug-based).
		// Previously this was c.Name (display name) which silently broke Layer A
		// filtering for every DB-backed skill. Display name is preserved via the
		// Description prefix so prompts retain human-readable context.
		desc := strings.TrimSpace(c.Description)
		if display := strings.TrimSpace(c.Name); display != "" && !strings.EqualFold(display, slug) {
			if desc == "" {
				desc = display
			} else {
				desc = display + " — " + desc
			}
		}
		e := dbSkillEntry{
			slug: slug,
			summary: trpcskill.Summary{
				Name:        slug,
				Description: desc,
			},
		}
		index[slug] = len(entries)
		entries = append(entries, e)
	}
	// Swap entries, index, and caches atomically under write lock.
	// Preserve the "invalidated" state: if Invalidate() was called while we
	// were fetching (generation changed), the data we fetched may predate the
	// invalidation — install it but stay stale so the next access re-fetches.
	r.mu.Lock()
	r.entries = entries
	r.index = index
	if gen != r.invalidateGen {
		r.loaded = time.Time{}
	} else {
		r.loaded = time.Now()
	}
	r.caches = newSkillRepoCaches()
	r.reloading = false
	r.mu.Unlock()
}

func (r *DBRepositoryAdapter) loadBody(ctx context.Context, entry dbSkillEntry) string {
	sk, err := r.uc.GetBySlug(ctx, entry.slug)
	if err != nil {
		r.lg.Warn("skill body 加载失败",
			loggateway.StepID("skill.load_body_fail"),
			loggateway.Str("slug", entry.slug),
			loggateway.Err(err))
		return ""
	}
	body, err := r.uc.GetLatestMarkdown(ctx, sk.ID)
	if err != nil {
		r.lg.Warn("skill markdown 加载失败",
			loggateway.StepID("skill.load_markdown_fail"),
			loggateway.Str("slug", entry.slug),
			loggateway.Str("skill_id", sk.ID),
			loggateway.Err(err))
		return ""
	}
	return body
}

func canonicalSlug(s string) string {
	return biz.NormalizeSkillSlug(s)
}

type skillNotFoundError struct{ name string }

func (e *skillNotFoundError) Error() string { return "skill not found: " + e.name }
