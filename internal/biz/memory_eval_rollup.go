package biz

import (
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

// memory_eval_rollup.go — AMC P3-a（F1 摘要/长历史全局综合）确定性 rollup
// 事实行的纯函数层。无 LLM：从事实行确定性拼装两类派生行：
//   - profile digest（单 user 单 active 行）：身份/偏好/约束/其他分区模板渲染，
//     让「总体/人生故事/profile」类聚合查询拿到一行跨会话合成证据；
//   - slot history（每 governable 槽一条）：取值链 v1(日期)→v2(日期)（失效值
//     带标注），反哺轨迹/改口类查询。
//
// 与 P3-c 数值 digest 同一哲学：canonical 单行、前缀可识别、写侧滚动轮换、
// 读侧按查询模式确定性置顶（不经向量）。

const (
	// evalProfileDigestPrefix marks the per-user profile digest row.
	evalProfileDigestPrefix = "[profile-digest]"
	// evalSlotHistoryPrefixPrefix marks per-slot history rows (slot follows).
	evalSlotHistoryLinePrefix = "[slot-history: "

	// evalProfileMaxPerSection caps facts rendered per section.
	evalProfileMaxPerSection = 8
	// evalRollupStmtMaxRunes caps one rendered statement's text.
	evalRollupStmtMaxRunes = 80
	// evalProfileMaxTotalRunes bounds the whole digest line.
	evalProfileMaxTotalRunes = 2000
	// evalRollupSlotMaxRunes bounds the slot name inside the canonical head
	// (AMC P3 评审 R2-C5-1：超长槽名会把首个 (日期 标记挤出总长上限，
	// 使渲染行过不了自己的 canonical 正则)。
	evalRollupSlotMaxRunes = 64
)

// EvalRollupFact is the store's row view handed to the pure assemblers.
// Statement keeps its role prefix (this layer strips it for rendering);
// ValidFrom/ValidUntil are RFC3339Nano strings ("" = unknown/active).
type EvalRollupFact struct {
	Statement  string
	Kind       string
	ValidFrom  string
	ValidUntil string
}

// Canonical identity regexes for rollup rows (AMC P3 评审 C5): a bare
// HasPrefix guard would adopt a role-less user utterance that merely STARTS
// with the marker (data injection) as derived state — the rotate path could
// then invalidate it / pin it. The regexes pin the rendered head shape:
// truncation only shortens the TAIL (head always survives the rune cap), so
// matching the head is safe for every rendered row.
var (
	evalProfileDigestRe = regexp.MustCompile(`^\[profile-digest\] (?:身份|偏好|约束|其他): `)
	evalSlotHistoryRe   = regexp.MustCompile(`^\[slot-history: [^\]]+\] .+\((?:\d{4}-\d{2}-\d{2}|未知日期)`)
)

// IsEvalProfileDigestStatement marks the profile digest row on the read path.
func IsEvalProfileDigestStatement(stmt string) bool {
	return evalProfileDigestRe.MatchString(strings.TrimSpace(stmt))
}

// IsEvalSlotHistoryStatement marks slot-history rows on the read path.
func IsEvalSlotHistoryStatement(stmt string) bool {
	return evalSlotHistoryRe.MatchString(strings.TrimSpace(stmt))
}

// IsEvalRollupStatement covers every derived rollup row (P3-a + P3-c).
func IsEvalRollupStatement(stmt string) bool {
	return IsEvalProfileDigestStatement(stmt) ||
		IsEvalSlotHistoryStatement(stmt) ||
		IsEvalSeriesDigestStatement(stmt)
}

// EvalProfileDigestStatementPrefix exposes the row identity prefix for the
// store's rotate/lookup scans.
func EvalProfileDigestStatementPrefix() string { return evalProfileDigestPrefix }

// EvalSlotHistoryStatementPrefix returns the identity prefix for one slot's
// history row. The slot name is normalized for the canonical envelope
// (AMC P3 评审 R2-C5-1): 槽名来自用户输入捕获（chineseFavoriteRe 不排除
// ']'），原始 ']' 会使渲染行过不了自己的 canonical 正则（轮转永不失效旧
// 行、profile digest 把链行当源事实重吸收）；剥离括号并定长后，渲染、
// 前缀扫描、canonical 识别三者对同一槽恒定一致。
func EvalSlotHistoryStatementPrefix(slot string) string {
	return evalSlotHistoryLinePrefix + evalSanitizeRollupSlot(slot) + "]"
}

// evalSanitizeRollupSlot strips envelope-breaking brackets and bounds the
// slot name for rendering. Chosen over changing PreferenceSlotKey so slot
// governance semantics stay untouched — only the rendered head normalizes.
func evalSanitizeRollupSlot(slot string) string {
	slot = strings.Map(func(r rune) rune {
		if r == '[' || r == ']' {
			return -1
		}
		return r
	}, slot)
	r := []rune(slot)
	if len(r) > evalRollupSlotMaxRunes {
		r = r[:evalRollupSlotMaxRunes]
	}
	return string(r)
}

// AssembleEvalProfileDigest renders one user's ACTIVE facts (caller filters)
// into the canonical single-line profile digest. Section order is fixed
// (身份 / 偏好 / 约束 / 其他); within a section facts are capped to the
// latest evalProfileMaxPerSection by event time and rendered oldest→newest
// for narrative readability. Rollup rows themselves are skipped by prefix
// guard. Returns "" when there is nothing to digest.
func AssembleEvalProfileDigest(facts []EvalRollupFact) string {
	sections := []struct {
		title string
		kinds map[string]bool
	}{
		{"身份", map[string]bool{"user_identity": true}},
		{"偏好", map[string]bool{"preference": true}},
		{"约束", map[string]bool{"constraint": true, "agent_instruction": true}},
		{"其他", map[string]bool{"event": true, "domain_knowledge": true, "": true}},
	}
	assigned := make([][]EvalRollupFact, len(sections))
	for _, f := range facts {
		if f.Statement == "" || IsEvalRollupStatement(f.Statement) {
			continue
		}
		for i, sec := range sections {
			if sec.kinds[CanonicalizeFactKind(f.Kind, "")] || sec.kinds[f.Kind] {
				assigned[i] = append(assigned[i], f)
				break
			}
		}
	}
	var b strings.Builder
	b.WriteString(evalProfileDigestPrefix)
	wrote := false
	for i, sec := range sections {
		rows := assigned[i]
		if len(rows) == 0 {
			continue
		}
		sort.SliceStable(rows, func(a, c int) bool {
			if rows[a].ValidFrom != rows[c].ValidFrom {
				return rows[a].ValidFrom < rows[c].ValidFrom
			}
			return rows[a].Statement < rows[c].Statement
		})
		if len(rows) > evalProfileMaxPerSection {
			rows = rows[len(rows)-evalProfileMaxPerSection:]
		}
		if wrote {
			b.WriteString(" | ")
		}
		b.WriteString(" ")
		b.WriteString(sec.title)
		b.WriteString(": ")
		for j, r := range rows {
			if j > 0 {
				b.WriteString("；")
			}
			b.WriteString(shortenEvalRollupText(stripRolePrefix(r.Statement), evalRollupStmtMaxRunes))
		}
		wrote = true
	}
	if !wrote {
		return ""
	}
	return shortenEvalRollupText(b.String(), evalProfileMaxTotalRunes)
}

// AssembleEvalSlotHistory renders one slot's full value chain (active AND
// invalidated rows, caller supplies both) ordered by valid_from asc:
//
//	[slot-history: favorite:color] 我最喜欢的颜色是蓝色(2025-03-01, 已失效) → 我最喜欢的颜色是红色(2025-03-05)
//
// Returns "" when fewer than two entries exist — a chain means change.
func AssembleEvalSlotHistory(slot string, entries []EvalRollupFact) string {
	if slot == "" || len(entries) < 2 {
		return ""
	}
	sort.SliceStable(entries, func(a, c int) bool {
		if entries[a].ValidFrom != entries[c].ValidFrom {
			return entries[a].ValidFrom < entries[c].ValidFrom
		}
		return entries[a].Statement < entries[c].Statement
	})
	var b strings.Builder
	b.WriteString(EvalSlotHistoryStatementPrefix(slot))
	b.WriteString(" ")
	for i, e := range entries {
		if i > 0 {
			b.WriteString(" → ")
		}
		b.WriteString(shortenEvalRollupText(stripRolePrefix(e.Statement), evalRollupStmtMaxRunes))
		b.WriteString("(")
		day := e.ValidFrom
		if len(day) >= 10 {
			day = day[:10]
		}
		if day == "" {
			day = "未知日期"
		}
		b.WriteString(day)
		if strings.TrimSpace(e.ValidUntil) != "" {
			b.WriteString(", 已失效")
		}
		b.WriteString(")")
	}
	return shortenEvalRollupText(b.String(), evalProfileMaxTotalRunes)
}

// shortenEvalRollupText truncates s to maxRunes runes with an ellipsis.
func shortenEvalRollupText(s string, maxRunes int) string {
	s = strings.TrimSpace(s)
	if maxRunes <= 0 || utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	r := []rune(s)
	return string(r[:maxRunes-1]) + "…"
}
