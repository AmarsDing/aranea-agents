package data

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
)

const knowledgeRAGFixtureDir = "../../docs/testing/agent-eval-20260818/03-knowledge-rag"

type knowledgeRAGDataset struct {
	Cases []struct {
		ID               string   `json:"id"`
		SourceDoc        string   `json:"source_doc"`
		Question         string   `json:"question"`
		ExpectedKeywords []string `json:"expected_keywords"`
	} `json:"cases"`
}

func TestKnowledgeRAGDatasetContract(t *testing.T) {
	dataset := loadKnowledgeRAGDataset(t)
	if len(dataset.Cases) < 30 {
		t.Fatalf("knowledge retrieval baseline has %d cases, want at least 30", len(dataset.Cases))
	}
	seen := make(map[string]struct{}, len(dataset.Cases))
	for _, tc := range dataset.Cases {
		if tc.ID == "" || tc.SourceDoc == "" || tc.Question == "" || len(tc.ExpectedKeywords) == 0 {
			t.Fatalf("incomplete retrieval case: %+v", tc)
		}
		if _, ok := seen[tc.ID]; ok {
			t.Fatalf("duplicate retrieval case id %q", tc.ID)
		}
		seen[tc.ID] = struct{}{}
		if _, err := os.Stat(filepath.Join(knowledgeRAGFixtureDir, tc.SourceDoc)); err != nil {
			t.Fatalf("case %s corpus %s: %v", tc.ID, tc.SourceDoc, err)
		}
	}
}

