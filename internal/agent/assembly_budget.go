package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// 包A（session-eval-20260825 A1，统筹优化方案 optimization-plan.md）：管理层
// Prompt 装配预算硬闸。
//
// 定位：单轮完全注入请求的**绝对 token 上限**，与窗口比例终审压缩闸
// （context_compression_inject.go，hard_trigger_ratio×window）互补——比例闸
// 管窗口溢出，本闸管绝对烧钱。513 万（轮数型）与 S02-armB（单轮装配型）两次
// 事故证明：比例闸对「窗口远未触线但单轮已烧 96K-1.2M」结构性失明。
//
// 触发链（每次模型调用评估，含工具循环续轮）：
//  1. est ≤ soft：零开销通过。
//  2. soft < est ≤ hard：once-per-turn 注入容量告警 cue（R2，MemGPT 范式——
//     引导 LLM 主动落盘关键状态，而非被动等截断）。
//  3. est > hard：截断至 target=hard×0.9（滞回，防下轮立即重触），两段降级：
//     a. 尾部 cue 按保护序丢弃（保底顺序：L3/L4 记忆 > knowledge（question
//        检索，R2 高相关）> 技能目录/指导 > playbook 全文 > roster 明细 >
//        工具目录 > reply reminder；未标记 cue 一律 protected 不丢——
//        intent/澄清结论（R1 置尾）与本闸告警天然豁免）；
//     b. 仍超则驱逐最旧历史（复用 partitionMessagesByTokenBudget 的
//        tool-pair 安全边界 + 截断标记，与压缩闸同构）。
//     静态头（身份/职责骨架）永不触碰（骨架永保）；头自身超预算时 Warn
//     放行（K3：绝不阻断模型调用）。
//  4. 截断落 flowlog `prompt.assembly.trimmed`（各段截断量）+ 进程 Warn。
//
// 配置：agent_runtime_settings.assembly_budget_soft/hard_tokens（估 token，
// 0=关闭）。默认全关，轻链路零开销；管理层（3 GM + 部门主管）经 SQL 灰度
// 40K/60K（A4）。读取为构建期快照（ag.Settings），与 hardTriggerRatioForAgent
// 同先例——预算变更触发 agent 重建生效。

// assemblyBudgetHookPriority 8：全量注入（memory 5 / knowledge 6 / 各 cue ≤6）
// 之后、终审压缩闸（9）与 L0 快照（10）之前，计量口径=完全注入后的请求。
// intent reorder（100）在其后运行，不影响本闸的总量计量与分段丢弃。
const assemblyBudgetHookPriority = 8

// assemblyBudgetTargetFactor 是 hard 触发后的滞回系数（与压缩闸
// truncationTargetFactor 同义）：截到 hard×0.9，下一次模型调用不会立即重触。
const assemblyBudgetTargetFactor = 0.9

// assemblyBudgetWarnStateKey 是容量告警 once-per-turn 标记在 invocation
// state 中的键（工具循环续轮以同一外层 invocation 重进 hook，invocation
// state 是唯一跨轮载体，同 memoryCueTurnCacheStateKey 先例）。
const assemblyBudgetWarnStateKey = "aranea.assembly_budget.warned"

// 段标记：隐藏 XML 注释前缀（LLM 视为惰性元数据），照 memoryInjectMarker
// 先例。集中声明于此——闸的分类表与常量同文件，注入点直接引用。
const (
	// knowledgeCueMarker 标识知识预检索 cue（knowledge_inject.go）。
	knowledgeCueMarker = "<!-- aranea:knowledge_cue -->\n"
	// skillGuidanceCueMarker 标识技能指导 cue（skill_guidance_inject.go，
	// full 与 progressive routed 两注入点共用）。
	skillGuidanceCueMarker = "<!-- aranea:skill_guidance -->\n"
	// toolCatalogCueMarker 标识 deferred 工具目录 cue（tool_catalog_cue.go）。
	toolCatalogCueMarker = "<!-- aranea:tool_catalog_cue -->\n"
	// replyReminderCueMarker 标识「已完成+下一步」提醒（reply_reminder_inject.go）。
	replyReminderCueMarker = "<!-- aranea:reply_reminder -->\n"
	// workspaceSkillsCueMarker 标识工作区技能扫描 cue（workspace_skills_inject.go）。
	workspaceSkillsCueMarker = "<!-- aranea:workspace_skills -->\n"
	// orchBriefCueMarker 标识会话编排阶段简报（orchestration_phase_hooks.go）。
	orchBriefCueMarker = "<!-- aranea:orch_brief -->\n"
	// assemblyBudgetWarnMarker 标识本闸注入的容量告警（豁免丢弃）。
	assemblyBudgetWarnMarker = "<!-- aranea:assembly_budget_warn -->\n"
)

