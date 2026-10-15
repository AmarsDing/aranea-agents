package configgraph

import (
	"strings"
	"testing"
	"time"
)

func TestExtractor_Team(t *testing.T) {
	defJSON := `{
		"version":1,"mode":"sequential",
		"synthesizer_agent_id":"uuid-a1",
		"intent_anchor_agent_id":"uuid-a2",
		"members":[
			{"agent_id":"uuid-a1","role":"leader","sort_order":1},
			{"agent_id":"uuid-a2","role":"worker","sort_order":2,"enabled":false}
		],
		"failure_policy":{"node_overrides":{"member-1":{"policy":"fallback","fallback_agent":"uuid-a3"}}},
		"graph_template_id":"uuid-g2",
		"collection_ids":["uuid-kc1"]
	}`
	src := &fakeSource{teams: []TeamRow{
		{ID: "uuid-team1", TeamKey: "main", DepartmentID: "uuid-org-dept", DeptLeadAgentID: "uuid-a1",
			CrossDeptMemberIDs: `["uuid-a2"," uuid-a3 "]`, LinkedGraphID: "uuid-g1", DefinitionJSON: defJSON},

		{ID: "uuid-team2", TeamKey: "legacy", DefinitionJSON: `{"linked_graph_id":"uuid-g2","members":[]}`},

		{ID: "uuid-team3", TeamKey: "builtin", DefinitionJSON: `{"graph_template_id":"pipeline","members":[]}`},

		{ID: "uuid-team4", TeamKey: "broken", DeptLeadAgentID: "uuid-a1", DefinitionJSON: `{oops`},

		{ID: "uuid-team5", TeamKey: "dead", DeptLeadAgentID: "uuid-a1", DeletedAt: "2026-01-01T00:00:00Z"},
	}}
	nodes := extractNodes(t, teamExtractor{}, src)
	if len(nodes) != 5 || nodes[0].Attrs["is_default"] != false {
		t.Fatalf("nodes: %+v", nodes)
	}
	edges := extractEdges(t, teamExtractor{}, src)
	cases := []struct{ typ, src, dstType, dst string }{
		{EdgeTypeHasMember, "uuid-team1", NodeTypeAgent, "uuid-a1"},
		{EdgeTypeHasMember, "uuid-team1", NodeTypeAgent, "uuid-a2"},
		{EdgeTypeSynthesizer, "uuid-team1", NodeTypeAgent, "uuid-a1"},
		{EdgeTypeIntentAnchor, "uuid-team1", NodeTypeAgent, "uuid-a2"},
		{EdgeTypeFallbackAgent, "uuid-team1", NodeTypeAgent, "uuid-a3"},
		{EdgeTypeDeptLead, "uuid-team1", NodeTypeAgent, "uuid-a1"},
		{EdgeTypeCrossDeptMember, "uuid-team1", NodeTypeAgent, "uuid-a2"},
		{EdgeTypeCrossDeptMember, "uuid-team1", NodeTypeAgent, "uuid-a3"},
		{EdgeTypeBelongsTo, "uuid-team1", NodeTypeOrganization, "uuid-org-dept"},
		{EdgeTypeLinkedGraph, "uuid-team1", NodeTypeGraph, "uuid-g1"},
		{EdgeTypeLinkedGraph, "uuid-team2", NodeTypeGraph, "uuid-g2"},
		{EdgeTypeGraphTemplate, "uuid-team1", NodeTypeGraph, "uuid-g2"},
		{EdgeTypeScopedKnowledge, "uuid-team1", NodeTypeKnowledgeCollection, "uuid-kc1"},
	}
	for _, c := range cases {
		if findEdge(edges, c.typ, c.src, c.dstType, c.dst) == nil {
			t.Fatalf("%s %s→%s missing", c.typ, c.src, c.dst)
		}
	}
	if e := findEdge(edges, EdgeTypeHasMember, "uuid-team1", NodeTypeAgent, "uuid-a1"); e.Evidence["role"] != "leader" {
		t.Fatalf("member role evidence: %+v", e.Evidence)
	}
	if e := findEdge(edges, EdgeTypeFallbackAgent, "uuid-team1", NodeTypeAgent, "uuid-a3"); e.Evidence["node_key"] != "member-1" {
		t.Fatalf("fallback node_key evidence: %+v", e.Evidence)
	}
	if findEdge(edges, EdgeTypeGraphTemplate, "uuid-team3", NodeTypeGraph, "pipeline") != nil {
		t.Fatal("builtin graph template must not emit edge")
	}
	if !hasExtractError(edges, EdgeTypeHasMember, "uuid-team4") {
		t.Fatal("bad definition_json must yield extract_error marker")
	}
	if findEdge(edges, EdgeTypeDeptLead, "uuid-team4", NodeTypeAgent, "uuid-a1") == nil {
		t.Fatal("column edges must survive definition_json parse failure")
	}
	for _, e := range edges {
		if e.SrcRef == "uuid-team5" {
			t.Fatal("deleted team must not emit edges")
		}
	}
}

