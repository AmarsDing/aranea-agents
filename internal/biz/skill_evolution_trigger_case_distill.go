package biz

import (
	"context"
	"encoding/json"
	"time"

	"aranea-agents/pkg/loggateway"
)

// ── P3 M4: CaseDistillTrigger（case→skill 蒸馏触发器，EverOS 蒸馏链落地）────
//
// Agent Case（M2 提取、M3 召回）积累到阈值后，把最近一批高质量任务经验
// 蒸馏成一份 SKILL.md 草稿，作为 create_skill 建议汇入统一进化建议漏斗。
// 挂为 orchestrator 的 EvolutionTrigger 而非独立 job：免费获得 pending 短路、
// per-action 7 天冷却（EvoTriggerCooldownHours）、D8 自适应降频、opt-in
// union gate 与 DB UNIQUE 兜底，且 LLM 蒸馏不占 memory 队列关键路径。

const (
	// caseDistillMinCases 触发蒸馏的最少 Case 数（评审确认：5 条起步，
	// 太少蒸馏出的模式不可靠）。
	caseDistillMinCases = 5
	// caseDistillRecallLimit 蒸馏输入的 Case 上限（最近高质量排序取前 10）。
	caseDistillRecallLimit = 10
	// CaseDistillTriggerSource 是建议的 trigger_source 值。
	CaseDistillTriggerSource = "agent_case_distill"
)

// EvoMetaSourceCaseIDs 记录蒸馏来源 Case ID 列表（JSON string array），
// 供审批界面追溯"这份草稿从哪些经验来"。
const EvoMetaSourceCaseIDs = "source_case_ids"

// CaseSkillDistiller 把一批 Agent Case 蒸馏成 SKILL.md 草稿（name+body）。
// LLM 实现在 service 层。LLM 不可用/失败时返回 error——Trigger 将其视为
// best-effort 跳过（本轮不产出建议，下轮重试）。
// Stability:evolving
type CaseSkillDistiller interface {
	DistillSkillFromCases(ctx context.Context, agentID string, cases []AgentCase) (name, body string, err error)
}

// CaseDistillTrigger 从 Agent 的历史任务经验中检测可固化的技能模式。
type CaseDistillTrigger struct {
	settings  AgentEvolutionSettingsReader // L1 opt-in gate（同 PatternTrigger）
	cases     AgentCaseRecaller            // 空 query = 最近高质量排序（M3 端口复用）
	distiller CaseSkillDistiller
	lg        loggateway.Logger
}

// NewCaseDistillTrigger 任一依赖为 nil 时 Check 整体 no-op（legacy 行为）。
func NewCaseDistillTrigger(
	settings AgentEvolutionSettingsReader,
	cases AgentCaseRecaller,
	distiller CaseSkillDistiller,
	lg loggateway.Logger,
) *CaseDistillTrigger {
	return &CaseDistillTrigger{settings: settings, cases: cases, distiller: distiller, lg: lg}
}

func (t *CaseDistillTrigger) TargetType() EvolutionTargetType { return EvolutionTargetAgent }
func (t *CaseDistillTrigger) ActionType() EvolutionActionType { return EvolutionActionCreate }
func (t *CaseDistillTrigger) TriggerSource() string           { return CaseDistillTriggerSource }

func (t *CaseDistillTrigger) Check(ctx context.Context, agentID string) ([]UnifiedEvolutionSuggestion, error) {
	if t.cases == nil || t.distiller == nil {
		return nil, nil
	}
	// L1 opt-in gate：动作是 create_skill，与 PatternTrigger 共用同一开关。
	if t.settings != nil {
		settings, err := t.settings.GetAgentRuntimeSettings(ctx, agentID)
		if err != nil {
			t.lg.Warn("case distill trigger: GetAgentRuntimeSettings failed", loggateway.Err(err))
			return nil, nil
		}
		if !settings.EvolutionSkillEvolve {
			return nil, nil
		}
	}

	cases, err := t.cases.RecallAgentCases(ctx, agentID, "", caseDistillRecallLimit)
	if err != nil {
		// DB 读失败属 K2 错误路径：上抛让 orchestrator 记 Warn。
		return nil, err
	}
	if len(cases) < caseDistillMinCases {
		return nil, nil
	}

	name, body, err := t.distiller.DistillSkillFromCases(ctx, agentID, cases)
	if err != nil {
		// LLM 失败是 best-effort 跳过：每 tick 都会重试，不向 orchestrator
		// 传播错误避免 Warn 刷屏；本地记一条 Warn 即可。
		t.lg.Warn("case distill trigger: distill failed, skipping this tick",
			loggateway.Str("agent_id", agentID),
			loggateway.Err(err))
		return nil, nil
	}
	if name == "" || body == "" {
		return nil, nil
	}

	caseIDs := make([]string, 0, len(cases))
	for _, c := range cases {
		if id := c.ID; id != "" {
			caseIDs = append(caseIDs, id)
		}
	}
	metadata, _ := json.Marshal(map[string]any{
		EvoMetaSourceCaseIDs: caseIDs,
		"case_count":         len(cases),
	})

	return []UnifiedEvolutionSuggestion{{
		ID:              newAgentCatalogID(),
		TargetType:      EvolutionTargetAgent,
		TargetID:        agentID,
		ActionType:      EvolutionActionCreate,
		TriggerSource:   CaseDistillTriggerSource,
		TriggerReason:   "distilled from agent task experience cases",
		Status:          "pending",
		Priority:        1,
		DraftName:       name,
		DraftBody:       body,
		LifecycleStatus: "draft",
		Metadata:        metadata,
		CreatedAt:       time.Now().UTC(),
	}}, nil
}
