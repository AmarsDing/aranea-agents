package knowledge

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/pkg/loggateway"
)

// 自治理图谱 M1-4：受限扩散激活 2 跳扩展。
// 契约：沿 active 边传播 2 跳，每跳能量 ×0.5；边类型权重 explicit>semantic>entity>co_activated；
// 侧抑制 top-N；零 LLM；未接线 ActiveLinkReader 时保持 1 跳旧行为。

type fakeActiveLinkReader struct {
	links []bizknowledge.ActiveLink
	// 记录每跳查询的 docIDs，验证 BFS 分层
	queries [][]string
	err   error
}

func (f *fakeActiveLinkReader) ListActiveLinks(_ context.Context, _ string, docIDs []string) ([]bizknowledge.ActiveLink, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.queries = append(f.queries, append([]string(nil), docIDs...))
	want := map[string]bool{}
	for _, id := range docIDs {
		want[id] = true
	}
	var out []bizknowledge.ActiveLink
	for _, l := range f.links {
		if want[l.DocID] || want[l.TargetDocID] {
			out = append(out, l)
		}
	}
	return out, nil
}

func newSpreadExpander(active *fakeActiveLinkReader, chunksByDoc map[string][]biz.KnowledgeChunk) *GraphExpander {
	exp := NewGraphExpander(fakeLinkReader{}, fakeChunkLister{byDoc: chunksByDoc}, loggateway.NewNoop())
	exp.SetActiveLinks(active)
	return exp
}

func TestGraphExpander_SpreadingActivation_TwoHop(t *testing.T) {
	// A(种子) →B explicit →C explicit：查 A 应 2 跳召回 C（无词重叠）。
	active := &fakeActiveLinkReader{links: []bizknowledge.ActiveLink{
		{DocID: "dA", TargetDocID: "dB", LinkType: bizknowledge.LinkTypeExplicit, WeightF: 1},
		{DocID: "dB", TargetDocID: "dC", LinkType: bizknowledge.LinkTypeExplicit, WeightF: 1},
	}}
	exp := newSpreadExpander(active, map[string][]biz.KnowledgeChunk{
		"dB": {{ID: "cB", DocID: "dB", Content: "B body"}},
		"dC": {{ID: "cC", DocID: "dC", Content: "C body"}},
	})
	seeds := []biz.KnowledgeChunk{{ID: "cA", DocID: "dA", CollectionID: "c1", Score: 1.0}}
	got := exp.Expand(context.Background(),
		biz.KnowledgeSearchQuery{CollectionID: "c1", Query: "关系如何", TopK: 10}, seeds)

	ids := map[string]float32{}
	for _, ch := range got {
		ids[ch.ID] = ch.Score
	}
	if _, ok := ids["cC"]; !ok {
		t.Fatalf("2-hop doc C not recalled: %+v", got)
	}
	if _, ok := ids["cB"]; !ok {
		t.Fatalf("1-hop doc B not recalled: %+v", got)
	}
	// 跳数衰减：B(0.5 能量) > C(0.25 能量)。
	if ids["cB"] <= ids["cC"] {
		t.Errorf("hop decay broken: B=%v C=%v, want B > C", ids["cB"], ids["cC"])
	}
	// BFS 分层：第一跳查种子，第二跳查 B（不含已激活的 C/A）。
	if len(active.queries) != 2 {
		t.Fatalf("hops = %d, want 2", len(active.queries))
	}
	if len(active.queries[0]) != 1 || active.queries[0][0] != "dA" {
		t.Errorf("hop1 frontier = %v, want [dA]", active.queries[0])
	}
	if len(active.queries[1]) != 1 || active.queries[1][0] != "dB" {
		t.Errorf("hop2 frontier = %v, want [dB]", active.queries[1])
	}
}

