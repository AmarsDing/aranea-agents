package team

import "testing"

func TestLinkedGraphIDFromDefinition(t *testing.T) {
	raw := `{"version":1,"mode":"sequential","linked_graph_id":"graph-1","members":[]}`
	if got := LinkedGraphIDFromDefinition(raw); got != "graph-1" {
		t.Fatalf("got %q", got)
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
