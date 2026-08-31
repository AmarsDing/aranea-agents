package twinops

import (
	"encoding/json"
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

// 方案 A 实证样本：2026-08-16 终验轮 gns3_exec(sw1, ip link show) 真实返回
// （控制台单行压平、lo 段头被滚动截断、eth1 管理性 DOWN + eth3 NO-CARRIER DOWN 并存）。
const linkShowNoisySample = "loopback 00:00:00:00:00:00 brd 00:00:00:00:00:00 " +
	"2: eth0: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 qdisc fq_codel master br-lan state UP qlen 1000 link/ether 0c:37:46:90:00:00 brd ff:ff:ff:ff:ff:ff " +
	"3: eth1: <BROADCAST,MULTICAST> mtu 1500 qdisc fq_codel master br-lan state DOWN qlen 1000 link/ether 0c:37:46:90:00:01 brd ff:ff:ff:ff:ff:ff " +
	"4: eth2: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 qdisc fq_codel master br-lan state UP qlen 1000 link/ether 0c:37:46:90:00:02 brd ff:ff:ff:ff:ff:ff " +
	"5: eth3: <NO-CARRIER,BROADCAST,MULTICAST,UP> mtu 1500 qdisc fq_codel master br-lan state DOWN qlen 1000 link/ether 0c:37:46:90:00:03 brd ff:ff:ff:ff:ff:ff " +
	"6: br-lan: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 qdisc noqueue state UP qlen 1000 link/ether 0c:37:46:90:00:00 brd ff:ff:ff:ff:ff:ff root@OpenWrt:~#"

func portStateByName(t *testing.T, ports []map[string]any, name string) (string, bool) {
	t.Helper()
	for _, p := range ports {
		if p["name"] == name {
			nc, _ := p["no_carrier"].(bool)
			s, _ := p["state"].(string)
			return s, nc
		}
	}
	t.Fatalf("port %s not parsed from %+v", name, ports)
	return "", false
}

func TestParseLinkShowPortsNoisySample(t *testing.T) {
	ports := parseLinkShowPorts(linkShowNoisySample)
	if len(ports) != 5 {
		t.Fatalf("want 5 ports (eth0/eth1/eth2/eth3/br-lan), got %d: %+v", len(ports), ports)
	}
	if s, nc := portStateByName(t, ports, "eth1"); s != "DOWN" || nc {
		t.Fatalf("eth1 should be DOWN without NO-CARRIER, got state=%s no_carrier=%v", s, nc)
	}
	if s, nc := portStateByName(t, ports, "eth3"); s != "DOWN" || !nc {
		t.Fatalf("eth3 should be DOWN with NO-CARRIER, got state=%s no_carrier=%v", s, nc)
	}
	if s, _ := portStateByName(t, ports, "eth0"); s != "UP" {
		t.Fatalf("eth0 should be UP, got %s", s)
	}
}

func TestEnrichExecResultNamesFaultPort(t *testing.T) {
	// gns3_agent 真实响应结构（无 ok 键）。
	m := map[string]any{"device": "sw1", "cmd": "ip link show", "output": linkShowNoisySample}
	enrichExecResult("ip link show", m)

	if _, ok := m["ports"].([]map[string]any); !ok {
		t.Fatalf("ports structured array missing: %+v", m)
	}
	hint, _ := m["next_action_hint"].(string)
	// 核心消歧：直接点名 eth1 为故障口、eth3 为 NO-CARRIER 常态，模型零判别负担。
	if !strings.Contains(hint, "eth1") {
		t.Fatalf("hint should name admin-down port eth1, got %q", hint)
	}
	if !strings.Contains(hint, "eth3") || !strings.Contains(hint, "NO-CARRIER") {
		t.Fatalf("hint should mark eth3 as NO-CARRIER normal, got %q", hint)
	}
	if strings.Contains(hint, "fault_clear") {
		t.Fatalf("hint must not hardcode fault_clear (read-only nodes also see it), got %q", hint)
	}
}

func TestEnrichExecResultSinglePortDown(t *testing.T) {
	out := "root@OpenWrt:~# ip link show eth1 3: eth1: <BROADCAST,MULTICAST> mtu 1500 qdisc fq_codel master br-lan state DOWN qlen 1000 link/ether 0c:37:46:90:00:01 brd ff:ff:ff:ff:ff:ff root@OpenWrt:~#"
	m := map[string]any{"device": "sw1", "cmd": "ip link show eth1", "output": out}
	enrichExecResult("ip link show eth1", m)
	hint, _ := m["next_action_hint"].(string)
	if !strings.Contains(hint, "eth1") {
		t.Fatalf("single-port DOWN should name eth1, got %q", hint)
	}
}

func TestEnrichExecResultAllUpNoHint(t *testing.T) {
	out := "3: eth1: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 qdisc fq_codel master br-lan state UP qlen 1000 link/ether 0c:37:46:90:00:01 brd ff:ff:ff:ff:ff:ff root@OpenWrt:~#"
	m := map[string]any{"device": "sw1", "cmd": "ip link show eth1", "output": out}
	enrichExecResult("ip link show eth1", m)
	if _, exists := m["next_action_hint"]; exists {
		t.Fatalf("state UP (复核通过场景) 不应加指引, got %v", m["next_action_hint"])
	}
	if _, ok := m["ports"].([]map[string]any); !ok {
		t.Fatalf("ports array should still be attached, got %+v", m)
	}
}

func TestEnrichExecResultOnlyCarrierDown(t *testing.T) {
	out := "5: eth3: <NO-CARRIER,BROADCAST,MULTICAST,UP> mtu 1500 qdisc fq_codel master br-lan state DOWN qlen 1000 link/ether 0c:37:46:90:00:03 brd ff:ff:ff:ff:ff:ff"
	m := map[string]any{"device": "sw1", "cmd": "ip link show eth3", "output": out}
	enrichExecResult("ip link show eth3", m)
	hint, _ := m["next_action_hint"].(string)
	if !strings.Contains(hint, "NO-CARRIER") || strings.Contains(hint, "故障端口，即修复目标") {
		t.Fatalf("carrier-only DOWN should say no admin-down port, got %q", hint)
	}
}

func TestEnrichExecResultFallbacks(t *testing.T) {
	// 非 link show 命令含 state DOWN → 回退通用指引。
	m := map[string]any{"ok": true, "output": "eth1 state Down"}
	enrichExecResult("ping -c 1 192.168.10.1", m)
	hint, _ := m["next_action_hint"].(string)
	if !strings.Contains(hint, "取证已完成") {
		t.Fatalf("non-link-show cmd should fall back to generic hint, got %q", hint)
	}
	// ok=false 错误包装跳过。
	m2 := map[string]any{"ok": false, "error": "x"}
	enrichExecResult("ip link show", m2)
	if _, exists := m2["next_action_hint"]; exists {
		t.Fatal("failed call must be skipped")
	}
	// link show 但输出解析不出端口（异常输出）→ 回退通用指引。
	m3 := map[string]any{"device": "sw1", "cmd": "ip link show", "output": "state DOWN but no parseable header"}
	enrichExecResult("ip link show", m3)
	hint3, _ := m3["next_action_hint"].(string)
	if !strings.Contains(hint3, "取证已完成") {
		t.Fatalf("unparseable output should fall back to generic hint, got %q", hint3)
	}
}

func TestGNS3ExecInput_CommandAlias(t *testing.T) {
	var in gns3ExecInput
	if err := json.Unmarshal([]byte(`{"device":"sw1","command":"ip link show"}`), &in); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got := in.resolvedCmd(); got != "ip link show" {
		t.Fatalf("command alias resolved to %q, want ip link show", got)
	}
	if err := json.Unmarshal([]byte(`{"device":"sw1","cmd":"show ip route","command":"ignored"}`), &in); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got := in.resolvedCmd(); got != "show ip route" {
		t.Fatalf("cmd must win over command, got %q", got)
	}
	empty := gns3ExecInput{Device: "sw1"}
	if got := empty.resolvedCmd(); got != "" {
		t.Fatalf("empty cmd+command must be empty, got %q", got)
	}
}

