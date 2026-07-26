package agent

import (
	"testing"

	"aranea-agents/internal/biz"
)

func TestStreamSubTaskParser_SingleObject(t *testing.T) {
	p := newStreamSubTaskParser()
	got := p.Feed(`[{"id":"st_1","name":"Research","depends_on":[]}]`)
	if len(got) != 1 {
		t.Fatalf("expected 1 complete object, got %d", len(got))
	}
	if got[0] != `{"id":"st_1","name":"Research","depends_on":[]}` {
		t.Errorf("unexpected object: %s", got[0])
	}
}

func TestStreamSubTaskParser_MultipleObjects(t *testing.T) {
	p := newStreamSubTaskParser()
	got := p.Feed(`[{"id":"st_1","name":"A","depends_on":[]},{"id":"st_2","name":"B","depends_on":["st_1"]}]`)
	if len(got) != 2 {
		t.Fatalf("expected 2 complete objects, got %d", len(got))
	}
	if got[0] != `{"id":"st_1","name":"A","depends_on":[]}` {
		t.Errorf("object 0: %s", got[0])
	}
	if got[1] != `{"id":"st_2","name":"B","depends_on":["st_1"]}` {
		t.Errorf("object 1: %s", got[1])
	}
}

func TestStreamSubTaskParser_ChunkedDelivery(t *testing.T) {
	p := newStreamSubTaskParser()
	// Feed in small chunks — simulates streaming LLM output.
	chunks := []string{
		`[{"id":"st_1",`,
		`"name":"Res`,
		`earch","depends`,
		`_on":[]},{"id":`,
		`"st_2","name":"B",`,
		`"depends_on":["st_1"]}]`,
	}
	var all []string
	for _, c := range chunks {
		all = append(all, p.Feed(c)...)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 complete objects, got %d: %v", len(all), all)
	}
	if all[0] != `{"id":"st_1","name":"Research","depends_on":[]}` {
		t.Errorf("object 0: %s", all[0])
	}
}

func TestStreamSubTaskParser_NestedBraces(t *testing.T) {
	p := newStreamSubTaskParser()
	got := p.Feed(`[{"id":"st_1","name":"A","deliverables":[{"name":"doc","type":"document"}],"depends_on":[]}]`)
	if len(got) != 1 {
		t.Fatalf("expected 1 complete object, got %d", len(got))
	}
}

func TestStreamSubTaskParser_StringWithBraces(t *testing.T) {
	p := newStreamSubTaskParser()
	got := p.Feed(`[{"id":"st_1","name":"A } { test","depends_on":[]}]`)
	if len(got) != 1 {
		t.Fatalf("expected 1 complete object, got %d", len(got))
	}
	if got[0] != `{"id":"st_1","name":"A } { test","depends_on":[]}` {
		t.Errorf("unexpected: %s", got[0])
	}
}

func TestStreamSubTaskParser_EscapedQuotes(t *testing.T) {
	p := newStreamSubTaskParser()
	got := p.Feed(`[{"id":"st_1","name":"A \"quoted\" value","depends_on":[]}]`)
	if len(got) != 1 {
		t.Fatalf("expected 1 complete object, got %d", len(got))
	}
}

func TestStreamSubTaskParser_NoCompleteObject(t *testing.T) {
	p := newStreamSubTaskParser()
	got := p.Feed(`[{"id":"st_1","name":"A"`)
	if len(got) != 0 {
		t.Fatalf("expected 0 complete objects, got %d", len(got))
	}
}

func TestStreamSubTaskParser_EmptyInput(t *testing.T) {
	p := newStreamSubTaskParser()
	got := p.Feed(``)
	if len(got) != 0 {
		t.Fatalf("expected 0, got %d", len(got))
	}
}

func TestStreamSubTaskParser_PrefixText(t *testing.T) {
	p := newStreamSubTaskParser()
	// LLM sometimes outputs text before the JSON array.
	got := p.Feed(`Here is the decomposition:\n[{"id":"st_1","name":"A","depends_on":[]}]`)
	if len(got) != 1 {
		t.Fatalf("expected 1, got %d", len(got))
	}
}

// --- parseStreamSubTask tests ---

func TestParseStreamSubTask_Basic(t *testing.T) {
	idRemap := make(map[string]string)
	st, err := parseStreamSubTask(`{"id":"st_1","name":"Research","description":"Do research","depends_on":[],"required_capabilities":["research"],"priority":1,"estimated_complexity":0.5}`, idRemap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if st.Name != "Research" {
		t.Errorf("expected name=Research, got %s", st.Name)
	}
	if st.ID == "st_1" {
		t.Error("ID should be remapped from st_1 to st_<uuid>")
	}
	if idRemap["st_1"] != st.ID {
		t.Errorf("idRemap[st_1] = %s, want %s", idRemap["st_1"], st.ID)
	}
}

func TestParseStreamSubTask_DependencyResolution(t *testing.T) {
	idRemap := make(map[string]string)
	st1, err := parseStreamSubTask(`{"id":"st_1","name":"A","depends_on":[]}`, idRemap)
	if err != nil {
		t.Fatal(err)
	}
	st2, err := parseStreamSubTask(`{"id":"st_2","name":"B","depends_on":["st_1"]}`, idRemap)
	if err != nil {
		t.Fatal(err)
	}
	if len(st2.DependsOn) != 1 || st2.DependsOn[0] != st1.ID {
		t.Errorf("st2.DependsOn = %v, want [%s]", st2.DependsOn, st1.ID)
	}
}

func TestParseStreamSubTask_ForwardReferenceSkipped(t *testing.T) {
	idRemap := make(map[string]string)
	// st_2 depends on st_3 which hasn't been parsed yet.
	st, err := parseStreamSubTask(`{"id":"st_2","name":"B","depends_on":["st_3"]}`, idRemap)
	if err != nil {
		t.Fatal(err)
	}
	// Forward reference should be skipped (empty DependsOn).
	if len(st.DependsOn) != 0 {
		t.Errorf("expected empty DependsOn for forward ref, got %v", st.DependsOn)
	}
}

func TestParseStreamSubTask_EmptyID(t *testing.T) {
	idRemap := make(map[string]string)
	_, err := parseStreamSubTask(`{"id":"","name":"A"}`, idRemap)
	if err == nil {
		t.Error("expected error for empty id")
	}
}

func TestParseStreamSubTask_InvalidJSON(t *testing.T) {
	idRemap := make(map[string]string)
	_, err := parseStreamSubTask(`{invalid}`, idRemap)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

// --- resolveForwardRefs tests ---

func TestResolveForwardRefs_RemovesInvalidRefs(t *testing.T) {
	sts := []biz.SubTask{
		{ID: "st_a", Name: "A", DependsOn: []string{"st_b", "st_invalid"}},
		{ID: "st_b", Name: "B", DependsOn: []string{}},
	}
	resolveForwardRefs(sts)
	if len(sts[0].DependsOn) != 1 || sts[0].DependsOn[0] != "st_b" {
		t.Errorf("expected [st_b], got %v", sts[0].DependsOn)
	}
}

func TestResolveForwardRefs_EmptyDependsOn(t *testing.T) {
	sts := []biz.SubTask{
		{ID: "st_a", Name: "A", DependsOn: []string{}},
	}
	resolveForwardRefs(sts)
	if len(sts[0].DependsOn) != 0 {
		t.Errorf("expected empty, got %v", sts[0].DependsOn)
	}
}
