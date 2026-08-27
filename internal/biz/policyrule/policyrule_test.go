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

// TestMatchText_GlobCrossesNewline 钉死 2026-08-27 二轮审查根修：globToRegexp
// 必须带 (?is)——缺 s 旗标时 '.' 不匹配 \n，任何内嵌换行的参数文本整体绕过
// glob 规则（gns3 '*' ask 兜底与 glob deny 均可经换行规避）。
func TestMatchText_GlobCrossesNewline(t *testing.T) {
	cases := []struct {
		pattern, text string
		want          bool
	}{
		{"*", "show version\nwrite erase", true},     // 兜底 ask glob 必须兜住多行
		{"rm -rf*", "rm -rf /\n# padding", true},     // glob deny 不被换行截断
		{"show *", "show ip\ninterface brief", true}, // glob '*' 跨 \n
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

// TestBuiltinSeedVectors_Hardened 钉死 20261263/20261265 内置种子 pattern 的
// 安全语义（pattern 与迁移文件保持一致——改迁移必须同步本表，反之亦然）：
//   - deny 分隔符类含 ( $ 反引号：$(cmd)/`cmd`/(cmd) 命令替换与子 shell 命中；
//   - rm flags 归一：-rf/-fr/-r -f/-f -r 变形全命中；
//   - gns3 allow 单行锚定：多行注入（"show version\nwrite erase"）不落 allow，
//     安全方向落兜底 ask。
func TestBuiltinSeedVectors_Hardened(t *testing.T) {
	const sep = "[;&|/\\s\"'($`]"
	const (
		denyRmrf     = `re:(?i)(^|` + sep + `)rm\s+-(rf|fr|r\s+-f|f\s+-r)\s+(/|~|\$HOME|\*)`
		denySudoRmrf = `re:(?i)(^|` + sep + `)sudo\s+(-\S+\s+)*rm\s+-(rf|fr|r\s+-f|f\s+-r)\s+(/|~|\$HOME|\*)`
		denyMkfs     = `re:(?i)(^|` + sep + `)mkfs[\s.]`
		denyShutdown = `re:(?i)(^|` + sep + `)shutdown(\s|$)`
		allowShow    = `re:(?i)^show [^\n]*$`
		allowPing    = `re:(?i)^ping [^\n]*$`
	)
	cases := []struct {
		name, pattern, text string
		want                bool
	}{
		// --- rm -rf deny：基本形态与 flags 归一 ---
		{"基本形态", denyRmrf, "rm -rf /", true},
		{"flags 逆序 -fr", denyRmrf, "rm -fr /", true},
		{"flags 分离 -r -f", denyRmrf, "rm -r -f /", true},
		{"flags 分离 -f -r", denyRmrf, "rm -f -r /", true},
		{"双空格", denyRmrf, "rm  -rf  /", true},
		{"家目录 ~", denyRmrf, "rm -rf ~", true},
		{"$HOME", denyRmrf, "rm -rf $HOME", true},
		{"通配 *", denyRmrf, "rm -rf *", true},
		// --- 命令替换/子 shell 分隔符（加固前全部绕过） ---
		{"$() 命令替换", denyRmrf, "$(rm -rf /)", true},
		{"反引号替换", denyRmrf, "`rm -rf /`", true},
		{"() 子 shell", denyRmrf, "(rm -rf /)", true},
		{"; 链式", denyRmrf, "echo ok; rm -rf /", true},
		// --- rm deny 反向：相对路径与非危险命令不误伤 ---
		{"相对路径不命中", denyRmrf, "rm -rf tmp/build", false},
		{"普通命令不命中", denyRmrf, "ls -la", false},
		// --- sudo 包装 ---
		{"sudo 基本", denySudoRmrf, "sudo rm -rf /", true},
		{"sudo flags 逆序", denySudoRmrf, "sudo rm -fr /", true},
		{"sudo 带选项", denySudoRmrf, "sudo -i rm -rf /", true},
		{"sudo $HOME", denySudoRmrf, "sudo rm -rf $HOME", true},
		// --- mkfs ---
		{"mkfs 点分", denyMkfs, "mkfs.ext4 /dev/sda", true},
		{"mkfs 空格", denyMkfs, "mkfs /dev/sda", true},
		{"mkfs 子 shell", denyMkfs, "(mkfs.ext4 /dev/sda)", true},
		// --- shutdown：分隔符类保留 / 的 fail-safe 误拒为设计取舍（迁移注释
		// 明载：优于放开 /sbin/shutdown 旁路），钉死防静默放开 ---
		{"shutdown 基本", denyShutdown, "shutdown -h now", true},
		{"shutdown 裸命令", denyShutdown, "shutdown", true},
		{"路径片段=命令名误拒（fail-safe 取舍）", denyShutdown, "ls /tmp/shutdown", true},
		// --- gns3 allow 单行锚定：多行注入不落 allow ---
		{"show 单行", allowShow, "show ip interface brief", true},
		{"show 大小写", allowShow, "SHOW RUN", true},
		{"show 多行注入拒绝", allowShow, "show version\nwrite erase", false},
		{"show 无参数不命中", allowShow, "show", false},
		{"ping 多行注入拒绝", allowPing, "ping 8.8.8.8\nreload", false},
		{"非白名单命令不命中", allowShow, "write erase", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := MatchText(tc.pattern, tc.text)
			if err != nil {
				t.Fatalf("MatchText(%q): %v", tc.pattern, err)
			}
			if got != tc.want {
				t.Fatalf("MatchText(%q, %q) = %v, want %v", tc.pattern, tc.text, got, tc.want)
			}
		})
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
