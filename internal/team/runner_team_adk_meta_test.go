package team

import (
	"testing"

	"aranea-agents/internal/biz"
)

func Test_registerPersistMetaKeys_lookup_aliases(t *testing.T) {
	m := map[string]persistMetaEntry{}
	mem := MemberDef{AgentID: "id1", Role: "worker", Name: "显示名"}
	ag := biz.Agent{ID: "id1", AgentKey: "coder-bot", DisplayName: "Coder"}
	registerPersistMetaKeys(m, mem, ag)

	for _, k := range []string{"coder-bot", "coder", "显示名"} {
		if _, ok := m[k]; !ok {
			t.Fatalf("missing alias key %q, have %v", k, m)
		}
	}
	ent, ok := lookupPersistMeta("Coder", "", m, 2)
	if !ok || ent.Agent.ID != "id1" {
		t.Fatalf("lookup by display name: ok=%v ent=%+v", ok, ent)
	}
}

func Test_lookupPersistMeta_singleMember_fallback(t *testing.T) {
	m := map[string]persistMetaEntry{}
	mem := MemberDef{AgentID: "x", Name: "only"}
	ag := biz.Agent{ID: "x", AgentKey: "solo"}
	registerPersistMetaKeys(m, mem, ag)

	ent, ok := lookupPersistMeta("mystery_author", "", m, 1)
	if !ok || ent.Agent.AgentKey != "solo" {
		t.Fatalf("want single-member fallback, got ok=%v ent=%+v", ok, ent)
	}
	_, ok = lookupPersistMeta("mystery_author", "", m, 2)
	if ok {
		t.Fatal("should not fallback when team has multiple members")
	}
}

func Test_metaIsStreamLeaf(t *testing.T) {
	m := map[string]persistMetaEntry{}
	stream := MemberDef{AgentID: "s", Role: "synthesizer"}
	streamAg := biz.Agent{ID: "s", AgentKey: "synth", DisplayName: "Synth UI"}
	registerPersistMetaKeys(m, stream, streamAg)

	if !metaIsStreamLeaf("Synth UI", "synth", m, 2) {
		t.Fatal("display name should match stream leaf")
	}
	if metaIsStreamLeaf("other_agent_key", "synth", m, 2) {
		t.Fatal("unrelated author should not match stream leaf")
	}
}

func Test_registerWorkflowAuthorAliases_mapsLoopParentsToStreamLeaf(t *testing.T) {
	m := map[string]persistMetaEntry{}
	memA := MemberDef{AgentID: "a", Role: "worker"}
	memB := MemberDef{AgentID: "b", Role: "worker"}
	agA := biz.Agent{ID: "a", AgentKey: "agent-a"}
	agB := biz.Agent{ID: "b", AgentKey: "agent-b"}
	registerPersistMetaKeys(m, memA, agA)
	registerPersistMetaKeys(m, memB, agB)
	registerWorkflowAuthorAliases(m, []string{"team_loop_coordinator", "team_loop_body"}, "agent-b")

	for _, k := range []string{"team_loop_coordinator", "team_loop_body"} {
		ent, ok := lookupPersistMeta(k, "agent-b", m, 2)
		if !ok || ent.Agent.ID != "b" {
			t.Fatalf("key %q: ok=%v ent=%+v", k, ok, ent)
		}
	}
}

func Test_lookupPersistMeta_streamLeafFallbackWhenAuthorUnknown(t *testing.T) {
	m := map[string]persistMetaEntry{}
	memA := MemberDef{AgentID: "a", Role: "worker"}
	memB := MemberDef{AgentID: "b", Role: "worker"}
	agA := biz.Agent{ID: "a", AgentKey: "agent-a"}
	agB := biz.Agent{ID: "b", AgentKey: "agent-b"}
	registerPersistMetaKeys(m, memA, agA)
	registerPersistMetaKeys(m, memB, agB)
	registerWorkflowAuthorAliases(m, []string{"team_sequential"}, "agent-b")

	_, ok := lookupPersistMeta("mystery_final_author", "agent-b", m, 2)
	if ok {
		t.Fatal("unknown author should not resolve without non-empty text fallback path")
	}
}
