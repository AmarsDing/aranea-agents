package agent

// tool_result_prune_inject.go — R2 确定性工具结果剪枝（79-runtime-governance
// design §3）。BeforeModel hook，priority 7（LayerDynamic）：全量注入类 hook
// （memory 5 / knowledge 6 / cue ≤6）之后、装配预算（8）与终审压缩（9）之前——
// 剪枝削峰 → 阈值压缩兜底（§3.1）。两轮驱动日志分别打 prune / compact 标签。
//
// 语义（§3.2）：扫描会话历史中的 RoleTool 结果消息，同时满足「距当前轮 > K 轮」
// 且「序列化大小 > S 字节」时，内容替换为摘记指针
//   [已剪枝 tool_result｜原 N 字节｜tool=<name>｜blob=<id>｜取回: 调用 read_tool_result(blob_id=...)]
// 原文经 ToolResultGate.Archive 写入既有 blob 设施（不新造存储），取回复用
// read_tool_result 工具（E2 裁定）。
//
// 契约相容性：
//   - pair 完整：只替换 result 消息体，不动对应 tool_call 消息；不删除/重排任何
//     消息（append-only 语义保持，C2 精神）。
//   - 幂等稳定：replacement 记录（sessionID+ToolID）钉住 blob id，指针文本跨轮
//     字节稳定——剪枝造成的 conv 就地改写只在首轮发生，其后前缀恢复稳定（与
//     F-A2 历史就地改写同类：request 副本，会话存储原文不动）。
//   - 豁免（§3.2）：本轮与最近 K 轮内的 result、错误结果（失败重试证据，
//     "Error:" 前缀约定）、白名单工具（runtime.tool_result_prune.exempt_tools）。
//   - fail-soft：blob 归档失败保留原文不剪（宁可本轮不省，不丢内容）。