// assemblyCueKind 是闸的分段分类。roster_detail / playbook_full 为 A2 预留
// （当前零注入）；保护序已在 drop rank 中留位，A2 落地时只需打标记。
type assemblyCueKind string

const (
	cueKindReplyReminder   assemblyCueKind = "reply_reminder"
	cueKindToolCatalog     assemblyCueKind = "tool_catalog"
	cueKindRosterDetail    assemblyCueKind = "roster_detail" // A2 预留
	cueKindPlaybookFull    assemblyCueKind = "playbook_full" // A2 预留
	cueKindOrchBrief       assemblyCueKind = "orch_brief"
	cueKindWorkspaceSkills assemblyCueKind = "workspace_skills"
	cueKindSkillGuidance   assemblyCueKind = "skill_guidance"
	cueKindKnowledge       assemblyCueKind = "knowledge"
	cueKindMemory          assemblyCueKind = "memory"
	cueKindProtected       assemblyCueKind = "protected"
)

// assemblyCueDropRank：值越大越先丢。保底顺序（optimization-plan.md A1，
// R2 question-aware 的确定性代理——knowledge 由本轮问题检索产出、memory
// L3/L4 由 question keyword 召回，二者最贴近当前问题，最后牺牲）：
// reply reminder > 工具目录 > roster 明细 > playbook 全文 > 编排简报 >
// 工作区技能 > 技能指导 > knowledge > 记忆（L3/L4/L1 融合块）。
var assemblyCueDropRank = map[assemblyCueKind]int{
	cueKindReplyReminder:   100,
	cueKindToolCatalog:     90,
	cueKindRosterDetail:    85,
	cueKindPlaybookFull:    80,
	cueKindOrchBrief:       70,
	cueKindWorkspaceSkills: 60,
	cueKindSkillGuidance:   50,
	cueKindKnowledge:       40,
	cueKindMemory:          30,
}

// classifyAssemblyCue 按段标记分类尾部 cue；未标记（intent 上下文、容量告
// 警、world-state、runtime cue 等）一律 protected——宁保勿丢（红线：不以牺
// 牲任务达成度换 token）。
func classifyAssemblyCue(msg trpcmodel.Message) assemblyCueKind {
	c := msg.Content
	switch {
	case strings.HasPrefix(c, replyReminderCueMarker):
		return cueKindReplyReminder
	case strings.HasPrefix(c, toolCatalogCueMarker):
		return cueKindToolCatalog
	case strings.HasPrefix(c, orchBriefCueMarker):
		return cueKindOrchBrief
	case strings.HasPrefix(c, workspaceSkillsCueMarker):
		return cueKindWorkspaceSkills
	case strings.HasPrefix(c, skillGuidanceCueMarker):
		return cueKindSkillGuidance
	case strings.HasPrefix(c, knowledgeCueMarker):
		return cueKindKnowledge
	case strings.HasPrefix(c, memoryInjectMarker):
		return cueKindMemory
	default:
		return cueKindProtected
	}
}

// assemblyDroppedCue 记录一段被丢弃的 cue（flowlog  payload）。
type assemblyDroppedCue struct {
	Kind      string `json:"kind"`
	EstTokens int    `json:"est_tokens"`
}

// assemblyTrimStats 是单次截断的台账。
type assemblyTrimStats struct {
	EstBefore       int
	EstAfter        int
	DroppedCues     []assemblyDroppedCue
	EvictedMessages int
	EvictedChars    int
	// HeadOverBudget=true 表示丢弃+驱逐后仍超 target（静态头过大）——
	// 放行不阻断（K3），仅靠日志/flowlog 曝光。
	HeadOverBudget bool
}

