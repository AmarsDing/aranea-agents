package service

import (
	"context"
	"time"

	v1 "aranea-agents/api/kratos/knowledge/v1"
	"aranea-agents/internal/biz"
	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/internal/event"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/appctx"
	"aranea-agents/pkg/safego"
)

// ── SP1-H：RebuildKnowledgeIndex RPC（设计 S9 / US-29） ──────────────────────

// rebuildProgressInterval 进度 WS 事件时间窗限流（高频路径红线：禁止每文档一条）。
const rebuildProgressInterval = 200 * time.Millisecond

// RebuildKnowledgeIndex 流式重建集合全部文档的块级派生索引（blocks/refs）。
// 异步受理：同库在途重建返回 409 Conflict（单进程部署，N-1）；进度经 knowledge
// WS 频道广播（EP-KN-02 模式，event_type=knowledge_rebuild_index）；sync_state
// 进入 rebuilding 期间检索走旧 chunks/FTS（降级可用），完成后恢复原态。
// 幂等：中断/崩溃后重跑等价于从头执行（biz 层整文档删了重插语义）。
func (s *KnowledgeService) RebuildKnowledgeIndex(ctx context.Context, req *v1.RebuildKnowledgeIndexRequest) (*v1.RebuildKnowledgeIndexResponse, error) {
	col, err := s.uc.GetCollection(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	if err := s.assertCollectionMutateAccess(ctx, col); err != nil {
		return nil, err
	}
	// 冲突门：进程内在途集合门（崩溃残留的 sync_state=rebuilding 不阻塞——
	// biz 层按 active 恢复语义处理）。
	if _, loaded := s.rebuildRuns.LoadOrStore(col.ID, struct{}{}); loaded {
		return nil, apierror.Conflict("KNOWLEDGE", "block index rebuild already running for collection %s", col.ID)
	}

	flow := s.knowledgeFlow(ctx)
	flow.LogStart("knowledge.rebuild_index", "知识块索引重建",
		event.P("collection_id", col.ID))

	// 后台重建需跨 workspace 取文档正文（biz 逐文档 GetDocument，带 workspace
	// 过滤）：系统工作区旁路，避免非 default 集合重建时逐文档 NotFound。
	bgCtx := workspace.WithSystemWorkspace(appctx.Ctx())
	safego.Go(bgCtx, "knowledge.rebuild_index."+col.ID, func() {
		defer s.rebuildRuns.Delete(col.ID)
		s.publishKnowledgeRebuild(col.ID, "rebuilding", 0, 0, 0, "")
		lastPub := time.Now()
		res, err := s.uc.RebuildCollectionBlockIndex(bgCtx, col.ID, func(done, total, failed int) {
			if time.Since(lastPub) < rebuildProgressInterval {
				return
			}
			lastPub = time.Now()
			s.publishKnowledgeRebuild(col.ID, "rebuilding", done, total, failed, "")
		})
		if err != nil {
			s.publishKnowledgeRebuild(col.ID, "error", res.Done, res.Total, res.Failed, err.Error())
			flow.LogError("knowledge.rebuild_index", "知识块索引重建失败",
				event.P("collection_id", col.ID),
				event.P("error", err.Error()))
			return
		}
		s.publishKnowledgeRebuild(col.ID, "done", res.Done, res.Total, res.Failed, "")
		flow.LogDone("knowledge.rebuild_index", "知识块索引重建完成",
			event.P("collection_id", col.ID),
			event.P("total", res.Total),
			event.P("done", res.Done),
			event.P("failed", res.Failed))
	})

	return &v1.RebuildKnowledgeIndexResponse{Status: bizknowledge.SyncStateRebuilding}, nil
}

// publishKnowledgeRebuild 广播重建进度/终态事件（EP-KN-02 模式：v2
// SystemNoticeEvent，WS-only 不持久化；eventBus 为 nil 时跳过）。
func (s *KnowledgeService) publishKnowledgeRebuild(collectionID, status string, done, total, failed int, errMsg string) {
	if s.eventBus == nil {
		return
	}
	meta := map[string]any{
		"event_type":    "knowledge_rebuild_index",
		"collection_id": collectionID,
		"status":        status,
		"done":          done,
		"total":         total,
		"failed":        failed,
		"error_message": errMsg,
	}
	msg := "Knowledge block index rebuild: " + status
	s.eventBus.Publish(context.Background(), biz.NewSystemNoticeEvent("", "knowledge_rebuild", msg, meta))
}
