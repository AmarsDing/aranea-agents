package knowledge

import (
	"context"
	"strings"
	"testing"
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
	})
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

type stubIngestEmbedder struct {
	dim int
}

func (s stubIngestEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	out := make([]float32, s.dim)
	return out, nil
}

func (s stubIngestEmbedder) EmbedBatch(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = make([]float32, s.dim)
	}
	return out, nil
}
