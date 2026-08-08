package service

import (
	"context"
	"fmt"

	"aranea-agents/internal/biz"
	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/pkg/loggateway"
)

// SP1-D D-2：knowledge.graph.delta WS 增量事件（设计 S5）。
// 事件分级（N-2）：按 AS-EVT-01 登记 Informational——WS-only 不持久化，
// 丢失可容忍（内存图可从 knowledge_block_refs 全量重放重建）。

// knowledgeGraphDeltaPublisher 把 biz GraphDelta 适配为 v2 SystemNoticeEvent
// 经 eventBus 广播（SystemNoticeEvent 模式同 knowledge_ingest）。
type knowledgeGraphDeltaPublisher struct {
	bus biz.EventBus
}

func newKnowledgeGraphDeltaPublisher(bus biz.EventBus) bizknowledge.GraphDeltaPublisher {
	if bus == nil {
		return nil
	}
	return &knowledgeGraphDeltaPublisher{bus: bus}
}

func (p *knowledgeGraphDeltaPublisher) PublishGraphDelta(ctx context.Context, delta bizknowledge.GraphDelta) {
	if p == nil || p.bus == nil || delta.Empty() {
		return
	}
	meta := map[string]any{
		"event_type": "knowledge.graph.delta",
		"version":    delta.Version,
		"added":      graphDeltaEdgePayloads(delta.Added),
		"removed":    graphDeltaEdgePayloads(delta.Removed),
	}
	msg := fmt.Sprintf("Knowledge graph delta: +%d/-%d (v%d)", len(delta.Added), len(delta.Removed), delta.Version)
	p.bus.Publish(ctx, biz.NewSystemNoticeEvent("", "knowledge.graph.delta", msg, meta))
}

func graphDeltaEdgePayloads(edges []bizknowledge.KnowledgeBlockRefEdge) []map[string]any {
	out := make([]map[string]any, 0, len(edges))
	for _, e := range edges {
		out = append(out, map[string]any{
			"collection_id":     e.CollectionID,
			"src_block_id":      e.SrcBlockID,
			"src_doc_id":        e.SrcDocID,
			"dst_collection_id": e.DstCollectionID,
			"dst_doc_id":        e.DstDocID,
			"dst_block_id":      e.DstBlockID,
			"raw_target":        e.RawTarget,
			"edge_type":         e.EdgeType,
			"context":           e.Context,
			"ambiguous":         e.Ambiguous,
		})
	}
	return out
}

// LoadKnowledgeLinkIndex 启动全量构建统一链接内存图（SP1-D 设计 S5，readiness
// 门控后由 app 层触发）。派生索引：失败仅降级（反链查询 DB 兜底，SP1-E），
// 不阻塞启动，错误已记进程日志。
func (s *KnowledgeService) LoadKnowledgeLinkIndex(ctx context.Context) error {
	if s == nil || s.uc == nil {
		return nil
	}
	s.lg.Info("knowledge link index full load start", loggateway.StepID("knowledge.link_index.load"))
	n, err := s.uc.LoadLinkIndex(ctx)
	if err != nil {
		s.lg.Error("knowledge link index full load failed", loggateway.StepID("knowledge.link_index.load"), loggateway.Err(err))
		return err
	}
	s.lg.Info("knowledge link index full load done", loggateway.StepID("knowledge.link_index.load"), loggateway.Int("edges", n))
	return nil
}