import (
	"context"
	"fmt"
	"strings"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/decision"
	"aranea-agents/internal/event"
	"aranea-agents/internal/toolspill"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// ToolResultPruneConfig 是剪枝 hook 的消费侧配置。wire 由
// conf.Runtime.ToolResultPruneConfig() 翻译注入（保持 agent 包不依赖
// internal/conf）。零值语义：Enabled=false 时 hook 为 nil（一键回退）。
type ToolResultPruneConfig struct {
	Enabled     bool
	AfterTurns  int             // K：距当前轮超过 K 轮才剪（默认 8）
	SizeBytes   int64           // S：序列化大小阈值（默认 4096）
	ExemptTools map[string]bool // 白名单工具名：永不剪（取证关键类）
	// RetrievalExempt 是 A8 检索豁免默认化开关（P0-3 设计 §4.4③，审批注记
	// §12.2 P0-3③）。nil = ON（默认启用内置检索豁免表
	// DefaultRetrievalExemptTools）；显式 false = 回到 A8 之前的空豁免表
	// 行为。覆盖语义（审批"替换而非合并、语义才无歧义"）：ExemptTools 非
	// nil 时整体替换内置表——conf 已有 exempt_tools 配置的部署若仍需检索
	// 豁免，须把六枚内置名显式写入自己的清单（批 B conf 透出时登记默认值）。
	RetrievalExempt *bool
}

// pruneMetaStateKey 是最近一轮剪枝统计的 invocation-state 键，供 R7 run 统计
// 读取（prune_count / prune_bytes，design §3.4）。
const pruneMetaStateKey = "aranea:tool_result_prune_meta"

// PruneMeta 记录最近一次剪枝结果（本次 model 调用）。
type PruneMeta struct {
	// Count 是本轮被剪枝的 result 消息条数（prune_count）。
	Count int
	// Bytes 是被剪枝原文的总字节数（prune_bytes，按替换前序列化大小计）。
	Bytes int64
}

// newToolResultPruneBeforeHook 创建剪枝 BeforeModel hook。gate 为 nil（无 blob
// 设施）或配置关闭时返回 nil——轻链路零开销，回退项
// runtime.tool_result_prune.enabled=false 即走此路。
// decisions 是 R7（G-1）剪枝事件的结构化持久化口：每次实际剪枝（pruned>0）
// 双写一条 system_guard 决策记录（trigger_rule=tool_result_pruned，
// outcome=truncated），run 统计经 decision_records 聚合回放；nil 时仅跳过
// 记录（loopGuard 同款可选语义），剪枝主流程不受影响。
func newToolResultPruneBeforeHook(gate *biz.ToolResultGate, cfg ToolResultPruneConfig, lg loggateway.Logger, decisions decision.Collector, spillOpt ...*toolspill.Deps) callbacks.Callback {
	if gate == nil || !cfg.Enabled {
		return nil
	}
	afterTurns := cfg.AfterTurns
	if afterTurns <= 0 {
		afterTurns = 8
	}
	sizeBytes := cfg.SizeBytes
	if sizeBytes <= 0 {
		sizeBytes = 4096
	}
	exempt := resolvePruneExemptTools(cfg)
	return callbacks.NewBeforeModelHook(7, callbacks.LayerDynamic, func(ctx context.Context, args *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error) {
		if args == nil || args.Request == nil {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		sessionID := sessionIDFromInvocationContext(ctx)
		if sessionID == "" {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		msgs := args.Request.Messages
		turnNumber := estimateTurnNumber(msgs)

		pruned, prunedBytes := pruneToolResultMessages(msgs, exempt, afterTurns, sizeBytes,
			func(toolID, toolName, content string) (string, bool) {
				res, err := gate.Archive(ctx, sessionID, toolID, toolName, content, turnNumber)
				if err != nil || !res.DidPersist {
					// fail-soft：归档失败保留原文，本轮不省但不丢内容。
					lg.Warn("tool result prune: archive failed, keeping original",
						loggateway.StepID("agent.tool_result.prune"),
						loggateway.Phase("degraded"),
						loggateway.Str("session_id", sessionID),
						loggateway.Str("tool", toolName),
						loggateway.Err(err))
					return "", false
				}
				return res.BlobID, true
			})

		if pruned > 0 {
			storePruneMeta(ctx, PruneMeta{Count: pruned, Bytes: prunedBytes})
			lg.Info("tool result prune completed",
				loggateway.StepID("agent.tool_result.prune"),
				loggateway.Phase("done"),
				loggateway.Str("session_id", sessionID),
				loggateway.Int("prune_count", pruned),
				loggateway.Int64("prune_bytes", prunedBytes))
			emitPruneGateDecision(ctx, decisions, sessionID, pruned, prunedBytes)
		}
		return &trpcmodel.BeforeModelResult{Context: ctx}, nil
	})
}

// emitPruneGateDecision 把一次实际剪枝双写为 system_guard 决策记录（R7 G-1）。
// run 归属经 gateRunID（2026-08-27 H5 根修）：team 图谱成员节点取 ctx 注入的
// team run id——框架 Clone 补丁前成员 invocation 共享 run.ID，补丁后成员获新
// uuid，InvocationID 不再等于 run 归属；chat 轮次回落 chat invocation id
// （不 join 任何 team run，stats 聚合自然忽略）。
func emitPruneGateDecision(ctx context.Context, c decision.Collector, sessionID string, pruned int, prunedBytes int64) {
	runID := gateRunID(ctx)
	event.EmitGate(ctx, c, decision.GateDecision{
		TriggerRule:   decision.TriggerToolResultPruned,
		Outcome:       "truncated",
		Scenario:      "工具结果确定性剪枝",
		Reasoning:     fmt.Sprintf("距当前轮超 K 轮且超尺寸阈值的 tool result 已替换为摘记指针（%d 条 / %d 字节）", pruned, prunedBytes),
		GuardName:     "tool_result_prune",
		RunID:         runID,
		SessionID:     sessionID,
		ObservedValue: pruned,
		Action:        "prune",
		Extra:         map[string]any{"prune_bytes": prunedBytes},
	})
}

// realUserMessagesAfterEachIndex 计算每个下标之后（不含自身）的真实用户消息
// 条数，作为「距当前轮数」——dynamic cue 是 user-role 哨兵消息，必须从轮数
// 口径中剔除，否则历史消息的轮距被 tail cue 数量虚增。
func realUserMessagesAfterEachIndex(msgs []trpcmodel.Message) []int {
	counts := make([]int, len(msgs))
	acc := 0
	for i := len(msgs) - 1; i >= 0; i-- {
		counts[i] = acc
		if msgs[i].Role == trpcmodel.RoleUser && !isDynamicCueMessage(msgs[i]) {
			acc++
		}
	}
	return counts
}

// storePruneMeta 把本轮剪枝统计写入 invocation state（R7 run 统计读取点）。
func storePruneMeta(ctx context.Context, meta PruneMeta) {
	inv, ok := trpcagent.InvocationFromContext(ctx)
	if !ok || inv == nil {
		return
	}
	inv.SetState(pruneMetaStateKey, meta)
}

// LoadPruneMeta 读取最近一次剪枝统计；本轮未剪枝时返回零值（Count=0）。
// 导出供 R7 run 统计聚合（design §3.4）。
func LoadPruneMeta(ctx context.Context) PruneMeta {
	inv, ok := trpcagent.InvocationFromContext(ctx)
	if !ok || inv == nil {
		return PruneMeta{}
	}
	if v, ok := inv.GetState(pruneMetaStateKey); ok {
		if m, ok := v.(PruneMeta); ok {
			return m
		}
	}
	return PruneMeta{}
}

// ---------------------------------------------------------------------------
// A8 — 检索豁免默认化 + eval 共用剪枝体（P0-3 设计 §4.4③ / §12.2 P0-3③）
// ---------------------------------------------------------------------------

// DefaultRetrievalExemptTools 是内置检索豁免表（六枚）：检索/取回类工具的
// result 永不折叠——它们是知识回读的源头，折叠后 readback 断链（P0-2 契约：
// toolspill 定位符必须可读回，检索族自身的指针更不能失水）。conf 的
// ExemptTools 非 nil 时整体替换本表（审批"替换而非合并、语义才无歧义"）。
var DefaultRetrievalExemptTools = map[string]bool{
	"knowledge_search": true,
	"knowledge_query":  true,
	"read_document":    true,
	"web_fetch":        true,
	"read_tool_result": true,
	"read_file":        true,
}

// resolvePruneExemptTools 解析生效的豁免表：ExemptTools 非 nil → 整体替换
// 内置表；否则 RetrievalExempt 显式 false → 空表（回到 A8 之前行为）；否则
// 启用内置检索豁免表（nil = ON 默认）。
func resolvePruneExemptTools(cfg ToolResultPruneConfig) map[string]bool {
	if cfg.ExemptTools != nil {
		return cfg.ExemptTools
	}
	if cfg.RetrievalExempt != nil && !*cfg.RetrievalExempt {
		return nil
	}
	return DefaultRetrievalExemptTools
}

// pruneToolResultMessages 是剪枝 hook 与压缩金闸门（internal/session/eval）
// 共用的确定性剪枝体，语义与 hook 内联历史实现逐项一致（§3.2 豁免）：
//   - 本轮与最近 K 轮内（距当前轮 ≤ K）的 result 不剪；
//   - 豁免表工具不剪；
//   - 序列化大小未超阈值不剪；
//   - 错误结果（"Error:" 前缀约定，失败重试证据）不剪；
//   - ToolID 为空按下标补 auto_%d（append-only 历史内稳定）；
//   - 剪中则替换为 biz.ToolResultPrunePointer 指针并清空 ContentParts；
//   - fail-soft：archive ok=false 保留原文（宁可本轮不省，不丢内容）。
//
// 剪枝就地改写传入副本（调用方负责传副本语义，hook 中是 request 副本）。
// 返回 (剪枝条数, 被剪原文总字节数)。
func pruneToolResultMessages(msgs []trpcmodel.Message, exempt map[string]bool, afterTurns int, sizeBytes int64, archive func(toolID, toolName, content string) (locator string, ok bool)) (pruned int, prunedBytes int64) {
	userAfter := realUserMessagesAfterEachIndex(msgs)
	for i := range msgs {
		msg := &msgs[i]
		if msg.Role != trpcmodel.RoleTool {
			continue
		}
		// 豁免：本轮与最近 K 轮内（距当前轮 ≤ K）。
		if userAfter[i] <= afterTurns {
			continue
		}
		// 豁免：白名单/内置检索表工具（取证关键类 + 检索族）。
		if exempt[msg.ToolName] {
			continue
		}
		content := extractTextContent(msg)
		// 豁免：未超尺寸阈值。
		if int64(len(content)) <= sizeBytes {
			continue
		}
		// 豁免：错误结果（失败重试证据，"Error: " 前缀为框架+产品约定）。
		if strings.HasPrefix(strings.TrimSpace(content), "Error:") {
			continue
		}

		toolID := msg.ToolID
		if toolID == "" {
			// 与 tool_result_gate_hook 同规则：下标在 append-only 历史内稳定。
			toolID = fmt.Sprintf("auto_%d", i)
		}
		locator, ok := archive(toolID, msg.ToolName, content)
		if !ok {
			continue
		}
		msg.Content = biz.ToolResultPrunePointer(len(content), msg.ToolName, locator)
		msg.ContentParts = nil
		pruned++
		prunedBytes += int64(len(content))
	}
	return pruned, prunedBytes
}

// RunToolResultPruneForEval 是压缩金闸门（internal/session/eval 的
// CheckCompressionGoldGate 驱动口）的整合入口：用与 hook 完全同源的剪枝体
// 跑一份会话导出副本，豁免表解析与默认阈值全走 hook 同路径，确保 gate 断言
// 的就是线上行为。archive 由评测侧提供（内存 map 即可，模拟 toolspill 取回）。
// AS-EVT-01：Informational——本函数不发任何事件（离线评测通路）。
func RunToolResultPruneForEval(msgs []trpcmodel.Message, cfg ToolResultPruneConfig, archive func(toolID, toolName, content string) (locator string, ok bool)) PruneMeta {
	afterTurns := cfg.AfterTurns
	if afterTurns <= 0 {
		afterTurns = 8
	}
	sizeBytes := cfg.SizeBytes
	if sizeBytes <= 0 {
		sizeBytes = 4096
	}
	pruned, prunedBytes := pruneToolResultMessages(msgs, resolvePruneExemptTools(cfg), afterTurns, sizeBytes, archive)
	return PruneMeta{Count: pruned, Bytes: prunedBytes}
}
