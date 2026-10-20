// Package knowledge provides text chunking strategies for document ingestion.
package knowledge

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// ChunkStrategy controls how text is split into chunks.
type ChunkStrategy string

const (
	ChunkByChar  ChunkStrategy = "char"
	ChunkByToken ChunkStrategy = "token"
	// ChunkByParentChild 父子分块（G-06）：小块召回、父块语境回填。
	// 经 SplitWithStrategy/SplitParentChild 进入；Chunker（无父块尺寸）下退化为 token。
	ChunkByParentChild ChunkStrategy = "parent_child"
)

// Chunk is one piece of text with its index.
// ParentIndex 为所属父块序号，-1 = 无父块（flat 策略经 flatChunk 构造恒 -1）。
// IsParent 标记父子分块中的父块（语境行）：不参与 embedding/召回，仅作语境回填来源。
// ByteStart 为块正文在源文本（与分块入口同款 TrimSpace 基准）中的起始字节偏移
// （G-07 页码/标题锚定用；-1 = 未知——trpc 策略产出无法回定位时锚定跳过）。
type Chunk struct {
	Content     string
	ChunkIndex  int
	IsParent    bool
	ParentIndex int
	ByteStart   int
}

// flatChunk 构造无父块的普通块（单一构造点，保证 ParentIndex=-1 约定不被零值破坏）。
func flatChunk(content string, idx, byteStart int) Chunk {
	return Chunk{Content: content, ChunkIndex: idx, ParentIndex: -1, ByteStart: byteStart}
}

// Chunker splits documents into overlapping text windows.
type Chunker struct {
	ChunkSize    int
	ChunkOverlap int
	Strategy     ChunkStrategy
}

// NewChunker returns a Chunker with sensible defaults.
func NewChunker(size, overlap int, strategy ChunkStrategy) *Chunker {
	if size <= 0 {
		size = 512
	}
	if overlap < 0 {
		overlap = 0
	}
	if overlap >= size {
		overlap = size / 4
	}
	if strategy == "" {
		strategy = ChunkByChar
	}
	return &Chunker{ChunkSize: size, ChunkOverlap: overlap, Strategy: strategy}
}

// Split splits text into overlapping chunks.
func (c *Chunker) Split(text string) []Chunk {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	switch c.Strategy {
	case ChunkByToken, ChunkByParentChild:
		// parent_child 经裸 Chunker（无父块尺寸上下文）时退化为 token 窗口——
		// 真父子双层由 SplitParentChild 专项入口产出（对抗评审 F7）。
		return c.splitByToken(text)
	default:
		return c.splitByChar(text)
	}
}

func (c *Chunker) splitByChar(text string) []Chunk {
	runes := []rune(text)
	n := len(runes)
	// rune→byte 一次线性换算（G-07：ByteStart 与锚定索引同属字节域）。
	byteOffs := make([]int, n+1)
	bi := 0
	for ri, r := range runes {
		byteOffs[ri] = bi
		bi += utf8.RuneLen(r)
	}
	byteOffs[n] = len(text)
	if n <= c.ChunkSize {
		return []Chunk{flatChunk(text, 0, 0)}
	}
	var out []Chunk
	idx := 0
	step := c.ChunkSize - c.ChunkOverlap
	if step <= 0 {
		step = c.ChunkSize
	}
	chunkIdx := 0
	for idx < n {
		end := idx + c.ChunkSize
		if end > n {
			end = n
		}
		raw := string(runes[idx:end])
		content := strings.TrimSpace(raw)
		if content != "" {
			byteStart := byteOffs[idx] + len(raw) - len(strings.TrimLeftFunc(raw, unicode.IsSpace))
			out = append(out, flatChunk(content, chunkIdx, byteStart))
			chunkIdx++
		}
		idx += step
	}
	return out
}

// splitByToken 用 cl100k BPE 真 token 计数切窗（G-06：原空格分词实现对中文等
// 长无空格文本会塌缩成单块）。窗口边界按 token 前缀和映射回原文切片，不重 join、
// 保留原始空白。编码器不可用时降级 rune 窗口。
func (c *Chunker) splitByToken(text string) []Chunk {
	toks := encodeTokens(text)
	offs := tokenOffsets(text, toks)
	if offs == nil {
		return c.splitByChar(text)
	}
	n := len(toks)
	if n <= c.ChunkSize {
		return []Chunk{flatChunk(text, 0, 0)}
	}
	var out []Chunk
	step := c.ChunkSize - c.ChunkOverlap
	if step <= 0 {
		step = c.ChunkSize
	}
	chunkIdx := 0
	for i := 0; i < n; i += step {
		end := i + c.ChunkSize
		if end > n {
			end = n
		}
		raw := text[offs[i]:offs[end]]
		content := strings.TrimSpace(raw)
		if content != "" {
			byteStart := offs[i] + len(raw) - len(strings.TrimLeftFunc(raw, unicode.IsSpace))
			out = append(out, flatChunk(content, chunkIdx, byteStart))
			chunkIdx++
		}
		if end == n {
			break
		}
	}
	return out
}
