package biz

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"aranea-agents/pkg/loggateway"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

type OrchestrationCacheEntry struct {
	TaskPattern  string     `json:"task_pattern"`
	Topology     TopologyType `json:"topology"`
	DQScore      float64    `json:"dq_score"`
	TeamCount    int        `json:"team_count"`
	AvgDurationMs int64     `json:"avg_duration_ms"`
	UpdatedAt    string     `json:"updated_at"`
}

// TECH-DEBT: OrchestrationCache 仅内存存储，服务重启后所有历史 DQ Score 丢失。
// 应在启动时从数据库加载缓存，或在 RecordCompletion 时持久化到 SystemSetting。
type OrchestrationCache struct {
	mu      sync.RWMutex
	entries map[string]*OrchestrationCacheEntry
	lg      loggateway.Logger
}

func NewOrchestrationCache(lg loggateway.Logger) *OrchestrationCache {
	return &OrchestrationCache{
		entries: make(map[string]*OrchestrationCacheEntry),
		lg:      lg,
	}
}

func (c *OrchestrationCache) Get(taskPattern string) (*OrchestrationCacheEntry, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.entries[taskPattern]
	if !ok {
		return nil, false
	}
	return entry, true
}

func (c *OrchestrationCache) Put(entry OrchestrationCacheEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	c.entries[entry.TaskPattern] = &entry
	c.lg.Info("编排缓存更新",
		loggateway.StepID("spirit.orchestration_cache"),
		loggateway.Str("task_pattern", entry.TaskPattern),
		loggateway.Str("topology", string(entry.Topology)),
	)
}

func (c *OrchestrationCache) List() []OrchestrationCacheEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()
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
		return kerrors.InternalServer("SPIRIT", "load orchestration cache: "+err.Error())
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
	entries := c.List()
	b, err := json.Marshal(entries)
	if err != nil {
		return "", kerrors.InternalServer("SPIRIT", "marshal orchestration cache: "+err.Error())
	}
	return string(b), nil
}

func ComputeDQScore(teamResult TeamSynthesisResult, durationMs int64) float64 {
	if teamResult.Status != "completed" {
		return 0.0
	}
	baseScore := 1.0
	if durationMs > 0 {
		timePenalty := float64(durationMs) / 60000.0
		if timePenalty > 5.0 {
			timePenalty = 5.0
		}
		baseScore -= timePenalty * 0.1
	}
	if baseScore < 0.1 {
		baseScore = 0.1
	}
	return baseScore
}

func (c *OrchestrationCache) RecordCompletion(ctx context.Context, taskPattern string, topology TopologyType, dqScore float64, teamCount int, avgDurationMs int64) {
	existing, found := c.Get(taskPattern)
	if found && existing.DQScore >= dqScore {
		return
	}
	c.Put(OrchestrationCacheEntry{
		TaskPattern:   taskPattern,
		Topology:      topology,
		DQScore:       dqScore,
		TeamCount:     teamCount,
		AvgDurationMs: avgDurationMs,
	})
}

func (c *OrchestrationCache) SuggestTopology(taskDescription string) (TopologyType, bool) {
	pattern := ExtractTaskPattern(taskDescription)
	entry, found := c.Get(pattern)
	if !found {
		return "", false
	}
	return entry.Topology, true
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
	if len(desc) > 64 {
		return desc[:64]
	}
	return desc
}

func FormatTopologyReason(topology TopologyType, cached bool, dag *TaskDAG) string {
	if cached {
		return fmt.Sprintf("基于历史编排缓存推荐拓扑: %s", topology)
	}
	if dag != nil {
		return fmt.Sprintf("基于任务 DAG 分析选择拓扑: %s (节点=%d, 根=%d)", topology, len(dag.Nodes), len(dag.Roots))
	}
	return fmt.Sprintf("默认拓扑: %s", topology)
}
