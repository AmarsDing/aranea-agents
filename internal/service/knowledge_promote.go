package service

import (
	"context"
	"strings"

	v1 "aranea-agents/api/kratos/knowledge/v1"
	"aranea-agents/internal/biz"
	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/internal/event"
	"aranea-agents/internal/knowledge"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// ── SP1-G：晋升 RPC（设计 S7 / US-27） ───────────────────────────────────────

// PromoteBlocks 把个人库块复制晋升到团队库：目标库 mutate + 源库逐库 mutate
// （晋升回写源块 promoted_to，属写操作）→ biz 克隆/谱系/cascade → 目标文档
// chunk/FTS 同步重放（B-3：晋升完成即可检索；单文档失败降级 status=error，
// 不回滚晋升——最终一致，重建索引自愈）。审计走 knowledgeFlow（K1/K2）。
func (s *KnowledgeService) PromoteBlocks(ctx context.Context, req *v1.PromoteBlocksRequest) (*v1.PromoteBlocksResponse, error) {
	blockIDs := req.GetBlockIds()
	docIDs := req.GetDocIds()
	// SP1-I：doc_ids 文档级入口（与 block_ids 互斥）——解析整文档全部块走同一晋升管线。
	if len(blockIDs) > 0 && len(docIDs) > 0 {
		return nil, apierror.BadRequest("KNOWLEDGE", "block_ids and doc_ids are mutually exclusive")
	}
	if len(blockIDs) == 0 && len(docIDs) == 0 {
		return nil, apierror.BadRequest("KNOWLEDGE", "block_ids or doc_ids is required")
	}
	target, err := s.uc.GetCollection(ctx, req.GetTargetCollectionId())
	if err != nil {
		return nil, err
	}
	if err := s.assertCollectionMutateAccess(ctx, target); err != nil {
		return nil, err
	}

	flow := s.knowledgeFlow(ctx)
	var res bizknowledge.PromoteResult
	if len(docIDs) > 0 {
		if err := s.assertPromoteDocSourceAccess(ctx, docIDs); err != nil {
			return nil, err
		}
		flow.LogStart("knowledge.block.promote", "知识块晋升",
			event.P("target_collection_id", target.ID),
			event.P("doc_count", len(docIDs)))
		res, err = s.uc.PromoteDocuments(ctx, docIDs, target.ID)
	} else {
		if err := s.assertPromoteSourceAccess(ctx, blockIDs); err != nil {
			return nil, err
		}
		flow.LogStart("knowledge.block.promote", "知识块晋升",
			event.P("target_collection_id", target.ID),
			event.P("block_count", len(blockIDs)))
		res, err = s.uc.PromoteBlocks(ctx, blockIDs, target.ID)
	}
	if err != nil {
		flow.LogError("knowledge.block.promote", "知识块晋升失败",
			event.P("target_collection_id", target.ID),
			event.P("error", err.Error()))
		return nil, err
	}

	replayed, replayFailed := s.replayPromotedDocChunks(ctx, target, res.TouchedTargetDocs)
	// P1-a 图谱收口（2026-08-16）：晋升路径不经写回管线，目标库（team）无 vault
	// 同步循环，实体共现/typed 关系抽取必须在此显式触发，否则晋升文档在图谱中
	// 恒为孤立节点。必须在 chunk 重放之后（与写回同序）；失败仅 Warn——钩子
	// content_hash 幂等，下次写回/热文档扫描自然重试。
	s.triggerKnowledgeGraph(ctx, target, res.TouchedTargetDocs)
	flow.LogDone("knowledge.block.promote", "知识块晋升完成",
		event.P("target_collection_id", target.ID),
		event.P("created_blocks", len(res.CreatedBlocks)),
		event.P("cascade_candidates", len(res.CascadeCandidates)),
		event.P("replayed_docs", replayed),
		event.P("replay_failed_docs", replayFailed))

	out := &v1.PromoteBlocksResponse{
		CreatedBlocks:     make([]*v1.PromotedBlockLineage, 0, len(res.CreatedBlocks)),
		CascadeCandidates: make([]*v1.PromoteCascadeCandidate, 0, len(res.CascadeCandidates)),
	}
	for _, p := range res.CreatedBlocks {
		out.CreatedBlocks = append(out.CreatedBlocks, &v1.PromotedBlockLineage{
			SrcBlockId:  p.SrcBlockID,
			NewBlockId:  p.NewBlockID,
			TargetDocId: p.TargetDocID,
		})
	}
	for _, c := range res.CascadeCandidates {
		out.CascadeCandidates = append(out.CascadeCandidates, &v1.PromoteCascadeCandidate{
			SrcBlockId:      c.SrcBlockID,
			RawTarget:       c.RawTarget,
			DstDocId:        c.DstDocID,
			DstCollectionId: c.DstCollectionID,
		})
	}
	return out, nil
}

// assertPromoteSourceAccess 源侧写权限断言：逐块解析归属（块 → 文档 → 库），
// 按库去重后逐库 mutate 断言（C-01 跨租户 NotFound 防泄漏）。
func (s *KnowledgeService) assertPromoteSourceAccess(ctx context.Context, blockIDs []string) error {
	seenDocs := map[string]bool{}
	seenCols := map[string]bool{}
	for _, id := range blockIDs {
		docID, err := s.uc.ResolveBlockOwnerDoc(ctx, id)
		if err != nil {
			return err
		}
		if seenDocs[docID] {
			continue
		}
		seenDocs[docID] = true
		doc, err := s.uc.GetDocument(ctx, docID)
		if err != nil {
			return err
		}
		if seenCols[doc.CollectionID] {
			continue
		}
		seenCols[doc.CollectionID] = true
		col, err := s.uc.GetCollection(ctx, doc.CollectionID)
		if err != nil {
			return err
		}
		if err := s.assertCollectionMutateAccess(ctx, col); err != nil {
			return err
		}
	}
	return nil
}

// assertPromoteDocSourceAccess 文档级晋升源侧写权限断言：逐文档解析归属库
// （未知文档 NotFound 透传），按库去重后逐库 mutate 断言（C-01 跨租户
// NotFound 防泄漏，同 assertPromoteSourceAccess 口径）。
func (s *KnowledgeService) assertPromoteDocSourceAccess(ctx context.Context, docIDs []string) error {
	seenCols := map[string]bool{}
	for _, id := range docIDs {
		doc, err := s.uc.GetDocument(ctx, id)
		if err != nil {
			return err
		}
		if seenCols[doc.CollectionID] {
			continue
		}
		seenCols[doc.CollectionID] = true
		col, err := s.uc.GetCollection(ctx, doc.CollectionID)
		if err != nil {
			return err
		}
		if err := s.assertCollectionMutateAccess(ctx, col); err != nil {
			return err
		}
	}
	return nil
}

// replayPromotedDocChunks 目标文档 chunk/FTS 重放（与 vault 同步链同哲学：
// 删旧插新 + status/counts 收尾；目标库无 embedding_model 时纯词法索引）。
func (s *KnowledgeService) replayPromotedDocChunks(ctx context.Context, target biz.KnowledgeCollection, touched []bizknowledge.PromoteTouchedDoc) (replayed, failed int) {
	embedder := s.embedder
	if strings.TrimSpace(target.EmbeddingModel) == "" {
		embedder = nil
	}
	for _, td := range touched {
		doc, err := s.uc.GetDocument(ctx, td.DocID)
		if err != nil {
			failed++
			s.lg.Warn("晋升目标文档读取失败，跳过 chunk 重放",
				loggateway.StepID("knowledge.block.promote_replay"),
				loggateway.Str("doc_id", td.DocID),
				loggateway.Err(err))
			continue
		}
		if err := s.uc.UpdateDocumentStatus(ctx, doc.ID, "indexing", "", 0); err != nil {
			failed++
			s.lg.Warn("晋升目标文档无法进入 indexing",
				loggateway.StepID("knowledge.block.promote_replay"),
				loggateway.Str("doc_id", doc.ID),
				loggateway.Err(err))
			continue
		}
		params := knowledge.IngestParams{
			DocID:        doc.ID,
			CollectionID: target.ID,
			Text:         doc.ContentText,
			Strategy:     knowledge.ChunkByMarkdown,
		}
		params.ApplyDefaults()
		chunks, err := knowledge.BuildIndexedChunks(ctx, embedder, params, nil)
		if err == nil {
			err = s.uc.DeleteChunksByDocument(ctx, doc.ID)
		}
		if err == nil {
			err = s.uc.InsertChunks(ctx, chunks)
		}
		if err == nil {
			err = s.uc.UpdateDocumentStatus(ctx, doc.ID, "indexed", "", len(chunks))
		}
		if err != nil {
			failed++
			if statusErr := s.uc.UpdateDocumentStatus(ctx, doc.ID, "error", err.Error(), 0); statusErr != nil {
				s.lg.Warn("晋升重放失败且状态回写失败",
					loggateway.StepID("knowledge.block.promote_replay"),
					loggateway.Str("doc_id", doc.ID),
					loggateway.Err(statusErr))
			}
			s.lg.Warn("晋升目标文档 chunk 重放失败（晋升已完成，重建索引可自愈）",
				loggateway.StepID("knowledge.block.promote_replay"),
				loggateway.Str("doc_id", doc.ID),
				loggateway.Err(err))
			continue
		}
		// 计数：新建/未计入文档 +1；chunk 按差值补齐（同 vault 同步链口径）。
		docDelta := 0
		if td.Created || doc.Status != "indexed" {
			docDelta = 1
		}
		if chunkDelta := len(chunks) - doc.ChunkCount; docDelta != 0 || chunkDelta != 0 {
			if err := s.uc.UpdateCollectionCounts(ctx, target.ID, docDelta, chunkDelta); err != nil {
				s.lg.Warn("晋升目标库计数更新失败",
					loggateway.StepID("knowledge.block.promote_replay"),
					loggateway.Str("collection_id", target.ID),
					loggateway.Err(err))
			}
		}
		replayed++
	}
	return replayed, failed
}
