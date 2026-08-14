package computeruse

import "strings"

// InjectionGuard 屏幕内容注入检测（M77）：与 Policy 并联的独立策略组件。
// Policy 判「动作语义是否危险」（目标文本），Guard 判「输入内容是否不可信」（屏幕文本）。
// 零值即安全默认（内置默认表生效）。只做检测-打标，不篡改屏幕内容（ADR-77-01/红线）。
type InjectionGuard struct {
	// Patterns 注入模式表；nil 使用 DefaultInjectionPatterns()；显式空切片=禁用（仅测试用）。
	Patterns []string
}

// InjectionHit 一次注入检出。
type InjectionHit struct {
	Pattern string `json:"pattern"` // 命中的模式表项（原始形态）
	Ref     string `json:"ref"`     // 命中元素 ref（g{n}.e{m}）
	Snippet string `json:"snippet"` // 命中片段摘要（截断 ≤80 字符，ADR-77-05）
}

// DefaultInjectionPatterns 内置注入模式表（中英双语指令性短语）。
// 经 normalize（小写+去空白/标点）后 contains 匹配，大小写/空白/标点变形同样命中。
func DefaultInjectionPatterns() []string {
	return []string{
		"ignore previous instructions", "ignore all instructions", "disregard previous instructions",
		"system prompt", "you are now", "new instructions", "override instructions", "do not follow",
		"忽略之前指令", "忽略以上指令", "忽略所有指令", "系统提示", "新指令", "覆盖指令", "无视之前指令",
	}
}

// snippetMaxRunes 命中摘要最大字符数（rune 计，中文安全）。
const snippetMaxRunes = 80

func (g InjectionGuard) patterns() []string {
	if g.Patterns == nil {
		return DefaultInjectionPatterns()
	}
	return g.Patterns
}

// Scan 扫描元素可访问名称中的注入模式。
// 只扫 Name（AppName 等进程名字段不是注入载体）；单元素多模式只记首个命中（防噪声）。
func (g InjectionGuard) Scan(elements []UIElement) []InjectionHit {
	var hits []InjectionHit
	for _, e := range elements {
		name := normalize(e.Name)
		if name == "" {
			continue
		}
		for _, p := range g.patterns() {
			np := normalize(p)
			if np == "" || !strings.Contains(name, np) {
				continue
			}
			hits = append(hits, InjectionHit{Pattern: p, Ref: e.Ref, Snippet: truncateRunes(e.Name, snippetMaxRunes)})
			break // 单元素只记首个命中
		}
	}
	return hits
}

// truncateRunes 按 rune 截断（中文安全）。
func truncateRunes(s string, max int) string {
	rs := []rune(s)
	if len(rs) <= max {
		return s
	}
	return string(rs[:max])
}
