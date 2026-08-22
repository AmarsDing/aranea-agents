package knowledge

import (
	"context"
	"strings"
	"testing"
	"time"

	"aranea-agents/pkg/apierror"
)

func TestParseWriteBackExperts(t *testing.T) {
	body := FormatWriteBackAppendix(WriteBackInput{SessionID: "s1", AgentID: "ag-ops", UserID: "u-1"}, []WriteBackFact{
		{FactID: "1", Statement: "部署必须走灰度且保留回滚开关", FactKind: "constraint", Confidence: 0.9},
		{FactID: "2", Statement: "用户偏好深色模式界面", FactKind: "preference", Confidence: 0.91},
	})
	got := ParseWriteBackExperts(body)
	if len(got) == 0 {
		t.Fatalf("empty experts from %q", body)
	}
	found := false
	for _, e := range got {
		if e.AgentID == "ag-ops" && e.FactCount >= 2 {
			found = true
		}
	}
	if !found {
		t.Fatalf("experts=%+v", got)
	}
}

func TestCollectionHealthSnapshot_Density(t *testing.T) {
	m := noOpMockRepo()
	m.docListFn = func(_ context.Context, _ string, _, _ int) ([]Document, int, error) {
		return []Document{
			{ID: "d1", RelPath: "a.md", Source: "a.md"},
			{ID: "d2", RelPath: "b.md", Source: "b.md"},
			{ID: "d3", RelPath: "inbox/writeback-2026-08-15.md", Source: "inbox/writeback-2026-08-15.md"},
		}, 3, nil
	}
	u := NewUsecaseFromRepo(m)
	u.SetGraphRepo(fakeGraphLinks{links: []Link{
		{DocID: "d1", TargetDocID: "d2", LinkType: LinkTypeExplicit},
	}})
	h, err := u.CollectionHealthSnapshot(context.Background(), "c1")
	if err != nil {
		t.Fatalf("health: %v", err)
	}
	if h.DocumentCount != 3 || h.ExplicitEdges != 1 || h.WriteBackNotes != 1 {
		t.Fatalf("health=%+v", h)
	}
	if h.IsolatedCount < 1 || h.LinkDensity <= 0 {
		t.Fatalf("orphan/density %+v", h)
	}
}

func TestFormatAgentMemoryProjection_OverwriteShape(t *testing.T) {
	body := FormatAgentMemoryProjection("ag-1", []WriteBackFact{
		{FactID: "f1", Statement: "用户偏好深色模式界面", FactKind: "preference", Confidence: 0.9},
	}, time.Date(2026, 8, 15, 1, 0, 0, 0, time.UTC))
	for _, need := range []string{
		"projection: agent-memory",
		"agent_id: ag-1",
		"只读投影",
		"MEMORY handbook index",
		"| `f1` | preference |",
		"## preference",
		"fact_id: `f1`",
		"agent_id: `ag-1`",
	} {
		if !strings.Contains(body, need) {
			t.Errorf("missing %q in %q", need, body)
		}
	}
	if p := AgentMemoryRelPath("ag/1"); p != "agents/ag_1.md" {
		t.Fatalf("rel=%q", p)
	}
}

func TestAgentMemoryProjector_CreatesDoc(t *testing.T) {
	m := noOpMockRepo()
	var created Document
	m.collListFn = func(_ context.Context, ws string, _, _ int) ([]Collection, int, error) {
		return []Collection{{ID: "team-1", Name: WriteBackCollectionName, VaultBackend: VaultBackendTeam, Workspace: ws}}, 1, nil
	}
	m.docGetByRelFn = func(_ context.Context, _, rel string) (Document, error) {
		if created.ID != "" && created.RelPath == rel {
			return created, nil
		}
		return Document{}, apierror.NotFound("KNOWLEDGE", "missing")
	}
	m.docCreateFn = func(_ context.Context, d Document) (Document, error) {
		d.ID = "proj-1"
		created = d
		return d, nil
	}
	u := NewUsecaseFromRepo(m)
	p := NewAgentMemoryProjector(u, stubFacts{rows: []WriteBackFact{{
		FactID: "f1", Statement: "用户偏好深色模式界面", FactKind: "preference", Confidence: 0.9,
	}}}, nil)
	if err := p.ProjectAgentMemory(context.Background(), "ws-1", "ag-1"); err != nil {
		t.Fatalf("project: %v", err)
	}
	if created.RelPath != "agents/ag-1.md" || !strings.Contains(created.ContentText, "用户偏好深色模式界面") {
		t.Fatalf("created=%+v", created)
	}
}

type stubFacts struct{ rows []WriteBackFact }

func (s stubFacts) ListAgentFacts(context.Context, string, int) ([]WriteBackFact, error) {
	return s.rows, nil
}

type fakeGraphLinks struct{ links []Link }

func (f fakeGraphLinks) ListCollectionLinks(context.Context, string, []string) ([]Link, error) {
	return f.links, nil
}
