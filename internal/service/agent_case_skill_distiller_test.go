package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
)

// ── P3 M4: Agent Case → Skill 蒸馏器（解析纯函数 + 降级语义）──────────────

// 合法 JSON → name+body。
func TestParseCaseDistillResponse_Valid(t *testing.T) {
	raw := `{
		"name": "batch-data-import",
		"body": "# 批量数据导入\n\n## 何时使用\n需要批量导入外部数据时。\n\n## 步骤\n1. 小批量试跑验证字段映射\n2. 分批提交并校验返回码\n\n##  pitfalls\n禁止全量刷新缓存。"
	}`
	name, body, err := parseCaseDistillResponse(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if name != "batch-data-import" {
		t.Fatalf("name=%q", name)
	}
	if !strings.Contains(body, "小批量试跑") {
		t.Fatalf("body missing approach, got %q", body)
	}
}

// markdown fence 包裹 → 正常解析。
func TestParseCaseDistillResponse_Fenced(t *testing.T) {
	raw := "```json\n{\"name\":\"cache-guard\",\"body\":\"# 缓存保护\\n内容足够长的技能正文内容足够长的技能正文\"}\n```"
	name, _, err := parseCaseDistillResponse(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if name != "cache-guard" {
		t.Fatalf("name=%q", name)
	}
}

// name 含非法字符 → 归一化为 slug。
func TestParseCaseDistillResponse_SlugifiesName(t *testing.T) {
	name, _, err := parseCaseDistillResponse(`{"name":"批量导入 Skill!","body":"足够长的技能正文足够长的技能正文足够长"}`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if strings.ContainsAny(name, " !") {
		t.Fatalf("name must be slugified, got %q", name)
	}
	if name == "" {
		t.Fatal("name must not be empty after slugify")
	}
}

// 空 name 或过短 body → 提取失败（调用方本轮跳过）。
func TestParseCaseDistillResponse_RejectsEmpty(t *testing.T) {
	if _, _, err := parseCaseDistillResponse(`{"name":"","body":"long enough body here"}`); !errors.Is(err, biz.ErrLLMExtractionFailed) {
		t.Fatalf("empty name: err=%v", err)
	}
	if _, _, err := parseCaseDistillResponse(`{"name":"ok-name","body":"短"}`); !errors.Is(err, biz.ErrLLMExtractionFailed) {
		t.Fatalf("short body: err=%v", err)
	}
}

// 非 JSON 垃圾 → ErrLLMExtractionFailed。
func TestParseCaseDistillResponse_Garbage(t *testing.T) {
	if _, _, err := parseCaseDistillResponse("这些都是经验，自己看着办。"); !errors.Is(err, biz.ErrLLMExtractionFailed) {
		t.Fatalf("err=%v want ErrLLMExtractionFailed", err)
	}
}

// nil LLM → ErrLLMExtractorUnavailable（Trigger 视所有 error 为 best-effort 跳过）。
func TestAgentCaseSkillDistiller_NilLLM(t *testing.T) {
	var d *AgentCaseSkillDistiller
	_, _, err := d.DistillSkillFromCases(context.Background(), "ag-1", []biz.AgentCase{{Goal: "g"}})
	if !errors.Is(err, biz.ErrLLMExtractorUnavailable) {
		t.Fatalf("err=%v want ErrLLMExtractorUnavailable", err)
	}
}

// case 摘要输入必须包含 goal/approach/pitfalls/tools（蒸馏质量依赖这些信号）。
func TestBuildCaseDistillDigest_IncludesSignals(t *testing.T) {
	cases := []biz.AgentCase{
		{Goal: "批量导入用户数据", Approach: "小批量试跑", Outcome: biz.AgentCaseOutcomeSuccess, ToolsUsed: []string{"query_db"}},
		{Goal: "修复缓存穿透", Pitfalls: "全量刷新导致雪崩", Outcome: biz.AgentCaseOutcomeFailure},
	}
	digest := buildCaseDistillDigest(cases)
	for _, want := range []string{"批量导入用户数据", "小批量试跑", "query_db", "全量刷新导致雪崩", "SUCCESS", "FAILURE"} {
		if !strings.Contains(digest, want) {
			t.Fatalf("digest missing %q, got %q", want, digest)
		}
	}
}
