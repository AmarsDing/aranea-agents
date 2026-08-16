package twinops

import (
	"strings"
	"testing"
)

func TestEmbedPortDownHint(t *testing.T) {
	cases := []struct {
		name     string
		result   any
		wantHint bool
	}{
		{
			// gns3_agent /exec 真实响应结构（无 ok 键）——2026-08-16 复验实证：
			// 此前误判 ok 缺省=false 提前 return，hint 从未内嵌。
			name:     "gns3_agent 真实结构（无 ok 键）端口 DOWN 内嵌指引",
			result:   map[string]any{"device": "sw1", "cmd": "ip link show", "output": "3: eth1: <BROADCAST,MULTICAST> mtu 1500\n  state DOWN qlen 1000"},
			wantHint: true,
		},
		{
			name:     "端口 DOWN 证据内嵌指引",
			result:   map[string]any{"ok": true, "output": "3: eth1: <BROADCAST,MULTICAST> mtu 1500\n  state DOWN qlen 1000"},
			wantHint: true,
		},
		{
			name:     "大小写不敏感（state Down）",
			result:   map[string]any{"ok": true, "output": "eth1 state Down"},
			wantHint: true,
		},
		{
			name:     "端口 UP 不加指引",
			result:   map[string]any{"ok": true, "output": "3: eth1: <BROADCAST,MULTICAST> mtu 1500\n  state UP qlen 1000"},
			wantHint: false,
		},
		{
			name:     "调用失败（ok=false）不加指引",
			result:   map[string]any{"ok": false, "error": "state down 目标不可达"},
			wantHint: false,
		},
		{
			name:     "非 map 结果跳过",
			result:   "state DOWN",
			wantHint: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			embedPortDownHint(tc.result)
			m, _ := tc.result.(map[string]any)
			_, got := m["next_action_hint"]
			if got != tc.wantHint {
				t.Fatalf("hint presence = %v, want %v", got, tc.wantHint)
			}
		})
	}
}

func TestEmbedPortDownHintMessageAndNoOverwrite(t *testing.T) {
	m := map[string]any{"ok": true, "output": "eth1 state DOWN", "next_action_hint": "上游已有指引"}
	embedPortDownHint(m)
	if m["next_action_hint"] != "上游已有指引" {
		t.Fatalf("existing hint must not be overwritten, got %v", m["next_action_hint"])
	}

	m2 := map[string]any{"ok": true, "output": "eth1 state DOWN"}
	embedPortDownHint(m2)
	hint, _ := m2["next_action_hint"].(string)
	// 文案三要素：取证已完成 / 禁止重发 / 推进下一步；且不得写死 fault_clear
	// （diagnose 只读节点也会被指引，防诱导越权）。
	for _, want := range []string{"取证已完成", "禁止", "下一步"} {
		if !strings.Contains(hint, want) {
			t.Fatalf("hint should contain %q, got %q", want, hint)
		}
	}
	if strings.Contains(hint, "fault_clear") {
		t.Fatalf("hint must not hardcode fault_clear (read-only nodes also see it), got %q", hint)
	}
}
