package biz

import "strings"

// ─── P2-4 中期 project-state JSON（Ensemble QSP 有界记忆对齐）─────────────────
//
// Team/长任务场景的结构化项目状态：活跃请求 / 最近变更 / 里程碑 / 决策摘要。
// 各字段滚动封顶，注入时按切片预算渲染，替代对话历史全量拼接——注入量与
// 项目时长解耦。状态以 JSON 兼容 map 存入 graph state（ProjectStateKey，
// MergeReducer），随团队 run 滚动更新。

// ProjectStateKey 是 project-state 在 graph state 中的字段名。
const ProjectStateKey = "project_state"

// 字段封顶与截断常量。值取自 Ensemble QSP 有界记忆实测口径（中位注入
// ~300 token）：任何单字段都不允许把切片预算独吞。
const (
	// ProjectStateMaxActive 活跃请求条目上限。
	ProjectStateMaxActive = 8
	// ProjectStateMaxRecent 最近变更条目上限。
	ProjectStateMaxRecent = 8
	// ProjectStateMaxMilestones 里程碑条目上限。
	ProjectStateMaxMilestones = 8
	// ProjectStateMaxDigestRunes 决策摘要字符上限（rune）。
	ProjectStateMaxDigestRunes = 400
	// projectStateChangeSummaryRunes 单条变更摘要字符上限（rune）。
	projectStateChangeSummaryRunes = 120
)

// ProjectStateChange 是最近变更滚动窗中的一条记录。
type ProjectStateChange struct {
	Actor   string `json:"actor"`
	Summary string `json:"summary"`
}

// TeamProjectState 是团队级中期项目状态（有界）。
// 零值可用；所有写方法自带封顶/截断，调用方无需预校验。
type TeamProjectState struct {
	ActiveRequests []string             `json:"active_requests,omitempty"`
	RecentChanges  []ProjectStateChange `json:"recent_changes,omitempty"` // 最新在前
	Milestones     []string             `json:"milestones,omitempty"`     // 最新在前
	DecisionDigest string               `json:"decision_digest,omitempty"`
}

// RollChange 滚动记录一条成员变更（最新在前，超帽丢最旧）。
func (ps *TeamProjectState) RollChange(actor, summary string) {
	summary = hardTruncateRunes(strings.TrimSpace(summary), projectStateChangeSummaryRunes)
	if summary == "" {
		return
	}
	entry := ProjectStateChange{Actor: strings.TrimSpace(actor), Summary: summary}
	ps.RecentChanges = append([]ProjectStateChange{entry}, ps.RecentChanges...)
	if len(ps.RecentChanges) > ProjectStateMaxRecent {
		ps.RecentChanges = ps.RecentChanges[:ProjectStateMaxRecent]
	}
}

// SetActiveRequests 全量替换活跃请求集（封顶取前 N 条，保持到达顺序）。
func (ps *TeamProjectState) SetActiveRequests(reqs []string) {
	out := make([]string, 0, ProjectStateMaxActive)
	for _, r := range reqs {
		if len(out) >= ProjectStateMaxActive {
			break
		}
		if s := strings.TrimSpace(r); s != "" {
			out = append(out, s)
		}
	}
	ps.ActiveRequests = out
}

// RecordMilestone 记录一个里程碑（最新在前，超帽丢最旧）。
func (ps *TeamProjectState) RecordMilestone(m string) {
	m = strings.TrimSpace(m)
	if m == "" {
		return
	}
	ps.Milestones = append([]string{m}, ps.Milestones...)
	if len(ps.Milestones) > ProjectStateMaxMilestones {
		ps.Milestones = ps.Milestones[:ProjectStateMaxMilestones]
	}
}

// SetDecisionDigest 设置决策摘要（按 ProjectStateMaxDigestRunes 截断）。
func (ps *TeamProjectState) SetDecisionDigest(digest string) {
	ps.DecisionDigest = hardTruncateRunes(strings.TrimSpace(digest), ProjectStateMaxDigestRunes)
}

