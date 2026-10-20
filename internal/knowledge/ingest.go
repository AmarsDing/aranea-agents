package knowledge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// newKnowledgeFlow 创建知识域流程日志发射器（Domain=knowledge，无会话上下文）。
// bus 为 nil 时仅跳过总线发布；lg 为 nil 时仅跳过进程日志。两者可同时为 nil。
func newKnowledgeFlow(ctx context.Context, bus contract.MonitorBus, lg loggateway.Logger) *event.TraceEmitter {
	return event.NewTraceEmitterForRun(event.TraceEmitterOpts{
		Ctx:    ctx,
		Domain: event.TraceDomainKnowledge,
		LG:     lg,
		Infra:  event.NewInfraFromBus(bus),
	})
}

const defaultEmbedBatchSize = 32

const (
	DefaultChunkSize    = 512
	DefaultChunkOverlap = 64
)

// IngestParams holds inputs for the chunk-and-embed pipeline.
type IngestParams struct {
	DocID        string
	CollectionID string
	Text         string
	MetadataJSON string
	Strategy     ChunkStrategy
	ChunkSize    int
	ChunkOverlap int
	// ParentChunkSize 父子分块（parent_child）的父块 token 窗口：0 = 默认 4×ChunkSize，
	// 规范化下限 2×ChunkSize。其他策略忽略。
	ParentChunkSize int
}

func (p *IngestParams) ApplyDefaults() {
	if p.ChunkSize <= 0 {
		p.ChunkSize = DefaultChunkSize
	}
	if p.ChunkOverlap < 0 {
		p.ChunkOverlap = DefaultChunkOverlap
	}
}

// NormalizeMetadataJSON validates document-level metadata for chunk storage.
func NormalizeMetadataJSON(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "{}", nil
	}
	if !json.Valid([]byte(s)) {
		return "", apierror.BadRequest(apierror.DomainKnowledge, "metadata_json must be valid JSON")
	}
	return s, nil
}

// BuildIndexedChunks splits text, embeds each chunk, and returns rows ready for persistence.
// flow 为可选流程日志发射器（nil 时跳过）：解析分块完成/失败发射 knowledge.ingest.parse。
// embedder 为 nil（词法库，collection 无 embedding_model）或返回 ErrEmbedderNotConfigured
// 时降级为词法索引（Embedding 留空），与 vault 同步链 buildChunks 的 R-4 降级一致；
// 其他 embed 错误照常返回（不掩盖真实故障）。
func BuildIndexedChunks(ctx context.Context, embedder Embedder, p IngestParams, flow *event.TraceEmitter) ([]biz.KnowledgeChunk, error) {
	meta, err := NormalizeMetadataJSON(p.MetadataJSON)
	if err != nil {
		return nil, err
	}
	strategy := ParseChunkStrategy(string(p.Strategy))
	var chunks []Chunk
	switch strategy {
	case ChunkByParentChild:
		chunks = SplitParentChild(p.Text, p.ChunkSize, p.ChunkOverlap, p.ParentChunkSize)
	default:
		chunks, err = SplitWithStrategy(strategy, p.Text, p.ChunkSize, p.ChunkOverlap)
	}
	if err != nil {
		if flow != nil {
			flow.LogError("knowledge.ingest.parse", "文档解析分块失败",
				event.P("doc_id", p.DocID),
				event.P("error", err.Error()))
		}
		return nil, err
	}
	if len(chunks) == 0 {
		return nil, apierror.Internal(apierror.DomainKnowledge, "ingest: no chunks produced")
	}
	// 父块（context 行）只作语境回填来源，不进 embed 批、不计 chunk_count。
	childCount := 0
	for _, ch := range chunks {
		if !ch.IsParent {
			childCount++
		}
	}
	if flow != nil {
		flow.LogDone("knowledge.ingest.parse", "文档解析分块完成",
			event.P("doc_id", p.DocID),
			event.P("chunk_count", childCount),
			event.P("parent_count", len(chunks)-childCount))
	}

	var vecs [][]float32
	if embedder != nil {
		var embedIdx []int
		texts := make([]string, 0, len(chunks))
		for i, ch := range chunks {
			if ch.IsParent {
				continue
			}
			embedIdx = append(embedIdx, i)
			texts = append(texts, ch.Content)
		}
		vecs, err = embedder.Embed(ctx, texts)
		if err != nil {
			if !errors.Is(err, ErrEmbedderNotConfigured) {
				return nil, err
			}
			// F5 降级：embedder 未配置 → 词法索引（空向量），检索走 BM25。
			vecs = nil
			if flow != nil {
				flow.LogError("knowledge.ingest.embed", "嵌入器未配置，降级为词法索引",
					event.P("doc_id", p.DocID),
					event.P("chunk_count", len(texts)))
			}
		} else if len(vecs) != len(texts) {
			return nil, apierror.Internal(apierror.DomainKnowledge,
				fmt.Sprintf("ingest: embedding count mismatch: expected %d, got %d", len(texts), len(vecs)))
		} else {
			// vecs[i] 对应 chunks[embedIdx[i]]（父块行不占位）。
			mapped := make([][]float32, len(chunks))
			for i, vec := range vecs {
				mapped[embedIdx[i]] = vec
			}
			vecs = mapped
		}
	}

	out := make([]biz.KnowledgeChunk, 0, len(chunks))
	// G-07 页码/标题锚定：锚索引与分块同基准（分块入口均先 TrimSpace），按块
	// ByteStart 并回 metadata_json（page_no / heading_path）——页锚原料来自
	// PDFToMarkdown 的 <!-- page:N -->，标题栈来自 blockparse；无锚文本保持
	// 文档级 meta 原样（字节不动）。
	anchors := newDocAnchors(strings.TrimSpace(p.Text))
	for i, ch := range chunks {
		row := biz.KnowledgeChunk{
			DocID:        p.DocID,
			CollectionID: p.CollectionID,
			Content:      ch.Content,
			MetadataJSON: meta,
			ChunkIndex:   ch.ChunkIndex,
		}
		if !anchors.empty() {
			row.MetadataJSON = mergeChunkAnchorMetadata(row.MetadataJSON, anchors.anchorAt(ch.ByteStart))
		}
		switch {
		case ch.IsParent:
			// 父块行 ID 命名空间独立于子块：retryDocEmbeddings 精确 ID 回填与
			// citation 锚点（-ch- 前缀）零碰撞。
			row.ID = fmt.Sprintf("%s-p-%d", p.DocID, ch.ChunkIndex)
			row.IsParent = true
		default:
			row.ID = fmt.Sprintf("%s-ch-%d", p.DocID, ch.ChunkIndex)
			if ch.ParentIndex >= 0 {
				row.ParentChunkID = fmt.Sprintf("%s-p-%d", p.DocID, ch.ParentIndex)
			}
		}
		if vecs != nil && i < len(vecs) {
			row.Embedding = vecs[i]
		}
		out = append(out, row)
	}
	return out, nil
}
