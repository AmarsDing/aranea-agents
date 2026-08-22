package team

import "testing"

func TestLinkedGraphIDFromDefinition(t *testing.T) {
	raw := `{"version":1,"mode":"sequential","linked_graph_id":"graph-1","members":[]}`
	if got := LinkedGraphIDFromDefinition(raw); got != "graph-1" {
		t.Fatalf("got %q", got)
	}
}

func TestStageGraphAssetIDPrefersTemplateWhenNotBuiltin(t *testing.T) {
	raw := `{"version":1,"mode":"sequential","graph_template_id":"g-asset-9","members":[]}`
	if got := StageGraphAssetID("", raw); got != "g-asset-9" {
		t.Fatalf("got %q", got)
	}
	builtin := `{"graph_template_id":"parallel_review"}`
	if got := StageGraphAssetID("", builtin); got != "" {
		t.Fatalf("builtin must not be an asset id: %q", got)
	}
	if CompileTemplateIDPreferringDefinition("sequential", builtin) != "parallel_review" {
		t.Fatalf("compile template=%q", CompileTemplateIDPreferringDefinition("sequential", builtin))
	}
}

func TestCollectionIDsFromDefinition(t *testing.T) {
	raw := `{"version":1,"mode":"sequential","collection_ids":[" kb-1 ","","kb-1","kb-2"],"members":[]}`
	got := CollectionIDsFromDefinition(raw)
	if len(got) != 2 || got[0] != "kb-1" || got[1] != "kb-2" {
		t.Fatalf("got %v", got)
	}
	if len(CollectionIDsFromDefinition(`{"mode":"sequential"}`)) != 0 {
		t.Fatal("empty must stay empty")
	}
}

func TestMergeLinkedGraphID(t *testing.T) {
	raw := `{"version":1,"mode":"sequential","members":[]}`
	merged, err := MergeLinkedGraphID(raw, "g-99")
	if err != nil {
		t.Fatal(err)
	}
	if LinkedGraphIDFromDefinition(merged) != "g-99" {
		t.Fatalf("merge failed: %s", merged)
	}
	cleared, err := MergeLinkedGraphID(merged, "")
	if err != nil {
		t.Fatal(err)
	}
	if LinkedGraphIDFromDefinition(cleared) != "" {
		t.Fatalf("clear failed: %s", cleared)
	}
}