// RenderSlice 按预算（rune 数）渲染注入切片。budget <= 0 视为不限。
// 优先级：活跃请求 > 决策摘要 > 里程碑 > 最近变更——活跃请求在任何
// 裁剪下都不丢；其余段按序填充，装不下即整体截断到预算。
func (ps TeamProjectState) RenderSlice(budget int) string {
	if len(ps.ActiveRequests) == 0 && len(ps.RecentChanges) == 0 &&
		len(ps.Milestones) == 0 && ps.DecisionDigest == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString("[项目状态]\n")
	// 活跃请求：最高优先级，单行拼接（条目数已封顶，单行有界）。
	if len(ps.ActiveRequests) > 0 {
		b.WriteString("活跃请求：")
		b.WriteString(strings.Join(ps.ActiveRequests, "；"))
		b.WriteString("\n")
	}
	if ps.DecisionDigest != "" {
		b.WriteString("决策摘要：")
		b.WriteString(ps.DecisionDigest)
		b.WriteString("\n")
	}
	if len(ps.Milestones) > 0 {
		b.WriteString("里程碑：")
		b.WriteString(strings.Join(ps.Milestones, "；"))
		b.WriteString("\n")
	}
	for _, c := range ps.RecentChanges {
		b.WriteString("- [")
		b.WriteString(c.Actor)
		b.WriteString("] ")
		b.WriteString(c.Summary)
		b.WriteString("\n")
	}
	out := strings.TrimRight(b.String(), "\n")
	if budget > 0 {
		out = hardTruncateRunes(out, budget)
	}
	return out
}

// ToMap 导出 JSON 兼容 map（graph state / session state 存储形态）。
func (ps TeamProjectState) ToMap() map[string]any {
	m := make(map[string]any, 4)
	if len(ps.ActiveRequests) > 0 {
		reqs := make([]any, len(ps.ActiveRequests))
		for i, r := range ps.ActiveRequests {
			reqs[i] = r
		}
		m["active_requests"] = reqs
	}
	if len(ps.RecentChanges) > 0 {
		changes := make([]any, len(ps.RecentChanges))
		for i, c := range ps.RecentChanges {
			changes[i] = map[string]any{"actor": c.Actor, "summary": c.Summary}
		}
		m["recent_changes"] = changes
	}
	if len(ps.Milestones) > 0 {
		ms := make([]any, len(ps.Milestones))
		for i, s := range ps.Milestones {
			ms[i] = s
		}
		m["milestones"] = ms
	}
	if ps.DecisionDigest != "" {
		m["decision_digest"] = ps.DecisionDigest
	}
	return m
}

// TeamProjectStateFromMap 从 JSON 兼容 map 还原状态。nil/垃圾输入容忍：
// 无法解析的条目跳过，绝不 panic。还原结果不重新封顶（写入侧已保证）。
func TeamProjectStateFromMap(m map[string]any) TeamProjectState {
	var ps TeamProjectState
	if m == nil {
		return ps
	}
	ps.ActiveRequests = stringSliceFromAny(m["active_requests"])
	for _, item := range anySlice(m["recent_changes"]) {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		summary, _ := entry["summary"].(string)
		if summary == "" {
			continue
		}
		actor, _ := entry["actor"].(string)
		ps.RecentChanges = append(ps.RecentChanges, ProjectStateChange{Actor: actor, Summary: summary})
	}
	ps.Milestones = stringSliceFromAny(m["milestones"])
	if d, ok := m["decision_digest"].(string); ok {
		ps.DecisionDigest = d
	}
	return ps
}

// hardTruncateRunes 严格截断到 max runes（不加省略号，不越帽）。
// 与 l1_field_extraction.go 的 truncateRunes（附 "…" 且按字节快路径）不同，
// 本函数服务注入预算硬约束：返回值 rune 数保证 ≤ max。
func hardTruncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}

func stringSliceFromAny(v any) []string {
	switch items := v.(type) {
	case []string:
		out := make([]string, 0, len(items))
		for _, s := range items {
			if s = strings.TrimSpace(s); s != "" {
				out = append(out, s)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(items))
		for _, it := range items {
			if s, ok := it.(string); ok {
				if s = strings.TrimSpace(s); s != "" {
					out = append(out, s)
				}
			}
		}
		return out
	}
	return nil
}

func anySlice(v any) []any {
	if items, ok := v.([]any); ok {
		return items
	}
	return nil
}
