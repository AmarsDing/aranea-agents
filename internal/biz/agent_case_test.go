package biz

import (
	"strings"
	"testing"
)

// ── P3 M2: Agent Case 经验提取（EverOS Agent Memory 启发）──────────────

func caseMsg(role, content string) ConsolidateMessage {
	return ConsolidateMessage{Role: role, Content: content}
}

func longCaseInput(userMsgs int) []ConsolidateMessage {
	// 每条 user 消息 120 runes，2 条即超过 200 runes 门槛。
	body := strings.Repeat("帮我分析一下这个接口为什么超时。", 10) // 160 runes
	msgs := make([]ConsolidateMessage, 0, userMsgs*2)
	for i := 0; i < userMsgs; i++ {
		msgs = append(msgs, caseMsg("user", body), caseMsg("assistant", "好的，我来排查。"))
	}
	return msgs
}

// 预过滤：user 消息不足 2 条 → 不提取（单轮问答无任务经验价值）。
func TestShouldExtractAgentCase_TooFewUserMessages(t *testing.T) {
	msgs := []ConsolidateMessage{
		caseMsg("user", strings.Repeat("很长的问题。", 40)),
		caseMsg("assistant", "回答"),
	}
	if ShouldExtractAgentCase(msgs) {
		t.Fatal("expected false for single user message")
	}
}

// 预过滤：消息总内容不足 200 runes → 不提取（太短无法形成可靠经验）。
func TestShouldExtractAgentCase_TooShort(t *testing.T) {
	msgs := []ConsolidateMessage{
		caseMsg("user", "你好"),
		caseMsg("assistant", "你好"),
		caseMsg("user", "在吗"),
		caseMsg("assistant", "在"),
	}
	if ShouldExtractAgentCase(msgs) {
		t.Fatal("expected false for very short conversation")
	}
}

// 预过滤通过：≥2 条 user 消息且总内容 ≥200 runes。
func TestShouldExtractAgentCase_OK(t *testing.T) {
	if !ShouldExtractAgentCase(longCaseInput(2)) {
		t.Fatal("expected true for substantive multi-turn conversation")
	}
}

// 启发式降级：goal 取首条 user 消息；末条为 assistant → outcome=success。
func TestHeuristicAgentCase_Basic(t *testing.T) {
	in := ConsolidateInput{
		SessionID: "s1",
		AgentID:   "a1",
		Messages: []ConsolidateMessage{
			caseMsg("user", "帮我写一个批量导入用户的脚本"),
			{Role: "assistant", Content: "我先看下表结构"},
			{Role: "tool", Content: "users(id,name)", MessageID: "m3"},
			{Role: "tool", Content: "ok", MessageID: "m4"},
			caseMsg("assistant", "脚本写好了"),
		},
	}
	// 补上 tool name（worker 从 OptionsJSON 解析后填充）。
	in.Messages[2].ToolName = "query_db"
	in.Messages[3].ToolName = "query_db"

	c := HeuristicAgentCase(in)
	if c == nil {
		t.Fatal("expected heuristic case")
	}
	if !strings.Contains(c.Goal, "批量导入用户") {
		t.Fatalf("goal should come from first user message, got %q", c.Goal)
	}
	if c.Outcome != AgentCaseOutcomeSuccess {
		t.Fatalf("outcome=%q want success (last message is assistant)", c.Outcome)
	}
	if len(c.ToolsUsed) != 1 || c.ToolsUsed[0] != "query_db" {
		t.Fatalf("tools_used should be deduped, got %v", c.ToolsUsed)
	}
	if c.Quality != ExtractionQualityHeuristic {
		t.Fatalf("quality=%v want %v", c.Quality, ExtractionQualityHeuristic)
	}
}

// 启发式降级：末条不是 assistant（如工具中断）→ outcome=partial。
func TestHeuristicAgentCase_PartialWhenNoFinalAssistant(t *testing.T) {
	in := ConsolidateInput{
		Messages: []ConsolidateMessage{
			caseMsg("user", strings.Repeat("执行一个长任务。", 20)),
			{Role: "tool", Content: "partial output", ToolName: "shell_exec"},
		},
	}
	c := HeuristicAgentCase(in)
	if c == nil {
		t.Fatal("expected heuristic case")
	}
	if c.Outcome != AgentCaseOutcomePartial {
		t.Fatalf("outcome=%q want partial", c.Outcome)
	}
}

// 启发式降级：无 user 消息 → nil（无可提取目标）。
func TestHeuristicAgentCase_NoUserMessage(t *testing.T) {
	in := ConsolidateInput{Messages: []ConsolidateMessage{caseMsg("assistant", "系统通知")}}
	if HeuristicAgentCase(in) != nil {
		t.Fatal("expected nil when no user message")
	}
}