func TestExtractor_CronTask(t *testing.T) {
	src := &fakeSource{crons: []CronTaskRow{
		{ID: "uuid-c1", TaskKey: "agent-job", AgentID: "uuid-a1", ConfigJSON: `{"target_type":"agent"}`},
		{ID: "uuid-c2", TaskKey: "team-job", ConfigJSON: `{"target_type":"team","team_id":"uuid-team1"}`},
		{ID: "uuid-c3", TaskKey: "sync-job", ConfigJSON: `{"target_type":"model_registry_sync"}`},

		{ID: "uuid-c4", TaskKey: "legacy-team", ConfigJSON: `{"team_id":"uuid-team2"}`},

		{ID: "uuid-c5", TaskKey: "broken-team", ConfigJSON: `{"target_type":"team"}`},

		{ID: "uuid-c6", TaskKey: "bad-json", AgentID: "uuid-a1", ConfigJSON: `{oops`},

		{ID: "uuid-c7", TaskKey: "no-agent", ConfigJSON: `{}`},

		{ID: "uuid-c8", TaskKey: "dead", AgentID: "uuid-a1", DeletedAt: "2026-01-01T00:00:00Z"},
	}}
	edges := extractEdges(t, cronTaskExtractor{}, src)
	if e := findEdge(edges, EdgeTypeRuns, "uuid-c1", NodeTypeAgent, "uuid-a1"); e == nil || e.Evidence["target_type"] != "agent" {
		t.Fatalf("agent runs wrong: %+v", e)
	}
	if findEdge(edges, EdgeTypeRuns, "uuid-c2", NodeTypeTeam, "uuid-team1") == nil {
		t.Fatal("team runs missing")
	}
	if findEdge(edges, EdgeTypeRuns, "uuid-c4", NodeTypeTeam, "uuid-team2") == nil {
		t.Fatal("legacy team inference missing")
	}
	for _, id := range []string{"uuid-c3", "uuid-c8"} {
		for _, e := range edges {
			if e.SrcRef == id {
				t.Fatalf("%s must not emit edges: %+v", id, e)
			}
		}
	}
	for _, id := range []string{"uuid-c5", "uuid-c6", "uuid-c7"} {
		if !hasExtractError(edges, EdgeTypeRuns, id) {
			t.Fatalf("%s must yield extract_error marker", id)
		}
	}
}

func TestExtractor_Channel(t *testing.T) {
	cfg := `{"routing":{"default_agent_id":"ops_master","default_team_id":"uuid-team2","rules":[` +
		`{"peer_pattern":"u_*","agent_id":"uuid-a2"},` +
		`{"peer_pattern":"g_*","team_id":"uuid-team1"}]}}`
	src := &fakeSource{channels: []ChannelRow{
		{ID: "uuid-ch1", ChannelKey: "feishu", ConfigJSON: cfg},
		{ID: "uuid-ch2", ChannelKey: "empty"},
		{ID: "uuid-ch3", ChannelKey: "bad", ConfigJSON: `{oops`},
		{ID: "uuid-ch4", ChannelKey: "dead", ConfigJSON: cfg, DeletedAt: "2026-01-01T00:00:00Z"},
	}}
	edges := extractEdges(t, channelExtractor{}, src)
	if findEdge(edges, EdgeTypeRoutesTo, "uuid-ch1", NodeTypeAgent, "ops_master") == nil {
		t.Fatal("default agent (key) missing")
	}
	if findEdge(edges, EdgeTypeRoutesTo, "uuid-ch1", NodeTypeTeam, "uuid-team2") == nil {
		t.Fatal("default team missing")
	}
	e := findEdge(edges, EdgeTypeRoutesTo, "uuid-ch1", NodeTypeAgent, "uuid-a2")
	if e == nil || e.Evidence["peer_pattern"] != "u_*" {
		t.Fatalf("rule agent wrong: %+v", e)
	}
	if findEdge(edges, EdgeTypeRoutesTo, "uuid-ch1", NodeTypeTeam, "uuid-team1") == nil {
		t.Fatal("rule team missing")
	}
	if countType(edges, EdgeTypeRoutesTo) != 4 {
		t.Fatalf("want 4 routes_to, got %d", countType(edges, EdgeTypeRoutesTo))
	}
	if !hasExtractError(edges, EdgeTypeRoutesTo, "uuid-ch3") {
		t.Fatal("bad config must yield extract_error marker")
	}
	for _, e := range edges {
		if e.SrcRef == "uuid-ch4" {
			t.Fatal("deleted channel must not emit edges")
		}
	}
}

