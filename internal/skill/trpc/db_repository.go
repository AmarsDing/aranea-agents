package trpc

import (
	"context"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	trpcskill "trpc.group/trpc-go/trpc-agent-go/skill"
)

// dbSkillEntry is a cached skill loaded from the DB.
type dbSkillEntry struct {
	summary  trpcskill.Summary
	skill    *trpcskill.Skill
	slug     string
	dir      string
	loadedAt time.Time
}

// DBRepositoryAdapter implements trpcskill.Repository backed by the skill DB.
// Skills are loaded from biz.SkillUsecase and cached in-process with a TTL.
type DBRepositoryAdapter struct {
	uc  *biz.SkillUsecase
	ttl time.Duration

	mu      sync.RWMutex
	entries []dbSkillEntry
	index   map[string]int // slug → entries index
	loaded  time.Time
}

var _ trpcskill.Repository = (*DBRepositoryAdapter)(nil)

// NewDBRepositoryAdapter creates a DB-backed skill repository.
// ttl controls how long cached summaries are retained before a re-fetch.
func NewDBRepositoryAdapter(uc *biz.SkillUsecase, ttl time.Duration) *DBRepositoryAdapter {
	if ttl <= 0 {
		ttl = 2 * time.Minute
	}
	return &DBRepositoryAdapter{
		uc:    uc,
		ttl:   ttl,
		index: make(map[string]int),
	}
}

// Summaries returns all enabled+published skill summaries, refreshing from DB when stale.
func (r *DBRepositoryAdapter) Summaries() []trpcskill.Summary {
	r.refreshIfStale(context.Background())
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
	r.refreshIfStale(context.Background())
	r.mu.RLock()
	idx, ok := r.index[canonicalSlug(name)]
	r.mu.RUnlock()
	if !ok {
		return nil, &skillNotFoundError{name: name}
	}

	r.mu.RLock()
	entry := r.entries[idx]
	r.mu.RUnlock()

	if entry.skill != nil {
		return entry.skill, nil
	}

	body := r.loadBody(context.Background(), entry)
	sk := &trpcskill.Skill{
		Summary: entry.summary,
		Body:    body,
	}

	if body != "" {
		r.mu.Lock()
		if idx < len(r.entries) {
			r.entries[idx].skill = sk
		}
		r.mu.Unlock()
	}
	return sk, nil
}

// Path returns the on-disk storage directory for the skill (may be empty for DB-only skills).
func (r *DBRepositoryAdapter) Path(name string) (string, error) {
	r.refreshIfStale(context.Background())
	r.mu.RLock()
	defer r.mu.RUnlock()
	idx, ok := r.index[canonicalSlug(name)]
	if !ok {
		return "", &skillNotFoundError{name: name}
	}
	return r.entries[idx].dir, nil
}

// Invalidate clears the cache, forcing a re-fetch on the next call.
func (r *DBRepositoryAdapter) Invalidate() {
	r.mu.Lock()
	r.loaded = time.Time{}
	r.mu.Unlock()
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
	candidates, err := r.uc.ListEnabledPublishedCandidates(ctx)
	if err != nil {
		loggateway.Global().Warn("skill 缓存刷新失败，保留陈旧数据",
			loggateway.StepID("system.skill.reload_fail"),
			loggateway.Err(err))
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
	r.mu.Lock()
	r.entries = entries
	r.index = index
	r.loaded = time.Now()
	r.mu.Unlock()
}

func (r *DBRepositoryAdapter) loadBody(ctx context.Context, entry dbSkillEntry) string {
	sk, err := r.uc.GetBySlug(ctx, entry.slug)
	if err != nil {
		loggateway.Global().Warn("skill body 加载失败",
			loggateway.StepID("system.skill.load_body_fail"),
			loggateway.Str("slug", entry.slug),
			loggateway.Err(err))
		return ""
	}
	body, err := r.uc.GetLatestMarkdown(ctx, sk.ID)
	if err != nil {
		loggateway.Global().Warn("skill markdown 加载失败",
			loggateway.StepID("system.skill.load_markdown_fail"),
			loggateway.Str("slug", entry.slug),
			loggateway.Str("skill_id", sk.ID),
			loggateway.Err(err))
		return ""
	}
	dir, _ := r.uc.GetStorageDir(ctx, sk.ID)
	r.mu.Lock()
	if idx, ok := r.index[entry.slug]; ok && idx < len(r.entries) {
		r.entries[idx].dir = dir
	}
	r.mu.Unlock()
	return body
}

func canonicalSlug(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

type skillNotFoundError struct{ name string }

func (e *skillNotFoundError) Error() string { return "skill not found: " + e.name }