// newAssemblyBudgetBeforeHook 构建装配预算闸；hard<=0（默认）返回 nil。
func newAssemblyBudgetBeforeHook(ag biz.Agent, deps TRPCBuilderDeps) callbacks.Callback {
	if ag.Settings == nil || ag.Settings.AssemblyBudgetHardTokens <= 0 {
		return nil
	}
	hard := ag.Settings.AssemblyBudgetHardTokens
	soft := ag.Settings.AssemblyBudgetSoftTokens
	if soft <= 0 {
		// soft 未配时默认 2/3 hard（60K hard → 40K soft，对齐计划值）。
		soft = hard * 2 / 3
	}
	if soft > hard {
		soft = hard
	}
	target := int(float64(hard) * assemblyBudgetTargetFactor)
	lg := deps.Logger()
	agentKey := ag.AgentKey
	return callbacks.NewBeforeModelHook(assemblyBudgetHookPriority, callbacks.LayerDynamic, func(ctx context.Context, args *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error) {
		if args == nil || args.Request == nil {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		msgs := args.Request.Messages
		est := analyzePromptRequest(msgs).EstTokens
		if est <= soft {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		// soft 越线（R2 MemGPT）：once-per-turn 容量告警，引导 LLM 主动
		// evict/落盘，而非被动等 hard 截断。markAssemblyBudgetWarned 返回
		// true=已告过（跳过），false=本 turn 首次（注入）。
		if !markAssemblyBudgetWarned(ctx) {
			warn := buildAssemblyBudgetWarning(est, soft, hard)
			msgs = appendDynamicCue(msgs, assemblyBudgetWarnMarker+warn)
			args.Request.Messages = msgs
			est = analyzePromptRequest(msgs).EstTokens
			lg.Info("assembly soft budget crossed, capacity warning injected",
				loggateway.StepID("agent.assembly_budget"),
				loggateway.AgentKey(agentKey),
				loggateway.Int("est_tokens", est),
				loggateway.Int("soft_tokens", soft),
				loggateway.Int("hard_tokens", hard))
		}
		if est <= hard {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		trimmed, stats := trimAssemblyToBudget(msgs, target)
		args.Request.Messages = trimmed
		lg.Warn("assembly over hard budget, trimmed",
			loggateway.StepID("agent.assembly_budget"),
			loggateway.AgentKey(agentKey),
			loggateway.Int("est_before", stats.EstBefore),
			loggateway.Int("est_after", stats.EstAfter),
			loggateway.Int("target_tokens", target),
			loggateway.Int("dropped_cues", len(stats.DroppedCues)),
			loggateway.Int("evicted_messages", stats.EvictedMessages),
			loggateway.Bool("head_over_budget", stats.HeadOverBudget))
		emitAssemblyTrimmedFlowLog(ctx, agentKey, stats, soft, hard, target)
		return &trpcmodel.BeforeModelResult{Context: ctx}, nil
	})
}

// markAssemblyBudgetWarned 实现 once-per-turn：本 turn 首次调用返回 false
// 并置标记；后续（含工具循环续轮）返回 true。无 invocation（测试/后台路径）
// 返回 true——告警依赖 invocation state 去重，无载体则跳过（防每轮重注）。
func markAssemblyBudgetWarned(ctx context.Context) bool {
	inv, ok := trpcagent.InvocationFromContext(ctx)
	if !ok || inv == nil {
		return true
	}
	if v, ok := inv.GetState(assemblyBudgetWarnStateKey); ok {
		if b, ok := v.(bool); ok && b {
			return true
		}
	}
	inv.SetState(assemblyBudgetWarnStateKey, true)
	return false
}

// buildAssemblyBudgetWarning 是容量告警 cue 文本（R2，MemGPT 式主动管理引导）。
func buildAssemblyBudgetWarning(est, soft, hard int) string {
	return fmt.Sprintf(`<context_budget_notice>
This request is using approximately %d tokens, above the soft assembly budget (%d; hard limit %d). When the hard limit is crossed, reference material and older history are discarded automatically. Actively preserve critical state now: write durable facts to working memory, checkpoint long-running subtasks, and keep the reply focused on the current question.
</context_budget_notice>`, est, soft, hard)
}

// trimAssemblyToBudget 把完全注入的请求截断到 target（估 token）。两段降级：
// 先按保护序丢尾部 cue（参考资料先于对话历史牺牲——R1：高价值头尾保留），
// 再驱逐最旧历史（tool-pair 安全）。静态头骨架永保。
func trimAssemblyToBudget(msgs []trpcmodel.Message, target int) ([]trpcmodel.Message, assemblyTrimStats) {
	stats := assemblyTrimStats{EstBefore: analyzePromptRequest(msgs).EstTokens}

	// 段 1：尾部 cue 降级链。按 drop rank 降序逐段丢弃直至达标；
	// protected（未标记）段永不进候选。
	head, conv, tail := splitPromptZones(msgs)
	type cand struct {
		idx  int
		rank int
		kind assemblyCueKind
		tok  int
	}
	var cands []cand
	for i, m := range tail {
		kind := classifyAssemblyCue(m)
		rank, ok := assemblyCueDropRank[kind]
		if !ok {
			continue
		}
		cands = append(cands, cand{idx: i, rank: rank, kind: kind, tok: estTokensFromChars(messageCharLen(m))})
	}
	sort.SliceStable(cands, func(a, b int) bool { return cands[a].rank > cands[b].rank })
	dropped := make(map[int]bool, len(cands))
	est := stats.EstBefore
	for _, c := range cands {
		if est <= target {
			break
		}
		dropped[c.idx] = true
		est -= c.tok
		stats.DroppedCues = append(stats.DroppedCues, assemblyDroppedCue{Kind: string(c.kind), EstTokens: c.tok})
	}
	if len(dropped) > 0 {
		newTail := make([]trpcmodel.Message, 0, len(tail)-len(dropped))
		for i, m := range tail {
			if !dropped[i] {
				newTail = append(newTail, m)
			}
		}
		merged := make([]trpcmodel.Message, 0, len(head)+len(conv)+len(newTail))
		merged = append(merged, head...)
		merged = append(merged, conv...)
		merged = append(merged, newTail...)
		msgs = merged
		est = analyzePromptRequest(msgs).EstTokens
	}

	// 段 2：仍超 target 才驱逐最旧历史（与压缩闸同构：token 口径 + 标记
	// 预留 + tool-pair 安全 + 标记落点在静态头之后）。
	if est > target {
		head2, _, _ := splitPromptZones(msgs)
		markerReserve := estTokensFromChars(utf8.RuneCountInString(buildTruncationMarker(0)))
		keep, evicted := partitionMessagesByTokenBudget(msgs, target-markerReserve)
		if len(evicted) > 0 {
			for _, m := range evicted {
				stats.EvictedChars += len(m.Content)
			}
			stats.EvictedMessages = len(evicted)
			marker := trpcmodel.NewSystemMessage(buildTruncationMarker(len(evicted)))
			msgs = insertMarkerAfterHead(keep, len(head2), marker)
		}
		est = analyzePromptRequest(msgs).EstTokens
	}

	stats.EstAfter = est
	stats.HeadOverBudget = est > target
	return msgs, stats
}

// emitAssemblyTrimmedFlowLog 落 flowlog `prompt.assembly.trimmed`（各段截断
// 量）。无 emitter 的 ctx（后台/测试路径）静默跳过。
func emitAssemblyTrimmedFlowLog(ctx context.Context, agentKey string, stats assemblyTrimStats, soft, hard, target int) {
	em := event.TraceEmitterFromContext(ctx)
	if em == nil {
		return
	}
	cuesJSON, err := json.Marshal(stats.DroppedCues)
	if err != nil {
		cuesJSON = []byte("[]")
	}
	em.LogWarn("prompt.assembly.trimmed", "装配预算截断",
		fmt.Sprintf("est %d→%d tokens（hard %d / target %d）：丢 cue %d 段、逐历史 %d 条",
			stats.EstBefore, stats.EstAfter, hard, target, len(stats.DroppedCues), stats.EvictedMessages),
		event.P("agent_key", agentKey),
		event.P("est_before", stats.EstBefore),
		event.P("est_after", stats.EstAfter),
		event.P("soft_tokens", soft),
		event.P("hard_tokens", hard),
		event.P("target_tokens", target),
		event.P("dropped_cues", string(cuesJSON)),
		event.P("evicted_messages", stats.EvictedMessages),
		event.P("evicted_chars", stats.EvictedChars),
		event.P("head_over_budget", stats.HeadOverBudget),
	)
}
