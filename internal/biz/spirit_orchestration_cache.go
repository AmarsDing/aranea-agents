package biz

import (
	"context"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

type OrchestrationCacheEntry struct {
	TaskPattern           string       `json:"task_pattern"`
	Topology              TopologyType `json:"topology"`
	DQScore               float64      `json:"dq_score"`
	TeamCount             int          `json:"team_count"`
	AvgDurationMs         int64        `json:"avg_duration_ms"`
	AgentKeys             []string     `json:"agent_keys,omitempty"`
	DomainPath            string       `json:"domain_path,omitempty"` // 配方所属领域路径（B.10.21）；旧 JSON 无此字段加载不报错
	Specialties           []string     `json:"specialties,omitempty"` // 专题槽位骨架（配方回放重建 DAG）
	PlaybookID            string       `json:"playbook_id,omitempty"`
	ConstraintFingerprint string       `json:"constraint_fingerprint,omitempty"`
	UpdatedAt             string       `json:"updated_at"`
}

type OrchestrationCacheRepo interface {
	LoadCacheJSON(ctx context.Context) (string, error)
	SaveCacheJSON(ctx context.Context, jsonStr string) error
}

// OrchestrationCacheTTL is the default time-to-live for cache entries.
// Entries older than this are considered stale and will be evicted on access.
const OrchestrationCacheTTL = 30 * 24 * time.Hour // 30 days

type OrchestrationCacheStats struct {
	Hits   uint64 `json:"hits"`
	Misses uint64 `json:"misses"`
	Size   int    `json:"size"`
}

type OrchestrationCache struct {
	mu      sync.RWMutex
	entries map[string]*OrchestrationCacheEntry
	repo    OrchestrationCacheRepo
	lg      loggateway.Logger
	hits    atomic.Uint64
	misses  atomic.Uint64
}

func NewOrchestrationCache(lg loggateway.Logger, repo OrchestrationCacheRepo) *OrchestrationCache {
	c := &OrchestrationCache{
		entries: make(map[string]*OrchestrationCacheEntry),
		lg:      lg,
	}
	if repo != nil {
		c.repo = repo
	}
	return c
}

func (c *OrchestrationCache) InitFromRepo(ctx context.Context) {
	if c.repo == nil {
		return
	}
	jsonStr, err := c.repo.LoadCacheJSON(ctx)
	if err != nil {
		c.lg.Warn("从数据库加载编排缓存失败",
			loggateway.StepID("spirit.orchestration_cache.load"),
			loggateway.Err(err),
		)
		return
	}
	if err := c.LoadFromJSON(jsonStr); err != nil {
		c.lg.Warn("解析编排缓存 JSON 失败",
			loggateway.StepID("spirit.orchestration_cache.parse"),
			loggateway.Err(err),
		)
	}
}

func (c *OrchestrationCache) Get(taskPattern string) (*OrchestrationCacheEntry, bool) {
	c.mu.RLock()
	entry, ok := c.entries[taskPattern]
	if !ok {
		c.mu.RUnlock()
		c.misses.Add(1)
		return nil, false
	}
	// Check TTL: evict stale entries
	if isEntryStale(entry) {
		c.mu.RUnlock()
		c.mu.Lock()
		delete(c.entries, taskPattern)
		c.mu.Unlock()
		c.misses.Add(1)
		return nil, false
	}
	c.mu.RUnlock()
	c.hits.Add(1)
	return entry, true
}

// GetStats returns a snapshot of cache hit/miss statistics.
func (c *OrchestrationCache) GetStats() OrchestrationCacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return OrchestrationCacheStats{
		Hits:   c.hits.Load(),
		Misses: c.misses.Load(),
		Size:   len(c.entries),
	}
}

// EvictStale removes all entries older than OrchestrationCacheTTL.
func (c *OrchestrationCache) EvictStale(ctx context.Context) int {
	c.mu.Lock()
	evicted := 0
	for key, entry := range c.entries {
		if isEntryStale(entry) {
			delete(c.entries, key)
			evicted++
		}
	}
	c.mu.Unlock()
	if evicted > 0 {
		c.lg.Info("编排缓存过期清理",
			loggateway.StepID("spirit.orchestration_cache.evict"),
			loggateway.Int("evicted", evicted),
		)
		c.persistToRepo(ctx)
	}
	return evicted
}

