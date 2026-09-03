package biz

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// deliverable_inspection.go —— P3-1（2026-09-03 语义面防线）：成员产出语义
// 检查器（规则版 Inspector）。
//
// 背景：写入时 MDC schema 校验（member_deliverable_contract.go）保证结构合法，
// 完成期 RequiredTopicsMissing 仅 advisory；但成员「执行成功却交付了垃圾」
// （拒绝语、错误转储、空/过薄内容）会无异常地流入 synthesizer，经聚合抛光后
// 静默放大（MAS 语义面故障，错误放大 17x）。本检查器在合成前对 deliverable
// map 做纯规则体检，产出 findings 供图层注入 synthesizer 上下文
// （graph/trpc/deliverable_inspection_wiring.go），与 P1 名册/失败通告构成
// 三层语义防线。fail-open 哲学：检查只标注不阻断，判决权留给 synthesizer LLM
// 与团队级质量门。

// Deliverable finding kinds。
const (
	// DeliverableFindingMissingTopic 契约声明的 Required topic 从未被写入
	// （覆盖「成员从未调用 set_deliverable」的绕过路径）。
	DeliverableFindingMissingTopic = "missing_required_topic"
	// DeliverableFindingEmptyContent topic 已写入但内容为空/纯空白。
	DeliverableFindingEmptyContent = "empty_content"
	// DeliverableFindingThinContent 有效文本过短，不可能是真实交付物。
	DeliverableFindingThinContent = "thin_content"
	// DeliverableFindingRefusal 内容是拒绝/推脱语而非任务产出。
	DeliverableFindingRefusal = "refusal"
	// DeliverableFindingErrorDump 内容是错误转储（堆栈/报错原文），
	// 通常是工具或运行时失败被原样塞进交付物。
	DeliverableFindingErrorDump = "error_dump"
)

// DeliverableFinding 是一条结构化产出语义检查发现，LLM-actionable。
type DeliverableFinding struct {
	Topic  string `json:"topic"`
	Kind   string `json:"kind"`
	Detail string `json:"detail"`
}

// deliverableInspectThinMinRunes 是 thin_content 判定的最小有效文本长度
// （按 rune 计）。低于此长度的 topic 内容不可能是实质交付物。
const deliverableInspectThinMinRunes = 32

// 拒绝/推脱模式：中英文常见 LLM 拒答起手式。全部小写，匹配前内容 lowercase。
// 只匹配起手式（出现在前 200 rune），避免正文引用「无法」类词误伤。
var deliverableRefusalPatterns = []string{
	"无法完成", "无法访问", "无法获取", "我没有办法", "我没办法",
	"作为ai", "作为人工智能", "作为一个ai",
	"i cannot", "i can't", "i'm unable", "i am unable",
	"cannot assist", "as an ai",
}

// 错误转储模式：强特征（堆栈/运行时错误原文），正文中出现即判。全部小写。
var deliverableErrorDumpPatterns = []string{
	"traceback (most recent call last)", "panic:", "goroutine ",
	"runtime error:", "调用失败", "工具执行失败", "exec failed",
}

// deliverableInspectRefusalWindow 是拒绝模式只扫描内容开头部分的窗口
// （rune 数）——真正的拒绝语出现在开头；正文深处的引用不算。
const deliverableInspectRefusalWindow = 200

