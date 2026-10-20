package knowledge

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"aranea-agents/pkg/apierror"
)

func TestNormalizeMetadataJSON(t *testing.T) {
	t.Parallel()
	got, err := NormalizeMetadataJSON("")
	if err != nil || got != "{}" {
		t.Fatalf("empty: got %q err %v", got, err)
	}
	got, err = NormalizeMetadataJSON(`{"category":"policy"}`)
	if err != nil || got != `{"category":"policy"}` {
		t.Fatalf("object: got %q err %v", got, err)
	}
	_, err = NormalizeMetadataJSON("{bad")
	if err == nil {
		t.Fatal("expected invalid JSON error")
	}
}

func TestBuildIndexedChunks_appliesMetadata(t *testing.T) {
	t.Parallel()
	emb := &stubIngestEmbedder{dim: 4}
	chunks, err := BuildIndexedChunks(t.Context(), emb, IngestParams{
		DocID:        "doc1",
		CollectionID: "col1",
		Text:         "hello world",
		MetadataJSON: `{"source":"test"}`,
		Strategy:     ChunkByChar,
		ChunkSize:    512,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) == 0 {
		t.Fatal("expected chunks")
	}
	for _, ch := range chunks {
		if ch.MetadataJSON != `{"source":"test"}` {
			t.Fatalf("metadata not applied: %q", ch.MetadataJSON)
		}
	}
}

func TestSplitWithStrategy_markdown(t *testing.T) {
	t.Parallel()
	text := "# Title\n\n" + strings.Repeat("Paragraph one sentence. ", 20) + "\n\n## Section\n\n" + strings.Repeat("Paragraph two sentence. ", 20)
	chunks, err := SplitWithStrategy(ChunkByMarkdown, text, 200, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) < 2 {
		t.Fatalf("expected multiple markdown chunks, got %d", len(chunks))
	}
}

func TestSplitWithStrategy_json_invalid(t *testing.T) {
	t.Parallel()
	_, err := SplitWithStrategy(ChunkByJSON, "{bad", 512, 64)
	if err == nil {
		t.Fatal("expected JSON parse error")
	}
}

func TestSplitWithStrategy_recursive(t *testing.T) {
	t.Parallel()
	text := "line one\n\nline two\n\nline three with more words"
	chunks, err := SplitWithStrategy(ChunkByRecursive, text, 30, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) == 0 {
		t.Fatal("expected chunks")
	}
}

// 词法库（embedder nil）：仅分块不嵌入，Embedding 留空，与 vault buildChunks 的 R-4 降级一致。
func TestBuildIndexedChunks_lexicalWhenEmbedderNil(t *testing.T) {
	t.Parallel()
	chunks, err := BuildIndexedChunks(t.Context(), nil, IngestParams{
		DocID:        "doc1",
		CollectionID: "col1",
		Text:         "hello world",
		Strategy:     ChunkByChar,
		ChunkSize:    512,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) == 0 {
		t.Fatal("expected chunks")
	}
	for _, ch := range chunks {
		if len(ch.Embedding) != 0 {
			t.Fatalf("lexical chunk must have empty embedding, got %d dims", len(ch.Embedding))
		}
	}
}

// embedder 未配置（ErrEmbedderNotConfigured）：降级词法索引而非整篇 error（F5 同哲学）。
func TestBuildIndexedChunks_degradeWhenEmbedderNotConfigured(t *testing.T) {
	t.Parallel()
	chunks, err := BuildIndexedChunks(t.Context(), failIngestEmbedder{err: ErrEmbedderNotConfigured}, IngestParams{
		DocID:        "doc1",
		CollectionID: "col1",
		Text:         "hello world",
		Strategy:     ChunkByChar,
		ChunkSize:    512,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) == 0 {
		t.Fatal("expected chunks")
	}
	for _, ch := range chunks {
		if len(ch.Embedding) != 0 {
			t.Fatalf("degraded chunk must have empty embedding, got %d dims", len(ch.Embedding))
		}
	}
}

// embedder 其他错误（网络/服务异常）仍整篇 error——不掩盖真实故障。
func TestBuildIndexedChunks_errorWhenEmbedderFails(t *testing.T) {
	t.Parallel()
	_, err := BuildIndexedChunks(t.Context(), failIngestEmbedder{err: apierror.Internal(apierror.DomainKnowledge, "boom")}, IngestParams{
		DocID:        "doc1",
		CollectionID: "col1",
		Text:         "hello world",
		Strategy:     ChunkByChar,
		ChunkSize:    512,
	}, nil)
	if err == nil {
		t.Fatal("expected embed failure to propagate")
	}
}

// 父子分块摄取（G-06）：父块行进 -p- 命名空间且不进 embed 批；子块带 ParentChunkID 照常嵌入。
func TestBuildIndexedChunks_parentChild(t *testing.T) {
	t.Parallel()
	emb := &stubIngestEmbedder{dim: 4}
	text := strings.Repeat("核心机房夜间值守必须双人同时在场，禁止单人进入配电区。", 20)
	chunks, err := BuildIndexedChunks(t.Context(), emb, IngestParams{
		DocID:        "doc1",
		CollectionID: "col1",
		Text:         text,
		Strategy:     ChunkByParentChild,
		ChunkSize:    16,
		ChunkOverlap: 4,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	parentIDs := map[string]struct{}{}
	childCount := 0
	for _, ch := range chunks {
		if ch.IsParent {
			if len(ch.Embedding) != 0 {
				t.Fatalf("parent row must not be embedded: %s", ch.ID)
			}
			if ch.ID != "doc1-p-"+strconv.Itoa(ch.ChunkIndex) {
				t.Fatalf("parent ID must be <docID>-p-<pidx>, got %s", ch.ID)
			}
			if ch.ParentChunkID != "" {
				t.Fatalf("parent row must not itself reference a parent: %s", ch.ID)
			}
			parentIDs[ch.ID] = struct{}{}
			continue
		}
		childCount++
		if len(ch.Embedding) != 4 {
			t.Fatalf("child row must be embedded: %s", ch.ID)
		}
		if !strings.HasPrefix(ch.ID, "doc1-ch-") {
			t.Fatalf("child ID must be <docID>-ch-<idx>, got %s", ch.ID)
		}
		if _, ok := parentIDs[ch.ParentChunkID]; !ok {
			t.Fatalf("child %s references unknown parent %q", ch.ID, ch.ParentChunkID)
		}
	}
	if len(parentIDs) == 0 || childCount == 0 {
		t.Fatalf("expected parents and children, got %d parents %d children", len(parentIDs), childCount)
	}
}

// 父子分块 + 词法库（embedder nil）：父块/子块均无向量、行结构不变。
func TestBuildIndexedChunks_parentChildLexical(t *testing.T) {
	t.Parallel()
	text := strings.Repeat("词法库父子分块一样要保留父块语境行。", 30)
	chunks, err := BuildIndexedChunks(t.Context(), nil, IngestParams{
		DocID: "doc1", CollectionID: "col1", Text: text,
		Strategy: ChunkByParentChild, ChunkSize: 8, ChunkOverlap: 2,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var sawParent, sawChildWithParent bool
	for _, ch := range chunks {
		if len(ch.Embedding) != 0 {
			t.Fatalf("lexical ingest must keep embeddings empty: %s", ch.ID)
		}
		if ch.IsParent {
			sawParent = true
		} else if ch.ParentChunkID != "" {
			sawChildWithParent = true
		}
	}
	if !sawParent || !sawChildWithParent {
		t.Fatalf("expected parent rows and linked children, got %+v", chunks)
	}
}

type stubIngestEmbedder struct {
	dim int
}

func (s stubIngestEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = make([]float32, s.dim)
	}
	return out, nil
}

func (s stubIngestEmbedder) Dim() int {
	return s.dim
}

// failIngestEmbedder 恒返回指定错误（模拟 embedder 未配置/故障）。
type failIngestEmbedder struct {
	err error
}

func (s failIngestEmbedder) Embed(_ context.Context, _ []string) ([][]float32, error) {
	return nil, s.err
}

func (s failIngestEmbedder) Dim() int {
	return 0
}
