package session

import (
	"encoding/json"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"aranea-agents/internal/biz"
)

// RewriteSnapshotWithCompression rebuilds runner snapshot events after rolling summary compaction.
// taskState 非空时，在叙事摘要之前注入结构化任务状态块（先读状态，再读叙事）；
// stateAsOfTurn > 0 时状态标题带 "as of turn N" 时点标注（状态新鲜度可辨认）。
//
// anchor 是稳定前缀锚点（P3，2026-08-30）：首轮 user/assistant 原文事件，
// 渲染在摘要事件之前。锚点内容跨压缩逐字节稳定，provider 提示缓存的前缀键
// （系统提示+首轮）因此不因摘要重写而失效（r4 S09 cached 48.3k→5.8k 清崖）。
// 锚点消息永不进压缩体（loadCompressBody 已排除），不会与摘要重复计账。
func RewriteSnapshotWithCompression(snapshotJSON, mergedSummariesMarkdown string, anchor, tail []biz.ChatMessage, assistantAuthor string, taskState *biz.TaskState, stateAsOfTurn int) (string, error) {
	snapshotJSON = strings.TrimSpace(snapshotJSON)
	if snapshotJSON == "" {
		snapshotJSON = "{}"
	}
	var bundle map[string]any
	if err := json.Unmarshal([]byte(snapshotJSON), &bundle); err != nil {
		return "", err
	}
	if bundle == nil {
		bundle = map[string]any{}
	}
	summaryEvent := map[string]any{
		"author":    "user",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"content":   buildSummaryEventContent(mergedSummariesMarkdown, taskState, stateAsOfTurn),
		"role":      "system",
	}
	render := func(msgs []biz.ChatMessage) []any {
		var out []any
		for _, m := range msgs {
			role := strings.ToLower(strings.TrimSpace(m.Role))
			if role != "user" && role != "assistant" {
				continue
			}
			author := role
			if role == "assistant" {
				author = strings.TrimSpace(assistantAuthor)
				if author == "" {
					author = "agent"
				}
			}
			out = append(out, map[string]any{
				"author":    author,
				"timestamp": m.CreatedAt,
				"content":   strings.TrimSpace(m.ContentMarkdown),
				"role":      role,
			})
		}
		return out
	}
	events := render(anchor)
	events = append(events, summaryEvent)
	events = append(events, render(tail)...)
	bundle["events"] = events
	bundle["updated_at"] = time.Now().UTC().Format(time.RFC3339)
	out, err := json.Marshal(bundle)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// buildSummaryEventContent 组装压缩摘要事件正文：结构化任务状态块在前，
// 叙事摘要在后（先读状态，再读叙事）。
func buildSummaryEventContent(mergedSummariesMarkdown string, taskState *biz.TaskState, stateAsOfTurn int) string {
	header := "[Conversation summary — earlier turns compressed]"
	narrative := strings.TrimSpace(mergedSummariesMarkdown)
	if taskState == nil || taskState.Empty() {
		return header + "\n\n" + narrative
	}
	return header + "\n\n" + taskState.RenderBlockAsOf(stateAsOfTurn) + "\n\n" + narrative
}

// latestTaskState 从滚动摘要行中挑最新的非空 TaskState（跳过空串与坏 JSON），
// 返回状态及其产出时的 ToTurn（时点标注用）。rows 按时间升序；空对象（{}）不视为有效状态。
func latestTaskState(rows []biz.SessionSummary) (*biz.TaskState, int) {
	for i := len(rows) - 1; i >= 0; i-- {
		raw := strings.TrimSpace(rows[i].TaskStateJSON)
		if raw == "" {
			continue
		}
		var st biz.TaskState
		if json.Unmarshal([]byte(raw), &st) != nil {
			continue
		}
		if st.Empty() {
			continue
		}
		return &st, rows[i].ToTurn
	}
	return nil, 0
}

// marshalTaskState 序列化任务状态为 TaskStateJSON（nil 状态持久化为空串）。
func marshalTaskState(st *biz.TaskState) string {
	if st == nil || st.Empty() {
		return ""
	}
	raw, err := json.Marshal(st)
	if err != nil {
		return ""
	}
	return string(raw)
}

func mergeSessionSummariesMarkdown(rows []biz.SessionSummary) string {
	if len(rows) == 0 {
		return ""
	}
	var b strings.Builder
	for i, r := range rows {
		if i > 0 {
			b.WriteString("\n\n---\n\n")
		}
		b.WriteString(strings.TrimSpace(r.SummaryMarkdown))
	}
	return strings.TrimSpace(b.String())
}

const (
	// compressMessageMaxRunes 单条 user/assistant/system 消息在压缩 transcript 中的
	// 上限——单条超大消息不能溢出整个 chunk（否则分块失效、上下文溢出确定性失败）。
	compressMessageMaxRunes = 8000
	// compressToolResultMaxRunes 单条工具结果在压缩 transcript 中的上限
	// （工具输出体积大、对摘要的信息密度低）。
	compressToolResultMaxRunes = 1000
	// compressTruncationMarker 追加在被截断消息体末尾的标记。
	compressTruncationMarker = "…[truncated]"
)

// compressChunkMaxRunes 单次 LLM 调用发送的 transcript 渲染上限（rune）。
// 超限的压缩体按 chunk 逐块摘要，前一块的产出作为下一块的 PriorSummary
// 滚动吸收（var 而非 const：测试覆盖以强制分块）。
var compressChunkMaxRunes = 24000

func buildCompressTranscript(msgs []biz.ChatMessage) string {
	var b strings.Builder
	for _, m := range msgs {
		b.WriteString(renderCompressMessage(m))
		b.WriteString("\n\n")
	}
	return strings.TrimSpace(b.String())
}

// renderCompressMessage 渲染单条消息为 transcript 行。工具消息渲染为
// "TOOL(name): body"；所有消息按角色上限截断，防止单条消息溢出 chunk。
func renderCompressMessage(m biz.ChatMessage) string {
	role := strings.ToUpper(strings.TrimSpace(m.Role))
	content := strings.TrimSpace(m.ContentMarkdown)
	if role == "TOOL" {
		content = truncateRunes(content, compressToolResultMaxRunes)
		if name := compressToolName(m.OptionsJSON); name != "" {
			return "TOOL(" + name + "): " + content
		}
		return "TOOL: " + content
	}
	content = truncateRunes(content, compressMessageMaxRunes)
	return role + ": " + content
}

// compressToolName 从消息 OptionsJSON 中提取工具名（提取失败返回空）。
func compressToolName(optionsJSON string) string {
	s := strings.TrimSpace(optionsJSON)
	if s == "" {
		return ""
	}
	var opts struct {
		ToolName string `json:"tool_name"`
	}
	if err := json.Unmarshal([]byte(s), &opts); err != nil {
		return ""
	}
	return strings.TrimSpace(opts.ToolName)
}

// truncateRunes 按 rune 数截断并追加截断标记；未超限原样返回。
func truncateRunes(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return s
	}
	r := []rune(s)
	if len(r) <= maxRunes {
		return s
	}
	return string(r[:maxRunes]) + compressTruncationMarker
}

