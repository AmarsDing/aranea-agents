package service

import (
	"context"

	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/pkg/loggateway"
)

var _ bizknowledge.SessionWriteBack = (*KnowledgeService)(nil)
var _ bizknowledge.SessionWriteBackReview = (*KnowledgeService)(nil)
var _ bizknowledge.AgentMemoryProjectPort = (*KnowledgeService)(nil)

// WriteBackSessionFacts 会话过门事实写入团队库日记，并重放 chunk/FTS（SP7 G2）。
// 正文已落后 chunk 失败只 Warn，不回滚（与晋升 B-3 同哲学）。
func (s *KnowledgeService) WriteBackSessionFacts(ctx context.Context, in bizknowledge.WriteBackInput) (bizknowledge.WriteBackResult, error) {
	if s == nil || s.uc == nil {
		return bizknowledge.WriteBackResult{}, nil
	}
	res, err := s.uc.WriteBackSessionFacts(ctx, in)
	if err != nil {
		s.lg.Warn("写回飞轮落库失败",
			loggateway.StepID("knowledge.writeback"),
			loggateway.Str("session_id", in.SessionID),
			loggateway.Err(err),
		)
		return bizknowledge.WriteBackResult{}, err
	}
	if res.Appended == 0 || res.DocID == "" {
		return res, nil
	}
	col, err := s.uc.GetCollection(ctx, res.CollectionID)
	if err != nil {
		s.lg.Warn("写回飞轮读集合失败，跳过 chunk 重放",
			loggateway.StepID("knowledge.writeback"),
			loggateway.Str("collection_id", res.CollectionID),
			loggateway.Err(err),
		)
		return res, nil
	}
	replayed, failed := s.replayPromotedDocChunks(ctx, col, []bizknowledge.PromoteTouchedDoc{{
		DocID:   res.DocID,
		Created: res.Created,
	}})
	if failed > 0 {
		s.lg.Warn("写回飞轮 chunk 重放部分失败",
			loggateway.StepID("knowledge.writeback"),
			loggateway.Str("doc_id", res.DocID),
			loggateway.Int("replayed", replayed),
			loggateway.Int("failed", failed),
		)
	}
	return res, nil
}

// EnqueueWriteBackReview 低置信白名单事实进入 pending 日记（US-44）。
func (s *KnowledgeService) EnqueueWriteBackReview(ctx context.Context, in bizknowledge.WriteBackInput) (bizknowledge.WriteBackResult, error) {
	if s == nil || s.uc == nil {
		return bizknowledge.WriteBackResult{}, nil
	}
	return s.uc.EnqueueWriteBackReview(ctx, in)
}

// ProjectAgentMemory 覆盖投影 L3 活动事实到 agents/{id}.md（SP7 G1）。
func (s *KnowledgeService) ProjectAgentMemory(ctx context.Context, workspace, agentID string) error {
	if s == nil || s.agentMem == nil {
		return nil
	}
	res, err := s.agentMem.Project(ctx, workspace, agentID)
	if err != nil {
		s.lg.Warn("agent 记忆投影失败",
			loggateway.StepID("knowledge.memory.project"),
			loggateway.Str("agent_id", agentID),
			loggateway.Err(err),
		)
		return err
	}
	if res.DocID == "" || res.CollectionID == "" {
		return nil
	}
	col, err := s.uc.GetCollection(ctx, res.CollectionID)
	if err != nil {
		return nil
	}
	_, _ = s.replayPromotedDocChunks(ctx, col, []bizknowledge.PromoteTouchedDoc{{DocID: res.DocID}})
	return nil
}
