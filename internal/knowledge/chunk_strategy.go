package knowledge

import (
	"strings"

	"aranea-agents/pkg/apierror"
	trpcchunk "trpc.group/trpc-go/trpc-agent-go/knowledge/chunking"
	trpcdoc "trpc.group/trpc-go/trpc-agent-go/knowledge/document"
	trpcsource "trpc.group/trpc-go/trpc-agent-go/knowledge/source"
)

const (
	ChunkByMarkdown  ChunkStrategy = "markdown"
	ChunkByJSON      ChunkStrategy = "json"
	ChunkByRecursive ChunkStrategy = "recursive"
)

// ParseChunkStrategy normalizes a strategy name; empty defaults to char.
func ParseChunkStrategy(raw string) ChunkStrategy {
	s := ChunkStrategy(strings.ToLower(strings.TrimSpace(raw)))
	switch s {
	case ChunkByChar, ChunkByToken, ChunkByMarkdown, ChunkByJSON, ChunkByRecursive:
		return s
	default:
		return ChunkByChar
	}
}

// SplitWithStrategy splits text using the requested strategy (trpc chunking for advanced modes).
func SplitWithStrategy(strategy ChunkStrategy, text string, size, overlap int) ([]Chunk, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, nil
	}
	if size <= 0 {
		size = DefaultChunkSize
	}
	if overlap < 0 {
		overlap = DefaultChunkOverlap
	}

	switch strategy {
	case ChunkByToken:
		return NewChunker(size, overlap, ChunkByToken).Split(text), nil
	case ChunkByChar:
		return NewChunker(size, overlap, ChunkByChar).Split(text), nil
	default:
		return splitWithTrpcStrategy(strategy, text, size, overlap)
	}
}

func splitWithTrpcStrategy(strategy ChunkStrategy, text string, size, overlap int) ([]Chunk, error) {
	doc := &trpcdoc.Document{Content: text}
	var strat trpcchunk.Strategy
	switch strategy {
	case ChunkByMarkdown:
		strat = trpcchunk.NewMarkdownChunking(
			trpcchunk.WithMarkdownChunkSize(size),
			trpcchunk.WithMarkdownOverlap(overlap),
		)
	case ChunkByJSON:
		strat = trpcchunk.NewJSONChunking(trpcchunk.WithJSONChunkSize(size))
	case ChunkByRecursive:
		strat = trpcchunk.NewRecursiveChunking(
			trpcchunk.WithRecursiveChunkSize(size),
			trpcchunk.WithRecursiveOverlap(overlap),
		)
	default:
		return nil, apierror.BadRequest(apierror.DomainKnowledge, "unsupported chunk strategy %q", strategy)
	}
	docs, err := strat.Chunk(doc)
	if err != nil {
		return nil, err
	}
	return trpcDocsToChunks(docs), nil
}

func trpcDocsToChunks(docs []*trpcdoc.Document) []Chunk {
	out := make([]Chunk, 0, len(docs))
	for i, d := range docs {
		if d == nil || strings.TrimSpace(d.Content) == "" {
			continue
		}
		chunkIndex := i
		if d.Metadata != nil {
			if v, ok := d.Metadata[trpcsource.MetaChunkIndex]; ok {
				if idx, ok := v.(int); ok && idx >= 0 {
					chunkIndex = idx
				} else if f, ok := v.(float64); ok && f >= 0 {
					chunkIndex = int(f)
				}
			}
		}
		out = append(out, Chunk{Content: d.Content, ChunkIndex: chunkIndex})
	}
	return out
}