// TestKnowledgeRepo_SearchChunksBM25_RetrievalBaseline gates the production
// lexical path on keyword, identifier, and natural-language queries against
// the five-document eval corpus. NL questions are compacted into content
// needles before BM25; they are a hard gate, not a hybrid-only profile.
func TestKnowledgeRepo_SearchChunksBM25_RetrievalBaseline(t *testing.T) {
	repo := setupKnowledgeSearchRepo(t)
	dataset := loadKnowledgeRAGDataset(t)
	ctx := context.Background()
	const collectionID = "gold-bm25-rag"
	seedKnowledgeRAGCorpus(t, repo, collectionID, dataset)

	kwHits1, kwHits5, kwMRR := scoreBM25Queries(t, repo, collectionID, keywordQueries(dataset), true)
	t.Logf("BM25 keyword gate: cases=%d hit@1=%.3f hit@5=%.3f mrr=%.3f",
		len(dataset.Cases), kwHits1, kwHits5, kwMRR)
	if kwHits5 < 0.90 {
		t.Fatalf("BM25 keyword HitRate@5 %.3f is below release gate 0.90", kwHits5)
	}

	nlHits1, nlHits5, nlMRR := scoreBM25Queries(t, repo, collectionID, nlQueries(dataset), true)
	t.Logf("BM25 natural-language gate: cases=%d hit@1=%.3f hit@5=%.3f mrr=%.3f",
		len(dataset.Cases), nlHits1, nlHits5, nlMRR)
	if nlHits5 < 0.90 {
		t.Fatalf("BM25 natural-language HitRate@5 %.3f is below release gate 0.90", nlHits5)
	}

	idQueries := []bm25GoldQuery{
		{id: "id-ins-ticket", query: "INS-YYYYMMDD-NN", want: "sample-doc-inspection.md"},
		{id: "id-sw-core", query: "SW-Core-01", want: "sample-doc-inspection.md"},
		{id: "id-emg-prefix", query: "EMG-", want: "sample-doc-change.md"},
		{id: "id-policy-test", query: "POLICY-TEST", want: "sample-doc-change.md"},
		{id: "id-tacacs", query: "TACACS+", want: "sample-doc-security.md"},
		{id: "id-mgmt-net", query: "10.99.0.0/24", want: "sample-doc-security.md"},
		{id: "id-postmortem", query: "TPL-POSTMORTEM-V3", want: "sample-doc-emergency.md"},
		{id: "id-duty-log", query: "DUTY-YYYYMMDD-NN", want: "sample-doc-duty.md"},
		{id: "id-cn2", query: "电信 CN2 专线", want: "sample-doc-emergency.md"},
		{id: "id-ups-runtime", query: "28 分钟", want: "sample-doc-emergency.md"},
	}
	_, idHits5, _ := scoreBM25Queries(t, repo, collectionID, idQueries, true)
	t.Logf("BM25 identifier slice: cases=%d hit@5=%.3f", len(idQueries), idHits5)
	if idHits5 < 0.80 {
		t.Fatalf("BM25 identifier HitRate@5 %.3f is below release gate 0.80", idHits5)
	}

	miss, err := repo.SearchChunksBM25(ctx, biz.KnowledgeSearchQuery{
		CollectionID: collectionID, Query: "不存在的词语xx", TopK: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(miss) != 0 {
		t.Errorf("negative query hit %d chunks", len(miss))
	}
}

type bm25GoldQuery struct {
	id, query, want string
}

func keywordQueries(dataset knowledgeRAGDataset) []bm25GoldQuery {
	out := make([]bm25GoldQuery, 0, len(dataset.Cases))
	for _, tc := range dataset.Cases {
		out = append(out, bm25GoldQuery{
			id: tc.ID + "-kw", query: distinctiveKeyword(tc.ExpectedKeywords), want: tc.SourceDoc,
		})
	}
	return out
}

func distinctiveKeyword(keywords []string) string {
	best := ""
	for _, kw := range keywords {
		kw = strings.TrimSpace(kw)
		if runeLen(kw) > runeLen(best) {
			best = kw
		}
	}
	return best
}

func runeLen(s string) int {
	return len([]rune(s))
}

func nlQueries(dataset knowledgeRAGDataset) []bm25GoldQuery {
	out := make([]bm25GoldQuery, 0, len(dataset.Cases))
	for _, tc := range dataset.Cases {
		out = append(out, bm25GoldQuery{id: tc.ID, query: tc.Question, want: tc.SourceDoc})
	}
	return out
}

func seedKnowledgeRAGCorpus(t *testing.T, repo *knowledgeRepo, collectionID string, dataset knowledgeRAGDataset) {
	t.Helper()
	ctx := context.Background()
	if _, err := repo.CreateCollection(ctx, biz.KnowledgeCollection{
		ID: collectionID, Name: "knowledge retrieval baseline",
	}); err != nil {
		t.Fatal(err)
	}
	seenDocs := make(map[string]struct{})
	for _, tc := range dataset.Cases {
		if _, ok := seenDocs[tc.SourceDoc]; ok {
			continue
		}
		seenDocs[tc.SourceDoc] = struct{}{}
		body, err := os.ReadFile(filepath.Join(knowledgeRAGFixtureDir, tc.SourceDoc))
		if err != nil {
			t.Fatalf("read corpus %s: %v", tc.SourceDoc, err)
		}
		if _, err := repo.CreateDocument(ctx, biz.KnowledgeDocument{
			ID: tc.SourceDoc, CollectionID: collectionID, RelPath: tc.SourceDoc,
			Source: tc.SourceDoc, Status: "indexed", ContentText: string(body),
		}); err != nil {
			t.Fatal(err)
		}
		if err := repo.InsertChunks(ctx, []biz.KnowledgeChunk{{
			ID: "chunk-" + tc.SourceDoc, DocID: tc.SourceDoc,
			CollectionID: collectionID, Content: string(body),
		}}); err != nil {
			t.Fatal(err)
		}
	}
}

func scoreBM25Queries(t *testing.T, repo *knowledgeRepo, collectionID string, queries []bm25GoldQuery, logMiss bool) (hit1, hit5, mrr float64) {
	t.Helper()
	ctx := context.Background()
	hitsAt1, hitsAt5 := 0, 0
	reciprocal := 0.0
	for _, q := range queries {
		chunks, err := repo.SearchChunksBM25(ctx, biz.KnowledgeSearchQuery{
			CollectionID: collectionID, Query: q.query, TopK: 5,
		})
		if err != nil {
			t.Fatalf("query %s (%q): %v", q.id, q.query, err)
		}
		hit := false
		ids := make([]string, 0, len(chunks))
		for i, chunk := range chunks {
			ids = append(ids, chunk.DocID)
			if chunk.DocID != q.want {
				continue
			}
			hit = true
			hitsAt5++
			if i == 0 {
				hitsAt1++
			}
			reciprocal += 1 / float64(i+1)
			break
		}
		if !hit && logMiss {
			t.Logf("miss %s query=%q docs=%v want=%s", q.id, q.query, ids, q.want)
		}
	}
	n := float64(len(queries))
	if n == 0 {
		return 0, 0, 0
	}
	return float64(hitsAt1) / n, float64(hitsAt5) / n, reciprocal / n
}

func loadKnowledgeRAGDataset(t *testing.T) knowledgeRAGDataset {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(knowledgeRAGFixtureDir, "sample-knowledge-qa.json"))
	if err != nil {
		t.Fatal(err)
	}
	var dataset knowledgeRAGDataset
	if err := json.Unmarshal(raw, &dataset); err != nil {
		t.Fatal(err)
	}
	return dataset
}
