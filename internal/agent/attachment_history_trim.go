package agent

// attachment_history_trim.go — T-1b 历史图片裁剪（2026-10-26 评审二轮）。
//
// 病灶：trpc 会话历史中的图片 part 每轮请求全量内联 replay（10MB 图 ≈
// 13.3MB base64/轮）。首轮模型已从该图取到答案，后续轮次重放纯烧 token
// 并把压缩/预算闸门推向误触发。
//
// 策略：BeforeModel hook，priority 7（LayerDynamic，与 R2 工具结果剪枝同
// 带——注入类 ≤6 之后、装配预算 8 与终审压缩 9 之前，削峰先行、压缩兜底
// 的次序不变）。把「最后一条带图消息之前」的所有图片 part 替换为文本占位
// （最近一条带图消息保留——它就是当前轮用户输入或紧邻上文）。
//
// 契约相容性（与 R2 剪枝同源）：
//   - 只改写 request 副本——trpc 会话存储原文不动，重放裁剪每轮重算；
//   - 不删除/重排消息，pair 完整性与 append-only 语义保持；
//   - 确定性纯函数，无 I/O、无配置项（裁剪不产生归档副作用，无需回退开关）。

import (
	"context"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/pkg/loggateway"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// imageHistoryPlaceholder 是历史图片 part 的文本占位（替换后模型仍知晓
// 该轮曾有图片输入，需要取回时可直接说明）。
const imageHistoryPlaceholder = "[历史图片附件已省略]"

// trimHistoricalImageParts 把最后一条带图消息之前的全部图片 part 替换为
// 文本占位，返回裁剪的 part 数。msgs 为 request 副本（调用方负责副本语义）。
func trimHistoricalImageParts(msgs []trpcmodel.Message) (trimmed int) {
	lastImg := -1
	for i := range msgs {
		if messageImagePartCount(msgs[i]) > 0 {
			lastImg = i
		}
	}
	if lastImg <= 0 {
		return 0
	}
	for i := 0; i < lastImg; i++ {
		msg := &msgs[i]
		if messageImagePartCount(*msg) == 0 {
			continue
		}
		kept := make([]trpcmodel.ContentPart, 0, len(msg.ContentParts))
		for _, p := range msg.ContentParts {
			if p.Type == trpcmodel.ContentTypeImage && p.Image != nil {
				text := imageHistoryPlaceholder
				kept = append(kept, trpcmodel.ContentPart{Type: trpcmodel.ContentTypeText, Text: &text})
				trimmed++
				continue
			}
			kept = append(kept, p)
		}
		msg.ContentParts = kept
	}
	return trimmed
}

// newImageHistoryTrimBeforeHook 创建历史图片裁剪 BeforeModel hook。
// 无条件注册：纯 request 副本改写，零副作用，轻链路开销为一次线性扫描。
// 返回具体类型以便测试直接调用 HandleBeforeModel（同 context_budget 惯例）。
func newImageHistoryTrimBeforeHook(lg loggateway.Logger) *callbacks.BeforeModelHookFunc {
	return callbacks.NewBeforeModelHook(7, callbacks.LayerDynamic, func(ctx context.Context, args *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error) {
		if args == nil || args.Request == nil {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		if trimmed := trimHistoricalImageParts(args.Request.Messages); trimmed > 0 && lg != nil {
			lg.Info("image history trim completed",
				loggateway.StepID("agent.image_history.trim"),
				loggateway.Phase("done"),
				loggateway.Str("session_id", sessionIDFromInvocationContext(ctx)),
				loggateway.Int("trimmed_parts", trimmed))
		}
		return &trpcmodel.BeforeModelResult{Context: ctx}, nil
	})
}
