package tool

import (
	"context"
	"strings"
	"testing"

	"aranea-agents/internal/biz/shared"
	"aranea-agents/pkg/loggateway"
)

// fakeParamRuleStore 是 ToolParamRuleStore 的内存实现，供 biz 层校验测试。
type fakeParamRuleStore struct {
	rules   []ToolParamRule // 按 upsert 顺序追加；同 ID 覆盖
	deleted []string
}

func (f *fakeParamRuleStore) ListEnabledParamRules(_ context.Context, toolKey string) ([]ToolParamRule, error) {
	var out []ToolParamRule
	for _, r := range f.rules {
		if r.ToolKey == toolKey && r.Enabled {
			out = append(out, r)
		}
	}
	return out, nil
}

func (f *fakeParamRuleStore) ListParamRules(_ context.Context, toolKey string) ([]ToolParamRule, error) {
	var out []ToolParamRule
	for _, r := range f.rules {
		if r.ToolKey == toolKey {
			out = append(out, r)
		}
	}
	return out, nil
}

func (f *fakeParamRuleStore) GetParamRuleByID(_ context.Context, id string) (ToolParamRule, error) {
	for _, r := range f.rules {
		if r.ID == id {
			return r, nil
		}
	}
	return ToolParamRule{}, shared.ErrNotFound
}

func (f *fakeParamRuleStore) UpsertParamRule(_ context.Context, rule ToolParamRule) error {
	for i, r := range f.rules {
		if r.ID == rule.ID {
			f.rules[i] = rule
			return nil
		}
	}
	f.rules = append(f.rules, rule)
	return nil
}

func (f *fakeParamRuleStore) DeleteParamRule(_ context.Context, id string) error {
	f.deleted = append(f.deleted, id)
	for i, r := range f.rules {
		if r.ID == id {
			f.rules = append(f.rules[:i], f.rules[i+1:]...)
			return nil
		}
	}
	return nil
}

