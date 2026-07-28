package biz

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"aranea-agents/pkg/loggateway"
)

// ---------------------------------------------------------------------------
// F9 (Phase 11): skillInstallAssertionGate — team-definition wiring
// ---------------------------------------------------------------------------

func TestSkillInstallAssertionGate_PrescribedIntentFormat(t *testing.T) {
	desc := "使用 cli_admin_skill_install_from_url 从 https://github.com/example/xlsx-skill 安装 xlsx skill，完成后用 cli_admin_skill_get 确认 enabled=true"
	g := skillInstallAssertionGate([]string{SystemAdminAgentKey}, desc)
	if g == nil {
		t.Fatal("expected a gate for single-member system-admin install task")
	}
	if g.GateType != GateTypeToolAssertion || g.Tool != "cli_admin_skill_get" {
		t.Fatalf("unexpected gate type/tool: %+v", g)
	}
	if g.AssertPath != "enabled" || g.AssertEquals != "true" {
		t.Fatalf("assertion should be enabled=true, got %s=%s", g.AssertPath, g.AssertEquals)
	}
	var args map[string]string
	if err := json.Unmarshal([]byte(g.ArgumentsJSON), &args); err != nil {
		t.Fatalf("arguments_json not parseable: %v", err)
	}
	if args["skill_key"] != "xlsx" {
		t.Fatalf("skill_key should come from the intent phrase (安装 xlsx skill), got %q", args["skill_key"])
	}
}

func TestSkillInstallAssertionGate_URLFallbackKey(t *testing.T) {
	desc := "使用 cli_admin_skill_install_from_url 从 https://github.com/example/docx-skill 完成安装并汇报结果"
	g := skillInstallAssertionGate([]string{SystemAdminAgentKey}, desc)
	if g == nil {
		t.Fatal("expected a gate when only the URL carries the skill identity")
	}
	var args map[string]string
	if err := json.Unmarshal([]byte(g.ArgumentsJSON), &args); err != nil {
		t.Fatalf("arguments_json not parseable: %v", err)
	}
	if args["skill_key"] != "docx-skill" {
		t.Fatalf("skill_key should fall back to the URL last path segment, got %q", args["skill_key"])
	}
}

func TestSkillInstallAssertionGate_Skips(t *testing.T) {
	cases := []struct {
		name string
		keys []string
		desc string
	}{
		{"non system-admin member", []string{"dept_lead_quant_trading"}, "使用 cli_admin_skill_install_from_url 安装 xlsx skill"},
		{"multi member team", []string{SystemAdminAgentKey, "agent___researcher__"}, "使用 cli_admin_skill_install_from_url 安装 xlsx skill"},
		{"no install intent", []string{SystemAdminAgentKey}, "查询已安装的 skill 列表"},
		{"no extractable key", []string{SystemAdminAgentKey}, "使用 cli_admin_skill_install_from_url 完成安装"},
	}
	for _, tc := range cases {
		if g := skillInstallAssertionGate(tc.keys, tc.desc); g != nil {
			t.Fatalf("%s: expected nil gate, got %+v", tc.name, g)
		}
	}
}

// ---------------------------------------------------------------------------
// F9 (Phase 11): executeToolAssertion — deterministic gate execution
// ---------------------------------------------------------------------------

type stubAssertionInvoker struct {
	raw     json.RawMessage
	err     error
	gotTool string
	gotArgs string
}

func (s *stubAssertionInvoker) InvokeForAssertion(_ context.Context, toolName, argumentsJSON string) (json.RawMessage, error) {
	s.gotTool, s.gotArgs = toolName, argumentsJSON
	return s.raw, s.err
}

func newToolAssertionGate(assertPath, assertEquals string) VerificationGate {
	return VerificationGate{
		GateType:      GateTypeToolAssertion,
		Tool:          "cli_admin_skill_get",
		ArgumentsJSON: `{"skill_key":"xlsx"}`,
		AssertPath:    assertPath,
		AssertEquals:  assertEquals,
	}
}

func TestExecuteToolAssertion_Pass(t *testing.T) {
	inv := &stubAssertionInvoker{raw: json.RawMessage(`{"id":"s1","skill_key":"xlsx","status":"published","enabled":true}`)}
	e := NewVerificationGateExecutor(nil, nil, loggateway.NewNoop(), WithToolAssertionInvoker(inv))
	ok, reason, err := e.ExecuteGate(context.Background(), newToolAssertionGate("enabled", "true"), "", 0)
	if err != nil || !ok {
		t.Fatalf("expected pass, got ok=%v err=%v reason=%s", ok, err, reason)
	}
	if inv.gotTool != "cli_admin_skill_get" || inv.gotArgs != `{"skill_key":"xlsx"}` {
		t.Fatalf("invoker received wrong tool/args: %s %s", inv.gotTool, inv.gotArgs)
	}
}

