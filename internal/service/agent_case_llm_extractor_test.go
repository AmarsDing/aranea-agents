package service

import (
	"errors"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
)

// ── P3 M2: Agent Case LLM 提取器（解析纯函数 + transcript 构建）─────────

// 合法 JSON → 完整 Case 字段。
func TestParseAgentCaseResponse_Valid(t *testing.T) {
	raw := `{
		"goal": "批量导入 5000 条用户数据",
		"approach": "先小批量试跑验证字段映射，再分批提交并校验每批返回码",
		"outcome": "success",
		"outcome_summary": "全部导入成功",
		"pitfalls": "",
		"tools_used": ["query_db", "shell_exec"],
		"quality": 0.9
	}`
	c, err := parseAgentCaseResponse(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if c.Goal != "批量导入 5000 条用户数据" {
		t.Fatalf("goal=%q", c.Goal)
	}
	if c.Outcome != biz.AgentCaseOutcomeSuccess {
		t.Fatalf("outcome=%q", c.Outcome)
	}
	if len(c.ToolsUsed) != 2 || c.ToolsUsed[0] != "query_db" {
		t.Fatalf("tools=%v", c.ToolsUsed)
	}
	if c.Quality != 0.9 {
		t.Fatalf("quality=%v", c.Quality)
	}
}

// LLM 判定无实质任务 → ErrAgentCaseSkip（调用方整条跳过，不走启发式）。
func TestParseAgentCaseResponse_Skip(t *testing.T) {
	_, err := parseAgentCaseResponse(`{"skip": true}`)
	if !errors.Is(err, biz.ErrAgentCaseSkip) {
		t.Fatalf("err=%v want ErrAgentCaseSkip", err)
	}
}

// markdown fence 包裹 → 正常解析。
func TestParseAgentCaseResponse_Fenced(t *testing.T) {
	raw := "```json\n{\"goal\":\"查日志定位超时\",\"outcome\":\"partial\",\"quality\":0.7}\n```"
	c, err := parseAgentCaseResponse(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if c.Goal != "查日志定位超时" || c.Outcome != biz.AgentCaseOutcomePartial {
		t.Fatalf("got %+v", c)
	}
}

// goal 为空 = LLM 没提取到任务目标 → 视为 skip。
func TestParseAgentCaseResponse_EmptyGoalSkips(t *testing.T) {
	_, err := parseAgentCaseResponse(`{"goal": "", "outcome": "success"}`)
	if !errors.Is(err, biz.ErrAgentCaseSkip) {
		t.Fatalf("err=%v want ErrAgentCaseSkip", err)
	}
}

// 非法 outcome 归一化为 partial；quality 超界截断到 [0,1]。
func TestParseAgentCaseResponse_Normalizes(t *testing.T) {
	c, err := parseAgentCaseResponse(`{"goal":"g","outcome":"great","quality":7}`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if c.Outcome != biz.AgentCaseOutcomePartial {
		t.Fatalf("outcome=%q want partial", c.Outcome)
	}
	if c.Quality != 1.0 {
		t.Fatalf("quality=%v want clamped 1.0", c.Quality)
	}
}

// 非 JSON 垃圾输出 → 提取失败错误（调用方降级启发式）。
func TestParseAgentCaseResponse_Garbage(t *testing.T) {
	_, err := parseAgentCaseResponse("I cannot help with that.")
	if !errors.Is(err, biz.ErrLLMExtractionFailed) {
		t.Fatalf("err=%v want ErrLLMExtractionFailed", err)
	}
}

// transcript 必须带工具名（tools_used 提取依赖此信号）。
func TestBuildCaseTranscript_IncludesToolNames(t *testing.T) {
	msgs := []biz.ConsolidateMessage{
		{Role: "user", Content: "帮我重启服务"},
		{Role: "assistant", Content: "好的"},
		{Role: "tool", Content: "restarted", ToolName: "shell_exec"},
		{Role: "assistant", Content: "已重启"},
	}
	tr := buildCaseTranscript(msgs)
	if !strings.Contains(tr, "shell_exec") {
		t.Fatalf("transcript should include tool name, got %q", tr)
	}
	if !strings.Contains(tr, "帮我重启服务") {
		t.Fatalf("transcript should include user content, got %q", tr)
	}
}