func TestCanonicalParamRuleToolKey_AliasFixedPoint(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"shell":        "exec_command", // 链式：shell → shell_exec → exec_command
		"shell_exec":   "exec_command",
		"exec_command": "exec_command",
		"gns3_exec":    "gns3_exec", // 无别名原样
		"write_file":   "save_file",
		"  shell  ":    "exec_command", // trim
		"":             "",
	}
	for in, want := range cases {
		if got := CanonicalParamRuleToolKey(in); got != want {
			t.Errorf("CanonicalParamRuleToolKey(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestUpsertToolParamRule_AliasCanonicalized(t *testing.T) {
	t.Parallel()
	store := &fakeParamRuleStore{}
	u := NewToolUsecase(nil, nil, loggateway.NewNoop(), WithToolParamRuleStore(store))
	err := u.UpsertToolParamRule(context.Background(), ToolParamRule{
		ID: "r1", ToolKey: "shell", Pattern: "rm -rf*", Effect: "deny", Priority: 10, Enabled: true,
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if len(store.rules) != 1 || store.rules[0].ToolKey != "exec_command" {
		t.Fatalf("stored rules = %+v, want one row canonicalized to exec_command", store.rules)
	}
	if store.rules[0].CreatedAt == 0 {
		t.Fatal("CreatedAt should be backfilled")
	}
}

func TestUpsertToolParamRule_Validation(t *testing.T) {
	t.Parallel()
	store := &fakeParamRuleStore{}
	u := NewToolUsecase(nil, nil, loggateway.NewNoop(), WithToolParamRuleStore(store))
	ctx := context.Background()
	base := ToolParamRule{ID: "r1", ToolKey: "gns3_exec", Pattern: "show *", Effect: "allow", Enabled: true}

	cases := []struct {
		name  string
		mut   func(*ToolParamRule)
		want  string
	}{
		{"空 ID", func(r *ToolParamRule) { r.ID = "" }, "required"},
		{"空 pattern", func(r *ToolParamRule) { r.Pattern = " " }, "required"},
		{"非法 effect", func(r *ToolParamRule) { r.Effect = "block" }, "deny|ask|allow"},
		{"坏正则", func(r *ToolParamRule) { r.Pattern = "re:[" }, "不可编译"},
	}
	for _, tt := range cases {
		r := base
		tt.mut(&r)
		err := u.UpsertToolParamRule(ctx, r)
		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s: err = %v, want contains %q", tt.name, err, tt.want)
		}
	}
	if len(store.rules) != 0 {
		t.Fatalf("validation failures must not persist, got %+v", store.rules)
	}
}

func TestUpsertToolParamRule_BuiltinEffectReadonly(t *testing.T) {
	t.Parallel()
	store := &fakeParamRuleStore{rules: []ToolParamRule{
		{ID: "builtin-gns3-allow", ToolKey: "gns3_exec", Pattern: "show *", Effect: "allow", Enabled: true},
	}}
	u := NewToolUsecase(nil, nil, loggateway.NewNoop(), WithToolParamRuleStore(store))
	ctx := context.Background()

	// 改 effect → 拒绝。
	err := u.UpsertToolParamRule(ctx, ToolParamRule{
		ID: "builtin-gns3-allow", ToolKey: "gns3_exec", Pattern: "show *", Effect: "deny", Enabled: true,
	})
	if err == nil || !strings.Contains(err.Error(), "builtin 规则 effect 只读") {
		t.Fatalf("effect change err = %v, want builtin readonly", err)
	}
	// 改 pattern/priority/enabled → 允许。
	err = u.UpsertToolParamRule(ctx, ToolParamRule{
		ID: "builtin-gns3-allow", ToolKey: "gns3_exec", Pattern: "show ip *", Effect: "allow", Priority: 5, Enabled: false,
	})
	if err != nil {
		t.Fatalf("pattern/priority/enabled change: %v", err)
	}
	if store.rules[0].Pattern != "show ip *" || store.rules[0].Enabled {
		t.Fatalf("builtin row = %+v, want pattern updated + disabled", store.rules[0])
	}
}

// TestUpsertToolParamRule_BuiltinToolKeySwapBlocked 钉死 H3 回归：builtin 行
// 经「同 Upsert 换 tool_key」搬走并改写 effect 的绕过路径必须被拒绝——
// 校验按 ID 全表查（非按新 tool_key 查）。
func TestUpsertToolParamRule_BuiltinToolKeySwapBlocked(t *testing.T) {
	t.Parallel()
	store := &fakeParamRuleStore{rules: []ToolParamRule{
		{ID: "builtin-exec-deny-mkfs", ToolKey: "exec_command", Pattern: "mkfs*", Effect: "deny", Enabled: true},
	}}
	u := NewToolUsecase(nil, nil, loggateway.NewNoop(), WithToolParamRuleStore(store))
	ctx := context.Background()

	// 换 tool_key + 改 effect（审计 H3 的攻击形态）→ 拒绝（tool_key 只读先于 effect 命中）。
	err := u.UpsertToolParamRule(ctx, ToolParamRule{
		ID: "builtin-exec-deny-mkfs", ToolKey: "gns3_exec", Pattern: "*", Effect: "allow", Enabled: true,
	})
	if err == nil || !strings.Contains(err.Error(), "tool_key 只读") {
		t.Fatalf("tool_key swap err = %v, want builtin tool_key readonly", err)
	}
	// 仅换 tool_key（effect 不变）→ 同样拒绝。
	err = u.UpsertToolParamRule(ctx, ToolParamRule{
		ID: "builtin-exec-deny-mkfs", ToolKey: "gns3_exec", Pattern: "mkfs*", Effect: "deny", Enabled: true,
	})
	if err == nil || !strings.Contains(err.Error(), "tool_key 只读") {
		t.Fatalf("tool_key-only swap err = %v, want builtin tool_key readonly", err)
	}
	// 原行未被搬走/改写。
	row, gerr := store.GetParamRuleByID(ctx, "builtin-exec-deny-mkfs")
	if gerr != nil || row.ToolKey != "exec_command" || row.Effect != "deny" {
		t.Fatalf("builtin row = %+v (err %v), want untouched exec_command/deny", row, gerr)
	}
	// 新建 builtin-* ID（库中尚不存在）→ 允许（补种场景）。
	err = u.UpsertToolParamRule(ctx, ToolParamRule{
		ID: "builtin-new-seed", ToolKey: "gns3_exec", Pattern: "show *", Effect: "allow", Enabled: true,
	})
	if err != nil {
		t.Fatalf("new builtin id seed: %v", err)
	}
}

func TestDeleteToolParamRule_BuiltinProtected(t *testing.T) {
	t.Parallel()
	store := &fakeParamRuleStore{rules: []ToolParamRule{
		{ID: "builtin-x", ToolKey: "gns3_exec", Pattern: "*", Effect: "ask"},
		{ID: "custom-x", ToolKey: "gns3_exec", Pattern: "*", Effect: "ask"},
	}}
	u := NewToolUsecase(nil, nil, loggateway.NewNoop(), WithToolParamRuleStore(store))
	ctx := context.Background()

	if err := u.DeleteToolParamRule(ctx, "builtin-x"); err == nil {
		t.Fatal("builtin delete should be rejected")
	}
	if err := u.DeleteToolParamRule(ctx, "custom-x"); err != nil {
		t.Fatalf("custom delete: %v", err)
	}
	if len(store.rules) != 1 || store.rules[0].ID != "builtin-x" {
		t.Fatalf("rules = %+v, want only builtin-x left", store.rules)
	}
}

func TestListEnabledParamRulesForGate_NilStore(t *testing.T) {
	t.Parallel()
	u := NewToolUsecase(nil, nil, loggateway.NewNoop())
	rules, err := u.ListEnabledParamRulesForGate(context.Background(), "gns3_exec")
	if err != nil || rules != nil {
		t.Fatalf("nil store = (%v, %v), want (nil, nil)", rules, err)
	}
}