func TestExecuteToolAssertion_Mismatch(t *testing.T) {
	inv := &stubAssertionInvoker{raw: json.RawMessage(`{"id":"s1","skill_key":"xlsx","status":"published","enabled":false}`)}
	e := NewVerificationGateExecutor(nil, nil, loggateway.NewNoop(), WithToolAssertionInvoker(inv))
	ok, reason, err := e.ExecuteGate(context.Background(), newToolAssertionGate("enabled", "true"), "", 0)
	if err != nil || ok {
		t.Fatalf("expected deterministic rejection (not error), got ok=%v err=%v", ok, err)
	}
	if !strings.Contains(reason, "断言失败") {
		t.Fatalf("reason should explain the mismatch, got %q", reason)
	}
}

func TestExecuteToolAssertion_MissingPath(t *testing.T) {
	inv := &stubAssertionInvoker{raw: json.RawMessage(`{"id":"s1"}`)}
	e := NewVerificationGateExecutor(nil, nil, loggateway.NewNoop(), WithToolAssertionInvoker(inv))
	ok, reason, err := e.ExecuteGate(context.Background(), newToolAssertionGate("enabled", "true"), "", 0)
	if err != nil || ok {
		t.Fatalf("expected rejection on missing path, got ok=%v err=%v", ok, err)
	}
	if !strings.Contains(reason, "不存在") {
		t.Fatalf("reason should mention the missing path, got %q", reason)
	}
}

func TestExecuteToolAssertion_InvokeFailure(t *testing.T) {
	inv := &stubAssertionInvoker{err: errors.New("skill not found")}
	e := NewVerificationGateExecutor(nil, nil, loggateway.NewNoop(), WithToolAssertionInvoker(inv))
	ok, reason, err := e.ExecuteGate(context.Background(), newToolAssertionGate("enabled", "true"), "", 0)
	if err != nil || ok {
		t.Fatalf("expected rejection on invoke failure, got ok=%v err=%v", ok, err)
	}
	if !strings.Contains(reason, "调用失败") {
		t.Fatalf("reason should mention the invoke failure, got %q", reason)
	}
}

func TestExecuteToolAssertion_NotWhitelisted(t *testing.T) {
	e := NewVerificationGateExecutor(nil, nil, loggateway.NewNoop(), WithToolAssertionInvoker(&stubAssertionInvoker{}))
	gate := newToolAssertionGate("enabled", "true")
	gate.Tool = "exec_command"
	_, _, err := e.ExecuteGate(context.Background(), gate, "", 0)
	if err == nil {
		t.Fatal("non-whitelisted tool must be a configuration error")
	}
}

// ---------------------------------------------------------------------------
// F9 (Phase 11): skillAssertionInvoker — exposes enabled for the assertion
// ---------------------------------------------------------------------------

type stubSkillLister struct {
	bySlug map[string]Skill
	byID   map[string]Skill
}

func (s stubSkillLister) List(_ context.Context, _ SkillListQuery) (SkillListResult, error) {
	return SkillListResult{}, nil
}

func (s stubSkillLister) Get(_ context.Context, id string) (Skill, error) {
	if sk, ok := s.byID[id]; ok {
		return sk, nil
	}
	return Skill{}, errors.New("not found")
}

func (s stubSkillLister) GetBySlug(_ context.Context, slug string) (Skill, error) {
	if sk, ok := s.bySlug[slug]; ok {
		return sk, nil
	}
	return Skill{}, errors.New("not found")
}

func TestSkillAssertionInvoker_ReturnsEnabled(t *testing.T) {
	inv := NewSkillAssertionInvoker(stubSkillLister{
		bySlug: map[string]Skill{
			"xlsx": {ID: "s1", Slug: "xlsx", Name: "XLSX", Status: "published", Enabled: true},
		},
	})
	raw, err := inv.InvokeForAssertion(context.Background(), "cli_admin_skill_get", `{"skill_key":"xlsx"}`)
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("result not JSON: %v", err)
	}
	if doc["skill_key"] != "xlsx" {
		t.Fatalf("skill_key mismatch: %v", doc["skill_key"])
	}
	if doc["enabled"] != true {
		t.Fatalf("enabled must be exposed for the gate assertion, got %v", doc["enabled"])
	}
}

func TestSkillAssertionInvoker_UnknownSkill(t *testing.T) {
	inv := NewSkillAssertionInvoker(stubSkillLister{})
	if _, err := inv.InvokeForAssertion(context.Background(), "cli_admin_skill_get", `{"skill_key":"nope"}`); err == nil {
		t.Fatal("unknown skill must surface as an invoke error (gate then rejects)")
	}
}
