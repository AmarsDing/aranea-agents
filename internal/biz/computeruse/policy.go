package computeruse

import (
	"strings"
	"unicode"
)

// Policy 安全策略：敏感词 / 进程禁区。零值即安全默认（内置默认表生效）。
type Policy struct {
	// DangerWords 高危敏感词（命中则 danger=true，强制人工确认）。
	// nil 时使用 DefaultDangerWords()；显式传空切片表示禁用。
	DangerWords []string
	// BlockedProcesses 进程禁区（前台进程命中时拒绝一切动作注入）。
	// nil 时使用 DefaultBlockedProcesses()。
	BlockedProcesses []string
}

// DefaultDangerWords 内置高危词表（覆盖删除/支付/转账/发送/格式化等不可逆动作语义）。
func DefaultDangerWords() []string {
	return []string{
		"删除", "永久删除", "格式化", "清空", "销毁",
		"支付", "付款", "转账", "确认支付", "确认付款", "充值", "提现",
		"发送", "提交订单", "下单", "购买",
		"delete", "format", "erase", "pay", "transfer", "purchase", "send",
		"shutdown", "关机", "重启",
	}
}

// latinWholeWordMax 拉丁危险词不超过此长度时按整词匹配，避免 send 命中 sender。
const latinWholeWordMax = 5

func latinRuneCount(s string) int {
	n := 0
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' {
			n++
		} else {
			return -1 // 含非拉丁字母：走 contains
		}
	}
	return n
}

func latinTokens(s string) []string {
	var out []string
	var b strings.Builder
	flush := func() {
		if b.Len() > 0 {
			out = append(out, b.String())
			b.Reset()
		}
	}
	for _, r := range strings.ToLower(s) {
		if r >= 'a' && r <= 'z' {
			b.WriteRune(r)
		} else {
			flush()
		}
	}
	flush()
	return out
}

func containsLatinWord(text, word string) bool {
	w := strings.ToLower(word)
	for _, tok := range latinTokens(text) {
		if tok == w {
			return true
		}
	}
	return false
}

func dangerWordHit(raw, norm, word string) bool {
	nw := normalize(word)
	if nw == "" {
		return false
	}
	if n := latinRuneCount(word); n > 0 && n <= latinWholeWordMax {
		return containsLatinWord(raw, word)
	}
	return strings.Contains(norm, nw)
}

// DefaultBlockedProcesses 内置进程禁区（密码管理器/银行安全控件）。
func DefaultBlockedProcesses() []string {
	return []string{
		"keepass.exe", "keepassxc.exe", "1password.exe", "bitwarden.exe",
		"lastpass.exe", "dashlane.exe",
		"icbccab.exe", "ccbnetpay.exe", "cmb.exe", "cmbc.exe",
		"entersafe.exe", "watchdata.exe", "unionpay.exe", "aliedit.exe",
	}
}

func (p Policy) dangerWords() []string {
	if p.DangerWords == nil {
		return DefaultDangerWords()
	}
	return p.DangerWords
}

func (p Policy) blockedProcesses() []string {
	if p.BlockedProcesses == nil {
		return DefaultBlockedProcesses()
	}
	return p.BlockedProcesses
}

// normalize 归一化：小写 + 去全部空白/标点（中英文混合语义匹配）。
func normalize(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range strings.ToLower(s) {
		if unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// IsDanger 判定目标描述+动作参数文本是否命中高危词。
func (p Policy) IsDanger(target string, args map[string]any) bool {
	raw := target
	for _, k := range []string{"text", "combo", "target"} {
		if v, ok := args[k].(string); ok {
			raw += " " + v
		}
	}
	norm := normalize(raw)
	for _, w := range p.dangerWords() {
		if dangerWordHit(raw, norm, w) {
			return true
		}
	}
	return false
}

// IsBlockedProcess 判定进程名是否在禁区内（大小写不敏感，精确匹配可执行文件名）。
func (p Policy) IsBlockedProcess(processName string) bool {
	pn := normalize(processName)
	for _, bp := range p.blockedProcesses() {
		if normalize(bp) == pn {
			return true
		}
	}
	return false
}
