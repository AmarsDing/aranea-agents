package tool

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/internal/biz/shared"
)

func validPolicy(id string) ApprovalPolicy {
	return ApprovalPolicy{
		ID:          id,
		Scope:       string(ApprovalScopeSession),
		ScopeKey:    "sess-1",
		ToolPattern: "*",
		Mode:        string(ApprovalModeAsk),
		Priority:    100,
		Enabled:     true,
		CreatedAt:   1700000000,
	}
}

// TestValidateApprovalPolicy_BadInputRejected 钉死写入期校验（bad pattern 编译期
// 拒收——与 param rules 的 Upsert 校验同一编译函数 policyrule.MatchText）。
func TestValidateApprovalPolicy_BadInputRejected(t *testing.T) {
	t.Parallel()
	cases := map[string]func(ApprovalPolicy) ApprovalPolicy{
		"missing_id":        func(p ApprovalPolicy) ApprovalPolicy { p.ID = ""; return p },
		"bad_scope":         func(p ApprovalPolicy) ApprovalPolicy { p.Scope = "org"; return p },
		"missing_scope_key": func(p ApprovalPolicy) ApprovalPolicy { p.ScopeKey = "  "; return p },
		"bad_mode":          func(p ApprovalPolicy) ApprovalPolicy { p.Mode = "assisted"; return p },
		"empty_tool_pattern": func(p ApprovalPolicy) ApprovalPolicy {
			p.ToolPattern = ""
			return p
		},
		"bad_tool_regex": func(p ApprovalPolicy) ApprovalPolicy { p.ToolPattern = "re:["; return p },
		"bad_command_regex": func(p ApprovalPolicy) ApprovalPolicy {
			p.CommandPrefix = "re:("
			return p
		},
		"bad_args_regex": func(p ApprovalPolicy) ApprovalPolicy {
			p.ArgsPattern = "re:*unclosed"
			return p
		},
		"negative_expiry": func(p ApprovalPolicy) ApprovalPolicy { p.ExpiresAt = -1; return p },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			if err := ValidateApprovalPolicy(mutate(validPolicy("p-1"))); err == nil {
				t.Fatalf("expected rejection, got nil")
			}
		})
	}
	// 显式通配与合法 re: 前缀应收。
	ok := validPolicy("p-ok")
	ok.ToolPattern = "*"
	ok.CommandPrefix = "re:^git\\s"
	ok.ArgsPattern = "rm -rf*"
	if err := ValidateApprovalPolicy(ok); err != nil {
		t.Fatalf("valid policy rejected: %v", err)
	}
	// D5/phase 2：auto 为合法第四档，写入期必须放行。
	auto := validPolicy("p-auto")
	auto.Mode = string(ApprovalModeAuto)
	if err := ValidateApprovalPolicy(auto); err != nil {
		t.Fatalf("auto mode must be accepted (D5), got %v", err)
	}
}