// InspectDeliverables 对团队 deliverable map 做合成前规则体检。contract 可
// 为 nil（无契约时跳过 missing_required_topic 检查）。返回 nil 表示无可疑
// 发现。纯函数，无 IO，供图层回调同步调用。
func InspectDeliverables(deliverable map[string]any, contract *MemberDeliverableContract) []DeliverableFinding {
	var out []DeliverableFinding
	// 1) 契约 Required topic 缺失。
	for _, topic := range contract.RequiredTopicsMissing(deliverable) {
		out = append(out, DeliverableFinding{
			Topic:  topic,
			Kind:   DeliverableFindingMissingTopic,
			Detail: "契约要求的 topic 从未被写入（成员未提交该部分产出）",
		})
	}
	// 2) 逐 topic 内容体检。
	for topic, v := range deliverable {
		text := extractDeliverableText(v)
		runes := utf8.RuneCountInString(strings.TrimSpace(text))
		if runes == 0 {
			out = append(out, DeliverableFinding{
				Topic:  topic,
				Kind:   DeliverableFindingEmptyContent,
				Detail: "内容为空或纯空白",
			})
			continue
		}
		lower := strings.ToLower(text)
		if kind, pat := matchDeliverableErrorDump(lower); kind != "" {
			out = append(out, DeliverableFinding{
				Topic:  topic,
				Kind:   kind,
				Detail: fmt.Sprintf("内容疑似错误转储（命中 %q），可能掩盖了执行失败", pat),
			})
			continue
		}
		head := lower
		if utf8.RuneCountInString(head) > deliverableInspectRefusalWindow {
			head = string([]rune(head)[:deliverableInspectRefusalWindow])
		}
		if pat := matchAnyPattern(head, deliverableRefusalPatterns); pat != "" {
			out = append(out, DeliverableFinding{
				Topic:  topic,
				Kind:   DeliverableFindingRefusal,
				Detail: fmt.Sprintf("内容开头疑似拒绝/推脱语（命中 %q），非任务产出", pat),
			})
			continue
		}
		if runes < deliverableInspectThinMinRunes {
			out = append(out, DeliverableFinding{
				Topic:  topic,
				Kind:   DeliverableFindingThinContent,
				Detail: fmt.Sprintf("有效内容仅 %d 字符，不足以构成实质交付物", runes),
			})
		}
	}
	return out
}

// extractDeliverableText 递归提取 topic 值中的全部文本（string/标量/[]any/
// map[string]any），拼接为单一文本供规则扫描。深度与总量双上限防畸形输入。
func extractDeliverableText(v any) string {
	var sb strings.Builder
	extractDeliverableTextInto(&sb, v, 0)
	return sb.String()
}

const (
	deliverableExtractMaxDepth = 6
	// deliverableExtractMaxBytes 是提取总量上限，按 Builder 字节数计
	// （sb.Len() 返回字节而非 rune——中文 1 rune=3 字节，命名须诚实）。
	deliverableExtractMaxBytes = 8192
)

func extractDeliverableTextInto(sb *strings.Builder, v any, depth int) {
	if sb.Len() >= deliverableExtractMaxBytes || depth > deliverableExtractMaxDepth {
		return
	}
	writePart := func(s string) {
		if sb.Len() > 0 {
			sb.WriteByte(' ')
		}
		sb.WriteString(s)
	}
	switch t := v.(type) {
	case string:
		writePart(t)
	case map[string]any:
		for _, mv := range t {
			extractDeliverableTextInto(sb, mv, depth+1)
		}
	case []any:
		for _, ev := range t {
			extractDeliverableTextInto(sb, ev, depth+1)
		}
	case nil:
		// 显式 nil 无内容，跳过（%v 会落成 "<nil>" 污染扫描文本）。
	default:
		// 标量兑底：JSON 数字（float64）、布尔等非字符串值也要进入扫描，
		// 否则 {"score": 0.95} 会被误判为 empty_content。
		writePart(fmt.Sprintf("%v", t))
	}
}

// matchDeliverableErrorDump 全文匹配错误转储强特征。返回 (kind, pattern)。
func matchDeliverableErrorDump(lower string) (string, string) {
	if pat := matchAnyPattern(lower, deliverableErrorDumpPatterns); pat != "" {
		return DeliverableFindingErrorDump, pat
	}
	return "", ""
}

func matchAnyPattern(s string, patterns []string) string {
	for _, p := range patterns {
		if strings.Contains(s, p) {
			return p
		}
	}
	return ""
}
