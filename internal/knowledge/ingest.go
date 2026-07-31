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
	chunks, err := SplitWithStrategy(strategy, p.Text, p.ChunkSize, p.ChunkOverlap)
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
	if flow != nil {
		flow.LogDone("knowledge.ingest.parse", "文档解析分块完成",
			event.P("doc_id", p.DocID),
			event.P("chunk_count", len(chunks)))
	}

	var vecs [][]float32
	if embedder != nil {
		texts := make([]string, len(chunks))
		for i, ch := range chunks {
			texts[i] = ch.Content
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
					event.P("chunk_count", len(chunks)))
			}
		} else if len(vecs) != len(chunks) {
			return nil, apierror.Internal(apierror.DomainKnowledge,
				fmt.Sprintf("ingest: embedding count mismatch: expected %d, got %d", len(chunks), len(vecs)))
		}
	}

	out := make([]biz.KnowledgeChunk, 0, len(chunks))
	for i, ch := range chunks {
		row := biz.KnowledgeChunk{
			ID:           fmt.Sprintf("%s-ch-%d", p.DocID, ch.ChunkIndex),
			DocID:        p.DocID,
			CollectionID: p.CollectionID,
			Content:      ch.Content,
			MetadataJSON: meta,
			ChunkIndex:   ch.ChunkIndex,
		}
		if vecs != nil {
			row.Embedding = vecs[i]
		}
		out = append(out, row)
	}
	return out, nil
}
