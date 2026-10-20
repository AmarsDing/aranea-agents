package knowledge

import (
	"strings"
	"unicode"
)

// SplitParentChild 父子分块（G-06）：父块是固定 token 窗口（父块间不重叠；parentSize<=0
// 时默认 4×size，规范化下限 2×size），子块是父块内的 token 窗口（size/overlap 语义与
// splitByToken 一致，窗口按父块边界钳制、不跨父块）。文本不足一个子块窗口时退化为
// 单 flat 子块（无父块行）。编码器不可用时降级为普通 token 分块（内部再降级 rune 窗口）。
//
// 输出顺序为 [父0, 父0子块..., 父1, 父1子块..., ...]：父块 IsParent=true、
// ChunkIndex=p 序号、ParentIndex=-1；子块 ChunkIndex 为全文连续递增（保证
// `<docID>-ch-<idx>` 命名空间稠密）、ParentIndex 指回所属父块序号。
func SplitParentChild(text string, size, overlap, parentSize int) []Chunk {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if size <= 0 {
		size = DefaultChunkSize
	}
	if overlap < 0 {
		overlap = DefaultChunkOverlap
	}
	if overlap >= size {
		overlap = size / 4
	}
	if parentSize <= 0 {
		parentSize = 4 * size
	}
	if parentSize < 2*size {
		parentSize = 2 * size
	}

	toks := encodeTokens(text)
	offs := tokenOffsets(text, toks)
	if offs == nil {
		return NewChunker(size, overlap, ChunkByToken).Split(text)
	}
	n := len(toks)
	if n <= size {
		return []Chunk{flatChunk(text, 0, 0)}
	}

	step := size - overlap
	if step <= 0 {
		step = size
	}
	var out []Chunk
	pIdx, cIdx := 0, 0
	for p0 := 0; p0 < n; p0 += parentSize {
		p1 := p0 + parentSize
		if p1 > n {
			p1 = n
		}
		pRaw := text[offs[p0]:offs[p1]]
		parentContent := strings.TrimSpace(pRaw)
		if strings.TrimSpace(parentContent) == "" {
			continue
		}
		out = append(out, Chunk{
			Content: parentContent, ChunkIndex: pIdx, IsParent: true, ParentIndex: -1,
			ByteStart: offs[p0] + len(pRaw) - len(strings.TrimLeftFunc(pRaw, unicode.IsSpace)),
		})
		for i := p0; i < p1; i += step {
			end := i + size
			if end > p1 {
				end = p1
			}
			raw := text[offs[i]:offs[end]]
			content := strings.TrimSpace(raw)
			if content != "" {
				out = append(out, Chunk{
					Content: content, ChunkIndex: cIdx, ParentIndex: pIdx,
					ByteStart: offs[i] + len(raw) - len(strings.TrimLeftFunc(raw, unicode.IsSpace)),
				})
				cIdx++
			}
			if end == p1 {
				break
			}
		}
		pIdx++
	}
	return out
}