// splitMessagesForCompress 将压缩体按渲染 rune 数切分为有序 chunk：
// 当前 chunk 非空且加入下一条会超限时封块；单条超限消息自成一块。
func splitMessagesForCompress(msgs []biz.ChatMessage, maxRunes int) [][]biz.ChatMessage {
	if len(msgs) == 0 {
		return nil
	}
	if maxRunes <= 0 {
		return [][]biz.ChatMessage{msgs}
	}
	var chunks [][]biz.ChatMessage
	var cur []biz.ChatMessage
	curRunes := 0
	for _, m := range msgs {
		n := utf8.RuneCountInString(renderCompressMessage(m)) + 2 // "\n\n" 分隔符
		if len(cur) > 0 && curRunes+n > maxRunes {
			chunks = append(chunks, cur)
			cur = nil
			curRunes = 0
		}
		cur = append(cur, m)
		curRunes += n
	}
	if len(cur) > 0 {
		chunks = append(chunks, cur)
	}
	return chunks
}

// mergeTranscriptMessages 将工具消息按 (TurnNumber, CreatedAt) 交织进
// user/assistant 时间线，让摘要看到工具调用发生的实际位置。
func mergeTranscriptMessages(body, tools []biz.ChatMessage) []biz.ChatMessage {
	out := make([]biz.ChatMessage, 0, len(body)+len(tools))
	out = append(out, body...)
	out = append(out, tools...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].TurnNumber != out[j].TurnNumber {
			return out[i].TurnNumber < out[j].TurnNumber
		}
		return out[i].CreatedAt < out[j].CreatedAt
	})
	return out
}

func timelineUserAssistant(msgs []biz.ChatMessage) []biz.ChatMessage {
	var out []biz.ChatMessage
	for _, m := range msgs {
		r := strings.ToLower(strings.TrimSpace(m.Role))
		if r != "user" && r != "assistant" {
			continue
		}
		if strings.TrimSpace(m.ContentMarkdown) == "" {
			continue
		}
		out = append(out, m)
	}
	return out
}

// anchorMaxRunes 是稳定前缀锚点（首轮原文）的总 rune 上限：首轮异常庞大时
// 放弃锚定回退旧行为（锚点常驻 prompt，超大首轮会持续挤占上下文预算）。
const anchorMaxRunes = 4000

// splitAnchorTurn 把时间线的首个 turn（user/assistant 原文）拆为稳定前缀
// 锚点，其余为可压缩/可保留区。仅在 timeline[0].TurnNumber==1（真会话起点）
// 且锚点总体积不超限时启用；否则 anchor=nil、rest=原时间线（回退旧行为）。
// 锚点轮次永远不进压缩 body，也不进 tail（避免同一内容在快照中出现两次）。
func splitAnchorTurn(timeline []biz.ChatMessage) (anchor, rest []biz.ChatMessage) {
	if len(timeline) == 0 || timeline[0].TurnNumber != 1 {
		return nil, timeline
	}
	end := 0
	runes := 0
	for end < len(timeline) && timeline[end].TurnNumber == 1 {
		runes += utf8.RuneCountInString(timeline[end].ContentMarkdown)
		end++
	}
	if end == 0 || runes > anchorMaxRunes {
		return nil, timeline
	}
	return timeline[:end], timeline[end:]
}

func firstSummaryLine(md string) string {
	md = strings.TrimSpace(md)
	if md == "" {
		return ""
	}
	for _, line := range strings.Split(md, "\n") {
		t := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "#"))
		t = strings.TrimSpace(t)
		if t != "" {
			r := []rune(t)
			if len(r) > 160 {
				return string(r[:160]) + "…"
			}
			return t
		}
	}
	return ""
}
