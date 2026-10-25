package tool

import (
	"strings"

	"aranea-agents/internal/biz/policyrule"
	"aranea-agents/pkg/apierror"
)

// P0-5 审批工程化（docs/reports/2026-10-21-review-harness-p0-gap-solutions.md §6）：
// 会话/Agent 作用域确定性审批姿态（approval policy）的声明式规则求值核心与
// 写入期校验。与 tool_param_rules（79 R9）共享 internal/biz/policyrule 的
// 求值引擎与编译函数（评审裁量 ①：两套 pattern 只许一套语义，不许再造引擎）：
//   - tool_pattern：对「别名家簇定点名」的工具名匹配（与运行时
//     loopGuardCanonicalToolName / CanonicalParamRuleToolKey 一键空间）。
//   - command_prefix：对参数 JSON 提取的命令文本匹配；glob/'re:' 语义与
//     tool_pattern 完全一致（沿引擎不予新造语义，命名随表结构列）。
//   - args_pattern：对参数 JSON 压平文本（paramRuleMatchText 同一构建器）匹配。
// 三个维度为 AND 关系；command_prefix/args_pattern 为空表示该维度不参与（恒真）。
// tool_pattern 必填（显式："*" 才允许通配，空串拒收——隐式通配在安全闸上是
// 配置事故高发形态）。

// ApprovalMatchInput 是一次工具调用对审批策略规则的输入切面。各字段由运行时
// gate 构建（internal/agent/tool_approval_policy.go），语义锚定见上。
type ApprovalMatchInput struct {
	ToolName string
	Command  string
	ArgsText string
}

// approvalModeRank 返回规则命中后的生效优先级（小者优先）。未知 mode 的规则
// 不参与竞选（与 policyrule.effectRank 同一折叠语义）。rank 序 as/is 独立成档：
// never=0 < auto=1 < ask=2 < trusted=3（auto 消费等同 ask，但竞选位更低——
// 同 scope 同 pattern 下显式 ask 压过 auto）。
func approvalModeRank(mode ApprovalMode) (int, bool) {
	switch mode {
	case ApprovalModeNever:
		return 0, true
	case ApprovalModeAuto:
		return 1, true
	case ApprovalModeAsk:
		return 2, true
	case ApprovalModeTrusted:
		return 3, true
	default:
		return 0, false
	}
}

// ValidateApprovalPolicy 拒收非法规则（写入期校验）：必填字段缺省、scope/mode
// 枚举越界、tool_pattern 为空（非显式通配）或任一 pattern 不可编译（'re:' 正
// 则语法错 / glob 引擎拒收）。编译探针复用 policyrule.MatchText——与 param
// rules 的 Upsert 校验同一编译函数。
func ValidateApprovalPolicy(p ApprovalPolicy) error {
	if strings.TrimSpace(p.ID) == "" {
		return apierror.BadRequest("TOOL", "approval policy id is required")
	}
	switch ApprovalScope(strings.TrimSpace(p.Scope)) {
	case ApprovalScopeSession, ApprovalScopeAgent:
	default:
		return apierror.BadRequest("TOOL", "scope must be session|agent")
	}
	if strings.TrimSpace(p.ScopeKey) == "" {
		return apierror.BadRequest("TOOL", "scope_key is required")
	}
	switch ApprovalMode(strings.TrimSpace(p.Mode)) {
	case ApprovalModeAsk, ApprovalModeNever, ApprovalModeTrusted, ApprovalModeAuto:
	default:
		return apierror.BadRequest("TOOL", "mode must be never|auto|ask|trusted")
	}
	if strings.TrimSpace(p.ToolPattern) == "" {
		return apierror.BadRequest("TOOL", "tool_pattern is required (use \"*\" for explicit wildcard)")
	}
	if _, err := policyrule.MatchText(p.ToolPattern, ""); err != nil {
		return apierror.BadRequest("TOOL", "tool_pattern 不可编译: "+err.Error())
	}
	if strings.TrimSpace(p.CommandPrefix) != "" {
		if _, err := policyrule.MatchText(p.CommandPrefix, ""); err != nil {
			return apierror.BadRequest("TOOL", "command_prefix 不可编译: "+err.Error())
		}
	}
	if strings.TrimSpace(p.ArgsPattern) != "" {
		if _, err := policyrule.MatchText(p.ArgsPattern, ""); err != nil {
			return apierror.BadRequest("TOOL", "args_pattern 不可编译: "+err.Error())
		}
	}
	if p.ExpiresAt < 0 {
		return apierror.BadRequest("TOOL", "expires_at must be >= 0 (0 = no expiry)")
	}
	return nil
}

// matchApprovalPolicy 判定单条规则是否命中输入切面（AND 三维）。坏 pattern
// 视为不命中（写入期已拦截；存量行防御性跳过，fail-safe 向「不收紧」折叠）。
func matchApprovalPolicy(p *ApprovalPolicy, in ApprovalMatchInput) bool {
	matched, err := policyrule.MatchText(p.ToolPattern, in.ToolName)
	if err != nil || !matched {
		return false
	}
	if strings.TrimSpace(p.CommandPrefix) != "" {
		matched, err = policyrule.MatchText(p.CommandPrefix, in.Command)
		if err != nil || !matched {
			return false
		}
	}
	if strings.TrimSpace(p.ArgsPattern) != "" {
		matched, err = policyrule.MatchText(p.ArgsPattern, in.ArgsText)
		if err != nil || !matched {
			return false
		}
	}
	return true
}

// approvalPolicyEffective 判定策略行在 nowUnix 时刻有效（enabled 且未过期）。
// nowUnix<=0 用一秒防御（ExpiresAt>0 即判过期比较仍成立）。
func approvalPolicyEffective(p *ApprovalPolicy, nowUnix int64) bool {
	if !p.Enabled {
		return false
	}
	if p.ExpiresAt > 0 && p.ExpiresAt <= nowUnix {
		return false
	}
	return true
}

// pickApprovalWinner 在一组（同 shadow 类别）规则中选胜者：先按 modeRank
// （never=0 < auto=1 < ask=2 < trusted=3）取最小，同 rank 按 priority 升序，
// 再按 ID 字典序保证确定性（与 policyrule.Evaluate 同一决胜序）。
func pickApprovalWinner(policies []ApprovalPolicy, in ApprovalMatchInput, nowUnix int64, shadow bool) *ApprovalPolicy {
	var winner *ApprovalPolicy
	winnerRank := 0
	for i := range policies {
		p := &policies[i]
		if p.ShadowOnly != shadow || !approvalPolicyEffective(p, nowUnix) {
			continue
		}
		rank, ok := approvalModeRank(ApprovalMode(p.Mode))
		if !ok || !matchApprovalPolicy(p, in) {
			continue
		}
		if winner == nil ||
			rank < winnerRank ||
			(rank == winnerRank && p.Priority < winner.Priority) ||
			(rank == winnerRank && p.Priority == winner.Priority && p.ID < winner.ID) {
			winner = p
			winnerRank = rank
		}
	}
	return winner
}

// EvalApprovalPolicies 返回 (enforce, shadow) 两枚胜者：shadow_only=1 的规则
// 参与 shadow 竞选（预演观测：命中必落审计、零强制）；enforce 胜者是唯一可
// 影响本次调用的规则。二者均可能为 nil（无命中）。
func EvalApprovalPolicies(policies []ApprovalPolicy, in ApprovalMatchInput, nowUnix int64) (enforce, shadow *ApprovalPolicy) {
	return pickApprovalWinner(policies, in, nowUnix, false),
		pickApprovalWinner(policies, in, nowUnix, true)
}
