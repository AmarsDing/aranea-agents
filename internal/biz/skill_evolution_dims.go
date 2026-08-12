package biz

import (
	"context"
	"sort"
	"strings"
	"time"
)

// ── P3 M5: 进化元数据维度（diversity dimensions，EverMind GSME 启发）───────
//
// 各 trigger 产出建议时在 Metadata 写入确定性可得的维度标签（dims 键），
// 供平台级多样性聚合（GetDiversityOverview）观测搜索塌缩——某维度长期无新
// 建议即信号。维度只取确定信号（工具名集合），不做 LLM 推断：贵且不稳定，
// 而塌缩观测只需要稳定可聚合的标签。

// EvoMetaDims 是 Metadata 中维度标签的键（JSON object，见 EvolutionDims）。
const EvoMetaDims = "dims"

// EvolutionDims 是 dims 键的结构。所有字段 omitempty：无信号的维度缺席，
// 聚合端按键存在性过滤。
type EvolutionDims struct {
	// Tools 是本建议涉及的工具名集合（归一化：trim/去重/排序）。
	Tools []string `json:"tools,omitempty"`
}

// normalizeToolNames 归一化工具名集合：trim、去空、去重、字典序排序。
// 排序保证聚合端 TopN 频次统计与测试断言稳定。全空输入返回 nil。
func normalizeToolNames(names []string) []string {
	var out []string
	seen := map[string]struct{}{}
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		out = append(out, n)
	}
	if len(out) == 0 {
		return nil
	}
	sort.Strings(out)
	return out
}

// withDimsTools 在 tools 非空（归一化后）时向 kv 写入 dims 键并返回 kv；
// 为空时不写，避免 `{}` 噪声（聚合 SQL 依赖键存在性过滤）。
func withDimsTools(kv map[string]any, tools []string) map[string]any {
	if norm := normalizeToolNames(tools); len(norm) > 0 {
		kv[EvoMetaDims] = EvolutionDims{Tools: norm}
	}
	return kv
}

// ── 多样性聚合观测（M5-B）──

// EvolutionDiversitySourceStat 是单个 trigger_source 在观测窗口内的分桶统计。
type EvolutionDiversitySourceStat struct {
	TriggerSource string
	Count         int       // 窗口内建议数
	LatestAt      time.Time // 窗口内最近建议时间；长期停滞即塌缩信号
	TopTools      []string  // dims.tools 频次 TopN（同频次按工具名字典序，保证稳定）
}

// UnifiedEvolutionDiversityReader 平台级多样性聚合（M5）：跨 target 按
// trigger_source 分桶 + dims.tools 频次 TopN。只读，best-effort。
// Stability:evolving
type UnifiedEvolutionDiversityReader interface {
	// GetDiversityOverview 聚合 since 以来的建议。topTools <= 0 时由实现
	// 给默认值。空窗口返回空切片而非错误。
	GetDiversityOverview(ctx context.Context, since time.Time, topTools int) ([]EvolutionDiversitySourceStat, error)
}
