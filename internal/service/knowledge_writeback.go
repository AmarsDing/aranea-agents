package service

import (
	"context"

	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/pkg/loggateway"
)

// WriteBackSessionFacts 透传 biz 写回（auto_memory worker 经 wire 以
// biz.KnowledgeWriteBack 接口消费本方法）。chunk 重放在 biz 层 writeBackFacts
// 尾部经 SetWriteBackReplay 钩子收口——直写（knowledge_write 工具）与本路径
// 共用同一重放。此前重放挂在本包装方法内，knowledge_write 工具直调 biz
// Usecase 绕过，entries/* 永久 pending（2026-08-15 事故）。
func (s *KnowledgeService) WriteBackSessionFacts(ctx context.Context, in bizknowledge.WriteBackInput) (bizknowledge.WriteBackResult, error) {
	return s.uc.WriteBackSessionFacts(ctx, in)
}

// replayWriteBackChunks 写回飞轮 chunk 重放钩子（注入 biz Usecase）。
func (s *KnowledgeService) replayWriteBackChunks(ctx context.Context, col bizknowledge.Collection, touched []bizknowledge.PromoteTouchedDoc) error {
	replayed, failed := s.replayPromotedDocChunks(ctx, col, touched)
	if failed > 0 {
		s.lg.Warn("写回飞轮 chunk 重放部分失败",
			loggateway.StepID("knowledge.writeback"),
			loggateway.Str("collection_id", col.ID),
			loggateway.Int("replayed", replayed),
			loggateway.Int("failed", failed),
		)
	}
	return nil
}
