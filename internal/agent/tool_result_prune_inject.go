package agent

// tool_result_prune_inject.go — R2 确定性工具结果剪枝（79-runtime-governance
// design §3）。BeforeModel hook，priority 7（LayerDynamic）：全量注入类 hook
// （memory 5 / knowledge 6 / cue ≤6）之后、装配预算（8）与终审压缩（9）之前——
// 剪枝削峰 → 阈值压缩兜底（§3.1）。两轮驱动日志分别打 prune / compact 标签。
//
// 语义（§3.2）：扫描会话历史中的 RoleTool 结果消息，同时满足「距当前轮 > K 轮」
// 且「序列化大小 > S 字节」时，内容替换为摘记指针
//   [已剪枝 tool_result｜原 N 字节｜tool=<name>｜blob=<id>｜取回: read_tool_result]
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
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// ToolResultPruneConfig 是剪枝 hook 的消费侧配置。wire 由
// conf.Runtime.ToolResultPruneConfig() 翻译注入（保持 agent 包不依赖
// internal/conf）。零值语义：Enabled=false 时 hook 为 nil（一键回退）。
type ToolResultPruneConfig struct {
	Enabled     bool
	AfterTurns  int               // K：距当前轮超过 K 轮才剪（默认 8）
	SizeBytes   int64             // S：序列化大小阈值（默认 4096）
	ExemptTools map[string]bool   // 白名单工具名：永不剪（取证关键类）
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
func newToolResultPruneBeforeHook(gate *biz.ToolResultGate, cfg ToolResultPruneConfig, lg loggateway.Logger, decisions decision.Collector) callbacks.Callback {
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
	return callbacks.NewBeforeModelHook(7, callbacks.LayerDynamic, func(ctx context.Context, args *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error) {
		if args == nil || args.Request == nil {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		sessionID := sessionIDFromInvocationContext(ctx)
		if sessionID == "" {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		msgs := args.Request.Messages
		userAfter := realUserMessagesAfterEachIndex(msgs)
		turnNumber := estimateTurnNumber(msgs)

		var pruned int
		var prunedBytes int64
		for i := range msgs {
			msg := &msgs[i]
			if msg.Role != trpcmodel.RoleTool {
				continue
			}
			// 豁免：本轮与最近 K 轮内（距当前轮 ≤ K）。
			if userAfter[i] <= afterTurns {
				continue
			}
			// 豁免：白名单工具（取证关键类）。
			if cfg.ExemptTools[msg.ToolName] {
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
			res, err := gate.Archive(ctx, sessionID, toolID, msg.ToolName, content, turnNumber)
			if err != nil || !res.DidPersist {
				// fail-soft：归档失败保留原文，本轮不省但不丢内容。
				lg.Warn("tool result prune: archive failed, keeping original",
					loggateway.StepID("agent.tool_result.prune"),
					loggateway.Phase("degraded"),
					loggateway.Str("session_id", sessionID),
					loggateway.Str("tool", msg.ToolName),
					loggateway.Err(err))
				continue
			}
			msg.Content = biz.ToolResultPrunePointer(len(content), msg.ToolName, res.BlobID)
			msg.ContentParts = nil
			pruned++
			prunedBytes += int64(len(content))
		}

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
	if c == nil {
		return
	}
	runID := gateRunID(ctx)
	decision.EmitGate(ctx, c, decision.GateDecision{
		TriggerRule:   decision.TriggerToolResultPruned,
		Outcome:       "truncated",
		Scenario:      "工具结果确定性剪枝",
		Reasoning:     fmt.Sprintf("距当前轮超 K 轮且超尺寸阈值的 tool result 已替换为摘记指针（%d 条 / %d 字节）", pruned, prunedBytes),
		GuardName:     "tool_result_prune",
		RunID:         runID,
		ObservedValue: pruned,
		Action:        "prune",
		Extra:         map[string]any{"prune_bytes": prunedBytes, "session_id": sessionID},
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
