package service

import (
	"context"
	"fmt"

	v1 "aranea-agents/api/kratos/knowledge/v1"
	"aranea-agents/internal/biz"
	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/internal/event"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/appctx"
	"aranea-agents/pkg/safego"
)

// ── G-30：CurateKnowledgeNow RPC（治理仪表盘手动触发入口）────────────────────

// CurateKnowledgeNow 对一个库立即触发一轮词条治理（同 dream 周期 CurateKnowledge
// 语义：decay/promote/stale 自动项 + conflict/orphan/moc_emerge 高风险提案探测；
// 此前只能等 cron 或 butler 工具间接触达）。异步受理：同库在途治理返回 409
// Conflict（job key knowledge-curate:<id>，跨进程 advisory lock + 本进程
// sync.Map 双层门——cron 与手动入口共用同键，单库一轮探测的幂等性不受重入
// 双倍提案破坏）；进度/终态经 knowledge WS 频道广播 event_type=knowledge_curate。
// dry_run=true 仅探测不应用（decay/置位/提案均不落库），用于治理效果预演。
func (s *KnowledgeService) CurateKnowledgeNow(ctx context.Context, req *v1.CurateKnowledgeNowRequest) (*v1.CurateKnowledgeNowResponse, error) {
	if err := assertKnowledgeGovernanceAdmin(ctx); err != nil {
		return nil, err
	}
	col, err := s.uc.GetCollection(ctx, req.GetCollectionId())
	if err != nil {
		return nil, err
	}
	if err := s.assertCollectionMutateAccess(ctx, col); err != nil {
		return nil, err
	}
	release, err := s.acquireJob(ctx, &s.curateRuns, col.ID, knowledgeCurateJobKey(col.ID),
		"knowledge curate already running for collection %s", col.ID)
	if err != nil {
		return nil, err
	}

	flow := s.knowledgeFlow(ctx)
	flow.LogStart("knowledge.curate", "知识库词条治理",
		event.P("collection_id", col.ID),
		event.P("dry_run", req.GetDryRun()))

	bgCtx := workspace.WithSystemWorkspace(appctx.Ctx())
	safego.Go(bgCtx, "knowledge.curate."+col.ID, func() {
		defer release()
		s.publishKnowledgeCurate(col.ID, "curating", "")
		rep, cerr := s.uc.CurateKnowledge(bgCtx, bizknowledge.CurateOptions{
			CollectionID: col.ID,
			Workspace:    col.Workspace,
			DryRun:       req.GetDryRun(),
		})
		if cerr != nil {
			s.publishKnowledgeCurate(col.ID, "error", cerr.Error())
			flow.LogError("knowledge.curate", "知识库词条治理失败",
				event.P("collection_id", col.ID),
				event.P("error", cerr.Error()))
			return
		}
		s.publishKnowledgeCurate(col.ID, "done", "")
		flow.LogDone("knowledge.curate", "知识库词条治理完成",
			event.P("collection_id", col.ID),
			event.P("actions", fmt.Sprintf("%v", rep.Actions)),
			event.P("dry_run", rep.DryRun))
	})
	return &v1.CurateKnowledgeNowResponse{Status: "curating"}, nil
}

// publishKnowledgeCurate 广播手动治理进度/终态事件（EP-KN-02 模式：
// v2 SystemNoticeEvent，WS-only 不持久化；eventBus 为 nil 时跳过）。
func (s *KnowledgeService) publishKnowledgeCurate(collectionID, status, errMsg string) {
	if s.eventBus == nil {
		return
	}
	meta := map[string]any{
		"event_type":    "knowledge_curate",
		"collection_id": collectionID,
		"status":        status,
		"error_message": errMsg,
	}
	msg := "Knowledge curate: " + status
	s.eventBus.Publish(context.Background(), biz.NewSystemNoticeEvent("", "knowledge_curate", msg, meta))
}