func isEntryStale(entry *OrchestrationCacheEntry) bool {
	if entry == nil {
		return true
	}
	t, err := time.Parse(time.RFC3339, entry.UpdatedAt)
	if err != nil {
		return false // unparseable time → keep entry
	}
	return time.Since(t) > OrchestrationCacheTTL
}

func (c *OrchestrationCache) List() []OrchestrationCacheEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.listLocked()
}

// listLocked returns all entries. Caller must hold at least c.mu.RLock.
func (c *OrchestrationCache) listLocked() []OrchestrationCacheEntry {
	result := make([]OrchestrationCacheEntry, 0, len(c.entries))
	for _, e := range c.entries {
		result = append(result, *e)
	}
	return result
}

func (c *OrchestrationCache) LoadFromJSON(jsonStr string) error {
	if jsonStr == "" {
		return nil
	}
	var entries []OrchestrationCacheEntry
	if err := json.Unmarshal([]byte(jsonStr), &entries); err != nil {
		c.lg.Warn("加载 orchestration cache 失败", loggateway.StepID("spirit.orchestration_cache"), loggateway.Err(err))
		return apierror.Wrap(err, apierror.CodeInternal, "SPIRIT")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, e := range entries {
		c.entries[e.TaskPattern] = &e
	}
	return nil
}

func (c *OrchestrationCache) ToJSON() (string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entries := c.listLocked()
	b, err := json.Marshal(entries)
	if err != nil {
		return "", apierror.Wrap(err, apierror.CodeInternal, "SPIRIT")
	}
	return string(b), nil
}

func ComputeDQScore(teamResult TeamSynthesisResult, durationMs int64) float64 {
	if teamResult.Status != TeamStatusCompleted {
		return 0.0
	}
	breakdown := ComputeDQScoreBreakdown(teamResult, durationMs)
	return breakdown.Overall()
}

type DQScoreBreakdown struct {
	Validity    float64 `json:"validity"`
	Specificity float64 `json:"specificity"`
	Correctness float64 `json:"correctness"`
	DurationMs  int64   `json:"duration_ms"`
}

// DQ Score weight constants.
const (
	DQWeightValidity    = 0.4
	DQWeightSpecificity = 0.3
	DQWeightCorrectness = 0.3
	DQScoreMin          = 0.1

	// Specificity thresholds based on summary length.
	DQSpecificityBase     = 0.7
	DQSpecificityMedium   = 0.85 // summary > 50 chars
	DQSpecificityHigh     = 1.0  // summary > 100 chars
	DQSpecificityFindings = 0.15 // bonus for key findings

	// Time penalty constants.
	DQTimePenaltyDivisorMs = 60000.0 // convert ms to minutes
	DQTimePenaltyMax       = 5.0     // max time penalty in minutes
	DQTimePenaltyFactor    = 0.1     // penalty per minute

	// Evolution suggestion threshold.
	DQEvolutionThreshold = 0.5

	// MaxTaskPatternLen is the maximum rune length for extracted task patterns.
	MaxTaskPatternLen = 64

	// DomainRecipeMinDQ is the minimum DQ score for a cache entry to be
	// served as a reusable domain recipe (B.10.21.5 L0).
	DomainRecipeMinDQ = 0.7
)

func (b DQScoreBreakdown) Overall() float64 {
	score := b.Validity*DQWeightValidity + b.Specificity*DQWeightSpecificity + b.Correctness*DQWeightCorrectness
	if score < DQScoreMin {
		return DQScoreMin
	}
	return score
}

func ComputeDQScoreBreakdown(teamResult TeamSynthesisResult, durationMs int64) DQScoreBreakdown {
	b := DQScoreBreakdown{DurationMs: durationMs}

	if teamResult.Status == TeamStatusCompleted {
		b.Validity = 1.0
	} else {
		b.Validity = 0.0
	}

	if teamResult.Summary != "" {
		b.Specificity = DQSpecificityBase
		if len(teamResult.Summary) > 100 {
			b.Specificity = DQSpecificityHigh
		} else if len(teamResult.Summary) > 50 {
			b.Specificity = DQSpecificityMedium
		}
	}
	if teamResult.KeyFindings != "" {
		b.Specificity = min(b.Specificity+DQSpecificityFindings, 1.0)
	}

	b.Correctness = 1.0
	if durationMs > 0 {
		timePenalty := float64(durationMs) / DQTimePenaltyDivisorMs
		if timePenalty > DQTimePenaltyMax {
			timePenalty = DQTimePenaltyMax
		}
		b.Correctness -= timePenalty * DQTimePenaltyFactor
	}
	if b.Correctness < DQScoreMin {
		b.Correctness = DQScoreMin
	}

	return b
}

func (c *OrchestrationCache) RecordCompletion(ctx context.Context, taskPattern string, topology TopologyType, dqScore float64, teamCount int, avgDurationMs int64) {
	c.RecordCompletionWithAgents(ctx, taskPattern, topology, dqScore, teamCount, avgDurationMs, nil)
}

func (c *OrchestrationCache) RecordCompletionWithAgents(ctx context.Context, taskPattern string, topology TopologyType, dqScore float64, teamCount int, avgDurationMs int64, agentKeys []string) {
	c.mu.Lock()
	existing, found := c.entries[taskPattern]
	if found && existing.DQScore >= dqScore {
		c.mu.Unlock()
		return
	}
	entry := OrchestrationCacheEntry{
		TaskPattern:   taskPattern,
		Topology:      topology,
		DQScore:       dqScore,
		TeamCount:     teamCount,
		AvgDurationMs: avgDurationMs,
		AgentKeys:     agentKeys,
		Specialties:   nil,
		UpdatedAt:     time.Now().UTC().Format(time.RFC3339),
	}
	c.entries[taskPattern] = &entry
	c.mu.Unlock()

	c.lg.Info("编排缓存更新",
		loggateway.StepID("spirit.orchestration_cache"),
		loggateway.Str("task_pattern", taskPattern),
		loggateway.Str("topology", string(topology)),
	)
	c.persistToRepo(ctx)
}

// QueryByTaskPattern returns cache entries matching the task pattern derived from
// the given description. Returns entries sorted by DQScore descending.
func (c *OrchestrationCache) QueryByTaskPattern(ctx context.Context, taskDescription string) ([]OrchestrationCacheEntry, error) {
	pattern := ExtractTaskPattern(taskDescription)
	entry, found := c.Get(pattern)
	if !found {
		return nil, nil
	}
	return []OrchestrationCacheEntry{*entry}, nil
}

func (c *OrchestrationCache) persistToRepo(ctx context.Context) {
	if c.repo == nil {
		return
	}
	jsonStr, err := c.ToJSON()
	if err != nil {
		c.lg.Warn("序列化编排缓存失败",
			loggateway.StepID("spirit.orchestration_cache.marshal"),
			loggateway.Err(err),
		)
		return
	}
	if saveErr := c.repo.SaveCacheJSON(ctx, jsonStr); saveErr != nil {
		c.lg.Warn("持久化编排缓存失败",
			loggateway.StepID("spirit.orchestration_cache.save"),
			loggateway.Err(saveErr),
		)
	}
}

func (c *OrchestrationCache) SuggestTopology(taskDescription string) (TopologyType, bool) {
	pattern := ExtractTaskPattern(taskDescription)
	entry, found := c.Get(pattern)
	if !found {
		return "", false
	}
	return entry.Topology, true
}

// BestRecipeForDomain returns the highest-DQ recipe whose DomainPath
// prefix-matches the query (either direction, path-boundary aware).
// Entries must have DomainPath set, DQScore >= DomainRecipeMinDQ, non-empty
// AgentKeys, and be within TTL. Callers must pass an already-normalized
// domain path (normalization lives in internal/agent to avoid an import
// cycle; this function compares strings only).
func (c *OrchestrationCache) BestRecipeForDomain(domainPath string) (*OrchestrationCacheEntry, bool) {
	query := strings.TrimSpace(domainPath)
	if query == "" {
		return nil, false
	}
	c.mu.RLock()
	var best *OrchestrationCacheEntry
	for _, e := range c.entries {
		if e.DomainPath == "" || len(e.AgentKeys) == 0 {
			continue
		}
		if e.DQScore < DomainRecipeMinDQ {
			continue
		}
		if isEntryStale(e) {
			continue
		}
		if !domainPathPrefixMatch(query, e.DomainPath) {
			continue
		}
		if best == nil || e.DQScore > best.DQScore {
			best = e
		}
	}
	c.mu.RUnlock()
	if best == nil {
		c.misses.Add(1)
		return nil, false
	}
	c.hits.Add(1)
	cp := *best
	return &cp, true
}

// domainPathPrefixMatch reports whether two normalized domain paths are equal
// or one is a path-boundary prefix of the other ("创作" ↔ "创作/文学").
func domainPathPrefixMatch(a, b string) bool {
	if a == b {
		return true
	}
	return strings.HasPrefix(b, a+"/") || strings.HasPrefix(a, b+"/")
}

// RecordDomainRecipe records a successful orchestration as a reusable recipe
// keyed by "domain:<domainPath>" (B.10.21.7). Only a higher DQ score replaces
// an existing recipe. Empty domainPath is a no-op.
func (c *OrchestrationCache) RecordDomainRecipe(ctx context.Context, domainPath string, topology TopologyType, dqScore float64, teamCount int, agentKeys []string) {
	dp := strings.TrimSpace(domainPath)
	if dp == "" {
		return
	}
	key := "domain:" + dp
	c.mu.Lock()
	if existing, found := c.entries[key]; found && existing.DQScore >= dqScore {
		c.mu.Unlock()
		return
	}
	c.entries[key] = &OrchestrationCacheEntry{
		TaskPattern:   key,
		DomainPath:    dp,
		Topology:      topology,
		DQScore:       dqScore,
		TeamCount:     teamCount,
		AvgDurationMs: 0,
		AgentKeys:     agentKeys,
		Specialties:   nil,
		UpdatedAt:     time.Now().UTC().Format(time.RFC3339),
	}
	c.mu.Unlock()

	c.lg.Info("领域配方已记录",
		loggateway.StepID("spirit.orchestration_cache"),
		loggateway.Str("domain_path", dp),
		loggateway.Str("topology", string(topology)),
		loggateway.Float64("dq_score", dqScore),
	)
	c.persistToRepo(ctx)
}

// SetEntrySpecialties stores the specialty-slot skeleton on an existing
// cache entry so recipe replay can rebuild a multi-subtask DAG.
func (c *OrchestrationCache) SetEntrySpecialties(ctx context.Context, taskPattern string, specialties []string) {
	if c == nil || strings.TrimSpace(taskPattern) == "" {
		return
	}
	c.mu.Lock()
	if e, ok := c.entries[taskPattern]; ok {
		e.Specialties = append([]string(nil), specialties...)
	}
	c.mu.Unlock()
	c.persistToRepo(ctx)
}

func (c *OrchestrationCache) SuggestBestAlternativeTopology(taskDescription string, excludeTopology TopologyType) (TopologyType, bool) {
	pattern := ExtractTaskPattern(taskDescription)
	c.mu.RLock()
	defer c.mu.RUnlock()
	var bestEntry *OrchestrationCacheEntry
	for _, e := range c.entries {
		if e.Topology == excludeTopology {
			continue
		}
		if e.TaskPattern != pattern {
			continue
		}
		if bestEntry == nil || e.DQScore > bestEntry.DQScore {
			bestEntry = e
		}
	}
	if bestEntry == nil {
		return "", false
	}
	return bestEntry.Topology, true
}

func ExtractTaskPattern(desc string) string {
	// Use a hash prefix + truncated description to avoid collisions
	// between different tasks that share the same first N characters.
	// The hash provides uniqueness while the prefix preserves readability.
	prefix := fmt.Sprintf("%08x", sha1.Sum([]byte(desc)))[:8]
	runes := []rune(desc)
	if len(runes) > MaxTaskPatternLen {
		return prefix + ":" + string(runes[:MaxTaskPatternLen])
	}
	return prefix + ":" + desc
}
