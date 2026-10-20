package knowledge

import (
	"context"

	bizknowledge "aranea-agents/internal/biz/knowledge"
)

// TaskTypeGraphCard G-33 卡片嵌入的 usage 记账口径第三值（ADR-KN-20261020-I：
// 与 RETRIEVAL_QUERY/” 并列拆分观测）。仅用于 usage 元数据——不传入 provider
// 任务提示（非 Gemini 枚举，embedGeminiBatch 白名单收缴为 RETRIEVAL_DOCUMENT）。
const TaskTypeGraphCard = "graph_card"

// graphCardEmbedder 图谱卡片投影工人的嵌入适配：复用嵌入漏斗单点
// （embedBatchWithTaskType——带用量记录/错误映射/超时纪律）。
type graphCardEmbedder struct {
	e *MultiProviderEmbedder
}

// NewGraphCardEmbedder 装配点；e 为 nil 视为未接线（工人 RunOnce 报装配错）。
func NewGraphCardEmbedder(e *MultiProviderEmbedder) bizknowledge.GraphCardEmbedder {
	if e == nil {
		return nil
	}
	return &graphCardEmbedder{e: e}
}

func (a *graphCardEmbedder) EmbedGraphCardTexts(ctx context.Context, texts []string) ([][]float32, error) {
	return a.e.EmbedBatchWithTaskType(ctx, texts, TaskTypeGraphCard)
}
