package biz

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"aranea-agents/pkg/ctxuser"
	"aranea-agents/pkg/loggateway"
)

// ---------------------------------------------------------------------------
// G3（ADR-G，2026-08-14）：交付物质量门 verdict —— rule-based v1
//
// 二元门（HasRealDeliverable）判「有无」，本门判「内容是否达标」（LLM-as-Judge
// 的规则快路径；LLM judge 档留作后续可选增强，对齐 G2 约束3「先规则后 LLM」）。
// fail-open 哲学：所有 infra 错误以 error 返回，由调用方（runner）决定放行，
// 本层不做静默放行——保证判定证据（RuleHits）可审计、可喂 MAST 标注。
// ---------------------------------------------------------------------------

// TeamQualityVerdict 是交付物质量门的三值判定。
type TeamQualityVerdict string

const (
	// TeamQualityPass 交付物达标（或本门不适用范围）。
	TeamQualityPass TeamQualityVerdict = "pass"
	// TeamQualityRevise 交付物存在但未达标，打回修订（携带 Feedback）。
	TeamQualityRevise TeamQualityVerdict = "revise"
	// TeamQualityFail 交付物根本性不可用（当前规则集与 revise 同处理，预留
	// 给 LLM judge 档的强否决语义）。
	TeamQualityFail TeamQualityVerdict = "fail"
)

// QualityGateResult 是质量门判定结果。RuleHits 记录命中的规则（审计/MAST）。
type QualityGateResult struct {
	Verdict  TeamQualityVerdict
	Feedback string
	RuleHits []string
}

// qualitySufficiencyMinRunes 是 J2 充分性规则的文本总长下限（rune）。
const qualitySufficiencyMinRunes = 80

// qualityPlaceholderMarkers 是 J3 占位/拒答标记（子串匹配，ASCII 部分小写归一）。
var qualityPlaceholderMarkers = []string{
	"todo", "tbd", "placeholder",
	"占位", "待定", "待补充", "无法完成", "作为ai", "作为 ai",
}

// EvaluateDeliverableQuality 对 DAG 团队的自有交付物（graph state − 上游种子）
// 做规则化质量判定。非 DAG 团队 / 无 state 交付通道 / 空交付物（二元门职责）
// 直接 pass；infra 读错返回 error（调用方 fail-open）。
func (d *SpiritDelivery) EvaluateDeliverableQuality(ctx context.Context, team Team) (QualityGateResult, error) {
	if team.DagNodeID == "" {
		return QualityGateResult{Verdict: TeamQualityPass}, nil
	}
	anchor, ok := d.stateDeliverableChannel(team)
	if !ok {
		return QualityGateResult{Verdict: TeamQualityPass}, nil
	}
	teamSessionID, err := d.resolveTeamMainSessionID(ctx, team.ID)
	if err != nil {
		return QualityGateResult{}, err
	}
	if teamSessionID == "" {
		return QualityGateResult{Verdict: TeamQualityPass}, nil
	}
	stateDeliv, err := d.graphDelivReader.ReadGraphDeliverable(ctx, anchor, ctxuser.TRPCUserKey(ctx), teamSessionID)
	if err != nil {
		return QualityGateResult{}, err
	}
	seed, serr := d.UpstreamDeliverableSeed(ctx, team)
	if serr != nil {
		return QualityGateResult{}, serr
	}
	own := subtractUpstreamSeed(stateDeliv, seed)
	if len(own) == 0 {
		// 空交付物由二元门（HasRealDeliverable）先行否决，本门不重复。
		return QualityGateResult{Verdict: TeamQualityPass}, nil
	}

	hits := deliverableQualityRuleHits(flattenDeliverableText(own))

	// J4：成员中途异常证据（pi-agentteam「Leader 评审验收」语义在门侧落地——
	// 框架无成员级 steer，中途纠偏收敛到本评审点，见 ADR-G）。
	for _, child := range d.listMemberChildSessions(ctx, teamSessionID) {
		failed, reason := d.MemberExecutionEvidence(ctx, child.ID)
		if !failed {
			continue
		}
		key := strings.TrimSpace(child.MemberAgentKey)
		if key == "" {
			key = child.ID
		}
		hits = append(hits, fmt.Sprintf("J4 成员异常：成员 %s 执行失败（%s），其任务范围可能未被交付物覆盖", key, reason))
	}

	if len(hits) == 0 {
		return QualityGateResult{Verdict: TeamQualityPass}, nil
	}
	return QualityGateResult{
		Verdict:  TeamQualityRevise,
		Feedback: strings.Join(hits, "；"),
		RuleHits: hits,
	}, nil
}

// listMemberChildSessions 列出团队 session 的成员子 session；读取失败按无
// 成员处理（保守：infra 读错不得制造误判打回）。
func (d *SpiritDelivery) listMemberChildSessions(ctx context.Context, teamSessionID string) []Session {
	children, err := d.sessionUC.ListChildSessions(ctx, teamSessionID)
	if err != nil {
		d.lg.Warn("质量门：读取成员子 session 失败，按无成员证据处理",
			loggateway.StepID("spirit.quality_gate.children_err"),
			loggateway.Str("team_session_id", teamSessionID),
			loggateway.Err(err),
		)
		return nil
	}
	out := make([]Session, 0, len(children))
	for _, c := range children {
		if strings.TrimSpace(c.MemberAgentKey) != "" {
			out = append(out, c)
		}
	}
	return out
}

// flattenDeliverableText 把自有交付物拍平为可判定文本：排除协议/元数据保留
// 键（ack/* 回执、cognition 推理元数据），summary 作为真实内容计入。
func flattenDeliverableText(stateDeliv map[string]any) string {
	var sb strings.Builder
	for k, v := range stateDeliv {
		if k == deliverableReservedKeyCognition || strings.HasPrefix(k, deliverableAckKeyPrefix) {
			continue
		}
		switch tv := v.(type) {
		case string:
			sb.WriteString(tv)
		default:
			b, err := json.Marshal(tv)
			if err != nil {
				continue
			}
			sb.Write(b)
		}
		sb.WriteByte('\n')
	}
	return sb.String()
}

// deliverableQualityRuleHits 是纯函数规则集：J2 充分性 + J3 占位/拒答。
func deliverableQualityRuleHits(text string) []string {
	var hits []string
	if n := len([]rune(strings.TrimSpace(text))); n < qualitySufficiencyMinRunes {
		hits = append(hits, fmt.Sprintf("J2 充分性：有效内容仅 %d 字（要求 ≥%d），交付物过于简略", n, qualitySufficiencyMinRunes))
	}
	lower := strings.ToLower(text)
	for _, marker := range qualityPlaceholderMarkers {
		if strings.Contains(lower, marker) {
			hits = append(hits, fmt.Sprintf("J3 占位/拒答：命中标记 %q", marker))
		}
	}
	return hits
}
