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

	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// 编排配方缓存（OrchestrationCache）。
//
// D7 经验复用缓存契约（docs/development/87-orchestration-chain-hardening）：
//   - F1 双向学习信号：失败（dq<0.5）乘法衰减 ×0.8，≤0.1 触底淘汰；
//   - F2 指纹溯源：provenance 落库 + RecipeKeysReusable fail-closed（playbook.go）；
//   - F4 同分确定性全序：DQ 降序 → UpdatedAt 降序 → TaskPattern 字典序升序；
//   - F5 持久化分行：orchestration_cache_entries 表逐条 upsert/delete，
//     InitFromRepo 一次性 legacy JSON 导入后接 EvictStale 启动清理；
//   - F6 workspace 隔离：配方按 workspace 分桶，system 归一 default，无跨租户泄漏。
type OrchestrationCacheEntry struct {
	TaskPattern string `json:"task_pattern"`
	// WorkspaceID 配方所属租户桶（D7-F6）。空值视为 default（legacy 兼容）。
	WorkspaceID           string       `json:"workspace_id,omitempty"`
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

// RecipeProvenance 配方来源（D7-F2）：记录配方由哪个 playbook/约束指纹产出。
type RecipeProvenance struct {
	PlaybookID            string
	ConstraintFingerprint string
}

type OrchestrationCacheRepo interface {
	// LoadCacheJSON 读 legacy system_settings 整包 JSON——仅用于 InitFromRepo
	// 一次性导入（D7-F5）。新表非空后不再被调用。
	LoadCacheJSON(ctx context.Context) (string, error)
	// LoadEntries 读 orchestration_cache_entries 全量行。
	LoadEntries(ctx context.Context) ([]OrchestrationCacheEntry, error)
	UpsertEntry(ctx context.Context, entry OrchestrationCacheEntry) error
	DeleteEntry(ctx context.Context, workspaceID, taskPattern string) error
}

// OrchestrationCacheTTL is the default time-to-live for cache entries.
// Entries older than this are considered stale and will be evicted on access.
const OrchestrationCacheTTL = 30 * 24 * time.Hour // 30 days

// cacheKeySep 隔离 map 键的分隔符（ws + "\x1f" + pattern），避免与用户文本冲突。
const cacheKeySep = "\x1f"

// DQRecipeFailureDecay 失败信号乘法衰减系数（D7-F1）。
// 0.9 起始约 10 次连续失败触底；0.6 起始约 9 次。
const DQRecipeFailureDecay = 0.8

type OrchestrationCacheStats struct {
	Hits   uint64 `json:"hits"`
	Misses uint64 `json:"misses"`
	Size   int    `json:"size"`
}

type OrchestrationCache struct {
	mu      sync.RWMutex
	entries map[string]*OrchestrationCacheEntry // key = ws + "\x1f" + pattern
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

// InitFromRepo 启动灌入（D7-F5）：
//  1. 新表 orchestration_cache_entries 非空 → 直接 seed；
//  2. 否则一次性导入 legacy system_settings JSON（逐条 upsert 回新表）；
//  3. 尾调 EvictStale 做启动清理。
func (c *OrchestrationCache) InitFromRepo(ctx context.Context) {
	if c.repo == nil {
		return
	}
	entries, err := c.repo.LoadEntries(ctx)
	if err != nil {
		c.lg.Warn("加载编排配方缓存行失败",
			loggateway.StepID("spirit.orchestration_cache.load_entries"),
			loggateway.Err(err),
		)
		return
	}
	if len(entries) > 0 {
		c.seedEntries(entries)
		c.EvictStale(ctx)
		return
	}

	// legacy 一次性导入。
	jsonStr, err := c.repo.LoadCacheJSON(ctx)
	if err != nil {
		c.lg.Warn("从数据库加载编排缓存失败",
			loggateway.StepID("spirit.orchestration_cache.load"),
			loggateway.Err(err),
		)
		return
	}
	if strings.TrimSpace(jsonStr) == "" {
		c.EvictStale(ctx)
		return
	}
	if err := c.LoadFromJSON(jsonStr); err != nil {
		c.lg.Warn("解析编排缓存 JSON 失败",
			loggateway.StepID("spirit.orchestration_cache.parse"),
			loggateway.Err(err),
		)
		return
	}
	for _, e := range c.List() {
		c.persistEntry(ctx, e)
	}
	c.EvictStale(ctx)
}

// wsOf 取本次调用的租户桶（D7-F6）：缺省/system 归一 default，
// 保证后台 cron 学习的配方不会跨租户泄漏。
func wsOf(ctx context.Context) string {
	id := workspace.IDFromContext(ctx)
	return normalizeCacheWorkspace(id)
}

// normalizeCacheWorkspace 归一 workspace 标识：空 / __system__ → default。
func normalizeCacheWorkspace(id string) string {
	id = strings.TrimSpace(id)
	if id == "" || id == workspace.SystemWorkspaceID {
		return workspace.DefaultWorkspaceID
	}
	return id
}

func scopeKey(ws, pattern string) string {
	return ws + cacheKeySep + pattern
}

// Get returns an entry by task pattern from the default bucket（向后兼容的无
// ctx 入口）。租户感知的读取走 getForWorkspace / 各 Suggest* 方法。
func (c *OrchestrationCache) Get(taskPattern string) (*OrchestrationCacheEntry, bool) {
	return c.getForWorkspace(workspace.DefaultWorkspaceID, taskPattern)
}

func (c *OrchestrationCache) getForWorkspace(ws, taskPattern string) (*OrchestrationCacheEntry, bool) {
	key := scopeKey(normalizeCacheWorkspace(ws), taskPattern)
	c.mu.RLock()
	entry, ok := c.entries[key]
	if !ok {
		c.mu.RUnlock()
		c.misses.Add(1)
		return nil, false
	}
	// Check TTL: evict stale entries
	if isEntryStale(entry) {
		c.mu.RUnlock()
		c.mu.Lock()
		delete(c.entries, key)
		c.mu.Unlock()
		c.misses.Add(1)
		c.deleteRepoEntry(context.Background(), entry)
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

// EvictStale removes all entries older than OrchestrationCacheTTL（D7-F5：
// 同步对新表逐条 DeleteEntry）。
func (c *OrchestrationCache) EvictStale(ctx context.Context) int {
	c.mu.Lock()
	evictedEntries := make([]OrchestrationCacheEntry, 0)
	for key, entry := range c.entries {
		if isEntryStale(entry) {
			evictedEntries = append(evictedEntries, *entry)
			delete(c.entries, key)
		}
	}
	c.mu.Unlock()
	if len(evictedEntries) > 0 {
		c.lg.Info("编排缓存过期清理",
			loggateway.StepID("spirit.orchestration_cache.evict"),
			loggateway.Int("evicted", len(evictedEntries)),
		)
		for _, e := range evictedEntries {
			c.deleteRepoEntry(ctx, &e)
		}
	}
	return len(evictedEntries)
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

// seedEntries 批量灌入（D7-F5/F6）：空 workspace 归一 default 桶。
func (c *OrchestrationCache) seedEntries(entries []OrchestrationCacheEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, e := range entries {
		e.WorkspaceID = normalizeCacheWorkspace(e.WorkspaceID)
		c.entries[scopeKey(e.WorkspaceID, e.TaskPattern)] = &e
	}
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
	c.seedEntries(entries)
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
	c.RecordCompletionWithAgentsWithProvenance(ctx, taskPattern, topology, dqScore, teamCount, avgDurationMs, agentKeys, RecipeProvenance{})
}

// RecordCompletionWithAgentsWithProvenance 记录一次任务完成信号（D7-F1/F2）。
// dqScore < biz.DQSuccessThreshold 视为失败信号：既有条目 DQ ×0.8 衰减，≤0.1 淘汰；
// 空缓存上的失败信号为零操作（不造负面首条）。成功信号维持 strict-upgrade。
func (c *OrchestrationCache) RecordCompletionWithAgentsWithProvenance(ctx context.Context, taskPattern string, topology TopologyType, dqScore float64, teamCount int, avgDurationMs int64, agentKeys []string, prov RecipeProvenance) {
	c.recordRecipeSignal(ctx, taskPattern, "", topology, dqScore, teamCount, avgDurationMs, agentKeys, prov)
}

// QueryByTaskPattern returns cache entries matching the task pattern derived from
// the given description（D7-F6：仅查当前调用方租户桶）.
func (c *OrchestrationCache) QueryByTaskPattern(ctx context.Context, taskDescription string) ([]OrchestrationCacheEntry, error) {
	pattern := ExtractTaskPattern(taskDescription)
	entry, found := c.getForWorkspace(wsOf(ctx), pattern)
	if !found {
		return nil, nil
	}
	return []OrchestrationCacheEntry{*entry}, nil
}

// persistEntry 逐条写回新表（D7-F5）。repo 失败仅告警，不阻塞学习主路。
func (c *OrchestrationCache) persistEntry(ctx context.Context, entry OrchestrationCacheEntry) {
	if c.repo == nil {
		return
	}
	entry.WorkspaceID = normalizeCacheWorkspace(entry.WorkspaceID)
	if err := c.repo.UpsertEntry(ctx, entry); err != nil {
		c.lg.Warn("持久化编排配方失败",
			loggateway.StepID("spirit.orchestration_cache.upsert"),
			loggateway.Str("task_pattern", entry.TaskPattern),
			loggateway.Err(err),
		)
	}
}

func (c *OrchestrationCache) deleteRepoEntry(ctx context.Context, entry *OrchestrationCacheEntry) {
	if c.repo == nil || entry == nil {
		return
	}
	ws := normalizeCacheWorkspace(entry.WorkspaceID)
	if err := c.repo.DeleteEntry(ctx, ws, entry.TaskPattern); err != nil {
		c.lg.Warn("删除编排配方行失败",
			loggateway.StepID("spirit.orchestration_cache.delete"),
			loggateway.Str("task_pattern", entry.TaskPattern),
			loggateway.Str("workspace_id", ws),
			loggateway.Err(err),
		)
	}
}

func (c *OrchestrationCache) SuggestTopology(ctx context.Context, taskDescription string) (TopologyType, bool) {
	pattern := ExtractTaskPattern(taskDescription)
	entry, found := c.getForWorkspace(wsOf(ctx), pattern)
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
//
// D7-F4 同分确定性全序：DQ 降序 → UpdatedAt 降序 → TaskPattern 字典序升序，
// 任何重启/回放同输入必须选出同一条配方（消除 map 遍历随机序）。
// D7-F6：仅匹配当前调用方租户桶内的配方。
func (c *OrchestrationCache) BestRecipeForDomain(ctx context.Context, domainPath string) (*OrchestrationCacheEntry, bool) {
	query := strings.TrimSpace(domainPath)
	if query == "" {
		return nil, false
	}
	ws := wsOf(ctx)
	c.mu.RLock()
	var best *OrchestrationCacheEntry
	for _, e := range c.entries {
		if normalizeCacheWorkspace(e.WorkspaceID) != ws {
			continue
		}
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
		if best == nil || recipeEntryStrictlyBetter(e, best) {
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

// recipeEntryStrictlyBetter D7-F4 全序比较器：
// DQ 降序 → UpdatedAt（RFC3339）降序 → TaskPattern 字典序升序（不可解析时间按零值）。
func recipeEntryStrictlyBetter(a, b *OrchestrationCacheEntry) bool {
	if a.DQScore != b.DQScore {
		return a.DQScore > b.DQScore
	}
	at, _ := time.Parse(time.RFC3339, a.UpdatedAt)
	bt, _ := time.Parse(time.RFC3339, b.UpdatedAt)
	if !at.Equal(bt) {
		return at.After(bt)
	}
	return a.TaskPattern < b.TaskPattern
}

// domainPathPrefixMatch reports whether two normalized domain paths are equal
// or one is a path-boundary prefix of the other ("创作" ↔ "创作/文学").
func domainPathPrefixMatch(a, b string) bool {
	if a == b {
		return true
	}
	return strings.HasPrefix(b, a+"/") || strings.HasPrefix(a, b+"/")
}

// RecordDomainRecipe records an orchestration signal keyed by
// "domain:<domainPath>" (B.10.21.7 / D7-F1)。仅更高 DQ 的成功信号替换既有配方；
// 失败信号（dq<0.5）走乘法衰减，≤0.1 触底淘汰。Empty domainPath is a no-op.
func (c *OrchestrationCache) RecordDomainRecipe(ctx context.Context, domainPath string, topology TopologyType, dqScore float64, teamCount int, agentKeys []string) {
	c.RecordDomainRecipeWithProvenance(ctx, domainPath, topology, dqScore, teamCount, agentKeys, RecipeProvenance{})
}

// RecordDomainRecipeWithProvenance 同 RecordDomainRecipe，额外落 provenance（D7-F2）。
func (c *OrchestrationCache) RecordDomainRecipeWithProvenance(ctx context.Context, domainPath string, topology TopologyType, dqScore float64, teamCount int, agentKeys []string, prov RecipeProvenance) {
	dp := strings.TrimSpace(domainPath)
	if dp == "" {
		return
	}
	c.recordRecipeSignal(ctx, "domain:"+dp, dp, topology, dqScore, teamCount, 0, agentKeys, prov)
}

// recordRecipeSignal D7-F1 共享核心：成功 strict-upgrade；失败衰减/触底淘汰。
// key 为未加 ws 前缀的原始 pattern；domainPath 仅领域配方传入（写入条目的
// DomainPath 字段），pattern 键传空串。
func (c *OrchestrationCache) recordRecipeSignal(ctx context.Context, key, domainPath string, topology TopologyType, dqScore float64, teamCount int, avgDurationMs int64, agentKeys []string, prov RecipeProvenance) {
	ws := wsOf(ctx)
	skey := scopeKey(ws, key)
	now := time.Now().UTC().Format(time.RFC3339)

	if dqScore < DQSuccessThreshold {
		// 失败信号：降权不毁配方体——AgentKeys/Topology/provenance 保留，只衰减 DQ。
		c.mu.Lock()
		existing, found := c.entries[skey]
		if !found {
			c.mu.Unlock()
			return // 空缓存上的失败信号为零操作（不造负面首条）
		}
		decayed := existing.DQScore * DQRecipeFailureDecay
		fromDQ := existing.DQScore
		if decayed <= DQScoreMin {
			evicted := *existing
			delete(c.entries, skey)
			c.mu.Unlock()
			c.lg.Warn("编排配方失败触底淘汰",
				loggateway.StepID("spirit.orchestration_cache.evict_floor"),
				loggateway.Str("task_pattern", key),
				loggateway.Str("workspace_id", ws),
				loggateway.Float64("dq_from", fromDQ),
			)
			c.deleteRepoEntry(ctx, &evicted)
			return
		}
		existing.DQScore = decayed
		existing.UpdatedAt = now // TTL 从最近信号起算
		snapshot := *existing
		c.mu.Unlock()
		c.lg.Warn("编排配方失败降权",
			loggateway.StepID("spirit.orchestration_cache.decay"),
			loggateway.Str("task_pattern", key),
			loggateway.Str("workspace_id", ws),
			loggateway.Float64("dq_score", decayed),
		)
		c.persistEntry(ctx, snapshot)
		return
	}

	// 成功信号：strict-upgrade。
	c.mu.Lock()
	if existing, found := c.entries[skey]; found && existing.DQScore >= dqScore {
		c.mu.Unlock()
		return
	}
	entry := OrchestrationCacheEntry{
		TaskPattern:           key,
		WorkspaceID:           ws,
		DomainPath:            domainPath,
		Topology:              topology,
		DQScore:               dqScore,
		TeamCount:             teamCount,
		AvgDurationMs:         avgDurationMs,
		AgentKeys:             agentKeys,
		Specialties:           nil,
		PlaybookID:            prov.PlaybookID,
		ConstraintFingerprint: prov.ConstraintFingerprint,
		UpdatedAt:             now,
	}
	c.entries[skey] = &entry
	c.mu.Unlock()

	c.lg.Info("编排缓存更新",
		loggateway.StepID("spirit.orchestration_cache"),
		loggateway.Str("task_pattern", key),
		loggateway.Str("workspace_id", ws),
		loggateway.Str("topology", string(topology)),
		loggateway.Float64("dq_score", dqScore),
	)
	c.persistEntry(ctx, entry)
}

// SetEntrySpecialties stores the specialty-slot skeleton on an existing
// cache entry so recipe replay can rebuild a multi-subtask DAG.
func (c *OrchestrationCache) SetEntrySpecialties(ctx context.Context, taskPattern string, specialties []string) {
	if c == nil || strings.TrimSpace(taskPattern) == "" {
		return
	}
	ws := wsOf(ctx)
	skey := scopeKey(ws, taskPattern)
	c.mu.Lock()
	e, ok := c.entries[skey]
	var snapshot OrchestrationCacheEntry
	if ok {
		e.Specialties = append([]string(nil), specialties...)
		snapshot = *e
	}
	c.mu.Unlock()
	if !ok {
		return
	}
	c.persistEntry(ctx, snapshot)
}

// SuggestBestAlternativeTopology 在当前租户桶内找同 pattern 下非 exclude
// 的次优配方（D7-F4 同分确定性复用同一全序比较器）。
func (c *OrchestrationCache) SuggestBestAlternativeTopology(ctx context.Context, taskDescription string, excludeTopology TopologyType) (TopologyType, bool) {
	pattern := ExtractTaskPattern(taskDescription)
	ws := wsOf(ctx)
	c.mu.RLock()
	defer c.mu.RUnlock()
	var bestEntry *OrchestrationCacheEntry
	for _, e := range c.entries {
		if normalizeCacheWorkspace(e.WorkspaceID) != ws {
			continue
		}
		if e.Topology == excludeTopology {
			continue
		}
		if e.TaskPattern != pattern {
			continue
		}
		if bestEntry == nil || recipeEntryStrictlyBetter(e, bestEntry) {
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
