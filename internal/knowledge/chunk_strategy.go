package knowledge

import (
	"strings"
	"unicode"

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
	case ChunkByChar, ChunkByToken, ChunkByMarkdown, ChunkByJSON, ChunkByRecursive, ChunkByParentChild:
		return s
	default:
		return ChunkByChar
	}
}

// SplitWithStrategy splits text using the requested strategy (trpc chunking for advanced modes).
// 非父子策略统一归一 ParentIndex=-1（单一归一点），下游可无条件依赖该约定。
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
	if strategy == ChunkByParentChild {
		return SplitParentChild(text, size, overlap, 0), nil
	}

	var chunks []Chunk
	var err error
	switch strategy {
	case ChunkByToken:
		chunks = NewChunker(size, overlap, ChunkByToken).Split(text)
	case ChunkByChar:
		chunks = NewChunker(size, overlap, ChunkByChar).Split(text)
	default:
		chunks, err = splitWithTrpcStrategy(strategy, text, size, overlap)
	}
	if err != nil {
		return nil, err
	}
	for i := range chunks {
		if !chunks[i].IsParent {
			chunks[i].ParentIndex = -1
		}
	}
	return chunks, nil
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
	return trpcDocsToChunks(docs, text), nil
}

// trpcDocsToChunks 转换 trpc 分块产物；ByteStart 按内容在源文本中顺序回定位
// （trpc 策略产出多数为原文连续切片；拼接式产出定位失败时置 -1，锚定跳过）。
func trpcDocsToChunks(docs []*trpcdoc.Document, text string) []Chunk {
	out := make([]Chunk, 0, len(docs))
	cursor := 0
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
		byteStart := -1
		probe := strings.TrimSpace(d.Content)
		if loc := strings.Index(text[cursor:], d.Content); loc >= 0 {
			byteStart = cursor + loc + len(d.Content) - len(strings.TrimLeftFunc(d.Content, unicode.IsSpace))
			cursor += loc + len(d.Content)
		} else if loc := strings.Index(text, probe); loc >= 0 {
			// 回退：全文定位 TrimSpace 后内容（cursor 失效——产出顺序与文本序错位时
			// 锚定按首个命中取，宁近似不丢）。
			byteStart = loc
		}
		out = append(out, Chunk{Content: d.Content, ChunkIndex: chunkIndex, ParentIndex: -1, ByteStart: byteStart})
	}
	return out
}
