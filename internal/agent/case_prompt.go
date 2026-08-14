package agent

import (
	"context"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/strutil"
)

// P3 M3: Agent Case 召回注入。Case 是 Agent 视角的任务经验（goal/approach/
// pitfalls），与用户画像（L3 facts）互补。召回块并入既有 memory cue 管线
// （buildRuntimeMemoryCue 的 recallParts），不新增注入点——保持前缀稳定与
// 统一预算截断。

const (
	// caseRecallMax 每 turn 最多注入的 case 条数（prompt 预算保护）。
	caseRecallMax = 3
	// caseFieldMaxRunes 单字段（goal/approach/pitfalls）截断长度。
	caseFieldMaxRunes = 120
)

// CaseMemoryCue 召回并格式化 Agent 历史任务经验块。所有失败路径（nil
// recaller / 召回错误 / 空结果）都返回 ""——best-effort，绝不能阻断 turn。
func CaseMemoryCue(ctx context.Context, recaller biz.AgentCaseRecaller, agentID, keyword string) string {
	if recaller == nil {
		return ""
	}
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return ""
	}
	cases, err := recaller.RecallAgentCases(ctx, agentID, strings.TrimSpace(keyword), caseRecallMax)
	if err != nil || len(cases) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## 任务经验（该 Agent 的历史案例）\n")
	b.WriteString("以下经验来自该 Agent 的历史任务，可参考其做法与教训：\n")
	written := 0
	for _, c := range cases {
		if written >= caseRecallMax {
			break
		}
		line := formatAgentCaseLine(c)
		if line == "" {
			continue
		}
		b.WriteString(line)
		b.WriteByte('\n')
		written++
	}
	if written == 0 {
		return ""
	}
	return strings.TrimSpace(b.String())
}

// formatAgentCaseLine 按 outcome 渲染单行：success/partial 给目标+做法，
// failure 给目标+教训（负例的价值在 pitfalls）。无 goal 的 case 是噪声，跳过。
func formatAgentCaseLine(c biz.AgentCase) string {
	goal := truncateCaseField(c.Goal)
	if goal == "" {
		return ""
	}
	marker := strings.ToUpper(strings.TrimSpace(c.Outcome))
	if marker == "" {
		marker = "PARTIAL"
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "- [%s] 目标：%s", marker, goal)
	if c.Outcome == biz.AgentCaseOutcomeFailure {
		if p := truncateCaseField(c.Pitfalls); p != "" {
			fmt.Fprintf(&sb, "｜教训：%s", p)
		}
	} else {
		if a := truncateCaseField(c.Approach); a != "" {
			fmt.Fprintf(&sb, "｜做法：%s", a)
		}
	}
	if s := truncateCaseField(c.OutcomeSummary); s != "" {
		fmt.Fprintf(&sb, "｜结果：%s", s)
	}
	return sb.String()
}

func truncateCaseField(s string) string {
	return strutil.TruncateRunesEllipsis(strings.TrimSpace(s), caseFieldMaxRunes)
}
