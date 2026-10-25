package knowledge

import (
	"context"
	"errors"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	bizknowledge "aranea-agents/internal/biz/knowledge"
)

// ===== G-05-asof（ADR-KN-20261020-J）：扩散激活时态腿 =====
//
// 契约：q.Temporal 非零 → 逐跳 BFS 改走 TemporalActiveLinkReader（双跳共用同一
// span，时态一致性天然）；未接线时态端口 → 回退当前 active 快照并记 warn
// （任一依赖降级不阻断检索的既有纪律）；span 零值 → 旧端口字节级等价当前路径。

type fakeTemporalActiveReader struct {
	links   []bizknowledge.ActiveLink
	queries [][]string
	spans   []bizknowledge.TemporalSpan
	err     error
}

func (f *fakeTemporalActiveReader) ListActiveLinksAsOf(_ context.Context, _ string, docIDs []string, span bizknowledge.TemporalSpan) ([]bizknowledge.ActiveLink, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.queries = append(f.queries, append([]string(nil), docIDs...))
	f.spans = append(f.spans, span)
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

func TestGraphExpander_AsOfUsesTemporalPortForBothHops(t *testing.T) {
	t0 := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	// 时态边集：A→B→C（彼时有效）；旧端口若被调用立即硬错，钉死走新端口。
	temporal := &fakeTemporalActiveReader{links: []bizknowledge.ActiveLink{
		{DocID: "dA", TargetDocID: "dB", LinkType: bizknowledge.LinkTypeSemantic, WeightF: 1},
		{DocID: "dB", TargetDocID: "dC", LinkType: bizknowledge.LinkTypeSemantic, WeightF: 1},
	}}
	legacy := &fakeActiveLinkReader{err: errors.New("current port must not be used under as-of")}
	exp := newSpreadExpander(legacy, map[string][]biz.KnowledgeChunk{
		"dB": {{ID: "cB", DocID: "dB", Content: "B body"}},
		"dC": {{ID: "cC", DocID: "dC", Content: "C body"}},
	})
	exp.SetTemporalActiveLinks(temporal)

	seeds := []biz.KnowledgeChunk{{ID: "cA", DocID: "dA", CollectionID: "c1", Score: 1.0}}
	got := exp.Expand(context.Background(),
		biz.KnowledgeSearchQuery{CollectionID: "c1", Query: "关系如何", TopK: 10,
			Temporal: bizknowledge.TemporalSpan{ValidAt: t0}}, seeds)

	ids := map[string]bool{}
	for _, ch := range got {
		ids[ch.ID] = true
	}
	if !ids["cB"] || !ids["cC"] {
		t.Fatalf("as-of 2-hop recall broken: %v, want cB+cC", ids)
	}
	if len(temporal.spans) != 2 {
		t.Fatalf("temporal port called %d hops, want 2", len(temporal.spans))
	}
	for i, sp := range temporal.spans {
		if !sp.ValidAt.Equal(t0) || sp.RecordedOn() {
			t.Fatalf("hop %d span drifted: %+v, want T-only @%v", i, sp, t0)
		}
	}
	if len(legacy.queries) != 0 {
		t.Fatalf("legacy active port must not be consulted under as-of: %v", legacy.queries)
	}
}

func TestGraphExpander_AsOfFallsBackToCurrentWhenTemporalMissing(t *testing.T) {
	t0 := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	// 未接线时态端口 → 回退当前 active 快照（warn 侧效应不钉）：dB 仍应召回。
	legacy := &fakeActiveLinkReader{links: []bizknowledge.ActiveLink{
		{DocID: "dA", TargetDocID: "dB", LinkType: bizknowledge.LinkTypeExplicit, WeightF: 1},
	}}
	exp := newSpreadExpander(legacy, map[string][]biz.KnowledgeChunk{
		"dB": {{ID: "cB", DocID: "dB"}},
	})
	seeds := []biz.KnowledgeChunk{{ID: "cA", DocID: "dA", CollectionID: "c1", Score: 1.0}}
	got := exp.Expand(context.Background(),
		biz.KnowledgeSearchQuery{CollectionID: "c1", Query: "关系如何", TopK: 10,
			Temporal: bizknowledge.TemporalSpan{ValidAt: t0}}, seeds)
	found := false
	for _, ch := range got {
		if ch.ID == "cB" {
			found = true
		}
	}
	if !found || len(legacy.queries) == 0 {
		t.Fatalf("temporal port missing must degrade to current snapshot: %v queries %v", found, legacy.queries)
	}
}

func TestGraphExpander_NoSpanIgnoresTemporalPort(t *testing.T) {
	// span 零值 = current 快报：旧端口服务、时态端口零调用（字节级等价红线）。
	temporal := &fakeTemporalActiveReader{err: errors.New("temporal port must not be used without span")}
	legacy := &fakeActiveLinkReader{links: []bizknowledge.ActiveLink{
		{DocID: "dA", TargetDocID: "dB", LinkType: bizknowledge.LinkTypeExplicit, WeightF: 1},
	}}
	exp := newSpreadExpander(legacy, map[string][]biz.KnowledgeChunk{
		"dB": {{ID: "cB", DocID: "dB"}},
	})
	exp.SetTemporalActiveLinks(temporal)
	seeds := []biz.KnowledgeChunk{{ID: "cA", DocID: "dA", CollectionID: "c1", Score: 1.0}}
	got := exp.Expand(context.Background(),
		biz.KnowledgeSearchQuery{CollectionID: "c1", Query: "关系如何", TopK: 10}, seeds)
	found := false
	for _, ch := range got {
		if ch.ID == "cB" {
			found = true
		}
	}
	if !found || len(temporal.queries) != 0 {
		t.Fatalf("current path must not touch temporal port: found=%v temporalQueries=%v", found, temporal.queries)
	}
}
