// Package policyrule 是 79-runtime-governance R9 与 80-decision-intelligence
// D-P2 审批路由共享的规则求值核心（C3 裁定，2026-08-25 用户认可）：pattern
// 匹配 + priority + enabled + matched_rule 留痕，两张域表（tool_param_rules /
// 审批路由规则表）复用同一引擎，不各造引擎。
//
// 匹配语义：
//   - pattern 以 "re:" 开头 → 正则（原样编译，子串语义，调用方自控大小写）；
//   - 否则 → glob：'*' 跨任意字符（含空格/斜杠，非 path.Match 语义）、'?' 单字符，
//     整串锚定、大小写不敏感（命令类文本在 Windows/Linux 间大小写行为不一，
//     安全闸向 fail-safe 方向折叠）。
//
// 生效优先级（deny > ask > allow > fallback）：所有 enabled 且命中的规则中，
// 先按 effectRank（deny=0 < ask=1 < allow=2）取最小，同 rank 按 priority 升序，
// 再按 ID 字典序保证确定性。无命中返回 nil——调用方走各自 fallback（param
// 规则 fallback = 工具自身 requires_confirmation）。
package policyrule

import (
	"regexp"
	"strings"
)

// Effect 是规则命中后的处置。
type Effect string

const (
	EffectDeny  Effect = "deny"
	EffectAsk   Effect = "ask"
	EffectAllow Effect = "allow"
)

// effectRank 返回生效优先级（小者优先）。未知 effect 不参与竞选。
func effectRank(e Effect) (int, bool) {
	switch e {
	case EffectDeny:
		return 0, true
	case EffectAsk:
		return 1, true
	case EffectAllow:
		return 2, true
	default:
		return 0, false
	}
}

// Rule 是一条可求值规则。域表行到 Rule 的映射由各域仓储负责。
type Rule struct {
	ID       string
	Pattern  string
	Effect   Effect
	Priority int
	Enabled  bool
}

const regexPrefix = "re:"

// MatchText 判定单条 pattern 是否命中 text。坏正则返回 error（调用方跳过
// 该规则，不阻断整体求值）。
func MatchText(pattern, text string) (bool, error) {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return false, nil
	}
	if strings.HasPrefix(pattern, regexPrefix) {
		re, err := regexp.Compile(strings.TrimPrefix(pattern, regexPrefix))
		if err != nil {
			return false, err
		}
		return re.MatchString(text), nil
	}
	re, err := regexp.Compile(globToRegexp(pattern))
	if err != nil {
		return false, err
	}
	return re.MatchString(strings.ToLower(text)), nil
}

// globToRegexp 把命令 glob 翻译为锚定整串的正则：'*'→'.*'（跨任意字符含换行，
// s 旗标——2026-08-27 二轮审查根修：缺 s 时任何含 \n 的参数文本整体绕过 glob
// 规则，gns3 '*' ask 兜底与 glob deny 均可内嵌换行规避）、'?'→'.'，其余字面量
// 转义；整体大小写不敏感（调用方负责折叠 text）。
func globToRegexp(glob string) string {
	var b strings.Builder
	b.WriteString("(?is)^")
	for _, r := range glob {
		switch r {
		case '*':
			b.WriteString(".*")
		case '?':
			b.WriteString(".")
		default:
			b.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	b.WriteString("$")
	return b.String()
}

// Evaluate 在 rules 中选胜者。rules 应由调用方按作用域（如 tool_key）过滤；
// Enabled=false 与未知 effect 的规则跳过。坏 pattern 的规则跳过，首个坏
// pattern 错误经返回值带出供观测（不影响胜者计算）。
func Evaluate(rules []Rule, text string) (*Rule, error) {
	var winner *Rule
	winnerRank := 0
	var firstErr error
	for i := range rules {
		r := &rules[i]
		if !r.Enabled {
			continue
		}
		rank, ok := effectRank(r.Effect)
		if !ok {
			continue
		}
		matched, err := MatchText(r.Pattern, text)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if !matched {
			continue
		}
		if winner == nil ||
			rank < winnerRank ||
			(rank == winnerRank && r.Priority < winner.Priority) ||
			(rank == winnerRank && r.Priority == winner.Priority && r.ID < winner.ID) {
			winner = r
			winnerRank = rank
		}
	}
	return winner, firstErr
}
