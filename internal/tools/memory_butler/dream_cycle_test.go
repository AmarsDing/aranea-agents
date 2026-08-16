package memory_butler

import (
	"encoding/json"
	"strings"
	"testing"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// ── P2-b：dream_cycle dry_run 默认 true 三态落地 ────────────────────────────
// 契约：框架 tag 解析器忽略 default= 键，值类型 bool 省略参数零值 false（真实
// 删除）；*bool 三态下 nil（模型省略参数）→ true 安全预览，仅显式 false 才执行。

func TestDreamCycleInput_DryRunDefaultsTrue(t *testing.T) {
	cases := []struct {
		name string
		args string
		want bool
	}{
		{"omitted → true（安全预览）", `{"agent_id":"a1"}`, true},
		{"explicit true → true", `{"agent_id":"a1","dry_run":true}`, true},
		{"explicit false → false（真实执行）", `{"agent_id":"a1","dry_run":false}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var in dreamCycleInput
			if err := json.Unmarshal([]byte(tc.args), &in); err != nil {
				t.Fatal(err)
			}
			if got := in.effectiveDryRun(); got != tc.want {
				t.Fatalf("effectiveDryRun() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDreamCycle_SchemaExposesDryRunSemantics(t *testing.T) {
	decl := newDreamCycleTool(Deps{}).(trpctool.CallableTool).Declaration()
	if decl == nil || decl.InputSchema == nil {
		t.Fatal("declaration/input schema missing")
	}
	prop, ok := decl.InputSchema.Properties["dry_run"]
	if !ok {
		t.Fatal("dry_run property missing from input schema")
	}
	if prop.Type != "boolean" {
		t.Fatalf("dry_run type = %q, want boolean", prop.Type)
	}
	// 默认值语义必须写进 description（模型唯一可见处）——框架忽略 default= tag 键。
	if !strings.Contains(prop.Description, "缺省") {
		t.Fatalf("dry_run description must carry default semantics, got %q", prop.Description)
	}
	for _, r := range decl.InputSchema.Required {
		if r == "dry_run" {
			t.Fatal("dry_run must NOT be required（省略即安全预览）")
		}
	}
}
