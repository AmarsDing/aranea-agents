package eval

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

// G-33（ADR-KN-20261020-I 测试策略）：金标卡片腿消融对照——装配 nil 关腿
// 必须复现基线（探针宿主文档消失），接线态探针的宿主命中全部归功第三腿。
// 探针宿主刻意选在种子一跳之外：sec 不被任何文档引用、非任何种子的邻居，
// 无卡片腿时无论如何扩展都不可达。

func loadGoldCorpus(t *testing.T) FixtureCorpus {
	t.Helper()
	c, err := LoadFixtureCorpus(filepath.Join("testdata", goldCorpusPath))
	if err != nil {
		t.Fatalf("load corpus: %v", err)
	}
	return c
}

func docSet(hits []Hit) map[string]struct{} {
	out := make(map[string]struct{}, len(hits))
	for _, h := range hits {
		out[h.DocID] = struct{}{}
	}
	return out
}

func hasID(hits []Hit, id string) bool {
	for _, h := range hits {
		if h.ChunkID == id {
			return true
		}
	}
	return false
}

func TestGoldCardLegAblation(t *testing.T) {
	corpus := loadGoldCorpus(t)
	wired, err := NewFixtureSearcher(corpus)
	if err != nil {
		t.Fatalf("wired: %v", err)
	}
	ablated, err := NewFixtureSearcherAblated(corpus)
	if err != nil {
		t.Fatalf("ablated: %v", err)
	}
	ctx := context.Background()

	probes := []struct {
		query       string
		cardChunkID string
		wiredDocs   []string // 接线态完整可达集（种子 ∪ 一跳 ∪ 卡宿主及其一跳）
		ablatedDocs []string // 关腿基线：种子 ∪ 一跳（卡宿主及其一跳消失）
	}{
		{"变更发布要遵守哪条安全基线", "g33e/101/sec", []string{"play", "esc", "sec", "net"}, []string{"play", "esc"}},
		{"从运维手册顺到哪份安全约束", "g33r/102/sec", []string{"ops", "duty", "esc", "sec", "net"}, []string{"ops", "duty", "esc"}},
	}
	for _, p := range probes {
		t.Run(p.cardChunkID, func(t *testing.T) {
			wiredHits, err := wired.Search(ctx, p.query, goldTopK)
			if err != nil {
				t.Fatalf("wired search: %v", err)
			}
			if !hasID(wiredHits, p.cardChunkID) {
				t.Fatalf("wired hits missing card chunk %s: %+v", p.cardChunkID, wiredHits)
			}
			wd := docSet(wiredHits)
			if len(wd) != len(p.wiredDocs) {
				t.Fatalf("wired docs = %v, want %v", wd, p.wiredDocs)
			}
			for _, d := range p.wiredDocs {
				if _, ok := wd[d]; !ok {
					t.Fatalf("wired docs %v missing %s", wd, d)
				}
			}

			ablHits, err := ablated.Search(ctx, p.query, goldTopK)
			if err != nil {
				t.Fatalf("ablated search: %v", err)
			}
			for _, h := range ablHits {
				if strings.HasPrefix(h.ChunkID, "g33") {
					t.Fatalf("ablated hits must carry no g33 residue: %+v", h)
				}
			}
			ad := docSet(ablHits)
			if len(ad) != len(p.ablatedDocs) {
				t.Fatalf("ablated docs = %v, want %v", ad, p.ablatedDocs)
			}
			for _, d := range p.ablatedDocs {
				if _, ok := ad[d]; !ok {
					t.Fatalf("ablated docs %v missing %s", ad, d)
				}
			}
		})
	}
}

func TestGoldCardLegNilReproducesBaseline(t *testing.T) {
	corpus := loadGoldCorpus(t)
	wired, err := NewFixtureSearcher(corpus)
	if err != nil {
		t.Fatalf("wired: %v", err)
	}
	ablated, err := NewFixtureSearcherAblated(corpus)
	if err != nil {
		t.Fatalf("ablated: %v", err)
	}
	ctx := context.Background()
	// 无卡声明的 12 条原始查询：接线与否输出逐 hit 一致（卡片腿对存量用例
	// 零扰动——与生产 nil 关腿语义对齐）。
	for q := range corpus.Seeds {
		if _, hasCards := corpus.Cards[q]; hasCards {
			continue
		}
		wh, err := wired.Search(ctx, q, goldTopK)
		if err != nil {
			t.Fatalf("wired %q: %v", q, err)
		}
		ah, err := ablated.Search(ctx, q, goldTopK)
		if err != nil {
			t.Fatalf("ablated %q: %v", q, err)
		}
		if len(wh) != len(ah) {
			t.Fatalf("query %q: wired %d hits vs ablated %d", q, len(wh), len(ah))
		}
		for i := range wh {
			if wh[i] != ah[i] {
				t.Fatalf("query %q hit %d mismatch: %+v vs %+v", q, i, wh[i], ah[i])
			}
		}
	}
}
