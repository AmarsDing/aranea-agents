package policyrule

import "testing"

// 79 Phase 5 验证：deny/ask/allow/fallback 优先级表驱动测试。
func TestEvaluate_Precedence(t *testing.T) {
	rules := []Rule{
		{ID: "allow-ls", Pattern: "ls *", Effect: EffectAllow, Priority: 10, Enabled: true},
		{ID: "ask-git", Pattern: "git *", Effect: EffectAsk, Priority: 50, Enabled: true},
		{ID: "deny-rm", Pattern: "rm -rf*", Effect: EffectDeny, Priority: 10, Enabled: true},
		{ID: "deny-mkfs", Pattern: "mkfs*", Effect: EffectDeny, Priority: 20, Enabled: true},
		{ID: "allow-rm-late", Pattern: "rm *", Effect: EffectAllow, Priority: 5, Enabled: true},
		{ID: "disabled-deny", Pattern: "ls *", Effect: EffectDeny, Priority: 1, Enabled: false},
	}
	cases := []struct {
		name string
		text string
		want string // 命中规则 ID；"" = fallback（无命中）
	}{
		{"deny 优先于 allow（即使 allow priority 更小）", "rm -rf /tmp/x", "deny-rm"},
		{"deny 内 priority 升序", "mkfs.ext4 /dev/sda", "deny-mkfs"},
		{"ask 命中", "git push origin main", "ask-git"},
		{"allow 命中", "ls -la", "allow-ls"},
		{"无命中 fallback", "cat /etc/hosts", ""},
		{"disabled 规则不参与（否则 deny 压制 allow）", "ls -la", "allow-ls"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Evaluate(rules, tc.text)
			if err != nil {
				t.Fatalf("Evaluate: %v", err)
			}
			if tc.want == "" {
				if got != nil {
					t.Fatalf("got %q, want fallback(nil)", got.ID)
				}
				return
			}
			if got == nil || got.ID != tc.want {
				t.Fatalf("got %+v, want %q", got, tc.want)
			}
		})
	}
}

func TestEvaluate_DeterministicTieBreak(t *testing.T) {
	// 同 effect 同 priority：ID 字典序保证确定性（规则顺序乱序输入结果一致）。
	a := []Rule{
		{ID: "b-rule", Pattern: "ping *", Effect: EffectAllow, Priority: 100, Enabled: true},
		{ID: "a-rule", Pattern: "ping *", Effect: EffectAllow, Priority: 100, Enabled: true},
	}
	b := []Rule{a[1], a[0]}
	for i, rules := range [][]Rule{a, b} {
		got, err := Evaluate(rules, "ping 8.8.8.8")
		if err != nil {
			t.Fatalf("Evaluate: %v", err)
		}
		if got == nil || got.ID != "a-rule" {
			t.Fatalf("input %d: got %+v, want a-rule", i, got)
		}
	}
}

func TestMatchText(t *testing.T) {
	cases := []struct {
		pattern, text string
		want          bool
	}{
		{"rm -rf*", "rm -rf /", true},
		{"rm -rf*", "RM -RF /", true},   // glob 大小写不敏感
		{"rm -rf*", "xterm -rf", false}, // 整串锚定
		{"dd *of=/dev*", "dd if=/dev/zero of=/dev/sda", true},
		{"ping *", "ping 8.8.8.8", true},
		{"show*", "show ip interface brief", true},
		{"re:^(?i)ping\\s", "PING 8.8.8.8", true}, // re: 原样编译，调用方自控
		{"re:rm\\s+-rf", "sudo rm -rf /", true},   // re: 子串语义
		{"", "anything", false},
	}
	for _, tc := range cases {
		got, err := MatchText(tc.pattern, tc.text)
		if err != nil {
			t.Fatalf("MatchText(%q): %v", tc.pattern, err)
		}
		if got != tc.want {
			t.Fatalf("MatchText(%q, %q) = %v, want %v", tc.pattern, tc.text, got, tc.want)
		}
	}
}

func TestMatchText_BadRegex(t *testing.T) {
	if _, err := MatchText("re:[unclosed", "x"); err == nil {
		t.Fatal("bad regex must error")
	}
}

func TestEvaluate_BadPatternSkipped(t *testing.T) {
	rules := []Rule{
		{ID: "bad", Pattern: "re:[unclosed", Effect: EffectDeny, Enabled: true},
		{ID: "good", Pattern: "ls *", Effect: EffectAllow, Enabled: true},
	}
	got, err := Evaluate(rules, "ls -la")
	if err == nil {
		t.Fatal("首个坏 pattern 错误应透出")
	}
	if got == nil || got.ID != "good" {
		t.Fatalf("坏规则不得阻断求值，got %+v", got)
	}
}
