package computeruse

import "testing"

func TestPolicy_IsDanger(t *testing.T) {
	p := Policy{} // 零值=默认表
	cases := []struct {
		target string
		args   map[string]any
		want   bool
	}{
		{"点击保存按钮", nil, false},
		{"点击删除按钮", nil, true},
		{"永久删除该文件", nil, true},
		{"点击确认支付", nil, true},
		{"输入文本", map[string]any{"text": "hello world"}, false},
		{"输入转账金额", map[string]any{"text": "100元"}, true},
		{"press", map[string]any{"combo": "ctrl+s"}, false},
		{"点 击 删 除", nil, true},     // 空白归一化后仍命中
		{"DELETE FILE", nil, true}, // 大小写归一化
	}
	for _, c := range cases {
		if got := p.IsDanger(c.target, c.args); got != c.want {
			t.Errorf("IsDanger(%q,%v) = %v, want %v", c.target, c.args, got, c.want)
		}
	}
}

func TestPolicy_CustomWordsOverride(t *testing.T) {
	p := Policy{DangerWords: []string{"爆炸"}}
	if !p.IsDanger("引爆爆炸物", nil) {
		t.Error("custom word should hit")
	}
	if p.IsDanger("删除文件", nil) {
		t.Error("custom table replaces default: 删除 should miss")
	}
}

func TestPolicy_IsBlockedProcess(t *testing.T) {
	p := Policy{}
	cases := []struct {
		name string
		want bool
	}{
		{"keepass.exe", true},
		{"KeePassXC.EXE", true}, // 大小写不敏感
		{"1password.exe", true},
		{"notepad.exe", false},
		{"explorer.exe", false},
	}
	for _, c := range cases {
		if got := p.IsBlockedProcess(c.name); got != c.want {
			t.Errorf("IsBlockedProcess(%q) = %v, want %v", c.name, got, c.want)
		}
	}
}
