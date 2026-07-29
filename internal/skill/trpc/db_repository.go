package trpc

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	trpcskill "trpc.group/trpc-go/trpc-agent-go/skill"
)

// dbSkillEntry is a cached skill loaded from the DB.
type dbSkillEntry struct {
	summary trpcskill.Summary
	slug    string
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

	// skillCache holds lazily-loaded *trpcskill.Skill keyed by slug.
	// Pointer so the entire map can be swapped atomically under mu.
	// Readers must call loadSkillCache() to get the current pointer.
	skillCache *sync.Map // string → *trpcskill.Skill
}

var _ trpcskill.Repository = (*DBRepositoryAdapter)(nil)

// NewDBRepositoryAdapter creates a DB-backed skill repository.
// ttl controls how long cached summaries are retained before a re-fetch.
func NewDBRepositoryAdapter(uc *biz.SkillUsecase, ttl time.Duration, lg loggateway.Logger) *DBRepositoryAdapter {
	if ttl <= 0 {
		ttl = 2 * time.Minute
	}
	return &DBRepositoryAdapter{
		uc:         uc,
		ttl:        ttl,
		lg:         lg,
		index:      make(map[string]int),
		skillCache: &sync.Map{},
	}
}

// loadSkillCache returns the current skillCache pointer.
// The pointer itself is immutable once read — only the map contents change
// via concurrent-safe sync.Map operations. Swaps happen under mu.
func (r *DBRepositoryAdapter) loadSkillCache() *sync.Map {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.skillCache
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

	// Fast path: check skillCache (no lock needed, sync.Map is concurrent-safe).
	cache := r.loadSkillCache()
	if cached, ok := cache.Load(key); ok {
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
		r.loadSkillCache().Store(key, sk)
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
		return "", nil // Graceful degradation
	}
	if sk.ID == "" {
		return "", nil
	}
	storageCtx, storageCancel := r.queryCtx()
	defer storageCancel()
	dir, _ := r.uc.GetStorageDir(storageCtx, sk.ID)
	return dir, nil
}

// Invalidate clears the cache, forcing a re-fetch on the next call.
func (r *DBRepositoryAdapter) Invalidate() {
	r.mu.Lock()
	r.loaded = time.Time{}
	r.skillCache = &sync.Map{}
	r.mu.Unlock()
}

// InvalidateSkillRuntimeCache implements biz skill.RuntimeCacheInvalidator（P0）。
// Skill 启用状态/可见性/正文变更后由 Usecase 主动调用，使变更立即生效，
// 而非等待 TTL（2min）兜底。
func (r *DBRepositoryAdapter) InvalidateSkillRuntimeCache() {
	r.Invalidate()
}

func (r *DBRepositoryAdapter) refreshIfStale(ctx context.Context) {
	r.mu.RLock()
	stale := time.Since(r.loaded) > r.ttl
	r.mu.RUnlock()
	if !stale {
		return
	}
	r.reload(ctx)
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
	r.mu.Unlock()

	candidates, err := r.uc.ListEnabledPublishedCandidates(ctx)
	if err != nil {
		r.lg.Warn("skill 缓存刷新失败，保留陈旧数据",
			loggateway.StepID("skill.reload_fail"),
			loggateway.Err(err))
		r.mu.Lock()
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
	// Swap entries, index, and skillCache atomically under write lock.
	// Preserve the "invalidated" state: if Invalidate() was called while we
	// were fetching, loaded will be zero — don't overwrite that with time.Now()
	// or the invalidation is silently lost.
	r.mu.Lock()
	r.entries = entries
	r.index = index
	if r.loaded.IsZero() {
		// Invalidate was called during fetch; mark as stale so next access
		// triggers another reload, but still install the fresh data.
		r.loaded = time.Time{}
	} else {
		r.loaded = time.Now()
	}
	r.skillCache = &sync.Map{}
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