// TestInMemApprovalPolicyStore_Roundtrip 覆盖读写回路、作用域过滤、编译期拒收、
// 排序确定性（Priority 升序、同档 ID 字典序）与过期/禁用过滤。
func TestInMemApprovalPolicyStore_Roundtrip(t *testing.T) {
	t.Parallel()
	s := NewInMemApprovalPolicyStore()
	ctx := context.Background()

	if err := s.UpsertApprovalPolicy(ctx, func() ApprovalPolicy {
		p := validPolicy("p-bad")
		p.ToolPattern = "re:["
		return p
	}()); err == nil {
		t.Fatal("bad pattern must be rejected at Upsert (编译期拒收)")
	}

	a := validPolicy("p-100")
	b := validPolicy("p-010")
	b.Priority = 10
	c := validPolicy("p-other-scope")
	c.Scope = string(ApprovalScopeAgent)
	c.ScopeKey = "agent-1"
	for _, p := range []ApprovalPolicy{a, b, c} {
		if err := s.UpsertApprovalPolicy(ctx, p); err != nil {
			t.Fatalf("upsert: %v", err)
		}
	}
	got, err := s.GetApprovalPolicyByID(ctx, "p-100")
	if err != nil || got.ID != "p-100" {
		t.Fatalf("get: %v %+v", err, got)
	}
	if _, err := s.GetApprovalPolicyByID(ctx, "missing"); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("missing id must return ErrNotFound, got %v", err)
	}
	rows, err := s.ListApprovalPolicies(ctx, string(ApprovalScopeSession), "sess-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 2 || rows[0].ID != "p-010" || rows[1].ID != "p-100" {
		t.Fatalf("order/scope wrong: %+v", rows)
	}
	// enabled=false 与已过期行在运行时读口不可见。
	d := validPolicy("p-disabled")
	d.Enabled = false
	e := validPolicy("p-expired")
	e.ExpiresAt = 1000
	if err := s.UpsertApprovalPolicy(ctx, d); err != nil {
		t.Fatalf("upsert disabled: %v", err)
	}
	if err := s.UpsertApprovalPolicy(ctx, e); err != nil {
		t.Fatalf("upsert expired: %v", err)
	}
	enabled, err := s.ListEnabledApprovalPolicies(ctx, string(ApprovalScopeSession), "sess-1", 5000)
	if err != nil {
		t.Fatalf("list enabled: %v", err)
	}
	if len(enabled) != 2 {
		t.Fatalf("enabled filter wrong: %+v", enabled)
	}
	// 未到期策略仍可见。
	f := validPolicy("p-future")
	f.ExpiresAt = 9999
	if err := s.UpsertApprovalPolicy(ctx, f); err != nil {
		t.Fatalf("upsert future: %v", err)
	}
	enabled, _ = s.ListEnabledApprovalPolicies(ctx, string(ApprovalScopeSession), "sess-1", 5000)
	if len(enabled) != 3 {
		t.Fatalf("future-expiry row must remain visible, got %d", len(enabled))
	}
	// 删除幂等。
	if err := s.DeleteApprovalPolicy(ctx, "p-100"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := s.DeleteApprovalPolicy(ctx, "p-100"); err != nil {
		t.Fatalf("delete must be idempotent: %v", err)
	}
	if _, err := s.GetApprovalPolicyByID(ctx, "p-100"); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("deleted row must be gone, got %v", err)
	}
}

// TestEvalApprovalPolicies_WinnerDeterminism 钉死决胜序：never > auto > ask >
// trusted（rank never=0/auto=1/ask=2/trusted=3，D5 auto as/is 独立档），同 rank
// priority 升序，再按 ID 字典序——与 policyrule.Evaluate 同序。
func TestEvalApprovalPolicies_WinnerDeterminism(t *testing.T) {
	t.Parallel()
	in := ApprovalMatchInput{ToolName: "exec_command", Command: "rm -rf /tmp/x", ArgsText: "rm -rf /tmp/x"}
	mk := func(id, mode string, prio int) ApprovalPolicy {
		p := validPolicy(id)
		p.Mode = mode
		p.Priority = prio
		return p
	}
	rows := []ApprovalPolicy{
		mk("z-trusted", string(ApprovalModeTrusted), 1),
		mk("a-ask-hi", string(ApprovalModeAsk), 50),
		mk("b-ask-lo", string(ApprovalModeAsk), 1),
		mk("c-never", string(ApprovalModeNever), 999),
	}
	enforce, shadow := EvalApprovalPolicies(rows, in, 100)
	if enforce == nil || enforce.ID != "c-never" {
		t.Fatalf("never must win over ask/trusted regardless of priority, got %+v", enforce)
	}
	if shadow != nil {
		t.Fatalf("no shadow rules, want nil, got %+v", shadow)
	}
	// never 拿掉后 auto 压过 ask（rank 1 < 2），与 priority 无关。
	auto := mk("auto-hi-prio", string(ApprovalModeAuto), 50)
	enforce, _ = EvalApprovalPolicies([]ApprovalPolicy{rows[1], rows[2], auto}, in, 100)
	if enforce == nil || enforce.ID != "auto-hi-prio" {
		t.Fatalf("auto must outrank ask regardless of priority, got %+v", enforce)
	}
	// auto 拿掉后按 priority 定 ask 胜者。
	enforce, _ = EvalApprovalPolicies(rows[0:3], in, 100)
	if enforce == nil || enforce.ID != "b-ask-lo" {
		t.Fatalf("priority asc expected, got %+v", enforce)
	}
	// never 压过 auto（rank 0 < 1）。
	enforce, _ = EvalApprovalPolicies([]ApprovalPolicy{rows[3], auto}, in, 100)
	if enforce == nil || enforce.ID != "c-never" {
		t.Fatalf("never must outrank auto, got %+v", enforce)
	}
	// 同 priority 按 ID。
	rows2 := []ApprovalPolicy{mk("b", string(ApprovalModeAsk), 1), mk("a", string(ApprovalModeAsk), 1)}
	enforce, _ = EvalApprovalPolicies(rows2, in, 100)
	if enforce == nil || enforce.ID != "a" {
		t.Fatalf("id tiebreak expected, got %+v", enforce)
	}
	// 未知 mode 不参与竞选。
	rows3 := []ApprovalPolicy{mk("x", "inherited", 0)}
	enforce, _ = EvalApprovalPolicies(rows3, in, 100)
	if enforce != nil {
		t.Fatalf("unknown mode must be skipped, got %+v", enforce)
	}
	// 过期/禁用跳过。
	exp := mk("exp", string(ApprovalModeNever), 0)
	exp.ExpiresAt = 50
	dis := mk("dis", string(ApprovalModeNever), 0)
	dis.Enabled = false
	enforce, _ = EvalApprovalPolicies([]ApprovalPolicy{exp, dis}, in, 100)
	if enforce != nil {
		t.Fatalf("expired/disabled must be skipped, got %+v", enforce)
	}
}

