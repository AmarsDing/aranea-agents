package knowledge

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
)

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
func BuildIndexedChunks(ctx context.Context, embedder Embedder, p IngestParams) ([]biz.KnowledgeChunk, error) {
	if embedder == nil {
		return nil, apierror.BadRequest(apierror.DomainKnowledge, "ingest: embedder is nil")
	}
	meta, err := NormalizeMetadataJSON(p.MetadataJSON)
	if err != nil {
		return nil, err
	}
	strategy := ParseChunkStrategy(string(p.Strategy))
	chunks, err := SplitWithStrategy(strategy, p.Text, p.ChunkSize, p.ChunkOverlap)
	if err != nil {
		return nil, err
	}
	if len(chunks) == 0 {
		return nil, apierror.Internal(apierror.DomainKnowledge, "ingest: no chunks produced")
	}

	texts := make([]string, len(chunks))
	for i, ch := range chunks {
		texts[i] = ch.Content
	}
	vecs, err := embedder.Embed(ctx, texts)
	if err != nil {
		return nil, err
	}
	if len(vecs) != len(chunks) {
		return nil, apierror.Internal(apierror.DomainKnowledge,
			fmt.Sprintf("ingest: embedding count mismatch: expected %d, got %d", len(chunks), len(vecs)))
	}

	out := make([]biz.KnowledgeChunk, 0, len(chunks))
	for i, ch := range chunks {
		out = append(out, biz.KnowledgeChunk{
			ID:           fmt.Sprintf("%s-ch-%d", p.DocID, ch.ChunkIndex),
			DocID:        p.DocID,
			CollectionID: p.CollectionID,
			Content:      ch.Content,
			Embedding:    vecs[i],
			MetadataJSON: meta,
			ChunkIndex:   ch.ChunkIndex,
		})
	}
	return out, nil
}