func TestResolveEdges_FullPipeline(t *testing.T) {
	src, prov := fullFixture()
	xs := Extractors(prov)

	var nodes []Node
	var edges []Edge
	for _, x := range xs {
		nodes = append(nodes, extractNodes(t, x, src)...)
		edges = append(edges, extractEdges(t, x, src)...)
	}
	idx := NewNodeIndex(nodes)
	gen := int64(1)
	now := parseTime(t, "2026-08-26T00:00:00Z")
	stored := ResolveEdges(edges, idx, gen, now)

	allTypes := []string{
		EdgeTypeHasMember, EdgeTypeSynthesizer, EdgeTypeIntentAnchor, EdgeTypeFallbackAgent,
		EdgeTypeDeptLead, EdgeTypeCrossDeptMember, EdgeTypeBelongsTo, EdgeTypeLinkedGraph,
		EdgeTypeGraphTemplate, EdgeTypeScopedKnowledge, EdgeTypeGraphAgent, EdgeTypeGraphTool,
		EdgeTypeGraphOwnedBy, EdgeTypeHasPromptFile, EdgeTypeGrantedTool, EdgeTypeToolOverride,
		EdgeTypeEnablesMCP, EdgeTypeOwnsSkill, EdgeTypeAllowsSkill, EdgeTypeSkillParent,
		EdgeTypeBoundPosition, EdgeTypeBoundPositionKey, EdgeTypeOrgDeptLead, EdgeTypeOrgParent,
		EdgeTypeRuns, EdgeTypeRoutesTo, EdgeTypeHookRef,
	}
	if len(allTypes) != 27 {
		t.Fatalf("test must track 27 edge types, got %d", len(allTypes))
	}
	byType := map[string][]StoredEdge{}
	for _, se := range stored {
		byType[se.Type] = append(byType[se.Type], se)
	}
	for _, typ := range allTypes {
		if len(byType[typ]) == 0 {
			t.Fatalf("edge type %s missing from full pipeline", typ)
		}
	}

	byID := map[string]Node{}
	for _, n := range nodes {
		byID[n.ID] = n
	}
	resolved := func(typ, srcRef, dstRef string) *StoredEdge {
		for i := range stored {
			se := &stored[i]
			if se.Type != typ || se.Broken() {
				continue
			}
			if byID[se.SrcID].RefID == srcRef && byID[se.DstID].RefID == dstRef {
				return se
			}
		}
		return nil
	}

	dualCases := []struct{ typ, src, dst string }{
		{EdgeTypeRoutesTo, "uuid-ch1", "uuid-a1"},
		{EdgeTypeGraphAgent, "uuid-g1", "uuid-a1"},
		{EdgeTypeGraphAgent, "uuid-g1", "uuid-a2"},
		{EdgeTypeGraphTool, "uuid-g1", "uuid-t1"},
		{EdgeTypeHookRef, "uuid-h1", "uuid-a1"},
		{EdgeTypeAllowsSkill, "uuid-a1", "uuid-s2"},
		{EdgeTypeEnablesMCP, "uuid-a1", "uuid-m1"},
		{EdgeTypeGrantedTool, "uuid-a1", "uuid-t1"},
		{EdgeTypeLinkedGraph, "uuid-team2", "uuid-g2"},
		{EdgeTypeRuns, "uuid-c2", "uuid-team1"},
	}
	for _, c := range dualCases {
		if resolved(c.typ, c.src, c.dst) == nil {
			t.Fatalf("resolved %s %s→%s missing", c.typ, c.src, c.dst)
		}
	}
	if se := resolved(EdgeTypeGrantedTool, "uuid-a1", "uuid-t2"); se == nil ||
		se.Evidence[EvidenceKeyGrantOrigin] != GrantOriginOverride {
		t.Fatalf("granted_tool override origin: %+v", se)
	}

	// ORPHAN position_key → broken（design 0.4 验收锚点）。
	var orphan *StoredEdge
	for i := range stored {
		se := &stored[i]
		if se.Type == EdgeTypeBoundPositionKey && se.Broken() {
			orphan = se
		}
	}
	if orphan == nil {
		t.Fatal("ORPHAN position_key must produce broken bound_position_key")
	}
	if orphan.Evidence[EvidenceKeyDstKey] != "ORPHAN-KEY" || orphan.DstID != "" {
		t.Fatalf("broken orphan edge wrong: %+v", orphan)
	}

	// provider 失败 → broken 标记边。
	var provBroken bool
	for i := range stored {
		se := &stored[i]
		if se.Type == EdgeTypeGrantedTool && se.Broken() {
			if msg, _ := se.Evidence["extract_error"].(string); strings.Contains(msg, "boom") {
				provBroken = true
			}
		}
	}
	if !provBroken {
		t.Fatal("provider failure must surface as broken granted_tool")
	}

	stored2 := ResolveEdges(edges, idx, gen, now)
	if len(stored) != len(stored2) {
		t.Fatalf("rerun size drift: %d vs %d", len(stored), len(stored2))
	}
	m1 := map[string]StoredEdge{}
	for _, se := range stored {
		m1[se.ID] = se
	}
	for _, se := range stored2 {
		prev, ok := m1[se.ID]
		if !ok {
			t.Fatalf("rerun produced new edge id %s", se.ID)
		}
		if prev.DstID != se.DstID || prev.Type != se.Type || prev.Broken() != se.Broken() {
			t.Fatalf("rerun mismatch for %s", se.ID)
		}
	}
}

func parseTime(t *testing.T, raw string) time.Time {
	t.Helper()
	ts, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		t.Fatal(err)
	}
	return ts
}