// TestEvalApprovalPolicies_DimensionsAndShadow 覆盖三维 AND 语义与 shadow 分流：
// command_prefix / args_pattern 任一维度不命中则整规则不命中；shadow_only 只进
// shadow 竞选，不进入 enforce。
func TestEvalApprovalPolicies_DimensionsAndShadow(t *testing.T) {
	t.Parallel()
	in := ApprovalMatchInput{ToolName: "shell_exec", Command: "git push origin main", ArgsText: "git push origin main"}
	mk := func(id string) ApprovalPolicy {
		p := validPolicy(id)
		p.Mode = string(ApprovalModeNever)
		return p
	}
	// 三维全配且全中。
	all := mk("all")
	all.ToolPattern = "shell*"
	all.CommandPrefix = "git push*"
	all.ArgsPattern = "re:^git push"
	enforce, _ := EvalApprovalPolicies([]ApprovalPolicy{all}, in, 100)
	if enforce == nil || enforce.ID != "all" {
		t.Fatalf("AND dimensions all matched, want hit, got %+v", enforce)
	}
	// command 维度不命中。
	cmiss := mk("cmiss")
	cmiss.CommandPrefix = "rm *"
	enforce, _ = EvalApprovalPolicies([]ApprovalPolicy{cmiss}, in, 100)
	if enforce != nil {
		t.Fatalf("command prefix mismatch must not match, got %+v", enforce)
	}
	// args 维度不命中。
	amiss := mk("amiss")
	amiss.ArgsPattern = "sudo *"
	enforce, _ = EvalApprovalPolicies([]ApprovalPolicy{amiss}, in, 100)
	if enforce != nil {
		t.Fatalf("args pattern mismatch must not match, got %+v", enforce)
	}
	// shadow 与 enforce 分流：shadow never 不影响 enforce ask 胜者（反之亦然）。
	sh := mk("sh")
	sh.ShadowOnly = true
	ask := validPolicy("ask-live")
	ask.Mode = string(ApprovalModeAsk)
	enforce, shadow := EvalApprovalPolicies([]ApprovalPolicy{sh, ask}, in, 100)
	if enforce == nil || enforce.ID != "ask-live" {
		t.Fatalf("shadow never must not enter enforce竞选, got %+v", enforce)
	}
	if shadow == nil || shadow.ID != "sh" {
		t.Fatalf("shadow winner expected, got %+v", shadow)
	}
}

// TestApprovalPolicyConstantsPinned 钉死 scope/mode 常量取值——迁移
// 20261291 的行语义与批 B 的 wire/读路径都依赖这些字面值不再漂移。
func TestApprovalPolicyConstantsPinned(t *testing.T) {
	t.Parallel()
	if ApprovalScopeSession != "session" || ApprovalScopeAgent != "agent" {
		t.Fatal("scope constants changed; migration 20261291 rows depend on these values")
	}
	if ApprovalModeAsk != "ask" || ApprovalModeNever != "never" || ApprovalModeTrusted != "trusted" || ApprovalModeAuto != "auto" {
		t.Fatal("mode constants changed; migration 20261291 rows depend on these values")
	}
}
