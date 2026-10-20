package service

import (
	"context"

	v1 "aranea-agents/api/kratos/knowledge/v1"
	"aranea-agents/internal/biz"
	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/internal/event"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/appctx"
	"aranea-agents/pkg/safego"
)

// ── G-11：ReindexVault RPC（全量重索引触达端点） ─────────────────────────────

// ReindexVault 以文件系统为唯一真相源强制重建 local vault 全部派生索引
// （chunks/向量/links，SP1 P1-4）——用于索引损坏、embedding 模型升级、分块策略
// 变更后的全量修复。异步受理：同库在途重索引返回 409 Conflict（job key
// knowledge-reindex:<id>，跨进程 advisory lock + 本进程 sync.Map 双层门）；
// 进度经 knowledge WS 频道广播（EP-KN-02 模式，event_type=knowledge_vault_reindex）。
// team 库（PG 即真相源）与无 root_path 的历史 collection 不适用，返回 BadRequest。
// 幂等：文件系统可重扫，ApplyEventsForced 删了重插；与周期 SyncOnce 幂等交错。
func (s *KnowledgeService) ReindexVault(ctx context.Context, req *v1.ReindexVaultRequest) (*v1.ReindexVaultResponse, error) {
	col, err := s.uc.GetCollection(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	if err := s.assertCollectionMutateAccess(ctx, col); err != nil {
		return nil, err
	}
	if col.VaultBackend == bizknowledge.VaultBackendTeam {
		return nil, apierror.BadRequest("KNOWLEDGE", "vault reindex is not applicable to team collections (PG is the source of truth)")
	}
	if col.RootPath == "" {
		return nil, apierror.BadRequest("KNOWLEDGE", "vault reindex requires a local vault with root_path")
	}
	if s.vaultSync == nil {
		return nil, apierror.Unavailable("KNOWLEDGE", "vault sync is not wired")
	}
	release, err := s.acquireJob(ctx, &s.reindexRuns, col.ID, knowledgeReindexJobKey(col.ID),
		"vault reindex already running for collection %s", col.ID)
	if err != nil {
		return nil, err
	}

	flow := s.knowledgeFlow(ctx)
	flow.LogStart("knowledge.vault_reindex", "知识库全量重索引",
		event.P("collection_id", col.ID))

	bgCtx := workspace.WithSystemWorkspace(appctx.Ctx())
	safego.Go(bgCtx, "knowledge.vault_reindex."+col.ID, func() {
		defer release()
		s.publishKnowledgeVaultReindex(col.ID, "reindexing", "")
		if rerr := s.vaultSync.ReindexVault(bgCtx, col); rerr != nil {
			s.publishKnowledgeVaultReindex(col.ID, "error", rerr.Error())
			flow.LogError("knowledge.vault_reindex", "知识库全量重索引失败",
				event.P("collection_id", col.ID),
				event.P("error", rerr.Error()))
			return
		}
		s.publishKnowledgeVaultReindex(col.ID, "done", "")
		flow.LogDone("knowledge.vault_reindex", "知识库全量重索引完成",
			event.P("collection_id", col.ID))
	})
	return &v1.ReindexVaultResponse{Status: "reindexing"}, nil
}

// publishKnowledgeVaultReindex 广播全量重索引进度/终态事件（EP-KN-02 模式：
// v2 SystemNoticeEvent，WS-only 不持久化；eventBus 为 nil 时跳过）。
func (s *KnowledgeService) publishKnowledgeVaultReindex(collectionID, status, errMsg string) {
	if s.eventBus == nil {
		return
	}
	meta := map[string]any{
		"event_type":    "knowledge_vault_reindex",
		"collection_id": collectionID,
		"status":        status,
		"error_message": errMsg,
	}
	msg := "Knowledge vault reindex: " + status
	s.eventBus.Publish(context.Background(), biz.NewSystemNoticeEvent("", "knowledge_reindex", msg, meta))
}
