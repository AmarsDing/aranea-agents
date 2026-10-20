package eval

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

// fixture 单测：语料编译（成链/切分/边提取）、锚点检索、确定性。

func mustFixtureSearcher(t *testing.T) Searcher {
	t.Helper()
	corpus, err := LoadFixtureCorpus(filepath.Join("testdata", goldCorpusPath))
	if err != nil {
		t.Fatalf("load corpus: %v", err)
	}
	s, err := NewFixtureSearcher(corpus)
	if err != nil {
		t.Fatalf("NewFixtureSearcher: %v", err)
	}
	return s
}

func TestFixtureSearcher_ExpansionReachesNeighborDoc(t *testing.T) {
	s := mustFixtureSearcher(t)
	hits, err := s.Search(context.Background(), "夜班能不能一个人值守", 8)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	docs := map[string]bool{}
	for _, h := range hits {
		docs[h.DocID] = true
	}
	// 锚点最坏假设：种子是 ops，duty 只能由一跳扩展补回。
	if !docs["ops"] || !docs["duty"] {
		t.Fatalf("expected ops+duty in hits, got %+v", hits)
	}
}

func TestFixtureSearcher_SeedAnchorsAllGoldQueries(t *testing.T) {
	corpus, err := LoadFixtureCorpus(filepath.Join("testdata", goldCorpusPath))
	if err != nil {
		t.Fatalf("load corpus: %v", err)
	}
	set, err := LoadEvalSet(filepath.Join("testdata", goldSetPath))
	if err != nil {
		t.Fatalf("load set: %v", err)
	}
	for _, c := range set.Cases {
		if _, ok := corpus.Seeds[c.Query]; !ok {
			t.Fatalf("case %q has no seed anchor in corpus (query %q)", c.ID, c.Query)
		}
	}
}

func TestFixtureSearcher_UnknownQueryErrors(t *testing.T) {
	s := mustFixtureSearcher(t)
	if _, err := s.Search(context.Background(), "从没登记过的查询", 5); err == nil {
		t.Fatal("expected error for unanchored query")
	}
}

func TestFixtureSearcher_Deterministic(t *testing.T) {
	s := mustFixtureSearcher(t)
	first, err := s.Search(context.Background(), "15 minutes of silence", 8)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	for i := 0; i < 5; i++ {
		got, err := s.Search(context.Background(), "15 minutes of silence", 8)
		if err != nil {
			t.Fatalf("Search run %d: %v", i, err)
		}
		if len(got) != len(first) {
			t.Fatalf("run %d hit count = %d, want %d (nondeterministic)", i, len(got), len(first))
		}
		for j := range got {
			if got[j] != first[j] {
				t.Fatalf("run %d hit %d = %+v, want %+v (nondeterministic)", i, j, got[j], first[j])
			}
		}
	}
}

func TestFixtureSearcher_ChunkIDsStable(t *testing.T) {
	s := mustFixtureSearcher(t)
	hits, err := s.Search(context.Background(), "机房单人禁入", 8)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) == 0 || !strings.HasSuffix(hits[0].ChunkID, "-c0") {
		t.Fatalf("seed hit should carry <doc>-c0 chunk id, got %+v", hits)
	}
}

func TestLoadFixtureCorpusErrors(t *testing.T) {
	if _, err := LoadFixtureCorpus(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("expected error for missing file")
	}
	cases := map[string]string{
		"empty docs":     `{"docs": [], "seeds": {}}`,
		"dup id":         `{"docs": [{"id":"a","title":"T","body":"B"},{"id":"a","title":"T2","body":"B2"}], "seeds": {}}`,
		"missing title":  `{"docs": [{"id":"a","title":"","body":"B"}], "seeds": {}}`,
		"dangling seed":  `{"docs": [{"id":"a","title":"T","body":"B"}], "seeds": {"q":"ghost"}}`,
		"empty seed key": `{"docs": [{"id":"a","title":"T","body":"B"}], "seeds": {"  ":"a"}}`,
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "c.json")
			if err := writeTestFile(p, raw); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadFixtureCorpus(p); err == nil {
				t.Fatalf("expected error for %s", name)
			}
		})
	}
}

// TestNewFixtureSearcherValidation 按模式参数化直接喂构造器——与
// TestLoadFixtureCorpusErrors 不同入口走通校验的每个分支，防止两侧校验能力
// 静默不对称（都为 validate() 的同一实现，但若将来分叉，这里逐模式能抓到）。
func TestNewFixtureSearcherValidation(t *testing.T) {
	cases := map[string]FixtureCorpus{
		"empty docs": {},
		"dup id": {
			Docs:  []FixtureDoc{{ID: "a", Title: "T", Body: "B"}, {ID: "a", Title: "T2", Body: "B2"}},
			Seeds: map[string]string{},
		},
		"missing title": {
			Docs: []FixtureDoc{{ID: "a", Title: "", Body: "B"}},
		},
		"dangling seed": {
			Docs:  []FixtureDoc{{ID: "a", Title: "T", Body: "B"}},
			Seeds: map[string]string{"q": "ghost"},
		},
		"empty seed key": {
			Docs:  []FixtureDoc{{ID: "a", Title: "T", Body: "B"}},
			Seeds: map[string]string{"  ": "a"},
		},
	}
	for name, corpus := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := NewFixtureSearcher(corpus); err == nil {
				t.Fatalf("NewFixtureSearcher must reject %s", name)
			}
		})
	}
}