func TestGraphExpander_SpreadingActivation_EdgeTypeWeights(t *testing.T) {
	// 同跳同 weight_f：explicit(1.0) > entity(0.7) > co_activated(0.4)。
	active := &fakeActiveLinkReader{links: []bizknowledge.ActiveLink{
		{DocID: "dA", TargetDocID: "dExp", LinkType: bizknowledge.LinkTypeExplicit, WeightF: 1},
		{DocID: "dA", TargetDocID: "dEnt", LinkType: bizknowledge.LinkTypeEntity, WeightF: 1},
		{DocID: "dA", TargetDocID: "dCoa", LinkType: bizknowledge.LinkTypeCoActivated, WeightF: 1},
	}}
	exp := newSpreadExpander(active, map[string][]biz.KnowledgeChunk{
		"dExp": {{ID: "cExp", DocID: "dExp"}},
		"dEnt": {{ID: "cEnt", DocID: "dEnt"}},
		"dCoa": {{ID: "cCoa", DocID: "dCoa"}},
	})
	seeds := []biz.KnowledgeChunk{{ID: "cA", DocID: "dA", CollectionID: "c1", Score: 1.0}}
	got := exp.Expand(context.Background(),
		biz.KnowledgeSearchQuery{CollectionID: "c1", Query: "关系如何", TopK: 10}, seeds)

	scores := map[string]float32{}
	for _, ch := range got {
		scores[ch.ID] = ch.Score
	}
	if !(scores["cExp"] > scores["cEnt"] && scores["cEnt"] > scores["cCoa"]) {
		t.Errorf("edge type weights broken: exp=%v ent=%v coa=%v",
			scores["cExp"], scores["cEnt"], scores["cCoa"])
	}
}

func TestGraphExpander_SpreadingActivation_LateralInhibition(t *testing.T) {
	// 12 个同权邻居 → 侧抑制只保留 top-10。
	var links []bizknowledge.ActiveLink
	chunksByDoc := map[string][]biz.KnowledgeChunk{}
	for _, id := range []string{"n01", "n02", "n03", "n04", "n05", "n06", "n07", "n08", "n09", "n10", "n11", "n12"} {
		links = append(links, bizknowledge.ActiveLink{DocID: "dA", TargetDocID: id, LinkType: bizknowledge.LinkTypeExplicit, WeightF: 1})
		chunksByDoc[id] = []biz.KnowledgeChunk{{ID: "c-" + id, DocID: id}}
	}
	active := &fakeActiveLinkReader{links: links}
	exp := newSpreadExpander(active, chunksByDoc)
	seeds := []biz.KnowledgeChunk{{ID: "cA", DocID: "dA", CollectionID: "c1", Score: 1.0}}
	got := exp.Expand(context.Background(),
		biz.KnowledgeSearchQuery{CollectionID: "c1", Query: "关系如何", TopK: 50}, seeds)

	neighbors := 0
	for _, ch := range got {
		if ch.DocID != "dA" {
			neighbors++
		}
	}
	if neighbors > graphExpandActivationCap {
		t.Errorf("lateral inhibition broken: %d neighbors > cap %d", neighbors, graphExpandActivationCap)
	}
}

func TestGraphExpander_SpreadingActivation_LegacyWithoutActiveReader(t *testing.T) {
	// 未接线 ActiveLinkReader：保持 1 跳旧行为（dB 召回、dC 不召回）。
	exp := NewGraphExpander(
		fakeLinkReader{byDoc: map[string][]bizknowledge.Link{
			"dA": {{DocID: "dA", TargetDocID: "dB", LinkType: bizknowledge.LinkTypeExplicit, Weight: 1}},
		}},
		fakeChunkLister{byDoc: map[string][]biz.KnowledgeChunk{
			"dB": {{ID: "cB", DocID: "dB"}},
			"dC": {{ID: "cC", DocID: "dC"}},
		}},
		loggateway.NewNoop(),
	)
	seeds := []biz.KnowledgeChunk{{ID: "cA", DocID: "dA", CollectionID: "c1", Score: 1.0}}
	got := exp.Expand(context.Background(),
		biz.KnowledgeSearchQuery{CollectionID: "c1", Query: "关系如何", TopK: 10}, seeds)
	ids := map[string]bool{}
	for _, ch := range got {
		ids[ch.ID] = true
	}
	if !ids["cB"] || ids["cC"] {
		t.Fatalf("legacy 1-hop broken: %v", ids)
	}
}
