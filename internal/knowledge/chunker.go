// Package knowledge provides text chunking strategies for document ingestion.
package knowledge

import "strings"

// ChunkStrategy controls how text is split into chunks.
type ChunkStrategy string

const (
	ChunkByChar  ChunkStrategy = "char"
	ChunkByToken ChunkStrategy = "token"
)

// Chunk is one piece of text with its index.
type Chunk struct {
	Content    string
	ChunkIndex int
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
	case ChunkByToken:
		return c.splitByToken(text)
	default:
		return c.splitByChar(text)
	}
}

func (c *Chunker) splitByChar(text string) []Chunk {
	runes := []rune(text)
	n := len(runes)
	if n <= c.ChunkSize {
		return []Chunk{{Content: text, ChunkIndex: 0}}
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
		content := strings.TrimSpace(string(runes[idx:end]))
		if content != "" {
			out = append(out, Chunk{Content: content, ChunkIndex: chunkIdx})
			chunkIdx++
		}
		idx += step
	}
	return out
}

// splitByToken uses whitespace-tokenization as a proxy for true token counts.
func (c *Chunker) splitByToken(text string) []Chunk {
	words := strings.Fields(text)
	n := len(words)
	if n <= c.ChunkSize {
		return []Chunk{{Content: text, ChunkIndex: 0}}
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
		content := strings.Join(words[i:end], " ")
		out = append(out, Chunk{Content: content, ChunkIndex: chunkIdx})
		chunkIdx++
		if end == n {
			break
		}
	}
	return out
}
